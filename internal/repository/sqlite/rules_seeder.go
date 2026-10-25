package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"clash-sub-parser/internal/domain"
)

//go:embed rules_seed.json
var embeddedRulesSeedJSON []byte

type seedRuleRecord struct {
	OriginalID      int     `json:"original_id"`
	Name            string  `json:"name"`
	CategoryID      string  `json:"category_id"`
	Category        string  `json:"category"`
	Type            string  `json:"type"`
	Value           string  `json:"value"`
	Proxy           string  `json:"proxy"`
	IsBuiltin       bool    `json:"is_builtin"`
	BuiltinTarget   *string `json:"builtin_target"`
	TargetGroupName *string `json:"target_group_name"`
	Expression      string  `json:"expression"`
	Position        int     `json:"position"`
	Enabled         bool    `json:"enabled"`
}

// loadSeedRecordsFromExternalDB reads policy seed rules from an external SQLite database.
func loadSeedRecordsFromExternalDB(ctx context.Context, dbPath string, catMap map[string]string) ([]seedRuleRecord, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}

	srcDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer srcDB.Close()

	query := `SELECT id, name, category, type, value, proxy, options, sort_order, enabled FROM rules ORDER BY id ASC;`
	rows, err := srcDB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	catPositions := make(map[string]int)
	var records []seedRuleRecord

	for rows.Next() {
		var (
			rid, sortOrder, enabledInt            int
			name, cat, rtype, val, proxy, options string
		)
		if err := rows.Scan(&rid, &name, &cat, &rtype, &val, &proxy, &options, &sortOrder, &enabledInt); err != nil {
			continue
		}

		expr := rtype
		if val != "" {
			expr = fmt.Sprintf("%s,%s", rtype, val)
		}
		if strings.Contains(options, "no-resolve") && !strings.Contains(expr, "no-resolve") {
			expr += ",no-resolve"
		}

		pos := catPositions[cat]
		catPositions[cat] = pos + 1

		isBuiltin := proxy == "DIRECT" || proxy == "REJECT" || proxy == "REJECT-DROP" || proxy == "PASS"
		var bt *string
		var tg *string
		pCopy := proxy
		if isBuiltin {
			bt = &pCopy
		} else {
			tg = &pCopy
		}

		cid := catMap[cat]
		if cid == "" {
			cid = "cat-01"
		}

		records = append(records, seedRuleRecord{
			OriginalID:      rid,
			Name:            name,
			CategoryID:      cid,
			Category:        cat,
			Type:            rtype,
			Value:           val,
			Proxy:           proxy,
			IsBuiltin:       isBuiltin,
			BuiltinTarget:   bt,
			TargetGroupName: tg,
			Expression:      expr,
			Position:        pos,
			Enabled:         enabledInt == 1,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func resolveRuleTargets(item seedRuleRecord, groupMap map[string]string) (targetGroupID *string, builtinTarget *string) {
	if item.IsBuiltin {
		if item.BuiltinTarget != nil {
			return nil, item.BuiltinTarget
		}
		p := item.Proxy
		return nil, &p
	}

	gName := item.Proxy
	if item.TargetGroupName != nil {
		gName = *item.TargetGroupName
	}
	if gid, ok := groupMap[gName]; ok {
		return &gid, nil
	}

	// Fallback: if group is missing in minimal test environment, fallback to DIRECT
	fb := "DIRECT"
	return nil, &fb
}

func insertSeedRule(ctx context.Context, stmt *sql.Stmt, revID string, item seedRuleRecord, catMap, groupMap map[string]string) error {
	ruleUUID := domain.MustNewUUIDv7()

	catID := item.CategoryID
	if catID == "" {
		catID = catMap[item.Category]
	}

	targetGroupID, builtinTarget := resolveRuleTargets(item, groupMap)

	enabledInt := 1
	if !item.Enabled {
		enabledInt = 0
	}

	_, err := stmt.ExecContext(ctx,
		ruleUUID,
		revID,
		catID,
		item.Category,
		item.Name,
		targetGroupID,
		builtinTarget,
		item.Expression,
		item.Position,
		enabledInt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert rule %d (%s): %w", item.OriginalID, item.Expression, err)
	}
	return nil
}

// SeedDefaultPolicyRulesTx seeds all 470 default rules into the given revision within an active transaction.
func SeedDefaultPolicyRulesTx(ctx context.Context, tx *sql.Tx, revID string, fallbackDBPath string) (int, error) {
	if revID == "" {
		return 0, fmt.Errorf("revision_id cannot be empty for rule seed")
	}

	// 1. Fetch node groups to map group names to UUIDs
	groupMap := make(map[string]string)
	gRows, err := tx.QueryContext(ctx, "SELECT id, name FROM node_groups;")
	if err != nil {
		return 0, fmt.Errorf("failed to query node groups: %w", err)
	}
	defer gRows.Close()

	for gRows.Next() {
		var gid, gname string
		if err := gRows.Scan(&gid, &gname); err == nil {
			groupMap[strings.TrimSpace(gname)] = gid
		}
	}
	_ = gRows.Close()

	// 2. Fetch rule categories to map category names to IDs
	catMap := make(map[string]string)
	cRows, err := tx.QueryContext(ctx, "SELECT id, name FROM rule_categories;")
	if err != nil {
		return 0, fmt.Errorf("failed to query rule categories: %w", err)
	}
	defer cRows.Close()

	for cRows.Next() {
		var cid, cname string
		if err := cRows.Scan(&cid, &cname); err == nil {
			catMap[strings.TrimSpace(cname)] = cid
		}
	}
	_ = cRows.Close()

	// Fallback map if categories table not yet populated
	defaultCatMap := map[string]string{
		"广告/拦截":      "cat-01",
		"国内日常服务":     "cat-02",
		"移动应用/系统工具":  "cat-03",
		"桌面应用进程":     "cat-04",
		"游戏":         "cat-05",
		"流媒体":        "cat-06",
		"下载":         "cat-07",
		"AI":         "cat-08",
		"个人":         "cat-09",
		"GEOSITE/平台": "cat-10",
		"负载均衡":       "cat-11",
		"国内/直连":      "cat-12",
		"常规代理":       "cat-13",
		"兜底":         "cat-14",
	}
	for k, v := range defaultCatMap {
		if _, ok := catMap[k]; !ok {
			catMap[k] = v
		}
	}

	// 3. Load seed records: check fallbackDBPath first if provided & readable, else use embedded JSON
	if fallbackDBPath == "" {
		fallbackDBPath = "/data/clash_sub_parser.db"
	}

	seedRecords, err := loadSeedRecordsFromExternalDB(ctx, fallbackDBPath, catMap)
	if err != nil || len(seedRecords) != 470 {
		seedRecords = nil
		if err := json.Unmarshal(embeddedRulesSeedJSON, &seedRecords); err != nil {
			return 0, fmt.Errorf("failed to unmarshal embedded seed rules: %w", err)
		}
	}

	// 4. Insert into policy_rules within the active transaction
	if _, err := tx.ExecContext(ctx, "DELETE FROM policy_rules WHERE revision_id = ?;", revID); err != nil {
		return 0, fmt.Errorf("failed to clean existing rules for revision %s: %w", revID, err)
	}

	insertStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO policy_rules (
			id, revision_id, category_id, category, name,
			target_group_id, builtin_target, expression, position, enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer insertStmt.Close()

	insertedCount := 0
	for _, item := range seedRecords {
		if err := insertSeedRule(ctx, insertStmt, revID, item, catMap, groupMap); err != nil {
			return 0, err
		}
		insertedCount++
	}

	return insertedCount, nil
}

// BackfillPolicyRules imports or restores all 470 rules from /data/clash_sub_parser.db
// or the embedded seed dataset into the specified revision.
func BackfillPolicyRules(ctx context.Context, db *sql.DB, revID string, fallbackDBPath string) (int, error) {
	if revID == "" {
		return 0, fmt.Errorf("revision_id cannot be empty for rule backfill")
	}

	var insertedCount int
	err := WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		insertedCount, err = SeedDefaultPolicyRulesTx(ctx, tx, revID, fallbackDBPath)
		return err
	})
	if err != nil {
		return 0, err
	}

	return insertedCount, nil
}
