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

func TestClassifyNodeHealthAndConservation(t *testing.T) {
	now := time.Date(2026, 9, 30, 5, 0, 0, 0, time.UTC)
	rev := int64(3)
	freshness := time.Hour

	// 1. Untested: 0 observations
	if cat := domain.ClassifyNodeHealth(nil, rev, now, freshness); cat != domain.HealthCategoryUntested {
		t.Fatalf("expected untested for nil map, got %s", cat)
	}
	if cat := domain.ClassifyNodeHealth(map[domain.ProbeKind]domain.ProbeObservation{}, rev, now, freshness); cat != domain.HealthCategoryUntested {
		t.Fatalf("expected untested for empty map, got %s", cat)
	}

	// 2. AI available, no baseline: MUST be undetermined (AI cannot fake healthy!)
	aiObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindAI: {
			Kind:               domain.ProbeKindAI,
			Verdict:            domain.VerdictAvailable,
			ObservedAt:         now.Add(-time.Minute),
			ConnectionRevision: &rev,
		},
	}
	if cat := domain.ClassifyNodeHealth(aiObs, rev, now, freshness); cat != domain.HealthCategoryUndetermined {
		t.Fatalf("expected undetermined for AI available without baseline, got %s", cat)
	}

	// 3. Healthy: fresh baseline, available, matching revision
	healthyObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindBaseline: {
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			LatencyMS:          50,
			ObservedAt:         now.Add(-time.Minute),
			ConnectionRevision: &rev,
		},
	}
	if cat := domain.ClassifyNodeHealth(healthyObs, rev, now, freshness); cat != domain.HealthCategoryHealthy {
		t.Fatalf("expected healthy, got %s", cat)
	}

	// 3b. Degraded: fresh baseline, restricted, matching revision
	degradedObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindBaseline: {
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictRestricted,
			LatencyMS:          120,
			ObservedAt:         now.Add(-time.Minute),
			ConnectionRevision: &rev,
		},
	}
	if cat := domain.ClassifyNodeHealth(degradedObs, rev, now, freshness); cat != domain.HealthCategoryDegraded {
		t.Fatalf("expected degraded, got %s", cat)
	}

	// 4. Unhealthy: fresh baseline, error with node_connect_failed
	unhealthyObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindBaseline: {
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictError,
			RedactedSummary:    "reason=node_connect_failed",
			ObservedAt:         now.Add(-time.Minute),
			ConnectionRevision: &rev,
		},
	}
	if cat := domain.ClassifyNodeHealth(unhealthyObs, rev, now, freshness); cat != domain.HealthCategoryUnhealthy {
		t.Fatalf("expected unhealthy, got %s", cat)
	}

	// 5. Stale baseline (> freshness): undetermined
	staleObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindBaseline: {
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			ObservedAt:         now.Add(-2 * time.Hour),
			ConnectionRevision: &rev,
		},
	}
	if cat := domain.ClassifyNodeHealth(staleObs, rev, now, freshness); cat != domain.HealthCategoryUndetermined {
		t.Fatalf("expected undetermined for stale baseline, got %s", cat)
	}

	// 6. Future timestamp baseline: undetermined
	futureObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindBaseline: {
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			ObservedAt:         now.Add(time.Hour),
			ConnectionRevision: &rev,
		},
	}
	if cat := domain.ClassifyNodeHealth(futureObs, rev, now, freshness); cat != domain.HealthCategoryUndetermined {
		t.Fatalf("expected undetermined for future baseline, got %s", cat)
	}

	// 7. Revision mismatch: undetermined
	oldRev := rev - 1
	mismatchObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindBaseline: {
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			ObservedAt:         now.Add(-time.Minute),
			ConnectionRevision: &oldRev,
		},
	}
	if cat := domain.ClassifyNodeHealth(mismatchObs, rev, now, freshness); cat != domain.HealthCategoryUndetermined {
		t.Fatalf("expected undetermined for revision mismatch, got %s", cat)
	}

	// 8. Transport error: undetermined
	transportErrObs := map[domain.ProbeKind]domain.ProbeObservation{
		domain.ProbeKindBaseline: {
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictError,
			RedactedSummary:    "reason=transport_error",
			ObservedAt:         now.Add(-time.Minute),
			ConnectionRevision: &rev,
		},
	}
	if cat := domain.ClassifyNodeHealth(transportErrObs, rev, now, freshness); cat != domain.HealthCategoryUndetermined {
		t.Fatalf("expected undetermined for transport error, got %s", cat)
	}

	// 9. Conservation verification
	pool := domain.ProbePoolStatus{
		TotalCount:         100,
		HealthyCount:       40,
		DegradedCount:      10,
		UnavailableCount:   20,
		UndeterminedCount:  15,
		UntestedCount:      15,
		AvailableCount:     50, // 40 + 10
		QueueNodesCount:    5,
		ProbingCount:       3,
		QueuedWaitingCount: 2,
	}
	if !pool.ValidateConservation() {
		t.Fatalf("expected pool to satisfy conservation, but ValidateConservation returned false")
	}

	// Invalid total sum: should fail
	badPool := pool
	badPool.TotalCount = 99
	if badPool.ValidateConservation() {
		t.Fatalf("expected ValidateConservation to return false for mismatching total")
	}

	// Invalid available sum: should fail
	badAvail := pool
	badAvail.AvailableCount = 49
	if badAvail.ValidateConservation() {
		t.Fatalf("expected ValidateConservation to return false for mismatching available")
	}
}
