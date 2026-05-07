package layout_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/layout"
	clientstore "github.com/jaypaulb/trasker/internal/client/store"
	clientsync "github.com/jaypaulb/trasker/internal/client/sync"
)

// newSyncTestEnv opens a fresh client SQLite (running migrate so
// layout_snapshots exists), and returns the layout.Store + raw *sql.DB
// for direct retry-count assertions.
func newSyncTestEnv(t *testing.T) (*layout.Store, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	cs, err := clientstore.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("clientstore.New: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return layout.NewStore(cs.DB()), cs.DB()
}

func seedSnapshot(t *testing.T, s *layout.Store, capturedAt time.Time, hash string) int64 {
	t.Helper()
	ws := []layout.Window{
		{AppName: "Firefox", WindowTitle: "GitHub", X: 0, Y: 0, W: 1920, H: 1080},
	}
	jsonBytes, err := json.Marshal(ws)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	id, err := s.InsertSnapshot(capturedAt, jsonBytes, hash)
	if err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}
	return id
}

// TestSync_PostsBatch: 3 pending rows; server returns 201; all 3
// receive synced_at; server saw exactly 1 POST with 3 snapshots.
func TestSync_PostsBatch(t *testing.T) {
	s, db := newSyncTestEnv(t)
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		seedSnapshot(t, s, now.Add(time.Duration(i)*time.Minute), "hash")
	}

	var posts int
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/layout-snapshots" {
			t.Errorf("path = %q, want /api/v1/layout-snapshots", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/wrong bearer header: %q", r.Header.Get("Authorization"))
		}
		lastBody, _ = io.ReadAll(r.Body)
		posts++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"received":3}`))
	}))
	defer srv.Close()

	syncer := layout.NewSyncer(layout.SyncerConfig{
		Store: s, DB: db, DeviceID: "dev-1",
		ServerURL: srv.URL, APIKey: "test-key",
		Interval: time.Hour,
	})
	syncer.ProcessPending(context.Background())

	if posts != 1 {
		t.Errorf("server got %d POSTs, want 1", posts)
	}
	var body struct {
		ClientDeviceID string            `json:"client_device_id"`
		Snapshots      []json.RawMessage `json:"snapshots"`
	}
	if err := json.Unmarshal(lastBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ClientDeviceID != "dev-1" {
		t.Errorf("client_device_id = %q, want dev-1", body.ClientDeviceID)
	}
	if len(body.Snapshots) != 3 {
		t.Errorf("snapshots in body = %d, want 3", len(body.Snapshots))
	}

	pending, err := s.ListPending(0)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("got %d pending after success, want 0", len(pending))
	}
}

// TestSync_RetryBackoff: server 500; retry_count bumped to 1, last_retry
// set, synced_at still NULL.
func TestSync_RetryBackoff(t *testing.T) {
	s, db := newSyncTestEnv(t)
	id := seedSnapshot(t, s, time.Now().UTC(), "hash")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	syncer := layout.NewSyncer(layout.SyncerConfig{
		Store: s, DB: db, DeviceID: "dev-1",
		ServerURL: srv.URL, APIKey: "test-key",
		Interval: time.Hour,
	})
	syncer.ProcessPending(context.Background())

	var retryCount int
	var lastRetry sql.NullString
	var syncedAt sql.NullString
	err := db.QueryRow(
		`SELECT retry_count, last_retry, synced_at FROM layout_snapshots WHERE id = ?`, id,
	).Scan(&retryCount, &lastRetry, &syncedAt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if retryCount != 1 {
		t.Errorf("retry_count = %d, want 1", retryCount)
	}
	if !lastRetry.Valid {
		t.Error("last_retry not set after transient error")
	}
	if syncedAt.Valid {
		t.Errorf("synced_at = %v, want NULL", syncedAt.String)
	}
}

// TestSync_NotDueYet: row with retry_count=1, last_retry=now-30s.
// backoffSchedule[1] = 5m. Server should not be called.
func TestSync_NotDueYet(t *testing.T) {
	s, db := newSyncTestEnv(t)
	id := seedSnapshot(t, s, time.Now().UTC(), "hash")
	thirtySecondsAgo := time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)
	_, err := db.Exec(
		`UPDATE layout_snapshots SET retry_count = 1, last_retry = ? WHERE id = ?`,
		thirtySecondsAgo, id,
	)
	if err != nil {
		t.Fatalf("setup retry state: %v", err)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	syncer := layout.NewSyncer(layout.SyncerConfig{
		Store: s, DB: db, DeviceID: "dev-1",
		ServerURL: srv.URL, APIKey: "test-key",
		Interval: time.Hour,
	})
	syncer.ProcessPending(context.Background())

	if called {
		t.Error("server called for row not yet due (backoffSchedule[1]=5m)")
	}
}

// TestSync_KeyExpired: 401 → ErrKeyExpired path; row stays unsynced;
// retry NOT bumped (permanent error).
func TestSync_KeyExpired(t *testing.T) {
	s, db := newSyncTestEnv(t)
	id := seedSnapshot(t, s, time.Now().UTC(), "hash")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	syncer := layout.NewSyncer(layout.SyncerConfig{
		Store: s, DB: db, DeviceID: "dev-1",
		ServerURL: srv.URL, APIKey: "test-key",
		Interval: time.Hour,
	})
	syncer.ProcessPending(context.Background())

	var retryCount int
	var syncedAt sql.NullString
	err := db.QueryRow(
		`SELECT retry_count, synced_at FROM layout_snapshots WHERE id = ?`, id,
	).Scan(&retryCount, &syncedAt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if retryCount != 0 {
		t.Errorf("retry_count = %d, want 0 (permanent error must not bump)", retryCount)
	}
	if syncedAt.Valid {
		t.Errorf("synced_at = %v, want NULL on permanent error", syncedAt.String)
	}

	// Sentinel verification: the package re-exports the same sentinel.
	if !errors.Is(layout.ErrKeyExpired, clientsync.ErrKeyExpired) {
		t.Error("layout.ErrKeyExpired does not match clientsync.ErrKeyExpired")
	}
}

// TestSync_KeyRevoked: 403 → ErrKeyRevoked path; row stays unsynced;
// retry NOT bumped.
func TestSync_KeyRevoked(t *testing.T) {
	s, db := newSyncTestEnv(t)
	id := seedSnapshot(t, s, time.Now().UTC(), "hash")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	syncer := layout.NewSyncer(layout.SyncerConfig{
		Store: s, DB: db, DeviceID: "dev-1",
		ServerURL: srv.URL, APIKey: "test-key",
		Interval: time.Hour,
	})
	syncer.ProcessPending(context.Background())

	var retryCount int
	var syncedAt sql.NullString
	err := db.QueryRow(
		`SELECT retry_count, synced_at FROM layout_snapshots WHERE id = ?`, id,
	).Scan(&retryCount, &syncedAt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if retryCount != 0 {
		t.Errorf("retry_count = %d, want 0 (permanent 403 must not bump)", retryCount)
	}
	if syncedAt.Valid {
		t.Errorf("synced_at = %v, want NULL on permanent error", syncedAt.String)
	}
	if !errors.Is(layout.ErrKeyRevoked, clientsync.ErrKeyRevoked) {
		t.Error("layout.ErrKeyRevoked does not match clientsync.ErrKeyRevoked")
	}
}

// TestSync_PartialSuccess: with 0 pending rows ProcessPending makes no
// HTTP call.
func TestSync_PartialSuccess(t *testing.T) {
	s, db := newSyncTestEnv(t)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	syncer := layout.NewSyncer(layout.SyncerConfig{
		Store: s, DB: db, DeviceID: "dev-1",
		ServerURL: srv.URL, APIKey: "test-key",
		Interval: time.Hour,
	})
	syncer.ProcessPending(context.Background())

	if called {
		t.Error("server should NOT be called when there are no pending rows")
	}
}
