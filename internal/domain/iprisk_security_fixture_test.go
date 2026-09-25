package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestIPRiskSecurityFixturesNeverSurviveErrorRedaction(t *testing.T) {
	fixtures := map[string]string{
		"api key in JSON":  `provider response {"api_key":"fixture-api-key"}`,
		"cookie header":    "provider response Cookie: session=fixture-cookie; account=fixture-account",
		"complete IPv4":    "provider contacted 198.51.100.7",
		"complete IPv6":    "provider contacted 2001:db8::7",
		"raw JSON payload": `{"ip":"198.51.100.7","risk":42}`,
		"error URL query":  "GET https://provider.invalid/check?api_key=fixture-api-key",
	}
	for name, input := range fixtures {
		t.Run(name, func(t *testing.T) {
			redacted := domain.RedactSensitiveInfo(input)
			for _, secret := range []string{"fixture-api-key", "fixture-cookie", "fixture-account", "198.51.100.7", "2001:db8::7"} {
				if strings.Contains(redacted, secret) {
					t.Fatalf("redacted error contains %q: %s", secret, redacted)
				}
			}
		})
	}
}

func TestIPRiskDecisionRejectsSensitiveReasonFixtures(t *testing.T) {
	fixtures := []string{
		`provider_api_key_fixture`,
		`provider_cookie_fixture`,
		`provider_ip_198.51.100.7`,
		`provider_ip_2001:db8::7`,
		`provider_payload_{"risk":42}`,
	}
	for _, reason := range fixtures {
		t.Run(reason, func(t *testing.T) {
			decision := validSecurityDecision()
			decision.ReasonCode = reason
			if err := decision.Validate(); err == nil {
				t.Fatalf("sensitive reason code %q was accepted", reason)
			}
		})
	}
}

func TestIPRiskSummaryIsAnExplicitAPIContractWithoutSensitiveFields(t *testing.T) {
	policyID := domain.MustNewUUIDv7()
	summary := domain.IPRiskSummary{
		Decision:              domain.RiskActionReview,
		RiskBand:              domain.RiskBandUnknown,
		Provider:              "fixture",
		ProviderSchemaVersion: "v1",
		ObservedAt:            time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
		ExpiresAt:             time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC),
		Status:                domain.IPRiskStatusUnknown,
		ReasonCode:            "observation_missing",
		PolicyRevisionID:      &policyID,
	}
	if err := summary.Validate(); err != nil {
		t.Fatalf("valid API summary rejected: %v", err)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal API summary: %v", err)
	}
	payload := string(encoded)
	for _, forbidden := range []string{"api_key", "cookie", "raw_payload", "198.51.100.7", "2001:db8::7", "evidence_digest", "score", "confidence"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("API summary contains forbidden field or value %q: %s", forbidden, payload)
		}
	}
}

