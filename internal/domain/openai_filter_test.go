package domain_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestOpenAIPlatformFilterUsesOpenAIVerdictNotAggregateAI(t *testing.T) {
	revision := int64(4)
	now := domain.NowUTC()
	node := domain.Node{LogicalID: "openai-filter-node", ConnectionRevision: revision}
	obs := domain.ProbeObservation{
		Kind:               domain.ProbeKindAI,
		NodeLogicalID:      node.LogicalID,
		Verdict:            domain.VerdictAvailable,
		ConnectionRevision: &revision,
		ObservedAt:         now.Add(-time.Minute),
		Platforms: map[string]domain.PlatformCapability{
			"openai": {Verdict: domain.VerdictUnknown, Reason: "endpoint_reachable_capability_unverified"},
			"claude": {Verdict: domain.VerdictAvailable},
		},
	}
	condition := domain.FilterCondition{
		Field: domain.FilterFieldPlatformVerdict,
		Op:    domain.FilterOpEquals, Value: "available",
		ProbeKind: func() *domain.ProbeKind {
			kind := domain.ProbeKindAI
			return &kind
		}(),
		Platform: "openai",
	}
	matched, reason := domain.MatchesCondition(condition, node, nil, map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindAI: obs,
	}, now.Add(time.Second))
	if matched {
		t.Fatalf("unknown OpenAI capability must not match available based on aggregate AI verdict (reason: %s)", reason)
	}

	condition.Value = "unknown"
	matched, reason = domain.MatchesCondition(condition, node, nil, map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindAI: obs,
	}, now.Add(time.Second))
	if !matched {
		t.Fatalf("OpenAI-specific unknown verdict should match unknown (reason: %s)", reason)
	}
}

func TestOpenAIPlatformFilterUsesOnlyPreflightVerdict(t *testing.T) {
	now := domain.NowUTC()
	revision := int64(6)
	node := domain.Node{LogicalID: "openai-preflight-filter-node", ConnectionRevision: revision}
	kind := domain.ProbeKindAI
	condition := domain.FilterCondition{
		Field: domain.FilterFieldPlatformVerdict,
		Op:    domain.FilterOpEquals, Value: string(domain.VerdictAvailable),
		ProbeKind: &kind, Platform: "openai",
	}

	for _, tc := range []struct {
		verdict domain.ProbeVerdict
		want    bool
	}{
		{verdict: domain.VerdictAvailable, want: true},
		{verdict: domain.VerdictUnknown, want: false},
		{verdict: domain.VerdictError, want: false},
		{verdict: domain.VerdictAbnormal, want: false},
	} {
		t.Run(string(tc.verdict), func(t *testing.T) {
			obs := domain.ProbeObservation{
				Kind: kind, NodeLogicalID: node.LogicalID,
				Verdict: domain.VerdictAvailable, ConnectionRevision: &revision,
				ObservedAt: now.Add(-time.Minute),
				Platforms: map[string]domain.PlatformCapability{
					"openai": {Verdict: tc.verdict, Reason: "anonymous_preflight_result"},
				},
			}
			matched, reason := domain.MatchesCondition(condition, node, nil,
				map[domain.ProbeKind]domain.ProbeObservation{kind: obs}, now)
			if matched != tc.want {
				t.Fatalf("OpenAI verdict %s matched = %t, want %t (reason: %s)", tc.verdict, matched, tc.want, reason)
			}
		})
	}
}

