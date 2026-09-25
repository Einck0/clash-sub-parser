package policy_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func setupTestDB(t *testing.T) *sql.DB {
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

	return db
}

func setupTestService(t *testing.T, db *sql.DB) (*policy.Service, domain.AuditRepository) {
	t.Helper()
	policyRepo := sqlite.NewPolicyRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	filterRepo := sqlite.NewNodeFilterRepository(db)

	svc := policy.NewService(policyRepo, revRepo, nodeRepo, auditRepo, filterRepo)
	return svc, auditRepo
}

func insertTestNode(t *testing.T, db *sql.DB, logicalID string) {
	t.Helper()
	const query = `
	INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
	VALUES (?, 'ss', 'Test Node', 'secret://test', 1, '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');`
	if _, err := db.Exec(query, logicalID); err != nil {
		t.Fatalf("failed to insert test node %s: %v", logicalID, err)
	}
}

// 1. Cycle Detection: A -> B -> A must be strictly detected and rejected.
func TestPolicyValidation_DirectCycle(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	// Create Group A
	grpA, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Group A",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating Group A: %v", err)
	}

	// Create Group B
	grpB, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Group B",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-2",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating Group B: %v", err)
	}

	// Add Edge A -> B
	err = svc.SetGroupEdges(ctx, policy.SetGroupEdgesCommand{
		ParentGroupID: grpA.ID,
		Edges: []policy.EdgeInput{
			{ChildGroupID: &grpB.ID, Position: 0},
		},
		RequestID: "req-3",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error adding edge A -> B: %v", err)
	}

	// Attempt to add Edge B -> A (causing A -> B -> A cycle)
	err = svc.SetGroupEdges(ctx, policy.SetGroupEdgesCommand{
		ParentGroupID: grpB.ID,
		Edges: []policy.EdgeInput{
			{ChildGroupID: &grpA.ID, Position: 0},
		},
		RequestID: "req-4",
		ActorKind: domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatalf("expected cycle_detected error for A -> B -> A, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if domErr.Code != "cycle_detected" {
		t.Errorf("expected error code 'cycle_detected', got %s", domErr.Code)
	}
	if !strings.Contains(domErr.Message, grpA.ID) || !strings.Contains(domErr.Message, grpB.ID) {
		t.Errorf("expected error message to reference cyclic group IDs, got: %s", domErr.Message)
	}

	// Verify that illegal edge was NOT saved
	edgesB, err := svc.GetGroupEdges(ctx, grpB.ID)
	if err != nil {
		t.Fatalf("unexpected error fetching edges: %v", err)
	}
	if len(edgesB) != 0 {
		t.Errorf("expected 0 edges for Group B, got %d", len(edgesB))
	}
}

// 2. Transitive Cycle Detection: A -> B -> C -> A must be detected.
func TestPolicyValidation_TransitiveCycle(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	grpA, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Group A",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	grpB, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Group B",
		GroupType: domain.GroupTypeURLTest,
		RequestID: "req-2",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	grpC, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Group C",
		GroupType: domain.GroupTypeFallback,
		RequestID: "req-3",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A -> B
	if err := svc.SetGroupEdges(ctx, policy.SetGroupEdgesCommand{
		ParentGroupID: grpA.ID,
		Edges:         []policy.EdgeInput{{ChildGroupID: &grpB.ID, Position: 0}},
		RequestID:     "req-4",
		ActorKind:     domain.ActorKindAdmin,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// B -> C
	if err := svc.SetGroupEdges(ctx, policy.SetGroupEdgesCommand{
		ParentGroupID: grpB.ID,
		Edges:         []policy.EdgeInput{{ChildGroupID: &grpC.ID, Position: 0}},
		RequestID:     "req-5",
		ActorKind:     domain.ActorKindAdmin,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// C -> A (closes cycle A -> B -> C -> A)
	err = svc.SetGroupEdges(ctx, policy.SetGroupEdgesCommand{
		ParentGroupID: grpC.ID,
		Edges:         []policy.EdgeInput{{ChildGroupID: &grpA.ID, Position: 0}},
		RequestID:     "req-6",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatalf("expected cycle_detected error, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "cycle_detected" {
		t.Fatalf("expected cycle_detected error, got %v", err)
	}

	// Ensure edges of C are empty
	edgesC, err := svc.GetGroupEdges(ctx, grpC.ID)
	if err != nil {
		t.Fatalf("unexpected error fetching edges: %v", err)
	}
	if len(edgesC) != 0 {
		t.Errorf("expected 0 edges for Group C, got %d", len(edgesC))
	}
}

// 3. Self-Loop Detection: A -> A must be strictly detected and rejected.
func TestPolicyValidation_SelfLoop(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	grpA, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Group A",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = svc.SetGroupEdges(ctx, policy.SetGroupEdgesCommand{
		ParentGroupID: grpA.ID,
		Edges: []policy.EdgeInput{
			{ChildGroupID: &grpA.ID, Position: 0},
		},
		RequestID: "req-2",
		ActorKind: domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatalf("expected self_loop_forbidden error, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "self_loop_forbidden" {
		t.Fatalf("expected self_loop_forbidden error, got %v", err)
	}
}

// 4. Missing Target Group: reference to non-existent group must be rejected.
func TestPolicyValidation_MissingTargetGroup(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	grpA, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Group A",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nonExistentID := domain.MustNewUUIDv7()
	err = svc.SetGroupEdges(ctx, policy.SetGroupEdgesCommand{
		ParentGroupID: grpA.ID,
		Edges: []policy.EdgeInput{
			{ChildGroupID: &nonExistentID, Position: 0},
		},
		RequestID: "req-2",
		ActorKind: domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatalf("expected missing_target_group error, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || (domErr.Code != "missing_target_group" && domErr.Code != "target_group_not_found") {
		t.Fatalf("expected target_group_not_found/missing_target_group error, got %v", err)
	}
}

// 5. MATCH Termination Rule Ordering Validation: MATCH rule must be terminal.
func TestPolicyValidation_MATCHRuleOrdering(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	grp, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Proxy",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating group: %v", err)
	}

	revID := domain.MustNewUUIDv7()

	// Valid rule at position 0: non-MATCH
	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    revID,
		TargetGroupID: grp.ID,
		Expression:    "DOMAIN-SUFFIX,google.com",
		Position:      0,
		RequestID:     "req-2",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating rule: %v", err)
	}

	// MATCH rule at position 1 (terminal so far)
	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    revID,
		TargetGroupID: grp.ID,
		Expression:    "MATCH",
		Position:      1,
		RequestID:     "req-3",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error adding terminal MATCH rule: %v", err)
	}

	// Attempt to add a rule at position 2 (after MATCH) - MUST be rejected!
	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    revID,
		TargetGroupID: grp.ID,
		Expression:    "IP-CIDR,1.1.1.1/32",
		Position:      2,
		RequestID:     "req-4",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatalf("expected invalid_match_rule_ordering error when adding rule after MATCH, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "invalid_match_rule_ordering" {
		t.Fatalf("expected invalid_match_rule_ordering error, got: %v", err)
	}
}

// 6. Multiple MATCH rules forbidden.
func TestPolicyValidation_MultipleMATCHRules(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	grp, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Proxy",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating group: %v", err)
	}

	revID := domain.MustNewUUIDv7()

	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    revID,
		TargetGroupID: grp.ID,
		Expression:    "MATCH",
		Position:      0,
		RequestID:     "req-2",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating MATCH rule: %v", err)
	}

	// Attempt second MATCH rule
	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    revID,
		TargetGroupID: grp.ID,
		Expression:    "MATCH",
		Position:      1,
		RequestID:     "req-3",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatalf("expected error when adding second MATCH rule, got nil")
	}
}

// 7. PolicyRule Target Group Must Exist.
func TestPolicyValidation_PolicyRuleTargetGroupMustExist(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	revID := domain.MustNewUUIDv7()
	fakeGroupID := domain.MustNewUUIDv7()

	_, err := svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    revID,
		TargetGroupID: fakeGroupID,
		Expression:    "MATCH",
		Position:      0,
		RequestID:     "req-1",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatalf("expected error for non-existent target group, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || (domErr.Code != "target_group_not_found" && domErr.Code != "missing_target_group") {
		t.Fatalf("expected target_group_not_found error, got %v", err)
	}
}

// 8. Valid Graph Creation, Retrieval, and Revision Generation with Stable Digest.
func TestPolicyService_ValidGraphAndRevision(t *testing.T) {
	db := setupTestDB(t)
	svc, auditRepo := setupTestService(t, db)
	ctx := context.Background()

	nodeLogicalID := "node_0123456789abcdef"
	insertTestNode(t, db, nodeLogicalID)

	// Create Subgroup Auto
	grpAuto, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Auto",
		GroupType: domain.GroupTypeURLTest,
		Edges: []policy.EdgeInput{
			{NodeLogicalID: &nodeLogicalID, Position: 0},
		},
		RequestID: "req-create-auto",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating Auto group: %v", err)
	}

	// Create Proxy group pointing to Auto and direct node
	grpProxy, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Proxy",
		GroupType: domain.GroupTypeSelect,
		Edges: []policy.EdgeInput{
			{ChildGroupID: &grpAuto.ID, Position: 0},
			{NodeLogicalID: &nodeLogicalID, Position: 1},
		},
		RequestID: "req-create-proxy",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating Proxy group: %v", err)
	}

	// List groups
	listResult, err := svc.ListGroups(ctx, policy.ListGroupsQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("unexpected error listing groups: %v", err)
	}
	if listResult.Total < 2 {
		t.Errorf("expected at least 2 groups, got %d", listResult.Total)
	}

	// Create a ConfigurationRevision
	rev, err := svc.CreateRevision(ctx, policy.CreateRevisionCommand{
		State:     domain.RevisionStateActive,
		RequestID: "req-create-rev",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating revision: %v", err)
	}
	if rev.ID == "" || rev.ContentDigest == "" {
		t.Errorf("expected valid revision ID and ContentDigest, got ID=%s Digest=%s", rev.ID, rev.ContentDigest)
	}
	if rev.State != domain.RevisionStateActive {
		t.Errorf("expected active state, got %s", rev.State)
	}

	// Add admission rule
	admRule, err := svc.CreateAdmissionRule(ctx, policy.CreateAdmissionRuleCommand{
		RevisionID: rev.ID,
		Name:       "allow-hk",
		Expression: "country == 'HK'",
		Action:     domain.RuleActionAllow,
		Position:   0,
		RequestID:  "req-adm-1",
		ActorKind:  domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating admission rule: %v", err)
	}
	if admRule.ID == "" || admRule.Action != domain.RuleActionAllow {
		t.Errorf("invalid admission rule: %#v", admRule)
	}

	// Add policy rules
	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    rev.ID,
		TargetGroupID: grpProxy.ID,
		Expression:    "DOMAIN-SUFFIX,google.com",
		Position:      0,
		RequestID:     "req-pol-1",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating policy rule: %v", err)
	}

	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    rev.ID,
		TargetGroupID: grpProxy.ID,
		Expression:    "MATCH",
		Position:      1,
		RequestID:     "req-pol-2",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected error creating terminal MATCH rule: %v", err)
	}

	// List rules
	rulesResult, err := svc.ListRules(ctx, policy.ListRulesQuery{RevisionID: rev.ID})
	if err != nil {
		t.Fatalf("unexpected error listing rules: %v", err)
	}
	if len(rulesResult.PolicyRules) != 2 {
		t.Errorf("expected 2 policy rules, got %d", len(rulesResult.PolicyRules))
	}
	if len(rulesResult.AdmissionRules) != 1 {
		t.Errorf("expected 1 admission rule, got %d", len(rulesResult.AdmissionRules))
	}

	// Verify Audit Events
	audits, totalAudits, err := auditRepo.List(ctx, domain.AuditFilter{Pagination: domain.Pagination{Page: 1, PageSize: 50}})
	if err != nil {
		t.Fatalf("unexpected error querying audits: %v", err)
	}
	if totalAudits == 0 || len(audits) == 0 {
		t.Errorf("expected audit events to be recorded, got total=%d", totalAudits)
	}
}

