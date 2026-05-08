---
phase: 09-multi-device-dogfooding
plan: bugs-C
subsystem: server/migrations
tags: [server, migrations, postgres, infrastructure]
requires: [pgx/v5, testcontainers]
provides: [migrations.Apply, schema_migrations bookkeeping]
affects: [cmd/trasker-server/main.go, deploy/initdb/001_schema.sql]
tech-stack:
  added: [github.com/jaypaulb/trasker/migrations package]
  patterns: [embedded SQL via go:embed, pgx transactional DDL, testcontainers integration tests]
key-files:
  created:
    - migrations/migrate.go
    - migrations/migrate_test.go
    - .planning/phases/09-multi-device-dogfooding/09-bugs-C-SUMMARY.md
  modified:
    - cmd/trasker-server/main.go
decisions:
  - Co-located migrate.go with *.up.sql so go:embed *.up.sql works directly (Go embed forbids ../).
  - Backfill detection keyed on EXISTS public.users + empty schema_migrations, since users is the canonical first-migration table.
  - Each migration runs in its own transaction (rollback discards both DDL and the schema_migrations row).
  - schema_migrations.version is TEXT (matches filename prefix) rather than INT to keep prefix width flexibility.
metrics:
  duration: ~25 min
  completed: 2026-05-08
---

# Phase 9 Plan bugs-C: Embedded Migration Runner Summary

One-liner: Replaces hal's manual `psql -f` rollout with an in-process migration runner that applies embedded `migrations/*.up.sql` files in lexical order, tracked via a `schema_migrations` bookkeeping table, with safe takeover for databases bootstrapped via `deploy/initdb/`.

## What changed

`migrations/migrate.go` (new package `github.com/jaypaulb/trasker/migrations`):

- `Apply(ctx, pool)` — production entry point; embeds `*.up.sql` co-located with the Go file via `//go:embed *.up.sql` and applies any unapplied versions.
- `ApplyFS(ctx, pool, fs.FS)` — same behavior over an arbitrary `fs.FS`; tests use this to inject extra migrations and malformed SQL.
- `schema_migrations(version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())` is created on every run before bookkeeping happens.
- Filename grammar `^(\d+)_([a-zA-Z0-9_-]+)\.up\.sql$`. A non-conforming filename surfaces a clear error rather than silently skipping.
- Per-migration transaction: `BEGIN; <SQL>; INSERT INTO schema_migrations(version) VALUES (NNN); COMMIT;`. Failure rolls back both the DDL and the version row.
- Takeover/backfill path: when `schema_migrations` has no rows AND `public.users` already exists, the runner assumes the schema came from `deploy/initdb/001_schema.sql` and seeds `schema_migrations` with every embedded version without applying any SQL on top.

`cmd/trasker-server/main.go`:

- Calls `migrations.Apply(ctx, pool)` between `store.ConnectPool` and `store.New`, so the schema is guaranteed current before any store-level query.
- Logs each applied version at INFO, or a single "schema migrations: up to date" line at steady state.

## Tests

`migrations/migrate_test.go` (testcontainers, `postgres:16-alpine`):

- `TestApply_FreshDatabase` — empty DB → every embedded version applied in lexical order, all recorded.
- `TestApply_Idempotent` — second `Apply` returns `nil` and leaves `schema_migrations` unchanged.
- `TestApply_BackfillFromInitdb` — pre-seed via `deploy/initdb/001_schema.sql`, then `Apply` records all known versions and applies zero SQL; a follow-up `Apply` is also a no-op.
- `TestApplyFS_AppliesNewMigration` — synthetic `999_test.up.sql` injected via `fstest.MapFS`; only that version is applied on top of the steady state.
- `TestApplyFS_TransactionRollbackOnFailure` — a migration containing invalid SQL surfaces an error, leaves `schema_migrations` empty, and the partial DDL inside the failed transaction is gone.
- `TestApplyFS_RejectsBadFilename` — a non-conforming filename produces an error containing `does not match` and applies nothing.

Test result: `ok  github.com/jaypaulb/trasker/migrations  11.304s` (six tests, all passing on Docker host).

## Commits

- `b25163a` feat(09-bugs-C): add embedded migration runner package
- `ae1552c` test(09-bugs-C): cover migration runner with testcontainers
- `35d2488` feat(09-bugs-C): run migrations on trasker-server startup

## Deviations from Plan

None of the auto-fix rules fired. The plan suggested `internal/server/store/migrations/migrate.go` as an alternative location; I chose `migrations/migrate.go` because Go's `//go:embed` forbids `..` patterns, so the cleanest layout is to co-locate the Go file with the SQL files. This matches the plan's recommended option ("put `migrate.go` AT `migrations/migrate.go`").

The plan also suggested adding a `migrate.down.go` or down-migration support; not requested explicitly and out of scope. The down `.sql` files remain on disk untouched.

## Out-of-Scope Discovery

`internal/client/webui/embed.go` fails `go build ./...` with `pattern all:static: no matching files found`. Reproduced on the parent commit before any changes — pre-existing on `488ff13`. Not fixed (scope boundary). Worth tracking elsewhere if the dashboard build pipeline doesn't already.

## Operational Notes for hal Rollout

When the next `trasker-server` container starts on hal:

1. The `users` table already exists (created via `deploy/initdb/001_schema.sql`).
2. `schema_migrations` does not exist yet.
3. The runner's first call creates `schema_migrations`, observes `users` present + bookkeeping empty, and backfills versions `001`, `002`, `003`, `004` without re-applying any SQL.
4. Subsequent restarts log `schema migrations: up to date`.
5. Future migration `005_*.up.sql` will be picked up automatically on the next deploy.

No manual psql step is required.

## Self-Check: PASSED

- migrations/migrate.go: FOUND
- migrations/migrate_test.go: FOUND
- cmd/trasker-server/main.go modification: FOUND (verified via `git diff 488ff13..HEAD -- cmd/trasker-server/main.go`)
- Commit b25163a: FOUND
- Commit ae1552c: FOUND
- Commit 35d2488: FOUND
- All migration tests pass: VERIFIED (`go test -count=1 ./migrations/` -> ok 11.304s)
- `go build ./cmd/trasker-server`: VERIFIED
- `go vet ./cmd/trasker-server ./migrations ./internal/server/store`: VERIFIED clean
