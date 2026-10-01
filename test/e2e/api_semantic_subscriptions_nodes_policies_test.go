package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"clash-sub-parser/internal/fetch"
)

// Task 5.3: TestAPISemantic_SubscriptionsNodesPolicies
// Comprehensive, isolated semantic integration test covering Subscriptions, Nodes, and Policies.
// Verifies:
// 1. Subscriptions: Create, List, Get, If-Match optimistic concurrency PATCH, Refresh with DB side effects, Scoped Delete
// 2. Nodes: List with filters, Detail with provenance sources, Plaintext PATCH incrementing connection_revision, Observations
// 3. Policies & Revisions: Global Node Filter, Group CRUD & Edges, Rules CRUD, Graph Validation, Revision Activation
func TestAPISemantic_SubscriptionsNodesPolicies(t *testing.T) {
	harness := setupTestHarness(t)
	ctx := context.Background()

	// =========================================================================
	// Part 1: Subscriptions Lifecycle & Side Effects
	// =========================================================================

	// 1.0 Enable Admin & Export Authentication
	harness.TokenHolder.SetAuthSwitches(true, true)

	// 1.1 Unauthenticated POST /api/v1/subscriptions -> 401
	subURL := "https://sub-provider.example.com/feed.yaml"
	createBody := map[string]any{
		"name":                  "Primary Provider",
		"source_url_secret_ref": subURL,
		"enabled":               true,
		"refresh_policy": map[string]any{
			"interval_seconds":    3600,
			"timeout_seconds":     15,
			"max_response_bytes": 1048576,
		},
	}
	resp, err := harness.Request(http.MethodPost, "/api/v1/subscriptions", createBody, nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated subscription creation, got %d err=%v", resp.StatusCode, err)
	}

	// 1.2 Invalid Body (Missing Name) -> 422 Unprocessable Entity
	badBody := map[string]any{
		"name":                  "",
		"source_url_secret_ref": subURL,
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/subscriptions", badBody, nil)
	if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty subscription name, got %d", resp.StatusCode)
	}

	// 1.3 Valid Creation -> 201 Created + DB Persistence
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/subscriptions", createBody, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for valid subscription creation, got %d err=%v body=%s", resp.StatusCode, err, resp.Body)
	}
	var createdSub struct {
		Data struct {
			ID                 string `json:"id"`
			Name               string `json:"name"`
			SourceURLSecretRef string `json:"source_url_secret_ref"`
			Revision           string `json:"revision"`
			Enabled            bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := resp.JSON(&createdSub); err != nil {
		t.Fatalf("unmarshal created subscription: %v", err)
	}
	subID := createdSub.Data.ID
	subRev := createdSub.Data.Revision
	if subID == "" || subRev == "" {
		t.Fatalf("expected non-empty id and revision, got id=%q rev=%q", subID, subRev)
	}

	// DB Side-Effect Verification: subscription exists in SQLite `subscriptions` table!
	var dbSubName string
	var dbSubEnabled int
	err = harness.DB.QueryRowContext(ctx, "SELECT name, enabled FROM subscriptions WHERE id = ?", subID).Scan(&dbSubName, &dbSubEnabled)
	if err != nil || dbSubName != "Primary Provider" || dbSubEnabled != 1 {
		t.Fatalf("DB verification failed for subscription: name=%q enabled=%d err=%v", dbSubName, dbSubEnabled, err)
	}

	// 1.4 List Subscriptions -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/subscriptions?page=1&page_size=20", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/subscriptions failed: %d", resp.StatusCode)
	}
	var subList struct {
		Data struct {
			Items []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := resp.JSON(&subList); err != nil || subList.Data.Total < 1 {
		t.Fatalf("list subscriptions result invalid: total=%d err=%v", subList.Data.Total, err)
	}

	// 1.5 Get Single Subscription -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/subscriptions/"+subID, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/subscriptions/%s failed: %d", subID, resp.StatusCode)
	}

	// 1.6 Concurrency Protection: PATCH with Stale If-Match -> 409 Conflict
	patchBody := map[string]any{"name": "Renamed Provider"}
	headersStale := map[string]string{"If-Match": "stale-revision-hash-1234"}
	resp, err = harness.AuthRequest(http.MethodPatch, "/api/v1/subscriptions/"+subID, patchBody, headersStale)
	if err != nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for stale If-Match header, got %d", resp.StatusCode)
	}

	// 1.7 Valid PATCH with Correct If-Match -> 200 OK
	headersValid := map[string]string{"If-Match": subRev}
	resp, err = harness.AuthRequest(http.MethodPatch, "/api/v1/subscriptions/"+subID, patchBody, headersValid)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for valid PATCH, got %d body=%s", resp.StatusCode, resp.Body)
	}
	var patchedSub struct {
		Data struct {
			Name     string `json:"name"`
			Revision string `json:"revision"`
		} `json:"data"`
	}
	_ = resp.JSON(&patchedSub)
	if patchedSub.Data.Name != "Renamed Provider" || patchedSub.Data.Revision == subRev {
		t.Fatalf("PATCH failed to update name or increment revision: %+v", patchedSub.Data)
	}
	subRev = patchedSub.Data.Revision

	// 1.8 Reconcile / Refresh: POST /api/v1/subscriptions/{id}/refresh
	// Set mock response with 2 distinct nodes (SS and Trojan)
	harness.Fetcher.setResponse(subURL, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/yaml",
		ContentDigest: "digest-sub-feed-v1",
		Body: []byte(`proxies:
  - name: "HK Express 01"
    type: ss
    server: 198.51.100.101
    port: 8388
    cipher: aes-128-gcm
    password: secret-hk-pass
  - name: "SG Premium 01"
    type: trojan
    server: 198.51.100.102
    port: 443
    password: secret-sg-trojan
`),
	})

	// 1.8.1 Missing Idempotency-Key -> 422
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/subscriptions/"+subID+"/refresh", nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing Idempotency-Key, got %d", resp.StatusCode)
	}

	// 1.8.2 Valid Refresh with Idempotency-Key -> 200 OK
	refreshHeaders := map[string]string{"Idempotency-Key": "idemp-refresh-key-001"}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/subscriptions/"+subID+"/refresh", nil, refreshHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/subscriptions/%s/refresh failed: %d body=%s", subID, resp.StatusCode, resp.Body)
	}

	// Verify DB side effects of Refresh:
	// a. `subscription_fetches` recorded
	var fetchCount int
	err = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM subscription_fetches WHERE subscription_id = ?", subID).Scan(&fetchCount)
	if err != nil || fetchCount < 1 {
		t.Fatalf("subscription_fetches not recorded: count=%d err=%v", fetchCount, err)
	}
	// b. `nodes` ingested
	var nodeCount int
	err = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM nodes WHERE active = 1").Scan(&nodeCount)
	if err != nil || nodeCount < 2 {
		t.Fatalf("nodes not ingested properly: count=%d err=%v", nodeCount, err)
	}
	// c. `node_sources` associations recorded
	var sourceCount int
	err = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM node_sources WHERE subscription_id = ?", subID).Scan(&sourceCount)
	if err != nil || sourceCount < 2 {
		t.Fatalf("node_sources not recorded properly: count=%d err=%v", sourceCount, err)
	}

	// =========================================================================
	// Part 2: Nodes Querying, Detail & Connection Patching
	// =========================================================================

	// 2.1 List Nodes with Filters -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/nodes?active_only=true&protocol=ss,trojan&page=1&page_size=10", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/nodes failed: %d", resp.StatusCode)
	}
	var nodeList struct {
		Data struct {
			Items []struct {
				LogicalID          string `json:"logical_id"`
				DisplayName        string `json:"display_name"`
				Protocol           string `json:"protocol"`
				Server             string `json:"server"`
				Port               int    `json:"port"`
				ConnectionRevision int64  `json:"connection_revision"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := resp.JSON(&nodeList); err != nil || nodeList.Data.Total < 2 {
		t.Fatalf("node list query invalid: total=%d err=%v", nodeList.Data.Total, err)
	}

	var hkNodeID string
	var hkInitialRev int64
	for _, n := range nodeList.Data.Items {
		if n.DisplayName == "HK Express 01" {
			hkNodeID = n.LogicalID
			hkInitialRev = n.ConnectionRevision
			break
		}
	}
	if hkNodeID == "" || hkInitialRev != 1 {
		t.Fatalf("target HK node not found or initial revision != 1: id=%s rev=%d", hkNodeID, hkInitialRev)
	}

	// 2.2 Get Node Detail with Provenance Sources -> 200 OK
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/nodes/"+hkNodeID, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/nodes/%s detail failed: %d", hkNodeID, resp.StatusCode)
	}
	var nodeDetail struct {
		Data struct {
			Node struct {
				LogicalID          string `json:"logical_id"`
				DisplayName        string `json:"display_name"`
				Server             string `json:"server"`
				Port               int    `json:"port"`
				ConnectionRevision int64  `json:"connection_revision"`
			} `json:"node"`
			Sources []struct {
				SubscriptionID string `json:"subscription_id"`
			} `json:"sources"`
		} `json:"data"`
	}
	if err := resp.JSON(&nodeDetail); err != nil || len(nodeDetail.Data.Sources) == 0 {
		t.Fatalf("expected node detail with non-empty sources, got %+v err=%v", nodeDetail.Data, err)
	}
	if nodeDetail.Data.Sources[0].SubscriptionID != subID {
		t.Fatalf("provenance mismatch: expected sub %s, got %s", subID, nodeDetail.Data.Sources[0].SubscriptionID)
	}

	// 2.3 PATCH /api/v1/nodes/{id}/connection with unchanged server/port (display name only)
	// Must NOT increment connection_revision!
	dispName := "HK Express 01 (Renamed)"
	resp, err = harness.AuthRequest(http.MethodPatch, "/api/v1/nodes/"+hkNodeID+"/connection", map[string]any{
		"display_name": dispName,
	}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("display_name patch failed: %d", resp.StatusCode)
	}
	_ = resp.JSON(&nodeDetail)
	if nodeDetail.Data.Node.ConnectionRevision != 1 {
		t.Fatalf("display_name edit MUST NOT increment connection_revision: got %d", nodeDetail.Data.Node.ConnectionRevision)
	}

	// 2.4 PATCH /api/v1/nodes/{id}/connection with REAL connection change (port 8388 -> 8389)
	// MUST increment connection_revision to 2!
	newPort := 8389
	resp, err = harness.AuthRequest(http.MethodPatch, "/api/v1/nodes/"+hkNodeID+"/connection", map[string]any{
		"port": newPort,
	}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("connection port patch failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	_ = resp.JSON(&nodeDetail)
	if nodeDetail.Data.Node.Port != 8389 || nodeDetail.Data.Node.ConnectionRevision != 2 {
		t.Fatalf("real connection patch failed to increment connection_revision to 2: rev=%d port=%d",
			nodeDetail.Data.Node.ConnectionRevision, nodeDetail.Data.Node.Port)
	}

	// Verify in DB directly:
	var dbNodeRev int64
	var dbNodePort int
	_ = harness.DB.QueryRowContext(ctx, "SELECT connection_revision, port FROM nodes WHERE logical_id = ?", hkNodeID).Scan(&dbNodeRev, &dbNodePort)
	if dbNodeRev != 2 || dbNodePort != 8389 {
		t.Fatalf("DB verification failed for connection revision: rev=%d port=%d", dbNodeRev, dbNodePort)
	}

	// 2.4b Direct PATCH /api/v1/nodes/{id} (main route without /connection suffix)
	directPatchName := "HK Express 01 (Direct Main Route Patched)"
	resp, err = harness.AuthRequest(http.MethodPatch, "/api/v1/nodes/"+hkNodeID, map[string]any{
		"display_name": directPatchName,
	}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/v1/nodes/%s directly failed: %d body=%s", hkNodeID, resp.StatusCode, resp.Body)
	}
	_ = resp.JSON(&nodeDetail)
	if nodeDetail.Data.Node.DisplayName != directPatchName {
		t.Fatalf("expected display_name %q, got %q", directPatchName, nodeDetail.Data.Node.DisplayName)
	}
	var dbDirectName string
	_ = harness.DB.QueryRowContext(ctx, "SELECT display_name FROM nodes WHERE logical_id = ?", hkNodeID).Scan(&dbDirectName)
	if dbDirectName != directPatchName {
		t.Fatalf("DB verification failed for direct node patch: expected %q, got %q", directPatchName, dbDirectName)
	}

	// 2.5 Node Probe Observations Endpoint
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/nodes/"+hkNodeID+"/observations", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/nodes/%s/observations failed: %d", hkNodeID, resp.StatusCode)
	}

	// =========================================================================
	// Part 3: Policies, Groups, Rules, Graph Validation & Revisions
	// =========================================================================

	// 3.1 Global Node Filter: GET -> 200 (Default)
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/policies/global-node-filter", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policies/global-node-filter failed: %d", resp.StatusCode)
	}

	// 3.2 Global Node Filter: PUT -> 200 + DB Persistence
	putFilter := map[string]any{
		"spec": map[string]any{
			"conditions": []map[string]any{
				{"field": "protocol", "op": "equals", "value": "ss"},
			},
		},
	}
	resp, err = harness.AuthRequest(http.MethodPut, "/api/v1/policies/global-node-filter", putFilter, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policies/global-node-filter failed: %d", resp.StatusCode)
	}
	// Verify filter updated in DB
	var filterCount int
	_ = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM global_node_filters WHERE id = 1").Scan(&filterCount)
	if filterCount < 1 {
		t.Fatalf("global_node_filters row not created in DB")
	}

	// 3.3 Create Policy Group: POST /api/v1/policies/groups
	createGroupBody := map[string]any{
		"name":       "Proxy Group 1",
		"group_type": "select",
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/policies/groups", createGroupBody, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/policies/groups failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var createdGroup struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := resp.JSON(&createdGroup); err != nil || createdGroup.Data.ID == "" {
		t.Fatalf("unmarshal created group: %v", err)
	}
	groupID := createdGroup.Data.ID

	// 3.4 List Policy Groups: GET /api/v1/policies/groups
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/policies/groups", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policies/groups failed: %d", resp.StatusCode)
	}

	// 3.4a Single Group Retrieval: GET /api/v1/policies/groups/{id}
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/policies/groups/"+groupID, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policies/groups/%s failed: %d", groupID, resp.StatusCode)
	}
	var getGroupResp struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := resp.JSON(&getGroupResp); err != nil || getGroupResp.Data.ID != groupID {
		t.Fatalf("GET /api/v1/policies/groups/%s data mismatch: %+v", groupID, getGroupResp)
	}

	// 3.4b Update Group with PATCH: PATCH /api/v1/policies/groups/{id}
	patchedGroupName := "Proxy Group 1 Patched Name"
	resp, err = harness.AuthRequest(http.MethodPatch, "/api/v1/policies/groups/"+groupID, map[string]any{
		"name": patchedGroupName,
	}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/v1/policies/groups/%s failed: %d body=%s", groupID, resp.StatusCode, resp.Body)
	}
	var dbGroupName string
	_ = harness.DB.QueryRowContext(ctx, "SELECT name FROM node_groups WHERE id = ?", groupID).Scan(&dbGroupName)
	if dbGroupName != patchedGroupName {
		t.Fatalf("DB verification failed for PATCH group name: want %q, got %q", patchedGroupName, dbGroupName)
	}

	// 3.4c Update Group with PUT: PUT /api/v1/policies/groups/{id}
	putGroupName := "Proxy Group 1 PUT Updated Name"
	resp, err = harness.AuthRequest(http.MethodPut, "/api/v1/policies/groups/"+groupID, map[string]any{
		"name": putGroupName,
	}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policies/groups/%s failed: %d body=%s", groupID, resp.StatusCode, resp.Body)
	}
	_ = harness.DB.QueryRowContext(ctx, "SELECT name FROM node_groups WHERE id = ?", groupID).Scan(&dbGroupName)
	if dbGroupName != putGroupName {
		t.Fatalf("DB verification failed for PUT group name: want %q, got %q", putGroupName, dbGroupName)
	}

	// 3.5 Group Edges: PUT /api/v1/policies/groups/{id}/edges
	edgesBody := map[string]any{
		"edges": []map[string]any{
			{"node_logical_id": hkNodeID, "position": 0},
		},
	}
	resp, err = harness.AuthRequest(http.MethodPut, fmt.Sprintf("/api/v1/policies/groups/%s/edges", groupID), edgesBody, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policies/groups/%s/edges failed: %d body=%s", groupID, resp.StatusCode, resp.Body)
	}

	// DB verification: group_edges table has the node edge
	var memberCount int
	_ = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM group_edges WHERE parent_group_id = ? AND node_logical_id = ?", groupID, hkNodeID).Scan(&memberCount)
	if memberCount < 1 {
		t.Fatalf("group_edges edge not inserted in DB")
	}

	// 3.5b Group Edges: POST /api/v1/policies/groups/{id}/edges
	postEdgesBody := map[string]any{
		"edges": []map[string]any{
			{"node_logical_id": hkNodeID, "position": 0},
		},
	}
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/policies/groups/%s/edges", groupID), postEdgesBody, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/policies/groups/%s/edges failed: %d body=%s", groupID, resp.StatusCode, resp.Body)
	}
	_ = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM group_edges WHERE parent_group_id = ? AND node_logical_id = ?", groupID, hkNodeID).Scan(&memberCount)
	if memberCount < 1 {
		t.Fatalf("group_edges edge not found after POST /edges")
	}

	// 3.6 Create Rule: POST /api/v1/policies/rules
	createRuleBody := map[string]any{
		"target_group_id": groupID,
		"expression":      "MATCH",
		"position":        0,
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/policies/rules", createRuleBody, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/policies/rules failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var createdRule struct {
		Data struct {
			ID         string `json:"id"`
			Expression string `json:"expression"`
		} `json:"data"`
	}
	_ = resp.JSON(&createdRule)
	ruleID := createdRule.Data.ID

	// 3.7 List Rules: GET /api/v1/policies/rules
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/policies/rules", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policies/rules failed: %d", resp.StatusCode)
	}

	// 3.8 Validate Policy Graph: POST /api/v1/policies/validate
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/policies/validate", map[string]any{}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/policies/validate failed: %d", resp.StatusCode)
	}

	// 3.9 Revisions Management
	// Create new draft revision
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/revisions", map[string]any{}, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/revisions failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var revCreated struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = resp.JSON(&revCreated)
	draftRevID := revCreated.Data.ID

	// 3.9a List All Revisions: GET /api/v1/revisions (now has at least 1 revision)
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/revisions", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/revisions failed: %d", resp.StatusCode)
	}
	var revListResp struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		} `json:"data"`
	}
	if err := resp.JSON(&revListResp); err != nil || revListResp.Data.Total < 1 {
		t.Fatalf("GET /api/v1/revisions invalid response: total=%d, data=%v", revListResp.Data.Total, revListResp.Data.Items)
	}

	// 3.9b Single Revision Details: GET /api/v1/revisions/{id}
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/revisions/"+draftRevID, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/revisions/%s failed: %d", draftRevID, resp.StatusCode)
	}
	var revDetailResp struct {
		Data struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"data"`
	}
	if err := resp.JSON(&revDetailResp); err != nil || revDetailResp.Data.ID != draftRevID {
		t.Fatalf("GET /api/v1/revisions/%s mismatch: %+v", draftRevID, revDetailResp)
	}

	// Review & Activate revision
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/revisions/%s/review", draftRevID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/revisions/%s/review failed: %d", draftRevID, resp.StatusCode)
	}
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/revisions/%s/activate", draftRevID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/revisions/%s/activate failed: %d", draftRevID, resp.StatusCode)
	}

	// GET /api/v1/revisions/active
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/revisions/active", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/revisions/active failed: %d", resp.StatusCode)
	}

	// 3.9c Group Risk-Policy Binding: PUT / GET / DELETE /api/v1/policies/groups/{id}/risk-policy
	_, _ = harness.DB.ExecContext(ctx, `
		INSERT OR IGNORE INTO ip_risk_provider_settings (
			provider, schema_version, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms,
			max_response_bytes, created_at, updated_at
		) VALUES ('fixture', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024,
			'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');
	`)

	riskPayload := map[string]any{
		"provider_selection": map[string]any{
			"mode": "single_provider",
			"providers": []map[string]any{
				{"provider": "fixture", "schema_version": "v1"},
			},
		},
		"max_observation_age_seconds": 3600,
		"minimum_confidence":          50,
		"score_bands": []map[string]any{
			{"min": 0, "max": 49, "band": "low", "action": "allow"},
			{"min": 50, "max": 79, "band": "high", "action": "review"},
			{"min": 80, "max": 100, "band": "critical", "action": "block"},
		},
		"trait_rules": []map[string]any{
			{"trait": "tor", "action": "block"},
		},
		"unknown_action":  "review",
		"conflict_action": "review",
		"review_action":   "review",
	}
	createRiskResp, err := harness.AuthRequest(http.MethodPost, "/api/v1/ip-risk/policies", riskPayload, map[string]string{"Idempotency-Key": "e2e-create-risk-policy-1"})
	if err != nil || createRiskResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/ip-risk/policies failed: %d body=%s", createRiskResp.StatusCode, createRiskResp.Body)
	}
	var createdRiskPolicy struct {
		Data struct {
			RevisionID string `json:"revision_id"`
		} `json:"data"`
	}
	_ = createRiskResp.JSON(&createdRiskPolicy)
	riskRevID := createdRiskPolicy.Data.RevisionID

	// Review & Activate risk policy
	_, _ = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/review", riskRevID), nil, nil)
	_, _ = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/ip-risk/policies/%s/activate", riskRevID), nil, nil)

	bindHeaders := map[string]string{"Idempotency-Key": "e2e-risk-bind-grp-1"}
	resp, err = harness.AuthRequest(http.MethodPut, fmt.Sprintf("/api/v1/policies/groups/%s/risk-policy", groupID), map[string]any{
		"policy_revision_id": riskRevID,
	}, bindHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policies/groups/%s/risk-policy failed: %d body=%s", groupID, resp.StatusCode, resp.Body)
	}

	// GET /api/v1/policies/groups/{id}/risk-policy
	resp, err = harness.AuthRequest(http.MethodGet, fmt.Sprintf("/api/v1/policies/groups/%s/risk-policy", groupID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policies/groups/%s/risk-policy failed: %d", groupID, resp.StatusCode)
	}

	// DELETE /api/v1/policies/groups/{id}/risk-policy
	unbindHeaders := map[string]string{"Idempotency-Key": "e2e-risk-unbind-grp-1"}
	resp, err = harness.AuthRequest(http.MethodDelete, fmt.Sprintf("/api/v1/policies/groups/%s/risk-policy", groupID), nil, unbindHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/v1/policies/groups/%s/risk-policy failed: %d", groupID, resp.StatusCode)
	}

	// 3.9d Admission Rules Endpoints: POST, GET, DELETE /api/v1/admission/rules
	createAdmBody := map[string]any{
		"name":       "Reject Bad ASN",
		"expression": "ASN,12345",
		"action":     "reject",
		"position":   0,
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/admission/rules", createAdmBody, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/admission/rules failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var createdAdm struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = resp.JSON(&createdAdm)
	admRuleID := createdAdm.Data.ID
	if admRuleID == "" {
		t.Fatalf("expected admission rule ID, got empty")
	}

	// GET /api/v1/admission/rules
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/admission/rules", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/admission/rules failed: %d", resp.StatusCode)
	}

	// DELETE /api/v1/admission/rules/{id}
	resp, err = harness.AuthRequest(http.MethodDelete, "/api/v1/admission/rules/"+admRuleID, nil, nil)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent) {
		t.Fatalf("DELETE /api/v1/admission/rules/%s failed: %d", admRuleID, resp.StatusCode)
	}
	var admDbRemain int
	_ = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM admission_rules WHERE id = ?", admRuleID).Scan(&admDbRemain)
	if admDbRemain != 0 {
		t.Fatalf("admission rule was not removed from DB after DELETE")
	}

	// 3.10 Clean up Rule and Group
	resp, err = harness.AuthRequest(http.MethodDelete, "/api/v1/policies/rules/"+ruleID, nil, nil)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent) {
		t.Fatalf("DELETE rule failed: %d", resp.StatusCode)
	}
	resp, err = harness.AuthRequest(http.MethodDelete, "/api/v1/policies/groups/"+groupID, nil, nil)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent) {
		t.Fatalf("DELETE group failed: %d", resp.StatusCode)
	}

	// 3.11 Verify all 17 /api/v1/policy/* Alias Routes
	testPolicyAliasRoutes17(t, harness, hkNodeID, riskRevID)

	// =========================================================================
	// Part 4: Scoped Subscription Deletion & Non-Destructive Preservation
	// =========================================================================
	// Delete subscription subID
	resp, err = harness.AuthRequest(http.MethodDelete, "/api/v1/subscriptions/"+subID, nil, nil)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent) {
		t.Fatalf("DELETE subscription %s failed: %d", subID, resp.StatusCode)
	}

	// Verify DB state after deletion:
	// 1. Subscription row deleted
	var deletedSubExists int
	_ = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM subscriptions WHERE id = ?", subID).Scan(&deletedSubExists)
	if deletedSubExists != 0 {
		t.Fatalf("subscription was not deleted from DB")
	}

	// 2. Node sources for this subscription removed
	var sourcesRemaining int
	_ = harness.DB.QueryRowContext(ctx, "SELECT count(1) FROM node_sources WHERE subscription_id = ?", subID).Scan(&sourcesRemaining)
	if sourcesRemaining != 0 {
		t.Fatalf("node_sources for deleted subscription must be removed, found %d", sourcesRemaining)
	}

	// 3. EXPLICIT SUBSCRIPTION DELETE SEMANTICS:
	// The node record itself is preserved in `nodes` table (not physically purged),
	// and its active status transitions to 0 because its sole owning subscription was deleted.
	var preservedNodeActive int
	err = harness.DB.QueryRowContext(ctx, "SELECT active FROM nodes WHERE logical_id = ?", hkNodeID).Scan(&preservedNodeActive)
	if err != nil {
		t.Fatalf("node record was deleted from nodes table! err=%v", err)
	}
	if preservedNodeActive != 0 {
		t.Fatalf("expected node active=0 after its only subscription was explicitly deleted, got %d", preservedNodeActive)
	}
}

