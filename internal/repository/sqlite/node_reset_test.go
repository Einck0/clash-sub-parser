package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/migrations"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	runner := NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	return db
}

func populateSampleData(t *testing.T, db *sql.DB) (string, string) {
	t.Helper()
	ctx := context.Background()

	subID := domain.MustNewUUIDv7()
	fetchID := domain.MustNewUUIDv7()
	nodeID := domain.MustNewUUIDv7()
	runID := domain.MustNewUUIDv7()
	batchID := domain.MustNewUUIDv7()
	groupID := domain.MustNewUUIDv7()
	nowStr := domain.NowUTC().Format(time.RFC3339)

	// Subscriptions
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'TestSub', 'https://example.com/sub', 1, 'rev1', ?, ?);
	`, subID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert subscription: %v", err)
	}

	// Subscription fetches
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 1, 1);
	`, fetchID, subID, nowStr)
	if err != nil {
		t.Fatalf("insert fetch: %v", err)
	}

	// Nodes
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, connection_revision, created_at, updated_at)
		VALUES (?, 'vmess', 'TestNode', '1.2.3.4', 443, '{}', 1, 1, ?, ?);
	`, nodeID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}

	// Connection version and head
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_connection_versions (node_logical_id, connection_revision, effective_config_json, config_fingerprint, created_at)
		VALUES (?, 1, '{"server":"1.2.3.4","port":443}', 'fp123', ?);
	`, nodeID, nowStr)
	if err != nil {
		t.Fatalf("insert connection version: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)
		VALUES (?, 1, ?);
	`, nodeID, nowStr)
	if err != nil {
		t.Fatalf("insert connection head: %v", err)
	}

	// Node sources
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, nodeID, subID, fetchID)
	if err != nil {
		t.Fatalf("insert node source: %v", err)
	}

	// Node source history
	histID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_source_history (id, node_logical_id, subscription_id, source_identity, source_label, relation_state, cause, evidence_kind, created_at)
		VALUES (?, ?, ?, 'sub:1', 'TestSub', 'verified', 'legacy_import', 'cold_archive', ?);
	`, histID, nodeID, subID, nowStr)
	if err != nil {
		t.Fatalf("insert node source history: %v", err)
	}

	// Probe batches and runs
	_, err = db.ExecContext(ctx, `
		INSERT INTO probe_batches (id, generation, window_at, total_nodes, state, created_at, updated_at)
		VALUES (?, 1, ?, 1, 'completed', ?, ?);
	`, batchID, nowStr, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert probe batch: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO probe_runs (id, actor_scope, idempotency_key, state, deadline_at, created_at, updated_at)
		VALUES (?, 'default', 'key1', 'completed', ?, ?, ?);
	`, runID, nowStr, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert probe run: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO probe_batch_runs (batch_id, probe_run_id)
		VALUES (?, ?);
	`, batchID, runID)
	if err != nil {
		t.Fatalf("insert probe batch run: %v", err)
	}

	obsID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO probe_observations (id, probe_run_id, node_logical_id, kind, verdict, latency_ms, observed_at)
		VALUES (?, ?, ?, 'connectivity', 'healthy', 50, ?);
	`, obsID, runID, nodeID, nowStr)
	if err != nil {
		t.Fatalf("insert probe observation: %v", err)
	}

	// Node groups
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at)
		VALUES (?, 'ProxyGroup', 'select', ?, ?);
	`, groupID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert node group: %v", err)
	}

	// Group edges pointing to node
	edgeID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO group_edges (id, parent_group_id, node_logical_id, position)
		VALUES (?, ?, ?, 0);
	`, edgeID, groupID, nodeID)
	if err != nil {
		t.Fatalf("insert group edge: %v", err)
	}

	return subID, nodeID
}

func TestNodeResetRepository_DryRun(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	populateSampleData(t, db)
	repo := NewNodeResetRepository(db)

	ctx := context.Background()
	report, err := repo.ExecuteReset(ctx, true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if !report.DryRun {
		t.Errorf("expected DryRun true, got %v", report.DryRun)
	}
	if report.PreCounts["nodes"] != 1 {
		t.Errorf("expected 1 pre node, got %d", report.PreCounts["nodes"])
	}
	if report.DeletedCounts["nodes"] != 1 {
		t.Errorf("expected 1 planned deleted node, got %d", report.DeletedCounts["nodes"])
	}

	// Check that node still exists in DB
	var nodeCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes;").Scan(&nodeCount)
	if nodeCount != 1 {
		t.Fatalf("expected node count 1 after dry run, got %d", nodeCount)
	}
}

func TestNodeResetRepository_Apply(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	subID, _ := populateSampleData(t, db)
	repo := NewNodeResetRepository(db)

	ctx := context.Background()
	report, err := repo.ExecuteReset(ctx, false)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if report.DryRun {
		t.Errorf("expected DryRun false")
	}
	if len(report.FKViolations) > 0 {
		t.Errorf("expected 0 FK violations, got %v", report.FKViolations)
	}
	if !report.PreservedAssetsUntouched {
		t.Errorf("expected PreservedAssetsUntouched true")
	}

	// Verify all node-derived tables are 0
	for _, tbl := range nodeDerivedTables {
		var cnt int
		_ = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s;", tbl)).Scan(&cnt)
		if cnt != 0 {
			t.Errorf("table %s not 0: %d", tbl, cnt)
		}
	}

	// Verify subscription is preserved
	var subCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscriptions WHERE id = ?;", subID).Scan(&subCount)
	if subCount != 1 {
		t.Errorf("subscription was deleted or altered")
	}

	// Verify group is preserved
	var groupCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_groups;").Scan(&groupCount)
	if groupCount != 1 {
		t.Errorf("node_group count expected 1, got %d", groupCount)
	}

	// Verify group edge pointing to node is cleared
	var edgeCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM group_edges WHERE node_logical_id IS NOT NULL AND node_logical_id != '';").Scan(&edgeCount)
	if edgeCount != 0 {
		t.Errorf("node-targeting group_edges not 0: %d", edgeCount)
	}

	// Verify FK integrity
	fkV, err := repo.CheckForeignKeyIntegrity(ctx)
	if err != nil {
		t.Fatalf("CheckForeignKeyIntegrity: %v", err)
	}
	if len(fkV) > 0 {
		t.Errorf("FK violations found: %v", fkV)
	}
}

func TestNodeResetRepository_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	populateSampleData(t, db)
	repo := NewNodeResetRepository(db)
	ctx := context.Background()

	// First reset
	_, err := repo.ExecuteReset(ctx, false)
	if err != nil {
		t.Fatalf("first reset: %v", err)
	}

	// Second reset on already clean DB
	report2, err := repo.ExecuteReset(ctx, false)
	if err != nil {
		t.Fatalf("second reset: %v", err)
	}
	if report2.PostCounts["nodes"] != 0 {
		t.Errorf("expected nodes 0 on second reset, got %d", report2.PostCounts["nodes"])
	}
	if len(report2.FKViolations) > 0 {
		t.Errorf("expected 0 FK violations on second reset, got %v", report2.FKViolations)
	}
}

func TestNodeResetRepository_PublicationPreservation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	subID, nodeID := populateSampleData(t, db)
	ctx := context.Background()
	nowStr := domain.NowUTC().Format(time.RFC3339)

	// Create payload and publication
	fetchID := domain.MustNewUUIDv7()
	payloadID := domain.MustNewUUIDv7()
	entryID := domain.MustNewUUIDv7()
	pubID := domain.MustNewUUIDv7()

	_, err := db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 15, 12);
	`, fetchID, subID, nowStr)
	if err != nil {
		t.Fatalf("insert fetch: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, pinned, created_at)
		VALUES (?, ?, ?, 'digest123', X'deadbeef', 1, ?);
	`, payloadID, subID, fetchID, nowStr)
	if err != nil {
		t.Fatalf("insert payload: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_entries (id, payload_id, subscription_id, ordinal, source_key, raw_name, protocol, entry_kind, node_logical_id, created_at)
		VALUES (?, ?, ?, 0, 'key1', 'raw1', 'vmess', 'proxy', ?, ?);
	`, entryID, payloadID, subID, nodeID, nowStr)
	if err != nil {
		t.Fatalf("insert entry: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO publications (id, target, snapshot_digest, compiler_version, token_hash, state, created_at, content)
		VALUES (?, 'clash', 'snap123', 'v1', 'hash123', 'published', ?, X'010203');
	`, pubID, nowStr)
	if err != nil {
		t.Fatalf("insert publication: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO publication_payload_refs (publication_id, payload_id)
		VALUES (?, ?);
	`, pubID, payloadID)
	if err != nil {
		t.Fatalf("insert pub ref: %v", err)
	}

	repo := NewNodeResetRepository(db)
	report, err := repo.ExecuteReset(ctx, false)
	if err != nil {
		t.Fatalf("execute reset with publication: %v", err)
	}

	if report.DetachedPublishedEntries != 1 {
		t.Errorf("expected 1 detached published entry, got %d", report.DetachedPublishedEntries)
	}

	// Verify publication is kept
	var pubCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications WHERE id = ?;", pubID).Scan(&pubCount)
	if pubCount != 1 {
		t.Errorf("publication not preserved")
	}

	// Verify payload is kept and body bytes are stable
	var payloadCount int
	var bodyBlob []byte
	err = db.QueryRowContext(ctx, "SELECT COUNT(*), body_blob FROM subscription_payloads WHERE id = ? GROUP BY body_blob;", payloadID).Scan(&payloadCount, &bodyBlob)
	if err != nil || payloadCount != 1 {
		t.Fatalf("published payload not preserved: count=%d, err=%v", payloadCount, err)
	}
	if len(bodyBlob) != 4 || bodyBlob[0] != 0xde || bodyBlob[1] != 0xad {
		t.Errorf("payload body_blob altered: got %x", bodyBlob)
	}

	// Verify publication content bytes are stable
	var pubContent []byte
	err = db.QueryRowContext(ctx, "SELECT content FROM publications WHERE id = ?;", pubID).Scan(&pubContent)
	if err != nil {
		t.Fatalf("query publication content: %v", err)
	}
	if len(pubContent) != 3 || pubContent[0] != 0x01 || pubContent[1] != 0x02 || pubContent[2] != 0x03 {
		t.Errorf("publication content altered: got %x", pubContent)
	}

	// Verify preserved fetch preserves historical audit counts (nodes_parsed=15, nodes_valid=12, NOT zeroed)
	var nodesParsed, nodesValid int
	err = db.QueryRowContext(ctx, "SELECT nodes_parsed, nodes_valid FROM subscription_fetches WHERE id = ?;", fetchID).Scan(&nodesParsed, &nodesValid)
	if err != nil {
		t.Fatalf("query fetch: %v", err)
	}
	if nodesParsed != 15 || nodesValid != 12 {
		t.Errorf("preserved fetch audit counts corrupted: nodes_parsed=%d (expected 15), nodes_valid=%d (expected 12)", nodesParsed, nodesValid)
	}

	// Verify entry is kept but node_logical_id is NULL
	var nodeLogID sql.NullString
	err = db.QueryRowContext(ctx, "SELECT node_logical_id FROM subscription_entries WHERE id = ?;", entryID).Scan(&nodeLogID)
	if err != nil {
		t.Fatalf("query entry: %v", err)
	}
	if nodeLogID.Valid {
		t.Errorf("expected node_logical_id to be NULL, got %s", nodeLogID.String)
	}

	// Verify FK integrity
	fkV, err := repo.CheckForeignKeyIntegrity(ctx)
	if err != nil {
		t.Fatalf("CheckForeignKeyIntegrity: %v", err)
	}
	if len(fkV) > 0 {
		t.Errorf("FK violations: %v", fkV)
	}
}

func TestNodeResetRepository_RollbackOnFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	populateSampleData(t, db)
	ctx := context.Background()

	// Corrupt a table schema or add a trigger that aborts deletion
	_, err := db.ExecContext(ctx, `
		CREATE TRIGGER abort_nodes_delete
		BEFORE DELETE ON nodes
		BEGIN
			SELECT RAISE(ABORT, 'forced rollback test failure');
		END;
	`)
	if err != nil {
		t.Fatalf("create abort trigger: %v", err)
	}

	repo := NewNodeResetRepository(db)
	_, err = repo.ExecuteReset(ctx, false)
	if err == nil {
		t.Fatalf("expected error from abort trigger, got nil")
	}

	// Verify rollback: nodes must still exist!
	var nodeCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes;").Scan(&nodeCount)
	if nodeCount != 1 {
		t.Fatalf("expected 1 node after rollback, got %d", nodeCount)
	}
}

