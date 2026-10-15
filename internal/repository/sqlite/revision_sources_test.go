package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestRevisionSourcesSnapshotRollbackAndConflict(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewPolicyRepository(db)
	revs := sqlite.NewRevisionRepository(db)
	svc := revision.NewService(revs, nil, revision.WithPolicyRepository(repo))
	child := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: "child", GroupType: domain.GroupTypeSelect}
	builtin := "DIRECT"
	pattern := "HK"
	if _, err := repo.SaveGroupWithSources(ctx, child, 0, []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, BuiltinName: &builtin}}); err != nil {
		t.Fatal(err)
	}
	parent := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: "parent", GroupType: domain.GroupTypeSelect}
	original := []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindGroupNodes, TargetGroupID: &child.ID}, {Operation: domain.OperationAdd, Kind: domain.SourceKindRegex, Pattern: &pattern, Position: 1}, {Operation: domain.OperationRemove, Kind: domain.SourceKindRegex, Pattern: &pattern, Position: 2}}
	saved, err := repo.SaveGroupWithSources(ctx, parent, 0, original)
	if err != nil {
		t.Fatal(err)
	}
	rev, err := svc.Create(ctx, revision.CreateCommand{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := revs.(domain.RevisionSnapshotRepository).GetPolicySnapshot(ctx, rev.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(snapshot)
	changed, err := repo.SaveGroupWithSources(ctx, saved, saved.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Activate(ctx, rev.ID, revision.Action{}); err == nil {
		t.Fatal("activated stale draft")
	}
	expected := map[string]int64{child.ID: 1, parent.ID: saved.Revision}
	if err := svc.Rollback(ctx, rev.ID, expected, revision.Action{}); err == nil {
		t.Fatal("rollback accepted stale revision vector")
	}
	current, err := repo.GetGroupByID(ctx, parent.ID)
	if err != nil || len(current.Sources) != 0 || current.Revision != changed.Revision {
		t.Fatalf("conflict mutated graph: %+v %v", current, err)
	}
	expected[parent.ID] = changed.Revision
	if err := svc.Rollback(ctx, rev.ID, expected, revision.Action{}); err != nil {
		t.Fatal(err)
	}
	restored, err := repo.GetGroupByID(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Sources) != 3 || restored.Sources[0].Kind != domain.SourceKindGroupNodes || restored.Sources[2].Operation != domain.OperationRemove || restored.Sources[1].ID != saved.Sources[1].ID || restored.Revision <= changed.Revision {
		t.Fatalf("lossy restore: %+v", restored)
	}
	afterSnap, err := revs.(domain.RevisionSnapshotRepository).GetPolicySnapshot(ctx, rev.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(afterSnap)
	if string(before) != string(after) {
		t.Fatal("rollback rewrote immutable snapshot")
	}
	groups, _ := repo.ListGroups(ctx)
	edges := map[string][]domain.GroupEdge{}
	for _, g := range groups {
		edges[g.ID] = domain.ProjectSourceEdges(g.Sources)
	}
	digest, err := policy.ComputeConfigurationDigest(groups, edges)
	if err != nil || digest != rev.ContentDigest {
		t.Fatalf("rollback logical digest mismatch %s != %s (%v)", digest, rev.ContentDigest, err)
	}
	if err := repo.SetEdgesForGroup(ctx, parent.ID, nil); err == nil {
		t.Fatal("old endpoint erased rich pipeline")
	} else {
		var de *domain.DomainError
		if !errors.As(err, &de) || de.Code != "sources_edit_required" {
			t.Fatal(err)
		}
	}
	if err := repo.DeleteGroup(ctx, child.ID); err == nil {
		t.Fatal("deleted referenced child")
	}
}

func TestPolicySourcesEmptyAndReadErrorsNeverUseLegacy(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewPolicyRepository(db)
	g := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: "empty", GroupType: domain.GroupTypeSelect}
	child := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: "child", GroupType: domain.GroupTypeSelect}
	if err := repo.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateGroup(ctx, child); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO group_edges(id,parent_group_id,child_group_id,position) VALUES(?,?,?,0)`, domain.MustNewUUIDv7(), g.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	groups, err := repo.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if len(g.Sources) != 0 {
			t.Fatal("empty sources resurrected stale edges")
		}
	}
	if _, err := db.Exec(`DROP TABLE policy_group_sources`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetGroupByID(ctx, g.ID); err == nil {
		t.Fatal("Get swallowed sources error")
	}
	if _, err := repo.ListGroups(ctx); err == nil {
		t.Fatal("List swallowed sources error")
	}
	if _, err := repo.ListAllSources(ctx); err == nil {
		t.Fatal("ListAll swallowed sources error")
	}
}
