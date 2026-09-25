package domain_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestIPRiskSecretReferenceRequiresReferenceScheme(t *testing.T) {
	settings := validProviderSettings()
	for _, value := range []string{"sk_live_123", "Bearer abc", "https://provider.invalid/key"} {
		settings.SecretReference = value
		if err := settings.Validate(); err == nil {
			t.Errorf("plaintext secret reference %q should be rejected", value)
		}
	}
}

func validProviderSettings() domain.IPRiskProviderSettings {
	return domain.IPRiskProviderSettings{
		Provider: "fixture", SchemaVersion: "v1", SecretReference: "secret://fixture",
		MaxConcurrency: 1, RequestsPerMinute: 1, DailyRequestBudget: 1,
		PerRequestTimeout: 1500 * time.Millisecond, MaxResponseBytes: 1024,
	}
}
