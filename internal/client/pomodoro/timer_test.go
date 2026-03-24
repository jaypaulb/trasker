// internal/client/pomodoro/timer_test.go
package pomodoro

import (
	"testing"
	"time"
)

func TestTimer_StartsInIdle(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	if timer.State() != StateIdle {
		t.Errorf("expected idle, got %s", timer.State())
	}
}

func TestTimer_StartTransitionsToWork(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	if err := timer.Start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	if timer.State() != StateWork {
		t.Errorf("expected work, got %s", timer.State())
	}

	// Should receive a state change
	select {
	case sc := <-timer.Changes():
		if sc.From != StateIdle || sc.To != StateWork {
			t.Errorf("expected idle->work, got %s->%s", sc.From, sc.To)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected state change")
	}

	timer.Cancel()
}

func TestTimer_CannotStartWhenRunning(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	timer.Start(nil)

	err := timer.Start(nil)
	if err == nil {
		t.Error("expected error starting while running")
	}

	timer.Cancel()
}

func TestTimer_CancelReturnsToIdle(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	timer.Start(nil)
	timer.Cancel()

	if timer.State() != StateIdle {
		t.Errorf("expected idle after cancel, got %s", timer.State())
	}
}

func TestTimer_ShortWorkTransitionsToBreak(t *testing.T) {
	// Use 1-second work/break for fast test
	config := Config{WorkMins: 0, BreakMins: 0}
	timer := &Timer{
		state:   StateIdle,
		config:  config,
		changes: make(chan StateChange, 16),
	}

	// Manually set very short durations
	timer.mu.Lock()
	timer.state = StateWork
	timer.startedAt = time.Now()
	timer.endAt = time.Now().Add(50 * time.Millisecond)
	stopCh := make(chan struct{})
	timer.stopCh = stopCh
	timer.mu.Unlock()

	go timer.runPhase(StateWork, stopCh)

	// Wait for transition
	time.Sleep(200 * time.Millisecond)

	state := timer.State()
	// Should be in break or done (break duration is also 0)
	if state != StateBreak && state != StateDone {
		t.Errorf("expected break or done after work phase, got %s", state)
	}

	timer.Cancel()
}

func TestTimer_RemainingDecrements(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	timer.Start(nil)

	rem := timer.Remaining()
	if rem > 25*time.Minute+time.Second || rem < 24*time.Minute {
		t.Errorf("expected ~25m remaining, got %v", rem)
	}

	timer.Cancel()
}

func TestTimer_TagAssociation(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	tagID := int64(42)
	timer.Start(&tagID)

	select {
	case sc := <-timer.Changes():
		if sc.TagID == nil || *sc.TagID != 42 {
			t.Errorf("expected tagID 42, got %v", sc.TagID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected state change")
	}

	timer.Cancel()
}
