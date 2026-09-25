package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestProbeScheduleDefaultsAndValidation(t *testing.T) {
	def := domain.DefaultProbeSchedule()
	if def.Enabled {
		t.Fatal("expected default schedule to be disabled")
	}
	if def.IntervalSeconds != 3600 {
		t.Fatalf("expected default interval 3600s, got %d", def.IntervalSeconds)
	}
	if len(def.Kinds) != 1 || def.Kinds[0] != domain.ProbeKindBaseline {
		t.Fatalf("expected default kind baseline, got %+v", def.Kinds)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("default schedule should be valid: %v", err)
	}

	tests := []struct {
		name      string
		schedule  domain.ProbeSchedule
		wantError bool
	}{
		{
			name: "valid custom schedule",
			schedule: domain.ProbeSchedule{
				Enabled:         true,
				IntervalSeconds: 900,
				Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindGeo},
			},
			wantError: false,
		},
		{
			name: "interval too short (< 60s)",
			schedule: domain.ProbeSchedule{
				IntervalSeconds: 30,
				Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
			},
			wantError: true,
		},
		{
			name: "interval too long (> 7 days)",
			schedule: domain.ProbeSchedule{
				IntervalSeconds: 700000,
				Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
			},
			wantError: true,
		},
		{
			name: "empty kinds",
			schedule: domain.ProbeSchedule{
				IntervalSeconds: 3600,
				Kinds:           []domain.ProbeKind{},
			},
			wantError: true,
		},
		{
			name: "invalid kind",
			schedule: domain.ProbeSchedule{
				IntervalSeconds: 3600,
				Kinds:           []domain.ProbeKind{"invalid_kind"},
			},
			wantError: true,
		},
		{
			name: "duplicate kind",
			schedule: domain.ProbeSchedule{
				IntervalSeconds: 3600,
				Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindBaseline},
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.schedule.Validate()
			if tc.wantError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestProbeBatchStateLifecycle(t *testing.T) {
	batch := domain.ProbeBatch{
		ID:         "0191e4a0-0000-7000-8000-000000000001",
		WindowAt:   domain.NowUTC(),
		Generation: 1,
		State:      domain.ProbeBatchStatePending,
	}

	// Pending -> Running: valid
	if !batch.CanTransitionTo(domain.ProbeBatchStateRunning) {
		t.Fatal("expected pending -> running to be valid")
	}
	if err := batch.TransitionTo(domain.ProbeBatchStateRunning); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// Running -> Succeeded: valid
	if !batch.CanTransitionTo(domain.ProbeBatchStateSucceeded) {
		t.Fatal("expected running -> succeeded to be valid")
	}
	if err := batch.TransitionTo(domain.ProbeBatchStateSucceeded); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// Succeeded is terminal -> cannot transition to Running or Failed
	if !batch.State.IsTerminal() {
		t.Fatal("expected succeeded state to be terminal")
	}
	if batch.CanTransitionTo(domain.ProbeBatchStateRunning) {
		t.Fatal("expected transition from terminal state to be rejected")
	}
	if err := batch.TransitionTo(domain.ProbeBatchStateRunning); err == nil {
		t.Fatal("expected conflict error when transitioning from terminal state")
	}
}

func TestProbeScheduleJSONRoundtrip(t *testing.T) {
	due := domain.NowUTC().Add(time.Hour)
	schedule := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 1800,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindSpeed},
		NextDueAt:       &due,
		Generation:      3,
		UpdatedAt:       domain.NowUTC(),
	}

	data, err := json.Marshal(schedule)
	if err != nil {
		t.Fatalf("failed to marshal schedule: %v", err)
	}

	var decoded domain.ProbeSchedule
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal schedule: %v", err)
	}

	if decoded.Enabled != schedule.Enabled ||
		decoded.IntervalSeconds != schedule.IntervalSeconds ||
		len(decoded.Kinds) != 2 ||
		decoded.Generation != schedule.Generation {
		t.Fatalf("decoded schedule mismatch: %+v vs %+v", decoded, schedule)
	}
}
