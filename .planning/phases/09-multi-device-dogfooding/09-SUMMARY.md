# Phase 9 — Multi-Device Dogfooding (cutover artifacts)

**Date:** 2026-05-07
**Status:** Server live on hal. Client-side cutover (autostart + smoke) deferred to next session per Jaypaul's request.

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

## Deferred (next session)

1. **Client cutover on this machine.** Build trasker-client with prod ldflags pointing at `https://trasker.nyolc.cc`, install autostart (systemd user unit), watch first 24h of layout snapshots flow.
2. **Change bootstrap admin password.** `admin@localhost` / `trasker-admin` is the well-known default; first login forces a change.
3. **End-to-end smoke** per Phase 7 plan 07-05 Task 3 — 8-step manual verification with real desktop activity.
4. **Tagged release.** Replace `:dev` GHCR push with a `v0.1.0` git-tag-driven CI build via `.github/workflows/release.yml` (Phase 8 added it).
5. **Backfill `pre-compiled binaries` directory.** Phase 8's builder loads from `/app/clients/`; this image was pushed without them. Server logs show `client downloads disabled`. CI release pipeline produces these; manual push doesn't.
6. **macOS / Windows clients.** Out of scope for v1 (deferred per CONTEXT.md D-05).
7. **Wayland client.** Out of scope for v1 (deferred per CONTEXT.md D-04).

## Roadmap status

- Phase 1-6 (existing implementation): Complete (already on `main` before this session)
- Phase 7 (Layout Snapshots): Complete — code merged, manual smoke deferred
- Phase 8 (Production Deployment): Complete — code + image + CI workflow shipped
- Phase 9 (Multi-Device Dogfooding): Server cutover done; client-side cutover + 24h soak deferred to next session
