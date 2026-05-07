-- Trasker layout snapshots (Phase 7).
-- Independent of focus_events; per CONTEXT.md D-14 the immutability invariant
-- on submitted timesheets is preserved by NOT linking snapshots to focus_events.
-- pgcrypto already enabled by migrations/001_init.up.sql; gen_random_uuid() is available.

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

-- Most queries: "most recent <= T for device". Covering index for that pattern.
CREATE INDEX idx_layout_snapshots_device_captured
    ON layout_snapshots (device_id, captured_at DESC);

-- Downsampling sweeps need rows by tier + age.
CREATE INDEX idx_layout_snapshots_tier_captured
    ON layout_snapshots (tier, captured_at);

-- Dedup on client retry double-sends (matches RESEARCH.md Open Question #3
-- recommendation: ON CONFLICT (device_id, captured_at, windows_hash) DO NOTHING).
CREATE UNIQUE INDEX idx_layout_snapshots_dedup
    ON layout_snapshots (device_id, captured_at, windows_hash);
