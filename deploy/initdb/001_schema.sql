-- Trasker initial schema
-- This migration creates all tables needed for the server.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- for gen_random_uuid()

CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entra_oid    TEXT UNIQUE NOT NULL,
    email        TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'member',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_user_role CHECK (role IN ('admin', 'manager', 'member'))
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

CREATE INDEX idx_api_keys_user ON api_keys(user_id);
CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);

CREATE TABLE devices (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    api_key_id       UUID NOT NULL REFERENCES api_keys(id),
    client_device_id TEXT NOT NULL,
    device_name      TEXT,
    os               TEXT NOT NULL,
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_devices_client_api ON devices(client_device_id, api_key_id);
CREATE INDEX idx_devices_user ON devices(user_id);

CREATE TABLE timesheets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id),
    device_id    UUID NOT NULL REFERENCES devices(id),
    submitted_at TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_timesheets_user ON timesheets(user_id);
CREATE INDEX idx_timesheets_submitted ON timesheets(submitted_at);

CREATE TABLE timesheet_entries (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timesheet_id UUID NOT NULL REFERENCES timesheets(id) ON DELETE CASCADE,
    tag          TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    ended_at     TIMESTAMPTZ NOT NULL,
    duration_s   INTEGER NOT NULL,
    notes        TEXT,
    app_summary  TEXT
);

CREATE INDEX idx_timesheet_entries_timesheet ON timesheet_entries(timesheet_id);
CREATE INDEX idx_timesheet_entries_tag ON timesheet_entries(tag);
CREATE INDEX idx_timesheet_entries_started ON timesheet_entries(started_at);

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

CREATE INDEX idx_audit_log_admin ON audit_log(admin_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_created ON audit_log(created_at);

CREATE TABLE org_settings (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    org_name        TEXT NOT NULL,
    entra_tenant    TEXT NOT NULL,
    entra_client    TEXT NOT NULL,
    key_expiry_days INTEGER NOT NULL DEFAULT 60,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Insert default org settings row (single-row table)
INSERT INTO org_settings (id, org_name, entra_tenant, entra_client)
VALUES (1, 'My Organization', '', '');
-- Add local authentication support.
-- password_hash stores bcrypt hash for local admin accounts.
-- entra_oid becomes nullable so local users don't need an Entra identity.
-- force_password_change flags accounts that must change password on next login.

ALTER TABLE users
    ADD COLUMN password_hash TEXT,
    ADD COLUMN force_password_change BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE users
    ALTER COLUMN entra_oid DROP NOT NULL;

-- Drop the unique constraint on entra_oid and re-add it as a partial unique index
-- (NULL values should not conflict).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_entra_oid_key;
CREATE UNIQUE INDEX idx_users_entra_oid ON users (entra_oid) WHERE entra_oid IS NOT NULL;
-- Add entra_secret to org_settings for full OIDC configuration from admin panel.
ALTER TABLE org_settings ADD COLUMN entra_secret TEXT NOT NULL DEFAULT '';

-- ===== Phase 7: layout_snapshots (mirrors migrations/004_layout_snapshots.up.sql) =====
-- Independent of focus_events; immutability invariant on submitted timesheets preserved.
-- pgcrypto already enabled above; gen_random_uuid() available.

CREATE TABLE layout_snapshots (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id    UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    captured_at  TIMESTAMPTZ NOT NULL,
    windows      JSONB NOT NULL,
    windows_hash TEXT NOT NULL,
    tier         TEXT NOT NULL DEFAULT 'raw',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_layout_tier CHECK (tier IN ('raw', '10min', '1hr'))
);

CREATE INDEX idx_layout_snapshots_device_captured
    ON layout_snapshots (device_id, captured_at DESC);
CREATE INDEX idx_layout_snapshots_tier_captured
    ON layout_snapshots (tier, captured_at);
CREATE UNIQUE INDEX idx_layout_snapshots_dedup
    ON layout_snapshots (device_id, captured_at, windows_hash);
