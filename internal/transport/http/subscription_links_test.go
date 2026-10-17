package http_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func setupLiveSubTestRouter(t *testing.T) (http.Handler, *sql.DB, domain.SettingsRepository, *publication.Service) {
	t.Helper()
	db := newCleanSQLiteDB(t)

	now := time.Now().UTC()
	revID := "rev-admin-test-1"
	nodeID := "node_0123456789abcdef"
	groupID := domain.MustNewUUIDv7()
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-rev-admin', 'active', ?);
	`, revID, now.Format(time.RFC3339Nano))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES ('sub-admin-1', 'Sub Admin', 'secret://url', 1, '1', ?, ?);
	`, now.Format(time.RFC3339), now.Format(time.RFC3339))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES ('fetch-admin-1', 'sub-admin-1', ?, 'success', 'hash-1', 1, 1);
	`, now.Format(time.RFC3339Nano))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'Admin Test Node', '1.2.3.4', 8388, '{"method":"aes-128-gcm","password":"secret"}', 1, ?, ?, 1);
	`, nodeID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, 'sub-admin-1', 'fetch-admin-1');
	`, nodeID)
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'Proxy', 'select', ?, ?, 0, 1);
	`, groupID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, created_at, updated_at)
		VALUES ('pgs-admin-1', ?, 'add', 'manual_node', 0, ?, ?, ?);
	`, groupID, nodeID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES ('pr-admin-1', ?, ?, 'MATCH', 0);
	`, revID, groupID)

	settingsRepo := sqlite.NewSettingsRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	linkRepo := sqlite.NewSubscriptionLinkRepository(db)
	factsLoader := sqlite.NewConsistentFactsLoader(db)

	pubSvc := publication.NewService(
		nil, auditRepo,
		publication.WithSubscriptionLinkRepository(linkRepo),
		publication.WithConsistentFactsLoader(factsLoader),
	)

	holder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		InitialToken: "admin-secret-token",
		HashCost:     transporthttp.MinHashCost,
	})

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		TokenHolder:        holder,
		SettingsRepository: settingsRepo,
		AuditRepository:    auditRepo,
		PublicationService: pubSvc,
		IsPublicationToken: pubSvc.IsPublicationToken,
	})

	return router, db, settingsRepo, pubSvc
}

