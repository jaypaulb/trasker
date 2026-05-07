# Roadmap: Trasker — Time Tracker

## Overview

Trasker's v1 journey is in three acts. **Act I (Phases 1–6, DONE):** the cross-platform client + server core were already built and merged to `main` at HEAD `21d3eca` before this roadmap was authored — focus tracking, presence, tagging, note cascade, submit flow, server dashboard with Entra OIDC, RBAC, API-key lifecycle, and full Linux/macOS/Windows platform implementations all exist in code. **Act II (Phase 7):** add layout-snapshots — a new feature surfaced after the original SPECs, capturing every open window + geometry every 60s and exposing a queryable history in the dashboard. **Act III (Phases 8–9):** stand the server up on `hal` at `trasker.nyolc.cc` using the 2-container autocert topology, then prove the product by multi-device dogfooding (Linux + macOS or Windows reporting to the same server, merged cross-device timeline + layout history). Multi-device dogfooding is the success metric for v1.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Shared Foundation** - Go module, shared models, apikey, version, Makefile, CI scaffolding (DONE on `main`)
- [x] **Phase 2: Server Core** - Postgres store, Entra OIDC, API-key middleware, Chi router, full REST endpoint set (DONE on `main`)
- [x] **Phase 3: Client Core** - Focus tracker, presence state machine, SQLite store, session engine + note cascade (DONE on `main`)
- [x] **Phase 4: Server Dashboard** - SvelteKit dashboard with login, personal/team views, devices, reports, admin (DONE on `main`)
- [x] **Phase 5: Client UI & Features** - Tagger, pomodoro, system tray, OS notifications, local SPA, sync/submit, first-run, autostart, dark mode (DONE on `main`)
- [x] **Phase 6: Cross-Platform & Integration** - macOS + Windows focus trackers, screen-lock, notifications, autostart; cross-compilation; integration tests (DONE on `main`)
- [ ] **Phase 7: Layout Snapshots** - Enumerate every open window + geometry every 60s, persist to SQLite, sync to server, show queryable history in dashboard
- [ ] **Phase 8: Production Deployment** - 2-container autocert topology deployed to `trasker.nyolc.cc` on hal; one-command compose UX; pre-compiled client binary distribution
- [ ] **Phase 9: Multi-Device Dogfooding** - Linux + macOS/Windows both reporting to the deployed server; merged cross-device timeline and layout history queryable in the dashboard

## Phase Details

### Phase 1: Shared Foundation
**Goal**: Single Go module monorepo bootstrapped with shared types, apikey utilities, version stamping, and a Makefile, ready for parallel server + client development.
**Depends on**: Nothing (first phase)
**Requirements**: REQ-shared-foundation
**Success Criteria** (what must be TRUE):
  1. `go build ./...` succeeds at repo root with both `cmd/trasker-client` and `cmd/trasker-server` skeletons present.
  2. `internal/shared/{models,apikey,version}` packages exist and are imported by both client and server.
  3. `make` runs the canonical build/test targets.
**Plans**: TBD (already executed pre-roadmap)
**Status**: Complete on `main` (commits leading up to `3efe07e`).

### Phase 2: Server Core
**Goal**: Production-shape Go API server backed by PostgreSQL with Entra OIDC for humans, bcrypt API keys for clients, RBAC, audit logging, and the full REST surface from the SPEC.
**Depends on**: Phase 1
**Requirements**: REQ-rbac, REQ-api-key-lifecycle
**Success Criteria** (what must be TRUE):
  1. Server starts against PostgreSQL 16 and migrations apply cleanly.
  2. Entra OIDC login flow issues a JWT that the dashboard accepts.
  3. Client API-key authentication via `Authorization` header resolves to a `(user_id, api_key_id)` tuple, with bcrypt verify and `last_used_at` updated.
  4. Admin/manager/member RBAC is enforced on every endpoint per the SPEC role table; unauthorized actions return 403.
  5. `audit_log` rows are written for every admin mutation (entry edit, key revoke, settings change).
**Plans**: TBD (already executed pre-roadmap)
**Status**: Complete on `main`.
**UI hint**: yes

### Phase 3: Client Core
**Goal**: Linux baseline daemon that watches the active window, runs the presence state machine, persists to local SQLite, and computes the note-cascade tag propagation on demand.
**Depends on**: Phase 1
**Requirements**: REQ-focus-tracking, REQ-presence-detection, REQ-note-cascade
**Success Criteria** (what must be TRUE):
  1. With the daemon running on Linux (X11 or Wayland), every active-window change appears as an immutable `focus_events` row within ~1s.
  2. Locking the screen flips presence to `AWAY`; unlocking returns to `TRACKING`. Hitting an inactivity interval (30/45/60/90/120 min) raises a 90s `CHECKING` countdown notification; ignoring it transitions to `PAUSED`.
  3. Adding a note to a focus event propagates the note's *tag* (not text) to neighboring events per the 60→30→15→7.5→<5 minute edge cascade, skipping already-tagged or already-submitted events.
**Plans**: TBD (already executed pre-roadmap)
**Status**: Complete on `main`.

