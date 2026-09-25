package resolver

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
)

type nodeAdmissionResult struct {
	AdmittedNodes   []domain.Node
	AdmittedNodeMap map[string]domain.Node
	AdmittedNodeIDs []string
	ExcludedNodeIDs []string
	Diagnostics     []Diagnostic
}

// evaluateNodeAdmission filters nodes by active state, explicit risk exclusions, and admission rules.
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

	riskExcludedSet := make(map[string]bool, len(excludedNodeIDs))
	for _, id := range excludedNodeIDs {
		riskExcludedSet[id] = true
	}

	// Sort admission rules deterministically by Position
	sortedRules := make([]domain.AdmissionRule, len(admissionRules))
	copy(sortedRules, admissionRules)
	for i := 0; i < len(sortedRules)-1; i++ {
		for j := i + 1; j < len(sortedRules); j++ {
			if sortedRules[i].Position > sortedRules[j].Position {
				sortedRules[i], sortedRules[j] = sortedRules[j], sortedRules[i]
			}
		}
	}

	for _, node := range nodes {
		// 1. Check active status
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

		// 2. Check risk exclusion
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

		// 3. Evaluate against admission rules
		admitted := true
		for _, rule := range sortedRules {
			if matchesAdmissionExpression(node, rule.Expression) {
				switch rule.Action {
				case domain.RuleActionReject:
					admitted = false
					res.ExcludedNodeIDs = append(res.ExcludedNodeIDs, node.LogicalID)
					res.Diagnostics = append(res.Diagnostics, Diagnostic{
						Severity: DiagnosticSeverityWarning,
						Code:     "node_rejected_by_admission_rule",
						Message:  fmt.Sprintf("node %s (%s) was rejected by admission rule %q", node.LogicalID, node.DisplayName, rule.Name),
						Target:   node.LogicalID,
					})
				case domain.RuleActionQuarantine:
					admitted = false
					res.ExcludedNodeIDs = append(res.ExcludedNodeIDs, node.LogicalID)
					res.Diagnostics = append(res.Diagnostics, Diagnostic{
						Severity: DiagnosticSeverityWarning,
						Code:     "node_quarantined_by_admission_rule",
						Message:  fmt.Sprintf("node %s (%s) was quarantined by admission rule %q", node.LogicalID, node.DisplayName, rule.Name),
						Target:   node.LogicalID,
					})
				case domain.RuleActionAllow:
					admitted = true
				}
				break // First matching admission rule determines initial action
			}
		}

		if admitted {
			res.AdmittedNodes = append(res.AdmittedNodes, node)
			res.AdmittedNodeMap[node.LogicalID] = node
			res.AdmittedNodeIDs = append(res.AdmittedNodeIDs, node.LogicalID)
		}
	}

	sort.Strings(res.AdmittedNodeIDs)
	sort.Strings(res.ExcludedNodeIDs)
	res.ExcludedNodeIDs = uniqueStrings(res.ExcludedNodeIDs)
	return res
}

// matchesAdmissionExpression evaluates whether a node matches an admission expression.
func matchesAdmissionExpression(node domain.Node, expr string) bool {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" || trimmed == "*" || strings.EqualFold(trimmed, "ALL") || strings.EqualFold(trimmed, "ANY") {
		return true
	}

	// 1. Structured syntax: e.g. "field op 'val'" or "field op val"
	// Example: country == 'HK', protocol == 'vmess', name contains 'HK'
	lower := strings.ToLower(trimmed)

	if strings.Contains(lower, "==") {
		parts := strings.SplitN(trimmed, "==", 2)
		field := strings.ToLower(strings.TrimSpace(parts[0]))
		val := cleanQuotes(strings.TrimSpace(parts[1]))
		return matchFieldEqual(node, field, val)
	}

	if strings.Contains(lower, "!=") {
		parts := strings.SplitN(trimmed, "!=", 2)
		field := strings.ToLower(strings.TrimSpace(parts[0]))
		val := cleanQuotes(strings.TrimSpace(parts[1]))
		return !matchFieldEqual(node, field, val)
	}

	if strings.Contains(lower, "contains") {
		parts := strings.SplitN(lower, "contains", 2)
		field := strings.TrimSpace(parts[0])
		val := cleanQuotes(strings.TrimSpace(parts[1]))
		return matchFieldContains(node, field, val)
	}

	if strings.Contains(lower, "=~") || strings.Contains(lower, "~=") {
		op := "=~"
		if strings.Contains(lower, "~=") {
			op = "~="
		}
		parts := strings.SplitN(trimmed, op, 2)
		field := strings.ToLower(strings.TrimSpace(parts[0]))
		pattern := cleanQuotes(strings.TrimSpace(parts[1]))
		return matchFieldRegex(node, field, pattern)
	}

	// 2. Fallback: try regular expression or substring against DisplayName
	if re, err := regexp.Compile("(?i)" + trimmed); err == nil {
		if re.MatchString(node.DisplayName) {
			return true
		}
	}

	return strings.Contains(strings.ToLower(node.DisplayName), strings.ToLower(trimmed))
}

func matchFieldEqual(node domain.Node, field, val string) bool {
	switch field {
	case "protocol":
		return strings.EqualFold(string(node.Protocol), val)
	case "name", "display_name":
		return strings.EqualFold(node.DisplayName, val)
	case "country":
		// Infer country from DisplayName prefix or tag (e.g. "HK", "US", "JP")
		return strings.Contains(strings.ToUpper(node.DisplayName), strings.ToUpper(val))
	case "logical_id", "id":
		return node.LogicalID == val
	default:
		return strings.EqualFold(node.DisplayName, val)
	}
}

func matchFieldContains(node domain.Node, field, val string) bool {
	switch field {
	case "protocol":
		return strings.Contains(strings.ToLower(string(node.Protocol)), strings.ToLower(val))
	case "name", "display_name":
		return strings.Contains(strings.ToLower(node.DisplayName), strings.ToLower(val))
	case "country":
		return strings.Contains(strings.ToUpper(node.DisplayName), strings.ToUpper(val))
	default:
		return strings.Contains(strings.ToLower(node.DisplayName), strings.ToLower(val))
	}
}

func matchFieldRegex(node domain.Node, field, pattern string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	switch field {
	case "protocol":
		return re.MatchString(string(node.Protocol))
	case "name", "display_name":
		return re.MatchString(node.DisplayName)
	default:
		return re.MatchString(node.DisplayName)
	}
}

func cleanQuotes(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) ||
		(strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) {
		if len(s) >= 2 {
			return s[1 : len(s)-1]
		}
	}
	return s
}
