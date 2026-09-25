package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func TestIPRiskAPIContractDoesNotExposeSensitiveFixtures(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node_0123456789abcdef", nil)
	rec := httptest.NewRecorder()
	transporthttp.WriteSuccess(rec, req, http.StatusOK, summary)

	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode API response: %v", err)
	}
	payload := string(rec.Body.Bytes())
	for _, forbidden := range []string{
		"fixture-api-key", "fixture-cookie", "198.51.100.7", "2001:db8::7", "raw_payload", "evidence_digest", "score", "confidence",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("API response contains forbidden fixture %q: %s", forbidden, payload)
		}
	}
}

func TestIPRiskAPIErrorContractRedactsSensitiveFixtures(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node_0123456789abcdef", nil)
	rec := httptest.NewRecorder()
	transporthttp.WriteError(rec, req, http.StatusBadRequest, "provider_error", "provider failed for https://user:fixture-password@provider.invalid/check?api_key=fixture-api-key Cookie: session=fixture-cookie ip=198.51.100.7")
	payload := rec.Body.String()
	for _, forbidden := range []string{"fixture-password", "fixture-api-key", "fixture-cookie", "198.51.100.7"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("API error contains forbidden fixture %q: %s", forbidden, payload)
		}
	}
}

func TestIPRiskAPIJSONRejectsSensitiveSummaryPayloads(t *testing.T) {
	policyID := domain.MustNewUUIDv7()
	baseJSON := func(overrides map[string]string) []byte {
		m := map[string]any{
			"decision":                "review",
			"risk_band":               "unknown",
			"provider":                "fixture",
			"provider_schema_version": "v1",
			"observed_at":             "2026-09-16T00:00:00Z",
			"expires_at":              "2026-09-16T01:00:00Z",
			"status":                  "unknown",
			"reason_code":             "observation_missing",
			"policy_revision_id":      policyID,
		}
		for k, v := range overrides {
			m[k] = v
		}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal test payload: %v", err)
		}
		return raw
	}

	for name, overrides := range map[string]map[string]string{
		"provider with API key":            {"provider": "api_key=secret"},
		"provider with URL query":          {"provider": "https://provider.invalid/check?api_key=secret"},
		"provider with IPv4":               {"provider": "198.51.100.7"},
		"provider with IPv6":               {"provider": "2001:db8::7"},
		"provider with sensitive token":    {"provider": "provider_api_key_test"},
		"schema version with API key":      {"provider_schema_version": "api_key=secret"},
		"schema version with IP":           {"provider_schema_version": "198.51.100.7"},
		"schema version with URL query":    {"provider_schema_version": "?token=secret"},
		"reason code with sensitive token": {"reason_code": "provider_api_key_fixture"},
		"reason code with cookie token":    {"reason_code": "provider_cookie_fixture"},
		"reason code with IP":              {"reason_code": "provider_ip_198.51.100.7"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := baseJSON(overrides)
			var summary domain.IPRiskSummary
			if err := json.Unmarshal(raw, &summary); err != nil {
				t.Fatalf("unmarshal API JSON payload: %v", err)
			}
			if err := summary.Validate(); err == nil {
				t.Fatalf("expected summary validation error for API JSON with %s, but passed", name)
			}
		})
	}
}
