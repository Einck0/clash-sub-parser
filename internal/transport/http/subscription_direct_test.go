package http_test

import (
	"database/sql"
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

func setupDirectSubRouter(t *testing.T, initialToken string, adminAuth, exportAuth bool) (http.Handler, *sql.DB, *transporthttp.DynamicTokenHolder) {
	t.Helper()
	db := newCleanSQLiteDB(t)

	now := time.Now().UTC()
	revID := "rev-direct-test-1"
	nodeID := "node_0123456789abcdef"
	groupID := domain.MustNewUUIDv7()

	_, _ = db.Exec(`
		INSERT OR IGNORE INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-rev-direct', 'active', ?);
	`, revID, now.Format(time.RFC3339Nano))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES ('sub-direct-1', 'Sub Direct', 'secret://url', 1, '1', ?, ?);
	`, now.Format(time.RFC3339), now.Format(time.RFC3339))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES ('fetch-direct-1', 'sub-direct-1', ?, 'success', 'hash-1', 1, 1);
	`, now.Format(time.RFC3339Nano))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'Direct SS Node', '1.2.3.4', 8388, '{"method":"aes-128-gcm","password":"secret"}', 1, ?, ?, 1);
	`, nodeID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, 'sub-direct-1', 'fetch-direct-1');
	`, nodeID)
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'Proxy', 'select', ?, ?, 0, 1);
	`, groupID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, created_at, updated_at)
		VALUES ('pgs-direct-1', ?, 'add', 'manual_node', 0, ?, ?, ?);
	`, groupID, nodeID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	_, _ = db.Exec(`
		INSERT OR IGNORE INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES ('pr-direct-1', ?, ?, 'MATCH', 0);
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
		InitialToken:      initialToken,
		HashCost:          transporthttp.MinHashCost,
		AdminAuthEnabled:  &adminAuth,
		ExportAuthEnabled: &exportAuth,
		SettingsRepo:      settingsRepo,
	})

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		TokenHolder:        holder,
		SettingsRepository: settingsRepo,
		AuditRepository:    auditRepo,
		PublicationService: pubSvc,
		IsPublicationToken: pubSvc.IsPublicationToken,
	})

	return router, db, holder
}

func TestDirectSubscriptionEndpoints_OpenMode(t *testing.T) {
	// Open mode: no token configured
	router, _, _ := setupDirectSubRouter(t, "", false, false)

	assertDirectEndpoint := func(t *testing.T, path, expectedContentType, expectedFilenameSnippet string) {
		t.Helper()
		rec := doRequest(router, http.MethodGet, path, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s returned code %d: %s", path, rec.Code, rec.Body.String())
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", path, cc)
		}
		if ref := rec.Header().Get("Referrer-Policy"); ref != "no-referrer" {
			t.Errorf("GET %s Referrer-Policy = %q, want no-referrer", path, ref)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, expectedContentType) {
			t.Errorf("GET %s Content-Type = %q, want %q", path, ct, expectedContentType)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, expectedFilenameSnippet) {
			t.Errorf("GET %s Content-Disposition = %q, want snippet %q", path, cd, expectedFilenameSnippet)
		}
		if etag := rec.Header().Get("ETag"); etag == "" {
			t.Errorf("GET %s missing ETag header", path)
		}
		if excl := rec.Header().Get("X-Export-Excluded-Count"); excl == "" {
			t.Errorf("GET %s missing X-Export-Excluded-Count header", path)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s returned empty body", path)
		}
	}

	t.Run("Mihomo/Clash endpoints", func(t *testing.T) {
		assertDirectEndpoint(t, "/clash", "application/yaml", "mihomo.yaml")
		assertDirectEndpoint(t, "/mihomo", "application/yaml", "mihomo.yaml")
		assertDirectEndpoint(t, "/sub/clash", "application/yaml", "mihomo.yaml")
		assertDirectEndpoint(t, "/sub/mihomo", "application/yaml", "mihomo.yaml")
	})

	t.Run("SingBox endpoints", func(t *testing.T) {
		assertDirectEndpoint(t, "/singbox", "application/json", "sing-box.json")
		assertDirectEndpoint(t, "/sub/singbox", "application/json", "sing-box.json")
		assertDirectEndpoint(t, "/sub/sing-box", "application/json", "sing-box.json")
	})

	t.Run("Surge endpoints", func(t *testing.T) {
		assertDirectEndpoint(t, "/surge", "text/plain", "surge.conf")
		assertDirectEndpoint(t, "/sub/surge", "text/plain", "surge.conf")
	})

	t.Run("Quantumult X endpoints", func(t *testing.T) {
		assertDirectEndpoint(t, "/qx", "text/plain", "quantumult-x.conf")
		assertDirectEndpoint(t, "/quantumult-x", "text/plain", "quantumult-x.conf")
		assertDirectEndpoint(t, "/sub/qx", "text/plain", "quantumult-x.conf")
		assertDirectEndpoint(t, "/sub/quantumult-x", "text/plain", "quantumult-x.conf")
	})

	t.Run("Invalid generic target returns 404", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/sub/unknown-target", "", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET /sub/unknown-target code = %d, want 404", rec.Code)
		}
	})
}

func TestDirectSubscriptionEndpoints_ProtectedMode(t *testing.T) {
	masterToken := "settings-secret-token-999"
	router, _, _ := setupDirectSubRouter(t, masterToken, true, true)

	t.Run("Anonymous request returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/clash", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Wrong query token returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/clash?token=wrong-token", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Wrong Bearer token returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/clash", "", "wrong-token")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Invalid Authorization scheme returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/clash", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for Basic scheme, got %d", rec.Code)
		}
	})

	t.Run("Conflicting Query token and Bearer header returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/clash?token="+masterToken, nil)
		req.Header.Set("Authorization", "Bearer different-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for conflicting tokens, got %d", rec.Code)
		}
	})

	t.Run("Valid Query token returns 200", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/clash?token="+masterToken, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Valid Bearer token returns 200", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/singbox", "", masterToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Matching Query and Bearer tokens return 200", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/surge?token="+masterToken, "", masterToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDirectSubscriptionEndpoints_ConditionalCaching(t *testing.T) {
	masterToken := "settings-secret-token-etag"
	router, _, _ := setupDirectSubRouter(t, masterToken, true, true)

	// 1. Initial request to obtain ETag
	recInitial := doRequest(router, http.MethodGet, "/clash?token="+masterToken, "", "")
	if recInitial.Code != http.StatusOK {
		t.Fatalf("initial request failed with %d", recInitial.Code)
	}
	etag := recInitial.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag in 200 response")
	}

	// 2. Request with matching If-None-Match -> 304 Not Modified
	req304 := httptest.NewRequest(http.MethodGet, "/clash?token="+masterToken, nil)
	req304.Header.Set("If-None-Match", etag)
	rec304 := httptest.NewRecorder()
	router.ServeHTTP(rec304, req304)

	if rec304.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d: %s", rec304.Code, rec304.Body.String())
	}
	if rec304.Body.Len() != 0 {
		t.Fatalf("expected empty body for 304, got %d bytes", rec304.Body.Len())
	}
	if cc := rec304.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("304 Cache-Control = %q, want no-store", cc)
	}
	if ref := rec304.Header().Get("Referrer-Policy"); ref != "no-referrer" {
		t.Errorf("304 Referrer-Policy = %q, want no-referrer", ref)
	}
	if rec304.Header().Get("ETag") != etag {
		t.Errorf("304 ETag = %q, want %q", rec304.Header().Get("ETag"), etag)
	}
	if excl := rec304.Header().Get("X-Export-Excluded-Count"); excl == "" {
		t.Errorf("304 missing X-Export-Excluded-Count header")
	}

	// 3. Request with mismatched If-None-Match -> 200 OK
	req200 := httptest.NewRequest(http.MethodGet, "/clash?token="+masterToken, nil)
	req200.Header.Set("If-None-Match", `"mismatched-etag"`)
	rec200 := httptest.NewRecorder()
	router.ServeHTTP(rec200, req200)

	if rec200.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec200.Code)
	}
}

func TestDirectSubscriptionEndpoints_LiveDynamicUpdate(t *testing.T) {
	masterToken := "settings-secret-token-live"
	router, db, _ := setupDirectSubRouter(t, masterToken, true, true)

	// Fetch initial config
	rec1 := doRequest(router, http.MethodGet, "/clash?token="+masterToken, "", "")
	if rec1.Code != http.StatusOK {
		t.Fatalf("initial GET failed: %d", rec1.Code)
	}
	initialBody := rec1.Body.String()
	initialEtag := rec1.Header().Get("ETag")

	// Now modify node in SQLite directly (simulating real-time update)
	now := time.Now().UTC()
	_, err := db.Exec(`
		UPDATE nodes SET display_name = 'Renamed Direct Node', updated_at = ? WHERE server = '1.2.3.4';
	`, now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to update node: %v", err)
	}

	// Fetch again: MUST reflect updated node immediately without any manual publication step!
	rec2 := doRequest(router, http.MethodGet, "/clash?token="+masterToken, "", "")
	if rec2.Code != http.StatusOK {
		t.Fatalf("second GET failed: %d", rec2.Code)
	}
	newBody := rec2.Body.String()
	newEtag := rec2.Header().Get("ETag")

	if !strings.Contains(newBody, "Renamed Direct Node") {
		t.Fatalf("updated node name not found in dynamic output: %s", newBody)
	}
	if newEtag == initialEtag {
		t.Fatalf("expected new ETag after update, got same %s", newEtag)
	}
	if newBody == initialBody {
		t.Fatal("expected content to change after node update")
	}
}

func TestDirectSubscriptionEndpoints_PrimarySubscriptionUserinfo(t *testing.T) {
	masterToken := "settings-secret-token-userinfo"
	router, db, _ := setupDirectSubRouter(t, masterToken, false, false)

	// 1. Initial state: sub-direct-1 is not primary -> no Subscription-Userinfo header
	rec := doRequest(router, http.MethodGet, "/clash", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if h := rec.Header().Get("Subscription-Userinfo"); h != "" {
		t.Fatalf("expected no Subscription-Userinfo header when no primary sub, got %q", h)
	}

	// 2. Mark sub-direct-1 as primary with userinfo
	sampleUserinfo := "upload=10240; download=20480; total=107374182400; expire=1893456000"
	_, err := db.Exec(`
		UPDATE subscriptions SET is_primary = 1, userinfo = ? WHERE id = 'sub-direct-1';
	`, sampleUserinfo)
	if err != nil {
		t.Fatalf("failed to update subscription: %v", err)
	}

	// 3. Request /clash, /singbox, /surge, /qx -> All should return Subscription-Userinfo header (for both GET and HEAD)!
	for _, path := range []string{"/clash", "/singbox", "/surge", "/qx", "/sub/mihomo"} {
		r := doRequest(router, http.MethodGet, path, "", "")
		if r.Code != http.StatusOK {
			t.Fatalf("GET %s returned %d", path, r.Code)
		}
		if got := r.Header().Get("Subscription-Userinfo"); got != sampleUserinfo {
			t.Errorf("GET %s Subscription-Userinfo = %q, want %q", path, got, sampleUserinfo)
		}

		rHead := doRequest(router, http.MethodHead, path, "", "")
		if rHead.Code != http.StatusOK {
			t.Fatalf("HEAD %s returned %d", path, rHead.Code)
		}
		if got := rHead.Header().Get("Subscription-Userinfo"); got != sampleUserinfo {
			t.Errorf("HEAD %s Subscription-Userinfo = %q, want %q", path, got, sampleUserinfo)
		}
	}

	// 4. Disable sub-direct-1 (enabled = 0) -> header should NOT be present
	_, err = db.Exec(`UPDATE subscriptions SET enabled = 0 WHERE id = 'sub-direct-1';`)
	if err != nil {
		t.Fatalf("failed to disable subscription: %v", err)
	}
	rDisabled := doRequest(router, http.MethodGet, "/clash", "", "")
	if rDisabled.Code == http.StatusOK {
		if got := rDisabled.Header().Get("Subscription-Userinfo"); got != "" {
			t.Errorf("disabled primary sub should not forward userinfo header, got %q", got)
		}
	}

	// 5. Re-enable and verify second subscription taking over primary
	_, _ = db.Exec(`UPDATE subscriptions SET enabled = 1, is_primary = 0 WHERE id = 'sub-direct-1';`)
	now := time.Now().UTC()
	sub2Userinfo := "upload=1; download=2; total=5000; expire=1900000000"
	_, err = db.Exec(`
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, is_primary, userinfo, created_at, updated_at)
		VALUES ('sub-direct-2', 'Sub Direct 2', 'secret://url2', 1, '1', 1, ?, ?, ?);
	`, sub2Userinfo, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to insert sub 2: %v", err)
	}

	rSub2 := doRequest(router, http.MethodGet, "/clash", "", "")
	if rSub2.Code != http.StatusOK {
		t.Fatalf("GET /clash returned %d", rSub2.Code)
	}
	if got := rSub2.Header().Get("Subscription-Userinfo"); got != sub2Userinfo {
		t.Errorf("Subscription-Userinfo = %q, want %q", got, sub2Userinfo)
	}
}

func TestDirectSubscriptionEndpoints_PolicyGroupOrdering(t *testing.T) {
	masterToken := "settings-secret-token-order"
	router, db, _ := setupDirectSubRouter(t, masterToken, false, false)

	now := time.Now().UTC()
	// Add two more groups with explicit positions:
	// Group ZZZ with position 1
	// Group AAA with position 10
	// Group Proxy (existing from setupDirectSubRouter) has position 0
	// Without position sorting, AAA would sort before ZZZ alphabetically.
	// With position sorting: Proxy (pos 0) -> ZZZ (pos 1) -> AAA (pos 10)
	gidZZZ := domain.MustNewUUIDv7()
	gidAAA := domain.MustNewUUIDv7()

	_, err := db.Exec(`
		INSERT INTO node_groups (id, name, group_type, position, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'ZZZ-First-By-Position', 'select', 1, ?, ?, 0, 1),
		       (?, 'AAA-Second-By-Position', 'select', 10, ?, ?, 0, 1);
	`, gidZZZ, now.Format(time.RFC3339), now.Format(time.RFC3339),
		gidAAA, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to insert test groups: %v", err)
	}

	// Link node to both groups
	_, _ = db.Exec(`
		INSERT INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, created_at, updated_at)
		VALUES ('pgs-zzz-1', ?, 'add', 'manual_node', 0, 'node_0123456789abcdef', ?, ?),
		       ('pgs-aaa-1', ?, 'add', 'manual_node', 0, 'node_0123456789abcdef', ?, ?);
	`, gidZZZ, now.Format(time.RFC3339), now.Format(time.RFC3339),
		gidAAA, now.Format(time.RFC3339), now.Format(time.RFC3339))

	rec := doRequest(router, http.MethodGet, "/clash", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /clash returned %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	posZZZ := strings.Index(body, "ZZZ-First-By-Position")
	posAAA := strings.Index(body, "AAA-Second-By-Position")

	if posZZZ == -1 || posAAA == -1 {
		t.Fatalf("groups not found in output: posZZZ=%d posAAA=%d\nBody:\n%s", posZZZ, posAAA, body)
	}

	if posZZZ >= posAAA {
		t.Errorf("expected ZZZ (position 1) to appear before AAA (position 10) in output, got posZZZ=%d, posAAA=%d", posZZZ, posAAA)
	}
}

func TestDirectSubscriptionEndpoints_TargetAdaptation_ExclusionsAndErrors(t *testing.T) {
	router, db, _ := setupDirectSubRouter(t, "", false, false)
	now := time.Now().UTC()

	// Add a VLESS node that is unsupported on Surge / Quantumult X
	vlessID := "node_vless_unsupported_on_surge"
	_, err := db.Exec(`
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'vless', 'Direct VLESS Node', '2.2.2.2', 443, '{"uuid":"01944aa0-0000-7000-8000-000000000001"}', 1, ?, ?, 1);
	`, vlessID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to insert VLESS node: %v", err)
	}
	_, _ = db.Exec(`
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, 'sub-direct-1', 'fetch-direct-1');
	`, vlessID)

	// 1. GET /surge: should project compatible SS node and exclude VLESS
	recSurge := doRequest(router, http.MethodGet, "/surge", "", "")
	if recSurge.Code != http.StatusOK {
		t.Fatalf("expected 200 for /surge with mixed inventory, got %d: %s", recSurge.Code, recSurge.Body.String())
	}
	if excl := recSurge.Header().Get("X-Export-Excluded-Count"); excl != "1" {
		t.Errorf("expected X-Export-Excluded-Count: 1, got %q", excl)
	}
	surgeBody := recSurge.Body.String()
	if !strings.Contains(surgeBody, "Direct SS Node") {
		t.Errorf("expected surge output to contain SS node, got: %s", surgeBody)
	}
	if strings.Contains(surgeBody, "Direct VLESS Node") {
		t.Errorf("expected surge output to exclude VLESS node, got: %s", surgeBody)
	}
	surgeEtag := recSurge.Header().Get("ETag")
	if surgeEtag == "" {
		t.Fatal("missing ETag in surge response")
	}

	// 2. 304 on /surge should also report X-Export-Excluded-Count: 1
	req304 := httptest.NewRequest(http.MethodGet, "/surge", nil)
	req304.Header.Set("If-None-Match", surgeEtag)
	rec304 := httptest.NewRecorder()
	router.ServeHTTP(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", rec304.Code)
	}
	if excl := rec304.Header().Get("X-Export-Excluded-Count"); excl != "1" {
		t.Errorf("expected 304 X-Export-Excluded-Count: 1, got %q", excl)
	}

	// 3. All-excluded nonempty inventory: disable SS node, leaving only VLESS
	_, err = db.Exec(`UPDATE nodes SET active = 0 WHERE logical_id = 'node_0123456789abcdef';`)
	if err != nil {
		t.Fatalf("failed to deactivate SS node: %v", err)
	}

	recAllExcluded := doRequest(router, http.MethodGet, "/surge", "", "")
	if recAllExcluded.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for all-excluded nodes on /surge, got %d: %s", recAllExcluded.Code, recAllExcluded.Body.String())
	}
	if !strings.Contains(recAllExcluded.Body.String(), "no_compatible_nodes") {
		t.Fatalf("expected no_compatible_nodes in error response, got: %s", recAllExcluded.Body.String())
	}

	// 4. Active chain on non-Mihomo: restore SS node, add enabled proxy chain binding
	_, err = db.Exec(`UPDATE nodes SET active = 1 WHERE logical_id = 'node_0123456789abcdef';`)
	if err != nil {
		t.Fatalf("failed to reactivate SS node: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO proxy_chain_bindings (id, target_type, target_id, dialer_type, dialer_id, enabled, sort_order, revision, created_at, updated_at)
		VALUES ('chain-direct-test', 'subscription', 'sub-direct-1', 'manual_node', 'node_0123456789abcdef', 1, 0, 1, ?, ?);
	`, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to insert chain binding: %v", err)
	}

	recChain := doRequest(router, http.MethodGet, "/surge", "", "")
	if recChain.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for active chain on /surge, got %d: %s", recChain.Code, recChain.Body.String())
	}
	if !strings.Contains(recChain.Body.String(), "unsupported_target_capability") {
		t.Fatalf("expected unsupported_target_capability in error response, got: %s", recChain.Body.String())
	}
}
