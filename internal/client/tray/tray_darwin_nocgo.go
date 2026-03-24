//go:build darwin && !cgo

// internal/client/tray/tray_darwin_nocgo.go
// No-op stub for darwin cross-compilation without CGo.
// fyne.io/systray requires Objective-C/CGo on macOS.
// When CGO_ENABLED=0 (cross-compiling from Linux), this stub is used instead.
// On native macOS builds, CGo is available and tray.go is used.
package tray

import "fmt"

// Actions is the interface the tray uses to trigger application behaviors.
type Actions interface {
	OpenDashboard()
	StartPomodoro()
	StopPomodoro()
	IsTrackingOn() bool
	SetTrackingOn(on bool)
	IsAutostartOn() bool
	SetAutostartOn(on bool)
	Quit()
}

// PomodoroInfo provides current pomodoro state for tray display.
type PomodoroInfo struct {
	Active    bool
	State     string // "work", "break", "idle"
	Remaining string // e.g., "18:32"
}

// Tray is a no-op tray for darwin cross-compiled without CGo.
// The actual systray implementation requires CGo (Objective-C frameworks).
type Tray struct {
	actions Actions
}

// New creates a no-op Tray.
func New(actions Actions) *Tray {
	return &Tray{actions: actions}
}

// Run is a no-op — systray requires CGo on macOS.
// Logs a warning and returns immediately.
func (t *Tray) Run() {
	fmt.Println("tray: system tray unavailable in this build (CGO_ENABLED=0); run natively on macOS for tray support")
}

// Quit is a no-op.
func (t *Tray) Quit() {}

// UpdateTracking is a no-op.
func (t *Tray) UpdateTracking(on bool) {}

// UpdatePomodoro is a no-op.
func (t *Tray) UpdatePomodoro(info PomodoroInfo) {}
