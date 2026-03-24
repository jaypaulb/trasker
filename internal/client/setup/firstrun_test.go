// internal/client/setup/firstrun_test.go
package setup

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFirstRun_CreatesDB(t *testing.T) {
	// Use temp dir instead of real data dir
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fr := NewFirstRun(Config{
		ServerURL: "https://test.example.com",
		APIKey:    "test-key",
	}, logger)

	db, err := fr.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	defer db.Close()

	// Verify DB file exists
	dbPath := filepath.Join(tmpDir, "trasker", "trasker.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file not created")
	}

	// Verify config was inserted
	var deviceID, serverURL string
	err = db.QueryRow(`SELECT device_id, server_url FROM config WHERE id = 1`).Scan(&deviceID, &serverURL)
	if err != nil {
		t.Fatalf("query config: %v", err)
	}
	if deviceID == "" {
		t.Error("expected non-empty device_id")
	}
	if serverURL != "https://test.example.com" {
		t.Errorf("expected server URL 'https://test.example.com', got %q", serverURL)
	}
}

func TestFirstRun_IdempotentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fr := NewFirstRun(Config{ServerURL: "https://test.example.com", APIKey: "key"}, logger)

	db1, err := fr.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var deviceID1 string
	db1.QueryRow(`SELECT device_id FROM config WHERE id = 1`).Scan(&deviceID1)
	db1.Close()

	// Second run should not change device_id (INSERT OR IGNORE)
	db2, err := fr.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	var deviceID2 string
	db2.QueryRow(`SELECT device_id FROM config WHERE id = 1`).Scan(&deviceID2)

	if deviceID1 != deviceID2 {
		t.Errorf("device_id changed on second run: %q vs %q", deviceID1, deviceID2)
	}
}

func TestIsFirstRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	first, err := IsFirstRun()
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Error("expected first run = true before setup")
	}
}

func TestDataDir_Linux(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmpDir, "trasker")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}
