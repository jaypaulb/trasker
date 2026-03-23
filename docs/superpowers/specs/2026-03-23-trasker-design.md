# Trasker — Design Specification

**Date:** 2026-03-23
**Status:** Draft
**Author:** Jaypaul + Claude

## Overview

Trasker is a cross-platform time tracking tool that monitors active window focus to build a picture of what the user is working on. Users tag and annotate their focus entries locally, then selectively submit entries to a self-hosted server for team visibility and reporting.

**Philosophy:** Zero-config, user-empowering, non-invasive. Trasker is a tool for the user, not surveillance for management. The user controls what gets submitted. No keystroke or input monitoring.

## System Architecture

### Deployment Model

Self-hosted per organization. Each org deploys their own Trasker server via Docker Compose, configures their own Entra tenant, and manages their own users. The server generates pre-configured client binaries for download.

### Components

```
┌─────────────────────────────────────────────────────┐
│              TRASKER SERVER (Docker Compose)          │
│                                                      │
│  Go API Server ← PostgreSQL                          │
│  Svelte SPA (server dashboard)                       │
│  Caddy (reverse proxy + TLS)                         │
│  Build Pipeline (cross-compile client binaries)      │
└──────────────────────┬───────────────────────────────┘
                       │ HTTPS (submitted entries only)
┌──────────────────────┴───────────────────────────────┐
│              TRASKER CLIENT (per machine)              │
│                                                       │
│  Go Daemon (focus tracker, presence, tray, pomodoro)  │
│  SQLite (all local data)                              │
│  Local Web UI (localhost, opens in default browser)   │
│  Baked-in: API key + server URL                       │
│  Generated on first run: device ID                    │
└───────────────────────────────────────────────────────┘
```

### Data Sovereignty

- Client tracks ALL focus events locally in SQLite
- Server only receives what the user explicitly submits
- Client-side deletes do not propagate to the server
- Once submitted, entries are immutable from the client — only an org admin can modify or remove them via the server dashboard (with full audit trail)

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Client daemon | Go (cross-compiled per platform) |
| Client storage | SQLite via `modernc.org/sqlite` (pure Go, no CGo — simplifies cross-compilation) |
| Client UI | Svelte SPA served on localhost by Go |
| Server API | Go (Chi router) |
| Server storage | PostgreSQL 16 |
| Server dashboard | Svelte SPA (SvelteKit) |
| Server proxy | Caddy (auto-TLS) |
| Server deployment | Docker Compose |
| Auth | Entra ID (OIDC), configured per deployment |

## Repository Structure

Monorepo, single Go module.

```
trasker/
├── cmd/
│   ├── trasker-client/           # Client binary entrypoint
│   │   └── main.go
│   └── trasker-server/           # Server binary entrypoint
│       └── main.go
├── internal/
│   ├── client/
│   │   ├── tracker/              # Focus window tracking (per-platform)
│   │   ├── presence/             # Deadman's switch + screen lock
│   │   ├── session/              # Session engine + note cascade
│   │   ├── tagger/               # Auto-tag rules + learning engine
│   │   ├── pomodoro/             # Pomodoro timer
│   │   ├── tray/                 # System tray (per-platform)
│   │   ├── notify/               # OS notifications (per-platform)
│   │   ├── store/                # SQLite storage layer
│   │   ├── sync/                 # Server submission client
│   │   └── webui/                # Local HTTP server + embedded assets
│   ├── server/
│   │   ├── api/                  # HTTP handlers
│   │   ├── auth/                 # Entra OIDC
│   │   ├── builder/              # Cross-compile + stamp API keys
│   │   ├── store/                # PostgreSQL storage layer
│   │   ├── admin/                # Admin operations + audit log
│   │   └── reports/              # Aggregation queries
│   └── shared/
│       ├── models/               # API request/response types
│       ├── apikey/               # Key generation, hashing, validation
│       └── version/              # Version info (set via ldflags)
├── web/
│   ├── client-ui/                # Svelte SPA for local dashboard
│   └── server-ui/                # Svelte SPA for server dashboard
├── migrations/                   # PostgreSQL migrations
├── deploy/
│   ├── docker-compose.yml
│   ├── Dockerfile.server
│   ├── Dockerfile.builder
│   └── caddy/Caddyfile
├── go.mod
├── go.sum
└── Makefile
```

