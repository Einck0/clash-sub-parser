package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// ConsistentFactsLoader defines the port for transactional loading of consistent domain facts.
type ConsistentFactsLoader = domain.ConsistentFactsLoader
type ConsistentFacts = domain.ConsistentFacts

type consistentFactsLoader struct {
	db *sql.DB
}

// NewConsistentFactsLoader constructs a SQLite facts loader that loads all facts in a single read transaction.
func NewConsistentFactsLoader(db *sql.DB) ConsistentFactsLoader {
	return &consistentFactsLoader{db: db}
}

func (l *consistentFactsLoader) LoadConsistentFacts(ctx context.Context, revisionID string, target domain.CompilerTarget, asOf time.Time) (*ConsistentFacts, error) {
	if asOf.IsZero() {
		asOf = domain.NowUTC()
	}

	tx, err := l.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin read transaction for consistent facts: %w", err)
	}
	defer tx.Rollback()

	facts := &ConsistentFacts{
		AsOf:               asOf,
		GroupSources:       make(map[string][]domain.MemberSource),
		GroupEdges:         make(map[string][]domain.GroupEdge),
		GroupFilters:       make(map[string]domain.NodeFilterSpec),
		NodeSources:        make(map[string][]domain.NodeSource),
		LatestObservations: make(map[string]map[domain.ProbeKind]domain.ProbeObservation),
	}

	// 1. Revision
	rev, err := loadRevision(ctx, tx, revisionID)
	if err != nil {
		return nil, err
	}
	facts.Revision = rev

	// 2. Admission Rules
	admRules, err := loadAdmissionRules(ctx, tx, rev.ID)
	if err != nil {
		return nil, err
	}
	facts.AdmissionRules = admRules

	// 3. Mihomo Graph (Groups, Edges, Group Filters, Policy Rules)
	if target == domain.TargetMihomo {
		if err := loadMihomoGraph(ctx, tx, rev, facts); err != nil {
			return nil, err
		}
	}

	// 4. Global Node Filter
	gf, err := loadGlobalNodeFilter(ctx, tx)
	if err != nil {
		return nil, err
	}
	facts.GlobalFilter = gf

	// 5. Active Nodes
	nodes, nodeIDs, err := loadActiveNodes(ctx, tx)
	if err != nil {
		return nil, err
	}
	facts.Nodes = nodes

	// 6. Node Sources and Observations (inspected conditionally based on filter dependencies)
	needsSources, needsObservations := inspectFilterDependencies(facts.GlobalFilter, facts.GroupFilters, facts.Groups)
	if needsSources && len(nodeIDs) > 0 {
		if err := loadNodeSources(ctx, tx, nodeIDs, facts.NodeSources); err != nil {
			return nil, err
		}
	}
	if needsObservations && len(nodeIDs) > 0 {
		if err := loadProbeObservations(ctx, tx, nodeIDs, facts.LatestObservations); err != nil {
			return nil, err
		}
	}

	// 7. Active Risk Policy & Observations
	if err := loadActiveRiskPolicyAndObservations(ctx, tx, nodeIDs, facts); err != nil {
		return nil, err
	}

	// 8. Primary Subscription UserInfo
	var primaryUserInfo sql.NullString
	_ = tx.QueryRowContext(ctx, "SELECT userinfo FROM subscriptions WHERE is_primary = 1 AND enabled = 1 LIMIT 1;").Scan(&primaryUserInfo)
	if primaryUserInfo.Valid {
		facts.PrimaryUserInfo = primaryUserInfo.String
	}

	// 9. DNS Settings
	dns, err := loadDNSSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	facts.DNSSettings = dns

	return facts, nil
}

func loadDNSSettings(ctx context.Context, tx *sql.Tx) (*domain.DNSSettings, error) {
	const q = `SELECT enabled, raw_yaml, revision, updated_at FROM dns_config WHERE id = 1;`
	var enabledInt int
	var rawYAML string
	var revision int64
	var updatedAt time.Time
	err := tx.QueryRowContext(ctx, q).Scan(&enabledInt, &rawYAML, &revision, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			def := domain.DefaultDNSSettings()
			return &def, nil
		}
		return nil, fmt.Errorf("failed to load dns settings: %w", err)
	}
	return &domain.DNSSettings{
		Enabled:   enabledInt == 1,
		RawYAML:   rawYAML,
		Revision:  revision,
		UpdatedAt: updatedAt,
		Source:    "saved",
	}, nil
}

