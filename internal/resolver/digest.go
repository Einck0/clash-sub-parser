package resolver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"clash-sub-parser/internal/domain"
)

type canonicalInputNode struct {
	LogicalID   string          `json:"logical_id"`
	DisplayName string          `json:"display_name"`
	Protocol    domain.Protocol `json:"protocol"`
	Active      bool            `json:"active"`
}

type canonicalInputGroup struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	GroupType domain.GroupType `json:"group_type"`
}

type canonicalInputEdge struct {
	ParentGroupID string  `json:"parent_group_id"`
	ChildGroupID  *string `json:"child_group_id,omitempty"`
	NodeLogicalID *string `json:"node_logical_id,omitempty"`
	Position      int     `json:"position"`
}

type canonicalInputRule struct {
	TargetGroupID string `json:"target_group_id"`
	Expression    string `json:"expression"`
	Position      int    `json:"position"`
}

type canonicalAdmissionRule struct {
	Name       string            `json:"name"`
	Expression string            `json:"expression"`
	Action     domain.RuleAction `json:"action"`
	Position   int               `json:"position"`
}

type canonicalRiskDecision struct {
	NodeLogicalID    string            `json:"node_logical_id"`
	PolicyRevisionID string            `json:"policy_revision_id"`
	Decision         domain.RiskAction `json:"decision"`
	ReasonCode       string            `json:"reason_code"`
	EvaluatedAt      string            `json:"evaluated_at"`
}

type canonicalInputPayload struct {
	RevisionID         string                   `json:"revision_id,omitempty"`
	InventoryWatermark string                   `json:"inventory_watermark,omitempty"`
	CompilerVersion    string                   `json:"compiler_version,omitempty"`
	Nodes              []canonicalInputNode     `json:"nodes"`
	Groups             []canonicalInputGroup    `json:"groups"`
	Edges              []canonicalInputEdge     `json:"edges"`
	PolicyRules        []canonicalInputRule     `json:"policy_rules"`
	AdmissionRules     []canonicalAdmissionRule `json:"admission_rules"`
	DNS                DNSConfig                `json:"dns"`
	RiskPolicyRevision string                   `json:"risk_policy_revision,omitempty"`
	RiskDecisionDigest string                   `json:"risk_decision_digest,omitempty"`
	RiskDecisions      []canonicalRiskDecision  `json:"risk_decisions,omitempty"`
}

