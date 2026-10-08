package resolver

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"clash-sub-parser/internal/domain"
)

// validateTopology checks the policy graph for self-loops, cycles, missing group references, and rule constraints.
func validateTopology(
	groups []domain.NodeGroup,
	edges map[string][]domain.GroupEdge,
	rules []domain.PolicyRule,
) error {
	return domain.ValidatePolicyTopology(groups, edges, rules, false)
}

func deterministicDerivedUUID(parentID, childID string) string {
	hash := sha256.Sum256([]byte("derived-group:" + parentID + ":" + childID))
	var raw [16]byte
	copy(raw[:], hash[:16])
	raw[6] = (raw[6] & 0x0f) | 0x70 // UUIDv7 version
	raw[8] = (raw[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

// resolveGroups constructs resolved groups with ordered members, child group links, and recursive node expansion.
func resolveGroups(
	groups []domain.NodeGroup,
	edges map[string][]domain.GroupEdge,
	globalAdmittedNodes []domain.Node,
	globalAdmittedMap map[string]domain.Node,
	admittedNodeMap map[string]domain.Node,
	groupFilters map[string]domain.NodeFilterSpec,
	globalFilter *domain.NodeFilterSpec,
	nodeSources map[string][]domain.NodeSource,
	latestObs map[string]map[domain.ProbeKind]domain.ProbeObservation,
	asOf time.Time,
	rules []domain.PolicyRule,
) ([]ResolvedGroup, []Diagnostic, map[string]GroupFilterCount) {
	diagnostics := make([]Diagnostic, 0)
	groupCounts := make(map[string]GroupFilterCount, len(groups))

	// Map groups by ID for rapid lookup
	groupMap := make(map[string]domain.NodeGroup, len(groups))
	groupFilterMap := make(map[string]domain.NodeFilterSpec, len(groups))
	for _, g := range groups {
		groupMap[g.ID] = g
		gf := groupFilters[g.ID]
		if gf.IsEmpty() && g.NodeFilter != nil {
			gf = *g.NodeFilter
		}
		groupFilterMap[g.ID] = gf
	}

	// Sort globalAdmittedNodes by DisplayName ASC, LogicalID ASC for deterministic dynamic selection
	sortedGlobalNodes := make([]domain.Node, len(globalAdmittedNodes))
	copy(sortedGlobalNodes, globalAdmittedNodes)
	sort.SliceStable(sortedGlobalNodes, func(i, j int) bool {
		if sortedGlobalNodes[i].DisplayName == sortedGlobalNodes[j].DisplayName {
			return sortedGlobalNodes[i].LogicalID < sortedGlobalNodes[j].LogicalID
		}
		return sortedGlobalNodes[i].DisplayName < sortedGlobalNodes[j].DisplayName
	})

	// Sort groups deterministically by Name ASC, then ID ASC
	sortedGroups := make([]domain.NodeGroup, len(groups))
	copy(sortedGroups, groups)
	sort.SliceStable(sortedGroups, func(i, j int) bool {
		if sortedGroups[i].Name == sortedGroups[j].Name {
			return sortedGroups[i].ID < sortedGroups[j].ID
		}
		return sortedGroups[i].Name < sortedGroups[j].Name
	})

	resolvedGroups := make([]ResolvedGroup, len(sortedGroups))
	resolvedGroupMap := make(map[string]*ResolvedGroup, len(sortedGroups))

	for i, g := range sortedGroups {
		edgeList := edges[g.ID]
		// Sort edges by Position ASC
		sortedEdges := make([]domain.GroupEdge, len(edgeList))
		copy(sortedEdges, edgeList)
		sort.SliceStable(sortedEdges, func(a, b int) bool {
			return sortedEdges[a].Position < sortedEdges[b].Position
		})

		var explicitNodeEdges []domain.GroupEdge
		var childGroupEdges []domain.GroupEdge
		for _, e := range sortedEdges {
			if e.NodeLogicalID != nil && *e.NodeLogicalID != "" {
				explicitNodeEdges = append(explicitNodeEdges, e)
			} else if e.ChildGroupID != nil && *e.ChildGroupID != "" {
				childGroupEdges = append(childGroupEdges, e)
			}
		}

		gFilter := groupFilterMap[g.ID]
		members := make([]ResolvedGroupMember, 0, len(sortedEdges))
		childGroupIDs := make([]string, 0)
		nodeLogicalIDs := make([]string, 0)

		if len(explicitNodeEdges) > 0 {
			// Case A: Explicit node edges
			for _, edge := range explicitNodeEdges {
				nodeID := *edge.NodeLogicalID
				if node, ok := globalAdmittedMap[nodeID]; ok {
					if !gFilter.IsEmpty() {
						srcs := nodeSources[nodeID]
						lobs := latestObs[nodeID]
						matched, reason := domain.MatchesFilter(&gFilter, node, srcs, lobs, asOf)
						if matched {
							members = append(members, ResolvedGroupMember{
								Kind:        MemberKindNode,
								TargetID:    nodeID,
								DisplayName: node.DisplayName,
								Position:    len(members),
							})
							nodeLogicalIDs = append(nodeLogicalIDs, nodeID)
						} else {
							diagnostics = append(diagnostics, Diagnostic{
								Severity: DiagnosticSeverityWarning,
								Code:     "group_member_filtered",
								Message:  fmt.Sprintf("group %q explicit member %q (%s) excluded by group filter: %s", g.Name, node.DisplayName, nodeID, reason),
								Target:   nodeID,
								Reason:   reason,
							})
						}
					} else {
						members = append(members, ResolvedGroupMember{
							Kind:        MemberKindNode,
							TargetID:    nodeID,
							DisplayName: node.DisplayName,
							Position:    len(members),
						})
						nodeLogicalIDs = append(nodeLogicalIDs, nodeID)
					}
				} else if origNode, inAdm := admittedNodeMap[nodeID]; inAdm {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: DiagnosticSeverityWarning,
						Code:     "group_member_filtered",
						Message:  fmt.Sprintf("group %q explicit member %q (%s) excluded by global filter", g.Name, origNode.DisplayName, nodeID),
						Target:   nodeID,
						Reason:   "excluded by global filter",
					})
				} else {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: DiagnosticSeverityWarning,
						Code:     "node_inactive_or_missing",
						Message:  fmt.Sprintf("group %q references node %s which is missing, inactive, or excluded", g.Name, nodeID),
						Target:   nodeID,
					})
				}
			}
		} else if len(childGroupEdges) == 0 && !gFilter.IsEmpty() {
			// Case B: Dynamic group dynamically selects from global admitted pool
			for _, node := range sortedGlobalNodes {
				srcs := nodeSources[node.LogicalID]
				lobs := latestObs[node.LogicalID]
				matched, _ := domain.MatchesFilter(&gFilter, node, srcs, lobs, asOf)
				if matched {
					members = append(members, ResolvedGroupMember{
						Kind:        MemberKindNode,
						TargetID:    node.LogicalID,
						DisplayName: node.DisplayName,
						Position:    len(members),
					})
					nodeLogicalIDs = append(nodeLogicalIDs, node.LogicalID)
				}
			}
		}

		// Child group edges
		for _, edge := range childGroupEdges {
			childID := *edge.ChildGroupID
			if childGroup, ok := groupMap[childID]; ok {
				members = append(members, ResolvedGroupMember{
					Kind:        MemberKindGroup,
					TargetID:    childID,
					DisplayName: childGroup.Name,
					Position:    len(members),
				})
				childGroupIDs = append(childGroupIDs, childID)
			}
		}

		rg := ResolvedGroup{
			EmptyFallbackPass: g.EmptyFallbackPass,
			ID:                g.ID,
			Name:              g.Name,
			GroupType:         g.GroupType,
			Members:           members,
			ChildGroupIDs:     childGroupIDs,
			NodeLogicalIDs:    nodeLogicalIDs,
			AllNodeLogicalIDs: nil, // Will compute after all groups initialized
			Position:          i,
		}

		candidate := len(nodeLogicalIDs)
		if len(explicitNodeEdges) > 0 {
			candidate = len(explicitNodeEdges)
		} else if len(childGroupEdges) == 0 && !gFilter.IsEmpty() {
			candidate = len(sortedGlobalNodes)
		}
		excl := candidate - len(nodeLogicalIDs)
		if excl < 0 {
			excl = 0
		}
		groupCounts[g.ID] = GroupFilterCount{
			Candidate: candidate,
			Kept:      len(nodeLogicalIDs),
			Excluded:  excl,
		}

		resolvedGroups[i] = rg
		resolvedGroupMap[g.ID] = &resolvedGroups[i]
	}

	// Nested Group Projection:
	// A parent group's filter restricts all its descendant nodes.
	// If a child group contains nodes that violate the parent's filter,
	// create a derived child group projection restricted by parent's filter.
	var derivedGroups []ResolvedGroup
	for i := range resolvedGroups {
		parent := &resolvedGroups[i]
		pFilter := groupFilterMap[parent.ID]
		if pFilter.IsEmpty() {
			continue
		}

		for mIdx, member := range parent.Members {
			if member.Kind != MemberKindGroup {
				continue
			}
			childID := member.TargetID
			childGroup := resolvedGroupMap[childID]
			if childGroup == nil {
				continue
			}

			var filteredChildMembers []ResolvedGroupMember
			var filteredChildNodeIDs []string
			anyFilteredOut := false

			for _, cMember := range childGroup.Members {
				if cMember.Kind == MemberKindNode {
					cNodeID := cMember.TargetID
					if node, ok := globalAdmittedMap[cNodeID]; ok {
						srcs := nodeSources[cNodeID]
						lobs := latestObs[cNodeID]
						matched, _ := domain.MatchesFilter(&pFilter, node, srcs, lobs, asOf)
						if matched {
							filteredChildMembers = append(filteredChildMembers, ResolvedGroupMember{
								Kind:        MemberKindNode,
								TargetID:    cNodeID,
								DisplayName: node.DisplayName,
								Position:    len(filteredChildMembers),
							})
							filteredChildNodeIDs = append(filteredChildNodeIDs, cNodeID)
						} else {
							anyFilteredOut = true
						}
					} else {
						anyFilteredOut = true
					}
				} else {
					filteredChildMembers = append(filteredChildMembers, cMember)
				}
			}

			if anyFilteredOut {
				derivedID := deterministicDerivedUUID(parent.ID, childID)
				derivedName := fmt.Sprintf("%s [%s]", childGroup.Name, parent.Name)

				derivedGroup := ResolvedGroup{
					ID:                derivedID,
					Name:              derivedName,
					GroupType:         childGroup.GroupType,
					Members:           filteredChildMembers,
					ChildGroupIDs:     childGroup.ChildGroupIDs,
					NodeLogicalIDs:    filteredChildNodeIDs,
					AllNodeLogicalIDs: filteredChildNodeIDs,
					Position:          len(resolvedGroups) + len(derivedGroups),
				}
				derivedGroups = append(derivedGroups, derivedGroup)

				// Update parent's member reference
				parent.Members[mIdx].TargetID = derivedID
				parent.Members[mIdx].DisplayName = derivedName
				for cIdx, cid := range parent.ChildGroupIDs {
					if cid == childID {
						parent.ChildGroupIDs[cIdx] = derivedID
						break
					}
				}
			}
		}
	}

	if len(derivedGroups) > 0 {
		for dIdx := range derivedGroups {
			resolvedGroups = append(resolvedGroups, derivedGroups[dIdx])
			resolvedGroupMap[derivedGroups[dIdx].ID] = &resolvedGroups[len(resolvedGroups)-1]
		}
	}

	// Compute recursive AllNodeLogicalIDs for each group (DAG traversal)
	for i := range resolvedGroups {
		rg := &resolvedGroups[i]
		allNodesMap := make(map[string]bool)

		var collect func(groupID string, visited map[string]bool)
		collect = func(groupID string, visited map[string]bool) {
			if visited[groupID] {
				return
			}
			visited[groupID] = true

			target := resolvedGroupMap[groupID]
			if target == nil {
				return
			}

			for _, nID := range target.NodeLogicalIDs {
				allNodesMap[nID] = true
			}
			for _, cID := range target.ChildGroupIDs {
				collect(cID, visited)
			}
		}

		collect(rg.ID, make(map[string]bool))

		allNodesList := make([]string, 0, len(allNodesMap))
		for nID := range allNodesMap {
			allNodesList = append(allNodesList, nID)
		}
		sort.Strings(allNodesList)
		rg.AllNodeLogicalIDs = allNodesList
		rg.EffectiveEmptyPass = rg.EmptyFallbackPass && (len(rg.Members) == 0 || len(allNodesList) == 0)
	}

	// Empty group & empty routed group diagnostics
	routedGroups := make(map[string]domain.PolicyRule)
	for _, rule := range rules {
		routedGroups[rule.TargetGroupID] = rule
	}

	anyFilterDefined := (globalFilter != nil && !globalFilter.IsEmpty())
	if !anyFilterDefined {
		for _, gf := range groupFilterMap {
			if !gf.IsEmpty() {
				anyFilterDefined = true
				break
			}
		}
	}

	for _, rg := range resolvedGroups {
		if !rg.UsesEmptyPass() && (len(rg.Members) == 0 || len(rg.AllNodeLogicalIDs) == 0) {
			if rule, isRouted := routedGroups[rg.ID]; isRouted {
				msg := fmt.Sprintf("routed group %q (%s) referenced by rule %q has 0 available nodes", rg.Name, rg.ID, rule.Expression)
				if len(globalAdmittedNodes) == 0 {
					msg = fmt.Sprintf("current effective inventory is empty; routed group %q (%s) referenced by rule %q has 0 available nodes", rg.Name, rg.ID, rule.Expression)
				} else if anyFilterDefined {
					msg = fmt.Sprintf("routed group %q (%s) referenced by rule %q has 0 available nodes after filtering", rg.Name, rg.ID, rule.Expression)
				}
				diagnostics = append(diagnostics, Diagnostic{
					Severity: DiagnosticSeverityError,
					Code:     "empty_routed_group",
					Message:  msg,
					Target:   rg.ID,
				})
			} else {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: DiagnosticSeverityWarning,
					Code:     "empty_group",
					Message:  fmt.Sprintf("group %q has 0 active members after resolution", rg.Name),
					Target:   rg.ID,
				})
			}
		}
	}

	return resolvedGroups, diagnostics, groupCounts
}

