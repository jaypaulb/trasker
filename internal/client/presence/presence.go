// internal/client/presence/presence.go
package presence

import (
	"context"
	"time"
)

// State represents the user's presence state.
type State int

const (
	// Tracking means the user is actively present and focus events are being recorded.
	Tracking State = iota
	// Checking means the deadman's switch has fired — awaiting acknowledgment (90s countdown).
	Checking
	// Paused means the user missed the deadman's switch check. Tracking is paused.
	Paused
	// Away means the screen is locked. Tracking is suspended.
	Away
)

// String returns the human-readable state name.
func (s State) String() string {
	switch s {
	case Tracking:
		return "TRACKING"
	case Checking:
		return "CHECKING"
	case Paused:
		return "PAUSED"
	case Away:
		return "AWAY"
	default:
		return "UNKNOWN"
	}
}

// StateChange represents a transition in presence state.
type StateChange struct {
	State     State
	Timestamp time.Time
}

// ScreenLockMonitor monitors screen lock/unlock events.
// Platform-specific implementations (Linux gdbus, macOS, Windows) all satisfy this.
type ScreenLockMonitor interface {
	Start(ctx context.Context) error
	Stop()
	Events() <-chan StateChange
}

// PresenceDetector monitors user presence through screen lock and deadman's switch.
//
// State machine:
//
//	TRACKING → (screen lock) → AWAY → (unlock) → TRACKING
//	TRACKING → (deadman fires) → CHECKING → (ack) → TRACKING
//	CHECKING → (90s timeout) → PAUSED
//	PAUSED → (focus change) → TRACKING
type PresenceDetector interface {
	// Start begins presence monitoring. Returns immediately.
	Start(ctx context.Context) error

	// Stop ceases presence monitoring.
	Stop()

	// States returns a read-only channel of state transitions.
	States() <-chan StateChange

	// AcknowledgeCheck is called when the user responds to a deadman's switch
	// notification (CHECKING → TRACKING). Resets to the next interval tier.
	AcknowledgeCheck()

	// ResetOnFocusChange is called when a focus change occurs.
	// Resets the deadman's switch interval back to the first tier (30 min).
	ResetOnFocusChange()
}
