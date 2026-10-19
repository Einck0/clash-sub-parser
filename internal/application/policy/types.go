package policy

import (
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
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
	EmptyFallbackPass   bool                   `json:"empty_fallback_pass"`
	InheritGlobalFilter *bool                  `json:"inherit_global_filter,omitempty"`
	ID                  string                 `json:"id,omitempty"`
	Name              string                 `json:"name"`
	GroupType         domain.GroupType       `json:"group_type"`
	Position          *int                   `json:"position,omitempty"`
	Sources           []domain.MemberSource  `json:"sources,omitempty"`
	Edges             []EdgeInput            `json:"edges,omitempty"`
	NodeFilter        *domain.NodeFilterSpec `json:"node_filter,omitempty"`
	RequestID         string                 `json:"request_id,omitempty"`
	ActorKind         domain.ActorKind       `json:"actor_kind,omitempty"`
}

// UpdateGroupCommand defines parameters for updating a node group.
type UpdateGroupCommand struct {
	EmptyFallbackPass   *bool                  `json:"empty_fallback_pass,omitempty"`
	InheritGlobalFilter *bool                  `json:"inherit_global_filter,omitempty"`
	ID                  string                 `json:"id"`
	Name              *string                `json:"name,omitempty"`
	GroupType         *domain.GroupType      `json:"group_type,omitempty"`
	Position          *int                   `json:"position,omitempty"`
	ExpectedRevision  *int64                 `json:"expected_revision,omitempty"`
	Sources           *[]domain.MemberSource `json:"sources,omitempty"`
	Edges             *[]EdgeInput           `json:"edges,omitempty"`
	NodeFilter        *domain.NodeFilterSpec `json:"node_filter,omitempty"`
	ClearNodeFilter   bool                   `json:"clear_node_filter,omitempty"`
	RequestID         string                 `json:"request_id,omitempty"`
	ActorKind         domain.ActorKind       `json:"actor_kind,omitempty"`
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
	EmptyFallbackPass   bool                   `json:"empty_fallback_pass"`
	InheritGlobalFilter bool                   `json:"inherit_global_filter"`
	ID                  string                 `json:"id"`
	Name              string                 `json:"name"`
	GroupType         domain.GroupType       `json:"group_type"`
	Position          int                    `json:"position"`
	Revision          int64                  `json:"revision"`
	Sources           []domain.MemberSource  `json:"sources"`
	Edges             []GroupEdgeView        `json:"edges"`
	NodeFilter        *domain.NodeFilterSpec `json:"node_filter,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// GroupPreviewResult represents resolved preview data for a policy group.
type GroupPreviewResult struct {
	GroupID            string                         `json:"group_id"`
	Name               string                         `json:"name"`
	GroupType          domain.GroupType               `json:"group_type"`
	EmptyFallbackPass  bool                           `json:"empty_fallback_pass"`
	EffectiveEmptyPass bool                           `json:"effective_empty_pass"`
	Members            []resolver.ResolvedGroupMember `json:"members"`
	ExpandedNodeIDs    []string                       `json:"expanded_node_ids"`
	ExpandedNodeCount  int                            `json:"expanded_node_count"`
}

// PreviewGroupCommand defines parameters for in-RAM preview of a policy group.
type PreviewGroupCommand struct {
	GroupID             *string                `json:"group_id,omitempty"`
	ExpectedRevision    *int64                 `json:"expected_revision,omitempty"`
	Name                *string                `json:"name,omitempty"`
	GroupType           *domain.GroupType      `json:"group_type,omitempty"`
	EmptyFallbackPass   *bool                  `json:"empty_fallback_pass,omitempty"`
	InheritGlobalFilter *bool                  `json:"inherit_global_filter,omitempty"`
	Sources             []domain.MemberSource  `json:"sources"`
	Target            *domain.CompilerTarget `json:"target,omitempty"`
	RequestID         string                 `json:"request_id,omitempty"`
	ActorKind         domain.ActorKind       `json:"actor_kind,omitempty"`
}

// GroupPreviewMember represents a member in the preview result without exposing secrets.
type GroupPreviewMember struct {
	Kind          resolver.MemberKind `json:"kind"`
	NodeLogicalID *string             `json:"node_logical_id,omitempty"`
	GroupID       *string             `json:"group_id,omitempty"`
	BuiltinName   *string             `json:"builtin_name,omitempty"`
	DisplayName   string              `json:"display_name"`
	Position      int                 `json:"position"`
}

// GroupPreviewResponse represents the resolved in-memory preview of a policy group draft.
type GroupPreviewResponse struct {
	GroupID          string                     `json:"group_id"`
	Revision         int64                      `json:"revision"`
	SelectionDigest  string                     `json:"selection_digest"`
	ContentDigest    *string                    `json:"content_digest,omitempty"`
	Members          []GroupPreviewMember       `json:"members"`
	MemberCount      int                        `json:"member_count"`
	NodeCount        int                        `json:"node_count"`
	CandidateCount   int                        `json:"candidate_count"`
	ExcludedCount    int                        `json:"excluded_count"`
	ExclusionSummary map[string]int             `json:"exclusion_summary,omitempty"`
	ExcludedReasons  []string                   `json:"excluded_reasons,omitempty"`
	FilterCounts     *resolver.GroupFilterCount `json:"filter_counts,omitempty"`
	Diagnostics      []resolver.Diagnostic      `json:"diagnostics"`
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

// RuleCategoryView is the view representation of a rule category.
type RuleCategoryView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	RuleCount int    `json:"rule_count"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CreateCategoryCommand defines parameters for creating a rule category.
type CreateCategoryCommand struct {
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name"`
	SortOrder int              `json:"sort_order"`
	RequestID string           `json:"request_id,omitempty"`
	ActorKind domain.ActorKind `json:"actor_kind,omitempty"`
}

// UpdateCategoryCommand defines parameters for updating a rule category.
type UpdateCategoryCommand struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	SortOrder int              `json:"sort_order"`
	RequestID string           `json:"request_id,omitempty"`
	ActorKind domain.ActorKind `json:"actor_kind,omitempty"`
}

