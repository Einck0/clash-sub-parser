package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	transporthttp "clash-sub-parser/internal/transport/http"
)

func TestAuthStatusEndpoint_OpenMode(t *testing.T) {
	testCases := []struct {
		name       string
		adminToken string
		authHeader string
	}{
		{name: "empty admin token, anonymous", adminToken: "", authHeader: ""},
		{name: "whitespace admin token, anonymous", adminToken: "   \t\n  ", authHeader: ""},
		{name: "open mode with garbage bearer token", adminToken: "", authHeader: "Bearer invalid-garbage-token"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestRouterConfig(true)
			cfg.AdminToken = tc.adminToken
			router := transporthttp.NewRouter(cfg)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
			}

			var resp testDataResponse[transporthttp.AuthStatusData]
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Data.Mode != transporthttp.SecurityModeOpen {
				t.Errorf("expected mode to be 'open', got %q", resp.Data.Mode)
			}
			if !resp.Data.Authenticated {
				t.Errorf("expected authenticated to be true in open mode")
			}
			if resp.Data.Subject != "admin" {
				t.Errorf("expected subject to be 'admin', got %q", resp.Data.Subject)
			}
		})
	}
}

func TestAuthStatusEndpoint_ProtectedMode_Anonymous(t *testing.T) {
	cfg := newTestRouterConfig(true)
	cfg.AdminToken = "super-secret-admin-token"
	router := transporthttp.NewRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp testDataResponse[transporthttp.AuthStatusData]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Data.Mode != transporthttp.SecurityModeProtected {
		t.Errorf("expected mode to be 'protected', got %q", resp.Data.Mode)
	}
	if resp.Data.Authenticated {
		t.Errorf("expected authenticated to be false for anonymous request in protected mode")
	}
	if resp.Data.Subject != "" {
		t.Errorf("expected subject to be empty, got %q", resp.Data.Subject)
	}
}

func TestAuthStatusEndpoint_ProtectedMode_WithBearer(t *testing.T) {
	secretToken := "my-secret-admin-pass-999"
	cfg := newTestRouterConfig(true)
	cfg.AdminToken = secretToken
	router := transporthttp.NewRouter(cfg)

	t.Run("valid_bearer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
		req.Header.Set("Authorization", "Bearer "+secretToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		var resp testDataResponse[transporthttp.AuthStatusData]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Data.Mode != transporthttp.SecurityModeProtected || !resp.Data.Authenticated || resp.Data.Subject != "admin" {
			t.Errorf("unexpected status data: %+v", resp.Data)
		}
	})

	t.Run("invalid_bearer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
		req.Header.Set("Authorization", "Bearer wrong-password")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		var resp testDataResponse[transporthttp.AuthStatusData]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Data.Authenticated {
			t.Errorf("expected authenticated to be false for invalid bearer")
		}
	})
}

func TestAuthStatusEndpoint_ProtectedMode_WithSessionCookie(t *testing.T) {
	store := transporthttp.NewMemorySessionStore(1 * time.Hour)
	info, err := store.Create("admin-session-user")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	cfg := newTestRouterConfig(true)
	cfg.AdminToken = "any-admin-token"
	cfg.SessionStore = store
	router := transporthttp.NewRouter(cfg)

	t.Run("valid_cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
		req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: info.SessionID})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		var resp testDataResponse[transporthttp.AuthStatusData]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Data.Mode != transporthttp.SecurityModeProtected || !resp.Data.Authenticated || resp.Data.Subject != "admin-session-user" {
			t.Errorf("unexpected status data: %+v", resp.Data)
		}
	})

	t.Run("invalid_cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
		req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: "invalid-or-revoked-id"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		var resp testDataResponse[transporthttp.AuthStatusData]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Data.Authenticated {
			t.Errorf("expected authenticated to be false for invalid session cookie")
		}
	})
}

func TestAuthLoginEndpoint(t *testing.T) {
	secretToken := "valid-admin-secret-key"
	store := transporthttp.NewMemorySessionStore(1 * time.Hour)

	cfg := newTestRouterConfig(true)
	cfg.AdminToken = secretToken
	cfg.SessionStore = store
	router := transporthttp.NewRouter(cfg)

	t.Run("login_open_mode", func(t *testing.T) {
		openCfg := newTestRouterConfig(true)
		openCfg.AdminToken = ""
		openRouter := transporthttp.NewRouter(openCfg)

		body := []byte(`{"token":""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		openRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK in open mode, got %d", rec.Code)
		}
		var resp testDataResponse[transporthttp.LoginResponseData]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Data.Mode != transporthttp.SecurityModeOpen || !resp.Data.Authenticated {
			t.Errorf("unexpected open mode login response: %+v", resp.Data)
		}
	})

	t.Run("login_invalid_json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte(`invalid-json`)))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("login_invalid_token", func(t *testing.T) {
		payload, _ := json.Marshal(transporthttp.LoginRequest{Token: "wrong-pass"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for wrong token, got %d", rec.Code)
		}
	})

	t.Run("login_valid_token", func(t *testing.T) {
		payload, _ := json.Marshal(transporthttp.LoginRequest{Token: secretToken})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for valid login, got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var resp testDataResponse[transporthttp.LoginResponseData]
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if !resp.Data.Authenticated || resp.Data.Subject != "admin" {
			t.Errorf("expected authenticated=true subject=admin, got %+v", resp.Data)
		}
		if resp.Data.Token != secretToken {
			t.Errorf("expected returned token %q, got %q", secretToken, resp.Data.Token)
		}
		if resp.Data.CSRFToken == "" {
			t.Error("expected non-empty CSRFToken in login response")
		}

		// Verify cookie was set
		cookies := rec.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, c := range cookies {
			if c.Name == transporthttp.SessionCookieName {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil {
			t.Fatal("expected csp_session cookie to be set")
		}
		if sessionCookie.Value == "" {
			t.Fatal("expected non-empty csp_session cookie value")
		}
		if !sessionCookie.HttpOnly {
			t.Error("expected HttpOnly cookie")
		}
		if sessionCookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("expected SameSite=Lax, got %v", sessionCookie.SameSite)
		}
	})
}

func TestAuthLogoutEndpoint(t *testing.T) {
	store := transporthttp.NewMemorySessionStore(1 * time.Hour)
	info, err := store.Create("admin")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	cfg := newTestRouterConfig(true)
	cfg.SessionStore = store
	router := transporthttp.NewRouter(cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: transporthttp.SessionCookieName, Value: info.SessionID})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for logout, got %d", rec.Code)
	}

	// Verify session was revoked in store
	if _, ok := store.Get(info.SessionID); ok {
		t.Error("expected session to be revoked in store upon logout")
	}

	// Verify clearing cookie was sent
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == transporthttp.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected csp_session clearing cookie to be set")
	}
	if sessionCookie.MaxAge != -1 {
		t.Errorf("expected cookie MaxAge -1, got %d", sessionCookie.MaxAge)
	}
}
