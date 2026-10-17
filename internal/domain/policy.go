package domain

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// NodeGroup represents a policy group in the routing graph.
// AtomicGroupRepository persists group metadata, edges and filter as one unit.
// A nil edges pointer preserves edges; changeFilter distinguishes omission from deletion.
type AtomicGroupRepository interface {
	SaveGroup(context.Context, *NodeGroup, bool, *[]GroupEdge, bool, *NodeFilterSpec) error
}

type NodeGroup struct {
	EmptyFallbackPass bool            `json:"empty_fallback_pass"`
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	GroupType         GroupType       `json:"group_type"`
	Position          int             `json:"position"`
	Revision          int64           `json:"revision"`
	Sources           []MemberSource  `json:"sources,omitempty"`
	NodeFilter        *NodeFilterSpec `json:"node_filter,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// SourceOperation defines candidate additions and removals.
type SourceOperation string

const (
	OperationAdd    SourceOperation = "add"
	OperationRemove SourceOperation = "remove"
)

func (op SourceOperation) Normalize() SourceOperation {
	return op
}

func (op SourceOperation) IsValid() bool {
	return op == OperationAdd || op == OperationRemove
}

// MemberSourceKind defines the kind of member source item.
type MemberSourceKind string

const (
	SourceKindManualNode   MemberSourceKind = "manual_node"
	SourceKindGroupRef     MemberSourceKind = "group_ref"
	SourceKindGroupNodes   MemberSourceKind = "group_nodes"
	SourceKindRegex        MemberSourceKind = "regex"
	SourceKindFilter       MemberSourceKind = "filter"
	SourceKindTypedBuiltin MemberSourceKind = "typed_builtin"
)

func (k MemberSourceKind) Normalize() MemberSourceKind {
	return k
}

func (k MemberSourceKind) IsValid() bool {
	switch k {
	case SourceKindManualNode, SourceKindGroupRef, SourceKindGroupNodes,
		SourceKindRegex, SourceKindFilter, SourceKindTypedBuiltin:
		return true
	default:
		return false
	}
}

// MemberSource represents an ordered candidate member operation inside a policy group.
type MemberSource struct {
	ID        string           `json:"id"`
	GroupID   string           `json:"group_id"`
	Operation SourceOperation  `json:"operation"`
	Kind      MemberSourceKind `json:"kind"`
	Position  int              `json:"position"`

	NodeLogicalID *string         `json:"node_logical_id,omitempty"`
	TargetGroupID *string         `json:"target_group_id,omitempty"`
	Pattern       *string         `json:"pattern,omitempty"`
	FilterSpec    *NodeFilterSpec `json:"filter_spec,omitempty"`
	BuiltinName   *string         `json:"builtin_name,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ValidateMemberSource(s MemberSource) error {
	if !s.Operation.IsValid() {
		return NewValidationError("invalid_source_operation", fmt.Sprintf("invalid source operation: %s", s.Operation))
	}
	if !s.Kind.IsValid() {
		return NewValidationError("invalid_source_kind", fmt.Sprintf("invalid source kind: %s", s.Kind))
	}
	if s.Position < 0 {
		return NewValidationError("invalid_position", "source position must be non-negative")
	}

	normKind := s.Kind.Normalize()
	switch normKind {
	case SourceKindManualNode:
		if s.NodeLogicalID == nil || strings.TrimSpace(*s.NodeLogicalID) == "" {
			err := NewValidationError("invalid_source_entry", "manual_node requires non-empty node_logical_id")
			err.Field = fmt.Sprintf("sources[%d].node_logical_id", s.Position)
			return err
		}
		if !IsValidLogicalID(*s.NodeLogicalID) {
			return NewValidationError("invalid_node_logical_id", fmt.Sprintf("invalid node logical ID: %s", *s.NodeLogicalID))
		}
		if s.TargetGroupID != nil || s.BuiltinName != nil {
			err := NewValidationError("invalid_source_entry", "manual_node cannot specify target_group_id or builtin_name")
			err.Field = fmt.Sprintf("sources[%d]", s.Position)
			return err
		}

	case SourceKindGroupRef:
		if s.TargetGroupID == nil || strings.TrimSpace(*s.TargetGroupID) == "" {
			err := NewValidationError("invalid_source_entry", "Source kind 'group_ref' requires non-empty target_group_id and pattern must be empty")
			err.Field = fmt.Sprintf("sources[%d].target_group_id", s.Position)
			return err
		}
		if !IsValidUUIDv7(*s.TargetGroupID) {
			return NewValidationError("invalid_target_group_id", fmt.Sprintf("invalid target group UUIDv7: %s", *s.TargetGroupID))
		}
		if s.NodeLogicalID != nil || s.BuiltinName != nil || (s.Pattern != nil && strings.TrimSpace(*s.Pattern) != "") {
			err := NewValidationError("invalid_source_entry", "Source kind 'group_ref' requires non-empty target_group_id and pattern must be empty")
			err.Field = fmt.Sprintf("sources[%d].target_group_id", s.Position)
			return err
		}

	case SourceKindGroupNodes:
		if s.TargetGroupID == nil || strings.TrimSpace(*s.TargetGroupID) == "" {
			err := NewValidationError("invalid_source_entry", "group_nodes requires non-empty target_group_id")
			err.Field = fmt.Sprintf("sources[%d].target_group_id", s.Position)
			return err
		}
		if !IsValidUUIDv7(*s.TargetGroupID) {
			return NewValidationError("invalid_target_group_id", fmt.Sprintf("invalid target group UUIDv7: %s", *s.TargetGroupID))
		}
		if s.NodeLogicalID != nil || s.BuiltinName != nil {
			err := NewValidationError("invalid_source_entry", "group_nodes cannot specify node_logical_id or builtin_name")
			err.Field = fmt.Sprintf("sources[%d]", s.Position)
			return err
		}
		if s.Pattern != nil && strings.TrimSpace(*s.Pattern) != "" {
			if _, err := CompileFilterRegex(*s.Pattern); err != nil {
				return NewValidationError("invalid_regex", fmt.Sprintf("invalid group_nodes pattern: %v", err))
			}
		}
		if s.FilterSpec != nil {
			if err := s.FilterSpec.Validate(); err != nil {
				return NewValidationError("invalid_filter_spec", fmt.Sprintf("invalid group_nodes filter_spec: %v", err))
			}
		}

	case SourceKindRegex:
		if s.Pattern == nil || strings.TrimSpace(*s.Pattern) == "" {
			err := NewValidationError("invalid_source_entry", "regex requires non-empty pattern")
			err.Field = fmt.Sprintf("sources[%d].pattern", s.Position)
			return err
		}
		if _, err := CompileFilterRegex(*s.Pattern); err != nil {
			return NewValidationError("invalid_regex", fmt.Sprintf("invalid regex pattern: %v", err))
		}
		if s.NodeLogicalID != nil || s.TargetGroupID != nil || s.BuiltinName != nil {
			err := NewValidationError("invalid_source_entry", "regex cannot specify node_logical_id, target_group_id, or builtin_name")
			err.Field = fmt.Sprintf("sources[%d]", s.Position)
			return err
		}

	case SourceKindFilter:
		if s.FilterSpec == nil {
			err := NewValidationError("invalid_source_entry", "filter requires non-empty filter_spec")
			err.Field = fmt.Sprintf("sources[%d].filter_spec", s.Position)
			return err
		}
		if err := s.FilterSpec.Validate(); err != nil {
			return NewValidationError("invalid_filter_spec", fmt.Sprintf("invalid filter_spec: %v", err))
		}
		if s.NodeLogicalID != nil || s.TargetGroupID != nil || s.BuiltinName != nil {
			err := NewValidationError("invalid_source_entry", "filter cannot specify node_logical_id, target_group_id, or builtin_name")
			err.Field = fmt.Sprintf("sources[%d]", s.Position)
			return err
		}

	case SourceKindTypedBuiltin:
		if s.BuiltinName == nil || strings.TrimSpace(*s.BuiltinName) == "" {
			err := NewValidationError("invalid_source_entry", "typed_builtin requires non-empty builtin_name")
			err.Field = fmt.Sprintf("sources[%d].builtin_name", s.Position)
			return err
		}
		name := strings.ToUpper(strings.TrimSpace(*s.BuiltinName))
		if name != "PASS" && name != "DIRECT" && name != "REJECT" && name != "REJECT-DROP" {
			err := NewValidationError("invalid_source_entry", fmt.Sprintf("unsupported builtin_name: %s", *s.BuiltinName))
			err.Field = fmt.Sprintf("sources[%d].builtin_name", s.Position)
			return err
		}
		if s.NodeLogicalID != nil || s.TargetGroupID != nil || s.Pattern != nil || s.FilterSpec != nil {
			err := NewValidationError("invalid_source_entry", "typed_builtin cannot specify node_logical_id, target_group_id, pattern, or filter_spec")
			err.Field = fmt.Sprintf("sources[%d]", s.Position)
			return err
		}
	}

	return nil
}

