// internal/client/sync/queue_test.go
package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupQueueDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT, created_at TEXT)`,
		`CREATE TABLE focus_events (
			id INTEGER PRIMARY KEY, app_name TEXT NOT NULL, window_title TEXT NOT NULL,
			started_at TEXT NOT NULL, ended_at TEXT, duration_s INTEGER,
			is_idle INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE TABLE event_tags (event_id INTEGER, tag_id INTEGER, source TEXT, cascade_from INTEGER,
			UNIQUE(event_id, tag_id))`,
		`CREATE TABLE notes (id INTEGER PRIMARY KEY, anchor_event INTEGER, text TEXT, created_at TEXT,
			cascade_applied INTEGER DEFAULT 0)`,
		`CREATE TABLE submissions (
			id INTEGER PRIMARY KEY, server_id TEXT, submitted_at TEXT NOT NULL,
			status TEXT NOT NULL, retry_count INTEGER NOT NULL DEFAULT 0, last_retry TEXT)`,
		`CREATE TABLE submission_events (submission_id INTEGER, event_id INTEGER,
			UNIQUE(submission_id, event_id))`,
		// Seed data
		`INSERT INTO tags (id, name, color, created_at) VALUES (1, 'Dev', '#00ff00', '2026-01-01T00:00:00Z')`,
		`INSERT INTO focus_events (id, app_name, window_title, started_at, ended_at, duration_s, created_at)
		 VALUES (1, 'terminal', 'claude', '2026-03-23T09:00:00Z', '2026-03-23T10:00:00Z', 3600, '2026-03-23T09:00:00Z')`,
		`INSERT INTO event_tags (event_id, tag_id, source) VALUES (1, 1, 'manual')`,
		`INSERT INTO submissions (id, submitted_at, status) VALUES (1, '2026-03-23T12:00:00Z', 'pending')`,
		`INSERT INTO submission_events (submission_id, event_id) VALUES (1, 1)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt[:40], err)
		}
	}
	return db
}

func TestQueue_ProcessPending_Success(t *testing.T) {
	db := setupQueueDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(TimesheetResponse{ID: "srv-1", SubmittedAt: "2026-03-23T12:00:00Z"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	queue := NewQueue(db, client, "dev-123", logger)

	queue.ProcessPending(context.Background())

	// Check submission was confirmed
	var status string
	var serverID sql.NullString
	db.QueryRow(`SELECT status, server_id FROM submissions WHERE id = 1`).Scan(&status, &serverID)
	if status != "confirmed" {
		t.Errorf("expected 'confirmed', got %q", status)
	}
	if !serverID.Valid || serverID.String != "srv-1" {
		t.Errorf("expected server_id 'srv-1', got %v", serverID)
	}
}

func TestQueue_ProcessPending_RetryOnError(t *testing.T) {
	db := setupQueueDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	queue := NewQueue(db, client, "dev-123", logger)

	queue.ProcessPending(context.Background())

	var retryCount int
	var lastRetry sql.NullString
	db.QueryRow(`SELECT retry_count, last_retry FROM submissions WHERE id = 1`).Scan(&retryCount, &lastRetry)
	if retryCount != 1 {
		t.Errorf("expected retry_count 1, got %d", retryCount)
	}
	if !lastRetry.Valid {
		t.Error("expected last_retry to be set")
	}
}

func TestQueue_BackoffSchedule(t *testing.T) {
	queue := &Queue{}

	tests := []struct {
		retry  int
		expect time.Duration
	}{
		{0, 1 * time.Minute},
		{1, 5 * time.Minute},
		{2, 15 * time.Minute},
		{3, 1 * time.Hour},
		{4, 1 * time.Hour}, // capped
		{99, 1 * time.Hour},
	}

	for _, tt := range tests {
		got := queue.backoffFor(tt.retry)
		if got != tt.expect {
			t.Errorf("backoff(%d) = %v, want %v", tt.retry, got, tt.expect)
		}
	}
}

func TestQueue_SkipsNotDueSubmissions(t *testing.T) {
	db := setupQueueDB(t)
	// Mark as recently retried so it's not due
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec(`UPDATE submissions SET retry_count = 1, last_retry = ? WHERE id = 1`, now)

	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(TimesheetResponse{ID: "srv-1"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	queue := NewQueue(db, client, "dev-123", logger)

	queue.ProcessPending(context.Background())

	if serverCalled {
		t.Error("server should not have been called — submission not yet due")
	}
}
