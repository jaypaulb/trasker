# Trasker — Time Tracker

## What This Is

Trasker is a privacy-respecting, self-hosted time tracker. A cross-platform Go daemon runs on each user's machine, watches active-window focus + screen-lock presence (no keystrokes, no mouse, no screenshots), tags blocks of activity, and lets the user submit signed-off timesheets to a self-hosted Go server. The server provides an Entra-authenticated SvelteKit dashboard for personal review, manager team views, and admin/audit. Built for Jaypaul's own org first; production target is `trasker.nyolc.cc` on the `hal` Proxmox host.

## Core Value

**A time tracker the user actually trusts** — focus + presence only, never keystrokes/mouse/screenshots; submitted entries are immutable from the client; data lives on a server the org controls. If everything else fails, this property cannot.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. None yet — code exists at HEAD but has not been multi-device dogfooded against a deployed server, so nothing is "validated" by the success metric. -->

(None yet — code exists at HEAD but is awaiting production deployment + multi-device dogfooding before any requirement is treated as validated.)

### Active

<!-- Current scope. Building toward these. See REQUIREMENTS.md for the canonical list with IDs. -->

- [ ] Cross-platform focus + presence tracking with manual + auto-rule tagging
- [ ] Local Svelte dashboard with submit flow → server timesheets
- [ ] Server-side SvelteKit dashboard (Entra OIDC) + RBAC + admin/audit
- [ ] **NEW:** Layout snapshots — enumerate every open window + geometry every 60s, sync to server, browse historical layouts in dashboard
- [ ] 2-container production deployment to `trasker.nyolc.cc` (autocert TLS, single docker compose file)
- [ ] Multi-device dogfooding (Linux + macOS + Windows reporting to one hal-hosted server, merged cross-device timeline + layout history)

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- Mobile clients (iOS/Android) — desktop-focus is the product; mobile devices don't have the same "what window am I in" semantic
- External integrations (Jira, Linear, Harvest) — defer until v1 dogfooded
- Billing / subscription management — self-hosted-per-org model, no SaaS
- Multi-org on a single deployment — one deployment = one org by design
- Browser extensions for tab-level tracking — focus on app-level focus only in v1
- ML / AI categorization — rule-based tagger only in v1
- Keystroke / mouse / screenshot monitoring — explicit anti-feature; this is the trust property
- Editing submitted entries from the client — only audit-logged admin endpoints can mutate

## Context

- **Existing implementation:** A working client + server is already merged to `main` (HEAD `21d3eca`, dark mode merge). Focus tracking (X11/Wayland/macOS NSWorkspace+AX/Windows SetWinEventHook), presence detection, tagging, note cascade, pomodoro, system tray, OS notifications, autostart (Linux .desktop / macOS LaunchAgent / Windows Registry Run), local Svelte SPA, server SvelteKit dashboard, integration tests — all present.
- **Not deployed:** The 2026-03-26 production deployment plan is written (42 tasks) but 0/42 are done. The server has never run on `hal`.
- **New scope discovered after intel ingest:** Layout snapshots (window enumeration + geometry, not just active focus) — not in any existing SPEC, must be added as `REQ-layout-snapshots` and routed into its own phase before production deployment.
- **Architectural pivot in flight:** 2026-03-26 SPEC supersedes the 2026-03-23 SPEC for *deployment topology only* (4-container Caddy+Nginx+API+Postgres → 2-container trasker+Postgres with native autocert). The application/data model from 2026-03-23 is unchanged.
- **Doc conflict resolved by user:** 2026-03-26 SPEC is authoritative for deployment topology; 2026-03-23 SPEC is authoritative for app/domain content; older deployment sections are superseded.
- **Hal infra reference:** Existing chain-oauth services on hal return 500 to HEAD requests; smoke tests against the deployed server must use `curl -sI -X GET`. Compose files included into deployrr v5 must NOT have a top-level `networks:` block.
- **Distroless / non-root note:** If the production runtime image switches to distroless `:nonroot`, bind-mounts must be chowned 65532:65532, not 1000:1000.

## Constraints

