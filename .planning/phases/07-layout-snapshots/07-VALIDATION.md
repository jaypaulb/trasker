---
phase: 7
slug: layout-snapshots
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-07
---

# Phase 7 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

- **Framework:** Go stdlib `testing` + `testify/assert,require` (already in go.mod)
- **Config file:** none — `go.mod` driven
- **Quick run command:** `go test ./internal/client/layout/... ./internal/server/store/... ./internal/server/api/... -count=1`
- **Full suite command:** `go test ./... -count=1`
- **X11 integration command:** `DISPLAY=:0 go test -tags x11_integration ./internal/client/layout/...`
- **Frontend build check:** `cd web/server-ui && npm run build`
- **Estimated quick runtime:** ~30s (target)
- **Estimated full runtime:** ~90s (current ./... is ~75s)

---

## Sampling Rate

- **After every task commit:** quick run command (target < 30s)
- **After every plan wave:** full suite + frontend build
- **Before `/gsd-verify-work`:** full suite green AND X11 integration smoke pass on Jaypaul's machine
- **Max feedback latency:** 30 seconds (per-task), 90 seconds (per-wave)

---

## Per-Task Verification Map

Status legend: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky · ❌ W0 = file does not exist yet (Wave 0)

### Hash determinism + change detection (REQ-layout-snapshots)

- **T-7-01-01** — Same window-set produces identical hash
  - Test type: unit · File: `internal/client/layout/layout_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestHashWindows -count=1`

- **T-7-01-02** — Any field change produces different hash
  - Test type: unit · File: `internal/client/layout/layout_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestHashChange -count=1`

### Capturer behavior (REQ-layout-snapshots)

- **T-7-01-03** — Capturer skips writes when hash unchanged
  - Test type: unit · File: `internal/client/layout/capturer_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestCapturer_SkipsUnchanged -count=1`

- **T-7-01-04** — Capturer skips entire tick when screen locked
  - Test type: unit · File: `internal/client/layout/capturer_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestCapturer_SkipsWhenLocked -count=1`

- **T-7-01-05** — Capturer fail-soft on enum error (logs WARN, no row)
  - Test type: unit · File: `internal/client/layout/capturer_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestCapturer_FailSoft -count=1`

### Local store (REQ-layout-snapshots)

- **T-7-01-06** — SQLite insert + retrieval roundtrip
  - Test type: unit · File: `internal/client/layout/store_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestStore_RoundTrip -count=1`

- **T-7-01-07** — Client 7-day prune deletes rows older than retention
  - Test type: unit · File: `internal/client/layout/store_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestStore_Prune7Days -count=1`

### Sync (REQ-layout-snapshots)

- **T-7-01-08** — Sync goroutine retries on transient failure with backoff
  - Test type: unit · File: `internal/client/layout/sync_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestSync_RetryBackoff -count=1`

- **T-7-01-09** — Sync goroutine handles permanent 401/403 (key revoked)
  - Test type: unit · File: `internal/client/layout/sync_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/client/layout -run TestSync_KeyExpired -count=1`

### Server ingest + dedup (REQ-layout-snapshots)

- **T-7-02-01** — `POST /api/v1/layout-snapshots` accepts JSONB payload
  - Test type: unit · File: `internal/server/api/layout_handlers_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/server/api -run TestLayoutIngest -count=1`

- **T-7-02-02** — Server dedups on `(device_id, captured_at, windows_hash)`
  - Test type: unit · File: `internal/server/store/layout_snapshots_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/server/store -run TestLayoutSnapshots_Dedup -count=1`

### Server query (REQ-layout-snapshots)

- **T-7-02-03** — `GET /api/v1/layout-snapshots?t=ISO` returns most-recent ≤ T
  - Test type: unit · File: `internal/server/api/layout_handlers_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/server/api -run TestLayoutAt -count=1`

- **T-7-02-04** — `GET /api/v1/layout-snapshots/timeline` returns ascending timestamps
  - Test type: unit · File: `internal/server/api/layout_handlers_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/server/api -run TestLayoutTimeline -count=1`

### Server downsampling (REQ-layout-snapshots)

- **T-7-02-05** — 7d→10min tier collapses correctly
  - Test type: unit · File: `internal/server/store/layout_snapshots_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/server/store -run TestDownsample_10min -count=1`

