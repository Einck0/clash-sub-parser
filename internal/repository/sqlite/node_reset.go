package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

type queryExecutor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type nodeResetRepository struct {
	db *sql.DB
}


// NewNodeResetRepository creates a new SQLite repository for executing clean-slate node resets.
func NewNodeResetRepository(db *sql.DB) domain.NodeResetRepository {
	return &nodeResetRepository{db: db}
}

// CheckForeignKeyIntegrity queries PRAGMA foreign_key_check and returns any violations.
func (r *nodeResetRepository) CheckForeignKeyIntegrity(ctx context.Context) ([]string, error) {
	return checkFKIntegrity(ctx, r.db)
}

func checkFKIntegrity(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return nil, fmt.Errorf("failed to execute foreign_key_check: %w", err)
	}
	defer rows.Close()

	var violations []string
	for rows.Next() {
		var (
			table  string
			rowid  int64
			parent string
			fkid   int
		)
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return nil, fmt.Errorf("failed to scan foreign_key_check row: %w", err)
		}
		violations = append(violations, fmt.Sprintf("table %s (rowid %d) references parent %s (fkid %d)", table, rowid, parent, fkid))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading foreign_key_check results: %w", err)
	}
	return violations, nil
}

// tableExists checks if a table exists in sqlite_master.
func tableExists(ctx context.Context, q queryExecutor, tableName string) bool {
	var count int
	_ = q.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?;", tableName).Scan(&count)
	return count > 0
}

// countTableSafe returns row count or 0 if table does not exist.
func countTableSafe(ctx context.Context, q queryExecutor, tableName string) int {
	if !tableExists(ctx, q, tableName) {
		return 0
	}
	var count int
	_ = q.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s;", tableName)).Scan(&count)
	return count
}

var nodeDerivedTables = []string{
	"nodes",
	"node_sources",
	"node_source_history",
	"node_connection_heads",
	"node_connection_versions",
	"node_overrides",
	"probe_observations",
	"probe_batch_runs",
	"probe_runs",
	"probe_batches",
	"ip_risk_observations",
}

var preservedBusinessTables = []string{
	"subscriptions",
	"node_groups",
	"policy_rules",
	"admission_rules",
	"global_node_filters",
	"group_node_filters",
	"probe_schedules",
	"publications",
	"publication_payload_refs",
	"settings",
	"configuration_revisions",
	"schema_migrations",
	"risk_policy_revisions",
	"risk_policy_providers",
	"risk_policy_score_bands",
	"risk_policy_trait_rules",
	"risk_policy_group_bindings",
	"ip_risk_provider_settings",
}

