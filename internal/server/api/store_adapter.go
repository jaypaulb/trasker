package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
)

// StoreAPIKeyAdapter adapts store.Store to auth.APIKeyLookup.
type StoreAPIKeyAdapter struct {
	store *store.Store
}

// NewStoreAPIKeyAdapter creates a new adapter.
func NewStoreAPIKeyAdapter(s *store.Store) *StoreAPIKeyAdapter {
	return &StoreAPIKeyAdapter{store: s}
}

// GetActiveAPIKeyByPrefix returns a single active API key matching the given prefix.
func (a *StoreAPIKeyAdapter) GetActiveAPIKeyByPrefix(ctx context.Context, prefix string) (*auth.APIKeyRecord, error) {
	return a.store.GetActiveAPIKeyByPrefix(ctx, prefix)
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp.
func (a *StoreAPIKeyAdapter) UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error {
	return a.store.UpdateAPIKeyLastUsed(ctx, id)
}
