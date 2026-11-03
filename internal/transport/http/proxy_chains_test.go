package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clash-sub-parser/internal/application/proxychain"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
	transporthttp "clash-sub-parser/internal/transport/http"
	"clash-sub-parser/migrations"
)

type dummyChainResolver struct{}

func (d dummyChainResolver) Resolve(ctx context.Context, input resolver.ResolveInput) (*resolver.ResolvedPolicySnapshot, error) {
	nodes := make([]resolver.ResolvedNode, 0)
	for _, n := range input.Nodes {
		dp := ""
		for _, b := range input.ChainBindings {
			if b.Enabled && b.TargetType == domain.ProxyChainTargetNode && b.TargetID == n.LogicalID {
				dp = b.DialerID
				break
			}
		}
		nodes = append(nodes, resolver.ResolvedNode{
			LogicalID:   n.LogicalID,
			DisplayName: n.DisplayName,
			DialerProxy: dp,
		})
	}
	return &resolver.ResolvedPolicySnapshot{
		Nodes: nodes,
		ChainEffects: []resolver.ChainEffect{
			{BindingID: "test-effect", TargetCount: len(nodes), ChainCount: 1},
		},
	}, nil
}

func setupTestChainRouter(t *testing.T) (http.Handler, *sqlite.ReadinessReport) {
	t.Helper()
	db := newCleanSQLiteDB(t)

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	chainRepo := sqlite.NewProxyChainRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	settingsRepo := sqlite.NewSettingsRepository(db)

	chainSvc := proxychain.NewService(
		chainRepo,
		nodeRepo,
		policyRepo,
		subRepo,
		proxychain.WithAuditRepository(auditRepo),
		proxychain.WithResolver(dummyChainResolver{}),
	)
	chainSvc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{
			Nodes: []domain.Node{
				{LogicalID: "node-1", DisplayName: "Node 1", Active: true},
				{LogicalID: "node-2", DisplayName: "Node 2", Active: true},
			},
		}, nil
	})

	store := transporthttp.NewMemorySessionStore(time.Hour)
	holder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		SettingsRepo: settingsRepo,
		SessionStore: store,
		HashCost:     transporthttp.MinHashCost,
	})

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		TokenHolder:          holder,
		SettingsRepository:   settingsRepo,
		AuditRepository:      auditRepo,
		SessionStore:         store,
		ProxyChainRepository: chainRepo,
		ProxyChainService:    chainSvc,
	})

	return router, nil
}

