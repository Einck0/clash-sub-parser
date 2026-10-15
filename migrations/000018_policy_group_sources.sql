-- Migration 000018: Add policy_group_sources and node_groups.revision for unified member sources rebuild

ALTER TABLE node_groups ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS policy_group_sources (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES node_groups(id) ON DELETE CASCADE,
    operation TEXT NOT NULL DEFAULT 'add',
    kind TEXT NOT NULL,
    position INTEGER NOT NULL,
    node_logical_id TEXT,
    target_group_id TEXT REFERENCES node_groups(id) ON DELETE RESTRICT,
    pattern TEXT,
    filter_spec_json TEXT,
    builtin_name TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_source_operation CHECK (operation IN ('add', 'remove')),
    CONSTRAINT chk_source_kind CHECK (kind IN ('manual_node', 'group_ref', 'group_nodes', 'regex', 'filter', 'typed_builtin')),
    CONSTRAINT chk_builtin_name CHECK (builtin_name IS NULL OR builtin_name IN ('PASS', 'DIRECT', 'REJECT', 'REJECT-DROP'))
);

CREATE INDEX IF NOT EXISTS idx_group_sources_group_pos ON policy_group_sources(group_id, position);
CREATE INDEX IF NOT EXISTS idx_group_sources_target_group ON policy_group_sources(target_group_id);
CREATE INDEX IF NOT EXISTS idx_group_sources_node_logical_id ON policy_group_sources(node_logical_id);

-- Migrate edges for groups (with filter_spec if group had a filter)
INSERT INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, target_group_id, filter_spec_json)
SELECT
    ge.id,
    ge.parent_group_id,
    'add',
    CASE WHEN ge.child_group_id IS NOT NULL THEN 'group_ref' ELSE 'manual_node' END,
    ge.position,
    ge.node_logical_id,
    ge.child_group_id,
    gnf.filter_spec
FROM group_edges ge
LEFT JOIN group_node_filters gnf ON ge.parent_group_id = gnf.group_id;

-- Migrate filter-only groups (groups with group_node_filters but NO edges in group_edges)
INSERT INTO policy_group_sources (id, group_id, operation, kind, position, filter_spec_json)
SELECT
    gnf.group_id,
    gnf.group_id,
    'add',
    'filter',
    0,
    gnf.filter_spec
FROM group_node_filters gnf
WHERE gnf.group_id NOT IN (SELECT DISTINCT parent_group_id FROM group_edges);
