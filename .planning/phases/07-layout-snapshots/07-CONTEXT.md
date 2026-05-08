# Phase 7: Layout Snapshots - Context

**Gathered:** 2026-05-07
**Status:** Ready for planning

<domain>
## Phase Boundary

Capture the set of open windows + their geometry on the client whenever the set changes (evaluated every 60s), persist locally to SQLite, sync to the trasker server using the existing API-key auth path, and expose a minimal "Layout History" view in the server dashboard so the user can browse "what was open at time T on this device" via a permalink.

**v1 explicitly excludes:** Wayland support, macOS/Windows clients, multi-monitor coordinates, multi-device merge UI, full-text search, screenshot or window-content capture.

</domain>

<decisions>
## Implementation Decisions

### Retention policy
- **D-01:** Client SQLite keeps raw 60s-evaluation snapshots for **7 days**, then deletes. Server is the source of truth beyond that window.
- **D-02:** Server PostgreSQL retention is tiered:
  - 0–7 days: raw 60s-evaluation cadence
  - 7–37 days: downsampled to 10-minute representative snapshots
  - 37+ days: downsampled to 1-hour representative snapshots, kept indefinitely
- **D-03:** No per-app retention overrides in v1. Single global policy.

### Cross-platform scope (v1)
- **D-04:** Linux X11 only. Wayland deferred. (Jaypaul's daily driver is X11; GNOME Wayland has no portable window-enum API.)
- **D-05:** macOS and Windows clients deferred to v2. Existing tracker code on those platforms is untouched in this phase.
- **D-06:** No multi-monitor handling v1. Capture only window-relative absolute X/Y coordinates from the X11 root window. No display-index field on the schema.

### Snapshot trigger
- **D-07:** Change-detect cadence: hash the current window-set (sorted `(app_name, window_title, x, y, w, h)` tuples) every 60s. Write a new `layout_snapshots` row only when the hash differs from the previous tick. Idle periods produce zero rows; reconstruction queries use "most recent snapshot ≤ T".
- **D-08:** Skip the tick entirely when the screen is locked. Reuse existing `internal/client/presence/screenlock_linux.go` lock signal.
- **D-09:** Fail-soft on enum failure: log a WARN with the X11 error, skip writing a row, retry on the next 60s tick. Do not poison the daemon or write empty error rows.

### Dashboard UX
- **D-10:** Primary view is a plain list of timestamps with a click-to-expand window list per row. No timeline scrubber, no geometry mosaic, no calendar picker.
- **D-11:** Permalink-driven instant selection. Default view = today's snapshot list for the current device. Past instants reachable only via `?t=<ISO-8601>` query param. No natural-language picker.
- **D-12:** Per-device view only. Multi-device merge UI deferred to a future phase (probably triggered when a second device starts syncing in Phase 9 dogfooding).
- **D-13:** No search v1. Add a window-title search bar later if dogfooding shows demand.

### Schema shape (locked from constraints + discussion)
- **D-14:** New `layout_snapshots` table independent of `focus_events`. Submitted-timesheet immutability invariant is preserved — layout snapshots have no relationship to focus_events or tags.
- **D-15:** Snapshot row schema (minimum):
  - `id` (uuid)
  - `device_id` (uuid, FK)
  - `captured_at` (timestamptz, UTC)
  - `windows` (JSONB array of `{app_name, window_title, x, y, w, h}` — NO display index, NO z-order, NO PID, NO cmdline)
  - `windows_hash` (text, for change detection on the client)

### Privacy
- **D-16:** Capture only the same `app_name + window_title + geometry` shape the existing focus tracker already stores. NEVER window contents, screenshots, focus content beyond title, PID, or cmdline. Already locked by Phase 7 success criterion #5; reaffirmed here.

### Claude's Discretion
- Wire format for sync payload (single `POST /api/v1/sync` call vs new `/api/v1/layout-snapshots` endpoint) — planner picks based on existing sync architecture.
- Server-side downsampling implementation — cron job, pg_cron extension, or in-process scheduler. Planner picks based on existing scheduling patterns.
- SvelteKit route shape for the Layout History list view — new `/layout` route or panel inside existing `/timeline`. Planner picks based on existing route conventions.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing X11 enumeration pattern
- `internal/client/tracker/tracker_linux_x11.go` — current 1Hz active-window polling via XGetInputFocus + XGetClassHint + _NET_WM_NAME. Layout enum extends this with XQueryTree from the root window + XGetWindowAttributes for geometry.
- `internal/client/tracker/tracker.go` — Tracker interface contract.

### Sync + auth path to reuse
- `internal/client/sync/` — existing outbox + sync goroutine. Layout snapshots reuse this pipeline.
- `internal/server/api/router.go` — API-key auth middleware mount point.

### Presence / screenlock signal to reuse
- `internal/client/presence/screenlock_linux.go` — locked-state detector used by D-08.

### Storage patterns
- `internal/client/store/` — sqlite migrations + accessors pattern.
- `internal/server/store/` — Postgres pgx accessors + migrations under `migrations/`.

### Specs and ingest intel
- `.planning/intel/SYNTHESIS.md` — ingested doc summary (note: no SPEC for layout-snapshots; this is greenfield within the project).
- `.planning/intel/constraints.md` — protocol/schema constraints from existing SPECs (focus tracker schema, API auth patterns).
- `docs/superpowers/specs/2026-03-26-production-deployment-design.md` — server deployment context (relevant for Phase 8 not Phase 7, but planner may reference for sync endpoint shape).

### Roadmap delta to file with planner
- `.planning/ROADMAP.md` Phase 7 success criteria #1, #3, #4 must be amended to match v1 scope captured here. Planner should call this out so the user can edit ROADMAP.md before plan-checker runs.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `tracker_linux_x11.go` cgo block: XOpenDisplay + atom interning + XGetWindowProperty fallback chain — copy the cgo skeleton, replace XGetInputFocus with XQueryTree(root) + per-window XGetClassHint/XGetWindowAttributes.
- Sync outbox pattern in `internal/client/sync/`: append-only queue with retry. Layout-snapshot rows enqueue here once written to local sqlite.
- `internal/client/presence/screenlock_linux.go`: provides locked/unlocked state via DBus screensaver inhibitor. Daemon subscribes and gates the layout tick.
- `internal/server/store/` migration runner: same pattern used in Phases 1-6. Add `migrations/NNN_layout_snapshots.up.sql` + `.down.sql`.

### Established Patterns
- Per-platform build tags: `tracker_linux_x11.go` is `//go:build linux && cgo`. New file `layout_linux_x11.go` follows the same tag. macOS/Windows files come later as `//go:build darwin` / `//go:build windows` no-op stubs that satisfy the interface.
- WAL-mode sqlite + UTC timestamps everywhere. Layout schema follows the same convention.
- Tests at `*_test.go` next to implementation. X11 tests use a fake display via Xvfb pattern already established in `tracker_linux_x11_test.go`.

### Integration Points
- New `internal/client/layout/` package, owns capture loop + change-detect + sqlite writes. Daemon main wires it next to the existing tracker.
- New `internal/server/store/layout_snapshots.go` accessors. Router mounts a `GET /api/v1/layout-snapshots` endpoint (or extends existing endpoint — planner's call).
- New `web/server-ui/.../routes/layout/` SvelteKit route. (Or panel within `/timeline` — planner's call.)

</code_context>

<specifics>
## Specific Ideas

- The original motivation: Hungarian-countryside power cuts. Jaypaul leaves windows open intentionally as memory aids; needs to recover "what was I working on" after a hard power loss. Phase 7 must work for this use case the moment client+server are both up — even before macOS/Windows/Wayland support arrive.
- Keep the wire format JSONB-style so Phase 8 deployment doesn't have to coordinate a new schema migration cadence.

</specifics>

<deferred>
## Deferred Ideas

- **Wayland support** (wlr-foreign-toplevel-management for Sway/Hyprland; GNOME Shell extension for GNOME) — own phase, depends on Phase 7 shape proving out.
- **macOS layout enum** via `CGWindowListCopyWindowInfo` — own phase or v2.
- **Windows layout enum** via `EnumWindows` + per-monitor DPI — own phase or v2.
- **Multi-monitor display index** + dashboard rendering of monitor layout.
- **Multi-device merge view** — likely triggered when Phase 9 dogfooding actually puts a second device on hal.
- **Window-title full-text search** in the dashboard.
- **Geometry mosaic visualization** ("what did the desktop literally look like").
- **Per-app retention overrides** / opt-out for sensitive apps (Bitwarden, etc.).
- **Adaptive cadence** (snap on focus-change events too) — current change-detect-every-60s already covers most use cases.

</deferred>

---

*Phase: 07-layout-snapshots*
*Context gathered: 2026-05-07*