Platform-specific code uses Go build tags: `tracker_linux.go`, `tracker_darwin.go`, `tracker_windows.go`.

## Client Design

### Focus Tracker

Polls the active window every 1 second. Records app name + window title + timestamp.

**Platform implementations:**
- **Linux (X11):** `XGetInputFocus` + `XGetWindowProperty` for `_NET_WM_NAME` and `_NET_WM_PID`
- **Linux (Wayland):** `ext-foreign-toplevel-list-v1` (standardized — GNOME 46+, KDE 6+) as primary, `wlr-foreign-toplevel-management-unstable-v1` (Sway/wlroots compositors) as fallback. Ubuntu 24.04 uses GNOME, so `ext-foreign-toplevel` is the target. Note: the protocol provides the list of toplevels with `activated` state changes — track the `activated` event to determine which toplevel has focus.
- **macOS:** `NSWorkspace.shared.frontmostApplication` + Accessibility API for window title
- **Windows:** `SetWinEventHook` for `EVENT_SYSTEM_FOREGROUND` + `GetWindowText`

### Data Storage

**Store raw, display aggregated.** Every focus change is recorded in SQLite with full fidelity. The dashboard aggregates raw events into meaningful sessions using tags and time proximity. Users can drill into raw data if needed.

### Tagging System

Tags represent outcomes/activities, not apps. The same app can serve different purposes and receive different tags.

**Four layers, evaluated in order:**

1. **Title-pattern rules** — glob match on app name + window title (e.g., Terminal + `*claude*` → "Development")
2. **App-only rules** — match on app name alone (e.g., Slack → "Communication")
3. **Suggested rules** — system notices patterns and proposes rules (e.g., "You tag Terminal as 'Development' 80% of the time when the title contains 'claude' — make this automatic?")
4. **Untagged** — entries without a matching rule appear as "Untagged" in the dashboard

Users can always manually tag or re-tag entries. Manual tags override auto-rules.

**Tag rule learning:** The tagger tracks `hit_count` per rule and observes patterns in manual tagging. When it detects a consistent pattern (e.g., user manually tags Terminal entries containing "ssh" as "Admin" more than N times), it surfaces a suggestion.

### Note Cascade (Half-Life Decay)

When a user adds a note to a focus entry, the entry's **tag** cascades to temporally adjacent entries using a decaying radius. The note text itself stays on the anchor event — only the tag propagates. Notes from multiple anchors are aggregated at submission time.

1. Anchor entry: note applied directly
2. ±60 minutes from anchor: note cascades
3. ±30 minutes from those: note cascades
4. ±15 minutes: cascades
5. ±7.5 minutes: cascades
6. <5 minutes: stop

**Purpose:** Bridge brief interruptions. A 30-second Spotify skip in the middle of a coding session shouldn't force the user to label two separate blocks. But a 60+ minute gap (lunch, different task) creates a natural session boundary.

**Algorithm:** The cascade propagates outward in steps from the anchor event, not as concentric rings:

1. Start at anchor event. Find all events within ±60 minutes → tag them.
2. From each newly tagged event (not the anchor), find events within ±30 minutes → tag them.
3. From each newly tagged event in step 2, find events within ±15 minutes → tag them.
4. From step 3 events, ±7.5 minutes → tag them.
5. From step 4 events, <5 minutes → stop propagating.

Each step uses the *edge events* from the previous step as the new reference points. Already-tagged events are skipped (no re-processing). This means the effective reach depends on the density and spacing of events, not a fixed radius from the anchor.

The cascade is computed on demand when the note is added, not continuously. Each cascaded `event_tag` entry records its `source` ("cascade") and the `cascade_from` anchor event for traceability.

**Cascade and submitted entries:** Cascade skips events that have already been submitted. Submitted events are frozen — no new tags or notes can be applied to them. If a cascade reaches a submitted event, it stops propagating through that event but continues through other non-submitted neighbors.

### Presence Detection

No input monitoring (no keystroke/mouse tracking). Presence is detected via two signals:

