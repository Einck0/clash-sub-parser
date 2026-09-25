package policy

import (
	"time"

	"clash-sub-parser/internal/domain"
)

// EdgeInput defines input for a group edge.
type EdgeInput struct {
	ID            string  `json:"id,omitempty"`
	ChildGroupID  *string `json:"child_group_id,omitempty"`
	NodeLogicalID *string `json:"node_logical_id,omitempty"`
	Position      int     `json:"position"`
}

// CreateGroupCommand defines parameters for creating a node group.
type CreateGroupCommand struct {
	ID         string                 `json:"id,omitempty"`
	Name       string                 `json:"name"`
	GroupType  domain.GroupType       `json:"group_type"`
	Edges      []EdgeInput            `json:"edges,omitempty"`
	NodeFilter *domain.NodeFilterSpec `json:"node_filter,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	ActorKind  domain.ActorKind       `json:"actor_kind,omitempty"`
}

// UpdateGroupCommand defines parameters for updating a node group.
type UpdateGroupCommand struct {
	ID              string                 `json:"id"`
	Name            *string                `json:"name,omitempty"`
	GroupType       *domain.GroupType      `json:"group_type,omitempty"`
	Edges           *[]EdgeInput           `json:"edges,omitempty"`
	NodeFilter      *domain.NodeFilterSpec `json:"node_filter,omitempty"`
	ClearNodeFilter bool                   `json:"clear_node_filter,omitempty"`
	RequestID       string                 `json:"request_id,omitempty"`
	ActorKind       domain.ActorKind       `json:"actor_kind,omitempty"`
}

// DeleteGroupCommand defines parameters for deleting a node group.
type DeleteGroupCommand struct {
	ID        string           `json:"id"`
	RequestID string           `json:"request_id,omitempty"`
	ActorKind domain.ActorKind `json:"actor_kind,omitempty"`
}

// SetGlobalNodeFilterCommand defines parameters for updating the global node filter.
type SetGlobalNodeFilterCommand struct {
	Spec      domain.NodeFilterSpec `json:"spec"`
	RequestID string                `json:"request_id,omitempty"`
	ActorKind domain.ActorKind      `json:"actor_kind,omitempty"`
}
// SetGroupEdgesCommand defines parameters for setting all edges of a group.
type SetGroupEdgesCommand struct {
	ParentGroupID string           `json:"parent_group_id"`
	Edges         []EdgeInput      `json:"edges"`
	RequestID     string           `json:"request_id,omitempty"`
	ActorKind     domain.ActorKind `json:"actor_kind,omitempty"`
}

// GroupEdgeView is the view representation of a group edge.
type GroupEdgeView struct {
	ID            string  `json:"id"`
	ParentGroupID string  `json:"parent_group_id"`
	ChildGroupID  *string `json:"child_group_id,omitempty"`
	NodeLogicalID *string `json:"node_logical_id,omitempty"`
	Position      int     `json:"position"`
}

// GroupView is the view representation of a policy group.
type GroupView struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	GroupType  domain.GroupType       `json:"group_type"`
	Edges      []GroupEdgeView        `json:"edges"`
	NodeFilter *domain.NodeFilterSpec `json:"node_filter,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// ListGroupsQuery defines pagination and search filters for listing groups.
type ListGroupsQuery struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Search   string `json:"search,omitempty"`
}

// ListGroupsResult is the paginated response for groups.
type ListGroupsResult struct {
	Items    []GroupView `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int         `json:"total"`
}

// CreateAdmissionRuleCommand defines parameters for creating an admission rule.
type CreateAdmissionRuleCommand struct {
	ID         string            `json:"id,omitempty"`
	RevisionID string            `json:"revision_id"`
	Name       string            `json:"name"`
	Expression string            `json:"expression"`
	Action     domain.RuleAction `json:"action"`
	Position   int               `json:"position"`
	RequestID  string            `json:"request_id,omitempty"`
	ActorKind  domain.ActorKind  `json:"actor_kind,omitempty"`
}

// AdmissionRuleView is the view representation of an admission rule.
type AdmissionRuleView struct {
	ID         string            `json:"id"`
	RevisionID string            `json:"revision_id"`
	Name       string            `json:"name"`
	Expression string            `json:"expression"`
	Action     domain.RuleAction `json:"action"`
	Position   int               `json:"position"`
}

// CreatePolicyRuleCommand defines parameters for creating a routing policy rule.
type CreatePolicyRuleCommand struct {
	ID            string           `json:"id,omitempty"`
	RevisionID    string           `json:"revision_id"`
	TargetGroupID string           `json:"target_group_id"`
	Expression    string           `json:"expression"`
	Position      int              `json:"position"`
	RequestID     string           `json:"request_id,omitempty"`
	ActorKind     domain.ActorKind `json:"actor_kind,omitempty"`
}

// PolicyRuleView is the view representation of a routing policy rule.
type PolicyRuleView struct {
	ID            string `json:"id"`
	RevisionID    string `json:"revision_id"`
	TargetGroupID string `json:"target_group_id"`
	Expression    string `json:"expression"`
	Position      int    `json:"position"`
}

// ListRulesQuery defines parameters for listing rules.
type ListRulesQuery struct {
	RevisionID string `json:"revision_id,omitempty"`
	Kind       string `json:"kind,omitempty"` // "admission", "policy", or "" for both
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
}

// ListRulesResult contains listed admission and policy rules.
type ListRulesResult struct {
	RevisionID     string              `json:"revision_id"`
	AdmissionRules []AdmissionRuleView `json:"admission_rules"`
	PolicyRules    []PolicyRuleView    `json:"policy_rules"`
	Page           int                 `json:"page,omitempty"`
	PageSize       int                 `json:"page_size,omitempty"`
	Total          int                 `json:"total"`
}

// CreateRevisionCommand defines parameters for generating an immutable configuration revision.
type CreateRevisionCommand struct {
	ParentID  *string                           `json:"parent_id,omitempty"`
	State     domain.ConfigurationRevisionState `json:"state,omitempty"`
	RequestID string                            `json:"request_id,omitempty"`
	ActorKind domain.ActorKind                  `json:"actor_kind,omitempty"`
}

// ValidationResult represents the output of graph and rule validation.
type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}
