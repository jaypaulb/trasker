//go:build darwin

// internal/client/setup/autostart_darwin.go
package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const (
	launchAgentDir = "Library/LaunchAgents"
	plistFileName  = "com.trasker.client.plist"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.trasker.client</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.ExecutablePath}}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardOutPath</key>
    <string>{{.LogDir}}/trasker-stdout.log</string>
    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/trasker-stderr.log</string>
</dict>
</plist>
`

type plistData struct {
	ExecutablePath string
	LogDir         string
}

// plistPathFn is the path resolver — overridable in tests to use temp dirs.
var plistPathFn = plistPathDefault

func plistPathDefault() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, launchAgentDir, plistFileName), nil
}

// DarwinAutostart manages ~/Library/LaunchAgents/com.trasker.client.plist.
type DarwinAutostart struct {
	execPath string
}

// NewDarwinAutostart creates a macOS autostart manager.
// execPath is the path to the trasker-client binary.
func NewDarwinAutostart(execPath string) *DarwinAutostart {
	return &DarwinAutostart{execPath: execPath}
}

// NewAutostart returns an Autostart implementation for the current platform.
// execPath is the path to the trasker-client binary.
func NewAutostart(execPath string) Autostart {
	return NewDarwinAutostart(execPath)
}

// Enable creates the LaunchAgent plist for macOS autostart.
func (a *DarwinAutostart) Enable() error {
	execPath := a.execPath
	if execPath == "" {
		var err error
		execPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("autostart darwin: cannot determine executable path: %w", err)
		}
		execPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			return fmt.Errorf("autostart darwin: cannot resolve executable path: %w", err)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("autostart darwin: cannot determine home directory: %w", err)
	}

	logDir := filepath.Join(home, "Library", "Logs", "Trasker")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("autostart darwin: cannot create log directory: %w", err)
	}

	pPath, err := plistPathFn()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(pPath), 0755); err != nil {
		return fmt.Errorf("autostart darwin: cannot create LaunchAgents directory: %w", err)
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("autostart darwin: cannot parse plist template: %w", err)
	}

	f, err := os.Create(pPath)
	if err != nil {
		return fmt.Errorf("autostart darwin: cannot create plist file: %w", err)
	}
	defer f.Close()

	data := plistData{
		ExecutablePath: execPath,
		LogDir:         logDir,
	}
	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("autostart darwin: cannot write plist file: %w", err)
	}

	return nil
}

// Disable removes the LaunchAgent plist.
func (a *DarwinAutostart) Disable() error {
	pPath, err := plistPathFn()
	if err != nil {
		return err
	}

	err = os.Remove(pPath)
	if os.IsNotExist(err) {
		return nil // Already disabled.
	}
	if err != nil {
		return fmt.Errorf("autostart darwin: remove plist file: %w", err)
	}
	return nil
}

// IsEnabled checks if the LaunchAgent plist exists.
func (a *DarwinAutostart) IsEnabled() bool {
	pPath, err := plistPathFn()
	if err != nil {
		return false
	}
	_, err = os.Stat(pPath)
	return err == nil
}
