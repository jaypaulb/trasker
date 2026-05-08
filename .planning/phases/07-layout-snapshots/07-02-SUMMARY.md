---
phase: 07-layout-snapshots
plan: 02
subsystem: client
tags: [phase-7, client, layout-snapshots, x11, sqlite]
requires:
  - 07-01 (REQUIREMENTS + STATE for Phase 7)
provides:
  - "internal/client/layout package: types, X11 enumerator, capturer, sqlite store, sync goroutine"
  - "client SQLite layout_snapshots table"
affects:
  - "internal/client/store/store.go (schema appended)"
tech-stack:
  added: []
  patterns:
    - "cgo X11 enumeration with EWMH _NET_CLIENT_LIST_STACKING + XQueryTree fallback (mirrors tracker_linux_x11.go)"
    - "Change-detect via SHA-256 over canonical sorted-tuple form"
    - "Per-platform build tags (linux+cgo, linux+!cgo, darwin, windows)"
    - "Sync goroutine duplicating sync.Queue retry/backoff [1m, 5m, 15m, 1h] (Three Examples)"
key-files:
  created:
    - internal/client/layout/layout.go
    - internal/client/layout/layout_test.go
    - internal/client/layout/enum_linux_x11.go
    - internal/client/layout/enum_linux_x11_integration_test.go
    - internal/client/layout/enum_linux_nocgo.go
    - internal/client/layout/enum_darwin.go
    - internal/client/layout/enum_windows.go
    - internal/client/layout/capturer.go
    - internal/client/layout/capturer_test.go
    - internal/client/layout/store.go
    - internal/client/layout/store_test.go
    - internal/client/layout/sync.go
    - internal/client/layout/sync_test.go
    - .planning/phases/07-layout-snapshots/deferred-items.md
  modified:
    - internal/client/store/store.go
decisions:
  - "Re-export ErrKeyExpired/ErrKeyRevoked from internal/client/sync rather than redeclaring; sentinel set is project-wide, not entity-specific."
  - "Sync state lives on the layout_snapshots row (synced_at/retry_count/last_retry columns) rather than a separate outbox table; aligns with RESEARCH.md Pattern 4 option A and avoids JOIN-heavy reads."
  - "json.RawMessage for the windows POST field so already-marshalled column values pass through without re-decoding (and the server validates the JSON shape itself)."
  - "Capturer's tick logs hash + window count only (Debug). Raw window titles are NEVER logged (mitigates T-7-02-05)."
metrics:
  duration: "~28 minutes"
  completed: "2026-05-07"
  tests_added: 23 unit + 1 integration (build-tag gated)
  files_created: 14
  files_modified: 1
---

# Phase 7 Plan 02: Client layout package (capture + store + sync) Summary

Built the entire client-side layout snapshot pipeline — types + hash, cgo X11 enumerator with non-cgo / macOS / Windows stubs, lock-gated change-detect capturer, local SQLite store with prune, and a parallel sync goroutine that mirrors `internal/client/sync/queue.go` verbatim — behind no feature flag (07-05-PLAN wires the daemon main).

## Goals Achieved

