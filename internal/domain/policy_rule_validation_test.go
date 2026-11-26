package domain_test

import (
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestValidatePolicyRuleExpressionNoResolveByRuleKind(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{name: "domain suffix rejects no-resolve", expr: "DOMAIN-SUFFIX,example.org,no-resolve", wantErr: true},
		{name: "domain rejects no-resolve case insensitively", expr: "domain,example.org,No-Resolve", wantErr: true},
		{name: "geosite rejects no-resolve", expr: "GEOSITE,category-ads-all,no-resolve", wantErr: true},
		{name: "IP CIDR retains no-resolve", expr: "IP-CIDR,192.0.2.0/24,no-resolve"},
		{name: "IPv6 CIDR retains no-resolve", expr: "IP-CIDR6,2001:db8::/32,no-resolve"},
		{name: "GEOIP retains no-resolve", expr: "GEOIP,CN,no-resolve"},
		{name: "rule set retains no-resolve", expr: "RULE-SET,provider,no-resolve"},
		{name: "ordinary domain suffix remains valid", expr: "DOMAIN-SUFFIX,example.org"},
		{name: "domain suffix extended matching remains available to other targets", expr: "DOMAIN-SUFFIX,example.org,extended-matching"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidatePolicyRuleExpression(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected validation error")
				}
				if !strings.Contains(err.Error(), "remove the option") {
					t.Fatalf("expected actionable guidance, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestValidatePolicyRulesPreservesExpressionOrder(t *testing.T) {
	rules := []domain.PolicyRule{
		{Expression: "DOMAIN-SUFFIX,first.example,extended-matching", Position: 20, BuiltinTarget: "DIRECT"},
		{Expression: "DOMAIN-SUFFIX,second.example", Position: 10, BuiltinTarget: "REJECT"},
	}
	before := append([]domain.PolicyRule(nil), rules...)
	if err := domain.ValidatePolicyRules(rules, nil, false); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	for i := range rules {
		if rules[i].Expression != before[i].Expression || rules[i].Position != before[i].Position {
			t.Fatalf("validation changed rule %d: before=%+v after=%+v", i, before[i], rules[i])
		}
	}
}

func TestValidatePolicyRulesDiagnosesPersistedInvalidRule(t *testing.T) {
	rules := []domain.PolicyRule{{
		Expression:    "DOMAIN-SUFFIX,example.org,no-resolve",
		BuiltinTarget: "DIRECT",
		Position:      0,
		Enabled:       true,
	}}
	err := domain.ValidatePolicyRules(rules, nil, false)
	if err == nil || !strings.Contains(err.Error(), "DOMAIN-SUFFIX rules do not support no-resolve") {
		t.Fatalf("expected stored invalid rule diagnostic, got %v", err)
	}
	if diagnostic, ok := domain.AsDomainError(err); !ok || diagnostic.Field != "expression" || diagnostic.Details["position"] != "0" {
		t.Fatalf("expected diagnostic to locate the invalid record: %v", err)
	}
}
