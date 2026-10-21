package resolver

import (
	"fmt"
	"sort"

	"clash-sub-parser/internal/domain"
)

type nodeAdmissionResult struct {
	AdmittedNodes   []domain.Node
	AdmittedNodeMap map[string]domain.Node
	AdmittedNodeIDs []string
	ExcludedNodeIDs []string
	Diagnostics     []Diagnostic
}

// evaluateNodeAdmission retains signature (nodes, admissionRules, excludedNodeIDs) for backward
// compatibility. Historical admission rule DSL matching is retired: admission rules no longer
// filter nodes, avoiding redundant/conflicting DSL evaluations and unverified string guessing.
// Hard constraints (node.Active status and explicit excludedNodeIDs from risk policies) are strictly
// enforced. If legacy admission rules are supplied, a single warning diagnostic is emitted advising
// operators to configure filtering using global or group member filters instead.
func evaluateNodeAdmission(
	nodes []domain.Node,
	admissionRules []domain.AdmissionRule,
	excludedNodeIDs []string,
) nodeAdmissionResult {
	res := nodeAdmissionResult{
		AdmittedNodes:   make([]domain.Node, 0, len(nodes)),
		AdmittedNodeMap: make(map[string]domain.Node, len(nodes)),
		AdmittedNodeIDs: make([]string, 0, len(nodes)),
		ExcludedNodeIDs: make([]string, 0),
		Diagnostics:     make([]Diagnostic, 0),
	}

	// Legacy admission rules are retired to prevent redundant/conflicting DSL evaluations.
	// We retain the signature for backward compatibility, but ignore rule expressions.
	if len(admissionRules) > 0 {
		res.Diagnostics = append(res.Diagnostics, Diagnostic{
			Severity: DiagnosticSeverityWarning,
			Code:     "legacy_admission_rules_ignored",
			Message:  "legacy admission rules are retired and ignored; configure filtering using global filters or group member filters instead",
		})
	}

	riskExcludedSet := make(map[string]bool, len(excludedNodeIDs))
	for _, id := range excludedNodeIDs {
		riskExcludedSet[id] = true
	}

	for _, node := range nodes {
		// 1. Check active status (hard constraint)
		if !node.Active {
			res.ExcludedNodeIDs = append(res.ExcludedNodeIDs, node.LogicalID)
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Severity: DiagnosticSeverityInfo,
				Code:     "node_inactive",
				Message:  fmt.Sprintf("node %s (%s) is inactive in inventory and excluded", node.LogicalID, node.DisplayName),
				Target:   node.LogicalID,
			})
			continue
		}

		// 2. Check risk exclusion (hard constraint)
		if riskExcludedSet[node.LogicalID] {
			res.ExcludedNodeIDs = append(res.ExcludedNodeIDs, node.LogicalID)
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Severity: DiagnosticSeverityWarning,
				Code:     "node_excluded_by_risk_policy",
				Message:  fmt.Sprintf("node %s (%s) was excluded by risk policy", node.LogicalID, node.DisplayName),
				Target:   node.LogicalID,
			})
			continue
		}

		// Admitted active node
		res.AdmittedNodes = append(res.AdmittedNodes, node)
		res.AdmittedNodeMap[node.LogicalID] = node
		res.AdmittedNodeIDs = append(res.AdmittedNodeIDs, node.LogicalID)
	}

	sort.Strings(res.AdmittedNodeIDs)
	sort.Strings(res.ExcludedNodeIDs)
	res.ExcludedNodeIDs = uniqueStrings(res.ExcludedNodeIDs)
	return res
}
