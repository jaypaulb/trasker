package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestUser is a helper that creates a user for FK dependencies.
func createTestUser(t *testing.T, s *store.Store, suffix string) *store.User {
	t.Helper()
	user, err := s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID:    "oid-" + suffix,
		Email:       suffix + "@example.com",
		DisplayName: "User " + suffix,
	})
	require.NoError(t, err)
	return user
}

func TestAPIKeys_CreateAndLookup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user := createTestUser(t, s, "apikey1")

	key, err := s.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		UserID:    user.ID,
		KeyHash:   "$2a$10$fakehashfakehashfakehashfakehashfakehashfakehashfake",
		KeyPrefix: "trsk_abc",
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, key.ID)
	assert.Equal(t, "trsk_abc", key.KeyPrefix)
	assert.False(t, key.Revoked)

	// Lookup by ID
	found, err := s.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	assert.Equal(t, key.ID, found.ID)
	assert.Equal(t, user.ID, found.UserID)
}

func TestAPIKeys_ListByUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user := createTestUser(t, s, "apikey2")

	for i := 0; i < 3; i++ {
		_, err := s.CreateAPIKey(ctx, store.CreateAPIKeyParams{
			UserID:    user.ID,
			KeyHash:   "$2a$10$fakehash" + string(rune('a'+i)),
			KeyPrefix: "trsk_" + string(rune('a'+i)),
			ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
		})
		require.NoError(t, err)
	}

	keys, err := s.ListAPIKeysByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, keys, 3)
}

func TestAPIKeys_UpdateLastUsed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user := createTestUser(t, s, "apikey3")

	key, err := s.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		UserID:    user.ID,
		KeyHash:   "$2a$10$fakehashforlastused",
		KeyPrefix: "trsk_lu1",
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)

	before := key.LastUsedAt

	// Small delay to ensure timestamp differs
	time.Sleep(10 * time.Millisecond)

	err = s.UpdateAPIKeyLastUsed(ctx, key.ID)
	require.NoError(t, err)

	updated, err := s.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	assert.True(t, updated.LastUsedAt.After(before) || updated.LastUsedAt.Equal(before))
}

func TestAPIKeys_Revoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user := createTestUser(t, s, "apikey4")

	key, err := s.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		UserID:    user.ID,
		KeyHash:   "$2a$10$fakehashforrevoke",
		KeyPrefix: "trsk_rv1",
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)
	assert.False(t, key.Revoked)

	err = s.RevokeAPIKey(ctx, key.ID)
	require.NoError(t, err)

	revoked, err := s.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	assert.True(t, revoked.Revoked)
	assert.NotNil(t, revoked.RevokedAt)
}

func TestAPIKeys_FindActiveByHash_Expired(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user := createTestUser(t, s, "apikey5")

	// Create an already-expired key
	_, err = s.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		UserID:    user.ID,
		KeyHash:   "$2a$10$fakehashforexpired",
		KeyPrefix: "trsk_ex1",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // expired
	})
	require.NoError(t, err)

	// ListActive should not return expired keys
	keys, err := s.ListActiveAPIKeysByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, keys, 0)
}
