package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// PlatformCapability represents a fine-grained, evidence-based platform capability observation.
type PlatformCapability struct {
	Verdict    ProbeVerdict `json:"verdict"`
	LatencyMS  *int64       `json:"latency_ms,omitempty"`
	ObservedAt *time.Time   `json:"observed_at,omitempty"`
	Summary    string       `json:"summary,omitempty"`
	Region     string       `json:"region,omitempty"`
	SubTier    string       `json:"sub_tier,omitempty"`
	Throughput *float64     `json:"throughput,omitempty"`
	RiskScore  string       `json:"risk_score,omitempty"`
	Reason     string       `json:"reason,omitempty"`
}

// ObservationEvidenceData contains structured metadata persisted alongside an observation.
type ObservationEvidenceData struct {
	Region     string                        `json:"region,omitempty"`
	SubTier    string                        `json:"sub_tier,omitempty"`
	Throughput *float64                      `json:"throughput,omitempty"`
	RiskScore  string                        `json:"risk_score,omitempty"`
	Platforms  map[string]PlatformCapability `json:"platforms,omitempty"`
	Reason     string                        `json:"reason,omitempty"`
}

// ProbeObservation represents an immutable observation made during a probe execution.
type ProbeObservation struct {
	ID                 string                        `json:"id"`
	ProbeRunID         string                        `json:"probe_run_id"`
	NodeLogicalID      string                        `json:"node_logical_id"`
	Kind               ProbeKind                     `json:"kind"`
	Verdict            ProbeVerdict                  `json:"verdict"`
	EvidenceDigest     string                        `json:"evidence_digest"`
	ObservedAt         time.Time                     `json:"observed_at"`
	LatencyMS          int64                         `json:"latency_ms"`
	RedactedSummary    string                        `json:"redacted_summary"`
	// Nil identifies pre-migration history, which cannot certify a current connection.
	ConnectionRevision *int64                        `json:"connection_revision,omitempty"`
	EvidenceData       string                        `json:"evidence_data,omitempty"`
	Region             string                        `json:"region,omitempty"`
	SubTier            string                        `json:"sub_tier,omitempty"`
	Throughput         *float64                      `json:"throughput,omitempty"`
	RiskScore          string                        `json:"risk_score,omitempty"`
	Platforms          map[string]PlatformCapability `json:"platforms,omitempty"`
}

// SyncEvidenceData synchronizes structured fields with EvidenceData JSON.
func (o *ProbeObservation) SyncEvidenceData() {
	if o == nil {
		return
	}
	if o.EvidenceData == "" && (o.Region != "" || o.SubTier != "" || o.Throughput != nil || o.RiskScore != "" || len(o.Platforms) > 0) {
		ed := ObservationEvidenceData{
			Region:     o.Region,
			SubTier:    o.SubTier,
			Throughput: o.Throughput,
			RiskScore:  o.RiskScore,
			Platforms:  o.Platforms,
		}
		if b, err := json.Marshal(ed); err == nil {
			o.EvidenceData = string(b)
		}
	} else if o.EvidenceData != "" && o.Region == "" && o.SubTier == "" && o.Throughput == nil && o.RiskScore == "" && len(o.Platforms) == 0 {
		var ed ObservationEvidenceData
		if err := json.Unmarshal([]byte(o.EvidenceData), &ed); err == nil {
			o.Region = ed.Region
			o.SubTier = ed.SubTier
			o.Throughput = ed.Throughput
			o.RiskScore = ed.RiskScore
			o.Platforms = ed.Platforms
		}
	}
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
