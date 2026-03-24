// internal/client/notify/notify_linux_test.go
//go:build linux

package notify

import (
	"testing"
)

func TestLinuxNotifier_ImplementsInterface(t *testing.T) {
	// Compile-time check that LinuxNotifier satisfies Notifier
	var _ Notifier = (*LinuxNotifier)(nil)
}
