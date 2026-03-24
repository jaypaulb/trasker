//go:build darwin

// internal/client/tracker/tracker_darwin_test.go
package tracker

import (
	"context"
	"testing"
	"time"
)

func TestDarwinTracker_NewPlatformTracker(t *testing.T) {
	tr, err := NewPlatformTracker()
	if err != nil {
		t.Fatalf("NewPlatformTracker returned error: %v", err)
	}
	if tr == nil {
		t.Fatal("NewPlatformTracker returned nil")
	}
}

func TestDarwinTracker_GetFocusedWindow(t *testing.T) {
	// This test requires a running macOS GUI session with Accessibility permissions.
	// It will return empty strings in headless/CI environments.
	dt := &darwinTracker{
		pollInterval: 1 * time.Second,
		events:       make(chan FocusChange, 64),
		done:         make(chan struct{}),
	}
	app, title := dt.getFocusedWindow()
	t.Logf("Focused app: %q, title: %q", app, title)
	// We don't assert specific values — just that it doesn't crash.
}

func TestDarwinTracker_StartStop(t *testing.T) {
	tr, err := NewPlatformTracker()
	if err != nil {
		t.Fatalf("NewPlatformTracker error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Let it run briefly, then stop.
	time.Sleep(100 * time.Millisecond)
	tr.Stop()
}

func TestDarwinTracker_EventsChannel(t *testing.T) {
	tr, err := NewPlatformTracker()
	if err != nil {
		t.Fatalf("NewPlatformTracker error: %v", err)
	}
	ch := tr.Events()
	if ch == nil {
		t.Fatal("Events() returned nil channel")
	}
}
