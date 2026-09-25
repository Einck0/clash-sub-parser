-- Migration: 000008_node_filters.sql
-- Global and group-level node filters storage

-- 1. Global Node Filter (single-row table with id = 1)
CREATE TABLE IF NOT EXISTS global_node_filters (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    filter_spec TEXT NOT NULL DEFAULT '{"conditions":[]}',
    updated_at TEXT NOT NULL
);

-- Seed default global node filter (empty conditions = pass-through / always true)
INSERT OR IGNORE INTO global_node_filters (id, filter_spec, updated_at)
VALUES (1, '{"conditions":[]}', datetime('now'));

-- 2. Group Node Filters (per-group filter binding)
CREATE TABLE IF NOT EXISTS group_node_filters (
    group_id TEXT PRIMARY KEY,
    filter_spec TEXT NOT NULL DEFAULT '{"conditions":[]}',
    updated_at TEXT NOT NULL,
    FOREIGN KEY (group_id) REFERENCES node_groups(id) ON DELETE CASCADE
);

-- 3. Composite index on node_sources for bulk node lookups
CREATE INDEX IF NOT EXISTS idx_node_sources_node_logical_id ON node_sources(node_logical_id);
