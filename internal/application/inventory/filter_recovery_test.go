package inventory_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
)

func TestTranslateLegacyRegexRules(t *testing.T) {
	// 1. Positive single regex
	t.Run("CanadaPositiveRegex", func(t *testing.T) {
		rules := []string{"加拿大|CA|Canada"}
		cat, spec, err := inventory.TranslateLegacyRegexRulesForTest(rules)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cat != "positive_regex" {
			t.Fatalf("expected positive_regex, got %s", cat)
		}
		if len(spec.Conditions) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(spec.Conditions))
		}
		if spec.Conditions[0].Op != domain.FilterOpRegex {
			t.Fatalf("expected op regex, got %s", spec.Conditions[0].Op)
		}
	})

	// 2. Positive composite regex
	t.Run("USCompositeRegex", func(t *testing.T) {
		rules := []string{"美西|美国|US", "美西"}
		cat, spec, err := inventory.TranslateLegacyRegexRulesForTest(rules)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cat != "positive_regex" {
			t.Fatalf("expected positive_regex, got %s", cat)
		}
		if spec.Conditions[0].Value != "(?:美西|美国|US)|(?:美西)" {
			t.Fatalf("unexpected combined value: %s", spec.Conditions[0].Value)
		}
	})

	// 3. Negation regex
	t.Run("AutoSelectNegationRegex", func(t *testing.T) {
		rules := []string{"^(?!.*(自动|故障|官网|套餐|机场|订阅)).*$"}
		cat, spec, err := inventory.TranslateLegacyRegexRulesForTest(rules)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cat != "negation_regex" {
			t.Fatalf("expected negation_regex, got %s", cat)
		}
		if spec.Conditions[0].Op != domain.FilterOpNotRegex {
			t.Fatalf("expected op not_regex, got %s", spec.Conditions[0].Op)
		}
		if spec.Conditions[0].Value != "自动|故障|官网|套餐|机场|订阅" {
			t.Fatalf("unexpected value: %s", spec.Conditions[0].Value)
		}
	})
}

func TestRunRestoreGroupFilters_DryRunAndApply(t *testing.T) {
	archivePath := "/home/service/backups/csp-legacy-cold-archive-20260919.db"
	if _, err := os.Stat(archivePath); err != nil {
		t.Skip("legacy cold archive not found, skipping integration test")
	}

	sourceDBPath := "/tmp/csp_fresh_preview/csp-shadow.db"
	if _, err := os.Stat(sourceDBPath); err != nil {
		t.Skip("shadow db not found, skipping integration test")
	}

	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test_restore.db")

	// Copy shadow DB to temporary location
	inputBytes, err := os.ReadFile(sourceDBPath)
	if err != nil {
		t.Fatalf("failed to read shadow db: %v", err)
	}
	if err := os.WriteFile(testDBPath, inputBytes, 0o600); err != nil {
		t.Fatalf("failed to write test db copy: %v", err)
	}

	ctx := context.Background()
	db, err := sql.Open("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	// Ensure hermetic test starting state: testDB has clean filter state
	_, _ = db.ExecContext(ctx, "DELETE FROM group_node_filters;")

	// 1. Dry run
	dryReport, err := inventory.RunRestoreGroupFilters(ctx, db, inventory.RestoreGroupFiltersOptions{
		TargetDBPath:  testDBPath,
		ArchiveDBPath: archivePath,
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if dryReport.TotalGroups != 29 {
		t.Fatalf("expected 29 total groups, got %d", dryReport.TotalGroups)
	}
	if dryReport.RecoveredCount != 15 {
		for _, g := range dryReport.Groups {
			t.Logf("Group %d %s: status=%s, reason=%s", g.LegacyID, g.TargetGroupName, g.Status, g.Reason)
		}
		t.Fatalf("expected 15 recovered groups with regex filters, got %d", dryReport.RecoveredCount)
	}
	if dryReport.RecoveredCount != 15 {
		t.Fatalf("expected 15 recovered groups with regex filters, got %d", dryReport.RecoveredCount)
	}
	if dryReport.ManualCount != 13 {
		t.Fatalf("expected 13 manual groups without regex filters, got %d", dryReport.ManualCount)
	}
	if dryReport.UnsupportedCount != 1 {
		t.Fatalf("expected 1 unsupported regex group, got %d", dryReport.UnsupportedCount)
	}
	if dryReport.GuardErrors != 0 {
		t.Fatalf("expected 0 guard errors, got %d", dryReport.GuardErrors)
	}

	// Verify simulated match on Canada in dry run: should be 3 nodes!
	for _, g := range dryReport.Groups {
		if g.TargetGroupName == "加拿大" {
			if g.MatchedNodeCount != 3 {
				t.Fatalf("expected Canada to match 3 nodes, got %d", g.MatchedNodeCount)
			}
		}
	}

	// Verify DB is unchanged in dry-run
	var countBefore int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM group_node_filters;").Scan(&countBefore)
	if countBefore != 0 {
		t.Fatalf("expected 0 filters in db after dry-run, got %d", countBefore)
	}

	// 2. Apply mode
	applyReport, err := inventory.RunRestoreGroupFilters(ctx, db, inventory.RestoreGroupFiltersOptions{
		TargetDBPath:  testDBPath,
		ArchiveDBPath: archivePath,
		DryRun:        false,
		ConfirmBackup: true,
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if applyReport.RecoveredCount != 15 {
		t.Fatalf("expected 15 recovered filters in apply mode, got %d", applyReport.RecoveredCount)
	}

	var countAfter int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM group_node_filters;").Scan(&countAfter)
	if countAfter != 15 {
		t.Fatalf("expected 15 filters in db after apply, got %d", countAfter)
	}

	// 3. Idempotent re-run: should skip all 15 existing filters!
	secondReport, err := inventory.RunRestoreGroupFilters(ctx, db, inventory.RestoreGroupFiltersOptions{
		TargetDBPath:  testDBPath,
		ArchiveDBPath: archivePath,
		DryRun:        false,
		ConfirmBackup: true,
	})
	if err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	if secondReport.RecoveredCount != 0 {
		t.Fatalf("expected 0 recovered on second run (idempotent), got %d", secondReport.RecoveredCount)
	}
	if secondReport.SkippedCount != 15 {
		t.Fatalf("expected 15 skipped existing filters on second run, got %d", secondReport.SkippedCount)
	}
}
