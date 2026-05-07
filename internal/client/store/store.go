// internal/client/store/store.go
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection for the Trasker client.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at the given path and runs
// schema migrations. The caller must call Close when done.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Enable foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// DB returns the underlying *sql.DB for direct queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS focus_events (
		id           INTEGER PRIMARY KEY,
		app_name     TEXT NOT NULL,
		window_title TEXT NOT NULL,
		started_at   TEXT NOT NULL,
		ended_at     TEXT,
		duration_s   INTEGER,
		is_idle      INTEGER NOT NULL DEFAULT 0,
		created_at   TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS tags (
		id         INTEGER PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE,
		color      TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS event_tags (
		event_id     INTEGER NOT NULL REFERENCES focus_events(id),
		tag_id       INTEGER NOT NULL REFERENCES tags(id),
		source       TEXT NOT NULL,
		cascade_from INTEGER,
		UNIQUE(event_id, tag_id)
	);

	CREATE TABLE IF NOT EXISTS notes (
		id              INTEGER PRIMARY KEY,
		anchor_event    INTEGER NOT NULL REFERENCES focus_events(id),
		text            TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		cascade_applied INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS tag_rules (
		id            INTEGER PRIMARY KEY,
		tag_id        INTEGER NOT NULL REFERENCES tags(id),
		app_pattern   TEXT NOT NULL,
		title_pattern TEXT,
		priority      INTEGER NOT NULL DEFAULT 0,
		suggested     INTEGER NOT NULL DEFAULT 0,
		hit_count     INTEGER NOT NULL DEFAULT 0,
		created_at    TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS submissions (
		id           INTEGER PRIMARY KEY,
		server_id    TEXT,
		submitted_at TEXT NOT NULL,
		status       TEXT NOT NULL,
		retry_count  INTEGER NOT NULL DEFAULT 0,
		last_retry   TEXT
	);

	CREATE TABLE IF NOT EXISTS submission_events (
		submission_id INTEGER NOT NULL REFERENCES submissions(id),
		event_id      INTEGER NOT NULL REFERENCES focus_events(id),
		UNIQUE(submission_id, event_id)
	);

	CREATE TABLE IF NOT EXISTS pomodoro_sessions (
		id         INTEGER PRIMARY KEY,
		started_at TEXT NOT NULL,
		ended_at   TEXT,
		work_mins  INTEGER NOT NULL DEFAULT 25,
		break_mins INTEGER NOT NULL DEFAULT 5,
		status     TEXT NOT NULL,
		tag_id     INTEGER REFERENCES tags(id),
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS config (
		id                 INTEGER PRIMARY KEY CHECK (id = 1),
		device_id          TEXT NOT NULL,
		server_url         TEXT NOT NULL,
		api_key            TEXT NOT NULL,
		tracking_on        INTEGER NOT NULL DEFAULT 1,
		autostart          INTEGER NOT NULL DEFAULT 0,
		presence_intervals TEXT NOT NULL DEFAULT '[30,45,60,90,120]',
		pomodoro_defaults  TEXT NOT NULL DEFAULT '{"work":25,"break":5}'
	);

	CREATE TABLE IF NOT EXISTS layout_snapshots (
		id           INTEGER PRIMARY KEY,
		captured_at  TEXT NOT NULL,
		windows      TEXT NOT NULL,
		windows_hash TEXT NOT NULL,
		synced_at    TEXT,
		retry_count  INTEGER NOT NULL DEFAULT 0,
		last_retry   TEXT,
		created_at   TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_layout_snapshots_captured ON layout_snapshots(captured_at);
	CREATE INDEX IF NOT EXISTS idx_layout_snapshots_pending  ON layout_snapshots(synced_at) WHERE synced_at IS NULL;
	`

	_, err := s.db.Exec(schema)
	return err
}
