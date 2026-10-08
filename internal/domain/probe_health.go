package domain

import (
	"strings"
	"time"
)

// BaselineHealth is the shared, conservative overall connectivity classification.
// Capability observations cannot substitute for the baseline. Historical NULL revisions
// and ambiguous network failures are unknown, never evidence of a broken node.
type BaselineHealth string

const (
	BaselineHealthy   BaselineHealth = "healthy"
	BaselineDegraded  BaselineHealth = "degraded"
	BaselineUnhealthy BaselineHealth = "unhealthy"
	BaselineUnknown   BaselineHealth = "unknown"
)

// EvaluateBaselineHealth returns overall health and whether baseline latency can be displayed.
// A nil observation, expired observation, or unversioned observation cannot certify this connection.
func EvaluateBaselineHealth(obs *ProbeObservation, revision int64, now time.Time, freshness time.Duration) (BaselineHealth, bool) {
	if obs == nil || obs.Kind != ProbeKindBaseline || obs.ConnectionRevision == nil || revision < 1 ||
		*obs.ConnectionRevision != revision || freshness <= 0 || obs.ObservedAt.After(now) ||
		now.Sub(obs.ObservedAt) >= freshness {
		return BaselineUnknown, false
	}
	if obs.Verdict == VerdictAvailable {
		return BaselineHealthy, obs.LatencyMS > 0
	}
	if obs.Verdict == VerdictRestricted {
		return BaselineDegraded, obs.LatencyMS > 0
	}
	// Only a specifically identified failure at the node connection/handshake stage
	// certifies node unavailability. Legacy transport_error lacks that evidence.
	if obs.Verdict == VerdictError && strings.Contains(obs.RedactedSummary, "reason=node_connect_failed") {
		return BaselineUnhealthy, false
	}
	return BaselineUnknown, false
}

// NodeHealthCategory defines the mutually exclusive health classification of a node.
type NodeHealthCategory string

const (
	HealthCategoryHealthy      NodeHealthCategory = "healthy"
	HealthCategoryDegraded     NodeHealthCategory = "degraded"
	HealthCategoryUnhealthy    NodeHealthCategory = "unhealthy"
	HealthCategoryUndetermined NodeHealthCategory = "undetermined"
	HealthCategoryUntested     NodeHealthCategory = "untested"
)

// ClassifyNodeHealth evaluates the mutually exclusive health category of a node based on its latest observations.
// - If observations map is empty or nil: HealthCategoryUntested
// - If fresh baseline probe exists with current revision:
//   - VerdictAvailable -> HealthCategoryHealthy
//   - VerdictRestricted -> HealthCategoryDegraded
//   - VerdictError with reason=node_connect_failed -> HealthCategoryUnhealthy
//
// - Otherwise (has observations, but baseline is missing, stale, revision-mismatched, future-dated, or inconclusive): HealthCategoryUndetermined
func ClassifyNodeHealth(observations map[ProbeKind]ProbeObservation, currentRevision int64, now time.Time, freshness time.Duration) NodeHealthCategory {
	if len(observations) == 0 {
		return HealthCategoryUntested
	}
	var baselinePtr *ProbeObservation
	if baselineObs, hasBaseline := observations[ProbeKindBaseline]; hasBaseline {
		obsCopy := baselineObs
		baselinePtr = &obsCopy
	}
	health, _ := EvaluateBaselineHealth(baselinePtr, currentRevision, now, freshness)
	switch health {
	case BaselineHealthy:
		return HealthCategoryHealthy
	case BaselineDegraded:
		return HealthCategoryDegraded
	case BaselineUnhealthy:
		return HealthCategoryUnhealthy
	default:
		return HealthCategoryUndetermined
	}
}

// ClassifyNodeHealthFromSlice evaluates the health category from a slice of observations.
func ClassifyNodeHealthFromSlice(observations []ProbeObservation, currentRevision int64, now time.Time, freshness time.Duration) NodeHealthCategory {
	if len(observations) == 0 {
		return HealthCategoryUntested
	}
	obsMap := make(map[ProbeKind]ProbeObservation, len(observations))
	for _, obs := range observations {
		existing, ok := obsMap[obs.Kind]
		if !ok || obs.ObservedAt.After(existing.ObservedAt) || (obs.ObservedAt.Equal(existing.ObservedAt) && obs.ID > existing.ID) {
			obsMap[obs.Kind] = obs
		}
	}
	return ClassifyNodeHealth(obsMap, currentRevision, now, freshness)
}
