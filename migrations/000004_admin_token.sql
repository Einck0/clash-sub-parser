-- CSP 1.0 Admin Token Migration
-- Migration: 000004_admin_token.sql

-- Adds admin_token column to settings table.
-- SECURITY: Stores cryptographic password verifier/hash (e.g. bcrypt), NEVER usable plaintext secret.
ALTER TABLE settings ADD COLUMN admin_token TEXT NOT NULL DEFAULT '';
