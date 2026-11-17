package resolver_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// Removal subtracts recursively reachable child nodes from direct members only;
// it does not mutate either group reference or make removal a permanent veto.
func TestGroupNodesRemovalPreservesReferencesAndOrder(t *testing.T) {
	a, b, c := "node_aaaaaaaaaaaaaaaa", "node_bbbbbbbbbbbbbbbb", "node_cccccccccccccccc"
	cheap, nested, hongkong, parent := domain.MustNewUUIDv7(), domain.MustNewUUIDv7(), domain.MustNewUUIDv7(), domain.MustNewUUIDv7()
	nodes := []domain.Node{
		{LogicalID: a, DisplayName: "A", Protocol: "ss", Active: true},
		{LogicalID: b, DisplayName: "B", Protocol: "ss", Active: true},
		{LogicalID: c, DisplayName: "C", Protocol: "ss", Active: true},
	}
	ref := func(id *string, position int) domain.MemberSource {
		return domain.MemberSource{Operation: domain.OperationAdd, Kind: domain.SourceKindGroupRef, Position: position, TargetGroupID: id}
	}
	manual := func(id *string, position int) domain.MemberSource {
		return domain.MemberSource{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: position, NodeLogicalID: id}
	}
	remove := domain.MemberSource{Operation: domain.OperationRemove, Kind: domain.SourceKindGroupNodes, Position: 5, TargetGroupID: &cheap}
	for _, tc := range []struct {
		name         string
		sources      []domain.MemberSource
		cheapSources []domain.MemberSource
		want         []string
		wantCheap    []string
	}{
		{name: "references only", sources: []domain.MemberSource{ref(&cheap, 0), ref(&hongkong, 1), remove}, want: []string{cheap, hongkong}, wantCheap: []string{a, nested}},
		{name: "intersection and readd", sources: []domain.MemberSource{ref(&cheap, 0), ref(&hongkong, 1), manual(&a, 2), manual(&b, 3), manual(&c, 4), remove, manual(&a, 6)}, want: []string{cheap, hongkong, c, a}, wantCheap: []string{a, nested}},
		{name: "position controls order rather than input slice", sources: []domain.MemberSource{manual(&a, 6), remove, manual(&c, 4), manual(&b, 3), manual(&a, 2), ref(&hongkong, 1), ref(&cheap, 0)}, want: []string{cheap, hongkong, c, a}, wantCheap: []string{a, nested}},
		{name: "nested unpack and target ordered removal", sources: []domain.MemberSource{ref(&cheap, 0), ref(&hongkong, 1), manual(&a, 2), manual(&b, 3), manual(&c, 4), remove}, cheapSources: []domain.MemberSource{manual(&a, 0), {Operation: domain.OperationAdd, Kind: domain.SourceKindGroupNodes, Position: 1, TargetGroupID: &nested}, {Operation: domain.OperationRemove, Kind: domain.SourceKindManualNode, Position: 2, NodeLogicalID: &b}}, want: []string{cheap, hongkong, b, c}, wantCheap: []string{a}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cheapSources := tc.cheapSources
			if cheapSources == nil {
				cheapSources = []domain.MemberSource{manual(&a, 0), ref(&nested, 1)}
			}
			groups := []domain.NodeGroup{
				{ID: nested, Name: "Nested", GroupType: domain.GroupTypeSelect, Sources: []domain.MemberSource{manual(&b, 0)}},
				{ID: cheap, Name: "Cheap", GroupType: domain.GroupTypeSelect, Sources: cheapSources},
				{ID: hongkong, Name: "Hong Kong", GroupType: domain.GroupTypeSelect, Sources: []domain.MemberSource{manual(&a, 0), manual(&c, 1)}},
				{ID: parent, Name: "Select", GroupType: domain.GroupTypeSelect, Sources: tc.sources},
			}
			snap, err := resolver.New().Resolve(context.Background(), resolver.ResolveInput{Nodes: nodes, Groups: groups})
			if err != nil {
				t.Fatal(err)
			}
			want := map[string][]string{parent: tc.want, cheap: tc.wantCheap, nested: {b}, hongkong: {a, c}}
			for _, group := range snap.Groups {
				expected, ok := want[group.ID]
				if !ok {
					continue
				}
				if len(group.Members) != len(expected) {
					t.Fatalf("%s: expected %v, got %+v", group.Name, expected, group.Members)
				}
				for i, id := range expected {
					if group.Members[i].TargetID != id {
						t.Fatalf("%s member %d: expected %s, got %+v", group.Name, i, id, group.Members[i])
					}
				}
				delete(want, group.ID)
			}
			if len(want) != 0 {
				t.Fatalf("groups missing from snapshot: %v", want)
			}
		})
	}
}
