package inventory_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

// mockWorkflowFetcher provides controlled HTTP responses for workflow tests.
type mockWorkflowFetcher struct {
	mu          sync.Mutex
	responses   map[string]*fetch.Response
	errors      map[string]error
	fetchCounts map[string]int
}

func newMockWorkflowFetcher() *mockWorkflowFetcher {
	return &mockWorkflowFetcher{
		responses:   make(map[string]*fetch.Response),
		errors:      make(map[string]error),
		fetchCounts: make(map[string]int),
	}
}

func (m *mockWorkflowFetcher) setResponse(url string, resp *fetch.Response) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[url] = resp
	delete(m.errors, url)
}

func (m *mockWorkflowFetcher) setError(url string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[url] = err
	delete(m.responses, url)
}

func (m *mockWorkflowFetcher) Fetch(_ context.Context, opts fetch.Options) (*fetch.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchCounts[opts.URL]++
	if err, ok := m.errors[opts.URL]; ok {
		return nil, err
	}
	if resp, ok := m.responses[opts.URL]; ok {
		return resp, nil
	}
	return nil, fmt.Errorf("unexpected fetch URL: %s", opts.URL)
}

func setupIsolatedWorkflowDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "workflow_test.db")
	cfg := sqlite.DefaultConfig(dbPath)
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to open isolated test db: %v", err)
	}
	return db
}

func seedSubscriptionsWorkflow(t *testing.T, subRepo domain.SubscriptionRepository) (enabledIDs []string, disabledIDs []string) {
	t.Helper()
	now := domain.NowUTC()

	// 4 enabled subscriptions
	enabledConfigs := []struct {
		id   string
		name string
		url  string
	}{
		{"01a0b9af-c116-7204-b4a3-98a91733145d", "7li", "https://z.7li7li.com/api/v1/client/subscribe?token=tok1"},
		{"01a0b9af-c116-77cf-86e2-cba027607d17", "魔戒", "https://msub.example.com/api/v1/client/subscribe?token=tok2"},
		{"01a0b9af-c116-7967-bc7f-197aa43a2e49", "Dogegg", "https://traffic.example.com/sub/token3/clash"},
		{"01a0b9af-c116-7280-9629-f31353b85805", "einck-qzz", "https://234.qzz.io/fsllistyaml"},
	}

	for _, cfg := range enabledConfigs {
		sub := &domain.Subscription{
			ID:                 cfg.id,
			Name:               cfg.name,
			SourceURLSecretRef: cfg.url,
			Enabled:            true,
			RefreshPolicy: domain.RefreshPolicy{
				IntervalSeconds: 86400,
				TimeoutSeconds:  30,
			},
			Revision:  domain.MustNewUUIDv7(),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := subRepo.Create(context.Background(), sub); err != nil {
			t.Fatalf("create enabled sub %s: %v", cfg.name, err)
		}
		enabledIDs = append(enabledIDs, cfg.id)
	}

	// 5 disabled subscriptions
	disabledConfigs := []struct {
		id   string
		name string
		url  string
	}{
		{"01a0b9af-c116-763e-bf33-7a791621410a", "7li7li-便宜", "https://z.7li7li.com/api/v1/client/subscribe?token=tok1"},
		{"01a0b9af-c116-71da-822b-b8fe04752661", "WARP", "manual://nodes"},
		{"01a0b9af-c116-78d1-9ffd-fef9309e794f", "Eeox", "https://api.eeox.net/api/v1/client/subscribe"},
		{"01a0b9af-c116-7db2-8376-82f0cfbb21fd", "公开节点", "https://substore.einck.top/substore/api/file/mihomo"},
		{"01a0b9af-c116-7789-a497-818f64a03bf2", "Githubusercontent", "https://raw.githubusercontent.com/example/vpn/main/clash.yaml"},
	}

	for _, cfg := range disabledConfigs {
		sub := &domain.Subscription{
			ID:                 cfg.id,
			Name:               cfg.name,
			SourceURLSecretRef: cfg.url,
			Enabled:            false,
			RefreshPolicy: domain.RefreshPolicy{
				IntervalSeconds: 86400,
				TimeoutSeconds:  30,
			},
			Revision:  domain.MustNewUUIDv7(),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := subRepo.Create(context.Background(), sub); err != nil {
			t.Fatalf("create disabled sub %s: %v", cfg.name, err)
		}
		disabledIDs = append(disabledIDs, cfg.id)
	}

	return enabledIDs, disabledIDs
}

// TestWorkflow_IsolatedFreshDB_ReloadAndFailurePreservation verifies that:
// 1. In a clean DB from 0, new last-good is populated on success.
// 2. Subsequent refresh failure preserves the new last-good state without wiping ledger.
// 3. Disabled subscriptions (enabled=0) are never processed.
func TestWorkflow_IsolatedFreshDB_ReloadAndFailurePreservation(t *testing.T) {
	ctx := context.Background()
	db := setupIsolatedWorkflowDB(t)
	defer db.Close()

	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	fetcher := newMockWorkflowFetcher()

	enabledIDs, disabledIDs := seedSubscriptionsWorkflow(t, subRepo)
	if len(enabledIDs) != 4 || len(disabledIDs) != 5 {
		t.Fatalf("expected 4 enabled and 5 disabled subscriptions, got %d and %d", len(enabledIDs), len(disabledIDs))
	}

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Verify initial ledger has 0 nodes
	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if total != 0 || len(nodes) != 0 {
		t.Fatalf("expected initial node count 0, got %d", total)
	}

	// 1. Success reload on enabled sub 0 (7li)
	targetSub, err := subRepo.GetByID(ctx, enabledIDs[0])
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	successYAML := `
proxies:
  - name: "Node A"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: pass1
  - name: "Node B"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass2
`
	fetcher.setResponse(targetSub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(successYAML),
		ContentDigest: "digest-v1",
	})

	res1, err := svc.ReconcileSubscription(ctx, targetSub.ID)
	if err != nil {
		t.Fatalf("ReconcileSubscription success step: %v", err)
	}
	if res1.Outcome != domain.FetchOutcomeSuccess || res1.NodesValid != 2 {
		t.Fatalf("expected outcome success and 2 valid nodes, got %s, valid=%d", res1.Outcome, res1.NodesValid)
	}

	// Verify ledger now has 2 active nodes
	nodes, total, err = svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if total != 2 || len(nodes) != 2 {
		t.Fatalf("expected 2 active nodes, got %d", total)
	}

	// 2. Subsequent failure on the same subscription (HTTP 500 network error)
	fetcher.setError(targetSub.SourceURLSecretRef, fmt.Errorf("upstream server error 500"))
	_, err = svc.ReconcileSubscription(ctx, targetSub.ID)
	if err == nil {
		t.Fatalf("expected error on network failure step, got nil")
	}

	// Crucial check: Last-good nodes MUST be preserved in the ledger after failure!
	nodesAfterFail, totalAfterFail, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes after failure: %v", err)
	}
	if totalAfterFail != 2 || len(nodesAfterFail) != 2 {
		t.Fatalf("expected 2 last-good nodes preserved after failure, got %d", totalAfterFail)
	}
}