// resolveRules sorts rules and normalizes terminal MATCH/FINAL rules to the bottom position.
func resolveRules(rules []domain.PolicyRule, groups []domain.NodeGroup) []ResolvedRule {
	groupNameMap := make(map[string]string, len(groups))
	for _, g := range groups {
		groupNameMap[g.ID] = g.Name
	}

	// Sort input rules by Position ASC
	sorted := make([]domain.PolicyRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Position < sorted[j].Position
	})

	standardRules := make([]ResolvedRule, 0, len(rules))
	terminalRules := make([]ResolvedRule, 0, 1)

	for _, r := range sorted {
		rr := ResolvedRule{
			ID:              r.ID,
			TargetGroupID:   r.TargetGroupID,
			TargetGroupName: groupNameMap[r.TargetGroupID],
			Expression:      r.Expression,
			Position:        r.Position,
			IsTerminal:      domain.IsMatchRule(r.Expression),
		}

		if rr.IsTerminal {
			terminalRules = append(terminalRules, rr)
		} else {
			standardRules = append(standardRules, rr)
		}
	}

	// Assemble final list: standard rules first, terminal MATCH rules strictly placed at the bottom
	finalRules := make([]ResolvedRule, 0, len(rules))
	pos := 0
	for _, r := range standardRules {
		r.Position = pos
		finalRules = append(finalRules, r)
		pos++
	}
	for _, r := range terminalRules {
		r.Position = pos
		finalRules = append(finalRules, r)
		pos++
	}

	return finalRules
}
