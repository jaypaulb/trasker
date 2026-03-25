-- Add entra_secret to org_settings for full OIDC configuration from admin panel.
ALTER TABLE org_settings ADD COLUMN entra_secret TEXT NOT NULL DEFAULT '';
