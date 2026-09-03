CREATE TABLE config_snapshots (
    id INTEGER NOT NULL,
    label VARCHAR(200),
    description VARCHAR(500),
    snapshot_data TEXT NOT NULL,
    created_at DATETIME,
    PRIMARY KEY (id)
);
CREATE INDEX ix_config_snapshots_id ON config_snapshots (id);

CREATE TABLE dns_config (
    id INTEGER NOT NULL,
    raw_yaml TEXT NOT NULL,
    enabled BOOLEAN NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE generate_config (
    id INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL,
    subscriptions BOOLEAN NOT NULL,
    node_groups BOOLEAN NOT NULL,
    rules BOOLEAN NOT NULL,
    dns BOOLEAN NOT NULL,
    exclude_node_proxies BOOLEAN NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE node_groups (
    id INTEGER NOT NULL,
    name VARCHAR(120) NOT NULL,
    kind VARCHAR(20) NOT NULL,
    group_type VARCHAR(20) NOT NULL,
    sort_order INTEGER NOT NULL,
    regex_rules JSON NOT NULL,
    filter_min_speed_mbps FLOAT,
    filter_media_unlock JSON NOT NULL,
    include_nodes JSON NOT NULL,
    include_group_ids JSON NOT NULL,
    include_group_nodes_ids JSON NOT NULL,
    include_entries JSON NOT NULL,
    add_fallback BOOLEAN NOT NULL,
    exclude_nodes JSON NOT NULL,
    exclude_group_ids JSON NOT NULL,
    url_test_config JSON NOT NULL,
    load_balance_config JSON NOT NULL,
    fallback_config JSON NOT NULL,
    PRIMARY KEY (id),
    UNIQUE (name)
);
CREATE INDEX ix_node_groups_id ON node_groups (id);

CREATE TABLE node_probe_results (
    id INTEGER NOT NULL,
    node_key VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    server VARCHAR(255) NOT NULL,
    port INTEGER,
    type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    latency_ms INTEGER,
    speed_mbps FLOAT,
    ip VARCHAR(128),
    country VARCHAR(32),
    asn INTEGER,
    organization VARCHAR(255),
    media JSON NOT NULL,
    error TEXT,
    checked_at INTEGER NOT NULL,
    PRIMARY KEY (id)
);
CREATE INDEX ix_node_probe_results_checked_at ON node_probe_results (checked_at);
CREATE INDEX ix_node_probe_results_name ON node_probe_results (name);
CREATE UNIQUE INDEX ix_node_probe_results_node_key ON node_probe_results (node_key);
CREATE INDEX ix_node_probe_results_server ON node_probe_results (server);

CREATE TABLE probe_config (
    id INTEGER NOT NULL,
    probe_enabled BOOLEAN NOT NULL,
    probe_interval_minutes INTEGER NOT NULL,
    speedtest_enabled BOOLEAN NOT NULL,
    speedtest_url VARCHAR(512) NOT NULL,
    speedtest_timeout_s INTEGER NOT NULL,
    speedtest_max_bytes INTEGER NOT NULL,
    speedtest_min_speed_mbps FLOAT NOT NULL,
    media_check_enabled BOOLEAN NOT NULL,
    media_platforms JSON NOT NULL,
    media_timeout_s INTEGER NOT NULL,
    probe_concurrency INTEGER NOT NULL,
    probe_timeout_ms INTEGER NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE proxy_chain_bindings (
    id INTEGER NOT NULL,
    target_type VARCHAR(20) NOT NULL,
    target_id INTEGER,
    target_name VARCHAR(255),
    dialer_type VARCHAR(20) NOT NULL,
    dialer_ref VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL,
    sort_order INTEGER NOT NULL,
    note TEXT,
    PRIMARY KEY (id)
);
CREATE INDEX ix_proxy_chain_bindings_id ON proxy_chain_bindings (id);
CREATE INDEX ix_proxy_chain_bindings_target_id ON proxy_chain_bindings (target_id);
CREATE INDEX ix_proxy_chain_bindings_target_type ON proxy_chain_bindings (target_type);

CREATE TABLE rule_categories (
    id INTEGER NOT NULL,
    name VARCHAR(80) NOT NULL,
    sort_order INTEGER NOT NULL,
    PRIMARY KEY (id),
    UNIQUE (name)
);
CREATE INDEX ix_rule_categories_id ON rule_categories (id);

CREATE TABLE rules (
    id INTEGER NOT NULL,
    name VARCHAR(160) NOT NULL,
    category VARCHAR(80) NOT NULL,
    type VARCHAR(40) NOT NULL,
    value VARCHAR(512) NOT NULL,
    proxy VARCHAR(160) NOT NULL,
    options JSON NOT NULL,
    sort_order INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE security_settings (
    id INTEGER NOT NULL,
    auth_enabled BOOLEAN NOT NULL,
    protect_frontend BOOLEAN NOT NULL,
    protect_api BOOLEAN NOT NULL,
    protect_exports BOOLEAN NOT NULL,
    token_hash VARCHAR(128) NOT NULL,
    fetch_proxy_enabled BOOLEAN NOT NULL,
    fetch_proxy_url VARCHAR(512) NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE subscriptions (
    id INTEGER NOT NULL,
    name VARCHAR(120) NOT NULL,
    url TEXT NOT NULL,
    update_interval INTEGER,
    is_primary BOOLEAN NOT NULL,
    enabled BOOLEAN NOT NULL,
    node_prefix VARCHAR(120),
    filter_regex JSON NOT NULL,
    filter_min_speed_mbps FLOAT,
    filter_media_unlock JSON NOT NULL,
    include_node_names JSON NOT NULL,
    exclude_node_names JSON NOT NULL,
    node_renames JSON NOT NULL,
    proxy_chain JSON NOT NULL DEFAULT '[]',
    node_proxy_chains JSON NOT NULL DEFAULT '{}',
    source_nodes JSON NOT NULL,
    manual_nodes JSON NOT NULL,
    raw_nodes JSON NOT NULL,
    last_fetched_at DATETIME,
    last_fetch_error TEXT,
    fetch_failed_count INTEGER NOT NULL,
    fetch_comments JSON NOT NULL,
    subscription_userinfo TEXT,
    profile_update_interval VARCHAR(40),
    profile_web_page_url TEXT,
    PRIMARY KEY (id),
    UNIQUE (name)
);
CREATE INDEX ix_subscriptions_id ON subscriptions (id);
