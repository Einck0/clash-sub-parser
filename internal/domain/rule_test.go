package domain_test

import (
	"encoding/json"
	"testing"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestRule_Validation(t *testing.T) {
	tests := []struct {
		name    string
		rule    domain.Rule
		wantErr bool
	}{
		{
			name: "valid DOMAIN-SUFFIX rule",
			rule: domain.Rule{
				Type:    domain.RuleTypeDomainSuffix,
				Value:   "google.com",
				Proxy:   "Proxy",
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "valid MATCH rule without value",
			rule: domain.Rule{
				Type:    domain.RuleTypeMatch,
				Value:   "",
				Proxy:   "DIRECT",
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "missing proxy target",
			rule: domain.Rule{
				Type:  domain.RuleTypeDomain,
				Value: "example.com",
				Proxy: "",
			},
			wantErr: true,
		},
		{
			name: "missing type",
			rule: domain.Rule{
				Type:  "",
				Value: "example.com",
				Proxy: "DIRECT",
			},
			wantErr: true,
		},
		{
			name: "missing value for non-MATCH rule",
			rule: domain.Rule{
				Type:  domain.RuleTypeDomainSuffix,
				Value: "",
				Proxy: "DIRECT",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.rule.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Rule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRule_ToClashFormat(t *testing.T) {
	tests := []struct {
		name     string
		rule     domain.Rule
		expected string
	}{
		{
			name: "domain suffix rule",
			rule: domain.Rule{
				Type:  domain.RuleTypeDomainSuffix,
				Value: "google.com",
				Proxy: "PROXY",
			},
			expected: "DOMAIN-SUFFIX,google.com,PROXY",
		},
		{
			name: "ip-cidr with options",
			rule: domain.Rule{
				Type:    domain.RuleTypeIPCIDR,
				Value:   "192.168.1.0/24",
				Proxy:   "DIRECT",
				Options: []string{"no-resolve"},
			},
			expected: "IP-CIDR,192.168.1.0/24,DIRECT,no-resolve",
		},
		{
			name: "match rule",
			rule: domain.Rule{
				Type:  domain.RuleTypeMatch,
				Proxy: "FINAL",
			},
			expected: "MATCH,FINAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rule.ToClashFormat()
			if got != tt.expected {
				t.Errorf("ToClashFormat() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRule_ParseClashRule(t *testing.T) {
	tests := []struct {
		raw      string
		wantType domain.RuleType
		wantVal  string
		wantProx string
		wantOpts []string
		wantErr  bool
	}{
		{
			raw:      "DOMAIN-SUFFIX,google.com,PROXY",
			wantType: domain.RuleTypeDomainSuffix,
			wantVal:  "google.com",
			wantProx: "PROXY",
			wantErr:  false,
		},
		{
			raw:      "IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
			wantType: domain.RuleTypeIPCIDR,
			wantVal:  "10.0.0.0/8",
			wantProx: "DIRECT",
			wantOpts: []string{"no-resolve"},
			wantErr:  false,
		},
		{
			raw:      "MATCH,DIRECT",
			wantType: domain.RuleTypeMatch,
			wantVal:  "",
			wantProx: "DIRECT",
			wantErr:  false,
		},
		{
			raw:     "INVALID",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			r, err := domain.ParseClashRule(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseClashRule(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if err == nil {
				if r.Type != tt.wantType || r.Value != tt.wantVal || r.Proxy != tt.wantProx {
					t.Errorf("ParseClashRule mismatch: got %+v", r)
				}
			}
		})
	}
}

func TestRule_Serialization(t *testing.T) {
	rule := domain.Rule{
		ID:        42,
		Name:      "Google Direct",
		Category:  "google",
		Type:      domain.RuleTypeDomainSuffix,
		Value:     "google.com",
		Proxy:     "DIRECT",
		Options:   []string{"no-resolve"},
		SortOrder: 10,
		Enabled:   true,
	}

	// JSON
	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled domain.Rule
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if unmarshaled.Value != rule.Value || unmarshaled.Proxy != rule.Proxy {
		t.Errorf("JSON mismatch: %+v vs %+v", unmarshaled, rule)
	}

	// YAML
	yamlBytes, err := yaml.Marshal(rule)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	var yamlRule domain.Rule
	if err := yaml.Unmarshal(yamlBytes, &yamlRule); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if yamlRule.Type != domain.RuleTypeDomainSuffix {
		t.Errorf("YAML mismatch: %+v vs %+v", yamlRule, rule)
	}
}
