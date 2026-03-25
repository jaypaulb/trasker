// internal/server/store/settings.go
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// OrgSettings represents the org_settings single-row table.
type OrgSettings struct {
	OrgName       string
	EntraTenant   string
	EntraClient   string
	EntraSecret   string
	KeyExpiryDays int
	UpdatedAt     time.Time
}

// GetOrgSettings retrieves the org settings (single row, id=1).
func (s *Store) GetOrgSettings(ctx context.Context) (*OrgSettings, error) {
	var o OrgSettings
	err := s.pool.QueryRow(ctx,
		`SELECT org_name, entra_tenant, entra_client, entra_secret, key_expiry_days, updated_at
		 FROM org_settings WHERE id = 1`,
	).Scan(&o.OrgName, &o.EntraTenant, &o.EntraClient, &o.EntraSecret, &o.KeyExpiryDays, &o.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting org settings: %w", err)
	}
	return &o, nil
}

// UpdateOrgSettingsParams holds the fields that can be updated on org settings.
type UpdateOrgSettingsParams struct {
	OrgName       *string
	EntraTenant   *string
	EntraClient   *string
	EntraSecret   *string
	KeyExpiryDays *int
}

// UpsertOrgSettings creates or updates the org settings.
func (s *Store) UpsertOrgSettings(ctx context.Context, p UpdateOrgSettingsParams) (*OrgSettings, error) {
	var o OrgSettings

	// Get current values (or defaults)
	current, err := s.GetOrgSettings(ctx)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("getting current settings: %w", err)
	}

	orgName := "My Organization"
	entraTenant := ""
	entraClient := ""
	entraSecret := ""
	keyExpiryDays := 60

	if current != nil {
		orgName = current.OrgName
		entraTenant = current.EntraTenant
		entraClient = current.EntraClient
		entraSecret = current.EntraSecret
		keyExpiryDays = current.KeyExpiryDays
	}

	if p.OrgName != nil {
		orgName = *p.OrgName
	}
	if p.EntraTenant != nil {
		entraTenant = *p.EntraTenant
	}
	if p.EntraClient != nil {
		entraClient = *p.EntraClient
	}
	if p.EntraSecret != nil {
		entraSecret = *p.EntraSecret
	}
	if p.KeyExpiryDays != nil {
		keyExpiryDays = *p.KeyExpiryDays
	}

	err = s.pool.QueryRow(ctx,
		`INSERT INTO org_settings (id, org_name, entra_tenant, entra_client, entra_secret, key_expiry_days, updated_at)
		 VALUES (1, $1, $2, $3, $4, $5, now())
		 ON CONFLICT (id) DO UPDATE SET
		   org_name = EXCLUDED.org_name,
		   entra_tenant = EXCLUDED.entra_tenant,
		   entra_client = EXCLUDED.entra_client,
		   entra_secret = EXCLUDED.entra_secret,
		   key_expiry_days = EXCLUDED.key_expiry_days,
		   updated_at = now()
		 RETURNING org_name, entra_tenant, entra_client, entra_secret, key_expiry_days, updated_at`,
		orgName, entraTenant, entraClient, entraSecret, keyExpiryDays,
	).Scan(&o.OrgName, &o.EntraTenant, &o.EntraClient, &o.EntraSecret, &o.KeyExpiryDays, &o.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting org settings: %w", err)
	}
	return &o, nil
}
