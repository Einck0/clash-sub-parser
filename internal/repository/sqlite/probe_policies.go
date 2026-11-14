package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"clash-sub-parser/internal/domain"
)

var _ domain.ProbePolicySubscriptionReader = (*subscriptionRepository)(nil)

// ListProbePolicies uses a single subscription-only snapshot. Scheduling does
// not need the historical inventory counts attached by the display List path.
func (r *subscriptionRepository) ListProbePolicies(ctx context.Context) ([]domain.Subscription, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, enabled, config_json FROM subscriptions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list probe policies: %w", err)
	}
	defer rows.Close()
	var subscriptions []domain.Subscription
	for rows.Next() {
		var sub domain.Subscription
		var enabled int
		var config sql.NullString
		if err := rows.Scan(&sub.ID, &enabled, &config); err != nil {
			return nil, fmt.Errorf("scan probe policy: %w", err)
		}
		sub.Enabled = enabled == 1
		if config.Valid && strings.TrimSpace(config.String) != "" {
			if err := json.Unmarshal([]byte(config.String), &sub.Config); err != nil {
				return nil, fmt.Errorf("decode probe policy for subscription %s: %w", sub.ID, err)
			}
		}
		normalizeSubscriptionConfig(&sub.Config)
		subscriptions = append(subscriptions, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read probe policies: %w", err)
	}
	return subscriptions, nil
}
