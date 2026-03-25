// internal/client/notify/notify_linux.go
//go:build linux

package notify

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

// New creates a platform-appropriate Notifier.
func New() (Notifier, error) {
	return NewLinuxNotifier()
}

// LinuxNotifier sends notifications via org.freedesktop.Notifications (libnotify/DBus).
type LinuxNotifier struct {
	conn      *dbus.Conn
	mu        sync.Mutex
	callbacks map[uint32]ClickAction
	nextID    uint32
}

// NewLinuxNotifier creates a new Linux notifier using the session DBus.
func NewLinuxNotifier() (*LinuxNotifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("notify linux: connect dbus: %w", err)
	}

	n := &LinuxNotifier{
		conn:      conn,
		callbacks: make(map[uint32]ClickAction),
	}

	// Listen for ActionInvoked signals
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/Notifications"),
		dbus.WithMatchInterface("org.freedesktop.Notifications"),
		dbus.WithMatchMember("ActionInvoked"),
	); err != nil {
		conn.Close()
		return nil, fmt.Errorf("notify linux: add match: %w", err)
	}

	go n.listenSignals()

	return n, nil
}

// Notify sends a desktop notification.
func (n *LinuxNotifier) Notify(title, body string, onClick ClickAction) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	obj := n.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")

	actions := []string{}
	if onClick != nil {
		actions = []string{"default", "Open"}
	}

	call := obj.Call("org.freedesktop.Notifications.Notify", 0,
		"trasker",                 // app_name
		uint32(0),                 // replaces_id
		"",                        // app_icon
		title,                     // summary
		body,                      // body
		actions,                   // actions
		map[string]dbus.Variant{}, // hints
		int32(10000),              // expire_timeout (ms), -1 = server default
	)
	if call.Err != nil {
		return fmt.Errorf("notify linux: send: %w", call.Err)
	}

	var id uint32
	if err := call.Store(&id); err != nil {
		return fmt.Errorf("notify linux: store id: %w", err)
	}

	if onClick != nil {
		n.callbacks[id] = onClick
	}

	return nil
}

// Close disconnects from DBus.
func (n *LinuxNotifier) Close() error {
	return n.conn.Close()
}

// listenSignals handles ActionInvoked signals from the notification server.
func (n *LinuxNotifier) listenSignals() {
	ch := make(chan *dbus.Signal, 16)
	n.conn.Signal(ch)

	for sig := range ch {
		if sig.Name != "org.freedesktop.Notifications.ActionInvoked" {
			continue
		}
		if len(sig.Body) < 2 {
			continue
		}

		id, ok := sig.Body[0].(uint32)
		if !ok {
			continue
		}

		n.mu.Lock()
		cb, exists := n.callbacks[id]
		if exists {
			delete(n.callbacks, id)
		}
		n.mu.Unlock()

		if exists && cb != nil {
			go cb()
		}
	}
}
