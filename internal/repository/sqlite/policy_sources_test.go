package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func setupPolicyTestDB(t *testing.T) (*sqlite.Config, domain.PolicyRepository) {
	t.Helper()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:test_policy_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}

	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations on test db: %v", err)
	}

	repo := sqlite.NewPolicyRepository(db)
	return &cfg, repo
}

func TestPolicyRepository_SaveGroupWithSources_AtomicAndOptimistic(t *testing.T) {
	ctx := context.Background()
	_, repo := setupPolicyTestDB(t)

	groupID := domain.MustNewUUIDv7()
	childGroupID := domain.MustNewUUIDv7()

	// Create child group first
	childGroup := &domain.NodeGroup{
		ID:        childGroupID,
		Name:      "Child Group",
		GroupType: domain.GroupTypeSelect,
	}
	_, err := repo.SaveGroupWithSources(ctx, childGroup, 0, nil)
	if err != nil {
		t.Fatalf("failed to create child group: %v", err)
	}

	nodeID := "node_01a0b9afc11670d1a027000000000001"
	pattern := ".*(HK|香港).*"
	filterSpec := &domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "HK"},
		},
	}
	builtinPass := "PASS"

	sources := []domain.MemberSource{
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupRef,
			TargetGroupID: &childGroupID,
		},
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupNodes,
			TargetGroupID: &childGroupID,
		},
		{
			Operation: domain.OperationRemove,
			Kind:      domain.SourceKindRegex,
			Pattern:   &pattern,
		},
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindManualNode,
			NodeLogicalID: &nodeID,
		},
		{
			Operation:  domain.OperationAdd,
			Kind:       domain.SourceKindFilter,
			FilterSpec: filterSpec,
		},
		{
			Operation:   domain.OperationAdd,
			Kind:        domain.SourceKindTypedBuiltin,
			BuiltinName: &builtinPass,
		},
	}

	parentGroup := &domain.NodeGroup{
		ID:                groupID,
		Name:              "Parent Selector",
		GroupType:         domain.GroupTypeSelect,
		EmptyFallbackPass: true,
	}

	// 1. Initial creation with revision 1
	saved, err := repo.SaveGroupWithSources(ctx, parentGroup, 0, sources)
	if err != nil {
		t.Fatalf("failed to save group with sources: %v", err)
	}
	if saved.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", saved.Revision)
	}
	if len(saved.Sources) != 6 {
		t.Fatalf("expected 6 sources, got %d", len(saved.Sources))
	}

	// 2. Reload and verify byte-for-byte exact ordering
	reloaded, err := repo.GetGroupByID(ctx, groupID)
	if err != nil {
		t.Fatalf("failed to reload group: %v", err)
	}
	if reloaded.Revision != 1 {
		t.Fatalf("reloaded revision expected 1, got %d", reloaded.Revision)
	}
	if len(reloaded.Sources) != 6 {
		t.Fatalf("reloaded sources expected 6, got %d", len(reloaded.Sources))
	}
	for i, s := range reloaded.Sources {
		if s.Position != i {
			t.Fatalf("expected position %d, got %d", i, s.Position)
		}
	}
	if *reloaded.Sources[0].TargetGroupID != childGroupID {
		t.Fatalf("expected target_group_id %s, got %s", childGroupID, *reloaded.Sources[0].TargetGroupID)
	}
	if reloaded.Sources[2].Operation != domain.OperationRemove {
		t.Fatalf("expected operation remove, got %s", reloaded.Sources[2].Operation)
	}
	if *reloaded.Sources[2].Pattern != pattern {
		t.Fatalf("expected pattern %s, got %s", pattern, *reloaded.Sources[2].Pattern)
	}
	if *reloaded.Sources[3].NodeLogicalID != nodeID {
		t.Fatalf("expected node_logical_id %s, got %s", nodeID, *reloaded.Sources[3].NodeLogicalID)
	}
	if reloaded.Sources[4].FilterSpec == nil || len(reloaded.Sources[4].FilterSpec.Conditions) != 1 {
		t.Fatalf("expected filter_spec with 1 condition, got %v", reloaded.Sources[4].FilterSpec)
	}
	if *reloaded.Sources[5].BuiltinName != "PASS" {
		t.Fatalf("expected builtin PASS, got %v", reloaded.Sources[5].BuiltinName)
	}

	// 3. Optimistic concurrency conflict test (wrong revision)
	_, errConflict := repo.SaveGroupWithSources(ctx, reloaded, 999, reloaded.Sources)
	if errConflict == nil {
		t.Fatal("expected revision conflict error, but succeeded")
	}
	var domErr *domain.DomainError
	if !errors.As(errConflict, &domErr) || domErr.Code != "revision_conflict" {
		t.Fatalf("expected revision_conflict domain error, got: %v", errConflict)
	}
	if domErr.CurrentRevision == nil || *domErr.CurrentRevision != 1 {
		t.Fatalf("expected CurrentRevision 1, got %v", domErr.CurrentRevision)
	}

	// 4. Successful update with expected_revision = 1 -> increments to 2
	updatedSources := append(reloaded.Sources[:2], reloaded.Sources[3:]...) // drop regex item
	saved2, err := repo.SaveGroupWithSources(ctx, reloaded, 1, updatedSources)
	if err != nil {
		t.Fatalf("expected successful update with revision 1, got: %v", err)
	}
	if saved2.Revision != 2 {
		t.Fatalf("expected updated revision 2, got %d", saved2.Revision)
	}
	if len(saved2.Sources) != 5 {
		t.Fatalf("expected 5 sources after update, got %d", len(saved2.Sources))
	}
}