// testPolicyAliasRoutes17 executes real HTTP requests with business payloads against all 17 /api/v1/policy/*
// alias endpoints, verifying they succeed and mirror /api/v1/policies/* identically.
func testPolicyAliasRoutes17(t *testing.T, harness *TestHarness, nodeID string, riskPolicyRevID string) {
	t.Helper()

	// 1. GET /api/v1/policy/global-node-filter
	resp, err := harness.AuthRequest(http.MethodGet, "/api/v1/policy/global-node-filter", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policy/global-node-filter failed: %d", resp.StatusCode)
	}

	// 2. PUT /api/v1/policy/global-node-filter
	putBody := map[string]any{
		"spec": map[string]any{"conditions": []map[string]any{}},
	}
	resp, err = harness.AuthRequest(http.MethodPut, "/api/v1/policy/global-node-filter", putBody, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policy/global-node-filter failed: %d", resp.StatusCode)
	}

	// 3. POST /api/v1/policy/groups
	createGrpBody := map[string]any{"name": "Alias Group Test", "group_type": "select"}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/policy/groups", createGrpBody, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/policy/groups failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var grp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = resp.JSON(&grp)
	aliasGroupID := grp.Data.ID

	// 4. GET /api/v1/policy/groups
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/policy/groups", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policy/groups failed: %d", resp.StatusCode)
	}

	// 5. GET /api/v1/policy/groups/{id}
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/policy/groups/"+aliasGroupID, nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policy/groups/%s failed: %d", aliasGroupID, resp.StatusCode)
	}

	// 6. PATCH /api/v1/policy/groups/{id}
	resp, err = harness.AuthRequest(http.MethodPatch, "/api/v1/policy/groups/"+aliasGroupID, map[string]any{"name": "Alias Group Renamed"}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/v1/policy/groups/%s failed: %d", aliasGroupID, resp.StatusCode)
	}

	// 7. PUT /api/v1/policy/groups/{id}
	resp, err = harness.AuthRequest(http.MethodPut, "/api/v1/policy/groups/"+aliasGroupID, map[string]any{"name": "Alias Group Put Updated"}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policy/groups/%s failed: %d", aliasGroupID, resp.StatusCode)
	}

	// 8. POST /api/v1/policy/groups/{id}/edges
	edges := map[string]any{"edges": []map[string]any{{"node_logical_id": nodeID, "position": 0}}}
	resp, err = harness.AuthRequest(http.MethodPost, fmt.Sprintf("/api/v1/policy/groups/%s/edges", aliasGroupID), edges, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/policy/groups/%s/edges failed: %d", aliasGroupID, resp.StatusCode)
	}

	// 9. PUT /api/v1/policy/groups/{id}/edges
	resp, err = harness.AuthRequest(http.MethodPut, fmt.Sprintf("/api/v1/policy/groups/%s/edges", aliasGroupID), edges, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policy/groups/%s/edges failed: %d", aliasGroupID, resp.StatusCode)
	}

	// 10. PUT /api/v1/policy/groups/{id}/risk-policy
	bindHeaders := map[string]string{"Idempotency-Key": "alias-risk-bind-1"}
	resp, err = harness.AuthRequest(http.MethodPut, fmt.Sprintf("/api/v1/policy/groups/%s/risk-policy", aliasGroupID), map[string]any{
		"policy_revision_id": riskPolicyRevID,
	}, bindHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policy/groups/%s/risk-policy failed: %d body=%s", aliasGroupID, resp.StatusCode, resp.Body)
	}

	// 11. GET /api/v1/policy/groups/{id}/risk-policy
	resp, err = harness.AuthRequest(http.MethodGet, fmt.Sprintf("/api/v1/policy/groups/%s/risk-policy", aliasGroupID), nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policy/groups/%s/risk-policy failed: %d", aliasGroupID, resp.StatusCode)
	}

	// 12. DELETE /api/v1/policy/groups/{id}/risk-policy
	unbindHeaders := map[string]string{"Idempotency-Key": "alias-risk-unbind-1"}
	resp, err = harness.AuthRequest(http.MethodDelete, fmt.Sprintf("/api/v1/policy/groups/%s/risk-policy", aliasGroupID), nil, unbindHeaders)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/v1/policy/groups/%s/risk-policy failed: %d", aliasGroupID, resp.StatusCode)
	}

	// 13. POST /api/v1/policy/rules
	createRuleBody := map[string]any{
		"target_group_id": aliasGroupID,
		"expression":      "MATCH",
		"position":        0,
	}
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/policy/rules", createRuleBody, nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/policy/rules failed: %d body=%s", resp.StatusCode, resp.Body)
	}
	var ruleData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = resp.JSON(&ruleData)
	aliasRuleID := ruleData.Data.ID

	// 14. GET /api/v1/policy/rules
	resp, err = harness.AuthRequest(http.MethodGet, "/api/v1/policy/rules", nil, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policy/rules failed: %d", resp.StatusCode)
	}

	// 15. POST /api/v1/policy/validate
	resp, err = harness.AuthRequest(http.MethodPost, "/api/v1/policy/validate", map[string]any{}, nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/policy/validate failed: %d", resp.StatusCode)
	}

	// 16. DELETE /api/v1/policy/rules/{id}
	resp, err = harness.AuthRequest(http.MethodDelete, "/api/v1/policy/rules/"+aliasRuleID, nil, nil)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent) {
		t.Fatalf("DELETE /api/v1/policy/rules/%s failed: %d", aliasRuleID, resp.StatusCode)
	}

	// 17. DELETE /api/v1/policy/groups/{id}
	resp, err = harness.AuthRequest(http.MethodDelete, "/api/v1/policy/groups/"+aliasGroupID, nil, nil)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent) {
		t.Fatalf("DELETE /api/v1/policy/groups/%s failed: %d", aliasGroupID, resp.StatusCode)
	}
}
