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
