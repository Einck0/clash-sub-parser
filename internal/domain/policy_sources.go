package domain

// ProjectSourceEdges is a read-only compatibility view, never a second store.
func ProjectSourceEdges(sources []MemberSource) []GroupEdge {
	edges := make([]GroupEdge, 0)
	for _, s := range sources {
		if s.Operation != OperationAdd {
			continue
		}
		if s.Kind != SourceKindGroupRef && s.Kind != SourceKindManualNode {
			continue
		}
		edges = append(edges, GroupEdge{ID: s.ID, ParentGroupID: s.GroupID, ChildGroupID: s.TargetGroupID, NodeLogicalID: s.NodeLogicalID, Position: s.Position})
	}
	return edges
}

// EditLegacySources accepts old graph edits only when their representation is lossless.
// Rich pipelines must be edited through sources; otherwise an old client could erase removals.
func EditLegacySources(groupID string, current []MemberSource, edges *[]GroupEdge, changeFilter bool, filter *NodeFilterSpec) ([]MemberSource, error) {
	for _, s := range current {
		if s.Operation != OperationAdd || (s.Kind != SourceKindManualNode && s.Kind != SourceKindGroupRef && s.Kind != SourceKindFilter) {
			return nil, NewConflictError("sources_edit_required", "this group uses a member pipeline; edit sources instead of legacy edges/filter")
		}
	}
	out := append([]MemberSource(nil), current...)
	if edges != nil {
		out = make([]MemberSource, 0, len(*edges))
		for i, e := range *edges {
			e.ParentGroupID = groupID
			e.Position = i
			if err := ValidateGroupEdge(e); err != nil {
				return nil, err
			}
			s := MemberSource{ID: e.ID, GroupID: groupID, Operation: OperationAdd, Kind: SourceKindManualNode, Position: i, NodeLogicalID: e.NodeLogicalID}
			if e.ChildGroupID != nil {
				s.Kind = SourceKindGroupRef
				s.TargetGroupID = e.ChildGroupID
			}
			out = append(out, s)
		}
		if !changeFilter && len(current) > 0 {
			filter = current[0].FilterSpec
			changeFilter = true
		}
	}
	if changeFilter {
		filter.NormalizeForSave()
		if err := filter.Validate(); err != nil {
			return nil, err
		}
		filtered := out[:0]
		for _, s := range out {
			if s.Kind == SourceKindFilter {
				continue
			}
			s.FilterSpec = nil
			if !filter.IsEmpty() {
				s.FilterSpec = filter
			}
			filtered = append(filtered, s)
		}
		out = filtered
		if len(out) == 0 && !filter.IsEmpty() {
			out = append(out, MemberSource{GroupID: groupID, Operation: OperationAdd, Kind: SourceKindFilter, FilterSpec: filter})
		}
	}
	for i := range out {
		if out[i].FilterSpec != nil {
			out[i].FilterSpec.NormalizeForSave()
		}
	}
	return out, nil
}
