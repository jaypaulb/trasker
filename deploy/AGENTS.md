# Deployment (deploy/)

## Purpose
Production and local Docker Compose stacks plus Dockerfiles for the self-hosted server. Target:
`trasker.nyolc.cc` on the `hal` Proxmox host. **One deployment = one org.**

## Files
- `docker-compose.production.yml` — 2-container stack: `postgres` + `trasker`.
- `docker-compose.local.yml` + `Dockerfile.server-local` — local dev stack.
- `Dockerfile.server` — multi-stage production build (node → golang → slim runtime).
- `initdb/001_schema.sql` — Postgres bootstrap schema (kept in sync with `../migrations/`).

## Locked architecture (2026-03-26 SPEC)
This SPEC **supersedes** the older 4-container design (Caddy + Nginx + API + Postgres) for
deployment topology only. Application/domain content from the 2026-03-23 SPEC is unchanged.

- **2 containers only:** `postgres` + `trasker`. No Caddy, no Nginx. The Go server does TLS, serves
  the embedded SPA, and serves the API in one binary.
- **Native autocert TLS.** When `TRASKER_FQDN` is set, the server runs three coordinated
  `http.Server`s: `:443` (TLS via `golang.org/x/crypto/acme/autocert`), `:80` (ACME HTTP-01 +
  redirect), `:8080` (internal plain-HTTP healthcheck). Cert cache `/data/certs/`; host whitelist =
  exactly the configured FQDN.
- **SPA embedding:** `internal/server/webui/embed.go` uses `//go:embed all:static`; the multi-stage
  Dockerfile copies the `server-ui` build into that dir before `go build`. Catch-all → `index.html`.
- **Client binaries are pre-compiled + sentinel-patched, not built per request.** CI cross-compiles 5
  targets (`CGO_ENABLED=0`) with 128-byte null-padded sentinels (`SentinelServerURL`, `SentinelAPIKey`,
  `SentinelVersion`). At download time `internal/server/builder` patches via `bytes.Replace` (~50ms).
  Fallback to `go build` only if `/app/clients/` is missing AND a Go toolchain is present.
- **JWT secret precedence:** `TRASKER_JWT_SECRET` env → `/data/jwt-secret` file (read or generate) →
  32 random bytes hex if generating. Never hard-fail on absence.
- **Admin bootstrap (first boot only, skipped once users exist):** both env vars set → that admin, no
  forced change; both unset → `admin@localhost` / `trasker-admin`, force-change; exactly one set → log
  a warning and skip.

## CI (.github/workflows/release.yml)
Triggers on git tag `v*`. Three jobs: cross-compile 5 client binaries → multi-stage Docker build
(node:22-slim → golang:1.25-bookworm → alpine:3.20, ~50MB) pushed to `ghcr.io/<owner>/trasker:<tag>`
and `:latest` → GitHub Release with `docker-compose.production.yml` attached.

## Gotchas (hal infra)
- Smoke-test with `curl -sI -X GET` — existing chain-oauth services on hal 500 on `HEAD`.
- Compose files pulled into deployrr v5 must **not** carry a top-level `networks:` block.
- If the runtime image switches to distroless `:nonroot`, bind-mounts must be chowned `65532:65532`,
  not `1000:1000`.

## Status
Written but **never deployed** — the production plan is 0/N done and the server has never run on hal.
Treat every "locked" behavior above as spec-intended, not battle-tested.

## Related
- Server internals that implement these: `../internal/server/AGENTS.md` (`builder/`, `secrets/`, `webui/`)
- Schema source of truth: `../migrations/`