**1. Screen lock / screensaver:**
- Detected via OS events (DBus on Linux, `NSWorkspace` on macOS, `WTSRegisterSessionNotification` on Windows)
- Screen lock → immediately stop tracking, mark time as dead time from last focus event
- Screen unlock → resume tracking, new focus event starts

**2. Deadman's switch (no focus change for extended period):**

Each notification IS the deadman's switch — a 90-second countdown.

| Check | Interval (no focus change) | Action |
|-------|---------------------------|--------|
| 1st | 30 minutes | Notification: "Still there?" — 90s to click |
| 2nd | 45 minutes | Same notification, 90s countdown |
| 3rd | 60 minutes | Same |
| 4th | 90 minutes | Same |
| 5th+ | 120 minutes (cap) | Same |

- **Click the notification** → tracking continues, interval escalates to next tier
- **Miss the 90s window** → tracking pauses, same as screen lock. Dead time marked from last confirmed presence.
- **Any focus change at any point** → resets the interval sequence back to 30 minutes

**State machine:**

```
TRACKING → (screen lock) → AWAY → (unlock) → TRACKING
TRACKING → (no focus change for interval) → CHECKING → (click) → TRACKING (next interval)
CHECKING → (90s expired) → PAUSED
PAUSED → (focus change) → resume prompt → TRACKING (reset to 30min)
```

### Tracking-Off Nag

When tracking is manually paused by the user:
- Tray icon shows ⏸ Paused
- On every focus window change, a non-intrusive notification: "Trasker is paused. Click to resume tracking."
- No sound, no modal — gentle persistent nudge
- User is always in control

### Pomodoro Timer

- Default: 25 min work / 5 min break
- Configurable intervals
- Optional tag association (tag a pomodoro session with an activity)
- Notifications for work→break and break→work transitions
- Managed from tray menu and local dashboard

### Submit Flow

1. User selects entries in the local dashboard (individually or batch)
2. Confirmation dialog: **"Submitted entries cannot be edited or deleted from your client. Only your org admin can modify or remove submitted entries. Are you sure?"**
3. On confirm: client aggregates selected focus events by tag into time blocks
4. Sends to server via `POST /api/v1/timesheets`
5. Client marks source focus events as "submitted" (visible but immutable locally)

**Submission payload** (what the server receives):
```json
{
  "client_device_id": "abc-123",
  "entries": [
    {
      "tag": "Development",
      "started_at": "2026-03-23T09:00:00Z",
      "ended_at": "2026-03-23T12:30:00Z",
      "duration_s": 12600,
      "notes": "Trasker client focus tracking",
      "app_summary": "VS Code (72%), Terminal (18%), Firefox (10%)"
    }
  ]
}
```

The server receives tagged time blocks with durations, notes, and an app-usage summary. It never sees raw focus events.

**Note aggregation:** When multiple notes exist within a submitted time block (from different anchor events), they are concatenated with timestamps: `"[09:15] Working on auth module\n[10:30] Switched to API tests"`. The most recent note appears last. This preserves the chronological narrative of the work session.

### System Tray

```
🟢 Tracking ON          │  ⏱ Pomodoro: 18:32
────────────────────────────────────────────
Open Dashboard...        (opens default browser)
Start Pomodoro           25m / 5m break
────────────────────────
● Tracking ON            toggle on/off
○ Start with OS          autostart toggle
────────────────────────
Quit Trasker
```

### Autostart

The binary manages its own autostart entries — no installer needed:

- **Linux:** `~/.config/autostart/trasker.desktop`
- **macOS:** `~/Library/LaunchAgents/com.trasker.client.plist`
- **Windows:** `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

Toggle via tray menu. Enable creates the entry, disable removes it.

### First Run

1. Generate device ID (UUID), store in SQLite `config` table
2. Create data directory (XDG on Linux, Application Support on macOS, %APPDATA% on Windows)
3. Initialize SQLite database
4. Register device with server: `POST /api/v1/devices { client_device_id, os, hostname }` — server upserts on `(client_device_id, api_key_id)`, so subsequent launches with the same device ID are idempotent
5. Start system tray icon
6. Start focus tracking
7. Open default browser to localhost dashboard with welcome screen
8. Prompt: "Start with OS?" (creates autostart entry if accepted)

Zero config. Zero typing. Baked-in API key and server URL handle auth automatically.

### Local Dashboard (Client Web UI)

Svelte SPA served by the Go daemon on `localhost:<port>`. Opens in default browser. No auth needed (local only).

**Views:**
- **Dashboard:** Today's activity by tag, time chart, current focus
- **Timeline:** Chronological list of focus events, tag/note management
- **Submit:** Select entries, review, confirm submission
- **Pomodoro:** Timer, history, statistics
- **Settings:** Autostart, presence intervals, tag rules, auto-tag management
- **Tags:** Create/edit tags, manage auto-tag rules, review suggestions

## Server Design

### Docker Compose Stack

```yaml
services:
  api:        # Go API server
  web:        # Svelte SPA (static files)
  postgres:   # PostgreSQL 16
  caddy:      # Reverse proxy + auto-TLS
