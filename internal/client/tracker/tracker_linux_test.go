//go:build linux

// internal/client/tracker/tracker_linux_test.go
package tracker_test

import (
	"os"
	"testing"

	"github.com/jaypaulb/trasker/internal/client/tracker"
)

func TestDetectSessionType(t *testing.T) {
	st := tracker.DetectSessionType()
	t.Logf("Detected session type: %q", st)

	// Should be one of the known types
	switch st {
	case "x11", "wayland", "unknown":
		// OK
	default:
		t.Errorf("unexpected session type: %q", st)
	}
}

func TestDetectSessionType_EnvOverride(t *testing.T) {
	// Save and restore
	origType := os.Getenv("XDG_SESSION_TYPE")
	origWayland := os.Getenv("WAYLAND_DISPLAY")
	defer func() {
		os.Setenv("XDG_SESSION_TYPE", origType)
		os.Setenv("WAYLAND_DISPLAY", origWayland)
	}()

	// Test explicit X11
	os.Setenv("XDG_SESSION_TYPE", "x11")
	os.Unsetenv("WAYLAND_DISPLAY")
	if st := tracker.DetectSessionType(); st != "x11" {
		t.Errorf("with XDG_SESSION_TYPE=x11: got %q, want %q", st, "x11")
	}

	// Test explicit Wayland
	os.Setenv("XDG_SESSION_TYPE", "wayland")
	if st := tracker.DetectSessionType(); st != "wayland" {
		t.Errorf("with XDG_SESSION_TYPE=wayland: got %q, want %q", st, "wayland")
	}

	// Test fallback via WAYLAND_DISPLAY
	os.Setenv("XDG_SESSION_TYPE", "")
	os.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if st := tracker.DetectSessionType(); st != "wayland" {
		t.Errorf("with WAYLAND_DISPLAY set: got %q, want %q", st, "wayland")
	}
}
