package http_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

type nodePaginatedResponse struct {
	Data struct {
		Items    []inventory.NodeView `json:"items"`
		Page     int                  `json:"page"`
		PageSize int                  `json:"page_size"`
		Total    int                  `json:"total"`
	} `json:"data"`
}

type nodeDetailResponse struct {
	Data struct {
		Node    inventory.NodeView  `json:"node"`
		Sources []domain.NodeSource `json:"sources"`
	} `json:"data"`
}

func setupNodesTestRouter(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()
	routerCfg := newTestRouterConfig(true)
	routerCfg.InventoryService = inventory.NewService(
		db,
		sqlite.NewSubscriptionRepository(db),
		sqlite.NewSubscriptionFetchRepository(db),
		sqlite.NewNodeRepository(db),
		sqlite.NewNodeSourceRepository(db),
		nil,
	)
	return transporthttp.NewRouter(routerCfg)
}

func seed10000Nodes(t *testing.T, db *sql.DB) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	nodeStmt, err := tx.Prepare(`
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		t.Fatalf("failed to prepare node stmt: %v", err)
	}
	defer nodeStmt.Close()

	sourceStmt, err := tx.Prepare(`
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		t.Fatalf("failed to prepare source stmt: %v", err)
	}
	defer sourceStmt.Close()

	// Ensure parent subscription exists for foreign keys
	_, err = tx.Exec(`
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES ('sub-001', 'Test Sub 1', 'secret://test/1', 1, 'rev-1', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z'),
		       ('sub-002', 'Test Sub 2', 'secret://test/2', 1, 'rev-2', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test subscriptions: %v", err)
	}

	protocols := []domain.Protocol{
		domain.ProtocolSS,
		domain.ProtocolVMess,
		domain.ProtocolVLESS,
		domain.ProtocolTrojan,
		domain.ProtocolHysteria2,
		domain.ProtocolWireGuard,
		domain.ProtocolTUIC,
	}

	now := time.Now().UTC()
	for i := 1; i <= 10000; i++ {
		logicalID := fmt.Sprintf("node-logical-%05d", i)
		proto := protocols[i%len(protocols)]

		region := "US"
		if i%4 == 1 {
			region = "HK"
		} else if i%4 == 2 {
			region = "JP"
		} else if i%4 == 3 {
			region = "SG"
		}

		displayName := fmt.Sprintf("Node-%s-%05d", region, i)
		secretRef := fmt.Sprintf("secret://credentials/node-%05d?token=super-secret-pw-%05d", i, i)

		// 7,000 active, 3,000 inactive
		active := 1
		if i > 7000 {
			active = 0
		}

		timestamp := now.Add(time.Duration(i) * time.Second).Format(time.RFC3339)

		if _, err := nodeStmt.Exec(logicalID, string(proto), displayName, secretRef, active, timestamp, timestamp); err != nil {
			t.Fatalf("failed to insert node %d: %v", i, err)
		}

		// Attach provenance sources to node 1
		if i == 1 {
			if _, err := sourceStmt.Exec(logicalID, "sub-001", "fetch-001"); err != nil {
				t.Fatalf("failed to insert source 1 for node %d: %v", i, err)
			}
			if _, err := sourceStmt.Exec(logicalID, "sub-002", "fetch-002"); err != nil {
				t.Fatalf("failed to insert source 2 for node %d: %v", i, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit 10,000 seed nodes: %v", err)
	}
}

func newCleanSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg := sqlite.Config{
		Path:            fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()),
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNodeDetailIncludesAPISafeRiskSummary(t *testing.T) {
	db := newCleanSQLiteDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	nodeID := "node_0123456789abcdef"

	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?);`,
		nodeID, string(domain.ProtocolSS), "Risk detail node", "secret://node/private", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}

	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	if err := providerRepo.Upsert(ctx, &domain.IPRiskProviderSettings{
		Provider:           "fixture",
		SchemaVersion:      "v1",
		Enabled:            true,
		SecretReference:    "secret://provider/private",
		MaxConcurrency:     1,
		RequestsPerMinute:  60,
		DailyRequestBudget: 100,
		PerRequestTimeout:  time.Second,
		MaxResponseBytes:   1024,
	}); err != nil {
		t.Fatalf("insert provider settings: %v", err)
	}

	policyID := domain.MustNewUUIDv7()
	policyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	if err := policyRepo.Create(ctx, &domain.RiskPolicyRevision{
		RiskPolicy: domain.RiskPolicy{
			RevisionID: policyID,
			ProviderSelection: domain.RiskProviderSelection{
				Mode:      domain.RiskFusionSingleProvider,
				Providers: []domain.ProviderRef{{Provider: "fixture", SchemaVersion: "v1"}},
			},
			MaxObservationAge: time.Hour,
			MinimumConfidence: 50,
			ScoreBands: []domain.ScoreBand{
				{Min: 0, Max: 49, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
				{Min: 50, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
			},
			UnknownAction: domain.RiskActionReview,
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if err := policyRepo.SetActive(ctx, policyID, true); err != nil {
		t.Fatalf("activate policy: %v", err)
	}

	score, confidence := 90, 90
	observationRepo := sqlite.NewIPRiskObservationRepository(db)
	if err := observationRepo.Create(ctx, &domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         nodeID,
		ExitIdentityDigest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Provider:              "fixture",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score,
		Confidence:            &confidence,
		NetworkClass:          domain.NetworkClassDatacenter,
		AnonymizerTraits:      []domain.AnonymizerTrait{},
		EvidenceDigest:        "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		RedactedSummary:       "redacted fixture summary",
	}); err != nil {
		t.Fatalf("create observation: %v", err)
	}

	router := setupNodesTestRouter(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+nodeID, nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var response nodeDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode node detail: %v", err)
	}
	if response.Data.Node.IPRiskSummary == nil {
		t.Fatalf("expected API-safe risk summary in node detail response")
	}
	if response.Data.Node.IPRiskSummary.Decision != domain.RiskActionBlock {
		t.Fatalf("expected block decision, got %s", response.Data.Node.IPRiskSummary.Decision)
	}
	if strings.Contains(rec.Body.String(), "secret://") || strings.Contains(rec.Body.String(), "198.51.100.") {
		t.Fatalf("node detail response leaked sensitive data: %s", rec.Body.String())
	}
}

func Test10000NodesServerSidePaginationAndBoundaryContracts(t *testing.T) {
	db := newCleanSQLiteDB(t)
	seed10000Nodes(t, db)
	router := setupNodesTestRouter(t, db)

	// 1. Database layer pagination: page=1, page_size=50
	t.Run("server_side_pagination_first_page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?page=1&page_size=50", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		// Check payload size is small (never a full 10,000 dump)
		bodyBytes := rec.Body.Bytes()
		if len(bodyBytes) > 50*1024 {
			t.Fatalf("response payload too large (%d bytes), full ledger dump suspected", len(bodyBytes))
		}

		var resp nodePaginatedResponse
		if err := json.Unmarshal(bodyBytes, &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}

		if resp.Data.Page != 1 {
			t.Errorf("expected page 1, got %d", resp.Data.Page)
		}
		if resp.Data.PageSize != 50 {
			t.Errorf("expected page_size 50, got %d", resp.Data.PageSize)
		}
		if resp.Data.Total != 10000 {
			t.Errorf("expected total 10000, got %d", resp.Data.Total)
		}
		if len(resp.Data.Items) != 50 {
			t.Errorf("expected 50 items, got %d", len(resp.Data.Items))
		}

		// Verify zero secrets leaked in items
		bodyStr := string(bodyBytes)
		if strings.Contains(bodyStr, "secret://") || strings.Contains(bodyStr, "super-secret-pw") {
			t.Errorf("secret reference leaked in node response: %s", bodyStr)
		}
		if strings.Contains(bodyStr, "normalized_config_secret_ref") {
			t.Errorf("normalized_config_secret_ref key exposed in node response")
		}

		first := resp.Data.Items[0]
		if first.LogicalID == "" || first.DisplayName == "" || first.Protocol == "" {
			t.Errorf("expected valid node fields, got %#v", first)
		}
	})

	// 2. Truncation: page_size > 100 is strictly capped at 100
	t.Run("page_size_exceeding_100_truncated_to_100", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?page=1&page_size=500", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp nodePaginatedResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}

		if resp.Data.PageSize != 100 {
			t.Errorf("expected page_size truncated to 100, got %d", resp.Data.PageSize)
		}
		if len(resp.Data.Items) != 100 {
			t.Errorf("expected items length 100, got %d", len(resp.Data.Items))
		}
		if resp.Data.Total != 10000 {
			t.Errorf("expected total 10000, got %d", resp.Data.Total)
		}

		// Also verify page_size=101
		req101 := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?page=2&page_size=101", nil)
		req101.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec101 := httptest.NewRecorder()
		router.ServeHTTP(rec101, req101)

		if rec101.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for page_size=101, got %d", rec101.Code)
		}
		var resp101 nodePaginatedResponse
		_ = json.Unmarshal(rec101.Body.Bytes(), &resp101)
		if resp101.Data.PageSize != 100 {
			t.Errorf("expected page_size=101 truncated to 100, got %d", resp101.Data.PageSize)
		}
		if len(resp101.Data.Items) != 100 {
			t.Errorf("expected items length 100, got %d", len(resp101.Data.Items))
		}
	})

	// 3. Multi-page switching: total remains stable and offsets are distinct
	t.Run("multi_page_switching_total_stable", func(t *testing.T) {
		testPages := []int{1, 2, 5, 20, 50, 100, 200}
		var page1IDs map[string]bool
		var page2IDs map[string]bool

		for _, p := range testPages {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/nodes?page=%d&page_size=50", p), nil)
			req.Header.Set("Authorization", "Bearer "+testAdminToken)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("page %d: expected 200 OK, got %d", p, rec.Code)
			}

			var resp nodePaginatedResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("page %d: failed to parse JSON: %v", p, err)
			}

			if resp.Data.Total != 10000 {
				t.Errorf("page %d: expected total 10000, got %d", p, resp.Data.Total)
			}
			if resp.Data.Page != p {
				t.Errorf("expected page %d, got %d", p, resp.Data.Page)
			}
			if len(resp.Data.Items) != 50 {
				t.Errorf("page %d: expected 50 items, got %d", p, len(resp.Data.Items))
			}

			if p == 1 {
				page1IDs = make(map[string]bool)
				for _, it := range resp.Data.Items {
					page1IDs[it.LogicalID] = true
				}
			} else if p == 2 {
				page2IDs = make(map[string]bool)
				for _, it := range resp.Data.Items {
					page2IDs[it.LogicalID] = true
				}
			}
		}

		// Ensure page 1 and page 2 items are disjoint
		for id := range page1IDs {
			if page2IDs[id] {
				t.Errorf("duplicate node logical_id %s found between page 1 and page 2", id)
			}
		}
	})

	// 4. Filtering: total correctly reflects filtered subset
	t.Run("filter_active_only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?active_only=true&page=1&page_size=50", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var resp nodePaginatedResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		if resp.Data.Total != 7000 {
			t.Errorf("expected filtered total 7000, got %d", resp.Data.Total)
		}
		for _, item := range resp.Data.Items {
			if !item.Active {
				t.Errorf("expected active node, got inactive node: %#v", item)
			}
		}
	})

	t.Run("filter_by_protocol", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?protocol=vmess&page=1&page_size=50", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var resp nodePaginatedResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		if resp.Data.Total == 0 || resp.Data.Total >= 10000 {
			t.Errorf("expected protocol-filtered total to be subset, got %d", resp.Data.Total)
		}
		for _, item := range resp.Data.Items {
			if item.Protocol != domain.ProtocolVMess {
				t.Errorf("expected vmess protocol, got %s", item.Protocol)
			}
		}
	})

	t.Run("filter_by_search", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?search=US&page=1&page_size=50", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var resp nodePaginatedResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		if resp.Data.Total == 0 || resp.Data.Total >= 10000 {
			t.Errorf("expected search-filtered total, got %d", resp.Data.Total)
		}
		for _, item := range resp.Data.Items {
			if !strings.Contains(item.DisplayName, "US") {
				t.Errorf("expected display_name to contain 'US', got %s", item.DisplayName)
			}
		}
	})

	t.Run("filter_combined", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?protocol=vmess&active_only=true&search=HK&page=1&page_size=50", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var resp nodePaginatedResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		if resp.Data.Total == 0 {
			t.Errorf("expected non-zero combined filtered total")
		}
		for _, item := range resp.Data.Items {
			if item.Protocol != domain.ProtocolVMess || !item.Active || !strings.Contains(item.DisplayName, "HK") {
				t.Errorf("node does not match combined filters: %#v", item)
			}
		}
	})

	// 5. Node Detail endpoint: GET /api/v1/nodes/{logical_id}
	t.Run("node_detail_success_with_sources", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-logical-00001", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		bodyBytes := rec.Body.Bytes()
		bodyStr := string(bodyBytes)
		if strings.Contains(bodyStr, "secret://") || strings.Contains(bodyStr, "super-secret-pw") {
			t.Errorf("secret leaked in node detail: %s", bodyStr)
		}
		if strings.Contains(bodyStr, "normalized_config_secret_ref") {
			t.Errorf("normalized_config_secret_ref key exposed in node detail")
		}

		var resp nodeDetailResponse
		if err := json.Unmarshal(bodyBytes, &resp); err != nil {
			t.Fatalf("failed to parse node detail JSON: %v", err)
		}

		if resp.Data.Node.LogicalID != "node-logical-00001" {
			t.Errorf("expected node logical_id 'node-logical-00001', got %s", resp.Data.Node.LogicalID)
		}
		if len(resp.Data.Sources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(resp.Data.Sources))
		}
		if resp.Data.Sources[0].SubscriptionID != "sub-001" && resp.Data.Sources[0].SubscriptionID != "sub-002" {
			t.Errorf("unexpected source: %#v", resp.Data.Sources[0])
		}
	})

	t.Run("node_detail_not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-logical-99999", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d: %s", rec.Code, rec.Body.String())
		}

		var errResp transporthttp.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to parse error JSON: %v", err)
		}
		if errResp.Code != "node_not_found" {
			t.Errorf("expected error code 'node_not_found', got '%s'", errResp.Code)
		}
	})

	// 6. Auth Protection
	t.Run("auth_required", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?page=1&page_size=50", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	// 7. Validation errors for invalid pagination params
	t.Run("invalid_page_returns_422", func(t *testing.T) {
		invalidPages := []string{"0", "-1", "abc"}
		for _, ip := range invalidPages {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/nodes?page=%s", ip), nil)
			req.Header.Set("Authorization", "Bearer "+testAdminToken)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("page=%s: expected 422 Unprocessable Entity, got %d", ip, rec.Code)
			}
		}
	})

	t.Run("invalid_page_size_returns_422", func(t *testing.T) {
		invalidSizes := []string{"0", "-5", "xyz"}
		for _, is := range invalidSizes {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/nodes?page_size=%s", is), nil)
			req.Header.Set("Authorization", "Bearer "+testAdminToken)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("page_size=%s: expected 422 Unprocessable Entity, got %d", is, rec.Code)
			}
		}
	})

	// 8. Sorting contract
	t.Run("sorting_display_name", func(t *testing.T) {
		reqAsc := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?sort_by=display_name&sort_order=asc&page=1&page_size=10", nil)
		reqAsc.Header.Set("Authorization", "Bearer "+testAdminToken)
		recAsc := httptest.NewRecorder()
		router.ServeHTTP(recAsc, reqAsc)

		if recAsc.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recAsc.Code)
		}

		var respAsc nodePaginatedResponse
		_ = json.Unmarshal(recAsc.Body.Bytes(), &respAsc)

		reqDesc := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?sort_by=display_name&sort_order=desc&page=1&page_size=10", nil)
		reqDesc.Header.Set("Authorization", "Bearer "+testAdminToken)
		recDesc := httptest.NewRecorder()
		router.ServeHTTP(recDesc, reqDesc)

		if recDesc.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recDesc.Code)
		}

		var respDesc nodePaginatedResponse
		_ = json.Unmarshal(recDesc.Body.Bytes(), &respDesc)

		if len(respAsc.Data.Items) > 0 && len(respDesc.Data.Items) > 0 {
			if respAsc.Data.Items[0].DisplayName == respDesc.Data.Items[0].DisplayName {
				t.Errorf("expected different top items for ASC and DESC sort")
			}
		}
	})

	// 9. Concurrent read race safety
	t.Run("concurrent_reads_race_safety", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				page := (idx % 3) + 1
				req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/nodes?page=%d&page_size=25", page), nil)
				req.Header.Set("Authorization", "Bearer "+testAdminToken)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Errorf("concurrent request %d failed with code %d", idx, rec.Code)
				}
			}(i)
		}
		wg.Wait()
	})
}

