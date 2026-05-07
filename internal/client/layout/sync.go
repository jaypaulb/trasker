package layout

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	gosync "sync"
	"time"

	clientsync "github.com/jaypaulb/trasker/internal/client/sync"
)

// backoffSchedule mirrors internal/client/sync/queue.go (CONSTRAINT-error-handling).
// Three Examples: layout sync is the second user of this schedule;
// extracting a shared abstraction is deferred until a third sync target
// exists (RESEARCH.md Pattern 4 option A).
var backoffSchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	1 * time.Hour,
}

// Re-export the shared sentinel errors so callers in this package can
// match against them without importing internal/client/sync directly.
// Three Examples: ErrKeyExpired/ErrKeyRevoked are project-wide error
// shapes (not entity-specific), so re-using them keeps the sentinel set
// unified.
var (
	ErrKeyExpired = clientsync.ErrKeyExpired
	ErrKeyRevoked = clientsync.ErrKeyRevoked
)

// SyncerConfig wires the layout-sync goroutine. Production callers pass
// Interval = 5*time.Minute (matches existing sync.Queue cadence) and
// BatchSize = 100. Tests pass shorter intervals + small batch sizes.
type SyncerConfig struct {
	Store      *Store
	DB         *sql.DB // shared client SQLite handle (for retry_count reads)
	DeviceID   string
	ServerURL  string
	APIKey     string
	HTTPClient *http.Client
	Interval   time.Duration
	Logger     *slog.Logger
	BatchSize  int // 0 = no limit
}

// Syncer drains pending layout_snapshots rows in batches over HTTPS.
// Mirrors internal/client/sync.Queue verbatim in shape (Start/Stop,
// running flag under Mutex, ProcessPending entry point) per
// 07-PATTERNS.md "sync.go".
type Syncer struct {
	cfg     SyncerConfig
	mu      gosync.Mutex
	running bool
	stopCh  chan struct{}
}

// NewSyncer constructs a Syncer. The HTTPClient field is injectable for
// tests; nil falls back to a 30s-timeout default.
func NewSyncer(cfg SyncerConfig) *Syncer {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}
	return &Syncer{cfg: cfg}
}

// Start begins periodic processing. Mirrors sync.Queue.Start exactly:
// runs ProcessPending immediately, then ticks at cfg.Interval.
func (s *Syncer) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()
	go s.run(ctx)
}

// Stop halts processing. Idempotent.
func (s *Syncer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false
	}
}

func (s *Syncer) run(ctx context.Context) {
	s.ProcessPending(ctx)
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.ProcessPending(ctx)
		}
	}
}

// pendingRow carries the raw row data plus retry counters that the
// public Snapshot type does not surface.
type pendingRow struct {
	id          int64
	capturedAt  time.Time
	windowsJSON string
	hash        string
	retryCount  int
	lastRetry   *time.Time
}

