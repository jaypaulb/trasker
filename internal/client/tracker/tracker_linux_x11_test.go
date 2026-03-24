//go:build linux && x11_integration

// internal/client/tracker/tracker_linux_x11_test.go
package tracker_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/tracker"
)

// This is an integration test that requires a running X11 session.
// Run with: go test -tags x11_integration ./internal/client/tracker/...

func TestX11Tracker_Integration(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY not set — skipping X11 integration test")
	}

	tr, err := tracker.NewX11Tracker()
	if err != nil {
		t.Fatalf("NewX11Tracker() error: %v", err)
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
		if ev.AppName == "" && ev.WindowTitle == "" {
			t.Error("both AppName and WindowTitle are empty")
		}
	case <-ctx.Done():
		t.Log("No focus change detected within timeout (this is OK if no window switch occurred)")
	}

	tr.Stop()
}
