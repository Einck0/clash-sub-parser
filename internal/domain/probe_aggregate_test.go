package domain_test

import (
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestAggregatePlatformVerdicts_FrozenPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		platforms   map[string]domain.PlatformCapability
		wantVerdict domain.ProbeVerdict
		wantReason  string
	}{
		{
			name:        "empty_map_returns_skipped",
			platforms:   map[string]domain.PlatformCapability{},
			wantVerdict: domain.VerdictSkipped,
			wantReason:  "no_platforms_selected",
		},
		{
			name: "abnormal_alone_returns_abnormal",
			platforms: map[string]domain.PlatformCapability{
				"gemini": {Verdict: domain.VerdictAbnormal, Reason: "region_mismatch"},
			},
			wantVerdict: domain.VerdictAbnormal,
			wantReason:  "partial_or_abnormal",
		},
		{
			name: "abnormal_wins_over_available",
			platforms: map[string]domain.PlatformCapability{
				"openai": {Verdict: domain.VerdictAvailable, Reason: "contract_matched"},
				"gemini": {Verdict: domain.VerdictAbnormal, Reason: "region_mismatch"},
			},
			wantVerdict: domain.VerdictAbnormal,
			wantReason:  "partial_or_abnormal",
		},
		{
			name: "abnormal_wins_over_error_and_restricted",
			platforms: map[string]domain.PlatformCapability{
				"gemini": {Verdict: domain.VerdictAbnormal, Reason: "region_mismatch"},
				"claude": {Verdict: domain.VerdictRestricted, Reason: "access_restricted"},
				"openai": {Verdict: domain.VerdictError, Reason: "transport_error"},
			},
			wantVerdict: domain.VerdictAbnormal,
			wantReason:  "partial_or_abnormal",
		},
		{
			name: "available_alone_returns_available",
			platforms: map[string]domain.PlatformCapability{
				"openai": {Verdict: domain.VerdictAvailable, Reason: "contract_matched"},
			},
			wantVerdict: domain.VerdictAvailable,
			wantReason:  "contract_matched",
		},
		{
			name: "available_wins_over_restricted_error_skipped_and_unknown",
			platforms: map[string]domain.PlatformCapability{
				"openai":      {Verdict: domain.VerdictAvailable, Reason: "contract_matched"},
				"claude":      {Verdict: domain.VerdictRestricted, Reason: "access_restricted"},
				"gemini":      {Verdict: domain.VerdictError, Reason: "detection_failed"},
				"antigravity": {Verdict: domain.VerdictSkipped, Reason: "cancelled"},
				"other":       {Verdict: domain.VerdictUnknown, Reason: "contract_drift"},
			},
			wantVerdict: domain.VerdictAvailable,
			wantReason:  "contract_matched",
		},
		{
			name: "unknown_alone_returns_unknown",
			platforms: map[string]domain.PlatformCapability{
				"openai": {Verdict: domain.VerdictUnknown, Reason: "contract_drift"},
			},
			wantVerdict: domain.VerdictUnknown,
			wantReason:  "evidence_inconclusive",
		},
		{
			name: "stale_alone_returns_unknown",
			platforms: map[string]domain.PlatformCapability{
				"openai": {Verdict: domain.VerdictStale, Reason: "expired"},
			},
			wantVerdict: domain.VerdictUnknown,
			wantReason:  "evidence_inconclusive",
		},
		{
			name: "invalid_verdict_treated_as_unknown",
			platforms: map[string]domain.PlatformCapability{
				"openai": {Verdict: domain.ProbeVerdict("bogus"), Reason: "bad"},
			},
			wantVerdict: domain.VerdictUnknown,
			wantReason:  "evidence_inconclusive",
		},
		{
			name: "empty_verdict_string_treated_as_unknown",
			platforms: map[string]domain.PlatformCapability{
				"openai": {Verdict: domain.ProbeVerdict(""), Reason: "empty"},
			},
			wantVerdict: domain.VerdictUnknown,
			wantReason:  "evidence_inconclusive",
		},
		{
			name: "unknown_wins_over_restricted_and_error",
			platforms: map[string]domain.PlatformCapability{
				"openai": {Verdict: domain.VerdictUnknown, Reason: "contract_drift"},
				"claude": {Verdict: domain.VerdictRestricted, Reason: "access_restricted"},
				"gemini": {Verdict: domain.VerdictError, Reason: "detection_failed"},
			},
			wantVerdict: domain.VerdictUnknown,
			wantReason:  "evidence_inconclusive",
		},
		{
			name: "restricted_alone_returns_restricted",
			platforms: map[string]domain.PlatformCapability{
				"claude": {Verdict: domain.VerdictRestricted, Reason: "access_restricted"},
			},
			wantVerdict: domain.VerdictRestricted,
			wantReason:  "access_restricted",
		},
		{
			name: "restricted_wins_over_error_and_skipped",
			platforms: map[string]domain.PlatformCapability{
				"claude": {Verdict: domain.VerdictRestricted, Reason: "access_restricted"},
				"gemini": {Verdict: domain.VerdictError, Reason: "detection_failed"},
				"openai": {Verdict: domain.VerdictSkipped, Reason: "not_executed"},
			},
			wantVerdict: domain.VerdictRestricted,
			wantReason:  "access_restricted",
		},
		{
			name: "error_alone_returns_error",
			platforms: map[string]domain.PlatformCapability{
				"gemini": {Verdict: domain.VerdictError, Reason: "detection_failed"},
			},
			wantVerdict: domain.VerdictError,
			wantReason:  "detection_failed",
		},
		{
			name: "error_wins_over_skipped",
			platforms: map[string]domain.PlatformCapability{
				"gemini": {Verdict: domain.VerdictError, Reason: "detection_failed"},
				"openai": {Verdict: domain.VerdictSkipped, Reason: "not_executed"},
			},
			wantVerdict: domain.VerdictError,
			wantReason:  "detection_failed",
		},
		{
			name: "all_skipped_returns_skipped",
			platforms: map[string]domain.PlatformCapability{
				"gemini": {Verdict: domain.VerdictSkipped, Reason: "cancelled"},
				"openai": {Verdict: domain.VerdictSkipped, Reason: "budget_not_executed"},
			},
			wantVerdict: domain.VerdictSkipped,
			wantReason:  "not_executed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotVerdict, gotReason := domain.AggregatePlatformVerdicts(tc.platforms)
			if gotVerdict != tc.wantVerdict || gotReason != tc.wantReason {
				t.Errorf("AggregatePlatformVerdicts() = (%v, %q), want (%v, %q)",
					gotVerdict, gotReason, tc.wantVerdict, tc.wantReason)
			}
		})
	}
}
