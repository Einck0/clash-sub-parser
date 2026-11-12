package sqlite_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestGlobalEmptyPassPersistsAndFreshStorageIsReady(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewNodeFilterRepository(db)

	initial, err := repo.GetGlobalFilter(ctx)
	if err != nil {
		t.Fatalf("read initial global filter: %v", err)
	}
	if initial.EmptyFallbackPass == nil || !*initial.EmptyFallbackPass {
		t.Fatalf("fresh global empty-group PASS default = %v, want true", initial.EmptyFallbackPass)
	}

	disabled := false
	if err := repo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
		Spec:              domain.NodeFilterSpec{Conditions: []domain.FilterCondition{}},
		EmptyFallbackPass: &disabled,
	}); err != nil {
		t.Fatalf("disable global empty-group PASS: %v", err)
	}
	loaded, err := repo.GetGlobalFilter(ctx)
	if err != nil {
		t.Fatalf("read disabled global empty-group PASS: %v", err)
	}
	if loaded.EmptyFallbackPass == nil || *loaded.EmptyFallbackPass {
		t.Fatalf("global empty-group PASS after disabling = %v, want false", loaded.EmptyFallbackPass)
	}

	if err := repo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
		Spec: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{}},
	}); err != nil {
		t.Fatalf("update filter while preserving global empty-group PASS: %v", err)
	}
	loaded, err = repo.GetGlobalFilter(ctx)
	if err != nil {
		t.Fatalf("read global filter after nil setting update: %v", err)
	}
	if loaded.EmptyFallbackPass == nil || *loaded.EmptyFallbackPass {
		t.Fatalf("nil setting update changed global empty-group PASS to %v, want false", loaded.EmptyFallbackPass)
	}

	if err := sqlite.CheckSubscriptionStorageReady(ctx, db); err != nil {
		t.Fatalf("fresh migrated database must satisfy subscription storage readiness: %v", err)
	}
}

func TestGlobalEmptyPassMigrationUpgradesLegacySettings(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.RunUpTo(ctx, 30); err != nil {
		t.Fatalf("apply migrations through schema 30: %v", err)
	}

	const legacyUpdatedAt = "2026-01-02T03:04:05Z"
	if _, err := db.ExecContext(ctx,
		`UPDATE global_node_filters SET filter_spec='{}', updated_at=? WHERE id=1`, legacyUpdatedAt); err != nil {
		t.Fatalf("update legacy global filter row: %v", err)
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	repo := sqlite.NewNodeFilterRepository(db)
	loaded, err := repo.GetGlobalFilter(ctx)
	if err != nil {
		t.Fatalf("read global filter after upgrade: %v", err)
	}
	if loaded.EmptyFallbackPass == nil || !*loaded.EmptyFallbackPass {
		t.Fatalf("upgraded legacy empty-group PASS default = %v, want true", loaded.EmptyFallbackPass)
	}
	if got := loaded.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"); got != legacyUpdatedAt {
		t.Fatalf("legacy global filter row was not preserved: updated_at = %q, want %q", got, legacyUpdatedAt)
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("rerun migrations after upgrade: %v", err)
	}
	loaded, err = repo.GetGlobalFilter(ctx)
	if err != nil {
		t.Fatalf("read global filter after idempotent rerun: %v", err)
	}
	if loaded.EmptyFallbackPass == nil || !*loaded.EmptyFallbackPass {
		t.Fatalf("empty-group PASS after idempotent rerun = %v, want true", loaded.EmptyFallbackPass)
	}
}
