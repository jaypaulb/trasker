//go:build darwin

// internal/client/presence/screenlock_darwin_test.go
package presence

import (
	"context"
	"testing"
)

func TestDarwinScreenLock_NewScreenLockListener(t *testing.T) {
	sl, err := NewScreenLockListener()
	if err != nil {
		t.Fatalf("NewScreenLockListener returned error: %v", err)
	}
	if sl == nil {
		t.Fatal("NewScreenLockListener returned nil")
	}
}

func TestDarwinScreenLock_EventsChannel(t *testing.T) {
	sl, err := NewScreenLockListener()
	if err != nil {
		t.Fatalf("NewScreenLockListener error: %v", err)
	}
	ch := sl.Events()
	if ch == nil {
		t.Fatal("Events() returned nil channel")
	}
}

func TestDarwinScreenLock_StartStop(t *testing.T) {
	// This test verifies Start/Stop don't crash.
	// Actual lock/unlock events require a macOS GUI session.
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