func BenchmarkNodesPagination(b *testing.B) {
	db := newCleanSQLiteDBBenchmark(b)
	seed10000NodesBenchmark(b, db)

	routerCfg := newTestRouterConfig(true)
	routerCfg.InventoryService = inventory.NewService(
		db,
		sqlite.NewSubscriptionRepository(db),
		sqlite.NewSubscriptionFetchRepository(db),
		sqlite.NewNodeRepository(db),
		sqlite.NewNodeSourceRepository(db),
		nil,
	)
	router := transporthttp.NewRouter(routerCfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?page=1&page_size=50", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("benchmark request failed with code %d", rec.Code)
		}
	}
}

func newCleanSQLiteDBBenchmark(b *testing.B) *sql.DB {
	b.Helper()
	cfg := sqlite.Config{
		Path:            fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name()),
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		b.Fatalf("OpenAndMigrate error: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })
	return db
}

func seed10000NodesBenchmark(b *testing.B, db *sql.DB) {
	b.Helper()
	tx, err := db.Begin()
	if err != nil {
		b.Fatalf("failed to begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	nodeStmt, err := tx.Prepare(`
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		b.Fatalf("failed to prepare stmt: %v", err)
	}
	defer nodeStmt.Close()

	now := time.Now().UTC()
	for i := 1; i <= 10000; i++ {
		logicalID := fmt.Sprintf("bench-node-%05d", i)
		displayName := fmt.Sprintf("Bench-Node-%05d", i)
		secretRef := fmt.Sprintf("secret://bench/%05d", i)
		ts := now.Format(time.RFC3339)
		if _, err := nodeStmt.Exec(logicalID, "vmess", displayName, secretRef, 1, ts, ts); err != nil {
			b.Fatalf("failed to insert bench node %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("failed to commit bench seed: %v", err)
	}
}
