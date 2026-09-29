package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// VerdictUnavailable aliases VerdictError for nodes that failed baseline connectivity or dial checks.
const VerdictUnavailable = VerdictError

// ProbeObservationNodePager is an optional extension of ProbeObservationRepository
// that performs SQL-level LIMIT/OFFSET pagination and exact COUNT(*) total for node observations.
type ProbeObservationNodePager interface {
	ListByNodePaginated(ctx context.Context, nodeLogicalID string, page, pageSize int) ([]ProbeObservation, int, error)
}

// ProbeRun represents a bounded capability probing job across a set of nodes.
type ProbeRun struct {
	ID             string        `json:"id"`
	IdempotencyKey string        `json:"idempotency_key"`
	ActorScope     string        `json:"actor_scope"`
	ConfigRevision string        `json:"config_revision"`
	State          ProbeRunState `json:"state"`
	DeadlineAt     time.Time     `json:"deadline_at"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// IsTerminal returns whether the probe run has reached an immutable terminal state.
func (r *ProbeRun) IsTerminal() bool {
	return r.State.IsTerminal()
}

// CanTransitionTo validates if the state transition from current state to target state is permissible.
func (r *ProbeRun) CanTransitionTo(target ProbeRunState) bool {
	if !target.IsValid() {
		return false
	}
	if r.IsTerminal() {
		return false
	}

	switch r.State {
	case ProbeRunStateQueued:
		return target == ProbeRunStateRunning ||
			target == ProbeRunStateCancelled ||
			target == ProbeRunStateExpired
	case ProbeRunStateRunning:
		return target == ProbeRunStateSucceeded ||
			target == ProbeRunStateFailed ||
			target == ProbeRunStateCancelled ||
			target == ProbeRunStateExpired
	default:
		return false
	}
}

// TransitionTo updates the probe run state if valid, or returns a domain conflict/validation error.
func (r *ProbeRun) TransitionTo(target ProbeRunState) error {
	if !r.CanTransitionTo(target) {
		return NewConflictError("invalid_state_transition",
			fmt.Sprintf("cannot transition probe run %s from %s to %s", r.ID, r.State, target))
	}
	r.State = target
	r.UpdatedAt = NowUTC()
	return nil
}

// ProbeObservation represents an immutable observation made during a probe execution.
type ProbeObservation struct {
	ID              string       `json:"id"`
	ProbeRunID      string       `json:"probe_run_id"`
	NodeLogicalID   string       `json:"node_logical_id"`
	Kind            ProbeKind    `json:"kind"`
	Verdict         ProbeVerdict `json:"verdict"`
	EvidenceDigest  string       `json:"evidence_digest"`
	ObservedAt      time.Time    `json:"observed_at"`
	LatencyMS       int64        `json:"latency_ms"`
	RedactedSummary string       `json:"redacted_summary"`
}

// ComputeProbeEvidenceDigest computes the canonical SHA-256 evidence digest for a probe observation.
func ComputeProbeEvidenceDigest(runID, nodeLogicalID, profileVersion string, verdict ProbeVerdict, statusCode int, reason string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%d\x00%s", runID, nodeLogicalID, profileVersion, verdict, statusCode, reason)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// ProbePoolStatus represents the real-time status of the node probe pool and active node health counts.
type ProbePoolStatus struct {
	QueueNodesCount    int       `json:"queue_nodes_count"`
	ProbingCount       int       `json:"probing_count"`
	QueuedWaitingCount int       `json:"queued_waiting_count"`
	UntestedCount      int       `json:"untested_count"`
	TotalCount         int       `json:"total_count"`
	UnavailableCount   int       `json:"unavailable_count"`
	AvailableCount     int       `json:"available_count"`
	HealthyCount       int       `json:"healthy_count"`
	DegradedCount      int       `json:"degraded_count"`
	ProbingNodeIDs     []string  `json:"probing_node_ids"`
	QueuedNodeIDs      []string  `json:"queued_node_ids"`
	UpdatedAt          time.Time `json:"updated_at"`
}
