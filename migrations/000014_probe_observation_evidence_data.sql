-- Add structured evidence data column for multi-platform probe observations
ALTER TABLE probe_observations ADD COLUMN evidence_data TEXT NOT NULL DEFAULT '';
