-- Revert local authentication support.
DROP INDEX IF EXISTS idx_users_entra_oid;
ALTER TABLE users ADD CONSTRAINT users_entra_oid_key UNIQUE (entra_oid);
ALTER TABLE users ALTER COLUMN entra_oid SET NOT NULL;
ALTER TABLE users DROP COLUMN IF EXISTS force_password_change;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
