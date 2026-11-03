-- 000027_proxy_chain_bindings.sql
-- Proxy chain bindings persistence.
-- Stable references to targets and dialers, explicit priority order, optimistic revision tracking.
-- No foreign key cascade to volatile nodes, groups, or subscriptions.

CREATE TABLE IF NOT EXISTS proxy_chain_bindings (
    id TEXT PRIMARY KEY,
    target_type TEXT NOT NULL CHECK (target_type IN ('subscription', 'node_group', 'node')),
    target_id TEXT NOT NULL,
    dialer_type TEXT NOT NULL CHECK (dialer_type IN ('node', 'node_group')),
    dialer_id TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_proxy_chain_bindings_sort ON proxy_chain_bindings(sort_order, id);
