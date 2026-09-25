package resolver

import (
	"fmt"
	"sort"

	"clash-sub-parser/internal/domain"
)

func evaluateRiskAdmission(
	nodes []domain.Node,
	policyRevision string,
	decisions []domain.RiskDecision,
	reviewAction domain.RiskAction,
) ([]string, []Diagnostic) {
	if policyRevision == "" || len(decisions) == 0 {
		return nil, nil
	}

	nodeSet := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		nodeSet[node.LogicalID] = struct{}{}
	}

	if reviewAction != domain.RiskActionAllow {
		reviewAction = domain.RiskActionBlock
	}

	excluded := make(map[string]struct{})
	diagnostics := make([]Diagnostic, 0)
	for _, decision := range decisions {
		if decision.PolicyRevisionID != policyRevision {
			continue
		}
		if _, exists := nodeSet[decision.NodeLogicalID]; !exists {
			continue
		}

		code := ""
		message := ""
		switch decision.Decision {
		case domain.RiskActionBlock:
			code = "risk_blocked"
			message = fmt.Sprintf("node %s was excluded by risk policy: %s", decision.NodeLogicalID, decision.ReasonCode)
		case domain.RiskActionReview:
			if reviewAction == domain.RiskActionAllow {
				code = "risk_review_allowed"
				message = fmt.Sprintf("node %s requires risk review but remains admitted: %s", decision.NodeLogicalID, decision.ReasonCode)
			} else {
				code = "risk_review"
				message = fmt.Sprintf("node %s was excluded pending risk review: %s", decision.NodeLogicalID, decision.ReasonCode)
			}
		case domain.RiskActionUnknown:
			code = "risk_unknown"
			message = fmt.Sprintf("node %s was excluded because risk decision is unknown: %s", decision.NodeLogicalID, decision.ReasonCode)
		default:
			continue
		}

		if decision.Decision == domain.RiskActionBlock ||
			(decision.Decision == domain.RiskActionReview && reviewAction != domain.RiskActionAllow) ||
			decision.Decision == domain.RiskActionUnknown {
			excluded[decision.NodeLogicalID] = struct{}{}
		}
		diagnostics = append(diagnostics, Diagnostic{
			Severity: riskDiagnosticSeverity(decision.Decision, reviewAction),
			Code:     code,
			Message:  message,
			Target:   decision.NodeLogicalID,
		})
	}

	excludedIDs := make([]string, 0, len(excluded))
	for nodeID := range excluded {
		excludedIDs = append(excludedIDs, nodeID)
	}
	sort.Strings(excludedIDs)
	return excludedIDs, diagnostics
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	unique := values[:1]
	for _, value := range values[1:] {
		if value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}

func riskDiagnosticSeverity(decision domain.RiskAction, reviewAction domain.RiskAction) DiagnosticSeverity {
	if decision == domain.RiskActionBlock || (decision == domain.RiskActionReview && reviewAction != domain.RiskActionAllow) || decision == domain.RiskActionUnknown {
		return DiagnosticSeverityWarning
	}
	return DiagnosticSeverityInfo
}
