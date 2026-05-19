//go:build !linux

package runtime

// WriteDesktopFile is a no-op on non-Linux platforms.
func WriteDesktopFile() error { return nil }
