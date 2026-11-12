ALTER TABLE global_node_filters ADD COLUMN empty_fallback_pass INTEGER NOT NULL DEFAULT 1 CHECK (empty_fallback_pass IN (0, 1));
