// internal/client/notify/notify.go
package notify

// ClickAction is a callback invoked when the user clicks a notification.
type ClickAction func()

// Notifier sends OS-native notifications.
type Notifier interface {
	// Notify sends a notification. onClick is called if the user clicks it (may be nil).
	Notify(title, body string, onClick ClickAction) error

	// Close cleans up resources.
	Close() error
}
