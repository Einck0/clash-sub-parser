package inventory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// Canonical legacy group ID mapping to stable target UUIDv7 identifiers.
var CanonicalLegacyGroupMap = map[int]string{
	1:  "01a0b9af-c116-783d-903e-6e0c59f066ae", // 低延迟 (select)
	2:  "01a0b9af-c116-7479-bcf8-338e825502ad", // 便宜 (select)
	3:  "01a0b9af-c116-7803-89e1-593bb58b23e8", // 美国 (urltest)
	4:  "01a0b9af-c116-7c5f-8a2e-c5363cc72b4d", // 香港 (urltest)
	5:  "01a0b9af-c116-7997-bf6b-b96edd2310a3", // 台湾 (urltest)
	6:  "01a0b9af-c116-7cca-8ee5-47d928361ec1", // 日本 (urltest)
	7:  "01a0b9af-c116-70d1-a027-dd08a0e3b9b1", // 新加坡 (urltest)
	8:  "01a0b9af-c116-727f-9e19-6cf6fe03facc", // 加拿大 (urltest)
	9:  "01a0b9af-c116-71ba-a125-31cefefe0d4b", // 其他 (select)
	10: "01a0b9af-c116-75ee-9e58-b4a163f17cbc", // 自动选择 (urltest)
	11: "01a0b9af-c116-79e1-9ce2-f6eb09df7b63", // 轮询(日本) (loadbalance)
	12: "01a0b9af-c116-770f-aeb9-68db80511fc7", // 轮询(新加坡) (loadbalance)
	13: "01a0b9af-c116-7bda-ab27-c6e4685f8de5", // 轮询(包含香港) (loadbalance)
	14: "01a0b9af-c116-7806-9740-66b8b070ba2f", // 选择节点 (select)
	15: "01a0b9af-c116-7a44-86bf-56dd772fcddf", // 自动切换 (fallback)
	16: "01a0b9af-c116-7851-a60b-432ede360adc", // 规则内代理模式 (select)
	17: "01a0b9af-c116-754b-accc-33cb197e4c4d", // 规则外代理模式 (select)
	18: "01a0b9af-c116-71af-a002-c12b733fa213", // AI (select)
	19: "01a0b9af-c116-7e92-a337-de880a661e2b", // 负载均衡 (select)
	20: "01a0b9af-c116-7b03-8d12-1186d45dad6d", // 下载节点 (select)
	21: "01a0b9af-c116-7b36-8b0d-1c29ed231d6a", // 流媒体节点 (select)
	22: "01a0b9af-c116-7d41-b551-cd3900304868", // 流媒体模式 (fallback)
	23: "01a0b9af-c116-7552-9177-34d11183117d", // 下载模式 (select)
	24: "01a0b9af-c116-70e5-a413-e1f5b04cb9b3", // 游戏 (select)
	25: "01a0b9af-c116-761a-a44d-ed61363f92e5", // 链式 (urltest)
	26: "01a0b9af-c116-771f-b1c6-7f453651c5ea", // 链式代理规则 (select)
	27: "01a0b9af-c116-7939-8ed8-afbf151e382d", // 无限制区域 (urltest)
	28: "01a0b9af-c116-77d8-91cc-5a03e86e620b", // WARP (urltest)
	29: "01a0b9af-c116-7f9a-a7c1-f9b5592da949", // other (select)
}

// RestoreGroupFiltersOptions provides parameter controls for filter recovery.
type RestoreGroupFiltersOptions struct {
	TargetDBPath  string
	ArchiveDBPath string
	DryRun        bool
	ConfirmBackup bool
	BackupFile    string
}

// RecoveredGroupFilterDetails provides structured audit view of each group's recovery status.
type RecoveredGroupFilterDetails struct {
	LegacyID         int                   `json:"legacy_id"`
	TargetGroupID    string                `json:"target_group_id"`
	TargetGroupName  string                `json:"target_group_name"`
	GroupType        string                `json:"group_type"`
	Category         string                `json:"semantic_category"` // "positive_regex" | "negation_regex" | "manual_edges"
	RawRegexRules    []string              `json:"raw_regex_rules"`
	FilterSpec       domain.NodeFilterSpec `json:"filter_spec"`
	Status           string                `json:"status"` // "recovered" | "skipped_existing" | "manual_no_filter" | "guard_mismatch"
	Reason           string                `json:"reason,omitempty"`
	MatchedNodeCount int                   `json:"matched_node_count"`
}

