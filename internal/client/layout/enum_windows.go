//go:build windows

// internal/client/layout/enum_windows.go
// Windows layout enumeration (EnumWindows + per-monitor DPI) is deferred
// to v2 per CONTEXT.md D-05. The constructor returns ErrUnsupported and
// the daemon logs a Warn and runs without layout capture.
package layout

// NewPlatformEnumerator returns ErrUnsupported on Windows (deferred to v2).
func NewPlatformEnumerator() (Enumerator, error) {
	return nil, ErrUnsupported
}
