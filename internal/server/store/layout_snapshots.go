package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// LayoutSnapshot represents a single row in layout_snapshots.
//
// The Windows column is held as raw JSON bytes — the server is a passthrough
// for the client's payload and never mutates JSONB in place (RESEARCH Pitfall 7;
// downsampling deletes losers + updates tier on survivors, never an in-place
// JSONB rewrite).
type LayoutSnapshot struct {
	ID          uuid.UUID
	DeviceID    uuid.UUID
	CapturedAt  time.Time
	Windows     json.RawMessage
	WindowsHash string
	Tier        string
	CreatedAt   time.Time
}

// InsertSnapshotParams captures the fields needed to ingest one snapshot row.
type InsertSnapshotParams struct {
	DeviceID    uuid.UUID
	CapturedAt  time.Time
	Windows     json.RawMessage
	WindowsHash string
}

// TimelineEntry is the lightweight projection returned by ListTimestamps.
// Window contents are NOT included — the dashboard timeline only needs counts.
type TimelineEntry struct {
	ID           uuid.UUID
	CapturedAt   time.Time
	WindowsCount int
}

// layoutSnapshotColumns is the standard SELECT list for layout_snapshots.
const layoutSnapshotColumns = `id, device_id, captured_at, windows, windows_hash, tier, created_at`

// scanLayoutSnapshot scans a row into a LayoutSnapshot. Windows is read into
// []byte then handed back as json.RawMessage — pgx returns JSONB as []byte.
func scanLayoutSnapshot(row pgx.Row) (*LayoutSnapshot, error) {
	var s LayoutSnapshot
	var windows []byte
	if err := row.Scan(&s.ID, &s.DeviceID, &s.CapturedAt, &windows, &s.WindowsHash, &s.Tier, &s.CreatedAt); err != nil {
		return nil, err
	}
	s.Windows = json.RawMessage(windows)
	return &s, nil
}

