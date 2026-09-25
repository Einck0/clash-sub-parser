package legacy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/import/legacy/inspect"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"

	_ "modernc.org/sqlite"
)

// Importer coordinates the offline allowlist-driven migration from legacy SQLite to CSP 1.0.
type Importer struct{}

// NewImporter constructs an Importer instance.
func NewImporter() *Importer {
	return &Importer{}
}

// Import executes the legacy database import, enforcing strict read-only access to source,
// secret exclusion, quarantine of unmapped fields, and draft configuration revision creation.
func (imp *Importer) Import(ctx context.Context, opts Options) (*ImportReport, error) {
	if strings.TrimSpace(opts.SourcePath) == "" {
		return nil, domain.NewValidationError("missing_source", "source path is required")
	}
	if !opts.DryRun && strings.TrimSpace(opts.TargetPath) == "" {
		return nil, domain.NewValidationError("missing_target", "target path is required for actual import")
	}

	info, err := os.Stat(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("cannot access legacy source database: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("source path is a directory, expected sqlite file: %s", opts.SourcePath)
	}

	// 1. Run read-only inspection to obtain source metadata and schema manifest
	inspectReport, err := inspect.InspectContext(ctx, opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect legacy database: %w", err)
	}

	// 2. Open source database with modernc sqlite in strict read-only mode (mode=ro & query_only=1)
	srcDB, err := inspect.OpenReadOnlyContext(ctx, opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source in read-only mode: %w", err)
	}
	defer srcDB.Close()

	now := domain.NowUTC()
	targetRevID := domain.MustNewUUIDv7()

	report := &ImportReport{
		DryRun:              opts.DryRun,
		SourcePath:          opts.SourcePath,
		SourceDigest:        inspectReport.SourceDigest,
		SchemaFingerprint:   inspectReport.SchemaFingerprint,
		TargetRevisionID:    targetRevID,
		TargetRevisionState: string(domain.RevisionStateDraft),
		Quarantine:          make([]QuarantineItem, 0),
		SecretExclusions:    make([]SecretExclusionItem, 0),
		Timestamp:           now,
	}

	// In-memory collections for converted target domain entities
	var importedSubs []domain.Subscription
	var importedGroups []domain.NodeGroup
	var importedEdges []domain.GroupEdge
	var importedRules []domain.PolicyRule
	var importedNodes []domain.Node

	legacyGroupIDMap := make(map[string]string)   // legacy ID string -> target UUIDv7
	legacyGroupNameMap := make(map[string]string) // legacy name -> target UUIDv7

	// --- Process Subscriptions Table ---
	if subTable, ok := inspectReport.Tables["subscriptions"]; ok {
		// Detect unallowed columns and quarantine them
		for _, col := range subTable.Columns {
			lowerCol := strings.ToLower(col.Name)
			if !subscriptionsAllowedColumns[lowerCol] {
				report.Quarantine = append(report.Quarantine, QuarantineItem{
					Table:         "subscriptions",
					RecordIDHash:  hashRecordID("subscriptions", "column:"+lowerCol),
					FieldOrEntity: col.Name,
					Category:      QuarantineCategoryLegacyBlob,
					Reason:        fmt.Sprintf("legacy field %q excluded from target schema and isolated in quarantine", col.Name),
				})
			}
		}

		rows, qErr := srcDB.QueryContext(ctx, "SELECT id, name, url, update_interval, enabled, is_primary FROM subscriptions;")
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var legacyID any
				var name, rawURL string
				var updateInterval sql.NullInt64
				var enabled, isPrimary sql.NullBool

				if scanErr := rows.Scan(&legacyID, &name, &rawURL, &updateInterval, &enabled, &isPrimary); scanErr != nil {
					continue
				}

				keyHash := hashRecordID("subscriptions", legacyID)

				sanitizedURL, hadSecrets, urlErr := SanitizeSubscriptionURL(rawURL)
				if hadSecrets {
					report.SecretExclusions = append(report.SecretExclusions, SecretExclusionItem{
						Table:        "subscriptions",
						RecordIDHash: keyHash,
						Field:        "url_credentials",
						Action:       "excluded_and_sanitized",
					})
				}

				if urlErr != nil {
					report.Quarantine = append(report.Quarantine, QuarantineItem{
						Table:         "subscriptions",
						RecordIDHash:  keyHash,
						FieldOrEntity: name,
						Category:      QuarantineCategoryValidationFailed,
						Reason:        urlErr.Error(),
					})
					continue
				}

				trimmedName := strings.TrimSpace(name)
				if trimmedName == "" {
					report.Quarantine = append(report.Quarantine, QuarantineItem{
						Table:         "subscriptions",
						RecordIDHash:  keyHash,
						FieldOrEntity: "subscriptions.name",
						Category:      QuarantineCategoryValidationFailed,
						Reason:        "subscription name is empty",
					})
					continue
				}

				interval := 86400
				if updateInterval.Valid && updateInterval.Int64 > 0 {
					interval = int(updateInterval.Int64)
				}

				isEnabled := true
				if enabled.Valid {
					isEnabled = enabled.Bool
				}

				subID := domain.MustNewUUIDv7()
				subRev := domain.MustNewUUIDv7()

				importedSubs = append(importedSubs, domain.Subscription{
					ID:                 subID,
					Name:               trimmedName,
					SourceURLSecretRef: sanitizedURL,
					Enabled:            isEnabled,
					RefreshPolicy: domain.RefreshPolicy{
						IntervalSeconds:  interval,
						UserAgentPolicy:  "",
						FetchProxyRef:    "",
						TimeoutSeconds:   30,
						MaxResponseBytes: 10485760,
					},
					Revision:  subRev,
					CreatedAt: now,
					UpdatedAt: now,
				})
			}
		}
	}

	// --- Process Node Groups Table ---
	type legacyGroupEntry struct {
		legacyIDStr      string
		targetGroupID    string
		includeGroupJSON string
	}
	var groupEntries []legacyGroupEntry

	if groupTable, ok := inspectReport.Tables["node_groups"]; ok {
		for _, col := range groupTable.Columns {
			lowerCol := strings.ToLower(col.Name)
			if !nodeGroupsAllowedColumns[lowerCol] {
				report.Quarantine = append(report.Quarantine, QuarantineItem{
					Table:         "node_groups",
					RecordIDHash:  hashRecordID("node_groups", "column:"+lowerCol),
					FieldOrEntity: col.Name,
					Category:      QuarantineCategoryLegacyBlob,
					Reason:        fmt.Sprintf("legacy node group field %q isolated in quarantine", col.Name),
				})
			}
		}

		rows, qErr := srcDB.QueryContext(ctx, "SELECT id, name, kind, group_type, sort_order, include_group_ids FROM node_groups ORDER BY sort_order ASC;")
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var legacyID any
				var name, kind, groupType string
				var sortOrder int
				var includeGroupIDs sql.NullString

				if scanErr := rows.Scan(&legacyID, &name, &kind, &groupType, &sortOrder, &includeGroupIDs); scanErr != nil {
					continue
				}

				keyHash := hashRecordID("node_groups", legacyID)
				trimmedName := strings.TrimSpace(name)
				if trimmedName == "" {
					report.Quarantine = append(report.Quarantine, QuarantineItem{
						Table:         "node_groups",
						RecordIDHash:  keyHash,
						FieldOrEntity: "node_groups.name",
						Category:      QuarantineCategoryValidationFailed,
						Reason:        "node group name is empty",
					})
					continue
				}

				gt, gtErr := NormalizeGroupType(groupType)
				if gtErr != nil {
					gt, gtErr = NormalizeGroupType(kind)
				}
				if gtErr != nil {
					report.Quarantine = append(report.Quarantine, QuarantineItem{
						Table:         "node_groups",
						RecordIDHash:  keyHash,
						FieldOrEntity: trimmedName,
						Category:      QuarantineCategoryValidationFailed,
						Reason:        fmt.Sprintf("invalid group type %q / kind %q", groupType, kind),
					})
					continue
				}

				targetGroupID := domain.MustNewUUIDv7()
				legacyIDStr := parseStringID(legacyID)
				legacyGroupIDMap[legacyIDStr] = targetGroupID
				legacyGroupNameMap[trimmedName] = targetGroupID

				importedGroups = append(importedGroups, domain.NodeGroup{
					ID:        targetGroupID,
					Name:      trimmedName,
					GroupType: gt,
					CreatedAt: now,
					UpdatedAt: now,
				})

				rawJSON := ""
				if includeGroupIDs.Valid {
					rawJSON = includeGroupIDs.String
				}
				groupEntries = append(groupEntries, legacyGroupEntry{
					legacyIDStr:      legacyIDStr,
					targetGroupID:    targetGroupID,
					includeGroupJSON: rawJSON,
				})
			}
		}
	}

	// --- Process Group Edges from include_group_ids ---
	for _, entry := range groupEntries {
		if strings.TrimSpace(entry.includeGroupJSON) == "" || entry.includeGroupJSON == "[]" {
			continue
		}
		var childIDs []any
		if jErr := json.Unmarshal([]byte(entry.includeGroupJSON), &childIDs); jErr != nil {
			report.Quarantine = append(report.Quarantine, QuarantineItem{
				Table:         "node_groups",
				RecordIDHash:  hashRecordID("node_groups", entry.legacyIDStr),
				FieldOrEntity: "include_group_ids",
				Category:      QuarantineCategoryValidationFailed,
				Reason:        fmt.Sprintf("malformed include_group_ids JSON: %v", jErr),
			})
			continue
		}

		pos := 0
		for _, rawChildID := range childIDs {
			childLegacyStr := parseStringID(rawChildID)
			childTargetID, found := legacyGroupIDMap[childLegacyStr]
			if !found {
				report.Quarantine = append(report.Quarantine, QuarantineItem{
					Table:         "node_groups",
					RecordIDHash:  hashRecordID("node_groups", entry.legacyIDStr),
					FieldOrEntity: fmt.Sprintf("child_group_ref:%s", childLegacyStr),
					Category:      QuarantineCategoryInvalidReference,
					Reason:        fmt.Sprintf("referenced child group %q not found in imported groups", childLegacyStr),
				})
				continue
			}

			if childTargetID == entry.targetGroupID {
				report.Quarantine = append(report.Quarantine, QuarantineItem{
					Table:         "group_edges",
					RecordIDHash:  hashRecordID("node_groups", entry.legacyIDStr),
					FieldOrEntity: "self_loop",
					Category:      QuarantineCategoryValidationFailed,
					Reason:        "self-referencing group edge rejected",
				})
				continue
			}

			edge := domain.GroupEdge{
				ID:            domain.MustNewUUIDv7(),
				ParentGroupID: entry.targetGroupID,
				ChildGroupID:  &childTargetID,
				Position:      pos,
			}
			if err := domain.ValidateGroupEdge(edge); err != nil {
				report.Quarantine = append(report.Quarantine, QuarantineItem{
					Table:         "group_edges",
					RecordIDHash:  hashRecordID("node_groups", entry.legacyIDStr),
					FieldOrEntity: "group_edge",
					Category:      QuarantineCategoryValidationFailed,
					Reason:        err.Error(),
				})
				continue
			}

			importedEdges = append(importedEdges, edge)
			pos++
		}
	}

	// --- Process Rules Table ---
	if ruleTable, ok := inspectReport.Tables["rules"]; ok {
		for _, col := range ruleTable.Columns {
			lowerCol := strings.ToLower(col.Name)
			if !rulesAllowedColumns[lowerCol] {
				report.Quarantine = append(report.Quarantine, QuarantineItem{
					Table:         "rules",
					RecordIDHash:  hashRecordID("rules", "column:"+lowerCol),
					FieldOrEntity: col.Name,
					Category:      QuarantineCategoryLegacyBlob,
					Reason:        fmt.Sprintf("legacy rule field %q isolated in quarantine", col.Name),
				})
			}
		}

		rulesQuery := "SELECT id, name, type, value, proxy, sort_order, enabled FROM rules ORDER BY sort_order ASC, id ASC;"
		if _, ok := inspectReport.Tables["rule_categories"]; ok {
			rulesQuery = "SELECT r.id, r.name, r.type, r.value, r.proxy, r.sort_order, r.enabled FROM rules r LEFT JOIN rule_categories rc ON r.category = rc.name ORDER BY COALESCE(rc.sort_order, 999999) ASC, r.sort_order ASC, r.id ASC;"
		}
		rows, qErr := srcDB.QueryContext(ctx, rulesQuery)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var legacyID any
				var name, ruleType, value, proxy string
				var sortOrder int
				var enabled sql.NullBool

				if scanErr := rows.Scan(&legacyID, &name, &ruleType, &value, &proxy, &sortOrder, &enabled); scanErr != nil {
					continue
				}

				keyHash := hashRecordID("rules", legacyID)

				if enabled.Valid && !enabled.Bool {
					// Disabled rule: record and omit from draft revision
					continue
				}

				trimmedProxy := strings.TrimSpace(proxy)
				targetGroupID, found := legacyGroupNameMap[trimmedProxy]
				if !found {
					targetGroupID, found = legacyGroupIDMap[trimmedProxy]
				}

				if !found {
					report.Quarantine = append(report.Quarantine, QuarantineItem{
						Table:         "rules",
						RecordIDHash:  keyHash,
						FieldOrEntity: name,
						Category:      QuarantineCategoryInvalidReference,
						Reason:        fmt.Sprintf("target proxy group %q not found in policy graph", trimmedProxy),
					})
					continue
				}

				expr := fmt.Sprintf("%s,%s", strings.TrimSpace(ruleType), strings.TrimSpace(value))
				if strings.TrimSpace(ruleType) == "" {
					expr = strings.TrimSpace(value)
				}

				importedRules = append(importedRules, domain.PolicyRule{
					ID:            domain.MustNewUUIDv7(),
					RevisionID:    targetRevID,
					TargetGroupID: targetGroupID,
					Expression:    expr,
					Position:      len(importedRules),
				})
			}
		}
	}

	// --- Process Nodes Table (if present in source) ---
	if nodeTable, ok := inspectReport.Tables["nodes"]; ok {
		hasPassCol := false
		hasUUIDCol := false
		hasPrivKeyCol := false

		for _, col := range nodeTable.Columns {
			lowerCol := strings.ToLower(col.Name)
			if lowerCol == "password" || strings.Contains(lowerCol, "pass") {
				hasPassCol = true
			}
			if lowerCol == "uuid" {
				hasUUIDCol = true
			}
			if lowerCol == "private_key" || strings.Contains(lowerCol, "key") {
				hasPrivKeyCol = true
			}
		}

		query := "SELECT id, name, protocol, server, port"
		if hasPassCol {
			query += ", password"
		} else {
			query += ", ''"
		}
		if hasUUIDCol {
			query += ", uuid"
		} else {
			query += ", ''"
		}
		if hasPrivKeyCol {
			query += ", private_key"
		} else {
			query += ", ''"
		}
		query += " FROM nodes ORDER BY id ASC;"

		seenNodeLogicalIDs := make(map[string]struct{})
		rows, qErr := srcDB.QueryContext(ctx, query)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var legacyID any
				var name, protocolStr, server string
				var port int
				var pass, uuidVal, privKey sql.NullString

				if scanErr := rows.Scan(&legacyID, &name, &protocolStr, &server, &port, &pass, &uuidVal, &privKey); scanErr != nil {
					continue
				}

				keyHash := hashRecordID("nodes", legacyID)

				if (pass.Valid && pass.String != "") || (uuidVal.Valid && uuidVal.String != "") || (privKey.Valid && privKey.String != "") {
					report.SecretExclusions = append(report.SecretExclusions, SecretExclusionItem{
						Table:        "nodes",
						RecordIDHash: keyHash,
						Field:        "credentials",
						Action:       "excluded_and_replaced_with_opaque_ref",
					})
				}

				proto, protoErr := domain.ParseProtocol(protocolStr)
				if protoErr != nil {
					report.Quarantine = append(report.Quarantine, QuarantineItem{
						Table:         "nodes",
						RecordIDHash:  keyHash,
						FieldOrEntity: name,
						Category:      QuarantineCategoryValidationFailed,
						Reason:        protoErr.Error(),
					})
					continue
				}

				trimmedServer := strings.ToLower(strings.TrimSpace(server))
				if trimmedServer == "" || port <= 0 || port > 65535 {
					report.Quarantine = append(report.Quarantine, QuarantineItem{
						Table:         "nodes",
						RecordIDHash:  keyHash,
						FieldOrEntity: name,
						Category:      QuarantineCategoryValidationFailed,
						Reason:        fmt.Sprintf("invalid endpoint server=%q port=%d", server, port),
					})
					continue
				}

				logicalID := domain.ComputeNodeLogicalID(proto, trimmedServer, port, nil)
				if _, exists := seenNodeLogicalIDs[logicalID]; exists {
					continue
				}
				seenNodeLogicalIDs[logicalID] = struct{}{}

				secretRef := ComputeOpaqueSecretRef(proto, trimmedServer, port)

				displayName := strings.TrimSpace(name)
				if displayName == "" {
					displayName = fmt.Sprintf("%s:%d", trimmedServer, port)
				}

				importedNodes = append(importedNodes, domain.Node{
					LogicalID:                 logicalID,
					Protocol:                  proto,
					DisplayName:               displayName,
					NormalizedConfigSecretRef: secretRef,
					Active:                    true,
					CreatedAt:                 now,
					UpdatedAt:                 now,
				})
			}
		}
	}

	// --- Process Security Settings & Other Tables ---
	if secTable, ok := inspectReport.Tables["security_settings"]; ok {
		for _, col := range secTable.Columns {
			if strings.Contains(strings.ToLower(col.Name), "token") {
				report.SecretExclusions = append(report.SecretExclusions, SecretExclusionItem{
					Table:        "security_settings",
					RecordIDHash: hashRecordID("security_settings", "column:"+col.Name),
					Field:        col.Name,
					Action:       "excluded_security_token",
				})
			}
		}
	}

	for tblName := range inspectReport.Tables {
		if tblName == "config_snapshots" || tblName == "generate_config" ||
			tblName == "node_probe_results" || tblName == "proxy_chain_bindings" ||
			tblName == "probe_config" || tblName == "rule_categories" || tblName == "dns_config" {
			report.Quarantine = append(report.Quarantine, QuarantineItem{
				Table:         tblName,
				RecordIDHash:  hashRecordID(tblName, "table"),
				FieldOrEntity: tblName,
				Category:      QuarantineCategoryUnmappedTable,
				Reason:        fmt.Sprintf("legacy runtime table %q omitted from clean-slate target schema", tblName),
			})
		}
	}

	// --- Compute ContentDigest for Draft Revision ---
	digest := computeImportContentDigest(importedGroups, importedEdges, importedRules, importedSubs)

	// Update counts
	report.Counts.SubscriptionsImported = len(importedSubs)
	report.Counts.NodeGroupsImported = len(importedGroups)
	report.Counts.GroupEdgesImported = len(importedEdges)
	report.Counts.PolicyRulesImported = len(importedRules)
	report.Counts.NodesImported = len(importedNodes)
	report.Counts.QuarantineCount = len(report.Quarantine)
	report.Counts.SecretExclusionCount = len(report.SecretExclusions)

	// If dry-run, we return without executing any target database writes
	if opts.DryRun {
		return report, nil
	}

	// --- 3. Actual Import: Short Transaction Safe Write ---
	targetCfg := sqlite.Config{
		Path:         opts.TargetPath,
		ForeignKeys:  true,
		WALMode:      false,
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 1,
	}

	targetDB, err := sqlite.Open(targetCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open target database: %w", err)
	}
	defer targetDB.Close()

	// If target database is completely empty (no schema_migrations), apply embedded migrations
	var schemaVerCount int
	err = targetDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations';").Scan(&schemaVerCount)
	if err == nil && schemaVerCount == 0 {
		runner := sqlite.NewMigrationRunner(targetDB, migrations.FS)
		if mErr := runner.Run(ctx); mErr != nil {
			return nil, fmt.Errorf("failed to initialize target schema migrations: %w", mErr)
		}
	}

	// Execute batch write in a single atomic transaction
	txErr := sqlite.WithTx(ctx, targetDB, func(ctx context.Context, tx *sql.Tx) error {
		// 1. Insert configuration revision in DRAFT state
		const insertRevQuery = `
			INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
			VALUES (?, NULL, ?, ?, ?);
		`
		if _, err := tx.ExecContext(ctx, insertRevQuery, targetRevID, digest, string(domain.RevisionStateDraft), now.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("failed to insert draft configuration revision: %w", err)
		}

		// 2. Insert Subscriptions
		const insertSubQuery = `
			INSERT INTO subscriptions (
				id, name, source_url_secret_ref, enabled,
				refresh_interval_seconds, refresh_user_agent_policy, refresh_fetch_proxy_ref,
				refresh_timeout_seconds, refresh_max_response_bytes,
				revision, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
		`
		subStmt, err := tx.PrepareContext(ctx, insertSubQuery)
		if err != nil {
			return fmt.Errorf("failed to prepare insert subscription: %w", err)
		}
		defer subStmt.Close()

		for _, sub := range importedSubs {
			enabledInt := 0
			if sub.Enabled {
				enabledInt = 1
			}
			if _, err := subStmt.ExecContext(ctx,
				sub.ID, sub.Name, sub.SourceURLSecretRef, enabledInt,
				sub.RefreshPolicy.IntervalSeconds, sub.RefreshPolicy.UserAgentPolicy, sub.RefreshPolicy.FetchProxyRef,
				sub.RefreshPolicy.TimeoutSeconds, sub.RefreshPolicy.MaxResponseBytes,
				sub.Revision, sub.CreatedAt.Format(time.RFC3339), sub.UpdatedAt.Format(time.RFC3339),
			); err != nil {
				return fmt.Errorf("failed to insert subscription %s: %w", sub.Name, err)
			}
		}

		// 3. Insert Node Groups
		const insertGroupQuery = `
			INSERT INTO node_groups (id, name, group_type, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?);
		`
		groupStmt, err := tx.PrepareContext(ctx, insertGroupQuery)
		if err != nil {
			return fmt.Errorf("failed to prepare insert node group: %w", err)
		}
		defer groupStmt.Close()

		for _, g := range importedGroups {
			if _, err := groupStmt.ExecContext(ctx, g.ID, g.Name, string(g.GroupType), g.CreatedAt.Format(time.RFC3339), g.UpdatedAt.Format(time.RFC3339)); err != nil {
				return fmt.Errorf("failed to insert node group %s: %w", g.Name, err)
			}
		}

		// 4. Insert Group Edges
		const insertEdgeQuery = `
			INSERT INTO group_edges (id, parent_group_id, child_group_id, node_logical_id, position)
			VALUES (?, ?, ?, ?, ?);
		`
		edgeStmt, err := tx.PrepareContext(ctx, insertEdgeQuery)
		if err != nil {
			return fmt.Errorf("failed to prepare insert group edge: %w", err)
		}
		defer edgeStmt.Close()

		for _, edge := range importedEdges {
			var childStr, nodeStr sql.NullString
			if edge.ChildGroupID != nil && *edge.ChildGroupID != "" {
				childStr = sql.NullString{String: *edge.ChildGroupID, Valid: true}
			}
			if edge.NodeLogicalID != nil && *edge.NodeLogicalID != "" {
				nodeStr = sql.NullString{String: *edge.NodeLogicalID, Valid: true}
			}
			if _, err := edgeStmt.ExecContext(ctx, edge.ID, edge.ParentGroupID, childStr, nodeStr, edge.Position); err != nil {
				return fmt.Errorf("failed to insert group edge %s: %w", edge.ID, err)
			}
		}

		// 5. Insert Policy Rules
		const insertRuleQuery = `
			INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
			VALUES (?, ?, ?, ?, ?);
		`
		ruleStmt, err := tx.PrepareContext(ctx, insertRuleQuery)
		if err != nil {
			return fmt.Errorf("failed to prepare insert policy rule: %w", err)
		}
		defer ruleStmt.Close()

		for _, rule := range importedRules {
			if _, err := ruleStmt.ExecContext(ctx, rule.ID, targetRevID, rule.TargetGroupID, rule.Expression, rule.Position); err != nil {
				return fmt.Errorf("failed to insert policy rule %s: %w", rule.ID, err)
			}
		}

		// 6. Insert Nodes (if any)
		if len(importedNodes) > 0 {
			const insertNodeQuery = `
				INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(logical_id) DO UPDATE SET
					protocol = excluded.protocol,
					display_name = excluded.display_name,
					normalized_config_secret_ref = excluded.normalized_config_secret_ref,
					active = excluded.active,
					updated_at = excluded.updated_at;
			`
			nodeStmt, err := tx.PrepareContext(ctx, insertNodeQuery)
			if err != nil {
				return fmt.Errorf("failed to prepare insert node: %w", err)
			}
			defer nodeStmt.Close()

			for _, n := range importedNodes {
				activeInt := 0
				if n.Active {
					activeInt = 1
				}
				if _, err := nodeStmt.ExecContext(ctx, n.LogicalID, string(n.Protocol), n.DisplayName, n.NormalizedConfigSecretRef, activeInt, n.CreatedAt.Format(time.RFC3339), n.UpdatedAt.Format(time.RFC3339)); err != nil {
					return fmt.Errorf("failed to insert node %s: %w", n.LogicalID, err)
				}
			}
		}

		// 7. Insert Audit Event
		const insertAuditQuery = `
			INSERT INTO audit_events (id, actor_kind, request_id, action, result, redacted_summary, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?);
		`
		summary := fmt.Sprintf("legacy import: %d subscriptions, %d groups, %d edges, %d rules, %d nodes into draft revision %s; %d quarantined, %d secrets excluded",
			report.Counts.SubscriptionsImported,
			report.Counts.NodeGroupsImported,
			report.Counts.GroupEdgesImported,
			report.Counts.PolicyRulesImported,
			report.Counts.NodesImported,
			targetRevID,
			report.Counts.QuarantineCount,
			report.Counts.SecretExclusionCount,
		)
		auditID := domain.MustNewUUIDv7()
		if _, err := tx.ExecContext(ctx, insertAuditQuery, auditID, string(domain.ActorKindAdmin), "", "legacy.import", string(domain.AuditResultSuccess), summary, now.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("failed to record import audit event: %w", err)
		}

		return nil
	})

	if txErr != nil {
		return nil, fmt.Errorf("failed to commit imported batch to target database: %w", txErr)
	}

	return report, nil
}

