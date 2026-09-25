package domain

import (
	"fmt"
	"time"
)

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
	ID                string       `json:"id"`
	ProbeRunID        string       `json:"probe_run_id"`
	NodeLogicalID     string       `json:"node_logical_id"`
	Kind              ProbeKind    `json:"kind"`
	Verdict           ProbeVerdict `json:"verdict"`
	EvidenceDigest    string       `json:"evidence_digest"`
	ObservedAt        time.Time    `json:"observed_at"`
	LatencyMS         int64        `json:"latency_ms"`
	RedactedSummary   string       `json:"redacted_summary"`
	CredentialVersion *int         `json:"credential_version,omitempty"`
}

// HasValidCredentialVersion verifies whether the observation's recorded credential version
// matches the node's current credential version (fail-closed if unversioned, unknown, or mismatched).
func (o ProbeObservation) HasValidCredentialVersion(nodeVersion int) bool {
	if o.CredentialVersion == nil || nodeVersion <= 0 {
		return false
	}
	return *o.CredentialVersion == nodeVersion
}