func validSecurityDecision() domain.RiskDecision {
	return domain.RiskDecision{
		NodeLogicalID:        "node_0123456789abcdef",
		PolicyRevisionID:     domain.MustNewUUIDv7(),
		Decision:             domain.RiskActionReview,
		ReasonCode:           "observation_missing",
		ObservationDigestSet: []string{"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		EvaluatedAt:          time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
	}
}

func TestIPRiskSummaryRejectsSensitiveAndInvalidFields(t *testing.T) {
	policyID := domain.MustNewUUIDv7()
	base := func() domain.IPRiskSummary {
		return domain.IPRiskSummary{
			Decision:              domain.RiskActionReview,
			RiskBand:              domain.RiskBandUnknown,
			Provider:              "fixture",
			ProviderSchemaVersion: "v1",
			ObservedAt:            time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
			ExpiresAt:             time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC),
			Status:                domain.IPRiskStatusUnknown,
			ReasonCode:            "observation_missing",
			PolicyRevisionID:      &policyID,
		}
	}

	for name, mutate := range map[string]func(*domain.IPRiskSummary){
		"provider with API key": func(s *domain.IPRiskSummary) {
			s.Provider = "api_key=secret"
		},
		"provider with URL query": func(s *domain.IPRiskSummary) {
			s.Provider = "https://provider.invalid/check?api_key=secret"
		},
		"provider with IPv4": func(s *domain.IPRiskSummary) {
			s.Provider = "198.51.100.7"
		},
		"provider with IPv6": func(s *domain.IPRiskSummary) {
			s.Provider = "2001:db8::7"
		},
		"provider with sensitive token": func(s *domain.IPRiskSummary) {
			s.Provider = "provider_api_key_test"
		},
		"schema version with API key": func(s *domain.IPRiskSummary) {
			s.ProviderSchemaVersion = "api_key=secret"
		},
		"schema version with IP": func(s *domain.IPRiskSummary) {
			s.ProviderSchemaVersion = "198.51.100.7"
		},
		"schema version with URL query": func(s *domain.IPRiskSummary) {
			s.ProviderSchemaVersion = "?token=secret"
		},
		"reason code with sensitive token": func(s *domain.IPRiskSummary) {
			s.ReasonCode = "provider_api_key_fixture"
		},
		"reason code with cookie token": func(s *domain.IPRiskSummary) {
			s.ReasonCode = "provider_cookie_fixture"
		},
		"reason code with IP": func(s *domain.IPRiskSummary) {
			s.ReasonCode = "provider_ip_198.51.100.7"
		},
	} {
		t.Run(name, func(t *testing.T) {
			summary := base()
			mutate(&summary)
			if err := summary.Validate(); err == nil {
				t.Fatalf("expected summary validation error for %s", name)
			}
		})
	}
}

func TestIPRiskObservationRejectsRawJSONAndOverlongSummaryAndSensitiveProviders(t *testing.T) {
	observedAt := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	score := 42
	confidence := 88
	base := func() domain.IPRiskObservation {
		return domain.IPRiskObservation{
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
		"raw JSON object": func(o *domain.IPRiskObservation) {
			o.RedactedSummary = `{"status":"ok","score":42}`
		},
		"raw JSON array": func(o *domain.IPRiskObservation) {
			o.RedactedSummary = `[{"risk":42}]`
		},
		"nested credential fields in JSON": func(o *domain.IPRiskObservation) {
			o.RedactedSummary = `{"credentials":{"token":"fixture-token"}}`
		},
		"nested auth in JSON": func(o *domain.IPRiskObservation) {
			o.RedactedSummary = `{"auth":{"api_key":"fixture-key"}}`
		},
		"summary exceeding max length": func(o *domain.IPRiskObservation) {
			o.RedactedSummary = strings.Repeat("a", 513)
		},
	} {
		t.Run(name, func(t *testing.T) {
			obs := base()
			mutate(&obs)
			if err := obs.Validate(); err == nil {
				t.Fatalf("expected observation validation error for %s", name)
			}
		})
	}
}

func TestIPRiskProviderSettingsRejectsSensitiveAndInvalidIdentifiers(t *testing.T) {
	base := func() domain.IPRiskProviderSettings {
		return domain.IPRiskProviderSettings{
			Provider:           "fixture",
			SchemaVersion:      "v1",
			SecretReference:    "secret://fixture",
			Enabled:            true,
			MaxConcurrency:     1,
			RequestsPerMinute:  10,
			DailyRequestBudget: 100,
			PerRequestTimeout:  5 * time.Second,
			MaxResponseBytes:   1024,
		}
	}

	for name, mutate := range map[string]func(*domain.IPRiskProviderSettings){
		"provider with URL query": func(s *domain.IPRiskProviderSettings) {
			s.Provider = "https://provider.invalid?api_key=foo"
		},
		"provider with API key assignment": func(s *domain.IPRiskProviderSettings) {
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
		"schema version with sensitive token": func(s *domain.IPRiskProviderSettings) {
			s.SchemaVersion = "v1_api_key"
		},
	} {
		t.Run(name, func(t *testing.T) {
			settings := base()
			mutate(&settings)
			if err := settings.Validate(); err == nil {
				t.Fatalf("expected provider settings validation error for %s", name)
			}
		})
	}
}
