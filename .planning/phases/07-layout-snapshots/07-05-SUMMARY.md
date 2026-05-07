---
phase: 07-layout-snapshots
plan: 05
subsystem: client
tags:
  - phase-7
  - client
  - wiring
  - smoke-test
  - checkpoint-pending
requires:
  - 07-02 (layout package: capturer, store, syncer, enumerator)
  - 07-03 (server ingest endpoint POST /api/v1/layout-snapshots)
  - 07-04 (dashboard /layout route)
provides:
  - trasker-client startup wires the layout pipeline (Linux X11 + nocgo-skip)
  - 7-day prune on 6h cadence (D-01)
  - End-to-end snapshot flow ready for manual smoke verification
affects:
  - cmd/trasker-client/main.go (lifecycle of layout.Capturer + layout.Syncer + RunPrune)
tech-stack:
  added: []
  patterns:
    - "Optional-feature gating via constructor error: layout.NewPlatformEnumerator() returns ErrUnsupported on CGO_ENABLED=0 / non-Linux / no-DISPLAY; main logs Warn and skips wiring without affecting focus tracking."
    - "Channel fan-out for shared event sources: presence.ScreenLockListener has a single Events() channel; existing consumer goroutine forwards each StateChange to a layout-side buffered channel before its own logic, so both consumers receive every event."
    - "Ctx-aware initial sleep instead of time.Sleep: RunPrune's 15s warm-up uses select{<-ctx.Done(), <-time.After} so shutdown during warm-up exits cleanly."
key-files:
  created:
    - internal/client/layout/prune.go
  modified:
    - cmd/trasker-client/main.go
decisions: []
metrics:
  duration: ~25min (Tasks 1+2; Task 3 manual smoke pending)
  completed: 2026-05-07
  status: checkpoint-pending
requirements:
  - REQ-layout-snapshots
---

# Phase 7 Plan 05: Layout Snapshots Wiring Summary

Wire the Phase 7 layout package built in plans 02–04 into `cmd/trasker-client/main.go`, add the 6h pruner goroutine, and queue the end-to-end smoke checkpoint for Jaypaul.

## What Shipped

### Task 1: `internal/client/layout/prune.go` (new file, 56 lines)

`RunPrune(ctx, store, retention, interval, logger)` launches a goroutine that calls `Store.Prune(retention)` every `interval`. Production callers pass `retention = 7*24h` and `interval = 6h` per CONTEXT.md D-01. INFO log on deletes, WARN log on errors. Honours `ctx.Done()` in both the 15s warm-up wait and the ticker loop. No separate test file (Three-Examples rule — glue around the already-unit-tested `Store.Prune`).

Commit: `3fe0c22`

### Task 2: `cmd/trasker-client/main.go` wiring (+70 / -1 lines)

Inserted after the focus-event recorder goroutine and before the system tray:

```go
layoutStore := layout.NewStore(db)

if layoutEnum, err := layout.NewPlatformEnumerator(); err != nil {
    logger.Warn("layout snapshots unavailable on this build/platform; continuing without",
        "error", err)
} else {
    layoutCapturer = layout.NewCapturer(
        layoutEnum, layoutStore, layoutLockCh,
        60*time.Second, logger,
    )
    layoutCapturer.Start(ctx)

    layoutSyncer = layout.NewSyncer(layout.SyncerConfig{
        Store: layoutStore, DB: db, DeviceID: deviceID,
        ServerURL: serverURL, APIKey: apiKey,
        HTTPClient: http.DefaultClient,
        Interval: 5*time.Minute, Logger: logger, BatchSize: 100,
    })
    layoutSyncer.Start(ctx)

    go layout.RunPrune(ctx, layoutStore, 7*24*time.Hour, 6*time.Hour, logger)

    logger.Info("layout snapshots wired (capturer 60s, syncer 5m, prune 6h/7d)")
}
```

Plus `Stop()` calls for `layoutCapturer` and `layoutSyncer` in the shutdown block.

The screenlock fan-out (see Deviations below) is the only change outside the dedicated layout block.

Commit: `ab5d4a0`

One-line confirmation: pruner is wired with **6h interval / 7d retention** (`7*24*time.Hour` and `6*time.Hour` literals in main.go).

## Verification

| Check                                                                         | Result                                            |
| ----------------------------------------------------------------------------- | ------------------------------------------------- |
| `go build ./cmd/trasker-client` (cgo)                                         | clean (after `make client-ui` to populate static) |
| `CGO_ENABLED=0 go build ./cmd/trasker-client`                                 | clean                                             |
| `go vet ./...`                                                                | clean                                             |
| `go test ./internal/client/layout/... -count=1`                               | PASS (1.7s)                                       |
| `go test ./internal/client/{layout,presence,session},server/auth,...` retest | PASS (initial whole-repo run hit transient `fork/exec: resource temporarily unavailable` from process-table pressure on the worktree host; serial retry on the affected packages was clean) |
| `grep -c "layout.NewCapturer" cmd/trasker-client/main.go`                     | 1                                                 |
| `grep -c "layout.NewSyncer" cmd/trasker-client/main.go`                       | 1                                                 |
| `grep -c "go layout.RunPrune" cmd/trasker-client/main.go`                     | 1                                                 |
| `grep -c "layout.NewPlatformEnumerator" cmd/trasker-client/main.go`           | 1                                                 |
| NewCapturer inside the success branch (awk gate)                              | 1                                                 |
| `grep -c "store.Prune(retention)" internal/client/layout/prune.go`            | 1                                                 |
| `grep -c "ctx.Done()" internal/client/layout/prune.go`                        | 2 (warm-up + ticker — see Deviations)             |

