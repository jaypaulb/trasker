# Requirements: Trasker — Time Tracker

**Defined:** 2026-05-07
**Core Value:** A time tracker the user actually trusts — focus + presence only, never keystrokes/mouse/screenshots; submitted entries are immutable from the client; data lives on a server the org controls.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases. IDs reuse the `REQ-*` shape from the SPEC-derived intel (since the original SPECs did not bucket into traditional category prefixes). One new requirement, `REQ-layout-snapshots`, was added after intel ingest from user direction.

### Foundation (FND)

- [x] **REQ-shared-foundation**: Single Go module monorepo with `cmd/trasker-{client,server}`, `internal/{client,server,shared}`, shared models, apikey package, version package, Makefile, CI scaffolding.

### Server (SRV)

- [x] **REQ-server-dashboard**: Server-side SvelteKit SPA with Login (Entra), personal dashboard, Team view (manager+), Devices, Reports (with CSV export), Admin (org settings, user mgmt, timesheet corrections, audit log).
- [x] **REQ-rbac**: Three roles — admin, manager, member — with permissions per the SPEC role table.
- [x] **REQ-api-key-lifecycle**: Per-download API keys, bcrypt-hashed server-side, identifiable by `key_prefix`, expire after 60 days inactivity (configurable via `org_settings.key_expiry_days`), admin-revocable. One key may register multiple devices via the `(client_device_id, api_key_id)` upsert.

### Client Core (CLI)

- [x] **REQ-focus-tracking**: Client daemon polls the active window every 1s and records `app_name + window_title + timestamps` as immutable `focus_events` rows in local SQLite.
- [x] **REQ-presence-detection**: Presence detected via OS screen-lock events and an escalating "deadman's switch" (30/45/60/90/120 min intervals, each with a 90s notification countdown). No keystroke/mouse monitoring.
- [x] **REQ-note-cascade**: Notes anchor to a focus event; the note's TAG (not text) propagates outward in 60→30→15→7.5→<5 minute steps via edge-event propagation, skipping already-tagged or already-submitted events.

### Client UI & Features (UIF)

- [x] **REQ-tagging-system**: Four-layer tagger evaluated in order: title-pattern rules → app-only rules → suggested rules → untagged. Manual tags override auto-rules. Rule-learning observes manual tagging frequency and surfaces suggestions.
- [x] **REQ-submit-flow**: User selects entries in the local dashboard, confirms via dialog warning that submitted entries cannot be edited/deleted from the client, and the client aggregates events into tagged time blocks (with concatenated timestamped notes and an app-usage summary) sent to `POST /api/v1/timesheets`.
- [x] **REQ-pomodoro**: Configurable pomodoro timer (default 25/5), optional tag association, transition notifications, controllable from tray and local dashboard.
- [x] **REQ-system-tray**: Per-platform tray icon showing tracking + pomodoro state; menu controls open dashboard, start pomodoro, toggle tracking, toggle autostart, quit.
- [x] **REQ-autostart**: Binary self-manages OS autostart (Linux .desktop / macOS LaunchAgent / Windows Run registry) — toggleable from the tray, no installer.
- [x] **REQ-first-run**: Generate device UUID, create platform data dir, init SQLite, idempotently register device with server, start tray + tracking, open localhost dashboard, prompt for autostart.
- [x] **REQ-local-dashboard**: Svelte SPA served by the Go daemon on `localhost` with views for Dashboard, Timeline, Submit, Pomodoro, Settings, Tags. No auth (local-only).
- [x] **REQ-offline-resilience**: Client functions fully offline; submissions stay `pending` and retry with exponential backoff (1m, 5m, 15m, 1h, then hourly). On API key expiry, surface a clear message linking to the dashboard.

### Cross-Platform (XPF)

- [x] **REQ-cross-platform-support**: Standalone binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64. Per-platform implementations of focus tracker, screen-lock, tray, notifications, autostart per the platform-support matrix.

### Layout Snapshots (LAY) — NEW

- [ ] **REQ-layout-snapshots**: Client enumerates all open windows (per-display geometry, app, title, z-order) every 60s, persists snapshots to local SQLite, syncs them to the server, and the dashboard shows the user a queryable history of "what did my desktop look like at <time> on <device>" — viewable per-device and merged across devices.

