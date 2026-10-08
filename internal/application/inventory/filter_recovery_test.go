package inventory_test

import (
	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var cheapArchiveRegex = []string{`去掉(流媒体|x(?:[0-5](?:\.[0-9]+)?)(?![\d.])|便宜|free )`, "Eeox", "einck"}

func TestTranslateLegacyRegexRules(t *testing.T) {
	for _, rules := range [][]string{{"加拿大|CA|Canada"}, {"美西|美国|US", "美西"}, {`^(?!.*(自动|故障|官网|套餐|机场|订阅)).*$`}, cheapArchiveRegex} {
		cat, spec, err := inventory.TranslateLegacyRegexRulesForTest(rules)
		if err != nil || cat != "positive_regex" || len(spec.Conditions) != 1 || spec.Conditions[0].Op != domain.FilterOpRegex {
			t.Fatalf("translation: %s %+v %v", cat, spec, err)
		}
		if len(rules) == 1 && spec.Conditions[0].Value != rules[0] {
			t.Fatal("original assertion was rewritten")
		}
	}
}

func TestRunRestoreGroupFilters_DryRunAndApply(t *testing.T) {
	// Self-contained archive and target; no machine database or skipped recovery.
	ctx := context.Background()
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "archive.db")
	archive, err := sql.Open("sqlite", archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Exec(`CREATE TABLE node_groups(id INTEGER PRIMARY KEY,name TEXT,kind TEXT,group_type TEXT,regex_rules TEXT)`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cheapArchiveRegex)
	if _, err := archive.Exec(`INSERT INTO node_groups VALUES (2,'便宜','custom','select',?)`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(sqlite.DefaultConfig(filepath.Join(dir, "target.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewPolicyRepository(db)
	gid := inventory.CanonicalLegacyGroupMap[2]
	if err := repo.CreateGroup(ctx, &domain.NodeGroup{ID: gid, Name: "便宜", GroupType: domain.GroupTypeSelect}); err != nil {
		t.Fatal(err)
	}
	rev := domain.ConfigurationRevision{ID: domain.MustNewUUIDv7(), ContentDigest: "fixture", State: domain.RevisionStateDraft, CreatedAt: domain.NowUTC()}
	if err := sqlite.NewRevisionRepository(db).CreateActive(ctx, &rev); err != nil {
		t.Fatal(err)
	}
	opts := inventory.RestoreGroupFiltersOptions{ArchiveDBPath: archivePath, DryRun: true}
	dry, err := inventory.RunRestoreGroupFilters(ctx, db, opts)
	if err != nil || dry.RecoveredCount != 1 || dry.UnsupportedCount != 0 {
		t.Fatalf("dry: %+v %v", dry, err)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM group_node_filters`).Scan(&n)
	if n != 0 {
		t.Fatal("dry wrote filters")
	}
	opts.DryRun = false
	opts.ConfirmBackup = true
	applied, err := inventory.RunRestoreGroupFilters(ctx, db, opts)
	if err != nil || applied.RecoveredCount != 1 || applied.NewRevisionID == "" {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	stored, err := sqlite.NewNodeFilterRepository(db).GetGroupFilter(ctx, gid)
	if err != nil {
		t.Fatal(err)
	}
	pattern := stored.Spec.Conditions[0].Value
	if !strings.Contains(pattern, `(?![\d.])`) {
		t.Fatal("lookahead removed")
	}
	for _, tc := range []struct {
		name  string
		match bool
	}{{"去掉x5", true}, {"去掉x5.9", true}, {"去掉x50", false}, {"去掉x5.9.1", false}, {"x5", false}, {"EINCK 美国", true}, {"Eeox", true}, {"中文普通节点", false}} {
		got, _ := domain.MatchesFilter(&stored.Spec, domain.Node{DisplayName: tc.name}, nil, nil, time.Time{})
		if got != tc.match {
			t.Fatalf("%q: got %v", tc.name, got)
		}
	}
	second, err := inventory.RunRestoreGroupFilters(ctx, db, opts)
	if err != nil || second.RecoveredCount != 0 || second.SkippedCount != 1 || second.NewRevisionID != "" {
		t.Fatalf("idempotence: %+v %v", second, err)
	}
	custom := domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpNotRegex, Value: "user-owned"}}}
	if err := sqlite.NewNodeFilterRepository(db).SetGroupFilter(ctx, &domain.GroupNodeFilter{GroupID: gid, Spec: custom}); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.RunRestoreGroupFilters(ctx, db, opts); err != nil {
		t.Fatal(err)
	}
	kept, _ := sqlite.NewNodeFilterRepository(db).GetGroupFilter(ctx, gid)
	if kept.Spec.Conditions[0].Value != "user-owned" {
		t.Fatal("overwrote user filter")
	}
	group, _ := repo.GetGroupByID(ctx, gid)
	group.Name = "different identity"
	_ = repo.UpdateGroup(ctx, group)
	guarded, err := inventory.RunRestoreGroupFilters(ctx, db, opts)
	if err != nil || guarded.GuardErrors != 1 || guarded.RecoveredCount != 0 {
		t.Fatalf("identity guard: %+v %v", guarded, err)
	}
}
