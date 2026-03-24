//go:build darwin

// internal/client/setup/autostart_darwin_test.go
package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwin_PlistPath(t *testing.T) {
	p, err := plistPathFn()
	if err != nil {
		t.Fatalf("plistPathFn failed: %v", err)
	}
	if !strings.HasSuffix(p, "Library/LaunchAgents/com.trasker.client.plist") {
		t.Errorf("unexpected plist path: %s", p)
	}
}

func TestDarwin_EnableDisableAutostart(t *testing.T) {
	// Use a temp directory to avoid modifying real LaunchAgents.
	tmpDir := t.TempDir()
	testPlistPath := filepath.Join(tmpDir, "com.trasker.client.plist")

	// Override the plist path for testing via the injectable var.
	origPlistPathFn := plistPathFn
	plistPathFn = func() (string, error) { return testPlistPath, nil }
	t.Cleanup(func() { plistPathFn = origPlistPathFn })

	a := NewDarwinAutostart("/usr/local/bin/trasker-client")

	err := a.Enable()
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	// Verify the plist was written to the temp directory.
	if _, err := os.Stat(testPlistPath); os.IsNotExist(err) {
		t.Fatal("plist file was not created in temp directory")
	}

	if !a.IsEnabled() {
		t.Error("expected IsEnabled() == true after Enable()")
	}

	if err := a.Disable(); err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	if a.IsEnabled() {
		t.Error("expected IsEnabled() == false after Disable()")
	}
}

func TestDarwin_DisableAutostart_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	testPlistPath := filepath.Join(tmpDir, "com.trasker.client.plist")

	origPlistPathFn := plistPathFn
	plistPathFn = func() (string, error) { return testPlistPath, nil }
	t.Cleanup(func() { plistPathFn = origPlistPathFn })

	a := NewDarwinAutostart("")

	// Disable when plist doesn't exist should not error.
	err := a.Disable()
	if err != nil {
		t.Fatalf("Disable on non-existent plist should not error: %v", err)
	}
}

func TestDarwin_NewAutostart_ReturnsAutostart(t *testing.T) {
	// NewAutostart must return an Autostart interface value.
	var _ Autostart = NewAutostart("/usr/local/bin/trasker-client")
}