func loadRevision(ctx context.Context, tx *sql.Tx, revisionID string) (*domain.ConfigurationRevision, error) {
	var rev domain.ConfigurationRevision
	var parentID sql.NullString
	var stateStr, createdStr string

	if strings.TrimSpace(revisionID) != "" {
		const q = `
		SELECT id, parent_id, content_digest, state, created_at
		FROM configuration_revisions
		WHERE id = ?;`
		err := tx.QueryRowContext(ctx, q, strings.TrimSpace(revisionID)).Scan(&rev.ID, &parentID, &rev.ContentDigest, &stateStr, &createdStr)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, domain.NewNotFoundError("revision_not_found", fmt.Sprintf("configuration revision %s not found", revisionID))
			}
			return nil, fmt.Errorf("failed to query configuration revision: %w", err)
		}
	} else {
		const q = `
		SELECT id, parent_id, content_digest, state, created_at
		FROM configuration_revisions
		WHERE state = 'active'
		ORDER BY created_at DESC
		LIMIT 1;`
		err := tx.QueryRowContext(ctx, q).Scan(&rev.ID, &parentID, &rev.ContentDigest, &stateStr, &createdStr)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, domain.NewConflictError("no_active_revision", "cannot resolve policy without an active configuration revision")
			}
			return nil, fmt.Errorf("failed to query configuration revision: %w", err)
		}
	}

	if parentID.Valid && parentID.String != "" {
		rev.ParentID = &parentID.String
	}
	rev.State = domain.ConfigurationRevisionState(stateStr)
	rev.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	if rev.CreatedAt.IsZero() {
		rev.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	}
	return &rev, nil
}

func loadAdmissionRules(ctx context.Context, tx *sql.Tx, revID string) ([]domain.AdmissionRule, error) {
	const admissionQuery = `
	SELECT id, revision_id, name, expression, action, position
	FROM admission_rules
	WHERE revision_id = ?
	ORDER BY position ASC;`
	aRows, err := tx.QueryContext(ctx, admissionQuery, revID)
	if err != nil {
		return nil, fmt.Errorf("failed to list admission rules: %w", err)
	}
	defer aRows.Close()

	var rules []domain.AdmissionRule
	for aRows.Next() {
		var ar domain.AdmissionRule
		var actionStr string
		if err := aRows.Scan(&ar.ID, &ar.RevisionID, &ar.Name, &ar.Expression, &actionStr, &ar.Position); err != nil {
			return nil, fmt.Errorf("failed to scan admission rule: %w", err)
		}
		ar.Action = domain.RuleAction(actionStr)
		rules = append(rules, ar)
	}
	if err := aRows.Err(); err != nil {
		return nil, fmt.Errorf("failed during admission rules iteration: %w", err)
	}
	return rules, nil
}

func loadMihomoGraph(ctx context.Context, tx *sql.Tx, rev *domain.ConfigurationRevision, facts *ConsistentFacts) error {
	if rev.State != domain.RevisionStateActive {
		var snapJSON string
		err := tx.QueryRowContext(ctx, `SELECT groups_json FROM policy_revision_snapshots WHERE revision_id = ?;`, rev.ID).Scan(&snapJSON)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to query policy revision snapshot: %w", err)
		}
		if err == nil && snapJSON != "" {
			var snapGroups []domain.NodeGroup
			if err := json.Unmarshal([]byte(snapJSON), &snapGroups); err != nil {
				return fmt.Errorf("failed to unmarshal policy revision snapshot groups: %w", err)
			}
			facts.Groups = snapGroups
			for _, g := range snapGroups {
				facts.GroupSources[g.ID] = g.Sources
				facts.GroupEdges[g.ID] = domain.ProjectSourceEdges(g.Sources)
			}
			return loadPolicyRules(ctx, tx, rev.ID, facts)
		}
	}

	if err := loadNodeGroups(ctx, tx, facts); err != nil {
		return err
	}
	if err := loadPolicyGroupSources(ctx, tx, facts); err != nil {
		return err
	}
	if err := loadGroupNodeFilters(ctx, tx, facts); err != nil {
		return err
	}
	return loadPolicyRules(ctx, tx, rev.ID, facts)
}

