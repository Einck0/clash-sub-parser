-- Add credential_version column to nodes table
-- Migration: 000006_node_credential_version.sql

ALTER TABLE nodes ADD COLUMN credential_version INTEGER NOT NULL DEFAULT 0;
