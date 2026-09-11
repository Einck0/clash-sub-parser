package domain

import (
	"fmt"
	"strings"
)

// RuleType represents standard Clash/Sing-box traffic routing rule types.
type RuleType string

const (
	RuleTypeDomain        RuleType = "DOMAIN"
	RuleTypeDomainSuffix  RuleType = "DOMAIN-SUFFIX"
	RuleTypeDomainKeyword RuleType = "DOMAIN-KEYWORD"
	RuleTypeIPCIDR        RuleType = "IP-CIDR"
	RuleTypeIPCIDR6       RuleType = "IP-CIDR6"
	RuleTypeGeoIP         RuleType = "GEOIP"
	RuleTypeGeoSite       RuleType = "GEOSITE"
	RuleTypePort          RuleType = "PORT"
	RuleTypeSrcPort       RuleType = "SRC-PORT"
	RuleTypeProcessName   RuleType = "PROCESS-NAME"
	RuleTypeRuleSet       RuleType = "RULE-SET"
	RuleTypeMatch         RuleType = "MATCH"
)

// Rule defines a routing classification rule mapping traffic to an outbound proxy or group.
type Rule struct {
	ID        int64    `json:"id" yaml:"id"`
	Name      string   `json:"name" yaml:"name"`
	Category  string   `json:"category" yaml:"category"`
	Type      RuleType `json:"type" yaml:"type"`
	Value     string   `json:"value" yaml:"value"`
	Proxy     string   `json:"proxy" yaml:"proxy"`
	Options   []string `json:"options,omitempty" yaml:"options,omitempty"`
	SortOrder int      `json:"sort_order" yaml:"sort_order"`
	Enabled   bool     `json:"enabled" yaml:"enabled"`
}

// Validate checks that rule attributes conform to routing syntax rules.
func (r *Rule) Validate() error {
	normType := RuleType(strings.ToUpper(strings.TrimSpace(string(r.Type))))
	if normType == "" {
		return ErrInvalidRuleType
	}
	if strings.TrimSpace(r.Proxy) == "" {
		return ErrMissingProxy
	}
	if normType != RuleTypeMatch && strings.TrimSpace(r.Value) == "" {
		return ErrInvalidRuleValue
	}
	return nil
}

// ToClashFormat converts the domain Rule into a standard Clash ruleset string.
func (r *Rule) ToClashFormat() string {
	normType := strings.ToUpper(strings.TrimSpace(string(r.Type)))
	target := strings.TrimSpace(r.Proxy)
	if target == "" {
		target = "DIRECT"
	}

	if normType == string(RuleTypeMatch) {
		return fmt.Sprintf("MATCH,%s", target)
	}

	val := strings.TrimSpace(r.Value)
	if len(r.Options) > 0 {
		return fmt.Sprintf("%s,%s,%s,%s", normType, val, target, strings.Join(r.Options, ","))
	}
	return fmt.Sprintf("%s,%s,%s", normType, val, target)
}

// ParseClashRule parses a single comma-separated Clash rule line into a domain Rule.
func ParseClashRule(raw string) (*Rule, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil, fmt.Errorf("empty or comment line")
	}

	parts := strings.Split(trimmed, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid rule format: %q", raw)
	}

	ruleType := RuleType(strings.ToUpper(parts[0]))
	if ruleType == RuleTypeMatch {
		return &Rule{
			Type:    RuleTypeMatch,
			Proxy:   parts[1],
			Enabled: true,
		}, nil
	}

	if len(parts) < 3 {
		return nil, fmt.Errorf("rule %q missing target proxy", raw)
	}

	rule := &Rule{
		Type:    ruleType,
		Value:   parts[1],
		Proxy:   parts[2],
		Enabled: true,
	}

	if len(parts) > 3 {
		rule.Options = parts[3:]
	}

	return rule, nil
}