// RestoreGroupFiltersReport provides comprehensive machine-readable report of the restoration.
type RestoreGroupFiltersReport struct {
	ArchiveDBPath     string                        `json:"archive_db_path"`
	TargetDBPath      string                        `json:"target_db_path"`
	DryRun            bool                          `json:"dry_run"`
	TotalGroups       int                           `json:"total_groups"`
	RecoveredCount    int                           `json:"recovered_count"`
	SkippedCount      int                           `json:"skipped_count"`
	ManualCount       int                           `json:"manual_count"`
	UnsupportedCount  int                           `json:"unsupported_count"`
	GuardErrors       int                           `json:"guard_errors"`
	InitialRevisionID string                        `json:"initial_revision_id"`
	NewRevisionID     string                        `json:"new_revision_id,omitempty"`
	Groups            []RecoveredGroupFilterDetails `json:"groups"`
	ExecutedAt        time.Time                     `json:"executed_at"`
}

// RunRestoreGroupFilters parses legacy cold archive node_groups, translates original filter semantics
// into CSP 1.0 group_node_filters specs, and restores them safely into the target SQLite database.
func RunRestoreGroupFilters(ctx context.Context, targetDB *sql.DB, opts RestoreGroupFiltersOptions) (*RestoreGroupFiltersReport, error) {
	if strings.TrimSpace(opts.ArchiveDBPath) == "" {
		opts.ArchiveDBPath = "/home/service/backups/csp-legacy-cold-archive-20260919.db"
	}

	// 1. Validate cold archive existence and readability
	if fi, err := os.Stat(opts.ArchiveDBPath); err != nil {
		return nil, fmt.Errorf("archive db %q inaccessible: %w", opts.ArchiveDBPath, err)
	} else if fi.IsDir() {
		return nil, fmt.Errorf("archive db %q is a directory", opts.ArchiveDBPath)
	}

	archiveDB, err := sqlite.OpenReadOnly(opts.ArchiveDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive db readonly: %w", err)
	}
	defer archiveDB.Close()

	// 2. Read legacy node_groups from archive
	type legacyGroupRow struct {
		ID         int
		Name       string
		Kind       string
		GroupType  string
		RegexRules string
	}
	legacyRows, err := archiveDB.QueryContext(ctx, "SELECT id, name, kind, group_type, regex_rules FROM node_groups ORDER BY id;")
	if err != nil {
		return nil, fmt.Errorf("failed to read legacy node_groups from archive: %w", err)
	}
	defer legacyRows.Close()

	var legacyGroups []legacyGroupRow
	for legacyRows.Next() {
		var g legacyGroupRow
		if err := legacyRows.Scan(&g.ID, &g.Name, &g.Kind, &g.GroupType, &g.RegexRules); err != nil {
			return nil, fmt.Errorf("scan legacy group row: %w", err)
		}
		legacyGroups = append(legacyGroups, g)
	}
	if err := legacyRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy groups: %w", err)
	}

	// 3. Inspect target database current state
	type targetGroupRow struct {
		ID        string
		Name      string
		GroupType string
	}
	tRows, err := targetDB.QueryContext(ctx, "SELECT id, name, group_type FROM node_groups;")
	if err != nil {
		return nil, fmt.Errorf("failed to query target node_groups: %w", err)
	}
	defer tRows.Close()

	targetGroupsByID := make(map[string]targetGroupRow)
	for tRows.Next() {
		var tg targetGroupRow
		if err := tRows.Scan(&tg.ID, &tg.Name, &tg.GroupType); err != nil {
			return nil, fmt.Errorf("scan target group row: %w", err)
		}
		targetGroupsByID[tg.ID] = tg
	}

	// Read existing group_node_filters
	existingFilters := make(map[string]string)
	fRows, err := targetDB.QueryContext(ctx, "SELECT group_id, filter_spec FROM group_node_filters;")
	if err == nil {
		defer fRows.Close()
		for fRows.Next() {
			var gid, spec string
			if err := fRows.Scan(&gid, &spec); err == nil {
				existingFilters[gid] = spec
			}
		}
	}

	// Read current active nodes for simulation
	type targetNodeRow struct {
		LogicalID   string
		DisplayName string
		Protocol    domain.Protocol
	}
	var targetNodes []targetNodeRow
	nRows, err := targetDB.QueryContext(ctx, "SELECT logical_id, display_name, protocol FROM nodes WHERE active = 1;")
	if err == nil {
		defer nRows.Close()
		for nRows.Next() {
			var n targetNodeRow
			if err := nRows.Scan(&n.LogicalID, &n.DisplayName, &n.Protocol); err == nil {
				targetNodes = append(targetNodes, n)
			}
		}
	}

	// Get active revision
	currentRevID := ""
	_ = targetDB.QueryRowContext(ctx, "SELECT id FROM configuration_revisions WHERE state = 'active' ORDER BY created_at DESC LIMIT 1;").Scan(&currentRevID)

	report := &RestoreGroupFiltersReport{
		ArchiveDBPath:     opts.ArchiveDBPath,
		TargetDBPath:      opts.TargetDBPath,
		DryRun:            opts.DryRun,
		TotalGroups:       len(legacyGroups),
		InitialRevisionID: currentRevID,
		ExecutedAt:        domain.NowUTC(),
		Groups:            make([]RecoveredGroupFilterDetails, 0, len(legacyGroups)),
	}

	type pendingRestore struct {
		GroupID    string
		FilterSpec domain.NodeFilterSpec
	}
	var pendingList []pendingRestore

	for _, lg := range legacyGroups {
		targetUUID, ok := CanonicalLegacyGroupMap[lg.ID]
		if !ok {
			report.GuardErrors++
			report.Groups = append(report.Groups, RecoveredGroupFilterDetails{
				LegacyID: lg.ID,
				Status:   "guard_mismatch",
				Reason:   fmt.Sprintf("no canonical UUID mapping defined for legacy group ID %d", lg.ID),
			})
			continue
		}

		targetGroup, exists := targetGroupsByID[targetUUID]
		if !exists {
			report.GuardErrors++
			report.Groups = append(report.Groups, RecoveredGroupFilterDetails{
				LegacyID:      lg.ID,
				TargetGroupID: targetUUID,
				Status:        "guard_mismatch",
				Reason:        fmt.Sprintf("target group %s not found in target database", targetUUID),
			})
			continue
		}

		// Verify matching attributes
		if targetGroup.Name != lg.Name {
			report.GuardErrors++
			report.Groups = append(report.Groups, RecoveredGroupFilterDetails{
				LegacyID:        lg.ID,
				TargetGroupID:   targetUUID,
				TargetGroupName: targetGroup.Name,
				Status:          "guard_mismatch",
				Reason:          fmt.Sprintf("name mismatch: target has %q, archive has %q", targetGroup.Name, lg.Name),
			})
			continue
		}

		// Parse raw regex rules
		var rawRegexes []string
		if lg.RegexRules != "" && lg.RegexRules != "[]" {
			_ = json.Unmarshal([]byte(lg.RegexRules), &rawRegexes)
		}

		// Check if already has customized filter
		existingSpecJSON := existingFilters[targetUUID]
		if existingSpecJSON != "" && existingSpecJSON != `{"conditions":[]}` {
			var parsedSpec domain.NodeFilterSpec
			if err := json.Unmarshal([]byte(existingSpecJSON), &parsedSpec); err == nil && !parsedSpec.IsEmpty() {
				report.SkippedCount++
				report.Groups = append(report.Groups, RecoveredGroupFilterDetails{
					LegacyID:        lg.ID,
					TargetGroupID:   targetUUID,
					TargetGroupName: targetGroup.Name,
					GroupType:       targetGroup.GroupType,
					RawRegexRules:   rawRegexes,
					FilterSpec:      parsedSpec,
					Status:          "skipped_existing",
					Reason:          "target group already has non-empty filter configuration (untouched)",
				})
				continue
			}
		}

		// If no regex rules, manual aggregation group
		if len(rawRegexes) == 0 {
			report.ManualCount++
			report.Groups = append(report.Groups, RecoveredGroupFilterDetails{
				LegacyID:        lg.ID,
				TargetGroupID:   targetUUID,
				TargetGroupName: targetGroup.Name,
				GroupType:       targetGroup.GroupType,
				Category:        "manual_edges",
				Status:          "manual_no_filter",
				Reason:          "group uses explicit child edges without node filter predicate",
			})
			continue
		}

		// Translate regex rules into NodeFilterSpec
		category, spec, err := translateLegacyRegexRules(rawRegexes)
		if err != nil {
			report.UnsupportedCount++
			report.Groups = append(report.Groups, RecoveredGroupFilterDetails{
				LegacyID:        lg.ID,
				TargetGroupID:   targetUUID,
				TargetGroupName: targetGroup.Name,
				GroupType:       targetGroup.GroupType,
				RawRegexRules:   rawRegexes,
				Status:          "unsupported_regex",
				Reason:          fmt.Sprintf("unsupported regex syntax: %v", err),
			})
			continue
		}

		// Simulate node match count
		matchedCount := 0
		for _, node := range targetNodes {
			matched, _ := domain.MatchesFilter(&spec, domain.Node{
				LogicalID:   node.LogicalID,
				DisplayName: node.DisplayName,
				Protocol:    node.Protocol,
			}, nil, nil, time.Time{})
			if matched {
				matchedCount++
			}
		}

		report.RecoveredCount++
		detail := RecoveredGroupFilterDetails{
			LegacyID:         lg.ID,
			TargetGroupID:    targetUUID,
			TargetGroupName:  targetGroup.Name,
			GroupType:        targetGroup.GroupType,
			Category:         category,
			RawRegexRules:    rawRegexes,
			FilterSpec:       spec,
			Status:           "recovered",
			MatchedNodeCount: matchedCount,
		}
		report.Groups = append(report.Groups, detail)
		pendingList = append(pendingList, pendingRestore{
			GroupID:    targetUUID,
			FilterSpec: spec,
		})
	}

	if opts.DryRun {
		return report, nil
	}

	// Apply mode: transaction
	tx, err := targetDB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	now := domain.NowUTC()
	nowStr := now.Format(time.RFC3339)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO group_node_filters (group_id, filter_spec, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(group_id) DO UPDATE SET
			filter_spec = excluded.filter_spec,
			updated_at = excluded.updated_at;
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare upsert statement: %w", err)
	}
	defer stmt.Close()

	for _, item := range pendingList {
		specJSON, mErr := json.Marshal(item.FilterSpec)
		if mErr != nil {
			return nil, fmt.Errorf("marshal spec for group %s: %w", item.GroupID, mErr)
		}
		if _, err := stmt.ExecContext(ctx, item.GroupID, string(specJSON), nowStr); err != nil {
			return nil, fmt.Errorf("upsert filter for group %s: %w", item.GroupID, err)
		}
	}

	// Create new configuration revision preserving immutable predecessor
	if len(pendingList) > 0 {
		newRevID, uErr := domain.NewUUIDv7()
		if uErr == nil {
			h := sha256.New()
			h.Write([]byte(newRevID + ":" + currentRevID + ":" + nowStr))
			snapDigest := "sha256:" + hex.EncodeToString(h.Sum(nil))

			// Archive previous active revision
			_, _ = tx.ExecContext(ctx, "UPDATE configuration_revisions SET state = 'archived' WHERE state = 'active';")
			_, revErr := tx.ExecContext(ctx, `
				INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
				VALUES (?, ?, ?, 'active', ?);
			`, newRevID, currentRevID, snapDigest, nowStr)
			if revErr != nil {
				return nil, fmt.Errorf("insert configuration revision: %w", revErr)
			}
			report.NewRevisionID = newRevID
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit filter restoration: %w", err)
	}

	return report, nil
}

