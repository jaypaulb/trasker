-- Trasker initial schema — rollback
-- WARNING: This drops all tables and data.

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS timesheet_entries;
DROP TABLE IF EXISTS timesheets;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS org_settings;
