//go:build linux

package tray

import (
	"time"

	"github.com/godbus/dbus/v5"
)

// IsAvailable reports whether a system tray host is registered on the
// session DBus. The fyne.io/systray library on Linux speaks the KDE
// StatusNotifierItem protocol, which requires a StatusNotifierWatcher
// — provided by the Plasma panel, the GNOME `appindicator` extension,
// xfce4-panel, etc. On vanilla GNOME or a headless session there is
// no watcher and `systray.Run` silently fails to put an icon up
// (it prints `failed to register: org.kde.StatusNotifierWatcher was
// not provided`, but Run() returns 0 anyway — see fyne.io/systray
// systray_unix.go register()).
//
// We probe with `org.freedesktop.DBus.NameHasOwner` BEFORE calling
// Run. If absent, callers should skip the tray and use a notify-send
// fallback to surface the dashboard URL on startup. 2-second timeout
// — a wedged DBus is treated as "no tray" so we don't block daemon
// startup.
func IsAvailable() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()

	type result struct {
		owns bool
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		var has bool
		call := conn.BusObject().Call(
			"org.freedesktop.DBus.NameHasOwner", 0,
			"org.kde.StatusNotifierWatcher",
		)
		if call.Err != nil {
			resultCh <- result{false, call.Err}
			return
		}
		if err := call.Store(&has); err != nil {
			resultCh <- result{false, err}
			return
		}
		resultCh <- result{has, nil}
	}()

	select {
	case r := <-resultCh:
		if r.err != nil {
			return false
		}
		return r.owns
	case <-time.After(2 * time.Second):
		return false
	}
}
