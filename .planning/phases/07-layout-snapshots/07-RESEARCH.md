# Phase 7: Layout Snapshots — Research

**Researched:** 2026-05-07
**Domain:** X11 window enumeration (cgo) + SQLite/Postgres time-series persistence + SvelteKit history view
**Confidence:** HIGH (codebase patterns, JSONB tradeoffs, X11 APIs all verified); MEDIUM (downsampling implementation choice — multiple viable options)

## Summary

Phase 7 adds a parallel "layout snapshot" data path alongside the existing focus-tracking pipeline. The work splits cleanly into five concerns: (1) Linux X11 window enumeration via cgo, (2) change-detect tick + lock-gating on the client, (3) local SQLite persistence, (4) sync to server (which CANNOT reuse the existing sync queue without modification — see below), and (5) a minimal SvelteKit "Layout History" view driven by a `?t=ISO` permalink.

Two findings dominate plan shape: **(A) the existing client `internal/client/sync/` package is hard-coded to focus_events/timesheets** — the queue's `loadSubmissionEntries` JOINs `submission_events`, `focus_events`, `event_tags`, `tags`, `notes`. Layout snapshots cannot simply be enqueued; they need their own sync loop or a refactored generic outbox. The path of least resistance is a NEW `internal/client/layout/sync.go` goroutine that mirrors the queue's retry/backoff pattern but operates on a `layout_snapshots_outbox` (or a `synced_at IS NULL` filter on the snapshots table itself). **(B) The server has no in-process migration runner** — schema is applied via Postgres' `/docker-entrypoint-initdb.d/` (one-shot, only on empty volume) and `migrations/*.up.sql` files are copied into the image but never executed by code. Adding `layout_snapshots` therefore requires (i) appending to `deploy/initdb/001_schema.sql` for fresh deployments AND (ii) a manual `psql -f migrations/004_layout_snapshots.up.sql` step for the eventual deployed instance. Phase 8 may introduce a runner; Phase 7 must not depend on that.

**Primary recommendation:** Create a new `internal/client/layout/` package (capture loop + change-detect + sync) that is *parallel* to, not built on top of, the existing `sync` package. Use cgo + EWMH `_NET_CLIENT_LIST_STACKING` (with `XQueryTree` fallback) for X11 enumeration. Schema = JSONB array of `{app_name, title, x, y, w, h}` per the locked CONTEXT.md decision — the JSONB write-amplification concern doesn't apply here because rows are insert-only, never updated. Server downsampling = in-process goroutine in `trasker-server` using `time.Ticker`, NOT pg_cron (pg_cron is not assumed available on hal's stock Postgres). Dashboard = new `/layout` route in `web/server-ui/src/routes/(app)/layout/` with a `+page.ts` load function reading `?t=` from `url.searchParams`.

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Retention policy**
- D-01: Client SQLite keeps raw 60s-evaluation snapshots for **7 days**, then deletes. Server is the source of truth beyond that window.
- D-02: Server PostgreSQL retention is tiered:
  - 0–7 days: raw 60s-evaluation cadence
  - 7–37 days: downsampled to 10-minute representative snapshots
  - 37+ days: downsampled to 1-hour representative snapshots, kept indefinitely
- D-03: No per-app retention overrides in v1. Single global policy.

**Cross-platform scope (v1)**
- D-04: Linux X11 only. Wayland deferred.
- D-05: macOS and Windows clients deferred to v2. Existing tracker code on those platforms is untouched in this phase.
- D-06: No multi-monitor handling v1. Capture only window-relative absolute X/Y coordinates from the X11 root window. No display-index field on the schema.

**Snapshot trigger**
- D-07: Change-detect cadence — hash the current window-set (sorted `(app_name, window_title, x, y, w, h)` tuples) every 60s. Write a new `layout_snapshots` row only when the hash differs from the previous tick. Idle periods produce zero rows; reconstruction queries use "most recent snapshot ≤ T".
- D-08: Skip the tick entirely when the screen is locked. Reuse existing `internal/client/presence/screenlock_linux.go` lock signal.
- D-09: Fail-soft on enum failure — log a WARN with the X11 error, skip writing a row, retry on the next 60s tick. Do not poison the daemon or write empty error rows.

**Dashboard UX**
- D-10: Primary view is a plain list of timestamps with a click-to-expand window list per row. No timeline scrubber, no geometry mosaic, no calendar picker.
- D-11: Permalink-driven instant selection. Default view = today's snapshot list for the current device. Past instants reachable only via `?t=<ISO-8601>` query param. No natural-language picker.
- D-12: Per-device view only. Multi-device merge UI deferred.
- D-13: No search v1.

**Schema**
- D-14: New `layout_snapshots` table independent of `focus_events`. Submitted-timesheet immutability invariant preserved.
- D-15: Snapshot row schema (minimum): `id` (uuid), `device_id` (uuid, FK), `captured_at` (timestamptz, UTC), `windows` (JSONB array of `{app_name, window_title, x, y, w, h}` — NO display index, NO z-order, NO PID, NO cmdline), `windows_hash` (text, for change detection on the client).

**Privacy**
- D-16: Capture only `app_name + window_title + geometry`. NEVER window contents, screenshots, focus content beyond title, PID, or cmdline.

### Claude's Discretion

- Wire format for sync payload (single `POST /api/v1/sync` call vs new `/api/v1/layout-snapshots` endpoint) — planner picks based on existing sync architecture.
- Server-side downsampling implementation — cron job, pg_cron extension, or in-process scheduler.
- SvelteKit route shape for the Layout History list view — new `/layout` route or panel inside existing `/timeline`.

### Deferred Ideas (OUT OF SCOPE)

- Wayland support
- macOS layout enum (`CGWindowListCopyWindowInfo`)
- Windows layout enum (`EnumWindows` + per-monitor DPI)
- Multi-monitor display index
- Multi-device merge view
- Window-title full-text search
- Geometry mosaic visualization
- Per-app retention overrides / opt-out
- Adaptive cadence (snap on focus-change)

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| REQ-layout-snapshots | Client enumerates open windows + geometry, persists to local SQLite, syncs to server, and dashboard shows queryable history per device. | All sections below. v1 scope explicitly narrowed by CONTEXT.md to Linux X11 only, change-detect, per-device only — multi-monitor/multi-device-merge deferred. ROADMAP.md success criteria #1, #3, #4 must be amended (planner must call this out). |

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| X11 window enumeration | Client (cgo, linux) | — | Only the local machine can see its own window manager state |
| Change-detect hashing | Client (Go) | — | Avoids unnecessary writes — local computation, never server-side |
| Lock-gated capture loop | Client (Go) | — | Reuses existing `internal/client/presence/screenlock_linux.go` D-Bus signal |
| Local snapshot persistence | Client (SQLite, modernc) | — | Offline resilience; 7-day retention then prune |
| Sync to server | Client (HTTP) → Server (Chi handler + pgx) | — | Mirrors existing API-key auth path; new endpoint or extension of existing |
| Server snapshot persistence | Server (Postgres JSONB) | — | Single source of truth beyond 7-day client window |
| Tiered downsampling (7d→10min→1hr) | Server (Go goroutine) | — | Background `time.Ticker` in `trasker-server` main; pg_cron not assumed available |
| Retention pruning (server + client) | Server + Client | — | Each side runs its own prune ticker per its retention policy |
| Layout History view | Frontend Server (SvelteKit SSR) | API (read-only `GET`) | New `/layout` route reads `?t=` and fetches via existing JWT-authed API |
| Permalink (`?t=ISO`) parsing | Browser (URL) → Frontend Server (SvelteKit `+page.ts` load) | — | Standard SvelteKit `url.searchParams` pattern |

