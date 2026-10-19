package domain

import (
	"testing"
)

func intPtr(v int) *int {
	return &v
}

func strPtr(v string) *string {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func TestSubscriptionProbePolicy_Validate(t *testing.T) {
	tests := []struct {
		name    string
		policy  SubscriptionProbePolicy
		wantErr bool
	}{
		{
			name:    "inherit mode is valid without interval",
			policy:  SubscriptionProbePolicy{Mode: ProbePolicyModeInherit},
			wantErr: false,
		},
		{
			name:    "disabled mode is valid without interval",
			policy:  SubscriptionProbePolicy{Mode: ProbePolicyModeDisabled},
			wantErr: false,
		},
		{
			name:    "custom mode with min bound 60s is valid",
			policy:  SubscriptionProbePolicy{Mode: ProbePolicyModeCustom, IntervalSeconds: intPtr(60)},
			wantErr: false,
		},
		{
			name:    "custom mode with mid bound 3600s is valid",
			policy:  SubscriptionProbePolicy{Mode: ProbePolicyModeCustom, IntervalSeconds: intPtr(3600)},
			wantErr: false,
		},
		{
			name:    "custom mode with max bound 604800s is valid",
			policy:  SubscriptionProbePolicy{Mode: ProbePolicyModeCustom, IntervalSeconds: intPtr(604800)},
			wantErr: false,
		},
		{
			name:    "custom mode without interval is invalid",
			policy:  SubscriptionProbePolicy{Mode: ProbePolicyModeCustom, IntervalSeconds: nil},
			wantErr: true,
		},
		{
			name:    "custom mode with interval below min (59s) is invalid",
			policy:  SubscriptionProbePolicy{Mode: ProbePolicyModeCustom, IntervalSeconds: intPtr(59)},
			wantErr: true,
		},
		{
			name:    "custom mode with interval above max (604801s) is invalid",
			policy:  SubscriptionProbePolicy{Mode: ProbePolicyModeCustom, IntervalSeconds: intPtr(604801)},
			wantErr: true,
		},
		{
			name:    "unknown mode is invalid",
			policy:  SubscriptionProbePolicy{Mode: "random_mode"},
			wantErr: true,
		},
		{
			name:    "empty mode is invalid",
			policy:  SubscriptionProbePolicy{Mode: ""},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("policy.Validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestSubscriptionConfig_ApplyPatch(t *testing.T) {
	initial := SubscriptionConfig{
		CronSchedule: "0 0 * * *",
		AutoTest:     true,
		RenameRules: []RenameRule{
			{Pattern: "old", Replace: "new"},
		},
		FilterRules: []FilterRule{
			{Type: "include", Pattern: "US"},
		},
		TargetGroups: []string{"Proxy"},
		ProbePolicy: &SubscriptionProbePolicy{
			Mode: ProbePolicyModeInherit,
		},
	}

	// 1. Patch only probe_policy: rename_rules, filter_rules, target_groups must be preserved
	cfg1 := initial
	customMode := ProbePolicyModeCustom
	patch1 := &SubscriptionConfigPatch{
		ProbePolicy: &SubscriptionProbePolicyPatch{
			Mode:            &customMode,
			IntervalSeconds: intPtr(300),
		},
	}
	if err := cfg1.ApplyPatch(patch1); err != nil {
		t.Fatalf("ApplyPatch(patch1) failed: %v", err)
	}
	if cfg1.ProbePolicy == nil || cfg1.ProbePolicy.Mode != ProbePolicyModeCustom || cfg1.ProbePolicy.IntervalSeconds == nil || *cfg1.ProbePolicy.IntervalSeconds != 300 {
		t.Fatalf("expected probe_policy custom(300), got %#v", cfg1.ProbePolicy)
	}
	if len(cfg1.RenameRules) != 1 || cfg1.RenameRules[0].Pattern != "old" {
		t.Fatalf("expected RenameRules preserved, got %#v", cfg1.RenameRules)
	}
	if len(cfg1.FilterRules) != 1 || cfg1.FilterRules[0].Pattern != "US" {
		t.Fatalf("expected FilterRules preserved, got %#v", cfg1.FilterRules)
	}
	if len(cfg1.TargetGroups) != 1 || cfg1.TargetGroups[0] != "Proxy" {
		t.Fatalf("expected TargetGroups preserved, got %#v", cfg1.TargetGroups)
	}
	if cfg1.CronSchedule != "0 0 * * *" || !cfg1.AutoTest {
		t.Fatalf("expected CronSchedule and AutoTest preserved")
	}

	// 2. Patch explicit empty slice: rename_rules cleared to empty
	emptyRename := []RenameRule{}
	patch2 := &SubscriptionConfigPatch{
		RenameRules: &emptyRename,
	}
	if err := cfg1.ApplyPatch(patch2); err != nil {
		t.Fatalf("ApplyPatch(patch2) failed: %v", err)
	}
	if len(cfg1.RenameRules) != 0 {
		t.Fatalf("expected RenameRules cleared, got %#v", cfg1.RenameRules)
	}
	// FilterRules still preserved
	if len(cfg1.FilterRules) != 1 {
		t.Fatalf("expected FilterRules still preserved, got %#v", cfg1.FilterRules)
	}
	// probe_policy still custom(300)
	if cfg1.ProbePolicy.Mode != ProbePolicyModeCustom {
		t.Fatalf("expected probe_policy still custom, got %#v", cfg1.ProbePolicy)
	}

	// 3. Switching mode to disabled clears interval
	disabledMode := ProbePolicyModeDisabled
	patch3 := &SubscriptionConfigPatch{
		ProbePolicy: &SubscriptionProbePolicyPatch{
			Mode: &disabledMode,
		},
	}
	if err := cfg1.ApplyPatch(patch3); err != nil {
		t.Fatalf("ApplyPatch(patch3) failed: %v", err)
	}
	if cfg1.ProbePolicy.Mode != ProbePolicyModeDisabled || cfg1.ProbePolicy.IntervalSeconds != nil {
		t.Fatalf("expected probe_policy disabled with nil interval, got %#v", cfg1.ProbePolicy)
	}

	// 4. Invalid mode returns error and does not corrupt config
	invalidMode := ProbePolicyMode("bogus")
	patch4 := &SubscriptionConfigPatch{
		ProbePolicy: &SubscriptionProbePolicyPatch{
			Mode: &invalidMode,
		},
	}
	if err := cfg1.ApplyPatch(patch4); err == nil {
		t.Fatalf("expected error for invalid mode, got nil")
	}
	if cfg1.ProbePolicy.Mode != ProbePolicyModeDisabled {
		t.Fatalf("expected mode still disabled after failed patch, got %s", cfg1.ProbePolicy.Mode)
	}
}