```

### Authentication

Entra ID via OIDC. The org admin configures their tenant ID and client ID in the deployment config (environment variables or config file).

- Web dashboard: Entra OIDC login → JWT session token
- Client API: API key in `Authorization` header (baked into client binary)

### Roles

| Role | Permissions |
|------|------------|
| admin | Full access: user management, timesheet corrections, org settings, key revocation, audit log |
| manager | View team submissions, generate reports, view devices |
| member | Submit entries, view own submissions, manage own devices |

### API Endpoints

```
Auth:
  POST   /api/v1/auth/login          Entra OIDC callback
  POST   /api/v1/auth/refresh        Refresh JWT

Users (admin):
  GET    /api/v1/users               List users
  PATCH  /api/v1/users/:id           Update role

Devices:
  POST   /api/v1/devices             Register device (client first-run)
  GET    /api/v1/devices              List user's devices
  PATCH  /api/v1/devices/:id          Update device name

Timesheets:
  POST   /api/v1/timesheets           Submit entries
  GET    /api/v1/timesheets           List own submissions
  GET    /api/v1/timesheets/team      List team submissions (manager+)

Reports:
  GET    /api/v1/reports/summary      Time by tag, date range, user
  GET    /api/v1/reports/export       CSV export

Build:
  POST   /api/v1/build/download       Generate client binary with API key
  GET    /api/v1/build/status          Check build progress

Admin:
  DELETE /api/v1/admin/entries/:id     Delete submitted entry (audit logged)
  PATCH  /api/v1/admin/entries/:id     Edit submitted entry (audit logged)
  POST   /api/v1/admin/keys/:id/revoke  Revoke API key
  GET    /api/v1/admin/audit           View audit log
  GET    /api/v1/admin/settings        Org settings
  PATCH  /api/v1/admin/settings        Update org settings

Health:
  GET    /api/v1/health                Server health check
```

### Build Pipeline

When a user clicks "Download Client" on the server dashboard:

1. Server generates a unique API key, stores bcrypt hash in `api_keys` table
2. Triggers cross-compilation of the client binary for the selected OS/arch
3. Stamps the API key and server URL into the binary via `go build -ldflags "-X main.apiKey=... -X main.serverURL=..."`
4. Returns the binary as a download

**Build targets:** linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64

The server needs a Go cross-compilation environment (Docker container with Go toolchain). Using `modernc.org/sqlite` (pure Go) means cross-compilation only requires setting `GOOS`/`GOARCH` — no C toolchains needed.

### API Key Lifecycle

- Generated per download, bcrypt-hashed in database
- `key_prefix` (first 8 chars) stored for identification in admin UI
- `last_used_at` updated on every client sync/submission
- Expires after 60 days of inactivity (no API calls)
- On expiry: client shows "Your access key has expired. Please download a new client from your Trasker dashboard."
- Admin can revoke keys manually (lost device, offboarding)
- Multiple active keys per user (one per download). The same binary (same API key) can be run on multiple machines — each registers as a separate device, all linked to the same `api_key_id`. The server resolves the API key from the `Authorization` header to determine both `user_id` and `api_key_id` for device registration.

### Server Dashboard (Svelte SPA)

**Views:**
- **Login:** Entra sign-in
- **Dashboard:** Personal submitted time overview
- **Team:** Manager view — team submissions, time by person, by tag
- **Devices:** Manage registered devices, download new clients
- **Reports:** Charts (time by tag, by day, by person), filters, CSV export
- **Admin:** Org settings (Entra config, key expiry), user management, timesheet corrections, audit log

## Data Models

### Client — SQLite

```sql
-- Raw immutable log of every focus change
CREATE TABLE focus_events (
    id           INTEGER PRIMARY KEY,
    app_name     TEXT NOT NULL,
    window_title TEXT NOT NULL,
    started_at   TEXT NOT NULL,  -- ISO 8601
    ended_at     TEXT,           -- NULL if current
    duration_s   INTEGER,        -- computed on end
    is_idle      INTEGER NOT NULL DEFAULT 0,  -- dead time
    created_at   TEXT NOT NULL
);

