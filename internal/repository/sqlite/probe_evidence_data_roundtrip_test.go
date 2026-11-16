package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestProbeObservationOpenAIEvidenceDataSQLiteRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	runRepo := sqlite.NewProbeRunRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	now := time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)

	if err := runRepo.Create(ctx, &domain.ProbeRun{
		ID:             "run-openai-evidence-roundtrip",
		IdempotencyKey: "openai-evidence-roundtrip",
		ActorScope:     "test",
		ConfigRevision: "test-revision",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("create probe run: %v", err)
	}

	observedAt := now.Add(-time.Minute)
	latency := int64(125)
	observation := &domain.ProbeObservation{
		ID:            "obs-openai-evidence-roundtrip",
		ProbeRunID:    "run-openai-evidence-roundtrip",
		NodeLogicalID: "node-openai-evidence-roundtrip",
		Kind:          domain.ProbeKindAI,
		Verdict:       domain.VerdictUnknown,
		ObservedAt:    observedAt,
		Platforms: map[string]domain.PlatformCapability{
			"openai": {
				Verdict:       domain.VerdictUnknown,
				LatencyMS:     &latency,
				Reason:        "endpoint_reachable_capability_unverified",
				EvidenceScope: "network_entry",
			},
		},
	}

	if err := obsRepo.Create(ctx, observation); err != nil {
		t.Fatalf("create OpenAI probe observation: %v", err)
	}
	if observation.EvidenceData == "" {
		t.Fatal("Create did not encode OpenAI platform evidence into evidence_data")
	}

	var persistedEvidence string
	if err := db.QueryRowContext(ctx,
		`SELECT evidence_data FROM probe_observations WHERE id = ?`, observation.ID,
	).Scan(&persistedEvidence); err != nil {
		t.Fatalf("read persisted evidence_data: %v", err)
	}
	if persistedEvidence != observation.EvidenceData {
		t.Fatalf("persisted evidence_data changed: got %q, want %q", persistedEvidence, observation.EvidenceData)
	}

	got, err := obsRepo.GetByID(ctx, observation.ID)
	if err != nil {
		t.Fatalf("read OpenAI probe observation: %v", err)
	}
	openAI, ok := got.Platforms["openai"]
	if !ok {
		t.Fatalf("OpenAI platform evidence missing after SQLite round trip: %+v", got.Platforms)
	}
	if openAI.Verdict != domain.VerdictUnknown || openAI.Reason != "endpoint_reachable_capability_unverified" || openAI.EvidenceScope != "network_entry" {
		t.Fatalf("OpenAI capability evidence did not round-trip: %+v", openAI)
	}
	if openAI.LatencyMS == nil || *openAI.LatencyMS != latency {
		t.Fatalf("OpenAI latency evidence did not round-trip: got %v, want %d", openAI.LatencyMS, latency)
	}
	if got.EvidenceData != persistedEvidence {
		t.Fatalf("read evidence_data changed: got %q, want %q", got.EvidenceData, persistedEvidence)
	}
}