func computeInputDigest(input ResolveInput) (string, error) {
	// Canonicalize nodes (sort by LogicalID)
	nodes := make([]canonicalInputNode, len(input.Nodes))
	for i, n := range input.Nodes {
		nodes[i] = canonicalInputNode{
			LogicalID:   n.LogicalID,
			DisplayName: n.DisplayName,
			Protocol:    n.Protocol,
			Active:      n.Active,
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].LogicalID < nodes[j].LogicalID
	})

	// Canonicalize groups (sort by ID)
	groups := make([]canonicalInputGroup, len(input.Groups))
	for i, g := range input.Groups {
		groups[i] = canonicalInputGroup{
			ID:        g.ID,
			Name:      g.Name,
			GroupType: g.GroupType,
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].ID < groups[j].ID
	})

	// Canonicalize edges (flatten and sort by parentGroupID then Position)
	var edges []canonicalInputEdge
	for parentID, edgeList := range input.Edges {
		for _, e := range edgeList {
			edges = append(edges, canonicalInputEdge{
				ParentGroupID: parentID,
				ChildGroupID:  e.ChildGroupID,
				NodeLogicalID: e.NodeLogicalID,
				Position:      e.Position,
			})
		}
	}
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].ParentGroupID == edges[j].ParentGroupID {
			return edges[i].Position < edges[j].Position
		}
		return edges[i].ParentGroupID < edges[j].ParentGroupID
	})

	// Canonicalize policy rules (sort by Position)
	rules := make([]canonicalInputRule, len(input.PolicyRules))
	for i, r := range input.PolicyRules {
		rules[i] = canonicalInputRule{
			TargetGroupID: r.TargetGroupID,
			Expression:    r.Expression,
			Position:      r.Position,
		}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].Position < rules[j].Position
	})

	// Canonicalize admission rules (sort by Position)
	admRules := make([]canonicalAdmissionRule, len(input.AdmissionRules))
	for i, r := range input.AdmissionRules {
		admRules[i] = canonicalAdmissionRule{
			Name:       r.Name,
			Expression: r.Expression,
			Action:     r.Action,
			Position:   r.Position,
		}
	}
	sort.SliceStable(admRules, func(i, j int) bool {
		return admRules[i].Position < admRules[j].Position
	})

	orderedDecisions := append([]domain.RiskDecision(nil), input.RiskDecisions...)
	sort.SliceStable(orderedDecisions, func(i, j int) bool {
		if orderedDecisions[i].NodeLogicalID != orderedDecisions[j].NodeLogicalID {
			return orderedDecisions[i].NodeLogicalID < orderedDecisions[j].NodeLogicalID
		}
		return orderedDecisions[i].ReasonCode < orderedDecisions[j].ReasonCode
	})
	riskDecisions := make([]canonicalRiskDecision, 0, len(orderedDecisions))
	for _, decision := range orderedDecisions {
		riskDecisions = append(riskDecisions, canonicalRiskDecision{
			NodeLogicalID: decision.NodeLogicalID, PolicyRevisionID: decision.PolicyRevisionID,
			Decision: decision.Decision, ReasonCode: decision.ReasonCode,
			EvaluatedAt: decision.EvaluatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	payload := canonicalInputPayload{
		RevisionID:         input.RevisionID,
		InventoryWatermark: input.InventoryWatermark,
		CompilerVersion:    input.CompilerVersion,
		Nodes:              nodes,
		Groups:             groups,
		Edges:              edges,
		PolicyRules:        rules,
		AdmissionRules:     admRules,
		DNS:                input.DNS,
		RiskPolicyRevision: input.RiskPolicyRevision,
		RiskDecisionDigest: input.RiskDecisionDigest,
		RiskDecisions:      riskDecisions,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

type canonicalSnapshotPayload struct {
	RevisionID         string          `json:"revision_id,omitempty"`
	InventoryWatermark string          `json:"inventory_watermark,omitempty"`
	CompilerVersion    string          `json:"compiler_version,omitempty"`
	Nodes              []ResolvedNode  `json:"nodes"`
	NodeLogicalIDs     []string        `json:"node_logical_ids"`
	Groups             []ResolvedGroup `json:"groups"`
	Rules              []ResolvedRule  `json:"rules"`
	DNS                DNSConfig       `json:"dns"`
	Diagnostics        []Diagnostic    `json:"diagnostics"`
	AdmittedNodeIDs    []string        `json:"admitted_node_ids,omitempty"`
	ExcludedNodeIDs    []string        `json:"excluded_node_ids,omitempty"`
	RiskPolicyRevision string          `json:"risk_policy_revision,omitempty"`
	RiskEvaluatedAt    string          `json:"risk_evaluated_at,omitempty"`
	RiskDecisionDigest string          `json:"risk_decision_digest,omitempty"`
}

func computeSnapshotDigest(snap *ResolvedPolicySnapshot) (string, error) {
	payload := canonicalSnapshotPayload{
		RevisionID:         snap.RevisionID,
		InventoryWatermark: snap.InventoryWatermark,
		CompilerVersion:    snap.CompilerVersion,
		Nodes:              snap.Nodes,
		NodeLogicalIDs:     snap.NodeLogicalIDs,
		Groups:             snap.Groups,
		Rules:              snap.Rules,
		DNS:                snap.DNS,
		Diagnostics:        snap.Diagnostics,
		AdmittedNodeIDs:    snap.AdmittedNodeIDs,
		ExcludedNodeIDs:    snap.ExcludedNodeIDs,
		RiskPolicyRevision: snap.RiskPolicyRevision,
		RiskEvaluatedAt: func() string {
			if snap.RiskEvaluatedAt == nil {
				return ""
			}
			return snap.RiskEvaluatedAt.UTC().Format(time.RFC3339Nano)
		}(),
		RiskDecisionDigest: snap.RiskDecisionDigest,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
