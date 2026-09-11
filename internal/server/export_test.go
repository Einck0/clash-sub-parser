package server_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
)

func seedExportData(t *testing.T, repos *repository.Repositories) (*domain.Subscription, []*domain.Node) {
	t.Helper()
	ctx := context.Background()

	sub := &domain.Subscription{
		Name:                  "Primary-Sub",
		URL:                   "https://example.com/feed",
		IsPrimary:             true,
		Enabled:               true,
		SubscriptionUserinfo:  "upload=1000; download=2000; total=10000; expire=1800000000",
		ProfileUpdateInterval: "24",
		ProfileWebPageURL:     "https://example.com/user",
	}
	if err := repos.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	nodes := []*domain.Node{
		{
			LogicalID: "node_hk_1",
			Name:      "HK-Node-01",
			Protocol:  domain.ProtocolShadowsocks,
			Server:    "hk.example.com",
			Port:      8388,
			NormalizedPayload: map[string]any{
				"cipher":           "aes-256-gcm",
				"password":         "secret123",
				"admin_token":      "leak-token-123",
				"management_token": "leak-mgmt-456",
			},
			LifecycleState: domain.LifecycleActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		{
			LogicalID: "node_us_1",
			Name:      "US-Node-02",
			Protocol:  domain.ProtocolVMess,
			Server:    "us.example.com",
			Port:      443,
			NormalizedPayload: map[string]any{
				"uuid":             "00000000-0000-4000-8000-000000000001",
				"alterId":          0,
				"cipher":           "auto",
				"tls":              true,
				"network":          "ws",
				"ws-opts":          map[string]any{"path": "/ws"},
				"management_token": "leak-mgmt-789",
			},
			LifecycleState: domain.LifecycleActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		{
			LogicalID: "node_jp_1",
			Name:      "JP-Node-03",
			Protocol:  domain.ProtocolVLESS,
			Server:    "jp.example.com",
			Port:      443,
			NormalizedPayload: map[string]any{
				"uuid": "00000000-0000-4000-8000-000000000002",
				"tls":  true,
				"reality-opts": map[string]any{
					"public-key": "test-pubkey",
					"short-id":   "815458e4",
				},
			},
			LifecycleState: domain.LifecycleInactive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
	}

	for _, n := range nodes {
		if err := repos.Nodes.Create(ctx, n); err != nil {
			t.Fatalf("failed to create node: %v", err)
		}
	}

	sub.RawNodes = nodes
	_ = repos.Subscriptions.Update(ctx, sub)

	group := &domain.NodeGroup{
		Name:      "PROXIES",
		Kind:      domain.GroupKindManual,
		GroupType: domain.GroupTypeSelect,
		SortOrder: 1,
		ResolvedNodeNames: []string{
			"HK-Node-01",
			"US-Node-02",
		},
	}
	if err := repos.NodeGroups.Create(ctx, group); err != nil {
		t.Fatalf("failed to create node group: %v", err)
	}

	rule := &domain.Rule{
		Type:      domain.RuleTypeDomainSuffix,
		Value:     "google.com",
		Proxy:     "PROXIES",
		SortOrder: 1,
		Enabled:   true,
	}
	if err := repos.Rules.Create(ctx, rule); err != nil {
		t.Fatalf("failed to create rule: %v", err)
	}

	return sub, nodes
}

func TestClientExport_RootEndpoints(t *testing.T) {
	srv, repos, _ := setupTestServer(t)
	seedExportData(t, repos)

	t.Run("GET /clash", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/clash", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-yaml") {
			t.Errorf("expected application/x-yaml, got %s", ct)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="config.yaml"`) {
			t.Errorf("expected filename config.yaml, got %s", cd)
		}
		if ui := rec.Header().Get("Subscription-Userinfo"); ui != "upload=1000; download=2000; total=10000; expire=1800000000" {
			t.Errorf("expected Subscription-Userinfo header, got %s", ui)
		}
		if pui := rec.Header().Get("Profile-Update-Interval"); pui != "24" {
			t.Errorf("expected Profile-Update-Interval header, got %s", pui)
		}
		if pwp := rec.Header().Get("Profile-Web-Page-Url"); pwp != "https://example.com/user" {
			t.Errorf("expected Profile-Web-Page-Url header, got %s", pwp)
		}
		if etag := rec.Header().Get("ETag"); etag == "" {
			t.Errorf("expected ETag header, got empty")
		}

		body := rec.Body.String()
		if !strings.Contains(body, "HK-Node-01") {
			t.Errorf("expected HK-Node-01 in yaml, got:\n%s", body)
		}
		if strings.Contains(body, "leak-token-123") || strings.Contains(body, "leak-mgmt") {
			t.Errorf("security leak: internal tokens present in export:\n%s", body)
		}

		var parsed map[string]any
		if err := yaml.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
			t.Errorf("yaml parse error: %v", err)
		}
	})

	t.Run("GET /mihomo", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/mihomo", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-yaml") {
			t.Errorf("expected application/x-yaml, got %s", ct)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "mihomo.yaml") && !strings.Contains(cd, "config.yaml") {
			t.Errorf("expected mihomo filename, got %s", cd)
		}
	})

	t.Run("GET /sing-box", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/sing-box", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("expected application/json, got %s", ct)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="config.json"`) {
			t.Errorf("expected filename config.json, got %s", cd)
		}
		var sbData map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &sbData); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if _, ok := sbData["outbounds"]; !ok {
			t.Errorf("sing-box config missing outbounds")
		}
	})

	t.Run("GET /surge", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/surge", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="surge.conf"`) {
			t.Errorf("expected filename surge.conf, got %s", cd)
		}
		if !strings.Contains(rec.Body.String(), "[Proxy]") {
			t.Errorf("expected [Proxy] section in surge config")
		}
	})

	t.Run("GET /loon", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/loon", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="loon.conf"`) {
			t.Errorf("expected filename loon.conf, got %s", cd)
		}
		if !strings.Contains(rec.Body.String(), "[Proxy]") {
			t.Errorf("expected [Proxy] section in loon config")
		}
	})

	t.Run("GET /stash", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/stash", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="stash.yaml"`) {
			t.Errorf("expected filename stash.yaml, got %s", cd)
		}
	})

	t.Run("GET /shadowrocket", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/shadowrocket", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="sub.txt"`) {
			t.Errorf("expected filename sub.txt, got %s", cd)
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rec.Body.String()))
		if err != nil {
			t.Fatalf("shadowrocket body is not valid base64: %v", err)
		}
		if !strings.Contains(string(decoded), "ss://") {
			t.Errorf("expected ss:// in decoded shadowrocket content: %s", string(decoded))
		}
	})

	t.Run("GET /yaml", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/yaml", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "application/x-yaml") {
			t.Errorf("expected application/x-yaml")
		}
	})

	t.Run("GET /script strictly 404", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/script", nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for deprecated /script, got %d", rec.Code)
		}
	})
}

