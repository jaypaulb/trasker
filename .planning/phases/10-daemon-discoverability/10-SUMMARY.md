---
phase: 10
plan: full
subsystem: client
tags: [cli, daemon, runtime, systray, autostart, discoverability]
requires:
  - REQ-daemon-discoverability
provides:
  - pidfile + URL state files at $XDG_STATE_HOME/trasker/
  - GET /api/status endpoint
  - CLI subcommands: status, open, quit, install-autostart, version, help
  - StatusNotifierWatcher pre-flight probe with notify-send fallback
affects:
  - cmd/trasker-client/main.go (subcommand dispatch + WriteState lifecycle + statusSnapshot)
  - internal/client/webui/server.go (StatusProvider injection)
  - internal/client/webui/api.go (/api/status handler)
  - internal/client/tray (IsAvailable probe, no API change)
tech-stack:
  added: []
  patterns:
    - "atomic.Pointer[string] for goroutine-safe status snapshot writes"
    - "DBus NameHasOwner pre-flight probe to avoid silent fyne.io/systray failure"
    - "Subcommand dispatch via positional argv switch (no flag.NewFlagSet, no cobra)"
key-files:
  created:
    - internal/client/runtime/runtime.go
    - internal/client/runtime/runtime_test.go
    - internal/client/tray/availability_linux.go
    - internal/client/tray/availability_other.go
    - internal/client/tray/availability_linux_test.go
    - cmd/trasker-client/cli.go
    - cmd/trasker-client/cli_test.go
  modified:
    - cmd/trasker-client/main.go
    - internal/client/webui/server.go
    - internal/client/webui/api.go
    - internal/client/webui/api_test.go
decisions:
  - "Skip libayatana-appindicator: would require CGO + system C deps. The DBus probe + notify-send fallback is dep-free and covers the actual failure mode."
  - "trasker-client install-autostart writes trasker-client.desktop, not trasker.desktop, to avoid colliding with the existing tray-managed entry from internal/client/setup."
  - "CLI uses positional argv switch instead of flag.NewFlagSet: every subcommand has zero or one optional arg, so a switch is plenty and avoids the flag-help-message overhead."
  - "Pre-flight DBus probe runs in a goroutine with 2s timeout: a wedged DBus is treated as no-tray rather than blocking daemon startup."
metrics:
  duration: ~25min
  completed: 2026-05-08
---

# Phase 10: Daemon Discoverability + CLI Summary

A running `trasker-client` daemon is now discoverable and operable without `pgrep`/`lsof`/port-archeology. The daemon writes a pidfile + dashboard-URL file under `$XDG_STATE_HOME/trasker/` on startup, removes them on clean exit, and overwrites stale entries (PID not running) on next start. New CLI subcommands `status`, `open`, `quit`, and `install-autostart` operate on the running daemon over loopback HTTP. The systray silent-failure on Ubuntu/GNOME without the appindicator extension is fixed by a DBus pre-flight probe that skips the tray when no `StatusNotifierWatcher` is registered, falling back to the notify-send chip already wired into startup.

## What was built

Four atomic commits, each with tests:

1. `94961da` — `internal/client/runtime` package. Owns `~/.local/state/trasker/{trasker.pid,trasker.url}` lifecycle. Stale-PID detection via signal-0 probe. ErrAlreadyRunning distinguishes stale-pid from a different live daemon. 8 tests covering write/read/clear, stale-pid detection, XDG_STATE_HOME respect, port parsing.

2. `ee005b8` — `GET /api/status` endpoint + `webui.StatusProvider` hook. Daemon registers a provider via `Server.SetStatusProvider`; nil-safe so the endpoint still returns pid/port/uptime when no provider is wired. 2 new webui tests cover with-provider and without-provider paths.

3. `325adab` — CLI subcommand dispatch in `cmd/trasker-client/cli.go`. Bare `trasker-client` still launches the daemon (backwards-compat). New subcommands shell to xdg-open / open / rundll32 (open), POST /api/quit (quit), or print formatted status from /api/status (status). `install-autostart` writes the .desktop file pointing at `os.Executable()`. Daemon path now writes the pidfile + URL file via `clientruntime.WriteState`, registers a `statusSnapshot` (atomic.Pointer for presence/lock + SQLite MAX queries for last-layout / last-sync), and clears state files on clean exit. 8 CLI tests using httptest.

4. `8f9d4da` — `tray.IsAvailable()` probe. On Linux, queries `org.freedesktop.DBus.NameHasOwner` for `org.kde.StatusNotifierWatcher` with 2s timeout. If absent, daemon skips `systray.Run` entirely and relies on the existing startup notify-send + new CLI subcommands. macOS/Windows always return true.

## Success criteria

| # | Criterion | Status |
|---|-----------|--------|
| 1 | Pidfile + URL file written on startup, cleared on clean exit, stale entries overwritten | DONE — `runtime_test.go::TestWriteState_OverwritesStalePID`, `TestClear_Idempotent` |
| 2 | `status / open / quit` work when daemon running; bare invocation still launches daemon | DONE — `cli_test.go::TestRunQuit_HitsRunningDaemon`, `TestRunStatus_FetchesFromDaemon`, `TestDispatch_BareInvocation` |
| 3 | `status` prints PID, port, uptime, last-layout, last-sync, presence, screen-lock; exit 0 if running, 1 if not | DONE — `cli_test.go::TestRunStatus_FetchesFromDaemon` asserts every field; `TestRunStatus_NotRunning` asserts exit 1 |
| 4 | Systray fallback when KStatusNotifierWatcher missing — emit notify-send with URL on startup | DONE — DBus probe in `availability_linux.go`, existing startup notify in `openDashboardAndNotify` already covers the URL surface |
| 5 | `install-autostart` writes idempotent .desktop file | DONE — `cli_test.go::TestWriteAutostartDesktop_Idempotent` |

