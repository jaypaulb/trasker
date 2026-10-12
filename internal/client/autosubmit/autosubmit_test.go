// internal/client/autosubmit/autosubmit_test.go
package autosubmit

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/store"
)

var t0 = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "client.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s.DB()
}

func insertTag(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO tags (name, color, created_at) VALUES (?, '#000', ?)`, name, t0.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// insertEvent adds a focus event and, when tagID > 0, tags it.
func insertEvent(t *testing.T, db *sql.DB, started time.Time, ended *time.Time, idle int, tagID int64) int64 {
	t.Helper()
	var endedStr any
	if ended != nil {
		endedStr = ended.Format(time.RFC3339)
	}
	res, err := db.Exec(
		`INSERT INTO focus_events (app_name, window_title, started_at, ended_at, is_idle, created_at)
		 VALUES ('code', 'x', ?, ?, ?, ?)`,
		started.Format(time.RFC3339), endedStr, idle, started.Format(time.RFC3339),
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if tagID > 0 {
		if _, err := db.Exec(`INSERT INTO event_tags (event_id, tag_id, source) VALUES (?, ?, 'manual')`, id, tagID); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func endAt(d time.Duration) *time.Time {
	e := t0.Add(d)
	return &e
}

func submittedEventIDs(t *testing.T, db *sql.DB) map[int64]bool {
	t.Helper()
	rows, err := db.Query(`SELECT event_id FROM submission_events`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got[id] = true
	}
	return got
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	db := setupDB(t)
	tagA := insertTag(t, db, "client-a")
	tagB := insertTag(t, db, "client-b")

	if err := Save(db, 4, []int64{tagB, tagA}, t0); err != nil {
		t.Fatal(err)
	}
	s, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Enabled() || s.IntervalHours != 4 {
		t.Fatalf("want enabled 4h, got %+v", s)
	}
	if len(s.TagIDs) != 2 || s.TagIDs[0] != tagA || s.TagIDs[1] != tagB {
		t.Fatalf("tag ids: got %v", s.TagIDs)
	}
	if s.EnabledAt == nil || !s.EnabledAt.Equal(t0) {
		t.Fatalf("enabled_at: got %v want %v", s.EnabledAt, t0)
	}
	if next := s.NextRun(); next == nil || !next.Equal(t0.Add(4*time.Hour)) {
		t.Fatalf("next run: got %v", next)
	}
}

func TestSave_ChangingCadenceKeepsCycle(t *testing.T) {
	db := setupDB(t)
	tag := insertTag(t, db, "client-a")

	if err := Save(db, 4, []int64{tag}, t0); err != nil {
		t.Fatal(err)
	}
	later := t0.Add(2 * time.Hour)
	if err := Save(db, 8, []int64{tag}, later); err != nil {
		t.Fatal(err)
	}
	s, _ := Load(db)
	if !s.EnabledAt.Equal(t0) {
		t.Fatalf("enabled_at moved on edit: got %v want %v", s.EnabledAt, t0)
	}
	if !s.LastRun.Equal(t0) {
		t.Fatalf("last_run moved on edit: got %v want %v", s.LastRun, t0)
	}
}

func TestSave_OffClearsCycle(t *testing.T) {
	db := setupDB(t)
	tag := insertTag(t, db, "client-a")

	if err := Save(db, 4, []int64{tag}, t0); err != nil {
		t.Fatal(err)
	}
	if err := Save(db, 0, nil, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	s, _ := Load(db)
	if s.Enabled() || s.EnabledAt != nil || s.LastRun != nil {
		t.Fatalf("want off with no cycle, got %+v", s)
	}
}

func TestSave_RejectsUnofferedInterval(t *testing.T) {
	db := setupDB(t)
	if err := Save(db, 3, nil, t0); err == nil {
		t.Fatal("want error for 3h interval")
	}
}

func TestSave_UnknownTagRollsBack(t *testing.T) {
	db := setupDB(t)
	tag := insertTag(t, db, "client-a")

	err := Save(db, 4, []int64{tag, 9999}, t0)
	if !errors.Is(err, ErrUnknownTag) {
		t.Fatalf("want ErrUnknownTag, got %v", err)
	}
	s, _ := Load(db)
	if s.Enabled() || len(s.TagIDs) != 0 {
		t.Fatalf("failed save must not persist, got %+v", s)
	}
}

func TestRunOnce_WaitsForInterval_ThenSubmitsTickedTagsOnly(t *testing.T) {
	db := setupDB(t)
	ticked := insertTag(t, db, "client-a")
	unticked := insertTag(t, db, "client-b")
	if err := Save(db, 4, []int64{ticked}, t0); err != nil {
		t.Fatal(err)
	}

	// Enabled at t0, so these start after the cutoff.
	want := insertEvent(t, db, t0.Add(10*time.Minute), endAt(20*time.Minute), 0, ticked)
	insertEvent(t, db, t0.Add(30*time.Minute), endAt(40*time.Minute), 0, unticked)

	if subID, n, err := RunOnce(db, t0.Add(3*time.Hour)); err != nil || subID != 0 || n != 0 {
		t.Fatalf("before interval: subID=%d n=%d err=%v", subID, n, err)
	}

	subID, n, err := RunOnce(db, t0.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if subID == 0 || n != 1 {
		t.Fatalf("want one submission with one event, got subID=%d n=%d", subID, n)
	}
	got := submittedEventIDs(t, db)
	if len(got) != 1 || !got[want] {
		t.Fatalf("submitted events: got %v want only %d", got, want)
	}
	s, _ := Load(db)
	if !s.LastRun.Equal(t0.Add(4 * time.Hour)) {
		t.Fatalf("last_run not advanced: %v", s.LastRun)
	}
}

func TestRunOnce_SkipsOpenIdleBeforeCutoffAndAlreadySubmitted(t *testing.T) {
	db := setupDB(t)
	tag := insertTag(t, db, "client-a")

	// Before enabling: must never be picked up.
	insertEvent(t, db, t0.Add(-time.Hour), endAt(-30*time.Minute), 0, tag)
	if err := Save(db, 1, []int64{tag}, t0); err != nil {
		t.Fatal(err)
	}
	// Still open (no ended_at): wait for a later run.
	open := insertEvent(t, db, t0.Add(5*time.Minute), nil, 0, tag)
	// Idle: skipped like the manual submit page does.
	insertEvent(t, db, t0.Add(6*time.Minute), endAt(7*time.Minute), 1, tag)
	// Finished and eligible.
	done := insertEvent(t, db, t0.Add(8*time.Minute), endAt(9*time.Minute), 0, tag)

	if _, n, err := RunOnce(db, t0.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("first run: n=%d err=%v", n, err)
	}
	got := submittedEventIDs(t, db)
	if len(got) != 1 || !got[done] || got[open] {
		t.Fatalf("submitted events: got %v want only %d", got, done)
	}

	// Next cycle: the open event has now finished; the already-submitted one is not repeated.
	if _, err := db.Exec(`UPDATE focus_events SET ended_at = ? WHERE id = ?`,
		t0.Add(70*time.Minute).Format(time.RFC3339), open); err != nil {
		t.Fatal(err)
	}
	subID, n, err := RunOnce(db, t0.Add(2*time.Hour))
	if err != nil || subID == 0 || n != 1 {
		t.Fatalf("second run: subID=%d n=%d err=%v", subID, n, err)
	}
	if got := submittedEventIDs(t, db); !got[open] || len(got) != 2 {
		t.Fatalf("after second run: got %v", got)
	}
}

func TestRunOnce_DisabledDoesNothing(t *testing.T) {
	db := setupDB(t)
	tag := insertTag(t, db, "client-a")
	insertEvent(t, db, t0.Add(time.Minute), endAt(2*time.Minute), 0, tag)

	if subID, n, err := RunOnce(db, t0.Add(48*time.Hour)); err != nil || subID != 0 || n != 0 {
		t.Fatalf("disabled: subID=%d n=%d err=%v", subID, n, err)
	}
}
