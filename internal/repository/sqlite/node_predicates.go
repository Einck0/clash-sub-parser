package sqlite

import (
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

const confirmedNoticeExclusionSubquery = `SELECT se1.node_logical_id FROM subscription_entries se1
WHERE se1.node_logical_id IS NOT NULL
  AND COALESCE(se1.user_kind_override, se1.entry_kind) = 'notice'
  AND NOT EXISTS (
      SELECT 1 FROM subscription_entries se2
      WHERE se2.node_logical_id = se1.node_logical_id
        AND se2.user_kind_override = 'proxy'
  )`

const enabledSubscriptionsInventorySubquery = `SELECT DISTINCT ns.node_logical_id
FROM node_sources ns
JOIN subscriptions s ON ns.subscription_id = s.id
WHERE s.enabled = 1
  AND ns.last_seen_fetch_id != ''
  AND ns.last_seen_fetch_id = (
      SELECT sf.id
      FROM subscription_fetches sf
      WHERE sf.subscription_id = s.id
        AND sf.outcome IN ('success', 'partial')
      ORDER BY sf.started_at DESC, sf.id DESC
      LIMIT 1
  )`

// buildNodeFilterPredicates constructs standard WHERE clauses and arguments for node queries.
// colPrefix can be "n." or "" or any table alias.
func buildNodeFilterPredicates(filter domain.NodeFilter, colPrefix string) ([]string, []any) {
	where := make([]string, 0)
	args := make([]any, 0)

	// Scope E: current enabled subscription inventory
	if filter.Scope == domain.NodeScopeEnabledSubscriptions {
		where = append(where, fmt.Sprintf("%slogical_id IN (%s)", colPrefix, enabledSubscriptionsInventorySubquery))
		where = append(where, fmt.Sprintf("%slogical_id NOT IN (%s)", colPrefix, confirmedNoticeExclusionSubquery))
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
		where = append(where, fmt.Sprintf("%slogical_id IN (SELECT ns_sub.node_logical_id FROM node_sources ns_sub WHERE ns_sub.subscription_id = ?)", colPrefix))
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
					THEN 'untested'

					WHEN lb.verdict = 'available'
					     AND lb.connection_revision IS NOT NULL
					     AND lb.connection_revision = %[2]s
					     AND %[2]s > 0
					     AND datetime(lb.observed_at) <= datetime(?)
					     AND datetime(lb.observed_at) > datetime(?, '-1 hour')
					THEN 'healthy'

					WHEN lb.verdict = 'restricted'
					     AND lb.connection_revision IS NOT NULL
					     AND lb.connection_revision = %[2]s
					     AND %[2]s > 0
					     AND datetime(lb.observed_at) <= datetime(?)
					     AND datetime(lb.observed_at) > datetime(?, '-1 hour')
					THEN 'degraded'

					WHEN lb.verdict = 'error'
					     AND lb.redacted_summary LIKE '%%reason=node_connect_failed%%'
					     AND lb.connection_revision IS NOT NULL
					     AND lb.connection_revision = %[2]s
					     AND %[2]s > 0
					     AND datetime(lb.observed_at) <= datetime(?)
					     AND datetime(lb.observed_at) > datetime(?, '-1 hour')
					THEN 'unhealthy'

					ELSE 'undetermined'
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
		args = append(args, nowStr, nowStr, nowStr, nowStr, nowStr, nowStr)
		for _, hs := range filter.HealthStatuses {
			args = append(args, strings.ToLower(strings.TrimSpace(hs)))
		}
	}

	return where, args
}
