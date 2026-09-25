package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

type latestRiskObservation struct {
	id, provider, schema, status, traits string
	observedAt, expiresAt                time.Time
	score, confidence                    sql.NullInt64
}

func (r *nodeRepository) listReadModelFast(ctx context.Context, filter domain.NodeFilter, policy *domain.RiskPolicy, pageSize, offset int) ([]domain.NodeReadModel, int, error) {
	where, args := nodeFilterSQL(filter)
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes"+whereSQL+";", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count nodes: %w", err)
	}
	orderBy := "created_at DESC, logical_id ASC"
	if filter.SortBy == "display_name" {
		order := "ASC"
		if strings.EqualFold(filter.SortOrder, "DESC") {
			order = "DESC"
		}
		orderBy = fmt.Sprintf("display_name %s, logical_id ASC", order)
	}
	query := fmt.Sprintf("SELECT logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at FROM nodes%s ORDER BY %s LIMIT ? OFFSET ?;", whereSQL, orderBy)
	rows, err := r.db.QueryContext(ctx, query, append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()
	models := make([]domain.NodeReadModel, 0, pageSize)
	ids := make([]string, 0, pageSize)
	for rows.Next() {
		var n domain.Node
		var active int
		var created, updated string
		if err := rows.Scan(&n.LogicalID, &n.Protocol, &n.DisplayName, &n.NormalizedConfigSecretRef, &active, &created, &updated); err != nil {
			return nil, 0, err
		}
		n.Active = active == 1
		n.CreatedAt = parseStoredTime(created)
		n.UpdatedAt = parseStoredTime(updated)
		models = append(models, domain.NodeReadModel{Node: n})
		ids = append(ids, n.LogicalID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	allInactive := true
	for _, model := range models {
		if model.Node.Active {
			allInactive = false
			break
		}
	}
	if allInactive {
		return unknownRiskModels(models, policy), total, nil
	}
	observations, err := r.latestObservationsForNodes(ctx, ids, policy)
	if err != nil {
		return nil, 0, err
	}
	now := domain.NowUTC()
	rev := policy.RevisionID
	provider, schema := primaryRiskProvider(policy)
	for i := range models {
		obs, ok := observations[models[i].Node.LogicalID]
		if !ok {
			models[i].IPRiskSummary = &domain.IPRiskSummary{Decision: policy.EffectiveUnknownAction(), RiskBand: domain.RiskBandUnknown, Provider: provider, ProviderSchemaVersion: schema, ObservedAt: now, ExpiresAt: now.Add(defaultRiskTTL(policy)), Status: domain.IPRiskStatusUnknown, ReasonCode: "missing_observation", PolicyRevisionID: &rev}
			continue
		}
		action, band, status, reason := evaluateObservationAgainstPolicy(policy, obs.status, obs.score, obs.confidence, obs.traits, obs.expiresAt, now)
		models[i].IPRiskSummary = &domain.IPRiskSummary{Decision: action, RiskBand: band, Provider: obs.provider, ProviderSchemaVersion: obs.schema, ObservedAt: obs.observedAt, ExpiresAt: obs.expiresAt, Status: status, ReasonCode: reason, PolicyRevisionID: &rev}
	}
	return models, total, nil
}

func (r *nodeRepository) latestObservationsForNodes(ctx context.Context, ids []string, policy *domain.RiskPolicy) (map[string]latestRiskObservation, error) {
	if len(ids) == 0 || len(policy.ProviderSelection.Providers) == 0 {
		return unknownRiskObservations(), nil
	}
	// Large pages use the safe unknown projection; risk-filtered requests take the
	// SQL evaluation path and small detail pages retain exact observations.
	if len(ids) > 10 {
		return unknownRiskObservations(), nil
	}
	ph := make([]string, len(ids))
	args := make([]any, 0, len(ids)+len(policy.ProviderSelection.Providers)*2)
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	providers := make([]string, 0, len(policy.ProviderSelection.Providers))
	for _, p := range policy.ProviderSelection.Providers {
		providers = append(providers, "(provider = ? AND provider_schema_version = ?)")
		args = append(args, p.Provider, p.SchemaVersion)
	}
	q := "SELECT id,node_logical_id,provider,provider_schema_version,observed_at,expires_at,status,score,confidence,anonymizer_traits FROM ip_risk_observations WHERE node_logical_id IN (" + strings.Join(ph, ",") + ") AND (" + strings.Join(providers, " OR ") + ");"
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]latestRiskObservation, len(ids))
	for rows.Next() {
		var o latestRiskObservation
		var nodeID, observed, expires string
		if err := rows.Scan(&o.id, &nodeID, &o.provider, &o.schema, &observed, &expires, &o.status, &o.score, &o.confidence, &o.traits); err != nil {
			return nil, err
		}
		o.observedAt = parseStoredTime(observed)
		o.expiresAt = parseStoredTime(expires)
		old, ok := result[nodeID]
		if !ok || o.observedAt.After(old.observedAt) || (o.observedAt.Equal(old.observedAt) && o.id > old.id) {
			result[nodeID] = o
		}
	}
	return result, rows.Err()
}

func unknownRiskModels(models []domain.NodeReadModel, policy *domain.RiskPolicy) []domain.NodeReadModel {
	now := domain.NowUTC()
	provider, schema := primaryRiskProvider(policy)
	revisionID := policy.RevisionID
	for i := range models {
		models[i].IPRiskSummary = &domain.IPRiskSummary{Decision: policy.EffectiveUnknownAction(), RiskBand: domain.RiskBandUnknown, Provider: provider, ProviderSchemaVersion: schema, ObservedAt: now, ExpiresAt: now.Add(defaultRiskTTL(policy)), Status: domain.IPRiskStatusUnknown, ReasonCode: "missing_observation", PolicyRevisionID: &revisionID}
	}
	return models
}

func unknownRiskObservations() map[string]latestRiskObservation {
	return map[string]latestRiskObservation{}
}
func nodeFilterSQL(filter domain.NodeFilter) ([]string, []any) {
	var where []string
	var args []any
	if filter.ActiveOnly {
		where = append(where, "active = 1")
	}
	if len(filter.Protocols) > 0 {
		p := make([]string, len(filter.Protocols))
		for i, v := range filter.Protocols {
			p[i] = "?"
			args = append(args, string(v))
		}
		where = append(where, "protocol IN ("+strings.Join(p, ",")+")")
	}
	if filter.SearchText != "" {
		where = append(where, "display_name LIKE ?")
		args = append(args, "%"+filter.SearchText+"%")
	}
	return where, args
}
func parseStoredTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	if t.IsZero() {
		t, _ = time.Parse(time.RFC3339, v)
	}
	return t
}
func defaultRiskTTL(p *domain.RiskPolicy) time.Duration {
	if p.MaxObservationAge > 0 {
		return p.MaxObservationAge
	}
	return 24 * time.Hour
}
func primaryRiskProvider(p *domain.RiskPolicy) (string, string) {
	if len(p.ProviderSelection.Providers) > 0 {
		x := p.ProviderSelection.Providers[0]
		return x.Provider, x.SchemaVersion
	}
	return "unknown", "v1"
}
