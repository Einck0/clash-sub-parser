package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestIPRiskRepositoriesRoundTripNormalizedData(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	observationRepo := sqlite.NewIPRiskObservationRepository(db)
	policyRepo := sqlite.NewRiskPolicyRevisionRepository(db)

	settings := &domain.IPRiskProviderSettings{
		Provider: "fixture", SchemaVersion: "v1", Enabled: true,
		SecretReference: "secret://fixture", MaxConcurrency: 1,
		RequestsPerMinute: 10, DailyRequestBudget: 100,
		PerRequestTimeout: 1500 * time.Millisecond, MaxResponseBytes: 1024,
	}
	if err := providerRepo.Upsert(ctx, settings); err != nil {
		t.Fatalf("upsert provider settings: %v", err)
	}
	gotSettings, err := providerRepo.Get(ctx, "fixture", "v1")
	if err != nil {
		t.Fatalf("get provider settings: %v", err)
	}
	if gotSettings.SecretReference != settings.SecretReference || !gotSettings.Enabled ||
		gotSettings.PerRequestTimeout != settings.PerRequestTimeout {
		t.Fatalf("provider settings round trip mismatch: %+v", gotSettings)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, created_at, updated_at)
		VALUES ('node_0123456789abcdef', 'ss', 'fixture', 'secret://node', '2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	score, confidence := 20, 90
	observation := &domain.IPRiskObservation{
		ID: domain.MustNewUUIDv7(), NodeLogicalID: "node_0123456789abcdef",
		ExitIdentityDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Provider: "fixture", ProviderSchemaVersion: "v1",
		ObservedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC),
		Status:     domain.IPRiskStatusAvailable, Score: &score, Confidence: &confidence,
		NetworkClass: domain.NetworkClassDatacenter, AnonymizerTraits: []domain.AnonymizerTrait{domain.TraitHosting},
		EvidenceDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", RedactedSummary: "safe summary",
	}
	if err := observationRepo.Create(ctx, observation); err != nil {
		t.Fatalf("create observation: %v", err)
	}
	observations, total, err := observationRepo.List(ctx, domain.IPRiskObservationFilter{Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(observations) != 1 {
		t.Fatalf("list observations = %d, %d, %v", len(observations), total, err)
	}
	if observations[0].Score == nil || *observations[0].Score != score || len(observations[0].AnonymizerTraits) != 1 {
		t.Fatalf("observation round trip mismatch: %+v", observations[0])
	}

	policy := &domain.RiskPolicyRevision{RiskPolicy: validPolicyForRepositoryTest(), CreatedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)}
	if err := policyRepo.Create(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if err := policyRepo.SetActive(ctx, policy.RevisionID, true); err != nil {
		t.Fatalf("activate policy: %v", err)
	}
	active, err := policyRepo.GetActive(ctx)
	if err != nil || active.RevisionID != policy.RevisionID || !active.Active {
		t.Fatalf("active policy mismatch: %+v, %v", active, err)
	}
}

func validPolicyForRepositoryTest() domain.RiskPolicy {
	return domain.RiskPolicy{
		RevisionID: domain.MustNewUUIDv7(),
		ProviderSelection: domain.RiskProviderSelection{
			Mode:      domain.RiskFusionSingleProvider,
			Providers: []domain.ProviderRef{{Provider: "fixture", SchemaVersion: "v1"}},
		},
		MaxObservationAge: time.Hour, MinimumConfidence: 50,
		ScoreBands: []domain.ScoreBand{
			{Min: 0, Max: 49, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
			{Min: 50, Max: 79, Band: domain.RiskBandHigh, Action: domain.RiskActionReview},
			{Min: 80, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
		},
	}
}
