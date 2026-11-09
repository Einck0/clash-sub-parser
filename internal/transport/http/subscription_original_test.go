package http_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

// setupOriginalConfigTestDB creates an isolated SQLite database, runs migrations via OpenAndMigrate,
// and builds services.
func setupOriginalConfigTestDB(t *testing.T) (*sql.DB, *subscription.Service, *inventory.Service) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "orig_config_test.db")
	cfg := sqlite.DefaultConfig(dbPath)
	cfg.ForeignKeys = true

	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	subRepo := sqlite.NewSubscriptionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeSourceRepo := sqlite.NewNodeSourceRepository(db)

	subService := subscription.NewService(subRepo, auditRepo)
	invService := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, nil)

	return db, subService, invService
}

func setupOriginalConfigRouter(t *testing.T, subService *subscription.Service, invService *inventory.Service, openAuth bool) http.Handler {
	t.Helper()

	routerCfg := newTestRouterConfig(true)
	// Management middleware recognizes export credentials independently of the
	// publication-specific validator, matching the production router wiring.
	routerCfg.IsPublicationToken = func(_ context.Context, token string) bool {
		return token == testPublicationToken
	}
	if openAuth {
		routerCfg.AdminToken = ""
		routerCfg.TokenHolder = nil
	}
	routerCfg.SubscriptionService = subService
	routerCfg.InventoryService = invService

	return transporthttp.NewRouter(routerCfg)
}

func seedTestSubscription(t *testing.T, subService *subscription.Service, name, url string) string {
	t.Helper()
	created, err := subService.Create(context.Background(), subscription.CreateSubscriptionCommand{
		Name:               name,
		SourceURLSecretRef: url,
		Enabled:            true,
		RefreshPolicy:      domain.RefreshPolicy{IntervalSeconds: 3600},
	})
	if err != nil {
		t.Fatalf("seedTestSubscription failed: %v", err)
	}
	return created.ID
}

