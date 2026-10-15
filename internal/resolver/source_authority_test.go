package resolver_test

import (
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"context"
	"testing"
)

func TestSourcesAuthority_EmptyIgnoresLegacyAndMapOverrides(t *testing.T) {
	g := domain.MustNewUUIDv7()
	id := testNodeLogicalID("authority")
	filter := domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "HK"}}}
	input := resolver.ResolveInput{Nodes: []domain.Node{{LogicalID: id, DisplayName: "HK", Protocol: domain.ProtocolSS, Active: true}}, Groups: []domain.NodeGroup{{ID: g, Name: "empty", GroupType: domain.GroupTypeSelect, NodeFilter: &filter}}, Edges: map[string][]domain.GroupEdge{g: {{ParentGroupID: g, NodeLogicalID: &id}}}, GroupFilters: map[string]domain.NodeFilterSpec{g: filter}}
	snap, err := resolver.New().Resolve(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Groups[0].Members) != 0 {
		t.Fatal("legacy edges/filter resurrected empty sources")
	}
	input.Groups[0].Sources = []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &id}}
	input.GroupSources = map[string][]domain.MemberSource{g: {}}
	snap, err = resolver.New().Resolve(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Groups[0].Members) != 0 {
		t.Fatal("explicit empty source map did not override stale group sources")
	}
}
