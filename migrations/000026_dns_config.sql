-- 000026_dns_config.sql
-- Singleton DNS configuration with optimistic concurrency control (revision) and payload validation constraint.

CREATE TABLE IF NOT EXISTS dns_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    raw_yaml TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (length(raw_yaml) <= 65536)
);