## Test results

```
ok  	github.com/jaypaulb/trasker/cmd/trasker-client       0.011s   (8 tests)
ok  	github.com/jaypaulb/trasker/internal/client/runtime  0.009s   (8 tests)
ok  	github.com/jaypaulb/trasker/internal/client/tray     0.006s   (1 new test)
ok  	github.com/jaypaulb/trasker/internal/client/webui    0.025s   (2 new tests)
```

All other packages pass (`go test ./...`); a couple of CGO-dependent packages (layout, tracker, session) hit transient `fork/exec resource temporarily unavailable` on the first run due to host process limits during a parallel run — they pass cleanly on retry. No regression introduced by Phase 10 changes.

Cross-compile sanity check (CGO_ENABLED=0):
- darwin/arm64: 16 MB binary, builds clean
- windows/amd64: 17 MB .exe, builds clean
- linux/amd64 (host): builds clean

CLI smoke (live binary):
```
$ trasker-client help          → prints usage
$ trasker-client status        → "trasker daemon: not running" (exit 1)
$ trasker-client unknown       → usage to stderr, exit 2
```

## Deviations from Plan

### Auto-fixed issues

**1. [Rule 3 - Blocking] `internal/client/webui/static/` missing for embed compilation**
- Found during: testing the new /api/status handler
- Issue: `embed.go` declares `//go:embed all:static` but the directory exists only after `make client-ui` builds the SPA, blocking all `go test ./internal/client/webui/` runs from a fresh checkout
- Fix: created `internal/client/webui/static/.gitkeep` placeholder so the embed compiles in CI / agent contexts where the SPA hasn't been built. The dir is partially gitignored (`_app/`, `index.html`, `robots.txt`) so the placeholder doesn't conflict with real builds
- Files touched: `internal/client/webui/static/.gitkeep` (untracked, kept locally)
- Note: Did not commit the .gitkeep — pre-existing repo convention is that `static/` is build output. The Makefile wipes and recreates it for production builds. Recording here so the next agent sees the workaround.

**2. [Rule 2 - Critical] `clientTrayActions.tray` field unused after fallback**
- Found during: skip-tray-when-unavailable wiring
- Issue: When `tray.IsAvailable()` returns false, `trayActions.tray` stays nil. The field is currently only assigned (never read) elsewhere in main, so a nil is harmless, but it's a latent footgun if a future change adds `a.tray.UpdateTracking(...)`.
- Fix: not a fix yet — left a comment in the future-work area; if `UpdateTracking` is ever called from main, it'll need a nil-check.
- Logged here for the next change to be aware.

### Decisions deferred to caller / future work

- **Linux libayatana-appindicator path:** Phase-10 success criterion 4 mentions "libayatana-appindicator OR notify-send" as the fallback. We chose notify-send (already wired). Adding the libayatana path requires CGO + a system C dep, which conflicts with the project-wide `CGO_ENABLED=0` cross-compile policy. The notify-send fallback covers the discoverability gap; an appindicator path can be added later without breaking the current contract.
- **macOS/Windows tray probe:** `IsAvailable() = true` unconditionally on those platforms. Their respective tray APIs (NSStatusItem, Shell_NotifyIcon) don't have the discovery problem that Linux's StatusNotifierItem protocol does. If a Windows machine without a tray is ever encountered, this could be revisited.

## Files

Created:
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/internal/client/runtime/runtime.go` (228 LOC, organism)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/internal/client/runtime/runtime_test.go` (200 LOC)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/internal/client/tray/availability_linux.go` (60 LOC)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/internal/client/tray/availability_other.go` (10 LOC)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/internal/client/tray/availability_linux_test.go` (28 LOC)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/cmd/trasker-client/cli.go` (290 LOC, organism)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/cmd/trasker-client/cli_test.go` (215 LOC)

Modified:
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/cmd/trasker-client/main.go` (+~110 LOC: dispatch hook, WriteState/Clear lifecycle, statusSnapshot type, tray probe wiring)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/internal/client/webui/server.go` (+30 LOC: StatusProvider type, SetStatusProvider, StartedAt accessor)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/internal/client/webui/api.go` (+45 LOC: /api/status handler)
- `/home/jaypaulb/Projects/gh/trasker/.claude/worktrees/agent-ac7cca4738bd3e1d8/internal/client/webui/api_test.go` (+85 LOC: 2 new tests + stub provider)

All organisms under the 400 LOC limit. Largest is `cmd/trasker-client/cli.go` at 290 LOC.

## Self-Check: PASSED

Verification:
- All 4 commits exist in `git log --oneline -8`: 94961da, ee005b8, 325adab, 8f9d4da — confirmed
- All 7 created files exist on disk — confirmed via `ls`
- `go build ./cmd/trasker-client` succeeds — confirmed
- `go test ./internal/client/runtime ./internal/client/webui ./internal/client/tray ./cmd/trasker-client` all PASS — confirmed
- `go vet ./...` clean — confirmed
- Cross-compile to darwin/arm64 and windows/amd64 clean — confirmed
- Live binary smoke (help / status / unknown) all behave as designed — confirmed