// GroupEdge represents a directed edge in the policy graph.
// Strictly prevents self-loops: ParentGroupID cannot be equal to ChildGroupID.
type GroupEdge struct {
	ID            string  `json:"id"`
	ParentGroupID string  `json:"parent_group_id"`
	ChildGroupID  *string `json:"child_group_id,omitempty"`
	NodeLogicalID *string `json:"node_logical_id,omitempty"`
	Position      int     `json:"position"`
}

// ValidateGroupEdge enforces graph safety invariants:
// 1. ParentGroupID must be valid UUIDv7.
// 2. Exactly one of ChildGroupID or NodeLogicalID must be specified.
// 3. ChildGroupID must not equal ParentGroupID (strictly no self-loops).
// 4. Position must be non-negative.
func ValidateGroupEdge(edge GroupEdge) error {
	if !IsValidUUIDv7(edge.ParentGroupID) {
		return NewValidationError("invalid_parent_group_id", fmt.Sprintf("invalid parent group UUIDv7: %s", edge.ParentGroupID))
	}

	hasChild := edge.ChildGroupID != nil && *edge.ChildGroupID != ""
	hasNode := edge.NodeLogicalID != nil && *edge.NodeLogicalID != ""

	if !hasChild && !hasNode {
		return NewValidationError("empty_edge_target", "group edge must specify either child_group_id or node_logical_id")
	}

	if hasChild && hasNode {
		return NewValidationError("ambiguous_edge_target", "group edge cannot specify both child_group_id and node_logical_id")
	}

	if hasChild {
		if !IsValidUUIDv7(*edge.ChildGroupID) {
			return NewValidationError("invalid_child_group_id", fmt.Sprintf("invalid child group UUIDv7: %s", *edge.ChildGroupID))
		}
		if edge.ParentGroupID == *edge.ChildGroupID {
			return NewValidationError("self_loop_forbidden", fmt.Sprintf("self-loop detected: parent group %s cannot reference itself", edge.ParentGroupID))
		}
	}

	if hasNode {
		if !IsValidLogicalID(*edge.NodeLogicalID) {
			return NewValidationError("invalid_node_logical_id", fmt.Sprintf("invalid node logical ID: %s", *edge.NodeLogicalID))
		}
	}

	if edge.Position < 0 {
		return NewValidationError("invalid_position", "edge position must be non-negative")
	}

	return nil
}

