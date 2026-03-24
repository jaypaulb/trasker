//go:build windows

// internal/client/setup/autostart_windows_test.go
package setup

import (
	"testing"
)

func TestWindows_EnableDisableAutostart(t *testing.T) {
	a := NewWindowsAutostart("")

	// Enable autostart.
	if err := a.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	if !a.IsEnabled() {
		t.Error("expected IsEnabled() == true after Enable()")
	}

	// Disable autostart (clean up registry).
	if err := a.Disable(); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	if a.IsEnabled() {
		t.Error("expected IsEnabled() == false after Disable()")
	}
}

func TestWindows_DisableAutostart_NotExists(t *testing.T) {
	a := NewWindowsAutostart("")

	// Ensure it's not there first.
	_ = a.Disable()

	// Disable when key doesn't exist should not error.
	if err := a.Disable(); err != nil {
		t.Fatalf("Disable on non-existent key should not error: %v", err)
	}
}

func TestWindows_IsEnabled_NotExists(t *testing.T) {
	a := NewWindowsAutostart("")

	_ = a.Disable()

	if a.IsEnabled() {
		t.Error("expected IsEnabled() == false when registry key doesn't exist")
	}
}

func TestWindows_NewAutostart_ReturnsAutostart(t *testing.T) {
	// NewAutostart must return an Autostart interface value.
	var _ Autostart = NewAutostart("")
}