// computeImportContentDigest creates a deterministic hash for the draft configuration revision.
func computeImportContentDigest(groups []domain.NodeGroup, edges []domain.GroupEdge, rules []domain.PolicyRule, subs []domain.Subscription) string {
	h := sha256.New()

	sortedGroups := make([]domain.NodeGroup, len(groups))
	copy(sortedGroups, groups)
	sort.Slice(sortedGroups, func(i, j int) bool {
		return sortedGroups[i].Name < sortedGroups[j].Name
	})

	for _, g := range sortedGroups {
		h.Write([]byte(fmt.Sprintf("group:%s:%s\n", g.Name, g.GroupType)))
	}

	sortedEdges := make([]domain.GroupEdge, len(edges))
	copy(sortedEdges, edges)
	sort.Slice(sortedEdges, func(i, j int) bool {
		if sortedEdges[i].ParentGroupID != sortedEdges[j].ParentGroupID {
			return sortedEdges[i].ParentGroupID < sortedEdges[j].ParentGroupID
		}
		return sortedEdges[i].Position < sortedEdges[j].Position
	})

	for _, e := range sortedEdges {
		child := ""
		if e.ChildGroupID != nil {
			child = *e.ChildGroupID
		}
		node := ""
		if e.NodeLogicalID != nil {
			node = *e.NodeLogicalID
		}
		h.Write([]byte(fmt.Sprintf("edge:%s:%s:%s:%d\n", e.ParentGroupID, child, node, e.Position)))
	}

	sortedRules := make([]domain.PolicyRule, len(rules))
	copy(sortedRules, rules)
	sort.Slice(sortedRules, func(i, j int) bool {
		return sortedRules[i].Position < sortedRules[j].Position
	})

	for _, r := range sortedRules {
		h.Write([]byte(fmt.Sprintf("rule:%s:%s:%d\n", r.TargetGroupID, r.Expression, r.Position)))
	}

	sortedSubs := make([]domain.Subscription, len(subs))
	copy(sortedSubs, subs)
	sort.Slice(sortedSubs, func(i, j int) bool {
		return sortedSubs[i].Name < sortedSubs[j].Name
	})

	for _, s := range sortedSubs {
		h.Write([]byte(fmt.Sprintf("sub:%s:%s:%d\n", s.Name, s.SourceURLSecretRef, s.RefreshPolicy.IntervalSeconds)))
	}

	return hex.EncodeToString(h.Sum(nil))
}
