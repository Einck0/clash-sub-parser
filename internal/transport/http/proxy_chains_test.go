package http_test

import (
	"bytes"
	"context"
	"encoding/json"
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
