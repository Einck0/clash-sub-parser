package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"

	_ "modernc.org/sqlite"
)

func setupTestLifecycleDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test-lifecycle.db")

	dbCfg := sqlite.DefaultConfig(dbPath)
	db, err := sqlite.Open(dbCfg)
	if err != nil {
		t.Fatalf("failed to open sqlite database: %v", err)
	}

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return dbPath, db
}

func countPolicyRules(t *testing.T, db *sql.DB, revID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM policy_rules WHERE revision_id = ?;", revID).Scan(&count); err != nil {
		t.Fatalf("count policy_rules failed: %v", err)
	}
	return count
}

func countAllRevisions(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM configuration_revisions;").Scan(&count); err != nil {
		t.Fatalf("count configuration_revisions failed: %v", err)
	}
	return count
}

// TestPolicyRuleLifecycle_FreshDB_Bootstrap470Rules verifies that a fresh database
// with no revisions automatically bootstraps an initial active revision with exactly 470 rules.
func TestPolicyRuleLifecycle_FreshDB_Bootstrap470Rules(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}

	activeRev, err := appServices.revisionRepo.GetActive(ctx)
	if err != nil || activeRev == nil {
		t.Fatalf("expected active revision on fresh bootstrap, got: %v", err)
	}

	count := countPolicyRules(t, db, activeRev.ID)
	if count != 470 {
		t.Fatalf("expected exactly 470 default policy rules on fresh DB, got %d", count)
	}

	rules, err := appServices.policyRepo.ListPolicyRules(ctx, activeRev.ID)
	if err != nil {
		t.Fatalf("ListPolicyRules failed: %v", err)
	}
	if len(rules) != 470 {
		t.Fatalf("expected 470 listed rules, got %d", len(rules))
	}

	lastRule := rules[len(rules)-1]
	if !domain.IsMatchRule(lastRule.Expression) {
		t.Fatalf("expected terminal rule to be MATCH, got %s", lastRule.Expression)
	}
}

// TestPolicyRuleLifecycle_SecondStartup_Preserves470Rules verifies that a second startup
// does not re-seed or duplicate rules when the database already has an active revision.
func TestPolicyRuleLifecycle_SecondStartup_Preserves470Rules(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	// First startup
	appServices1, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("first makeApplicationServices failed: %v", err)
	}
	activeRev1, err := appServices1.revisionRepo.GetActive(ctx)
	if err != nil || activeRev1 == nil {
		t.Fatalf("active revision missing after first start: %v", err)
	}
	if count := countPolicyRules(t, db, activeRev1.ID); count != 470 {
		t.Fatalf("expected 470 rules after first start, got %d", count)
	}

	// Second startup (restart simulation on same DB)
	appServices2, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("second makeApplicationServices failed: %v", err)
	}
	activeRev2, err := appServices2.revisionRepo.GetActive(ctx)
	if err != nil || activeRev2 == nil {
		t.Fatalf("active revision missing after second start: %v", err)
	}
	if activeRev2.ID != activeRev1.ID {
		t.Fatalf("active revision ID changed across restarts: %s != %s", activeRev2.ID, activeRev1.ID)
	}
	if count := countPolicyRules(t, db, activeRev2.ID); count != 470 {
		t.Fatalf("expected rules to remain exactly 470 after restart, got %d", count)
	}
}

