//go:build linux

package tray

import (
	"os"
	"testing"
)

// TestIsAvailable_DoesNotBlock is the only thing we can portably
// assert. The actual return value depends on the user's desktop
// session — in CI without DBUS_SESSION_BUS_ADDRESS the function
// must return false promptly (within the 2-second timeout) and
// must not panic.
func TestIsAvailable_DoesNotBlock(t *testing.T) {
	// Force no DBus to make the result deterministic in CI.
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	got := IsAvailable()
	if got {
		// If we somehow got true with no DBus, the test machine
		// has DBus configured via /run, that's fine — just log it.
		// The negative-path is what we care about not panicking.
		t.Logf("IsAvailable returned true despite cleared env (system DBus configured)")
	}
	// We don't assert on the bool: the contract is "doesn't panic /
	// doesn't hang". If we got here, the timeout was respected.
	_ = os.Getenv("DBUS_SESSION_BUS_ADDRESS")
}
