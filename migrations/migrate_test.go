package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jaypaulb/trasker/migrations"
)

// startPostgres spins up a postgres:16-alpine container, returns a pool
// pointed at an empty database, and registers cleanup with t.
func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("trasker_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}

// queryAppliedVersions returns every row in schema_migrations sorted ascending.
func queryAppliedVersions(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT version FROM schema_migrations ORDER BY version ASC`)
	require.NoError(t, err)
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))
		out = append(out, v)
	}
	require.NoError(t, rows.Err())
	return out
}

// tableExists reports whether public.<name> exists.
func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(), `
        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = 'public' AND table_name = $1
        )
    `, name).Scan(&exists)
	require.NoError(t, err)
	return exists
}

// embeddedVersions returns every version known to the embedded fs (kept in
// sync with the runner). Used by tests that compare backfill output to
// what is shipped in the binary.
func embeddedVersions(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "migrations"))
	require.NoError(t, err)

	var out []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".sql" || len(name) < 8 {
			continue
		}
		if name[len(name)-7:] != ".up.sql" {
			continue
		}
		// "001_init.up.sql" -> "001"
		out = append(out, name[:3])
	}
	return out
}

// repoRoot walks up from the test working dir until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found above %s", wd)
		dir = parent
	}
}

// TestApply_FreshDatabase: starting from an empty postgres, Apply runs every
// embedded migration and records each version.
func TestApply_FreshDatabase(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	applied, err := migrations.Apply(ctx, pool)
	require.NoError(t, err)
	require.NotEmpty(t, applied, "expected at least one migration to apply on fresh DB")

	want := embeddedVersions(t)
	require.Equal(t, want, applied,
		"every embedded version should have been applied in order")

	got := queryAppliedVersions(t, pool)
	require.Equal(t, want, got, "schema_migrations should record every version")

	// Spot-check that the schema actually landed.
	require.True(t, tableExists(t, pool, "users"))
	require.True(t, tableExists(t, pool, "layout_snapshots"))
}

// TestApply_Idempotent: a second Apply on a freshly migrated DB applies
// nothing and leaves schema_migrations untouched.
func TestApply_Idempotent(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	first, err := migrations.Apply(ctx, pool)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	second, err := migrations.Apply(ctx, pool)
	require.NoError(t, err)
	require.Empty(t, second, "second Apply must be a no-op")

	got := queryAppliedVersions(t, pool)
	require.Equal(t, embeddedVersions(t), got)
}

// TestApply_BackfillFromInitdb: pre-seed an empty DB with the canonical
// deploy/initdb/001_schema.sql, then call Apply. The runner must detect the
// legacy schema, backfill schema_migrations with every embedded version, and
// apply no SQL on top.
func TestApply_BackfillFromInitdb(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	initSQL, err := os.ReadFile(filepath.Join(repoRoot(t), "deploy", "initdb", "001_schema.sql"))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(initSQL))
	require.NoError(t, err)

	// Sanity: schema_migrations must not exist yet.
	require.False(t, tableExists(t, pool, "schema_migrations"),
		"initdb script should not create schema_migrations")

	applied, err := migrations.Apply(ctx, pool)
	require.NoError(t, err)
	require.Empty(t, applied,
		"backfill path must apply zero SQL migrations")

	require.True(t, tableExists(t, pool, "schema_migrations"))
	require.Equal(t, embeddedVersions(t), queryAppliedVersions(t, pool),
		"every embedded version should be recorded as backfilled")

	// And a follow-up Apply is a no-op.
	again, err := migrations.Apply(ctx, pool)
	require.NoError(t, err)
	require.Empty(t, again)
}

// TestApplyFS_AppliesNewMigration: after the embedded migrations are in
// place, a custom fs.FS that adds an extra 999_test.up.sql should cause
// only that new migration to be applied on top.
func TestApplyFS_AppliesNewMigration(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	// First, get to the steady state with the real embed.
	_, err := migrations.Apply(ctx, pool)
	require.NoError(t, err)

	// Build an fs.FS containing one synthetic migration.
	memFS := fstest.MapFS{
		"999_test.up.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE migrate_runner_test (id INT PRIMARY KEY);`),
		},
	}

	applied, err := migrations.ApplyFS(ctx, pool, memFS)
	require.NoError(t, err)
	require.Equal(t, []string{"999"}, applied)

	require.True(t, tableExists(t, pool, "migrate_runner_test"))

	// schema_migrations should now contain the embedded versions plus 999.
	want := append(embeddedVersions(t), "999")
	require.Equal(t, want, queryAppliedVersions(t, pool))
}

// TestApplyFS_TransactionRollbackOnFailure: a migration with broken SQL must
// roll back both the partial DDL and the schema_migrations insert, so a
// retry sees the same starting state.
func TestApplyFS_TransactionRollbackOnFailure(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	memFS := fstest.MapFS{
		"001_bad.up.sql": &fstest.MapFile{
			Data: []byte(`
                CREATE TABLE rollback_probe (id INT PRIMARY KEY);
                THIS IS NOT VALID SQL;
            `),
		},
	}

	applied, err := migrations.ApplyFS(ctx, pool, memFS)
	require.Error(t, err, "broken SQL must surface as an error")
	require.Empty(t, applied,
		"no migration should be reported as applied when its tx rolled back")

	// schema_migrations was created (that's outside the per-migration tx),
	// but it must be empty.
	require.True(t, tableExists(t, pool, "schema_migrations"))
	require.Empty(t, queryAppliedVersions(t, pool))

	// And the partial DDL inside the failed tx must have rolled back.
	require.False(t, tableExists(t, pool, "rollback_probe"),
		"DDL from the failed migration must not have committed")
}

// TestApplyFS_RejectsBadFilename: filenames that do not match
// NNN_<slug>.up.sql must surface a clear error rather than apply silently.
func TestApplyFS_RejectsBadFilename(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	memFS := fstest.MapFS{
		"not-a-migration.up.sql": &fstest.MapFile{Data: []byte(`SELECT 1;`)},
	}

	_, err := migrations.ApplyFS(ctx, pool, memFS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match")
}
