-- CSP 1.0 Real-time Subscription Links and Public Base URL Settings
-- Migration: 000022_subscription_links.sql

CREATE TABLE IF NOT EXISTS subscription_links (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL DEFAULT '',
    target TEXT NOT NULL,
    token TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'active',
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_subscription_links_token_hash ON subscription_links(token_hash);
CREATE INDEX IF NOT EXISTS idx_subscription_links_target ON subscription_links(target);
CREATE UNIQUE INDEX IF NOT EXISTS uq_subscription_links_default_target
    ON subscription_links(target)
    WHERE state = 'active' AND is_default = 1;

ALTER TABLE settings ADD COLUMN public_base_url TEXT NOT NULL DEFAULT '';
