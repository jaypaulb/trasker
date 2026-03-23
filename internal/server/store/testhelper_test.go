package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testDB holds a connection pool and cleanup function for a test PostgreSQL instance.
type testDB struct {
	Pool    *pgxpool.Pool
	Cleanup func()
}

// newTestDB starts a PostgreSQL container and returns a connection pool.
// Call cleanup() when done (typically via t.Cleanup).
func newTestDB(t *testing.T) *testDB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("trasker_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		t.Fatalf("failed to create pool: %v", err)
	}

	// Run migrations
	if err := runTestMigrations(ctx, pool); err != nil {
		pool.Close()
		pgContainer.Terminate(ctx)
		t.Fatalf("failed to run migrations: %v", err)
	}

	cleanup := func() {
		pool.Close()
		pgContainer.Terminate(ctx)
	}

	t.Cleanup(cleanup)

	return &testDB{Pool: pool, Cleanup: cleanup}
}

// runTestMigrations reads and executes all .up.sql files from the migrations directory.
func runTestMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// Walk up from internal/server/store to find repo root
	migrationsDir := filepath.Join(findRepoRoot(), "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("reading migrations dir %s: %w", migrationsDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		// Only run .up.sql files
		if len(entry.Name()) < 7 || entry.Name()[len(entry.Name())-7:] != ".up.sql" {
			continue
		}

		sql, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("executing migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// findRepoRoot walks up from the current working directory looking for go.mod.
func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Fallback: assume 3 levels up from internal/server/store
			wd, _ := os.Getwd()
			return filepath.Join(wd, "..", "..", "..")
		}
		dir = parent
	}
}
