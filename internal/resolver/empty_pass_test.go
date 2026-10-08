package resolver_test

import (
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"context"
	"testing"
)

func TestResolverExplicitPassNoInheritanceAndDigest(t *testing.T) {
	child, parent := domain.MustNewUUIDv7(), domain.MustNewUUIDv7()
	input := resolver.ResolveInput{Groups: []domain.NodeGroup{{ID: child, Name: "child", GroupType: domain.GroupTypeSelect, EmptyFallbackPass: true}, {ID: parent, Name: "parent", GroupType: domain.GroupTypeFallback}}, Edges: map[string][]domain.GroupEdge{parent: {{ParentGroupID: parent, ChildGroupID: &child}}}, PolicyRules: []domain.PolicyRule{{TargetGroupID: parent, Expression: "MATCH"}}}
	r := resolver.New()
	snap, err := r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	var passCount int
	for _, g := range snap.Groups {
		if g.UsesEmptyPass() {
			passCount++
			if g.ID != child {
				t.Fatal("parent inherited PASS")
			}
		}
		if len(g.AllNodeLogicalIDs) != 0 {
			t.Fatal("PASS forged node ID")
		}
	}
	if passCount != 1 || len(snap.Nodes) != 0 || len(snap.NodeLogicalIDs) != 0 {
		t.Fatal("unexpected PASS/node counts")
	}
	found := false
	for _, d := range snap.Diagnostics {
		if d.Code == "empty_routed_group" && d.Target == parent {
			found = true
		}
	}
	if !found {
		t.Fatal("unselected parent no longer strict")
	}
	repeat, err := r.Resolve(context.Background(), input)
	if err != nil || repeat.InputDigest != snap.InputDigest || repeat.SnapshotDigest != snap.SnapshotDigest {
		t.Fatal("nondeterministic empty PASS")
	}
	input.Groups[0].EmptyFallbackPass = false
	off, err := r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if off.InputDigest == snap.InputDigest || off.SnapshotDigest == snap.SnapshotDigest {
		t.Fatal("flag omitted from digest")
	}
	input.Edges[child] = []domain.GroupEdge{{ParentGroupID: child, ChildGroupID: &parent}}
	if _, err := r.Resolve(context.Background(), input); err == nil {
		t.Fatal("PASS bypassed cycle guard")
	}
}

func TestResolverNonemptyFlagAndDerivedProjection(t *testing.T) {
	child, parent := domain.MustNewUUIDv7(), domain.MustNewUUIDv7()
	n := domain.Node{LogicalID: testNodeLogicalID("pass-node"), DisplayName: "HK", Protocol: domain.ProtocolTrojan, Active: true}
	input := resolver.ResolveInput{Nodes: []domain.Node{n}, Groups: []domain.NodeGroup{{ID: child, Name: "child", GroupType: domain.GroupTypeSelect, EmptyFallbackPass: true}, {ID: parent, Name: "parent", GroupType: domain.GroupTypeSelect}}, Edges: map[string][]domain.GroupEdge{child: {{ParentGroupID: child, NodeLogicalID: &n.LogicalID}}, parent: {{ParentGroupID: parent, ChildGroupID: &child}}}}
	r := resolver.New()
	snap, err := r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range snap.Groups {
		if g.UsesEmptyPass() {
			t.Fatal("nonempty group gained PASS")
		}
	}
	input.GroupFilters = map[string]domain.NodeFilterSpec{parent: {Conditions: []domain.FilterCondition{{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "US"}}}}
	snap, err = r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	derived := false
	for _, g := range snap.Groups {
		if g.ID != child && g.ID != parent {
			derived = true
			if g.EmptyFallbackPass || g.UsesEmptyPass() {
				t.Fatal("derived projection inherited permission")
			}
		}
	}
	if !derived {
		t.Fatal("fixture did not create derived projection")
	}
}
