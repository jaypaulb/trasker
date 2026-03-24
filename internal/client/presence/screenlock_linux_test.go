//go:build linux && dbus_integration

// internal/client/presence/screenlock_linux_test.go
package presence_test

import (
	"context"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/presence"
)

// Integration test — requires running D-Bus session bus.
// Run with: go test -tags dbus_integration ./internal/client/presence/... -run TestScreenLockListener

func TestScreenLockListener_Start(t *testing.T) {
	listener, err := presence.NewScreenLockListener()
	if err != nil {
		t.Skipf("D-Bus not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := listener.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// We can't easily trigger a screen lock in a test, but we can verify
	// the listener starts without error and the channel is open.
	select {
	case sc := <-listener.Events():
		t.Logf("Got screen lock event: %v", sc)
	case <-ctx.Done():
		t.Log("No screen lock event (expected — we didn't lock the screen)")
	}

	listener.Stop()
}