- `Window` struct frozen at six fields (`AppName`, `WindowTitle`, `X`, `Y`, `W`, `H`); privacy invariant D-15/D-16 enforced by the package's grep gate (returns 0 for all forbidden tokens).
- `HashWindows([]Window)` deterministic SHA-256 over canonically sorted tuples with `\x00` separators and `\x01` row terminators; defensive-copies the input.
- X11 enumerator (`enum_linux_x11.go`, cgo) uses `_NET_CLIENT_LIST_STACKING` (with `XQueryTree` fallback) for window IDs, and `XGetWindowAttributes` + `XTranslateCoordinates` per window for root-absolute geometry (07-RESEARCH Pitfall 1 mitigation). Caps at 256 windows.
- Build tag matrix: `//go:build linux && cgo` (real impl), `//go:build linux && !cgo`, `//go:build darwin`, `//go:build windows` (all stubs returning `ErrUnsupported`). `NewPlatformEnumerator()` is the symbol the daemon main wires.
- Capturer: 60s ticker (configurable for tests), `presence.StateChange` subscriber maintaining an `atomic.Bool`, change-detect via `lastHash`, fail-soft on enum error (D-07/D-08/D-09). `nil` lockEvents = always-unlocked (RESEARCH Pattern 3).
- Local store: `InsertSnapshot`, `ListPending`, `MarkSynced`, `BumpRetry`, `ListInRange`, `Prune` accessors using the focus-events scanner pattern verbatim. RFC3339 UTC timestamps throughout (Pattern S-5).
- Schema delta appended to `internal/client/store/store.go` migrate() literal:
  - `CREATE TABLE IF NOT EXISTS layout_snapshots (id, captured_at, windows TEXT, windows_hash, synced_at, retry_count, last_retry, created_at)`
  - `CREATE INDEX IF NOT EXISTS idx_layout_snapshots_captured ON layout_snapshots(captured_at)`
  - `CREATE INDEX IF NOT EXISTS idx_layout_snapshots_pending ON layout_snapshots(synced_at) WHERE synced_at IS NULL`
- Sync goroutine: mirrors `sync.Queue` exactly — `Start/Stop` under `sync.Mutex.running` flag, immediate `ProcessPending` then 5-minute `time.Ticker`, backoff `[1m, 5m, 15m, 1h]` capped at 1h, permanent (`401`/`403`) vs transient (`5xx`/network) error split. Re-exports the existing `ErrKeyExpired/ErrKeyRevoked` sentinels.
- POST `/api/v1/layout-snapshots` with `Bearer` auth; body `{client_device_id, snapshots:[{captured_at, windows_hash, windows}]}`. Never includes the API key in the body, never logs raw titles (T-7-02-05).

## Tasks Completed

| Task | Name | Commit | Key files |
| ---- | ---- | ------ | --------- |
| 1 | Types, hash, enumerator interface, platform stubs | `12d8318` | layout.go, layout_test.go, enum_*.go (4), enum_linux_x11_integration_test.go |
| 2 | Capturer + sqlite store + schema migration | `8946dad` | capturer.go, capturer_test.go, store.go, store_test.go, internal/client/store/store.go |
| 3 | Sync goroutine (mirrors sync.Queue) | `fd1e262` | sync.go, sync_test.go, deferred-items.md |

## Test Counts

- `layout_test.go`: 7 hash unit tests
- `store_test.go`: 5 store unit tests
- `capturer_test.go`: 5 capturer unit tests (using `fakeEnumerator` + a mock screenlock channel)
- `sync_test.go`: 6 sync unit tests (using `httptest.NewServer` + raw-SQL retry-state assertions)
- `enum_linux_x11_integration_test.go`: 1 build-tag-gated integration test (`//go:build linux && cgo && x11_integration`); not run in CI

Total automated tests: **23 unit + 1 integration**.

Whole-package suite: `go test ./internal/client/layout/... -count=1` runs in ~1.6s under both cgo and `CGO_ENABLED=0`. Regression: `go test ./internal/client/store/...` still green (schema append did not break existing tests).

## Schema Delta Added to internal/client/store/store.go

```sql
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
```

`grep -c 'CREATE TABLE IF NOT EXISTS layout_snapshots' internal/client/store/store.go` returns `1`.

## Privacy Grep Gate

