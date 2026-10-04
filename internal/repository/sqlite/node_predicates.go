package sqlite

import (
	"fmt"
	"strings"

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

	return where, args
}
