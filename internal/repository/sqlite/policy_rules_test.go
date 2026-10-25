package sqlite_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestPolicyRules_CategoryCRUDAndBackfill(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:test_rules_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}

	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	repo := sqlite.NewPolicyRepository(db)

	// 1. Verify 14 standard categories were preset by migration 25
	cats, err := repo.ListRuleCategories(ctx)
	if err != nil {
		t.Fatalf("ListRuleCategories failed: %v", err)
	}
	if len(cats) != 14 {
		t.Fatalf("expected 14 preset categories, got %d", len(cats))
	}
	if cats[0].Name != "广告/拦截" || cats[0].SortOrder != 0 {
		t.Fatalf("unexpected first category: %+v", cats[0])
	}
	if cats[13].Name != "兜底" || cats[13].SortOrder != 130 {
		t.Fatalf("unexpected last category: %+v", cats[13])
	}

	// 2. Test Category CRUD
	newCat := domain.RuleCategory{
		ID:        "cat-custom",
		Name:      "自定义分类",
		SortOrder: 200,
	}
	if err := repo.CreateRuleCategory(ctx, &newCat); err != nil {
		t.Fatalf("CreateRuleCategory failed: %v", err)
	}

	gotCat, err := repo.GetRuleCategory(ctx, "cat-custom")
	if err != nil {
		t.Fatalf("GetRuleCategory failed: %v", err)
	}
	if gotCat.Name != "自定义分类" {
		t.Fatalf("expected 自定义分类, got %s", gotCat.Name)
	}

	gotCat.Name = "自定义分类修改"
	if err := repo.UpdateRuleCategory(ctx, gotCat); err != nil {
		t.Fatalf("UpdateRuleCategory failed: %v", err)
	}

	// Test Reorder
	if err := repo.ReorderRuleCategories(ctx, []string{"cat-01", "cat-02", "cat-03", "cat-04", "cat-05", "cat-06", "cat-07", "cat-08", "cat-09", "cat-10", "cat-11", "cat-12", "cat-13", "cat-14"}); err != nil {
		t.Fatalf("ReorderRuleCategories failed: %v", err)
	}

	// Clean up custom category
	if err := repo.DeleteRuleCategory(ctx, "cat-custom"); err != nil {
		t.Fatalf("DeleteRuleCategory failed: %v", err)
	}

	// 3. Test Backfill of all 470 rules
	revID := domain.MustNewUUIDv7()
	// Insert revision row
	_, err = db.ExecContext(ctx, `INSERT INTO configuration_revisions (id, state, content_digest, created_at) VALUES (?, 'active', 'digest', CURRENT_TIMESTAMP);`, revID)
	if err != nil {
		t.Fatalf("failed to insert revision: %v", err)
	}

	count, err := sqlite.BackfillPolicyRules(ctx, db, revID, "/data/clash_sub_parser.db")
	if err != nil {
		t.Fatalf("BackfillPolicyRules failed: %v", err)
	}
	if count != 470 {
		t.Fatalf("expected 470 backfilled rules, got %d", count)
	}

	// Query rules
	rules, err := repo.ListPolicyRules(ctx, revID)
	if err != nil {
		t.Fatalf("ListPolicyRules failed: %v", err)
	}
	if len(rules) != 470 {
		t.Fatalf("expected 470 listed rules, got %d", len(rules))
	}

	// Terminal MATCH rule must be absolute bottom
	lastRule := rules[len(rules)-1]
	if !domain.IsMatchRule(lastRule.Expression) {
		t.Fatalf("expected last rule to be MATCH, got %s (category: %s)", lastRule.Expression, lastRule.Category)
	}
	if lastRule.Category != "兜底" {
		t.Fatalf("expected MATCH rule to be in 兜底 category, got %s", lastRule.Category)
	}

	// 4. Test Embedded Seed Fallback (when external DB doesn't exist)
	revID2 := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `INSERT INTO configuration_revisions (id, state, content_digest, created_at) VALUES (?, 'active', 'digest2', CURRENT_TIMESTAMP);`, revID2)
	if err != nil {
		t.Fatalf("failed to insert revision 2: %v", err)
	}

	count2, err := sqlite.BackfillPolicyRules(ctx, db, revID2, "/nonexistent/path/db.sqlite")
	if err != nil {
		t.Fatalf("BackfillPolicyRules with fallback failed: %v", err)
	}
	if count2 != 470 {
		t.Fatalf("expected 470 backfilled rules from embedded seed, got %d", count2)
	}

	rules2, err := repo.ListPolicyRules(ctx, revID2)
	if err != nil || len(rules2) != 470 {
		t.Fatalf("expected 470 listed rules from embedded seed, got %d (err: %v)", len(rules2), err)
	}
}

func TestRevisionRepository_CreateActiveWithSeed_AtomicRollback(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	revRepo := sqlite.NewRevisionRepository(db)

	seeder, ok := revRepo.(interface {
		CreateActiveWithSeed(ctx context.Context, rev *domain.ConfigurationRevision) error
	})
	if !ok {
		t.Fatalf("expected revision repository to implement CreateActiveWithSeed")
	}

	// 1. Success path: inserts revision, snapshot, active state and 470 rules
	revID := domain.MustNewUUIDv7()
	rev := domain.ConfigurationRevision{
		ID:            revID,
		ContentDigest: "digest-initial-seed",
		State:         domain.RevisionStateDraft,
		CreatedAt:     domain.NowUTC(),
	}

	if err := seeder.CreateActiveWithSeed(ctx, &rev); err != nil {
		t.Fatalf("CreateActiveWithSeed failed: %v", err)
	}

	active, err := revRepo.GetActive(ctx)
	if err != nil || active == nil || active.ID != revID {
		t.Fatalf("expected active revision %s, got %+v (err: %v)", revID, active, err)
	}

	policyRepo := sqlite.NewPolicyRepository(db)
	rules, err := policyRepo.ListPolicyRules(ctx, revID)
	if err != nil || len(rules) != 470 {
		t.Fatalf("expected 470 seeded rules, got %d (err: %v)", len(rules), err)
	}

	// 2. Failure path: trigger on policy_rules aborts -> entire transaction rolls back
	db2, _ := setupTestDB(t)
	revRepo2 := sqlite.NewRevisionRepository(db2)
	seeder2 := revRepo2.(interface {
		CreateActiveWithSeed(ctx context.Context, rev *domain.ConfigurationRevision) error
	})

	_, err = db2.ExecContext(ctx, `
		CREATE TRIGGER abort_seed BEFORE INSERT ON policy_rules
		BEGIN
			SELECT RAISE(ABORT, 'abort initial seed');
		END;
	`)
	if err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	failRevID := domain.MustNewUUIDv7()
	failRev := domain.ConfigurationRevision{
		ID:            failRevID,
		ContentDigest: "digest-fail-seed",
		State:         domain.RevisionStateDraft,
		CreatedAt:     domain.NowUTC(),
	}

	if err := seeder2.CreateActiveWithSeed(ctx, &failRev); err == nil || !strings.Contains(err.Error(), "abort initial seed") {
		t.Fatalf("expected abort initial seed error, got: %v", err)
	}

	var count int
	if err := db2.QueryRowContext(ctx, "SELECT COUNT(*) FROM configuration_revisions;").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed seed must roll back revision: count=%d", count)
	}
	if err := db2.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_rules;").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed seed must roll back rules: count=%d", count)
	}
	if err := db2.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_revision_snapshots;").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed seed must roll back snapshots: count=%d", count)
	}
}
