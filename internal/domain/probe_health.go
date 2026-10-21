package domain

import (
	"time"
)

// BaselineHealth is the shared overall connectivity classification.
// Capability observations cannot substitute for the current connection's baseline.
type BaselineHealth string

const (
	BaselineHealthy   BaselineHealth = "healthy"
	// BaselineDegraded is retained for source compatibility; canonical health never emits it.
	BaselineDegraded  BaselineHealth = "degraded"
	BaselineUnhealthy BaselineHealth = "unhealthy"
	BaselineUnknown   BaselineHealth = "unknown"
)

// EvaluateBaselineHealth returns the canonical health and whether real baseline latency can be displayed.
// Freshness is retained for API compatibility but does not change health; connection revision does.
func EvaluateBaselineHealth(obs *ProbeObservation, revision int64, now time.Time, freshness time.Duration) (BaselineHealth, bool) {
	_ = freshness
	if obs == nil || obs.Kind != ProbeKindBaseline || obs.ConnectionRevision == nil || revision < 1 ||
		*obs.ConnectionRevision != revision || obs.ObservedAt.IsZero() || obs.ObservedAt.After(now) {
		return BaselineUnknown, false
	}
	switch obs.Verdict {
	case VerdictAvailable, VerdictRestricted:
		return BaselineHealthy, obs.LatencyMS > 0
	case VerdictError:
		return BaselineUnhealthy, false
	default:
		return BaselineUnknown, false
	}
}

// NodeHealthCategory defines the mutually exclusive canonical health classification of a node.
type NodeHealthCategory string

const (
	HealthCategoryHealthy      NodeHealthCategory = "healthy"
	// Retained for compatibility; canonical classification does not emit degraded.
	HealthCategoryDegraded     NodeHealthCategory = "degraded"
	HealthCategoryUnhealthy    NodeHealthCategory = "unhealthy"
	// Retained for compatibility; missing or invalid current baseline is untested.
	HealthCategoryUndetermined NodeHealthCategory = "undetermined"
	HealthCategoryUntested     NodeHealthCategory = "untested"
)

// ClassifyNodeHealth classifies only the current baseline observation; auxiliary observations do not establish health.
// Missing or invalid baseline evidence is untested. Observation age does not expire a valid current-revision result.
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
	case BaselineUnhealthy:
		return HealthCategoryUnhealthy
	default:
		return HealthCategoryUntested
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
