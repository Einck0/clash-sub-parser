package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestURLTestMigrationPersistenceFactsAndSnapshotRollback(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.RunUpTo(ctx, 32); err != nil {
		t.Fatal(err)
	}
	id := domain.MustNewUUIDv7()
	if _, err := db.ExecContext(ctx, `INSERT INTO node_groups(id,name,group_type,created_at,updated_at) VALUES(?,'Auto','urltest','2026-01-01','2026-01-01')`, id); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewPolicyRepository(db)
	g, err := repo.GetGroupByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if g.URLTestInterval != nil || g.URLTestTolerance != nil {
		t.Fatalf("migration changed legacy defaults: %+v", g)
	}
	zero := 0
	interval := 120
	g.URLTestInterval = &interval
	g.URLTestTolerance = &zero
	builtin := "DIRECT"
	if _, err = repo.SaveGroupWithSources(ctx, g, g.Revision, []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, BuiltinName: &builtin}}); err != nil {
		t.Fatal(err)
	}
	revs := sqlite.NewRevisionRepository(db)
	svc := revision.NewService(revs, nil, revision.WithPolicyRepository(repo))
	rev, err := svc.Create(ctx, revision.CreateCommand{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := revs.(domain.RevisionSnapshotRepository).GetPolicySnapshot(ctx, rev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != 1 || snapshot[0].URLTestInterval == nil || *snapshot[0].URLTestInterval != 120 || snapshot[0].URLTestTolerance == nil || *snapshot[0].URLTestTolerance != 0 {
		t.Fatalf("snapshot lost settings: %+v", snapshot)
	}
	facts, err := sqlite.NewConsistentFactsLoader(db).LoadConsistentFacts(ctx, rev.ID, domain.TargetMihomo, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Groups) != 1 || facts.Groups[0].URLTestInterval == nil || *facts.Groups[0].URLTestInterval != 120 || facts.Groups[0].URLTestTolerance == nil || *facts.Groups[0].URLTestTolerance != 0 {
		t.Fatalf("publication facts lost settings: %+v", facts.Groups)
	}
	g, err = repo.GetGroupByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ten := 10
	sixty := 60
	g.URLTestInterval, g.URLTestTolerance = &sixty, &ten
	changed, err := repo.SaveGroupWithSources(ctx, g, g.Revision, g.Sources)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Rollback(ctx, rev.ID, map[string]int64{id: changed.Revision}, revision.Action{}); err != nil {
		t.Fatal(err)
	}
	restored, err := repo.GetGroupByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if restored.URLTestInterval == nil || *restored.URLTestInterval != 120 || restored.URLTestTolerance == nil || *restored.URLTestTolerance != 0 {
		t.Fatalf("rollback lost settings: %+v", restored)
	}
}
