package sqlite_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestProxyChainRepository_EmptyGet(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	repo := sqlite.NewProxyChainRepository(db)

	cfg, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("unexpected Get error on empty table: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil ProxyChainConfig")
	}
	if len(cfg.Bindings) != 0 {
		t.Fatalf("expected 0 bindings, got %d", len(cfg.Bindings))
	}
	emptyVer := domain.ProxyChainConfigVersion(nil)
	if cfg.Version != emptyVer {
		t.Fatalf("expected empty version %s, got %s", emptyVer, cfg.Version)
	}
}

func TestProxyChainRepository_LifecycleAndCAS(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	repo := sqlite.NewProxyChainRepository(db)

	initialCfg, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("initial Get failed: %v", err)
	}

	// 1. Wrong expected version rejected
	_, err = repo.Replace(ctx, "wrong-version", []domain.ProxyChainBinding{
		{
			ID:         "b1",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-2",
			Enabled:    true,
			SortOrder:  10,
			Note:       "test chain",
			Revision:   1,
		},
	})
	if err == nil {
		t.Fatal("expected conflict error with wrong expected version")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Category != domain.CategoryConflict {
		t.Fatalf("expected conflict domain error, got: %v", err)
	}

	// 2. Correct expected version succeeds
	b1 := domain.ProxyChainBinding{
		ID:         "b1",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   "node-1",
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   "node-2",
		Enabled:    true,
		SortOrder:  20,
		Note:       "chain 1",
		Revision:   1,
	}
	b2 := domain.ProxyChainBinding{
		ID:         "b2",
		TargetType: domain.ProxyChainTargetNodeGroup,
		TargetID:   "group-1",
		DialerType: domain.ProxyChainDialerNodeGroup,
		DialerID:   "group-2",
		Enabled:    true,
		SortOrder:  10,
		Note:       "chain 2",
		Revision:   1,
	}

	updatedCfg, err := repo.Replace(ctx, initialCfg.Version, []domain.ProxyChainBinding{b1, b2})
	if err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	if len(updatedCfg.Bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(updatedCfg.Bindings))
	}
	// b2 has sort_order 10, b1 has sort_order 20 -> b2 must come first
	if updatedCfg.Bindings[0].ID != "b2" || updatedCfg.Bindings[1].ID != "b1" {
		t.Fatalf("expected b2 then b1, got %s then %s", updatedCfg.Bindings[0].ID, updatedCfg.Bindings[1].ID)
	}

	// 3. Read back confirms persistence
	readBack, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("readBack Get failed: %v", err)
	}
	if readBack.Version != updatedCfg.Version {
		t.Fatalf("expected version %s, got %s", updatedCfg.Version, readBack.Version)
	}
	if len(readBack.Bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(readBack.Bindings))
	}

	// 4. Replacing with empty list clears table
	clearedCfg, err := repo.Replace(ctx, updatedCfg.Version, []domain.ProxyChainBinding{})
	if err != nil {
		t.Fatalf("replace with empty failed: %v", err)
	}
	if len(clearedCfg.Bindings) != 0 {
		t.Fatalf("expected 0 bindings, got %d", len(clearedCfg.Bindings))
	}
	if clearedCfg.Version != initialCfg.Version {
		t.Fatalf("expected empty version %s, got %s", initialCfg.Version, clearedCfg.Version)
	}
}

func TestProxyChainRepository_ValidationFailures(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	repo := sqlite.NewProxyChainRepository(db)

	cfg, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get config: %v", err)
	}

	// 1. Blank expectedVersion rejected (missing / whitespace)
	_, err = repo.Replace(ctx, "", []domain.ProxyChainBinding{
		{
			ID:         "b-valid",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-A",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-B",
			Enabled:    true,
			SortOrder:  1,
			Revision:   1,
		},
	})
	if err == nil {
		t.Fatal("expected validation error for empty expectedVersion")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Category != domain.CategoryValidation {
		t.Fatalf("expected validation category for empty expectedVersion, got: %v", err)
	}

	_, err = repo.Replace(ctx, "   ", []domain.ProxyChainBinding{
		{
			ID:         "b-valid",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-A",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-B",
			Enabled:    true,
			SortOrder:  1,
			Revision:   1,
		},
	})
	if err == nil {
		t.Fatal("expected validation error for whitespace expectedVersion")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Category != domain.CategoryValidation {
		t.Fatalf("expected validation category for whitespace expectedVersion, got: %v", err)
	}

	// 2. Self-referencing node cycle
	_, err = repo.Replace(ctx, cfg.Version, []domain.ProxyChainBinding{
		{
			ID:         "cycle1",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-A",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-A",
			Enabled:    true,
			SortOrder:  1,
			Revision:   1,
		},
	})
	if err == nil {
		t.Fatal("expected cycle validation failure for self-referencing node")
	}

	// 3. Duplicate ID
	_, err = repo.Replace(ctx, cfg.Version, []domain.ProxyChainBinding{
		{
			ID:         "dup-id",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-A",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-B",
			Enabled:    true,
			SortOrder:  1,
			Revision:   1,
		},
		{
			ID:         "dup-id",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-C",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-D",
			Enabled:    true,
			SortOrder:  2,
			Revision:   1,
		},
	})
	if err == nil {
		t.Fatal("expected validation failure for duplicate binding ID")
	}
}
