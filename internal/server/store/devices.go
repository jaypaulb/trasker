package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Device represents a row in the devices table.
type Device struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	APIKeyID       uuid.UUID
	ClientDeviceID string
	DeviceName     *string
	OS             string
	LastSeenAt     time.Time
	CreatedAt      time.Time
}

// UpsertDeviceParams holds the parameters for registering or updating a device.
type UpsertDeviceParams struct {
	UserID         uuid.UUID
	APIKeyID       uuid.UUID
	ClientDeviceID string
	OS             string
	Hostname       string // used as default device_name on first registration
}

// UpsertDevice inserts a device or updates last_seen_at on conflict.
// Conflict key is (client_device_id, api_key_id).
func (s *Store) UpsertDevice(ctx context.Context, p UpsertDeviceParams) (*Device, error) {
	// Use hostname as default device_name on first insert (NULLIF avoids storing empty string).
	var d Device
	err := s.pool.QueryRow(ctx,
		`INSERT INTO devices (user_id, api_key_id, client_device_id, os, device_name)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		 ON CONFLICT (client_device_id, api_key_id) DO UPDATE SET last_seen_at = now()
		 RETURNING id, user_id, api_key_id, client_device_id, device_name, os, last_seen_at, created_at`,
		p.UserID, p.APIKeyID, p.ClientDeviceID, p.OS, p.Hostname,
	).Scan(&d.ID, &d.UserID, &d.APIKeyID, &d.ClientDeviceID, &d.DeviceName, &d.OS, &d.LastSeenAt, &d.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting device: %w", err)
	}
	return &d, nil
}

// ListDevicesByUser returns all devices for a user.
func (s *Store) ListDevicesByUser(ctx context.Context, userID uuid.UUID) ([]Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, api_key_id, client_device_id, device_name, os, last_seen_at, created_at
		 FROM devices WHERE user_id = $1 ORDER BY last_seen_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.APIKeyID, &d.ClientDeviceID, &d.DeviceName, &d.OS, &d.LastSeenAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetDeviceByClientID looks up a device by client_device_id and api_key_id.
func (s *Store) GetDeviceByClientID(ctx context.Context, clientDeviceID string, apiKeyID uuid.UUID) (*Device, error) {
	var d Device
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, api_key_id, client_device_id, device_name, os, last_seen_at, created_at
		 FROM devices WHERE client_device_id = $1 AND api_key_id = $2`,
		clientDeviceID, apiKeyID,
	).Scan(&d.ID, &d.UserID, &d.APIKeyID, &d.ClientDeviceID, &d.DeviceName, &d.OS, &d.LastSeenAt, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting device by client id: %w", err)
	}
	return &d, nil
}

// UpdateDeviceName sets the display name for a device.
func (s *Store) UpdateDeviceName(ctx context.Context, id uuid.UUID, name string) (*Device, error) {
	var d Device
	err := s.pool.QueryRow(ctx,
		`UPDATE devices SET device_name = $1
		 WHERE id = $2
		 RETURNING id, user_id, api_key_id, client_device_id, device_name, os, last_seen_at, created_at`,
		name, id,
	).Scan(&d.ID, &d.UserID, &d.APIKeyID, &d.ClientDeviceID, &d.DeviceName, &d.OS, &d.LastSeenAt, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("updating device name: %w", err)
	}
	return &d, nil
}
