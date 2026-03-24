// internal/client/presence/deadman.go
package presence

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Deadman implements the deadman's switch timer logic.
// It escalates through configured intervals when no focus change occurs,
// fires a CHECKING state with a countdown, and transitions to PAUSED
// if the countdown expires without acknowledgment.
type Deadman struct {
	intervals         []time.Duration
	countdownDuration time.Duration

	states chan StateChange
	mu     sync.Mutex
	tier   int
	cancel context.CancelFunc

	// Internal signals
	ackCh   chan struct{}
	resetCh chan struct{}
}

// NewDeadman creates a deadman's switch with the given escalation intervals
// and countdown duration. Production defaults: intervals=[30,45,60,90,120] minutes,
// countdown=90 seconds. Returns an error if intervals is empty or any duration is <= 0.
func NewDeadman(intervals []time.Duration, countdownDuration time.Duration) (*Deadman, error) {
	if len(intervals) == 0 {
		return nil, errors.New("deadman: intervals must not be empty")
	}
	for i, d := range intervals {
		if d <= 0 {
			return nil, fmt.Errorf("deadman: interval[%d] must be > 0, got %v", i, d)
		}
	}
	if countdownDuration <= 0 {
		return nil, fmt.Errorf("deadman: countdownDuration must be > 0, got %v", countdownDuration)
	}
	return &Deadman{
		intervals:         intervals,
		countdownDuration: countdownDuration,
		states:            make(chan StateChange, 16),
		ackCh:             make(chan struct{}, 1),
		resetCh:           make(chan struct{}, 1),
	}, nil
}

// DefaultDeadman creates a deadman's switch with production defaults.
func DefaultDeadman() *Deadman {
	// Safe to ignore error: these are known-valid production defaults.
	dm, _ := NewDeadman(
		[]time.Duration{
			30 * time.Minute,
			45 * time.Minute,
			60 * time.Minute,
			90 * time.Minute,
			120 * time.Minute,
		},
		90*time.Second,
	)
	return dm
}

// Start begins the deadman's switch timer loop.
func (d *Deadman) Start(ctx context.Context) {
	ctx, d.cancel = context.WithCancel(ctx)
	go d.loop(ctx)
}

// Stop ceases the deadman's switch.
func (d *Deadman) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
}

// States returns the channel of state transitions.
func (d *Deadman) States() <-chan StateChange {
	return d.states
}

// Acknowledge is called when the user responds to the CHECKING notification.
// Transitions back to TRACKING and escalates to the next interval tier.
func (d *Deadman) Acknowledge() {
	select {
	case d.ackCh <- struct{}{}:
	default:
	}
}

// Reset is called on focus change. Resets the interval tier back to 0
// and restarts the timer.
func (d *Deadman) Reset() {
	select {
	case d.resetCh <- struct{}{}:
	default:
	}
}

func (d *Deadman) loop(ctx context.Context) {
	for {
		interval := d.currentInterval()
		timer := time.NewTimer(interval)

		select {
		case <-ctx.Done():
			timer.Stop()
			return

		case <-d.resetCh:
			timer.Stop()
			d.mu.Lock()
			d.tier = 0
			d.mu.Unlock()
			d.emit(Tracking)
			continue

		case <-timer.C:
			// Fire CHECKING
			d.emit(Checking)

			// Start countdown
			countdown := time.NewTimer(d.countdownDuration)

			select {
			case <-ctx.Done():
				countdown.Stop()
				return

			case <-d.ackCh:
				countdown.Stop()
				// Escalate to next tier
				d.mu.Lock()
				if d.tier < len(d.intervals)-1 {
					d.tier++
				}
				d.mu.Unlock()
				d.emit(Tracking)
				continue

			case <-d.resetCh:
				countdown.Stop()
				d.mu.Lock()
				d.tier = 0
				d.mu.Unlock()
				d.emit(Tracking)
				continue

			case <-countdown.C:
				// Missed — transition to PAUSED
				d.emit(Paused)
				// Wait for reset (focus change) to resume
				d.waitForResume(ctx)
				continue
			}
		}
	}
}

func (d *Deadman) waitForResume(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-d.resetCh:
		d.mu.Lock()
		d.tier = 0
		d.mu.Unlock()
		d.emit(Tracking)
	case <-d.ackCh:
		d.mu.Lock()
		d.tier = 0
		d.mu.Unlock()
		d.emit(Tracking)
	}
}

func (d *Deadman) currentInterval() time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tier >= len(d.intervals) {
		return d.intervals[len(d.intervals)-1]
	}
	return d.intervals[d.tier]
}

func (d *Deadman) emit(state State) {
	d.states <- StateChange{
		State:     state,
		Timestamp: time.Now().UTC(),
	}
}
