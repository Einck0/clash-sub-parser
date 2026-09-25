package sqlite_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestIPRiskRepositoryRejectsSensitiveObservationFixtures(t *testing.T) {
	db, _ := setupTestDB(t)
	repo := sqlite.NewIPRiskObservationRepository(db)
	observedAt := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	score, confidence := 42, 88
	base := domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         "node_0123456789abcdef",
		ExitIdentityDigest:    "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Provider:              "fixture",
		ProviderSchemaVersion: "v1",
		ObservedAt:            observedAt,
		ExpiresAt:             observedAt.Add(time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score,
		Confidence:            &confidence,
		NetworkClass:          domain.NetworkClassDatacenter,
		AnonymizerTraits:      []domain.AnonymizerTrait{domain.TraitHosting},
		EvidenceDigest:        "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		RedactedSummary:       "safe summary",
	}
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, created_at, updated_at)
		VALUES ('node_0123456789abcdef', 'ss', 'fixture', 'secret://node', '2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	if err := providerRepo.Upsert(ctx, &domain.IPRiskProviderSettings{
		Provider: "fixture", SchemaVersion: "v1", Enabled: true,
		SecretReference: "secret://fixture", MaxConcurrency: 1,
		RequestsPerMinute: 1, DailyRequestBudget: 1,
		PerRequestTimeout: time.Second, MaxResponseBytes: 1024,
	}); err != nil {
		t.Fatalf("insert provider settings: %v", err)
	}

	for name, summary := range map[string]string{
		"api key":                          `provider api_key=fixture-api-key`,
		"cookie":                           "provider Cookie: session=fixture-cookie",
		"ipv4":                             "provider observed 198.51.100.7",
		"ipv6":                             "provider observed 2001:db8::7",
		"raw JSON":                         `{"ip":"198.51.100.7","risk":42}`,
		"raw JSON safe keywords":          `{"status":"ok","score":42}`,
		"raw JSON array":                  `[{"risk":42}]`,
		"nested credentials JSON":          `{"credentials":{"token":"fixture-token"}}`,
		"nested auth JSON":                 `{"auth":{"api_key":"fixture-key"}}`,
		"overlong summary":                 strings.Repeat("a", 513),
		"URL query":                        "https://provider.invalid/check?token=fixture-token",
	} {
		t.Run(name, func(t *testing.T) {
			observation := base
			observation.ID = domain.MustNewUUIDv7()
			observation.RedactedSummary = summary
			if err := repo.Create(ctx, &observation); err == nil {
				t.Fatalf("sensitive observation fixture was persisted: %q", summary)
			}
		})
	}

	for name, mutate := range map[string]func(*domain.IPRiskObservation){
		"provider with query": func(o *domain.IPRiskObservation) {
			o.Provider = "https://provider.invalid?token=123"
		},
		"provider with API key": func(o *domain.IPRiskObservation) {
			o.Provider = "api_key=secret"
		},
		"provider with IP": func(o *domain.IPRiskObservation) {
			o.Provider = "198.51.100.7"
		},
		"schema version with query": func(o *domain.IPRiskObservation) {
			o.ProviderSchemaVersion = "v1?token=123"
		},
		"schema version with IP": func(o *domain.IPRiskObservation) {
			o.ProviderSchemaVersion = "2001:db8::7"
		},
	} {
		t.Run(name, func(t *testing.T) {
			observation := base
			observation.ID = domain.MustNewUUIDv7()
			mutate(&observation)
			if err := repo.Create(ctx, &observation); err == nil {
				t.Fatalf("observation with invalid identifier was persisted: %s", name)
			}
		})
	}
}

func TestIPRiskProviderSettingsRepositoryRejectsSensitiveIdentifiers(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)

	base := func() domain.IPRiskProviderSettings {
		return domain.IPRiskProviderSettings{
			Provider:           "fixture",
			SchemaVersion:      "v1",
			SecretReference:    "secret://fixture",
			Enabled:            true,
			MaxConcurrency:     1,
			RequestsPerMinute:  10,
			DailyRequestBudget: 100,
			PerRequestTimeout:  time.Second,
			MaxResponseBytes:   1024,
		}
	}

	for name, mutate := range map[string]func(*domain.IPRiskProviderSettings){
		"provider with URL query": func(s *domain.IPRiskProviderSettings) {
			s.Provider = "https://provider.invalid?api_key=foo"
		},
		"provider with API key": func(s *domain.IPRiskProviderSettings) {
			s.Provider = "api_key=secret"
		},
		"provider with IP": func(s *domain.IPRiskProviderSettings) {
			s.Provider = "198.51.100.7"
		},
		"schema version with URL query": func(s *domain.IPRiskProviderSettings) {
			s.SchemaVersion = "v1?key=val"
		},
		"schema version with IP": func(s *domain.IPRiskProviderSettings) {
			s.SchemaVersion = "2001:db8::7"
		},
	} {
		t.Run(name, func(t *testing.T) {
			settings := base()
			mutate(&settings)
			if err := providerRepo.Upsert(ctx, &settings); err == nil {
				t.Fatalf("provider settings with invalid identifier was persisted: %s", name)
			}
		})
	}
}

func TestIPRiskRepositoryRoundTripContainsNoSensitiveSourceValues(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	if err := providerRepo.Upsert(ctx, &domain.IPRiskProviderSettings{
		Provider: "fixture", SchemaVersion: "v1", Enabled: true,
		SecretReference: "secret://fixture", MaxConcurrency: 1,
		RequestsPerMinute: 1, DailyRequestBudget: 1,
		PerRequestTimeout: time.Second, MaxResponseBytes: 1024,
	}); err != nil {
		t.Fatalf("upsert provider settings: %v", err)
	}
	encoded, err := providerRepo.Get(ctx, "fixture", "v1")
	if err != nil {
		t.Fatalf("get provider settings: %v", err)
	}
	if encoded.SecretReference != "secret://fixture" {
		t.Fatalf("repository lost secret reference needed for runtime resolution")
	}
	text := string(mustJSON(t, encoded))
	for _, forbidden := range []string{"secret://fixture", "api_key", "cookie", "198.51.100.7", "2001:db8::7"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("provider API serialization contains %q: %s", forbidden, text)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return encoded
}
