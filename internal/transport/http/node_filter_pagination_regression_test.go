package http_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func setupRegressionTestRouter(t *testing.T, db *sql.DB, probeSvc *probe.Service) http.Handler {
	t.Helper()
	obsRepo := sqlite.NewProbeObservationRepository(db)
	routerCfg := newTestRouterConfig(true)
	routerCfg.InventoryService = inventory.NewService(
		db,
		sqlite.NewSubscriptionRepository(db),
		sqlite.NewSubscriptionFetchRepository(db),
		sqlite.NewNodeRepository(db),
		sqlite.NewNodeSourceRepository(db),
		nil,
		inventory.WithProbeObservationRepository(obsRepo),
	)
	routerCfg.ProbeService = probeSvc
	return transporthttp.NewRouter(routerCfg)
}

func getNodesPaginated(t *testing.T, router http.Handler, path string, token string) (*httptest.ResponseRecorder, nodePaginatedResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp nodePaginatedResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode JSON response from %s: %v", path, err)
		}
	}
	return rec, resp
}

func getNodesError(t *testing.T, router http.Handler, path string, token string) (*httptest.ResponseRecorder, transporthttp.ErrorResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var errResp transporthttp.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error JSON from %s: %v", path, err)
	}
	return rec, errResp
}

