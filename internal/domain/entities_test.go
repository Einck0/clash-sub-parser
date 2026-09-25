package domain_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestProbeRunStateTransitions(t *testing.T) {
	run := domain.ProbeRun{
		ID:             domain.MustNewUUIDv7(),
		IdempotencyKey: "test-run-1",
		ActorScope:     "admin",
		ConfigRevision: domain.MustNewUUIDv7(),
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().UTC().Add(time.Minute),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	if run.IsTerminal() {
		t.Fatal("queued state should not be terminal")
	}

	// Valid transition: queued -> running
	if err := run.TransitionTo(domain.ProbeRunStateRunning); err != nil {
		t.Fatalf("valid transition queued -> running failed: %v", err)
	}
	if run.State != domain.ProbeRunStateRunning {
		t.Fatalf("expected state running, got %s", run.State)
	}

	// Invalid transition: running -> queued
	if err := run.TransitionTo(domain.ProbeRunStateQueued); err == nil {
		t.Fatal("invalid transition running -> queued should return error")
	}

	// Valid transition: running -> succeeded
	if err := run.TransitionTo(domain.ProbeRunStateSucceeded); err != nil {
		t.Fatalf("valid transition running -> succeeded failed: %v", err)
	}
	if !run.IsTerminal() {
		t.Fatal("succeeded state must be terminal")
	}

	// Terminal states cannot transition anywhere
	if err := run.TransitionTo(domain.ProbeRunStateRunning); err == nil {
		t.Fatal("terminal state transition to running must fail")
	}
	if err := run.TransitionTo(domain.ProbeRunStateFailed); err == nil {
		t.Fatal("terminal state transition to failed must fail")
	}
}

func TestGroupEdgeSelfLoopPrevention(t *testing.T) {
	parentID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()

	validEdge := domain.GroupEdge{
		ID:            domain.MustNewUUIDv7(),
		ParentGroupID: parentID,
		ChildGroupID:  &childID,
		Position:      0,
	}

	if err := domain.ValidateGroupEdge(validEdge); err != nil {
		t.Fatalf("valid GroupEdge failed validation: %v", err)
	}

	// Self-loop: ParentGroupID == ChildGroupID
	selfLoopEdge := domain.GroupEdge{
		ID:            domain.MustNewUUIDv7(),
		ParentGroupID: parentID,
		ChildGroupID:  &parentID,
		Position:      0,
	}

	if err := domain.ValidateGroupEdge(selfLoopEdge); err == nil {
		t.Fatal("expected self-loop GroupEdge to fail validation, but it passed")
	}

	// Both child group and node set -> invalid
	nodeID := "node_1234567890abcdef"
	ambiguousEdge := domain.GroupEdge{
		ID:            domain.MustNewUUIDv7(),
		ParentGroupID: parentID,
		ChildGroupID:  &childID,
		NodeLogicalID: &nodeID,
		Position:      0,
	}
	if err := domain.ValidateGroupEdge(ambiguousEdge); err == nil {
		t.Fatal("edge specifying both child group and node must be rejected")
	}

	// Neither child group nor node set -> invalid
	emptyEdge := domain.GroupEdge{
		ID:            domain.MustNewUUIDv7(),
		ParentGroupID: parentID,
		Position:      0,
	}
	if err := domain.ValidateGroupEdge(emptyEdge); err == nil {
		t.Fatal("edge specifying neither child group nor node must be rejected")
	}
}

func TestPublicationRevocation(t *testing.T) {
	pub := domain.Publication{
		ID:              domain.MustNewUUIDv7(),
		Target:          domain.TargetClash,
		SnapshotDigest:  "sha256:abcd",
		CompilerVersion: "v1.0.0",
		TokenHash:       "sha256:token",
		State:           domain.PublicationStateActive,
		CreatedAt:       time.Now().UTC(),
	}

	if !pub.IsActive() {
		t.Fatal("new publication should be active")
	}

	now := time.Now().UTC()
	if err := pub.Revoke(now); err != nil {
		t.Fatalf("revoking publication failed: %v", err)
	}

	if pub.IsActive() {
		t.Fatal("revoked publication must not be active")
	}
	if pub.State != domain.PublicationStateRevoked {
		t.Fatalf("expected state revoked, got %s", pub.State)
	}
	if pub.RevokedAt == nil || !pub.RevokedAt.Equal(now) {
		t.Fatalf("revoked_at not set properly: %v", pub.RevokedAt)
	}

	// Repeated revocation
	if err := pub.Revoke(now); err == nil {
		t.Fatal("revoking an already revoked publication should return an error")
	}
}

func TestSettingsValidation(t *testing.T) {
	defaultSettings := domain.DefaultSettings()
	if err := defaultSettings.Validate(); err != nil {
		t.Fatalf("DefaultSettings failed validation: %v", err)
	}

	// Invalid concurrency window (< 10 or > 32 per D4 spec: default 16, range 10-20, hard limit 32)
	invalidSettings := defaultSettings
	invalidSettings.ProbeConcurrencyWindow = 5
	if err := invalidSettings.Validate(); err == nil {
		t.Fatal("ProbeConcurrencyWindow < 10 must fail validation")
	}

	invalidSettings.ProbeConcurrencyWindow = 50
	if err := invalidSettings.Validate(); err == nil {
		t.Fatal("ProbeConcurrencyWindow > 32 must fail validation")
	}

	// Invalid max page size (> 100 per D8 spec)
	invalidSettings = defaultSettings
	invalidSettings.MaxPageSize = 200
	if err := invalidSettings.Validate(); err == nil {
		t.Fatal("MaxPageSize > 100 must fail validation")
	}
}
