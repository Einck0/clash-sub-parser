-- Connection generation is per node, monotonically increasing, and independent of metadata updates.
-- Historical observations intentionally retain NULL: they cannot certify the current generation.
ALTER TABLE nodes ADD COLUMN connection_revision INTEGER NOT NULL DEFAULT 1 CHECK (connection_revision > 0);
ALTER TABLE probe_observations ADD COLUMN connection_revision INTEGER;
