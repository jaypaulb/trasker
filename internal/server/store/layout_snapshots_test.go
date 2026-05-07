package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedLayoutDevice creates a user, API key, and device suitable for layout snapshot
// FK dependencies. Returns the device.
func seedLayoutDevice(t *testing.T, s *store.Store, suffix string) *store.Device {
	t.Helper()
	user, key := createTestUserAndKey(t, s, suffix)
	device, err := s.UpsertDevice(context.Background(), store.UpsertDeviceParams{
		UserID:         user.ID,
		APIKeyID:       key.ID,
		ClientDeviceID: "client-" + suffix,
		OS:             "linux",
	})
	require.NoError(t, err)
	return device
}

// fakeWindowsJSON returns a tiny but valid windows JSONB payload.
func fakeWindowsJSON(title string) json.RawMessage {
	w := []map[string]any{
		{"app_name": "firefox", "window_title": title, "x": 0, "y": 0, "w": 800, "h": 600},
	}
	b, _ := json.Marshal(w)
	return json.RawMessage(b)
}

func TestLayoutSnapshots_InsertAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	device := seedLayoutDevice(t, s, "lay7aa")
	captured := time.Now().UTC().Truncate(time.Second)

	snap, err := s.InsertSnapshot(ctx, store.InsertSnapshotParams{
		DeviceID:    device.ID,
		CapturedAt:  captured,
		Windows:     fakeWindowsJSON("hello"),
		WindowsHash: "hash-aa",
	})
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Equal(t, device.ID, snap.DeviceID)
	assert.Equal(t, "raw", snap.Tier)

	got, err := s.GetSnapshotAt(ctx, device.ID, captured)
	require.NoError(t, err)
	assert.Equal(t, snap.ID, got.ID)
	assert.Equal(t, "hash-aa", got.WindowsHash)
}

func TestLayoutSnapshots_GetAt_MostRecentBefore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	device := seedLayoutDevice(t, s, "lay7bb")

	now := time.Now().UTC().Truncate(time.Second)
	// Insert at t-2h, t-1h, t.
	for i, off := range []time.Duration{-2 * time.Hour, -1 * time.Hour, 0} {
		_, err := s.InsertSnapshot(ctx, store.InsertSnapshotParams{
			DeviceID:    device.ID,
			CapturedAt:  now.Add(off),
			Windows:     fakeWindowsJSON("snap"),
			WindowsHash: "hash-bb-" + time.Duration(i).String(),
		})
		require.NoError(t, err)
	}

	// Query at t-30m -> expect t-1h row.
	got, err := s.GetSnapshotAt(ctx, device.ID, now.Add(-30*time.Minute))
	require.NoError(t, err)
	assert.WithinDuration(t, now.Add(-1*time.Hour), got.CapturedAt, time.Second)
}