func TestOpenAIPlatformFilterRequiresCurrentConnectionRevision(t *testing.T) {
	now := domain.NowUTC()
	currentRevision := int64(8)
	node := domain.Node{LogicalID: "openai-revision-node", ConnectionRevision: currentRevision}
	kind := domain.ProbeKindAI
	condition := domain.FilterCondition{
		Field: domain.FilterFieldPlatformVerdict,
		Op:    domain.FilterOpEquals, Value: string(domain.VerdictAvailable),
		ProbeKind: &kind, Platform: "openai",
	}

	for _, tc := range []struct {
		name       string
		revision   *int64
		observedAt time.Time
		wantMatch  bool
	}{
		{name: "current revision", revision: &currentRevision, observedAt: now.Add(-time.Hour), wantMatch: true},
		{name: "previous revision", revision: func() *int64 { r := currentRevision - 1; return &r }(), observedAt: now, wantMatch: false},
		{name: "missing revision", revision: nil, observedAt: now, wantMatch: false},
		{name: "future observation", revision: &currentRevision, observedAt: now.Add(time.Second), wantMatch: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obs := domain.ProbeObservation{
				Kind: kind, NodeLogicalID: node.LogicalID,
				Verdict: domain.VerdictAvailable, ConnectionRevision: tc.revision,
				ObservedAt: tc.observedAt,
				Platforms: map[string]domain.PlatformCapability{
					"openai": {Verdict: domain.VerdictAvailable, Reason: "anonymous_preflight_passed"},
				},
			}
			matched, reason := domain.MatchesCondition(condition, node, nil,
				map[domain.ProbeKind]domain.ProbeObservation{kind: obs}, now)
			if matched != tc.wantMatch {
				t.Fatalf("MatchesCondition() = %t, want %t (reason: %s)", matched, tc.wantMatch, reason)
			}
		})
	}
}

func TestOpenAIAndRegionPlatformFilterKeepsAllConditionsRequired(t *testing.T) {
	now := domain.NowUTC()
	revision := int64(12)
	node := domain.Node{LogicalID: "openai-and-node", ConnectionRevision: revision}
	aiKind, geoKind := domain.ProbeKindAI, domain.ProbeKindGeo
	aiObservation := domain.ProbeObservation{
		Kind: aiKind, NodeLogicalID: node.LogicalID,
		Verdict: domain.VerdictAvailable, ConnectionRevision: &revision,
		ObservedAt: now.Add(-time.Minute),
		Platforms: map[string]domain.PlatformCapability{
			"openai":      {Verdict: domain.VerdictAvailable, Reason: "anonymous_preflight_passed"},
			"gemini":      {Verdict: domain.VerdictAvailable},
			"antigravity": {Verdict: domain.VerdictAvailable},
		},
	}
	geoObservation := domain.ProbeObservation{
		Kind: geoKind, NodeLogicalID: node.LogicalID,
		Verdict: domain.VerdictAvailable, Region: "US", ConnectionRevision: &revision,
		ObservedAt: now.Add(-time.Minute),
	}
	observations := map[domain.ProbeKind]domain.ProbeObservation{
		aiKind: aiObservation, geoKind: geoObservation,
	}
	platformCondition := func(platform string) domain.FilterCondition {
		return domain.FilterCondition{
			Field: domain.FilterFieldPlatformVerdict,
			Op:    domain.FilterOpEquals, Value: string(domain.VerdictAvailable),
			ProbeKind: &aiKind, Platform: platform,
		}
	}
	conditions := []domain.FilterCondition{
		platformCondition("gemini"),
		{Field: domain.FilterFieldExitRegion, Op: domain.FilterOpEquals, Value: "US"},
		platformCondition("openai"),
		platformCondition("antigravity"),
	}

	matches, reason := domain.MatchesFilter(&domain.NodeFilterSpec{Conditions: conditions}, node, nil, observations, now)
	if !matches {
		t.Fatalf("all four available/current conditions should pass (reason: %s)", reason)
	}

	for _, tc := range []struct {
		name  string
		index int
	}{
		{name: "Gemini unavailable", index: 0},
		{name: "not US", index: 1},
		{name: "OpenAI preflight unknown", index: 2},
		{name: "Antigravity unavailable", index: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failedConditions := append([]domain.FilterCondition(nil), conditions...)
			if tc.index == 1 {
				failedConditions[tc.index].Value = "CA"
			} else {
				platform := conditions[tc.index].Platform
				aiObservation.Platforms[platform] = domain.PlatformCapability{Verdict: domain.VerdictUnknown}
			}
			if got, failureReason := domain.MatchesFilter(&domain.NodeFilterSpec{Conditions: failedConditions}, node, nil, observations, now); got {
				t.Fatalf("AND filter matched despite failed %s condition (reason: %s)", tc.name, failureReason)
			}
			if tc.index != 1 {
				platform := conditions[tc.index].Platform
				aiObservation.Platforms[platform] = domain.PlatformCapability{Verdict: domain.VerdictAvailable}
			}
		})
	}
}