func TestNodeResetRepository_ImpactedBindingsAndRebindPlan(t *testing.T) {
	t.Setenv("CSP_MAINTENANCE_DIR", t.TempDir())
	db := setupTestDB(t)
	defer db.Close()

	_, nodeID := populateSampleData(t, db)
	repo := NewNodeResetRepository(db)
	ctx := context.Background()

	// Dry run should surface impacted bindings and rebind plan without touching DB
	dryReport, err := repo.ExecuteReset(ctx, true)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if dryReport.ImpactedBindingsCount != 1 {
		t.Errorf("expected 1 impacted binding in dry run, got %d", dryReport.ImpactedBindingsCount)
	}
	if dryReport.RebindPlanCount != 1 {
		t.Errorf("expected 1 rebind plan item in dry run, got %d", dryReport.RebindPlanCount)
	}
	if len(dryReport.RebindPlan) > 0 && dryReport.RebindPlan[0].OldNodeLogicalID != nodeID {
		t.Errorf("expected old node ID %s, got %s", nodeID, dryReport.RebindPlan[0].OldNodeLogicalID)
	}

	// Apply should clear node and write maintenance manifest
	applyReport, err := repo.ExecuteReset(ctx, false)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if applyReport.ImpactedBindingsCount != 1 {
		t.Errorf("expected 1 impacted binding in apply, got %d", applyReport.ImpactedBindingsCount)
	}
	if applyReport.RebindManifestPath != "" {
		fi, err := os.Stat(applyReport.RebindManifestPath)
		if err != nil {
			t.Errorf("manifest file not accessible: %v", err)
		} else {
			// Permission check: file mode should be 0600
			if fi.Mode().Perm() != 0600 {
				t.Errorf("manifest permissions expected 0600, got %o", fi.Mode().Perm())
			}
			_ = os.Remove(applyReport.RebindManifestPath)
		}
	}

	// Verify all node-targeting group_edges deleted
	var edgeCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM group_edges WHERE node_logical_id IS NOT NULL;").Scan(&edgeCount)
	if edgeCount != 0 {
		t.Errorf("expected 0 node-targeting edges, got %d", edgeCount)
	}
}

func TestNodeResetRepository_PreflightFKViolation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	populateSampleData(t, db)
	ctx := context.Background()

	// Temporarily disable foreign keys to inject a dirty orphan FK
	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF;")
	if err != nil {
		t.Fatalf("disable FKs: %v", err)
	}
	// Insert group edge referencing non-existent node
	orphanNodeID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO group_edges (id, parent_group_id, node_logical_id, position)
		SELECT ?, id, ?, 99 FROM node_groups LIMIT 1;
	`, domain.MustNewUUIDv7(), orphanNodeID)
	if err != nil {
		t.Fatalf("insert orphan edge: %v", err)
	}
	_, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON;")
	if err != nil {
		t.Fatalf("enable FKs: %v", err)
	}

	repo := NewNodeResetRepository(db)

	// In apply mode, reset must reject the dirty database ahead of time
	report, err := repo.ExecuteReset(ctx, false)
	if err == nil {
		t.Fatalf("expected apply to fail on pre-existing FK violation, got nil error")
	}
	if report != nil && len(report.FKViolations) == 0 {
		t.Errorf("expected FKViolations in report")
	}
}

