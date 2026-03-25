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
	"github.com/jaypaulb/trasker/internal/client/store"
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

	// 2. Open SQLite database and initialize schema via store.New
	dbPath := filepath.Join(dataDir, "trasker.db")
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("firstrun: init store: %w", err)
	}
	// Extract the underlying *sql.DB; caller takes ownership.
	db := s.DB()
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