- **Tech stack (locked by SPEC):** Client = Go + `modernc.org/sqlite` (pure-Go, no CGo) + Svelte SPA served on localhost; Server = Go (Chi router) + PostgreSQL 16 + SvelteKit dashboard; Auth = Entra ID OIDC for dashboard, bcrypt-hashed API keys for client.
- **Repository layout:** Single Go module monorepo — `cmd/trasker-{client,server}`, `internal/{client,server,shared}`, `web/{client-ui,server-ui}`, `migrations/`, `deploy/`. Platform-specific code uses build tags.
- **Platform support matrix:** linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64. Per-platform implementations of focus tracker, screen-lock, tray, notifications, autostart per the SPEC's platform matrix.
- **Timezones:** Client SQLite stores UTC ISO-8601; server uses `TIMESTAMPTZ`; UI converts UTC→local for display only; submission payloads are always UTC.
- **Privacy:** No keystroke / mouse / screenshot capture. Focus events store only `app_name + window_title`. SQLite DB lives in user home dir with standard file permissions.
- **Production architecture (locked by 2026-03-26 SPEC):** 2-container compose — `postgres` + `trasker` (Go server with embedded SPA + autocert TLS + pre-compiled client blobs). No Caddy, no Nginx.
- **Autocert listeners:** When `TRASKER_FQDN` is set → :443 (TLS via `golang.org/x/crypto/acme/autocert`), :80 (ACME HTTP-01 + redirect), :8080 (internal plain HTTP healthcheck). Three coordinated `http.Server` instances. Cert cache `/data/certs/`. Host whitelist = exactly the configured FQDN.
- **SPA embedding:** `internal/server/webui/embed.go` with `//go:embed all:static`. Catch-all route falls back to `index.html` for client-side routing. Multi-stage Dockerfile copies SPA build into the embed directory before `go build`.
- **Client binary distribution:** CI cross-compiles 5 targets with `CGO_ENABLED=0` and 128-byte null-padded sentinels (`SentinelServerURL`, `SentinelAPIKey`, `SentinelVersion`). Runtime patches via `bytes.Replace` (~50ms). Fallback to `go build` only if `/app/clients/` missing AND a Go toolchain is available.
- **JWT secret precedence:** `TRASKER_JWT_SECRET` env → `/data/jwt-secret` file (read or generate) → 32 random bytes hex-encoded if generating. Replaces hard-fail behavior.
- **Admin bootstrap matrix:** First boot only (skipped when users exist). Both env vars set → that admin, no force-change. Both unset → `admin@localhost`/`trasker-admin`, force-change. Exactly one set → log warning, skip.
- **CI pipeline:** Trigger on git tag `v*`. Three jobs: cross-compile 5 client binaries → multi-stage Docker build (node:22-slim → golang:1.25-bookworm → alpine:3.20 ~50MB) pushed to `ghcr.io/<owner>/trasker:<tag>` and `:latest` → GitHub Release with `docker-compose.production.yml` attached.
- **Server FQDN:** `trasker.nyolc.cc` on `hal`.

## Key Decisions

<!-- Decisions that constrain future work. Add throughout project lifecycle. -->

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Self-hosted-per-org, not multi-tenant SaaS | Trust property: org owns the server, the data, the keys | — Pending validation |
| Pure-Go SQLite (`modernc.org/sqlite`) on the client | CGo cross-compilation pain across linux/macOS/Windows is the largest historical blocker for Go desktop apps; pure-Go avoids it entirely | ✓ Good — 5-target cross-compile already shipping in repo |
| Entra OIDC for dashboard, bcrypt API keys for clients | Two different trust models: humans get SSO; daemons get long-lived rotatable tokens | — Pending validation |
| Submitted entries are immutable from the client | Trust property: once you sign off, you cannot retroactively edit your timesheet | — Pending validation |
| 4-container → 2-container deployment pivot (2026-03-26 SPEC) | Caddy + Nginx were operational complexity for no real benefit; Go server can do TLS + serve SPA + serve API in one binary | — Pending — never deployed yet |
| Native autocert TLS in the Go server (no external TLS tooling) | One container, one cert lifecycle, one log stream | — Pending — never deployed yet |
| Pre-compiled client binaries with sentinel `bytes.Replace` patching | Avoids needing a Go toolchain in production runtime image; ~50ms patch vs full `go build` per download | — Pending — never deployed yet |
| Layout snapshots as a distinct phase (not folded into focus tracking) | Different cadence (60s vs 1s), different schema, different sync semantics, and the feature was discovered after the original SPECs were written | — Pending |
| Multi-device dogfooding as the success metric | The product fails its core value if it cannot consolidate one user's activity across their actual workstations | — Pending |

---
*Last updated: 2026-05-07 after roadmap initialization from intel ingest*