func TestClientExport_ETag304Caching(t *testing.T) {
	srv, repos, _ := setupTestServer(t)
	seedExportData(t, repos)

	// First request: gets 200 with ETag
	rec1 := doRequest(srv.Router(), "GET", "/clash", nil, nil)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}
	etag := rec1.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("expected non-empty ETag header")
	}

	// Second request with matching If-None-Match: must get 304 Not Modified and empty body
	rec2 := doRequest(srv.Router(), "GET", "/clash", nil, map[string]string{
		"If-None-Match": etag,
	})
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("expected 304 Not Modified, got %d", rec2.Code)
	}
	if rec2.Body.Len() > 0 {
		t.Errorf("expected empty body for 304 response, got %d bytes", rec2.Body.Len())
	}
	if rec2.Header().Get("ETag") != etag {
		t.Errorf("expected ETag header %s on 304, got %s", etag, rec2.Header().Get("ETag"))
	}

	// Third request with mismatched If-None-Match: gets 200 OK
	rec3 := doRequest(srv.Router(), "GET", "/clash", nil, map[string]string{
		"If-None-Match": `"mismatched-etag-12345"`,
	})
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on mismatched etag, got %d", rec3.Code)
	}
	if rec3.Body.Len() == 0 {
		t.Errorf("expected non-empty body on mismatched etag")
	}
}

