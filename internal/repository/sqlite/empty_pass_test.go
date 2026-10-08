package sqlite_test

import (
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
	"context"
	"path/filepath"
	"testing"
)

func TestEmptyPassOldMigrationAndAtomicRollback(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(sqlite.DefaultConfig(filepath.Join(t.TempDir(), "old.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.RunUpTo(ctx, 16); err != nil {
		t.Fatal(err)
	}
	gid := domain.MustNewUUIDv7()
	if _, err := db.ExecContext(ctx, `INSERT INTO node_groups (id,name,group_type,created_at,updated_at) VALUES (?,'old','select','2026-01-01','2026-01-01')`, gid); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewPolicyRepository(db)
	g, err := repo.GetGroupByID(ctx, gid)
	if err != nil || g.EmptyFallbackPass {
		t.Fatalf("migration changed old opt-in: %+v %v", g, err)
	}
	g.EmptyFallbackPass = true
	if err := repo.UpdateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	groups, err := repo.ListGroups(ctx)
	if err != nil || !groups[0].EmptyFallbackPass {
		t.Fatal("list lost flag")
	}
	atomic := repo.(domain.AtomicGroupRepository)
	// A real SQLite trigger injects a storage failure after metadata and edges.
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_filter BEFORE INSERT ON group_node_filters BEGIN SELECT RAISE(ABORT,'fixture failure'); END;`); err != nil {
		t.Fatal(err)
	}
	filter := &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "HK"}}}
	g.Name = "must rollback"
	g.EmptyFallbackPass = false
	edges := []domain.GroupEdge{}
	if err := atomic.SaveGroup(ctx, g, false, &edges, true, filter); err == nil {
		t.Fatal("storage failure was ignored")
	}
	after, err := repo.GetGroupByID(ctx, gid)
	if err != nil || after.Name != "old" || !after.EmptyFallbackPass {
		t.Fatalf("partial metadata write: %+v %v", after, err)
	}
	fresh := *g
	fresh.ID = domain.MustNewUUIDv7()
	if err := atomic.SaveGroup(ctx, &fresh, true, &edges, true, filter); err == nil {
		t.Fatal("create storage failure ignored")
	}
	if _, err := repo.GetGroupByID(ctx, fresh.ID); err == nil {
		t.Fatal("partial create persisted")
	}
	var violations int
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		violations++
	}
	if violations != 0 {
		t.Fatal("migration broke FKs")
	}
}