## Standard Stack

### Core (already in tree)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `modernc.org/sqlite` | (in-tree, locked by CONSTRAINT-tech-stack) | Client local store | [VERIFIED: codebase] Pure Go, no cgo — used everywhere in client store |
| `github.com/jackc/pgx/v5` | (in-tree, locked) | Server Postgres driver + pool | [VERIFIED: codebase] All `internal/server/store/` files use `pgxpool` |
| `github.com/go-chi/chi/v5` | (in-tree, locked) | Server router + middleware | [VERIFIED: codebase] `router.go` uses Chi exclusively |
| `github.com/google/uuid` | (in-tree) | UUID generation | [VERIFIED: codebase] Used for device.id, timesheet.id |
| `golang.org/x/sys` | (transitive) | Cross-platform syscalls | [ASSUMED] Available; not directly required for X11 |
| Cgo + libX11 | system lib | X11 window enumeration | [VERIFIED: codebase] `tracker_linux_x11.go` already uses `#cgo LDFLAGS: -lX11` and includes `<X11/Xlib.h>`, `<X11/Xatom.h>`, `<X11/Xutil.h>` |

### Supporting (no new third-party deps required for v1)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Standard `crypto/sha256` | stdlib | Window-set hash | Hashing the sorted tuple — collision risk is negligible at 60s cadence |
| Standard `encoding/json` | stdlib | Marshal `windows` JSONB column | pgx supports `[]byte` JSON marshalling natively for `JSONB` columns |
| Standard `time.Ticker` | stdlib | 60s capture loop, downsampling, retention prune | Existing pattern in `internal/client/sync/queue.go` (5-minute ticker) |

**Version verification:**
```bash
# X11 dev headers required at build time on Linux for cgo build
dpkg -s libx11-dev 2>/dev/null | head -3   # or pacman/dnf equivalent
# Confirmed installed via existing tracker_linux_x11.go build
```

