-- Migration: 000010_ip_risk_latest_observation_index.sql
-- Composite index for bounded per-node latest IP risk observation lookup and window ranking.

CREATE INDEX IF NOT EXISTS idx_ip_risk_obs_node_observed_id
    ON ip_risk_observations(node_logical_id, observed_at DESC, id DESC);