- `grep -c -E '\b(PID|Pid|cmdline|Cmdline|Screenshot|FocusedFlag|ZOrder|exec\.Command)\b' internal/client/layout/*.go` returns `0` across every file.
- `Window` struct contains exactly six fields; `grep -E '^\s+(AppName|WindowTitle|X|Y|W|H)\s+' internal/client/layout/layout.go` returns 6 lines.
- Sync logs only `count` and `snapshot_id`; never the windows array. Capturer logs `id`, `hash`, `window_count`; never raw titles.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] Privacy grep gate failed on doc comments listing forbidden tokens**
- **Found during:** Task 1 verification.
- **Issue:** `grep -c -E '\b(PID|Pid|cmdline|...)\b' internal/client/layout/layout.go` returned `2` because the package + struct doc comments listed exactly those tokens to explain what the privacy invariant rejects. The acceptance criterion required `0`.
- **Fix:** Reworded the doc comments to convey the same intent in prose ("Process identifiers, command-line strings, executable paths, screen-capture bytes, focus state, stacking order, and monitor indices are all out of scope") without using the literal forbidden tokens.
- **Files modified:** `internal/client/layout/layout.go`.
- **Commit:** `12d8318`.

### Out-of-scope discoveries

**1. Pre-existing `internal/client/webui/embed.go` `//go:embed all:static` build failure.**
- Verified by checking out `embed.go` from the base commit (`7154943`) in isolation; same failure. Not introduced by this plan.
- The plan acceptance criterion `go build ./...` is therefore impossible to satisfy from base; the layout package itself (`go build ./internal/client/layout/...`) builds clean under both cgo and `CGO_ENABLED=0`.
- Logged to `.planning/phases/07-layout-snapshots/deferred-items.md` for a future plan that owns the webui package (likely the SvelteKit static-build wiring).

## TDD Gate Compliance

The plan declared `tdd="true"` per task. Each task created its tests before its production code (RED → GREEN); however, all three commits in this plan are conventional-commit `feat(...)` rather than the strict `test(...)` then `feat(...)` two-commit cadence. The test files and production files for each task were therefore committed atomically in a single `feat(...)` commit — RED was verified locally during development (e.g. `go test ./internal/client/layout` failed on missing impl before each `Write` of the production file) but not split across two commits.

This is a process deviation worth noting; future plans following the same pattern should consider splitting per the strict TDD cadence so `git log --oneline` shows the RED commit explicitly.

## Verification Results

- `go test ./internal/client/layout/... -count=1` — **PASS** (~1.6s, 23 tests green)
- `CGO_ENABLED=0 go test ./internal/client/layout/... -count=1` — **PASS** (~1.6s)
- `go test ./internal/client/store/... -count=1` — **PASS** (regression check on schema append)
- `go vet ./internal/client/layout/...` — **CLEAN**
- `go vet -tags x11_integration ./internal/client/layout/...` — **CLEAN**
- `go build ./internal/client/layout/...` — **PASS** (cgo)
- `CGO_ENABLED=0 go build ./internal/client/layout/...` — **PASS** (uses nocgo enum stub)
- `go build ./...` — **FAIL** at unrelated `internal/client/webui/embed.go` (pre-existing; deferred)

## Self-Check: PASSED

All 14 created files exist on disk:

- `internal/client/layout/layout.go` — FOUND
- `internal/client/layout/layout_test.go` — FOUND
- `internal/client/layout/enum_linux_x11.go` — FOUND
- `internal/client/layout/enum_linux_x11_integration_test.go` — FOUND
- `internal/client/layout/enum_linux_nocgo.go` — FOUND
- `internal/client/layout/enum_darwin.go` — FOUND
- `internal/client/layout/enum_windows.go` — FOUND
- `internal/client/layout/capturer.go` — FOUND
- `internal/client/layout/capturer_test.go` — FOUND
- `internal/client/layout/store.go` — FOUND
- `internal/client/layout/store_test.go` — FOUND
- `internal/client/layout/sync.go` — FOUND
- `internal/client/layout/sync_test.go` — FOUND
- `.planning/phases/07-layout-snapshots/deferred-items.md` — FOUND

Modified file: `internal/client/store/store.go` — FOUND (with appended schema block, grep returns 1).

All three commits exist in `git log --oneline`: `12d8318`, `8946dad`, `fd1e262`.