func TestLayoutSnapshots_GetAt_NoneBefore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	device := seedLayoutDevice(t, s, "lay7cc")

	now := time.Now().UTC().Truncate(time.Second)
	_, err = s.InsertSnapshot(ctx, store.InsertSnapshotParams{
		DeviceID:    device.ID,
		CapturedAt:  now,
		Windows:     fakeWindowsJSON("snap"),
		WindowsHash: "hash-cc",
	})
	require.NoError(t, err)

	_, err = s.GetSnapshotAt(ctx, device.ID, now.Add(-1*time.Hour))
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestLayoutSnapshots_Dedup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	device := seedLayoutDevice(t, s, "lay7dd")
	captured := time.Now().UTC().Truncate(time.Second)
	params := store.InsertSnapshotParams{
		DeviceID:    device.ID,
		CapturedAt:  captured,
		Windows:     fakeWindowsJSON("dup"),
		WindowsHash: "hash-dup",
	}

	first, err := s.InsertSnapshot(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := s.InsertSnapshot(ctx, params)
	require.NoError(t, err)
	assert.Nil(t, second, "second insert with same dedup key should return (nil, nil)")

	var count int
	err = tdb.Pool.QueryRow(ctx,
		`SELECT count(*) FROM layout_snapshots WHERE device_id = $1`, device.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestLayoutSnapshots_ListTimestamps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	device := seedLayoutDevice(t, s, "lay7ee")
	now := time.Now().UTC().Truncate(time.Second)

	for i := 0; i < 5; i++ {
		_, err := s.InsertSnapshot(ctx, store.InsertSnapshotParams{
			DeviceID:    device.ID,
			CapturedAt:  now.Add(time.Duration(i) * time.Minute),
			Windows:     fakeWindowsJSON("listing"),
			WindowsHash: "hash-list-" + time.Duration(i).String(),
		})
		require.NoError(t, err)
	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	entries, err := s.ListTimestamps(ctx, device.ID, from, to, 10, 0)
	require.NoError(t, err)
	require.Len(t, entries, 5)
	// Ascending order on captured_at.
	for i := 1; i < len(entries); i++ {
		assert.True(t, !entries[i].CapturedAt.Before(entries[i-1].CapturedAt),
			"entries must be ascending")
	}
	for _, e := range entries {
		assert.Equal(t, 1, e.WindowsCount)
	}
}

// insertRawAged inserts a row directly (bypassing InsertSnapshot) so we can
// control captured_at and tier — needed for downsampling tests.
func insertRawAged(t *testing.T, tdb *testDB, deviceID interface{}, captured time.Time, hash string, tier string) {
	t.Helper()
	_, err := tdb.Pool.Exec(context.Background(),
		`INSERT INTO layout_snapshots (device_id, captured_at, windows, windows_hash, tier)
		 VALUES ($1, $2, $3::jsonb, $4, $5)`,
		deviceID, captured, string(fakeWindowsJSON("aged")), hash, tier,
	)
	require.NoError(t, err)
}

func TestDownsample_10min(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	device := seedLayoutDevice(t, s, "lay7ff")
	now := time.Now().UTC().Truncate(time.Second)

	// 6 raw rows, all aged > 7 days, all in the same 10-min bucket.
	bucketBase := now.Add(-10 * 24 * time.Hour).Truncate(10 * time.Minute)
	for i := 0; i < 6; i++ {
		insertRawAged(t, tdb, device.ID, bucketBase.Add(time.Duration(i)*time.Minute),
			"hash-down-"+time.Duration(i).String(), "raw")
	}

	deleted, err := s.DownsampleTo10Min(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, int64(5), deleted)

	var remaining int
	err = tdb.Pool.QueryRow(ctx,
		`SELECT count(*) FROM layout_snapshots WHERE device_id = $1`, device.ID,
	).Scan(&remaining)
	require.NoError(t, err)
	assert.Equal(t, 1, remaining)

	var tier string
	err = tdb.Pool.QueryRow(ctx,
		`SELECT tier FROM layout_snapshots WHERE device_id = $1`, device.ID,
	).Scan(&tier)
	require.NoError(t, err)
	assert.Equal(t, "10min", tier)
}

func TestDownsample_10min_LeavesRecentRaw(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	device := seedLayoutDevice(t, s, "lay7gg")
	now := time.Now().UTC().Truncate(time.Second)

	// 3 raw rows aged 1 day — under threshold.
	for i := 0; i < 3; i++ {
		insertRawAged(t, tdb, device.ID,
			now.Add(-24*time.Hour).Add(time.Duration(i)*time.Minute),
			"hash-recent-"+time.Duration(i).String(), "raw")
	}

	deleted, err := s.DownsampleTo10Min(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)

	var remaining int
	err = tdb.Pool.QueryRow(ctx,
		`SELECT count(*) FROM layout_snapshots WHERE device_id = $1`, device.ID,
	).Scan(&remaining)
	require.NoError(t, err)
	assert.Equal(t, 3, remaining)
}

func TestDownsample_1hr(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	device := seedLayoutDevice(t, s, "lay7hh")
	now := time.Now().UTC().Truncate(time.Second)

	// 12 tier='10min' rows aged > 37 days, all in same 1-hour bucket.
	bucketBase := now.Add(-40 * 24 * time.Hour).Truncate(time.Hour)
	for i := 0; i < 12; i++ {
		insertRawAged(t, tdb, device.ID,
			bucketBase.Add(time.Duration(i*5)*time.Minute),
			"hash-1hr-"+time.Duration(i).String(), "10min")
	}

	deleted, err := s.DownsampleTo1Hr(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, int64(11), deleted)

	var remaining int
	var tier string
	err = tdb.Pool.QueryRow(ctx,
		`SELECT count(*), max(tier) FROM layout_snapshots WHERE device_id = $1`, device.ID,
	).Scan(&remaining, &tier)
	require.NoError(t, err)
	assert.Equal(t, 1, remaining)
	assert.Equal(t, "1hr", tier)
}

func TestHasLayoutSnapshotsTable_Present(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	has, err := s.HasLayoutSnapshotsTable(ctx)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasLayoutSnapshotsTable_Absent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tdb := newTestDB(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)
	ctx := context.Background()

	_, err = tdb.Pool.Exec(ctx, `DROP TABLE layout_snapshots`)
	require.NoError(t, err)

	has, err := s.HasLayoutSnapshotsTable(ctx)
	require.NoError(t, err)
	assert.False(t, has)
}
