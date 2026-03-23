package store_test

import (
	"context"
	"testing"

	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/require"
)

func TestNewStore_ConnectsSuccessfully(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tdb := newTestDB(t)

	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	require.NotNil(t, s)

	// Verify we can ping
	err = s.Ping(context.Background())
	require.NoError(t, err)
}
