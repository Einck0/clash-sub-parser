package http_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func TestSubscriptionFetchSettingsSQLiteLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "settings.db")
	cfg := sqlite.Config{Path: path, ForeignKeys: true}
	db, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repo := sqlite.NewSettingsRepository(db)
	initial, err := repo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initial.AdminToken = "existing-verifier"
	initial.AdminAuthEnabled = false
	initial.ExportAuthEnabled = false
	initial.FetchTimeoutSeconds = 67
	if err := repo.Update(ctx, initial); err != nil {
		t.Fatal(err)
	}
	router := transporthttp.NewRouter(transporthttp.RouterConfig{SettingsRepository: repo, EnvironmentFetchProxy: "http://env-user:env-secret@127.0.0.1:7890"})
	request := func(method, payload string, status int) map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/settings/subscription-fetch", bytes.NewBufferString(payload)))
		if rec.Code != status {
			t.Fatalf("%s expected %d got %d: %s", payload, status, rec.Code, rec.Body.String())
		}
		for _, secret := range []string{"env-user", "env-secret", "proxy-user", "proxy-secret", "existing-verifier", base64.StdEncoding.EncodeToString([]byte("proxy-user:proxy-secret"))} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Fatalf("response leaked credential: %s", rec.Body.String())
			}
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data
	}
	first := request("GET", "", 200)
	if first["mode"] != "environment" || first["effective_source"] != "environment" || first["effective_proxy_endpoint"] != "http://127.0.0.1:7890" || first["effective_credentials_present"] != true {
		t.Fatalf("bad migration default: %+v", first)
	}
	custom := request("PUT", `{"mode":"proxy","proxy_url":"http://proxy-user:proxy-secret@127.0.0.1:8888"}`, 200)
	if custom["effective_source"] != "settings" || custom["credentials_present"] != true || custom["configured_proxy_endpoint"] != "http://127.0.0.1:8888" {
		t.Fatalf("bad proxy view: %+v", custom)
	}
	saved, _ := repo.Get(ctx)
	for _, payload := range []string{
		`{"mode":"bad"}`, `{"mode":"proxy","proxy_url":"socks5://127.0.0.1:80"}`,
		`{"mode":"direct","proxy_url":"http://u:***@127.0.0.1:80"}`,
		`{"mode":"proxy","proxy_url":"http://127.0.0.1:80/path"}`,
		`{"mode":"proxy","proxy_url":"http://127.0.0.1:80?"}`,
		`{"mode":"proxy","proxy_url":"http://127.0.0.1:80#"}`,
		`{"mode":"proxy","proxy_url":"http://0177.0.0.1:80"}`,
		`{"mode":"proxy","proxy_url":"http://127.0.0.1:080"}`,
		`{"mode":"proxy","proxy_url":"http://127.0.0.1"}`,
		`{"mode":"direct","unknown":true}`, `{"mode":"direct","admin_auth_enabled":true}`,
		`{"mode":"direct","mode":"proxy"}`, `{"mode":null}`, `{"mode":"direct","proxy_url":null}`,
		`{"mode":"direct","clear_proxy_url":null}`, `{"mode":"direct","clear_proxy_url":true,"proxy_url":""}`,
		`{"mode":"proxy","proxy_url":""}`, `{"mode":"proxy","clear_proxy_url":true}`,
		`{}`, `null`, `[]`, `{"mode":"direct"} {}`, `{"mode":"direct","proxy_url":"` + strings.Repeat("x", 17000) + `"}`,
	} {
		request("PUT", payload, 400)
		after, _ := repo.Get(ctx)
		if *after != *saved {
			t.Fatalf("invalid save mutated settings for %s", payload)
		}
	}
	request("PUT", `{"mode":"direct"}`, 200)
	retained := request("PUT", `{"mode":"proxy"}`, 200)
	if retained["credentials_present"] != true {
		t.Fatal("omission lost credentials")
	}
	request("PUT", `{"mode":"environment"}`, 200)
	if err := repo.UpdateAuthSettings(ctx, false, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateAdminToken(ctx, "updated-verifier"); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.Get(ctx)
	if after.SubscriptionFetchProxyURL != saved.SubscriptionFetchProxyURL || after.FetchTimeoutSeconds != 67 || after.AdminAuthEnabled || !after.ExportAuthEnabled || after.AdminToken != "updated-verifier" {
		t.Fatal("settings paths clobbered unrelated fields")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	repo = sqlite.NewSettingsRepository(db)
	router = transporthttp.NewRouter(transporthttp.RouterConfig{SettingsRepository: repo})
	restarted := request("GET", "", 200)
	if restarted["mode"] != "environment" || restarted["credentials_present"] != true {
		t.Fatalf("restart lost settings: %+v", restarted)
	}
	request("PUT", `{"mode":"direct","clear_proxy_url":true}`, 200)
	after, _ = repo.Get(ctx)
	if after.SubscriptionFetchProxyURL != "" {
		t.Fatal("explicit clear failed")
	}
	request("PUT", `{"mode":"proxy"}`, 400)
}

func TestSubscriptionFetchSettingsManagementBoundary(t *testing.T) {
	repo := sqlite.NewSettingsRepository(newCleanSQLiteDB(t))
	holder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{InitialToken: "admin-secret", HashCost: transporthttp.MinHashCost})
	store := transporthttp.NewMemorySessionStore(time.Hour)
	router := transporthttp.NewRouter(transporthttp.RouterConfig{TokenHolder: holder, SettingsRepository: repo, SessionStore: store,
		SessionValidator: func(id string) (*transporthttp.SessionInfo, bool) { return store.Get(id) }})
	request := func(method, path, payload, bearer string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(payload))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, method := range []string{"GET", "PUT"} {
		if rec := request(method, "/api/v1/settings/subscription-fetch", `{"mode":"direct"}`, "", nil, ""); rec.Code != 401 {
			t.Fatalf("anonymous %s: %d", method, rec.Code)
		}
	}
	login := request("POST", "/api/v1/auth/login", `{"token":"admin-secret"}`, "", nil, "")
	if login.Code != 200 {
		t.Fatalf("login: %s", login.Body.String())
	}
	var envelope struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	cookies := login.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}
	if rec := request("PUT", "/api/v1/settings/subscription-fetch", `{"mode":"direct"}`, "", cookies[0], ""); rec.Code != 403 {
		t.Fatalf("missing CSRF: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request("PUT", "/api/v1/settings/subscription-fetch", `{"mode":"direct"}`, "", cookies[0], envelope.Data.CSRFToken); rec.Code != 200 {
		t.Fatalf("valid CSRF: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request("PUT", "/api/v1/settings/subscription-fetch", `{"mode":"environment"}`, "admin-secret", nil, ""); rec.Code != 200 {
		t.Fatalf("valid bearer: %d", rec.Code)
	}
}
