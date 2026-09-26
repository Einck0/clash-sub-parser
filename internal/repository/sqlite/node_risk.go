package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// ListReadModel queries the node projection with server-side risk filtering and stable pagination.
func (r *nodeRepository) ListReadModel(ctx context.Context, filter domain.NodeFilter) ([]domain.NodeReadModel, int, error) {
	policy, err := r.resolveTargetRiskPolicy(ctx, filter.RiskPolicyRevisions)
	if err != nil {
		return nil, 0, err
	}

	pageSize := filter.Pagination.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	page := filter.Pagination.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	now := domain.NowUTC()
	nowStr := now.Format(time.RFC3339Nano)

	// Build the SQL query with CTE for risk evaluation
	var (
		whereClauses []string
		args         []any
		cteSQL       string
	)

	if policy != nil {
		if len(filter.RiskDecisions) == 0 && len(filter.RiskBands) == 0 && len(filter.RiskProviders) == 0 && len(filter.RiskStatuses) == 0 {
			return r.listReadModelFast(ctx, filter, policy, pageSize, offset)
		}
		cteSQL, args = buildRiskEvaluationCTE(policy, nowStr)
	} else {
		cteSQL, args = buildNoPolicyCTE()
	}

	// Apply filter clauses against the unified node_eval projection
	if filter.ActiveOnly {
		whereClauses = append(whereClauses, "active = 1")
	}

	if len(filter.Protocols) > 0 {
		placeholders := make([]string, len(filter.Protocols))
		for i, p := range filter.Protocols {
			placeholders[i] = "?"
			args = append(args, string(p))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("protocol IN (%s)", strings.Join(placeholders, ",")))
	}

	if filter.SearchText != "" {
		whereClauses = append(whereClauses, "display_name LIKE ?")
		args = append(args, "%"+filter.SearchText+"%")
	}

	if len(filter.RiskDecisions) > 0 {
		placeholders := make([]string, len(filter.RiskDecisions))
		for i, d := range filter.RiskDecisions {
			placeholders[i] = "?"
			args = append(args, string(d))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("risk_decision IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.RiskBands) > 0 {
		placeholders := make([]string, len(filter.RiskBands))
		for i, b := range filter.RiskBands {
			placeholders[i] = "?"
			args = append(args, string(b))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("risk_band IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.RiskProviders) > 0 {
		placeholders := make([]string, len(filter.RiskProviders))
		for i, p := range filter.RiskProviders {
			placeholders[i] = "?"
			args = append(args, p)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("risk_provider IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.RiskStatuses) > 0 {
		placeholders := make([]string, len(filter.RiskStatuses))
		for i, s := range filter.RiskStatuses {
			placeholders[i] = "?"
			args = append(args, string(s))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("risk_status IN (%s)", strings.Join(placeholders, ",")))
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count directly from nodes when risk fields are not used as filters. This avoids
	// materializing the full risk projection twice for ordinary ledger pagination.
	var total int
	countQuery := fmt.Sprintf("%s SELECT COUNT(*) FROM node_eval %s;", cteSQL, whereSQL)
	if policy != nil && len(filter.RiskDecisions) == 0 && len(filter.RiskBands) == 0 &&
		len(filter.RiskProviders) == 0 && len(filter.RiskStatuses) == 0 {
		countQuery = "SELECT COUNT(*) FROM nodes"
		if len(whereClauses) > 0 {
			countQuery += " WHERE " + strings.Join(whereClauses, " AND ")
		}
		countQuery += ";"
	}
	countArgs := args
	if !strings.Contains(countQuery, "?") {
		countArgs = nil
	}
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count read model nodes: %w", err)
	}

	// 2. Stable sorting
	orderBy := "created_at DESC, logical_id ASC"
	if filter.SortBy == "display_name" {
		order := "ASC"
		if strings.ToUpper(filter.SortOrder) == "DESC" {
			order = "DESC"
		}
		orderBy = fmt.Sprintf("display_name %s, logical_id ASC", order)
	}

	// 3. Paginated items query
	selectQuery := fmt.Sprintf(`%s
		SELECT logical_id, protocol, display_name, normalized_config_secret_ref, credential_version, active,
		       created_at, updated_at, risk_decision, risk_band, risk_provider,
		       risk_schema_version, risk_status, risk_reason_code, risk_observed_at,
		       risk_expires_at
		FROM node_eval
		%s
		ORDER BY %s
		LIMIT ? OFFSET ?;`, cteSQL, whereSQL, orderBy)

	queryArgs := append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query read model nodes: %w", err)
	}
	defer rows.Close()

	items := make([]domain.NodeReadModel, 0, pageSize)
	for rows.Next() {
		var (
			node                                 domain.Node
			activeInt                            int
			createdStr, updatedStr               string
			riskDecision, riskBand, riskProvider string
			riskSchemaVersion, riskStatus        string
			riskReasonCode                       string
			riskObservedAt, riskExpiresAt        sql.NullString
		)

		if err := rows.Scan(
			&node.LogicalID,
			&node.Protocol,
			&node.DisplayName,
			&node.NormalizedConfigSecretRef,
			&node.CredentialVersion,
			&activeInt,
			&createdStr,
			&updatedStr,
			&riskDecision,
			&riskBand,
			&riskProvider,
			&riskSchemaVersion,
			&riskStatus,
			&riskReasonCode,
			&riskObservedAt,
			&riskExpiresAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan read model node: %w", err)
		}

		node.Active = activeInt == 1
		node.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		if node.CreatedAt.IsZero() {
			node.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		}
		node.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		if node.UpdatedAt.IsZero() {
			node.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		}

		// Construct safe IPRiskSummary
		var summary *domain.IPRiskSummary
		if policy != nil {
			revID := policy.RevisionID
			defaultTTL := policy.MaxObservationAge
			if defaultTTL <= 0 {
				defaultTTL = 24 * time.Hour
			}

			obsAt := now
			if riskObservedAt.Valid && riskObservedAt.String != "" {
				parsed, err := time.Parse(time.RFC3339Nano, riskObservedAt.String)
				if err == nil {
					obsAt = parsed
				} else if parsed, err := time.Parse(time.RFC3339, riskObservedAt.String); err == nil {
					obsAt = parsed
				}
			}

			expAt := obsAt.Add(defaultTTL)
			if riskExpiresAt.Valid && riskExpiresAt.String != "" {
				parsed, err := time.Parse(time.RFC3339Nano, riskExpiresAt.String)
				if err == nil {
					expAt = parsed
				} else if parsed, err := time.Parse(time.RFC3339, riskExpiresAt.String); err == nil {
					expAt = parsed
				}
			}

			if !expAt.After(obsAt) {
				expAt = obsAt.Add(defaultTTL)
			}

			reason := normalizeRiskReasonCode(riskReasonCode)

			summary = &domain.IPRiskSummary{
				Decision:              domain.RiskAction(riskDecision),
				RiskBand:              domain.RiskBand(riskBand),
				Provider:              riskProvider,
				ProviderSchemaVersion: riskSchemaVersion,
				ObservedAt:            obsAt,
				ExpiresAt:             expAt,
				Status:                domain.IPRiskStatus(riskStatus),
				ReasonCode:            reason,
				PolicyRevisionID:      &revID,
			}
		}

		items = append(items, domain.NodeReadModel{
			Node:          node,
			IPRiskSummary: summary,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error reading node read model rows: %w", err)
	}

	return items, total, nil
}

// GetReadModel queries a single node projection with its API-safe risk summary.
func (r *nodeRepository) GetReadModel(ctx context.Context, logicalID string, policyRevisionID string) (*domain.NodeReadModel, error) {
	node, err := r.GetByLogicalID(ctx, logicalID)
	if err != nil {
		return nil, err
	}

	var reqRevs []string
	if policyRevisionID != "" {
		reqRevs = []string{policyRevisionID}
	}
	policy, err := r.resolveTargetRiskPolicy(ctx, reqRevs)
	if err != nil {
		return nil, err
	}

	var summary *domain.IPRiskSummary
	if policy != nil {
		primaryProvider := "unknown"
		primarySchemaVersion := "v1"
		if len(policy.ProviderSelection.Providers) > 0 {
			primaryProvider = policy.ProviderSelection.Providers[0].Provider
			primarySchemaVersion = policy.ProviderSelection.Providers[0].SchemaVersion
		}

		now := domain.NowUTC()
		defaultTTL := policy.MaxObservationAge
		if defaultTTL <= 0 {
			defaultTTL = 24 * time.Hour
		}

		// Query latest observation for this node across policy's allowed providers
		const obsQuery = `
			SELECT o.provider, o.provider_schema_version, o.observed_at, o.expires_at,
			       o.status, o.score, o.confidence, o.anonymizer_traits
			FROM ip_risk_observations o
			WHERE o.node_logical_id = ?
			  AND EXISTS (
			      SELECT 1
			      FROM risk_policy_providers rpp
			      WHERE rpp.revision_id = ?
			        AND rpp.provider = o.provider
			        AND rpp.schema_version = o.provider_schema_version
			  )
			ORDER BY o.observed_at DESC, o.id DESC
			LIMIT 1;`

		var (
			provider, schemaVersion, observedAtStr, expiresAtStr, status string
			score, confidence                                            sql.NullInt64
			traitsJSON                                                   string
		)

		err := r.db.QueryRowContext(ctx, obsQuery, logicalID, policy.RevisionID).Scan(
			&provider, &schemaVersion, &observedAtStr, &expiresAtStr, &status, &score, &confidence, &traitsJSON,
		)

		revID := policy.RevisionID
		if errors.Is(err, sql.ErrNoRows) {
			summary = &domain.IPRiskSummary{
				Decision:              policy.EffectiveUnknownAction(),
				RiskBand:              domain.RiskBandUnknown,
				Provider:              primaryProvider,
				ProviderSchemaVersion: primarySchemaVersion,
				ObservedAt:            now,
				ExpiresAt:             now.Add(defaultTTL),
				Status:                domain.IPRiskStatusUnknown,
				ReasonCode:            "missing_observation",
				PolicyRevisionID:      &revID,
			}
		} else if err != nil {
			return nil, fmt.Errorf("failed to query risk observation for node %s: %w", logicalID, err)
		} else {
			obsAt, _ := time.Parse(time.RFC3339Nano, observedAtStr)
			if obsAt.IsZero() {
				obsAt, _ = time.Parse(time.RFC3339, observedAtStr)
			}
			expAt, _ := time.Parse(time.RFC3339Nano, expiresAtStr)
			if expAt.IsZero() {
				expAt, _ = time.Parse(time.RFC3339, expiresAtStr)
			}
			if !expAt.After(obsAt) {
				expAt = obsAt.Add(defaultTTL)
			}

			evalAction, evalBand, evalStatus, evalReason := evaluateObservationAgainstPolicy(
				policy, status, score, confidence, traitsJSON, expAt, now,
			)

			summary = &domain.IPRiskSummary{
				Decision:              evalAction,
				RiskBand:              evalBand,
				Provider:              provider,
				ProviderSchemaVersion: schemaVersion,
				ObservedAt:            obsAt,
				ExpiresAt:             expAt,
				Status:                evalStatus,
				ReasonCode:            evalReason,
				PolicyRevisionID:      &revID,
			}
		}
	}

	return &domain.NodeReadModel{
		Node:          *node,
		IPRiskSummary: summary,
	}, nil
}

func (r *nodeRepository) resolveTargetRiskPolicy(ctx context.Context, requestedRevs []string) (*domain.RiskPolicy, error) {
	if len(requestedRevs) > 0 && requestedRevs[0] != "" {
		revID := requestedRevs[0]
		return r.loadRiskPolicy(ctx, "WHERE id = ?", revID)
	}
	return r.loadRiskPolicy(ctx, "WHERE active = 1 ORDER BY created_at DESC LIMIT 1")
}

func (r *nodeRepository) loadRiskPolicy(ctx context.Context, whereClause string, args ...any) (*domain.RiskPolicy, error) {
	query := fmt.Sprintf(`SELECT id, fusion_mode, max_observation_age_seconds, minimum_confidence,
		unknown_action, conflict_action, review_action
		FROM risk_policy_revisions %s;`, whereClause)

	var (
		policy                                            domain.RiskPolicy
		mode, unknownAction, conflictAction, reviewAction string
		maxAgeSeconds, minConfidence                      int
	)

	row := r.db.QueryRowContext(ctx, query, args...)
	err := row.Scan(&policy.RevisionID, &mode, &maxAgeSeconds, &minConfidence, &unknownAction, &conflictAction, &reviewAction)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query risk policy: %w", err)
	}

	policy.ProviderSelection.Mode = domain.RiskFusionMode(mode)
	policy.MaxObservationAge = time.Duration(maxAgeSeconds) * time.Second
	policy.MinimumConfidence = minConfidence
	policy.UnknownAction = domain.RiskAction(unknownAction)
	policy.ConflictAction = domain.RiskAction(conflictAction)
	policy.ReviewAction = domain.RiskAction(reviewAction)

	// Load providers
	provRows, err := r.db.QueryContext(ctx, "SELECT provider, schema_version FROM risk_policy_providers WHERE revision_id = ? ORDER BY position ASC;", policy.RevisionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load risk policy providers: %w", err)
	}
	defer provRows.Close()
	for provRows.Next() {
		var p, v string
		if err := provRows.Scan(&p, &v); err != nil {
			return nil, err
		}
		policy.ProviderSelection.Providers = append(policy.ProviderSelection.Providers, domain.ProviderRef{Provider: p, SchemaVersion: v})
	}

	// Load score bands
	bandRows, err := r.db.QueryContext(ctx, "SELECT min_score, max_score, band, action FROM risk_policy_score_bands WHERE revision_id = ? ORDER BY position ASC;", policy.RevisionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load risk policy score bands: %w", err)
	}
	defer bandRows.Close()
	for bandRows.Next() {
		var b domain.ScoreBand
		if err := bandRows.Scan(&b.Min, &b.Max, &b.Band, &b.Action); err != nil {
			return nil, err
		}
		policy.ScoreBands = append(policy.ScoreBands, b)
	}

	// Load trait rules
	traitRows, err := r.db.QueryContext(ctx, "SELECT trait, action FROM risk_policy_trait_rules WHERE revision_id = ?;", policy.RevisionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load risk policy trait rules: %w", err)
	}
	defer traitRows.Close()
	for traitRows.Next() {
		var t domain.TraitRule
		if err := traitRows.Scan(&t.Trait, &t.Action); err != nil {
			return nil, err
		}
		policy.TraitRules = append(policy.TraitRules, t)
	}

	return &policy, nil
}

func buildRiskEvaluationCTE(policy *domain.RiskPolicy, nowStr string, nodeIDs ...string) (string, []any) {
	var args []any

	primaryProvider := "unknown"
	primarySchemaVersion := "v1"
	if len(policy.ProviderSelection.Providers) > 0 {
		primaryProvider = policy.ProviderSelection.Providers[0].Provider
		primarySchemaVersion = policy.ProviderSelection.Providers[0].SchemaVersion
	}

	unknownAction := string(policy.EffectiveUnknownAction())
	minConfidence := policy.MinimumConfidence

	// Arg for ranked_obs CTE:
	args = append(args, policy.RevisionID)

	// Build CASE expressions for obs_eval
	// 1. Action CASE
	var actionCases []string
	var actionArgs []any

	actionCases = append(actionCases, "WHEN lo.status != 'available' THEN ?")
	actionArgs = append(actionArgs, unknownAction)

	actionCases = append(actionCases, "WHEN datetime(lo.expires_at) <= datetime(?) THEN ?")
	actionArgs = append(actionArgs, nowStr, unknownAction)

	if minConfidence > 0 {
		actionCases = append(actionCases, "WHEN lo.confidence IS NOT NULL AND lo.confidence < ? THEN ?")
		actionArgs = append(actionArgs, minConfidence, unknownAction)
	}

	for _, tr := range policy.TraitRules {
		actionCases = append(actionCases, "WHEN lo.anonymizer_traits LIKE ? THEN ?")
		actionArgs = append(actionArgs, fmt.Sprintf("%%\"%s\"%%", tr.Trait), string(tr.Action))
	}

	for _, sb := range policy.ScoreBands {
		actionCases = append(actionCases, "WHEN lo.score BETWEEN ? AND ? THEN ?")
		actionArgs = append(actionArgs, sb.Min, sb.Max, string(sb.Action))
	}

	actionCases = append(actionCases, "ELSE ?")
	actionArgs = append(actionArgs, unknownAction)
	actionExpr := fmt.Sprintf("CASE %s END", strings.Join(actionCases, " "))

	// 2. Band CASE
	var bandCases []string
	var bandArgs []any

	bandCases = append(bandCases, "WHEN lo.status != 'available' OR datetime(lo.expires_at) <= datetime(?) THEN 'unknown'")
	bandArgs = append(bandArgs, nowStr)

	if minConfidence > 0 {
		bandCases = append(bandCases, "WHEN lo.confidence IS NOT NULL AND lo.confidence < ? THEN 'unknown'")
		bandArgs = append(bandArgs, minConfidence)
	}

	for _, tr := range policy.TraitRules {
		bandCases = append(bandCases, "WHEN lo.anonymizer_traits LIKE ? THEN ?")
		bandVal := "critical"
		if tr.Action == domain.RiskActionReview {
			bandVal = "medium"
		} else if tr.Action == domain.RiskActionAllow {
			bandVal = "low"
		}
		bandArgs = append(bandArgs, fmt.Sprintf("%%\"%s\"%%", tr.Trait), bandVal)
	}

	for _, sb := range policy.ScoreBands {
		bandCases = append(bandCases, "WHEN lo.score BETWEEN ? AND ? THEN ?")
		bandArgs = append(bandArgs, sb.Min, sb.Max, string(sb.Band))
	}

	bandCases = append(bandCases, "ELSE 'unknown'")
	bandExpr := fmt.Sprintf("CASE %s END", strings.Join(bandCases, " "))

	// 3. Status CASE
	var statusCases []string
	var statusArgs []any
	statusCases = append(statusCases, "WHEN datetime(lo.expires_at) <= datetime(?) THEN 'stale'")
	statusArgs = append(statusArgs, nowStr)
	statusCases = append(statusCases, "ELSE lo.status")
	statusExpr := fmt.Sprintf("CASE %s END", strings.Join(statusCases, " "))

	// 4. Reason CASE
	var reasonCases []string
	var reasonArgs []any
	reasonCases = append(reasonCases, "WHEN lo.status != 'available' THEN 'observation_status_' || lo.status")
	reasonCases = append(reasonCases, "WHEN datetime(lo.expires_at) <= datetime(?) THEN 'observation_status_stale'")
	reasonArgs = append(reasonArgs, nowStr)
	if minConfidence > 0 {
		reasonCases = append(reasonCases, "WHEN lo.confidence IS NOT NULL AND lo.confidence < ? THEN 'confidence_below_minimum'")
		reasonArgs = append(reasonArgs, minConfidence)
	}
	for _, tr := range policy.TraitRules {
		reasonCases = append(reasonCases, "WHEN lo.anonymizer_traits LIKE ? THEN ?")
		reasonArgs = append(reasonArgs, fmt.Sprintf("%%\"%s\"%%", tr.Trait), fmt.Sprintf("trait_rule_%s", strings.ToLower(string(tr.Trait))))
	}
	for _, sb := range policy.ScoreBands {
		reasonCases = append(reasonCases, "WHEN lo.score BETWEEN ? AND ? THEN ?")
		reasonArgs = append(reasonArgs, sb.Min, sb.Max, fmt.Sprintf("score_band_%s", strings.ToLower(string(sb.Band))))
	}
	reasonCases = append(reasonCases, "ELSE 'score_within_range'")
	reasonExpr := fmt.Sprintf("CASE %s END", strings.Join(reasonCases, " "))

	// Append all CTE expr args in order
	args = append(args, actionArgs...)
	args = append(args, bandArgs...)
	args = append(args, statusArgs...)
	args = append(args, reasonArgs...)

	// Args for COALESCE defaults in node_eval:
	args = append(args, unknownAction, primaryProvider, primarySchemaVersion)

	cteSQL := fmt.Sprintf(`
		WITH ranked_obs AS (
			SELECT o.id AS obs_id,
			       o.node_logical_id,
			       o.provider,
			       o.provider_schema_version,
			       o.observed_at,
			       o.expires_at,
			       o.status,
			       o.score,
			       o.confidence,
			       o.anonymizer_traits,
			       ROW_NUMBER() OVER (
			           PARTITION BY o.node_logical_id
			           ORDER BY o.observed_at DESC, o.id DESC
			       ) AS rn
			FROM ip_risk_observations o
			WHERE EXISTS (
				SELECT 1
				FROM risk_policy_providers rpp
				WHERE rpp.revision_id = ?
				  AND rpp.provider = o.provider
				  AND rpp.schema_version = o.provider_schema_version
			)
		),
		latest_obs AS (
			SELECT obs_id,
			       node_logical_id,
			       provider,
			       provider_schema_version,
			       observed_at,
			       expires_at,
			       status,
			       score,
			       confidence,
			       anonymizer_traits
			FROM ranked_obs
			WHERE rn = 1
		),
		obs_eval AS (
			SELECT lo.node_logical_id,
			       lo.provider,
			       lo.provider_schema_version,
			       lo.observed_at,
			       lo.expires_at,
			       %s AS action,
			       %s AS band,
			       %s AS eval_status,
			       %s AS reason_code
			FROM latest_obs lo
		),
		node_eval AS (
			SELECT n.logical_id,
			       n.protocol,
			       n.display_name,
			       n.normalized_config_secret_ref,
			       n.credential_version,
			       n.active,
			       n.created_at,
			       n.updated_at,
			       COALESCE(oe.action, ?) AS risk_decision,
			       COALESCE(oe.band, 'unknown') AS risk_band,
			       COALESCE(oe.provider, ?) AS risk_provider,
			       COALESCE(oe.provider_schema_version, ?) AS risk_schema_version,
			       COALESCE(oe.eval_status, 'unknown') AS risk_status,
			       COALESCE(oe.reason_code, 'missing_observation') AS risk_reason_code,
			       oe.observed_at AS risk_observed_at,
			       oe.expires_at AS risk_expires_at
			FROM nodes n
			LEFT JOIN obs_eval oe ON n.logical_id = oe.node_logical_id
		)
	`, actionExpr, bandExpr, statusExpr, reasonExpr)

	return cteSQL, args
}

func buildNoPolicyCTE() (string, []any) {
	cteSQL := `
		WITH node_eval AS (
			SELECT n.logical_id,
			       n.protocol,
			       n.display_name,
			       n.normalized_config_secret_ref,
			       n.credential_version,
			       n.active,
			       n.created_at,
			       n.updated_at,
			       'unknown' AS risk_decision,
			       'unknown' AS risk_band,
			       '' AS risk_provider,
			       '' AS risk_schema_version,
			       'unknown' AS risk_status,
			       'no_active_risk_policy' AS risk_reason_code,
			       NULL AS risk_observed_at,
			       NULL AS risk_expires_at
			FROM nodes n
		)
	`
	return cteSQL, nil
}

func evaluateObservationAgainstPolicy(
	policy *domain.RiskPolicy,
	status string,
	score, confidence sql.NullInt64,
	traitsJSON string,
	expiresAt time.Time,
	now time.Time,
) (domain.RiskAction, domain.RiskBand, domain.IPRiskStatus, string) {
	unknownAction := policy.EffectiveUnknownAction()

	if status != string(domain.IPRiskStatusAvailable) {
		return unknownAction, domain.RiskBandUnknown, domain.IPRiskStatus(status), "observation_status_" + status
	}

	if !now.Before(expiresAt) {
		return unknownAction, domain.RiskBandUnknown, domain.IPRiskStatusStale, "observation_status_stale"
	}

	if policy.MinimumConfidence > 0 {
		if !confidence.Valid || int(confidence.Int64) < policy.MinimumConfidence {
			return unknownAction, domain.RiskBandUnknown, domain.IPRiskStatusAvailable, "confidence_below_minimum"
		}
	}

	// Check trait rules
	for _, tr := range policy.TraitRules {
		if strings.Contains(traitsJSON, fmt.Sprintf(`"%s"`, tr.Trait)) {
			band := domain.RiskBandCritical
			if tr.Action == domain.RiskActionReview {
				band = domain.RiskBandMedium
			} else if tr.Action == domain.RiskActionAllow {
				band = domain.RiskBandLow
			}
			return tr.Action, band, domain.IPRiskStatusAvailable, fmt.Sprintf("trait_rule_%s", strings.ToLower(string(tr.Trait)))
		}
	}

	// Check score bands
	if score.Valid {
		scoreVal := int(score.Int64)
		for _, sb := range policy.ScoreBands {
			if scoreVal >= sb.Min && scoreVal <= sb.Max {
				return sb.Action, sb.Band, domain.IPRiskStatusAvailable, fmt.Sprintf("score_band_%s", strings.ToLower(string(sb.Band)))
			}
		}
	}

	return unknownAction, domain.RiskBandUnknown, domain.IPRiskStatusAvailable, "missing_score"
}

func normalizeRiskReasonCode(reason string) string {
	cleaned := strings.ToLower(strings.TrimSpace(reason))
	if cleaned == "" {
		return "missing_observation"
	}
	var b strings.Builder
	for _, r := range cleaned {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	res := b.String()
	for strings.Contains(res, "__") {
		res = strings.ReplaceAll(res, "__", "_")
	}
	return strings.Trim(res, "_")
}

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
	query := fmt.Sprintf("SELECT logical_id, protocol, display_name, normalized_config_secret_ref, credential_version, active, created_at, updated_at FROM nodes%s ORDER BY %s LIMIT ? OFFSET ?;", whereSQL, orderBy)
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
		if err := rows.Scan(&n.LogicalID, &n.Protocol, &n.DisplayName, &n.NormalizedConfigSecretRef, &n.CredentialVersion, &active, &created, &updated); err != nil {
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
	observations, err := r.latestObservationsForNodes(ctx, ids, policy)
	if err != nil {
		return nil, 0, err
	}
	now := domain.NowUTC()
	rev := policy.RevisionID
	provider, schema := primaryRiskProvider(policy)
	ttl := defaultRiskTTL(policy)
	for i := range models {
		obs, ok := observations[models[i].Node.LogicalID]
		if !ok {
			models[i].IPRiskSummary = &domain.IPRiskSummary{
				Decision:              policy.EffectiveUnknownAction(),
				RiskBand:              domain.RiskBandUnknown,
				Provider:              provider,
				ProviderSchemaVersion: schema,
				ObservedAt:            now,
				ExpiresAt:             now.Add(ttl),
				Status:                domain.IPRiskStatusUnknown,
				ReasonCode:            "missing_observation",
				PolicyRevisionID:      &rev,
			}
			continue
		}
		expAt := obs.expiresAt
		if !expAt.After(obs.observedAt) {
			expAt = obs.observedAt.Add(ttl)
		}
		action, band, status, reason := evaluateObservationAgainstPolicy(policy, obs.status, obs.score, obs.confidence, obs.traits, expAt, now)
		models[i].IPRiskSummary = &domain.IPRiskSummary{
			Decision:              action,
			RiskBand:              band,
			Provider:              obs.provider,
			ProviderSchemaVersion: obs.schema,
			ObservedAt:            obs.observedAt,
			ExpiresAt:             expAt,
			Status:                status,
			ReasonCode:            reason,
			PolicyRevisionID:      &rev,
		}
	}
	return models, total, nil
}

func (r *nodeRepository) latestObservationsForNodes(ctx context.Context, ids []string, policy *domain.RiskPolicy) (map[string]latestRiskObservation, error) {
	return r.loadObservationsForNodes(ctx, ids, policy)
}

func (r *nodeRepository) loadObservationsForNodes(ctx context.Context, ids []string, policy *domain.RiskPolicy) (map[string]latestRiskObservation, error) {
	if len(ids) == 0 || policy == nil || len(policy.ProviderSelection.Providers) == 0 {
		return map[string]latestRiskObservation{}, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	args = append(args, policy.RevisionID)
	q := `
		WITH ranked_obs AS (
			SELECT o.id,
			       o.node_logical_id,
			       o.provider,
			       o.provider_schema_version,
			       o.observed_at,
			       o.expires_at,
			       o.status,
			       o.score,
			       o.confidence,
			       o.anonymizer_traits,
			       ROW_NUMBER() OVER (
			           PARTITION BY o.node_logical_id
			           ORDER BY o.observed_at DESC, o.id DESC
			       ) AS rn
			FROM ip_risk_observations o
			WHERE o.node_logical_id IN (` + strings.Join(ph, ",") + `)
			  AND EXISTS (
			      SELECT 1
			      FROM risk_policy_providers rpp
			      WHERE rpp.revision_id = ?
			        AND rpp.provider = o.provider
			        AND rpp.schema_version = o.provider_schema_version
			  )
		)
		SELECT id,
		       node_logical_id,
		       provider,
		       provider_schema_version,
		       observed_at,
		       expires_at,
		       status,
		       score,
		       confidence,
		       anonymizer_traits
		FROM ranked_obs
		WHERE rn = 1;`
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
		result[nodeID] = o
	}
	return result, rows.Err()
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