### Production Deployment (DEP)

- [ ] **REQ-production-deployment-ux**: End-user (= Jaypaul, in the dogfooding case) deploys via a single `docker-compose.production.yml` (downloaded from GitHub Releases), edits 4 values (FQDN, admin email, admin password, postgres password), runs `docker compose up -d`, visits `https://<fqdn>` — TLS acquired automatically.
- [ ] **REQ-admin-bootstrap**: On first boot with no users in DB: if `TRASKER_ADMIN_EMAIL` and `TRASKER_ADMIN_PASSWORD` both set → create that admin with `force_password_change=false`; if both unset → create `admin@localhost` / `trasker-admin` with `force_password_change=true`; if exactly one set → log warning, skip bootstrap. Subsequent boots skip entirely.
- [ ] **REQ-client-binary-build-pipeline**: Server admin clicks "Download Client" → server stamps API key + server URL into a pre-compiled client binary via `bytes.Replace` against 128-byte null-padded sentinels (~50ms), returns binary. CI cross-compiles 5 targets with `CGO_ENABLED=0` and bakes the sentinels via `-ldflags -X`. Runtime falls back to `go build` only if `/app/clients/` missing AND a Go toolchain is available.

### Multi-Device Dogfooding (DOG)

- [ ] **REQ-multi-device-dogfooding**: Two or more of Jaypaul's machines (Linux + at least one of macOS/Windows) report focus events, presence, submitted timesheets, and layout snapshots to the deployed server at `trasker.nyolc.cc`. The server dashboard shows a merged cross-device timeline and lets Jaypaul query layout history filtered by device or merged across all devices. This is the primary success metric.

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

(None — anything not in v1 above is in Out of Scope below.)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Mobile clients (iOS/Android) | Desktop focus is the product; mobile lacks the "what window am I in" semantic |
| External integrations (Jira, Linear, Harvest) | Defer until v1 dogfooded |
| Billing / subscription management | Self-hosted-per-org model, no SaaS |
| Multi-org on a single deployment | One deployment = one org by design |
| Browser extensions for tab-level tracking | App-level focus only in v1 |
| ML / AI categorization | Rule-based tagger only in v1 |
| Keystroke / mouse / screenshot monitoring | Explicit anti-feature; trust property |
| Editing submitted entries from the client | Only audit-logged admin endpoints can mutate |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| REQ-shared-foundation | Phase 1 | Complete |
| REQ-server-dashboard | Phase 4 | Complete |
| REQ-rbac | Phase 2 | Complete |
| REQ-api-key-lifecycle | Phase 2 | Complete |
| REQ-focus-tracking | Phase 3 | Complete |
| REQ-presence-detection | Phase 3 | Complete |
| REQ-note-cascade | Phase 3 | Complete |
| REQ-tagging-system | Phase 5 | Complete |
| REQ-submit-flow | Phase 5 | Complete |
| REQ-pomodoro | Phase 5 | Complete |
| REQ-system-tray | Phase 5 | Complete |
| REQ-autostart | Phase 5 | Complete |
| REQ-first-run | Phase 5 | Complete |
| REQ-local-dashboard | Phase 5 | Complete |
| REQ-offline-resilience | Phase 5 | Complete |
| REQ-cross-platform-support | Phase 6 | Complete |
| REQ-layout-snapshots | Phase 7 | Pending |
| REQ-production-deployment-ux | Phase 8 | Pending |
| REQ-admin-bootstrap | Phase 8 | Pending |
| REQ-client-binary-build-pipeline | Phase 8 | Pending |
| REQ-multi-device-dogfooding | Phase 9 | Pending |

**Coverage:**
- v1 requirements: 21 total
- Mapped to phases: 21
- Unmapped: 0 ✓

**Note on completion status:** Phases 1–6 are marked `Complete` here because the corresponding code is on `main` at HEAD `21d3eca`. They have not been *validated* in the PROJECT.md sense — that requires multi-device dogfooding against a deployed server, which is Phase 9. Marking them `Complete` reflects "the code has been written and merged"; marking a requirement *Validated* in PROJECT.md requires shipping against the success metric.

---
*Requirements defined: 2026-05-07*
*Last updated: 2026-05-07 after initial definition + intel ingest + REQ-layout-snapshots addition*
