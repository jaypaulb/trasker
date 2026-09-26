# Client daemon (internal/client)

## Purpose
The cross-platform Go daemon that runs on each user's machine: tracks active-window focus and
screen-lock presence, tags activity, captures window layout snapshots, persists everything to local
SQLite, serves a localhost Svelte SPA, and syncs signed-off timesheets to the server. Entry point is
`cmd/trasker-client/main.go` (+ `cli.go`). Largest component in the repo (~13.7k LOC).

## Packages
- `tracker/` — polls the active window every 1s → `app_name + window_title`. Per-platform:
  `tracker_darwin` (NSWorkspace/AX, + `_nocgo`), `tracker_linux_x11` / `_wayland` (+ `_cgo`/`_nocgo`),
  `tracker_windows` (SetWinEventHook). **Only these two fields are ever captured.**
- `presence/` — screen-lock detection (`screenlock_{darwin,linux,windows}`) plus `deadman.go`, the
  escalating "deadman's switch" (30/45/60/90/120 min intervals, each a 90s notification countdown).
- `layout/` — layout snapshots: enumerate every open window + geometry every 60s
  (`enum_{linux_x11,darwin,windows}`, `_nocgo` fallback), `capturer.go`, `store.go`, `prune.go`,
  `sync.go`. Distinct cadence/schema/sync from focus tracking — keep them separate.
- `tagger/` — rule `engine.go` + `learner.go` (auto-rules from user behavior) + rule `store.go`.
- `session/` — `engine.go` and `cascade.go`: the **note-tag cascade** — a note's TAG (not text)
  propagates outward in 60→30→15→7.5→<5 min steps, skipping already-tagged/already-submitted events.
- `store/` — local SQLite (`modernc.org/sqlite`, pure-Go): `focus_events`, `event_tags`, `notes`,
  `tags`, `pomodoro`, `submissions`, `config`.
- `sync/` — `queue.go` (offline outbox), `client.go` (HTTP to server), `submit.go` (timesheet submit).
- `notify/`, `tray/`, `pomodoro/`, `setup/` (autostart per-OS + `firstrun`), `runtime/` (GNOME/desktop
  launcher wiring), `webui/` (`api.go` localhost REST + `embed.go` for the client SPA).

## Contracts & Invariants
- **Privacy anti-feature is absolute:** no keystroke, mouse, or screenshot capture. Focus events are
  `app_name + window_title` only. Any PR adding richer capture violates the core value.
- **`focus_events` rows are immutable once written.** Tagging attaches `event_tags`; it never rewrites
  the event. Submitted events are frozen — the cascade and tagger must skip them.
- **Local SQLite stores UTC ISO-8601.** Convert to local only in the UI layer.
- **Every OS-touching feature has a file per platform + a `_nocgo` fallback**, selected by build tags.
  When you change behavior on one platform, update the others and the nocgo path, plus their `_test.go`.
- **Sync is offline-first:** writes go to the local `sync/queue`, which drains to the server when
  reachable. Never block the tracker loop on network I/O.

## Patterns
Add a tracked signal: implement the per-platform capture (with `_nocgo`), persist via a `store/`
table, expose it through `webui/api.go` for the SPA, and if it should reach the server, enqueue it in
`sync/queue.go`. New OS integration → new build-tagged file set + tests for each platform.

## Anti-patterns
- Never add mouse/keystroke/screenshot hooks.
- Never mutate a submitted `focus_event` or timesheet from the client.
- Don't do OS branching with runtime `runtime.GOOS` switches where a build-tagged file belongs.

## Related
- Wire DTOs / API key format: `../shared/models`, `../shared/apikey`
- Server it syncs to: `../server/AGENTS.md` · Client SPA source: `../../web/client-ui/` (`web/AGENTS.md`)
