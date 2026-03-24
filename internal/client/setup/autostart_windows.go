//go:build windows

// internal/client/setup/autostart_windows.go
package setup

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	registryKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	registryValueName = "Trasker"
)

// WindowsAutostart manages the HKCU\...\Run registry entry for Windows autostart.
type WindowsAutostart struct {
	execPath string
}

// NewWindowsAutostart creates a Windows autostart manager.
// execPath is the path to the trasker-client binary.
func NewWindowsAutostart(execPath string) *WindowsAutostart {
	return &WindowsAutostart{execPath: execPath}
}

// NewAutostart returns an Autostart implementation for the current platform.
// execPath is the path to the trasker-client binary.
func NewAutostart(execPath string) Autostart {
	return NewWindowsAutostart(execPath)
}

// Enable creates a registry entry for Windows autostart.
func (a *WindowsAutostart) Enable() error {
	execPath := a.execPath
	if execPath == "" {
		var err error
		execPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("autostart windows: cannot determine executable path: %w", err)
		}
		execPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			return fmt.Errorf("autostart windows: cannot resolve executable path: %w", err)
		}
	}

	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		registryKeyPath,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("autostart windows: cannot open registry key: %w", err)
	}
	defer key.Close()

	// Quote the path in case it contains spaces.
	quotedPath := fmt.Sprintf(`"%s"`, execPath)
	if err := key.SetStringValue(registryValueName, quotedPath); err != nil {
		return fmt.Errorf("autostart windows: cannot set registry value: %w", err)
	}

	return nil
}

// Disable removes the registry entry for Windows autostart.
func (a *WindowsAutostart) Disable() error {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		registryKeyPath,
		registry.SET_VALUE,
	)
	if err != nil {
		// Key doesn't exist — already disabled.
		return nil
	}
	defer key.Close()

	err = key.DeleteValue(registryValueName)
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return fmt.Errorf("autostart windows: cannot delete registry value: %w", err)
	}
	return nil
}

// IsEnabled checks if the registry entry exists.
func (a *WindowsAutostart) IsEnabled() bool {
	key, err := registry.OpenKey(
		registry.CURRENT_USER,
		registryKeyPath,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return false // Key doesn't exist.
	}
	defer key.Close()

	_, _, err = key.GetStringValue(registryValueName)
	return err == nil
}
