//go:build darwin && !cgo

// internal/client/presence/screenlock_darwin_nocgo.go
// No-op stub for darwin cross-compilation without CGo.
// The CGo-based screenlock_darwin.go uses NSDistributedNotificationCenter.
// When CGO_ENABLED=0 (cross-compiling from Linux), this stub is used instead.
// On native macOS builds, CGo is available and screenlock_darwin.go is used.
package presence

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// DarwinScreenLockListener is the non-CGo fallback for macOS screen lock detection.
// Polls the CGSession dictionary via python3 (available on all macOS).
type DarwinScreenLockListener struct {
	events chan StateChange
	done   chan struct{}
}

// NewScreenLockListener creates a macOS screen lock listener using python3 (no CGo).
func NewScreenLockListener() (*DarwinScreenLockListener, error) {
	return &DarwinScreenLockListener{
		events: make(chan StateChange, 16),
		done:   make(chan struct{}),
	}, nil
}

func (s *DarwinScreenLockListener) Start(ctx context.Context) error {
	go s.pollLoop(ctx)
	return nil
}

func (s *DarwinScreenLockListener) Events() <-chan StateChange {
	return s.events
}

func (s *DarwinScreenLockListener) Stop() {
	close(s.done)
}

func (s *DarwinScreenLockListener) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	wasLocked := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			locked := isScreenLocked()
			if locked != wasLocked {
				state := Away
				if !locked {
					state = Tracking
				}
				select {
				case s.events <- StateChange{
					State:     state,
					Timestamp: time.Now().UTC(),
				}:
				default:
				}
				wasLocked = locked
			}
		}
	}
}

func isScreenLocked() bool {
	script := `import Quartz; print(Quartz.CGSessionCopyCurrentDictionary().get("CGSSessionScreenIsLocked", 0))`
	out, err := exec.Command("python3", "-c", script).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}
