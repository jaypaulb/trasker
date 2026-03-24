//go:build linux && wayland_integration

// internal/client/tracker/tracker_linux_wayland_test.go
package tracker_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/tracker"
)

// This is an integration test that requires a running Wayland session.
// Run with: go test -tags wayland_integration ./internal/client/tracker/...

func TestWaylandTracker_Integration(t *testing.T) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("WAYLAND_DISPLAY not set — skipping Wayland integration test")
	}

	tr, err := tracker.NewWaylandTracker()
	if err != nil {
		t.Fatalf("NewWaylandTracker() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Wait for at least one event (the initial focus)
	select {
	case ev := <-tr.Events():
		t.Logf("Got focus event: app=%q title=%q", ev.AppName, ev.WindowTitle)
	case <-ctx.Done():
		t.Log("No focus change detected within timeout (OK if no window switch)")
	}

	tr.Stop()
}