- **T-7-02-06** — 37d→1hr tier collapses correctly
  - Test type: unit · File: `internal/server/store/layout_snapshots_test.go` · ❌ W0 · Status: ⬜
  - Command: `go test ./internal/server/store -run TestDownsample_1hr -count=1`

### Manual smoke (REQ-layout-snapshots)

- **T-7-03-01** — X11 capture against real GNOME/i3 X11 session
  - Test type: manual · File: n/a · Status: ⬜
  - Command: smoke run `trasker-client` on Jaypaul's machine, inspect `layout_snapshots` rows
  - Note: requires real DISPLAY; cannot run in CI

- **T-7-03-02** — SvelteKit `/layout` view renders timestamp list + click-to-expand
  - Test type: manual · File: n/a · Status: ⬜
  - Command: `cd web/server-ui && npm run dev`; navigate to `/layout`; verify timeline and `?t=` permalink behavior
  - Note: requires running server + dev UI

---

## Wave 0 Requirements

Files needed before any production code:

- [ ] `internal/client/layout/layout.go` — `Window` struct + `HashWindows()` function
- [ ] `internal/client/layout/layout_test.go` — hash unit tests (no X11 needed)
- [ ] `internal/client/layout/capturer.go` — capture loop with injected `Enumerator` interface (X11 impl satisfies it)
- [ ] `internal/client/layout/capturer_test.go` — fake Enumerator + mock screenlock channel
- [ ] `internal/client/layout/store.go` — sqlite insert + range-query + prune
- [ ] `internal/client/layout/store_test.go` — roundtrip + prune coverage
- [ ] `internal/client/layout/sync.go` — sync goroutine, mirrors existing `internal/client/sync` retry/backoff
- [ ] `internal/client/layout/sync_test.go` — `httptest.NewServer` based, mirrors `internal/client/sync/client_test.go`
- [ ] `internal/client/layout/enum_linux_x11.go` — cgo X11 enumerator (XQueryTree + XTranslateCoordinates + EWMH `_NET_FRAME_EXTENTS`)
- [ ] `internal/client/layout/enum_linux_nocgo.go` — `ErrUnsupported` stub
- [ ] `internal/client/layout/enum_darwin.go` — stub (returns `ErrUnsupported`)
- [ ] `internal/client/layout/enum_windows.go` — stub (returns `ErrUnsupported`)
- [ ] `internal/server/store/layout_snapshots.go` — pgx accessors + downsample helpers
- [ ] `internal/server/store/layout_snapshots_test.go` — uses existing `testhelper_test.go` pgx pool
- [ ] `internal/server/api/layout_handlers.go` — POST ingest, GET at-time, GET timeline
- [ ] `internal/server/api/layout_handlers_test.go` — handler tests
- [ ] `migrations/004_layout_snapshots.up.sql` + `.down.sql`
- [ ] `deploy/initdb/001_schema.sql` — append `layout_snapshots` block for fresh installs
- [ ] `web/server-ui/src/routes/(app)/layout/+page.ts` — load function, reads `?t=` query param
- [ ] `web/server-ui/src/routes/(app)/layout/+page.svelte` — timestamp list + click-to-expand
- [ ] `web/server-ui/src/lib/types.ts` — add `LayoutSnapshot`, `LayoutWindow` types
- [ ] `web/server-ui/src/lib/api.ts` — add `layout: { timeline, at }` namespace
- [ ] X11 integration test: `internal/client/layout/enum_linux_x11_integration_test.go` guarded by `//go:build linux && cgo && x11_integration`

Wave 0 marks `wave_0_complete: true` once all listed files exist (even with stubbed bodies) so subsequent waves can fill them.

---

## Notes

- **No multi-monitor coverage** v1 — display_index is intentionally absent from schema.
- **No Wayland coverage** v1 — enum_linux_wayland.go intentionally not added; deferred.
- **No macOS/Windows real implementations** — stubs only, returning `ErrUnsupported`. Phase 7 success criterion #3 amendment captures this.
- **Privacy invariant** — all tests must verify the row payload contains ONLY `app_name`, `window_title`, `x`, `y`, `w`, `h`. Any test that adds PID, cmdline, or screenshot fields is a plan-checker red flag.
