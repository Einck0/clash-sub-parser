package resolver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// TestSourcesPipeline_SequentialReentrantAddRemove verifies that:
// 1. sources evaluate strictly in position order;
// 2. remove drops matching items and clears seen markers;
// 3. subsequent add re-adds previously removed items to the tail;
// 4. first-seen deduplication applies to current working set.
func TestSourcesPipeline_SequentialReentrantAddRemove(t *testing.T) {
	ctx := context.Background()
	r := resolver.New()

	n1ID := "node_01a0b9afc11670d1a027000000000001"
	n2ID := "node_01a0b9afc11670d1a027000000000002"
	n3ID := "node_01a0b9afc11670d1a027000000000003"

	nodes := []domain.Node{
		{LogicalID: n1ID, DisplayName: "香港 01 IPLC", Protocol: "ss", Active: true},
		{LogicalID: n2ID, DisplayName: "香港 02 BGP", Protocol: "ss", Active: true},
		{LogicalID: n3ID, DisplayName: "日本 01 专线", Protocol: "ss", Active: true},
	}

	groupID := domain.MustNewUUIDv7()

	regexHK := ".*香港.*"
	regex01 := ".*01.*"

	// Pipeline:
	// 0: add regex .*香港.* (adds HK 01, HK 02)
	// 1: remove regex .*01.* (removes HK 01; leaves HK 02)
	// 2: add manual_node HK 01 (re-adds HK 01 to the tail!)
	// 3: add manual_node HK 02 (already in working set, deduplicated!)
	sources := []domain.MemberSource{
		{
			Operation: domain.OperationAdd,
			Kind:      domain.SourceKindRegex,
			Position:  0,
			Pattern:   &regexHK,
		},
		{
			Operation: domain.OperationRemove,
			Kind:      domain.SourceKindRegex,
			Position:  1,
			Pattern:   &regex01,
		},
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindManualNode,
			Position:      2,
			NodeLogicalID: &n1ID,
		},
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindManualNode,
			Position:      3,
			NodeLogicalID: &n2ID,
		},
	}

	groups := []domain.NodeGroup{
		{
			ID:        groupID,
			Name:      "Test Pipeline Group",
			GroupType: domain.GroupTypeSelect,
			Sources:   sources,
		},
	}

	snap, err := r.Resolve(ctx, resolver.ResolveInput{
		Nodes:  nodes,
		Groups: groups,
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if len(snap.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(snap.Groups))
	}

	members := snap.Groups[0].Members
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d: %+v", len(members), members)
	}

	// First should be HK 02 (remained from step 1)
	if members[0].TargetID != n2ID || members[0].DisplayName != "香港 02 BGP" {
		t.Fatalf("expected position 0 to be HK 02, got: %+v", members[0])
	}
	// Second should be HK 01 (re-added to tail at step 2)
	if members[1].TargetID != n1ID || members[1].DisplayName != "香港 01 IPLC" {
		t.Fatalf("expected position 1 to be HK 01, got: %+v", members[1])
	}
}

