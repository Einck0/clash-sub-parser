package resolver_test

import (
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"context"
	"sort"
)

// sourceFixtureResolver migrates historical test data before invoking the real resolver.
// Production resolution has no legacy conversion; source-authority tests use resolver.New directly.
type sourceFixtureResolver struct{}

func (sourceFixtureResolver) Resolve(ctx context.Context, input resolver.ResolveInput) (*resolver.ResolvedPolicySnapshot, error) {
	input.Groups = append([]domain.NodeGroup(nil), input.Groups...)
	for i, g := range input.Groups {
		if len(g.Sources) > 0 {
			continue
		}
		edges := append([]domain.GroupEdge(nil), input.Edges[g.ID]...)
		sort.SliceStable(edges, func(i, j int) bool { return edges[i].Position < edges[j].Position })
		filter, hasFilter := input.GroupFilters[g.ID]
		if g.NodeFilter != nil {
			filter = *g.NodeFilter
			hasFilter = true
		}
		for _, e := range edges {
			s := domain.MemberSource{ID: e.ID, GroupID: g.ID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: e.Position, NodeLogicalID: e.NodeLogicalID}
			if e.ChildGroupID != nil {
				s.Kind = domain.SourceKindGroupRef
				s.TargetGroupID = e.ChildGroupID
			}
			if hasFilter {
				s.FilterSpec = &filter
			}
			input.Groups[i].Sources = append(input.Groups[i].Sources, s)
		}
		if len(edges) == 0 && hasFilter {
			input.Groups[i].Sources = []domain.MemberSource{{GroupID: g.ID, Operation: domain.OperationAdd, Kind: domain.SourceKindFilter, FilterSpec: &filter}}
		}
	}
	input.Edges = nil
	input.GroupFilters = nil
	return resolver.New().Resolve(ctx, input)
}
