// internal/client/session/engine_test.go
package session_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/presence"
	"github.com/jaypaulb/trasker/internal/client/session"
	"github.com/jaypaulb/trasker/internal/client/store"
	"github.com/jaypaulb/trasker/internal/client/tracker"
)

// --- Mock Tracker ---

type mockTracker struct {
	events chan tracker.FocusChange
}

func newMockTracker() *mockTracker {
	return &mockTracker{events: make(chan tracker.FocusChange, 10)}
}

func (m *mockTracker) Start(ctx context.Context) error { return nil }
func (m *mockTracker) Stop()                           { close(m.events) }
func (m *mockTracker) Events() <-chan tracker.FocusChange {
	return m.events
}
func (m *mockTracker) Emit(app, title string) {
	m.events <- tracker.FocusChange{
		AppName:     app,
		WindowTitle: title,
		Timestamp:   time.Now().UTC(),
	}
}

var _ tracker.Tracker = (*mockTracker)(nil)

// --- Mock Presence Detector ---

type mockPresence struct {
	states chan presence.StateChange
}

func newMockPresence() *mockPresence {
	return &mockPresence{states: make(chan presence.StateChange, 10)}
}

func (m *mockPresence) Start(ctx context.Context) error    { return nil }
func (m *mockPresence) Stop()                              {}
func (m *mockPresence) States() <-chan presence.StateChange { return m.states }
func (m *mockPresence) AcknowledgeCheck()                  {}
func (m *mockPresence) ResetOnFocusChange()                {}

func (m *mockPresence) Emit(state presence.State) {
	m.states <- presence.StateChange{
		State:     state,
		Timestamp: time.Now().UTC(),
	}
}

var _ presence.PresenceDetector = (*mockPresence)(nil)

// --- Helpers ---

func newTestEngine(t *testing.T) (*session.Engine, *mockTracker, *mockPresence, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.New() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	mt := newMockTracker()
	mp := newMockPresence()

	eng := session.NewEngine(s, mt, mp, slog.Default())
	return eng, mt, mp, s
}

// --- Tests ---

func TestEngine_RecordsFocusEvents(t *testing.T) {
	eng, mt, _, s := newTestEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng.Start(ctx)

	// Emit two focus changes
	mt.Emit("Firefox", "GitHub")
	time.Sleep(50 * time.Millisecond)
	mt.Emit("Code", "main.go")
	time.Sleep(50 * time.Millisecond)

	cancel()
	eng.Wait()

	// Query all events in a wide range
	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)
	events, err := s.ListFocusEvents(from, to)
	if err != nil {
		t.Fatalf("ListFocusEvents() error: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("got %d events, want at least 2", len(events))
	}

	// First event should be ended (because second event started)
	if events[0].EndedAt == nil {
		t.Error("first event should have EndedAt set")
	}
	if events[0].AppName != "Firefox" {
		t.Errorf("first event AppName = %q, want %q", events[0].AppName, "Firefox")
	}
}

func TestEngine_MarksIdleOnAway(t *testing.T) {
	eng, mt, mp, s := newTestEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng.Start(ctx)

	// Start tracking
	mt.Emit("Firefox", "GitHub")
	time.Sleep(50 * time.Millisecond)

	// Screen lock
	mp.Emit(presence.Away)
	time.Sleep(50 * time.Millisecond)

	cancel()
	eng.Wait()

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)
	events, _ := s.ListFocusEvents(from, to)

	if len(events) < 1 {
		t.Fatalf("got %d events, want at least 1", len(events))
	}

	// The last event should be ended and marked idle
	last := events[len(events)-1]
	if last.EndedAt == nil {
		t.Error("event should be ended on AWAY")
	}
}

func TestEngine_PausesAndResumes(t *testing.T) {
	eng, mt, mp, s := newTestEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng.Start(ctx)

	// Start tracking
	mt.Emit("Firefox", "GitHub")
	time.Sleep(50 * time.Millisecond)

	// Pause (deadman expired)
	mp.Emit(presence.Paused)
	time.Sleep(50 * time.Millisecond)

	// Resume
	mp.Emit(presence.Tracking)
	time.Sleep(50 * time.Millisecond)

	// New focus after resume
	mt.Emit("Code", "main.go")
	time.Sleep(50 * time.Millisecond)

	cancel()
	eng.Wait()

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)
	events, _ := s.ListFocusEvents(from, to)

	// Should have at least 2 events (before pause, after resume)
	if len(events) < 2 {
		t.Fatalf("got %d events, want at least 2", len(events))
	}
	_ = s
}

func TestEngine_IgnoresFocusWhilePaused(t *testing.T) {
	eng, mt, mp, s := newTestEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng.Start(ctx)

	mt.Emit("Firefox", "GitHub")
	time.Sleep(50 * time.Millisecond)

	// Go to PAUSED
	mp.Emit(presence.Paused)
	time.Sleep(50 * time.Millisecond)

	// These should be ignored while paused
	mt.Emit("Slack", "Chat")
	mt.Emit("Terminal", "bash")
	time.Sleep(50 * time.Millisecond)

	cancel()
	eng.Wait()

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)
	events, _ := s.ListFocusEvents(from, to)

	// Should only have the Firefox event (Slack and Terminal were while paused)
	if len(events) > 1 {
		t.Errorf("got %d events, want 1 (focus during pause should be ignored)", len(events))
	}
}