// TestSourcesPipeline_TypeDomainIsolation verifies:
// 1. group_ref and group_nodes can coexist for the same child group;
// 2. remove group_ref removes only the group reference, preserving expanded leaf nodes;
// 3. remove group_nodes removes only leaf nodes, preserving group reference.
func TestSourcesPipeline_TypeDomainIsolation(t *testing.T) {
	ctx := context.Background()
	r := resolver.New()

	n1ID := "node_01a0b9afc11670d1a027000000000001"
	nodes := []domain.Node{
		{LogicalID: n1ID, DisplayName: "US 01", Protocol: "ss", Active: true},
	}

	childGroupID := domain.MustNewUUIDv7()
	parentGroupID := domain.MustNewUUIDv7()

	childSources := []domain.MemberSource{
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindManualNode,
			Position:      0,
			NodeLogicalID: &n1ID,
		},
	}

	// Case A: parent has both group_ref and group_nodes -> both coexist!
	parentSourcesA := []domain.MemberSource{
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupRef,
			Position:      0,
			TargetGroupID: &childGroupID,
		},
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupNodes,
			Position:      1,
			TargetGroupID: &childGroupID,
		},
	}

	groupsA := []domain.NodeGroup{
		{ID: childGroupID, Name: "Child Group", GroupType: domain.GroupTypeSelect, Sources: childSources},
		{ID: parentGroupID, Name: "Parent Group", GroupType: domain.GroupTypeSelect, Sources: parentSourcesA},
	}

	snapA, err := r.Resolve(ctx, resolver.ResolveInput{Nodes: nodes, Groups: groupsA})
	if err != nil {
		t.Fatalf("resolve A failed: %v", err)
	}

	var parentA resolver.ResolvedGroup
	for _, g := range snapA.Groups {
		if g.ID == parentGroupID {
			parentA = g
		}
	}
	if len(parentA.Members) != 2 {
		t.Fatalf("expected 2 members in parent A (1 group, 1 node), got %d: %+v", len(parentA.Members), parentA.Members)
	}
	if parentA.Members[0].Kind != resolver.MemberKindGroup || parentA.Members[0].TargetID != childGroupID {
		t.Fatalf("expected member 0 to be child group, got: %+v", parentA.Members[0])
	}
	if parentA.Members[1].Kind != resolver.MemberKindNode || parentA.Members[1].TargetID != n1ID {
		t.Fatalf("expected member 1 to be node n1, got: %+v", parentA.Members[1])
	}

	// Case B: parent adds both then removes group_ref -> only leaf node remains!
	parentSourcesB := append(append([]domain.MemberSource(nil), parentSourcesA...), domain.MemberSource{
		Operation:     domain.OperationRemove,
		Kind:          domain.SourceKindGroupRef,
		Position:      2,
		TargetGroupID: &childGroupID,
	})

	groupsB := []domain.NodeGroup{
		{ID: childGroupID, Name: "Child Group", GroupType: domain.GroupTypeSelect, Sources: childSources},
		{ID: parentGroupID, Name: "Parent Group", GroupType: domain.GroupTypeSelect, Sources: parentSourcesB},
	}

	snapB, err := r.Resolve(ctx, resolver.ResolveInput{Nodes: nodes, Groups: groupsB})
	if err != nil {
		t.Fatalf("resolve B failed: %v", err)
	}
	for _, g := range snapB.Groups {
		if g.ID == parentGroupID {
			if len(g.Members) != 1 || g.Members[0].Kind != resolver.MemberKindNode || g.Members[0].TargetID != n1ID {
				t.Fatalf("expected only node member remaining after removing group_ref, got: %+v", g.Members)
			}
		}
	}

	// Case C: parent adds both then removes group_nodes -> only group_ref remains!
	parentSourcesC := append(append([]domain.MemberSource(nil), parentSourcesA...), domain.MemberSource{
		Operation:     domain.OperationRemove,
		Kind:          domain.SourceKindGroupNodes,
		Position:      2,
		TargetGroupID: &childGroupID,
	})

	groupsC := []domain.NodeGroup{
		{ID: childGroupID, Name: "Child Group", GroupType: domain.GroupTypeSelect, Sources: childSources},
		{ID: parentGroupID, Name: "Parent Group", GroupType: domain.GroupTypeSelect, Sources: parentSourcesC},
	}

	snapC, err := r.Resolve(ctx, resolver.ResolveInput{Nodes: nodes, Groups: groupsC})
	if err != nil {
		t.Fatalf("resolve C failed: %v", err)
	}
	for _, g := range snapC.Groups {
		if g.ID == parentGroupID {
			if len(g.Members) != 1 || g.Members[0].Kind != resolver.MemberKindGroup || g.Members[0].TargetID != childGroupID {
				t.Fatalf("expected only group_ref member remaining after removing group_nodes, got: %+v", g.Members)
			}
		}
	}
}

// TestSourcesPipeline_ChildConditionFirstParentItemConditionAfter verifies:
// Child group evaluates its own sources first, then parent item filter filters the unpacked leaves.
func TestSourcesPipeline_ChildConditionFirstParentItemConditionAfter(t *testing.T) {
	ctx := context.Background()
	r := resolver.New()

	n1ID := "node_01a0b9afc11670d1a027000000000001"
	n2ID := "node_01a0b9afc11670d1a027000000000002"
	n3ID := "node_01a0b9afc11670d1a027000000000003"

	nodes := []domain.Node{
		{LogicalID: n1ID, DisplayName: "HK 01 Fast", Protocol: "ss", Active: true},
		{LogicalID: n2ID, DisplayName: "HK 02 Slow", Protocol: "ss", Active: true},
		{LogicalID: n3ID, DisplayName: "JP 01 Fast", Protocol: "ss", Active: true},
	}

	childGroupID := domain.MustNewUUIDv7()
	parentGroupID := domain.MustNewUUIDv7()

	childRegex := ".*HK.*"
	parentItemPattern := ".*Fast.*"

	// Child: selects all HK (HK 01 Fast, HK 02 Slow)
	childSources := []domain.MemberSource{
		{
			Operation: domain.OperationAdd,
			Kind:      domain.SourceKindRegex,
			Position:  0,
			Pattern:   &childRegex,
		},
	}

	// Parent: group_nodes unpacking child group with pattern ".*Fast.*"
	parentSources := []domain.MemberSource{
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupNodes,
			Position:      0,
			TargetGroupID: &childGroupID,
			Pattern:       &parentItemPattern,
		},
	}

	groups := []domain.NodeGroup{
		{ID: childGroupID, Name: "Child HK", GroupType: domain.GroupTypeSelect, Sources: childSources},
		{ID: parentGroupID, Name: "Parent Fast HK", GroupType: domain.GroupTypeSelect, Sources: parentSources},
	}

	snap, err := r.Resolve(ctx, resolver.ResolveInput{Nodes: nodes, Groups: groups})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	for _, g := range snap.Groups {
		if g.ID == parentGroupID {
			if len(g.Members) != 1 || g.Members[0].TargetID != n1ID {
				t.Fatalf("expected only HK 01 Fast in parent, got: %+v", g.Members)
			}
		}
	}
}