// InsertSnapshot inserts a new snapshot row.
//
// Conflict on (device_id, captured_at, windows_hash) returns (nil, nil) — the
// caller treats absent row as "client retry already ingested" per RESEARCH
// Open Question #3. This makes the endpoint idempotent.
func (s *Store) InsertSnapshot(ctx context.Context, p InsertSnapshotParams) (*LayoutSnapshot, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO layout_snapshots (device_id, captured_at, windows, windows_hash, tier)
		 VALUES ($1, $2, $3, $4, 'raw')
		 ON CONFLICT (device_id, captured_at, windows_hash) DO NOTHING
		 RETURNING `+layoutSnapshotColumns,
		p.DeviceID, p.CapturedAt, []byte(p.Windows), p.WindowsHash,
	)
	snap, err := scanLayoutSnapshot(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Conflict — caller treats as idempotent success.
			return nil, nil
		}
		return nil, fmt.Errorf("inserting layout snapshot: %w", err)
	}
	return snap, nil
}

// GetSnapshotAt returns the most-recent snapshot for the device with
// captured_at <= t. Snapshots are sparse (no row when nothing changed), so
// the at-or-before semantic is what callers want — never use captured_at = $.
func (s *Store) GetSnapshotAt(ctx context.Context, deviceID uuid.UUID, t time.Time) (*LayoutSnapshot, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+layoutSnapshotColumns+`
		 FROM layout_snapshots
		 WHERE device_id = $1 AND captured_at <= $2
		 ORDER BY captured_at DESC
		 LIMIT 1`,
		deviceID, t,
	)
	snap, err := scanLayoutSnapshot(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting layout snapshot at: %w", err)
	}
	return snap, nil
}

// ListTimestamps returns timeline entries for the device in [from, to], ascending.
//
// Window contents are NOT fetched here (only jsonb_array_length); the timeline
// view only needs counts. Use GetSnapshotAt to fetch a specific row's payload.
func (s *Store) ListTimestamps(ctx context.Context, deviceID uuid.UUID, from, to time.Time, limit, offset int) ([]TimelineEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, captured_at, jsonb_array_length(windows) AS windows_count
		 FROM layout_snapshots
		 WHERE device_id = $1 AND captured_at >= $2 AND captured_at <= $3
		 ORDER BY captured_at ASC
		 LIMIT $4 OFFSET $5`,
		deviceID, from, to, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing layout snapshots: %w", err)
	}
	defer rows.Close()

	entries := []TimelineEntry{}
	for rows.Next() {
		var e TimelineEntry
		if err := rows.Scan(&e.ID, &e.CapturedAt, &e.WindowsCount); err != nil {
			return nil, fmt.Errorf("scanning timeline entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// downsampleSQL returns the bucketing CTE used by both downsamplers.
// targetTier is 'raw'->'10min' or '10min'->'1hr'. ageInterval is the
// minimum age threshold ('7 days' or '37 days'). bucketExpr is the
// PARTITION BY expression that defines a bucket.
//
// IMPORTANT: never UPDATE the windows JSONB column (RESEARCH Pitfall 7
// — JSONB in-place mutation causes WAL bloat). The CTE picks the survivor
// (rn=1, ordered by captured_at ASC), promotes it via UPDATE on tier only,
// and DELETEs the losers.
const downsampleSQL = `
WITH bucketed AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY device_id, %s
               ORDER BY captured_at ASC, id ASC
           ) AS rn
    FROM layout_snapshots
    WHERE tier = $2 AND captured_at < $1::timestamptz - $3::interval
),
survivors AS (
    SELECT id FROM bucketed WHERE rn = 1
),
losers AS (
    SELECT id FROM bucketed WHERE rn > 1
),
promoted AS (
    UPDATE layout_snapshots SET tier = $4
    WHERE id IN (SELECT id FROM survivors)
    RETURNING id
),
deleted AS (
    DELETE FROM layout_snapshots
    WHERE id IN (SELECT id FROM losers)
    RETURNING id
)
SELECT
    (SELECT count(*) FROM deleted)::bigint,
    (SELECT count(*) FROM promoted)::bigint
`

// DownsampleTo10Min collapses raw snapshots older than 7 days into one
// representative row per (device_id, 10-minute bucket). Returns the number
// of rows deleted (the losers); the survivor count is logged via the
// query but only the delete count is surfaced to the caller.
func (s *Store) DownsampleTo10Min(ctx context.Context, now time.Time) (int64, error) {
	// Bucket = floor(captured_at to 10-minute boundary).
	bucketExpr := `date_trunc('hour', captured_at) + ` +
		`(floor(extract(minute FROM captured_at)::int / 10) * interval '10 minutes')`
	q := fmt.Sprintf(downsampleSQL, bucketExpr)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin downsample 10min: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is best-effort after Commit

	var deleted, promoted int64
	if err := tx.QueryRow(ctx, q, now, "raw", "7 days", "10min").Scan(&deleted, &promoted); err != nil {
		return 0, fmt.Errorf("downsample 10min: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit downsample 10min: %w", err)
	}
	_ = promoted
	return deleted, nil
}

// DownsampleTo1Hr collapses tier='10min' snapshots older than 37 days into
// one representative row per (device_id, 1-hour bucket). Returns delete count.
func (s *Store) DownsampleTo1Hr(ctx context.Context, now time.Time) (int64, error) {
	bucketExpr := `date_trunc('hour', captured_at)`
	q := fmt.Sprintf(downsampleSQL, bucketExpr)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin downsample 1hr: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var deleted, promoted int64
	if err := tx.QueryRow(ctx, q, now, "10min", "37 days", "1hr").Scan(&deleted, &promoted); err != nil {
		return 0, fmt.Errorf("downsample 1hr: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit downsample 1hr: %w", err)
	}
	_ = promoted
	return deleted, nil
}

// HasLayoutSnapshotsTable returns true if the layout_snapshots table exists in
// the public schema. Handlers use this to degrade gracefully (503 with operator
// instructions) when migrations/004 has not been applied — the server has no
// in-process migration runner (RESEARCH Pitfall 5).
func (s *Store) HasLayoutSnapshotsTable(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'layout_snapshots'
		)`,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking layout_snapshots table: %w", err)
	}
	return exists, nil
}
