package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestIPRiskProviderSettingsNeverExposeSecretReferenceInJSON(t *testing.T) {
	settings := domain.IPRiskProviderSettings{
		Provider: "fixture", SchemaVersion: "v1", SecretReference: "secret://fixture",
		MaxConcurrency: 1, RequestsPerMinute: 1, DailyRequestBudget: 1,
		PerRequestTimeout: time.Second, MaxResponseBytes: 1024,
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal provider settings: %v", err)
	}
	if strings.Contains(string(encoded), "secret://fixture") || strings.Contains(string(encoded), "secret_reference") {
		t.Fatalf("provider settings JSON exposes secret reference: %s", encoded)
	}
}