func TestPolicyRepository_Migration_000018_ZeroLoss(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:test_mig18_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}

	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	// Run up to 17 (pre-000018 baseline)
	if err := runner.RunUpTo(ctx, 17); err != nil {
		t.Fatalf("failed to run migrations up to 17: %v", err)
	}

	// Insert legacy data:
	// Group 1: has edges + group filter (intersection)
	g1ID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()
	nodeID := "node_01a0b9afc11670d1a027000000000002"
	now := time.Now().UTC().Format(time.RFC3339)

	res1, err1 := db.Exec(`INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass) VALUES (?, 'Group 1', 'select', ?, ?, 0)`, g1ID, now, now)
	if err1 != nil {
		t.Fatalf("insert g1: %v", err1)
	}
	_, _ = res1, err1
	_, err2 := db.Exec(`INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass) VALUES (?, 'Child Group', 'select', ?, ?, 0)`, childID, now, now)
	if err2 != nil {
		t.Fatalf("insert child: %v", err2)
	}
	_, err3 := db.Exec(`INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at) VALUES (?, 'ss', 'N1', '1.1.1.1', 8388, '{}', 1, ?, ?)`, nodeID, now, now)
	if err3 != nil {
		t.Fatalf("insert node: %v", err3)
	}

	edge1ID := domain.MustNewUUIDv7()
	edge2ID := domain.MustNewUUIDv7()
	_, errE1 := db.Exec(`INSERT INTO group_edges (id, parent_group_id, child_group_id, node_logical_id, position) VALUES (?, ?, ?, NULL, 0)`, edge1ID, g1ID, childID)
	if errE1 != nil {
		t.Fatalf("insert edge 1: %v", errE1)
	}
	_, errE2 := db.Exec(`INSERT INTO group_edges (id, parent_group_id, child_group_id, node_logical_id, position) VALUES (?, ?, NULL, ?, 1)`, edge2ID, g1ID, nodeID)
	if errE2 != nil {
		t.Fatalf("insert edge 2: %v", errE2)
	}

	filterJSON := `{"conditions":[{"field":"display_name","op":"contains","value":"HK"}]}`
	_, _ = db.Exec(`INSERT INTO group_node_filters (group_id, filter_spec, updated_at) VALUES (?, ?, ?)`, g1ID, filterJSON, now)

	// Group 2: Filter-only group (no edges)
	g2ID := domain.MustNewUUIDv7()
	_, _ = db.Exec(`INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass) VALUES (?, 'Group 2 Filter Only', 'select', ?, ?, 0)`, g2ID, now, now)
	_, _ = db.Exec(`INSERT INTO group_node_filters (group_id, filter_spec, updated_at) VALUES (?, ?, ?)`, g2ID, filterJSON, now)

	// Run migration 18
	if err := runner.RunUpTo(ctx, 18); err != nil {
		t.Fatalf("failed to run migration 18: %v", err)
	}
	// Run remaining migrations to match the latest repository schema (e.g. migration 23)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to run remaining migrations: %v", err)
	}

	repo := sqlite.NewPolicyRepository(db)

	// Verify Group 1:
	g1, err := repo.GetGroupByID(ctx, g1ID)
	if err != nil {
		t.Fatalf("failed to get group 1: %v", err)
	}
	if len(g1.Sources) != 2 {
		t.Fatalf("expected 2 sources in group 1, got %d", len(g1.Sources))
	}
	if g1.Sources[0].Kind != domain.SourceKindGroupRef || *g1.Sources[0].TargetGroupID != childID {
		t.Fatalf("expected group 1 source 0 to be group_ref pointing to childID, got %+v", g1.Sources[0])
	}
	if g1.Sources[0].FilterSpec == nil {
		t.Fatalf("expected group 1 source 0 to retain filter_spec predicate from intersection")
	}
	if g1.Sources[1].Kind != domain.SourceKindManualNode || *g1.Sources[1].NodeLogicalID != nodeID {
		t.Fatalf("expected group 1 source 1 to be manual_node pointing to nodeID, got %+v", g1.Sources[1])
	}

	// Verify Group 2 (filter-only):
	g2, err := repo.GetGroupByID(ctx, g2ID)
	if err != nil {
		t.Fatalf("failed to get group 2: %v", err)
	}
	if len(g2.Sources) != 1 {
		t.Fatalf("expected 1 source in group 2, got %d", len(g2.Sources))
	}
	if g2.Sources[0].Kind != domain.SourceKindFilter || g2.Sources[0].FilterSpec == nil {
		t.Fatalf("expected group 2 source to be filter kind with filter_spec, got %+v", g2.Sources[0])
	}
}

