---
phase: 07-layout-snapshots
plan: 03
subsystem: server
tags:
  - phase-7
  - server
  - layout-snapshots
  - postgres
  - downsampling
dependency_graph:
  requires:
    - 07-01  # migrations/004_layout_snapshots.up.sql + initdb append
  provides:
    - server.layout-snapshots.store
    - server.layout-snapshots.api
    - server.layout-snapshots.downsampler
  affects:
    - cmd/trasker-server (new background goroutine on lifecycle ctx)
    - internal/server/api/router.go (3 new routes)
    - internal/server/store/devices.go (new GetDeviceByID accessor)
tech_stack:
  added: []   # zero new third-party deps; everything is stdlib + already-in-tree pgx/chi/uuid
  patterns:
    - pgx accessor with ON CONFLICT DO NOTHING + RETURNING for idempotent ingest
    - bucketing CTE (row_number() OVER PARTITION BY) for in-place downsampling
    - JSONB-as-passthrough — never mutate via in-place rewrite (Pitfall 7)
    - schema-missing degrade via information_schema probe → 503 with operator instructions
    - server-side struct re-marshal as a privacy boundary (extra JSON fields silently dropped)
    - background time.Ticker goroutine on the server's lifecycle context (no pg_cron)
key_files:
  created:
    - internal/server/store/layout_snapshots.go
    - internal/server/store/layout_snapshots_test.go
    - internal/server/api/layout_handlers.go
    - internal/server/api/layout_handlers_test.go
    - cmd/trasker-server/layout_downsampler.go
    - .planning/phases/07-layout-snapshots/deferred-items.md
  modified:
    - internal/server/store/devices.go      # +GetDeviceByID
    - internal/server/api/router.go         # +3 routes (1 API-key + 2 JWT)
    - cmd/trasker-server/main.go            # +1 goroutine spawn
decisions:
  - "Downsampling runs as an in-process Go time.Ticker (1h cadence), NOT pg_cron — Pitfall 4 reaffirmed by RESEARCH.md A1."
  - "Server-side re-marshal of windows through a 6-field struct is the canonical privacy boundary — even if json.Decode tolerates unknown fields, storage gets exactly 6 keys per window."
  - "ON CONFLICT (device_id, captured_at, windows_hash) DO NOTHING with (nil, nil) sentinel for the dedup case — handler counts those as 'deduped' and still responds 201, so client retries are idempotent without coupling the wire format to a 409."
  - "Schema-missing detection lives in the handlers (returns 503 with the migration filename to apply); the downsampler tolerates the same condition by logging WARN and continuing."
  - "GetDeviceByID was added (one accessor, ~14 lines) instead of routing every device-authorization through ListDevicesByUser+filter — clearer intent and saves a list-scan per request."
metrics:
  duration: ~25 min wall clock
  tasks_completed: 3
  files_changed: 9 (6 created, 3 modified)
  tests_added: 23 (10 store + 13 handler)
  lines_added: ~1100
  completed: 2026-05-07
---

# Phase 7 Plan 03: Server ingest + query + downsampler Summary

JWT-authed dashboard reads, API-key-authed client ingest, and an in-process hourly downsampler — all wired through pgx accessors that never UPDATE the JSONB column, with a 503 fallback when migrations/004 has not been applied.

## What Was Built

**New endpoints (API-key auth):**
- `POST /api/v1/layout-snapshots` — accepts `{client_device_id, snapshots: [...]}`; per-snapshot `windows` is re-marshalled through a 6-field struct (privacy boundary); 4MB body cap via `http.MaxBytesReader`; idempotent (dedup returns 201).

**New endpoints (JWT auth):**
- `GET /api/v1/layout-snapshots?device_id=&t=ISO` — most-recent snapshot at-or-before T; 404 when nothing precedes T; 403 on cross-user device read.
- `GET /api/v1/layout-snapshots/timeline?device_id=&from=&to=` — ascending list of `{id, captured_at, windows_count}`; default `from`=midnight UTC, `to`=now; uses shared `parsePagination`.

**Store accessors (`internal/server/store/layout_snapshots.go`):**
```go
func (s *Store) InsertSnapshot(ctx, InsertSnapshotParams) (*LayoutSnapshot, error)  // (nil,nil) on conflict
func (s *Store) GetSnapshotAt(ctx, deviceID, t) (*LayoutSnapshot, error)            // ErrNotFound on no row
func (s *Store) ListTimestamps(ctx, deviceID, from, to, limit, offset) ([]TimelineEntry, error)
func (s *Store) DownsampleTo10Min(ctx, now) (deleted int64, err error)
func (s *Store) DownsampleTo1Hr(ctx, now) (deleted int64, err error)
func (s *Store) HasLayoutSnapshotsTable(ctx) (bool, error)
```

