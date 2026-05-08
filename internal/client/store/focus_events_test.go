// internal/client/store/focus_events_test.go
package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertFocusEvent(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	id, err := s.InsertFocusEvent("Firefox", "GitHub - trasker", now)
	if err != nil {
		t.Fatalf("InsertFocusEvent() error: %v", err)
	}
	if id < 1 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	ev, err := s.GetFocusEvent(id)
	if err != nil {
		t.Fatalf("GetFocusEvent() error: %v", err)
	}
	if ev.AppName != "Firefox" {
		t.Errorf("AppName = %q, want %q", ev.AppName, "Firefox")
	}
	if ev.WindowTitle != "GitHub - trasker" {
		t.Errorf("WindowTitle = %q, want %q", ev.WindowTitle, "GitHub - trasker")
	}
	if ev.EndedAt != nil {
		t.Errorf("EndedAt = %v, want nil (still current)", ev.EndedAt)
	}
}

func TestEndCurrentFocusEvent(t *testing.T) {
	s := newTestStore(t)

	start := time.Now().UTC()
	id, err := s.InsertFocusEvent("Code", "main.go", start)
	if err != nil {
		t.Fatalf("InsertFocusEvent() error: %v", err)
	}

	end := start.Add(5 * time.Minute)
	err = s.EndFocusEvent(id, end)
	if err != nil {
		t.Fatalf("EndFocusEvent() error: %v", err)
	}

	ev, err := s.GetFocusEvent(id)
	if err != nil {
		t.Fatalf("GetFocusEvent() error: %v", err)
	}
	if ev.EndedAt == nil {
		t.Fatal("EndedAt is nil after EndFocusEvent()")
	}
	if ev.DurationS == nil || *ev.DurationS != 300 {
		t.Errorf("DurationS = %v, want 300", ev.DurationS)
	}
}

func TestListFocusEventsByTimeRange(t *testing.T) {
	s := newTestStore(t)

	base := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)

	// Insert 3 events: 09:00, 09:30, 10:00
	for i, offset := range []time.Duration{0, 30 * time.Minute, 60 * time.Minute} {
		id, err := s.InsertFocusEvent("App", "Window", base.Add(offset))
		if err != nil {
			t.Fatalf("InsertFocusEvent(%d) error: %v", i, err)
		}
		if i < 2 {
			_ = s.EndFocusEvent(id, base.Add(offset).Add(25*time.Minute))
		}
	}

	// Query 09:00 to 09:45 — should get 2 events
	events, err := s.ListFocusEvents(base, base.Add(45*time.Minute))
	if err != nil {
		t.Fatalf("ListFocusEvents() error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
}

func TestMarkFocusEventIdle(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	id, err := s.InsertFocusEvent("idle", "", now)
	if err != nil {
		t.Fatalf("InsertFocusEvent() error: %v", err)
	}

	err = s.MarkFocusEventIdle(id)
	if err != nil {
		t.Fatalf("MarkFocusEventIdle() error: %v", err)
	}

	ev, err := s.GetFocusEvent(id)
	if err != nil {
		t.Fatalf("GetFocusEvent() error: %v", err)
	}
	if !ev.IsIdle {
		t.Error("IsIdle = false, want true")
	}
}

func TestGetFocusEvent_NotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetFocusEvent(9999)
	if err == nil {
		t.Fatal("expected error for non-existent event, got nil")
	}
}

