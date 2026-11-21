package domain

// EnabledPolicyRules projects persisted rules into effective routing input.
// It preserves order and never reuses the caller's backing array, so disabled
// records remain available to storage, editing and API consumers.
func EnabledPolicyRules(rules []PolicyRule) []PolicyRule {
	enabled := make([]PolicyRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	return enabled
}
