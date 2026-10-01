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