func loadNodeGroups(ctx context.Context, tx *sql.Tx, facts *ConsistentFacts) error {
	const groupQuery = `
	SELECT id, name, group_type, created_at, updated_at, empty_fallback_pass, COALESCE(revision, 1), position, COALESCE(inherit_global_filter, 1)
	FROM node_groups
	ORDER BY position ASC, created_at ASC, id ASC;`
	gRows, err := tx.QueryContext(ctx, groupQuery)
	if err != nil {
		return fmt.Errorf("failed to list node groups: %w", err)
	}
	defer gRows.Close()

	for gRows.Next() {
		var group domain.NodeGroup
		var groupTypeStr, createdStr, updatedStr string
		if err := gRows.Scan(
			&group.ID,
			&group.Name,
			&groupTypeStr,
			&createdStr,
			&updatedStr,
			&group.EmptyFallbackPass,
			&group.Revision,
			&group.Position,
			&group.InheritGlobalFilter,
		); err != nil {
			return fmt.Errorf("failed to scan node group: %w", err)
		}
		group.GroupType = domain.GroupType(groupTypeStr)
		group.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		group.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		facts.Groups = append(facts.Groups, group)
	}
	if err := gRows.Err(); err != nil {
		return fmt.Errorf("failed during node groups iteration: %w", err)
	}
	return nil
}

func loadPolicyGroupSources(ctx context.Context, tx *sql.Tx, facts *ConsistentFacts) error {
	const pgsQuery = `
	SELECT id, group_id, operation, kind, position, node_logical_id, target_group_id, pattern, filter_spec_json, builtin_name, created_at, updated_at
	FROM policy_group_sources
	ORDER BY group_id ASC, position ASC;`
	pgsRows, err := tx.QueryContext(ctx, pgsQuery)
	if err != nil {
		return fmt.Errorf("failed to query policy group sources: %w", err)
	}
	defer pgsRows.Close()

	for pgsRows.Next() {
		var s domain.MemberSource
		var opStr, kindStr, cStr, uStr string
		var nodeID, targetID, pattern, filterJSON, builtin sql.NullString
		if err := pgsRows.Scan(
			&s.ID,
			&s.GroupID,
			&opStr,
			&kindStr,
			&s.Position,
			&nodeID,
			&targetID,
			&pattern,
			&filterJSON,
			&builtin,
			&cStr,
			&uStr,
		); err != nil {
			return fmt.Errorf("failed to scan policy group source: %w", err)
		}
		s.Operation = domain.SourceOperation(opStr)
		s.Kind = domain.MemberSourceKind(kindStr)
		if nodeID.Valid {
			s.NodeLogicalID = &nodeID.String
		}
		if targetID.Valid {
			s.TargetGroupID = &targetID.String
		}
		if pattern.Valid {
			s.Pattern = &pattern.String
		}
		if filterJSON.Valid && filterJSON.String != "" {
			var spec domain.NodeFilterSpec
			if err := json.Unmarshal([]byte(filterJSON.String), &spec); err != nil {
				return fmt.Errorf("failed to unmarshal member source filter: %w", err)
			}
			s.FilterSpec = &spec
		}
		if builtin.Valid {
			s.BuiltinName = &builtin.String
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, cStr)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, uStr)
		facts.GroupSources[s.GroupID] = append(facts.GroupSources[s.GroupID], s)
	}
	if err := pgsRows.Err(); err != nil {
		return fmt.Errorf("failed during policy group sources iteration: %w", err)
	}

	for i := range facts.Groups {
		facts.Groups[i].Sources = facts.GroupSources[facts.Groups[i].ID]
		facts.GroupEdges[facts.Groups[i].ID] = domain.ProjectSourceEdges(facts.Groups[i].Sources)
	}
	return nil
}

