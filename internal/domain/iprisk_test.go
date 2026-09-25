package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestRiskDomainEnumsParseCanonicalValues(t *testing.T) {
	for _, value := range []domain.RiskAction{
		domain.RiskActionAllow, domain.RiskActionReview, domain.RiskActionBlock, domain.RiskActionUnknown,
	} {
		parsed, err := domain.ParseRiskAction(" " + string(value) + " ")
		if err != nil || parsed != value {
			t.Fatalf("ParseRiskAction(%q) = %q, %v", value, parsed, err)
		}
	}
	for _, value := range []domain.RiskBand{
		domain.RiskBandLow, domain.RiskBandMedium, domain.RiskBandHigh, domain.RiskBandCritical, domain.RiskBandUnknown,
	} {
		parsed, err := domain.ParseRiskBand(string(value))
		if err != nil || parsed != value {
			t.Fatalf("ParseRiskBand(%q) = %q, %v", value, parsed, err)
		}
	}
	for _, value := range []domain.IPRiskStatus{
		domain.IPRiskStatusAvailable, domain.IPRiskStatusUnknown, domain.IPRiskStatusError, domain.IPRiskStatusStale,
	} {
		parsed, err := domain.ParseIPRiskStatus(string(value))
		if err != nil || parsed != value {
			t.Fatalf("ParseIPRiskStatus(%q) = %q, %v", value, parsed, err)
		}
	}
	for _, value := range []domain.NetworkClass{
		domain.NetworkClassResidential, domain.NetworkClassDatacenter, domain.NetworkClassMobile,
		domain.NetworkClassBusiness, domain.NetworkClassUnknown,
	} {
		parsed, err := domain.ParseNetworkClass(string(value))
		if err != nil || parsed != value {
			t.Fatalf("ParseNetworkClass(%q) = %q, %v", value, parsed, err)
		}
	}
	for _, value := range []domain.AnonymizerTrait{
		domain.TraitProxy, domain.TraitVPN, domain.TraitTor, domain.TraitResidentialProxy,
		domain.TraitHosting, domain.TraitUnknown,
	} {
		parsed, err := domain.ParseAnonymizerTrait(string(value))
		if err != nil || parsed != value {
			t.Fatalf("ParseAnonymizerTrait(%q) = %q, %v", value, parsed, err)
		}
	}
	for _, value := range []domain.RiskFusionMode{
		domain.RiskFusionSingleProvider, domain.RiskFusionAllMustAllow, domain.RiskFusionHighestRisk,
	} {
		parsed, err := domain.ParseRiskFusionMode(string(value))
		if err != nil || parsed != value {
			t.Fatalf("ParseRiskFusionMode(%q) = %q, %v", value, parsed, err)
		}
	}
}

func TestRiskDomainEnumsRejectUnsupportedValues(t *testing.T) {
	for name, parse := range map[string]func(string) error{
		"action":  func(value string) error { _, err := domain.ParseRiskAction(value); return err },
		"band":    func(value string) error { _, err := domain.ParseRiskBand(value); return err },
		"status":  func(value string) error { _, err := domain.ParseIPRiskStatus(value); return err },
		"network": func(value string) error { _, err := domain.ParseNetworkClass(value); return err },
		"trait":   func(value string) error { _, err := domain.ParseAnonymizerTrait(value); return err },
		"fusion":  func(value string) error { _, err := domain.ParseRiskFusionMode(value); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := parse("not-a-risk-value"); err == nil {
				t.Fatal("unsupported enum value should be rejected")
			}
		})
	}
}

