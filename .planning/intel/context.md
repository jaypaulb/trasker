# Context Intel

Running notes from the 8 DOC inputs (build sequence + 6 implementation plans + production deployment plan). These are non-authoritative — design SPECs in `constraints.md` outrank anything here when content disagrees.

---

## Topic: Build sequence and parallelization
source: docs/superpowers/plans/2026-03-23-00-build-sequence.md

Four-phase build order with explicit parallelization windows:

- **Phase 1 (sequential):** Plan 01 — Shared Foundation. Go module init, shared models, API contracts, apikey package, version, Makefile.
- **Phase 2 (parallel):** Plan 02 (Server Core) and Plan 03 (Client Core) run side-by-side; both depend only on Plan 01.
- **Phase 3 (parallel):** Plan 04 (Server Dashboard & Deploy) depends on Plan 02; Plan 05 (Client UI & Features) depends on Plan 03.
- **Phase 4 (sequential):** Plan 06 — Cross-Platform & Integration. Needs everything green.

Plans 02+03 touch disjoint directory trees (server/ vs client/). Plans 04+05 touch web/server-ui vs internal/client + web/client-ui — also disjoint. Suitable for an agent swarm.

---

## Topic: Shared foundation (Plan 01)
source: docs/superpowers/plans/2026-03-23-01-shared-foundation.md

Step-by-step bootstrap of:

- Go module initialization
- `cmd/trasker-client/` and `cmd/trasker-server/` skeleton entrypoints
- `internal/shared/version` — version info set via ldflags
- `internal/shared/models` — request/response DTOs
- `internal/shared/apikey` — key generation, hashing, validation
- `Makefile` — build targets and dev scripts
- Initial CI / build infrastructure

Cross-refs: none.

---

## Topic: Server core (Plan 02)
source: docs/superpowers/plans/2026-03-23-02-server-core.md

Implementation plan for the Go API server: PostgreSQL store, Entra OIDC auth, API key middleware, Chi router, full REST endpoint set defined in the design SPEC. Largest plan in the set (~159k, 5788 lines) — consider sub-decomposition before execution.

Cross-refs (human-readable, not file paths): "Plan 01 (shared foundation)", "Plan 03 (client code)", "Plan 04 (build pipeline)".

---

## Topic: Client core (Plan 03)
source: docs/superpowers/plans/2026-03-23-03-client-core.md

Implementation plan for the daemon core: focus tracker (Linux only initially — X11 + Wayland), presence detector, SQLite store, session engine including the note cascade. Linux-only baseline; macOS/Windows added in Plan 06.

Cross-ref: docs/superpowers/specs/2026-03-23-trasker-design.md.

---

## Topic: Server dashboard & deployment (Plan 04)
source: docs/superpowers/plans/2026-03-23-04-server-dashboard-deploy.md

Tasks for the SvelteKit server dashboard, TailwindCSS styling, Docker Compose deployment, Caddy reverse proxy, PostgreSQL migrations, Go cross-compilation pipeline.

**Note:** This plan is from the original 4-container architecture (Caddy + Nginx + API + Postgres). The newer production-deployment SPEC (2026-03-26) supersedes the deployment portion: Caddy and Nginx are removed, the Go server handles TLS via autocert and serves the SPA via `go:embed`. Use Plan 04 only for the SvelteKit dashboard build content; defer to the 2026-03-26 plan for deployment specifics.

Cross-ref: docs/superpowers/specs/2026-03-23-trasker-design.md.

---

## Topic: Client UI & features (Plan 05)
source: docs/superpowers/plans/2026-03-23-05-client-ui-features.md

Largest plan (~177k, 6827 lines). Covers: tagger (rule engine + learning), pomodoro, system tray, OS notifications, local web dashboard (SvelteKit SPA), server sync/submit, first-run flow, autostart toggling.

Cross-refs (informal): "Plan 01", "Plan 03", `internal/shared/models/`, `internal/client/store/`, `internal/client/tracker/`, `internal/client/presence/`, `internal/client/session/`, `internal/client/tagger/`.

---

## Topic: Cross-platform & integration (Plan 06)
source: docs/superpowers/plans/2026-03-23-06-cross-platform-integration.md

macOS focus tracker (NSWorkspace + Accessibility API), Windows focus tracker (Win32 SetWinEventHook + GetWindowText), CGo considerations (kept minimal — pure-Go SQLite avoids CGo for storage), cross-compilation verification, end-to-end integration testing across the three platforms.

Cross-ref: "Plans 01-05".

---

## Topic: Production deployment plan (Plan 26)
source: docs/superpowers/plans/2026-03-26-production-deployment-plan.md

Task-by-task migration to the 2-container production deployment described in the matching SPEC. Highlights:

- Add `internal/server/webui/embed.go` and SPA catch-all route
- Implement `internal/server/secrets/jwt.go` (LoadOrGenerateJWTSecret)
- Rewrite `cmd/trasker-server/main.go` for triple-listener autocert + JWT-from-file + env-driven admin bootstrap
- Rewrite `internal/server/builder/builder.go` to load pre-compiled binaries from `/app/clients/` instead of compiling at startup
- Rewrite `deploy/Dockerfile.server` as multi-stage production image
- Add `.github/workflows/release.yml` (5-target client cross-compile + GHCR image push + GitHub Release with compose template asset)
- Remove `deploy/docker-compose.yml`, `deploy/caddy/`, `deploy/nginx/`, `deploy/Dockerfile.builder`, `deploy/builder-entrypoint.sh`
- Preserve `deploy/docker-compose.local.yml` unchanged for local dev

Cross-ref: docs/superpowers/specs/2026-03-26-production-deployment-design.md.

---

## Topic: Architectural pivot to note (for downstream consumers)

The two SPEC documents represent **two snapshots in time**:

- **2026-03-23 design SPEC** describes a 4-container production stack (Caddy + Nginx + API + Postgres) — drove plans 00–06.
- **2026-03-26 production-deployment SPEC** explicitly supersedes the deployment portion of the 2026-03-23 SPEC, moving to a 2-container stack (Go server + Postgres) with native autocert TLS and embedded SPA.

The 2026-03-26 SPEC self-identifies as the replacement ("Replace the 4-container deployment ..."). It does NOT contradict the application/data-model portions of the older SPEC — only the deployment topology and the client-binary distribution pipeline. This is a deliberate revision, not a conflict, but downstream planners must treat the newer SPEC as authoritative for deployment content. See `INGEST-CONFLICTS.md` for the formal record.
