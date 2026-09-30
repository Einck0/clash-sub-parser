package domain_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestEvaluateBaselineHealthRequiresCurrentVersionAndEvidence(t *testing.T) {
	now := time.Date(2026, 9, 30, 5, 0, 0, 0, time.UTC)
	rev := int64(3)
	obs := domain.ProbeObservation{Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ObservedAt: now.Add(-time.Minute), LatencyMS: 20, ConnectionRevision: &rev}
	currentRevision := rev
	check := func(want domain.BaselineHealth, latency bool) {
		t.Helper()
		status, valid := domain.EvaluateBaselineHealth(&obs, currentRevision, now, time.Hour)
		if status != want || valid != latency {
			t.Fatalf("status %s latency %v want %s %v", status, valid, want, latency)
		}
	}
	check(domain.BaselineHealthy, true)
	obs.ConnectionRevision = nil
	check(domain.BaselineUnknown, false)
	obs.ConnectionRevision = &rev
	currentRevision++
	check(domain.BaselineUnknown, false)
	currentRevision--
	obs.ObservedAt = now.Add(-2 * time.Hour)
	check(domain.BaselineUnknown, false)
	obs.ObservedAt = now
	obs.Kind = domain.ProbeKindAI
	check(domain.BaselineUnknown, false)
	obs.Kind = domain.ProbeKindBaseline
	obs.Verdict = domain.VerdictError
	obs.RedactedSummary = "profile=baseline reason=credentials_unavailable"
	check(domain.BaselineUnknown, false)
	obs.RedactedSummary = "profile=baseline reason=transport_error"
	check(domain.BaselineUnknown, false)
	obs.RedactedSummary = "profile=baseline reason=node_connect_failed"
	check(domain.BaselineUnhealthy, false)
}
