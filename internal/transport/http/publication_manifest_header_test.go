package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestPublicManifestHeaderOmitsManagementDisplayNames(t *testing.T) {
	manifest := &domain.PublicationManifest{
		Target: domain.TargetMihomo, NodeCount: 1, ExcludedCount: 1,
		Excluded: []domain.ManifestExcludedNode{{
			NodeID: "node_0123456789abcdef", DisplayName: "management-only-name",
			Code: "unsupported_target_capability", Reason: "unsupported transport",
		}},
	}
	rec := httptest.NewRecorder()
	setSafeManifestHeaders(rec, manifest)
	header := rec.Header().Get("X-Export-Manifest")
	if header == "" || len(header) > 4096 {
		t.Fatalf("missing or unbounded public manifest header: %d bytes", len(header))
	}
	payload, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Target        domain.CompilerTarget         `json:"target"`
		IncludedCount int                           `json:"included_count"`
		ExcludedCount int                           `json:"excluded_count"`
		Excluded      []domain.ManifestExcludedNode `json:"excluded"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Target != domain.TargetMihomo || decoded.IncludedCount != 1 || decoded.ExcludedCount != 1 || len(decoded.Excluded) != 1 || decoded.Excluded[0].NodeID != manifest.Excluded[0].NodeID || decoded.Excluded[0].Code != manifest.Excluded[0].Code {
		t.Fatalf("public identity or counts changed: %s", payload)
	}
	if strings.Contains(string(payload), "display_name") || strings.Contains(string(payload), "management-only-name") {
		t.Fatalf("management-only metadata reached public header: %s", payload)
	}
	if manifest.Excluded[0].DisplayName != "management-only-name" {
		t.Fatal("public header filtering mutated the frozen management manifest")
	}
}
