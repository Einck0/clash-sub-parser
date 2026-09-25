package domain

import (
	"fmt"
	"strings"
	"time"
)

// ProbeSchedule represents the configuration for periodic capability probing across active nodes.
type ProbeSchedule struct {
	Enabled         bool        `json:"enabled"`
	IntervalSeconds int         `json:"interval_seconds"`
	Kinds           []ProbeKind `json:"kinds"`
	NextDueAt       *time.Time  `json:"next_due_at,omitempty"`
	Generation      int64       `json:"generation"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// DefaultProbeSchedule returns the standard baseline configuration (disabled by default).
func DefaultProbeSchedule() ProbeSchedule {
	return ProbeSchedule{
		Enabled:         false,
		IntervalSeconds: 3600, // 1 hour default
		Kinds:           []ProbeKind{ProbeKindBaseline},
		NextDueAt:       nil,
		Generation:      0,
		UpdatedAt:       NowUTC(),
	}
}

// Validate checks that probe schedule parameters adhere to system bounds.
// Bounds: IntervalSeconds between 60s and 604800s (7 days).
// Kinds must be non-empty and each kind must be a valid ProbeKind.
func (s ProbeSchedule) Validate() error {
	if s.IntervalSeconds < 60 || s.IntervalSeconds > 604800 {
		return NewValidationError("invalid_probe_schedule_interval",
			fmt.Sprintf("interval_seconds must be between 60 and 604800, got %d", s.IntervalSeconds))
	}
	if len(s.Kinds) == 0 {
		return NewValidationError("invalid_probe_schedule_kinds", "probe schedule must configure at least one probe kind")
	}
	seen := make(map[ProbeKind]bool, len(s.Kinds))
	for _, k := range s.Kinds {
		if !k.IsValid() {
			return NewValidationError("invalid_probe_kind", fmt.Sprintf("invalid probe kind: %s", k))
		}
		if seen[k] {
			return NewValidationError("duplicate_probe_kind", fmt.Sprintf("duplicate probe kind: %s", k))
		}
		seen[k] = true
	}
	return nil
}

// ProbeBatchState represents the lifecycle states of a periodic probe execution batch.
type ProbeBatchState string

const (
	ProbeBatchStatePending   ProbeBatchState = "pending"
	ProbeBatchStateRunning   ProbeBatchState = "running"
	ProbeBatchStateSucceeded ProbeBatchState = "succeeded"
	ProbeBatchStateFailed    ProbeBatchState = "failed"
	ProbeBatchStateCancelled ProbeBatchState = "cancelled"
	ProbeBatchStateExpired   ProbeBatchState = "expired"
)

var validProbeBatchStates = map[ProbeBatchState]bool{
	ProbeBatchStatePending:   true,
	ProbeBatchStateRunning:   true,
	ProbeBatchStateSucceeded: true,
	ProbeBatchStateFailed:    true,
	ProbeBatchStateCancelled: true,
	ProbeBatchStateExpired:   true,
}

func (s ProbeBatchState) IsValid() bool {
	return validProbeBatchStates[s]
}

func (s ProbeBatchState) IsTerminal() bool {
	return s == ProbeBatchStateSucceeded || s == ProbeBatchStateFailed ||
		s == ProbeBatchStateCancelled || s == ProbeBatchStateExpired
}

func ParseProbeBatchState(s string) (ProbeBatchState, error) {
	st := ProbeBatchState(strings.ToLower(strings.TrimSpace(s)))
	if !st.IsValid() {
		return "", NewValidationError("invalid_probe_batch_state", fmt.Sprintf("unsupported probe batch state: %s", s))
	}
	return st, nil
}

// ProbeBatchCounts captures execution and node coverage metrics for a batch.
type ProbeBatchCounts struct {
	TotalNodes     int `json:"total_nodes"`
	DispatchedRuns int `json:"dispatched_runs"`
	CompletedRuns  int `json:"completed_runs"`
	SkippedNodes   int `json:"skipped_nodes"`
}

// ProbeBatch represents a coordinated periodic probing execution window across active nodes.
type ProbeBatch struct {
	ID            string           `json:"id"`
	WindowAt      time.Time        `json:"window_at"`
	Generation    int64            `json:"generation"`
	Owner         string           `json:"owner"`
	LeaseUntil    *time.Time       `json:"lease_until,omitempty"`
	State         ProbeBatchState  `json:"state"`
	RunIDs        []string         `json:"run_ids"`
	Counts        ProbeBatchCounts `json:"counts"`
	RedactedError string           `json:"redacted_error,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// CanTransitionTo validates if the batch state transition is permissible.
func (b *ProbeBatch) CanTransitionTo(target ProbeBatchState) bool {
	if !target.IsValid() {
		return false
	}
	if b.State.IsTerminal() {
		return false
	}
	switch b.State {
	case ProbeBatchStatePending:
		return target == ProbeBatchStateRunning ||
			target == ProbeBatchStateCancelled ||
			target == ProbeBatchStateExpired
	case ProbeBatchStateRunning:
		return target == ProbeBatchStateSucceeded ||
			target == ProbeBatchStateFailed ||
			target == ProbeBatchStateCancelled ||
			target == ProbeBatchStateExpired
	default:
		return false
	}
}

// TransitionTo updates the batch state if permissible.
func (b *ProbeBatch) TransitionTo(target ProbeBatchState) error {
	if !b.CanTransitionTo(target) {
		return NewConflictError("invalid_batch_state_transition",
			fmt.Sprintf("cannot transition probe batch %s from %s to %s", b.ID, b.State, target))
	}
	b.State = target
	b.UpdatedAt = NowUTC()
	return nil
}

// UpdateProbeScheduleRequest is the API DTO for updating the probe schedule.
type UpdateProbeScheduleRequest struct {
	Enabled         *bool        `json:"enabled,omitempty"`
	IntervalSeconds *int         `json:"interval_seconds,omitempty"`
	Kinds           *[]ProbeKind `json:"kinds,omitempty"`
}
