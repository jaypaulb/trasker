// internal/client/webui/api_test.go
package webui

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// stubStatusProvider is a fixed-value StatusProvider for /api/status tests.
type stubStatusProvider struct {
	presence    string
	screenLock  string
	lastLayout  time.Time
	lastSync    time.Time
}

func (s *stubStatusProvider) PresenceState() string         { return s.presence }
func (s *stubStatusProvider) ScreenLockState() string       { return s.screenLock }
func (s *stubStatusProvider) LastLayoutSnapshot() time.Time { return s.lastLayout }
func (s *stubStatusProvider) LastServerSync() time.Time     { return s.lastSync }

func setupAPIDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE focus_events (id INTEGER PRIMARY KEY, app_name TEXT NOT NULL,
			window_title TEXT NOT NULL, started_at TEXT NOT NULL, ended_at TEXT,
			duration_s INTEGER, is_idle INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE,
			color TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE event_tags (event_id INTEGER, tag_id INTEGER, source TEXT,
			cascade_from INTEGER, UNIQUE(event_id, tag_id))`,
		`CREATE TABLE notes (id INTEGER PRIMARY KEY, anchor_event INTEGER, text TEXT,
			created_at TEXT, cascade_applied INTEGER DEFAULT 0)`,
		`CREATE TABLE submissions (id INTEGER PRIMARY KEY, server_id TEXT,
			submitted_at TEXT NOT NULL, status TEXT NOT NULL,
			retry_count INTEGER NOT NULL DEFAULT 0, last_retry TEXT)`,
		`CREATE TABLE submission_events (submission_id INTEGER, event_id INTEGER,
			UNIQUE(submission_id, event_id))`,
		`CREATE TABLE pomodoro_sessions (id INTEGER PRIMARY KEY, started_at TEXT NOT NULL,
			ended_at TEXT, work_mins INTEGER DEFAULT 25, break_mins INTEGER DEFAULT 5,
			status TEXT NOT NULL, tag_id INTEGER, created_at TEXT NOT NULL)`,
		`CREATE TABLE config (id INTEGER PRIMARY KEY CHECK (id = 1), device_id TEXT NOT NULL,
			server_url TEXT NOT NULL, api_key TEXT NOT NULL,
			tracking_on INTEGER NOT NULL DEFAULT 1, autostart INTEGER NOT NULL DEFAULT 0,
			presence_intervals TEXT NOT NULL DEFAULT '[30,45,60,90,120]',
			pomodoro_defaults TEXT NOT NULL DEFAULT '{"work":25,"break":5}')`,
		// Seed
		`INSERT INTO config (id, device_id, server_url, api_key) VALUES (1, 'test-dev', 'https://test.example.com', 'key-123')`,
		`INSERT INTO tags (id, name, color, created_at) VALUES (1, 'Dev', '#00ff00', '2026-01-01T00:00:00Z')`,
		`INSERT INTO focus_events (id, app_name, window_title, started_at, ended_at, duration_s, created_at)
		 VALUES (1, 'terminal', 'claude', '2026-03-23T09:00:00Z', '2026-03-23T09:30:00Z', 1800, '2026-03-23T09:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return db
}

func startTestServer(t *testing.T, db *sql.DB) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := NewServer(db, 0, logger)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Stop(context.Background()) })
	return srv
}

func TestAPI_GetEvents(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	resp, err := http.Get(srv.URL() + "/api/events?date=2026-03-23")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var events []map[string]any
	json.NewDecoder(resp.Body).Decode(&events)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestAPI_GetTags(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	resp, err := http.Get(srv.URL() + "/api/tags")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var tags []map[string]any
	json.NewDecoder(resp.Body).Decode(&tags)
	if len(tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(tags))
	}
}

func TestAPI_CreateTag(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"name":"Testing","color":"#ff0000"}`)
	resp, err := http.Post(srv.URL()+"/api/tags", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["name"] != "Testing" {
		t.Errorf("expected name 'Testing', got %v", result["name"])
	}
}

func TestAPI_TagEvent(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"tag_id":1}`)
	resp, err := http.Post(srv.URL()+"/api/events/1/tag", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAPI_AddNote(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"text":"Working on auth module"}`)
	resp, err := http.Post(srv.URL()+"/api/events/1/note", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAPI_GetConfig(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	resp, err := http.Get(srv.URL() + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var config map[string]any
	json.NewDecoder(resp.Body).Decode(&config)
	if config["device_id"] != "test-dev" {
		t.Errorf("expected device_id 'test-dev', got %v", config["device_id"])
	}
}

func TestAPI_UpdateConfig(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"tracking_on":0}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL()+"/api/config", body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify the change
	var trackingOn int
	db.QueryRow(`SELECT tracking_on FROM config WHERE id = 1`).Scan(&trackingOn)
	if trackingOn != 0 {
		t.Errorf("expected tracking_on=0, got %d", trackingOn)
	}
}

func TestAPI_Status_NoProvider(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	resp, err := http.Get(srv.URL() + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var st map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if pid, ok := st["pid"].(float64); !ok || int(pid) != os.Getpid() {
		t.Errorf("pid = %v, want %d", st["pid"], os.Getpid())
	}
	if port, ok := st["port"].(float64); !ok || port == 0 {
		t.Errorf("port = %v, want non-zero", st["port"])
	}
	// uptime_seconds is always present (numeric); presence/lock fields
	// are omitted when no provider is registered.
	if _, ok := st["uptime_seconds"]; !ok {
		t.Errorf("uptime_seconds missing")
	}
	if _, ok := st["presence_state"]; ok {
		t.Errorf("presence_state should be omitted when no provider, got %v", st["presence_state"])
	}
}

func TestAPI_Status_WithProvider(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	layoutTime := time.Date(2026, 5, 8, 9, 0, 0, 0, time.UTC)
	syncTime := time.Date(2026, 5, 8, 9, 5, 0, 0, time.UTC)
	srv.SetStatusProvider(&stubStatusProvider{
		presence:   "TRACKING",
		screenLock: "UNLOCKED",
		lastLayout: layoutTime,
		lastSync:   syncTime,
	})

	resp, err := http.Get(srv.URL() + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var st map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st["presence_state"] != "TRACKING" {
		t.Errorf("presence_state = %v, want TRACKING", st["presence_state"])
	}
	if st["screen_lock_state"] != "UNLOCKED" {
		t.Errorf("screen_lock_state = %v, want UNLOCKED", st["screen_lock_state"])
	}
	if st["last_layout_snapshot"] != "2026-05-08T09:00:00Z" {
		t.Errorf("last_layout_snapshot = %v", st["last_layout_snapshot"])
	}
	if st["last_server_sync"] != "2026-05-08T09:05:00Z" {
		t.Errorf("last_server_sync = %v", st["last_server_sync"])
	}
}

func TestAPI_Submit(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"event_ids":[1]}`)
	resp, err := http.Post(srv.URL()+"/api/submit", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", result["status"])
	}
}
