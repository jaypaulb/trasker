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
