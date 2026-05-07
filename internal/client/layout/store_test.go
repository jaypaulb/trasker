package layout_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/layout"
	clientstore "github.com/jaypaulb/trasker/internal/client/store"
)

// newLayoutTestStore opens a fresh client SQLite, runs the shared
// migrate() (which creates layout_snapshots), and returns a layout.Store
// wired to the same *sql.DB.
func newLayoutTestStore(t *testing.T) *layout.Store {
	t.Helper()
	dir := t.TempDir()
	cs, err := clientstore.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("clientstore.New() error: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return layout.NewStore(cs.DB())
}

func sampleWindows() []layout.Window {
	return []layout.Window{
		{AppName: "Firefox", WindowTitle: "GitHub", X: 0, Y: 0, W: 1920, H: 1080},
		{AppName: "Code", WindowTitle: "main.go", X: 100, Y: 50, W: 1280, H: 800},
	}
}

func sampleJSON(t *testing.T, ws []layout.Window) []byte {
	t.Helper()
	b, err := json.Marshal(ws)
	if err != nil {
		t.Fatalf("marshal sample windows: %v", err)
	}
	return b
}

// TestStore_RoundTrip: insert one snapshot, read it back via ListPending,
// assert all fields match.
func TestStore_RoundTrip(t *testing.T) {
	s := newLayoutTestStore(t)

	ws := sampleWindows()
	hash := layout.HashWindows(ws)
	captured := time.Now().UTC().Truncate(time.Second)

	id, err := s.InsertSnapshot(captured, sampleJSON(t, ws), hash)
	if err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}
	if id < 1 {
		t.Fatalf("expected positive id, got %d", id)
	}

	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("got %d pending rows, want 1", len(pending))
	}
	got := pending[0]
	if got.ID != id {
		t.Errorf("ID = %d, want %d", got.ID, id)
	}
	if got.WindowsHash != hash {
		t.Errorf("WindowsHash = %q, want %q", got.WindowsHash, hash)
	}
	if !got.CapturedAt.Equal(captured) {
		t.Errorf("CapturedAt = %v, want %v", got.CapturedAt, captured)
	}
	if len(got.Windows) != len(ws) {
		t.Fatalf("Windows len = %d, want %d", len(got.Windows), len(ws))
	}
	if got.Windows[0] != ws[0] {
		t.Errorf("Windows[0] = %+v, want %+v", got.Windows[0], ws[0])
	}
	if got.SyncedAt != nil {
		t.Errorf("SyncedAt = %v, want nil (still pending)", got.SyncedAt)
	}
}

// TestStore_ListPending_OnlyUnsynced: insert two; mark one synced;
// ListPending returns only the unsynced row.
func TestStore_ListPending_OnlyUnsynced(t *testing.T) {
	s := newLayoutTestStore(t)

	ws := sampleWindows()
	now := time.Now().UTC().Truncate(time.Second)

	id1, err := s.InsertSnapshot(now, sampleJSON(t, ws), "hash-1")
	if err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	id2, err := s.InsertSnapshot(now.Add(time.Minute), sampleJSON(t, ws), "hash-2")
	if err != nil {
		t.Fatalf("insert 2: %v", err)
	}

	if err := s.MarkSynced(id1, time.Now().UTC()); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}

	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("got %d pending rows, want 1", len(pending))
	}
	if pending[0].ID != id2 {
		t.Errorf("pending[0].ID = %d, want %d", pending[0].ID, id2)
	}
}

// TestStore_MarkSynced: subsequent ListPending returns 0 rows.
func TestStore_MarkSynced(t *testing.T) {
	s := newLayoutTestStore(t)

	ws := sampleWindows()
	id, err := s.InsertSnapshot(time.Now().UTC(), sampleJSON(t, ws), "hash-1")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := s.MarkSynced(id, time.Now().UTC()); err != nil {
		t.Fatalf("MarkSynced: %v", err)
	}
	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("got %d pending rows, want 0", len(pending))
	}
}

// TestStore_Prune7Days: rows at -8d, -3d, now. Prune(7d) deletes the -8d
// row only; ListInRange(-30d, now) returns 2 rows.
func TestStore_Prune7Days(t *testing.T) {
	s := newLayoutTestStore(t)

	ws := sampleWindows()
	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-8 * 24 * time.Hour)
	mid := now.Add(-3 * 24 * time.Hour)

	for i, captured := range []time.Time{old, mid, now} {
		_, err := s.InsertSnapshot(captured, sampleJSON(t, ws), "hash")
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	deleted, err := s.Prune(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Prune deleted %d, want 1", deleted)
	}

	in, err := s.ListInRange(now.Add(-30*24*time.Hour), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ListInRange: %v", err)
	}
	if len(in) != 2 {
		t.Errorf("ListInRange returned %d rows, want 2", len(in))
	}
}

// TestStore_ListInRange: t-2h, t-1h, t. Range(t-90m, t-30m) returns the
// t-1h row only (boundary inclusive on both ends).
func TestStore_ListInRange(t *testing.T) {
	s := newLayoutTestStore(t)

	ws := sampleWindows()
	now := time.Now().UTC().Truncate(time.Second)

	for _, captured := range []time.Time{
		now.Add(-2 * time.Hour),
		now.Add(-1 * time.Hour),
		now,
	} {
		if _, err := s.InsertSnapshot(captured, sampleJSON(t, ws), "hash"); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	in, err := s.ListInRange(now.Add(-90*time.Minute), now.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("ListInRange: %v", err)
	}
	if len(in) != 1 {
		t.Fatalf("got %d rows, want 1", len(in))
	}
	want := now.Add(-1 * time.Hour)
	if !in[0].CapturedAt.Equal(want) {
		t.Errorf("CapturedAt = %v, want %v", in[0].CapturedAt, want)
	}
}
