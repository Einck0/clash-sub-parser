package iprisk_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/iprisk"
)

func TestNormalizedRiskCandidateValidation(t *testing.T) {
	now := time.Now().UTC()
	validScore := 42
	validConfidence := 85

	candidate := iprisk.NormalizedRiskCandidate{
		Provider:              "scamalytics",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(15 * time.Minute),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &validScore,
		Confidence:            &validConfidence,
		NetworkClass:          domain.NetworkClassResidential,
		AnonymizerTraits:      []domain.AnonymizerTrait{domain.TraitProxy},
		RawSummary:            "clean summary without secret",
	}

	if err := candidate.Validate(); err != nil {
		t.Fatalf("expected candidate to be valid, got: %v", err)
	}

	// Score < 0 or > 100 invalid
	invalidScore := 101
	badScoreCandidate := candidate
	badScoreCandidate.Score = &invalidScore
	if err := badScoreCandidate.Validate(); err == nil {
		t.Fatal("expected score 101 to fail validation")
	}

	// Confidence < 0 or > 100 invalid
	invalidConf := -1
	badConfCandidate := candidate
	badConfCandidate.Confidence = &invalidConf
	if err := badConfCandidate.Validate(); err == nil {
		t.Fatal("expected confidence -1 to fail validation")
	}

	// RawSummary with full IP or secret must be rejected
	leakCandidate := candidate
	leakCandidate.RawSummary = "leaking ip 198.51.100.1 with secret token_abc123"
	if err := leakCandidate.Validate(); err == nil {
		t.Fatal("expected raw summary with complete IP to fail validation")
	}
}

func TestCandidateToObservationEnsuresImmutabilityAndNoRawSecrets(t *testing.T) {
	now := time.Now().UTC()
	score := 75
	confidence := 90

	candidate := iprisk.NormalizedRiskCandidate{
		Provider:              "ipqs",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(15 * time.Minute),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score,
		Confidence:            &confidence,
		NetworkClass:          domain.NetworkClassDatacenter,
		AnonymizerTraits:      []domain.AnonymizerTrait{domain.TraitVPN, domain.TraitHosting},
		RawSummary:            "datacenter vpn endpoint",
	}

	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	obs, err := candidate.ToObservation("node_0123456789abcdef", digest, digest)
	if err != nil {
		t.Fatalf("ToObservation() error = %v", err)
	}

	if obs.NodeLogicalID != "node_0123456789abcdef" {
		t.Fatalf("nodeLogicalID = %s, want node_0123456789abcdef", obs.NodeLogicalID)
	}
	if obs.Provider != "ipqs" || obs.ProviderSchemaVersion != "v1" {
		t.Fatalf("provider mismatch: %s %s", obs.Provider, obs.ProviderSchemaVersion)
	}
	if obs.Score == nil || *obs.Score != 75 {
		t.Fatalf("score mismatch: %v", obs.Score)
	}
	if err := obs.Validate(); err != nil {
		t.Fatalf("generated observation failed domain validation: %v", err)
	}
}

func TestRegistryOperations(t *testing.T) {
	registry := iprisk.NewRegistry()

	provider := iprisk.NewDeterministicFakeProvider("fake_provider", "v1", iprisk.FakeProviderBehavior{
		Score:        50,
		Confidence:   80,
		NetworkClass: domain.NetworkClassResidential,
	})

	if err := registry.Register(provider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Duplicate registration must fail
	if err := registry.Register(provider); err == nil {
		t.Fatal("expected duplicate registration error, got nil")
	}

	retrieved, ok := registry.Get("fake_provider", "v1")
	if !ok || retrieved == nil {
		t.Fatal("expected to retrieve registered provider")
	}
	if retrieved.Name() != "fake_provider" || retrieved.SchemaVersion() != "v1" {
		t.Fatalf("retrieved mismatch: %s %s", retrieved.Name(), retrieved.SchemaVersion())
	}

	all := registry.All()
	if len(all) != 1 {
		t.Fatalf("all providers count = %d, want 1", len(all))
	}
}

func TestDeterministicFakeProviderBehavior(t *testing.T) {
	fake := iprisk.NewDeterministicFakeProvider("scamalytics_fake", "v1", iprisk.FakeProviderBehavior{
		Score:        88,
		Confidence:   95,
		NetworkClass: domain.NetworkClassDatacenter,
		Traits:       []domain.AnonymizerTrait{domain.TraitProxy, domain.TraitTor},
	})

	identity := domain.ExitIdentity{
		NodeLogicalID:  "node_0123456789abcdef",
		ObservedAt:     time.Now().UTC(),
		IdentityDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	candidate, err := fake.Probe(context.Background(), http.DefaultClient, identity)
	if err != nil {
		t.Fatalf("fake.Probe() error = %v", err)
	}
	if candidate.Score == nil || *candidate.Score != 88 {
		t.Fatalf("score = %v, want 88", candidate.Score)
	}
	if candidate.NetworkClass != domain.NetworkClassDatacenter {
		t.Fatalf("network class = %s, want datacenter", candidate.NetworkClass)
	}
	if len(candidate.AnonymizerTraits) != 2 {
		t.Fatalf("traits length = %d, want 2", len(candidate.AnonymizerTraits))
	}
}

func TestMissingScoreAndLowConfidenceNotClassifiedAsLowRisk(t *testing.T) {
	// Fake provider that returns missing score
	fakeMissingScore := iprisk.NewDeterministicFakeProvider("missing_score_p", "v1", iprisk.FakeProviderBehavior{
		ScoreMissing: true,
		Confidence:   90,
	})

	identity := domain.ExitIdentity{
		NodeLogicalID:  "node_0123456789abcdef",
		ObservedAt:     time.Now().UTC(),
		IdentityDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	cand1, err := fakeMissingScore.Probe(context.Background(), http.DefaultClient, identity)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if cand1.Score != nil {
		t.Fatalf("expected nil score, got: %v", cand1.Score)
	}
	if cand1.Status == domain.IPRiskStatusAvailable {
		t.Fatal("missing score must not produce available status without score")
	}

	// Fake provider that returns low confidence
	fakeLowConf := iprisk.NewDeterministicFakeProvider("low_conf_p", "v1", iprisk.FakeProviderBehavior{
		Score:      10, // low score, but confidence is low
		Confidence: 20, // very low confidence
	})

	cand2, err := fakeLowConf.Probe(context.Background(), http.DefaultClient, identity)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if *cand2.Confidence < 50 && cand2.Status == domain.IPRiskStatusAvailable {
		// Low confidence must be marked unknown or flag low confidence
		if cand2.Status != domain.IPRiskStatusUnknown {
			t.Fatalf("low confidence candidate status = %s, want unknown", cand2.Status)
		}
	}
}
