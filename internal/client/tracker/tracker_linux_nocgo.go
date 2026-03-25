//go:build linux && !cgo

// internal/client/tracker/tracker_linux_nocgo.go
// When CGO_ENABLED=0 (cross-compiling from Linux server), the X11 tracker
// is unavailable because it requires linking against libX11. Only the
// Wayland tracker (which uses gdbus shell commands) works without CGo.
// On X11-only sessions this returns an error; the client logs a warning
// and runs without focus tracking.
package tracker

import "fmt"

// NewPlatformTracker creates a focus tracker without CGo.
// Only Wayland (gdbus-based) is available; X11 requires CGo.
func NewPlatformTracker() (Tracker, error) {
	sessionType := DetectSessionType()
	switch sessionType {
	case "wayland":
		return NewWaylandTracker()
	case "x11":
		return nil, fmt.Errorf("X11 focus tracking requires a CGo-enabled build (this binary was cross-compiled without CGo)")
	default:
		return nil, fmt.Errorf("unsupported session type: %q (set XDG_SESSION_TYPE or WAYLAND_DISPLAY)", sessionType)
	}
}
