package resolver_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func TestResolver_EnabledRulesProjectionAndDigests(t *testing.T) {
	groupID := domain.MustNewUUIDv7()
	direct := "DIRECT"
	input := resolver.ResolveInput{
		AsOf: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
		Groups: []domain.NodeGroup{{ID: groupID, Name: "Proxy", GroupType: domain.GroupTypeSelect,
			Sources: []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, BuiltinName: &direct}}}},
		PolicyRules: []domain.PolicyRule{
			{ID: "terminal", Expression: "MATCH", BuiltinTarget: "DIRECT", Category: "default", Enabled: true},
			{ID: "ad", Expression: "DOMAIN-SUFFIX,local.adguard.org", BuiltinTarget: "REJECT", Category: "ads", Position: 9, Enabled: true},
			{ID: "group", Expression: "DOMAIN,example.org", TargetGroupID: groupID, Category: "custom", Position: 1, Enabled: true},
			{ID: "disabled-builtin", Expression: "DOMAIN,disabled.invalid", BuiltinTarget: "REJECT", Enabled: false},
			{ID: "disabled-group", Expression: "DOMAIN,disabled-group.invalid", TargetGroupID: groupID, Enabled: false},
			{ID: "disabled-terminal", Expression: "FINAL", BuiltinTarget: "REJECT", Enabled: false},
		},
	}
	original := append([]domain.PolicyRule(nil), input.PolicyRules...)
	resolve := func() *resolver.ResolvedPolicySnapshot {
		t.Helper()
		snap, err := resolver.New().Resolve(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		return snap
	}
	on := resolve()
	if !reflect.DeepEqual(input.PolicyRules, original) {
		t.Fatal("resolution mutated persisted rule input")
	}
	if len(on.Rules) != 3 || on.Rules[0].ID != "ad" || on.Rules[1].ID != "group" || on.Rules[2].ID != "terminal" || !on.Rules[2].IsTerminal {
		t.Fatalf("effective category order and terminal normalization lost: %+v", on.Rules)
	}
	for i, rule := range on.Rules {
		if rule.Position != i {
			t.Fatalf("rule positions are not contiguous: %+v", on.Rules)
		}
	}

	input.PolicyRules[1].Enabled = false
	off := resolve()
	if len(off.Rules) != 2 || off.Rules[0].ID != "group" || off.Rules[1].ID != "terminal" {
		t.Fatalf("disabled rule leaked into snapshot: %+v", off.Rules)
	}
	if on.InputDigest == off.InputDigest || on.SnapshotDigest == off.SnapshotDigest {
		t.Fatal("turning a rule off must change effective input and snapshot digests")
	}
	input.PolicyRules[1].Expression = "DOMAIN,edited-disabled.invalid"
	input.PolicyRules[1].BuiltinTarget = "DIRECT"
	input.PolicyRules[1].Position = 100
	editedOff := resolve()
	if off.InputDigest != editedOff.InputDigest || off.SnapshotDigest != editedOff.SnapshotDigest {
		t.Fatal("editing a disabled rule changed effective digests")
	}
	input.PolicyRules[1] = original[1]
	onAgain := resolve()
	if on.InputDigest != onAgain.InputDigest || on.SnapshotDigest != onAgain.SnapshotDigest || !reflect.DeepEqual(on.Rules, onAgain.Rules) {
		t.Fatal("re-enabling the original rule did not restore effective semantics")
	}
	input.PolicyRules[1].BuiltinTarget = "DIRECT"
	retargeted := resolve()
	if retargeted.Rules[0].TargetGroupName != "DIRECT" || on.InputDigest == retargeted.InputDigest || on.SnapshotDigest == retargeted.SnapshotDigest {
		t.Fatal("enabled builtin target change must change output and both digests")
	}
}

func TestResolver_DisabledRulesDoNotImposeEffectiveConstraints(t *testing.T) {
	groupID := domain.MustNewUUIDv7()
	for _, tc := range []struct {
		name     string
		rule     domain.PolicyRule
		wantCode string
	}{
		{"missing target", domain.PolicyRule{TargetGroupID: domain.MustNewUUIDv7(), Expression: "DOMAIN,missing.invalid"}, "target_group_not_found"},
		{"additional terminal", domain.PolicyRule{BuiltinTarget: "REJECT", Expression: "FINAL"}, "invalid_match_rule_ordering"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := resolver.ResolveInput{
				Groups:      []domain.NodeGroup{{ID: groupID, Name: "Unrouted", GroupType: domain.GroupTypeSelect}},
				PolicyRules: []domain.PolicyRule{{Expression: "MATCH", BuiltinTarget: "DIRECT", Enabled: true}, tc.rule},
			}
			if _, err := resolver.New().Resolve(context.Background(), input); err != nil {
				t.Fatalf("disabled rule blocked resolution: %v", err)
			}
			input.PolicyRules[1].Enabled = true
			_, err := resolver.New().Resolve(context.Background(), input)
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != tc.wantCode {
				t.Fatalf("re-enabled constraint: want %s, got %v", tc.wantCode, err)
			}
		})
	}

	input := resolver.ResolveInput{
		Groups:      []domain.NodeGroup{{ID: groupID, Name: "Empty", GroupType: domain.GroupTypeSelect}},
		PolicyRules: []domain.PolicyRule{{TargetGroupID: groupID, Expression: "MATCH", Enabled: false}},
	}
	for _, enabled := range []bool{false, true, false} {
		input.PolicyRules[0].Enabled = enabled
		snap, err := resolver.New().Resolve(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		var routedError, unroutedWarning bool
		for _, d := range snap.Diagnostics {
			routedError = routedError || d.Code == "empty_routed_group"
			unroutedWarning = unroutedWarning || d.Code == "empty_group"
		}
		if routedError != enabled || unroutedWarning == enabled {
			t.Fatalf("enabled=%v: inconsistent group diagnostics: %+v", enabled, snap.Diagnostics)
		}
	}
}

func TestResolver_NoEnabledRulesEqualsEmptyInput(t *testing.T) {
	r := resolver.New()
	empty, err := r.Resolve(context.Background(), resolver.ResolveInput{})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := r.Resolve(context.Background(), resolver.ResolveInput{PolicyRules: []domain.PolicyRule{
		{Expression: "MATCH", BuiltinTarget: "DIRECT", Enabled: false},
		{Expression: "FINAL", BuiltinTarget: "REJECT", Enabled: false},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled.Rules) != 0 || disabled.InputDigest != empty.InputDigest || disabled.SnapshotDigest != empty.SnapshotDigest {
		t.Fatal("all-disabled input must have the same effective semantics as an empty rule set")
	}
}
