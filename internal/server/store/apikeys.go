package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jaypaulb/trasker/internal/server/auth"
)

// APIKey represents a row in the api_keys table.
type APIKey struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	KeyHash    string
	KeyPrefix  string
	LastUsedAt time.Time
	ExpiresAt  time.Time
	Revoked    bool
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

// CreateAPIKeyParams holds the parameters for creating an API key.
type CreateAPIKeyParams struct {
	UserID    uuid.UUID
	KeyHash   string
	KeyPrefix string
	ExpiresAt time.Time
}

// CreateAPIKey inserts a new API key record.
func (s *Store) CreateAPIKey(ctx context.Context, p CreateAPIKeyParams) (*APIKey, error) {
	var k APIKey
	err := s.pool.QueryRow(ctx,
		`INSERT INTO api_keys (user_id, key_hash, key_prefix, expires_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, key_hash, key_prefix, last_used_at, expires_at, revoked, created_at, revoked_at`,
		p.UserID, p.KeyHash, p.KeyPrefix, p.ExpiresAt,
	).Scan(&k.ID, &k.UserID, &k.KeyHash, &k.KeyPrefix, &k.LastUsedAt, &k.ExpiresAt, &k.Revoked, &k.CreatedAt, &k.RevokedAt)
	if err != nil {
		return nil, fmt.Errorf("creating api key: %w", err)
	}
	return &k, nil
}

// GetAPIKeyByID retrieves an API key by its UUID.
func (s *Store) GetAPIKeyByID(ctx context.Context, id uuid.UUID) (*APIKey, error) {
	var k APIKey
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, key_hash, key_prefix, last_used_at, expires_at, revoked, created_at, revoked_at
		 FROM api_keys WHERE id = $1`,
		id,
	).Scan(&k.ID, &k.UserID, &k.KeyHash, &k.KeyPrefix, &k.LastUsedAt, &k.ExpiresAt, &k.Revoked, &k.CreatedAt, &k.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting api key by id: %w", err)
	}
	return &k, nil
}

// ListAPIKeysByUser returns all API keys for a user (including revoked/expired).
func (s *Store) ListAPIKeysByUser(ctx context.Context, userID uuid.UUID) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, key_hash, key_prefix, last_used_at, expires_at, revoked, created_at, revoked_at
		 FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyHash, &k.KeyPrefix, &k.LastUsedAt, &k.ExpiresAt, &k.Revoked, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scanning api key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// ListActiveAPIKeysByUser returns non-revoked, non-expired keys for a user.
func (s *Store) ListActiveAPIKeysByUser(ctx context.Context, userID uuid.UUID) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, key_hash, key_prefix, last_used_at, expires_at, revoked, created_at, revoked_at
		 FROM api_keys
		 WHERE user_id = $1 AND revoked = false AND expires_at > now()
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing active api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyHash, &k.KeyPrefix, &k.LastUsedAt, &k.ExpiresAt, &k.Revoked, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scanning api key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp to now().
func (s *Store) UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = now() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("updating api key last_used: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAllActiveAPIKeysWithRoles returns all active (non-revoked, non-expired) API keys
// joined with their user's role. Used by the API key authentication middleware.
func (s *Store) ListAllActiveAPIKeysWithRoles(ctx context.Context) ([]auth.APIKeyRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT k.id, k.user_id, k.key_hash, u.role, k.expires_at, k.revoked
		 FROM api_keys k
		 JOIN users u ON k.user_id = u.id
		 WHERE k.revoked = false AND k.expires_at > now()`)
	if err != nil {
		return nil, fmt.Errorf("listing active api keys with roles: %w", err)
	}
	defer rows.Close()

	var keys []auth.APIKeyRecord
	for rows.Next() {
		var k auth.APIKeyRecord
		if err := rows.Scan(&k.ID, &k.UserID, &k.KeyHash, &k.Role, &k.ExpiresAt, &k.Revoked); err != nil {
			return nil, fmt.Errorf("scanning api key record: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeAPIKey marks an API key as revoked.
func (s *Store) RevokeAPIKey(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET revoked = true, revoked_at = now() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("revoking api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
