-- Migration: 000029_subscription_original_configs.sql
-- Standalone one-row raw original subscription configuration table.

CREATE TABLE IF NOT EXISTS subscription_original_configs (
    subscription_id TEXT PRIMARY KEY,
    source_revision TEXT NOT NULL DEFAULT '',
    content_digest TEXT NOT NULL DEFAULT '',
    body_blob BLOB NOT NULL CHECK (length(body_blob) >= 1),
    fetched_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    content_type TEXT NOT NULL DEFAULT '',
    fetch_id TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
);