// 9. UpdateGroup and DeleteGroup functionality and edge cases.
func TestPolicyService_UpdateAndDeleteGroup(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	grpA, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Group A",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	// Update name and group_type
	newName := "Group A Updated"
	newType := domain.GroupTypeURLTest
	updated, err := svc.UpdateGroup(ctx, policy.UpdateGroupCommand{
		ID:        grpA.ID,
		Name:      &newName,
		GroupType: &newType,
		RequestID: "req-2",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to update group: %v", err)
	}
	if updated.Name != newName || updated.GroupType != newType {
		t.Errorf("unexpected updated group: %#v", updated)
	}

	// Delete group
	err = svc.DeleteGroup(ctx, policy.DeleteGroupCommand{
		ID:        grpA.ID,
		RequestID: "req-3",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to delete group: %v", err)
	}

	// Fetch deleted group must fail with not found
	_, err = svc.GetGroup(ctx, grpA.ID)
	if err == nil {
		t.Fatalf("expected error fetching deleted group, got nil")
	}
}

// 10. ValidateGraph API check.
func TestPolicyService_ValidateGraphAPI(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	// Empty graph is valid
	res, err := svc.ValidateGraph(ctx)
	if err != nil {
		t.Fatalf("unexpected error on empty graph: %v", err)
	}
	if !res.Valid {
		t.Errorf("expected empty graph to be valid")
	}

	// Insert valid group
	_, err = svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "DirectGroup",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	res2, err := svc.ValidateGraph(ctx)
	if err != nil || !res2.Valid {
		t.Errorf("expected valid graph, got res=%#v, err=%v", res2, err)
	}
}

func TestPolicyService_GlobalNodeFilter_CRUDAndAudit(t *testing.T) {
	db := setupTestDB(t)
	svc, auditRepo := setupTestService(t, db)
	ctx := context.Background()

	// 1. Initial global filter should default to empty conditions
	initial, err := svc.GetGlobalNodeFilter(ctx)
	if err != nil {
		t.Fatalf("GetGlobalNodeFilter failed: %v", err)
	}
	if len(initial.Spec.Conditions) != 0 {
		t.Fatalf("expected empty conditions initially, got %d", len(initial.Spec.Conditions))
	}

	// 2. Set global filter
	setCmd := policy.SetGlobalNodeFilterCommand{
		Spec: domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpEquals, Value: "ss"},
			},
		},
		RequestID: "req-filter-1",
		ActorKind: domain.ActorKindAdmin,
	}
	saved, err := svc.SetGlobalNodeFilter(ctx, setCmd)
	if err != nil {
		t.Fatalf("SetGlobalNodeFilter failed: %v", err)
	}
	if len(saved.Spec.Conditions) != 1 || saved.Spec.Conditions[0].Value != "ss" {
		t.Fatalf("unexpected saved filter: %+v", saved)
	}

	// 3. Verify Get retrieves updated filter
	retrieved, err := svc.GetGlobalNodeFilter(ctx)
	if err != nil {
		t.Fatalf("GetGlobalNodeFilter failed: %v", err)
	}
	if len(retrieved.Spec.Conditions) != 1 || retrieved.Spec.Conditions[0].Value != "ss" {
		t.Fatalf("unexpected retrieved filter: %+v", retrieved)
	}

	// 4. Verify audit event was logged
	events, total, err := auditRepo.List(ctx, domain.AuditFilter{})
	if err != nil || total == 0 {
		t.Fatalf("expected audit events, got err=%v total=%d", err, total)
	}
	found := false
	for _, e := range events {
		if e.Action == "policy.global_filter.update" && e.Result == domain.AuditResultSuccess {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected policy.global_filter.update audit event")
	}
}