No new Go dependencies needed. All work uses stdlib + libraries already in `go.mod`.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Cgo + libX11 directly | `github.com/BurntSushi/xgbutil` (pure-Go X11) | Pure Go, no cgo. Could fix the `CGO_ENABLED=0` cross-compile gap. Rejected for v1 because (a) introduces a new dependency, (b) inconsistent with existing `tracker_linux_x11.go` cgo pattern, (c) the same `CGO_ENABLED=0` warning already applies to focus tracking and is documented; layout enum can mirror that. Worth revisiting in v2. |
| In-process Go downsampling goroutine | `pg_cron` extension | pg_cron is not in the stock Postgres image used by `deploy/docker-compose.yml` and Phase 8 has not chosen to add it. In-process goroutine is zero-config and tied to server lifecycle. |
| In-process Go downsampling goroutine | TimescaleDB hypertables + continuous aggregates | TimescaleDB is the "right" tool for tiered time-series but requires switching the Postgres image. Out of scope for v1 — single-user-volume use case doesn't justify the operational change. |
| JSONB `windows` column | Normalized `layout_windows` child table (one row per window per snapshot) | JSONB write amplification concern from [Heap blog](https://www.heap.io/blog/when-to-avoid-jsonb-in-a-postgresql-schema) does NOT apply here: snapshots are insert-only (never `jsonb_set`), and reads always need the entire window set together. Normalized form would 30-50× the row count for no read-pattern benefit and complicate the wire format. JSONB is the correct choice. [VERIFIED via web search; CITED: heap.io] |
| EWMH `_NET_CLIENT_LIST_STACKING` only | `XQueryTree(root)` only | EWMH is faster + more semantic but unreliable on some WMs (notably GNOME 3+ misorders it across workspaces, and Chromium removed reliance on it for that reason — [CITED: chromium commit 58b7467]). Use EWMH first, fall back to `XQueryTree` if the property is absent. |

## Architecture Patterns

### System Architecture Diagram

```
                        Linux X11 client (daemon)
┌──────────────────────────────────────────────────────────────────────────┐
│                                                                          │
│  ┌─────────────┐   60s tick     ┌─────────────────────────────────────┐  │
│  │ time.Ticker │──────────────▶ │  layout.Capturer (NEW pkg)          │  │
│  └─────────────┘                │                                     │  │
│                                 │  1. screenlock signal? → skip       │  │
│  ┌─────────────────────┐        │  2. cgo: enum windows               │  │
│  │ presence.ScreenLock │ events │  3. compute windows_hash            │  │
│  │ Listener (existing) │───────▶│  4. if hash != last → write row     │  │
│  └─────────────────────┘        │  5. else → no write                 │  │
│                                 └──────────────┬──────────────────────┘  │
│                                                │                          │
│                                                ▼                          │
│                                 ┌──────────────────────────────────┐    │
│                                 │ SQLite: layout_snapshots table   │    │
│                                 │ (windows JSON, hash, captured_at)│    │
│                                 └──────────────┬───────────────────┘    │
│                                                │                          │
│                                                ▼                          │
│  ┌─────────────────────┐ retry  ┌──────────────────────────────────┐    │
│  │ layout.Syncer (NEW) │◀───────│ unsynced rows (synced_at NULL)   │    │
│  │ separate goroutine  │        └──────────────────────────────────┘    │
│  │ from sync.Queue     │                                                 │
│  └────────┬────────────┘                                                 │
│           │ HTTPS POST                                                    │
└───────────┼──────────────────────────────────────────────────────────────┘
            │ POST /api/v1/layout-snapshots  (Bearer api_key)
            ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                          trasker-server (hal)                            │
│                                                                          │
│  Chi router ──▶ APIKeyMiddleware ──▶ layoutSnapshotIngestHandler          │
│                                              │                            │
│                                              ▼                            │
│                            ┌────────────────────────────────────┐        │
│                            │ pgx: INSERT INTO layout_snapshots  │        │
│                            └────────────────────────────────────┘        │
│                                                                          │
│  ┌──────────────────────────────────┐                                    │
│  │ background goroutine (NEW)       │  every 1h                          │
│  │  • prune raw  > 7d                │                                    │
│  │  • collapse 7-37d → 10min         │                                    │
│  │  • collapse 37+d → 1hr            │                                    │
│  └──────────────────────────────────┘                                    │
│                                                                          │
│  Chi router ──▶ JWTMiddleware ──▶ GET /api/v1/layout-snapshots/timeline   │
│                                       (returns list of captured_at)       │
│                              ──▶ GET /api/v1/layout-snapshots?t=<ISO>     │
│                                       (returns single snapshot ≤ t)       │
│                                              │                            │
└──────────────────────────────────────────────┼───────────────────────────┘
                                               │
                              SvelteKit SSR    ▼
                              ┌──────────────────────────┐
                              │ /layout/+page.ts         │
                              │  load: fetch ?t= or list │
                              │ /layout/+page.svelte     │
                              │  list of timestamps,     │
                              │  click → expand windows  │
                              └──────────────────────────┘
```

### Recommended Project Structure

```
internal/client/
├── layout/                            # NEW package
│   ├── layout.go                      # Snapshot type, Window struct, hash function (no cgo)
│   ├── layout_test.go                 # Hash determinism, change-detect logic, no-X11-needed
│   ├── enum_linux_x11.go              # //go:build linux && cgo  — cgo enumeration
│   ├── enum_linux_x11_test.go         # //go:build linux && cgo && x11_integration
│   ├── enum_linux_nocgo.go            # //go:build linux && !cgo — returns ErrUnsupported
│   ├── enum_linux_wayland.go          # //go:build linux         — stub: ErrUnsupported (deferred)
│   ├── enum_darwin.go                 # //go:build darwin        — stub: ErrUnsupported
│   ├── enum_windows.go                # //go:build windows       — stub: ErrUnsupported
│   ├── capturer.go                    # 60s ticker + lock-gating + change-detect (no build tag)
│   ├── capturer_test.go               # uses fake Enumerator, mock screenlock
│   ├── store.go                       # SQLite accessors (Insert, ListPending, MarkSynced, Prune)
│   ├── store_test.go
│   └── sync.go                        # Separate goroutine; mirrors sync.Queue retry/backoff
│
internal/server/
├── store/
│   └── layout_snapshots.go            # NEW: pgx accessors (Insert, GetAt, ListTimestamps, Downsample, Prune)
├── api/
│   └── layout_handlers.go             # NEW: ingest (API-key) + dashboard reads (JWT)
└── (no changes to router.go beyond mounting the new handlers)

migrations/
├── 004_layout_snapshots.up.sql        # CREATE TABLE
└── 004_layout_snapshots.down.sql      # DROP TABLE

deploy/initdb/
└── 001_schema.sql                     # APPEND layout_snapshots block (for fresh deploys)

web/server-ui/src/
├── routes/(app)/layout/
│   ├── +page.ts                       # SvelteKit load: reads ?t= from url.searchParams
│   └── +page.svelte                   # list-of-timestamps + click-to-expand
└── lib/
    ├── api.ts                         # extend with layout: { timeline(), at(t) }
    └── types.ts                       # add LayoutSnapshot, LayoutWindow types
```

### Pattern 1: cgo X11 enumeration (extends tracker_linux_x11.go skeleton)

**What:** Open the X11 display once, intern atoms once, query the EWMH client list (with `XQueryTree` fallback), per-window fetch class hint + window name + geometry.

**When to use:** Capture loop calls `Enumerator.Enumerate() ([]Window, error)` once per 60s tick.

**Example (sketch — not full file):**
```go
//go:build linux && cgo

package layout

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/Xutil.h>
#include <stdlib.h>
#include <string.h>

// Returns count of windows enumerated, or -1 on error.
// Caller must allocate enough space; we cap at 256 windows for v1.
static int enumWindows(Display *dpy,
                       Window *out_wids, int max,
                       Atom net_client_list_stacking) {
    Window root = DefaultRootWindow(dpy);
    Atom actual_type;
    int actual_format;
    unsigned long nitems, bytes_after;
    unsigned char *prop = NULL;

    // EWMH path
    if (XGetWindowProperty(dpy, root, net_client_list_stacking, 0, 1024, False,
                           XA_WINDOW, &actual_type, &actual_format,
                           &nitems, &bytes_after, &prop) == Success && prop) {
        Window *wids = (Window *)prop;
        int n = (int)nitems;
        if (n > max) n = max;
        for (int i = 0; i < n; i++) out_wids[i] = wids[i];
        XFree(prop);
        return n;
    }

    // XQueryTree fallback
    Window root2, parent;
    Window *children = NULL;
    unsigned int nchildren = 0;
    if (XQueryTree(dpy, root, &root2, &parent, &children, &nchildren) == 0) {
        return -1;
    }
    int n = (int)nchildren;
    if (n > max) n = max;
    for (int i = 0; i < n; i++) out_wids[i] = children[i];
    if (children) XFree(children);
    return n;
}

// Get geometry in absolute screen coords by combining XGetWindowAttributes +
// XTranslateCoordinates(parent, root) — XGetWindowAttributes returns
// parent-relative coordinates, NOT screen coordinates.
// Source: https://tronche.com/gui/x/xlib/window-information/translate.html
static int getGeom(Display *dpy, Window w,
                   int *x, int *y, int *width, int *height) {
    XWindowAttributes attrs;
    if (!XGetWindowAttributes(dpy, w, &attrs)) return -1;
    if (attrs.map_state != IsViewable) return -2;  // skip unmapped
    Window child;
    if (!XTranslateCoordinates(dpy, w, DefaultRootWindow(dpy),
                               0, 0, x, y, &child)) return -3;
    *width = attrs.width;
    *height = attrs.height;
    return 0;
}
*/
import "C"
```

**Critical detail:** `XGetWindowAttributes` returns coordinates relative to the window's *parent*, not the root. To get screen coordinates, ALWAYS combine with `XTranslateCoordinates(dpy, w, root, 0, 0, &x, &y, &child)`. [CITED: https://tronche.com/gui/x/xlib/window-information/translate.html] Skipping this step is the #1 reason DIY X11 geometry capture is wrong.

### Pattern 2: Change-detect hash

**What:** Hash the canonical sorted-tuple form of the current window-set. Insert only when hash differs.

**Canonical form:**
```go
// Sort by (app_name, window_title, x, y, w, h) lexicographically,
// then SHA-256 the concatenation. windows_hash is hex of full digest.
sort.Slice(windows, func(i, j int) bool {
    a, b := windows[i], windows[j]
    if a.AppName != b.AppName { return a.AppName < b.AppName }
    if a.WindowTitle != b.WindowTitle { return a.WindowTitle < b.WindowTitle }
    if a.X != b.X { return a.X < b.X }
    if a.Y != b.Y { return a.Y < b.Y }
    if a.Width != b.Width { return a.Width < b.Width }
    return a.Height < b.Height
})
h := sha256.New()
for _, w := range windows {
    fmt.Fprintf(h, "%s\x00%s\x00%d\x00%d\x00%d\x00%d\x01",
        w.AppName, w.WindowTitle, w.X, w.Y, w.Width, w.Height)
}
hash := hex.EncodeToString(h.Sum(nil))
```

**Why these sort keys:** matches D-07 verbatim. The `\x00` separator + `\x01` row terminator avoids ambiguous boundaries (e.g., a title containing a digit followed by another window's app name).

**What to deliberately NOT include in the hash:**
- Window stacking order (z-order is unstable across focus changes — would create spurious deltas every focus change)
- Window IDs (X11 IDs are non-stable across remap)
- Cursor position, focused-flag, decoration state

### Pattern 3: Lock-gated capture

The existing `screenlock_linux.go` is **push-only** — it sends `StateChange` events on a channel. There is NO `IsLocked()` query method. The capturer must subscribe and maintain its own boolean:

```go
type Capturer struct {
    locked atomic.Bool   // updated from screenlock events
    // ...
}

// in Start:
go func() {
    for ev := range screenLock.Events() {
        c.locked.Store(ev.State == presence.Away)
    }
}()

// in tick:
if c.locked.Load() {
    return  // skip per D-08
}
```

**Important:** the screenlock listener may not be available (no `gdbus`, headless, etc.). The existing code logs a Warn and continues without it. For Phase 7, the capturer must treat "no screenlock signal available" as "always unlocked" — better to capture too much than to never capture.

### Pattern 4: Sync goroutine (separate from existing sync.Queue)

The existing `sync.Queue` cannot be reused — its `loadSubmissionEntries` is hard-coded to `submission_events JOIN focus_events`. Three options:

| Option | Description | Tradeoff | Recommendation |
|--------|-------------|----------|----------------|
| A. New goroutine in `internal/client/layout/sync.go` | Mirrors `sync.Queue` pattern: ticker, backoff schedule, status column on snapshots table | Clean, no churn in existing code; ~150 LOC duplication of retry logic | **RECOMMENDED for v1** |
| B. Refactor `sync.Queue` to be entity-agnostic (interface-based) | Define `Syncable` interface with `Load() ([]Payload, error)`, `MarkSent(id)`, etc. | Architecturally cleaner; touches working Phase 5/6 code; bigger blast radius | Defer; do this when Phase 7 + a third sync target both exist (rule of three) |
| C. Single big `POST /api/v1/sync` endpoint that takes mixed payload | Server demultiplexes per type | Wire-format coupling; one bad payload type fails all; doesn't match REST conventions | Reject |

Choose **A**. The duplicated retry logic is acceptable; revisit refactor when a third sync target appears (Chesterton: do not abstract before three concrete examples).

### Anti-Patterns to Avoid

- **Reusing `sync.Queue.processOne` for snapshots.** It JOINs focus_events; calling it on layout rows fails or returns garbage. Build a parallel goroutine.
- **Storing `XGetWindowAttributes` coords directly.** They are parent-relative. ALWAYS translate to root via `XTranslateCoordinates`.
- **Hashing X11 Window IDs.** IDs are unstable across remap and meaningless for change-detect.
- **Including z-order in the hash.** Z-order changes on every focus change → would defeat change-detect.
- **`SELECT * FROM layout_snapshots WHERE captured_at = $1`.** Snapshots are sparse (no row written when nothing changed). Use `WHERE captured_at <= $1 ORDER BY captured_at DESC LIMIT 1`.
- **Capturing window contents / screenshots.** Forbidden by D-16 + the project's core trust property (PROJECT.md). Privacy is a one-way door.
- **Running pg_cron-style downsampling without checking pg_cron is installed.** Stock Postgres in `deploy/docker-compose.yml` is `postgres:16-alpine` — no extensions beyond `pgcrypto`.
- **Auto-rerunning migrations on server start.** No in-process migration runner exists. Phase 7 must NOT introduce one (that's Phase 8 territory). Append to `deploy/initdb/001_schema.sql` AND ship a numbered `migrations/004_layout_snapshots.up.sql` for manual psql application.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| X11 client list / stacking | Custom protocol parsing | `_NET_CLIENT_LIST_STACKING` via `XGetWindowProperty` (EWMH); fall back to `XQueryTree(root)` | EWMH is the standardized API; `XQueryTree` is the universal fallback. [CITED: tronche.com Xlib manual] |
| Window absolute coords | Manually walk parent chain | `XTranslateCoordinates(w, root)` | Single library call; X server does the math |
| UTC timestamp formatting | Custom format strings | `time.Now().UTC().Format(time.RFC3339)` (existing convention) | Matches CONSTRAINT-timezone-convention; everything else in the codebase uses RFC3339 strings in SQLite, `TIMESTAMPTZ` in Postgres |
| Retry/backoff for sync | New scheduler | Mirror `internal/client/sync/queue.go` schedule `[1m, 5m, 15m, 1h, hourly]` | Already exists, already tested, matches CONSTRAINT-error-handling |
| HTTP API-key auth | Add new auth path | Reuse `auth.APIKeyMiddleware` in router | Already wraps `/devices` and `/timesheets`; layout endpoint mounts under same group |
| JSON marshalling of windows array | Custom serialization | `encoding/json` → pgx native JSONB | pgx handles `[]byte` JSON marshalling for JSONB columns |
| SHA-256 hex digest | Custom hash | `crypto/sha256` + `encoding/hex` stdlib | Standard, stable across builds |

**Key insight:** Every primitive Phase 7 needs already exists in the codebase or stdlib. The risk is NOT in choosing the right library — it's in **integration** (especially the sync architecture coupling and the missing migration runner).

## Runtime State Inventory

> Phase 7 is a greenfield additive feature, not a rename/refactor. This section is included for completeness because the phase touches client SQLite + server Postgres schemas, and stale state could cause first-startup confusion.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None — `layout_snapshots` table doesn't exist yet on either side | Create via migrations |
| Live service config | None — no n8n, no Datadog, no Tailscale tags reference layout snapshots | None |
| OS-registered state | None — no scheduled tasks, no launchd plists, no pm2 processes for layout | None |
| Secrets/env vars | None new — layout sync uses the existing `apiKey` baked into the binary | None |
| Build artifacts | The CGo build constraint matters: production binaries cross-compile with `CGO_ENABLED=0` and will get the X11 stub (`ErrUnsupported`). Same gap exists today for focus tracking; layout enum inherits it. | Document in stub error message; daemon must log Warn and continue (already the existing pattern in `tracker_linux_nocgo.go`) |
| **Existing client SQLite migration model** | Inline `CREATE TABLE IF NOT EXISTS` blob in `internal/client/store/store.go` `migrate()` — no version table, no numbered files | Append `CREATE TABLE IF NOT EXISTS layout_snapshots (...)` to the same blob |
| **Existing server Postgres migration model** | NO in-process runner. Schema applied via Postgres `/docker-entrypoint-initdb.d/` (only on empty data volume) + numbered files in `migrations/` that are NEVER auto-executed. | (a) APPEND `layout_snapshots` block to `deploy/initdb/001_schema.sql` for fresh installs. (b) SHIP `migrations/004_layout_snapshots.up.sql` for manual `psql -f` application against existing DBs. (c) DOCUMENT that Phase 8 should solve the runner problem; Phase 7 must not. |

## Common Pitfalls

### Pitfall 1: `XGetWindowAttributes` coords appear absolute but aren't
**What goes wrong:** Plotting captured x/y on a screen-shaped grid produces misaligned windows.
**Why it happens:** `XWindowAttributes.x/y` is relative to the window's *parent* (often a window-manager frame, not the root).
**How to avoid:** Always combine with `XTranslateCoordinates(dpy, w, DefaultRootWindow(dpy), 0, 0, &x, &y, &child)`. [CITED: tronche.com]
**Warning signs:** Tiny offsets (16, 32 px) appearing on every window — that's the WM decoration size leaking through.

### Pitfall 2: `_NET_CLIENT_LIST_STACKING` order is workspace-grouped on GNOME
**What goes wrong:** Windows from inactive workspaces appear in the list, stacking order is wrong across workspaces.
**Why it happens:** GNOME 3+ groups by workspace. Chromium removed reliance on this property for that reason. [CITED: github.com/chromium/chromium commit 58b7467]
**How to avoid:** For v1, treat `_NET_CLIENT_LIST_STACKING` as "list of windows that exist," NOT "list in true stacking order." Fall back to `XQueryTree(root)` for stacking order if you ever need it (we don't in v1 — z-order is excluded from the schema).
**Warning signs:** GNOME users see windows from minimized workspaces in their snapshots — actually correct behavior for "what was open" but worth noting.

### Pitfall 3: Capture loop runs while screen is locked
**What goes wrong:** Records the lock screen as "the only window," polluting history with junk rows.
**Why it happens:** Forgot to gate on screenlock state, OR forgot that `screenlock_linux.go` events channel must be subscribed-to (push-only, no query API).
**How to avoid:** Subscribe to `presence.ScreenLockMonitor.Events()` and maintain an `atomic.Bool` for current state. Tick aborts early when locked.
**Warning signs:** First snapshot after a lock event shows only `gnome-screensaver` or `xscreensaver` as the open window.

### Pitfall 4: pg_cron assumed available
**What goes wrong:** Downsampling SQL is written but never runs because the extension isn't installed.
**Why it happens:** Stock `postgres:16-alpine` does not include pg_cron. The existing `001_schema.sql` only enables `pgcrypto`.
**How to avoid:** Implement downsampling as a Go goroutine in the trasker-server `main.go`, using `time.Ticker(1 * time.Hour)` to call SQL functions that downsample + prune. NO extensions needed.
**Warning signs:** `extension "pg_cron" is not available` error on first deploy.

### Pitfall 5: Adding a migration but expecting it to auto-apply
**What goes wrong:** Server starts cleanly, layout endpoints return `relation "layout_snapshots" does not exist`.
**Why it happens:** No in-process migration runner exists. The `migrations/` directory is informational only — Postgres init scripts only run on EMPTY data volumes.
**How to avoid:** (a) Append schema to `deploy/initdb/001_schema.sql` for new deploys; (b) provide `migrations/004_layout_snapshots.up.sql` AND clear documentation that operators must apply it via `psql -f`; (c) consider adding a startup log warning if the table is missing.
**Warning signs:** Tests pass locally with a fresh DB; first deployment to existing hal Postgres fails with relation-does-not-exist.

### Pitfall 6: Sync goroutine and submit-flow goroutine race on the same connection
**What goes wrong:** SQLite `database is locked` errors at peak load.
**Why it happens:** `modernc.org/sqlite` defaults to a single connection unless tuned; both pipelines compete for write locks.
**How to avoid:** SQLite is opened with WAL mode (`PRAGMA journal_mode=WAL`) per `store.go` — concurrent readers are fine, but only one writer at a time. The 60s capture cadence is so low that contention is negligible. Document, don't optimize. If contention does appear, the standard fix is to set `db.SetMaxOpenConns(1)` and serialize through it.
**Warning signs:** Intermittent `SQLITE_BUSY` errors in logs.

### Pitfall 7: JSONB write amplification
**What goes wrong:** Disk usage and WAL volume balloon when downsampling rewrites a row's JSONB column.
**Why it happens:** `jsonb_set` rewrites the entire JSONB value, not just the changed field. [CITED: heap.io blog]
**How to avoid:** Downsampling MUST be implemented as `INSERT new row + DELETE old rows`, NOT as `UPDATE rows SET windows = jsonb_set(...)`. Snapshots are immutable per row; downsampling chooses a representative row to keep and deletes the rest. No JSONB mutation occurs.
**Warning signs:** WAL volume per downsampling cycle far exceeds size of "10-min picked row" — would indicate accidental UPDATEs.

## Code Examples

### List windows on a Linux X11 display (cgo)

```go
//go:build linux && cgo

package layout

// (cgo block as in Pattern 1 above)

type X11Enumerator struct {
    display              *C.Display
    netClientListStacking C.Atom
    netWMName             C.Atom
    utf8String            C.Atom
}

func NewX11Enumerator() (*X11Enumerator, error) {
    dpy := C.XOpenDisplay(nil)
    if dpy == nil {
        return nil, fmt.Errorf("layout: cannot open X11 display (is DISPLAY set?)")
    }
    return &X11Enumerator{
        display:               dpy,
        netClientListStacking: C.XInternAtom(dpy, C.CString("_NET_CLIENT_LIST_STACKING"), C.True),
        netWMName:             C.XInternAtom(dpy, C.CString("_NET_WM_NAME"), C.True),
        utf8String:            C.XInternAtom(dpy, C.CString("UTF8_STRING"), C.True),
    }, nil
}

func (e *X11Enumerator) Enumerate() ([]Window, error) {
    const maxWindows = 256
    var wids [maxWindows]C.Window
    n := int(C.enumWindows(e.display, &wids[0], maxWindows, e.netClientListStacking))
    if n < 0 {
        return nil, fmt.Errorf("layout: window enumeration failed")
    }

    out := make([]Window, 0, n)
    for i := 0; i < n; i++ {
        // 1. class hint → app_name
        // 2. _NET_WM_NAME (UTF8) or WM_NAME → window_title
        // 3. getGeom → x, y, width, height (with XTranslateCoordinates inside)
        // 4. skip on map_state != IsViewable (returned as -2 from getGeom)
        // ... see existing tracker_linux_x11.go for the property-fetch boilerplate
        out = append(out, w)
    }
    return out, nil
}
```

### Postgres schema for `layout_snapshots`

```sql
-- migrations/004_layout_snapshots.up.sql
CREATE TABLE layout_snapshots (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id    UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    captured_at  TIMESTAMPTZ NOT NULL,
    windows      JSONB NOT NULL,
    windows_hash TEXT NOT NULL,
    tier         TEXT NOT NULL DEFAULT 'raw',  -- 'raw' | '10min' | '1hr'
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Most queries are "most recent ≤ T for device": (device_id, captured_at DESC) covers that
CREATE INDEX idx_layout_snapshots_device_captured
    ON layout_snapshots (device_id, captured_at DESC);

-- Downsampling jobs need to find rows by tier + age
CREATE INDEX idx_layout_snapshots_tier_captured
    ON layout_snapshots (tier, captured_at);

-- Hash duplicate-detection (rare; protects against client retry double-sends)
CREATE UNIQUE INDEX idx_layout_snapshots_dedup
    ON layout_snapshots (device_id, captured_at, windows_hash);
```

```sql
-- migrations/004_layout_snapshots.down.sql
DROP TABLE IF EXISTS layout_snapshots;
```

### SvelteKit `+page.ts` reading `?t=`

```typescript
// web/server-ui/src/routes/(app)/layout/+page.ts
// Source: https://svelte.dev/docs/kit/load — using url.searchParams pattern
import { api } from '$lib/api';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ url, fetch, parent }) => {
    await parent();  // guarantees auth/tokens available
    const tParam = url.searchParams.get('t');

    // Default device = first device for the user (UI selector can override)
    const devices = await api.devices.list();
    const deviceId = url.searchParams.get('device') ?? devices[0]?.id;

    if (tParam) {
        // Permalink mode: fetch single snapshot at-or-before t
        const snapshot = await api.layout.at(deviceId, tParam);
        return { mode: 'permalink', snapshot, deviceId };
    }

    // Default mode: today's timeline (list of captured_at)
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const timeline = await api.layout.timeline({
        device_id: deviceId,
        from: today.toISOString(),
        to: new Date().toISOString(),
    });
    return { mode: 'timeline', timeline, deviceId };
};
```

```typescript
// extension to web/server-ui/src/lib/api.ts
layout: {
    timeline(params: { device_id: string; from: string; to: string }):
        Promise<Array<{ id: string; captured_at: string; windows_count: number }>> {
        const q = new URLSearchParams(params).toString();
        return request('GET', `/layout-snapshots/timeline?${q}`);
    },
    at(device_id: string, t: string): Promise<LayoutSnapshot> {
        return request('GET', `/layout-snapshots?device_id=${device_id}&t=${encodeURIComponent(t)}`);
    },
},
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Polling X11 every 1Hz for all metadata | Change-detect at 60s — hash window-set, write only on delta | This phase | Reduces row count by ~10× during steady state; reconstruction queries use `≤ T` semantics |
| Hand-rolled cron in app code for cleanup | Goroutine-based ticker tied to server lifecycle | This phase | Avoids pg_cron extension dependency; goroutine restarts cleanly with the server |
| Storing per-window rows in a normalized child table | Single JSONB column per snapshot | This phase | 30-50× fewer rows; matches insert-only access pattern; safe because we never UPDATE the JSONB |
| `_NET_CLIENT_LIST_STACKING` for window order | `_NET_CLIENT_LIST_STACKING` for *enumeration*, ignore order | Discovered during research | GNOME 3+ misorders the property across workspaces; we don't need stacking order in v1 anyway |

**Deprecated/outdated (within trasker):**
- The original ROADMAP.md Phase 7 success criteria #1, #3, #4 mention `display index`, `z-order`, `Wayland/macOS/Windows enumeration`, and `Merged mode`. These are explicitly deferred per CONTEXT.md. The planner MUST flag this for ROADMAP.md amendment before plan-checker runs. (Already noted in `<canonical_refs>` of CONTEXT.md.)

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | hal's Postgres image is `postgres:16-alpine` (or another image without pg_cron) | Common Pitfalls #4; Alternatives Considered | LOW — even if pg_cron were available, in-process goroutine works just as well; if available we just don't use it |
| A2 | Operators will manually apply `migrations/004_layout_snapshots.up.sql` via `psql -f` for existing deployments | Runtime State Inventory; Pitfall 5 | MEDIUM — operator could miss the step. Mitigate with a startup log warning if the table is absent. Verify with Jaypaul during planning whether Phase 7 should ship before the deployed server exists (in which case fresh-install path via `initdb/001_schema.sql` is the only path that matters) |
| A3 | The cgo build tag matters less in dev because Jaypaul builds locally with cgo enabled; cross-compiled `CGO_ENABLED=0` binaries simply log "X11 layout enum unavailable" and run without it | Don't Hand-Roll; Pattern stubs | LOW — exact same pattern works today for focus tracking; users on cross-compiled binaries get a documented degradation |
| A4 | Aggregate snapshot row size at 60s cadence × 24h × 365d × 1 device fits comfortably under 1 GB even without compression. Estimate: ~100 windows × 200 bytes JSON × 1440/day × 365 = ~10 GB/year before downsampling, ~500 MB/year after the 7d→10min→1hr tier collapse. | (implicit in retention design) | LOW — if growth exceeds expectations, downsampling tier intervals can be tightened |
| A5 | `XGetClassHint` returns `res_name` rather than `res_class` for the user-meaningful app identifier (matches existing `tracker_linux_x11.go`) | Pattern 1 | LOW — convention already validated in production focus tracker code |
| A6 | The X11 root window's children from `XQueryTree` are top-level windows (not the WM's frame windows). Some WMs reparent — our enumeration should look one level down when needed. | Pattern 1; Pitfall 1 | MEDIUM — on reparenting WMs (most modern ones), `XQueryTree(root)` returns frame windows, not application windows. EWMH `_NET_CLIENT_LIST_STACKING` returns application windows. This is another reason to prefer EWMH and use XQueryTree only as last-resort fallback. Confirm with a smoke test against Jaypaul's daily-driver WM (likely GNOME or i3 — both are EWMH-compliant). |
| A7 | The existing screenlock listener's "Away" state corresponds to "screen locked" for our purposes. (Currently it can also fire on the deadman's switch transitioning to "Paused" — that's a separate state.) | Pattern 3 | LOW — D-08 explicitly says "skip when screen is locked" not "skip when AFK" — we want exactly the `Away` state from the screenlock channel, not the deadman's `Paused` state |

## Open Questions

1. **Should layout sync happen continuously or be batched?**
   - What we know: Snapshots are tiny (single row JSONB); the existing submit pipeline batches per-submit.
   - What's unclear: Whether the planner should mirror the per-tick "send immediately" pattern or batch every N minutes. Continuous = freshest data, more HTTP overhead. Batched = bulkier requests, slightly stale.
   - Recommendation: Batched every 5 minutes (matches existing `sync.Queue` cadence). Offline buffer = up to 7 days × 24h × 60min / 60s = up to 10,080 rows worst case, but change-detect keeps it far smaller.

2. **What happens when a user has 0 layout_snapshots on the server (fresh install)?**
   - What we know: SvelteKit `+page.ts` will fetch and get an empty array.
   - What's unclear: UX preference — show "No snapshots yet, your daemon hasn't reported one" vs blank list.
   - Recommendation: Empty-state message, pointing at the device's last_seen_at to differentiate "device offline" from "device online but no changes captured today."

3. **Do we need a unique constraint on `(device_id, captured_at)` to dedupe retries?**
   - What we know: Client retry could re-POST a snapshot if the server's response is lost. Hash-based dedup is in the schema above.
   - What's unclear: Should the server return 409 Conflict on duplicate or silently absorb?
   - Recommendation: `INSERT ... ON CONFLICT (device_id, captured_at, windows_hash) DO NOTHING RETURNING id`. Returns either the new id or no rows; the handler treats both as 201 from the client's perspective.

4. **Should the daemon WARN-log every X11 enum failure or rate-limit?**
   - What we know: D-09 says fail-soft + WARN.
   - What's unclear: A misconfigured DISPLAY env could spam WARN every 60s indefinitely.
   - Recommendation: Log first failure, then once per hour after that with a counter. Standard "first + repeat" pattern.

5. **Should ROADMAP.md be amended before or during Phase 7 planning?**
   - What we know: CONTEXT.md `<canonical_refs>` flags this explicitly.
   - What's unclear: Plan-checker may complain if PLAN.md targets a scope smaller than ROADMAP.md success criteria.
   - Recommendation: Planner edits ROADMAP.md as Wave 0 of the first plan, OR raises a blocker for Jaypaul to amend before planning continues. Pick the cheaper path.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| libX11-dev (build-time) | cgo `-lX11` | ✓ (Jaypaul's daily-driver) | 1.x | None for v1 — no-cgo build returns ErrUnsupported (mirrors existing focus tracker) |
| libX11 runtime | dev/runtime X11 enum | ✓ (any X11 desktop) | 1.x | None |
| running X11 session (`DISPLAY` set) | layout enum | ✓ (Linux daily driver) | — | Daemon logs Warn and runs without layout capture |
| `gdbus` CLI | screenlock signal | ✓ (already required by Phase 5) | — | None — locked-state gating is best-effort; missing gdbus = always-unlocked assumption |
| Postgres 16 (or compatible) on hal | server store | ✓ (locked by CONSTRAINT-tech-stack) | 16.x | None |
| pg_cron extension | (would-be) downsampling scheduler | ✗ | — | Use Go goroutine with `time.Ticker` instead (CONFIRMED via stack analysis) |
| In-process Postgres migration runner | apply `migrations/004_layout_snapshots.up.sql` | ✗ | — | Manual `psql -f` against deployed instance + append to `deploy/initdb/001_schema.sql` for fresh installs |
| Node.js + npm (build-time) | SvelteKit build | ✓ (already required by Phase 4/6) | per package.json | None |

**Missing dependencies with no fallback:**
- None blocking Phase 7

**Missing dependencies with fallback:**
- pg_cron → use Go ticker goroutine (preferred regardless)
- In-process migration runner → manual `psql -f` + initdb append (Phase 8 should solve)

## Validation Architecture

> Nyquist validation is enabled (no `.planning/config.json`, default = enabled).

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `testify/assert,require` (already in tree) |
| Config file | None — `go.mod` driven |
| Quick run command | `go test ./internal/client/layout/... ./internal/server/store/... ./internal/server/api/... -count=1` |
| Full suite command | `go test ./... -count=1 -tags=` (omit `x11_integration` tag for CI) |
| X11 integration command | `DISPLAY=:0 go test -tags x11_integration ./internal/client/layout/...` |
| SvelteKit unit/build | `cd web/server-ui && npm run build` (existing `vite build`) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| REQ-layout-snapshots | Hash determinism: same windows → same hash | unit | `go test ./internal/client/layout -run TestHashWindows -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Hash sensitivity: any field change → different hash | unit | `go test ./internal/client/layout -run TestHashChange -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Capturer skips writes when hash unchanged | unit | `go test ./internal/client/layout -run TestCapturer_SkipsUnchanged -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Capturer skips entire tick when locked | unit | `go test ./internal/client/layout -run TestCapturer_SkipsWhenLocked -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Capturer fails-soft on enum error (logs WARN, no row) | unit | `go test ./internal/client/layout -run TestCapturer_FailSoft -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Local SQLite insert + retrieval roundtrip | unit | `go test ./internal/client/layout -run TestStore_RoundTrip -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Client 7-day prune | unit | `go test ./internal/client/layout -run TestStore_Prune7Days -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Sync goroutine retries on transient failure with backoff | unit | `go test ./internal/client/layout -run TestSync_RetryBackoff -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Sync goroutine handles permanent 401/403 | unit | `go test ./internal/client/layout -run TestSync_KeyExpired -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Server `POST /api/v1/layout-snapshots` accepts JSONB payload | unit | `go test ./internal/server/api -run TestLayoutIngest -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Server dedup on `(device_id, captured_at, windows_hash)` | unit | `go test ./internal/server/store -run TestLayoutSnapshots_Dedup -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Server `GET ...?t=ISO` returns most-recent ≤ T | unit | `go test ./internal/server/api -run TestLayoutAt -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Server `GET .../timeline` returns ascending timestamps | unit | `go test ./internal/server/api -run TestLayoutTimeline -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Downsampling 7d→10min collapses correctly | unit | `go test ./internal/server/store -run TestDownsample_10min -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Downsampling 37d→1hr collapses correctly | unit | `go test ./internal/server/store -run TestDownsample_1hr -count=1` | ❌ Wave 0 |
| REQ-layout-snapshots | Manual: X11 capture against real GNOME/i3 session | manual | (smoke test on Jaypaul's machine) | n/a — requires real X11 |
| REQ-layout-snapshots | Manual: SvelteKit `/layout` view renders timestamp list + expand | manual | `cd web/server-ui && npm run dev`; navigate to `/layout` | n/a — requires running server + dev UI |

### Sampling Rate

- **Per task commit:** `go test ./internal/client/layout/... ./internal/server/store/... ./internal/server/api/... -count=1` (target < 30s)
- **Per wave merge:** `go test ./... -count=1 && cd web/server-ui && npm run build`
- **Phase gate:** Full suite green + X11 integration smoke (`DISPLAY=:0 go test -tags x11_integration ./internal/client/layout/...`) before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/client/layout/layout.go` — Window struct + HashWindows function
- [ ] `internal/client/layout/layout_test.go` — hash unit tests (no X11 needed)
- [ ] `internal/client/layout/capturer.go` — capture loop with injected Enumerator interface
- [ ] `internal/client/layout/capturer_test.go` — fake Enumerator + mock screenlock channel
- [ ] `internal/client/layout/store.go` + test — SQLite roundtrip + prune
- [ ] `internal/client/layout/sync.go` + test — `httptest.NewServer`-based, mirrors `internal/client/sync/client_test.go` style
- [ ] `internal/server/store/layout_snapshots.go` + test — pgx accessor; tests use `testhelper_test.go` pgx pool helper
- [ ] `internal/server/api/layout_handlers.go` + test — handler tests using `testhelper_test.go`
- [ ] `migrations/004_layout_snapshots.up.sql` + `.down.sql`
- [ ] `deploy/initdb/001_schema.sql` — append `layout_snapshots` block
- [ ] `web/server-ui/src/routes/(app)/layout/+page.ts` + `+page.svelte`
- [ ] `web/server-ui/src/lib/types.ts` — add `LayoutSnapshot`, `LayoutWindow` types
- [ ] `web/server-ui/src/lib/api.ts` — add `layout: { timeline, at }` namespace
- [ ] X11 integration test guarded by `//go:build linux && cgo && x11_integration` tag (mirrors `tracker_linux_x11_test.go`)

## Project Constraints (from CLAUDE.md and constraints.md)

The project's reasoning protocol applies (`~/.claude/CLAUDE.md`):

- **Explicit Reasoning Protocol** — every action with possible failure must declare DOING/EXPECT/IF YES/IF NO before tool calls.
- **Failure handling** — when any tool call fails, agents must STOP and ask Jaypaul before proceeding.
- **Chesterton's Fence** — do not refactor `internal/client/sync/` to be entity-agnostic without three concrete examples (Phase 7 layout + Phase 5 timesheet + a hypothetical third → not yet justified).
- **Three examples before abstracting** — duplicate the retry/backoff schedule rather than extracting it.
- **No silent fallbacks** — if X11 enum fails, log WARN and skip the row; do NOT write a row with empty windows.
- **Branch safety** — work on `feature/phase-7-layout-snapshots` (or similar), not `main`.
- **Atomicity** — atoms <50 LOC, molecules <150 LOC, organisms <400 LOC. The capturer + sync goroutine are organism-scale; split into atoms (hash function, enum interface, store accessor) and a thin organism that wires them.
- **Privacy is a one-way door** — D-16 captures only `app_name + window_title + geometry`. Any task that would extend this (PID, cmdline, screenshot) must be REJECTED at plan-checker time.

From `constraints.md`:

- **CONSTRAINT-tech-stack** — modernc.org/sqlite (no cgo for client store), pgx for server. Already aligned.
- **CONSTRAINT-timezone-convention** — UTC RFC3339 in client SQLite, TIMESTAMPTZ on server. Already aligned via Pattern 2.
- **CONSTRAINT-error-handling** — backoff schedule `[1m, 5m, 15m, 1h, hourly]`. Mirrored exactly in layout sync goroutine.
- **CONSTRAINT-security** — API keys bcrypt-hashed server-side, plaintext only in client binary. Layout sync uses the same `Authorization: Bearer <apiKey>` header as `sync.Client.post`.
- **CONSTRAINT-out-of-scope-v1** — no screenshot or window-content monitoring. Reaffirmed.

## Sources

### Primary (HIGH confidence)
- Codebase: `/home/jaypaulb/Projects/gh/trasker/` — `tracker_linux_x11.go`, `screenlock_linux.go`, `sync/queue.go`, `sync/client.go`, `store/store.go`, `migrations/001_init.up.sql`, `deploy/Dockerfile.server`, `cmd/trasker-client/main.go`, `web/server-ui/src/routes/(app)/+layout.svelte`, `web/server-ui/src/lib/api.ts`. (All read in this session.)
- `.planning/phases/07-layout-snapshots/07-CONTEXT.md` — locked decisions D-01 through D-16 (authoritative).
- `.planning/intel/constraints.md` — CONSTRAINT-tech-stack, CONSTRAINT-error-handling, CONSTRAINT-security, CONSTRAINT-timezone-convention.
- [SvelteKit Routing — official docs](https://svelte.dev/docs/kit/routing)
- [SvelteKit Loading data — official docs](https://svelte.dev/docs/kit/load) — confirms `url.searchParams` access in `+page.ts` `load`.
- [Xlib XTranslateCoordinates — tronche.com](https://tronche.com/gui/x/xlib/window-information/translate.html) — confirms parent-relative-to-root coordinate translation pattern.
- [Xlib XGetWindowAttributes — x.org R7.7](https://www.x.org/releases/X11R7.7/doc/man/man3/XGetWindowAttributes.3.xhtml) — confirms `attrs.x/y` is parent-relative.
- [Xlib XQueryTree — tronche.com](https://tronche.com/gui/x/xlib/window-information/XQueryTree.html) — confirms children listed in stacking order, bottom to top.
- [Chromium commit 58b7467](https://github.com/chromium/chromium/commit/58b74676f3ddd57add8dc3132755a5fe9eedd456) — evidence that `_NET_CLIENT_LIST_STACKING` is unreliable on GNOME 3+.

### Secondary (MEDIUM confidence)
- [When To Avoid JSONB In A PostgreSQL Schema — Heap blog](https://www.heap.io/blog/when-to-avoid-jsonb-in-a-postgresql-schema) — JSONB write amplification considerations; confirmed our use case (insert-only, never UPDATE) is safe.
- [No HOT updates on JSONB — DEV.to](https://dev.to/mongodb/no-hot-updates-on-jsonb-13k7) — write amplification mechanism details.
- [Sequin: Time-based retention strategies in Postgres](https://blog.sequinstream.com/time-based-retention-strategies-in-postgres/) — partition-based retention as alternative to in-app pruning; not adopted for v1 due to volume not justifying partitioning.
- [pg_cron — Citus Data on GitHub](https://github.com/citusdata/pg_cron) — extension capabilities and the requirement that it be installed; we choose not to depend on it.

### Tertiary (LOW confidence — needs validation if adopted)
- Any specific row-size estimate in A4 — not measured against real data.
- WM-specific behavior on `XQueryTree` (reparenting): A6 needs a smoke test.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every dependency is already in `go.mod` or stdlib; X11 cgo pattern proven by `tracker_linux_x11.go`.
- Architecture: HIGH (capturer + store) / MEDIUM (sync goroutine — three architectural options exist, recommendation is opinionated).
- Schema: HIGH — JSONB is the right choice for this insert-only access pattern; verified via primary sources.
- Pitfalls: HIGH — every pitfall listed is either grounded in codebase reality (existing migration model, sync coupling) or in cited X11 documentation.
- Downsampling impl: MEDIUM — Go goroutine recommended but in-process scheduling is the more opinionated of multiple valid options.
- Dashboard: MEDIUM — SvelteKit `+page.ts` pattern is standard but the *exact* permalink/empty-state/device-selector UX has not been wireframed.

**Research date:** 2026-05-07
**Valid until:** 2026-06-07 (30 days — stack is stable, X11 / SvelteKit / pgx are mature; revisit if Phase 8 lands a migration runner before Phase 7 implementation begins)