// TranslateLegacyRegexRulesForTest exports translation logic for unit tests.
func TranslateLegacyRegexRulesForTest(rawRegexes []string) (string, domain.NodeFilterSpec, error) {
	return translateLegacyRegexRules(rawRegexes)
}

func translateLegacyRegexRules(rawRegexes []string) (string, domain.NodeFilterSpec, error) {
	if len(rawRegexes) == 0 {
		return "manual_edges", domain.NodeFilterSpec{}, nil
	}

	// Check if this is a negative lookahead pattern: e.g. ^(?!.*(WORD1|WORD2|...)).*$
	negRegex := regexp.MustCompile(`^\^\(\?!.*\((.+?)\)\)\.\*\$$`)
	if len(rawRegexes) == 1 {
		if matches := negRegex.FindStringSubmatch(rawRegexes[0]); len(matches) > 1 {
			excludedWords := matches[1]
			spec := domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{
						Field: domain.FilterFieldDisplayName,
						Op:    domain.FilterOpNotRegex,
						Value: excludedWords,
					},
				},
			}
			if err := spec.Validate(); err != nil {
				return "", domain.NodeFilterSpec{}, fmt.Errorf("invalid negation spec: %w", err)
			}
			return "negation_regex", spec, nil
		}
	}

	// Positive regex rules
	var combinedPattern string
	if len(rawRegexes) == 1 {
		combinedPattern = rawRegexes[0]
	} else {
		parts := make([]string, len(rawRegexes))
		for i, r := range rawRegexes {
			parts[i] = "(?:" + r + ")"
		}
		combinedPattern = strings.Join(parts, "|")
	}

	spec := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpRegex,
				Value: combinedPattern,
			},
		},
	}
	if err := spec.Validate(); err != nil {
		return "", domain.NodeFilterSpec{}, fmt.Errorf("invalid positive regex spec: %w", err)
	}

	return "positive_regex", spec, nil
}
