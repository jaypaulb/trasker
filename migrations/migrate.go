// Package migrations provides an in-process database migration runner for
// trasker-server. On startup the server calls Apply, which scans the embedded
// *.up.sql files in lexical order, tracks which versions have run in the
// schema_migrations table, and applies any unapplied migrations inside a
// transaction.
//
// Backfill: if the database already contains tables (users exists) but
// schema_migrations is empty, the runner assumes the schema was applied via
// the deploy/initdb/*.sql bootstrap (which only runs on a fresh data volume)
// and seeds schema_migrations with every embedded version without re-applying
// any SQL. This lets the migration runner take over an existing deployment
// safely.
package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// embeddedFS holds the *.up.sql files baked into the binary at compile time.
// Co-located with this file so //go:embed *.up.sql works without parent
// references (Go's embed forbids ../).
//
//go:embed *.up.sql
var embeddedFS embed.FS

// Migration is a single up-migration discovered on disk or in the embed.
type Migration struct {
	Version  string // zero-padded numeric prefix, e.g. "001"
	Name     string // slug after the prefix, e.g. "init"
	Filename string // original filename, e.g. "001_init.up.sql"
	SQL      string // raw SQL body
}

// migrationFilenameRE matches "NNN_<slug>.up.sql" and captures version + slug.
var migrationFilenameRE = regexp.MustCompile(`^(\d+)_([a-zA-Z0-9_-]+)\.up\.sql$`)

// schemaMigrationsDDL is the bookkeeping table. Idempotent — created on every
// run before any migration is applied so the runner can record itself.
const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// Apply discovers migrations from the embedded filesystem and applies any
// that are not yet recorded in schema_migrations. Returns the list of
// versions that were applied (empty if none).
//
// Behavior:
//   - Creates schema_migrations if missing.
//   - If schema_migrations is empty AND a known pre-runner table (users)
//     already exists, backfills schema_migrations with every embedded
//     version and applies no SQL. This is the takeover path for databases
//     bootstrapped via deploy/initdb/*.sql.
//   - Otherwise, applies each unapplied migration inside a transaction.
//     A failure in any migration rolls back that migration's transaction
//     (including its schema_migrations insert) and aborts the run.
func Apply(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	return ApplyFS(ctx, pool, embeddedFS)
}

// ApplyFS is the same as Apply but reads migrations from an arbitrary
// fs.FS. Exposed for tests so they can inject extra migrations or
// malformed SQL without touching the embed.
func ApplyFS(ctx context.Context, pool *pgxpool.Pool, source fs.FS) ([]string, error) {
	migs, err := discover(source)
	if err != nil {
		return nil, fmt.Errorf("discovering migrations: %w", err)
	}

	if _, err := pool.Exec(ctx, schemaMigrationsDDL); err != nil {
		return nil, fmt.Errorf("creating schema_migrations: %w", err)
	}

	applied, err := loadAppliedVersions(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("loading applied versions: %w", err)
	}

	// Backfill path: schema_migrations is empty but the canonical first-table
	// already exists. Seed schema_migrations with every embedded version and
	// return without applying any SQL.
	if len(applied) == 0 {
		legacy, err := hasLegacySchema(ctx, pool)
		if err != nil {
			return nil, fmt.Errorf("checking for legacy schema: %w", err)
		}
		if legacy {
			versions := make([]string, 0, len(migs))
			for _, m := range migs {
				versions = append(versions, m.Version)
			}
			if err := backfillVersions(ctx, pool, versions); err != nil {
				return nil, fmt.Errorf("backfilling schema_migrations: %w", err)
			}
			return nil, nil
		}
	}

	var newlyApplied []string
	for _, m := range migs {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if err := applyOne(ctx, pool, m); err != nil {
			return newlyApplied, fmt.Errorf("applying migration %s: %w", m.Filename, err)
		}
		newlyApplied = append(newlyApplied, m.Version)
	}
	return newlyApplied, nil
}

// discover lists *.up.sql files in source, parses each filename, and sorts
// them ascending by version string. Sort by zero-padded version is correct
// because the prefixes have a consistent width per project; if a future
// migration uses a wider prefix (e.g. 1000_) lexical and numeric order
// agree as long as no zero-padded prefix is shorter than another.
func discover(source fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, fmt.Errorf("reading migrations dir: %w", err)
	}

	var migs []Migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		match := migrationFilenameRE.FindStringSubmatch(name)
		if match == nil {
			return nil, fmt.Errorf("migration filename %q does not match NNN_<slug>.up.sql", name)
		}
		body, err := fs.ReadFile(source, name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		migs = append(migs, Migration{
			Version:  match[1],
			Name:     match[2],
			Filename: name,
			SQL:      string(body),
		})
	}

	sort.Slice(migs, func(i, j int) bool { return migs[i].Version < migs[j].Version })
	return migs, nil
}

// loadAppliedVersions reads the schema_migrations table and returns the set
// of versions already applied.
func loadAppliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]struct{}, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]struct{}{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = struct{}{}
	}
	return out, rows.Err()
}

// hasLegacySchema returns true when the canonical first-migration table
// (users) already exists. We treat that as proof that the DB was bootstrapped
// via deploy/initdb/*.sql before the migration runner existed.
func hasLegacySchema(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = 'public' AND table_name = 'users'
        )
    `).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// backfillVersions inserts every supplied version into schema_migrations in
// a single transaction. Used by the takeover path.
func backfillVersions(ctx context.Context, pool *pgxpool.Pool, versions []string) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is best-effort after commit

	for _, v := range versions {
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`,
			v,
		); err != nil {
			return fmt.Errorf("inserting backfill version %s: %w", v, err)
		}
	}
	return tx.Commit(ctx)
}

// applyOne runs a single migration plus its schema_migrations insert in one
// transaction. If anything fails, the transaction rolls back so neither the
// SQL changes nor the version row land.
func applyOne(ctx context.Context, pool *pgxpool.Pool, m Migration) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is best-effort after commit

	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("executing SQL: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`,
		m.Version,
	); err != nil {
		return fmt.Errorf("recording version: %w", err)
	}
	return tx.Commit(ctx)
}
