package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe"
	"clash-sub-parser/internal/repository"
	"clash-sub-parser/internal/server"
)

// setupTestServer creates an in-memory SQLite DB, mock probe engine, and initializes the server.
func setupTestServer(t *testing.T) (*server.Server, *repository.Repositories, *repository.SQLiteDB) {
	t.Helper()

	dbName := fmt.Sprintf("file:test_server_%d?mode=memory&cache=shared", time.Now().UnixNano())
	opts := repository.Options{
		Path:         dbName,
		MaxOpenConns: 5,
		MaxIdleConns: 2,
		BusyTimeout:  10 * time.Second,
	}

	db, err := repository.NewSQLiteDB(opts)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.InitSchema(ctx); err != nil {
		t.Fatalf("failed to init test db schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	repos := db.Repositories()
	engine := probe.NewEngine(nil)

	srv := server.NewServer(repos, engine)
	return srv, repos, db
}

func doRequest(handler http.Handler, method, target string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var bodyReader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req := httptest.NewRequest(method, target, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// 1. Test Middlewares
func TestRouter_Middleware(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	t.Run("CORS preflight request", func(t *testing.T) {
		rec := doRequest(srv.Router(), "OPTIONS", "/api/subscriptions", nil, map[string]string{
			"Origin":                         "http://localhost:5173",
			"Access-Control-Request-Method":  "POST",
			"Access-Control-Request-Headers": "Content-Type, X-Clash-CSRF",
		})
		if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
			t.Errorf("expected 204 or 200 for CORS preflight, got %d", rec.Code)
		}
		if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin == "" {
			t.Errorf("expected Access-Control-Allow-Origin header, got empty")
		}
	})

	t.Run("RequestID header injection", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/health", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for /health, got %d", rec.Code)
		}
		if reqID := rec.Header().Get("X-Request-Id"); reqID == "" {
			t.Errorf("expected X-Request-Id header to be injected, got empty")
		}
	})

	t.Run("Health check endpoint", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/health", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["status"] != "ok" {
			t.Errorf("expected status 'ok', got %v", resp["status"])
		}
	})

	t.Run("Unified Error JSON structure", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/subscriptions/999999", nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		var errResp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to parse error response JSON: %v", err)
		}
		if errResp["detail"] == nil && errResp["error"] == nil {
			t.Errorf("expected detail or error field in error response: %v", errResp)
		}
	})
}

