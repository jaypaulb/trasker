# Phase 9 — Multi-Device Dogfooding (cutover artifacts)

**Date:** 2026-05-07
**Status:** Server live on hal. End-to-end smoke verified.

## What shipped

### Server on hal at `https://trasker.nyolc.cc`

- **Image:** `ghcr.io/jaypaulb/trasker:dev` (pushed manually; CI release.yml will replace this with tagged builds)
- **Compose:** `~/docker/compose/hal2020/trasker.yml` (deployrr v5 conformant; no top-level `networks:`, joins `default` + `t3_proxy`)
- **Traefik rule:** `~/docker/appdata/traefik3/rules/hal2020/app-trasker.yml` (chain-no-auth — trasker handles auth in-app)
- **Database:** `trasker` role + `trasker` DB on hal's shared `postgresql:16-alpine`. Schema applied manually via `psql -U trasker -d trasker -f deploy/initdb/001_schema.sql`. 8 tables incl `layout_snapshots` (Phase 7).
- **Bootstrap admin:** `admin@localhost` / `trasker-admin` (force_password_change=true on first login)
- **JWT secret:** auto-generated and persisted to `/data/jwt-secret` on first boot (Phase 8 secret-loader)
- **OIDC:** unset (admin can configure via dashboard later)
- **TLS:** Traefik + Cloudflare cert resolver (no autocert needed; `TRASKER_FQDN` deliberately unset so server runs plain HTTP behind Traefik)
- **Persistence:** `~/docker/appdata/trasker/` (chowned 1001:1001 per Phase 8 Dockerfile non-root user)

### Smoke (server-side only)

- `GET https://trasker.nyolc.cc/api/v1/health` → `200 {"status":"ok",...}`
- `GET https://trasker.nyolc.cc/` → `200 <SPA index.html>` (embedded dashboard SPA served)
- `POST https://trasker.nyolc.cc/api/v1/layout-snapshots` (no auth) → `401` (auth gate enforced)
- `GET https://trasker.nyolc.cc/api/v1/auth/config` → `{"local_enabled":true,"oidc_enabled":false}`

## Hal-side files added (live on hal, not in repo)

- `~/docker/compose/hal2020/trasker.yml`
- `~/docker/appdata/traefik3/rules/hal2020/app-trasker.yml`
- `~/docker/.env` appended `TRASKER_TAG=dev`, `TRASKER_PG_PASSWORD=<generated>`
- `~/docker/docker-compose-hal2020.yml` include line added above SERVICE-PLACEHOLDER

These should eventually be committed to the deployrr unofficial repo or wherever Jaypaul keeps hal config — out of scope for this repo.

## End-to-end smoke (2026-05-07 23:18-23:30)

Driven from this machine against `https://trasker.nyolc.cc`.

**Setup:**
- Bootstrap admin password rotated via `/api/v1/auth/change-password` JWT call
- API key generated via internal/shared/apikey, hash inserted into hal Postgres (Builder service nil on hal — no Go toolchain in image; manual key insert is the bypass)
- Client built with `go build -ldflags "-X main.serverURL=https://trasker.nyolc.cc -X main.apiKey=tsk_820a..." -o /tmp/trasker-client-prod`

**Daemon run:**
- `TRASKER_LOG_LEVEL=debug DISPLAY=:0 /tmp/trasker-client-prod` ran ~12 minutes
- First-run setup created fresh sqlite at `~/.local/share/trasker/trasker.db`, registered device locally and with server
- Layout capturer started: "layout snapshots wired (capturer 60s, syncer 5m, prune 6h/7d)"

**Capture loop (D-07 change-detect):** 4 ticks fired in 4 minutes, 4 distinct windows_hash values — change-detect correctly snapshotted only when the window-set changed.

**Local SQLite verification:**
- 4 layout_snapshots rows, sizes 1645-1854 bytes
- Privacy gate: `SELECT DISTINCT key FROM json_each(value)` over `windows` JSONB returned exactly 6 keys: `app_name, window_title, x, y, w, h`. No `pid`, `cmdline`, `screenshot`, `exe_path`, `z_order`, `display_index`. (D-15, D-16)

**Server sync (5-min cadence):**
- First sync at 23:25:42 pushed 3 of 4 local rows
- Server postgres now has 3 rows, all `tier=raw`, all unique `windows_hash`
- Dedup unique constraint `(device_id, captured_at, windows_hash)` enforced

**Dashboard `/layout`:**
- Login UX has a real gap (see Issues below); JWT injected via localStorage to bypass
- Default view: 3 timestamp rows in the format `HH:MM:SS · N windows` matching UI-SPEC D-10
- Click-to-expand: 13-15 windows per row in the exact UI-SPEC format `{app_name} — {window_title} · {w}×{h} @ ({x}, {y})`
- Permalink: `https://trasker.nyolc.cc/layout?t=2026-05-07T21:23:32Z` rendered the right snapshot card with a `Copy permalink` button (D-11)
- Dark mode rendered correctly (UI-SPEC dark: pairings)
- Sidebar nav added "Layout" link, active state styled correctly