func TestIPRiskDomainRejectsSensitiveAndInvalidObservations(t *testing.T) {
	observation := validObservation()
	if err := observation.Validate(); err != nil {
		t.Fatalf("valid observation rejected: %v", err)
	}

	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}
	payload := string(encoded)
	for _, forbidden := range []string{"198.51.100.7", "api_key", "raw_payload", "cookie"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("observation JSON contains forbidden value %q: %s", forbidden, payload)
		}
	}

	for name, mutate := range map[string]func(*domain.IPRiskObservation){
		"score below zero":             func(value *domain.IPRiskObservation) { score := -1; value.Score = &score },
		"confidence above one hundred": func(value *domain.IPRiskObservation) { confidence := 101; value.Confidence = &confidence },
		"expires before observed":      func(value *domain.IPRiskObservation) { value.ExpiresAt = value.ObservedAt.Add(-time.Second) },
		"unknown trait with another trait": func(value *domain.IPRiskObservation) {
			value.AnonymizerTraits = []domain.AnonymizerTrait{domain.TraitUnknown, domain.TraitProxy}
		},
		"sensitive summary": func(value *domain.IPRiskObservation) {
			value.RedactedSummary = "provider response api_key=secret"
		},
		"query summary": func(value *domain.IPRiskObservation) {
			value.RedactedSummary = "https://provider.invalid/check?token=secret"
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := validObservation()
			mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRiskPolicyRequiresCompleteNonOverlappingBands(t *testing.T) {
	policy := validPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	invalid := policy
	invalid.ScoreBands = []domain.ScoreBand{
		{Min: 0, Max: 70, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
		{Min: 70, Max: 100, Band: domain.RiskBandHigh, Action: domain.RiskActionBlock},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("overlapping score bands should be rejected")
	}

	invalid = policy
	invalid.UnknownAction = domain.RiskAction("")
	if invalid.EffectiveUnknownAction() != domain.RiskActionReview {
		t.Fatalf("unknown action default = %q, want %q", invalid.EffectiveUnknownAction(), domain.RiskActionReview)
	}
}

func TestRiskDecisionValidationRejectsInvalidDecision(t *testing.T) {
	decision := domain.RiskDecision{
		NodeLogicalID:        "node_0123456789abcdef",
		PolicyRevisionID:     domain.MustNewUUIDv7(),
		Decision:             domain.RiskActionReview,
		ReasonCode:           "observation_missing",
		ObservationDigestSet: []string{"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		EvaluatedAt:          time.Now().UTC(),
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("valid decision rejected: %v", err)
	}
	decision.Decision = domain.RiskAction("allow-all")
	if err := decision.Validate(); err == nil {
		t.Fatal("invalid decision action should be rejected")
	}
}

func TestRiskPolicyRejectsAmbiguousProviderSelectionAndTraitRules(t *testing.T) {
	policy := validPolicy()
	policy.ProviderSelection.Providers = append(policy.ProviderSelection.Providers,
		domain.ProviderRef{Provider: "other", SchemaVersion: "v1"})
	if err := policy.Validate(); err == nil {
		t.Fatal("single-provider policy must select exactly one provider")
	}

	policy = validPolicy()
	policy.TraitRules = append(policy.TraitRules,
		domain.TraitRule{Trait: domain.TraitProxy, Action: domain.RiskActionBlock})
	if err := policy.Validate(); err == nil {
		t.Fatal("duplicate trait rules must be rejected")
	}
}

func TestRiskDecisionRejectsSensitiveReasonCode(t *testing.T) {
	decision := domain.RiskDecision{
		NodeLogicalID:    "node_0123456789abcdef",
		PolicyRevisionID: domain.MustNewUUIDv7(),
		Decision:         domain.RiskActionReview,
		ReasonCode:       "provider_query_https://risk.invalid/check?token=secret",
		EvaluatedAt:      time.Now().UTC(),
	}
	if err := decision.Validate(); err == nil {
		t.Fatal("risk decision reason code must not contain sensitive URL query data")
	}
}

func TestExitIdentityValidationRequiresDigestAndSafeCountry(t *testing.T) {
	identity := domain.ExitIdentity{
		NodeLogicalID:  "node_0123456789abcdef",
		ObservedAt:     time.Now().UTC(),
		IdentityDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		CountryCode:    "US",
	}
	if err := identity.Validate(); err != nil {
		t.Fatalf("valid exit identity rejected: %v", err)
	}

	identity.IdentityDigest = "not-a-digest"
	if err := identity.Validate(); err == nil {
		t.Fatal("exit identity must use a sha256 digest")
	}

	identity.IdentityDigest = "198.51.100.7"
	if err := identity.Validate(); err == nil {
		t.Fatal("complete IP must not be accepted as an exit identity digest")
	}

	identity.IdentityDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	identity.CountryCode = "us"
	if err := identity.Validate(); err == nil {
		t.Fatal("country code must use uppercase ISO alpha-2 form")
	}
}

func TestRiskPolicyUsesSafeDefaultsAndRejectsDuplicateRules(t *testing.T) {
	policy := validPolicy()
	policy.ProviderSelection.Providers = nil
	policy.UnknownAction = ""
	policy.ConflictAction = ""
	policy.ReviewAction = ""
	if policy.EffectiveUnknownAction() != domain.RiskActionReview ||
		policy.EffectiveConflictAction() != domain.RiskActionReview ||
		policy.EffectiveReviewAction() != domain.RiskActionReview {
		t.Fatal("risk policy defaults must use review for unknown, conflict, and review")
	}

	policy = validPolicy()
	policy.TraitRules = append(policy.TraitRules,
		domain.TraitRule{Trait: domain.TraitProxy, Action: domain.RiskActionBlock})
	if err := policy.Validate(); err == nil {
		t.Fatal("duplicate trait rules must be rejected")
	}
}

func validObservation() domain.IPRiskObservation {
	observedAt := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	score := 42
	confidence := 88
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
		RedactedSummary:       "risk result from provider; network=datacenter",
	}
}

func validPolicy() domain.RiskPolicy {
	return domain.RiskPolicy{
		RevisionID: domain.MustNewUUIDv7(),
		ProviderSelection: domain.RiskProviderSelection{
			Mode:      domain.RiskFusionSingleProvider,
			Providers: []domain.ProviderRef{{Provider: "fixture", SchemaVersion: "v1"}},
		},
		MaxObservationAge: 24 * time.Hour,
		MinimumConfidence: 70,
		ScoreBands: []domain.ScoreBand{
			{Min: 0, Max: 39, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
			{Min: 40, Max: 69, Band: domain.RiskBandMedium, Action: domain.RiskActionReview},
			{Min: 70, Max: 89, Band: domain.RiskBandHigh, Action: domain.RiskActionBlock},
			{Min: 90, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
		},
		TraitRules:   []domain.TraitRule{{Trait: domain.TraitProxy, Action: domain.RiskActionReview}},
		ReviewAction: domain.RiskActionReview,
	}
}