func TestPolicyRepository_LegacyEditsUseSourcesAndCascade(t *testing.T) {
	ctx := context.Background()
	cfg, repo := setupPolicyTestDB(t)

	pID := domain.MustNewUUIDv7()
	cID := domain.MustNewUUIDv7()

	childGroup := &domain.NodeGroup{
		ID:        cID,
		Name:      "Child For Sync",
		GroupType: domain.GroupTypeSelect,
	}
	if _, err := repo.SaveGroupWithSources(ctx, childGroup, 0, nil); err != nil {
		t.Fatalf("create child failed: %v", err)
	}

	parentGroup := &domain.NodeGroup{
		ID:        pID,
		Name:      "Parent For Sync",
		GroupType: domain.GroupTypeSelect,
	}
	if _, err := repo.SaveGroupWithSources(ctx, parentGroup, 0, nil); err != nil {
		t.Fatalf("create parent failed: %v", err)
	}

	// 1. Write via SetEdgesForGroup (legacy edge endpoint)
	nodeID := "node_01a0b9afc11670d1a027000000000001"
	edges := []domain.GroupEdge{
		{ChildGroupID: &cID, Position: 0},
		{NodeLogicalID: &nodeID, Position: 1},
	}
	if err := repo.SetEdgesForGroup(ctx, pID, edges); err != nil {
		t.Fatalf("SetEdgesForGroup failed: %v", err)
	}

	// Reload group and verify policy_group_sources got synchronized
	reloaded, err := repo.GetGroupByID(ctx, pID)
	if err != nil {
		t.Fatalf("GetGroupByID failed: %v", err)
	}
	if len(reloaded.Sources) != 2 {
		t.Fatalf("expected 2 sources synced from edges, got %d", len(reloaded.Sources))
	}
	if reloaded.Sources[0].Kind != domain.SourceKindGroupRef || *reloaded.Sources[0].TargetGroupID != cID {
		t.Fatalf("expected source 0 to be group_ref to cID, got %+v", reloaded.Sources[0])
	}
	if reloaded.Sources[1].Kind != domain.SourceKindManualNode || *reloaded.Sources[1].NodeLogicalID != nodeID {
		t.Fatalf("expected source 1 to be manual_node to nodeID, got %+v", reloaded.Sources[1])
	}
	if reloaded.Revision <= 1 {
		t.Fatalf("expected revision incremented past 1, got %d", reloaded.Revision)
	}

	// 2. The old edge read is a projection; there is no ongoing legacy mirror.
	newSources := []domain.MemberSource{
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupRef,
			TargetGroupID: &cID,
		},
	}
	saved, err := repo.SaveGroupWithSources(ctx, reloaded, reloaded.Revision, newSources)
	if err != nil {
		t.Fatalf("SaveGroupWithSources failed: %v", err)
	}
	db, err := sqlite.Open(*cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var legacyCount int
	if err := db.QueryRow(`SELECT count(*) FROM group_edges WHERE parent_group_id=?`, pID).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatalf("sources were mirrored to legacy storage: %d", legacyCount)
	}
	legacyEdges, err := repo.ListEdgesByGroup(ctx, pID)
	if err != nil {
		t.Fatalf("ListEdgesByGroup failed: %v", err)
	}
	if len(legacyEdges) != 1 || legacyEdges[0].ChildGroupID == nil || *legacyEdges[0].ChildGroupID != cID {
		t.Fatalf("expected 1 synced legacy edge pointing to cID, got %+v", legacyEdges)
	}

	// 3. Deleting group cascades to both policy_group_sources and group_edges
	if err := repo.DeleteGroup(ctx, pID); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}
	postSources, err := repo.GetSourcesByGroup(ctx, pID)
	if err != nil {
		t.Fatalf("GetSourcesByGroup post delete failed: %v", err)
	}
	if len(postSources) != 0 {
		t.Fatalf("expected 0 sources post delete, got %d", len(postSources))
	}
	postEdges, err := repo.ListEdgesByGroup(ctx, pID)
	if err != nil {
		t.Fatalf("ListEdgesByGroup post delete failed: %v", err)
	}
	if len(postEdges) != 0 {
		t.Fatalf("expected 0 edges post delete, got %d", len(postEdges))
	}
	_ = saved
}
