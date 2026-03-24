// internal/client/webui/server_test.go
package webui

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func setupWebDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestServer_StartAndStop(t *testing.T) {
	db := setupWebDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv := NewServer(db, 0, logger) // port 0 = random
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Stop(context.Background())

	// Server should be reachable
	resp, err := http.Get(srv.URL() + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	// Expect 200 or 404 (no static files embedded in test)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		t.Errorf("unexpected status: %d", resp.StatusCode)
	}
}
