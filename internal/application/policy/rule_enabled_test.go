package policy_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
)

func TestPolicyService_ValidateGraphRuleEnabledLifecycle(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()
	group, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{Name: "Empty", GroupType: domain.GroupTypeSelect})
	if err != nil {
		t.Fatal(err)
	}
	strict := false
	if _, err := svc.SetGlobalNodeFilter(ctx, policy.SetGlobalNodeFilterCommand{EmptyFallbackPass: &strict}); err != nil {
		t.Fatal(err)
	}
	rule, err := svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{TargetGroupID: group.ID, Expression: "DOMAIN,example.org"})
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false, true} {
		if _, err := svc.UpdatePolicyRule(ctx, policy.UpdatePolicyRuleCommand{ID: rule.ID, Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
		result, err := svc.ValidateGraph(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var routedError, unroutedWarning bool
		for _, issue := range result.Issues {
			routedError = routedError || issue.Code == "empty_routed_group" && issue.RuleID == rule.ID
			unroutedWarning = unroutedWarning || issue.Code == "empty_group" && issue.Severity == "warning" && issue.TargetGroupID == group.ID
		}
		if result.Valid == enabled || routedError != enabled || unroutedWarning == enabled {
			t.Fatalf("enabled=%v: inconsistent effective graph validation: %+v", enabled, result)
		}
	}
}

func TestPolicyService_DisabledTerminalDoesNotConflictWithEnabledCreate(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()
	disabled := false
	if _, err := svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{BuiltinTarget: "REJECT", Expression: "FINAL", Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	active, err := svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{BuiltinTarget: "DIRECT", Expression: "MATCH", Position: 1})
	if err != nil || active == nil || !active.Enabled {
		t.Fatalf("disabled terminal blocked default-enabled create: %+v, %v", active, err)
	}
	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{BuiltinTarget: "REJECT", Expression: "FINAL", Position: 2})
	if de, ok := domain.AsDomainError(err); !ok || de.Code != "invalid_match_rule_ordering" {
		t.Fatalf("two enabled terminals must still fail: %v", err)
	}
	// Disabling a record never exempts its submitted expression/target from editing validation.
	for _, cmd := range []policy.CreatePolicyRuleCommand{
		{BuiltinTarget: "DIRECT", Expression: "", Enabled: &disabled},
		{BuiltinTarget: "INVALID", Expression: "DOMAIN,example.org", Enabled: &disabled},
		{TargetGroupID: domain.MustNewUUIDv7(), Expression: "DOMAIN,example.org", Enabled: &disabled},
	} {
		if _, err := svc.CreatePolicyRule(ctx, cmd); err == nil {
			t.Fatalf("invalid disabled record was accepted: %+v", cmd)
		}
	}
}

func TestPolicyValidation_EffectiveAggregateRules(t *testing.T) {
	groupID := domain.MustNewUUIDv7()
	groups := []domain.NodeGroup{{ID: groupID, Name: "Empty", GroupType: domain.GroupTypeSelect}}
	for _, tc := range []struct {
		name string
		rule domain.PolicyRule
		code string
	}{
		{"missing target", domain.PolicyRule{TargetGroupID: domain.MustNewUUIDv7(), Expression: "DOMAIN,missing.invalid", Position: 0}, "target_group_not_found"},
		{"second terminal", domain.PolicyRule{BuiltinTarget: "REJECT", Expression: "FINAL", Position: 0}, "invalid_match_rule_ordering"},
		{"after terminal", domain.PolicyRule{BuiltinTarget: "DIRECT", Expression: "DOMAIN,after.invalid", Position: 2}, "invalid_match_rule_ordering"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := []domain.PolicyRule{{Expression: "MATCH", BuiltinTarget: "DIRECT", Position: 1, Enabled: true}, tc.rule}
			check := func() error { return policy.ValidatePolicyGraph(groups, nil, rules, nil) }
			if err := check(); err != nil {
				t.Fatalf("disabled rule blocked effective graph: %v", err)
			}
			rules[1].Enabled = true
			if de, ok := domain.AsDomainError(check()); !ok || de.Code != tc.code {
				t.Fatalf("expected re-enabled constraint %s, got %+v", tc.code, de)
			}
		})
	}
}
