-- Immutable snapshots for newly created revisions only. IF NOT EXISTS also
-- supports pre-release databases which carried this table in migration 18.
CREATE TABLE IF NOT EXISTS policy_revision_snapshots (
    revision_id TEXT PRIMARY KEY REFERENCES configuration_revisions(id) ON DELETE CASCADE,
    groups_json TEXT NOT NULL
);