func seedPaginationRegressionNodes(t *testing.T, db *sql.DB) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Subscription & Fetch headers for enabled-subscription inventory scope
	_, err = tx.Exec(`
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES ('sub-reg-001', 'Pagination Regression Sub', 'secret://test/reg', 1, 'rev-reg-1', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test subscription: %v", err)
	}

	_, err = tx.Exec(`
		INSERT INTO subscription_fetches (id, subscription_id, started_at, finished_at, outcome)
		VALUES ('fetch-reg-001', 'sub-reg-001', '2026-09-15T00:00:00Z', '2026-09-15T00:01:00Z', 'success')
	`)
	if err != nil {
		t.Fatalf("failed to insert test subscription fetch: %v", err)
	}

	// 2. Probe run for foreign key referencing by probe_observations
	_, err = tx.Exec(`
		INSERT INTO probe_runs (id, idempotency_key, actor_scope, config_revision, state, deadline_at, created_at, updated_at)
		VALUES ('run-reg-001', 'run-key-reg-001', 'default', 'rev-reg-1', 'completed', '2026-09-15T13:00:00Z', '2026-09-15T12:00:00Z', '2026-09-15T12:05:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test probe run: %v", err)
	}

	nodeStmt, err := tx.Prepare(`
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, connection_revision, created_at, updated_at)
		VALUES (?, ?, ?, '203.0.113.10', 8388, '{}', ?, 1, ?, ?)
	`)
	if err != nil {
		t.Fatalf("failed to prepare node stmt: %v", err)
	}
	defer nodeStmt.Close()

	sourceStmt, err := tx.Prepare(`
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, 'sub-reg-001', 'fetch-reg-001')
	`)
	if err != nil {
		t.Fatalf("failed to prepare source stmt: %v", err)
	}
	defer sourceStmt.Close()

	obsStmt, err := tx.Prepare(`
		INSERT INTO probe_observations (id, probe_run_id, node_logical_id, kind, verdict, evidence_digest, observed_at, latency_ms, redacted_summary, connection_revision)
		VALUES (?, 'run-reg-001', ?, 'baseline', ?, 'digest-ok', ?, 45, 'ok', 1)
	`)
	if err != nil {
		t.Fatalf("failed to prepare observation stmt: %v", err)
	}
	defer obsStmt.Close()

	// Seed 250 nodes:
	// - created_at is strictly decreasing with i (node-reg-0001 newest, node-reg-0250 oldest).
	//   Under default "created_at DESC, logical_id ASC", unfiltered page 1 (1..100) contains node-reg-0001..node-reg-0100.
	// - Indices 1..100 contain NO HongKong search matches, NO trojan protocol, and NO probe observations (health="unknown").
	// - Target matches strictly beyond first unfiltered 100:
	//   * Search match "HongKong": nodes 101..105 (5 nodes).
	//   * Protocol match "trojan": nodes 110..112 (3 nodes).
	//   * HealthStatus "healthy": nodes 115..118 (4 nodes).
	//   * HealthStatus "unhealthy": node 119 (1 node).
	//   * Combined filter (>100 matches): nodes 121..245 (125 nodes).
	now := time.Now().UTC()
	baseCreatedAt := now.Add(-2 * time.Hour)
	observedAt := now.Add(-1 * time.Hour).Format(time.RFC3339Nano)

	for i := 1; i <= 250; i++ {
		logicalID := fmt.Sprintf("node-reg-%04d", i)
		protocol := domain.ProtocolSS
		displayName := fmt.Sprintf("US-Standard-Node-%04d", i)
		active := 0
		obsVerdict := ""

		switch {
		case i >= 101 && i <= 105:
			displayName = fmt.Sprintf("HongKong-Node-%04d", i)
			active = 1
		case i >= 110 && i <= 112:
			protocol = domain.ProtocolTrojan
			displayName = fmt.Sprintf("Trojan-Node-%04d", i)
			active = 1
		case i >= 115 && i <= 118:
			displayName = fmt.Sprintf("Healthy-Node-%04d", i)
			active = 1
			obsVerdict = string(domain.VerdictAvailable)
		case i == 119:
			displayName = fmt.Sprintf("Unhealthy-Node-%04d", i)
			active = 1
			obsVerdict = string(domain.VerdictError)
		case i >= 121 && i <= 245:
			protocol = domain.ProtocolVMess
			active = 1
			displayName = fmt.Sprintf("Combined-Cluster-VMess-%04d", i)
		}

		createdAt := baseCreatedAt.Add(time.Duration(300-i) * time.Second).Format(time.RFC3339Nano)

		// 1. Insert node first to satisfy foreign key constraints (ForeignKeys=true)
		if _, err := nodeStmt.Exec(logicalID, string(protocol), displayName, active, createdAt, createdAt); err != nil {
			t.Fatalf("failed to insert node %s: %v", logicalID, err)
		}

		// 2. Insert node source provenance
		if _, err := sourceStmt.Exec(logicalID); err != nil {
			t.Fatalf("failed to insert node source %s: %v", logicalID, err)
		}

		// 3. Insert probe observation AFTER node exists
		if obsVerdict != "" {
			obsID := fmt.Sprintf("obs-%s-%04d", obsVerdict, i)
			if _, err := obsStmt.Exec(obsID, logicalID, obsVerdict, observedAt); err != nil {
				t.Fatalf("failed to insert observation for %s: %v", logicalID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit regression nodes transaction: %v", err)
	}
}

func TestNodeFilterPaginationRegression(t *testing.T) {
	db := newCleanSQLiteDB(t)
	seedPaginationRegressionNodes(t, db)
	router := setupRegressionTestRouter(t, db, nil)

	// 1. Baseline unfiltered ordering: verify first 100 nodes exclude all target criteria
	t.Run("baseline_unfiltered_first_100_excludes_target_criteria", func(t *testing.T) {
		rec, resp := getNodesPaginated(t, router, "/api/v1/nodes?page=1&page_size=100", testAdminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if resp.Data.Total != 250 {
			t.Fatalf("expected total 250, got %d", resp.Data.Total)
		}
		if resp.Data.Page != 1 || resp.Data.PageSize != 100 {
			t.Fatalf("expected page=1 page_size=100, got page=%d page_size=%d", resp.Data.Page, resp.Data.PageSize)
		}
		if len(resp.Data.Items) != 100 {
			t.Fatalf("expected 100 items on page 1, got %d", len(resp.Data.Items))
		}
		if resp.Data.Items[0].LogicalID != "node-reg-0001" {
			t.Errorf("expected first item 'node-reg-0001', got %s", resp.Data.Items[0].LogicalID)
		}
		if resp.Data.Items[99].LogicalID != "node-reg-0100" {
			t.Errorf("expected 100th item 'node-reg-0100', got %s", resp.Data.Items[99].LogicalID)
		}

		for _, item := range resp.Data.Items {
			if strings.Contains(item.DisplayName, "HongKong") {
				t.Fatalf("unfiltered first 100 leaked HongKong node: %s", item.LogicalID)
			}
			if item.Protocol == domain.ProtocolTrojan {
				t.Fatalf("unfiltered first 100 leaked trojan node: %s", item.LogicalID)
			}
			if item.HealthStatus == "healthy" || item.HealthStatus == "unhealthy" {
				t.Fatalf("unfiltered first 100 leaked tested health node: %s (%s)", item.LogicalID, item.HealthStatus)
			}
		}
	})

	// 2. Search match strictly beyond unfiltered 100
	t.Run("search_filters_whole_inventory_beyond_first_100", func(t *testing.T) {
		rec, resp := getNodesPaginated(t, router, "/api/v1/nodes?search=HongKong&page=1&page_size=50", testAdminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if resp.Data.Total != 5 {
			t.Fatalf("expected total 5 for HongKong search across whole inventory, got %d", resp.Data.Total)
		}
		if len(resp.Data.Items) != 5 {
			t.Fatalf("expected 5 items, got %d", len(resp.Data.Items))
		}
		for _, item := range resp.Data.Items {
			if !strings.Contains(item.DisplayName, "HongKong") {
				t.Errorf("expected item display name to contain 'HongKong', got %s", item.DisplayName)
			}
		}
	})

	// 3. Protocol match strictly beyond unfiltered 100
	t.Run("protocol_filters_whole_inventory_beyond_first_100", func(t *testing.T) {
		rec, resp := getNodesPaginated(t, router, "/api/v1/nodes?protocol=trojan&page=1&page_size=50", testAdminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if resp.Data.Total != 3 {
			t.Fatalf("expected total 3 for trojan protocol across whole inventory, got %d", resp.Data.Total)
		}
		if len(resp.Data.Items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(resp.Data.Items))
		}
		for _, item := range resp.Data.Items {
			if item.Protocol != domain.ProtocolTrojan {
				t.Errorf("expected protocol trojan, got %s", item.Protocol)
			}
		}
	})

	// 4. Health status match strictly beyond unfiltered 100
	t.Run("health_status_filters_whole_inventory_beyond_first_100", func(t *testing.T) {
		recHealthy, respHealthy := getNodesPaginated(t, router, "/api/v1/nodes?health_status=healthy&page=1&page_size=50", testAdminToken)
		if recHealthy.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", recHealthy.Code, recHealthy.Body.String())
		}
		if respHealthy.Data.Total != 4 {
			t.Fatalf("expected total 4 for healthy nodes across whole inventory, got %d", respHealthy.Data.Total)
		}
		if len(respHealthy.Data.Items) != 4 {
			t.Fatalf("expected 4 items, got %d", len(respHealthy.Data.Items))
		}
		for _, item := range respHealthy.Data.Items {
			if item.HealthStatus != "healthy" {
				t.Errorf("expected health_status healthy, got %s", item.HealthStatus)
			}
		}

		recUnhealthy, respUnhealthy := getNodesPaginated(t, router, "/api/v1/nodes?health_status=unhealthy&page=1&page_size=50", testAdminToken)
		if recUnhealthy.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", recUnhealthy.Code, recUnhealthy.Body.String())
		}
		if respUnhealthy.Data.Total != 1 {
			t.Fatalf("expected total 1 for unhealthy nodes, got %d", respUnhealthy.Data.Total)
		}
		if len(respUnhealthy.Data.Items) != 1 || respUnhealthy.Data.Items[0].LogicalID != "node-reg-0119" {
			t.Fatalf("expected item node-reg-0119, got %+v", respUnhealthy.Data.Items)
		}
	})

	// 5. Combined filters with >100 matches: assert total, first and second pages, no duplicates
	t.Run("combined_filters_with_over_100_matches_multi_page_no_duplicates", func(t *testing.T) {
		const combinedQuery = "/api/v1/nodes?protocol=vmess&active_only=true&search=Combined-Cluster"

		// Page 1: page_size=100
		recPage1, respPage1 := getNodesPaginated(t, router, combinedQuery+"&page=1&page_size=100", testAdminToken)
		if recPage1.Code != http.StatusOK {
			t.Fatalf("page 1: expected 200 OK, got %d: %s", recPage1.Code, recPage1.Body.String())
		}
		if respPage1.Data.Total != 125 {
			t.Fatalf("expected total 125 for combined filter, got %d", respPage1.Data.Total)
		}
		if respPage1.Data.Page != 1 || respPage1.Data.PageSize != 100 {
			t.Errorf("page 1: expected page 1 page_size 100, got %d / %d", respPage1.Data.Page, respPage1.Data.PageSize)
		}
		if len(respPage1.Data.Items) != 100 {
			t.Fatalf("expected 100 items on page 1, got %d", len(respPage1.Data.Items))
		}

		page1IDs := make(map[string]bool, len(respPage1.Data.Items))
		for _, item := range respPage1.Data.Items {
			if item.Protocol != domain.ProtocolVMess || !item.Active || !strings.Contains(item.DisplayName, "Combined-Cluster") {
				t.Fatalf("item does not match combined filter: %+v", item)
			}
			if page1IDs[item.LogicalID] {
				t.Fatalf("duplicate item on page 1: %s", item.LogicalID)
			}
			page1IDs[item.LogicalID] = true
		}

		// Page 2: page_size=100
		recPage2, respPage2 := getNodesPaginated(t, router, combinedQuery+"&page=2&page_size=100", testAdminToken)
		if recPage2.Code != http.StatusOK {
			t.Fatalf("page 2: expected 200 OK, got %d: %s", recPage2.Code, recPage2.Body.String())
		}
		if respPage2.Data.Total != 125 {
			t.Fatalf("page 2: expected total 125, got %d", respPage2.Data.Total)
		}
		if respPage2.Data.Page != 2 {
			t.Errorf("expected page 2, got %d", respPage2.Data.Page)
		}
		if len(respPage2.Data.Items) != 25 {
			t.Fatalf("expected 25 items on page 2, got %d", len(respPage2.Data.Items))
		}

		for _, item := range respPage2.Data.Items {
			if item.Protocol != domain.ProtocolVMess || !item.Active || !strings.Contains(item.DisplayName, "Combined-Cluster") {
				t.Fatalf("page 2 item does not match combined filter: %+v", item)
			}
			if page1IDs[item.LogicalID] {
				t.Fatalf("duplicate node logical_id %s found across page 1 and page 2", item.LogicalID)
			}
		}

		// Page 3: beyond total -> empty items
		recPage3, respPage3 := getNodesPaginated(t, router, combinedQuery+"&page=3&page_size=100", testAdminToken)
		if recPage3.Code != http.StatusOK {
			t.Fatalf("page 3: expected 200 OK, got %d: %s", recPage3.Code, recPage3.Body.String())
		}
		if respPage3.Data.Total != 125 {
			t.Errorf("page 3: expected total 125, got %d", respPage3.Data.Total)
		}
		if len(respPage3.Data.Items) != 0 {
			t.Errorf("expected 0 items on page 3, got %d", len(respPage3.Data.Items))
		}
	})

	// 6. Authentication and invalid filter errors preserved
	t.Run("authentication_and_invalid_filter_errors_preserved", func(t *testing.T) {
		// A: Unauthenticated request must return 401 Unauthorized
		recNoAuth, _ := getNodesPaginated(t, router, "/api/v1/nodes?page=1&page_size=50", "")
		if recNoAuth.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for missing auth, got %d", recNoAuth.Code)
		}

		// B: Invalid page values must return 422 Unprocessable Entity with invalid_page
		for _, badPage := range []string{"0", "-1", "abc"} {
			rec, errResp := getNodesError(t, router, "/api/v1/nodes?page="+badPage, testAdminToken)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("page=%s: expected 422, got %d", badPage, rec.Code)
			}
			if errResp.Code != "invalid_page" {
				t.Errorf("page=%s: expected code 'invalid_page', got '%s'", badPage, errResp.Code)
			}
		}

		// C: Invalid page_size values must return 422 Unprocessable Entity with invalid_page_size
		for _, badSize := range []string{"0", "-10", "xyz"} {
			rec, errResp := getNodesError(t, router, "/api/v1/nodes?page_size="+badSize, testAdminToken)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("page_size=%s: expected 422, got %d", badSize, rec.Code)
			}
			if errResp.Code != "invalid_page_size" {
				t.Errorf("page_size=%s: expected code 'invalid_page_size', got '%s'", badSize, errResp.Code)
			}
		}

		// D: Invalid health_status values must return 400 Bad Request with invalid_health_status
		for _, badHealth := range []string{"degraded", "foo", "healthy,degraded"} {
			rec, errResp := getNodesError(t, router, "/api/v1/nodes?health_status="+badHealth, testAdminToken)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("health_status=%s: expected 400, got %d", badHealth, rec.Code)
			}
			if errResp.Code != "invalid_health_status" {
				t.Errorf("health_status=%s: expected code 'invalid_health_status', got '%s'", badHealth, errResp.Code)
			}
		}

		// E: Invalid scope value must return 400 Bad Request with invalid_node_scope
		for _, badScope := range []string{"invalid_scope_value", "unknown"} {
			rec, errResp := getNodesError(t, router, "/api/v1/nodes?scope="+badScope, testAdminToken)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("scope=%s: expected 400, got %d", badScope, rec.Code)
			}
			if errResp.Code != "invalid_node_scope" {
				t.Errorf("scope=%s: expected code 'invalid_node_scope', got '%s'", badScope, errResp.Code)
			}
		}
	})

	// 7. Probe state active tests
	t.Run("probe_state_active_handling_and_guardrails", func(t *testing.T) {
		// A: Invalid probe_state values must return 400 Bad Request
		for _, badState := range []string{"idle", "foo", "probing"} {
			rec, errResp := getNodesError(t, router, "/api/v1/nodes?probe_state="+badState, testAdminToken)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("probe_state=%s: expected 400, got %d", badState, rec.Code)
			}
			if errResp.Code != "invalid_probe_state" {
				t.Errorf("probe_state=%s: expected code 'invalid_probe_state', got '%s'", badState, errResp.Code)
			}
		}

		// B: probe_state=active without probe service must return 503 Service Unavailable
		recNoSvc, errRespSvc := getNodesError(t, router, "/api/v1/nodes?probe_state=active", testAdminToken)
		if recNoSvc.Code != http.StatusServiceUnavailable {
			t.Errorf("probe_state=active without service: expected 503, got %d", recNoSvc.Code)
		}
		if errRespSvc.Code != "probe_unavailable" {
			t.Errorf("expected code 'probe_unavailable', got '%s'", errRespSvc.Code)
		}

		// C: probe_state=active with safe probe service fixture using WithNodeRepository
		runsRepo := sqlite.NewProbeRunRepository(db)
		nodeRepo := sqlite.NewNodeRepository(db)
		probeSvc := probe.NewService(runsRepo, probe.WithNodeRepository(nodeRepo))
		probeRouter := setupRegressionTestRouter(t, db, probeSvc)

		recActive, respActive := getNodesPaginated(t, probeRouter, "/api/v1/nodes?probe_state=active", testAdminToken)
		if recActive.Code != http.StatusOK {
			t.Fatalf("probe_state=active with fixture: expected 200 OK, got %d: %s", recActive.Code, recActive.Body.String())
		}
		if respActive.Data.Total != 0 {
			t.Errorf("expected total 0 for empty active pool, got %d", respActive.Data.Total)
		}
		if len(respActive.Data.Items) != 0 {
			t.Errorf("expected 0 items for empty active pool, got %d", len(respActive.Data.Items))
		}
	})
}
