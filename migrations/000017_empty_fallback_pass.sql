-- Explicit opt-in; historical and newly inserted groups remain strict by default.
ALTER TABLE node_groups ADD COLUMN empty_fallback_pass INTEGER NOT NULL DEFAULT 0 CHECK (empty_fallback_pass IN (0, 1));
