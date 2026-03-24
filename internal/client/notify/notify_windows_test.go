//go:build windows

// internal/client/notify/notify_windows_test.go
package notify

import (
	"testing"
)

func TestWindowsNotifier_NewWindowsNotifier(t *testing.T) {
	n, err := NewWindowsNotifier()
	if err != nil {
		t.Fatalf("NewWindowsNotifier returned error: %v", err)
	}
	if n == nil {
		t.Fatal("NewWindowsNotifier returned nil")
	}
}

func TestWindowsNotifier_Notify(t *testing.T) {
	// Requires Windows desktop session with PowerShell.
	n, err := NewWindowsNotifier()
	if err != nil {
		t.Fatalf("NewWindowsNotifier error: %v", err)
	}
	err = n.Notify("Trasker Test", "This is a test notification", nil)
	if err != nil {
		t.Logf("Notify returned error (may be expected in CI): %v", err)
	}
}

func TestWindowsNotifier_NotifyWithCallback(t *testing.T) {
	n, err := NewWindowsNotifier()
	if err != nil {
		t.Fatalf("NewWindowsNotifier error: %v", err)
	}
	err = n.Notify("Trasker Test", "Click callback test", func() {})
	if err != nil {
		t.Logf("Notify with callback returned error (may be expected in CI): %v", err)
	}
}

func TestWindowsNotifier_Close(t *testing.T) {
	n, err := NewWindowsNotifier()
	if err != nil {
		t.Fatalf("NewWindowsNotifier error: %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
