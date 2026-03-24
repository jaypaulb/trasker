//go:build windows

// internal/client/presence/screenlock_windows_test.go
package presence

import (
	"context"
	"testing"
)

func TestWindowsScreenLock_NewScreenLockListener(t *testing.T) {
	sl, err := NewScreenLockListener()
	if err != nil {
		t.Fatalf("NewScreenLockListener returned error: %v", err)
	}
	if sl == nil {
		t.Fatal("NewScreenLockListener returned nil")
	}
}

func TestWindowsScreenLock_EventsChannel(t *testing.T) {
	sl, err := NewScreenLockListener()
	if err != nil {
		t.Fatalf("NewScreenLockListener error: %v", err)
	}
	ch := sl.Events()
	if ch == nil {
		t.Fatal("Events() returned nil channel")
	}
}

func TestWindowsScreenLock_StartStop(t *testing.T) {
	// Verifies Start/Stop lifecycle doesn't crash.
	// Actual lock/unlock events require a Windows desktop session.
	sl, err := NewScreenLockListener()
	if err != nil {
		t.Fatalf("NewScreenLockListener error: %v", err)
	}

	ctx := context.Background()
	if err := sl.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	sl.Stop()
}
