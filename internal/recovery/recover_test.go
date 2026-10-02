package recovery_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/recovery"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func setupRecoveryTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "recovery_test.sqlite")
	cfg := sqlite.Config{
		Path:        dbPath,
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}

	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	return db, dbPath
}

func TestRecovery_AttributionProtectionAndIdempotency(t *testing.T) {
	ctx := context.Background()
	db, dbPath := setupRecoveryTestDB(t)

	// Seed 4 nodes:
	// Node 1: Heuristic candidate (2026-09-29T00:52, active=0, valid config)
	// Node 2: Manually disabled node (2026-09-29T00:52, active=0, but has audit_event node.disable)
	// Node 3: Other inactive node from different time window (2026-09-30T10:00, active=0)
	// Node 4: Active node (active=1)

	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES
		('node_attributed_1', 'ss', 'Heuristic Node', '1.1.1.1', 8388, '{"password":"p1"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z'),
		('node_manually_disabled', 'ss', 'Disabled by User', '1.1.1.2', 8388, '{"password":"p2"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z'),
		('node_other_window', 'trojan', 'Other Window', '1.1.1.3', 443, '{"password":"p3"}', 0, '2026-09-28T00:00:00Z', '2026-09-30T10:00:00Z'),
		('node_already_active', 'vless', 'Already Active', '1.1.1.4', 443, '{"uuid":"u4"}', 1, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z');
	`)
	if err != nil {
		t.Fatalf("failed to seed nodes: %v", err)
	}

	// Insert audit event marking node_manually_disabled
	_, err = db.ExecContext(ctx, `
		INSERT INTO audit_events (id, actor_kind, request_id, action, result, redacted_summary, created_at)
		VALUES ('audit-1', 'user', 'req-1', 'node.disable', 'success', 'user disabled node_manually_disabled', '2026-09-29T00:50:00Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert audit event: %v", err)
	}

	// 1. Run Inspect
	plan, err := recovery.Inspect(ctx, db, dbPath, "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if plan.TotalInactiveNodes != 3 {
		t.Fatalf("expected 3 total inactive nodes, got %d", plan.TotalInactiveNodes)
	}

	// Heuristic candidates are separated and NOT automatically treated as confirmed proof
	if len(plan.HeuristicCandidates) != 1 || plan.HeuristicCandidates[0].LogicalID != "node_attributed_1" {
		t.Fatalf("expected exactly 1 heuristic candidate 'node_attributed_1', got %#v", plan.HeuristicCandidates)
	}
	if len(plan.AttributedCandidates) != 0 {
		t.Fatalf("expected 0 attributed candidates before explicit approval, got %#v", plan.AttributedCandidates)
	}

	if len(plan.AmbiguousNodes) != 1 || plan.AmbiguousNodes[0].LogicalID != "node_other_window" {
		t.Fatalf("expected exactly 1 ambiguous node 'node_other_window', got %#v", plan.AmbiguousNodes)
	}

	if len(plan.ManuallyDisabledIDs) != 1 || plan.ManuallyDisabledIDs[0] != "node_manually_disabled" {
		t.Fatalf("expected 'node_manually_disabled' protected, got %#v", plan.ManuallyDisabledIDs)
	}

	// 2. Fail-Closed: Attempting Apply without approval MUST fail
	_, err = recovery.Apply(ctx, db, plan)
	if err == nil {
		t.Fatalf("expected Apply to fail-closed on unapproved heuristic plan, got nil")
	}
	if !strings.Contains(err.Error(), "fail-closed") {
		t.Fatalf("expected fail-closed error message, got %v", err)
	}

	// 3. Rehearsal on test DB with simulated approval
	if err = recovery.ApproveForRehearsal(plan, "orchestrator-simulation", "test-rehearsal-drill"); err != nil {
		t.Fatalf("ApproveForRehearsal failed: %v", err)
	}
	if !plan.Approval.Simulated {
		t.Fatalf("expected rehearsal approval to be explicitly marked as simulated")
	}
	if len(plan.AttributedCandidates) != 1 {
		t.Fatalf("expected 1 candidate ready after rehearsal approval, got %d", len(plan.AttributedCandidates))
	}

	rep1, err := recovery.Apply(ctx, db, plan)
	if err != nil {
		t.Fatalf("Apply failed after rehearsal approval: %v", err)
	}
	if rep1.RowsAffected != 1 {
		t.Fatalf("expected 1 row affected on apply, got %d", rep1.RowsAffected)
	}
	if rep1.Mode != "rehearsal_apply" {
		t.Fatalf("expected mode 'rehearsal_apply', got %s", rep1.Mode)
	}

	// Verify node_attributed_1 is now active=1
	var active1, activeManual, activeOther int
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_attributed_1'").Scan(&active1)
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_manually_disabled'").Scan(&activeManual)
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_other_window'").Scan(&activeOther)

	if active1 != 1 {
		t.Fatalf("expected node_attributed_1 to be active (1), got %d", active1)
	}
	if activeManual != 0 {
		t.Fatalf("CRITICAL: manually disabled node was revived! must be 0, got %d", activeManual)
	}
	if activeOther != 0 {
		t.Fatalf("CRITICAL: ambiguous node was revived! must be 0, got %d", activeOther)
	}

	// 4. Test Idempotency: Re-running Apply must affect 0 rows
	rep2, err := recovery.Apply(ctx, db, plan)
	if err != nil {
		t.Fatalf("second Apply failed: %v", err)
	}
	if rep2.RowsAffected != 0 {
		t.Fatalf("expected 0 rows affected on second apply (idempotency), got %d", rep2.RowsAffected)
	}

	// 5. Test Rollback
	repRoll, err := recovery.Rollback(ctx, db, plan)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	if repRoll.RowsAffected != 1 {
		t.Fatalf("expected 1 row affected on rollback, got %d", repRoll.RowsAffected)
	}

	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_attributed_1'").Scan(&active1)
	if active1 != 0 {
		t.Fatalf("expected node_attributed_1 to be rolled back to inactive (0), got %d", active1)
	}

	// 6. Test Rollback Idempotency: Re-running Rollback must affect 0 rows
	repRoll2, err := recovery.Rollback(ctx, db, plan)
	if err != nil {
		t.Fatalf("second Rollback failed: %v", err)
	}
	if repRoll2.RowsAffected != 0 {
		t.Fatalf("expected 0 rows affected on second rollback, got %d", repRoll2.RowsAffected)
	}
}

func TestRecovery_FailClosedNegativeCases(t *testing.T) {
	ctx := context.Background()
	db, dbPath := setupRecoveryTestDB(t)

	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES ('node_safe_1', 'ss', 'Safe 1', '1.1.1.1', 8388, '{"password":"p1"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}

	plan, err := recovery.Inspect(ctx, db, dbPath, "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// Negative 1: Nil plan
	if _, err := recovery.Apply(ctx, db, nil); err == nil {
		t.Fatalf("expected error on nil plan")
	}

	// Negative 2: Unapproved heuristic candidates
	if _, err := recovery.Apply(ctx, db, plan); err == nil {
		t.Fatalf("expected fail-closed error without explicit approval")
	}

	// Negative 3: Empty approved_by or approval_evidence
	plan.Approval = &recovery.PlanApproval{ApprovedBy: "  ", ApprovalEvidence: ""}
	if _, err := recovery.Apply(ctx, db, plan); err == nil {
		t.Fatalf("expected fail-closed error with blank approval fields")
	}

	// Negative 4: Tampered plan hash
	_ = recovery.ApproveForRehearsal(plan, "orchestrator", "audit-token-1")
	plan.PlanHash = "corrupted_or_tampered_hash_0123456789abcdef"
	if _, err := recovery.Apply(ctx, db, plan); err == nil {
		t.Fatalf("expected apply to fail on tampered plan hash, got nil")
	}

	// Negative 5: Tampered approval evidence in approved plan
	_ = recovery.ApproveForRehearsal(plan, "orchestrator", "audit-token-1")
	plan.Approval.ApprovalEvidence = "different-evidence-tampered"
	if _, err := recovery.Apply(ctx, db, plan); err == nil {
		t.Fatalf("expected apply to fail when approval evidence altered without recomputing hash, got nil")
	}

	// Negative 6: Production path rejected with simulated approval
	prodPlan, _ := recovery.Inspect(ctx, db, "/data/csp-v1.db", "")
	_ = recovery.ApproveForRehearsal(prodPlan, "orchestrator-simulation", "audit-token-rehearsal")
	if _, err := recovery.Apply(ctx, db, prodPlan); err == nil {
		t.Fatalf("CRITICAL SECURITY HOLE: Apply allowed simulated approval on production DB path /data/csp-v1.db!")
	} else if !strings.Contains(err.Error(), "production database") && !strings.Contains(err.Error(), "fail-closed") {
		t.Fatalf("unexpected error message on production path: %v", err)
	}

	// Negative 7: Non-sandbox path rejected with simulated approval
	nonSandboxPlan, _ := recovery.Inspect(ctx, db, "/var/opt/important-production-db.sqlite", "")
	_ = recovery.ApproveForRehearsal(nonSandboxPlan, "orchestrator-simulation", "audit-token-rehearsal")
	if _, err := recovery.Apply(ctx, db, nonSandboxPlan); err == nil {
		t.Fatalf("CRITICAL SECURITY HOLE: Apply allowed simulated approval on non-sandbox DB path!")
	}

	// Negative 8: Real production apply without transactional causality proof is fail-closed rejected
	prodNonSimulatedPlan, _ := recovery.Inspect(ctx, db, dbPath, "")
	prodNonSimulatedPlan.Approval = &recovery.PlanApproval{
		ApprovedBy:       "user:admin",
		ApprovalEvidence: "manual-user-approval",
		ApprovedAt:       time.Now().UTC().Format(time.RFC3339),
		Simulated:        false,
	}
	prodNonSimulatedPlan.AttributedCandidates = append([]recovery.CandidateNode(nil), prodNonSimulatedPlan.HeuristicCandidates...)
	prodNonSimulatedPlan.PlanHash = recovery.ComputePlanHash(prodNonSimulatedPlan.AttributedCandidates, prodNonSimulatedPlan.Approval)
	if _, err := recovery.Apply(ctx, db, prodNonSimulatedPlan); err == nil {
		t.Fatalf("CRITICAL: Production apply without transactional causality proof must be strictly disabled!")
	} else if !strings.Contains(err.Error(), "transactional causality proof") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRecovery_ConcurrencyConflictProtection(t *testing.T) {
	ctx := context.Background()
	db, dbPath := setupRecoveryTestDB(t)

	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES ('node_concurrent_1', 'ss', 'Concurrent 1', '1.1.1.1', 8388, '{"password":"p1"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}

	plan, err := recovery.Inspect(ctx, db, dbPath, "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	_ = recovery.ApproveForRehearsal(plan, "orchestrator-simulation", "concurrency-test")

	// Another process modifies updated_at concurrently before Apply runs
	_, err = db.ExecContext(ctx, `UPDATE nodes SET updated_at = '2026-09-29T12:00:00Z' WHERE logical_id = 'node_concurrent_1'`)
	if err != nil {
		t.Fatalf("concurrent update failed: %v", err)
	}

	// Apply must detect conflict and affect 0 rows
	report, err := recovery.Apply(ctx, db, plan)
	if err != nil {
		t.Fatalf("Apply returned error on concurrent change: %v", err)
	}
	if report.RowsAffected != 0 {
		t.Fatalf("expected 0 rows affected due to optimistic concurrency conflict, got %d", report.RowsAffected)
	}
}

func TestRecovery_RollbackProtectsSubsequentUserModifications(t *testing.T) {
	ctx := context.Background()
	db, dbPath := setupRecoveryTestDB(t)

	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES
		('node_user_edited', 'ss', 'User Edited', '1.1.1.1', 8388, '{"password":"p1"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z'),
		('node_untouched', 'ss', 'Untouched', '1.1.1.2', 8388, '{"password":"p2"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert nodes: %v", err)
	}

	plan, err := recovery.Inspect(ctx, db, dbPath, "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	_ = recovery.ApproveForRehearsal(plan, "orchestrator-simulation", "drill-user-protect")

	// 1. Apply recovery: restores both nodes
	repApply, err := recovery.Apply(ctx, db, plan)
	if err != nil || repApply.RowsAffected != 2 {
		t.Fatalf("expected 2 rows affected on apply, got rows=%d err=%v", repApply.RowsAffected, err)
	}

	// 2. User subsequently modifies 'node_user_edited' (e.g. renames or changes settings, updating updated_at)
	_, err = db.ExecContext(ctx, `UPDATE nodes SET display_name = 'User Renamed', updated_at = '2026-10-01T12:00:00Z' WHERE logical_id = 'node_user_edited'`)
	if err != nil {
		t.Fatalf("user edit failed: %v", err)
	}

	// 3. Rollback: MUST ONLY rollback 'node_untouched' and NOT revert 'node_user_edited'
	repRoll, err := recovery.Rollback(ctx, db, plan)
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	if repRoll.RowsAffected != 1 {
		t.Fatalf("expected exactly 1 row rolled back (protecting user edit), got %d", repRoll.RowsAffected)
	}

	var activeEdited, activeUntouched int
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_user_edited'").Scan(&activeEdited)
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_untouched'").Scan(&activeUntouched)

	if activeEdited != 1 {
		t.Fatalf("subsequent user edit was clobbered by rollback! expected active=1, got %d", activeEdited)
	}
	if activeUntouched != 0 {
		t.Fatalf("untouched node was not rolled back! expected active=0, got %d", activeUntouched)
	}
}

func TestRecovery_StalePrecondition_AbortsEntireBatchWithoutChanges(t *testing.T) {
	ctx := context.Background()
	db, dbPath := setupRecoveryTestDB(t)

	// Seed 2 candidate nodes
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES
		('node_batch_1', 'ss', 'Batch 1', '1.1.1.1', 8388, '{"password":"p1"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z'),
		('node_batch_2', 'ss', 'Batch 2', '1.1.1.2', 8388, '{"password":"p2"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert batch nodes: %v", err)
	}

	plan, err := recovery.Inspect(ctx, db, dbPath, "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	_ = recovery.ApproveForRehearsal(plan, "orchestrator-simulation", "stale-precondition-drill")

	if len(plan.AttributedCandidates) != 2 {
		t.Fatalf("expected 2 candidates in plan, got %d", len(plan.AttributedCandidates))
	}

	// Concurrently tamper/modify node_batch_2 state (stale precondition)
	_, err = db.ExecContext(ctx, `UPDATE nodes SET updated_at = '2026-10-02T00:00:00Z' WHERE logical_id = 'node_batch_2'`)
	if err != nil {
		t.Fatalf("concurrent update failed: %v", err)
	}

	// Apply must detect the stale precondition on node_batch_2 and abort the ENTIRE batch.
	// Neither node_batch_1 nor node_batch_2 should be activated!
	report, err := recovery.Apply(ctx, db, plan)
	if err != nil {
		t.Fatalf("Apply returned unexpected error: %v", err)
	}
	if !report.AbortedDueToStale {
		t.Fatalf("expected report.AbortedDueToStale to be true")
	}
	if report.StalePreconditionNode != "node_batch_2" {
		t.Fatalf("expected stale node to be node_batch_2, got %q", report.StalePreconditionNode)
	}
	if report.RowsAffected != 0 {
		t.Fatalf("expected 0 rows affected (entire batch aborted), got %d", report.RowsAffected)
	}

	// Verify both nodes remain active = 0 in database
	var active1, active2 int
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_batch_1'").Scan(&active1)
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_batch_2'").Scan(&active2)
	if active1 != 0 || active2 != 0 {
		t.Fatalf("CRITICAL: node was activated despite stale precondition! active1=%d, active2=%d", active1, active2)
	}
}

func TestRecovery_AllowlistFilteringAndPlanHash(t *testing.T) {
	ctx := context.Background()
	db, dbPath := setupRecoveryTestDB(t)

	// Seed 3 candidate nodes
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES
		('node_allow_1', 'ss', 'Allow 1', '1.1.1.1', 8388, '{"password":"p1"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z'),
		('node_allow_2', 'ss', 'Allow 2', '1.1.1.2', 8388, '{"password":"p2"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z'),
		('node_allow_3', 'ss', 'Allow 3', '1.1.1.3', 8388, '{"password":"p3"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert allowlist test nodes: %v", err)
	}

	plan, err := recovery.Inspect(ctx, db, dbPath, "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	_ = recovery.ApproveForRehearsal(plan, "orchestrator-simulation", "allowlist-test")

	if len(plan.AttributedCandidates) != 3 {
		t.Fatalf("expected 3 candidates before filter, got %d", len(plan.AttributedCandidates))
	}

	// Filter strictly to allowlist containing only node_allow_1 and node_allow_3
	plan.FilterByAllowlist([]string{"node_allow_1", "node_allow_3"})

	if len(plan.AttributedCandidates) != 2 {
		t.Fatalf("expected 2 candidates after allowlist filter, got %d", len(plan.AttributedCandidates))
	}
	if plan.AttributedCandidates[0].LogicalID != "node_allow_1" || plan.AttributedCandidates[1].LogicalID != "node_allow_3" {
		t.Fatalf("unexpected candidates after filter: %#v", plan.AttributedCandidates)
	}

	// Apply should only restore the 2 allowlisted nodes
	report, err := recovery.Apply(ctx, db, plan)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if report.RowsAffected != 2 {
		t.Fatalf("expected 2 rows affected, got %d", report.RowsAffected)
	}

	var active1, active2, active3 int
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_allow_1'").Scan(&active1)
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_allow_2'").Scan(&active2)
	_ = db.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = 'node_allow_3'").Scan(&active3)

	if active1 != 1 || active3 != 1 {
		t.Fatalf("expected allowlisted nodes to be active, got active1=%d, active3=%d", active1, active3)
	}
	if active2 != 0 {
		t.Fatalf("excluded node_allow_2 was revived! expected active=0, got %d", active2)
	}
}

func TestRecovery_AmbiguousCandidatesFixture(t *testing.T) {
	ctx := context.Background()
	db, dbPath := setupRecoveryTestDB(t)

	// Seed 4 nodes with distinct causal ambiguity conditions:
	// 1. Invalid JSON config
	// 2. Missing server / port <= 0
	// 3. Mismatched timestamp window
	// 4. Inactive node that still has active node_sources binding
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at)
		VALUES ('sub-fixture-1', 'Fixture Sub', 'secret-ref', 'rev-1', '2026-09-28T00:00:00Z', '2026-09-28T00:00:00Z');

		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES
		('node_bad_json', 'ss', 'Bad JSON', '1.1.1.1', 8388, '{invalid-json', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z'),
		('node_no_server', 'ss', 'No Server', '', 0, '{"password":"p"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z'),
		('node_wrong_time', 'ss', 'Wrong Time', '1.1.1.3', 8388, '{"password":"p"}', 0, '2026-09-28T00:00:00Z', '2026-09-30T12:00:00Z'),
		('node_with_sources', 'ss', 'With Source', '1.1.1.4', 8388, '{"password":"p"}', 0, '2026-09-28T00:00:00Z', '2026-09-29T00:52:15Z');

		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES ('node_with_sources', 'sub-fixture-1', 'fetch-1');
	`)
	if err != nil {
		t.Fatalf("failed to insert ambiguity fixture nodes: %v", err)
	}

	plan, err := recovery.Inspect(ctx, db, dbPath, "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// ALL 4 anomalous nodes MUST be categorized as AmbiguousNodes
	if len(plan.HeuristicCandidates) != 0 {
		t.Fatalf("expected 0 heuristic candidates from anomalous nodes, got %d", len(plan.HeuristicCandidates))
	}
	if len(plan.AttributedCandidates) != 0 {
		t.Fatalf("expected 0 attributed candidates, got %d", len(plan.AttributedCandidates))
	}
	if len(plan.AmbiguousNodes) != 4 {
		t.Fatalf("expected exactly 4 ambiguous nodes, got %d", len(plan.AmbiguousNodes))
	}

	ambiguousMap := make(map[string]bool)
	for _, n := range plan.AmbiguousNodes {
		ambiguousMap[n.LogicalID] = true
	}
	for _, expectedID := range []string{"node_bad_json", "node_no_server", "node_wrong_time", "node_with_sources"} {
		if !ambiguousMap[expectedID] {
			t.Fatalf("expected node %s to be in AmbiguousNodes", expectedID)
		}
	}
}

