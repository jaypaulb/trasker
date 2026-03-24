// internal/client/setup/firstrun.go
package setup

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
)

// Config holds the baked-in values from build-time ldflags.
type Config struct {
	ServerURL string
	APIKey    string
}

// FirstRun handles initial setup: data dir, SQLite init, device registration.
type FirstRun struct {
	config  Config
	logger  *slog.Logger
	dataDir string
}

// NewFirstRun creates the first-run handler.
func NewFirstRun(config Config, logger *slog.Logger) *FirstRun {
	return &FirstRun{
		config: config,
		logger: logger,
	}
}

// DataDir returns the platform-appropriate data directory.
func DataDir() (string, error) {
	switch runtime.GOOS {
	case "linux":
		// XDG_DATA_HOME or ~/.local/share
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("firstrun: home dir: %w", err)
			}
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "trasker"), nil

	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("firstrun: home dir: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support", "Trasker"), nil

	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("firstrun: APPDATA not set")
		}
		return filepath.Join(appData, "Trasker"), nil

	default:
		return "", fmt.Errorf("firstrun: unsupported OS: %s", runtime.GOOS)
	}
}

// DBPath returns the path to the SQLite database file.
func DBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "trasker.db"), nil
}

// IsFirstRun checks if the data directory and DB exist.
func IsFirstRun() (bool, error) {
	dbPath, err := DBPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(dbPath)
	return os.IsNotExist(err), nil
}

// Run performs the first-run setup. Returns the database connection.
func (f *FirstRun) Run(ctx context.Context) (*sql.DB, error) {
	// 1. Create data directory
	dataDir, err := DataDir()
	if err != nil {
		return nil, err
	}
	f.dataDir = dataDir

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("firstrun: create data dir: %w", err)
	}
	f.logger.Info("data directory created", "path", dataDir)

	// 2. Open SQLite database
	dbPath := filepath.Join(dataDir, "trasker.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("firstrun: open db: %w", err)
	}

	// 3. Initialize schema
	if err := f.initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("firstrun: init schema: %w", err)
	}
	f.logger.Info("database initialized", "path", dbPath)

	// 4. Generate device ID and insert config
	deviceID := uuid.New().String()
	_, err = db.Exec(
		`INSERT OR IGNORE INTO config (id, device_id, server_url, api_key)
		 VALUES (1, ?, ?, ?)`,
		deviceID, f.config.ServerURL, f.config.APIKey,
	)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("firstrun: insert config: %w", err)
	}
	f.logger.Info("device registered locally", "device_id", deviceID)

	return db, nil
}

// initSchema creates all SQLite tables.
func (f *FirstRun) initSchema(db *sql.DB) error {
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
	`
	_, err := db.Exec(schema)
	return err
}

// OpenBrowser opens the default browser to the given URL.
func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}
