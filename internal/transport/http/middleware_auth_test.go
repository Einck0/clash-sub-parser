package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	transporthttp "clash-sub-parser/internal/transport/http"
)

func TestAdminAuthMiddleware_OpenMode_Anonymous(t *testing.T) {
	testCases := []struct {
		name       string
		adminToken string
	}{
		{name: "empty_token", adminToken: ""},
		{name: "whitespace_token", adminToken: "   \t\n  "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := transporthttp.RouterConfig{
				AdminToken: tc.adminToken,
			}
			var capturedAuth *transporthttp.AuthContext
			handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuth = transporthttp.GetAuthContext(r.Context())
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data":"ok"}`))
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK in open mode, got %d. Body: %s", rec.Code, rec.Body.String())
			}

			if capturedAuth == nil {
				t.Fatal("expected AuthContext to be injected in open mode, got nil")
			}
			if capturedAuth.ActorKind != transporthttp.ActorKindAdmin {
				t.Errorf("expected ActorKindAdmin, got %v", capturedAuth.ActorKind)
			}
			if capturedAuth.Subject != "admin" {
				t.Errorf("expected Subject 'admin', got %q", capturedAuth.Subject)
			}
			if string(capturedAuth.AuthMethod) != "open_mode" {
				t.Errorf("expected AuthMethod 'open_mode', got %q", capturedAuth.AuthMethod)
			}
			if len(capturedAuth.Scopes) != 1 || capturedAuth.Scopes[0] != "admin" {
				t.Errorf("expected Scopes ['admin'], got %v", capturedAuth.Scopes)
			}
		})
	}
}

func TestAdminAuthMiddleware_OpenMode_WithGarbageBearerToken(t *testing.T) {
	cfg := transporthttp.RouterConfig{
		AdminToken: "", // Open Mode
	}
	var capturedAuth *transporthttp.AuthContext
	handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = transporthttp.GetAuthContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer invalid-garbage-stale-token-from-history")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK in open mode even with garbage bearer token, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	if capturedAuth == nil {
		t.Fatal("expected AuthContext to be injected in open mode, got nil")
	}
	if capturedAuth.ActorKind != transporthttp.ActorKindAdmin {
		t.Errorf("expected ActorKindAdmin, got %v", capturedAuth.ActorKind)
	}
	if capturedAuth.Subject != "admin" {
		t.Errorf("expected Subject 'admin', got %q", capturedAuth.Subject)
	}
	if capturedAuth.AuthMethod != transporthttp.AuthMethodOpenMode {
		t.Errorf("expected AuthMethodOpenMode, got %q", capturedAuth.AuthMethod)
	}
}

func TestAdminAuthMiddleware_OpenMode_WithInvalidSessionCookie(t *testing.T) {
	cfg := transporthttp.RouterConfig{
		AdminToken: "", // Open Mode
	}
	var capturedAuth *transporthttp.AuthContext
	handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = transporthttp.GetAuthContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: "stale-expired-invalid-cookie"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK in open mode even with invalid session cookie, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	if capturedAuth == nil {
		t.Fatal("expected AuthContext to be injected in open mode, got nil")
	}
	if capturedAuth.AuthMethod != transporthttp.AuthMethodOpenMode {
		t.Errorf("expected AuthMethodOpenMode, got %q", capturedAuth.AuthMethod)
	}
}

func TestAdminAuthMiddleware_TokenMode_AnonymousBlocked(t *testing.T) {
	cfg := transporthttp.RouterConfig{
		AdminToken: "test-admin-secret",
	}
	handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized in token mode for anonymous request, got %d", rec.Code)
	}

	var errResp testErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Code != "unauthorized" {
		t.Errorf("expected error code 'unauthorized', got %q", errResp.Code)
	}
	if !strings.Contains(errResp.Message, "Authentication credentials required") {
		t.Errorf("expected error message containing 'Authentication credentials required', got %q", errResp.Message)
	}
}

