package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestGlobalEmptyPassStartupUpgradesSealedSchema30(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "startup.db")
	cfg := sqlite.DefaultConfig(path)

	// Create only an isolated scratch database. Reaching version 30 in an
	// otherwise empty DB causes the runner to install its empty-storage seal.
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("open scratch database: %v", err)
	}
	if err := sqlite.NewMigrationRunner(db, migrations.FS).RunUpTo(ctx, 30); err != nil {
		_ = db.Close()
		t.Fatalf("prepare sealed schema 30: %v", err)
	}
	if err := sqlite.CheckSubscriptionStorageReady(ctx, db); err != nil {
		_ = db.Close()
		t.Fatalf("schema 30 fixture is not sealed and ready: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close schema 30 fixture: %v", err)
	}

	assertStartupSchemaAndDefault := func(db *sql.DB, wantVersion int) {
		t.Helper()
		if err := sqlite.CheckSubscriptionStorageReady(ctx, db); err != nil {
			t.Fatalf("subscription storage not ready at schema %d: %v", wantVersion, err)
		}
		var version int
		if err := db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
			t.Fatalf("read schema version: %v", err)
		}
		if version != wantVersion {
			t.Fatalf("schema version = %d, want %d", version, wantVersion)
		}
		filter, err := sqlite.NewNodeFilterRepository(db).GetGlobalFilter(ctx)
		if err != nil {
			t.Fatalf("read default global node filter: %v", err)
		}
		if filter.EmptyFallbackPass == nil || !*filter.EmptyFallbackPass {
			t.Fatalf("global empty-group PASS default = %v, want true", filter.EmptyFallbackPass)
		}
	}

	upgraded, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("startup upgrade from sealed schema 30: %v", err)
	}
	assertStartupSchemaAndDefault(upgraded, 31)
	if err := upgraded.Close(); err != nil {
		t.Fatalf("close upgraded database: %v", err)
	}

	reopened, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("startup reopen at schema 31: %v", err)
	}
	assertStartupSchemaAndDefault(reopened, 31)
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened database: %v", err)
	}
}
