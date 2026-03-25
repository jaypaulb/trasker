//go:build linux

// internal/client/tracker/tracker_linux.go
package tracker

import "os"

// DetectSessionType returns "x11", "wayland", or "unknown" based on
// environment variables.
func DetectSessionType() string {
	sessionType := os.Getenv("XDG_SESSION_TYPE")
	switch sessionType {
	case "x11":
		return "x11"
	case "wayland":
		return "wayland"
	}

	// Fallback: check for WAYLAND_DISPLAY
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland"
	}

	// Fallback: check for DISPLAY (X11)
	if os.Getenv("DISPLAY") != "" {
		return "x11"
	}

	return "unknown"
}