// ProcessPending drains all pending layout_snapshots that are due
// according to the backoff schedule. Mirrors sync.Queue.ProcessPending.
func (s *Syncer) ProcessPending(ctx context.Context) {
	due, err := s.listDueSnapshots()
	if err != nil {
		s.cfg.Logger.Error("layout sync: failed to list pending", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}

	if s.cfg.BatchSize > 0 && len(due) > s.cfg.BatchSize {
		due = due[:s.cfg.BatchSize]
	}

	if err := s.postBatch(ctx, due); err != nil {
		// Permanent errors stop the entire batch: do NOT bump retry
		// counters (matches sync.Queue.processOne semantics).
		if err == clientsync.ErrKeyExpired || err == clientsync.ErrKeyRevoked {
			s.cfg.Logger.Error("layout sync: permanent error", "error", err)
			return
		}
		// Transient — bump retry on every row in this batch.
		for _, r := range due {
			s.cfg.Logger.Warn("layout sync: post failed, will retry",
				"snapshot_id", r.id,
				"retry_count", r.retryCount+1,
				"error", err,
			)
			if bumpErr := s.cfg.Store.BumpRetry(r.id, r.retryCount+1); bumpErr != nil {
				s.cfg.Logger.Error("layout sync: bump retry failed", "id", r.id, "error", bumpErr)
			}
		}
		return
	}

	// Success — mark each row synced.
	now := time.Now().UTC()
	for _, r := range due {
		if err := s.cfg.Store.MarkSynced(r.id, now); err != nil {
			s.cfg.Logger.Error("layout sync: mark synced failed", "id", r.id, "error", err)
			continue
		}
	}
	s.cfg.Logger.Info("layout sync: batch posted",
		"count", len(due),
	)
}

// listDueSnapshots reads pending rows directly via raw SQL so it can
// access retry_count/last_retry which Snapshot deliberately doesn't
// surface. Filters by backoff schedule, mirroring queue.go listDueSubmissions.
func (s *Syncer) listDueSnapshots() ([]pendingRow, error) {
	rows, err := s.cfg.DB.Query(
		`SELECT id, captured_at, windows, windows_hash, retry_count, last_retry
		 FROM layout_snapshots
		 WHERE synced_at IS NULL
		 ORDER BY captured_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("layout sync: list pending: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var due []pendingRow
	for rows.Next() {
		var r pendingRow
		var capturedAt string
		var lastRetry sql.NullString
		if err := rows.Scan(&r.id, &capturedAt, &r.windowsJSON, &r.hash, &r.retryCount, &lastRetry); err != nil {
			return nil, fmt.Errorf("layout sync: scan: %w", err)
		}
		t, parseErr := time.Parse(time.RFC3339, capturedAt)
		if parseErr != nil {
			return nil, fmt.Errorf("layout sync: parse captured_at %q: %w", capturedAt, parseErr)
		}
		r.capturedAt = t
		if lastRetry.Valid {
			lt, parseErr := time.Parse(time.RFC3339, lastRetry.String)
			if parseErr != nil {
				return nil, fmt.Errorf("layout sync: parse last_retry %q: %w", lastRetry.String, parseErr)
			}
			r.lastRetry = &lt
			backoff := backoffFor(r.retryCount)
			if now.Before(lt.Add(backoff)) {
				continue // not due yet
			}
		}
		due = append(due, r)
	}
	return due, rows.Err()
}

// backoffFor mirrors sync.Queue.backoffFor (capped at 1h).
func backoffFor(retryCount int) time.Duration {
	if retryCount < len(backoffSchedule) {
		return backoffSchedule[retryCount]
	}
	return 1 * time.Hour
}

// snapshotPayload is one element of the POST body. Note: deliberately
// holds a json.RawMessage for windows so the already-marshalled column
// value passes through without re-decoding (and the trasker server can
// validate the JSON shape itself).
type snapshotPayload struct {
	CapturedAt  string          `json:"captured_at"`
	WindowsHash string          `json:"windows_hash"`
	Windows     json.RawMessage `json:"windows"`
}

type batchPayload struct {
	ClientDeviceID string            `json:"client_device_id"`
	Snapshots      []snapshotPayload `json:"snapshots"`
}

// postBatch POSTs the batch to /api/v1/layout-snapshots with bearer auth.
// Returns ErrKeyExpired on 401 and ErrKeyRevoked on 403 (Pattern S-6).
func (s *Syncer) postBatch(ctx context.Context, due []pendingRow) error {
	payload := batchPayload{
		ClientDeviceID: s.cfg.DeviceID,
		Snapshots:      make([]snapshotPayload, 0, len(due)),
	}
	for _, r := range due {
		payload.Snapshots = append(payload.Snapshots, snapshotPayload{
			CapturedAt:  r.capturedAt.UTC().Format(time.RFC3339),
			WindowsHash: r.hash,
			Windows:     json.RawMessage(r.windowsJSON),
		})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("layout sync: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.cfg.ServerURL+"/api/v1/layout-snapshots", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("layout sync: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("layout sync: request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return nil
	case http.StatusUnauthorized:
		return clientsync.ErrKeyExpired
	case http.StatusForbidden:
		return clientsync.ErrKeyRevoked
	default:
		return fmt.Errorf("layout sync: server returned %d: %s",
			resp.StatusCode, string(respBody))
	}
}
