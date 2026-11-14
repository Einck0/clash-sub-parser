package sqlite

import (
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// Classification follows only the latest useful snapshot of each subscription.
// Historical source references protect provenance/retention, not current notice
// membership. Failed fetches leave the last useful snapshot in effect.
const currentUsefulPayloadsSubquery = `SELECT p.id
FROM subscription_payloads p
JOIN (
    SELECT p2.id,
           ROW_NUMBER() OVER (
               PARTITION BY p2.subscription_id
               ORDER BY p2.created_at DESC, p2.rowid DESC
           ) AS rn
    FROM subscription_payloads p2
    LEFT JOIN subscription_fetches sf ON p2.fetch_id = sf.id
    WHERE (sf.outcome IN ('success', 'partial') OR (sf.id IS NULL AND p2.http_status = 200))
      AND (sf.nodes_valid IS NULL OR sf.nodes_valid > 0 OR EXISTS (SELECT 1 FROM subscription_entries se_check WHERE se_check.payload_id = p2.id))
) ranked ON p.id = ranked.id AND ranked.rn = 1`

const confirmedNoticeExclusionSubquery = `SELECT se1.node_logical_id FROM subscription_entries se1
WHERE se1.node_logical_id IS NOT NULL
  AND se1.payload_id IN (` + currentUsefulPayloadsSubquery + `)
  AND COALESCE(
      (SELECT seo.user_kind_override
       FROM subscription_entry_overrides seo
       WHERE seo.subscription_id = se1.subscription_id
         AND ((se1.override_anchor != '' AND seo.override_anchor = se1.override_anchor) OR (se1.source_key != '' AND seo.override_anchor = se1.source_key))
       ORDER BY seo.override_at DESC, seo.rowid DESC
       LIMIT 1),
      se1.entry_kind
  ) = 'notice'
  AND NOT EXISTS (
      SELECT 1 FROM subscription_entries se2
      WHERE se2.node_logical_id = se1.node_logical_id
        AND se2.payload_id IN (` + currentUsefulPayloadsSubquery + `)
        AND (
            SELECT seo2.user_kind_override
            FROM subscription_entry_overrides seo2
            WHERE seo2.subscription_id = se2.subscription_id
              AND ((se2.override_anchor != '' AND seo2.override_anchor = se2.override_anchor) OR (se2.source_key != '' AND seo2.override_anchor = se2.source_key))
            ORDER BY seo2.override_at DESC, seo2.rowid DESC
            LIMIT 1
        ) = 'proxy'
  )`

// Enabled inventory classification must ignore disabled subscriptions on both
// sides: neither a disabled notice nor a disabled explicit proxy may affect a
// currently enabled shared node. Keep the unscoped query above for legacy asset
// classification and disabled subscriptions' retained source counts.
// Materialize the current payload set before reading entries, then resolve each
// current enabled entry's override once. CROSS JOIN keeps payloads as the outer
// loop so the existing (payload_id, ordinal) index avoids scanning retained
// history. A plain proxy entry is not an explicit proxy override: only the latter
// rescues a shared node from another current entry's notice classification.
const enabledNoticeExclusionSubquery = `WITH
current_payloads AS MATERIALIZED (` + currentUsefulPayloadsSubquery + `),
resolved_entries AS MATERIALIZED (
    SELECT se.node_logical_id, se.entry_kind,
           (SELECT seo.user_kind_override
            FROM subscription_entry_overrides seo
            WHERE seo.subscription_id = se.subscription_id
              AND ((se.override_anchor != '' AND seo.override_anchor = se.override_anchor)
                   OR (se.source_key != '' AND seo.override_anchor = se.source_key))
            ORDER BY seo.override_at DESC, seo.rowid DESC
            LIMIT 1) AS explicit_kind
    FROM current_payloads cp
    CROSS JOIN subscription_entries se ON se.payload_id = cp.id
    JOIN subscriptions s ON s.id = se.subscription_id AND s.enabled = 1
    WHERE se.node_logical_id IS NOT NULL
)
SELECT node_logical_id
FROM resolved_entries
GROUP BY node_logical_id
HAVING MAX(CASE WHEN COALESCE(explicit_kind, entry_kind) = 'notice' THEN 1 ELSE 0 END) = 1
   AND MAX(CASE WHEN explicit_kind = 'proxy' THEN 1 ELSE 0 END) = 0`

// Aliases ns and s must identify the source and its subscription respectively.
const currentEnabledSourcePredicate = `s.enabled = 1
  AND ns.last_seen_fetch_id != ''
  AND ns.last_seen_fetch_id = (
      SELECT sf.id
      FROM subscription_fetches sf
      WHERE sf.subscription_id = s.id
        AND sf.outcome IN ('success', 'partial')
      ORDER BY sf.started_at DESC, sf.rowid DESC
      LIMIT 1
  )`

const enabledSubscriptionsInventorySubquery = `SELECT DISTINCT ns.node_logical_id
FROM node_sources ns
JOIN subscriptions s ON ns.subscription_id = s.id
WHERE ` + currentEnabledSourcePredicate + `
UNION
SELECT n.logical_id
FROM nodes n
WHERE n.active = 1
  AND EXISTS (SELECT 1 FROM node_source_history h WHERE h.node_logical_id = n.logical_id AND h.cause = 'manual_confirmed')`

// buildNodeFilterPredicates constructs standard WHERE clauses and arguments for node queries.
// colPrefix can be "n." or "" or any table alias.
func buildNodeFilterPredicates(filter domain.NodeFilter, colPrefix string) ([]string, []any) {
	where := make([]string, 0)
	args := make([]any, 0)

	// Scope E: current enabled subscription inventory
	if filter.Scope == domain.NodeScopeEnabledSubscriptions {
		where = append(where, fmt.Sprintf("%slogical_id IN (%s)", colPrefix, enabledSubscriptionsInventorySubquery))
		where = append(where, fmt.Sprintf("%slogical_id NOT IN (%s)", colPrefix, enabledNoticeExclusionSubquery))
	} else if filter.ExcludeNotices {
		where = append(where, fmt.Sprintf("%slogical_id NOT IN (%s)", colPrefix, confirmedNoticeExclusionSubquery))
	}

	if filter.ActiveOnly {
		where = append(where, fmt.Sprintf("%sactive = 1", colPrefix))
	}

	if len(filter.LogicalIDs) > 0 {
		placeholders := make([]string, len(filter.LogicalIDs))
		for i, id := range filter.LogicalIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, fmt.Sprintf("%slogical_id IN (%s)", colPrefix, strings.Join(placeholders, ",")))
	}

	if len(filter.Protocols) > 0 {
		placeholders := make([]string, len(filter.Protocols))
		for i, p := range filter.Protocols {
			placeholders[i] = "?"
			args = append(args, string(p))
		}
		where = append(where, fmt.Sprintf("%sprotocol IN (%s)", colPrefix, strings.Join(placeholders, ",")))
	}

	if filter.SearchText != "" {
		where = append(where, fmt.Sprintf("%sdisplay_name LIKE ?", colPrefix))
		args = append(args, "%"+filter.SearchText+"%")
	}

	if filter.SubscriptionID != "" {
		if filter.Scope == domain.NodeScopeEnabledSubscriptions {
			where = append(where, fmt.Sprintf("%slogical_id IN (SELECT ns.node_logical_id FROM node_sources ns JOIN subscriptions s ON s.id = ns.subscription_id WHERE ns.subscription_id = ? AND %s)", colPrefix, currentEnabledSourcePredicate))
		} else {
			where = append(where, fmt.Sprintf("%slogical_id IN (SELECT ns_sub.node_logical_id FROM node_sources ns_sub WHERE ns_sub.subscription_id = ?)", colPrefix))
		}
		args = append(args, filter.SubscriptionID)
	}

	if len(filter.HealthStatuses) > 0 {
		now := domain.NowUTC()
		if filter.Now != nil && !filter.Now.IsZero() {
			now = filter.Now.UTC()
		}
		nowStr := now.Format(time.RFC3339Nano)

		colLogicalID := fmt.Sprintf("%slogical_id", colPrefix)
		colConnRev := fmt.Sprintf("%sconnection_revision", colPrefix)
		var effRevExpr string
		if colPrefix != "" {
			effRevExpr = fmt.Sprintf("COALESCE((SELECT nch.connection_revision FROM node_connection_heads nch WHERE nch.logical_id = %s), %s)", colLogicalID, colConnRev)
		} else {
			effRevExpr = colConnRev
		}

		healthSubquery := fmt.Sprintf(`(
			SELECT
				CASE
					WHEN NOT EXISTS (SELECT 1 FROM probe_observations WHERE node_logical_id = %[1]s)
					THEN 'unknown'

					WHEN lb.verdict IN ('available', 'restricted')
					     AND lb.connection_revision IS NOT NULL
					     AND lb.connection_revision = %[2]s
					     AND %[2]s > 0
					     AND datetime(lb.observed_at) <= datetime(?)
					THEN 'healthy'

					WHEN lb.verdict = 'error'
					     AND lb.connection_revision IS NOT NULL
					     AND lb.connection_revision = %[2]s
					     AND %[2]s > 0
					     AND datetime(lb.observed_at) <= datetime(?)
					THEN 'unhealthy'

					ELSE 'unknown'
				END
			FROM (SELECT 1)
			LEFT JOIN (
				SELECT verdict, connection_revision, observed_at, redacted_summary
				FROM probe_observations
				WHERE node_logical_id = %[1]s AND kind = 'baseline'
				ORDER BY observed_at DESC, id DESC
				LIMIT 1
			) lb ON 1=1
		)`, colLogicalID, effRevExpr)

		placeholders := make([]string, len(filter.HealthStatuses))
		for i := range filter.HealthStatuses {
			placeholders[i] = "?"
		}
		where = append(where, fmt.Sprintf("%s IN (%s)", healthSubquery, strings.Join(placeholders, ",")))
		args = append(args, nowStr, nowStr)
		for _, hs := range filter.HealthStatuses {
			args = append(args, strings.ToLower(strings.TrimSpace(hs)))
		}
	}

	if len(filter.IPRiskBands) > 0 || filter.IPRiskScoreMax != nil {
		now := domain.NowUTC()
		if filter.Now != nil && !filter.Now.IsZero() {
			now = filter.Now.UTC()
		}
		scoreSQL := currentIPRiskScoreSQL(colPrefix)
		args = append(args, now.Format(time.RFC3339Nano))
		clauses := make([]string, 0, 2)
		if len(filter.IPRiskBands) > 0 {
			placeholders := make([]string, len(filter.IPRiskBands))
			for i, band := range filter.IPRiskBands {
				placeholders[i] = "?"
				args = append(args, band)
			}
			clauses = append(clauses, "CASE WHEN score IS NULL THEN 'unknown' WHEN score < 25 THEN 'low' WHEN score < 65 THEN 'medium' ELSE 'high' END IN ("+strings.Join(placeholders, ",")+")")
		}
		if filter.IPRiskScoreMax != nil {
			clauses = append(clauses, "score <= ?")
			args = append(args, *filter.IPRiskScoreMax)
		}
		// The observation is read once even when both score and band are used.
		where = append(where, "EXISTS (WITH risk_score AS MATERIALIZED (SELECT "+scoreSQL+" AS score) SELECT 1 FROM risk_score WHERE "+strings.Join(clauses, " AND ")+")")
	}

	return where, args
}

