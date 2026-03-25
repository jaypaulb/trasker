//go:build linux && cgo

// internal/client/tracker/tracker_linux_cgo.go
// When CGo is available, NewPlatformTracker can use either X11 or Wayland.
package tracker

import "fmt"

// NewPlatformTracker creates the appropriate focus tracker for the current
// Linux session (X11 or Wayland). When CGo is available, both are supported.
func NewPlatformTracker() (Tracker, error) {
	sessionType := DetectSessionType()
	switch sessionType {
	case "x11":
		return NewX11Tracker()
	case "wayland":
		return NewWaylandTracker()
	default:
		return nil, fmt.Errorf("unsupported session type: %q (set XDG_SESSION_TYPE or WAYLAND_DISPLAY)", sessionType)
	}
}
