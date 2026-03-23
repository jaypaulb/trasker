// internal/client/store/store_test.go
package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jaypaulb/trasker/internal/client/store"
)

func TestNewStore_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}
}

func TestNewStore_RunsMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer s.Close()

	// Verify all expected tables exist
	tables := []string{
		"focus_events",
		"tags",
		"event_tags",
		"notes",
		"tag_rules",
		"submissions",
		"submission_events",
		"pomodoro_sessions",
		"config",
	}

	for _, table := range tables {
		var name string
		err := s.DB().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestNewStore_IdempotentMigrations(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s1, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("first New() error: %v", err)
	}
	s1.Close()

	// Opening again should not fail (migrations are idempotent)
	s2, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("second New() error: %v", err)
	}
	s2.Close()
}

func TestStore_Close(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// Verify the connection is closed by attempting a query
	_, err = s.DB().Query("SELECT 1")
	if err == nil {
		t.Fatal("expected error after Close(), got nil")
	}
}