// TestPolicyRuleLifecycle_DeleteOneAndModifyAnother_RestartPreserves469 verifies that
// deleting a rule (leaving 469) and modifying another rule is NOT overwritten on restart.
func TestPolicyRuleLifecycle_DeleteOneAndModifyAnother_RestartPreserves469(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	// Initial startup
	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}
	activeRev, err := appServices.revisionRepo.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	rules, err := appServices.policyRepo.ListPolicyRules(ctx, activeRev.ID)
	if err != nil || len(rules) != 470 {
		t.Fatalf("expected 470 rules, got %d (err: %v)", len(rules), err)
	}

	// Delete rule 0
	deletedRuleID := rules[0].ID
	if err := appServices.policyService.DeleteRule(ctx, policy.DeleteRuleCommand{
		ID:        deletedRuleID,
		RequestID: "req-del-1",
		ActorKind: domain.ActorKindAdmin,
	}); err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}

	// Modify rule 1
	modifiedRuleID := rules[1].ID
	newExpr := "DOMAIN-SUFFIX,custom-remediated-domain.org"
	updated, err := appServices.policyService.UpdatePolicyRule(ctx, policy.UpdatePolicyRuleCommand{
		ID:         modifiedRuleID,
		Expression: &newExpr,
		RequestID:  "req-mod-1",
		ActorKind:  domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("UpdatePolicyRule failed: %v", err)
	}
	if updated.Expression != newExpr {
		t.Fatalf("expected updated expression %s, got %s", newExpr, updated.Expression)
	}

	if count := countPolicyRules(t, db, activeRev.ID); count != 469 {
		t.Fatalf("expected 469 rules after deletion, got %d", count)
	}

	// Restart service on same DB
	appServicesRestarted, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("restarted makeApplicationServices failed: %v", err)
	}

	// Verify rule count remains 469 (must NOT auto-reseed to 470)
	countAfterRestart := countPolicyRules(t, db, activeRev.ID)
	if countAfterRestart != 469 {
		t.Fatalf("expected rule count to remain 469 after restart, got %d", countAfterRestart)
	}

	// Verify deleted rule is still gone
	_, err = appServicesRestarted.policyRepo.GetPolicyRule(ctx, deletedRuleID)
	if err == nil {
		t.Fatalf("expected deleted rule %s to remain gone after restart, but found it", deletedRuleID)
	}

	// Verify modified rule retains its updated expression
	modRule, err := appServicesRestarted.policyRepo.GetPolicyRule(ctx, modifiedRuleID)
	if err != nil {
		t.Fatalf("failed to get modified rule after restart: %v", err)
	}
	if modRule.Expression != newExpr {
		t.Fatalf("modified rule reverted on restart: expected %s, got %s", newExpr, modRule.Expression)
	}
}

// TestPolicyRuleLifecycle_DeliberatelyCleared_RestartPreservesZero verifies that
// when all rules are deliberately cleared from an active revision, restart does NOT re-seed.
func TestPolicyRuleLifecycle_DeliberatelyCleared_RestartPreservesZero(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}
	activeRev, err := appServices.revisionRepo.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	// Clear all rules for the active revision
	if _, err := db.ExecContext(ctx, "DELETE FROM policy_rules WHERE revision_id = ?;", activeRev.ID); err != nil {
		t.Fatalf("clearing rules failed: %v", err)
	}
	if count := countPolicyRules(t, db, activeRev.ID); count != 0 {
		t.Fatalf("expected 0 rules after deliberate clear, got %d", count)
	}

	// Restart service
	appServicesRestarted, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("restarted makeApplicationServices failed: %v", err)
	}
	_ = appServicesRestarted

	// Must remain 0 rules, no auto-reseed
	countAfterRestart := countPolicyRules(t, db, activeRev.ID)
	if countAfterRestart != 0 {
		t.Fatalf("deliberately emptied rules were resurrected on restart: got %d rules, want 0", countAfterRestart)
	}
}

// TestPolicyRuleLifecycle_CustomRules_RestartPreservesCustom verifies that a small set of
// custom user rules is untouched on restart.
func TestPolicyRuleLifecycle_CustomRules_RestartPreservesCustom(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}
	activeRev, err := appServices.revisionRepo.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	// Clear default rules and insert 3 custom rules
	if _, err := db.ExecContext(ctx, "DELETE FROM policy_rules WHERE revision_id = ?;", activeRev.ID); err != nil {
		t.Fatalf("clearing rules failed: %v", err)
	}

	customExprs := []string{
		"DOMAIN-SUFFIX,google.com",
		"DOMAIN-KEYWORD,github",
		"MATCH",
	}
	for i, expr := range customExprs {
		_, err := appServices.policyService.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
			RevisionID:    activeRev.ID,
			BuiltinTarget: "DIRECT",
			Expression:    expr,
			Position:      i,
			RequestID:     "req-custom",
			ActorKind:     domain.ActorKindAdmin,
		})
		if err != nil {
			t.Fatalf("CreatePolicyRule for %s failed: %v", expr, err)
		}
	}

	if count := countPolicyRules(t, db, activeRev.ID); count != 3 {
		t.Fatalf("expected 3 custom rules, got %d", count)
	}

	// Restart
	_, err = makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("restarted makeApplicationServices failed: %v", err)
	}

	// Verify count is still 3 and expressions match
	rules, err := appServices.policyRepo.ListPolicyRules(ctx, activeRev.ID)
	if err != nil {
		t.Fatalf("ListPolicyRules failed: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules after restart, got %d", len(rules))
	}
	for i, expr := range customExprs {
		if rules[i].Expression != expr {
			t.Fatalf("rule %d mismatch: got %s, want %s", i, rules[i].Expression, expr)
		}
	}
}

