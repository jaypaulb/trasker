//go:build linux

// Package-level doc is in runtime.go.
package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

const desktopEntry = `[Desktop Entry]
Type=Application
Name=Trasker
Comment=Open the Trasker time-tracking dashboard
Exec=%s open
Icon=appointment-new
Terminal=false
Categories=Utility;Office;
Keywords=time;tracking;focus;pomodoro;
`

// WriteDesktopFile installs ~/.local/share/applications/trasker.desktop so
// the dashboard appears in the GNOME app grid and search. The Exec path is
// derived from the running binary so it stays correct after moves or updates.
func WriteDesktopFile() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("runtime: resolve executable: %w", err)
	}

	dir, err := desktopAppDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runtime: mkdir %s: %w", dir, err)
	}

	content := fmt.Sprintf(desktopEntry, execPath)
	dest := filepath.Join(dir, "trasker.desktop")
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return fmt.Errorf("runtime: write desktop file: %w", err)
	}
	return nil
}

func desktopAppDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "applications"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("runtime: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "applications"), nil
}