// TestSourcesPipeline_TypedBuiltin verifies PASS, DIRECT, REJECT ordering and inclusion.
func TestSourcesPipeline_TypedBuiltin(t *testing.T) {
	ctx := context.Background()
	r := resolver.New()

	n1ID := "node_01a0b9afc11670d1a027000000000001"
	nodes := []domain.Node{
		{LogicalID: n1ID, DisplayName: "Node 1", Protocol: "ss", Active: true},
	}

	passStr := "PASS"
	directStr := "DIRECT"
	rejectStr := "REJECT"

	sources := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, Position: 0, BuiltinName: &directStr},
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 1, NodeLogicalID: &n1ID},
		{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, Position: 2, BuiltinName: &passStr},
		{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, Position: 3, BuiltinName: &rejectStr},
		{Operation: domain.OperationRemove, Kind: domain.SourceKindTypedBuiltin, Position: 4, BuiltinName: &rejectStr},
	}

	groupID := domain.MustNewUUIDv7()
	groups := []domain.NodeGroup{
		{ID: groupID, Name: "Builtin Group", GroupType: domain.GroupTypeSelect, Sources: sources},
	}

	snap, err := r.Resolve(ctx, resolver.ResolveInput{Nodes: nodes, Groups: groups})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	members := snap.Groups[0].Members
	if len(members) != 3 {
		t.Fatalf("expected 3 members (DIRECT, Node 1, PASS), got %d: %+v", len(members), members)
	}
	if members[0].Kind != resolver.MemberKindBuiltin || members[0].DisplayName != "DIRECT" {
		t.Fatalf("expected member 0 DIRECT, got %+v", members[0])
	}
	if members[1].Kind != resolver.MemberKindNode || members[1].TargetID != n1ID {
		t.Fatalf("expected member 1 Node 1, got %+v", members[1])
	}
	if members[2].Kind != resolver.MemberKindBuiltin || members[2].DisplayName != "PASS" {
		t.Fatalf("expected member 2 PASS, got %+v", members[2])
	}
}

// TestSourcesPipeline_CycleDetection_GroupRefAndGroupNodes verifies that
// topological cycles across group_ref and group_nodes are caught and produce 422 with cycle_path.
func TestSourcesPipeline_CycleDetection_GroupRefAndGroupNodes(t *testing.T) {
	ctx := context.Background()
	r := resolver.New()

	g1ID := domain.MustNewUUIDv7()
	g2ID := domain.MustNewUUIDv7()

	// G1 -> G2 (group_ref)
	// G2 -> G1 (group_nodes)
	g1Sources := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindGroupRef, Position: 0, TargetGroupID: &g2ID},
	}
	g2Sources := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindGroupNodes, Position: 0, TargetGroupID: &g1ID},
	}

	groups := []domain.NodeGroup{
		{ID: g1ID, Name: "G1", GroupType: domain.GroupTypeSelect, Sources: g1Sources},
		{ID: g2ID, Name: "G2", GroupType: domain.GroupTypeSelect, Sources: g2Sources},
	}

	_, err := r.Resolve(ctx, resolver.ResolveInput{Groups: groups})
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}

	var domErr *domain.DomainError
	if !strings.Contains(err.Error(), "cycle_detected") && (!errors.As(err, &domErr) || domErr.Code != "cycle_detected") {
		t.Fatalf("expected cycle_detected error, got: %v", err)
	}
}