// AdmissionRule defines criteria for filtering and classifying imported nodes.
type AdmissionRule struct {
	ID         string     `json:"id"`
	RevisionID string     `json:"revision_id"`
	Name       string     `json:"name"`
	Expression string     `json:"expression"`
	Action     RuleAction `json:"action"`
	Position   int        `json:"position"`
}

// PolicyRule defines routing rules that steer traffic matching an expression to a target group.
type PolicyRule struct {
	ID            string `json:"id"`
	RevisionID    string `json:"revision_id"`
	TargetGroupID string `json:"target_group_id"`
	Expression    string `json:"expression"`
	Position      int    `json:"position"`
}

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

// ValidatePolicyTopology checks the policy graph for self-loops, cycles, missing group references, and rule constraints.
// When requireTerminalMatch is true, a single MATCH/FINAL rule must also be positioned at the end of the sorted rule list.
func ValidatePolicyTopology(
	groups []NodeGroup,
	edges map[string][]GroupEdge,
	rules []PolicyRule,
	requireTerminalMatch bool,
) error {
	groupSet := make(map[string]bool, len(groups))
	for _, g := range groups {
		if !IsValidUUIDv7(g.ID) {
			return NewValidationError("invalid_group_id", fmt.Sprintf("group ID %s is not valid UUIDv7", g.ID))
		}
		if strings.TrimSpace(g.Name) == "" {
			return NewValidationError("invalid_group_name", "group name cannot be empty")
		}
		if !g.GroupType.IsValid() {
			return NewValidationError("invalid_group_type", fmt.Sprintf("unsupported group type: %s", g.GroupType))
		}
		groupSet[g.ID] = true
	}

	adj := make(map[string][]string, len(groups))
	for _, g := range groups {
		adj[g.ID] = make([]string, 0)
	}

	for parentID, edgeList := range edges {
		if !groupSet[parentID] {
			return NewValidationError("parent_group_not_found", fmt.Sprintf("parent group %s does not exist in policy graph", parentID))
		}

		for _, edge := range edgeList {
			edge.ParentGroupID = parentID
			if err := ValidateGroupEdge(edge); err != nil {
				return err
			}

			if edge.ChildGroupID != nil && *edge.ChildGroupID != "" {
				childID := *edge.ChildGroupID
				if childID == parentID {
					return NewValidationError("self_loop_forbidden", fmt.Sprintf("self-loop detected: parent group %s cannot reference itself", parentID))
				}
				if !groupSet[childID] {
					return NewValidationError("target_group_not_found", fmt.Sprintf("target child group %s does not exist in policy graph", childID))
				}
				adj[parentID] = append(adj[parentID], childID)
			}
		}
	}

	for _, g := range groups {
		if len(g.Sources) > 0 {
			for _, src := range g.Sources {
				if err := ValidateMemberSource(src); err != nil {
					return err
				}
				normKind := src.Kind.Normalize()
				if normKind == SourceKindGroupRef || normKind == SourceKindGroupNodes {
					if src.TargetGroupID != nil && *src.TargetGroupID != "" {
						childID := *src.TargetGroupID
						if childID == g.ID {
							return NewValidationError("self_loop_forbidden", fmt.Sprintf("self-loop detected: parent group %s cannot reference itself", g.ID))
						}
						if !groupSet[childID] {
							return NewValidationError("target_group_not_found", fmt.Sprintf("target child group %s does not exist in policy graph", childID))
						}
						adj[g.ID] = append(adj[g.ID], childID)
					}
				}
			}
		}
	}

	// Cycle detection using 3-color DFS (0 = white, 1 = gray, 2 = black)
	color := make(map[string]int, len(groups))
	var stack []string

	groupIDs := make([]string, 0, len(groups))
	for _, g := range groups {
		groupIDs = append(groupIDs, g.ID)
	}
	sort.Strings(groupIDs)

	var dfs func(u string) error
	dfs = func(u string) error {
		color[u] = 1
		stack = append(stack, u)

		neighbors := make([]string, len(adj[u]))
		copy(neighbors, adj[u])
		sort.Strings(neighbors)

		for _, v := range neighbors {
			if color[v] == 1 {
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
				domErr := NewValidationError("cycle_detected", fmt.Sprintf("cycle detected in policy graph: %s", pathStr))
				domErr.CyclePath = cyclePath
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

	return ValidatePolicyRules(rules, groupSet, requireTerminalMatch)
}

// ValidatePolicyRules verifies rule expressions, positions, target group references, and MATCH rule constraints.
func ValidatePolicyRules(rules []PolicyRule, groupSet map[string]bool, requireTerminalMatch bool) error {
	if len(rules) == 0 {
		return nil
	}

	sorted := make([]PolicyRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Position < sorted[j].Position
	})

	var matchIndices []int
	for i, rule := range sorted {
		if strings.TrimSpace(rule.Expression) == "" {
			return NewValidationError("invalid_expression", "policy rule expression cannot be empty")
		}
		if rule.Position < 0 {
			return NewValidationError("invalid_position", "policy rule position must be non-negative")
		}
		if len(groupSet) > 0 && !groupSet[rule.TargetGroupID] {
			return NewValidationError("target_group_not_found", fmt.Sprintf("target group %s does not exist for policy rule", rule.TargetGroupID))
		}
		if IsMatchRule(rule.Expression) {
			matchIndices = append(matchIndices, i)
		}
	}

	if len(matchIndices) > 1 {
		return NewValidationError("invalid_match_rule_ordering", fmt.Sprintf("multiple MATCH termination rules found at positions %d and %d", sorted[matchIndices[0]].Position, sorted[matchIndices[1]].Position))
	}

	if requireTerminalMatch && len(matchIndices) == 1 {
		matchIdx := matchIndices[0]
		if matchIdx != len(sorted)-1 {
			subsequentCount := len(sorted) - 1 - matchIdx
			return NewValidationError("invalid_match_rule_ordering", fmt.Sprintf("MATCH rule at position %d must be the terminal rule; found %d subsequent rules following it", sorted[matchIdx].Position, subsequentCount))
		}
	}

	return nil
}
