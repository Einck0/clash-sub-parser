package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

type subscriptionResponse struct {
	ID                 string                    `json:"id"`
	Name               string                    `json:"name"`
	SourceURLSecretRef string                    `json:"source_url_secret_ref"`
	Enabled            bool                      `json:"enabled"`
	Config             domain.SubscriptionConfig `json:"config"`
	Revision           string                    `json:"revision"`
}

func subscriptionTestRouter(t *testing.T) http.Handler {
	t.Helper()
	cfg := sqlite.Config{
		Path:            "file:subscription_http_test?mode=memory&cache=shared",
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	routerCfg := newTestRouterConfig(true)
	routerCfg.SubscriptionService = subscription.NewService(sqlite.NewSubscriptionRepository(db), sqlite.NewAuditRepository(db))
	return transporthttp.NewRouter(routerCfg)
}

func TestSubscriptionEndpointsEnforceSecurityAndRedactSecrets(t *testing.T) {
	router := subscriptionTestRouter(t)
	secretRef := "secret://subscriptions/primary?token=leak-me"
	body := `{"name":"Primary","source_url_secret_ref":"` + secretRef + `","enabled":true,"refresh_policy":{"interval_seconds":3600}}`

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized POST status = %d, want 401", unauthorized.Code)
	}

	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body))
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: testValidSessionID})
	missingCSRFRecorder := httptest.NewRecorder()
	router.ServeHTTP(missingCSRFRecorder, missingCSRF)
	if missingCSRFRecorder.Code != http.StatusForbidden {
		t.Fatalf("cookie POST without CSRF status = %d, want 403", missingCSRFRecorder.Code)
	}

	create := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Authorization", "Bearer "+testAdminToken)
	created := httptest.NewRecorder()
	router.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "leak-me") {
		t.Fatalf("create response leaked secret: %s", created.Body.String())
	}
	var createResponse testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(created.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResponse.Data.SourceURLSecretRef != "***" || createResponse.Data.ID == "" || createResponse.Data.Revision == "" {
		t.Fatalf("unexpected redacted create response: %#v", createResponse.Data)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions?page=1&page_size=50&enabled_only=true&search_text=Primary", nil)
	list.Header.Set("Authorization", "Bearer "+testAdminToken)
	listed := httptest.NewRecorder()
	router.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), "leak-me") {
		t.Fatalf("list status/body = %d %s", listed.Code, listed.Body.String())
	}

	patch := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+createResponse.Data.ID, strings.NewReader(`{"name":"Renamed"}`))
	patch.Header.Set("Content-Type", "application/json")
	patch.Header.Set("Authorization", "Bearer "+testAdminToken)
	patch.Header.Set("If-Match", "wrong-revision")
	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, patch)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("stale patch status = %d, body = %s", conflict.Code, conflict.Body.String())
	}

	// Test update with "***" secret ref preserves original secret without leaking or clearing
	patchWithMask := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+createResponse.Data.ID, strings.NewReader(`{"source_url_secret_ref":"***","config":{"cron_schedule":"0 0 * * *","auto_test":true}}`))
	patchWithMask.Header.Set("Content-Type", "application/json")
	patchWithMask.Header.Set("Authorization", "Bearer "+testAdminToken)
	patchWithMask.Header.Set("If-Match", createResponse.Data.Revision)
	patchRecorder := httptest.NewRecorder()
	router.ServeHTTP(patchRecorder, patchWithMask)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch with mask status = %d, body = %s", patchRecorder.Code, patchRecorder.Body.String())
	}
	var patchResponse testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(patchRecorder.Body.Bytes(), &patchResponse); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if patchResponse.Data.SourceURLSecretRef != "***" {
		t.Fatalf("expected redacted '***', got %s", patchResponse.Data.SourceURLSecretRef)
	}
	if patchResponse.Data.Config.CronSchedule != "0 0 * * *" || !patchResponse.Data.Config.AutoTest {
		t.Fatalf("expected updated config, got %#v", patchResponse.Data.Config)
	}

	missingRefreshKey := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/"+createResponse.Data.ID+"/refresh", nil)
	missingRefreshKey.Header.Set("Authorization", "Bearer "+testAdminToken)
	missingRefreshKeyRecorder := httptest.NewRecorder()
	router.ServeHTTP(missingRefreshKeyRecorder, missingRefreshKey)
	if missingRefreshKeyRecorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("refresh without idempotency key status = %d, want 422", missingRefreshKeyRecorder.Code)
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/subscriptions/"+createResponse.Data.ID, nil)
	deleteRequest.Header.Set("Authorization", "Bearer "+testAdminToken)
	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, deleteRequest)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
}

func TestSubscriptionConcurrentPatchRejection(t *testing.T) {
	router := subscriptionTestRouter(t)
	secretRef := "secret://subscriptions/concurrent?token=safe"
	body := `{"name":"ConcurrentBase","source_url_secret_ref":"` + secretRef + `","enabled":true,"refresh_policy":{"interval_seconds":3600}}`

	create := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(body))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Authorization", "Bearer "+testAdminToken)
	created := httptest.NewRecorder()
	router.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	var createResponse testDataResponse[subscriptionResponse]
	if err := json.Unmarshal(created.Body.Bytes(), &createResponse); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	subID := createResponse.Data.ID
	baseRevision := createResponse.Data.Revision

	const n = 2
	start := make(chan struct{})
	type patchResult struct {
		code int
		body string
	}
	results := make(chan patchResult, n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			<-start
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/"+subID, strings.NewReader(`{"name":"PatchWorker"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testAdminToken)
			req.Header.Set("If-Match", baseRevision)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			results <- patchResult{code: rec.Code, body: rec.Body.String()}
		}(i)
	}

	close(start)

	var okCount, conflictCount int
	for i := 0; i < n; i++ {
		res := <-results
		switch res.code {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Errorf("unexpected status code %d: %s", res.code, res.body)
		}
	}

	if okCount != 1 || conflictCount != 1 {
		t.Fatalf("expected 1 OK (200) and 1 Conflict (409), got %d OK and %d Conflict", okCount, conflictCount)
	}
}
