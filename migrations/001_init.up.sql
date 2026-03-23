-- migrations/001_init.up.sql
-- Trasker server schema — initial migration

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

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
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    api_key_id       UUID NOT NULL REFERENCES api_keys(id),
    client_device_id TEXT NOT NULL,
    device_name      TEXT,
    hostname         TEXT,
    os               TEXT NOT NULL,
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_devices_client_api ON devices(client_device_id, api_key_id);

CREATE TABLE timesheets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id),
    device_id    UUID NOT NULL REFERENCES devices(id),
    submitted_at TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
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
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    org_name        TEXT NOT NULL,
    entra_tenant    TEXT NOT NULL,
    entra_client    TEXT NOT NULL,
    key_expiry_days INTEGER NOT NULL DEFAULT 60,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