func TestAdminAuthMiddleware_TokenMode_ValidBearerToken(t *testing.T) {
	adminSecret := "valid-secret-token-12345"
	cfg := transporthttp.RouterConfig{
		AdminToken: adminSecret,
	}
	var capturedAuth *transporthttp.AuthContext
	handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = transporthttp.GetAuthContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+adminSecret)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid bearer token, got %d. Body: %s", rec.Code, rec.Body.String())
	}
	if capturedAuth == nil {
		t.Fatal("expected AuthContext, got nil")
	}
	if capturedAuth.ActorKind != transporthttp.ActorKindAdmin {
		t.Errorf("expected ActorKindAdmin, got %v", capturedAuth.ActorKind)
	}
	if capturedAuth.Subject != "admin" {
		t.Errorf("expected Subject 'admin', got %q", capturedAuth.Subject)
	}
	if capturedAuth.AuthMethod != transporthttp.AuthMethodBearer {
		t.Errorf("expected AuthMethodBearer, got %v", capturedAuth.AuthMethod)
	}
	if len(capturedAuth.Scopes) != 1 || capturedAuth.Scopes[0] != "admin" {
		t.Errorf("expected Scopes ['admin'], got %v", capturedAuth.Scopes)
	}
}

func TestAdminAuthMiddleware_TokenMode_InvalidBearerToken(t *testing.T) {
	cfg := transporthttp.RouterConfig{
		AdminToken: "correct-secret",
	}
	handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for invalid bearer token, got %d", rec.Code)
	}
	var errResp testErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Code != "unauthorized" {
		t.Errorf("expected error code 'unauthorized', got %q", errResp.Code)
	}
	if !strings.Contains(errResp.Message, "Invalid authentication credentials") {
		t.Errorf("expected error message containing 'Invalid authentication credentials', got %q", errResp.Message)
	}
}

func TestAdminAuthMiddleware_PublicationToken_Forbidden(t *testing.T) {
	pubToken := "pub-export-token-secret-999"
	isPubChecker := func(ctx context.Context, token string) bool {
		return token == pubToken
	}

	modes := []struct {
		name       string
		adminToken string
	}{
		{name: "open_mode", adminToken: ""},
		{name: "token_mode", adminToken: "admin-secret-key"},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			cfg := transporthttp.RouterConfig{
				AdminToken:         mode.adminToken,
				IsPublicationToken: isPubChecker,
			}
			handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions", nil)
			req.Header.Set("Authorization", "Bearer "+pubToken)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 Forbidden for publication token in %s, got %d. Body: %s", mode.name, rec.Code, rec.Body.String())
			}

			var errResp testErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if errResp.Code != "invalid_token_scope" {
				t.Errorf("expected error code 'invalid_token_scope', got %q", errResp.Code)
			}
			if !strings.Contains(errResp.Message, "Publication export token cannot access administration API") {
				t.Errorf("expected error message containing 'Publication export token cannot access administration API', got %q", errResp.Message)
			}
		})
	}
}

func TestAdminAuthMiddleware_TokenMode_SessionCookie(t *testing.T) {
	validSessionID := "valid-session-id"
	cfg := transporthttp.RouterConfig{
		AdminToken: "admin-secret",
		SessionValidator: func(sessionID string) (*transporthttp.SessionInfo, bool) {
			if sessionID == validSessionID {
				return &transporthttp.SessionInfo{
					SessionID: validSessionID,
					Subject:   "session-user",
					CSRFToken: "csrf-token-123",
				}, true
			}
			return nil, false
		},
	}

	t.Run("valid_session_cookie", func(t *testing.T) {
		var capturedAuth *transporthttp.AuthContext
		handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = transporthttp.GetAuthContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
		req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: validSessionID})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if capturedAuth == nil || capturedAuth.Subject != "session-user" {
			t.Errorf("expected Subject 'session-user', got %v", capturedAuth)
		}
		if capturedAuth.AuthMethod != transporthttp.AuthMethodCookie {
			t.Errorf("expected AuthMethodCookie, got %v", capturedAuth.AuthMethod)
		}
	})

	t.Run("invalid_session_cookie", func(t *testing.T) {
		handler := transporthttp.AdminAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
		req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: "invalid-id"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})
}

func TestAdminAuthMiddleware_RouterIntegration_OpenModeVsTokenMode(t *testing.T) {
	t.Run("open_mode_allows_admin_route", func(t *testing.T) {
		cfg := newTestRouterConfig(true)
		cfg.AdminToken = ""
		router := transporthttp.NewRouter(cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("expected router to allow /api/v1/subscriptions in open mode, got 401: %s", rec.Body.String())
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK in open mode, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("token_mode_blocks_admin_route", func(t *testing.T) {
		cfg := newTestRouterConfig(true)
		cfg.AdminToken = "secret-token"
		router := transporthttp.NewRouter(cfg)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized in token mode, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
