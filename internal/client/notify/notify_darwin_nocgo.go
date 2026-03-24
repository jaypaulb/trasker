//go:build darwin && !cgo

// internal/client/notify/notify_darwin_nocgo.go
// No-op stub for darwin cross-compilation without CGo.
// The CGo-based notify_darwin.go uses NSUserNotificationCenter.
// When CGO_ENABLED=0 (cross-compiling from Linux), this stub is used instead.
// On native macOS builds, CGo is available and notify_darwin.go is used.
package notify

import (
	"fmt"
	"os/exec"
)

// DarwinNotifier is the non-CGo fallback for macOS notifications.
// Uses osascript to display notifications.
type DarwinNotifier struct{}

// NewDarwinNotifier creates a macOS notifier using osascript (no CGo).
func NewDarwinNotifier() (*DarwinNotifier, error) {
	return &DarwinNotifier{}, nil
}

// Notify sends a macOS desktop notification via osascript.
// onClick is not supported in the osascript fallback.
func (n *DarwinNotifier) Notify(title, body string, onClick ClickAction) error {
	script := fmt.Sprintf(`display notification %q with title %q`, body, title)
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript notification failed: %w: %s", err, string(output))
	}
	return nil
}

// Close is a no-op for the Darwin nocgo notifier.
func (n *DarwinNotifier) Close() error {
	return nil
}
