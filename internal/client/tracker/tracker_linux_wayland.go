//go:build linux

// internal/client/tracker/tracker_linux_wayland.go
package tracker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// WaylandTracker implements the Tracker interface for Wayland sessions.
// It uses gdbus to monitor the GNOME Shell's active window changes via
// the org.gnome.Shell.Introspect interface, polling every 1 second.
//
// For Ubuntu 24.04 (GNOME 46+), this is the pragmatic approach.
// A native ext-foreign-toplevel-list-v1 implementation can replace this later.
type WaylandTracker struct {
	events chan FocusChange
	cancel context.CancelFunc

	lastApp   string
	lastTitle string
}

// NewWaylandTracker creates a new Wayland focus tracker.
func NewWaylandTracker() (*WaylandTracker, error) {
	// Verify that gdbus and the GNOME Shell introspection interface are available.
	cmd := exec.Command("gdbus", "call", "--session",
		"--dest", "org.gnome.Shell",
		"--object-path", "/org/gnome/Shell",
		"--method", "org.gnome.Shell.Eval",
		"global.display.focus_window ? global.display.focus_window.get_wm_class() : ''",
	)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("GNOME Shell D-Bus not available: %w (is this a GNOME Wayland session?)", err)
	}

	return &WaylandTracker{
		events: make(chan FocusChange, 32),
	}, nil
}

// Start begins polling the active window every 1 second.
func (t *WaylandTracker) Start(ctx context.Context) error {
	ctx, t.cancel = context.WithCancel(ctx)
	go t.poll(ctx)
	return nil
}

// Stop ceases polling and closes the events channel.
func (t *WaylandTracker) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	close(t.events)
}

// Events returns the channel of focus change events.
func (t *WaylandTracker) Events() <-chan FocusChange {
	return t.events
}

func (t *WaylandTracker) poll(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app, title := t.getActive()
			if app != t.lastApp || title != t.lastTitle {
				t.lastApp = app
				t.lastTitle = title
				t.events <- FocusChange{
					AppName:     app,
					WindowTitle: title,
					Timestamp:   time.Now().UTC(),
				}
			}
		}
	}
}

// getActive queries GNOME Shell for the focused window's WM class and title.
// It uses two separate gdbus calls:
//  1. Eval "global.display.focus_window.get_wm_class()" for the app name
//  2. Eval "global.display.focus_window.get_title()" for the window title
func (t *WaylandTracker) getActive() (string, string) {
	app := t.gnomeShellEval("global.display.focus_window ? global.display.focus_window.get_wm_class() : ''")
	title := t.gnomeShellEval("global.display.focus_window ? global.display.focus_window.get_title() : ''")
	return app, title
}

// gnomeShellEval runs a GNOME Shell JS expression via D-Bus and returns the result string.
func (t *WaylandTracker) gnomeShellEval(jsExpr string) string {
	cmd := exec.Command("gdbus", "call", "--session",
		"--dest", "org.gnome.Shell",
		"--object-path", "/org/gnome/Shell",
		"--method", "org.gnome.Shell.Eval",
		jsExpr,
	)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	// gdbus output format: (true, 'result_string')
	return parseGDBusEvalResult(string(out))
}

// parseGDBusEvalResult parses "(true, 'some string')" into "some string".
func parseGDBusEvalResult(raw string) string {
	raw = strings.TrimSpace(raw)

	// Format: (true, 'value') or (false, '')
	if !strings.HasPrefix(raw, "(true,") {
		return ""
	}

	// Try JSON-decoding the value part. The value is between single quotes,
	// but it may also be valid JSON string in double quotes depending on
	// GNOME version. Handle both.
	parts := strings.SplitN(raw, ",", 2)
	if len(parts) < 2 {
		return ""
	}

	val := strings.TrimSpace(parts[1])
	val = strings.TrimSuffix(val, ")")
	val = strings.TrimSpace(val)

	// Try stripping single quotes
	if strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'") {
		return val[1 : len(val)-1]
	}

	// Try JSON string
	var s string
	if json.Unmarshal([]byte(val), &s) == nil {
		return s
	}

	// Try scanning
	scanner := bufio.NewScanner(strings.NewReader(val))
	if scanner.Scan() {
		return scanner.Text()
	}

	return val
}
