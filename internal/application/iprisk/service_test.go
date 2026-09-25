package iprisk_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func makeTestPolicy(revisionID string, mode domain.RiskFusionMode, providers []domain.ProviderRef) domain.RiskPolicy {
	if revisionID == "" {
		revisionID = domain.MustNewUUIDv7()
	}
	return domain.RiskPolicy{
		RevisionID: revisionID,
		ProviderSelection: domain.RiskProviderSelection{
			Mode:      mode,
			Providers: providers,
		},
		MaxObservationAge: 24 * time.Hour,
		MinimumConfidence: 50,
		ScoreBands: []domain.ScoreBand{
			{Min: 0, Max: 25, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
			{Min: 26, Max: 60, Band: domain.RiskBandMedium, Action: domain.RiskActionReview},
			{Min: 61, Max: 85, Band: domain.RiskBandHigh, Action: domain.RiskActionBlock},
			{Min: 86, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
		},
		TraitRules: []domain.TraitRule{
			{Trait: domain.TraitTor, Action: domain.RiskActionBlock},
			{Trait: domain.TraitResidentialProxy, Action: domain.RiskActionReview},
		},
		UnknownAction:  domain.RiskActionReview,
		ConflictAction: domain.RiskActionReview,
		ReviewAction:   domain.RiskActionReview,
	}
}

func makeObservation(id, nodeID, provider, schemaVersion string, score, confidence int, traits []domain.AnonymizerTrait, observedAt, expiresAt time.Time, status domain.IPRiskStatus) domain.IPRiskObservation {
	if id == "" {
		id = domain.MustNewUUIDv7()
	}
	var scorePtr *int
	if score >= 0 {
		scorePtr = &score
	}
	var confPtr *int
	if confidence >= 0 {
		confPtr = &confidence
	}
	hexDigest := strings.Repeat("a", 64)
	h := sha256.Sum256([]byte(id + provider + schemaVersion))
	return domain.IPRiskObservation{
		ID:                    id,
		NodeLogicalID:         nodeID,
		ExitIdentityDigest:    "sha256:" + hexDigest,
		Provider:              provider,
		ProviderSchemaVersion: schemaVersion,
		ObservedAt:            observedAt,
		ExpiresAt:             expiresAt,
		Status:                status,
		Score:                 scorePtr,
		Confidence:            confPtr,
		NetworkClass:          domain.NetworkClassResidential,
		AnonymizerTraits:      traits,
		EvidenceDigest:        "sha256:" + hex.EncodeToString(h[:]),
		RedactedSummary:       "observation clean summary",
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:test_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations on test db: %v", err)
	}
	return db
}

func TestSingleProviderDecision(t *testing.T) {
	nodeID := "node_0123456789abcdef"
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	providerRef := domain.ProviderRef{Provider: "scamalytics", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionSingleProvider, []domain.ProviderRef{providerRef})
	svc := iprisk.NewService(nil, nil)

	t.Run("valid low score allows node", func(t *testing.T) {
		obs := makeObservation("", nodeID, "scamalytics", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obs},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionAllow {
			t.Errorf("expected decision allow, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand != domain.RiskBandLow {
			t.Errorf("expected band low, got %s", eval.Summary.RiskBand)
		}
		if len(eval.Decision.ObservationDigestSet) != 1 || eval.Decision.ObservationDigestSet[0] != obs.EvidenceDigest {
			t.Errorf("expected observation digest in decision, got %v", eval.Decision.ObservationDigestSet)
		}
		if err := eval.Decision.Validate(); err != nil {
			t.Errorf("invalid decision: %v", err)
		}
		if err := eval.Summary.Validate(); err != nil {
			t.Errorf("invalid summary: %v", err)
		}
	})

	t.Run("high score blocks node", func(t *testing.T) {
		obs := makeObservation("", nodeID, "scamalytics", "v1", 75, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obs},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionBlock {
			t.Errorf("expected decision block, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand != domain.RiskBandHigh {
			t.Errorf("expected band high, got %s", eval.Summary.RiskBand)
		}
	})

	t.Run("trait rule tor blocks even with low score", func(t *testing.T) {
		obs := makeObservation("", nodeID, "scamalytics", "v1", 5, 90, []domain.AnonymizerTrait{domain.TraitTor}, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obs},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionBlock {
			t.Errorf("expected decision block from trait rule, got %s", eval.Decision.Decision)
		}
		if !strings.Contains(eval.Decision.ReasonCode, "tor") && !strings.Contains(eval.Decision.ReasonCode, "trait") {
			t.Errorf("expected reason code to reference trait/tor, got %s", eval.Decision.ReasonCode)
		}
	})

	t.Run("missing observation defaults to unknown action review", func(t *testing.T) {
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  nil,
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected unknown default to review, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand != domain.RiskBandUnknown {
			t.Errorf("expected band unknown, got %s", eval.Summary.RiskBand)
		}
		if eval.Decision.ReasonCode != "missing_observation" && eval.Decision.ReasonCode != "observation_missing" {
			t.Errorf("unexpected reason code: %s", eval.Decision.ReasonCode)
		}
		if len(eval.Decision.ObservationDigestSet) != 0 {
			t.Errorf("expected empty digest set for missing observation, got %v", eval.Decision.ObservationDigestSet)
		}
		if err := eval.Decision.Validate(); err != nil {
			t.Errorf("invalid decision: %v", err)
		}
		if err := eval.Summary.Validate(); err != nil {
			t.Errorf("invalid summary: %v", err)
		}
	})

	t.Run("expired observation defaults to review", func(t *testing.T) {
		obs := makeObservation("", nodeID, "scamalytics", "v1", 10, 80, nil, now.Add(-5*time.Hour), now.Add(-time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obs},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected decision review for expired observation, got %s", eval.Decision.Decision)
		}
		if eval.Decision.ReasonCode != "observation_expired" {
			t.Errorf("expected reason code observation_expired, got %s", eval.Decision.ReasonCode)
		}
	})

	t.Run("observation exceeding max age defaults to review", func(t *testing.T) {
		obs := makeObservation("", nodeID, "scamalytics", "v1", 10, 80, nil, now.Add(-30*time.Hour), now.Add(10*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obs},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected decision review for over-aged observation, got %s", eval.Decision.Decision)
		}
	})

	t.Run("exit identity mismatch defaults to review", func(t *testing.T) {
		obs := makeObservation("", nodeID, "scamalytics", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		expectedExit := "sha256:" + strings.Repeat("c", 64)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID:      nodeID,
			ExitIdentityDigest: expectedExit,
			Policy:             &policy,
			EvaluatedAt:        now,
			Observations:       []domain.IPRiskObservation{obs},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected decision review on exit identity mismatch, got %s", eval.Decision.Decision)
		}
		if eval.Decision.ReasonCode != "exit_identity_mismatch" {
			t.Errorf("expected reason exit_identity_mismatch, got %s", eval.Decision.ReasonCode)
		}
	})

	t.Run("low confidence defaults to review and never low-risk", func(t *testing.T) {
		obs := makeObservation("", nodeID, "scamalytics", "v1", 5, 20, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obs},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected low confidence to become review, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand == domain.RiskBandLow {
			t.Errorf("low confidence observation must not be marked low risk band")
		}
	})

	t.Run("missing score defaults to review and never low-risk", func(t *testing.T) {
		obs := makeObservation("", nodeID, "scamalytics", "v1", -1, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obs},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected missing score to become review, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand == domain.RiskBandLow {
			t.Errorf("missing score observation must not be marked low risk band")
		}
	})

	t.Run("policy with explicit unknown action block", func(t *testing.T) {
		strictPolicy := policy
		strictPolicy.UnknownAction = domain.RiskActionBlock
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &strictPolicy,
			EvaluatedAt:   now,
			Observations:  nil,
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionBlock {
			t.Errorf("expected strict policy unknown action block, got %s", eval.Decision.Decision)
		}
	})
}

func TestScoreBandsBoundaryTable(t *testing.T) {
	nodeID := "node_0123456789abcdef"
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	providerRef := domain.ProviderRef{Provider: "scamalytics", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionSingleProvider, []domain.ProviderRef{providerRef})
	svc := iprisk.NewService(nil, nil)

	testCases := []struct {
		score          int
		expectedAction domain.RiskAction
		expectedBand   domain.RiskBand
	}{
		{score: 0, expectedAction: domain.RiskActionAllow, expectedBand: domain.RiskBandLow},
		{score: 25, expectedAction: domain.RiskActionAllow, expectedBand: domain.RiskBandLow},
		{score: 26, expectedAction: domain.RiskActionReview, expectedBand: domain.RiskBandMedium},
		{score: 60, expectedAction: domain.RiskActionReview, expectedBand: domain.RiskBandMedium},
		{score: 61, expectedAction: domain.RiskActionBlock, expectedBand: domain.RiskBandHigh},
		{score: 85, expectedAction: domain.RiskActionBlock, expectedBand: domain.RiskBandHigh},
		{score: 86, expectedAction: domain.RiskActionBlock, expectedBand: domain.RiskBandCritical},
		{score: 100, expectedAction: domain.RiskActionBlock, expectedBand: domain.RiskBandCritical},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("score_%d", tc.score), func(t *testing.T) {
			obs := makeObservation("", nodeID, "scamalytics", "v1", tc.score, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
			eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
				NodeLogicalID: nodeID,
				Policy:        &policy,
				EvaluatedAt:   now,
				Observations:  []domain.IPRiskObservation{obs},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if eval.Decision.Decision != tc.expectedAction {
				t.Errorf("score %d: expected action %s, got %s", tc.score, tc.expectedAction, eval.Decision.Decision)
			}
			if eval.Summary.RiskBand != tc.expectedBand {
				t.Errorf("score %d: expected band %s, got %s", tc.score, tc.expectedBand, eval.Summary.RiskBand)
			}
		})
	}
}

func TestNewestObservationWins(t *testing.T) {
	nodeID := "node_0123456789abcdef"
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	providerRef := domain.ProviderRef{Provider: "scamalytics", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionSingleProvider, []domain.ProviderRef{providerRef})
	svc := iprisk.NewService(nil, nil)

	t.Run("newer low score overrides older high score", func(t *testing.T) {
		oldHigh := makeObservation("", nodeID, "scamalytics", "v1", 90, 80, nil, now.Add(-5*time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		newLow := makeObservation("", nodeID, "scamalytics", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)

		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{oldHigh, newLow},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionAllow {
			t.Errorf("expected newer low score to win and allow, got %s", eval.Decision.Decision)
		}
		if eval.Decision.ObservationDigestSet[0] != newLow.EvidenceDigest {
			t.Errorf("expected newLow digest, got %v", eval.Decision.ObservationDigestSet)
		}
	})

	t.Run("newer high score overrides older low score", func(t *testing.T) {
		oldLow := makeObservation("", nodeID, "scamalytics", "v1", 10, 80, nil, now.Add(-5*time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		newHigh := makeObservation("", nodeID, "scamalytics", "v1", 95, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)

		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{oldLow, newHigh},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionBlock {
			t.Errorf("expected newer high score to win and block, got %s", eval.Decision.Decision)
		}
		if eval.Decision.ObservationDigestSet[0] != newHigh.EvidenceDigest {
			t.Errorf("expected newHigh digest, got %v", eval.Decision.ObservationDigestSet)
		}
	})
}

func TestAllMustAllowFusion(t *testing.T) {
	nodeID := "node_0123456789abcdef"
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	providerA := domain.ProviderRef{Provider: "provider_a", SchemaVersion: "v1"}
	providerB := domain.ProviderRef{Provider: "provider_b", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionAllMustAllow, []domain.ProviderRef{providerA, providerB})
	svc := iprisk.NewService(nil, nil)

	t.Run("both providers allow -> overall allow", func(t *testing.T) {
		obsA := makeObservation("", nodeID, "provider_a", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		obsB := makeObservation("", nodeID, "provider_b", "v1", 15, 85, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obsA, obsB},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionAllow {
			t.Errorf("expected overall allow, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand != domain.RiskBandLow {
			t.Errorf("expected band low, got %s", eval.Summary.RiskBand)
		}
		if len(eval.Decision.ObservationDigestSet) != 2 {
			t.Errorf("expected 2 digests in set, got %d", len(eval.Decision.ObservationDigestSet))
		}
	})

	t.Run("one provider blocks -> overall block", func(t *testing.T) {
		obsA := makeObservation("", nodeID, "provider_a", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		obsB := makeObservation("", nodeID, "provider_b", "v1", 90, 85, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obsA, obsB},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionBlock {
			t.Errorf("expected overall block when one provider blocks, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand != domain.RiskBandCritical {
			t.Errorf("expected band critical, got %s", eval.Summary.RiskBand)
		}
	})

	t.Run("one provider allows and one provider missing -> defaults to review and never allow", func(t *testing.T) {
		obsA := makeObservation("", nodeID, "provider_a", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obsA},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected missing provider to trigger review, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand == domain.RiskBandLow {
			t.Errorf("incomplete provider set must not be marked low risk band")
		}
	})

	t.Run("one provider allows and one provider review -> overall review", func(t *testing.T) {
		obsA := makeObservation("", nodeID, "provider_a", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		obsB := makeObservation("", nodeID, "provider_b", "v1", 50, 85, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obsA, obsB},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected overall review, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand != domain.RiskBandMedium {
			t.Errorf("expected band medium, got %s", eval.Summary.RiskBand)
		}
	})
}

func TestMappedHighestRiskFusion(t *testing.T) {
	nodeID := "node_0123456789abcdef"
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	providerA := domain.ProviderRef{Provider: "provider_a", SchemaVersion: "v1"}
	providerB := domain.ProviderRef{Provider: "provider_b", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionHighestRisk, []domain.ProviderRef{providerA, providerB})
	svc := iprisk.NewService(nil, nil)

	t.Run("low and critical -> takes highest critical block", func(t *testing.T) {
		obsA := makeObservation("", nodeID, "provider_a", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		obsB := makeObservation("", nodeID, "provider_b", "v1", 95, 90, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obsA, obsB},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionBlock {
			t.Errorf("expected decision block, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand != domain.RiskBandCritical {
			t.Errorf("expected band critical, got %s", eval.Summary.RiskBand)
		}
	})

	t.Run("low and medium -> takes highest medium review", func(t *testing.T) {
		obsA := makeObservation("", nodeID, "provider_a", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		obsB := makeObservation("", nodeID, "provider_b", "v1", 45, 90, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obsA, obsB},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected decision review, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand != domain.RiskBandMedium {
			t.Errorf("expected band medium, got %s", eval.Summary.RiskBand)
		}
	})

	t.Run("one provider low and one missing -> does not become low risk", func(t *testing.T) {
		obsA := makeObservation("", nodeID, "provider_a", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
		eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
			NodeLogicalID: nodeID,
			Policy:        &policy,
			EvaluatedAt:   now,
			Observations:  []domain.IPRiskObservation{obsA},
		})
		if err != nil {
			t.Fatalf("EvaluateNode failed: %v", err)
		}
		if eval.Decision.Decision != domain.RiskActionReview {
			t.Errorf("expected review for incomplete provider set, got %s", eval.Decision.Decision)
		}
		if eval.Summary.RiskBand == domain.RiskBandLow {
			t.Errorf("must not produce low risk band when provider data is missing")
		}
	})
}

func TestDeterministicDigestAndBatchEvaluation(t *testing.T) {
	node1 := "node_0123456789abcdef"
	node2 := "node_fedcba9876543210"
	node3 := "node_1111222233334444"
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	providerRef := domain.ProviderRef{Provider: "scamalytics", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionSingleProvider, []domain.ProviderRef{providerRef})
	svc := iprisk.NewService(nil, nil)

	obs1 := makeObservation("", node1, "scamalytics", "v1", 10, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
	obs2 := makeObservation("", node2, "scamalytics", "v1", 90, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
	// node3 has no observation (missing)

	batch1, err := svc.EvaluateBatch(context.Background(), iprisk.EvaluateBatchInput{
		NodeLogicalIDs:     []string{node1, node2, node3},
		Policy:             &policy,
		InventoryWatermark: "wm_20260916_v1",
		EvaluatedAt:        now,
		Observations:       []domain.IPRiskObservation{obs1, obs2},
	})
	if err != nil {
		t.Fatalf("EvaluateBatch 1 failed: %v", err)
	}

	// Permute node order in input
	batch2, err := svc.EvaluateBatch(context.Background(), iprisk.EvaluateBatchInput{
		NodeLogicalIDs:     []string{node3, node1, node2},
		Policy:             &policy,
		InventoryWatermark: "wm_20260916_v1",
		EvaluatedAt:        now,
		Observations:       []domain.IPRiskObservation{obs2, obs1},
	})
	if err != nil {
		t.Fatalf("EvaluateBatch 2 failed: %v", err)
	}

	if batch1.DecisionDigest == "" {
		t.Errorf("empty DecisionDigest")
	}
	if !strings.HasPrefix(batch1.DecisionDigest, "sha256:") || len(batch1.DecisionDigest) != 71 {
		t.Errorf("invalid DecisionDigest format: %s", batch1.DecisionDigest)
	}
	if batch1.DecisionDigest != batch2.DecisionDigest {
		t.Errorf("digest mismatch under input permutation: %s vs %s", batch1.DecisionDigest, batch2.DecisionDigest)
	}

	// Verify node admission & exclusion lists
	if len(batch1.AdmittedNodeIDs) != 1 || batch1.AdmittedNodeIDs[0] != node1 {
		t.Errorf("expected admitted node1, got %v", batch1.AdmittedNodeIDs)
	}
	// node2 is blocked, node3 is review (which is excluded under default review action)
	if len(batch1.ExcludedNodeIDs) < 2 {
		t.Errorf("expected at least 2 excluded nodes (block & review), got %v", batch1.ExcludedNodeIDs)
	}

	// Verify individual decisions
	for _, dec := range batch1.Decisions {
		if err := dec.Validate(); err != nil {
			t.Errorf("invalid decision for %s: %v", dec.NodeLogicalID, err)
		}
	}
	for _, sum := range batch1.Summaries {
		if err := sum.Validate(); err != nil {
			t.Errorf("invalid summary: %v", err)
		}
	}
}

func TestRepositoryIntegration(t *testing.T) {
	db := setupTestDB(t)
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	obsRepo := sqlite.NewIPRiskObservationRepository(db)
	policyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	svc := iprisk.NewService(obsRepo, policyRepo)

	ctx := context.Background()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	// Persist provider settings to satisfy foreign keys
	settings := &domain.IPRiskProviderSettings{
		Provider:           "scamalytics",
		SchemaVersion:      "v1",
		Enabled:            true,
		SecretReference:    "secret://scamalytics",
		MaxConcurrency:     1,
		RequestsPerMinute:  10,
		DailyRequestBudget: 100,
		PerRequestTimeout:  1500 * time.Millisecond,
		MaxResponseBytes:   1024,
	}
	if err := providerRepo.Upsert(ctx, settings); err != nil {
		t.Fatalf("upsert provider settings: %v", err)
	}

	// Insert test node to satisfy foreign key
	nodeID := "node_0123456789abcdef"
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, created_at, updated_at)
		VALUES (?, 'ss', 'fixture_node', 'secret://node', '2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`, nodeID)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}

	// Create and persist policy revision
	providerRef := domain.ProviderRef{Provider: "scamalytics", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionSingleProvider, []domain.ProviderRef{providerRef})
	rev := domain.RiskPolicyRevision{
		RiskPolicy: policy,
		Active:     true,
		CreatedAt:  now.Add(-time.Hour),
	}
	if err := policyRepo.Create(ctx, &rev); err != nil {
		t.Fatalf("failed to create policy revision: %v", err)
	}

	// Create and persist observation
	obs := makeObservation("", nodeID, "scamalytics", "v1", 15, 85, nil, now.Add(-30*time.Minute), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
	if err := obsRepo.Create(ctx, &obs); err != nil {
		t.Fatalf("failed to create observation: %v", err)
	}

	// Evaluate single node resolving policy and observations via repository
	eval, err := svc.EvaluateNode(ctx, iprisk.EvaluateNodeInput{
		NodeLogicalID:    nodeID,
		PolicyRevisionID: policy.RevisionID,
		EvaluatedAt:      now,
	})
	if err != nil {
		t.Fatalf("EvaluateNode from repo failed: %v", err)
	}
	if eval.Decision.Decision != domain.RiskActionAllow {
		t.Errorf("expected decision allow, got %s", eval.Decision.Decision)
	}
	if eval.Summary.RiskBand != domain.RiskBandLow {
		t.Errorf("expected band low, got %s", eval.Summary.RiskBand)
	}

	// Evaluate batch with active policy loaded from repo
	batch, err := svc.EvaluateBatch(ctx, iprisk.EvaluateBatchInput{
		NodeLogicalIDs:     []string{nodeID},
		InventoryWatermark: "wm_repo_v1",
		EvaluatedAt:        now,
	})
	if err != nil {
		t.Fatalf("EvaluateBatch from repo failed: %v", err)
	}
	if len(batch.Decisions) != 1 || batch.Decisions[0].Decision != domain.RiskActionAllow {
		t.Errorf("expected 1 allowed decision from batch, got %v", batch.Decisions)
	}
}

func TestConcurrentEvaluationsRaceFree(t *testing.T) {
	svc := iprisk.NewService(nil, nil)
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	providerRef := domain.ProviderRef{Provider: "scamalytics", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionSingleProvider, []domain.ProviderRef{providerRef})

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		nodeID := fmt.Sprintf("node_%016x", i)
		score := (i * 7) % 100
		go func(nid string, sc int) {
			defer wg.Done()
			obs := makeObservation("", nid, "scamalytics", "v1", sc, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
			eval, err := svc.EvaluateNode(context.Background(), iprisk.EvaluateNodeInput{
				NodeLogicalID: nid,
				Policy:        &policy,
				EvaluatedAt:   now,
				Observations:  []domain.IPRiskObservation{obs},
			})
			if err != nil {
				t.Errorf("concurrent evaluate error for %s: %v", nid, err)
				return
			}
			if err := eval.Decision.Validate(); err != nil {
				t.Errorf("concurrent invalid decision for %s: %v", nid, err)
			}
		}(nodeID, score)
	}
	wg.Wait()
}

func TestSecurityRedaction(t *testing.T) {
	nodeID := "node_0123456789abcdef"
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	providerRef := domain.ProviderRef{Provider: "scamalytics", SchemaVersion: "v1"}
	policy := makeTestPolicy("", domain.RiskFusionSingleProvider, []domain.ProviderRef{providerRef})
	svc := iprisk.NewService(nil, nil)

	obs := makeObservation("", nodeID, "scamalytics", "v1", 20, 80, nil, now.Add(-time.Hour), now.Add(12*time.Hour), domain.IPRiskStatusAvailable)
	res, err := svc.EvaluateBatch(context.Background(), iprisk.EvaluateBatchInput{
		NodeLogicalIDs:     []string{nodeID},
		Policy:             &policy,
		InventoryWatermark: "wm_sec_v1",
		EvaluatedAt:        now,
		Observations:       []domain.IPRiskObservation{obs},
	})
	if err != nil {
		t.Fatalf("EvaluateBatch failed: %v", err)
	}

	marshaled, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("failed to marshal batch result: %v", err)
	}
	payload := string(marshaled)

	forbiddenStrings := []string{
		"api_key",
		"apikey",
		"token=",
		"cookie",
		"password=",
		"secret=",
		"raw_payload",
		"raw json",
		"192.168.1.1",
		"10.0.0.1",
		"198.51.100.",
	}
	for _, forbidden := range forbiddenStrings {
		if strings.Contains(strings.ToLower(payload), forbidden) {
			t.Errorf("security violation: marshaled batch result contains forbidden token %q: %s", forbidden, payload)
		}
	}
}
