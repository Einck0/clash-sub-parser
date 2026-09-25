package policy

import (
	"fmt"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
)

// IsMatchRule checks if a routing rule expression represents a MATCH/FINAL termination catch-all rule.
func IsMatchRule(expr string) bool {
	norm := strings.ToUpper(strings.TrimSpace(expr))
	return norm == "MATCH" ||
		strings.HasPrefix(norm, "MATCH,") ||
		strings.HasPrefix(norm, "MATCH ") ||
		norm == "FINAL" ||
		strings.HasPrefix(norm, "FINAL,") ||
		strings.HasPrefix(norm, "FINAL ")
}

// ValidatePolicyGraph performs topological, cycle, self-loop, target existence, and rule order validation.
func ValidatePolicyGraph(
	groups []domain.NodeGroup,
	edges map[string][]domain.GroupEdge,
	policyRules []domain.PolicyRule,
	admissionRules []domain.AdmissionRule,
) error {
	groupSet := make(map[string]bool, len(groups))
	groupNames := make(map[string]string, len(groups))
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
		groupNames[g.ID] = g.Name
	}

	// Build adjacency list for cycle detection
	adj := make(map[string][]string, len(groups))
	for _, g := range groups {
		adj[g.ID] = make([]string, 0)
	}

	for parentID, edgeList := range edges {
		if !groupSet[parentID] {
			return domain.NewValidationError("parent_group_not_found", fmt.Sprintf("parent group %s does not exist in policy graph", parentID))
		}

		for _, edge := range edgeList {
			edge.ParentGroupID = parentID
			if err := domain.ValidateGroupEdge(edge); err != nil {
				return err
			}

			if edge.ChildGroupID != nil && *edge.ChildGroupID != "" {
				childID := *edge.ChildGroupID
				if childID == parentID {
					return domain.NewValidationError("self_loop_forbidden", fmt.Sprintf("self-loop detected: parent group %s cannot reference itself", parentID))
				}
				if !groupSet[childID] {
					return domain.NewValidationError("target_group_not_found", fmt.Sprintf("target child group %s does not exist in policy graph", childID))
				}
				adj[parentID] = append(adj[parentID], childID)
			}
		}
	}

	// Cycle detection using 3-color DFS
	// Colors: 0 = white (unvisited), 1 = gray (in current stack), 2 = black (completed)
	color := make(map[string]int, len(groups))
	var stack []string

	// Sort group IDs deterministically
	groupIDs := make([]string, 0, len(groups))
	for _, g := range groups {
		groupIDs = append(groupIDs, g.ID)
	}
	sort.Strings(groupIDs)

	var dfs func(u string) error
	dfs = func(u string) error {
		color[u] = 1
		stack = append(stack, u)

		// Sort neighbors for deterministic cycle reporting
		neighbors := make([]string, len(adj[u]))
		copy(neighbors, adj[u])
		sort.Strings(neighbors)

		for _, v := range neighbors {
			if color[v] == 1 {
				// Cycle detected!
				idx := -1
				for i, s := range stack {
					if s == v {
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

	for _, id := range groupIDs {
		if color[id] == 0 {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}

	// Validate Policy Rules
	if err := ValidatePolicyRules(policyRules, groupSet); err != nil {
		return err
	}

	// Validate Admission Rules
	if err := ValidateAdmissionRules(admissionRules); err != nil {
		return err
	}

	return nil
}

// ValidatePolicyRules verifies rule ordering, target group references, and termination rules.
func ValidatePolicyRules(rules []domain.PolicyRule, groupSet map[string]bool) error {
	if len(rules) == 0 {
		return nil
	}

	sorted := make([]domain.PolicyRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Position < sorted[j].Position
	})

	var matchIndices []int
	for i, rule := range sorted {
		if strings.TrimSpace(rule.Expression) == "" {
			return domain.NewValidationError("invalid_expression", "policy rule expression cannot be empty")
		}
		if rule.Position < 0 {
			return domain.NewValidationError("invalid_position", "policy rule position must be non-negative")
		}

		if groupSet != nil && len(groupSet) > 0 {
			if !groupSet[rule.TargetGroupID] {
				return domain.NewValidationError("target_group_not_found", fmt.Sprintf("target group %s does not exist for policy rule", rule.TargetGroupID))
			}
		}

		if IsMatchRule(rule.Expression) {
			matchIndices = append(matchIndices, i)
		}
	}

	if len(matchIndices) > 1 {
		return domain.NewValidationError("invalid_match_rule_ordering", fmt.Sprintf("multiple MATCH termination rules found at positions %d and %d", sorted[matchIndices[0]].Position, sorted[matchIndices[1]].Position))
	}

	if len(matchIndices) == 1 {
		matchIdx := matchIndices[0]
		if matchIdx != len(sorted)-1 {
			subsequentCount := len(sorted) - 1 - matchIdx
			return domain.NewValidationError("invalid_match_rule_ordering", fmt.Sprintf("MATCH rule at position %d must be the terminal rule; found %d subsequent rules following it", sorted[matchIdx].Position, subsequentCount))
		}
	}

	return nil
}

// ValidateAdmissionRules checks field constraints on admission rules.
func ValidateAdmissionRules(rules []domain.AdmissionRule) error {
	for _, rule := range rules {
		if strings.TrimSpace(rule.Name) == "" {
			return domain.NewValidationError("invalid_admission_rule_name", "admission rule name cannot be empty")
		}
		if strings.TrimSpace(rule.Expression) == "" {
			return domain.NewValidationError("invalid_expression", "admission rule expression cannot be empty")
		}
		if !rule.Action.IsValid() {
			return domain.NewValidationError("invalid_rule_action", fmt.Sprintf("unsupported rule action: %s", rule.Action))
		}
		if rule.Position < 0 {
			return domain.NewValidationError("invalid_position", "admission rule position must be non-negative")
		}
	}
	return nil
}