### Phase 4: Server Dashboard
**Goal**: SvelteKit dashboard for humans — login, personal review, team view (manager+), devices, reports with CSV export, and admin (org settings, users, timesheet corrections, audit log).
**Depends on**: Phase 2
**Requirements**: REQ-server-dashboard
**Success Criteria** (what must be TRUE):
  1. User logs in via Entra and sees their personal dashboard with their submitted timesheets.
  2. Manager-role users see a Team view with direct reports' submissions.
  3. Admin can list users, revoke API keys, edit/remove timesheet entries (audit-logged), and view the audit log.
  4. Reports page produces CSV exports filterable by date range and tag.
**Plans**: TBD (already executed pre-roadmap)
**Status**: Complete on `main`. Note: built originally against the 4-container architecture; the SvelteKit content is unchanged by the 2026-03-26 deployment pivot, but the SPA is now `go:embed`-ed into the server binary in Phase 8.
**UI hint**: yes

### Phase 5: Client UI & Features
**Goal**: Everything that turns the client core into a daily-driver — rule-engine + learning tagger, pomodoro, tray, OS notifications, local SvelteKit dashboard, sync/submit with offline retry, first-run flow, autostart toggle, dark mode.
**Depends on**: Phase 3
**Requirements**: REQ-tagging-system, REQ-submit-flow, REQ-pomodoro, REQ-system-tray, REQ-autostart, REQ-first-run, REQ-local-dashboard, REQ-offline-resilience
**Success Criteria** (what must be TRUE):
  1. User opens `http://localhost:<port>` and lands in a Svelte SPA with Dashboard / Timeline / Submit / Pomodoro / Settings / Tags views, no auth required.
  2. User selects entries in Submit, gets the "submitted entries cannot be edited from the client" warning, confirms, and sees those entries leave the local pending list and appear on the server dashboard.
  3. With the server unreachable, submissions remain `pending` and retry on the 1m → 5m → 15m → 1h → hourly schedule; on API-key expiry the user sees a clear message linking to the dashboard.
  4. Tray icon reflects tracking + pomodoro state; tray menu can open dashboard, start pomodoro, toggle tracking, toggle autostart, quit.
  5. First-run produces a device UUID, registers idempotently with the server, prompts for autostart, and opens the localhost dashboard.
  6. Dashboard respects system dark/light preference and persists user override.
**Plans**: TBD (already executed pre-roadmap)
**Status**: Complete on `main` (dark mode merged at HEAD `21d3eca`).
**UI hint**: yes

### Phase 6: Cross-Platform & Integration
**Goal**: Bring macOS and Windows up to feature parity with Linux for focus, screen-lock, tray, notifications, autostart; lock down cross-compilation; cover the cross-platform paths with integration tests.
**Depends on**: Phase 5
**Requirements**: REQ-cross-platform-support
**Success Criteria** (what must be TRUE):
  1. `make` (or the documented build target) cross-compiles binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 with `CGO_ENABLED=0`.
  2. On macOS the daemon tracks focus via NSWorkspace + Accessibility API, detects screen lock via NSDistributedNotificationCenter, posts notifications via NSUserNotificationCenter, and installs autostart via LaunchAgent plist.
  3. On Windows the daemon tracks focus via SetWinEventHook + GetWindowText, detects screen lock via WTSRegisterSessionNotification, posts toasts via PowerShell, and installs autostart via the Run registry key.
  4. Integration tests in `tests/` cover the client→server flow and the build-pipeline stamping path on at least one platform in CI.
**Plans**: TBD (already executed pre-roadmap)
**Status**: Complete on `main` (commits `24c4c5e`, `1b9dcec`, `cd18168`, `56589bf`, `8ae203e`, `01dab44`, `30eefb9`, `ff066d6`, `27f4ed9`, `1151348`, `d43860d`).

### Phase 7: Layout Snapshots
**Goal**: Capture every open window + geometry every 60s on the client, sync the snapshots to the server, and let the user browse "what did my desktop look like at this time on this device" in the dashboard — per-device and merged across devices.
**Depends on**: Phase 6
**Requirements**: REQ-layout-snapshots
**Success Criteria** (what must be TRUE):
  1. On change (evaluated every 60s), a new `layout_snapshots` row is written to local SQLite — same set of `(app_name, window_title, x, y, w, h)` tuples produces no row, any change produces a row containing per-window `app_name`, `window_title`, x/y/width/height, and a UTC timestamp. Captured fields are exhaustive — no other per-window data is recorded; capture surface beyond geometry + identity is deferred.
  2. Layout snapshots sync to the server using the existing API-key auth path; on-device retention (7 days raw) and server retention (raw 0–7d → 10-min 7–37d → 1-hr 37+d) are independently scheduled; submitted-timesheet immutability is preserved (snapshots are independent of focus_events).
  3. Per-platform window enumeration works on Linux X11 (cgo + EWMH `_NET_CLIENT_LIST_STACKING` with `XQueryTree` fallback). Linux non-X11 sessions, macOS, and Windows are deferred and ship as `ErrUnsupported` stubs.
  4. The server dashboard exposes a "Layout History" view at `/layout` that lets the user pick a timestamp (default = today; permalinked instants reachable via `?t=<ISO-8601>`) and see the windows that were open then for the current device. Per-device only; multi-device merged mode is deferred.
  5. Layout-snapshot capture is privacy-preserving — it captures only `app_name + window_title + geometry`, never window contents, screenshots, PIDs, command lines, or focus *content* beyond what the existing focus tracker already stores.