func computePreservedAssetFingerprint(ctx context.Context, q queryExecutor) (string, error) {
	h := sha256.New()
	for _, tbl := range preservedBusinessTables {
		if !tableExists(ctx, q, tbl) {
			continue
		}
		var count int
		if err := q.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s;", tbl)).Scan(&count); err != nil {
			return "", fmt.Errorf("failed to count preserved table %s: %w", tbl, err)
		}
		fmt.Fprintf(h, "%s:%d;", tbl, count)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ExecuteReset performs the clean-slate node inventory reset transaction.
func (r *nodeResetRepository) ExecuteReset(ctx context.Context, dryRun bool) (*domain.NodeInventoryResetReport, error) {
	// Enable foreign keys on connection
	if _, err := r.db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Pre-flight foreign key check: detect pre-existing violations
	preViolations, err := checkFKIntegrity(ctx, r.db)
	if err != nil {
		return nil, fmt.Errorf("pre-flight foreign key check failed: %w", err)
	}
	if len(preViolations) > 0 && !dryRun {
		return nil, fmt.Errorf("cannot reset: pre-flight check found existing foreign key violations in database: %s", strings.Join(preViolations, "; "))
	}

	preCounts := make(map[string]int)
	for _, tbl := range nodeDerivedTables {
		preCounts[tbl] = countTableSafe(ctx, r.db, tbl)
	}
	for _, tbl := range preservedBusinessTables {
		preCounts[tbl] = countTableSafe(ctx, r.db, tbl)
	}
	preCounts["subscription_entries"] = countTableSafe(ctx, r.db, "subscription_entries")
	preCounts["subscription_payloads"] = countTableSafe(ctx, r.db, "subscription_payloads")
	preCounts["subscription_fetches"] = countTableSafe(ctx, r.db, "subscription_fetches")
	preCounts["group_edges"] = countTableSafe(ctx, r.db, "group_edges")

	preFingerprint, err := computePreservedAssetFingerprint(ctx, r.db)
	if err != nil {
		return nil, err
	}

	var pubPayloadCount int
	if tableExists(ctx, r.db, "publication_payload_refs") {
		_ = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM publication_payload_refs;").Scan(&pubPayloadCount)
	}

	// Detached entries count calculation
	var detachedPublishedEntries int
	if pubPayloadCount > 0 && tableExists(ctx, r.db, "subscription_entries") {
		_ = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM subscription_entries
			WHERE payload_id IN (SELECT payload_id FROM publication_payload_refs)
			  AND node_logical_id IS NOT NULL;
		`).Scan(&detachedPublishedEntries)
	}

	// Capture impacted group_edges pointing to nodes before reset and construct re-binding plan
	var impactedBindings []domain.ImpactedBinding
	var rebindPlan []domain.RebindPlanItem
	if tableExists(ctx, r.db, "group_edges") {
		rows, qErr := r.db.QueryContext(ctx, `
			SELECT e.id, e.parent_group_id, e.node_logical_id, e.position,
			       COALESCE(n.protocol, ''), COALESCE(n.server, ''), COALESCE(n.port, 0)
			FROM group_edges e
			LEFT JOIN nodes n ON e.node_logical_id = n.logical_id
			WHERE e.node_logical_id IS NOT NULL AND trim(e.node_logical_id) != '';
		`)
		if qErr == nil {
			for rows.Next() {
				var (
					edgeID, parentID, nodeLogID, protoStr, srv string
					pos, port int
				)
				if scanErr := rows.Scan(&edgeID, &parentID, &nodeLogID, &pos, &protoStr, &srv, &port); scanErr == nil {
					binding := domain.ImpactedBinding{
						EdgeID:        edgeID,
						ParentGroupID: parentID,
						NodeLogicalID: nodeLogID,
						Position:      pos,
						TargetType:    "node",
					}
					impactedBindings = append(impactedBindings, binding)

					item := domain.RebindPlanItem{
						EdgeID:            edgeID,
						ParentGroupID:     parentID,
						OldNodeLogicalID:  nodeLogID,
						Protocol:          domain.Protocol(protoStr),
						Server:            srv,
						Port:              port,
						Position:          pos,
						Status:            "unbound_pending_fresh_match",
						DiagnosticMessage: "Node reference removed in clean reset; require exact match on fresh ComputeConnectionLogicalID",
					}
					if protoStr != "" && srv != "" && port > 0 {
						item.ExactConnectionID = domain.ComputeNodeLogicalID(domain.Protocol(protoStr), srv, port, nil)
					}
					rebindPlan = append(rebindPlan, item)
				}
			}
			rows.Close()
		}
	}

	report := &domain.NodeInventoryResetReport{
		DryRun:                   dryRun,
		PreCounts:                preCounts,
		PostCounts:               make(map[string]int),
		DeletedCounts:            make(map[string]int),
		FKViolations:             preViolations,
		DetachedPublishedEntries: detachedPublishedEntries,
		PreservedPublications:    preCounts["publications"],
		AssetFingerprint:         preFingerprint,
		ImpactedBindingsCount:    len(impactedBindings),
		ImpactedBindings:         impactedBindings,
		RebindPlanCount:          len(rebindPlan),
		RebindPlan:               rebindPlan,
		ExecutedAt:               domain.NowUTC(),
	}

	if dryRun {
		for _, tbl := range nodeDerivedTables {
			report.DeletedCounts[tbl] = preCounts[tbl]
			report.PostCounts[tbl] = 0
		}
		for _, tbl := range preservedBusinessTables {
			report.PostCounts[tbl] = preCounts[tbl]
		}
		report.PreservedAssetsUntouched = true
		return report, nil
	}

	// In apply mode, if there are impacted bindings, save protected private maintenance manifest (mode 0700/0600)
	if len(rebindPlan) > 0 {
		manifestDir := os.Getenv("CSP_MAINTENANCE_DIR")
		if manifestDir == "" {
			manifestDir = filepath.Join("data", "maintenance")
		}
		if mErr := os.MkdirAll(manifestDir, 0700); mErr != nil {
			manifestDir = os.TempDir()
		}
		manifestPath := filepath.Join(manifestDir, fmt.Sprintf("rebind-manifest-%d.json", time.Now().UnixNano()))
		if data, jErr := json.MarshalIndent(rebindPlan, "", "  "); jErr == nil {
			if wErr := os.WriteFile(manifestPath, data, 0600); wErr == nil {
				report.RebindManifestPath = manifestPath
			}
		}
	}

	// Begin atomic transaction
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to begin reset transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// Ensure FKs inside transaction
	if _, err := tx.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to set foreign_keys inside transaction: %w", err)
	}

	// 1. Probe observation and run tables
	if tableExists(ctx, tx, "probe_observations") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM probe_observations;"); err != nil {
			return nil, fmt.Errorf("failed to clear probe_observations: %w", err)
		}
	}
	if tableExists(ctx, tx, "probe_batch_runs") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM probe_batch_runs;"); err != nil {
			return nil, fmt.Errorf("failed to clear probe_batch_runs: %w", err)
		}
	}
	if tableExists(ctx, tx, "probe_runs") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM probe_runs;"); err != nil {
			return nil, fmt.Errorf("failed to clear probe_runs: %w", err)
		}
	}
	if tableExists(ctx, tx, "probe_batches") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM probe_batches;"); err != nil {
			return nil, fmt.Errorf("failed to clear probe_batches: %w", err)
		}
	}

	// 2. IP risk observations (foreign key RESTRICT)
	if tableExists(ctx, tx, "ip_risk_observations") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM ip_risk_observations;"); err != nil {
			return nil, fmt.Errorf("failed to clear ip_risk_observations: %w", err)
		}
	}

	// 3. Node history, sources, and overrides
	if tableExists(ctx, tx, "node_source_history") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM node_source_history;"); err != nil {
			return nil, fmt.Errorf("failed to clear node_source_history: %w", err)
		}
	}
	if tableExists(ctx, tx, "node_sources") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM node_sources;"); err != nil {
			return nil, fmt.Errorf("failed to clear node_sources: %w", err)
		}
	}
	if tableExists(ctx, tx, "node_overrides") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM node_overrides;"); err != nil {
			return nil, fmt.Errorf("failed to clear node_overrides: %w", err)
		}
	}

	// 4. Node connection heads & versions (heads has RESTRICT to versions)
	if tableExists(ctx, tx, "node_connection_heads") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM node_connection_heads;"); err != nil {
			return nil, fmt.Errorf("failed to clear node_connection_heads: %w", err)
		}
	}
	if tableExists(ctx, tx, "node_connection_versions") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM node_connection_versions;"); err != nil {
			return nil, fmt.Errorf("failed to clear node_connection_versions: %w", err)
		}
	}

	// 5. Explicit node edges in group_edges
	if tableExists(ctx, tx, "group_edges") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM group_edges WHERE node_logical_id IS NOT NULL AND trim(node_logical_id) != '';"); err != nil {
			return nil, fmt.Errorf("failed to clear node-targeting group_edges: %w", err)
		}
	}

	// 6. Subscriptions entries, payloads, and fetches (protect published payloads)
	hasPubRefs := tableExists(ctx, tx, "publication_payload_refs")
	if tableExists(ctx, tx, "subscription_entries") {
		if hasPubRefs {
			// Detach node_logical_id to NULL on published entries
			if _, err := tx.ExecContext(ctx, "UPDATE subscription_entries SET node_logical_id = NULL WHERE payload_id IN (SELECT payload_id FROM publication_payload_refs);"); err != nil {
				return nil, fmt.Errorf("failed to detach node on published subscription_entries: %w", err)
			}
			// Delete entries not pinned by publications
			if _, err := tx.ExecContext(ctx, "DELETE FROM subscription_entries WHERE payload_id NOT IN (SELECT payload_id FROM publication_payload_refs);"); err != nil {
				return nil, fmt.Errorf("failed to delete unpinned subscription_entries: %w", err)
			}
		} else {
			if _, err := tx.ExecContext(ctx, "DELETE FROM subscription_entries;"); err != nil {
				return nil, fmt.Errorf("failed to clear subscription_entries: %w", err)
			}
		}
	}

	if tableExists(ctx, tx, "subscription_payloads") {
		if hasPubRefs {
			if _, err := tx.ExecContext(ctx, "DELETE FROM subscription_payloads WHERE id NOT IN (SELECT payload_id FROM publication_payload_refs);"); err != nil {
				return nil, fmt.Errorf("failed to delete unpinned subscription_payloads: %w", err)
			}
		} else {
			if _, err := tx.ExecContext(ctx, "DELETE FROM subscription_payloads;"); err != nil {
				return nil, fmt.Errorf("failed to clear subscription_payloads: %w", err)
			}
		}
	}

	if tableExists(ctx, tx, "subscription_fetches") {
		if hasPubRefs {
			if _, err := tx.ExecContext(ctx, "DELETE FROM subscription_fetches WHERE id NOT IN (SELECT fetch_id FROM subscription_payloads);"); err != nil {
				return nil, fmt.Errorf("failed to delete unpinned subscription_fetches: %w", err)
			}
			// Preserved fetches retain their historical nodes_parsed and nodes_valid audit counts.
			// Membership to old nodes is severed by clearing node_sources, preserving fetch historical truth.
		} else {
			if _, err := tx.ExecContext(ctx, "DELETE FROM subscription_fetches;"); err != nil {
				return nil, fmt.Errorf("failed to clear subscription_fetches: %w", err)
			}
		}
	}

	// 7. Clear nodes table
	if tableExists(ctx, tx, "nodes") {
		if _, err := tx.ExecContext(ctx, "DELETE FROM nodes;"); err != nil {
			return nil, fmt.Errorf("failed to clear nodes: %w", err)
		}
	}

	// 8. Record audit event
	if tableExists(ctx, tx, "audit_events") {
		auditID := domain.MustNewUUIDv7()
		nowStr := domain.NowUTC().Format(time.RFC3339)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_events (id, actor_kind, action, redacted_summary, result, created_at)
			VALUES (?, 'system', 'inventory.nodes.clean_slate_reset', 'Clean-slate node inventory reset completed: all nodes and node-derived tables cleared', 'success', ?);
		`, auditID, nowStr); err != nil {
			return nil, fmt.Errorf("failed to record reset audit event: %w", err)
		}
	}

	// 9. Foreign key check within transaction
	fkRows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return nil, fmt.Errorf("failed to check foreign keys in transaction: %w", err)
	}
	var violations []string
	for fkRows.Next() {
		var table, parent string
		var rowid int64
		var fkid int
		if scanErr := fkRows.Scan(&table, &rowid, &parent, &fkid); scanErr == nil {
			violations = append(violations, fmt.Sprintf("table %s (rowid %d) references parent %s (fkid %d)", table, rowid, parent, fkid))
		}
	}
	fkRows.Close()

	if len(violations) > 0 {
		return nil, fmt.Errorf("foreign key integrity violation after reset: %s", strings.Join(violations, "; "))
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit reset transaction: %w", err)
	}
	tx = nil // mark committed

	// Compute post-counts and verify
	for _, tbl := range nodeDerivedTables {
		cnt := countTableSafe(ctx, r.db, tbl)
		report.PostCounts[tbl] = cnt
		report.DeletedCounts[tbl] = preCounts[tbl] - cnt
	}
	for _, tbl := range preservedBusinessTables {
		report.PostCounts[tbl] = countTableSafe(ctx, r.db, tbl)
	}
	report.PostCounts["subscription_entries"] = countTableSafe(ctx, r.db, "subscription_entries")
	report.PostCounts["subscription_payloads"] = countTableSafe(ctx, r.db, "subscription_payloads")
	report.PostCounts["subscription_fetches"] = countTableSafe(ctx, r.db, "subscription_fetches")
	report.PostCounts["group_edges"] = countTableSafe(ctx, r.db, "group_edges")

	postFingerprint, err := computePreservedAssetFingerprint(ctx, r.db)
	if err != nil {
		return nil, err
	}
	report.PreservedAssetsUntouched = (preFingerprint == postFingerprint)

	// Check post-commit foreign key integrity
	postViolations, err := checkFKIntegrity(ctx, r.db)
	if err != nil {
		return nil, err
	}
	report.FKViolations = postViolations

	return report, nil
}
