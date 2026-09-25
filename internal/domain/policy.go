package domain

import (
	"fmt"
	"time"
)

// NodeGroup represents a policy group in the routing graph.
type NodeGroup struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	GroupType  GroupType       `json:"group_type"`
	NodeFilter *NodeFilterSpec `json:"node_filter,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
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
