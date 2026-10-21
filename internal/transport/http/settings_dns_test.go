package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func TestDNSSettingsEndpoints(t *testing.T) {
	db := newCleanSQLiteDB(t)
	dnsRepo := sqlite.NewDNSSettingsRepository(db)
	settingsRepo := sqlite.NewSettingsRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	store := transporthttp.NewMemorySessionStore(time.Hour)
	holder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		SettingsRepo: settingsRepo,
		SessionStore: store,
		HashCost:     transporthttp.MinHashCost,
	})

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		TokenHolder:           holder,
		SettingsRepository:    settingsRepo,
		DNSSettingsRepository: dnsRepo,
		AuditRepository:       auditRepo,
		SessionStore:          store,
	})

	request := func(method, path, payload string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1. Defaults GET returns 200, source="default", revision=0, enabled=true, default nameservers
	t.Run("defaults GET", func(t *testing.T) {
		rec := request(http.MethodGet, "/api/v1/settings/dns", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var env struct {
			Data struct {
				Enabled   bool           `json:"enabled"`
				RawYAML   string         `json:"raw_yaml"`
				Revision  int64          `json:"revision"`
				UpdatedAt *time.Time     `json:"updated_at"`
				Source    string         `json:"source"`
				Config    map[string]any `json:"config"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if !env.Data.Enabled {
			t.Fatalf("expected enabled=true by default")
		}
		if env.Data.Source != "default" {
			t.Fatalf("expected source=default, got %s", env.Data.Source)
		}
		if env.Data.Revision != 0 {
			t.Fatalf("expected revision=0, got %d", env.Data.Revision)
		}
		if env.Data.Config == nil || env.Data.Config["enable"] != true {
			t.Fatalf("expected config.enable=true, got %+v", env.Data.Config)
		}
	})

	// 2. Validate endpoint: POST /settings/dns/validate computes normalization without writing
	t.Run("validate without writing", func(t *testing.T) {
		valPayload := `{"enabled": true, "raw_yaml": "enable: true\nnameserver:\n  - 9.9.9.9\n"}`
		rec := request(http.MethodPost, "/api/v1/settings/dns/validate", valPayload)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var env struct {
			Data struct {
				RawYAML string         `json:"raw_yaml"`
				Config  map[string]any `json:"config"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if env.Data.Config["enable"] != true {
			t.Fatalf("expected config.enable=true")
		}

		// Read back from repo: revision must still be 0 (no write occurred)
		getRec := request(http.MethodGet, "/api/v1/settings/dns", "")
		var getEnv struct {
			Data struct {
				Revision int64  `json:"revision"`
				Source   string `json:"source"`
			} `json:"data"`
		}
		_ = json.Unmarshal(getRec.Body.Bytes(), &getEnv)
		if getEnv.Data.Revision != 0 || getEnv.Data.Source != "default" {
			t.Fatalf("validate endpoint must not persist changes")
		}
	})

	// 3. PUT valid DNS settings with expected_revision=0
	t.Run("PUT valid DNS settings", func(t *testing.T) {
		putPayload := `{"enabled": true, "raw_yaml": "enable: true\nnameserver:\n  - 1.1.1.1\n  - 8.8.4.4\nenhanced-mode: fake-ip\n", "expected_revision": 0}`
		rec := request(http.MethodPut, "/api/v1/settings/dns", putPayload)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var env struct {
			Data struct {
				Enabled   bool           `json:"enabled"`
				RawYAML   string         `json:"raw_yaml"`
				Revision  int64          `json:"revision"`
				UpdatedAt *time.Time     `json:"updated_at"`
				Source    string         `json:"source"`
				Config    map[string]any `json:"config"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if env.Data.Revision != 1 {
			t.Fatalf("expected revision 1, got %d", env.Data.Revision)
		}
		if env.Data.Source != "saved" {
			t.Fatalf("expected source saved, got %s", env.Data.Source)
		}
		if env.Data.UpdatedAt == nil {
			t.Fatalf("expected non-nil updated_at after save")
		}
		if env.Data.Config["enhanced-mode"] != "fake-ip" {
			t.Fatalf("expected config to retain enhanced-mode: fake-ip")
		}

		// Read back via GET
		readbackRec := request(http.MethodGet, "/api/v1/settings/dns", "")
		if readbackRec.Code != http.StatusOK {
			t.Fatalf("readback failed: %d", readbackRec.Code)
		}
		var rbEnv struct {
			Data struct {
				Revision int64          `json:"revision"`
				Source   string         `json:"source"`
				Config   map[string]any `json:"config"`
			} `json:"data"`
		}
		_ = json.Unmarshal(readbackRec.Body.Bytes(), &rbEnv)
		if rbEnv.Data.Revision != 1 || rbEnv.Data.Source != "saved" {
			t.Fatalf("readback mismatch: %+v", rbEnv.Data)
		}
		if rbEnv.Data.Config["enhanced-mode"] != "fake-ip" {
			t.Fatalf("readback missing enhanced-mode")
		}
	})

	// 4. 409 Conflict: PUT with stale expected_revision=0 when current is 1
	t.Run("409 revision conflict", func(t *testing.T) {
		putPayload := `{"enabled": true, "raw_yaml": "enable: true\nnameserver:\n  - 9.9.9.9\n", "expected_revision": 0}`
		rec := request(http.MethodPut, "/api/v1/settings/dns", putPayload)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
		var errResp struct {
			Code            string `json:"code"`
			CurrentRevision *int64 `json:"current_revision"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("unmarshal error resp: %v", err)
		}
		if errResp.CurrentRevision == nil || *errResp.CurrentRevision != 1 {
			t.Fatalf("expected current_revision=1, got %v", errResp.CurrentRevision)
		}
	})

	// 5. 422 Validation errors: invalid YAML, negative expected_revision, missing enabled
	t.Run("422 validation errors", func(t *testing.T) {
		cases := []struct {
			name    string
			payload string
		}{
			{"invalid YAML", `{"enabled": true, "raw_yaml": "invalid: [unclosed", "expected_revision": 1}`},
			{"negative expected_revision", `{"enabled": true, "raw_yaml": "enable: true\n", "expected_revision": -1}`},
			{"missing enabled", `{"raw_yaml": "enable: true\n", "expected_revision": 1}`},
			{"missing expected_revision", `{"enabled": true, "raw_yaml": "enable: true\n"}`},
			{"duplicate key in YAML", `{"enabled": true, "raw_yaml": "enable: true\nenable: false\n", "expected_revision": 1}`},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := request(http.MethodPut, "/api/v1/settings/dns", tc.payload)
				if rec.Code != http.StatusUnprocessableEntity {
					t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
	})

	// 6. 503 Service Unavailable when DNSSettingsRepository is nil
	t.Run("503 when repo is nil", func(t *testing.T) {
		nilRepoRouter := transporthttp.NewRouter(transporthttp.RouterConfig{
			TokenHolder:           holder,
			SettingsRepository:    settingsRepo,
			DNSSettingsRepository: nil, // explicitly nil
		})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/dns", nil)
		rec := httptest.NewRecorder()
		nilRepoRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
		}

		putReq := httptest.NewRequest(http.MethodPut, "/api/v1/settings/dns", bytes.NewBufferString(`{"enabled": true, "raw_yaml": "enable: true\n", "expected_revision": 0}`))
		putRec := httptest.NewRecorder()
		nilRepoRouter.ServeHTTP(putRec, putReq)
		if putRec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 on PUT, got %d: %s", putRec.Code, putRec.Body.String())
		}
	})
}