func seedTestOriginalConfig(t *testing.T, db *sql.DB, subID string, body []byte, fetchedAt time.Time) {
	t.Helper()
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	fetchedAtStr := fetchedAt.UTC().Format(time.RFC3339Nano)
	_, err := db.Exec(`
		INSERT INTO subscription_original_configs (
			subscription_id, source_revision, content_digest, body_blob, fetched_at, content_type, fetch_id
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, subID, "rev-test-01", digest, body, fetchedAtStr, "application/octet-stream", "fetch-test-01")
	if err != nil {
		t.Fatalf("seedTestOriginalConfig failed: %v", err)
	}
}

// TestOriginalConfigEndpoint_AuthEnforcement verifies the management-only authentication
// policy in both protected mode and open mode.
func TestOriginalConfigEndpoint_AuthEnforcement(t *testing.T) {
	db, subService, invService := setupOriginalConfigTestDB(t)
	subID := seedTestSubscription(t, subService, "Auth Sub", "https://example.com/sub")
	seedTestOriginalConfig(t, db, subID, []byte("proxies:\n  - name: test-p\n"), time.Now())

	path := fmt.Sprintf("/api/v1/subscriptions/%s/original-config", subID)

	t.Run("protected_mode_unauthenticated_rejected_401", func(t *testing.T) {
		router := setupOriginalConfigRouter(t, subService, invService, false)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d, body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("protected_mode_invalid_bearer_token_rejected_401", func(t *testing.T) {
		router := setupOriginalConfigRouter(t, subService, invService, false)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer invalid-bearer-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for invalid token, got %d", rec.Code)
		}
	})

	for _, mode := range []struct {
		name string
		open bool
	}{
		{name: "protected", open: false},
		{name: "open", open: true},
	} {
		t.Run(mode.name+"_mode_publication_export_token_rejected_403", func(t *testing.T) {
			router := setupOriginalConfigRouter(t, subService, invService, mode.open)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer "+testPublicationToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 Forbidden for publication export token, got %d", rec.Code)
			}
			var errResp testErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode json error response: %v", err)
			}
			if errResp.Code != "invalid_token_scope" {
				t.Fatalf("expected code invalid_token_scope, got %q", errResp.Code)
			}
			if bytes.Contains(rec.Body.Bytes(), []byte("proxies:")) {
				t.Fatal("forbidden response leaked original configuration")
			}
		})
	}

	t.Run("protected_mode_valid_admin_bearer_allowed_200", func(t *testing.T) {
		router := setupOriginalConfigRouter(t, subService, invService, false)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK with admin token, got %d, body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("protected_mode_valid_session_cookie_allowed_200", func(t *testing.T) {
		router := setupOriginalConfigRouter(t, subService, invService, false)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: testValidSessionID})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK with valid session cookie, got %d", rec.Code)
		}
	})

	t.Run("open_mode_unauthenticated_allowed_200", func(t *testing.T) {
		router := setupOriginalConfigRouter(t, subService, invService, true)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK in open mode, got %d, body: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestOriginalConfigEndpoint_ExactBytesAndHeaders verifies exact binary payload
// preservation and strict attachment response headers.
func TestOriginalConfigEndpoint_ExactBytesAndHeaders(t *testing.T) {
	db, subService, invService := setupOriginalConfigTestDB(t)
	router := setupOriginalConfigRouter(t, subService, invService, false)

	// Construct test body with binary zero, high bytes, UTF-8 text, and YAML formatting
	expectedBody := []byte("mixed-config-test:\n  key: value\n\x00\x01\x02\xfe\xff\n  unicode: 你好世界\n")
	subID := seedTestSubscription(t, subService, "Binary Sub", "https://example.com/binary")
	seedTestOriginalConfig(t, db, subID, expectedBody, time.Now())

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/subscriptions/%s/original-config", subID), nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", rec.Code, rec.Body.String())
	}

	// 1. Verify exact body bytes
	if !bytes.Equal(rec.Body.Bytes(), expectedBody) {
		t.Fatalf("response body does not match expected bytes:\ngot:  %q\nwant: %q", rec.Body.Bytes(), expectedBody)
	}

	// 2. Verify strict headers
	assertHeader := func(header, expected string) {
		t.Helper()
		got := rec.Header().Get(header)
		if got != expected {
			t.Errorf("header %s: got %q, want %q", header, got, expected)
		}
	}

	assertHeader("Content-Type", "application/octet-stream")
	assertHeader("Content-Disposition", `attachment; filename="subscription-original.conf"`)
	assertHeader("Cache-Control", "no-store")
	assertHeader("X-Content-Type-Options", "nosniff")
	assertHeader("Referrer-Policy", "no-referrer")
	assertHeader("Content-Length", strconv.Itoa(len(expectedBody)))
}

// TestOriginalConfigEndpoint_NoRefetchAndNoMutation asserts that reading the original
// configuration performs NO network fetch, inserts NO fetch audit records, and mutates NO rows.
func TestOriginalConfigEndpoint_NoRefetchAndNoMutation(t *testing.T) {
	db, subService, invService := setupOriginalConfigTestDB(t)
	router := setupOriginalConfigRouter(t, subService, invService, false)

	subID := seedTestSubscription(t, subService, "No-Refetch Sub", "https://example.com/norefetch")
	fixedTime := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	configBytes := []byte("proxies:\n  - name: node-1\n    type: ss\n")
	seedTestOriginalConfig(t, db, subID, configBytes, fixedTime)

	countFetches := func() int {
		var cnt int
		if err := db.QueryRow("SELECT COUNT(*) FROM subscription_fetches").Scan(&cnt); err != nil {
			t.Fatalf("failed to query subscription_fetches: %v", err)
		}
		return cnt
	}

	getStoredFetchedAt := func() string {
		var str string
		if err := db.QueryRow("SELECT fetched_at FROM subscription_original_configs WHERE subscription_id = ?", subID).Scan(&str); err != nil {
			t.Fatalf("failed to query fetched_at: %v", err)
		}
		return str
	}

	getSubscriptionUpdatedAt := func() string {
		var str string
		if err := db.QueryRow("SELECT updated_at FROM subscriptions WHERE id = ?", subID).Scan(&str); err != nil {
			t.Fatalf("failed to query updated_at: %v", err)
		}
		return str
	}

	initialFetches := countFetches()
	initialFetchedAt := getStoredFetchedAt()
	initialUpdatedAt := getSubscriptionUpdatedAt()

	// Call the endpoint multiple times
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/subscriptions/%s/original-config", subID), nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200 OK, got %d", i+1, rec.Code)
		}
		if !bytes.Equal(rec.Body.Bytes(), configBytes) {
			t.Fatalf("call %d: bytes mismatch", i+1)
		}
	}

	// Verify no fetches were performed and no state was mutated
	if finalFetches := countFetches(); finalFetches != initialFetches {
		t.Fatalf("subscription_fetches mutated: had %d, now %d (refetch occurred!)", initialFetches, finalFetches)
	}
	if finalFetchedAt := getStoredFetchedAt(); finalFetchedAt != initialFetchedAt {
		t.Fatalf("original config fetched_at changed: had %s, now %s", initialFetchedAt, finalFetchedAt)
	}
	if finalUpdatedAt := getSubscriptionUpdatedAt(); finalUpdatedAt != initialUpdatedAt {
		t.Fatalf("subscription updated_at changed: had %s, now %s", initialUpdatedAt, finalUpdatedAt)
	}
}

// TestOriginalConfigEndpoint_MissingSubscription verifies safe 404 response
// when requesting a non-existent subscription ID.
func TestOriginalConfigEndpoint_MissingSubscription(t *testing.T) {
	_, subService, invService := setupOriginalConfigTestDB(t)
	router := setupOriginalConfigRouter(t, subService, invService, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-missing-id-9999/original-config", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var errResp testErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON error response: %v", err)
	}
	if errResp.Code != "subscription_not_found" {
		t.Fatalf("expected code subscription_not_found, got %q", errResp.Code)
	}
	if strings.Contains(errResp.Message, "sub-missing-id-9999") {
		t.Fatalf("subscription ID must not be echoed in error message: %q", errResp.Message)
	}
}

// TestOriginalConfigEndpoint_MissingOriginalConfig verifies safe 404 response
// when the subscription exists but no raw original configuration is stored.
func TestOriginalConfigEndpoint_MissingOriginalConfig(t *testing.T) {
	_, subService, invService := setupOriginalConfigTestDB(t)
	router := setupOriginalConfigRouter(t, subService, invService, false)

	// Create subscription, but intentionally do NOT seed subscription_original_configs
	subID := seedTestSubscription(t, subService, "Empty Config Sub", "https://example.com/empty")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/subscriptions/%s/original-config", subID), nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var errResp testErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON error response: %v", err)
	}
	if errResp.Code != "original_config_not_found" {
		t.Fatalf("expected code original_config_not_found, got %q", errResp.Code)
	}
	if strings.Contains(errResp.Message, subID) {
		t.Fatalf("subscription ID must not be echoed in error message: %q", errResp.Message)
	}
}

// TestOriginalConfigEndpoint_CrossSubscriptionIsolation verifies that
// stored configs are strictly partitioned by subscription ID.
func TestOriginalConfigEndpoint_CrossSubscriptionIsolation(t *testing.T) {
	db, subService, invService := setupOriginalConfigTestDB(t)
	router := setupOriginalConfigRouter(t, subService, invService, false)

	subID1 := seedTestSubscription(t, subService, "Sub Alpha", "https://example.com/alpha")
	subID2 := seedTestSubscription(t, subService, "Sub Beta", "https://example.com/beta")

	bodyAlpha := []byte("--- ALPHA SECRET CONFIG ---")
	bodyBeta := []byte("--- BETA SECRET CONFIG ---")

	seedTestOriginalConfig(t, db, subID1, bodyAlpha, time.Now())
	seedTestOriginalConfig(t, db, subID2, bodyBeta, time.Now())

	// Query Sub 1 -> must return only Alpha bytes
	req1 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/subscriptions/%s/original-config", subID1), nil)
	req1.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("sub1 status: expected 200, got %d", rec1.Code)
	}
	if !bytes.Equal(rec1.Body.Bytes(), bodyAlpha) {
		t.Fatalf("sub1 body mismatch: got %q, want %q", rec1.Body.Bytes(), bodyAlpha)
	}
	if strings.Contains(rec1.Body.String(), string(bodyBeta)) {
		t.Fatalf("CRITICAL: sub1 response leaks sub2 content!")
	}

	// Query Sub 2 -> must return only Beta bytes
	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/subscriptions/%s/original-config", subID2), nil)
	req2.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("sub2 status: expected 200, got %d", rec2.Code)
	}
	if !bytes.Equal(rec2.Body.Bytes(), bodyBeta) {
		t.Fatalf("sub2 body mismatch: got %q, want %q", rec2.Body.Bytes(), bodyBeta)
	}
	if strings.Contains(rec2.Body.String(), string(bodyAlpha)) {
		t.Fatalf("CRITICAL: sub2 response leaks sub1 content!")
	}
}

// TestOriginalConfigEndpoint_NoLeakThroughOrdinaryDTOs verifies that
// ordinary subscription endpoints (list, get, entries) do not expose raw original config.
func TestOriginalConfigEndpoint_NoLeakThroughOrdinaryDTOs(t *testing.T) {
	db, subService, invService := setupOriginalConfigTestDB(t)
	router := setupOriginalConfigRouter(t, subService, invService, false)

	sensitiveOriginalBody := []byte("SECRET_RAW_PAYLOAD_TOKEN_MUST_NOT_APPEAR_IN_ORDINARY_DTOS")
	subID := seedTestSubscription(t, subService, "Leak Check Sub", "https://example.com/leakcheck")
	seedTestOriginalConfig(t, db, subID, sensitiveOriginalBody, time.Now())

	marker := "SECRET_RAW_PAYLOAD_TOKEN_MUST_NOT_APPEAR_IN_ORDINARY_DTOS"

	// 1. GET /api/v1/subscriptions/{id}
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/subscriptions/%s", subID), nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("get sub status: expected 200, got %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), marker) {
			t.Fatalf("CRITICAL: GET /subscriptions/{id} leaked raw config: %s", rec.Body.String())
		}
	}

	// 2. GET /api/v1/subscriptions
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions?page=1&page_size=50", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("list sub status: expected 200, got %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), marker) {
			t.Fatalf("CRITICAL: GET /subscriptions leaked raw config: %s", rec.Body.String())
		}
	}

	// 3. GET /api/v1/subscriptions/{id}/entries
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/subscriptions/%s/entries", subID), nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("list entries status: expected 200, got %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), marker) {
			t.Fatalf("CRITICAL: GET /subscriptions/{id}/entries leaked raw config: %s", rec.Body.String())
		}
	}
}

// TestOriginalConfigEndpoint_InventoryUnavailable verifies that
// if the inventory service is unconfigured, the endpoint returns 500 inventory_unavailable.
func TestOriginalConfigEndpoint_InventoryUnavailable(t *testing.T) {
	_, subService, _ := setupOriginalConfigTestDB(t)
	// Create router with nil inventory service
	router := setupOriginalConfigRouter(t, subService, nil, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-123/original-config", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", rec.Code)
	}

	var errResp testErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode JSON error response: %v", err)
	}
	if errResp.Code != "inventory_unavailable" {
		t.Fatalf("expected code inventory_unavailable, got %q", errResp.Code)
	}
}
