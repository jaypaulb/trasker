//go:build linux

// internal/client/setup/autostart_linux_test.go
package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxAutostart_EnableDisable(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	a := NewLinuxAutostart("/usr/local/bin/trasker-client")

	if a.IsEnabled() {
		t.Error("expected not enabled initially")
	}

	if err := a.Enable(); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if !a.IsEnabled() {
		t.Error("expected enabled after Enable()")
	}

	// Verify file content
	content, err := os.ReadFile(filepath.Join(tmpDir, "autostart", "trasker.desktop"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty desktop file")
	}

	if err := a.Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if a.IsEnabled() {
		t.Error("expected not enabled after Disable()")
	}
}

func TestLinuxAutostart_DisableWhenNotEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	a := NewLinuxAutostart("/usr/local/bin/trasker-client")

	// Should not error when disabling something that doesn't exist
	if err := a.Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}
}

func TestLinuxAutostart_ImplementsInterface(t *testing.T) {
	var _ Autostart = (*LinuxAutostart)(nil)
}
