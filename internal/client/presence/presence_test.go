// internal/client/presence/presence_test.go
package presence_test

import (
	"context"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/presence"
)

func TestPresenceState_String(t *testing.T) {
	tests := []struct {
		state presence.State
		want  string
	}{
		{presence.Tracking, "TRACKING"},
		{presence.Checking, "CHECKING"},
		{presence.Paused, "PAUSED"},
		{presence.Away, "AWAY"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// MockPresenceDetector satisfies the PresenceDetector interface for testing.
type MockPresenceDetector struct {
	states chan presence.StateChange
}

func NewMockPresenceDetector() *MockPresenceDetector {
	return &MockPresenceDetector{
		states: make(chan presence.StateChange, 10),
	}
}

func (m *MockPresenceDetector) Start(ctx context.Context) error { return nil }
func (m *MockPresenceDetector) Stop()                           {}
func (m *MockPresenceDetector) States() <-chan presence.StateChange {
	return m.states
}
func (m *MockPresenceDetector) AcknowledgeCheck()   {}
func (m *MockPresenceDetector) ResetOnFocusChange() {}

func (m *MockPresenceDetector) Emit(state presence.State) {
	m.states <- presence.StateChange{
		State:     state,
		Timestamp: time.Now().UTC(),
	}
}

// Verify interface compliance at compile time.
var _ presence.PresenceDetector = (*MockPresenceDetector)(nil)

func TestMockPresenceDetector_EmitAndReceive(t *testing.T) {
	mock := NewMockPresenceDetector()
	mock.Emit(presence.Checking)

	select {
	case sc := <-mock.States():
		if sc.State != presence.Checking {
			t.Errorf("State = %v, want CHECKING", sc.State)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestStateChange_Fields(t *testing.T) {
	sc := presence.StateChange{
		State:     presence.Away,
		Timestamp: time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
	}
	if sc.State != presence.Away {
		t.Errorf("State = %v, want AWAY", sc.State)
	}
}
