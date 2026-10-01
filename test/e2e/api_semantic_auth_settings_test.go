package e2e_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	transporthttp "clash-sub-parser/internal/transport/http"
)

// Task 5.2: TestAPISemantic_AuthAndSettings
// Verifies complete interface semantics, session lifecycle, cookie propagation,
// independent dual-auth toggle persistence in SQLite, admin token rotation, and auth boundaries.
func TestAPISemantic_AuthAndSettings(t *testing.T) {
	harness := setupTestHarness(t)
	ctx := context.Background()

	// =========================================================================
	// 1. Initial State: Open Mode (No auth required by default)
	// =========================================================================
	resp, err := harness.Request(http.MethodGet, "/api/v1/auth/status", nil, nil, nil)
	if err != nil {
		t.Fatalf("GET /api/v1/auth/status failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for auth/status, got %d", resp.StatusCode)
	}

	var statusData struct {
		Success bool `json:"success"`
		Data    struct {
			Mode          string `json:"mode"`
			AdminMode     string `json:"admin_mode"`
			ExportMode    string `json:"export_mode"`
			Authenticated bool   `json:"authenticated"`
			Subject       string `json:"subject"`
		} `json:"data"`
	}
	if err := resp.JSON(&statusData); err != nil {
		t.Fatalf("parse auth/status JSON: %v", err)
	}
	if !statusData.Data.Authenticated || statusData.Data.Mode != "open" {
		t.Fatalf("expected authenticated=true and mode=open initially, got %+v", statusData.Data)
	}

	// =========================================================================
	// 2. Settings: Independent Auth Switches & SQLite Persistence
	// =========================================================================
	// Read current auth settings
	resp, err = harness.Request(http.MethodGet, "/api/v1/settings/auth", nil, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/settings/auth failed: code=%d err=%v", resp.StatusCode, err)
	}

	// Enable Admin Authentication & Export Authentication independently
	putAuthBody := map[string]bool{
		"admin_auth_enabled":  true,
		"export_auth_enabled": true,
	}
	resp, err = harness.Request(http.MethodPut, "/api/v1/settings/auth", putAuthBody, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/settings/auth failed: code=%d err=%v", resp.StatusCode, err)
	}

	// Verify Database Side-Effect: settings table has persisted the switches!
	var adminAuthInt, exportAuthInt int
	err = harness.DB.QueryRowContext(ctx, "SELECT admin_auth_enabled, export_auth_enabled FROM settings WHERE id = 1").Scan(&adminAuthInt, &exportAuthInt)
	if err != nil || adminAuthInt != 1 || exportAuthInt != 1 {
		t.Fatalf("SQLite persistence failed! admin_auth=%d export_auth=%d err=%v", adminAuthInt, exportAuthInt, err)
	}

	// Now protected mode is active! Unauthenticated auth/status must show authenticated=false
	resp, err = harness.Request(http.MethodGet, "/api/v1/auth/status", nil, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/auth/status failed: %v", err)
	}
	_ = resp.JSON(&statusData)
	if statusData.Data.Authenticated || statusData.Data.Mode != "protected" {
		t.Fatalf("expected authenticated=false and mode=protected after switch, got %+v", statusData.Data)
	}

	// =========================================================================
	// 3. Auth Boundary: Unauthenticated access to admin routes is strictly 401
	// =========================================================================
	resp, err = harness.Request(http.MethodGet, "/api/v1/subscriptions", nil, nil, nil)
	if err != nil {
		t.Fatalf("GET /api/v1/subscriptions: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated access, got %d", resp.StatusCode)
	}

	// Invalid Bearer Token is strictly 401
	badAuthHeaders := map[string]string{"Authorization": "Bearer totally-invalid-token"}
	resp, err = harness.Request(http.MethodGet, "/api/v1/subscriptions", nil, badAuthHeaders, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad token, got %d", resp.StatusCode)
	}

	// =========================================================================
	// 4. Login Lifecycle: Input Validation, Cookie Generation, and Session
	// =========================================================================
	// Negative 1: Malformed JSON login payload -> 400 Bad Request
	resp, err = harness.Request(http.MethodPost, "/api/v1/auth/login", "{invalid-json", nil, nil)
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed login json, got %d", resp.StatusCode)
	}

	// Negative 2: Empty token -> 401 Unauthorized
	resp, err = harness.Request(http.MethodPost, "/api/v1/auth/login", map[string]string{"token": ""}, nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for empty login token, got %d", resp.StatusCode)
	}

	// Negative 3: Wrong password -> 401 Unauthorized
	resp, err = harness.Request(http.MethodPost, "/api/v1/auth/login", map[string]string{"token": "wrong-secret"}, nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong login credentials, got %d", resp.StatusCode)
	}

	// Positive: Correct token -> 200 + Set-Cookie
	loginBody := map[string]string{"token": harness.InitialAdminToken}
	resp, err = harness.Request(http.MethodPost, "/api/v1/auth/login", loginBody, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for valid login, got %d err=%v", resp.StatusCode, err)
	}

	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies {
		if c.Name == transporthttp.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatalf("expected %s cookie in login response, got cookies: %#v", transporthttp.SessionCookieName, resp.Cookies)
	}
	if !sessionCookie.HttpOnly {
		t.Fatalf("session cookie MUST be HttpOnly for security")
	}

	// Request with Session Cookie succeeds without Bearer header!
	resp, err = harness.Request(http.MethodGet, "/api/v1/auth/status", nil, nil, []*http.Cookie{sessionCookie})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/auth/status with cookie failed: %v", err)
	}
	_ = resp.JSON(&statusData)
	if !statusData.Data.Authenticated || statusData.Data.Subject != "admin" {
		t.Fatalf("expected authenticated=true with cookie, got %+v", statusData.Data)
	}

	// Admin API accessible via session cookie
	resp, err = harness.Request(http.MethodGet, "/api/v1/subscriptions", nil, nil, []*http.Cookie{sessionCookie})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK accessing admin API with session cookie, got %d", resp.StatusCode)
	}

	// =========================================================================
	// 5. Logout Lifecycle: Cookie Invalidation & Session Destruction
	// =========================================================================
	resp, err = harness.Request(http.MethodPost, "/api/v1/auth/logout", nil, nil, []*http.Cookie{sessionCookie})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/auth/logout failed: %v", err)
	}

	// Check cookie expired
	var clearedCookie *http.Cookie
	for _, c := range resp.Cookies {
		if c.Name == transporthttp.SessionCookieName {
			clearedCookie = c
			break
		}
	}
	if clearedCookie != nil && clearedCookie.MaxAge > 0 {
		t.Fatalf("expected session cookie to be expired (MaxAge<=0), got MaxAge=%d", clearedCookie.MaxAge)
	}

	// Subsequent request with old cookie must now be UNAUTHENTICATED
	resp, err = harness.Request(http.MethodGet, "/api/v1/auth/status", nil, nil, []*http.Cookie{sessionCookie})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/auth/status: %v", err)
	}
	_ = resp.JSON(&statusData)
	if statusData.Data.Authenticated {
		t.Fatalf("expected unauthenticated status after logout, got authenticated=true")
	}

	// =========================================================================
	// 6. Admin Token Rotation: Persistence, Old Token Invalidation, New Token Acceptance
	// =========================================================================
	// Check GET /api/v1/settings/admin-token: NEVER leaks token secret or hash
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/settings/admin-token", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/settings/admin-token failed: %v", err)
	}
	var tokenStatusData struct {
		Success bool `json:"success"`
		Data    struct {
			Mode       string `json:"mode"`
			Configured bool   `json:"configured"`
			Message    string `json:"message"`
		} `json:"data"`
	}
	_ = resp.JSON(&tokenStatusData)
	if !tokenStatusData.Data.Configured {
		t.Fatalf("expected admin token configured=true, got %+v", tokenStatusData.Data)
	}
	// Verify raw body string has zero occurrences of secret token
	if strings.Contains(string(resp.Body), harness.InitialAdminToken) {
		t.Fatalf("CRITICAL SECURITY LEAK: admin-token endpoint leaked plaintext token!")
	}

	// Rotate Admin Token to a new secret token
	newToken := "new-rotated-super-secret-admin-token-2026"
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/settings/admin-token", map[string]string{"token": newToken}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/settings/admin-token failed: code=%d err=%v", resp.StatusCode, err)
	}

	// Verify Database Side-Effect: settings table has updated hash
	var tokenHash string
	_ = harness.DB.QueryRowContext(ctx, "SELECT admin_token FROM settings WHERE id = 1").Scan(&tokenHash)
	if tokenHash == "" || strings.Contains(tokenHash, newToken) {
		t.Fatalf("token hash must be non-empty and hashed (never plaintext), got %q", tokenHash)
	}

	// Verify Old Token is now REJECTED (401 Unauthorized)
	oldAuthHeaders := map[string]string{"Authorization": "Bearer " + harness.InitialAdminToken}
	resp, err = harness.Request(http.MethodGet, "/api/v1/subscriptions", nil, oldAuthHeaders, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked/rotated old token, got %d", resp.StatusCode)
	}

	// Verify New Token is now ACCEPTED (200 OK)
	newAuthHeaders := map[string]string{"Authorization": "Bearer " + newToken}
	resp, err = harness.Request(http.MethodGet, "/api/v1/subscriptions", nil, newAuthHeaders, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for new rotated token, got %d", resp.StatusCode)
	}

	// =========================================================================
	// 7. Publication Scope Enforcement: Publication token CANNOT access Admin API
	// =========================================================================
	// Insert publication token into DB to simulate publication token
	pubID := domain.MustNewUUIDv7()
	pubToken := "pub-isolated-export-token-999"
	pubTokenHash, _ := transporthttp.HashToken(pubToken)

	_, err = harness.DB.ExecContext(ctx, `
		INSERT INTO publications (id, target, snapshot_digest, compiler_version, token_hash, state, created_at, content)
		VALUES (?, 'singbox', 'snap-fake', '1.0.0', ?, 'active', '2026-10-01T00:00:00Z', X'7b7d');
	`, pubID, pubTokenHash)
	if err != nil {
		t.Fatalf("insert test publication: %v", err)
	}

	// Attempt to access Admin API using publication token -> MUST be 401 or 403 Forbidden
	pubHeaders := map[string]string{"Authorization": "Bearer " + pubToken}
	resp, err = harness.Request(http.MethodGet, "/api/v1/subscriptions", nil, pubHeaders, nil)
	if err != nil || (resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden) {
		t.Fatalf("expected 401/403 when publication token attempts admin API access, got %d", resp.StatusCode)
	}

	// =========================================================================
	// 8. 404 Unified Error Envelope
	// =========================================================================
	resp, err = harness.Request(http.MethodGet, "/api/v1/non-existent-route-for-testing", nil, newAuthHeaders, nil)
	if err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown route, got %d", resp.StatusCode)
	}
	var errEnvelope transporthttp.ErrorResponse
	_ = resp.JSON(&errEnvelope)
	if errEnvelope.Code != "not_found" || errEnvelope.Message == "" {
		t.Fatalf("expected unified 404 error envelope with code='not_found', got %+v", errEnvelope)
	}
}