**Recursive evidence:** the very Claude conversation window driving the smoke appeared in the captured layout — title `"Save and restore window layout on power loss"`, app `Alacritty`. Phase 7 captured itself.

## Issues found during smoke

1. **Login UX gap (real bug).** When OIDC is unset, `/login` renders only the OIDC config message; no local-login email/password form is exposed. Workaround for smoke: `POST /api/v1/auth/local-login` via curl, inject `trasker_tokens` + `trasker_user` into `localStorage`. Should be a follow-up issue: SPA must surface local-login form when `auth/config.local_enabled === true`.
2. **Misleading sync error.** `internal/client/sync/client.go:163` maps any HTTP 401 to `ErrKeyExpired` ("API key expired — download a new client from your Trasker dashboard"). When the real error is `invalid API key`, the message lies to the user. Pre-existing Phase 5 bug, not Phase 7.
3. **Pre-existing focus_events ORDER syntax error.** Daemon logs `failed to close focus event` `SQL logic error: near "ORDER": syntax error (1)` on every focus tick. Pre-existing in Phase 5 focus_events code — predates this branch. Not Phase 7.
4. **`force_password_change=true` not enforced by SPA.** Initial admin login returned the flag; we changed the password via direct `/api/v1/auth/change-password` curl, which works regardless. SPA doesn't gate routes on this flag.
5. **Hash-truncation footgun.** Bcrypt hashes contain `$` chars; passing them through unquoted ssh shell strings truncates at the first `$` (got 30 chars stored vs 60 expected). Fix: scp the hash to a file on hal first, then `cat` it inside the heredoc. Not a bug in code, but worth documenting for future operator scripts.
6. **Manual schema-apply prereq.** Postgres only runs `/docker-entrypoint-initdb.d/*.sql` on a fresh data volume. hal's shared `postgresql` volume predates Phase 7, so we ran `psql -f deploy/initdb/001_schema.sql` after creating the `trasker` role+db. Server has no migration runner yet (RESEARCH.md A2 — Phase 8 deferred). Document in operator runbook.

## Steps NOT performed in this smoke

- **Screen-lock test (CONTEXT.md D-08).** Skipped — locking the active X11 session mid-Claude-session is unsafe. Future verification: lock for 90s, confirm no rows written during locked window, debug log shows "skipping tick (screen locked)".
- **24h soak.** Daemon ran ~12 min; long-soak observation deferred.
- **Backlog drain on server outage.** Stop server, accumulate local rows, restart, watch drain. Deferred.

## Hal-side files added (live on hal, not in repo)

- `~/docker/compose/hal2020/trasker.yml`
- `~/docker/appdata/traefik3/rules/hal2020/app-trasker.yml`
- `~/docker/.env` appended `TRASKER_TAG=dev`, `TRASKER_PG_PASSWORD=<generated>`
- `~/docker/docker-compose-hal2020.yml` include line added above SERVICE-PLACEHOLDER

These should eventually be committed to the deployrr unofficial repo or wherever Jaypaul keeps hal config — out of scope for this repo.

## Deferred (next session)

1. **Autostart on this machine.** systemd user unit so trasker-client survives reboots and starts on login.
2. **Tagged release.** Replace `:dev` GHCR push with a `v0.1.0` git-tag-driven CI build via `.github/workflows/release.yml` (Phase 8 added it).
3. **Backfill `pre-compiled binaries` directory.** Phase 8's builder loads from `/app/clients/`; this image was pushed without them. Server logs show `client downloads disabled`. CI release pipeline produces these; manual push doesn't.
4. **Fix login UX bug** — surface local-login form when OIDC is disabled. Issue #1 above.
5. **Fix focus_events ORDER syntax bug** — pre-existing Phase 5 bug, surfaced loudly during smoke.
6. **Fix sync client 401 → ErrKeyExpired mapping** — issue #2 above.
7. **macOS / Windows clients.** Out of scope for v1 (deferred per CONTEXT.md D-05).
8. **Wayland client.** Out of scope for v1 (deferred per CONTEXT.md D-04).

## Roadmap status

- Phase 1-6 (existing implementation): Complete (already on `main` before this session)
- Phase 7 (Layout Snapshots): Complete — code merged, manual smoke deferred
- Phase 8 (Production Deployment): Complete — code + image + CI workflow shipped
- Phase 9 (Multi-Device Dogfooding): Server cutover done; client-side cutover + 24h soak deferred to next session
