// internal/client/tracker/tracker_test.go
package tracker_test

import (
	"context"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/tracker"
)

// MockTracker is a test double that satisfies the Tracker interface.
type MockTracker struct {
	events chan tracker.FocusChange
	done   chan struct{}
}

func NewMockTracker() *MockTracker {
	return &MockTracker{
		events: make(chan tracker.FocusChange, 10),
		done:   make(chan struct{}),
	}
}

func (m *MockTracker) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		close(m.done)
	}()
	return nil
}

func (m *MockTracker) Stop() {
	close(m.events)
}

func (m *MockTracker) Events() <-chan tracker.FocusChange {
	return m.events
}

// Emit sends a focus change event (test helper).
func (m *MockTracker) Emit(appName, windowTitle string) {
	m.events <- tracker.FocusChange{
		AppName:     appName,
		WindowTitle: windowTitle,
		Timestamp:   time.Now().UTC(),
	}
}

// Verify MockTracker satisfies the Tracker interface at compile time.
var _ tracker.Tracker = (*MockTracker)(nil)

func TestMockTracker_EmitAndReceive(t *testing.T) {
	mock := NewMockTracker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	mock.Emit("Firefox", "GitHub")

	select {
	case ev := <-mock.Events():
		if ev.AppName != "Firefox" {
			t.Errorf("AppName = %q, want %q", ev.AppName, "Firefox")
		}
		if ev.WindowTitle != "GitHub" {
			t.Errorf("WindowTitle = %q, want %q", ev.WindowTitle, "GitHub")
		}
		if ev.Timestamp.IsZero() {
			t.Error("Timestamp is zero")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestFocusChange_Fields(t *testing.T) {
	fc := tracker.FocusChange{
		AppName:     "Code",
		WindowTitle: "main.go — trasker",
		Timestamp:   time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC),
	}
	if fc.AppName != "Code" {
		t.Errorf("AppName = %q, want %q", fc.AppName, "Code")
	}
}