// TestCloseLatestOpenFocusEvent_OnlyClosesOpenRow verifies that the subquery
// form of UPDATE used in CloseLatestOpenFocusEvent (Bug-2 fix) targets ONLY
// the most recent open row (ended_at IS NULL) and does NOT touch already-closed
// rows. Regression test for #PHASE-09-BUG-2 where the original SQL used
// `UPDATE ... ORDER BY id DESC LIMIT 1`, which modernc.org/sqlite rejects
// with `near "ORDER": syntax error`.
func TestCloseLatestOpenFocusEvent_OnlyClosesOpenRow(t *testing.T) {
	s := newTestStore(t)

	base := time.Date(2026, 5, 8, 9, 0, 0, 0, time.UTC)

	// Row 1: already closed (insert + end).
	closedID, err := s.InsertFocusEvent("Editor", "main.go", base)
	if err != nil {
		t.Fatalf("InsertFocusEvent (closed): %v", err)
	}
	closedEnd := base.Add(2 * time.Minute)
	if err := s.EndFocusEvent(closedID, closedEnd); err != nil {
		t.Fatalf("EndFocusEvent: %v", err)
	}

	// Row 2: still open (no ended_at).
	openID, err := s.InsertFocusEvent("Browser", "GitHub", base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("InsertFocusEvent (open): %v", err)
	}

	// Close the latest open row.
	closeAt := base.Add(8 * time.Minute)
	n, err := store.CloseLatestOpenFocusEvent(s.DB(), closeAt)
	if err != nil {
		t.Fatalf("CloseLatestOpenFocusEvent: %v", err)
	}
	if n != 1 {
		t.Fatalf("RowsAffected = %d, want 1", n)
	}

	// Row 2 should now be closed with the expected duration.
	openEv, err := s.GetFocusEvent(openID)
	if err != nil {
		t.Fatalf("GetFocusEvent(open): %v", err)
	}
	if openEv.EndedAt == nil {
		t.Fatal("open row not closed: EndedAt is nil")
	}
	if !openEv.EndedAt.Equal(closeAt) {
		t.Errorf("EndedAt = %v, want %v", openEv.EndedAt, closeAt)
	}
	if openEv.DurationS == nil || *openEv.DurationS != 300 {
		t.Errorf("DurationS = %v, want 300 (5 minutes)", openEv.DurationS)
	}

	// Row 1 must be unchanged.
	closedEv, err := s.GetFocusEvent(closedID)
	if err != nil {
		t.Fatalf("GetFocusEvent(closed): %v", err)
	}
	if closedEv.EndedAt == nil || !closedEv.EndedAt.Equal(closedEnd) {
		t.Errorf("closed row changed: EndedAt = %v, want %v", closedEv.EndedAt, closedEnd)
	}
	if closedEv.DurationS == nil || *closedEv.DurationS != 120 {
		t.Errorf("closed row DurationS = %v, want 120", closedEv.DurationS)
	}
}

// TestCloseLatestOpenFocusEvent_NoOpenRow verifies that calling Close when
// there is no open row is a safe no-op (returns 0 rows affected, no error).
func TestCloseLatestOpenFocusEvent_NoOpenRow(t *testing.T) {
	s := newTestStore(t)

	// Insert + close one row so there are no open rows.
	id, err := s.InsertFocusEvent("App", "Win", time.Now().UTC())
	if err != nil {
		t.Fatalf("InsertFocusEvent: %v", err)
	}
	if err := s.EndFocusEvent(id, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatalf("EndFocusEvent: %v", err)
	}

	n, err := store.CloseLatestOpenFocusEvent(s.DB(), time.Now())
	if err != nil {
		t.Fatalf("CloseLatestOpenFocusEvent: %v", err)
	}
	if n != 0 {
		t.Errorf("RowsAffected = %d, want 0", n)
	}
}

// TestCloseLatestOpenFocusEvent_ClosesOnlyMostRecentOpen verifies that when
// multiple open rows exist (a state we don't intentionally produce, but want
// to be robust against), only the most recent (highest id) is closed.
func TestCloseLatestOpenFocusEvent_ClosesOnlyMostRecentOpen(t *testing.T) {
	s := newTestStore(t)

	base := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	older, err := s.InsertFocusEvent("App1", "W1", base)
	if err != nil {
		t.Fatalf("InsertFocusEvent older: %v", err)
	}
	newer, err := s.InsertFocusEvent("App2", "W2", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("InsertFocusEvent newer: %v", err)
	}

	n, err := store.CloseLatestOpenFocusEvent(s.DB(), base.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("CloseLatestOpenFocusEvent: %v", err)
	}
	if n != 1 {
		t.Fatalf("RowsAffected = %d, want 1", n)
	}

	olderEv, _ := s.GetFocusEvent(older)
	newerEv, _ := s.GetFocusEvent(newer)
	if olderEv.EndedAt != nil {
		t.Error("older open row was unexpectedly closed")
	}
	if newerEv.EndedAt == nil {
		t.Error("newer open row was NOT closed")
	}
}
