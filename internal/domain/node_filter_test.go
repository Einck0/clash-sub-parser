package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func intPtr(i int) *int {
	return &i
}

func probeKindPtr(k domain.ProbeKind) *domain.ProbeKind {
	return &k
}

func TestFilterConditionValidationMatrix(t *testing.T) {
	validUUID := "0191e4a0-0000-7000-8000-000000000001"

	tests := []struct {
		name      string
		condition domain.FilterCondition
		wantError bool
		errSubstr string
	}{
		// 1. Display Name
		{
			name: "display_name valid contains",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpContains,
				Value: "HK",
			},
			wantError: false,
		},
		{
			name: "display_name valid not_contains",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpNotContains,
				Value: "Expire",
			},
			wantError: false,
		},
		{
			name: "display_name invalid op equals",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpEquals,
				Value: "HK",
			},
			wantError: true,
			errSubstr: "display_name only supports contains/not_contains",
		},
		{
			name: "display_name empty value",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpContains,
				Value: "",
			},
			wantError: true,
			errSubstr: "cannot be empty",
		},
		{
			name: "display_name value too long",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpContains,
				Value: strings.Repeat("A", 256),
			},
			wantError: true,
			errSubstr: "cannot exceed 255 characters",
		},
		{
			name: "display_name with probe parameter rejected",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldDisplayName,
				Op:        domain.FilterOpContains,
				Value:     "HK",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "must not specify probe_kind",
		},

		// 2. Protocol
		{
			name: "protocol valid equals",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldProtocol,
				Op:    domain.FilterOpEquals,
				Value: "ss",
			},
			wantError: false,
		},
		{
			name: "protocol valid not_equals",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldProtocol,
				Op:    domain.FilterOpNotEquals,
				Value: "vmess",
			},
			wantError: false,
		},
		{
			name: "protocol invalid op contains",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldProtocol,
				Op:    domain.FilterOpContains,
				Value: "ss",
			},
			wantError: true,
			errSubstr: "protocol only supports equals/not_equals",
		},
		{
			name: "protocol invalid value",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldProtocol,
				Op:    domain.FilterOpEquals,
				Value: "wireguard3",
			},
			wantError: true,
			errSubstr: "invalid protocol value",
		},

		// 3. Source Subscriptions
		{
			name: "source_subscription_ids valid contains",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldSourceSubscriptions,
				Op:    domain.FilterOpContains,
				Value: validUUID,
			},
			wantError: false,
		},
		{
			name: "source_subscription_ids valid not_contains",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldSourceSubscriptions,
				Op:    domain.FilterOpNotContains,
				Value: validUUID,
			},
			wantError: false,
		},
		{
			name: "source_subscription_ids invalid uuid",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldSourceSubscriptions,
				Op:    domain.FilterOpContains,
				Value: "not-a-uuid",
			},
			wantError: true,
			errSubstr: "valid UUIDv7",
		},

		// 4. Probe Verdict
		{
			name: "probe_verdict valid equals",
			condition: domain.FilterCondition{
				Field:            domain.FilterFieldProbeVerdict,
				Op:               domain.FilterOpEquals,
				Value:            "available",
				ProbeKind:        probeKindPtr(domain.ProbeKindBaseline),
				FreshnessSeconds: intPtr(3600),
			},
			wantError: false,
		},
		{
			name: "probe_verdict missing probe_kind",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldProbeVerdict,
				Op:    domain.FilterOpEquals,
				Value: "available",
			},
			wantError: true,
			errSubstr: "requires a valid probe_kind",
		},
		{
			name: "probe_verdict invalid verdict value",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldProbeVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "nonexistent_verdict",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "invalid probe verdict",
		},
		{
			name: "probe_verdict invalid freshness (negative)",
			condition: domain.FilterCondition{
				Field:            domain.FilterFieldProbeVerdict,
				Op:               domain.FilterOpEquals,
				Value:            "available",
				ProbeKind:        probeKindPtr(domain.ProbeKindBaseline),
				FreshnessSeconds: intPtr(-10),
			},
			wantError: true,
			errSubstr: "freshness_seconds must be between",
		},

		// 5. Probe Latency MS
		{
			name: "probe_latency_ms valid lte",
			condition: domain.FilterCondition{
				Field:            domain.FilterFieldProbeLatencyMS,
				Op:               domain.FilterOpLTE,
				Value:            "300",
				ProbeKind:        probeKindPtr(domain.ProbeKindBaseline),
				FreshnessSeconds: intPtr(7200),
			},
			wantError: false,
		},
		{
			name: "probe_latency_ms valid <=",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldProbeLatencyMS,
				Op:        domain.FilterOp("<="),
				Value:     "500",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: false,
		},
		{
			name: "probe_latency_ms negative latency",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldProbeLatencyMS,
				Op:        domain.FilterOpLTE,
				Value:     "-50",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "latency_ms must be an integer between 0 and 60000",
		},
		{
			name: "probe_latency_ms excessive latency",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldProbeLatencyMS,
				Op:        domain.FilterOpLTE,
				Value:     "100000",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "latency_ms must be an integer between 0 and 60000",
		},
		{
			name: "probe_latency_ms invalid op equals",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldProbeLatencyMS,
				Op:        domain.FilterOpEquals,
				Value:     "300",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "probe_latency_ms only supports lte",
		},

		// Unknown field
		{
			name: "unknown field",
			condition: domain.FilterCondition{
				Field: "custom_score",
				Op:    domain.FilterOpEquals,
				Value: "100",
			},
			wantError: true,
			errSubstr: "unsupported filter field",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.condition.Validate()
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected validation error: %v", err)
				}
			}
		})
	}
}

