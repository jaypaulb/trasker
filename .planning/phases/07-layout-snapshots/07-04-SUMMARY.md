---
phase: 07-layout-snapshots
plan: 04
subsystem: web/server-ui
tags:
  - phase-7
  - frontend
  - svelte
  - layout-snapshots
  - dark-mode
dependency-graph:
  requires:
    - 07-03 (server endpoints /api/v1/layout-snapshots and /timeline)
  provides:
    - "/layout route in dashboard"
    - "api.layout.{timeline,at} client namespace"
    - "LayoutSnapshot/LayoutWindow/LayoutTimelineEntry TS types"
  affects:
    - web/server-ui/src/lib/components/Sidebar.svelte (nav addition)
tech-stack:
  added: []
  patterns:
    - "Svelte 5 runes ($props, $state, $derived) — matches existing dashboard convention"
    - "SvelteKit +page.ts loader (first in server-ui tree) with discriminated-union LoadResult"
    - "Tailwind v4 utility classes with full dark: pairings (mirrored from /devices)"
key-files:
  created:
    - web/server-ui/src/routes/(app)/layout/+page.ts
    - web/server-ui/src/routes/(app)/layout/+page.svelte
    - web/server-ui/src/lib/components/LayoutSnapshotRow.svelte
    - web/server-ui/src/lib/components/LayoutSnapshotWindowList.svelte
  modified:
    - web/server-ui/src/lib/types.ts
    - web/server-ui/src/lib/api.ts
    - web/server-ui/src/lib/components/Sidebar.svelte
decisions:
  - "Exported LayoutPageData type from +page.ts and imported into +page.svelte to bypass SvelteKit's auto-generated PageData widening (OptionalUnion flattens discriminated unions, breaking narrowing)."
  - "Adopted Svelte 5 rune syntax ($props, $state, $derived, onclick=) instead of plan's legacy syntax, to match the existing server-ui codebase (Devices/Reports/Admin pages all use runes)."
metrics:
  duration: ~25 minutes
  completed: 2026-05-07
  tasks: 2
  files-created: 4
  files-modified: 3
requirements:
  - REQ-layout-snapshots
---

# Phase 7 Plan 04: SvelteKit /layout Route + Components Summary

Implemented the dashboard surface for layout snapshots — a new `/layout` SvelteKit route with click-to-expand timestamp rows (mode A) and `?t=ISO` permalink view (mode B), wired to the server endpoints from 07-03.

## What Was Built

### Task 1 — Types, API namespace, sidebar nav (commit 5ccef9c)

- **web/server-ui/src/lib/types.ts**: Appended `LayoutWindow` (6 fields exactly: app_name, window_title, x, y, w, h), `LayoutSnapshot`, `LayoutTimelineEntry`. Privacy-locked — no pid/cmdline/screenshot/exe_path/focused/z_order/display_index.
- **web/server-ui/src/lib/api.ts**: Extended type-import then inserted `layout: { timeline(), at() }` namespace immediately after `timesheets`. URL shapes: `/layout-snapshots/timeline?device_id=&from=&to=` and `/layout-snapshots?device_id=&t=...`.
- **web/server-ui/src/lib/components/Sidebar.svelte**: Inserted Layout entry between Devices and Team with the Heroicons squares-2x2 outline path verbatim from UI-SPEC. No `roles` field — every authenticated user sees their own snapshots.

### Task 2 — Route + components (commit d9a734d)

- **+page.ts** (36 LOC): The first `+page.ts` in server-ui. Reads `url.searchParams.get('t')`, fetches devices, then dispatches to `api.layout.timeline` (no `t`) or `api.layout.at` (with `t`). Exports `LayoutPageData` discriminated union (`no-device | timeline | permalink`).
- **+page.svelte** (120 LOC): Page composition. Renders `Layout Snapshots` h1 plus mode-A or mode-B card. All copy strings verbatim per UI-SPEC: `Today's snapshots`, `No snapshots yet today.`, `No snapshot for this moment.`, `No client devices registered yet.`, `Copy permalink`. Dark-mode pairings on every text/background. Click-to-expand calls `api.layout.at` to fetch full snapshot lazily; only one row open at a time.
- **LayoutSnapshotRow.svelte** (46 LOC): Real `<button type="button">` with `aria-expanded` + `aria-controls`, chevron that rotates 90° on expand, mono timestamp + count-label badge. Focus-visible ring matches dashboard convention.
- **LayoutSnapshotWindowList.svelte** (26 LOC): Pure presentation. Renders `{app_name} — {window_title}` joined by em-dash with geometry suffix ` · {w}×{h} @ ({x}, {y})` using U+00D7 (×) and `&mdash;`. Falls back gracefully when title or app_name is empty.

## Sizes (atomic-design budgets)

