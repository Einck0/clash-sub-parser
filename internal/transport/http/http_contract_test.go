package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

type testErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type testDataResponse[T any] struct {
	Data T `json:"data"`
}

type testPaginationData[T any] struct {
	Items    []T `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

const (
	testAdminToken       = "admin-bearer-token-secret-999"
	testPublicationToken = "pub-token-export-111"
	testValidSessionID   = "valid-session-session-xyz"
	testValidCSRFToken   = "csrf-secret-token-abc"
)

func newTestRouterConfig(ready bool) transporthttp.RouterConfig {
	return transporthttp.RouterConfig{
		AdminToken: testAdminToken,
		ReadinessChecker: func(ctx context.Context) (*sqlite.ReadinessReport, error) {
			if !ready {
				return &sqlite.ReadinessReport{
					Ready:         false,
					SchemaVersion: 0,
					Error:         "database unmigrated: missing tables",
					MissingTables: []string{"subscriptions", "nodes"},
				}, errors.New("readiness check failed: database not ready")
			}
			return &sqlite.ReadinessReport{
				Ready:          true,
				SchemaVersion:  1,
				RequiredTables: sqlite.RequiredTables(),
			}, nil
		},
		PublicationTokenValidator: func(ctx context.Context, publicationID, token string) (bool, error) {
			if token == testPublicationToken && publicationID == "pub-12345" {
				return true, nil
			}
			return false, nil
		},
		SessionValidator: func(sessionID string) (*transporthttp.SessionInfo, bool) {
			if sessionID == testValidSessionID {
				return &transporthttp.SessionInfo{
					SessionID: testValidSessionID,
					Subject:   "admin",
					CSRFToken: testValidCSRFToken,
				}, true
			}
			return nil, false
		},
	}
}

func setupTestRouter(t *testing.T, ready bool) http.Handler {
	t.Helper()
	return transporthttp.NewRouter(newTestRouterConfig(ready))
}

func TestHealthzEndpoint(t *testing.T) {
	router := setupTestRouter(t, true)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}
	var resp testDataResponse[map[string]any]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if resp.Data["status"] != "ok" {
		t.Errorf("expected data.status to be 'ok', got %v", resp.Data["status"])
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Errorf("expected X-Request-ID header to be present")
	}
}

func TestEmbeddedWebRoutes(t *testing.T) {
	router := setupTestRouter(t, true)
	for _, route := range []string{"/", "/nodes", "/settings/theme"} {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}
			if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("expected HTML response, got %q", rec.Header().Get("Content-Type"))
			}
			if !strings.Contains(rec.Body.String(), `<div id="app">`) {
				t.Fatal("expected embedded SPA index")
			}
		})
	}
}