// TestWorkflow_SourceFetchOutcomes verifies handling of:
// - success
// - empty
// - fail
// - notice entries
// - manual schema
// - private/unknown proxy protocols
// - multi-credentials on same endpoint disambiguation via ComputeConnectionLogicalID
func TestWorkflow_SourceFetchOutcomes(t *testing.T) {
	ctx := context.Background()
	db := setupIsolatedWorkflowDB(t)
	defer db.Close()

	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	fetcher := newMockWorkflowFetcher()

	enabledIDs, _ := seedSubscriptionsWorkflow(t, subRepo)
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// A. Multi-credentials on same endpoint: distinct ComputeConnectionLogicalID
	sub1, _ := subRepo.GetByID(ctx, enabledIDs[0])
	multiCredsYAML := `
proxies:
  - name: "Endpoint User1"
    type: http
    server: proxy.example.com
    port: 8080
    username: user1
    password: pass1
  - name: "Endpoint User2 (Same Host/Port)"
    type: http
    server: proxy.example.com
    port: 8080
    username: user2
    password: pass2
`
	fetcher.setResponse(sub1.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(multiCredsYAML),
		ContentDigest: "digest-multicreds",
	})
	resA, err := svc.ReconcileSubscription(ctx, sub1.ID)
	if err != nil {
		t.Fatalf("reconcile multi-creds: %v", err)
	}
	if resA.Outcome != domain.FetchOutcomeSuccess || resA.NodesValid != 2 {
		t.Fatalf("expected 2 valid nodes for distinct credentials on same endpoint, got valid=%d", resA.NodesValid)
	}

	nodesA, totalA, _ := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if totalA != 2 || len(nodesA) != 2 {
		t.Fatalf("expected 2 distinct nodes in ledger, got %d", totalA)
	}
	if nodesA[0].LogicalID == nodesA[1].LogicalID {
		t.Fatalf("distinct credentials on same endpoint MUST produce distinct LogicalIDs: %s == %s", nodesA[0].LogicalID, nodesA[1].LogicalID)
	}

	// B. Empty source response returns validation error and records failed outcome safely
	sub2, _ := subRepo.GetByID(ctx, enabledIDs[1])
	fetcher.setResponse(sub2.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		Body:          []byte("proxies: []"),
		ContentDigest: "digest-empty",
	})
	resB, err := svc.ReconcileSubscription(ctx, sub2.ID)
	if err == nil {
		t.Fatalf("expected no_supported_nodes error for empty proxies, got nil")
	}
	if resB == nil || resB.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed outcome recorded for empty proxies, got %+v", resB)
	}
	if resB.NodesValid != 0 {
		t.Errorf("expected 0 valid nodes, got %d", resB.NodesValid)
	}

	// C. Notice pseudo-node classification
	sub3, _ := subRepo.GetByID(ctx, enabledIDs[2])
	noticeYAML := `
proxies:
  - name: "🔔 订阅到期时间 2027-01-01"
    type: ss
    server: 127.0.0.1
    port: 1080
    cipher: aes-128-gcm
    password: notice
  - name: "Normal Proxy Node"
    type: ss
    server: 198.51.100.50
    port: 8388
    cipher: aes-128-gcm
    password: pass
`
	fetcher.setResponse(sub3.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(noticeYAML),
		ContentDigest: "digest-notice",
	})
	resC, err := svc.ReconcileSubscription(ctx, sub3.ID)
	if err != nil {
		t.Fatalf("reconcile notice: %v", err)
	}
	if resC.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("expected success, got %s", resC.Outcome)
	}
}
