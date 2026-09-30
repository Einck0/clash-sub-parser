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
	// Only a specifically identified failure at the node connection/handshake stage
	// certifies node unavailability. Legacy transport_error lacks that evidence.
	if obs.Verdict == VerdictError && strings.Contains(obs.RedactedSummary, "reason=node_connect_failed") {
		return BaselineUnhealthy, false
	}
	return BaselineUnknown, false
}
