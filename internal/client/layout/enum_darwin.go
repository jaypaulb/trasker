//go:build darwin

// internal/client/layout/enum_darwin.go
// macOS layout enumeration (CGWindowListCopyWindowInfo) is deferred to v2
// per CONTEXT.md D-05. The constructor returns ErrUnsupported and the
// daemon logs a Warn and runs without layout capture.
package layout

// NewPlatformEnumerator returns ErrUnsupported on macOS (deferred to v2).
func NewPlatformEnumerator() (Enumerator, error) {
	return nil, ErrUnsupported
}
