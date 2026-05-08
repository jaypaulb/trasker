//go:build linux && !cgo

// internal/client/layout/enum_linux_nocgo.go
// When CGO_ENABLED=0 (cross-compiled from a server build host), the X11
// enumerator is unavailable because it links against libX11. The daemon
// logs a Warn and runs without layout capture.
package layout

// NewPlatformEnumerator returns ErrUnsupported on no-cgo Linux builds.
// The Wayland path is deferred to v2 (see CONTEXT.md D-04).
func NewPlatformEnumerator() (Enumerator, error) {
	return nil, ErrUnsupported
}