// TestPolicyRuleLifecycle_ExistingDraftWithoutActive_DoesNotAutoBootstrap verifies that
// if the DB already has draft revisions but no active revision, startup does not auto-activate
// or seed rules into the draft.
func TestPolicyRuleLifecycle_ExistingDraftWithoutActive_DoesNotAutoBootstrap(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	draftID := domain.MustNewUUIDv7()
	_, err := db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, state, content_digest, created_at)
		VALUES (?, 'draft', 'digest-draft-only', CURRENT_TIMESTAMP);
	`, draftID)
	if err != nil {
		t.Fatalf("insert draft revision failed: %v", err)
	}

	// Startup with existing draft
	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}

	// Active revision must still be absent
	activeRev, err := appServices.revisionRepo.GetActive(ctx)
	if err == nil && activeRev != nil {
		t.Fatalf("expected no active revision when only drafts exist, got: %+v", activeRev)
	}

	// Total revisions must remain 1 (no new revision created)
	if total := countAllRevisions(t, db); total != 1 {
		t.Fatalf("expected total revisions to remain 1, got %d", total)
	}

	// Draft must have 0 rules seeded
	if count := countPolicyRules(t, db, draftID); count != 0 {
		t.Fatalf("expected 0 rules in draft revision, got %d", count)
	}
}

// TestPolicyRuleLifecycle_BootstrapFailure_AtomicRollback verifies that if rule seeding
// fails during initial bootstrap, the revision and rules roll back atomically.
func TestPolicyRuleLifecycle_BootstrapFailure_AtomicRollback(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	// Inject a failure trigger on policy_rules insert
	_, err := db.ExecContext(ctx, `
		CREATE TRIGGER fail_seed_insert BEFORE INSERT ON policy_rules
		BEGIN
			SELECT RAISE(ABORT, 'forced seed failure for rollback test');
		END;
	`)
	if err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	// Attempt startup: must fail
	_, err = makeApplicationServices(ctx, db, appServiceOptions{})
	if err == nil || !strings.Contains(err.Error(), "forced seed failure for rollback test") {
		t.Fatalf("expected failure from seed trigger, got: %v", err)
	}

	// Verify complete rollback: no revisions, no snapshots, no rules
	if total := countAllRevisions(t, db); total != 0 {
		t.Fatalf("failed bootstrap left stranded revision: count=%d", total)
	}

	var rulesCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_rules;").Scan(&rulesCount); err != nil || rulesCount != 0 {
		t.Fatalf("failed bootstrap left rules: count=%d (err: %v)", rulesCount, rulesCount)
	}

	var snapshotsCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_revision_snapshots;").Scan(&snapshotsCount); err != nil || snapshotsCount != 0 {
		t.Fatalf("failed bootstrap left snapshots: count=%d (err: %v)", snapshotsCount, snapshotsCount)
	}

	// Drop trigger and retry startup: must succeed cleanly
	if _, err := db.ExecContext(ctx, "DROP TRIGGER fail_seed_insert;"); err != nil {
		t.Fatalf("drop trigger failed: %v", err)
	}

	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("retry makeApplicationServices failed: %v", err)
	}

	activeRev, err := appServices.revisionRepo.GetActive(ctx)
	if err != nil || activeRev == nil {
		t.Fatalf("expected active revision on retry: %v", err)
	}
	if count := countPolicyRules(t, db, activeRev.ID); count != 470 {
		t.Fatalf("expected 470 rules on successful retry, got %d", count)
	}
}

// TestPolicyRuleLifecycle_ExplicitBackfillAPI_Preserved verifies that the explicit
// BackfillRules API remains available and correctly restores 470 rules on demand.
func TestPolicyRuleLifecycle_ExplicitBackfillAPI_Preserved(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}
	activeRev, err := appServices.revisionRepo.GetActive(ctx)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}

	// Clear all rules
	if _, err := db.ExecContext(ctx, "DELETE FROM policy_rules WHERE revision_id = ?;", activeRev.ID); err != nil {
		t.Fatalf("clearing rules failed: %v", err)
	}
	if count := countPolicyRules(t, db, activeRev.ID); count != 0 {
		t.Fatalf("expected 0 rules, got %d", count)
	}

	// Explicitly invoke BackfillRules API
	backfilled, err := appServices.policyService.BackfillRules(ctx, activeRev.ID)
	if err != nil {
		t.Fatalf("explicit BackfillRules failed: %v", err)
	}
	if backfilled != 470 {
		t.Fatalf("expected 470 backfilled rules, got %d", backfilled)
	}

	if count := countPolicyRules(t, db, activeRev.ID); count != 470 {
		t.Fatalf("expected 470 rules after explicit backfill, got %d", count)
	}
}