// currentIPRiskScoreSQL mirrors domain.ParseIPRiskScore and the revision-bound
// evidence contract. Validate JSON and the complete numeric token BEFORE CAST:
// SQLite would otherwise turn malformed strings into low-risk zero scores.
func currentIPRiskScoreSQL(colPrefix string) string {
	// strings.TrimSpace's Unicode whitespace set. The suffix after '%' follows
	// the provider parser's byte-based whitespace check (ASCII whitespace).
	const whitespace = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"
	return `(
		WITH latest_risk AS MATERIALIZED (
			SELECT node_logical_id, verdict, connection_revision, observed_at, evidence_data
			FROM probe_observations po
			WHERE po.node_logical_id = ` + colPrefix + `logical_id AND po.kind = 'ip_risk'
			ORDER BY observed_at DESC, id DESC LIMIT 1
		), raw_risk AS (
			SELECT *, CASE WHEN json_valid(evidence_data) THEN
				CASE WHEN json_type(evidence_data, '$.risk_score') = 'text'
				THEN trim(json_extract(evidence_data, '$.risk_score'), '` + whitespace + `') END END AS raw
			FROM latest_risk
		), score_token AS (
			SELECT *, CASE WHEN instr(raw, '%') = 0 THEN raw
				ELSE trim(substr(raw, 1, instr(raw, '%') - 1), '` + whitespace + `') END AS num
			FROM raw_risk
		)
		SELECT CASE WHEN verdict = 'available'
			AND connection_revision > 0
			AND connection_revision = (
				SELECT COALESCE(h.connection_revision, n.connection_revision)
				FROM nodes n LEFT JOIN node_connection_heads h ON h.logical_id = n.logical_id
				WHERE n.logical_id = score_token.node_logical_id
			)
			AND julianday(observed_at) != julianday('0001-01-01T00:00:00Z')
			AND julianday(observed_at) <= julianday(?)
			AND (instr(raw, char(0)) = 0 OR
				(instr(raw, '%') > 0 AND instr(raw, char(0)) > instr(raw, '%') + 1))
			AND length(CAST(raw AS BLOB)) - length(CAST(replace(raw, '%', '') AS BLOB)) <= 1
			AND (instr(raw, '%') = 0 OR substr(raw, instr(raw, '%') + 1) = ''
				OR substr(raw, instr(raw, '%') + 1, 1) IN (' ', char(9), char(10), char(11), char(12), char(13)))
			AND length(num) BETWEEN 1 AND 3
			AND num NOT GLOB '*[^0-9]*'
			AND (length(num) = 1 OR substr(num, 1, 1) != '0')
			AND CAST(num AS INTEGER) BETWEEN 0 AND 100
			THEN CAST(num AS INTEGER) END
		FROM score_token
	)`
}
