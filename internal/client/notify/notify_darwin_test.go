//go:build darwin

// internal/client/notify/notify_darwin_test.go
package notify

import (
	"testing"
)

func TestDarwinNotifier_NewDarwinNotifier(t *testing.T) {
	n, err := NewDarwinNotifier()
	if err != nil {
		t.Fatalf("NewDarwinNotifier returned error: %v", err)
	}
	if n == nil {
		t.Fatal("NewDarwinNotifier returned nil")
	}
}

func TestDarwinNotifier_Notify(t *testing.T) {
	// Requires macOS GUI session. Verify no crash.
	n, err := NewDarwinNotifier()
	if err != nil {
		t.Fatalf("NewDarwinNotifier error: %v", err)
	}
	err = n.Notify("Trasker Test", "This is a test notification", nil)
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}
}

func TestDarwinNotifier_NotifyWithCallback(t *testing.T) {
	n, err := NewDarwinNotifier()
	if err != nil {
		t.Fatalf("NewDarwinNotifier error: %v", err)
	}
	called := false
	err = n.Notify("Trasker Test", "Click callback test", func() {
		called = true
	})
	if err != nil {
		t.Fatalf("Notify with callback failed: %v", err)
	}
	// Can't programmatically click a notification in tests.
	// Just verify registration doesn't crash.
	_ = called
}

func TestDarwinNotifier_Close(t *testing.T) {
	n, err := NewDarwinNotifier()
	if err != nil {
		t.Fatalf("NewDarwinNotifier error: %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
