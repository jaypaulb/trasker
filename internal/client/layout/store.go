package layout

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// decodeWindows JSON-decodes a windows column. Empty or missing
// payloads decode to a nil slice (well-defined empty set).
func decodeWindows(raw string, dst *[]Window) error {
	if raw == "" {
		*dst = nil
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}

// Store wraps a *sql.DB and exposes layout_snapshots accessors. The
// underlying schema is created by internal/client/store.Store.migrate()
// so the same *sql.DB is shared between the focus-events and layout
// stores. Callers obtain it via internal/client/store.Store.DB().
type Store struct {
	db *sql.DB
}

// NewStore wraps a *sql.DB. It does not run migrations — the calling
// internal/client/store package's migrate() owns the schema literal.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// InsertSnapshot inserts a layout snapshot row and returns its id.
// windowsJSON is the encoding/json marshalling of []Window; the caller
// computes it (capturer marshals after change-detect).
func (s *Store) InsertSnapshot(capturedAt time.Time, windowsJSON []byte, hash string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT INTO layout_snapshots (captured_at, windows, windows_hash, created_at)
		 VALUES (?, ?, ?, ?)`,
		capturedAt.UTC().Format(time.RFC3339), string(windowsJSON), hash, now,
	)
	if err != nil {
		return 0, fmt.Errorf("layout store: insert snapshot: %w", err)
	}
	return result.LastInsertId()
}

// ListPending returns up to limit snapshots whose synced_at is NULL,
// oldest first. limit <= 0 means no limit.
func (s *Store) ListPending(limit int) ([]Snapshot, error) {
	q := `SELECT id, captured_at, windows, windows_hash, synced_at, retry_count, last_retry, created_at
		  FROM layout_snapshots
		  WHERE synced_at IS NULL
		  ORDER BY captured_at ASC`
	var (
		rows *sql.Rows
		err  error
	)
	if limit > 0 {
		rows, err = s.db.Query(q+" LIMIT ?", limit)
	} else {
		rows, err = s.db.Query(q)
	}
	if err != nil {
		return nil, fmt.Errorf("layout store: list pending: %w", err)
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		snap, err := scanSnapshotRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *snap)
	}
	return out, rows.Err()
}

// MarkSynced records syncedAt against the row and clears any retry state
// so the row will not be re-attempted.
func (s *Store) MarkSynced(id int64, syncedAt time.Time) error {
	_, err := s.db.Exec(
		`UPDATE layout_snapshots SET synced_at = ?, last_retry = NULL WHERE id = ?`,
		syncedAt.UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("layout store: mark synced %d: %w", id, err)
	}
	return nil
}

// BumpRetry sets retry_count and last_retry on a row that just failed
// transiently; sync goroutine consults backoff against last_retry.
func (s *Store) BumpRetry(id int64, retryCount int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE layout_snapshots SET retry_count = ?, last_retry = ? WHERE id = ?`,
		retryCount, now, id,
	)
	if err != nil {
		return fmt.Errorf("layout store: bump retry %d: %w", id, err)
	}
	return nil
}

// ListInRange returns all snapshots whose captured_at falls within
// [from, to] inclusive, ordered ascending.
func (s *Store) ListInRange(from, to time.Time) ([]Snapshot, error) {
	rows, err := s.db.Query(
		`SELECT id, captured_at, windows, windows_hash, synced_at, retry_count, last_retry, created_at
		 FROM layout_snapshots
		 WHERE captured_at >= ? AND captured_at <= ?
		 ORDER BY captured_at ASC`,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("layout store: list in range: %w", err)
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		snap, err := scanSnapshotRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *snap)
	}
	return out, rows.Err()
}

// Prune deletes rows whose captured_at is older than now - retention.
// Per D-01 the production caller passes 7 * 24 * time.Hour. Returns the
// number of rows deleted.
func (s *Store) Prune(retention time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-retention).Format(time.RFC3339)
	result, err := s.db.Exec(
		`DELETE FROM layout_snapshots WHERE captured_at < ?`, cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("layout store: prune: %w", err)
	}
	return result.RowsAffected()
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanSnapshotFromScanner parses one snapshot row, including JSON-decoding
// the windows column back into []Window. RFC3339 timestamps are parsed
// into time.Time per Pattern S-5.
func scanSnapshotFromScanner(sc scanner) (*Snapshot, error) {
	var snap Snapshot
	var capturedAt, createdAt, windowsJSON string
	var syncedAt, lastRetry sql.NullString
	var retryCount int

	err := sc.Scan(
		&snap.ID, &capturedAt, &windowsJSON, &snap.WindowsHash,
		&syncedAt, &retryCount, &lastRetry, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("layout snapshot not found")
		}
		return nil, fmt.Errorf("layout store: scan: %w", err)
	}

	t, parseErr := time.Parse(time.RFC3339, capturedAt)
	if parseErr != nil {
		return nil, fmt.Errorf("layout store: parse captured_at %q: %w", capturedAt, parseErr)
	}
	snap.CapturedAt = t

	t, parseErr = time.Parse(time.RFC3339, createdAt)
	if parseErr != nil {
		return nil, fmt.Errorf("layout store: parse created_at %q: %w", createdAt, parseErr)
	}
	snap.CreatedAt = t

	if syncedAt.Valid {
		t, parseErr := time.Parse(time.RFC3339, syncedAt.String)
		if parseErr != nil {
			return nil, fmt.Errorf("layout store: parse synced_at %q: %w", syncedAt.String, parseErr)
		}
		snap.SyncedAt = &t
	}
	_ = lastRetry  // present in the row but not surfaced on Snapshot
	_ = retryCount // ditto; sync.go reads these via raw SQL when needed

	if err := decodeWindows(windowsJSON, &snap.Windows); err != nil {
		return nil, fmt.Errorf("layout store: decode windows: %w", err)
	}
	return &snap, nil
}

func scanSnapshotRow(rows *sql.Rows) (*Snapshot, error) {
	return scanSnapshotFromScanner(rows)
}
