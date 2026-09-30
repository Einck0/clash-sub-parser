-- Migration: 000013_auth_settings.sql
-- Decoupled admin and export authentication settings

ALTER TABLE settings ADD COLUMN admin_auth_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE settings ADD COLUMN export_auth_enabled INTEGER NOT NULL DEFAULT 1;
