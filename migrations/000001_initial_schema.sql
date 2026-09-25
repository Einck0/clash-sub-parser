-- CSP 1.0 Clean-Slate Initial Schema Migration
-- Migration: 000001_initial_schema.sql

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TEXT NOT NULL
);

-- 1. Subscriptions: mutable configuration for remote subscription sources
CREATE TABLE IF NOT EXISTS subscriptions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    source_url_secret_ref TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    refresh_interval_seconds INTEGER NOT NULL DEFAULT 86400,
    refresh_user_agent_policy TEXT NOT NULL DEFAULT '',
    refresh_fetch_proxy_ref TEXT NOT NULL DEFAULT '',
    refresh_timeout_seconds INTEGER NOT NULL DEFAULT 30,
    refresh_max_response_bytes INTEGER NOT NULL DEFAULT 10485760,
    revision TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_enabled ON subscriptions(enabled);
CREATE INDEX IF NOT EXISTS idx_subscriptions_updated_at ON subscriptions(updated_at);

-- 2. Subscription Fetches: immutable audit trail of fetch operations
CREATE TABLE IF NOT EXISTS subscription_fetches (
    id TEXT PRIMARY KEY,
    subscription_id TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    outcome TEXT NOT NULL,
    content_digest TEXT NOT NULL DEFAULT '',
    redacted_error TEXT NOT NULL DEFAULT '',
    nodes_parsed INTEGER NOT NULL DEFAULT 0,
    nodes_valid INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_subscription_fetches_sub_id ON subscription_fetches(subscription_id, started_at DESC);

-- 3. Nodes: normalized proxy nodes with non-secret stable logical_id as primary key
CREATE TABLE IF NOT EXISTS nodes (
    logical_id TEXT PRIMARY KEY,
    protocol TEXT NOT NULL,
    display_name TEXT NOT NULL,
    normalized_config_secret_ref TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nodes_protocol ON nodes(protocol);
CREATE INDEX IF NOT EXISTS idx_nodes_active ON nodes(active);
CREATE INDEX IF NOT EXISTS idx_nodes_updated_at ON nodes(updated_at);
CREATE INDEX IF NOT EXISTS idx_nodes_created_at ON nodes(created_at DESC, logical_id ASC);

-- 4. Node Sources: provenance association between nodes and subscriptions
CREATE TABLE IF NOT EXISTS node_sources (
    node_logical_id TEXT NOT NULL,
    subscription_id TEXT NOT NULL,
    last_seen_fetch_id TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (node_logical_id, subscription_id),
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE,
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_node_sources_sub ON node_sources(subscription_id);

-- 5. Probe Runs: bounded capability probing state machine
CREATE TABLE IF NOT EXISTS probe_runs (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    actor_scope TEXT NOT NULL DEFAULT 'default',
    config_revision TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL,
    deadline_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CONSTRAINT uq_probe_runs_idempotency UNIQUE (actor_scope, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_probe_runs_state ON probe_runs(state);
CREATE INDEX IF NOT EXISTS idx_probe_runs_created_at ON probe_runs(created_at);

-- 6. Probe Observations: immutable capability evidence records
CREATE TABLE IF NOT EXISTS probe_observations (
    id TEXT PRIMARY KEY,
    probe_run_id TEXT NOT NULL,
    node_logical_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    verdict TEXT NOT NULL,
    evidence_digest TEXT NOT NULL DEFAULT '',
    observed_at TEXT NOT NULL,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    redacted_summary TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (probe_run_id) REFERENCES probe_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_probe_obs_run ON probe_observations(probe_run_id);
CREATE INDEX IF NOT EXISTS idx_probe_obs_node ON probe_observations(node_logical_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_probe_obs_kind_verdict ON probe_observations(kind, verdict);

-- 7. Node Groups: policy routing groups
CREATE TABLE IF NOT EXISTS node_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    group_type TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_node_groups_name ON node_groups(name);

-- 8. Group Edges: directed edges in the policy graph with self-loop prevention
CREATE TABLE IF NOT EXISTS group_edges (
    id TEXT PRIMARY KEY,
    parent_group_id TEXT NOT NULL,
    child_group_id TEXT,
    node_logical_id TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (parent_group_id) REFERENCES node_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (child_group_id) REFERENCES node_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE,
    CHECK (parent_group_id != child_group_id),
    CHECK (
        (child_group_id IS NOT NULL AND node_logical_id IS NULL) OR
        (child_group_id IS NULL AND node_logical_id IS NOT NULL)
    ),
    CHECK (position >= 0)
);
CREATE INDEX IF NOT EXISTS idx_group_edges_parent ON group_edges(parent_group_id, position ASC);
CREATE INDEX IF NOT EXISTS idx_group_edges_child ON group_edges(child_group_id);
CREATE INDEX IF NOT EXISTS idx_group_edges_node ON group_edges(node_logical_id);

-- 9. Admission Rules: criteria for filtering and classifying nodes
CREATE TABLE IF NOT EXISTS admission_rules (
    id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    name TEXT NOT NULL,
    expression TEXT NOT NULL,
    action TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    CHECK (position >= 0)
);
CREATE INDEX IF NOT EXISTS idx_admission_rules_rev ON admission_rules(revision_id, position ASC);

-- 10. Policy Rules: rules steering traffic matching expressions to target groups
CREATE TABLE IF NOT EXISTS policy_rules (
    id TEXT PRIMARY KEY,
    revision_id TEXT NOT NULL,
    target_group_id TEXT NOT NULL,
    expression TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (target_group_id) REFERENCES node_groups(id) ON DELETE CASCADE,
    CHECK (position >= 0)
);
CREATE INDEX IF NOT EXISTS idx_policy_rules_rev ON policy_rules(revision_id, position ASC);
CREATE INDEX IF NOT EXISTS idx_policy_rules_target ON policy_rules(target_group_id);

-- 11. Configuration Revisions: immutable snapshots of configuration
CREATE TABLE IF NOT EXISTS configuration_revisions (
    id TEXT PRIMARY KEY,
    parent_id TEXT,
    content_digest TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (parent_id) REFERENCES configuration_revisions(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_config_rev_state ON configuration_revisions(state);
CREATE INDEX IF NOT EXISTS idx_config_rev_created_at ON configuration_revisions(created_at);

-- 12. Publications: immutable published configuration bundles
CREATE TABLE IF NOT EXISTS publications (
    id TEXT PRIMARY KEY,
    target TEXT NOT NULL,
    snapshot_digest TEXT NOT NULL,
    compiler_version TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TEXT NOT NULL,
    revoked_at TEXT,
    CONSTRAINT uq_publications_token_hash UNIQUE (token_hash)
);
CREATE INDEX IF NOT EXISTS idx_publications_target ON publications(target);
CREATE INDEX IF NOT EXISTS idx_publications_state ON publications(state);

-- 13. Settings: system parameter bounds and defaults (single row with id=1)
CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    probe_concurrency_window INTEGER NOT NULL DEFAULT 16,
    max_concurrent_probes INTEGER NOT NULL DEFAULT 16,
    probe_per_node_ttl_seconds INTEGER NOT NULL DEFAULT 300,
    fetch_timeout_seconds INTEGER NOT NULL DEFAULT 30,
    fetch_max_response_bytes INTEGER NOT NULL DEFAULT 10485760,
    max_page_size INTEGER NOT NULL DEFAULT 100,
    default_page_size INTEGER NOT NULL DEFAULT 50,
    updated_at TEXT NOT NULL
);

-- Seed default settings row if absent
INSERT OR IGNORE INTO settings (
    id,
    probe_concurrency_window,
    max_concurrent_probes,
    probe_per_node_ttl_seconds,
    fetch_timeout_seconds,
    fetch_max_response_bytes,
    max_page_size,
    default_page_size,
    updated_at
) VALUES (
    1,
    16,
    16,
    300,
    30,
    10485760,
    100,
    50,
    '2026-09-15T00:00:00Z'
);

-- 14. Audit Events: append-only operational action records
CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT PRIMARY KEY,
    actor_kind TEXT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    result TEXT NOT NULL,
    redacted_summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_events_action ON audit_events(action);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events(actor_kind);
