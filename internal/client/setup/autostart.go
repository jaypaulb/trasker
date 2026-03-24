// internal/client/setup/autostart.go
package setup

// Autostart manages OS-level autostart entries.
type Autostart interface {
	// Enable creates an autostart entry for the current binary.
	Enable() error

	// Disable removes the autostart entry.
	Disable() error

	// IsEnabled checks if autostart is configured.
	IsEnabled() bool
}
