//go:build !linux

package tray

// IsAvailable reports whether a system tray is available on this
// platform. macOS NSStatusItem and Windows Shell_NotifyIcon are
// always available when the user has a desktop session, so we
// return true unconditionally. The Linux build wires a real DBus
// probe (see availability_linux.go).
func IsAvailable() bool { return true }
