-- Migration 000024: Add inherit_global_filter to node_groups, probe_timeout_seconds to settings, and prune probe records
ALTER TABLE node_groups ADD COLUMN inherit_global_filter INTEGER NOT NULL DEFAULT 1;
UPDATE node_groups SET inherit_global_filter = 0 WHERE UPPER(name) = 'WARP' OR id = 'WARP';
ALTER TABLE settings ADD COLUMN probe_timeout_seconds INTEGER NOT NULL DEFAULT 5;

-- 滚动修剪历史冗余数据：仅保留最新 100 批次/运行
DELETE FROM probe_runs WHERE id NOT IN (SELECT id FROM probe_runs ORDER BY created_at DESC LIMIT 100);
DELETE FROM probe_batches WHERE id NOT IN (SELECT id FROM probe_batches ORDER BY created_at DESC LIMIT 100);
DELETE FROM probe_observations 
WHERE probe_run_id NOT IN (SELECT id FROM probe_runs)
  AND id NOT IN (
    SELECT po.id FROM probe_observations po
    JOIN (
      SELECT node_logical_id, kind, MAX(observed_at) AS max_at
      FROM probe_observations GROUP BY node_logical_id, kind
    ) latest ON po.node_logical_id = latest.node_logical_id AND po.kind = latest.kind AND po.observed_at = latest.max_at
  );
