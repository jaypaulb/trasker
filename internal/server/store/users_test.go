package store_test

import (
	"context"
	"testing"

	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsers_CreateAndGetByEntraOID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()

	user, err := s.CreateUser(ctx, store.CreateUserParams{
		EntraOID:    "oid-123",
		Email:       "alice@example.com",
		DisplayName: "Alice Smith",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "member", user.Role)
	assert.Equal(t, "alice@example.com", user.Email)

	// Get by Entra OID
	found, err := s.GetUserByEntraOID(ctx, "oid-123")
	require.NoError(t, err)
	assert.Equal(t, user.ID, found.ID)
	assert.Equal(t, "Alice Smith", found.DisplayName)
}

func TestUsers_GetByID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()

	user, err := s.CreateUser(ctx, store.CreateUserParams{
		EntraOID:    "oid-456",
		Email:       "bob@example.com",
		DisplayName: "Bob Jones",
	})
	require.NoError(t, err)

	found, err := s.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, found.ID)
	assert.Equal(t, "bob@example.com", found.Email)
}

func TestUsers_UpdateRole(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()

	user, err := s.CreateUser(ctx, store.CreateUserParams{
		EntraOID:    "oid-789",
		Email:       "carol@example.com",
		DisplayName: "Carol White",
	})
	require.NoError(t, err)
	assert.Equal(t, "member", user.Role)

	updated, err := s.UpdateUserRole(ctx, user.ID, "admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", updated.Role)
}

func TestUsers_List(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = s.CreateUser(ctx, store.CreateUserParams{
		EntraOID: "oid-a", Email: "a@example.com", DisplayName: "A",
	})
	require.NoError(t, err)
	_, err = s.CreateUser(ctx, store.CreateUserParams{
		EntraOID: "oid-b", Email: "b@example.com", DisplayName: "B",
	})
	require.NoError(t, err)

	users, err := s.ListUsers(ctx)
	require.NoError(t, err)
	assert.Len(t, users, 2)
}

func TestUsers_GetByEntraOID_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	_, err = s.GetUserByEntraOID(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, store.ErrNotFound)
}
