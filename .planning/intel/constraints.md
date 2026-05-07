# Constraints Intel

Constraints extracted from the two SPEC documents. Each entry has `type ∈ {api-contract, schema, nfr, protocol}`.

---

## CONSTRAINT-tech-stack
type: nfr
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Tech Stack)

- Client daemon: Go, cross-compiled per platform
- Client storage: SQLite via `modernc.org/sqlite` (pure Go, no CGo)
- Client UI: Svelte SPA served on localhost by Go
- Server API: Go (Chi router)
- Server storage: PostgreSQL 16
- Server dashboard: Svelte SPA (SvelteKit)
- Server proxy (legacy, removed in production-deployment SPEC): Caddy
- Server deployment: Docker Compose
- Auth (dashboard): Entra ID (OIDC), per-deployment
- Auth (client): bcrypt-hashed API key in `Authorization` header

---

## CONSTRAINT-repository-layout
type: nfr
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Repository Structure)

Single Go module monorepo with `cmd/trasker-{client,server}`, `internal/{client,server,shared}`, `web/{client-ui,server-ui}`, `migrations/`, `deploy/`. Platform-specific code uses Go build tags: `tracker_linux.go`, `tracker_darwin.go`, `tracker_windows.go`.

---

## CONSTRAINT-platform-support-matrix
type: protocol
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Platform Support)

| Platform | Focus Tracking | Screen Lock | Tray | Notifications | Autostart |
|----------|---------------|-------------|------|---------------|-----------|
| Ubuntu 24.04 (X11) | XGetInputFocus | DBus screensaver | systray | libnotify | .desktop |
| Ubuntu 24.04 (Wayland) | ext-foreign-toplevel-list-v1 (primary), wlr-foreign-toplevel-management (fallback) | DBus screensaver | systray | libnotify | .desktop |
| macOS | NSWorkspace.frontmostApplication + Accessibility API | NSWorkspace | systray | NSUserNotification | LaunchAgent |
| Windows | SetWinEventHook (EVENT_SYSTEM_FOREGROUND) + GetWindowText | WTSRegisterSessionNotification | systray | Toast | Registry Run |

Build targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64.

---

## CONSTRAINT-timezone-convention
type: nfr
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Timezone Convention)

- Client SQLite stores ISO-8601 strings in UTC
- Server PostgreSQL uses `TIMESTAMPTZ`
- Client UI converts UTC → local for display only
- Submission payloads always UTC

---

## CONSTRAINT-security
type: nfr
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Security)

- API keys bcrypt-hashed server-side; plaintext only inside the client binary
- API key generation requires prior Entra login (key always FK'd to existing `users` row)
- All client→server traffic over HTTPS
- No keystroke / mouse / screenshot capture
- Focus events store only `app_name + window_title` (no content)
- SQLite DB lives in user home dir with standard file permissions

---

## CONSTRAINT-error-handling
type: nfr
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Error Handling)

- Client operates fully offline; submissions enter local `submissions` table with `status = "pending"`
- Retry backoff: 1m → 5m → 15m → 1h → hourly
- On 5xx / timeout: same backoff
- On API key expiry: surface clear user-facing message linking to dashboard
- No silent fallbacks — errors must surface to user

---

## CONSTRAINT-api-endpoints (REST contract)
type: api-contract
source: docs/superpowers/specs/2026-03-23-trasker-design.md (API Endpoints)

```
Auth:
  POST   /api/v1/auth/login          Entra OIDC callback
  POST   /api/v1/auth/refresh        Refresh JWT

Users (admin):
  GET    /api/v1/users
  PATCH  /api/v1/users/:id

Devices:
  POST   /api/v1/devices             Register device (first-run)
  GET    /api/v1/devices              List user's devices
  PATCH  /api/v1/devices/:id          Update name

Timesheets:
  POST   /api/v1/timesheets           Submit entries
  GET    /api/v1/timesheets           List own submissions
  GET    /api/v1/timesheets/team      Team view (manager+)

Reports:
  GET    /api/v1/reports/summary
  GET    /api/v1/reports/export       CSV

Build:
  POST   /api/v1/build/download       Generate client binary with API key
  GET    /api/v1/build/status

Admin:
  DELETE /api/v1/admin/entries/:id    Audit-logged
  PATCH  /api/v1/admin/entries/:id    Audit-logged
  POST   /api/v1/admin/keys/:id/revoke
  GET    /api/v1/admin/audit
  GET    /api/v1/admin/settings
  PATCH  /api/v1/admin/settings

Health:
  GET    /api/v1/health
```

---

## CONSTRAINT-submission-payload
type: api-contract
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Submit Flow)

```json
{
  "client_device_id": "abc-123",
  "entries": [
    {
      "tag": "Development",
      "started_at": "2026-03-23T09:00:00Z",
      "ended_at": "2026-03-23T12:30:00Z",
      "duration_s": 12600,
      "notes": "...",
      "app_summary": "VS Code (72%), Terminal (18%), Firefox (10%)"
    }
  ]
}
```

Note aggregation: chronologically concatenated, prefixed by `[HH:MM]` per anchor.

Server never receives raw focus events.

---

## CONSTRAINT-client-sqlite-schema
type: schema
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Client — SQLite)

Tables: `focus_events`, `tags`, `event_tags` (FK source ∈ {manual, cascade, auto_rule} + `cascade_from`), `notes`, `tag_rules`, `submissions` (status ∈ {pending, submitted, confirmed}), `submission_events`, `pomodoro_sessions`, `config` (single-row, CHECK id=1).

Full DDL preserved verbatim in the SPEC.

---

## CONSTRAINT-server-postgres-schema
type: schema
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Server — PostgreSQL)

