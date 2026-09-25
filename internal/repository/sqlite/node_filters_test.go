package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestNodeFilterRepository_GlobalCRUD(t *testing.T) {
	db, _ := setupTestDB(t)
	repo := sqlite.NewNodeFilterRepository(db)
	ctx := context.Background()

	// 1. Initial global filter should be present from migration (empty spec)
	initial, err := repo.GetGlobalFilter(ctx)
	if err != nil {
		t.Fatalf("GetGlobalFilter failed: %v", err)
	}
	if initial == nil {
		t.Fatal("expected non-nil initial GlobalNodeFilter")
	}
	if !initial.Spec.IsEmpty() {
		t.Fatalf("expected initial spec to be empty, got %+v", initial.Spec)
	}

	// 2. Set new global filter with valid conditions
	probeKind := domain.ProbeKindBaseline
	freshness := 3600
	newSpec := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field: domain.FilterFieldProtocol,
				Op:    domain.FilterOpEquals,
				Value: "hysteria2",
			},
			{
				Field:            domain.FilterFieldProbeVerdict,
				Op:               domain.FilterOpEquals,
				Value:            "available",
				ProbeKind:        &probeKind,
				FreshnessSeconds: &freshness,
			},
		},
	}

	now := time.Now().UTC().Truncate(time.Second)
	err = repo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
		Spec:      newSpec,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("SetGlobalFilter failed: %v", err)
	}

	// 3. Read back and verify
	loaded, err := repo.GetGlobalFilter(ctx)
	if err != nil {
		t.Fatalf("GetGlobalFilter after set failed: %v", err)
	}
	if len(loaded.Spec.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(loaded.Spec.Conditions))
	}
	if loaded.Spec.Conditions[0].Field != domain.FilterFieldProtocol || loaded.Spec.Conditions[0].Value != "hysteria2" {
		t.Errorf("condition 0 mismatch: %+v", loaded.Spec.Conditions[0])
	}
	if loaded.Spec.Conditions[1].Field != domain.FilterFieldProbeVerdict || *loaded.Spec.Conditions[1].ProbeKind != domain.ProbeKindBaseline {
		t.Errorf("condition 1 mismatch: %+v", loaded.Spec.Conditions[1])
	}

	// 4. Validation error on invalid condition
	invalidSpec := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field: domain.FilterFieldProtocol,
				Op:    domain.FilterOpLTE, // LTE not valid for protocol
				Value: "trojan",
			},
		},
	}
	err = repo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{Spec: invalidSpec})
	if err == nil {
		t.Fatal("expected error on invalid condition, got nil")
	}
}

func TestNodeFilterRepository_GroupCRUDAndCascade(t *testing.T) {
	db, _ := setupTestDB(t)
	repo := sqlite.NewNodeFilterRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	ctx := context.Background()

	// 1. Create a group first (foreign key requirement)
	groupID, err := domain.NewUUIDv7()
	if err != nil {
		t.Fatalf("failed to generate UUID: %v", err)
	}
	group := &domain.NodeGroup{
		ID:        groupID,
		Name:      "FilterGroup1",
		GroupType: domain.GroupTypeSelect,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := policyRepo.CreateGroup(ctx, group); err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	// 2. Group filter should not exist initially
	_, err = repo.GetGroupFilter(ctx, groupID)
	if err == nil {
		t.Fatal("expected NotFound error for non-existent group filter, got nil")
	}

	// 3. Set group filter
	spec := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpContains,
				Value: "Tokyo",
			},
		},
	}
	err = repo.SetGroupFilter(ctx, &domain.GroupNodeFilter{
		GroupID:   groupID,
		Spec:      spec,
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("SetGroupFilter failed: %v", err)
	}

	// 4. GetGroupFilter
	gf, err := repo.GetGroupFilter(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupFilter failed: %v", err)
	}
	if len(gf.Spec.Conditions) != 1 || gf.Spec.Conditions[0].Value != "Tokyo" {
		t.Fatalf("unexpected group filter: %+v", gf)
	}

	// 5. ListGroupFilters
	all, err := repo.ListGroupFilters(ctx)
	if err != nil {
		t.Fatalf("ListGroupFilters failed: %v", err)
	}
	if len(all) != 1 || len(all[groupID].Conditions) != 1 {
		t.Fatalf("expected map with 1 entry, got %+v", all)
	}

	// 6. DeleteGroupFilter explicitly
	err = repo.DeleteGroupFilter(ctx, groupID)
	if err != nil {
		t.Fatalf("DeleteGroupFilter failed: %v", err)
	}
	_, err = repo.GetGroupFilter(ctx, groupID)
	if err == nil {
		t.Fatal("expected NotFound error after delete, got nil")
	}

	// 7. Cascade delete test: Set group filter again, then delete group
	err = repo.SetGroupFilter(ctx, &domain.GroupNodeFilter{
		GroupID:   groupID,
		Spec:      spec,
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("SetGroupFilter failed: %v", err)
	}
	if err := policyRepo.DeleteGroup(ctx, groupID); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}
	_, err = repo.GetGroupFilter(ctx, groupID)
	if err == nil {
		t.Fatal("expected NotFound error after parent group cascade delete, got nil")
	}
}