func TestClientExport_APIGenerateRoutes(t *testing.T) {
	srv, repos, _ := setupTestServer(t)
	sub, _ := seedExportData(t, repos)

	t.Run("GET /api/generate?target=mihomo", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/generate?target=mihomo", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "application/x-yaml") {
			t.Errorf("expected application/x-yaml")
		}
	})

	t.Run("GET and PATCH /api/generate/settings", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/generate/settings", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var cfg domain.GenerateConfig
		if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
			t.Fatalf("failed to unmarshal config: %v", err)
		}

		patchBody := map[string]any{
			"exclude_node_proxies": false,
		}
		recPatch := doRequest(srv.Router(), "PATCH", "/api/generate/settings", patchBody, nil)
		if recPatch.Code != http.StatusOK {
			t.Fatalf("expected 200 on patch, got %d", recPatch.Code)
		}
	})

	t.Run("GET /api/generate/quick-export", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/generate/quick-export", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var qeResp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &qeResp); err != nil {
			t.Fatalf("failed to unmarshal quick-export: %v", err)
		}
		targets, ok := qeResp["targets"].(map[string]any)
		if !ok {
			t.Fatalf("missing targets map in quick-export")
		}
		for _, key := range []string{"clash", "mihomo", "stash", "sing-box", "shadowrocket"} {
			tInfo, exists := targets[key].(map[string]any)
			if !exists {
				t.Errorf("target %s missing from quick-export", key)
				continue
			}
			scheme, _ := tInfo["scheme_url"].(string)
			if scheme == "" {
				t.Errorf("target %s missing scheme_url", key)
			}
			qr, _ := tInfo["qrcode_payload"].(string)
			if qr == "" {
				t.Errorf("target %s missing qrcode_payload", key)
			}
		}

		// Single subscription quick-export
		subRec := doRequest(srv.Router(), "GET", "/api/generate/quick-export?subscription_id=1", nil, nil)
		if subRec.Code != http.StatusOK {
			t.Fatalf("expected 200 for sub quick export, got %d", subRec.Code)
		}
	})

	t.Run("GET /api/generate/qrcode", func(t *testing.T) {
		// PNG
		recPNG := doRequest(srv.Router(), "GET", "/api/generate/qrcode?url=https://example.com/test", nil, nil)
		if recPNG.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recPNG.Code)
		}
		if ct := recPNG.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("expected image/png, got %s", ct)
		}
		pngMagic := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
		if !bytes.HasPrefix(recPNG.Body.Bytes(), pngMagic) {
			t.Errorf("invalid PNG header")
		}

		// SVG
		recSVG := doRequest(srv.Router(), "GET", "/api/generate/qrcode?url=https://example.com/test&format=svg", nil, nil)
		if recSVG.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recSVG.Code)
		}
		if ct := recSVG.Header().Get("Content-Type"); ct != "image/svg+xml" {
			t.Errorf("expected image/svg+xml, got %s", ct)
		}
		if !strings.HasPrefix(recSVG.Body.String(), "<svg") {
			t.Errorf("invalid SVG response")
		}

		// Missing param
		recMissing := doRequest(srv.Router(), "GET", "/api/generate/qrcode", nil, nil)
		if recMissing.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing url/text, got %d", recMissing.Code)
		}
	})

	t.Run("GET /api/generate/script strictly 404", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/generate/script", nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("GET /api/generate/{target}/current attachment", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/generate/clash/current", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
			t.Errorf("expected attachment Content-Disposition, got %s", cd)
		}
	})

	t.Run("GET /api/generate/subscription/{id}", func(t *testing.T) {
		targetURL := "/api/generate/subscription/" + string(rune('0'+sub.ID)) + "?target=sing-box"
		rec := doRequest(srv.Router(), "GET", targetURL, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
			t.Errorf("expected application/json, got %s", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("Filter and include_unchecked query parameters", func(t *testing.T) {
		// filter by HK keyword
		recHK := doRequest(srv.Router(), "GET", "/clash?filter=HK", nil, nil)
		if recHK.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recHK.Code)
		}
		if !strings.Contains(recHK.Body.String(), "HK-Node-01") {
			t.Errorf("expected HK-Node-01")
		}
		if strings.Contains(recHK.Body.String(), "US-Node-02") {
			t.Errorf("filter=HK should exclude US-Node-02")
		}
	})
}
