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
