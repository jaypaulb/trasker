//go:build windows

// internal/client/tracker/tracker_windows_test.go
package tracker

import (
	"context"
	"testing"
	"time"
)

func TestWindowsTracker_NewPlatformTracker(t *testing.T) {
	tr, err := NewPlatformTracker()
	if err != nil {
		t.Fatalf("NewPlatformTracker returned error: %v", err)
	}
	if tr == nil {
		t.Fatal("NewPlatformTracker returned nil")
	}
}

func TestWindowsTracker_GetWindowText(t *testing.T) {
	// Requires a running Windows GUI session.
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		t.Skip("No foreground window available (headless environment)")
	}
	title := getWindowText(hwnd)
	t.Logf("Foreground window title: %q", title)
	// Just verify no crash.
}

func TestWindowsTracker_GetProcessName(t *testing.T) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		t.Skip("No foreground window available (headless environment)")
	}
	name := getProcessName(hwnd)
	t.Logf("Foreground process name: %q", name)
}

func TestWindowsTracker_StartStop(t *testing.T) {
	tr, err := NewPlatformTracker()
	if err != nil {
		t.Fatalf("NewPlatformTracker error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	tr.Stop()
}

func TestWindowsTracker_EventsChannel(t *testing.T) {
	tr, err := NewPlatformTracker()
	if err != nil {
		t.Fatalf("NewPlatformTracker error: %v", err)
	}
	ch := tr.Events()
	if ch == nil {
		t.Fatal("Events() returned nil channel")
	}
}
