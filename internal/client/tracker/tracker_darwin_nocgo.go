//go:build darwin && !cgo

// internal/client/tracker/tracker_darwin_nocgo.go
// No-op stub for darwin cross-compilation without CGo.
// The CGo-based tracker_darwin.go requires Objective-C frameworks.
// When CGO_ENABLED=0 (cross-compiling from Linux), this stub is used instead.
// On native macOS builds, CGo is available and tracker_darwin.go is used.
package tracker

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// darwinTracker is the non-CGo fallback for macOS.
// Uses osascript to query the frontmost application.
type darwinTracker struct {
	pollInterval time.Duration
	events       chan FocusChange
	done         chan struct{}
}

// NewPlatformTracker creates a macOS focus tracker using osascript (no CGo).
func NewPlatformTracker() (Tracker, error) {
	return &darwinTracker{
		pollInterval: 1 * time.Second,
		events:       make(chan FocusChange, 64),
		done:         make(chan struct{}),
	}, nil
}

func (t *darwinTracker) Start(ctx context.Context) error {
	go t.pollLoop(ctx)
	return nil
}

func (t *darwinTracker) Events() <-chan FocusChange {
	return t.events
}

func (t *darwinTracker) Stop() {
	close(t.done)
}

func (t *darwinTracker) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	var lastApp, lastTitle string

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		case <-ticker.C:
			app, title := t.getFocusedWindow()
			if app != lastApp || title != lastTitle {
				event := FocusChange{
					AppName:     app,
					WindowTitle: title,
					Timestamp:   time.Now().UTC(),
				}
				select {
				case t.events <- event:
				default:
				}
				lastApp = app
				lastTitle = title
			}
		}
	}
}

func (t *darwinTracker) getFocusedWindow() (appName, windowTitle string) {
	appScript := `tell application "System Events" to get name of first application process whose frontmost is true`
	out, err := exec.Command("osascript", "-e", appScript).Output()
	if err == nil {
		appName = strings.TrimSpace(string(out))
	}
	if appName != "" {
		titleScript := `tell application "System Events" to get name of front window of application process "` + appName + `"`
		out, err = exec.Command("osascript", "-e", titleScript).Output()
		if err == nil {
			windowTitle = strings.TrimSpace(string(out))
		}
	}
	return appName, windowTitle
}