func doRequest(handler http.Handler, method, path, payload, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(payload))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if payload != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSubscriptionLinksAdminAPI_FullLifecycle(t *testing.T) {
	router, _, settingsRepo, _ := setupLiveSubTestRouter(t)
	adminToken := "admin-secret-token"
	ctx := context.Background()

	// 1. Initial list should be empty
	rec := doRequest(router, http.MethodGet, "/api/v1/subscription-links", "", adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/subscription-links failed: %d, body: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(listResp.Data) != 0 {
		t.Fatalf("expected 0 links, got %d", len(listResp.Data))
	}

	// 2. Create subscription link
	createPayload := `{"target":"mihomo","label":"Mihomo Production","is_default":true}`
	recCreate := doRequest(router, http.MethodPost, "/api/v1/subscription-links", createPayload, adminToken)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/subscription-links failed: %d, body: %s", recCreate.Code, recCreate.Body.String())
	}
	var createResp struct {
		Data struct {
			ID          string `json:"id"`
			Target      string `json:"target"`
			Label       string `json:"label"`
			Token       string `json:"token"`
			TokenMasked string `json:"token_masked"`
			IsDefault   bool   `json:"is_default"`
			State       string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recCreate.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	linkID := createResp.Data.ID
	if linkID == "" || createResp.Data.Token == "" {
		t.Fatalf("expected ID and Token in create response, got %+v", createResp.Data)
	}
	if createResp.Data.Target != "mihomo" || !createResp.Data.IsDefault || createResp.Data.State != "active" {
		t.Fatalf("unexpected create fields: %+v", createResp.Data)
	}

	// Verify headers: no-store, no-referrer
	if cc := recCreate.Header().Get("Cache-Control"); cc == "" {
		t.Fatal("expected Cache-Control header")
	}
	if ref := recCreate.Header().Get("Referrer-Policy"); ref != "no-referrer" {
		t.Fatalf("expected Referrer-Policy no-referrer, got %s", ref)
	}

	// 3. Create again with is_default: true and target: mihomo should be idempotent
	recCreate2 := doRequest(router, http.MethodPost, "/api/v1/subscription-links", createPayload, adminToken)
	if recCreate2.Code != http.StatusCreated {
		t.Fatalf("idempotent create failed: %d", recCreate2.Code)
	}
	var createResp2 struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(recCreate2.Body.Bytes(), &createResp2)
	if createResp2.Data.ID != linkID {
		t.Fatalf("expected idempotent create to return existing ID %s, got %s", linkID, createResp2.Data.ID)
	}

	// 4. Copy URL when PublicBaseURL is not configured -> must succeed with 200 OK returning relative path and url
	recCopyNoBase := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+linkID+"/copy", "", adminToken)
	if recCopyNoBase.Code != http.StatusOK {
		t.Fatalf("expected 200 when public base URL is empty, got %d, body: %s", recCopyNoBase.Code, recCopyNoBase.Body.String())
	}
	var copyNoBaseResp struct {
		Data struct {
			ID            string `json:"id"`
			Target        string `json:"target"`
			Path          string `json:"path"`
			URL           string `json:"url"`
			Token         string `json:"token"`
			PublicBaseURL string `json:"public_base_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recCopyNoBase.Body.Bytes(), &copyNoBaseResp); err != nil {
		t.Fatalf("failed to decode copyNoBase response: %v", err)
	}
	expectedRelativePath := "/subscribe/v1/" + linkID + "?token=" + createResp.Data.Token
	if copyNoBaseResp.Data.Path != expectedRelativePath {
		t.Fatalf("expected relative path %s, got %s", expectedRelativePath, copyNoBaseResp.Data.Path)
	}
	if copyNoBaseResp.Data.URL != expectedRelativePath {
		t.Fatalf("expected relative URL %s when publicBaseURL is empty, got %s", expectedRelativePath, copyNoBaseResp.Data.URL)
	}
	if copyNoBaseResp.Data.PublicBaseURL != "" {
		t.Fatalf("expected empty public_base_url, got %s", copyNoBaseResp.Data.PublicBaseURL)
	}

	// Configure PublicBaseURL via PUT /api/v1/settings/public-base-url
	setBasePayload := `{"public_base_url":"https://sub.mycsp.example.com"}`
	recSetBase := doRequest(router, http.MethodPut, "/api/v1/settings/public-base-url", setBasePayload, adminToken)
	if recSetBase.Code != http.StatusOK {
		t.Fatalf("PUT /api/v1/settings/public-base-url failed: %d, body: %s", recSetBase.Code, recSetBase.Body.String())
	}

	// GET /api/v1/settings/public-base-url
	recGetBase := doRequest(router, http.MethodGet, "/api/v1/settings/public-base-url", "", adminToken)
	if recGetBase.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/settings/public-base-url failed: %d", recGetBase.Code)
	}

	// 5. Copy URL now -> must return 200 with full URL containing public base URL, path, and link token
	recCopy := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+linkID+"/copy", "", adminToken)
	if recCopy.Code != http.StatusOK {
		t.Fatalf("POST copy failed: %d, body: %s", recCopy.Code, recCopy.Body.String())
	}
	var copyResp struct {
		Data struct {
			ID            string `json:"id"`
			Target        string `json:"target"`
			Path          string `json:"path"`
			URL           string `json:"url"`
			Token         string `json:"token"`
			PublicBaseURL string `json:"public_base_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recCopy.Body.Bytes(), &copyResp); err != nil {
		t.Fatalf("failed to decode copy response: %v", err)
	}
	if copyResp.Data.Path != expectedRelativePath {
		t.Fatalf("expected path %s, got %s", expectedRelativePath, copyResp.Data.Path)
	}
	expectedPrefix := "https://sub.mycsp.example.com/subscribe/v1/" + linkID + "?token="
	if len(copyResp.Data.URL) < len(expectedPrefix) || copyResp.Data.URL[:len(expectedPrefix)] != expectedPrefix {
		t.Fatalf("unexpected copy URL: %s", copyResp.Data.URL)
	}

	// 6. Update label (PATCH)
	patchPayload := `{"label":"Updated Mihomo Label"}`
	recPatch := doRequest(router, http.MethodPatch, "/api/v1/subscription-links/"+linkID, patchPayload, adminToken)
	if recPatch.Code != http.StatusOK {
		t.Fatalf("PATCH failed: %d, body: %s", recPatch.Code, recPatch.Body.String())
	}

	// 7. Rotate token
	recRotate := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+linkID+"/rotate-token", "", adminToken)
	if recRotate.Code != http.StatusOK {
		t.Fatalf("rotate-token failed: %d, body: %s", recRotate.Code, recRotate.Body.String())
	}
	var rotateResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(recRotate.Body.Bytes(), &rotateResp)
	if rotateResp.Data.Token == "" || rotateResp.Data.Token == createResp.Data.Token {
		t.Fatalf("expected new token on rotation, got %s", rotateResp.Data.Token)
	}

	// 8. Check subscription link before revocation
	recCheck := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+linkID+"/check", "", adminToken)
	if recCheck.Code != http.StatusOK {
		t.Fatalf("check failed: %d, body: %s", recCheck.Code, recCheck.Body.String())
	}
	var checkResp struct {
		Data struct {
			ID      string `json:"id"`
			Target  string `json:"target"`
			Allowed bool   `json:"allowed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recCheck.Body.Bytes(), &checkResp); err != nil {
		t.Fatalf("failed to decode check response: %v", err)
	}
	if checkResp.Data.ID != linkID || checkResp.Data.Target != "mihomo" {
		t.Fatalf("unexpected check data: %+v", checkResp.Data)
	}

	// 9. Revoke
	recRevoke := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+linkID+"/revoke", "", adminToken)
	if recRevoke.Code != http.StatusOK {
		t.Fatalf("revoke failed: %d, body: %s", recRevoke.Code, recRevoke.Body.String())
	}

	// Subsequent check on revoked link must return 403
	recCheckRevoked := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+linkID+"/check", "", adminToken)
	if recCheckRevoked.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on checking revoked link, got %d", recCheckRevoked.Code)
	}

	// Subsequent rotate on revoked link must return 403
	recRotateRevoked := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+linkID+"/rotate-token", "", adminToken)
	if recRotateRevoked.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on rotating revoked link, got %d", recRotateRevoked.Code)
	}

	// Subsequent copy on revoked link must return 403
	recCopyRevoked := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+linkID+"/copy", "", adminToken)
	if recCopyRevoked.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on copying revoked link, got %d", recCopyRevoked.Code)
	}

	_ = settingsRepo
	_ = ctx
}

func TestSettingsPublicBaseURL_Rejections(t *testing.T) {
	router, _, _, _ := setupLiveSubTestRouter(t)
	adminToken := "admin-secret-token"

	tests := []struct {
		name    string
		payload string
	}{
		{"userinfo", `{"public_base_url":"https://admin:pass@example.com"}`},
		{"query parameter", `{"public_base_url":"https://example.com/sub?key=val"}`},
		{"query question mark", `{"public_base_url":"https://example.com/sub?"}`},
		{"fragment", `{"public_base_url":"https://example.com/sub#frag"}`},
		{"ftp scheme", `{"public_base_url":"ftp://example.com"}`},
		{"relative url", `{"public_base_url":"/sub"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(router, http.MethodPut, "/api/v1/settings/public-base-url", tt.payload, adminToken)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422 for %s, got %d body: %s", tt.name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSubscriptionLinksAdminAPI_AllFourTargetsDefaultAndCopyNoBase(t *testing.T) {
	router, _, _, _ := setupLiveSubTestRouter(t)
	adminToken := "admin-secret-token"

	targets := []string{"mihomo", "singbox", "surge", "qx"}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			createPayload := fmt.Sprintf(`{"target":%q,"is_default":true}`, target)
			recCreate := doRequest(router, http.MethodPost, "/api/v1/subscription-links", createPayload, adminToken)
			if recCreate.Code != http.StatusCreated {
				t.Fatalf("create failed for %s: %d, body: %s", target, recCreate.Code, recCreate.Body.String())
			}
			var created struct {
				Data struct {
					ID     string `json:"id"`
					Target string `json:"target"`
					Token  string `json:"token"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recCreate.Body.Bytes(), &created); err != nil {
				t.Fatalf("unmarshal create: %v", err)
			}
			if created.Data.Target != target || created.Data.ID == "" || created.Data.Token == "" {
				t.Fatalf("invalid created data: %+v", created.Data)
			}

			// Copy without public base url -> must succeed with 200 OK and relative path
			recCopy := doRequest(router, http.MethodPost, "/api/v1/subscription-links/"+created.Data.ID+"/copy", "", adminToken)
			if recCopy.Code != http.StatusOK {
				t.Fatalf("copy failed for %s without base url: %d, body: %s", target, recCopy.Code, recCopy.Body.String())
			}
			var copyResp struct {
				Data struct {
					ID            string `json:"id"`
					Target        string `json:"target"`
					Path          string `json:"path"`
					URL           string `json:"url"`
					Token         string `json:"token"`
					PublicBaseURL string `json:"public_base_url"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recCopy.Body.Bytes(), &copyResp); err != nil {
				t.Fatalf("unmarshal copy: %v", err)
			}
			expectedPath := fmt.Sprintf("/subscribe/v1/%s?token=%s", created.Data.ID, created.Data.Token)
			if copyResp.Data.Path != expectedPath || copyResp.Data.URL != expectedPath {
				t.Fatalf("expected path and URL to be %q, got path=%q url=%q", expectedPath, copyResp.Data.Path, copyResp.Data.URL)
			}
			if copyResp.Data.PublicBaseURL != "" {
				t.Fatalf("expected empty public_base_url, got %q", copyResp.Data.PublicBaseURL)
			}
		})
	}
}

func TestSubscriptionLinksAdminAPI_MaliciousHostHeadersIgnored(t *testing.T) {
	router, _, _, _ := setupLiveSubTestRouter(t)
	adminToken := "admin-secret-token"

	createPayload := `{"target":"mihomo","is_default":true}`
	recCreate := doRequest(router, http.MethodPost, "/api/v1/subscription-links", createPayload, adminToken)
	var created struct {
		Data struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(recCreate.Body.Bytes(), &created)

	// Send copy request with spoofed Host and X-Forwarded-Host
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscription-links/"+created.Data.ID+"/copy", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Host = "attacker.example.com"
	req.Header.Set("X-Forwarded-Host", "malicious.org")
	req.Header.Set("X-Forwarded-Proto", "https")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "attacker.example.com") || strings.Contains(bodyStr, "malicious.org") {
		t.Fatalf("malicious host header leaked into copy response: %s", bodyStr)
	}
}
