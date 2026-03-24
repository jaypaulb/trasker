// internal/client/pomodoro/timer.go
package pomodoro

import (
	"fmt"
	"sync"
	"time"
)

// State represents the pomodoro timer state.
type State string

const (
	StateIdle   State = "idle"
	StateWork   State = "work"
	StateBreak  State = "break"
	StateDone   State = "done"
	StatePaused State = "paused"
)

// Config holds pomodoro timer settings.
type Config struct {
	WorkMins  int
	BreakMins int
}

// DefaultConfig returns 25/5 defaults.
func DefaultConfig() Config {
	return Config{WorkMins: 25, BreakMins: 5}
}

// StateChange is emitted when the timer transitions.
type StateChange struct {
	From      State
	To        State
	Remaining time.Duration
	TagID     *int64 // optional tag association
}

// Timer implements a pomodoro state machine.
type Timer struct {
	mu        sync.Mutex
	state     State
	config    Config
	tagID     *int64
	startedAt time.Time
	endAt     time.Time
	changes   chan StateChange
	stopCh    chan struct{}
	ticker    *time.Ticker
}

// NewTimer creates a new pomodoro timer. The changes channel receives state transitions.
func NewTimer(config Config) *Timer {
	return &Timer{
		state:   StateIdle,
		config:  config,
		changes: make(chan StateChange, 16),
	}
}

// Changes returns the channel for state transition notifications.
func (t *Timer) Changes() <-chan StateChange {
	return t.changes
}

// State returns the current timer state.
func (t *Timer) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// Remaining returns the time left in the current phase.
func (t *Timer) Remaining() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == StateIdle || t.state == StateDone {
		return 0
	}
	rem := time.Until(t.endAt)
	if rem < 0 {
		return 0
	}
	return rem
}

// Start begins a work phase. Optional tagID associates the session with an activity tag.
func (t *Timer) Start(tagID *int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != StateIdle && t.state != StateDone {
		return fmt.Errorf("pomodoro: cannot start from state %s", t.state)
	}

	t.tagID = tagID
	t.transitionTo(StateWork)
	return nil
}

// Cancel stops the timer and returns to idle.
func (t *Timer) Cancel() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopCh != nil {
		close(t.stopCh)
		t.stopCh = nil
	}
	if t.ticker != nil {
		t.ticker.Stop()
		t.ticker = nil
	}

	old := t.state
	t.state = StateIdle
	t.emitChange(old, StateIdle)
}

// transitionTo changes state and starts the appropriate countdown.
// Must be called with t.mu held.
func (t *Timer) transitionTo(newState State) {
	old := t.state

	// Stop previous ticker
	if t.stopCh != nil {
		close(t.stopCh)
	}
	if t.ticker != nil {
		t.ticker.Stop()
	}

	t.state = newState
	t.startedAt = time.Now()

	var duration time.Duration
	switch newState {
	case StateWork:
		duration = time.Duration(t.config.WorkMins) * time.Minute
	case StateBreak:
		duration = time.Duration(t.config.BreakMins) * time.Minute
	default:
		t.emitChange(old, newState)
		return
	}

	t.endAt = t.startedAt.Add(duration)
	t.emitChange(old, newState)

	stopCh := make(chan struct{})
	t.stopCh = stopCh
	t.ticker = time.NewTicker(1 * time.Second)

	go t.runPhase(newState, stopCh)
}

// runPhase waits for the phase to end, then transitions to the next state.
func (t *Timer) runPhase(phase State, stopCh chan struct{}) {
	for {
		select {
		case <-stopCh:
			return
		case <-time.After(time.Until(t.endAt)):
			t.mu.Lock()
			// Verify we're still in the expected phase
			if t.state != phase {
				t.mu.Unlock()
				return
			}
			switch phase {
			case StateWork:
				t.transitionTo(StateBreak)
			case StateBreak:
				old := t.state
				t.state = StateDone
				if t.ticker != nil {
					t.ticker.Stop()
					t.ticker = nil
				}
				t.emitChange(old, StateDone)
			}
			t.mu.Unlock()
			return
		}
	}
}

// emitChange sends a state change notification (non-blocking).
func (t *Timer) emitChange(from, to State) {
	sc := StateChange{
		From:      from,
		To:        to,
		Remaining: time.Until(t.endAt),
		TagID:     t.tagID,
	}
	select {
	case t.changes <- sc:
	default:
	}
}