**Plans**: TBD
**UI hint**: yes

### Phase 8: Production Deployment
**Goal**: Stand the 2-container autocert topology up at `trasker.nyolc.cc` on `hal` so a fresh download of `docker-compose.production.yml` + 4 env edits + `docker compose up -d` produces a working trusted-TLS server, with first-boot admin bootstrap and `bytes.Replace` client binary distribution working end-to-end.
**Depends on**: Phase 7
**Requirements**: REQ-production-deployment-ux, REQ-admin-bootstrap, REQ-client-binary-build-pipeline
**Success Criteria** (what must be TRUE):
  1. A user (Jaypaul) can download `docker-compose.production.yml` from the GitHub Release asset, edit `TRASKER_FQDN`, `TRASKER_ADMIN_EMAIL`, `TRASKER_ADMIN_PASSWORD`, and the postgres password, run `docker compose up -d`, and reach `https://trasker.nyolc.cc` with a Let's Encrypt-trusted cert within ACME's normal acquisition window.
  2. First boot with both admin env vars set produces an admin user with `force_password_change=false`; first boot with both unset produces `admin@localhost`/`trasker-admin` with `force_password_change=true`; setting exactly one logs a warning and skips bootstrap. Subsequent boots never re-run bootstrap.
  3. JWT secret loading follows env → `/data/jwt-secret` file → generate-and-write-0600 precedence; the server never hard-fails on a missing `TRASKER_JWT_SECRET`.
  4. Server admin clicks "Download Client" in the dashboard for a chosen target (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64) and receives a binary stamped with a freshly-generated API key + the server URL via `bytes.Replace` against the 128-byte sentinels — the binary connects back to the deployed server on first run without any further config.
  5. CI on a `v*` git tag cross-compiles all 5 client targets, builds the multi-stage runtime image (target ~50MB on alpine:3.20), pushes to `ghcr.io/<owner>/trasker:<tag>` and `:latest`, and creates a GitHub Release with `docker-compose.production.yml` attached.
  6. Smoke tests against `https://trasker.nyolc.cc` use `GET` (not `HEAD`) per the hal chain-oauth note, healthcheck on :8080 returns 200, and there is no top-level `networks:` block in the compose file (deployrr v5 constraint).
**Plans**: TBD
**UI hint**: yes

### Phase 9: Multi-Device Dogfooding
**Goal**: Validate v1 by running the deployed Trasker against Jaypaul's actual workflow on two or more machines, with the dashboard showing a coherent merged view of focus, presence, submitted timesheets, and layout snapshots across all devices.
**Depends on**: Phase 8
**Requirements**: REQ-multi-device-dogfooding
**Success Criteria** (what must be TRUE):
  1. Two or more of Jaypaul's machines (Linux + at least one of macOS or Windows) are registered with the deployed server at `trasker.nyolc.cc`, each with its own API-key-stamped binary obtained via the dashboard's Download Client flow.
  2. Each device produces focus_events, presence transitions, submitted timesheets, and `layout_snapshots` continuously over a multi-day period; offline retry survives a deliberate server outage and resyncs cleanly.
  3. The server dashboard shows a merged cross-device timeline that interleaves focus blocks from all of Jaypaul's devices on a single time axis, with per-device filtering available.
  4. Layout-history queries can be scoped to one device or merged across all devices for a given timestamp; results are correct against spot-checked ground truth.
  5. Jaypaul submits at least one full week of real timesheets through the deployed server and reports no data-loss, no privacy-policy violations (no keystroke/mouse/screenshot data ever), and no need to manually edit submitted entries from the client.
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9. Phases 1–6 are already complete on `main`; the next phase to plan is Phase 7.

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Shared Foundation | n/a (pre-roadmap) | Complete | 2026-03-23 (approx, from commit history) |
| 2. Server Core | n/a (pre-roadmap) | Complete | 2026-03-24 (approx) |
| 3. Client Core | n/a (pre-roadmap) | Complete | 2026-03-24 (approx) |
| 4. Server Dashboard | n/a (pre-roadmap) | Complete | 2026-03-24 (approx) |
| 5. Client UI & Features | n/a (pre-roadmap) | Complete | 2026-05-07 (dark mode merged at HEAD) |
| 6. Cross-Platform & Integration | n/a (pre-roadmap) | Complete | 2026-03-26 (approx) |
| 7. Layout Snapshots | 0/TBD | Not started | - |
| 8. Production Deployment | 0/TBD | Not started | - |
| 9. Multi-Device Dogfooding | 0/TBD | Not started | - |
