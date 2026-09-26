# Trasker

Privacy-respecting, self-hosted time tracker. A cross-platform Go **client daemon** runs on each
user's machine, watches active-window focus + screen-lock presence (**never** keystrokes, mouse, or
screenshots), tags blocks of activity, and lets the user submit signed-off timesheets to a
self-hosted Go **server**. The server serves an Entra-OIDC SvelteKit dashboard for personal review,
manager team views, and admin/audit. Single Go module monorepo, one deployment = one org.

- Module: `github.com/jaypaulb/trasker` · Go 1.25 · Postgres 16 (server) / pure-Go SQLite (client)
- Canonical planning context: `.planning/PROJECT.md`, `REQUIREMENTS.md`, `ROADMAP.md`, `STATE.md`

## Layout

| Path | What |
|------|------|
| `cmd/trasker-server/` | Server entrypoint (`main.go`, `layout_downsampler.go`) |
| `cmd/trasker-client/` | Client daemon entrypoint + `cli.go` subcommands |
| `internal/server/` | HTTP API, Postgres store, auth (Entra + API keys), client-binary builder — see its AGENTS.md |
| `internal/client/` | Focus/presence tracking, tagging, layout snapshots, local SQLite, sync — see its AGENTS.md |
| `internal/shared/` | `models` (wire DTOs), `apikey` (`tsk_` key format), `version` (ldflags sentinels) |
| `web/client-ui/` | Svelte SPA served on client localhost — see `web/AGENTS.md` |
| `web/server-ui/` | SvelteKit dashboard SPA embedded in the server — see `web/AGENTS.md` |
| `migrations/` | Postgres migrations (`NNN_*.up/down.sql`) + `migrate.go` runner |
| `deploy/` | Docker Compose + Dockerfiles — see its AGENTS.md for production invariants |
| `tests/integration/` | Cross-component tests (client↔server, build pipeline) |

## Build & Test

- `make all` — build client + server for current platform into `dist/`
- `make test` / `go test ./...` — full suite · `make vet`
- `make client-ui` — build Svelte SPA and copy into `internal/client/webui/static/`
- `make build-all-clients` — cross-compile 5 targets (`CGO_ENABLED=0`); `make verify-cross-compile` checks them
- Version stamping is via ldflags on `internal/shared/version` — do not hardcode version strings

## Intent Layer

**Before modifying code in a subdirectory, read its AGENTS.md first** for local patterns and invariants.

- **Server**: `internal/server/AGENTS.md` — API, store, auth, builder
- **Client daemon**: `internal/client/AGENTS.md` — trackers, presence, tagging, layout, sync
- **Web UIs**: `web/AGENTS.md` — both Svelte apps
- **Deployment**: `deploy/AGENTS.md` — autocert, sentinel-patched binaries, JWT/admin bootstrap

## Global Invariants

- **Privacy is the product.** Focus events store only `app_name + window_title`. No keystroke,
  mouse, or screenshot capture — ever. This is an explicit anti-feature, not a gap to fill.
- **Submitted entries are immutable from the client.** Once signed off, only audit-logged admin
  server endpoints may mutate a timesheet. Never add a client path that edits submitted data.
- **Time is UTC end to end.** Client SQLite stores UTC ISO-8601; server uses `TIMESTAMPTZ`;
  submission payloads are always UTC. UI converts UTC→local for *display only*.
- **Platform code uses Go build tags**, not runtime OS switches. A feature that touches the OS
  (tracker, screen-lock, tray, notifications, autostart, window enum) has one file per platform
  and usually a `_nocgo` fallback. Keep all platforms + the nocgo path in sync when editing.
- **Two auth models on the server:** humans authenticate via Entra OIDC (JWT); client daemons
  authenticate via bcrypt-hashed `tsk_` API keys. Do not cross the streams.
- **`internal/shared/models` is the wire contract** between client and server. Changing a DTO ripples
  to the sync client, the API handlers, and both Svelte UIs — treat it as a public API.
