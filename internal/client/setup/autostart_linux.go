//go:build linux

// internal/client/setup/autostart_linux.go
package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

const desktopEntry = `[Desktop Entry]
Type=Application
Name=Trasker
Comment=Time tracking daemon
Exec=%s
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
`

// LinuxAutostart manages ~/.config/autostart/trasker.desktop.
type LinuxAutostart struct {
	execPath string
}

// NewLinuxAutostart creates a Linux autostart manager.
// execPath is the path to the trasker-client binary.
func NewLinuxAutostart(execPath string) *LinuxAutostart {
	return &LinuxAutostart{execPath: execPath}
}

// NewAutostart returns an Autostart implementation for the current platform.
// execPath is the path to the trasker-client binary.
func NewAutostart(execPath string) Autostart {
	return NewLinuxAutostart(execPath)
}

func (a *LinuxAutostart) desktopFilePath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "autostart", "trasker.desktop")
}

// Enable creates the .desktop file for autostart.
func (a *LinuxAutostart) Enable() error {
	path := a.desktopFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("autostart: create dir: %w", err)
	}

	content := fmt.Sprintf(desktopEntry, a.execPath)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("autostart: write desktop file: %w", err)
	}
	return nil
}

// Disable removes the .desktop file.
func (a *LinuxAutostart) Disable() error {
	path := a.desktopFilePath()
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // already disabled
	}
	if err != nil {
		return fmt.Errorf("autostart: remove desktop file: %w", err)
	}
	return nil
}

// IsEnabled checks if the .desktop file exists.
func (a *LinuxAutostart) IsEnabled() bool {
	_, err := os.Stat(a.desktopFilePath())
	return err == nil
}
