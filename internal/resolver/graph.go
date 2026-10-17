package resolver

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
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

type resolvedItem struct {
	id          string // "node:<id>", "group:<id>", "builtin:<name>"
	kind        MemberKind
	targetID    string
	displayName string
}

type groupMemo struct {
	members            []ResolvedGroupMember
	childGroupIDs      []string
	nodeLogicalIDs     []string
	expandedNodeIDs    []string
	effectiveEmptyPass bool
	leafItems          []resolvedItem
	leafNodeSet        map[string]bool
	candidateCount     int
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
	groupSources := make(map[string][]domain.MemberSource, len(groups))

	for _, g := range groups {
		groupMap[g.ID] = g

		srcs := append([]domain.MemberSource(nil), g.Sources...)
		sort.SliceStable(srcs, func(a, b int) bool { return srcs[a].Position < srcs[b].Position })
		groupSources[g.ID] = srcs
	}

	// Sort globalAdmittedNodes by DisplayName ASC, LogicalID ASC
	sortedGlobalNodes := make([]domain.Node, len(globalAdmittedNodes))
	copy(sortedGlobalNodes, globalAdmittedNodes)
	sort.SliceStable(sortedGlobalNodes, func(i, j int) bool {
		if sortedGlobalNodes[i].DisplayName == sortedGlobalNodes[j].DisplayName {
			return sortedGlobalNodes[i].LogicalID < sortedGlobalNodes[j].LogicalID
		}
		return sortedGlobalNodes[i].DisplayName < sortedGlobalNodes[j].DisplayName
	})

	// Sort all admitted nodes by DisplayName ASC, LogicalID ASC (used when InheritGlobalFilter is false)
	sortedAdmittedNodes := make([]domain.Node, 0, len(admittedNodeMap))
	for _, n := range admittedNodeMap {
		sortedAdmittedNodes = append(sortedAdmittedNodes, n)
	}
	sort.SliceStable(sortedAdmittedNodes, func(i, j int) bool {
		if sortedAdmittedNodes[i].DisplayName == sortedAdmittedNodes[j].DisplayName {
			return sortedAdmittedNodes[i].LogicalID < sortedAdmittedNodes[j].LogicalID
		}
		return sortedAdmittedNodes[i].DisplayName < sortedAdmittedNodes[j].DisplayName
	})

	// Sort groups deterministically by Position ASC, Name ASC, ID ASC
	sortedGroups := make([]domain.NodeGroup, len(groups))
	copy(sortedGroups, groups)
	sort.SliceStable(sortedGroups, func(i, j int) bool {
		if sortedGroups[i].Position != sortedGroups[j].Position {
			return sortedGroups[i].Position < sortedGroups[j].Position
		}
		if sortedGroups[i].Name != sortedGroups[j].Name {
			return sortedGroups[i].Name < sortedGroups[j].Name
		}
		return sortedGroups[i].ID < sortedGroups[j].ID
	})

	memo := make(map[string]*groupMemo, len(groups))
	resolving := make(map[string]bool, len(groups))

	var resolveGroupMemo func(gid string) *groupMemo
	resolveGroupMemo = func(gid string) *groupMemo {
		if m, ok := memo[gid]; ok {
			return m
		}
		if resolving[gid] {
			return &groupMemo{leafNodeSet: make(map[string]bool)}
		}
		resolving[gid] = true
		defer func() { resolving[gid] = false }()

		g, ok := groupMap[gid]
		if !ok {
			return &groupMemo{leafNodeSet: make(map[string]bool)}
		}

		inheritFilter := g.InheritGlobalFilter
		candidateNodes := sortedGlobalNodes
		candidateMap := globalAdmittedMap
		if !inheritFilter {
			candidateNodes = sortedAdmittedNodes
			candidateMap = admittedNodeMap
		}

		sources := groupSources[gid]
		var currentItems []resolvedItem
		seenInCurrentSet := make(map[string]bool)
		candidateCount := 0

		for _, s := range sources {
			op := s.Operation.Normalize()
			normKind := s.Kind.Normalize()

			if op == domain.OperationAdd {
				switch normKind {
				case domain.SourceKindManualNode:
					candidateCount++
					if s.NodeLogicalID != nil {
						nodeID := *s.NodeLogicalID
						if node, ok := candidateMap[nodeID]; ok {
							if s.FilterSpec != nil {
								srcs := nodeSources[nodeID]
								lobs := latestObs[nodeID]
								matched, reason := domain.MatchesFilter(s.FilterSpec, node, srcs, lobs, asOf)
								if !matched {
									diagnostics = append(diagnostics, Diagnostic{
										Severity: DiagnosticSeverityWarning,
										Code:     "group_member_filtered",
										Message:  fmt.Sprintf("group %q explicit member %q (%s) excluded by source filter: %s", g.Name, node.DisplayName, nodeID, reason),
										Target:   nodeID,
										Reason:   reason,
									})
									continue
								}
							}
							id := "node:" + nodeID
							if !seenInCurrentSet[id] {
								seenInCurrentSet[id] = true
								currentItems = append(currentItems, resolvedItem{id: id, kind: MemberKindNode, targetID: nodeID, displayName: node.DisplayName})
							}
						} else if origNode, inAdm := admittedNodeMap[nodeID]; inAdm && inheritFilter {
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

				case domain.SourceKindGroupRef:
					if s.TargetGroupID != nil {
						targetID := *s.TargetGroupID
						if tg, ok := groupMap[targetID]; ok {
							id := "group:" + targetID
							if !seenInCurrentSet[id] {
								seenInCurrentSet[id] = true
								currentItems = append(currentItems, resolvedItem{id: id, kind: MemberKindGroup, targetID: targetID, displayName: tg.Name})
							}
						} else {
							diagnostics = append(diagnostics, Diagnostic{
								Severity: DiagnosticSeverityWarning,
								Code:     "target_group_not_found",
								Message:  fmt.Sprintf("target group %s not found", targetID),
								Target:   targetID,
							})
						}
					}

				case domain.SourceKindGroupNodes:
					if s.TargetGroupID != nil {
						targetID := *s.TargetGroupID
						if _, ok := groupMap[targetID]; ok {
							childRes := resolveGroupMemo(targetID)
							candidateCount += len(childRes.leafItems)
							for _, leaf := range childRes.leafItems {
								if leaf.kind == MemberKindNode {
									node, ok := candidateMap[leaf.targetID]
									if !ok {
										continue
									}
									if s.Pattern != nil && strings.TrimSpace(*s.Pattern) != "" {
										re, err := domain.CompileFilterRegex(*s.Pattern)
										if err == nil {
											matched, _ := re.Match(node.DisplayName)
											if !matched {
												continue
											}
										}
									}
									if s.FilterSpec != nil {
										srcs := nodeSources[leaf.targetID]
										lobs := latestObs[leaf.targetID]
										matched, _ := domain.MatchesFilter(s.FilterSpec, node, srcs, lobs, asOf)
										if !matched {
											continue
										}
									}
								}
								if !seenInCurrentSet[leaf.id] {
									seenInCurrentSet[leaf.id] = true
									currentItems = append(currentItems, leaf)
								}
							}
						}
					}

				case domain.SourceKindRegex:
					candidateCount += len(candidateNodes)
					if s.Pattern != nil && strings.TrimSpace(*s.Pattern) != "" {
						re, err := domain.CompileFilterRegex(*s.Pattern)
						if err != nil {
							diagnostics = append(diagnostics, Diagnostic{
								Severity: DiagnosticSeverityError,
								Code:     "invalid_regex",
								Message:  fmt.Sprintf("group %q regex invalid: %v", g.Name, err),
							})
							continue
						}
						for _, node := range candidateNodes {
							matched, _ := re.Match(node.DisplayName)
							if matched {
								id := "node:" + node.LogicalID
								if !seenInCurrentSet[id] {
									seenInCurrentSet[id] = true
									currentItems = append(currentItems, resolvedItem{id: id, kind: MemberKindNode, targetID: node.LogicalID, displayName: node.DisplayName})
								}
							}
						}
					}

				case domain.SourceKindFilter:
					candidateCount += len(candidateNodes)
					if s.FilterSpec != nil {
						for _, node := range candidateNodes {
							srcs := nodeSources[node.LogicalID]
							lobs := latestObs[node.LogicalID]
							matched, _ := domain.MatchesFilter(s.FilterSpec, node, srcs, lobs, asOf)
							if matched {
								id := "node:" + node.LogicalID
								if !seenInCurrentSet[id] {
									seenInCurrentSet[id] = true
									currentItems = append(currentItems, resolvedItem{id: id, kind: MemberKindNode, targetID: node.LogicalID, displayName: node.DisplayName})
								}
							}
						}
					}

				case domain.SourceKindTypedBuiltin:
					if s.BuiltinName != nil {
						name := strings.ToUpper(strings.TrimSpace(*s.BuiltinName))
						id := "builtin:" + name
						if !seenInCurrentSet[id] {
							seenInCurrentSet[id] = true
							currentItems = append(currentItems, resolvedItem{id: id, kind: MemberKindBuiltin, targetID: name, displayName: name})
						}
					}
				}
			} else if op == domain.OperationRemove {
				shouldRemove := func(item resolvedItem) bool {
					switch normKind {
					case domain.SourceKindManualNode:
						return s.NodeLogicalID != nil && item.kind == MemberKindNode && item.targetID == *s.NodeLogicalID
					case domain.SourceKindGroupRef:
						return s.TargetGroupID != nil && item.kind == MemberKindGroup && item.targetID == *s.TargetGroupID
					case domain.SourceKindGroupNodes:
						if s.TargetGroupID != nil && item.kind == MemberKindNode {
							childRes := resolveGroupMemo(*s.TargetGroupID)
							return childRes.leafNodeSet[item.targetID]
						}
						return false
					case domain.SourceKindRegex:
						if s.Pattern != nil && item.kind == MemberKindNode {
							re, err := domain.CompileFilterRegex(*s.Pattern)
							if err == nil {
								matched, _ := re.Match(item.displayName)
								return matched
							}
						}
						return false
					case domain.SourceKindFilter:
						if s.FilterSpec != nil && item.kind == MemberKindNode {
							node, ok := globalAdmittedMap[item.targetID]
							if !ok {
								return false
							}
							srcs := nodeSources[item.targetID]
							lobs := latestObs[item.targetID]
							matched, _ := domain.MatchesFilter(s.FilterSpec, node, srcs, lobs, asOf)
							return matched
						}
						return false
					case domain.SourceKindTypedBuiltin:
						return s.BuiltinName != nil && item.kind == MemberKindBuiltin && item.targetID == strings.ToUpper(strings.TrimSpace(*s.BuiltinName))
					default:
						return false
					}
				}

				var remaining []resolvedItem
				for _, item := range currentItems {
					if shouldRemove(item) {
						delete(seenInCurrentSet, item.id) // Crucial: removes seen marker so item can be added back later!
					} else {
						remaining = append(remaining, item)
					}
				}
				currentItems = remaining
			}
		}

		members := make([]ResolvedGroupMember, len(currentItems))
		var childGroupIDs []string
		var nodeLogicalIDs []string
		leafItems := make([]resolvedItem, 0)
		leafNodeSet := make(map[string]bool)

		for idx, item := range currentItems {
			members[idx] = ResolvedGroupMember{
				Kind:        item.kind,
				TargetID:    item.targetID,
				DisplayName: item.displayName,
				Position:    idx,
			}
			switch item.kind {
			case MemberKindGroup:
				childGroupIDs = append(childGroupIDs, item.targetID)
				childRes := resolveGroupMemo(item.targetID)
				var srcFilter *domain.NodeFilterSpec
				for _, src := range g.Sources {
					if src.Kind == domain.SourceKindGroupRef && src.TargetGroupID != nil && *src.TargetGroupID == item.targetID {
						if src.FilterSpec != nil {
							srcFilter = src.FilterSpec
							break
						}
					}
				}
				for _, leaf := range childRes.leafItems {
					if leaf.kind == MemberKindNode && srcFilter != nil {
						node, ok := candidateMap[leaf.targetID]
						if !ok {
							continue
						}
						srcs := nodeSources[leaf.targetID]
						lobs := latestObs[leaf.targetID]
						matched, _ := domain.MatchesFilter(srcFilter, node, srcs, lobs, asOf)
						if !matched {
							continue
						}
					}
					leafItems = append(leafItems, leaf)
					if leaf.kind == MemberKindNode {
						leafNodeSet[leaf.targetID] = true
					}
				}
			case MemberKindNode:
				nodeLogicalIDs = append(nodeLogicalIDs, item.targetID)
				leafItems = append(leafItems, item)
				leafNodeSet[item.targetID] = true
			case MemberKindBuiltin:
				leafItems = append(leafItems, item)
			}
		}

		expandedNodeIDs := make([]string, 0, len(leafNodeSet))
		for nid := range leafNodeSet {
			expandedNodeIDs = append(expandedNodeIDs, nid)
		}
		sort.Strings(expandedNodeIDs)

		hasDestinations := len(expandedNodeIDs) > 0
		if !hasDestinations {
			for _, item := range currentItems {
				if item.kind == MemberKindBuiltin {
					hasDestinations = true
					break
				}
			}
		}

		effectiveEmptyPass := g.EmptyFallbackPass && !hasDestinations

		res := &groupMemo{
			members:            members,
			childGroupIDs:      childGroupIDs,
			nodeLogicalIDs:     nodeLogicalIDs,
			expandedNodeIDs:    expandedNodeIDs,
			effectiveEmptyPass: effectiveEmptyPass,
			leafItems:          leafItems,
			leafNodeSet:        leafNodeSet,
			candidateCount:     candidateCount,
		}
		memo[gid] = res
		return res
	}

	resolvedGroups := make([]ResolvedGroup, len(sortedGroups))
	for i, g := range sortedGroups {
		m := resolveGroupMemo(g.ID)

		rg := ResolvedGroup{
			EmptyFallbackPass:   g.EmptyFallbackPass,
			EffectiveEmptyPass:  m.effectiveEmptyPass,
			InheritGlobalFilter: g.InheritGlobalFilter,
			ID:                  g.ID,
			Name:                g.Name,
			GroupType:          g.GroupType,
			Members:            m.members,
			ChildGroupIDs:      m.childGroupIDs,
			NodeLogicalIDs:     m.nodeLogicalIDs,
			AllNodeLogicalIDs:  m.expandedNodeIDs,
			ExpandedNodeIDs:    m.expandedNodeIDs,
			Sources:            groupSources[g.ID],
			Position:           g.Position,
		}
		resolvedGroups[i] = rg

		candidate := m.candidateCount
		if candidate < len(m.expandedNodeIDs) {
			candidate = len(m.expandedNodeIDs)
		}
		excl := candidate - len(m.expandedNodeIDs)
		if excl < 0 {
			excl = 0
		}
		groupCounts[g.ID] = GroupFilterCount{
			Candidate: candidate,
			Kept:      len(m.expandedNodeIDs),
			Excluded:  excl,
		}
	}

	// Empty group & empty routed group diagnostics
	routedGroups := make(map[string]domain.PolicyRule)
	for _, rule := range rules {
		routedGroups[rule.TargetGroupID] = rule
	}

	resolvedGroupMap := make(map[string]*ResolvedGroup, len(resolvedGroups))
	for i := range resolvedGroups {
		resolvedGroupMap[resolvedGroups[i].ID] = &resolvedGroups[i]
	}

	// A group_ref retains a reference. Item filters project the child's own
	// evaluated leaf set into a derived group without altering the child.
	var derivedGroups []ResolvedGroup
	for i := range resolvedGroups {
		parent := &resolvedGroups[i]
		for mIdx, member := range parent.Members {
			if member.Kind != MemberKindGroup {
				continue
			}
			childID := member.TargetID
			var pFilter *domain.NodeFilterSpec
			for _, s := range groupSources[parent.ID] {
				if s.Operation == domain.OperationAdd && s.Kind == domain.SourceKindGroupRef && s.TargetGroupID != nil && *s.TargetGroupID == childID {
					pFilter = s.FilterSpec
					break
				}
			}
			if pFilter.IsEmpty() {
				continue
			}
			childGroup := resolvedGroupMap[childID]
			if childGroup == nil {
				continue
			}

			parentGroup := groupMap[parent.ID]
			candMap := globalAdmittedMap
			if !parentGroup.InheritGlobalFilter {
				candMap = admittedNodeMap
			}

			var filteredChildMembers []ResolvedGroupMember
			var filteredChildNodeIDs []string
			anyFilteredOut := false

			for _, leaf := range resolveGroupMemo(childID).leafItems {
				cMember := ResolvedGroupMember{Kind: leaf.kind, TargetID: leaf.targetID, DisplayName: leaf.displayName, Position: len(filteredChildMembers)}
				if cMember.Kind == MemberKindNode {
					cNodeID := cMember.TargetID
					if node, ok := candMap[cNodeID]; ok {
						srcs := nodeSources[cNodeID]
						lobs := latestObs[cNodeID]
						matched, _ := domain.MatchesFilter(pFilter, node, srcs, lobs, asOf)
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
					ID:                  derivedID,
					Name:                derivedName,
					GroupType:           childGroup.GroupType,
					InheritGlobalFilter: parentGroup.InheritGlobalFilter,
					Members:             filteredChildMembers,
					ChildGroupIDs:     nil,
					NodeLogicalIDs:    filteredChildNodeIDs,
					AllNodeLogicalIDs: filteredChildNodeIDs,
					ExpandedNodeIDs:   filteredChildNodeIDs,
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

	// Compute recursive AllNodeLogicalIDs for each group
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
		rg.ExpandedNodeIDs = allNodesList
		hasBuiltin := false
		for _, m := range rg.Members {
			if m.Kind == MemberKindBuiltin {
				hasBuiltin = true
				break
			}
		}
		rg.EffectiveEmptyPass = rg.EmptyFallbackPass && len(allNodesList) == 0 && !hasBuiltin
	}

	anyFilterDefined := (globalFilter != nil && !globalFilter.IsEmpty())
	if !anyFilterDefined {
		for _, gf := range groupFilters {
			if !gf.IsEmpty() {
				anyFilterDefined = true
				break
			}
		}
	}

	for _, rg := range resolvedGroups {
		hasDest := len(rg.AllNodeLogicalIDs) > 0
		if !hasDest {
			for _, m := range rg.Members {
				if m.Kind == MemberKindBuiltin {
					hasDest = true
					break
				}
			}
		}
		if !rg.UsesEmptyPass() && (len(rg.Members) == 0 || !hasDest) {
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
