package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// Both JSON and raw previews must reject an invalid group without exposing
// credentials embedded in its display name through capability diagnostics.
func TestPublicationPreviewCapabilityDiagnosticsAreRedacted(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	_, nodeID := seedSamplePolicyData(t, db)

	var parentID string
	if err := db.QueryRow("SELECT id FROM node_groups WHERE name = 'ProxyGroup'").Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	const privateName = "Blocked https://fixture-user:fixture-password@proxy.example.com/feed?token=fixture-query-token Bearer fixture-bearer-token"
	groupID := domain.MustNewUUIDv7()
	policyRepo := sqlite.NewPolicyRepository(db)
	if err := policyRepo.CreateGroup(ctx, &domain.NodeGroup{
		ID: groupID, Name: privateName, GroupType: domain.GroupTypeSelect,
		CreatedAt: domain.NowUTC(), UpdatedAt: domain.NowUTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := policyRepo.SetEdgesForGroup(ctx, parentID, []domain.GroupEdge{
		{ID: domain.MustNewUUIDv7(), ParentGroupID: parentID, NodeLogicalID: &nodeID, Position: 0},
		{ID: domain.MustNewUUIDv7(), ParentGroupID: parentID, ChildGroupID: &groupID, Position: 1},
	}); err != nil {
		t.Fatal(err)
	}
	// Keep the populated parent and an intentionally empty child. Global PASS
	// would otherwise make that child valid and bypass the capability error.
	emptyPass := false
	filterRepo := sqlite.NewNodeFilterRepository(db)
	if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{EmptyFallbackPass: &emptyPass}); err != nil {
		t.Fatal(err)
	}
	router, _ := setupPublicationTestRouter(t, db, publication.WithNodeFilterRepository(filterRepo))

	for _, endpoint := range []string{"/api/v1/publications/preview", "/api/v1/publications/preview?format=raw"} {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"target":"mihomo"}`))
			req.Header.Set("Authorization", "Bearer "+testAdminToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-ID", "capability-redaction-request")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected capability rejection 422, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || rec.Header().Get("Content-Disposition") != "" {
				t.Fatalf("rejected preview must return JSON, never a configuration attachment: %v", rec.Header())
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("capability error is missing no-store")
			}
			var response struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				RequestID string `json:"request_id"`
				Details   struct {
					SnapshotID  string                          `json:"snapshot_id"`
					Diagnostics []compiler.CapabilityDiagnostic `json:"diagnostics"`
				} `json:"details"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != "required_nonempty" || response.RequestID != "capability-redaction-request" || response.Details.SnapshotID == "" {
				t.Fatalf("lost error identity or failed-draft reference: %+v", response)
			}
			for _, secret := range []string{"fixture-user", "fixture-password", "fixture-query-token", "fixture-bearer-token"} {
				if strings.Contains(rec.Body.String(), secret) {
					t.Fatalf("capability response leaks synthetic credential %q", secret)
				}
			}
			if !strings.Contains(response.Message, "group nonempty validation failed") || !strings.Contains(response.Message, "token=***") {
				t.Fatalf("redaction lost actionable summary: %q", response.Message)
			}
			found := false
			for _, diagnostic := range response.Details.Diagnostics {
				if diagnostic.Code == "required_nonempty" && diagnostic.Target == "mihomo" && strings.Contains(diagnostic.Message, "has no proxies") && strings.Contains(diagnostic.Message, "Bearer ***") {
					found = true
				}
			}
			if !found {
				t.Fatalf("redaction lost the actionable diagnostic: %+v", response.Details.Diagnostics)
			}

			var state string
			var contentSize int
			if err := db.QueryRow("SELECT state, length(content) FROM publications WHERE id = ?", response.Details.SnapshotID).Scan(&state, &contentSize); err != nil {
				t.Fatal(err)
			}
			if state != "draft" || contentSize != 0 {
				t.Fatalf("rejection produced a usable publication: state=%s bytes=%d", state, contentSize)
			}
		})
	}
	var storedName string
	if err := db.QueryRow("SELECT name FROM node_groups WHERE id = ?", groupID).Scan(&storedName); err != nil {
		t.Fatal(err)
	}
	if storedName != privateName {
		t.Fatal("response redaction mutated the saved policy group")
	}
}
