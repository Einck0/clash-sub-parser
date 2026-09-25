-- CSP IP risk evidence and policy schema
-- Migration: 000002_ip_risk_schema.sql

-- Provider deployment settings contain references only and are safe to expose to no API.
CREATE TABLE IF NOT EXISTS ip_risk_provider_settings (
    provider TEXT NOT NULL CHECK (
        length(provider) > 0 AND length(provider) <= 64 AND
        provider NOT LIKE '%?%' AND
        provider NOT LIKE '%=%' AND
        provider NOT LIKE '%:%' AND
        provider NOT LIKE '%/%' AND
        provider NOT LIKE '%.%.%.%' AND
        lower(provider) NOT LIKE '%api_key%' AND
        lower(provider) NOT LIKE '%apikey%'
    ),
    schema_version TEXT NOT NULL CHECK (
        length(schema_version) > 0 AND length(schema_version) <= 64 AND
        schema_version NOT LIKE '%?%' AND
        schema_version NOT LIKE '%=%' AND
        schema_version NOT LIKE '%:%' AND
        schema_version NOT LIKE '%/%' AND
        schema_version NOT LIKE '%.%.%.%' AND
        lower(schema_version) NOT LIKE '%api_key%' AND
        lower(schema_version) NOT LIKE '%apikey%'
    ),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    secret_reference TEXT NOT NULL CHECK (secret_reference LIKE 'secret://%' OR secret_reference LIKE 'vault://%'),
    max_concurrency INTEGER NOT NULL CHECK (max_concurrency > 0),
    requests_per_minute INTEGER NOT NULL CHECK (requests_per_minute > 0),
    daily_request_budget INTEGER NOT NULL CHECK (daily_request_budget > 0),
    per_request_timeout_ms INTEGER NOT NULL CHECK (per_request_timeout_ms > 0),
    max_response_bytes INTEGER NOT NULL CHECK (max_response_bytes > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (provider, schema_version)
);
CREATE INDEX IF NOT EXISTS idx_ip_risk_provider_settings_enabled
    ON ip_risk_provider_settings(enabled, provider, schema_version);

-- IP risk observations are immutable, normalized provider facts.
CREATE TABLE IF NOT EXISTS ip_risk_observations (
    id TEXT PRIMARY KEY,
    node_logical_id TEXT NOT NULL,
    exit_identity_digest TEXT NOT NULL,
    provider TEXT NOT NULL CHECK (
        length(provider) > 0 AND length(provider) <= 64 AND
        provider NOT LIKE '%?%' AND
        provider NOT LIKE '%=%' AND
        provider NOT LIKE '%:%' AND
        provider NOT LIKE '%/%' AND
        provider NOT LIKE '%.%.%.%'
    ),
    provider_schema_version TEXT NOT NULL CHECK (
        length(provider_schema_version) > 0 AND length(provider_schema_version) <= 64 AND
        provider_schema_version NOT LIKE '%?%' AND
        provider_schema_version NOT LIKE '%=%' AND
        provider_schema_version NOT LIKE '%:%' AND
        provider_schema_version NOT LIKE '%/%' AND
        provider_schema_version NOT LIKE '%.%.%.%'
    ),
    observed_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('available', 'unknown', 'error', 'stale')),
    score INTEGER CHECK (score IS NULL OR (score >= 0 AND score <= 100)),
    confidence INTEGER CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 100)),
    network_class TEXT NOT NULL CHECK (network_class IN ('residential', 'datacenter', 'mobile', 'business', 'unknown')),
    anonymizer_traits TEXT NOT NULL DEFAULT '[]',
    evidence_digest TEXT NOT NULL,
    redacted_summary TEXT NOT NULL DEFAULT '' CHECK (
        length(redacted_summary) <= 512 AND
        redacted_summary NOT LIKE '%{%' AND
        redacted_summary NOT LIKE '%}%' AND
        redacted_summary NOT LIKE '%[%' AND
        redacted_summary NOT LIKE '%]%'
    ),
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE RESTRICT,
    FOREIGN KEY (provider, provider_schema_version)
        REFERENCES ip_risk_provider_settings(provider, schema_version)
);
CREATE INDEX IF NOT EXISTS idx_ip_risk_observations_node_observed
    ON ip_risk_observations(node_logical_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_ip_risk_observations_cache_key
    ON ip_risk_observations(node_logical_id, exit_identity_digest, provider, provider_schema_version, expires_at);
CREATE INDEX IF NOT EXISTS idx_ip_risk_observations_provider_status
    ON ip_risk_observations(provider, provider_schema_version, status, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_ip_risk_observations_latest
    ON ip_risk_observations(provider, provider_schema_version, node_logical_id, observed_at DESC, id DESC);

-- SQLite has no table-level immutable constraint, so reject both mutation paths explicitly.
CREATE TRIGGER IF NOT EXISTS ip_risk_observations_no_update
BEFORE UPDATE ON ip_risk_observations
BEGIN
    SELECT RAISE(ABORT, 'ip risk observations are append-only');
END;
CREATE TRIGGER IF NOT EXISTS ip_risk_observations_no_delete
BEFORE DELETE ON ip_risk_observations
BEGIN
    SELECT RAISE(ABORT, 'ip risk observations are append-only');
END;

-- Versioned local policy revisions keep provider semantics in child rows.
CREATE TABLE IF NOT EXISTS risk_policy_revisions (
    id TEXT PRIMARY KEY,
    fusion_mode TEXT NOT NULL CHECK (fusion_mode IN ('single_provider', 'all_must_allow', 'highest_risk')),
    max_observation_age_seconds INTEGER NOT NULL CHECK (max_observation_age_seconds > 0),
    minimum_confidence INTEGER NOT NULL CHECK (minimum_confidence >= 0 AND minimum_confidence <= 100),
    unknown_action TEXT NOT NULL DEFAULT 'review' CHECK (unknown_action IN ('allow', 'review', 'block')),
    conflict_action TEXT NOT NULL DEFAULT 'review' CHECK (conflict_action IN ('allow', 'review', 'block')),
    review_action TEXT NOT NULL DEFAULT 'review' CHECK (review_action IN ('allow', 'review', 'block')),
    active INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    activated_at TEXT,
    deactivated_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_risk_policy_revisions_single_active
    ON risk_policy_revisions(active) WHERE active = 1;

CREATE TABLE IF NOT EXISTS risk_policy_providers (
    revision_id TEXT NOT NULL,
    provider TEXT NOT NULL CHECK (
        length(provider) > 0 AND length(provider) <= 64 AND
        provider NOT LIKE '%?%' AND
        provider NOT LIKE '%=%' AND
        provider NOT LIKE '%:%' AND
        provider NOT LIKE '%/%' AND
        provider NOT LIKE '%.%.%.%' AND
        lower(provider) NOT LIKE '%api_key%' AND
        lower(provider) NOT LIKE '%apikey%'
    ),
    schema_version TEXT NOT NULL CHECK (
        length(schema_version) > 0 AND length(schema_version) <= 64 AND
        schema_version NOT LIKE '%?%' AND
        schema_version NOT LIKE '%=%' AND
        schema_version NOT LIKE '%:%' AND
        schema_version NOT LIKE '%/%' AND
        schema_version NOT LIKE '%.%.%.%' AND
        lower(schema_version) NOT LIKE '%api_key%' AND
        lower(schema_version) NOT LIKE '%apikey%'
    ),
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (revision_id, provider, schema_version),
    UNIQUE (revision_id, position),
    FOREIGN KEY (revision_id) REFERENCES risk_policy_revisions(id) ON DELETE CASCADE,
    FOREIGN KEY (provider, schema_version)
        REFERENCES ip_risk_provider_settings(provider, schema_version)
);
CREATE INDEX IF NOT EXISTS idx_risk_policy_providers_revision
    ON risk_policy_providers(revision_id, position ASC);

CREATE TABLE IF NOT EXISTS risk_policy_score_bands (
    revision_id TEXT NOT NULL,
    min_score INTEGER NOT NULL CHECK (min_score >= 0 AND min_score <= 100),
    max_score INTEGER NOT NULL CHECK (max_score >= 0 AND max_score <= 100 AND min_score <= max_score),
    band TEXT NOT NULL CHECK (band IN ('low', 'medium', 'high', 'critical')),
    action TEXT NOT NULL CHECK (action IN ('allow', 'review', 'block')),
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (revision_id, min_score, max_score),
    UNIQUE (revision_id, position),
    FOREIGN KEY (revision_id) REFERENCES risk_policy_revisions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_risk_policy_score_bands_revision
    ON risk_policy_score_bands(revision_id, min_score ASC);

CREATE TABLE IF NOT EXISTS risk_policy_trait_rules (
    revision_id TEXT NOT NULL,
    trait TEXT NOT NULL CHECK (trait IN ('proxy', 'vpn', 'tor', 'residential_proxy', 'hosting')),
    action TEXT NOT NULL CHECK (action IN ('allow', 'review', 'block')),
    PRIMARY KEY (revision_id, trait),
    FOREIGN KEY (revision_id) REFERENCES risk_policy_revisions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_risk_policy_trait_rules_revision
    ON risk_policy_trait_rules(revision_id, trait);

CREATE TABLE IF NOT EXISTS risk_policy_group_bindings (
    group_id TEXT PRIMARY KEY,
    policy_revision_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (group_id) REFERENCES node_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (policy_revision_id) REFERENCES risk_policy_revisions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_risk_policy_group_bindings_revision
    ON risk_policy_group_bindings(policy_revision_id);
