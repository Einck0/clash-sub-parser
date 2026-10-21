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
			name: "display_name valid regex op",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpRegex,
				Value: "加拿大|CA|Canada",
			},
			wantError: false,
		},
		{
			name: "display_name invalid regex op syntax",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpRegex,
				Value: "[unclosed",
			},
			wantError: true,
			errSubstr: "invalid regex pattern",
		},
		{
			name: "display_name valid not_regex op",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpNotRegex,
				Value: "自动|故障|官网",
			},
			wantError: false,
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
			name: "probe_latency_ms valid lte with nil probe_kind",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldProbeLatencyMS,
				Op:    domain.FilterOpLTE,
				Value: "300",
			},
			wantError: false,
		},
		{
			name: "probe_latency_ms valid lte with baseline",
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
			name: "probe_latency_ms rejects non-baseline probe_kind",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldProbeLatencyMS,
				Op:        domain.FilterOpLTE,
				Value:     "300",
				ProbeKind: probeKindPtr(domain.ProbeKindGeo),
			},
			wantError: true,
			errSubstr: "probe_latency_ms is fixed to baseline",
		},
		{
			name: "probe_latency_ms legacy alias <= rejected",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldProbeLatencyMS,
				Op:        domain.FilterOp("<="),
				Value:     "500",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "unsupported filter operator",
		},
		{
			name: "probe:latency_ms legacy alias rejected",
			condition: domain.FilterCondition{
				Field:     domain.FilterField("probe:latency_ms"),
				Op:        domain.FilterOpLTE,
				Value:     "500",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "unsupported filter field",
		},
		{
			name: "probe:verdict legacy alias rejected",
			condition: domain.FilterCondition{
				Field:     domain.FilterField("probe:verdict"),
				Op:        domain.FilterOpEquals,
				Value:     "available",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "unsupported filter field",
		},
		{
			name: "source_subscription_id legacy alias rejected",
			condition: domain.FilterCondition{
				Field: domain.FilterField("source_subscription_id"),
				Op:    domain.FilterOpContains,
				Value: validUUID,
			},
			wantError: true,
			errSubstr: "unsupported filter field",
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

		// 6. Health Status
		{
			name: "health_status valid healthy",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldHealthStatus,
				Op:    domain.FilterOpEquals,
				Value: "healthy",
			},
			wantError: false,
		},
		{
			name: "health_status valid not_equals unhealthy",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldHealthStatus,
				Op:    domain.FilterOpNotEquals,
				Value: "unhealthy",
			},
			wantError: false,
		},
		{
			name: "health_status valid untested",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldHealthStatus,
				Op:    domain.FilterOpEquals,
				Value: "untested",
			},
			wantError: false,
		},
		{
			name: "health_status invalid value degraded",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldHealthStatus,
				Op:    domain.FilterOpEquals,
				Value: "degraded",
			},
			wantError: true,
			errSubstr: "health_status must be healthy, unhealthy, or untested",
		},
		{
			name: "health_status invalid op contains",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldHealthStatus,
				Op:    domain.FilterOpContains,
				Value: "healthy",
			},
			wantError: true,
			errSubstr: "health_status only supports equals/not_equals",
		},
		{
			name: "health_status rejects probe_kind",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldHealthStatus,
				Op:        domain.FilterOpEquals,
				Value:     "healthy",
				ProbeKind: probeKindPtr(domain.ProbeKindBaseline),
			},
			wantError: true,
			errSubstr: "health_status does not accept probe parameters",
		},

		// 7. Exit Region
		{
			name: "exit_region valid HK",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldExitRegion,
				Op:    domain.FilterOpEquals,
				Value: "HK",
			},
			wantError: false,
		},
		{
			name: "exit_region valid not_equals US",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldExitRegion,
				Op:    domain.FilterOpNotEquals,
				Value: "US",
			},
			wantError: false,
		},
		{
			name: "exit_region invalid 3 letters",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldExitRegion,
				Op:    domain.FilterOpEquals,
				Value: "HKG",
			},
			wantError: true,
			errSubstr: "exit_region must be an ISO alpha-2 code",
		},
		{
			name: "exit_region invalid lowercase",
			condition: domain.FilterCondition{
				Field: domain.FilterFieldExitRegion,
				Op:    domain.FilterOpEquals,
				Value: "hk",
			},
			wantError: true,
			errSubstr: "exit_region must be an ISO alpha-2 code",
		},
		{
			name: "exit_region rejects probe_kind",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldExitRegion,
				Op:        domain.FilterOpEquals,
				Value:     "HK",
				ProbeKind: probeKindPtr(domain.ProbeKindGeo),
			},
			wantError: true,
			errSubstr: "exit_region does not accept probe parameters",
		},

		// 8. Platform Verdict
		{
			name: "platform_verdict valid streaming disney",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "available",
				ProbeKind: probeKindPtr(domain.ProbeKindStreaming),
				Platform:  "disney",
			},
			wantError: false,
		},
		{
			name: "platform_verdict valid ai openai",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "restricted",
				ProbeKind: probeKindPtr(domain.ProbeKindAI),
				Platform:  "openai",
			},
			wantError: false,
		},
		{
			name: "platform_verdict invalid platform for streaming",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "available",
				ProbeKind: probeKindPtr(domain.ProbeKindStreaming),
				Platform:  "openai",
			},
			wantError: true,
			errSubstr: "unsupported platform for probe kind",
		},
		{
			name: "platform_verdict invalid probe_kind geo",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "available",
				ProbeKind: probeKindPtr(domain.ProbeKindGeo),
				Platform:  "disney",
			},
			wantError: true,
			errSubstr: "platform_verdict requires ai or streaming probe_kind",
		},
		{
			name: "platform_verdict invalid verdict value",
			condition: domain.FilterCondition{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "passed",
				ProbeKind: probeKindPtr(domain.ProbeKindStreaming),
				Platform:  "disney",
			},
			wantError: true,
			errSubstr: "platform_verdict must be available, restricted, unknown, or error",
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
	rev1 := int64(1)
	rev2 := int64(2)

	node := domain.Node{
		LogicalID:          "node-test-01",
		Protocol:           domain.ProtocolSS,
		DisplayName:        "Hong Kong [01] Premium",
		Active:             true,
		ConnectionRevision: rev1,
	}

	sources := []domain.NodeSource{
		{NodeLogicalID: node.LogicalID, SubscriptionID: subID1},
	}

	freshObs := domain.ProbeObservation{
		ID:                 "obs-01",
		NodeLogicalID:      node.LogicalID,
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictAvailable,
		LatencyMS:          180,
		ObservedAt:         now.Add(-10 * time.Minute),
		ConnectionRevision: &rev1,
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

		// Stale observation with same revision STILL MATCHES (age does not invalidate valid revision)
		staleObs := freshObs
		staleObs.ObservedAt = now.Add(-72 * time.Hour)
		staleMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: staleObs}
		matchedStale, _ := domain.MatchesCondition(condAvail, node, sources, staleMap, asOf)
		if !matchedStale {
			t.Fatal("expected observation on same valid revision to match regardless of age")
		}

		// Wrong revision fails
		wrongRevObs := freshObs
		wrongRevObs.ConnectionRevision = &rev2
		wrongRevMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: wrongRevObs}
		matchedWrongRev, _ := domain.MatchesCondition(condAvail, node, sources, wrongRevMap, asOf)
		if matchedWrongRev {
			t.Fatal("expected wrong connection revision to fail closed")
		}

		// Future observation fails
		futureObs := freshObs
		futureObs.ObservedAt = now.Add(2 * time.Hour)
		futureMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: futureObs}
		matchedFuture, _ := domain.MatchesCondition(condAvail, node, sources, futureMap, asOf)
		if matchedFuture {
			t.Fatal("expected future observation to fail closed")
		}

		// Zero observed_at fails
		zeroObs := freshObs
		zeroObs.ObservedAt = time.Time{}
		zeroMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: zeroObs}
		matchedZero, _ := domain.MatchesCondition(condAvail, node, sources, zeroMap, asOf)
		if matchedZero {
			t.Fatal("expected zero observation time to fail closed")
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

		// Negated condition with missing observation -> MUST ALSO BE FALSE (fail-closed, not bypassable)
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

	t.Run("probe latency condition requires baseline available verdict and latency threshold", func(t *testing.T) {
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

		// nil probe_kind defaults to baseline
		condLatencyNilKind := domain.FilterCondition{
			Field: domain.FilterFieldProbeLatencyMS,
			Op:    domain.FilterOpLTE,
			Value: "300",
		}
		matchedNilKind, _ := domain.MatchesCondition(condLatencyNilKind, node, sources, latestObs, asOf)
		if !matchedNilKind {
			t.Fatal("expected nil probe_kind to default to baseline and match")
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

		// Zero or negative latency -> fails closed
		zeroLatencyObs := freshObs
		zeroLatencyObs.LatencyMS = 0
		zeroLatencyMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: zeroLatencyObs}
		matchedZeroLatency, _ := domain.MatchesCondition(condLatency300, node, sources, zeroLatencyMap, asOf)
		if matchedZeroLatency {
			t.Fatal("expected latency <= 0 to fail closed")
		}
	})

	t.Run("health_status evaluates baseline 3 categories and rejects smuggling", func(t *testing.T) {
		condHealthy := domain.FilterCondition{
			Field: domain.FilterFieldHealthStatus,
			Op:    domain.FilterOpEquals,
			Value: "healthy",
		}
		matched, _ := domain.MatchesCondition(condHealthy, node, sources, latestObs, asOf)
		if !matched {
			t.Fatal("expected healthy condition to match fresh available baseline")
		}

		condNotUnhealthy := domain.FilterCondition{
			Field: domain.FilterFieldHealthStatus,
			Op:    domain.FilterOpNotEquals,
			Value: "unhealthy",
		}
		matchedNotUnhealthy, _ := domain.MatchesCondition(condNotUnhealthy, node, sources, latestObs, asOf)
		if !matchedNotUnhealthy {
			t.Fatal("healthy node should satisfy not_equals unhealthy")
		}

		// Empty observations => untested
		condUntested := domain.FilterCondition{
			Field: domain.FilterFieldHealthStatus,
			Op:    domain.FilterOpEquals,
			Value: "untested",
		}
		emptyMap := map[domain.ProbeKind]domain.ProbeObservation{}
		matchedUntested, _ := domain.MatchesCondition(condUntested, node, sources, emptyMap, asOf)
		if !matchedUntested {
			t.Fatal("empty observations should evaluate to untested")
		}
		// Untested must NOT match healthy
		matchedUntestedAsHealthy, _ := domain.MatchesCondition(condHealthy, node, sources, emptyMap, asOf)
		if matchedUntestedAsHealthy {
			t.Fatal("untested node must not match healthy")
		}

		// Unhealthy verdict
		unhealthyObs := freshObs
		unhealthyObs.Verdict = domain.VerdictError
		unhealthyMap := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindBaseline: unhealthyObs}
		condUnhealthy := domain.FilterCondition{
			Field: domain.FilterFieldHealthStatus,
			Op:    domain.FilterOpEquals,
			Value: "unhealthy",
		}
		matchedUnhealthy, _ := domain.MatchesCondition(condUnhealthy, node, sources, unhealthyMap, asOf)
		if !matchedUnhealthy {
			t.Fatal("expected error verdict to evaluate to unhealthy")
		}
	})

	t.Run("exit_region requires current successful geo observation with non-empty region", func(t *testing.T) {
		geoObs := domain.ProbeObservation{
			ID:                 "obs-geo-01",
			NodeLogicalID:      node.LogicalID,
			Kind:               domain.ProbeKindGeo,
			Verdict:            domain.VerdictAvailable,
			Region:             "HK",
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-5 * time.Minute),
		}
		geoMap := map[domain.ProbeKind]domain.ProbeObservation{
			domain.ProbeKindBaseline: freshObs,
			domain.ProbeKindGeo:      geoObs,
		}

		condHK := domain.FilterCondition{
			Field: domain.FilterFieldExitRegion,
			Op:    domain.FilterOpEquals,
			Value: "HK",
		}
		matchedHK, _ := domain.MatchesCondition(condHK, node, sources, geoMap, asOf)
		if !matchedHK {
			t.Fatal("expected exit_region HK to match")
		}

		condNotUS := domain.FilterCondition{
			Field: domain.FilterFieldExitRegion,
			Op:    domain.FilterOpNotEquals,
			Value: "US",
		}
		matchedNotUS, _ := domain.MatchesCondition(condNotUS, node, sources, geoMap, asOf)
		if !matchedNotUS {
			t.Fatal("expected exit_region not_equals US to match HK")
		}

		// Failed geo -> both equals and not_equals FAIL closed
		failedGeoObs := geoObs
		failedGeoObs.Verdict = domain.VerdictError
		failedGeoObs.Region = "HK"
		failedGeoMap := map[domain.ProbeKind]domain.ProbeObservation{
			domain.ProbeKindBaseline: freshObs,
			domain.ProbeKindGeo:      failedGeoObs,
		}
		matchedEqFailed, _ := domain.MatchesCondition(condHK, node, sources, failedGeoMap, asOf)
		if matchedEqFailed {
			t.Fatal("failed geo observation must not match equals")
		}
		matchedNeqFailed, _ := domain.MatchesCondition(condNotUS, node, sources, failedGeoMap, asOf)
		if matchedNeqFailed {
			t.Fatal("failed geo observation must not match not_equals (fail-closed)")
		}

		// Missing geo observation -> fails closed for both
		matchedEqMissing, _ := domain.MatchesCondition(condHK, node, sources, latestObs, asOf)
		if matchedEqMissing {
			t.Fatal("missing geo observation must not match equals")
		}
		matchedNeqMissing, _ := domain.MatchesCondition(condNotUS, node, sources, latestObs, asOf)
		if matchedNeqMissing {
			t.Fatal("missing geo observation must not match not_equals")
		}

		// Cross revision -> fails closed
		crossRevGeoObs := geoObs
		crossRevGeoObs.ConnectionRevision = &rev2
		crossRevGeoMap := map[domain.ProbeKind]domain.ProbeObservation{
			domain.ProbeKindBaseline: freshObs,
			domain.ProbeKindGeo:      crossRevGeoObs,
		}
		matchedCrossRev, _ := domain.MatchesCondition(condHK, node, sources, crossRevGeoMap, asOf)
		if matchedCrossRev {
			t.Fatal("cross revision geo observation must fail closed")
		}
	})

	t.Run("platform_verdict checks specific platform evidence on current revision", func(t *testing.T) {
		streamingObs := domain.ProbeObservation{
			ID:                 "obs-streaming-01",
			NodeLogicalID:      node.LogicalID,
			Kind:               domain.ProbeKindStreaming,
			Verdict:            domain.VerdictAvailable,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-5 * time.Minute),
			Platforms: map[string]domain.PlatformCapability{
				"disney": {Verdict: domain.VerdictAvailable},
			},
		}
		platformMap := map[domain.ProbeKind]domain.ProbeObservation{
			domain.ProbeKindBaseline:  freshObs,
			domain.ProbeKindStreaming: streamingObs,
		}

		condDisneyAvail := domain.FilterCondition{
			Field:     domain.FilterFieldPlatformVerdict,
			Op:        domain.FilterOpEquals,
			Value:     "available",
			ProbeKind: probeKindPtr(domain.ProbeKindStreaming),
			Platform:  "disney",
		}
		matchedDisney, _ := domain.MatchesCondition(condDisneyAvail, node, sources, platformMap, asOf)
		if !matchedDisney {
			t.Fatal("expected disney available to match")
		}

		// Missing platform within streaming observation -> fails closed
		condNetflix := domain.FilterCondition{
			Field:     domain.FilterFieldPlatformVerdict,
			Op:        domain.FilterOpEquals,
			Value:     "available",
			ProbeKind: probeKindPtr(domain.ProbeKindStreaming),
			Platform:  "netflix",
		}
		matchedNetflix, _ := domain.MatchesCondition(condNetflix, node, sources, platformMap, asOf)
		if matchedNetflix {
			t.Fatal("missing platform evidence must fail closed")
		}
		condNetflixNeq := domain.FilterCondition{
			Field:     domain.FilterFieldPlatformVerdict,
			Op:        domain.FilterOpNotEquals,
			Value:     "error",
			ProbeKind: probeKindPtr(domain.ProbeKindStreaming),
			Platform:  "netflix",
		}
		matchedNetflixNeq, _ := domain.MatchesCondition(condNetflixNeq, node, sources, platformMap, asOf)
		if matchedNetflixNeq {
			t.Fatal("missing platform evidence must fail closed even on not_equals")
		}

		// nil ProbeKind in condition does not panic, returns false
		condNilProbeKind := domain.FilterCondition{
			Field:    domain.FilterFieldPlatformVerdict,
			Op:       domain.FilterOpEquals,
			Value:    "available",
			Platform: "disney",
		}
		matchedNilKind, _ := domain.MatchesCondition(condNilProbeKind, node, sources, platformMap, asOf)
		if matchedNilKind {
			t.Fatal("nil ProbeKind on platform verdict must return false")
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
