package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// Embed the repository port to inject only the failing read under test.
type failingPublicationFilterRepository struct {
	domain.NodeFilterRepository
	secret string
}

func (r failingPublicationFilterRepository) GetGlobalFilter(context.Context) (*domain.GlobalNodeFilter, error) {
	return nil, errors.New(r.secret)
}

func publicationFilterRequest(t *testing.T, router http.Handler, endpoint, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(fmt.Sprintf(`{"target":%q}`, target)))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPublicationFilterDiagnosticsOverHTTP(t *testing.T) {
	for _, target := range []string{"mihomo", "singbox", "surge", "qx"} {
		for _, withRoute := range []bool{false, true} {
			name := target + "/without_route"
			if withRoute {
				name = target + "/with_route"
			}
			t.Run(name, func(t *testing.T) {
				db := newCleanSQLiteDB(t)
				seedSamplePolicyData(t, db)
				if !withRoute {
					if _, err := db.Exec("DELETE FROM policy_rules"); err != nil {
						t.Fatal(err)
					}
				}
				filterRepo := sqlite.NewNodeFilterRepository(db)
				router, _ := setupPublicationTestRouter(t, db, publication.WithNodeFilterRepository(filterRepo))
				ctx := context.Background()

				// A configured condition eliminates the seeded node; both raw and JSON previews
				// must report the same admission rejection as preflight and publish.
				filter := &domain.GlobalNodeFilter{Spec: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{
					Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "never-matches-tokyo",
				}}}}
				if err := filterRepo.SetGlobalFilter(ctx, filter); err != nil {
					t.Fatal(err)
				}
				for _, endpoint := range []string{"/api/v1/publications/preview", "/api/v1/publications/preview?format=raw", "/api/v1/publications"} {
					rec := publicationFilterRequest(t, router, endpoint, target)
					if rec.Code != http.StatusConflict {
						t.Fatalf("%s: expected 409, got %d: %s", endpoint, rec.Code, rec.Body.String())
					}
					var body struct {
						Code        string `json:"code"`
						Diagnostics []struct {
							Code    string `json:"code"`
							Message string `json:"message"`
						} `json:"diagnostics"`
					}
					if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					found := false
					for _, diagnostic := range body.Diagnostics {
						if diagnostic.Code == "filtered_nodes_empty" && diagnostic.Message != "" {
							found = true
						}
					}
					if body.Code != "publication_preflight_rejected" || !found {
						t.Fatalf("%s: missing filter diagnostic: %+v", endpoint, body)
					}
					if rec.Header().Get("Cache-Control") != "no-store" {
						t.Fatalf("%s: missing actionable message or no-store", endpoint)
					}
				}
				pre := publicationFilterRequest(t, router, "/api/v1/publications/preflight", target)
				var result struct {
					Data struct {
						Allowed     bool `json:"allowed"`
						Diagnostics []struct {
							Code string `json:"code"`
						} `json:"diagnostics"`
					} `json:"data"`
				}
				if err := json.Unmarshal(pre.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, diagnostic := range result.Data.Diagnostics {
					if diagnostic.Code == "filtered_nodes_empty" {
						found = true
					}
				}
				if pre.Code != http.StatusOK || result.Data.Allowed || !found {
					t.Fatalf("preflight expected filtered_nodes_empty: %d %+v", pre.Code, result)
				}
				var count int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications").Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("rejected publish created %d records", count)
				}

				// Empty conditions pass through the original node for every target.
				if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{Spec: domain.NodeFilterSpec{}}); err != nil {
					t.Fatal(err)
				}
				for _, endpoint := range []string{"/api/v1/publications/preview", "/api/v1/publications/preview?format=raw", "/api/v1/publications/preflight", "/api/v1/publications"} {
					rec := publicationFilterRequest(t, router, endpoint, target)
					want := http.StatusOK
					if endpoint == "/api/v1/publications" {
						want = http.StatusCreated
					}
					if rec.Code != want {
						t.Fatalf("empty filter %s: expected %d, got %d: %s", endpoint, want, rec.Code, rec.Body.String())
					}
					if strings.Contains(endpoint, "/preview") && !strings.Contains(rec.Body.String(), "Tokyo-01") {
						t.Fatalf("empty filter did not pass through node on %s", endpoint)
					}
				}
			})
		}
	}
}

func TestPublicationFilterReadFailureHTTPIsClosedAndRedacted(t *testing.T) {
	const secret = "fixture-db-password-never-return"
	for _, target := range []string{"mihomo", "singbox", "surge", "qx"} {
		t.Run(target, func(t *testing.T) {
			db := newCleanSQLiteDB(t)
			seedSamplePolicyData(t, db)
			router, _ := setupPublicationTestRouter(t, db, publication.WithNodeFilterRepository(failingPublicationFilterRepository{secret: secret}))
			for _, endpoint := range []string{"/api/v1/publications/preview", "/api/v1/publications/preview?format=raw", "/api/v1/publications/preflight", "/api/v1/publications"} {
				rec := publicationFilterRequest(t, router, endpoint, target)
				var body struct {
					Code        string            `json:"code"`
					Diagnostics []json.RawMessage `json:"diagnostics"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if rec.Code != http.StatusInternalServerError || body.Code != "global_filter_unavailable" || len(body.Diagnostics) != 0 || strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "Tokyo-01") {
					t.Fatalf("%s: unsafe failure: status=%d body=%s", endpoint, rec.Code, rec.Body.String())
				}
			}
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM publications").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("failed read created %d records", count)
			}
		})
	}
}
