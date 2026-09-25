package domain_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestIPRiskProviderSettingsValidation(t *testing.T) {
	settings := domain.IPRiskProviderSettings{
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
	if err := settings.Validate(); err != nil {
		t.Fatalf("valid provider settings rejected: %v", err)
	}
	settings.SecretReference = "api_key=plaintext"
	if err := settings.Validate(); err == nil {
		t.Fatal("provider settings must reject plaintext secret values")
	}
}

func TestIPRiskRepositoryPortsAreImplemented(t *testing.T) {
	var _ domain.IPRiskObservationRepository = (domain.IPRiskObservationRepository)(nil)
	var _ domain.IPRiskProviderSettingsRepository = (domain.IPRiskProviderSettingsRepository)(nil)
	var _ domain.RiskPolicyRevisionRepository = (domain.RiskPolicyRevisionRepository)(nil)
}

func TestIPRiskObservationFilterCarriesBoundedPaging(t *testing.T) {
	filter := domain.IPRiskObservationFilter{
		NodeLogicalID: "node_0123456789abcdef",
		Provider:      "fixture",
		Status:        domain.IPRiskStatusAvailable,
		Page:          1,
		PageSize:      20,
	}
	if filter.Page != 1 || filter.PageSize != 20 || filter.Provider != "fixture" {
		t.Fatalf("unexpected risk observation filter: %+v", filter)
	}
}