func TestNodeFilterSpecBoundsAndValidation(t *testing.T) {
	// Empty spec is valid (evaluates to true)
	var nilSpec *domain.NodeFilterSpec
	if !nilSpec.IsEmpty() {
		t.Fatal("nil spec must be empty")
	}
	if err := nilSpec.Validate(); err != nil {
		t.Fatalf("nil spec should validate cleanly: %v", err)
	}

	emptySpec := &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{}}
	if !emptySpec.IsEmpty() {
		t.Fatal("spec with 0 conditions must be empty")
	}
	if err := emptySpec.Validate(); err != nil {
		t.Fatalf("empty spec should validate cleanly: %v", err)
	}

	// Excessive conditions (> 32)
	manyConditions := make([]domain.FilterCondition, 33)
	for i := range manyConditions {
		manyConditions[i] = domain.FilterCondition{
			Field: domain.FilterFieldDisplayName,
			Op:    domain.FilterOpContains,
			Value: "Node",
		}
	}
	spec33 := &domain.NodeFilterSpec{Conditions: manyConditions}
	if err := spec33.Validate(); err == nil {
		t.Fatal("expected error for > 32 conditions, got nil")
	}
}

func TestFilterConditionMatchingSemantics(t *testing.T) {
	now := domain.NowUTC()
	asOf := now
	subID1 := "0191e4a0-0000-7000-8000-000000000010"
	subID2 := "0191e4a0-0000-7000-8000-000000000020"

	credVer := 2
	node := domain.Node{
		LogicalID:         "node-test-01",
		Protocol:          domain.ProtocolSS,
		DisplayName:       "Hong Kong [01] Premium",
		CredentialVersion: credVer,
		Active:            true,
	}

	sources := []domain.NodeSource{
		{NodeLogicalID: node.LogicalID, SubscriptionID: subID1},
	}

	freshObs := domain.ProbeObservation{
		ID:                "obs-01",
		NodeLogicalID:     node.LogicalID,
		Kind:              domain.ProbeKindBaseline,
		Verdict:           domain.VerdictAvailable,
		LatencyMS:         180,
		CredentialVersion: &credVer,
		ObservedAt:        now.Add(-10 * time.Minute),
	}

	latestObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindBaseline: freshObs,
	}

	t.Run("display_name case-insensitive matching", func(t *testing.T) {
		cond := domain.FilterCondition{
			Field: domain.FilterFieldDisplayName,
			Op:    domain.FilterOpContains,
			Value: "hong kong", // lowercase match against "Hong Kong"
		}
		matched, _ := domain.MatchesCondition(cond, node, sources, latestObs, asOf)
		if !matched {
			t.Fatal("expected case-insensitive display_name match")
		}

		condNeg := domain.FilterCondition{
			Field: domain.FilterFieldDisplayName,
			Op:    domain.FilterOpNotContains,
			Value: "Tokyo",
		}
		matchedNeg, _ := domain.MatchesCondition(condNeg, node, sources, latestObs, asOf)
		if !matchedNeg {
			t.Fatal("expected not_contains to match")
		}
	})

	t.Run("protocol equals and not_equals", func(t *testing.T) {
		cond := domain.FilterCondition{
			Field: domain.FilterFieldProtocol,
			Op:    domain.FilterOpEquals,
			Value: "ss",
		}
		matched, _ := domain.MatchesCondition(cond, node, sources, latestObs, asOf)
		if !matched {
			t.Fatal("expected protocol ss match")
		}

		condMismatch := domain.FilterCondition{
			Field: domain.FilterFieldProtocol,
			Op:    domain.FilterOpEquals,
			Value: "vmess",
		}
		matchedM, _ := domain.MatchesCondition(condMismatch, node, sources, latestObs, asOf)
		if matchedM {
			t.Fatal("expected protocol vmess to not match")
		}
	})

	t.Run("source subscription contains and not_contains", func(t *testing.T) {
		cond1 := domain.FilterCondition{
			Field: domain.FilterFieldSourceSubscriptions,
			Op:    domain.FilterOpContains,
			Value: subID1,
		}
		matched1, _ := domain.MatchesCondition(cond1, node, sources, latestObs, asOf)
		if !matched1 {
			t.Fatal("expected source subscription match")
		}

		cond2 := domain.FilterCondition{
			Field: domain.FilterFieldSourceSubscriptions,
			Op:    domain.FilterOpContains,
			Value: subID2,
		}
		matched2, _ := domain.MatchesCondition(cond2, node, sources, latestObs, asOf)
		if matched2 {
			t.Fatal("expected unlinked subscription to fail")
		}
	})

	t.Run("probe verdict and fail-closed security invariants", func(t *testing.T) {
		// Valid match
		condAvail := domain.FilterCondition{
			Field:            domain.FilterFieldProbeVerdict,
			Op:               domain.FilterOpEquals,
			Value:            "available",
			ProbeKind:        probeKindPtr(domain.ProbeKindBaseline),
			FreshnessSeconds: intPtr(3600),
		}
		matched, _ := domain.MatchesCondition(condAvail, node, sources, latestObs, asOf)
		if !matched {
			t.Fatal("expected fresh available probe to match")
		}

		// Missing observation kind -> fails closed
		condGeo := domain.FilterCondition{
			Field:     domain.FilterFieldProbeVerdict,
			Op:        domain.FilterOpEquals,
			Value:     "available",
			ProbeKind: probeKindPtr(domain.ProbeKindGeo),
		}
		matchedGeo, _ := domain.MatchesCondition(condGeo, node, sources, latestObs, asOf)
		if matchedGeo {
			t.Fatal("expected missing probe kind to fail closed")
		}

		// Expired observation -> fails closed
		staleObs := freshObs
		staleObs.ObservedAt = now.Add(-2 * time.Hour) // 2 hours old
		staleMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: staleObs}
		matchedStale, _ := domain.MatchesCondition(condAvail, node, sources, staleMap, asOf)
		if matchedStale {
			t.Fatal("expected expired observation to fail closed")
		}

		// Credential version mismatch (node credVersion changed to 3) -> fails closed
		mismatchedNode := node
		mismatchedNode.CredentialVersion = 3
		matchedMismatch, _ := domain.MatchesCondition(condAvail, mismatchedNode, sources, latestObs, asOf)
		if matchedMismatch {
			t.Fatal("expected mismatched credential version observation to fail closed")
		}

		// Unversioned legacy observation (CredentialVersion == nil) -> fails closed
		legacyObs := freshObs
		legacyObs.CredentialVersion = nil
		legacyMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: legacyObs}
		matchedLegacy, _ := domain.MatchesCondition(condAvail, node, sources, legacyMap, asOf)
		if matchedLegacy {
			t.Fatal("expected unversioned legacy observation to fail closed")
		}

		// Negated condition with missing/expired observation -> MUST ALSO BE FALSE (fail-closed, not bypassable)
		condNotError := domain.FilterCondition{
			Field:     domain.FilterFieldProbeVerdict,
			Op:        domain.FilterOpNotEquals,
			Value:     "error",
			ProbeKind: probeKindPtr(domain.ProbeKindGeo), // missing
		}
		matchedNeg, _ := domain.MatchesCondition(condNotError, node, sources, latestObs, asOf)
		if matchedNeg {
			t.Fatal("negated probe condition with missing observation must evaluate to false")
		}
	})

	t.Run("probe latency condition requires available verdict and latency threshold", func(t *testing.T) {
		condLatency300 := domain.FilterCondition{
			Field:     domain.FilterFieldProbeLatencyMS,
			Op:        domain.FilterOpLTE,
			Value:     "300",
			ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
		}
		matched, _ := domain.MatchesCondition(condLatency300, node, sources, latestObs, asOf)
		if !matched {
			t.Fatal("expected latency 180ms <= 300ms to match")
		}

		condLatency100 := domain.FilterCondition{
			Field:     domain.FilterFieldProbeLatencyMS,
			Op:        domain.FilterOpLTE,
			Value:     "100",
			ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
		}
		matchedLow, _ := domain.MatchesCondition(condLatency100, node, sources, latestObs, asOf)
		if matchedLow {
			t.Fatal("expected latency 180ms <= 100ms to fail")
		}

		// Non-available verdict node with latency -> fails closed
		nonAvailObs := freshObs
		nonAvailObs.Verdict = domain.VerdictRestricted
		nonAvailMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: nonAvailObs}
		matchedRestricted, _ := domain.MatchesCondition(condLatency300, node, sources, nonAvailMap, asOf)
		if matchedRestricted {
			t.Fatal("expected restricted verdict to fail latency condition even if numeric latency is low")
		}
	})
}

