-- Preparation only. No survivor selection, backfill, DELETE or readiness seal.
-- Legacy conversion must be performed explicitly on authorized offline scratch.
CREATE TABLE subscription_payload_evidence (
    payload_id TEXT PRIMARY KEY,
    subscription_id TEXT REFERENCES subscriptions(id) ON DELETE SET NULL,
    source_identity TEXT NOT NULL,
    fetch_id TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    original_created_at TEXT NOT NULL,
    captured_at TEXT NOT NULL,
    http_status INTEGER NOT NULL,
    evidence_kind TEXT NOT NULL,
    quality TEXT NOT NULL CHECK (quality IN ('observed','unknown')),
    pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0,1))
);
CREATE TABLE node_connection_provenance (
    node_logical_id TEXT NOT NULL REFERENCES nodes(logical_id) ON DELETE CASCADE,
    connection_revision INTEGER NOT NULL,
    origin_payload_id TEXT NOT NULL REFERENCES subscription_payload_evidence(payload_id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (node_logical_id, connection_revision, origin_payload_id)
);
CREATE TABLE node_source_observations (
    node_logical_id TEXT NOT NULL REFERENCES nodes(logical_id) ON DELETE CASCADE,
    source_identity TEXT NOT NULL,
    subscription_id TEXT REFERENCES subscriptions(id) ON DELETE SET NULL,
    raw_name TEXT NOT NULL,
    protocol TEXT NOT NULL,
    source_key TEXT NOT NULL,
    observed_at TEXT,
    PRIMARY KEY (node_logical_id, source_identity)
);
CREATE TABLE subscription_current_state (
    subscription_id TEXT PRIMARY KEY REFERENCES subscriptions(id) ON DELETE CASCADE,
    source_revision TEXT NOT NULL,
    applied_payload_id TEXT REFERENCES subscription_payloads(id) ON DELETE SET NULL,
    applied_fetch_id TEXT REFERENCES subscription_fetches(id) ON DELETE RESTRICT,
    last_attempt_fetch_id TEXT REFERENCES subscription_fetches(id) ON DELETE RESTRICT,
    last_attempt_status TEXT NOT NULL DEFAULT '',
    attempt_sequence INTEGER NOT NULL DEFAULT 0,
    applied_at TEXT
);
CREATE TABLE node_connection_version_leases (
    consumer_kind TEXT NOT NULL,
    consumer_id TEXT NOT NULL,
    node_logical_id TEXT NOT NULL,
    connection_revision INTEGER NOT NULL,
    owner TEXT NOT NULL,
    fencing_token TEXT NOT NULL,
    heartbeat_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (consumer_kind, consumer_id, node_logical_id, connection_revision),
    FOREIGN KEY (node_logical_id, connection_revision)
        REFERENCES node_connection_versions(node_logical_id, connection_revision) ON DELETE RESTRICT
);
CREATE TABLE subscription_storage_readiness (
    singleton INTEGER PRIMARY KEY CHECK (singleton=1),
    contract_version INTEGER NOT NULL,
    consumers_closed INTEGER NOT NULL DEFAULT 0 CHECK (consumers_closed IN (0,1)),
    sealed_at TEXT NOT NULL
);