Tables: `users`, `api_keys`, `devices` (with `UNIQUE(client_device_id, api_key_id)` for idempotent registration), `timesheets`, `timesheet_entries`, `audit_log`, `org_settings` (single-row, CHECK id=1).

Full DDL preserved verbatim in the SPEC.

---

## CONSTRAINT-presence-state-machine
type: protocol
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Presence Detection)

```
TRACKING → (screen lock)            → AWAY     → (unlock)        → TRACKING
TRACKING → (no focus, interval hit) → CHECKING → (click 90s win) → TRACKING (next interval)
CHECKING → (90s expired)            → PAUSED
PAUSED   → (focus change)           → resume prompt → TRACKING (reset to 30min)
```

Default intervals: [30, 45, 60, 90, 120] minutes (capped at 120). Stored in `config.presence_intervals` as a JSON array string.

---

## CONSTRAINT-note-cascade-algorithm
type: protocol
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Note Cascade)

Edge-propagation cascade with steps 60 → 30 → 15 → 7.5 → <5 minutes; only the **tag** propagates, not the note text. Already-tagged events are skipped. Submitted events block propagation but do not consume neighbors. Cascade is computed on demand at note-add time. Each derived `event_tags` row carries `source = "cascade"` and `cascade_from = anchor_event_id`.

---

## CONSTRAINT-api-key-lifecycle
type: protocol
source: docs/superpowers/specs/2026-03-23-trasker-design.md (API Key Lifecycle)

- Bcrypt-hashed server-side
- `key_prefix` (first 8 chars) stored for admin UI identification
- `last_used_at` updated every sync/submission
- Default expiry: 60 days inactivity (configurable via `org_settings.key_expiry_days`)
- Admin-revocable (lost device, offboarding)
- One API key may register many devices (resolved via `Authorization` header → user_id, api_key_id)

---

## CONSTRAINT-production-architecture
type: nfr
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (Architecture)

Production target = 2-container compose: `postgres` + `trasker` (Go server with embedded SPA + autocert TLS + pre-compiled client blobs). Replaces previous 4-container Caddy + Nginx + API + Postgres design.

---

## CONSTRAINT-autocert-listeners
type: protocol
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (Autocert TLS Integration)

```
if TRASKER_FQDN is set:
  listen :443  (TLS via golang.org/x/crypto/acme/autocert)
  listen :80   (ACME HTTP-01 + HTTP→HTTPS redirect)
  listen :8080 (internal plain HTTP for healthcheck / LB)
  three http.Server instances, coordinated graceful shutdown on SIGTERM/SIGINT
else:
  listen LISTEN_ADDR (default :8080), plain HTTP, single http.Server
```

Cert cache `/data/certs/` persisted via `trasker_data` volume. Host whitelist contains exactly the configured FQDN. Public URL derived as `https://<TRASKER_FQDN>` (replaces removed `TRASKER_SERVER_URL` env var).

---

## CONSTRAINT-spa-embedding
type: nfr
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (SPA Embedding)

New `internal/server/webui/embed.go` with `//go:embed all:static`. Catch-all route in `internal/server/api/router.go` serves the embedded FS, falling back to `index.html` for client-side routing. Multi-stage Dockerfile copies SPA build into `internal/server/webui/static/` before `go build`.

---

## CONSTRAINT-client-binary-distribution
type: protocol
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (Client Binary Distribution)

CI cross-compiles 5 client binaries with `CGO_ENABLED=0` and 128-byte null-padded sentinels (`SentinelServerURL`, `SentinelAPIKey`, `SentinelVersion`) baked via `-ldflags -X`. Binaries copied into image at `/app/clients/`. Runtime patching uses `bytes.Replace` (~50ms per download). Builder falls back to `go build` only if `/app/clients/` missing AND a Go toolchain is available.

---

## CONSTRAINT-jwt-secret-precedence
type: protocol
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (JWT Secret Auto-Generation)

1. `TRASKER_JWT_SECRET` env var if set
2. `/data/jwt-secret` file (read or generate)
3. If generating: 32 random bytes hex-encoded → 64-char string, written 0600

Implemented in new package `internal/server/secrets/jwt.go` exposing `LoadOrGenerateJWTSecret(envVar, filePath string) (string, error)`. Replaces the existing hard-fail `mustEnvMulti("TRASKER_JWT_SECRET", "JWT_SECRET")`.

---

## CONSTRAINT-admin-bootstrap-matrix
type: protocol
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (Admin Bootstrap)

First boot only (skipped when users already exist):

- both env vars set → create that admin, `force_password_change = false`
- both unset → create `admin@localhost` / `trasker-admin`, `force_password_change = true`
- exactly one set → log warning, skip bootstrap

---

## CONSTRAINT-ci-pipeline
type: nfr
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (CI Pipeline)

Trigger: git tag `v*`. Three jobs:

1. Cross-compile 5 client binaries (`CGO_ENABLED=0`)
2. Multi-stage Docker build (node:22-slim spa-builder → golang:1.25-bookworm go-builder → alpine:3.20 runtime ~50MB), pushed to `ghcr.io/<owner>/trasker:<tag>` and `:latest`
3. Create GitHub Release with `docker-compose.production.yml` attached

Runtime image contents: `/app/trasker-server`, embedded SPA, `/app/clients/*`, `/app/migrations/`, non-root `trasker:trasker` (uid 1001), `ca-certificates`, `tzdata`, pre-created `/data/`.

---

## CONSTRAINT-out-of-scope-v1
type: nfr
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Out of Scope (v1))

- Mobile clients (iOS/Android)
- External integrations (Jira, Linear, Harvest)
- Billing / subscription management
- Multi-org on a single deployment
- Browser extensions for tab-level tracking
- ML/AI categorization
