# Phase 7: Layout Snapshots - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-07
**Phase:** 07-layout-snapshots
**Areas discussed:** Retention policy, Cross-platform v1, Snapshot trigger, Dashboard UX

---

## Retention policy

**Q1 — Client SQLite retention?**
- 30 days raw, then drop (Recommended) — 50MB/yr, server keeps long tail. *Not selected.*
- **7 days raw, server is truth — Selected.** Tightest local footprint (~12MB/wk), trust server beyond.
- Forever, with downsampling — keep all locally with downsample tiers. *Not selected.*

**Q2 — Server PostgreSQL retention?**
- Raw 90d, downsampled forever (Recommended). *Not selected.*
- Raw forever — never downsample. *Not selected.*
- Raw 30d, drop older. *Not selected.*
- **Free-text answer:** "7 days at 60s, then 10m 30 days, then 1 hr for older."

User specified custom tiered policy: 0–7d at 60s raw, 7–37d at 10min, 37+d at 1hr indefinitely.

**Q3 — Per-app retention overrides?**
- **No overrides v1 (Recommended) — Selected.** Single global policy.
- Allow per-app TTL. *Not selected.*

---

## Cross-platform v1 (Wayland/macOS/Windows/multi-monitor)

**Q4 — Wayland in v1?**
- **X11 only, Wayland deferred (Recommended) — Selected.** Reuses existing CGo X11 pattern, days of work.
- X11 + wlr-foreign-toplevel-management (partial Wayland). *Not selected.*
- Block until Wayland works everywhere (would need GNOME Shell extension). *Not selected.*

**Q5 — macOS + Windows in v1?**
- **Linux v1, macOS/Windows v2 (Recommended) — Selected.** Ship Linux first, dogfood, then expand.
- All three at once. *Not selected.*
- Linux + Windows (skip macOS). *Not selected.*

**Q6 — Multi-monitor handling?**
- Capture display index per window (Recommended). *Not selected.*
- **Skip multi-monitor v1 — Selected.** Just absolute X/Y, no display index. Simpler schema.

---

## Snapshot trigger

**Q7 — When to capture a snapshot?**
- Fixed 60s tick (Recommended). *Not selected.*
- **Change-detect only — Selected.** Hash window-set, only write a row when set changes. Idle = zero rows.
- 60s tick + extra snap on focus change. *Not selected.*

**Q8 — Skip when screen locked?**
- **Skip when locked (Recommended) — Selected.** Reuse existing screenlock detector. Privacy bonus.
- Always capture. *Not selected.*

**Q9 — What if X11 enum fails mid-tick?**
- **Log warn, skip tick, retry next 60s (Recommended) — Selected.** Fail-soft.
- Write row with error marker. *Not selected.*

---

## Dashboard UX

**Q10 — Layout History primary view?**
- Timeline scrubber + window list (Recommended). *Not selected.*
- Geometry mosaic. *Not selected.*
- **List of timestamps — Selected.** Plain table: timestamp + window count + click-to-expand. Boring, fastest.

**Q11 — How to pick the instant?**
- Date picker + time scrubber (Recommended). *Not selected.*
- Free text "15:23 yesterday". *Not selected.*
- **Permalink only — Selected.** URL-driven, `?t=ISO`. Devops-friendly, less casual.

**Q12 — Multi-device merge view?**
- Tabbed per-device + "All" tab (Recommended). *Not selected.*
- Side-by-side columns always. *Not selected.*
- **Skip merge UI v1 — Selected.** Per-device only, defer until 2+ devices online.

**Q13 — Search by window title?**
- **Skip search v1 (Recommended) — Selected.** Add later if dogfooding shows demand.
- Search bar v1. *Not selected.*

---

## Claude's Discretion

- Wire format for sync payload (single existing sync endpoint vs new `/api/v1/layout-snapshots`) — planner picks.
- Server-side downsampling implementation (cron job / pg_cron / in-process scheduler) — planner picks.
- SvelteKit route shape (new `/layout` route vs panel inside `/timeline`) — planner picks.

## Deferred Ideas

- Wayland support (wlr-foreign-toplevel-management for Sway/Hyprland; GNOME Shell extension for GNOME).
- macOS layout enum (`CGWindowListCopyWindowInfo`).
- Windows layout enum (`EnumWindows` + per-monitor DPI).
- Multi-monitor display index + dashboard monitor-layout rendering.
- Multi-device merge view (likely Phase 9 dogfooding trigger).
- Window-title full-text search.
- Geometry mosaic visualization.
- Per-app retention overrides / opt-out for sensitive apps.
- Adaptive cadence (snap on focus-change events too).

## ROADMAP amendments needed

Phase 7 success criteria currently say:
- #1 "every 60s" → should be "evaluated every 60s, written on change"
- #3 "Linux X11+Wayland, macOS, Windows" → should be "Linux X11; Wayland/macOS/Windows deferred"
- #4 "Merged mode shows all devices' layouts side by side" → should be "Per-device only; merge view deferred"

Planner should surface these to user before plan-checker runs so ROADMAP.md stays in sync.
