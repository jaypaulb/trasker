// internal/server/store/audit_test.go
package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudit_LogAndList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	admin := createTestUser(t, s, "audit1")

	targetID := uuid.New()
	oldVal, _ := json.Marshal(map[string]string{"tag": "Dev"})
	newVal, _ := json.Marshal(map[string]string{"tag": "Development"})

	entry, err := s.CreateAuditLog(ctx, store.CreateAuditLogParams{
		AdminID:    admin.ID,
		Action:     "entry.update",
		TargetType: "timesheet_entry",
		TargetID:   targetID,
		OldValue:   oldVal,
		NewValue:   newVal,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, entry.ID)
	assert.Equal(t, "entry.update", entry.Action)

	// List all
	logs, err := s.ListAuditLogs(ctx, store.AuditLogFilters{})
	require.NoError(t, err)
	assert.Len(t, logs, 1)
}

func TestAudit_FilterByAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	admin := createTestUser(t, s, "audit2")

	for _, action := range []string{"entry.update", "entry.delete", "key.revoke"} {
		_, err := s.CreateAuditLog(ctx, store.CreateAuditLogParams{
			AdminID:    admin.ID,
			Action:     action,
			TargetType: "test",
			TargetID:   uuid.New(),
		})
		require.NoError(t, err)
	}

	action := "entry.delete"
	logs, err := s.ListAuditLogs(ctx, store.AuditLogFilters{Action: &action})
	require.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, "entry.delete", logs[0].Action)
}
