package compiler_test

import (
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func TestPruneUnavailableOptionalGroups(t *testing.T) {
	node1 := resolver.ResolvedNode{
		LogicalID:   "node-1",
		DisplayName: "Node 1",
		Protocol:    domain.ProtocolSS,
	}

	t.Run("PrunesEmptyGroupReferencedBySelectorWithoutRuleTarget", func(t *testing.T) {
		snap := &resolver.ResolvedPolicySnapshot{
			Nodes: []resolver.ResolvedNode{node1},
			Groups: []resolver.ResolvedGroup{
				{
					ID:        "grp-warp",
					Name:      "WARP",
					GroupType: domain.GroupTypeURLTest,
					Members:   []resolver.ResolvedGroupMember{}, // 0 members!
				},
				{
					ID:        "grp-select-1",
					Name:      "选择节点",
					GroupType: domain.GroupTypeSelect,
					Members: []resolver.ResolvedGroupMember{
						{Kind: resolver.MemberKindNode, TargetID: "node-1", DisplayName: "Node 1"},
						{Kind: resolver.MemberKindGroup, TargetID: "grp-warp", DisplayName: "WARP"},
					},
				},
				{
					ID:        "grp-select-2",
					Name:      "AI",
					GroupType: domain.GroupTypeSelect,
					Members: []resolver.ResolvedGroupMember{
						{Kind: resolver.MemberKindNode, TargetID: "node-1", DisplayName: "Node 1"},
						{Kind: resolver.MemberKindGroup, TargetID: "grp-warp", DisplayName: "WARP"},
					},
				},
			},
			Rules: []resolver.ResolvedRule{
				{
					TargetGroupID:   "grp-select-1",
					TargetGroupName: "选择节点",
					Expression:      "MATCH",
				},
			},
		}

		pruned, diags, err := compiler.PruneUnavailableOptionalGroups(snap)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pruned.Groups) != 2 {
			t.Fatalf("expected 2 groups after pruning, got %d", len(pruned.Groups))
		}
		for _, g := range pruned.Groups {
			if g.Name == "WARP" {
				t.Errorf("expected WARP group to be pruned")
			}
			if len(g.Members) != 1 {
				t.Errorf("expected parent group %s to have 1 remaining member, got %d", g.Name, len(g.Members))
			}
			if g.Members[0].DisplayName != "Node 1" {
				t.Errorf("expected remaining member to be Node 1, got %s", g.Members[0].DisplayName)
			}
		}

		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Code != "optional_group_pruned" {
			t.Errorf("expected optional_group_pruned code, got %s", diags[0].Code)
		}
		if diags[0].Target != "WARP" {
			t.Errorf("expected diagnostic target WARP, got %s", diags[0].Target)
		}
	})

	t.Run("RejectsPruningWhenDirectlyTargetedByRule", func(t *testing.T) {
		snap := &resolver.ResolvedPolicySnapshot{
			Nodes: []resolver.ResolvedNode{node1},
			Groups: []resolver.ResolvedGroup{
				{
					ID:        "grp-empty-routed",
					Name:      "EmptyRouted",
					GroupType: domain.GroupTypeSelect,
					Members:   []resolver.ResolvedGroupMember{}, // 0 members!
				},
			},
			Rules: []resolver.ResolvedRule{
				{
					TargetGroupID:   "grp-empty-routed",
					TargetGroupName: "EmptyRouted",
					Expression:      "MATCH",
				},
			},
		}

		pruned, diags, err := compiler.PruneUnavailableOptionalGroups(snap)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pruned.Groups) != 1 {
			t.Errorf("empty routed group must NOT be pruned, got %d groups", len(pruned.Groups))
		}
		if len(diags) != 0 {
			t.Errorf("expected 0 diagnostics emitted, got %d", len(diags))
		}
	})

	t.Run("RejectsPruningWhenReferencedByNonSelectorGroup", func(t *testing.T) {
		snap := &resolver.ResolvedPolicySnapshot{
			Nodes: []resolver.ResolvedNode{node1},
			Groups: []resolver.ResolvedGroup{
				{
					ID:        "grp-empty",
					Name:      "EmptyChild",
					GroupType: domain.GroupTypeURLTest,
					Members:   []resolver.ResolvedGroupMember{},
				},
				{
					ID:        "grp-parent-urltest",
					Name:      "ParentURLTest",
					GroupType: domain.GroupTypeURLTest, // NOT Select!
					Members: []resolver.ResolvedGroupMember{
						{Kind: resolver.MemberKindNode, TargetID: "node-1", DisplayName: "Node 1"},
						{Kind: resolver.MemberKindGroup, TargetID: "grp-empty", DisplayName: "EmptyChild"},
					},
				},
			},
		}

		pruned, diags, err := compiler.PruneUnavailableOptionalGroups(snap)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pruned.Groups) != 2 {
			t.Errorf("empty child under urltest parent must NOT be pruned, got %d groups", len(pruned.Groups))
		}
		if len(diags) != 0 {
			t.Errorf("expected 0 diagnostics, got %d", len(diags))
		}
	})

	t.Run("RejectsPruningWhenRemovingLeavesParentEmpty", func(t *testing.T) {
		snap := &resolver.ResolvedPolicySnapshot{
			Nodes: []resolver.ResolvedNode{node1},
			Groups: []resolver.ResolvedGroup{
				{
					ID:        "grp-empty",
					Name:      "EmptyChild",
					GroupType: domain.GroupTypeURLTest,
					Members:   []resolver.ResolvedGroupMember{},
				},
				{
					ID:        "grp-parent-select",
					Name:      "ParentSelect",
					GroupType: domain.GroupTypeSelect,
					Members: []resolver.ResolvedGroupMember{
						// Only this empty group!
						{Kind: resolver.MemberKindGroup, TargetID: "grp-empty", DisplayName: "EmptyChild"},
					},
				},
			},
		}

		pruned, diags, err := compiler.PruneUnavailableOptionalGroups(snap)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pruned.Groups) != 2 {
			t.Errorf("must NOT prune when parent would become empty, got %d groups", len(pruned.Groups))
		}
		if len(diags) != 0 {
			t.Errorf("expected 0 diagnostics, got %d", len(diags))
		}
	})
}
