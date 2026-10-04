-- Migration: 000016_node_source_history.sql
-- Implements independent immutable provenance ledger for node source attribution.
-- Strict DDL only: establishes node_source_history table and idempotency indices.

CREATE TABLE IF NOT EXISTS node_source_history (
    id TEXT PRIMARY KEY,
    node_logical_id TEXT NOT NULL,
    subscription_id TEXT,
    source_identity TEXT NOT NULL,
    source_label TEXT NOT NULL,
    connection_revision INTEGER,
    relation_state TEXT NOT NULL CHECK (relation_state IN ('verified', 'conflict', 'unknown')),
    cause TEXT NOT NULL CHECK (cause IN ('legacy_import', 'refresh_removed', 'subscription_deleted', 'manual_confirmed', 'unresolved')),
    first_observed_at TEXT,
    last_observed_at TEXT,
    evidence_kind TEXT NOT NULL,
    evidence_key TEXT NOT NULL DEFAULT '',
    evidence_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE,
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_node_source_hist_unique ON node_source_history(node_logical_id, evidence_key);
CREATE INDEX IF NOT EXISTS idx_node_source_hist_node ON node_source_history(node_logical_id, last_observed_at DESC, first_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_source_hist_sub ON node_source_history(subscription_id);
CREATE INDEX IF NOT EXISTS idx_node_source_hist_identity ON node_source_history(source_identity);