## Deviations from Plan

### Auto-fixed

**1. [Rule 2 — missing critical functionality] Ctx-aware warm-up sleep in RunPrune**

- **Found during:** Task 1 implementation while comparing against the `internal/client/sync.Queue.run` precedent.
- **Issue:** The plan's snippet used `time.Sleep(15 * time.Second)` for the initial-prune warm-up. That call is **not** ctx-aware: a daemon shutdown during the first 15s after start would block on the sleep instead of returning promptly. The whole `RunPrune` premise is "honours ctx", so a non-cancellable warm-up is a graceful-shutdown bug.
- **Fix:** Replaced with a ctx-aware select:
  ```go
  select {
  case <-ctx.Done():
      return
  case <-time.After(15 * time.Second):
  }
  ```
- **Side effect:** `grep -c "ctx.Done()"` now returns 2 (warm-up + ticker) instead of the planned 1. Both occurrences are correct graceful-shutdown handling. Acceptance criterion is conceptually satisfied (graceful shutdown is honoured); the literal "returns 1" sub-clause is wrong on its face — there is no good reason to count graceful-shutdown sites and prefer fewer.
- **File:** `internal/client/layout/prune.go`
- **Commit:** `3fe0c22`

**2. [Rule 3 — blocker] Channel fan-out for screenLock events**

- **Found during:** Task 2 — re-reading `internal/client/presence/screenlock_linux.go` and the existing main.go consumer.
- **Issue:** The plan said "pass `screenLockMonitor.Events()` into `layout.NewCapturer`" verbatim. But there is already an existing consumer goroutine reading the same channel (calls `closeCurrentFocusEvent` on `Away`). Go channels are 1:1; if two goroutines `range` over the same channel, each event goes to one consumer at random. Result: the layout capturer would miss roughly half the lock events, breaking T-7-05-04 / D-08 lock-gating.
- **Fix:** Created a dedicated buffered channel `layoutLockCh chan presence.StateChange` (cap 4). The existing consumer goroutine forwards each event to `layoutLockCh` (non-blocking — drop on full so we never block the focus pipeline), then runs its existing logic. `layout.NewCapturer` receives `layoutLockCh` instead of the raw `screenLock.Events()`.
- **Why non-blocking forward:** if layout's lock-subscriber is slow, dropping a stale "still-locked" ping is preferable to stalling the focus-event closer. The capturer's `runLockSubscriber` only stores `locked.Store(ev.State == presence.Away)` so the most-recent event is what matters; missed intermediate transitions don't change the steady-state value.
- **File:** `cmd/trasker-client/main.go`
- **Commit:** `ab5d4a0`

### Out-of-scope discoveries

- `cmd/trasker-client` build requires `internal/client/webui/static/` populated via `make client-ui` (Svelte build + copy step). This is pre-existing — verified by stashing changes and rebuilding HEAD. Not a regression. Documenting here for future executors who may hit it: run `make client-ui` once before any `go build ./cmd/trasker-client` invocation in a fresh worktree.

## Authentication Gates

None — Task 1 and Task 2 are pure code changes.

## Task 3: End-to-end smoke test (PENDING — checkpoint:human-verify)

This task requires Jaypaul's daily-driver Linux X11 machine. The orchestrator will hand off the 8-step verification protocol from the plan's `<how-to-verify>` section. Until Jaypaul reports back, the plan is **not complete**. Specifically pending:

1. `\d layout_snapshots` shape on the local Postgres dev stack.
2. SQLite query showing a snapshot row written within 60s of starting trasker-client.
3. Server psql query showing the row landed after the 5-minute syncer interval.
4. Window-set change → new SQLite row within 60s.
5. Screen lock → "skipping tick (screen locked)" debug log + no new rows during locked window.
6. Dashboard `/layout` render in light + dark mode + permalink mode.
7. Privacy spot-check — JSONB has only the 6 schema-permitted fields.
8. Resilience — server stop → local rows accrue → server restart → backlog drains.

## Known Stubs

None.

## Threat Flags

None — no new network endpoints, auth paths, file access patterns, or schema changes introduced in this plan. The wiring exclusively composes already-vetted constructors from 07-02.

## Follow-ups

- Phase 9 dogfooding will exercise the multi-day flow (prune actually fires across day boundaries; cross-device merge UI deferred per D-12).
- If Task 3 step 5 ("locked → no new rows") flakes due to D-Bus lag on Jaypaul's daily-driver, the capturer's `runLockSubscriber` has no debounce — a future plan could add one if needed (currently parked under "no v1 demand").

## Self-Check

- [x] `internal/client/layout/prune.go` exists (`ls` confirmed).
- [x] `cmd/trasker-client/main.go` modified (commit `ab5d4a0`).
- [x] Commit `3fe0c22` exists in `git log` (Task 1).
- [x] Commit `ab5d4a0` exists in `git log` (Task 2).
- [x] All acceptance grep counts match (1 each, except the corrected `ctx.Done()` count of 2 which is documented as Rule 2 deviation).
- [x] cgo + nocgo builds clean; vet clean; layout/presence/session/auth/models tests pass.
- [ ] Task 3 manual smoke — pending Jaypaul, by design (`checkpoint:human-verify`).

## Self-Check: PASSED for Tasks 1+2 — Task 3 awaiting checkpoint