func TestNodeFilterSpecJSONRoundtripAndAPIExample(t *testing.T) {
	spec := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field: domain.FilterFieldProtocol,
				Op:    domain.FilterOpEquals,
				Value: "ss",
			},
			{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpContains,
				Value: "HK",
			},
			{
				Field:            domain.FilterFieldProbeVerdict,
				Op:               domain.FilterOpEquals,
				Value:            "available",
				ProbeKind:        probeKindPtr(domain.ProbeKindBaseline),
				FreshnessSeconds: intPtr(86400),
			},
			{
				Field:            domain.FilterFieldProbeLatencyMS,
				Op:               domain.FilterOpLTE,
				Value:            "200",
				ProbeKind:        probeKindPtr(domain.ProbeKindBaseline),
				FreshnessSeconds: intPtr(86400),
			},
		},
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal filter spec: %v", err)
	}

	var decoded domain.NodeFilterSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal filter spec: %v", err)
	}

	if len(decoded.Conditions) != len(spec.Conditions) {
		t.Fatalf("condition length mismatch: %d vs %d", len(decoded.Conditions), len(spec.Conditions))
	}
	for i := range decoded.Conditions {
		if decoded.Conditions[i].Field != spec.Conditions[i].Field ||
			decoded.Conditions[i].Op != spec.Conditions[i].Op ||
			decoded.Conditions[i].Value != spec.Conditions[i].Value {
			t.Errorf("condition %d mismatch: %+v vs %+v", i, decoded.Conditions[i], spec.Conditions[i])
		}
	}
}
