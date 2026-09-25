package resolver

import (
	"fmt"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
)

// validateTopology checks the policy graph for self-loops, cycles, missing group references, and rule constraints.
func validateTopology(
	groups []domain.NodeGroup,
	edges map[string][]domain.GroupEdge,
	rules []domain.PolicyRule,
) error {
	groupSet := make(map[string]bool, len(groups))
	for _, g := range groups {
		if !domain.IsValidUUIDv7(g.ID) {
			return domain.NewValidationError("invalid_group_id", fmt.Sprintf("group ID %s is not valid UUIDv7", g.ID))
		}
		if strings.TrimSpace(g.Name) == "" {
			return domain.NewValidationError("invalid_group_name", "group name cannot be empty")
		}
		if !g.GroupType.IsValid() {
			return domain.NewValidationError("invalid_group_type", fmt.Sprintf("unsupported group type: %s", g.GroupType))
		}
		groupSet[g.ID] = true
	}

	// Check self-loops and missing child group references
	adj := make(map[string][]string, len(groups))
	for _, g := range groups {
		adj[g.ID] = make([]string, 0)
	}

	for parentID, edgeList := range edges {
		if !groupSet[parentID] {
			return domain.NewValidationError("parent_group_not_found", fmt.Sprintf("parent group %s does not exist", parentID))
		}

		for _, edge := range edgeList {
			edge.ParentGroupID = parentID
			if err := domain.ValidateGroupEdge(edge); err != nil {
				return err
			}

			if edge.ChildGroupID != nil && *edge.ChildGroupID != "" {
				childID := *edge.ChildGroupID
				if childID == parentID {
					return domain.NewValidationError("self_loop_forbidden", fmt.Sprintf("self-loop detected: group %s references itself", parentID))
				}
				if !groupSet[childID] {
					return domain.NewValidationError("target_group_not_found", fmt.Sprintf("target group %s does not exist", childID))
				}
				adj[parentID] = append(adj[parentID], childID)
			}
		}
	}

	// 3-color DFS cycle detection with explicit stack
	// Colors: 0 = unvisited (white), 1 = visiting (gray), 2 = visited (black)
	color := make(map[string]int, len(groups))
	var stack []string

	// Sort group IDs for deterministic cycle detection
	sortedGroupIDs := make([]string, 0, len(groups))
	for _, g := range groups {
		sortedGroupIDs = append(sortedGroupIDs, g.ID)
	}
	sort.Strings(sortedGroupIDs)

	var dfs func(u string) error
	dfs = func(u string) error {
		color[u] = 1
		stack = append(stack, u)

		// Sort neighbors deterministically
		neighbors := make([]string, len(adj[u]))
		copy(neighbors, adj[u])
		sort.Strings(neighbors)

		for _, v := range neighbors {
			if color[v] == 1 {
				// Cycle detected: extract path from stack
				idx := -1
				for i, node := range stack {
					if node == v {
						idx = i
						break
					}
				}
				var cyclePath []string
				if idx >= 0 {
					cyclePath = append(cyclePath, stack[idx:]...)
					cyclePath = append(cyclePath, v)
				} else {
					cyclePath = []string{u, v, u}
				}
				pathStr := strings.Join(cyclePath, " -> ")
				domErr := domain.NewValidationError("cycle_detected", fmt.Sprintf("cycle detected in policy graph: %s", pathStr))
				domErr.Details = map[string]string{
					"cycle":    pathStr,
					"group_id": v,
				}
				return domErr
			}
			if color[v] == 0 {
				if err := dfs(v); err != nil {
					return err
				}
			}
		}

		stack = stack[:len(stack)-1]
		color[u] = 2
		return nil
	}

	for _, id := range sortedGroupIDs {
		if color[id] == 0 {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}

	// Validate rule references and match rule ordering
	matchCount := 0
	for _, rule := range rules {
		if strings.TrimSpace(rule.Expression) == "" {
			return domain.NewValidationError("invalid_expression", "rule expression cannot be empty")
		}
		if rule.Position < 0 {
			return domain.NewValidationError("invalid_position", "rule position must be non-negative")
		}
		if len(groupSet) > 0 && !groupSet[rule.TargetGroupID] {
			return domain.NewValidationError("target_group_not_found", fmt.Sprintf("rule target group %s does not exist", rule.TargetGroupID))
		}
		if isMatchRule(rule.Expression) {
			matchCount++
		}
	}

	if matchCount > 1 {
		return domain.NewValidationError("invalid_match_rule_ordering", "multiple terminal MATCH rules detected")
	}

	return nil
}

// resolveGroups constructs resolved groups with ordered members, child group links, and recursive node expansion.
func resolveGroups(
	groups []domain.NodeGroup,
	edges map[string][]domain.GroupEdge,
	admittedNodeMap map[string]domain.Node,
) ([]ResolvedGroup, []Diagnostic) {
	diagnostics := make([]Diagnostic, 0)

	// Map groups by ID for rapid lookup
	groupMap := make(map[string]domain.NodeGroup, len(groups))
	for _, g := range groups {
		groupMap[g.ID] = g
	}

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

		members := make([]ResolvedGroupMember, 0, len(sortedEdges))
		childGroupIDs := make([]string, 0)
		nodeLogicalIDs := make([]string, 0)

		for pos, edge := range sortedEdges {
			if edge.NodeLogicalID != nil && *edge.NodeLogicalID != "" {
				nodeID := *edge.NodeLogicalID
				if node, ok := admittedNodeMap[nodeID]; ok {
					members = append(members, ResolvedGroupMember{
						Kind:        MemberKindNode,
						TargetID:    nodeID,
						DisplayName: node.DisplayName,
						Position:    pos,
					})
					nodeLogicalIDs = append(nodeLogicalIDs, nodeID)
				} else {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: DiagnosticSeverityWarning,
						Code:     "node_inactive_or_missing",
						Message:  fmt.Sprintf("group %q references node %s which is missing, inactive, or excluded", g.Name, nodeID),
						Target:   nodeID,
					})
				}
			} else if edge.ChildGroupID != nil && *edge.ChildGroupID != "" {
				childID := *edge.ChildGroupID
				if childGroup, ok := groupMap[childID]; ok {
					members = append(members, ResolvedGroupMember{
						Kind:        MemberKindGroup,
						TargetID:    childID,
						DisplayName: childGroup.Name,
						Position:    pos,
					})
					childGroupIDs = append(childGroupIDs, childID)
				}
			}
		}

		rg := ResolvedGroup{
			ID:                g.ID,
			Name:              g.Name,
			GroupType:         g.GroupType,
			Members:           members,
			ChildGroupIDs:     childGroupIDs,
			NodeLogicalIDs:    nodeLogicalIDs,
			AllNodeLogicalIDs: nil, // Will compute after all groups initialized
			Position:          i,
		}

		if len(members) == 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: DiagnosticSeverityWarning,
				Code:     "empty_group",
				Message:  fmt.Sprintf("group %q has 0 active members after resolution", g.Name),
				Target:   g.ID,
			})
		}

		resolvedGroups[i] = rg
		resolvedGroupMap[g.ID] = &resolvedGroups[i]
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
	}

	return resolvedGroups, diagnostics
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
			IsTerminal:      isMatchRule(r.Expression),
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

// isMatchRule returns whether an expression represents a terminal MATCH or FINAL routing rule.
func isMatchRule(expr string) bool {
	norm := strings.ToUpper(strings.TrimSpace(expr))
	return norm == "MATCH" ||
		strings.HasPrefix(norm, "MATCH,") ||
		strings.HasPrefix(norm, "MATCH ") ||
		norm == "FINAL" ||
		strings.HasPrefix(norm, "FINAL,") ||
		strings.HasPrefix(norm, "FINAL ")
}