func loadGroupNodeFilters(ctx context.Context, tx *sql.Tx, facts *ConsistentFacts) error {
	const gnfQuery = `SELECT group_id, filter_spec FROM group_node_filters;`
	gnfRows, err := tx.QueryContext(ctx, gnfQuery)
	if err != nil {
		return fmt.Errorf("failed to query group node filters: %w", err)
	}
	defer gnfRows.Close()

	for gnfRows.Next() {
		var gid, fJSON string
		if err := gnfRows.Scan(&gid, &fJSON); err != nil {
			return fmt.Errorf("failed to scan group node filter: %w", err)
		}
		if fJSON == "" {
			continue
		}
		var spec domain.NodeFilterSpec
		if err := json.Unmarshal([]byte(fJSON), &spec); err != nil {
			return fmt.Errorf("failed to unmarshal group node filter spec: %w", err)
		}
		if !spec.IsEmpty() {
			if _, exists := facts.GroupFilters[gid]; !exists {
				facts.GroupFilters[gid] = spec
			}
		}
	}
	if err := gnfRows.Err(); err != nil {
		return fmt.Errorf("failed during group node filters iteration: %w", err)
	}
	return nil
}

func loadPolicyRules(ctx context.Context, tx *sql.Tx, revID string, facts *ConsistentFacts) error {
	const policyRuleQuery = `
	SELECT r.id, r.revision_id, COALESCE(r.category_id, ''), r.category, r.name, COALESCE(r.target_group_id, ''), COALESCE(r.builtin_target, ''), r.expression, r.position, r.enabled
	FROM policy_rules r
	LEFT JOIN rule_categories c ON (r.category_id IS NOT NULL AND r.category_id = c.id) OR (r.category_id IS NULL AND r.category = c.name)
	WHERE r.revision_id = ?
	ORDER BY COALESCE(c.sort_order, 999999) ASC, r.position ASC, r.id ASC;`
	prRows, err := tx.QueryContext(ctx, policyRuleQuery, revID)
	if err != nil {
		return fmt.Errorf("failed to list policy rules: %w", err)
	}
	defer prRows.Close()

	for prRows.Next() {
		var rule domain.PolicyRule
		var enabledInt int
		if err := prRows.Scan(
			&rule.ID,
			&rule.RevisionID,
			&rule.CategoryID,
			&rule.Category,
			&rule.Name,
			&rule.TargetGroupID,
			&rule.BuiltinTarget,
			&rule.Expression,
			&rule.Position,
			&enabledInt,
		); err != nil {
			return fmt.Errorf("failed to scan policy rule: %w", err)
		}
		rule.Enabled = enabledInt == 1
		facts.PolicyRules = append(facts.PolicyRules, rule)
	}
	if err := prRows.Err(); err != nil {
		return fmt.Errorf("failed during policy rules iteration: %w", err)
	}
	return nil
}

func loadGlobalNodeFilter(ctx context.Context, tx *sql.Tx) (*domain.NodeFilterSpec, error) {
	var gfJSON string
	err := tx.QueryRowContext(ctx, `SELECT filter_spec FROM global_node_filters WHERE id = 1;`).Scan(&gfJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query global node filter: %w", err)
	}
	if gfJSON == "" || gfJSON == "{}" {
		return nil, nil
	}
	var spec domain.NodeFilterSpec
	if err := json.Unmarshal([]byte(gfJSON), &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal global node filter: %w", err)
	}
	if spec.IsEmpty() {
		return nil, nil
	}
	return &spec, nil
}