func TestPolicyService_GroupNodeFilter_CreateUpdateClear(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	// 1. Create group with filter
	filterSpec := &domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "US"},
		},
	}
	grp, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:       "US-Group",
		GroupType:  domain.GroupTypeSelect,
		NodeFilter: filterSpec,
		RequestID:  "req-grp-1",
		ActorKind:  domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if grp.NodeFilter == nil || len(grp.NodeFilter.Conditions) != 1 {
		t.Fatalf("expected group to have filter, got %+v", grp.NodeFilter)
	}

	// 2. GetGroup retrieves the filter
	fetched, err := svc.GetGroup(ctx, grp.ID)
	if err != nil {
		t.Fatalf("GetGroup failed: %v", err)
	}
	if fetched.NodeFilter == nil || len(fetched.NodeFilter.Conditions) != 1 {
		t.Fatalf("expected fetched group to have filter, got %+v", fetched.NodeFilter)
	}

	// 3. ListGroups includes filter in views
	listRes, err := svc.ListGroups(ctx, policy.ListGroupsQuery{})
	if err != nil {
		t.Fatalf("ListGroups failed: %v", err)
	}
	if len(listRes.Items) != 1 || listRes.Items[0].NodeFilter == nil {
		t.Fatalf("expected ListGroups to include filter, got %+v", listRes.Items)
	}

	// 4. UpdateGroup omitting NodeFilter preserves existing filter
	newName := "US-Group-Renamed"
	updated, err := svc.UpdateGroup(ctx, policy.UpdateGroupCommand{
		ID:        grp.ID,
		Name:      &newName,
		RequestID: "req-grp-2",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}
	if updated.NodeFilter == nil || len(updated.NodeFilter.Conditions) != 1 {
		t.Fatalf("omitted NodeFilter in update should preserve existing filter, got %+v", updated.NodeFilter)
	}

	// 5. UpdateGroup with ClearNodeFilter clears filter
	cleared, err := svc.UpdateGroup(ctx, policy.UpdateGroupCommand{
		ID:              grp.ID,
		ClearNodeFilter: true,
		RequestID:       "req-grp-3",
		ActorKind:       domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("UpdateGroup with ClearNodeFilter failed: %v", err)
	}
	if cleared.NodeFilter != nil {
		t.Fatalf("expected NodeFilter to be cleared, got %+v", cleared.NodeFilter)
	}

	// Verify persistence also cleared
	fetchedCleared, err := svc.GetGroup(ctx, grp.ID)
	if err != nil {
		t.Fatalf("GetGroup failed: %v", err)
	}
	if fetchedCleared.NodeFilter != nil {
		t.Fatalf("persisted filter should be cleared, got %+v", fetchedCleared.NodeFilter)
	}
}

func TestPolicyService_FilterSpecValidationErrors(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()

	// 1. Invalid operator for display_name
	_, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "BadFilterGroup",
		GroupType: domain.GroupTypeSelect,
		NodeFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpEquals, Value: "test"},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected error for invalid op on display_name, got nil")
	}

	// 2. Negative latency
	_, err = svc.SetGlobalNodeFilter(ctx, policy.SetGlobalNodeFilterCommand{
		Spec: domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field:     domain.FilterFieldProbeLatencyMS,
					Op:        domain.FilterOpLTE,
					Value:     "-10",
					ProbeKind: &[]domain.ProbeKind{domain.ProbeKindBaseline}[0],
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected error for negative latency, got nil")
	}
}