// DeleteCategoryCommand defines parameters for deleting a rule category.
type DeleteCategoryCommand struct {
	ID        string           `json:"id"`
	RequestID string           `json:"request_id,omitempty"`
	ActorKind domain.ActorKind `json:"actor_kind,omitempty"`
}

// CreatePolicyRuleCommand defines parameters for creating a routing policy rule.
type CreatePolicyRuleCommand struct {
	ID            string           `json:"id,omitempty"`
	RevisionID    string           `json:"revision_id"`
	CategoryID    string           `json:"category_id,omitempty"`
	Category      string           `json:"category,omitempty"`
	Name          string           `json:"name,omitempty"`
	TargetGroupID string           `json:"target_group_id,omitempty"`
	BuiltinTarget string           `json:"builtin_target,omitempty"`
	Expression    string           `json:"expression"`
	Position      int              `json:"position"`
	Enabled       *bool            `json:"enabled,omitempty"`
	RequestID     string           `json:"request_id,omitempty"`
	ActorKind     domain.ActorKind `json:"actor_kind,omitempty"`
}

// UpdatePolicyRuleCommand defines parameters for updating a routing policy rule.
type UpdatePolicyRuleCommand struct {
	ID            string           `json:"id"`
	CategoryID    *string          `json:"category_id,omitempty"`
	Category      *string          `json:"category,omitempty"`
	Name          *string          `json:"name,omitempty"`
	TargetGroupID *string          `json:"target_group_id,omitempty"`
	BuiltinTarget *string          `json:"builtin_target,omitempty"`
	Expression    *string          `json:"expression,omitempty"`
	Position      *int             `json:"position,omitempty"`
	Enabled       *bool            `json:"enabled,omitempty"`
	RequestID     string           `json:"request_id,omitempty"`
	ActorKind     domain.ActorKind `json:"actor_kind,omitempty"`
}

// ReorderPolicyRulesCommand defines parameters for reordering routing policy rules within a category or revision.
type ReorderPolicyRulesCommand struct {
	RevisionID string           `json:"revision_id,omitempty"`
	CategoryID string           `json:"category_id,omitempty"`
	Order      []string         `json:"order"`
	RequestID  string           `json:"request_id,omitempty"`
	ActorKind  domain.ActorKind `json:"actor_kind,omitempty"`
}

// PolicyRuleView is the view representation of a routing policy rule.
type PolicyRuleView struct {
	ID            string `json:"id"`
	RevisionID    string `json:"revision_id"`
	CategoryID    string `json:"category_id,omitempty"`
	Category      string `json:"category,omitempty"`
	Name          string `json:"name,omitempty"`
	TargetGroupID string `json:"target_group_id,omitempty"`
	BuiltinTarget string `json:"builtin_target,omitempty"`
	Expression    string `json:"expression"`
	Position      int    `json:"position"`
	Enabled       bool   `json:"enabled"`
}

// DeleteRuleCommand defines parameters for deleting an admission or routing policy rule.
type DeleteRuleCommand struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind,omitempty"` // "admission", "policy", or "" to auto-detect
	RequestID string           `json:"request_id,omitempty"`
	ActorKind domain.ActorKind `json:"actor_kind,omitempty"`
}

// ListRulesQuery defines parameters for listing rules.
type ListRulesQuery struct {
	RevisionID string `json:"revision_id,omitempty"`
	Kind       string `json:"kind,omitempty"` // "admission", "policy", or "" for both
	CategoryID string `json:"category_id,omitempty"`
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

// ValidationIssue describes a single structural or dynamic routing diagnostic finding.
type ValidationIssue struct {
	Code            string `json:"code"`
	Severity        string `json:"severity"` // "error" or "warning"
	RuleID          string `json:"rule_id,omitempty"`
	Position        int    `json:"position,omitempty"`
	Type            string `json:"type,omitempty"`
	Value           string `json:"value,omitempty"`
	TargetGroupID   string `json:"target_group_id,omitempty"`
	TargetGroupName string `json:"target_group_name,omitempty"`
	Message         string `json:"message"`
}

// ValidationResult represents the output of graph and rule validation.
type ValidationResult struct {
	Valid      bool              `json:"valid"`
	Errors     []string          `json:"errors,omitempty"`
	RevisionID string            `json:"revision_id,omitempty"`
	Issues     []ValidationIssue `json:"issues,omitempty"`
}
