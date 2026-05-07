# Requirements Intel

No PRDs were ingested in this batch. This file is intentionally near-empty so downstream consumers (`gsd-roadmapper`) get a stable file path.

User-visible behaviors and acceptance signals are encoded narratively in the design SPECs. Below is a derived synthesis of what would become formal requirements; treat each as PRD-shaped extracts pending formal authoring. None of these have explicit acceptance-criteria language and so are NOT competing variants.

## Derived Requirements (extracted from SPECs and plans)

### REQ-focus-tracking
Description: The client daemon polls the active window every 1s and records `app_name + window_title + timestamps` as immutable `focus_events` rows in local SQLite.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Focus Tracker, Data Storage)

### REQ-presence-detection
Description: Presence is detected via OS screen-lock events and an escalating "deadman's switch" (30/45/60/90/120 min intervals, each with a 90s notification countdown). No keystroke/mouse monitoring.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Presence Detection)

### REQ-tagging-system
Description: Four-layer tagger evaluated in order: title-pattern rules → app-only rules → suggested rules → untagged. Manual tags override auto-rules. Rule-learning observes manual tagging frequency and surfaces suggestions.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Tagging System)

### REQ-note-cascade
Description: Notes anchor to a focus event; the note's TAG (not text) propagates outward in 60→30→15→7.5→<5 minute steps via edge-event propagation, skipping already-tagged or already-submitted events.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Note Cascade)

### REQ-submit-flow
Description: User selects entries in the local dashboard, confirms via dialog warning that submitted entries cannot be edited/deleted from the client, and the client aggregates events into tagged time blocks (with concatenated timestamped notes and an app-usage summary) sent to `POST /api/v1/timesheets`.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Submit Flow)

### REQ-pomodoro
Description: Configurable pomodoro timer (default 25/5), optional tag association, transition notifications, controllable from tray and local dashboard.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Pomodoro Timer)

### REQ-system-tray
Description: Per-platform tray icon showing tracking + pomodoro state; menu controls open dashboard, start pomodoro, toggle tracking, toggle autostart, quit.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (System Tray)

### REQ-autostart
Description: Binary self-manages OS autostart (Linux .desktop / macOS LaunchAgent / Windows Run registry) — toggleable from the tray, no installer.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Autostart)

### REQ-first-run
Description: Generate device UUID, create platform data dir, init SQLite, idempotently register device with server, start tray + tracking, open localhost dashboard, prompt for autostart.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (First Run)

### REQ-local-dashboard
Description: Svelte SPA served by the Go daemon on `localhost` with views for Dashboard, Timeline, Submit, Pomodoro, Settings, Tags. No auth (local-only).
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Local Dashboard)

### REQ-server-dashboard
Description: Server-side Svelte SPA with Login (Entra), personal dashboard, Team view (manager+), Devices, Reports (with CSV export), Admin (org settings, user mgmt, timesheet corrections, audit log).
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Server Dashboard)

### REQ-rbac
Description: Three roles — admin, manager, member — with permissions defined in the SPEC role table.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Roles)

### REQ-api-key-lifecycle
Description: Per-download API keys, bcrypt-hashed server-side, identifiable by `key_prefix`, expire after 60 days inactivity (configurable via `org_settings.key_expiry_days`), admin-revocable. One key may register multiple devices via the `(client_device_id, api_key_id)` upsert.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (API Key Lifecycle, Server schema)

### REQ-client-binary-build-pipeline
Description: Server admin clicks "Download Client" → server generates API key, cross-compiles (or patches a pre-compiled binary in production), stamps API key + server URL via `-ldflags -X` or `bytes.Replace` sentinels, returns binary.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Build Pipeline) + docs/superpowers/specs/2026-03-26-production-deployment-design.md (Client Binary Distribution)

### REQ-offline-resilience
Description: Client functions fully offline; submissions stay `pending` and retry with exponential backoff (1m, 5m, 15m, 1h, then hourly). On API key expiry, surface a clear message linking to the dashboard.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Error Handling)

### REQ-cross-platform-support
Description: Standalone binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64. Per-platform implementations of focus tracker, screen-lock, tray, notifications, autostart per the platform-support matrix.
source: docs/superpowers/specs/2026-03-23-trasker-design.md (Platform Support, Build & Distribution)

### REQ-production-deployment-ux
Description: End-user deploys via a single `docker-compose.production.yml` (downloaded from GitHub Releases), edits 4 values (FQDN, admin email, admin password, postgres password), runs `docker compose up -d`, visits `https://<fqdn>` — TLS acquired automatically.
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (User Experience)

### REQ-admin-bootstrap
Description: On first boot with no users in DB: if `TRASKER_ADMIN_EMAIL` and `TRASKER_ADMIN_PASSWORD` both set → create that admin with `force_password_change=false`; if both unset → create `admin@localhost` / `trasker-admin` with `force_password_change=true`; if exactly one set → log warning, skip bootstrap. Subsequent boots skip entirely.
source: docs/superpowers/specs/2026-03-26-production-deployment-design.md (Admin Bootstrap)
