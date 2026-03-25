// internal/client/notify/dispatcher.go
package notify

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

// Event types that trigger notifications.
type EventType int

const (
	EventDeadman       EventType = iota // "Still there?" presence check
	EventLongFocus                      // 1hr+ on same window
	EventTrackingOff                    // Tracking is paused reminder
	EventPomodoroWork                   // Break finished, back to work
	EventPomodoroBreak                  // Work finished, take a break
	EventPomodoroDone                   // Full cycle complete
)

// Dispatcher routes application events to OS notifications.
type Dispatcher struct {
	notifier Notifier
	logger   *slog.Logger
}

// NewDispatcher creates a notification dispatcher.
func NewDispatcher(notifier Notifier, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		notifier: notifier,
		logger:   logger,
	}
}

// DeadmanCheck sends a "Still there?" notification with a countdown.
// Returns a channel that receives true if clicked, false if expired.
func (d *Dispatcher) DeadmanCheck(timeout time.Duration) chan bool {
	result := make(chan bool, 1)
	var clicked atomic.Bool

	onClick := func() {
		if clicked.CompareAndSwap(false, true) {
			result <- true
		}
	}

	err := d.notifier.Notify(
		"Trasker — Still there?",
		fmt.Sprintf("Click within %d seconds to confirm you're active.", int(timeout.Seconds())),
		onClick,
	)
	if err != nil {
		d.logger.Error("failed to send deadman notification", "error", err)
		result <- false
		return result
	}

	go func() {
		time.Sleep(timeout)
		if clicked.CompareAndSwap(false, true) {
			result <- false
		}
	}()

	return result
}

// LongFocusReminder notifies about extended focus on one window.
func (d *Dispatcher) LongFocusReminder(appName string, duration time.Duration) {
	hours := int(duration.Hours())
	mins := int(duration.Minutes()) % 60
	body := fmt.Sprintf("You've been focused on %s for %dh%dm.", appName, hours, mins)

	if err := d.notifier.Notify("Trasker — Long Focus", body, nil); err != nil {
		d.logger.Error("failed to send long focus notification", "error", err)
	}
}

// TrackingOffNag sends a gentle reminder that tracking is paused.
func (d *Dispatcher) TrackingOffNag(onResume ClickAction) {
	if err := d.notifier.Notify(
		"Trasker is paused",
		"Click to resume tracking.",
		onResume,
	); err != nil {
		d.logger.Error("failed to send tracking-off nag", "error", err)
	}
}

// PomodoroTransition notifies about pomodoro state changes.
func (d *Dispatcher) PomodoroTransition(eventType EventType) {
	var title, body string
	switch eventType {
	case EventPomodoroBreak:
		title = "Trasker — Break Time"
		body = "Work phase complete. Take a break!"
	case EventPomodoroWork:
		title = "Trasker — Back to Work"
		body = "Break is over. Time to focus!"
	case EventPomodoroDone:
		title = "Trasker — Pomodoro Complete"
		body = "Full pomodoro cycle finished."
	default:
		return
	}

	if err := d.notifier.Notify(title, body, nil); err != nil {
		d.logger.Error("failed to send pomodoro notification", "error", err)
	}
}
