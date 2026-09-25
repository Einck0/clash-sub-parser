-- Migration: 000007_periodic_probes.sql
-- Periodic probe schedules, batch execution coordination, and observation credential versioning

-- 1. Periodic Probe Schedules (single-row table with id = 1)
CREATE TABLE IF NOT EXISTS probe_schedules (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0,
    interval_seconds INTEGER NOT NULL DEFAULT 3600,
    kinds TEXT NOT NULL DEFAULT '["baseline"]',
    next_due_at TEXT,
    generation INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);

-- Seed default schedule (disabled)
INSERT OR IGNORE INTO probe_schedules (id, enabled, interval_seconds, kinds, next_due_at, generation, updated_at)
VALUES (1, 0, 3600, '["baseline"]', NULL, 0, datetime('now'));

-- 2. Probe Batches (periodic probe execution coordination windows)
CREATE TABLE IF NOT EXISTS probe_batches (
    id TEXT PRIMARY KEY,
    window_at TEXT NOT NULL,
    generation INTEGER NOT NULL,
    owner TEXT NOT NULL DEFAULT '',
    lease_until TEXT,
    state TEXT NOT NULL DEFAULT 'pending',
    run_ids TEXT NOT NULL DEFAULT '[]',
    total_nodes INTEGER NOT NULL DEFAULT 0,
    dispatched_runs INTEGER NOT NULL DEFAULT 0,
    completed_runs INTEGER NOT NULL DEFAULT 0,
    skipped_nodes INTEGER NOT NULL DEFAULT 0,
    redacted_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CONSTRAINT uq_probe_batch_window UNIQUE (generation, window_at)
);

CREATE INDEX IF NOT EXISTS idx_probe_batches_state ON probe_batches(state);
CREATE INDEX IF NOT EXISTS idx_probe_batches_window ON probe_batches(window_at DESC);

-- 3. Probe Batch Runs association table
CREATE TABLE IF NOT EXISTS probe_batch_runs (
    batch_id TEXT NOT NULL,
    probe_run_id TEXT NOT NULL,
    PRIMARY KEY (batch_id, probe_run_id),
    FOREIGN KEY (batch_id) REFERENCES probe_batches(id) ON DELETE CASCADE,
    FOREIGN KEY (probe_run_id) REFERENCES probe_runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_probe_batch_runs_run ON probe_batch_runs(probe_run_id);

-- 4. Add credential_version to probe_observations table
ALTER TABLE probe_observations ADD COLUMN credential_version INTEGER;

-- 5. Add index for bulk latest observation lookups
CREATE INDEX IF NOT EXISTS idx_probe_obs_latest_lookup ON probe_observations(node_logical_id, kind, observed_at DESC, id DESC);
