// internal/client/store/config.go
package store

import (
	"database/sql"
	"fmt"
)

// Config represents a row in the config table (always id=1).
type Config struct {
	DeviceID          string
	ServerURL         string
	APIKey            string
	TrackingOn        bool
	Autostart         bool
	PresenceIntervals string
	PomodoroDefaults  string
}

// InitConfig inserts the initial config row if it does not exist.
// If the row already exists, this is a no-op (first init wins).
func (s *Store) InitConfig(deviceID, serverURL, apiKey string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO config (id, device_id, server_url, api_key)
		 VALUES (1, ?, ?, ?)`,
		deviceID, serverURL, apiKey,
	)
	if err != nil {
		return fmt.Errorf("init config: %w", err)
	}
	return nil
}

// GetConfig returns the client configuration.
func (s *Store) GetConfig() (*Config, error) {
	var cfg Config
	var trackingOn, autostart int

	err := s.db.QueryRow(
		`SELECT device_id, server_url, api_key, tracking_on, autostart,
		        presence_intervals, pomodoro_defaults
		 FROM config WHERE id = 1`,
	).Scan(
		&cfg.DeviceID, &cfg.ServerURL, &cfg.APIKey,
		&trackingOn, &autostart,
		&cfg.PresenceIntervals, &cfg.PomodoroDefaults,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("config not initialized")
		}
		return nil, fmt.Errorf("get config: %w", err)
	}

	cfg.TrackingOn = trackingOn != 0
	cfg.Autostart = autostart != 0
	return &cfg, nil
}

// UpdateTrackingState sets the tracking_on flag.
func (s *Store) UpdateTrackingState(on bool) error {
	val := 0
	if on {
		val = 1
	}
	_, err := s.db.Exec(`UPDATE config SET tracking_on = ? WHERE id = 1`, val)
	if err != nil {
		return fmt.Errorf("update tracking state: %w", err)
	}
	return nil
}

// UpdateAutostart sets the autostart flag.
func (s *Store) UpdateAutostart(on bool) error {
	val := 0
	if on {
		val = 1
	}
	_, err := s.db.Exec(`UPDATE config SET autostart = ? WHERE id = 1`, val)
	if err != nil {
		return fmt.Errorf("update autostart: %w", err)
	}
	return nil
}