**Background downsampler (`cmd/trasker-server/layout_downsampler.go`):**
- `runLayoutDownsampler(ctx, store, logger)` — 30s settle delay, then `time.Ticker(1h)`.
- Each cycle: `DownsampleTo10Min` (raw → 10min for rows aged > 7d), then `DownsampleTo1Hr` (10min → 1hr for rows aged > 37d).
- INFO log on rows deleted; WARN log on errors (table-missing is non-fatal — operator-facing 503 is the visible signal).
- Inherits server lifecycle context — exits cleanly on SIGTERM.

**Schema-missing handling:** All three HTTP handlers probe `HasLayoutSnapshotsTable` before doing real work. If absent, they respond `503 Service Unavailable` with body `{"error":"layout_snapshots table not migrated; apply migrations/004_layout_snapshots.up.sql"}` — the operator sees the exact file to apply with `psql -f`.

## Privacy Strip Mechanism

Three layers of defense — only one is required for correctness; the others are belt-and-suspenders:

1. **Inbound struct** — `layoutWindowReq` declares exactly 6 fields (`app_name`, `window_title`, `x`, `y`, `w`, `h`). Default `json.Decode` already drops unknown fields silently (no `DisallowUnknownFields()` is needed — see https://pkg.go.dev/encoding/json#Decoder).
2. **Server-side re-marshal** — every accepted snapshot's `Windows` slice is `json.Marshal`'d back to bytes through that 6-field struct before reaching `InsertSnapshot`, so even hypothetical decoder quirks cannot leak unknown fields into JSONB.
3. **Test asserts the round-trip** — `TestLayoutIngest_PrivacyReject` POSTs a snapshot whose inner window includes `{"pid": 1234, "cmdline": "..."}`, then queries `windows::text` directly from Postgres and asserts neither string is present in the stored row.

The cross-user 403 case (T-7-03-01) is covered by `TestLayoutAt_WrongUser` — a second user's JWT against the first user's `device_id` returns 403, never the snapshot.

## Tests Added (23 total, all green)

**Store-level (10):** `TestLayoutSnapshots_InsertAndGet`, `TestLayoutSnapshots_GetAt_MostRecentBefore`, `TestLayoutSnapshots_GetAt_NoneBefore`, `TestLayoutSnapshots_Dedup`, `TestLayoutSnapshots_ListTimestamps`, `TestDownsample_10min`, `TestDownsample_10min_LeavesRecentRaw`, `TestDownsample_1hr`, `TestHasLayoutSnapshotsTable_Present`, `TestHasLayoutSnapshotsTable_Absent`.

**Handler-level (13):** `TestLayoutIngest_Single`, `TestLayoutIngest_Batch`, `TestLayoutIngest_Dedup`, `TestLayoutIngest_BadAuth`, `TestLayoutIngest_BadDevice`, `TestLayoutIngest_OversizePayload`, `TestLayoutIngest_InvalidJSON`, `TestLayoutIngest_PrivacyReject`, `TestLayoutAt`, `TestLayoutAt_NotFound`, `TestLayoutTimeline`, `TestLayoutAt_TableMissing`, `TestLayoutAt_WrongUser`.

Suite results:
- `go test ./internal/server/store -count=1` → ok (~56s incl. testcontainer)
- `go test ./internal/server/api -count=1` → ok (~50s)
- `go test ./internal/server/auth -count=1` → ok
- `go vet ./internal/server/... ./cmd/trasker-server/...` → clean
- `go build ./internal/server/... ./cmd/trasker-server/...` → clean
- `CGO_ENABLED=0 go build ./cmd/trasker-server` → clean

## TDD Gate Compliance

Both store and handler tasks followed RED → GREEN:
- 35ed509 `test(07-03): add failing tests for layout_snapshots store` (RED)
- 052f5f8 `feat(07-03): implement layout_snapshots store accessors` (GREEN)
- 4e5692c `test(07-03): add failing tests for layout snapshot handlers` (RED — verified 404 from missing route before GREEN)
- 57f02ea `feat(07-03): add layout snapshot HTTP handlers + router mounts` (GREEN)
- ebde3c0 `feat(07-03): wire layout downsampler goroutine into trasker-server` (Task 3 — no TDD per plan; behavior is exercised by the store-level downsample tests)

REFACTOR was unnecessary — the GREEN implementations were already aligned with the plan's atomic structure (each accessor < 50 LOC, handler < 150 LOC).

## Acceptance Criteria — All Met

| Check | Result |
|-------|--------|
| `grep -c "ON CONFLICT (device_id, captured_at, windows_hash) DO NOTHING" layout_snapshots.go` | 1 (required: 1) |
| `grep -c "jsonb_set" layout_snapshots.go` | 0 (required: 0 — Pitfall 7) |
| `grep -c -E 'WHERE captured_at = ' layout_snapshots.go` | 0 (required: 0 — sparse rows) |
| `grep -c "BeginTx" layout_snapshots.go` | 2 (required: ≥2 — both downsamplers transactional) |
| Privacy gate handlers + store | 0 (required: 0) |
| `grep -c -E 'r\.(Post\|Get)\("/layout-snapshots' router.go` | 3 (required: 3) |
| API-key group has /layout-snapshots | 1 |
| JWT group has /layout-snapshots routes | 2 |
| `grep -c "4 << 20" layout_handlers.go` | 1 (4MB cap) |
| `grep -c "go runLayoutDownsampler" main.go` | 1 |
| `grep -c "time.NewTicker" layout_downsampler.go` | 1 |
| `go test ./internal/server/... -count=1` | green |
| `go build ./internal/server/... ./cmd/trasker-server/...` | green |
| `CGO_ENABLED=0 go build ./cmd/trasker-server` | green |

## Threat Model Mitigations Verified

| Threat | Mitigation | Verified by |
|--------|------------|-------------|
| T-7-03-01 cross-user device read | `authorizeDeviceForUser` → 403 on `device.UserID != userID` | `TestLayoutAt_WrongUser` |
| T-7-03-02 privacy field smuggling | 6-field struct + server-side re-marshal | `TestLayoutIngest_PrivacyReject` |
| T-7-03-04 oversize payload DoS | `http.MaxBytesReader(4MB)` | `TestLayoutIngest_OversizePayload` |
| T-7-03-05 raw SQL injection | All accessors use parameterized pgx queries | code review (pgx pattern) |
| T-7-03-06 JSONB UPDATE bloat | Bucketing CTE = DELETE losers + UPDATE tier on survivors | `grep -c jsonb_set` returns 0 |
| T-7-03-07 downsampler runaway | Transactional CTE with explicit `row_number() OVER PARTITION BY` | `TestDownsample_10min`, `TestDownsample_1hr` (exact survivor counts) |
| T-7-03-08 repudiation | INFO log on every successful downsample with `deleted` count | code review of `runLayoutDownsampler` |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Missing critical functionality] Added GetDeviceByID accessor**
- **Found during:** Task 2 (handler implementation).
- **Issue:** Plan asked for "device.UserID == userID" authorization but the existing store had only `GetDeviceByClientID(clientID, apiKeyID)` and `ListDevicesByUser(userID)`. Neither maps a server-side `uuid` to a `Device` by primary key.
- **Fix:** Added `GetDeviceByID(ctx, id) (*Device, error)` to `internal/server/store/devices.go` — single QueryRow, ~14 LOC, mirrors the existing `GetDeviceByClientID` shape.
- **Files modified:** `internal/server/store/devices.go`.
- **Commit:** 57f02ea (folded into Task 2 GREEN).

