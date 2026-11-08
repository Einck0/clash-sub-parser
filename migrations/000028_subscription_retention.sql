-- Migration: 000028_subscription_retention.sql
-- Implements durable subscription entry classification override ledger and retention indexes.

-- 1. Durable classification override ledger
CREATE TABLE IF NOT EXISTS subscription_entry_overrides (
    subscription_id TEXT NOT NULL,
    override_anchor TEXT NOT NULL,
    source_key TEXT NOT NULL DEFAULT '',
    raw_name TEXT NOT NULL DEFAULT '',
    user_kind_override TEXT NOT NULL CHECK (user_kind_override IN ('proxy', 'notice', 'unknown')),
    override_reason TEXT NOT NULL DEFAULT '',
    override_at TEXT NOT NULL,
    actor_ref TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (subscription_id, override_anchor),
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sub_entry_overrides_sub ON subscription_entry_overrides(subscription_id);

-- 2. Deterministic backfill of latest explicit override intent
-- Explicit clear evidence rows (with override_at present or nonempty override_anchor) are included
-- in the ranking to prevent resurrecting an older override if a newer explicit clear exists.
-- Note on irrecoverable historical clears: If a historical clear removed all columns including
-- override_at and override_anchor without leaving evidence in old rows, the historical clear
-- cannot be recovered without inventing data. Only explicit clear evidence rows are ranked.
INSERT OR IGNORE INTO subscription_entry_overrides (
    subscription_id,
    override_anchor,
    source_key,
    raw_name,
    user_kind_override,
    override_reason,
    override_at,
    actor_ref
)
SELECT
    subscription_id,
    effective_anchor,
    source_key,
    raw_name,
    user_kind_override,
    override_reason,
    COALESCE(NULLIF(override_at, ''), created_at) AS override_at,
    actor_ref
FROM (
    SELECT
        subscription_id,
        COALESCE(NULLIF(override_anchor, ''), NULLIF(source_key, ''), protocol || '|' || server || '|' || port || '|' || raw_name) AS effective_anchor,
        source_key,
        raw_name,
        user_kind_override,
        override_reason,
        override_at,
        created_at,
        actor_ref,
        ROW_NUMBER() OVER (
            PARTITION BY subscription_id, COALESCE(NULLIF(override_anchor, ''), NULLIF(source_key, ''), protocol || '|' || server || '|' || port || '|' || raw_name)
            ORDER BY COALESCE(NULLIF(override_at, ''), created_at) DESC, created_at DESC, rowid DESC
        ) AS rn
    FROM subscription_entries
    WHERE (user_kind_override IS NOT NULL AND user_kind_override != '')
       OR (override_at IS NOT NULL AND override_at != '')
       OR (override_anchor != '')
) ranked
WHERE ranked.rn = 1
  AND ranked.effective_anchor != ''
  AND ranked.user_kind_override IS NOT NULL
  AND ranked.user_kind_override != '';

-- 3. Retention and reference performance indexes
CREATE INDEX IF NOT EXISTS idx_sub_payloads_retention ON subscription_payloads(subscription_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_conn_ver_source_entry ON node_connection_versions(source_entry_id) WHERE source_entry_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_node_sources_last_seen_fetch ON node_sources(last_seen_fetch_id) WHERE last_seen_fetch_id != '';
CREATE INDEX IF NOT EXISTS idx_sub_fetches_retention ON subscription_fetches(subscription_id, started_at DESC);