func loadActiveNodes(ctx context.Context, tx *sql.Tx) ([]domain.Node, []string, error) {
	const selectNodesSQL = `
	SELECT n.logical_id, n.protocol, n.display_name,
	       COALESCE(json_extract(v.effective_config_json, '$.server'), n.server),
	       COALESCE(json_extract(v.effective_config_json, '$.port'), n.port),
	       COALESCE(json_extract(v.effective_config_json, '$.credentials'), v.effective_config_json, n.config_json),
	       n.active, n.created_at, n.updated_at,
	       COALESCE(h.connection_revision, n.connection_revision)
	FROM nodes n
	LEFT JOIN node_connection_heads h ON n.logical_id = h.logical_id
	LEFT JOIN node_connection_versions v ON h.logical_id = v.node_logical_id AND h.connection_revision = v.connection_revision
	WHERE n.logical_id IN (` + enabledSubscriptionsInventorySubquery + `)
	  AND n.logical_id NOT IN (` + confirmedNoticeExclusionSubquery + `)
	  AND n.active = 1
	ORDER BY n.created_at DESC, n.logical_id ASC;`

	nRows, err := tx.QueryContext(ctx, selectNodesSQL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query active nodes: %w", err)
	}
	defer nRows.Close()

	var nodes []domain.Node
	var nodeIDs []string
	for nRows.Next() {
		var node domain.Node
		var configJSON string
		var activeInt int
		var cStr, uStr string

		if err := nRows.Scan(
			&node.LogicalID,
			&node.Protocol,
			&node.DisplayName,
			&node.Server,
			&node.Port,
			&configJSON,
			&activeInt,
			&cStr,
			&uStr,
			&node.ConnectionRevision,
		); err != nil {
			return nil, nil, fmt.Errorf("failed to scan node: %w", err)
		}

		creds, err := unmarshalNodeCredentials(configJSON)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal credentials for node %s: %w", node.LogicalID, err)
		}
		node.Credentials = creds
		node.Active = activeInt == 1
		node.CreatedAt, _ = time.Parse(time.RFC3339Nano, cStr)
		if node.CreatedAt.IsZero() {
			node.CreatedAt, _ = time.Parse(time.RFC3339, cStr)
		}
		node.UpdatedAt, _ = time.Parse(time.RFC3339Nano, uStr)
		if node.UpdatedAt.IsZero() {
			node.UpdatedAt, _ = time.Parse(time.RFC3339, uStr)
		}

		nodes = append(nodes, node)
		nodeIDs = append(nodeIDs, node.LogicalID)
	}
	if err := nRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed during active nodes iteration: %w", err)
	}
	return nodes, nodeIDs, nil
}

func loadNodeSources(ctx context.Context, tx *sql.Tx, nodeIDs []string, nodeSources map[string][]domain.NodeSource) error {
	const chunkSize = 100
	for i := 0; i < len(nodeIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}
		chunk := nodeIDs[i:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}
		q := fmt.Sprintf(`SELECT node_logical_id, subscription_id, last_seen_fetch_id FROM node_sources WHERE node_logical_id IN (%s);`, strings.Join(placeholders, ","))
		sRows, err := tx.QueryContext(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("failed to query node sources: %w", err)
		}

		for sRows.Next() {
			var s domain.NodeSource
			if err := sRows.Scan(&s.NodeLogicalID, &s.SubscriptionID, &s.LastSeenFetchID); err != nil {
				sRows.Close()
				return fmt.Errorf("failed to scan node source: %w", err)
			}
			nodeSources[s.NodeLogicalID] = append(nodeSources[s.NodeLogicalID], s)
		}
		sRows.Close()
		if err := sRows.Err(); err != nil {
			return fmt.Errorf("failed during node sources iteration: %w", err)
		}
	}
	return nil
}

func loadProbeObservations(ctx context.Context, tx *sql.Tx, nodeIDs []string, latestObs map[string]map[domain.ProbeKind]domain.ProbeObservation) error {
	const chunkSize = 100
	for i := 0; i < len(nodeIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}
		chunk := nodeIDs[i:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}
		query := buildLatestByNodesQuery(placeholders, nil)
		obsRows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("failed to query probe observations: %w", err)
		}

		for obsRows.Next() {
			var obs domain.ProbeObservation
			var kindStr, verdictStr, observedStr string
			if err := obsRows.Scan(
				&obs.ID,
				&obs.ProbeRunID,
				&obs.NodeLogicalID,
				&kindStr,
				&verdictStr,
				&obs.EvidenceDigest,
				&observedStr,
				&obs.LatencyMS,
				&obs.RedactedSummary,
				&obs.ConnectionRevision,
				&obs.EvidenceData,
			); err != nil {
				obsRows.Close()
				return fmt.Errorf("failed to scan probe observation: %w", err)
			}
			obs.Kind = domain.ProbeKind(kindStr)
			obs.Verdict = domain.ProbeVerdict(verdictStr)
			obs.ObservedAt, _ = time.Parse(time.RFC3339Nano, observedStr)
			if obs.ObservedAt.IsZero() {
				obs.ObservedAt, _ = time.Parse(time.RFC3339, observedStr)
			}
			obs.SyncEvidenceData()
			if latestObs[obs.NodeLogicalID] == nil {
				latestObs[obs.NodeLogicalID] = make(map[domain.ProbeKind]domain.ProbeObservation)
			}
			latestObs[obs.NodeLogicalID][obs.Kind] = obs
		}
		obsRows.Close()
		if err := obsRows.Err(); err != nil {
			return fmt.Errorf("failed during probe observations iteration: %w", err)
		}
	}
	return nil
}

