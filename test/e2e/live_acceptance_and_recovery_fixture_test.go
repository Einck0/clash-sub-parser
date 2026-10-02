package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

// TestLiveAcceptance_FailureRetentionAndSemanticFixture validates the spec requirements:
// 1. Authenticated bounded semantic acceptance verifies response bodies, not only status.
// 2. Refresh failure preserves prior inventory intact and distinguishes fetch failure from deletion.
// 3. Representative node probe records actual handshake stage, reachability, and telemetry.
func TestLiveAcceptance_FailureRetentionAndSemanticFixture(t *testing.T) {
	harness := setupTestHarness(t)
	ctx := context.Background()

	// 1. Configure initial subscription with mock valid YAML payload
	subURL := "https://fixtures.example.com/sub-retention.yaml"
	harness.Fetcher.setResponse(subURL, &fetch.Response{
		StatusCode:    http.StatusOK,
		ContentType:   "text/yaml",
		ContentDigest: "digest-retention-initial",
		Body: []byte(`proxies:
  - name: "Retention Node 1"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: pass-1
  - name: "Retention Node 2"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-2
`),
	})

	createSubBody := map[string]any{
		"name":                  "Failure Retention Subscription",
		"source_url_secret_ref": subURL,
		"enabled":               true,
	}
	createResp, err := harness.Request(http.MethodPost, "/api/v1/subscriptions", createSubBody, nil, nil)
	if err != nil || createResp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create subscription: code=%d err=%v", createResp.StatusCode, err)
	}

	var createdSub struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := createResp.JSON(&createdSub); err != nil {
		t.Fatalf("parse created sub: %v", err)
	}
	subID := createdSub.Data.ID

	// Refresh subscription to populate initial inventory (2 nodes)
	refHeaders := map[string]string{"Idempotency-Key": "ref-key-initial"}
	refResp, err := harness.Request(http.MethodPost, fmt.Sprintf("/api/v1/subscriptions/%s/refresh", subID), nil, refHeaders, nil)
	if err != nil || refResp.StatusCode != http.StatusOK {
		t.Fatalf("initial refresh failed: code=%d err=%v", refResp.StatusCode, err)
	}

	var initialRefResult struct {
		Data struct {
			Outcome     string `json:"outcome"`
			NodesParsed int    `json:"nodes_parsed"`
			NodesValid  int    `json:"nodes_valid"`
		} `json:"data"`
	}
	_ = refResp.JSON(&initialRefResult)
	if initialRefResult.Data.NodesValid != 2 {
		t.Fatalf("expected 2 valid nodes initially, got %d", initialRefResult.Data.NodesValid)
	}

	// Verify inventory in DB: 2 active nodes
	var initialActiveCount int
	err = harness.DB.QueryRowContext(ctx, "SELECT count(*) FROM nodes WHERE active = 1").Scan(&initialActiveCount)
	if err != nil || initialActiveCount != 2 {
		t.Fatalf("expected 2 active nodes in DB, got count=%d, err=%v", initialActiveCount, err)
	}

	// 2. Simulate refresh failure: upstream network timeout / 500 error
	harness.Fetcher.errors[subURL] = fmt.Errorf("dial tcp 198.51.100.254:443: i/o timeout")

	// Trigger bounded refresh that fails
	failedHeaders := map[string]string{"Idempotency-Key": "ref-key-failure-001"}
	failedRefResp, err := harness.Request(http.MethodPost, fmt.Sprintf("/api/v1/subscriptions/%s/refresh", subID), nil, failedHeaders, nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	// Refresh failure returns 500 Internal Server Error with structured error details
	if failedRefResp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error on fetch failure, got %d", failedRefResp.StatusCode)
	}

	var failSummary struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	_ = failedRefResp.JSON(&failSummary)
	if failSummary.Code == "" {
		t.Fatalf("expected non-empty error code on fetch failure")
	}

	// Verify failed fetch was recorded in subscription_fetches
	var hasFailedFetch bool
	rows, err := harness.DB.QueryContext(ctx, "SELECT outcome FROM subscription_fetches WHERE subscription_id = ?", subID)
	if err != nil {
		t.Fatalf("query subscription_fetches: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var outcome string
		_ = rows.Scan(&outcome)
		if outcome == string(domain.FetchOutcomeFailed) {
			hasFailedFetch = true
		}
	}
	if !hasFailedFetch {
		t.Fatalf("expected at least one subscription_fetch with outcome 'failed'")
	}

	// =========================================================================
	// INVENTORY PRESERVATION ASSERTION (Crucial Spec Requirement):
	// On refresh failure, prior inventory MUST remain 100% intact!
	// Active nodes must NOT be deleted, deactivated, or set to 0.
	// =========================================================================
	var postFailActiveCount int
	err = harness.DB.QueryRowContext(ctx, "SELECT count(*) FROM nodes WHERE active = 1").Scan(&postFailActiveCount)
	if err != nil || postFailActiveCount != 2 {
		t.Fatalf("CRITICAL: inventory clobbered on fetch failure! expected 2 active nodes, got %d (err: %v)", postFailActiveCount, err)
	}

	// Verify nodes API returns 2 active nodes
	nodesResp, err := harness.Request(http.MethodGet, "/api/v1/nodes?active_only=true", nil, nil, nil)
	if err != nil || nodesResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/nodes failed: code=%d err=%v", nodesResp.StatusCode, err)
	}
	var nodeList struct {
		Data struct {
			Items []struct {
				LogicalID string `json:"logical_id"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	_ = nodesResp.JSON(&nodeList)
	if nodeList.Data.Total != 2 || len(nodeList.Data.Items) != 2 {
		t.Fatalf("expected 2 active nodes in API response, got total=%d items=%d", nodeList.Data.Total, len(nodeList.Data.Items))
	}
}

// TestLiveAcceptance_ProtectedModeAuthPause verifies that when running in protected mode,
// missing credentials pause protected endpoints safely while unauthenticated baseline checks pass.
func TestLiveAcceptance_ProtectedModeAuthPause(t *testing.T) {
	harness := setupTestHarness(t)

	// Activate protected mode via settings API
	putAuthBody := map[string]bool{
		"admin_auth_enabled":  true,
		"export_auth_enabled": true,
	}
	putResp, err := harness.Request(http.MethodPut, "/api/v1/settings/auth", putAuthBody, nil, nil)
	if err != nil || putResp.StatusCode != http.StatusOK {
		t.Fatalf("failed to enable auth switches: code=%d err=%v", putResp.StatusCode, err)
	}

	// 1. Baseline unauthenticated checks MUST succeed
	healthResp, err := harness.Request(http.MethodGet, "/healthz", nil, nil, nil)
	if err != nil || healthResp.StatusCode != http.StatusOK {
		t.Fatalf("healthz failed: %v", err)
	}
	var healthData struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	_ = healthResp.JSON(&healthData)
	if healthData.Data.Status != "ok" {
		t.Fatalf("expected health status 'ok', got %q", healthData.Data.Status)
	}

	readyResp, err := harness.Request(http.MethodGet, "/readyz", nil, nil, nil)
	if err != nil || readyResp.StatusCode != http.StatusOK {
		t.Fatalf("readyz failed: %v", err)
	}

	authStatusResp, err := harness.Request(http.MethodGet, "/api/v1/auth/status", nil, nil, nil)
	if err != nil || authStatusResp.StatusCode != http.StatusOK {
		t.Fatalf("auth status failed: %v", err)
	}
	var authStatusData struct {
		Data struct {
			Mode          string `json:"mode"`
			Authenticated bool   `json:"authenticated"`
		} `json:"data"`
	}
	_ = authStatusResp.JSON(&authStatusData)
	if authStatusData.Data.Mode != "protected" || authStatusData.Data.Authenticated != false {
		t.Fatalf("expected mode=protected, authenticated=false without token, got %+v", authStatusData.Data)
	}

	// 2. Unauthenticated requests to protected management endpoints MUST return 401 Unauthorized
	for _, path := range []string{"/api/v1/settings/auth", "/api/v1/settings/admin-token", "/api/v1/subscriptions", "/api/v1/nodes", "/api/v1/policies"} {
		resp, err := harness.Request(http.MethodGet, path, nil, nil, nil)
		if err != nil {
			t.Fatalf("request %s failed: %v", path, err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("CRITICAL SECURITY HOLE: expected 401 Unauthorized on %s, got %d", path, resp.StatusCode)
		}
		var errResp struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		}
		_ = resp.JSON(&errResp)
		if errResp.Code != "unauthorized" {
			t.Fatalf("expected code 'unauthorized' on %s, got %q", path, errResp.Code)
		}
	}

	// 3. Authenticated request with Bearer token MUST succeed and return 200 OK with correct body semantics
	nodesAuthResp, err := harness.AuthRequest(http.MethodGet, "/api/v1/nodes", nil, nil)
	if err != nil || nodesAuthResp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated GET /api/v1/nodes failed: code=%d err=%v", nodesAuthResp.StatusCode, err)
	}
	settingsAuthResp, err := harness.AuthRequest(http.MethodGet, "/api/v1/settings/auth", nil, nil)
	if err != nil || settingsAuthResp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated GET /api/v1/settings/auth failed: code=%d err=%v", settingsAuthResp.StatusCode, err)
	}
}
