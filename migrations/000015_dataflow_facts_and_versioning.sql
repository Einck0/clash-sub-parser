-- Migration: 000015_dataflow_facts_and_versioning.sql
-- Implements upstream fact layer, node connection versioning, authoritative heads,
-- user overrides, publication payload reference protection, IP risk unification, and rules digest.

-- 1. Upstream Payloads: raw subscription response storage
CREATE TABLE IF NOT EXISTS subscription_payloads (
    id TEXT PRIMARY KEY,
    subscription_id TEXT NOT NULL,
    fetch_id TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    body_blob BLOB NOT NULL,
    http_status INTEGER NOT NULL DEFAULT 200,
    headers_json TEXT NOT NULL DEFAULT '{}',
    pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
    created_at TEXT NOT NULL,
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE,
    FOREIGN KEY (fetch_id) REFERENCES subscription_fetches(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sub_payloads_sub_created ON subscription_payloads(subscription_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sub_payloads_digest ON subscription_payloads(content_digest);

-- 2. Upstream Entries: parsed entries and notice evidence
CREATE TABLE IF NOT EXISTS subscription_entries (
    id TEXT PRIMARY KEY,
    payload_id TEXT NOT NULL,
    subscription_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    source_key TEXT NOT NULL,
    raw_name TEXT NOT NULL,
    protocol TEXT NOT NULL,
    server TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 0,
    entry_kind TEXT NOT NULL CHECK (entry_kind IN ('proxy', 'notice', 'unknown')),
    classification_reason TEXT NOT NULL DEFAULT '',
    classification_version TEXT NOT NULL DEFAULT 'v2-proven-combo',
    source_provenance_json TEXT NOT NULL DEFAULT '{}',
    user_kind_override TEXT CHECK (user_kind_override IN ('proxy', 'notice', 'unknown')),
    override_anchor TEXT NOT NULL DEFAULT '',
    override_reason TEXT NOT NULL DEFAULT '',
    override_at TEXT,
    actor_ref TEXT NOT NULL DEFAULT '',
    parsed_config_json TEXT NOT NULL DEFAULT '{}',
    parser_version TEXT NOT NULL DEFAULT '1.0.0',
    warnings_json TEXT NOT NULL DEFAULT '[]',
    node_logical_id TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (payload_id) REFERENCES subscription_payloads(id) ON DELETE CASCADE,
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE,
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE SET NULL,
    UNIQUE (payload_id, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_sub_entries_source_key ON subscription_entries(subscription_id, source_key);
CREATE INDEX IF NOT EXISTS idx_sub_entries_node ON subscription_entries(node_logical_id);

-- 3. Node Connection Versions: immutable connection parameter versions
CREATE TABLE IF NOT EXISTS node_connection_versions (
    node_logical_id TEXT NOT NULL,
    connection_revision INTEGER NOT NULL,
    effective_config_json TEXT NOT NULL,
    config_fingerprint TEXT NOT NULL,
    source_entry_id TEXT,
    schema_version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    PRIMARY KEY (node_logical_id, connection_revision),
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE,
    FOREIGN KEY (source_entry_id) REFERENCES subscription_entries(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_node_conn_ver_fingerprint ON node_connection_versions(config_fingerprint);

-- 4. Node Connection Heads: single authoritative head pointer per node
CREATE TABLE IF NOT EXISTS node_connection_heads (
    logical_id TEXT PRIMARY KEY,
    connection_revision INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE,
    FOREIGN KEY (logical_id, connection_revision) REFERENCES node_connection_versions(node_logical_id, connection_revision) ON DELETE RESTRICT
);

-- 5. Node Overrides: explicit user field-level overrides
CREATE TABLE IF NOT EXISTS node_overrides (
    node_logical_id TEXT NOT NULL,
    field_path TEXT NOT NULL,
    override_value_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (node_logical_id, field_path),
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE
);

-- 6. Publication Payload Refs: foreign key PIN protection against deletion
CREATE TABLE IF NOT EXISTS publication_payload_refs (
    publication_id TEXT NOT NULL,
    payload_id TEXT NOT NULL,
    PRIMARY KEY (publication_id, payload_id),
    FOREIGN KEY (publication_id) REFERENCES publications(id) ON DELETE CASCADE,
    FOREIGN KEY (payload_id) REFERENCES subscription_payloads(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_pub_payload_refs_payload ON publication_payload_refs(payload_id);

-- 7. Extend IP Risk Observations & Risk Policy Revisions
ALTER TABLE ip_risk_observations ADD COLUMN probe_observation_id TEXT REFERENCES probe_observations(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_ip_risk_obs_probe_obs ON ip_risk_observations(probe_observation_id);
ALTER TABLE risk_policy_revisions ADD COLUMN rules_digest TEXT NOT NULL DEFAULT '';

-- 8. Probe Observations latest index (formerly created dynamically at runtime)
CREATE INDEX IF NOT EXISTS idx_probe_obs_node_kind_observed_id
    ON probe_observations(node_logical_id, kind, observed_at DESC, id DESC);

-- 9. Initialize connection versions and authoritative heads for all existing nodes
INSERT OR IGNORE INTO node_connection_versions (
    node_logical_id,
    connection_revision,
    effective_config_json,
    config_fingerprint,
    schema_version,
    created_at
)
SELECT
    logical_id,
    CASE WHEN connection_revision > 0 THEN connection_revision ELSE 1 END,
    json_object('server', server, 'port', port, 'credentials', json(CASE WHEN config_json IS NULL OR config_json = '' THEN '{}' ELSE config_json END)),
    'legacy:' || logical_id || ':' || (CASE WHEN connection_revision > 0 THEN connection_revision ELSE 1 END),
    1,
    created_at
FROM nodes;

INSERT OR IGNORE INTO node_connection_heads (
    logical_id,
    connection_revision,
    updated_at
)
SELECT
    logical_id,
    CASE WHEN connection_revision > 0 THEN connection_revision ELSE 1 END,
    updated_at
FROM nodes;
