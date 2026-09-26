package policy

import (
	"fmt"
	"strings"

	"clash-sub-parser/internal/domain"
)

// IsMatchRule checks if a routing rule expression represents a MATCH/FINAL termination catch-all rule.
func IsMatchRule(expr string) bool {
	return domain.IsMatchRule(expr)
}

// ValidatePolicyGraph performs topological, cycle, self-loop, target existence, and rule order validation.
func ValidatePolicyGraph(
	groups []domain.NodeGroup,
	edges map[string][]domain.GroupEdge,
	policyRules []domain.PolicyRule,
	admissionRules []domain.AdmissionRule,
) error {
	if err := domain.ValidatePolicyTopology(groups, edges, policyRules, true); err != nil {
		return err
	}
	return ValidateAdmissionRules(admissionRules)
}

// ValidatePolicyRules verifies rule ordering, target group references, and termination rules.
func ValidatePolicyRules(rules []domain.PolicyRule, groupSet map[string]bool) error {
	return domain.ValidatePolicyRules(rules, groupSet, true)
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
