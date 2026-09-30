package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func TestAuthSettingsAPIAndAdminModes(t *testing.T) {
	db := newCleanSQLiteDB(t)
	repo := sqlite.NewSettingsRepository(db)
	store := transporthttp.NewMemorySessionStore(time.Hour)
	holder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		SettingsRepo: repo, SessionStore: store, HashCost: transporthttp.MinHashCost,
	})
	router := transporthttp.NewRouter(transporthttp.RouterConfig{TokenHolder: holder, SettingsRepository: repo, SessionStore: store})

	request := func(method, path, payload, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(payload))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}
	decode := func(rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data
	}

	initial := request(http.MethodGet, "/api/v1/settings/auth", "", "")
	if initial.Code != http.StatusOK {
		t.Fatalf("initial GET: %d %s", initial.Code, initial.Body.String())
	}
	if data := decode(initial); data["admin_auth_enabled"] != true || data["export_auth_enabled"] != true || data["token_configured"] != false || data["admin_mode"] != "open" || data["export_mode"] != "protected" {
		t.Fatalf("unexpected defaults: %+v", data)
	}

	invalid := request(http.MethodPut, "/api/v1/settings/auth", `{"admin_auth_enabled":false}`, "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("missing switch: expected 400, got %d", invalid.Code)
	}
	if data := decode(request(http.MethodGet, "/api/v1/settings/auth", "", "")); data["admin_auth_enabled"] != true {
		t.Fatalf("invalid payload changed state: %+v", data)
	}

	change := `{"token":"shared-secret","admin_auth_enabled":false,"export_auth_enabled":true}`
	put := request(http.MethodPut, "/api/v1/settings/auth", change, "")
	if put.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", put.Code, put.Body.String())
	}
	if data := decode(put); data["token_configured"] != true || data["admin_mode"] != "open" || data["export_mode"] != "protected" {
		t.Fatalf("unexpected auth state: %+v", data)
	}
	if strings.Contains(put.Body.String(), "shared-secret") || strings.Contains(put.Body.String(), `"token"`) || strings.Contains(put.Body.String(), `"token_hash"`) {
		t.Fatalf("auth settings response leaked token material: %s", put.Body.String())
	}
	if rec := request(http.MethodGet, "/api/v1/settings/auth", "", "pub_bad"); rec.Code != http.StatusOK {
		t.Fatalf("open admin must accept arbitrary bearer: %d", rec.Code)
	}
	if rec := request(http.MethodGet, "/api/v1/settings/admin-token", "", ""); rec.Code != http.StatusOK || decode(rec)["configured"] != true {
		t.Fatalf("legacy status must report configured even when admin switch is open: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodGet, "/api/v1/auth/status", "", ""); rec.Code != http.StatusOK || decode(rec)["export_mode"] != "protected" {
		t.Fatalf("auth status did not reflect export protection: %s", rec.Body.String())
	}
	st, err := repo.Get(context.Background())
	if err != nil || st.AdminAuthEnabled || !st.ExportAuthEnabled || st.AdminToken == "" || st.AdminToken == "shared-secret" {
		t.Fatalf("failed persistence or plaintext leaked: %+v %v", st, err)
	}
	// A new holder booted from persisted settings must retain independent modes and the master verifier.
	restarted := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		InitialVerifier: st.AdminToken, AdminAuthEnabled: &st.AdminAuthEnabled,
		ExportAuthEnabled: &st.ExportAuthEnabled, HashCost: transporthttp.MinHashCost,
	})
	if restarted.AdminMode() != transporthttp.SecurityModeOpen || !restarted.IsExportAuthRequired() || !restarted.Verify("shared-secret") {
		t.Fatal("restarted holder lost persisted auth state")
	}

	protected := request(http.MethodPut, "/api/v1/settings/auth", `{"admin_auth_enabled":true,"export_auth_enabled":false}`, "")
	if protected.Code != http.StatusOK || decode(protected)["admin_mode"] != "protected" || decode(protected)["export_mode"] != "open" {
		t.Fatalf("protected mode transition: %d %s", protected.Code, protected.Body.String())
	}
	if rec := request(http.MethodGet, "/api/v1/settings/auth", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected admin must reject anonymous request: %d", rec.Code)
	}
	if rec := request(http.MethodGet, "/api/v1/settings/auth", "", "shared-secret"); rec.Code != http.StatusOK {
		t.Fatalf("protected admin must accept master token: %d", rec.Code)
	}
	for _, bearer := range []string{"", "incorrect", "pub_unknown"} {
		if rec := request(http.MethodGet, "/api/v1/settings/auth", "", bearer); rec.Code != http.StatusUnauthorized {
			t.Fatalf("protected admin must return 401 for invalid credential %q: %d", bearer, rec.Code)
		}
	}
	st, err = repo.Get(context.Background())
	if err != nil || !st.AdminAuthEnabled || st.ExportAuthEnabled || st.AdminToken == "" {
		t.Fatalf("switch update should preserve token: %+v %v", st, err)
	}

	clear := request(http.MethodPut, "/api/v1/settings/auth", `{"clear_token":true,"admin_auth_enabled":true,"export_auth_enabled":false}`, "shared-secret")
	if clear.Code != http.StatusOK || decode(clear)["token_configured"] != false || decode(clear)["admin_mode"] != "open" {
		t.Fatalf("clear token: %d %s", clear.Code, clear.Body.String())
	}
	st, err = repo.Get(context.Background())
	if err != nil || st.AdminToken != "" {
		t.Fatalf("token not cleared: %+v %v", st, err)
	}

	legacy := request(http.MethodPost, "/api/v1/settings/admin-token", `{"token":"legacy-secret"}`, "")
	if legacy.Code != http.StatusOK {
		t.Fatalf("legacy update: %d %s", legacy.Code, legacy.Body.String())
	}
	if data := decode(request(http.MethodGet, "/api/v1/settings/auth", "", "legacy-secret")); data["token_configured"] != true || data["export_auth_enabled"] != false {
		t.Fatalf("legacy token update must preserve export switch: %+v", data)
	}
}
