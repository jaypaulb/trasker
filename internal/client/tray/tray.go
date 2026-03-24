// internal/client/tray/tray.go
package tray

import (
	"fmt"

	"fyne.io/systray"
)

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

// Tray manages the system tray icon and menu.
type Tray struct {
	actions   Actions
	menuDash  *systray.MenuItem
	menuPomo  *systray.MenuItem
	menuTrack *systray.MenuItem
	menuAuto  *systray.MenuItem
	menuQuit  *systray.MenuItem
}

// New creates a Tray. Call Run() to start it (blocks until quit).
func New(actions Actions) *Tray {
	return &Tray{actions: actions}
}

// Run starts the system tray. This blocks until the tray exits.
// Must be called from the main goroutine on macOS.
func (t *Tray) Run() {
	systray.Run(t.onReady, t.onExit)
}

// Quit requests the tray to close.
func (t *Tray) Quit() {
	systray.Quit()
}

func (t *Tray) onReady() {
	systray.SetTitle("Trasker")
	systray.SetTooltip("Trasker — Time Tracking")

	t.menuDash = systray.AddMenuItem("Open Dashboard...", "Open browser dashboard")
	t.menuPomo = systray.AddMenuItem("Start Pomodoro", "25m / 5m break")
	systray.AddSeparator()

	t.menuTrack = systray.AddMenuItemCheckbox("Tracking ON", "Toggle tracking", t.actions.IsTrackingOn())
	t.menuAuto = systray.AddMenuItemCheckbox("Start with OS", "Toggle autostart", t.actions.IsAutostartOn())
	systray.AddSeparator()

	t.menuQuit = systray.AddMenuItem("Quit Trasker", "Exit application")

	go t.eventLoop()
}

func (t *Tray) onExit() {
	// Cleanup if needed
}

func (t *Tray) eventLoop() {
	for {
		select {
		case <-t.menuDash.ClickedCh:
			t.actions.OpenDashboard()

		case <-t.menuPomo.ClickedCh:
			t.actions.StartPomodoro()

		case <-t.menuTrack.ClickedCh:
			newState := !t.actions.IsTrackingOn()
			t.actions.SetTrackingOn(newState)
			if newState {
				t.menuTrack.Check()
				t.menuTrack.SetTitle("Tracking ON")
			} else {
				t.menuTrack.Uncheck()
				t.menuTrack.SetTitle("Tracking OFF")
			}

		case <-t.menuAuto.ClickedCh:
			newState := !t.actions.IsAutostartOn()
			t.actions.SetAutostartOn(newState)
			if newState {
				t.menuAuto.Check()
			} else {
				t.menuAuto.Uncheck()
			}

		case <-t.menuQuit.ClickedCh:
			t.actions.Quit()
			systray.Quit()
			return
		}
	}
}

// UpdateTracking updates the tray display to reflect tracking state.
func (t *Tray) UpdateTracking(on bool) {
	if t.menuTrack == nil {
		return
	}
	if on {
		t.menuTrack.Check()
		t.menuTrack.SetTitle("Tracking ON")
		systray.SetTooltip("Trasker — Tracking")
	} else {
		t.menuTrack.Uncheck()
		t.menuTrack.SetTitle("Tracking OFF")
		systray.SetTooltip("Trasker — Paused")
	}
}

// UpdatePomodoro updates the tray to show pomodoro state.
func (t *Tray) UpdatePomodoro(info PomodoroInfo) {
	if t.menuPomo == nil {
		return
	}
	if info.Active {
		t.menuPomo.SetTitle(fmt.Sprintf("Pomodoro: %s (%s)", info.Remaining, info.State))
		systray.SetTitle(fmt.Sprintf("⏱ %s", info.Remaining))
	} else {
		t.menuPomo.SetTitle("Start Pomodoro")
		systray.SetTitle("Trasker")
	}
}