// 2. Test Subscriptions API
func TestSubscriptions_API(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	var createdID int64

	t.Run("GET empty subscriptions", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/subscriptions", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var list []any
		_ = json.Unmarshal(rec.Body.Bytes(), &list)
		if len(list) != 0 {
			t.Errorf("expected empty list, got %d items", len(list))
		}
	})

	t.Run("POST create subscription with validation", func(t *testing.T) {
		// Missing name
		rec := doRequest(srv.Router(), "POST", "/api/subscriptions", map[string]any{
			"url": "https://example.com/sub",
		}, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for missing name, got %d", rec.Code)
		}

		// Valid create
		rec = doRequest(srv.Router(), "POST", "/api/subscriptions", map[string]any{
			"name":        "Test Sub",
			"url":         "https://example.com/sub1",
			"enabled":     true,
			"is_primary":  true,
			"node_prefix": "TST-",
		}, nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 for valid subscription, got %d: %s", rec.Code, rec.Body.String())
		}
		var created map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &created)
		if created["name"] != "Test Sub" {
			t.Errorf("expected name 'Test Sub', got %v", created["name"])
		}
		createdID = int64(created["id"].(float64))
	})

	t.Run("GET subscription by ID", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", fmt.Sprintf("/api/subscriptions/%d", createdID), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var sub map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &sub)
		if sub["name"] != "Test Sub" {
			t.Errorf("expected 'Test Sub', got %v", sub["name"])
		}
	})

	t.Run("PATCH and PUT update subscription", func(t *testing.T) {
		rec := doRequest(srv.Router(), "PATCH", fmt.Sprintf("/api/subscriptions/%d", createdID), map[string]any{
			"name": "Updated Sub Name",
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on PATCH, got %d: %s", rec.Code, rec.Body.String())
		}

		// Check name updated
		rec = doRequest(srv.Router(), "GET", fmt.Sprintf("/api/subscriptions/%d", createdID), nil, nil)
		var sub map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &sub)
		if sub["name"] != "Updated Sub Name" {
			t.Errorf("expected name 'Updated Sub Name', got %v", sub["name"])
		}
	})

	t.Run("POST refresh/fetch subscription", func(t *testing.T) {
		rec := doRequest(srv.Router(), "POST", fmt.Sprintf("/api/subscriptions/%d/refresh", createdID), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on refresh, got %d: %s", rec.Code, rec.Body.String())
		}

		// Also check /fetch alias for frontend compatibility
		rec = doRequest(srv.Router(), "POST", fmt.Sprintf("/api/subscriptions/%d/fetch", createdID), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on fetch alias, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET subscription nodes", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", fmt.Sprintf("/api/subscriptions/%d/nodes", createdID), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("DELETE subscription", func(t *testing.T) {
		rec := doRequest(srv.Router(), "DELETE", fmt.Sprintf("/api/subscriptions/%d", createdID), nil, nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 on delete, got %d", rec.Code)
		}

		rec = doRequest(srv.Router(), "GET", fmt.Sprintf("/api/subscriptions/%d", createdID), nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 after delete, got %d", rec.Code)
		}
	})
}

// 3. Test Nodes API
func TestNodes_API(t *testing.T) {
	srv, repos, _ := setupTestServer(t)
	ctx := context.Background()

	// Seed nodes
	n1 := &domain.Node{
		LogicalID: "node-1",
		Name:      "Tokyo-01-Vmess",
		Protocol:  domain.ProtocolVMess,
		Server:    "tokyo.example.com",
		Port:      443,
	}
	n2 := &domain.Node{
		LogicalID: "node-2",
		Name:      "HongKong-01-SS",
		Protocol:  domain.ProtocolShadowsocks,
		Server:    "hk.example.com",
		Port:      8388,
	}
	n3 := &domain.Node{
		LogicalID: "node-3",
		Name:      "Tokyo-02-Trojan",
		Protocol:  domain.ProtocolTrojan,
		Server:    "tokyo2.example.com",
		Port:      443,
	}
	_ = repos.Nodes.Create(ctx, n1)
	_ = repos.Nodes.Create(ctx, n2)
	_ = repos.Nodes.Create(ctx, n3)

	t.Run("GET /api/nodes list and pagination", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/nodes?limit=2&offset=0", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var paged map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &paged)
		items, ok := paged["items"].([]any)
		if !ok || len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}
		if total := int(paged["total"].(float64)); total != 3 {
			t.Errorf("expected total 3, got %d", total)
		}
	})

	t.Run("GET /api/nodes filter by protocol", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/nodes?protocol=vmess", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var paged map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &paged)
		items := paged["items"].([]any)
		if len(items) != 1 {
			t.Errorf("expected 1 vmess node, got %d", len(items))
		}
	})

	t.Run("GET /api/nodes filter by keyword", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/nodes?keyword=Tokyo", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var paged map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &paged)
		items := paged["items"].([]any)
		if len(items) != 2 {
			t.Errorf("expected 2 Tokyo nodes, got %d", len(items))
		}
	})

	t.Run("GET /api/nodes/{id}", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", fmt.Sprintf("/api/nodes/%d", n1.ID), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var node map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &node)
		if node["name"] != "Tokyo-01-Vmess" {
			t.Errorf("expected node name Tokyo-01-Vmess, got %v", node["name"])
		}
	})

	t.Run("DELETE /api/nodes/{id}", func(t *testing.T) {
		rec := doRequest(srv.Router(), "DELETE", fmt.Sprintf("/api/nodes/%d", n3.ID), nil, nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}

		rec = doRequest(srv.Router(), "GET", fmt.Sprintf("/api/nodes/%d", n3.ID), nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 after delete, got %d", rec.Code)
		}
	})

	t.Run("POST /api/nodes/batch-delete", func(t *testing.T) {
		rec := doRequest(srv.Router(), "POST", "/api/nodes/batch-delete", map[string]any{
			"ids": []int64{n1.ID, n2.ID},
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for batch-delete, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if deleted := int(resp["deleted"].(float64)); deleted != 2 {
			t.Errorf("expected 2 deleted nodes, got %d", deleted)
		}
	})
}

// 4. Test Node Groups API
func TestNodeGroups_API(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	var groupID int64

	t.Run("GET /api/groups and /api/node-groups empty", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/groups", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		rec2 := doRequest(srv.Router(), "GET", "/api/node-groups", nil, nil)
		if rec2.Code != http.StatusOK {
			t.Fatalf("expected 200 on /api/node-groups alias, got %d", rec2.Code)
		}
	})

	t.Run("POST /api/groups create group", func(t *testing.T) {
		rec := doRequest(srv.Router(), "POST", "/api/groups", map[string]any{
			"name":       "Auto-Fastest",
			"group_type": "url-test",
			"kind":       "auto",
			"sort_order": 1,
		}, nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var created map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &created)
		groupID = int64(created["id"].(float64))
	})

	t.Run("GET /api/groups/{id}", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", fmt.Sprintf("/api/groups/%d", groupID), nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("PATCH /api/groups/{id}", func(t *testing.T) {
		rec := doRequest(srv.Router(), "PATCH", fmt.Sprintf("/api/groups/%d", groupID), map[string]any{
			"name": "Auto-Fastest-Updated",
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /api/groups/validate detection", func(t *testing.T) {
		// Valid groups
		rec := doRequest(srv.Router(), "POST", "/api/groups/validate", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on valid validate, got %d: %s", rec.Code, rec.Body.String())
		}

		// Self-inclusion validation error
		rec = doRequest(srv.Router(), "PATCH", fmt.Sprintf("/api/groups/%d", groupID), map[string]any{
			"include_group_ids": []int64{groupID},
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on patch, got %d", rec.Code)
		}

		rec = doRequest(srv.Router(), "POST", "/api/groups/validate", nil, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 when circular/self-inclusion exists, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /api/groups/reorder", func(t *testing.T) {
		rec := doRequest(srv.Router(), "POST", "/api/groups/reorder", map[string]any{
			"items": []map[string]any{
				{"id": groupID, "sort_order": 99},
			},
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on reorder, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE /api/groups/{id}", func(t *testing.T) {
		rec := doRequest(srv.Router(), "DELETE", fmt.Sprintf("/api/groups/%d", groupID), nil, nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
	})
}

// 5. Test Rules API
func TestRules_API(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	var ruleID int64

	t.Run("POST /api/rules create rule", func(t *testing.T) {
		rec := doRequest(srv.Router(), "POST", "/api/rules", map[string]any{
			"name":     "Google",
			"category": "Google",
			"type":     "DOMAIN-SUFFIX",
			"value":    "google.com",
			"proxy":    "Auto-Fastest",
			"enabled":  true,
		}, nil)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var created map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &created)
		ruleID = int64(created["id"].(float64))
	})

	t.Run("GET /api/rules", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/rules", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var list []any
		_ = json.Unmarshal(rec.Body.Bytes(), &list)
		if len(list) != 1 {
			t.Errorf("expected 1 rule, got %d", len(list))
		}
	})

	t.Run("PATCH /api/rules/{id}", func(t *testing.T) {
		rec := doRequest(srv.Router(), "PATCH", fmt.Sprintf("/api/rules/%d", ruleID), map[string]any{
			"value": "google.com.hk",
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("POST /api/rules/reorder", func(t *testing.T) {
		rec := doRequest(srv.Router(), "POST", "/api/rules/reorder", map[string]any{
			"items": []map[string]any{
				{"id": ruleID, "sort_order": 50},
			},
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on reorder, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /api/rules/batch operations", func(t *testing.T) {
		rec := doRequest(srv.Router(), "POST", "/api/rules/batch", map[string]any{
			"create": []map[string]any{
				{
					"name":     "YouTube",
					"category": "YouTube",
					"type":     "DOMAIN-SUFFIX",
					"value":    "youtube.com",
					"proxy":    "Proxy",
					"enabled":  true,
				},
			},
			"delete": []int64{ruleID},
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on batch, got %d: %s", rec.Code, rec.Body.String())
		}

		// Verify old rule deleted, new rule exists
		rec = doRequest(srv.Router(), "GET", "/api/rules", nil, nil)
		var list []map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &list)
		if len(list) != 1 || list[0]["name"] != "YouTube" {
			t.Errorf("expected only YouTube rule, got: %v", list)
		}
	})
}

// 6. Test Probe API
func TestProbe_API(t *testing.T) {
	srv, repos, _ := setupTestServer(t)
	ctx := context.Background()

	// Seed probe result
	res := &domain.NodeProbeResult{
		NodeKey:   "test-node-key-1",
		Name:      "Test-Node-1",
		Server:    "1.1.1.1",
		Port:      443,
		Type:      "vmess",
		Status:    domain.ProbeStatusOK,
		CheckedAt: time.Now().Unix(),
	}
	_ = repos.Probes.Upsert(ctx, res)

	t.Run("GET /api/probe/status", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/probe/status", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var status map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &status)
		if status["state"] == nil {
			t.Errorf("expected state field in probe status: %v", status)
		}
	})

	t.Run("POST /api/probe/start triggers async probing", func(t *testing.T) {
		rec := doRequest(srv.Router(), "POST", "/api/probe/start", map[string]any{
			"concurrency": 10,
		}, nil)
		if rec.Code != http.StatusOK && rec.Code != http.StatusAccepted {
			t.Fatalf("expected 200 or 202, got %d: %s", rec.Code, rec.Body.String())
		}
		var startResp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &startResp)
		if startResp["status"] == nil {
			t.Errorf("expected status in probe start response: %v", startResp)
		}
	})

	t.Run("GET /api/probe/results list", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/probe/results", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		results, ok := resp["results"].(map[string]any)
		if !ok || len(results) == 0 {
			t.Errorf("expected probe results map with items, got: %v", resp)
		}
	})

	t.Run("GET /api/probe/results/detail", func(t *testing.T) {
		rec := doRequest(srv.Router(), "GET", "/api/probe/results/detail?node_key=test-node-key-1", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var detail map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &detail)
		if detail["node_key"] != "test-node-key-1" {
			t.Errorf("expected node_key test-node-key-1, got %v", detail["node_key"])
		}
	})

	t.Run("DELETE /api/probe/results clears results", func(t *testing.T) {
		rec := doRequest(srv.Router(), "DELETE", "/api/probe/results", nil, nil)
		if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
			t.Fatalf("expected 200 or 204, got %d", rec.Code)
		}
	})
}
