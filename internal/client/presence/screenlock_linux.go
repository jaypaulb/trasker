//go:build linux

// internal/client/presence/screenlock_linux.go
package presence

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// ScreenLockListener monitors D-Bus for screen lock/unlock signals.
// It listens on both org.freedesktop.ScreenSaver and org.gnome.ScreenSaver
// for the ActiveChanged signal.
type ScreenLockListener struct {
	events chan StateChange
	cancel context.CancelFunc
}

// NewScreenLockListener creates a new screen lock listener.
// Returns an error if gdbus is not available.
func NewScreenLockListener() (*ScreenLockListener, error) {
	if _, err := exec.LookPath("gdbus"); err != nil {
		return nil, fmt.Errorf("gdbus not found: %w", err)
	}
	return &ScreenLockListener{
		events: make(chan StateChange, 4),
	}, nil
}

// Start begins listening for screen lock/unlock D-Bus signals.
func (l *ScreenLockListener) Start(ctx context.Context) error {
	ctx, l.cancel = context.WithCancel(ctx)

	// Listen on the session bus for screensaver signals.
	// Try org.gnome.ScreenSaver first (GNOME), then org.freedesktop.ScreenSaver.
	go l.listenDBus(ctx, "org.gnome.ScreenSaver", "/org/gnome/ScreenSaver", "org.gnome.ScreenSaver.ActiveChanged")
	go l.listenDBus(ctx, "org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver.ActiveChanged")

	return nil
}

// Stop ceases listening and closes the events channel.
func (l *ScreenLockListener) Stop() {
	if l.cancel != nil {
		l.cancel()
	}
	// Note: channel is closed when both goroutines exit. For simplicity,
	// we don't close here to avoid double-close on multiple signal sources.
}

// Events returns the channel of lock/unlock state changes.
func (l *ScreenLockListener) Events() <-chan StateChange {
	return l.events
}

func (l *ScreenLockListener) listenDBus(ctx context.Context, sender, objectPath, signal string) {
	// Use gdbus monitor to listen for the signal.
	// This spawns a long-running process that outputs a line for each signal.
	cmd := exec.CommandContext(ctx, "gdbus", "monitor", "--session",
		"--dest", sender,
		"--object-path", objectPath,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("screenlock: failed to get stdout pipe", "sender", sender, "error", err)
		return
	}

	if err := cmd.Start(); err != nil {
		slog.Error("screenlock: failed to start gdbus monitor", "sender", sender, "error", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		// Lines look like: /org/gnome/ScreenSaver: org.gnome.ScreenSaver.ActiveChanged (true,)
		if !strings.Contains(line, "ActiveChanged") {
			continue
		}

		locked := strings.Contains(line, "true")
		state := Tracking
		if locked {
			state = Away
		}

		select {
		case l.events <- StateChange{
			State:     state,
			Timestamp: time.Now().UTC(),
		}:
		case <-ctx.Done():
			return
		}
	}

	if err := cmd.Wait(); err != nil {
		slog.Error("screenlock: gdbus monitor exited with error", "sender", sender, "error", err)
	}
}