-- User-defined activity tags
CREATE TABLE tags (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    color      TEXT NOT NULL,     -- hex
    created_at TEXT NOT NULL
);

-- Many-to-many: tags applied to focus events
CREATE TABLE event_tags (
    event_id     INTEGER NOT NULL REFERENCES focus_events(id),
    tag_id       INTEGER NOT NULL REFERENCES tags(id),
    source       TEXT NOT NULL,   -- "manual", "cascade", "auto_rule"
    cascade_from INTEGER,         -- anchor event_id (NULL if manual/auto)
    UNIQUE(event_id, tag_id)
);

-- Notes on entries (cascade anchors)
CREATE TABLE notes (
    id           INTEGER PRIMARY KEY,
    anchor_event INTEGER NOT NULL REFERENCES focus_events(id),
    text         TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    cascade_applied INTEGER NOT NULL DEFAULT 0  -- 1 = cascade has been computed from this note
);

-- Auto-tag rules
CREATE TABLE tag_rules (
    id            INTEGER PRIMARY KEY,
    tag_id        INTEGER NOT NULL REFERENCES tags(id),
    app_pattern   TEXT NOT NULL,     -- glob
    title_pattern TEXT,              -- glob (NULL = app-only)
    priority      INTEGER NOT NULL DEFAULT 0,
    suggested     INTEGER NOT NULL DEFAULT 0,
    hit_count     INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL
);

-- Submission tracking
-- Status lifecycle: "pending" (queued locally) → "submitted" (sent, awaiting confirm)
--                   → "confirmed" (server acknowledged)
-- If server is unreachable, submissions stay "pending" and retry with backoff.
CREATE TABLE submissions (
    id           INTEGER PRIMARY KEY,
    server_id    TEXT,              -- ID returned by server (NULL while pending)
    submitted_at TEXT NOT NULL,
    status       TEXT NOT NULL,     -- "pending", "submitted", "confirmed"
    retry_count  INTEGER NOT NULL DEFAULT 0,
    last_retry   TEXT               -- ISO 8601, NULL if not yet retried
);

CREATE TABLE submission_events (
    submission_id INTEGER NOT NULL REFERENCES submissions(id),
    event_id      INTEGER NOT NULL REFERENCES focus_events(id),
    UNIQUE(submission_id, event_id)
);

-- Pomodoro sessions
CREATE TABLE pomodoro_sessions (
    id         INTEGER PRIMARY KEY,
    started_at TEXT NOT NULL,
    ended_at   TEXT,
    work_mins  INTEGER NOT NULL DEFAULT 25,
    break_mins INTEGER NOT NULL DEFAULT 5,
    status     TEXT NOT NULL,  -- "work", "break", "done", "cancelled"
    tag_id     INTEGER REFERENCES tags(id),
    created_at TEXT NOT NULL
);

-- Client config (single row)
CREATE TABLE config (
    id                 INTEGER PRIMARY KEY CHECK (id = 1),
    device_id          TEXT NOT NULL,
    server_url         TEXT NOT NULL,
    api_key            TEXT NOT NULL,
    tracking_on        INTEGER NOT NULL DEFAULT 1,
    autostart          INTEGER NOT NULL DEFAULT 0,
    presence_intervals TEXT NOT NULL DEFAULT '[30,45,60,90,120]',
    pomodoro_defaults  TEXT NOT NULL DEFAULT '{"work":25,"break":5}'
);
```

### Server — PostgreSQL

```sql
CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entra_oid    TEXT UNIQUE NOT NULL,
    email        TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'member',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id),
    key_hash     TEXT NOT NULL,
    key_prefix   TEXT NOT NULL,
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked      BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ
);