func loadActiveRiskPolicyAndObservations(ctx context.Context, tx *sql.Tx, nodeIDs []string, facts *ConsistentFacts) error {
	const riskPolicyQuery = `
	SELECT id, fusion_mode, max_observation_age_seconds, minimum_confidence,
	       unknown_action, conflict_action, review_action, active, created_at, activated_at, deactivated_at, rules_digest
	FROM risk_policy_revisions WHERE active = 1 ORDER BY created_at DESC LIMIT 1;`
	var riskRev domain.RiskPolicyRevision
	var mode, unkAct, confAct, revAct string
	var maxAgeSec, minConf, rActive int
	var rCreatedStr, rulesDigest string
	var rActivatedStr, rDeactivatedStr sql.NullString

	rpErr := tx.QueryRowContext(ctx, riskPolicyQuery).Scan(
		&riskRev.RevisionID,
		&mode,
		&maxAgeSec,
		&minConf,
		&unkAct,
		&confAct,
		&revAct,
		&rActive,
		&rCreatedStr,
		&rActivatedStr,
		&rDeactivatedStr,
		&rulesDigest,
	)
	if rpErr != nil {
		if errors.Is(rpErr, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("failed to query active risk policy revision: %w", rpErr)
	}

	riskRev.Active = rActive == 1
	riskRev.RulesDigest = rulesDigest
	riskRev.ProviderSelection.Mode = domain.RiskFusionMode(mode)
	riskRev.MaxObservationAge = time.Duration(maxAgeSec) * time.Second
	riskRev.MinimumConfidence = minConf
	riskRev.UnknownAction = domain.RiskAction(unkAct)
	riskRev.ConflictAction = domain.RiskAction(confAct)
	riskRev.ReviewAction = domain.RiskAction(revAct)
	riskRev.CreatedAt, _ = time.Parse(time.RFC3339Nano, rCreatedStr)
	if riskRev.CreatedAt.IsZero() {
		riskRev.CreatedAt, _ = time.Parse(time.RFC3339, rCreatedStr)
	}
	if rActivatedStr.Valid && rActivatedStr.String != "" {
		t, _ := time.Parse(time.RFC3339Nano, rActivatedStr.String)
		if t.IsZero() {
			t, _ = time.Parse(time.RFC3339, rActivatedStr.String)
		}
		riskRev.ActivatedAt = &t
	}
	if rDeactivatedStr.Valid && rDeactivatedStr.String != "" {
		t, _ := time.Parse(time.RFC3339Nano, rDeactivatedStr.String)
		if t.IsZero() {
			t, _ = time.Parse(time.RFC3339, rDeactivatedStr.String)
		}
		riskRev.DeactivatedAt = &t
	}

	// Providers
	pRows, err := tx.QueryContext(ctx, "SELECT provider, schema_version FROM risk_policy_providers WHERE revision_id = ? ORDER BY position ASC;", riskRev.RevisionID)
	if err != nil {
		return fmt.Errorf("failed to list risk policy providers: %w", err)
	}
	for pRows.Next() {
		var p, v string
		if err := pRows.Scan(&p, &v); err != nil {
			pRows.Close()
			return fmt.Errorf("failed to scan risk policy provider: %w", err)
		}
		riskRev.ProviderSelection.Providers = append(riskRev.ProviderSelection.Providers, domain.ProviderRef{Provider: p, SchemaVersion: v})
	}
	pRows.Close()
	if err := pRows.Err(); err != nil {
		return fmt.Errorf("failed during risk policy providers iteration: %w", err)
	}

	// Score Bands
	bRows, err := tx.QueryContext(ctx, "SELECT min_score, max_score, band, action FROM risk_policy_score_bands WHERE revision_id = ? ORDER BY position ASC;", riskRev.RevisionID)
	if err != nil {
		return fmt.Errorf("failed to list risk policy score bands: %w", err)
	}
	for bRows.Next() {
		var b domain.ScoreBand
		if err := bRows.Scan(&b.Min, &b.Max, &b.Band, &b.Action); err != nil {
			bRows.Close()
			return fmt.Errorf("failed to scan risk policy score band: %w", err)
		}
		riskRev.ScoreBands = append(riskRev.ScoreBands, b)
	}
	bRows.Close()
	if err := bRows.Err(); err != nil {
		return fmt.Errorf("failed during risk policy score bands iteration: %w", err)
	}

	// Trait Rules
	tRows, err := tx.QueryContext(ctx, "SELECT trait, action FROM risk_policy_trait_rules WHERE revision_id = ? ORDER BY trait ASC;", riskRev.RevisionID)
	if err != nil {
		return fmt.Errorf("failed to list risk policy trait rules: %w", err)
	}
	for tRows.Next() {
		var tr domain.TraitRule
		if err := tRows.Scan(&tr.Trait, &tr.Action); err != nil {
			tRows.Close()
			return fmt.Errorf("failed to scan risk policy trait rule: %w", err)
		}
		riskRev.TraitRules = append(riskRev.TraitRules, tr)
	}
	tRows.Close()
	if err := tRows.Err(); err != nil {
		return fmt.Errorf("failed during risk policy trait rules iteration: %w", err)
	}

	facts.ActiveRiskPolicy = &riskRev

	// IP Risk Observations for loaded nodes
	if len(nodeIDs) == 0 {
		return nil
	}

	const chunkSize = 100
	for i := 0; i < len(nodeIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}
		chunk := nodeIDs[i:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}
		q := fmt.Sprintf(`SELECT %s FROM ip_risk_observations WHERE node_logical_id IN (%s) ORDER BY observed_at DESC, id DESC;`, ipRiskObservationColumns, strings.Join(placeholders, ","))
		oRows, err := tx.QueryContext(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("failed to query ip risk observations: %w", err)
		}

		for oRows.Next() {
			obs, err := scanIPRiskObservation(oRows)
			if err != nil {
				oRows.Close()
				return fmt.Errorf("failed to scan ip risk observation: %w", err)
			}
			if obs != nil {
				facts.RiskObservations = append(facts.RiskObservations, *obs)
			}
		}
		oRows.Close()
		if err := oRows.Err(); err != nil {
			return fmt.Errorf("failed during ip risk observations iteration: %w", err)
		}
	}

	return nil
}

func inspectFilterDependencies(global *domain.NodeFilterSpec, groupFilters map[string]domain.NodeFilterSpec, groups []domain.NodeGroup) (sources, observations bool) {
	inspect := func(spec *domain.NodeFilterSpec) {
		if spec == nil {
			return
		}
		for _, condition := range spec.Conditions {
			switch condition.Field {
			case domain.FilterFieldSourceSubscriptions:
				sources = true
			case domain.FilterFieldProbeVerdict, domain.FilterFieldProbeLatencyMS, domain.FilterFieldHealthStatus, domain.FilterFieldExitRegion, domain.FilterFieldPlatformVerdict:
				observations = true
			}
		}
	}
	inspect(global)
	for _, spec := range groupFilters {
		inspect(&spec)
	}
	for _, group := range groups {
		if group.NodeFilter != nil {
			inspect(group.NodeFilter)
		}
		for _, src := range group.Sources {
			if src.FilterSpec != nil {
				inspect(src.FilterSpec)
			}
		}
	}
	return sources, observations
}