- LayoutSnapshotRow.svelte: 46 LOC (atom budget ≤50 ✓)
- LayoutSnapshotWindowList.svelte: 26 LOC (atom budget ≤30 ✓)
- +page.svelte: 120 LOC (molecule budget ≤200 ✓; UI-SPEC target ~120 ✓)
- +page.ts: 36 LOC (atom budget ≤80 ✓; UI-SPEC target ~40 ✓)

## First +page.ts precedent

Per 07-PATTERNS.md, this is the first SvelteKit `+page.ts` loader in server-ui — every existing route uses `onMount` inside `+page.svelte`. The new loader sets the precedent UI-SPEC mandates without conflicting with existing `onMount` routes (they coexist; `onMount` fires when the SPA is hydrated regardless of whether a sibling route uses a loader).

## Verification

- `npm run build` — exit 0 (vite/SvelteKit static adapter, all 250+ files compiled).
- `npm run check` (svelte-check) — zero errors and zero warnings on the four new files. Pre-existing warnings/errors in unrelated files (BarChart, admin/settings entra_secret, DateRangeFilter labels, etc.) left in place per scope-boundary rule.
- Privacy grep across all four new frontend files — `\b(pid|cmdline|screenshot|exe_path|focused|z_order|display_index)\b` → 0 matches.
- XSS gate — `@html` → 0 matches. All template interpolation uses Svelte's default HTML-escaping.
- Acceptance grep checks — all UI-SPEC copy strings present byte-identical (`Layout Snapshots`, `Today's snapshots`, `No snapshots yet today`, `No snapshot for this moment`, `No client devices registered yet`, `Copy permalink`). U+00D7 multiplication sign present. `aria-expanded` and `type="button"` both present on the row component. `url.searchParams.get('t')` present in the loader.
- Dark-mode pairings — `dark:bg-slate-800`, `dark:text-white` present multiple times in +page.svelte; every component mirrors the /devices `dark:` palette.
- Sidebar order — `Devices` line 14, `Layout` line 15, `Team` line 16 (all three appear in the awk-extracted navItems block in that order).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Svelte 5 runes adoption**
- **Found during:** Task 2 (immediately on file creation)
- **Issue:** Plan provided Svelte legacy syntax (`export let`, `on:click`, `$:` reactive labels). The server-ui codebase uses Svelte 5 runes everywhere (`$state`, `$props`, `$derived`, `onclick=`). Mixing legacy and runes within a single project triggers svelte-check warnings and is not permitted.
- **Fix:** Translated all components to Svelte 5 runes — `$props()` for component inputs, `$state(...)` for reactive variables, `$derived(...)` for computed values, `onclick={handler}` for events. Semantics identical; only the idiom changed.
- **Files modified:** LayoutSnapshotRow.svelte, LayoutSnapshotWindowList.svelte, +page.svelte
- **Commit:** d9a734d

**2. [Rule 1 - Bug] PageData type widening broke discriminated-union narrowing**
- **Found during:** Task 2, after first svelte-check run (3 errors: `data.timeline possibly undefined`, `data.snapshot possibly undefined`, `data.deviceId is string | undefined`)
- **Issue:** SvelteKit's auto-generated `PageData` wraps the `LoadResult` return with `Expand<Omit<...> & OptionalUnion<...>>`, which collapses the discriminated union `{mode:'timeline'; deviceId:string; ...} | {mode:'permalink'; ...}` into a single object `{mode?:...; deviceId?:string; timeline?:...; ...}` with all fields optional. Type narrowing (`if (data.mode === 'timeline')`) cannot recover from this.
- **Fix:** Exported the load-result union type as `LayoutPageData` from `+page.ts`, imported it into `+page.svelte`, and typed `Props.data` as `LayoutPageData` directly instead of `PageData`. SvelteKit still calls the loader correctly because it's the runtime `load` export that matters; the static `PageData` is only a type alias.
- **Files modified:** +page.ts, +page.svelte
- **Commit:** d9a734d

## Authentication Gates

None — implementation purely client-side, no new auth surface.

## Self-Check: PASSED

- All 7 expected artifacts present: types.ts, api.ts, Sidebar.svelte (modified); +page.ts, +page.svelte, LayoutSnapshotRow.svelte, LayoutSnapshotWindowList.svelte (created).
- Both commits exist: 5ccef9c (Task 1), d9a734d (Task 2).
- Privacy grep gate clean across all four new frontend files.
- Build green; svelte-check zero errors/warnings on new files.

## Threat Flags

None new. The threat register from the plan is fully covered:
- T-7-04-01 (XSS): mitigated — no `{@html}`, all interpolation uses Svelte's default escaping.
- T-7-04-02 (URL leak): mitigated — permalink contains only ISO timestamp, no titles.
- T-7-04-03 (cross-user device id): mitigated server-side in 07-03.
- T-7-04-04 (privacy field smuggling): mitigated — LayoutWindow has exactly 6 fields, no Svelte template references forbidden fields.
- T-7-04-05 (huge timeline DoS): accepted with server-side limit from 07-03.
- T-7-04-06 (clipboard fallback): accepted — try/catch with "Copy failed" label.