CREATE TABLE devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    api_key_id      UUID NOT NULL REFERENCES api_keys(id),
    client_device_id TEXT NOT NULL,  -- client-generated UUID string
    device_name     TEXT,
    os              TEXT NOT NULL,
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Note: `client_device_id` is the string UUID generated by the client on first run.
-- `id` is the server's internal UUID PK. API payloads use `client_device_id`.
-- UNIQUE constraint ensures re-registration is idempotent (upsert on conflict).
CREATE UNIQUE INDEX idx_devices_client_api ON devices(client_device_id, api_key_id);

CREATE TABLE timesheets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    device_id       UUID NOT NULL REFERENCES devices(id),  -- server-side FK, resolved from client_device_id
    submitted_at    TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE timesheet_entries (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timesheet_id UUID NOT NULL REFERENCES timesheets(id),
    tag          TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    ended_at     TIMESTAMPTZ NOT NULL,
    duration_s   INTEGER NOT NULL,
    notes        TEXT,
    app_summary  TEXT
);

CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id    UUID NOT NULL REFERENCES users(id),
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id   UUID NOT NULL,
    old_value   JSONB,
    new_value   JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE org_settings (
    id              INTEGER PRIMARY KEY CHECK (id = 1),  -- single-row table
    org_name        TEXT NOT NULL,
    entra_tenant    TEXT NOT NULL,
    entra_client    TEXT NOT NULL,
    key_expiry_days INTEGER NOT NULL DEFAULT 60,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Cross-Cutting Concerns

### Timezone Convention

- Client SQLite stores all timestamps as ISO 8601 in **UTC** (e.g., `2026-03-23T14:30:00Z`)
- Server PostgreSQL uses `TIMESTAMPTZ` (stores UTC, renders in session timezone)
- Client UI converts UTC → local timezone for display only
- Submission payloads use UTC timestamps — the server never needs to know the client's timezone

### Error Handling

- Client operates fully offline if server is unreachable — submissions are created with status "pending" in the local `submissions` table
- Pending submissions retry with exponential backoff (1min, 5min, 15min, 1hr, then hourly). On success, status moves to "confirmed". The submission queue is processed on startup and periodically while running.
- On API key expiry: clear user-facing message, link to dashboard for new download
- On transient server errors (5xx, timeout): retry per backoff schedule above
- No silent fallbacks — errors surface to the user

### Security

- API keys are bcrypt-hashed server-side; plaintext only exists in the client binary
- API key generation only occurs after Entra login — the key is always linked to an existing `users` row. The download link is gated behind the authenticated dashboard, so there is no scenario where a client registers before its user exists.
- Client-to-server communication over HTTPS (Caddy auto-TLS)
- No sensitive data in focus events (app name + window title only, no content)
- SQLite database is local to the user's home directory with standard file permissions
- No input monitoring — no keystrokes, no mouse tracking, no screenshots

### Platform Support

| Platform | Focus Tracking | Screen Lock | Tray | Notifications | Autostart |
|----------|---------------|-------------|------|---------------|-----------|
| Ubuntu 24.04 (X11) | XGetInputFocus | DBus screensaver | systray | libnotify | .desktop |
| Ubuntu 24.04 (Wayland) | ext-foreign-toplevel | DBus screensaver | systray | libnotify | .desktop |
| macOS | NSWorkspace | NSWorkspace | systray | NSUserNotification | LaunchAgent |
| Windows | SetWinEventHook | WTSSession | systray | Toast | Registry Run |

### Build & Distribution

- Standalone binary per platform — no installer
- Server cross-compiles client binaries with baked-in API key via `-ldflags`
- Using `modernc.org/sqlite` (pure Go) eliminates CGo cross-compilation complexity — no platform-specific C toolchains needed
- Build environment runs in Docker for reproducibility

## Out of Scope (v1)

- Mobile clients (iOS/Android)
- Integrations with external tools (Jira, Linear, Harvest)
- Billing / subscription management
- Multi-org on a single server deployment
- Browser extension for tab-level tracking
- Automatic categorization via ML/AI
