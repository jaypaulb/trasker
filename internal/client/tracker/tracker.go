// internal/client/tracker/tracker.go
package tracker

import (
	"context"
	"time"
)

// FocusChange represents a single focus window change detected by the tracker.
type FocusChange struct {
	AppName     string
	WindowTitle string
	Timestamp   time.Time
}

// Tracker is the interface that platform-specific focus trackers implement.
// Start begins monitoring active windows. Events are delivered via the Events channel.
// Stop ceases monitoring and closes the Events channel.
type Tracker interface {
	// Start begins focus tracking. It should return immediately.
	// Tracking runs until the context is cancelled or Stop is called.
	Start(ctx context.Context) error

	// Stop ceases focus tracking and closes the Events channel.
	Stop()

	// Events returns a read-only channel of focus change events.
	Events() <-chan FocusChange
}
