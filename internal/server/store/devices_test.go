package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestUserAndKey creates a user + API key for FK dependencies.
func createTestUserAndKey(t *testing.T, s *store.Store, suffix string) (*store.User, *store.APIKey) {
	t.Helper()
	user := createTestUser(t, s, suffix)
	key, err := s.CreateAPIKey(context.Background(), store.CreateAPIKeyParams{
		UserID:    user.ID,
		KeyHash:   "$2a$10$fakehash" + suffix,
		KeyPrefix: "trsk_" + suffix[:3],
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)
	return user, key
}

func TestDevices_RegisterAndList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user, key := createTestUserAndKey(t, s, "dev1aa")

	device, err := s.UpsertDevice(ctx, store.UpsertDeviceParams{
		UserID:         user.ID,
		APIKeyID:       key.ID,
		ClientDeviceID: "client-uuid-111",
		OS:             "linux",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, device.ID)
	assert.Equal(t, "linux", device.OS)

	devices, err := s.ListDevicesByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, devices, 1)
	assert.Equal(t, "client-uuid-111", devices[0].ClientDeviceID)
}

func TestDevices_UpsertIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user, key := createTestUserAndKey(t, s, "dev2bb")

	params := store.UpsertDeviceParams{
		UserID:         user.ID,
		APIKeyID:       key.ID,
		ClientDeviceID: "client-uuid-222",
		OS:             "darwin",
	}

	d1, err := s.UpsertDevice(ctx, params)
	require.NoError(t, err)

	d2, err := s.UpsertDevice(ctx, params)
	require.NoError(t, err)

	// Same server-side ID (upsert, not duplicate)
	assert.Equal(t, d1.ID, d2.ID)
}

func TestDevices_UpdateName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user, key := createTestUserAndKey(t, s, "dev3cc")

	device, err := s.UpsertDevice(ctx, store.UpsertDeviceParams{
		UserID:         user.ID,
		APIKeyID:       key.ID,
		ClientDeviceID: "client-uuid-333",
		OS:             "windows",
	})
	require.NoError(t, err)
	assert.Nil(t, device.DeviceName)

	updated, err := s.UpdateDeviceName(ctx, device.ID, "My Work Laptop")
	require.NoError(t, err)
	require.NotNil(t, updated.DeviceName)
	assert.Equal(t, "My Work Laptop", *updated.DeviceName)
}