**2. [Rule 2 — Missing critical functionality] Added TestLayoutAt_WrongUser**
- **Found during:** Task 2 test design — RESEARCH/threat model lists T-7-03-01 (cross-user device read) but the plan's `<behavior>` section did not include this test case explicitly.
- **Fix:** Added the 13th handler test (`TestLayoutAt_WrongUser`) — creates a second user, issues their own JWT, and verifies a GET against the first user's device returns 403. Threat T-7-03-01 is now covered by automated test, not just code review.
- **Commit:** 4e5692c (RED) + 57f02ea (GREEN passes the new test).

### Out-of-scope discoveries (logged, not fixed)

**D-1: Pre-existing `internal/client/webui/embed.go` go:embed failure**
- `go build ./...` fails with `pattern all:static: no matching files found`. Verified pre-existing on the base commit by stashing 07-03 changes and re-running `go build ./...` — same error.
- Logged in `.planning/phases/07-layout-snapshots/deferred-items.md`. Belongs to a frontend or deploy-bundling phase, not 07-03.

**Workaround used:** all 07-03 verifications run on `./internal/server/... ./cmd/trasker-server/...` directly, which build and test cleanly.

## Authentication Gates

None. The implementation never paused for human input.

## Known Stubs

None. Every method has a real implementation; every test asserts real behavior. The downsampler's "table missing" path logs WARN intentionally (no stub) — the user-facing 503 in handlers is the actionable surface, by design.

## Self-Check: PASSED

Files created/modified verified by `ls`:
- FOUND: internal/server/store/layout_snapshots.go
- FOUND: internal/server/store/layout_snapshots_test.go
- FOUND: internal/server/api/layout_handlers.go
- FOUND: internal/server/api/layout_handlers_test.go
- FOUND: cmd/trasker-server/layout_downsampler.go
- FOUND: cmd/trasker-server/main.go (modified)
- FOUND: internal/server/api/router.go (modified)
- FOUND: internal/server/store/devices.go (modified)
- FOUND: .planning/phases/07-layout-snapshots/deferred-items.md

Commits verified by `git log --oneline 7154943..HEAD`:
- FOUND: 35ed509 test(07-03) — store RED
- FOUND: 052f5f8 feat(07-03) — store GREEN
- FOUND: 4e5692c test(07-03) — handler RED
- FOUND: 57f02ea feat(07-03) — handler GREEN + GetDeviceByID + router mount
- FOUND: ebde3c0 feat(07-03) — downsampler goroutine + deferred items

Cross-user 403 case (T-7-03-01): COVERED by `TestLayoutAt_WrongUser`.
