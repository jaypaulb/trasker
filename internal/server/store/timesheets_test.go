// internal/server/store/timesheets_test.go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestDeviceChain creates user -> key -> device for timesheet FK deps.
func createTestDeviceChain(t *testing.T, s *store.Store, suffix string) (*store.User, *store.APIKey, *store.Device) {
	t.Helper()
	user, key := createTestUserAndKey(t, s, suffix)
	device, err := s.UpsertDevice(context.Background(), store.UpsertDeviceParams{
		UserID:         user.ID,
		APIKeyID:       key.ID,
		ClientDeviceID: "client-" + suffix,
		OS:             "linux",
	})
	require.NoError(t, err)
	return user, key, device
}

func TestTimesheets_CreateAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user, _, device := createTestDeviceChain(t, s, "ts1aaa")

	now := time.Now()
	ts, err := s.CreateTimesheet(ctx, store.CreateTimesheetParams{
		UserID:      user.ID,
		DeviceID:    device.ID,
		SubmittedAt: now,
		Entries: []store.CreateTimesheetEntryParams{
			{
				Tag:       "Development",
				StartedAt: now.Add(-3 * time.Hour),
				EndedAt:   now.Add(-30 * time.Minute),
				DurationS: 9000,
				Notes:     strPtr("Worked on auth module"),
			},
			{
				Tag:       "Communication",
				StartedAt: now.Add(-30 * time.Minute),
				EndedAt:   now,
				DurationS: 1800,
			},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, ts.ID)

	// Get by ID
	found, err := s.GetTimesheetByID(ctx, ts.ID)
	require.NoError(t, err)
	assert.Equal(t, ts.ID, found.ID)
	assert.Len(t, found.Entries, 2)
	assert.Equal(t, "Development", found.Entries[0].Tag)
}

func TestTimesheets_ListByUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	user, _, device := createTestDeviceChain(t, s, "ts2bbb")

	now := time.Now()
	for i := 0; i < 3; i++ {
		_, err := s.CreateTimesheet(ctx, store.CreateTimesheetParams{
			UserID:      user.ID,
			DeviceID:    device.ID,
			SubmittedAt: now,
			Entries: []store.CreateTimesheetEntryParams{
				{Tag: "Work", StartedAt: now.Add(-1 * time.Hour), EndedAt: now, DurationS: 3600},
			},
		})
		require.NoError(t, err)
	}

	timesheets, err := s.ListTimesheetsByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, timesheets, 3)
}

func TestTimesheets_ListByTeam(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now()

	// Create 2 users, each with a timesheet
	for _, suffix := range []string{"ts3cc1", "ts3cc2"} {
		user, _, device := createTestDeviceChain(t, s, suffix)
		_, err := s.CreateTimesheet(ctx, store.CreateTimesheetParams{
			UserID:      user.ID,
			DeviceID:    device.ID,
			SubmittedAt: now,
			Entries: []store.CreateTimesheetEntryParams{
				{Tag: "Dev", StartedAt: now.Add(-1 * time.Hour), EndedAt: now, DurationS: 3600},
			},
		})
		require.NoError(t, err)
	}

	// Team listing returns all timesheets
	all, err := s.ListTimesheetsAll(ctx, store.TimesheetFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestTimesheets_GetByID_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	_, err = s.GetTimesheetByID(context.Background(), uuid.New())
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func strPtr(s string) *string { return &s }