func TestProxyChainsHTTP_FullLifecycle(t *testing.T) {
	router, _ := setupTestChainRouter(t)

	request := func(method, path, payload string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1. Initial GET /api/v1/proxy-chains returns empty list with deterministic version
	t.Run("initial GET returns empty list", func(t *testing.T) {
		rec := request(http.MethodGet, "/api/v1/proxy-chains", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var env struct {
			Data struct {
				Bindings []domain.ProxyChainBinding `json:"bindings"`
				Version  string                     `json:"version"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(env.Data.Bindings) != 0 {
			t.Fatalf("expected 0 bindings, got %d", len(env.Data.Bindings))
		}
		if env.Data.Version == "" {
			t.Fatal("expected non-empty deterministic initial version")
		}
	})

	// 2. GET /api/v1/proxy-chains/options returns 200 with non-nil arrays
	t.Run("options GET returns 200", func(t *testing.T) {
		rec := request(http.MethodGet, "/api/v1/proxy-chains/options", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var env struct {
			Data proxychain.OptionsResult `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if env.Data.Nodes == nil || env.Data.Groups == nil || env.Data.Subscriptions == nil {
			t.Fatal("expected non-nil slices in options result")
		}
	})

	// 3. POST /api/v1/proxy-chains/preview validates without writing
	var initialVersion string
	t.Run("preview POST", func(t *testing.T) {
		getRec := request(http.MethodGet, "/api/v1/proxy-chains", "")
		var getEnv struct {
			Data struct {
				Version string `json:"version"`
			} `json:"data"`
		}
		_ = json.Unmarshal(getRec.Body.Bytes(), &getEnv)
		initialVersion = getEnv.Data.Version

		// Preview with stale version fails 409
		staleBody := `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"stale-v"}`
		rec409 := request(http.MethodPost, "/api/v1/proxy-chains/preview", staleBody)
		if rec409.Code != http.StatusConflict {
			t.Fatalf("expected 409 conflict, got %d: %s", rec409.Code, rec409.Body.String())
		}

		// Valid preview succeeds
		validBody := `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"` + initialVersion + `"}`
		rec200 := request(http.MethodPost, "/api/v1/proxy-chains/preview", validBody)
		if rec200.Code != http.StatusOK {
			t.Fatalf("expected 200 ok, got %d: %s", rec200.Code, rec200.Body.String())
		}
	})

	// 4. POST /api/v1/proxy-chains creates binding with 201
	var createdID string
	var newVersion string
	t.Run("create binding POST", func(t *testing.T) {
		// Nonempty client ID rejected with 422
		badBody := `{"binding":{"id":"custom-id","target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"` + initialVersion + `"}`
		rec422 := request(http.MethodPost, "/api/v1/proxy-chains", badBody)
		if rec422.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec422.Code, rec422.Body.String())
		}

		// Valid creation
		validBody := `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true,"sort_order":10,"note":"chain test"},"config_version":"` + initialVersion + `"}`
		rec201 := request(http.MethodPost, "/api/v1/proxy-chains", validBody)
		if rec201.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec201.Code, rec201.Body.String())
		}

		var env struct {
			Data struct {
				Binding domain.ProxyChainBinding `json:"binding"`
				Version string                   `json:"version"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec201.Body.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if env.Data.Binding.ID == "" {
			t.Fatal("expected server-generated ID")
		}
		if env.Data.Binding.Revision != 1 {
			t.Fatalf("expected revision 1, got %d", env.Data.Binding.Revision)
		}
		createdID = env.Data.Binding.ID
		newVersion = env.Data.Version
	})

	// 5. PUT /api/v1/proxy-chains/{id} updates binding with revision increment
	t.Run("update binding PUT", func(t *testing.T) {
		// Inconsistent ID rejected (422)
		mismatchBody := `{"binding":{"id":"different-id","target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"` + newVersion + `","expected_revision":1}`
		recMismatch := request(http.MethodPut, "/api/v1/proxy-chains/"+createdID, mismatchBody)
		if recMismatch.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for ID mismatch, got %d: %s", recMismatch.Code, recMismatch.Body.String())
		}

		// Revision mismatch rejected (409)
		revMismatchBody := `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"` + newVersion + `","expected_revision":99}`
		recRevMismatch := request(http.MethodPut, "/api/v1/proxy-chains/"+createdID, revMismatchBody)
		if recRevMismatch.Code != http.StatusConflict {
			t.Fatalf("expected 409 for revision mismatch, got %d: %s", recRevMismatch.Code, recRevMismatch.Body.String())
		}

		// Valid update
		validUpdateBody := `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true,"sort_order":20,"note":"updated note"},"config_version":"` + newVersion + `","expected_revision":1}`
		recUpdate := request(http.MethodPut, "/api/v1/proxy-chains/"+createdID, validUpdateBody)
		if recUpdate.Code != http.StatusOK {
			t.Fatalf("expected 200 ok for update, got %d: %s", recUpdate.Code, recUpdate.Body.String())
		}

		var updateEnv struct {
			Data struct {
				Binding domain.ProxyChainBinding `json:"binding"`
				Version string                   `json:"version"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recUpdate.Body.Bytes(), &updateEnv); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if updateEnv.Data.Binding.Revision != 2 {
			t.Fatalf("expected revision 2, got %d", updateEnv.Data.Binding.Revision)
		}
		newVersion = updateEnv.Data.Version
	})

	// 6. DELETE /api/v1/proxy-chains/{id} deletes binding
	t.Run("delete binding DELETE", func(t *testing.T) {
		// Missing binding returns 404
		rec404 := request(http.MethodDelete, "/api/v1/proxy-chains/missing-id", `{"config_version":"`+newVersion+`","expected_revision":1}`)
		if rec404.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec404.Code, rec404.Body.String())
		}

		// Valid delete returns 200
		rec200 := request(http.MethodDelete, "/api/v1/proxy-chains/"+createdID, `{"config_version":"`+newVersion+`","expected_revision":2}`)
		if rec200.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec200.Code, rec200.Body.String())
		}

		// Verify GET returns empty list
		getRec := request(http.MethodGet, "/api/v1/proxy-chains", "")
		var getEnv struct {
			Data struct {
				Bindings []domain.ProxyChainBinding `json:"bindings"`
			} `json:"data"`
		}
		_ = json.Unmarshal(getRec.Body.Bytes(), &getEnv)
		if len(getEnv.Data.Bindings) != 0 {
			t.Fatalf("expected 0 bindings after delete, got %d", len(getEnv.Data.Bindings))
		}
	})
}

func TestProxyChainsHTTP_CASAndValidationTable(t *testing.T) {
	router, _ := setupTestChainRouter(t)

	request := func(method, path, payload string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// Fetch initial version
	getRec := request(http.MethodGet, "/api/v1/proxy-chains", "")
	var getEnv struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(getRec.Body.Bytes(), &getEnv)
	initialVersion := getEnv.Data.Version

	// Create a binding to test Update and Delete
	validCreatePayload := fmt.Sprintf(`{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true,"sort_order":1},"config_version":"%s"}`, initialVersion)
	createRec := request(http.MethodPost, "/api/v1/proxy-chains", validCreatePayload)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("failed to setup binding: %s", createRec.Body.String())
	}
	var createEnv struct {
		Data struct {
			Binding domain.ProxyChainBinding `json:"binding"`
			Version string                   `json:"version"`
		} `json:"data"`
	}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createEnv)
	bindingID := createEnv.Data.Binding.ID
	currentVersion := createEnv.Data.Version

	type testCase struct {
		name           string
		method         string
		path           string
		payload        string
		expectedStatus int
	}

	// 1. Table tests for Preview, Create, Update, Delete: bad JSON, missing/blank config_version, revisions, query bypass
	tests := []testCase{
		// Preview
		{
			name:           "preview bad JSON syntax returns 400",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains/preview",
			payload:        `{"binding": {invalid`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "preview empty body returns 400",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains/preview",
			payload:        "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "preview missing config_version returns 422",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains/preview",
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true}}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "preview empty config_version returns 422",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains/preview",
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":""}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "preview whitespace config_version returns 422",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains/preview",
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"   "}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "preview query config_version with empty body does not bypass (400)",
			method:         http.MethodPost,
			path:           fmt.Sprintf("/api/v1/proxy-chains/preview?config_version=%s", currentVersion),
			payload:        "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "preview stale config_version returns 409",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains/preview",
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"stale-v"}`,
			expectedStatus: http.StatusConflict,
		},

		// Create
		{
			name:           "create bad JSON syntax returns 400",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains",
			payload:        `{"binding": {bad`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "create empty body returns 400",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains",
			payload:        "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "create missing config_version returns 422",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains",
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true}}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "create empty config_version returns 422",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains",
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":""}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "create whitespace config_version returns 422",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains",
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":" \t "}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "create query config_version with empty body does not bypass (400)",
			method:         http.MethodPost,
			path:           fmt.Sprintf("/api/v1/proxy-chains?config_version=%s", currentVersion),
			payload:        "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "create stale config_version returns 409",
			method:         http.MethodPost,
			path:           "/api/v1/proxy-chains",
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"stale-v"}`,
			expectedStatus: http.StatusConflict,
		},

		// Update
		{
			name:           "update bad JSON syntax returns 400",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"binding": {bad`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "update empty body returns 400",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "update missing config_version returns 422",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"expected_revision":1}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "update empty config_version returns 422",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"","expected_revision":1}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "update whitespace config_version returns 422",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"   ","expected_revision":1}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "update missing expected_revision returns 422",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        fmt.Sprintf(`{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"%s"}`, currentVersion),
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "update expected_revision = 0 returns 422",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        fmt.Sprintf(`{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"%s","expected_revision":0}`, currentVersion),
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "update expected_revision negative returns 422",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        fmt.Sprintf(`{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"%s","expected_revision":-1}`, currentVersion),
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "update query params with empty body does not bypass (400)",
			method:         http.MethodPut,
			path:           fmt.Sprintf("/api/v1/proxy-chains/%s?config_version=%s&expected_revision=1", bindingID, currentVersion),
			payload:        "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "update stale config_version returns 409",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"stale-v","expected_revision":1}`,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "update stale expected_revision returns 409",
			method:         http.MethodPut,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        fmt.Sprintf(`{"binding":{"target_type":"node","target_id":"node-1","dialer_type":"node","dialer_id":"node-2","enabled":true},"config_version":"%s","expected_revision":99}`, currentVersion),
			expectedStatus: http.StatusConflict,
		},

		// Delete
		{
			name:           "delete bad JSON syntax returns 400",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{invalid json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "delete empty body returns 400 (no EOF bypass)",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "delete query params alone with empty body does not bypass (400)",
			method:         http.MethodDelete,
			path:           fmt.Sprintf("/api/v1/proxy-chains/%s?config_version=%s&expected_revision=1", bindingID, currentVersion),
			payload:        "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "delete empty JSON object returns 422",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "delete missing config_version returns 422",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"expected_revision":1}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "delete empty config_version returns 422",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"config_version":"","expected_revision":1}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "delete whitespace config_version returns 422",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"config_version":"   ","expected_revision":1}`,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "delete missing expected_revision returns 422",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        fmt.Sprintf(`{"config_version":"%s"}`, currentVersion),
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "delete expected_revision = 0 returns 422",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        fmt.Sprintf(`{"config_version":"%s","expected_revision":0}`, currentVersion),
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "delete expected_revision negative returns 422",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        fmt.Sprintf(`{"config_version":"%s","expected_revision":-1}`, currentVersion),
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "delete query param does not satisfy missing body field (422)",
			method:         http.MethodDelete,
			path:           fmt.Sprintf("/api/v1/proxy-chains/%s?expected_revision=1", bindingID),
			payload:        fmt.Sprintf(`{"config_version":"%s"}`, currentVersion),
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "delete stale config_version returns 409",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        `{"config_version":"stale-v","expected_revision":1}`,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "delete stale expected_revision returns 409",
			method:         http.MethodDelete,
			path:           "/api/v1/proxy-chains/" + bindingID,
			payload:        fmt.Sprintf(`{"config_version":"%s","expected_revision":99}`, currentVersion),
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := request(tc.method, tc.path, tc.payload)
			if rec.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}
}
