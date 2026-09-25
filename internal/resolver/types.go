package resolver

import (
	"time"

	"clash-sub-parser/internal/domain"
)

// MemberKind denotes whether a group member is another group or a node.
type MemberKind string

const (
	MemberKindGroup MemberKind = "group"
	MemberKindNode  MemberKind = "node"
)

// DiagnosticSeverity denotes the level of a diagnostic entry.
type DiagnosticSeverity string

const (
	DiagnosticSeverityInfo    DiagnosticSeverity = "info"
	DiagnosticSeverityWarning DiagnosticSeverity = "warning"
	DiagnosticSeverityError   DiagnosticSeverity = "error"
)

// Diagnostic records a condition or filtered entity detected during resolution.
type Diagnostic struct {
	Severity DiagnosticSeverity `json:"severity"`
	Code     string             `json:"code"`
	Message  string             `json:"message"`
	Target   string             `json:"target,omitempty"`
}

// DNSConfig specifies DNS settings included in the resolved snapshot.
type DNSConfig struct {
	Enabled     bool     `json:"enabled"`
	Nameservers []string `json:"nameservers"`
	Fallback    []string `json:"fallback,omitempty"`
}

// ResolvedNode represents an active, admitted node in the resolved policy snapshot.
type ResolvedNode struct {
	LogicalID   string          `json:"logical_id"`
	DisplayName string          `json:"display_name"`
	Protocol    domain.Protocol `json:"protocol"`
	Active      bool            `json:"active"`
	Position    int             `json:"position"`
}

// ResolvedGroupMember represents an ordered reference inside a policy group.
type ResolvedGroupMember struct {
	Kind        MemberKind `json:"kind"` // "group" or "node"
	TargetID    string     `json:"target_id"`
	DisplayName string     `json:"display_name"`
	Position    int        `json:"position"`
}

// ResolvedGroup represents a fully resolved policy group in the routing tree.
type ResolvedGroup struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	GroupType         domain.GroupType      `json:"group_type"`
	Members           []ResolvedGroupMember `json:"members"`
	ChildGroupIDs     []string              `json:"child_group_ids"`
	NodeLogicalIDs    []string              `json:"node_logical_ids"`
	AllNodeLogicalIDs []string              `json:"all_node_logical_ids"`
	Position          int                   `json:"position"`
}

// ResolvedRule represents an ordered routing policy rule with terminal rule normalization.
type ResolvedRule struct {
	ID              string `json:"id"`
	TargetGroupID   string `json:"target_group_id"`
	TargetGroupName string `json:"target_group_name"`
	Expression      string `json:"expression"`
	Position        int    `json:"position"`
	IsTerminal      bool   `json:"is_terminal"`
}

// ResolvedPolicySnapshot is the single, canonical, immutable resolved snapshot
// consumed by preview, diagnostics, 5 client compilers, and publication services.
type ResolvedPolicySnapshot struct {
	InputDigest        string          `json:"input_digest"`
	SnapshotDigest     string          `json:"snapshot_digest"`
	RevisionID         string          `json:"revision_id,omitempty"`
	InventoryWatermark string          `json:"inventory_watermark,omitempty"`
	CompilerVersion    string          `json:"compiler_version,omitempty"`
	Nodes              []ResolvedNode  `json:"nodes"`
	NodeLogicalIDs     []string        `json:"node_logical_ids"`
	Groups             []ResolvedGroup `json:"groups"`
	Rules              []ResolvedRule  `json:"rules"`
	DNS                DNSConfig       `json:"dns"`
	Diagnostics        []Diagnostic    `json:"diagnostics"`

	// IP Risk integration extensions (conform to csp-ip-risk-v1 contract)
	RiskPolicyRevision string     `json:"risk_policy_revision,omitempty"`
	RiskDecisionDigest string     `json:"risk_decision_digest,omitempty"`
	RiskEvaluatedAt    *time.Time `json:"risk_evaluated_at,omitempty"`
	AdmittedNodeIDs    []string   `json:"admitted_node_ids,omitempty"`
	ExcludedNodeIDs    []string   `json:"excluded_node_ids,omitempty"`

	ResolvedAt time.Time `json:"resolved_at"`
}

// ResolveInput encapsulates all raw inputs to be resolved into a ResolvedPolicySnapshot.
type ResolveInput struct {
	RevisionID         string
	InventoryWatermark string
	CompilerVersion    string
	Nodes              []domain.Node
	Groups             []domain.NodeGroup
	Edges              map[string][]domain.GroupEdge
	PolicyRules        []domain.PolicyRule
	AdmissionRules     []domain.AdmissionRule
	DNS                DNSConfig

	// Optional risk inputs
	RiskPolicyRevision string
	RiskDecisionDigest string
	RiskEvaluatedAt    *time.Time
	RiskReviewAction   domain.RiskAction
	RiskDecisions      []domain.RiskDecision
	ExcludedNodeIDs    []string
}
