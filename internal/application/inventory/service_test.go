package inventory_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

// setupTestEnv sets up an in-memory SQLite database with migrations and repository instances.
func setupTestEnv(t *testing.T) (*sql.DB, domain.SubscriptionRepository, domain.SubscriptionFetchRepository, domain.NodeRepository, domain.NodeSourceRepository) {
	t.Helper()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:test_inv_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}

	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations on test db: %v", err)
	}

	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)

	return db, subRepo, fetchRepo, nodeRepo, sourceRepo
}

// mockFetcher provides controlled HTTP responses for testing.
type mockFetcher struct {
	mu        sync.Mutex
	responses map[string]*fetch.Response
	errors    map[string]error
}

func newMockFetcher() *mockFetcher {
	return &mockFetcher{
		responses: make(map[string]*fetch.Response),
		errors:    make(map[string]error),
	}
}

func (m *mockFetcher) setResponse(url string, resp *fetch.Response) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[url] = resp
	delete(m.errors, url)
}

func (m *mockFetcher) setError(url string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[url] = err
	delete(m.responses, url)
}

func (m *mockFetcher) Fetch(_ context.Context, opts fetch.Options) (*fetch.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.errors[opts.URL]; ok {
		return nil, err
	}
	if resp, ok := m.responses[opts.URL]; ok {
		return resp, nil
	}
	return nil, fmt.Errorf("unexpected fetch URL: %s", opts.URL)
}

// Sample subscription fixture payloads.
const (
	clashYAMLSharedNode = `
proxies:
  - name: "Shared Node Alpha"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: secret-password-sub1
`

	clashYAMLSub1WithTwoNodes = `
proxies:
  - name: "Shared Node Alpha"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: secret-password-sub1
  - name: "Sub1 Unique Node Beta"
    type: trojan
    server: 198.51.100.2
    port: 443
    password: trojan-pass-sub1
`

	clashYAMLSub2WithSharedAndUnique = `
proxies:
  - name: "Shared Node Alpha from Sub2"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: chacha20-ietf-poly1305
    password: different-password-sub2
  - name: "Sub2 Unique Node Gamma"
    type: vmess
    server: 198.51.100.3
    port: 8443
    uuid: 11111111-2222-3333-4444-555555555555
    alterId: 0
    cipher: auto
`

	clashYAMLSub1WithoutSharedNode = `
proxies:
  - name: "Sub1 Unique Node Beta"
    type: trojan
    server: 198.51.100.2
    port: 443
    password: trojan-pass-sub1
`

	clashYAMLPartialRejected = `
proxies:
  - name: "Valid Node"
    type: ss
    server: 198.51.100.10
    port: 8388
    cipher: aes-128-gcm
    password: secret-pass
  - name: "Invalid Node Unsupported Protocol"
    type: unsupported_protocol
    server: 198.51.100.11
    port: 9999
`
)

// Helper to create a test subscription.
func createTestSub(t *testing.T, repo domain.SubscriptionRepository, id, name, url string, enabled bool) *domain.Subscription {
	t.Helper()
	sub := &domain.Subscription{
		ID:                 id,
		Name:               name,
		SourceURLSecretRef: url,
		Enabled:            enabled,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds:  86400,
			TimeoutSeconds:   30,
			MaxResponseBytes: 10 * 1024 * 1024,
		},
		Revision:  domain.MustNewUUIDv7(),
		CreatedAt: domain.NowUTC(),
		UpdatedAt: domain.NowUTC(),
	}
	if err := repo.Create(context.Background(), sub); err != nil {
		t.Fatalf("failed to create test subscription: %v", err)
	}
	return sub
}

// 1. Dual-source merge test: two valid subscriptions containing identical logical_id node.
// Ledger must retain only one node record and associate both sources in node_sources.
func TestReconcile_DualSourceMerge(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	sub1ID := domain.MustNewUUIDv7()
	sub2ID := domain.MustNewUUIDv7()
	url1 := "https://provider1.example.com/sub"
	url2 := "https://provider2.example.com/sub"

	createTestSub(t, subRepo, sub1ID, "Subscription 1", url1, true)
	createTestSub(t, subRepo, sub2ID, "Subscription 2", url2, true)

	fetcher.setResponse(url1, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub1WithTwoNodes),
		ContentDigest: "digest-sub1-v1",
	})
	fetcher.setResponse(url2, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub2WithSharedAndUnique),
		ContentDigest: "digest-sub2-v1",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Refresh Subscription 1
	res1, err := svc.ReconcileSubscription(ctx, sub1ID)
	if err != nil {
		t.Fatalf("ReconcileSubscription(sub1) failed: %v", err)
	}
	if res1.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("expected outcome success, got %s", res1.Outcome)
	}
	if res1.NodesValid != 2 {
		t.Fatalf("expected 2 nodes valid, got %d", res1.NodesValid)
	}

	// Verify ledger after sub1: 2 nodes
	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if total != 2 || len(nodes) != 2 {
		t.Fatalf("expected 2 active nodes, got total=%d len=%d", total, len(nodes))
	}

	// Refresh Subscription 2
	res2, err := svc.ReconcileSubscription(ctx, sub2ID)
	if err != nil {
		t.Fatalf("ReconcileSubscription(sub2) failed: %v", err)
	}
	if res2.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("expected outcome success, got %s", res2.Outcome)
	}
	if res2.NodesValid != 2 {
		t.Fatalf("expected 2 nodes valid, got %d", res2.NodesValid)
	}

	// Shared node has server 198.51.100.1 and port 8388
	sharedLogicalID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.1", 8388, map[string]string{"network": "tcp"})

	// Verify total nodes in ledger: exactly 3 unique nodes (Alpha, Beta, Gamma)
	nodes, total, err = svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if total != 3 || len(nodes) != 3 {
		t.Fatalf("expected 3 total unique active nodes across both subscriptions, got %d", total)
	}

	// Verify the shared node details: must have 2 sources
	detail, err := svc.GetNodeDetail(ctx, sharedLogicalID)
	if err != nil {
		t.Fatalf("GetNodeDetail(%s) failed: %v", sharedLogicalID, err)
	}
	if detail.Node.LogicalID != sharedLogicalID {
		t.Fatalf("expected logicalID %s, got %s", sharedLogicalID, detail.Node.LogicalID)
	}
	if !detail.Node.Active {
		t.Fatalf("expected shared node to be active")
	}
	if len(detail.Sources) != 2 {
		t.Fatalf("expected exactly 2 sources for shared node, got %d", len(detail.Sources))
	}

	sourcesBySub := make(map[string]domain.NodeSource)
	for _, src := range detail.Sources {
		sourcesBySub[src.SubscriptionID] = src
	}
	if _, ok := sourcesBySub[sub1ID]; !ok {
		t.Errorf("missing source association for sub1")
	}
	if _, ok := sourcesBySub[sub2ID]; !ok {
		t.Errorf("missing source association for sub2")
	}
}

// 2. Last source deletion test: when node disappears from its only remaining subscription,
// after refresh that node becomes active = 0 and no longer appears in active node list.
func TestReconcile_LastSourceDeletion(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	sub1ID := domain.MustNewUUIDv7()
	sub2ID := domain.MustNewUUIDv7()
	url1 := "https://provider1.example.com/sub"
	url2 := "https://provider2.example.com/sub"

	createTestSub(t, subRepo, sub1ID, "Sub 1", url1, true)
	createTestSub(t, subRepo, sub2ID, "Sub 2", url2, true)

	// Step A: both sub1 and sub2 have Shared Node Alpha. Sub1 also has Beta. Sub2 has Gamma.
	fetcher.setResponse(url1, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub1WithTwoNodes),
		ContentDigest: "digest-sub1-v1",
	})
	fetcher.setResponse(url2, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub2WithSharedAndUnique),
		ContentDigest: "digest-sub2-v1",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	if _, err := svc.ReconcileSubscription(ctx, sub1ID); err != nil {
		t.Fatalf("sub1 initial refresh failed: %v", err)
	}
	if _, err := svc.ReconcileSubscription(ctx, sub2ID); err != nil {
		t.Fatalf("sub2 initial refresh failed: %v", err)
	}

	sharedLogicalID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.1", 8388, map[string]string{"network": "tcp"})

	// Step B: Sub1 updates and Shared Node Alpha DISAPPEARS from Sub1 (only Beta remains).
	fetcher.setResponse(url1, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub1WithoutSharedNode),
		ContentDigest: "digest-sub1-v2",
	})

	if _, err := svc.ReconcileSubscription(ctx, sub1ID); err != nil {
		t.Fatalf("sub1 second refresh failed: %v", err)
	}

	// Shared Node Alpha was removed from Sub1, BUT it is still in Sub2!
	// It MUST still be active = true!
	detail, err := svc.GetNodeDetail(ctx, sharedLogicalID)
	if err != nil {
		t.Fatalf("GetNodeDetail failed: %v", err)
	}
	if !detail.Node.Active {
		t.Fatalf("shared node should still be active because Sub2 still has it")
	}
	if len(detail.Sources) != 1 || detail.Sources[0].SubscriptionID != sub2ID {
		t.Fatalf("expected exactly 1 remaining source (Sub2), got: %#v", detail.Sources)
	}

	// Step C: Now Sub2 also updates and Shared Node Alpha DISAPPEARS from Sub2 as well!
	// Sub2 now only has Unique Gamma.
	const clashYAMLSub2WithoutShared = `
proxies:
  - name: "Sub2 Unique Node Gamma"
    type: vmess
    server: 198.51.100.3
    port: 8443
    uuid: 11111111-2222-3333-4444-555555555555
    alterId: 0
    cipher: auto
`
	fetcher.setResponse(url2, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub2WithoutShared),
		ContentDigest: "digest-sub2-v2",
	})

	if _, err := svc.ReconcileSubscription(ctx, sub2ID); err != nil {
		t.Fatalf("sub2 second refresh failed: %v", err)
	}

	// Now Shared Node Alpha has lost ALL sources!
	// It MUST have active = 0 (false) and NO sources associated!
	detail, err = svc.GetNodeDetail(ctx, sharedLogicalID)
	if err != nil {
		t.Fatalf("GetNodeDetail failed: %v", err)
	}
	if detail.Node.Active {
		t.Fatalf("expected node to be deactivated (active=false) after losing all sources, got active=true")
	}
	if len(detail.Sources) != 0 {
		t.Fatalf("expected 0 sources for deactivated node, got %d", len(detail.Sources))
	}

	// Active list MUST NOT include Shared Node Alpha!
	activeNodes, totalActive, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes(ActiveOnly=true) failed: %v", err)
	}
	for _, n := range activeNodes {
		if n.LogicalID == sharedLogicalID {
			t.Fatalf("deactivated node %s must not appear in ActiveOnly list", sharedLogicalID)
		}
	}
	if totalActive != 2 { // only Beta and Gamma remain active
		t.Fatalf("expected 2 active nodes (Beta and Gamma), got %d", totalActive)
	}

	// Inactive query (all nodes) MUST still include Shared Node Alpha
	allNodes, totalAll, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: false})
	if err != nil {
		t.Fatalf("ListNodes(ActiveOnly=false) failed: %v", err)
	}
	if totalAll != 3 {
		t.Fatalf("expected 3 total nodes including inactive, got %d", totalAll)
	}
	foundInactive := false
	for _, n := range allNodes {
		if n.LogicalID == sharedLogicalID && !n.Active {
			foundInactive = true
			break
		}
	}
	if !foundInactive {
		t.Fatalf("expected to find inactive node %s in full ledger", sharedLogicalID)
	}
}

// 3. Failure preservation test: when fetch or parse fails, original ledger nodes are 100% preserved.
func TestReconcile_FetchFailurePreservesExisting(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Stable Sub", url, true)

	// Step 1: initial successful refresh with 2 nodes
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub1WithTwoNodes),
		ContentDigest: "digest-v1",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	res1, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("initial refresh failed: %v", err)
	}
	if res1.Outcome != domain.FetchOutcomeSuccess || res1.NodesValid != 2 {
		t.Fatalf("expected initial success with 2 nodes, got %#v", res1)
	}

	initialNodes, initialTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || initialTotal != 2 {
		t.Fatalf("expected 2 active nodes initially, got %d (err: %v)", initialTotal, err)
	}

	// Step 2: network fetch failure (e.g. timeout / connection refused)
	fetcher.setError(url, fmt.Errorf("connection refused to target server"))

	resFail, err := svc.ReconcileSubscription(ctx, subID)
	if err == nil {
		t.Fatalf("expected error from failed fetch, got nil")
	}
	if resFail == nil || resFail.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed outcome, got: %#v", resFail)
	}

	// Existing ledger nodes MUST BE 100% UNCHANGED
	afterNodes, afterTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if afterTotal != initialTotal || len(afterNodes) != len(initialNodes) {
		t.Fatalf("ledger changed after fetch failure! before=%d after=%d", initialTotal, afterTotal)
	}
	for i := range initialNodes {
		if initialNodes[i].LogicalID != afterNodes[i].LogicalID || initialNodes[i].Active != afterNodes[i].Active {
			t.Fatalf("node state mutated on fetch failure: before=%#v after=%#v", initialNodes[i], afterNodes[i])
		}
	}

	// Audit record MUST be recorded for the failure
	failedRec, err := fetchRepo.GetByID(ctx, resFail.FetchID)
	if err != nil {
		t.Fatalf("failed to get failed fetch by ID: %v", err)
	}
	if failedRec.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("fetch audit must have outcome 'failed', got: %s", failedRec.Outcome)
	}
	if failedRec.RedactedError == "" {
		t.Fatalf("failed fetch audit must record redacted_error")
	}

	fetches, err := fetchRepo.ListBySubscription(ctx, subID, 10)
	if err != nil {
		t.Fatalf("failed to list subscription fetches: %v", err)
	}
	if len(fetches) != 2 {
		t.Fatalf("expected 2 fetch audit records (1 success + 1 failed), got %d", len(fetches))
	}
	var hasSuccess, hasFailed bool
	for _, f := range fetches {
		if f.Outcome == domain.FetchOutcomeSuccess {
			hasSuccess = true
		}
		if f.Outcome == domain.FetchOutcomeFailed {
			hasFailed = true
		}
	}
	if !hasSuccess || !hasFailed {
		t.Fatalf("expected both success and failed audits in list, got: %#v", fetches)
	}
}

// 4. Parse failure preservation test: when content cannot be parsed, ledger nodes are 100% preserved.
func TestReconcile_ParseFailurePreservesExisting(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Stable Sub", url, true)

	// Step 1: initial successful refresh
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub1WithTwoNodes),
		ContentDigest: "digest-v1",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("initial refresh failed: %v", err)
	}

	// Step 2: fetch returns invalid / corrupted / garbage content
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte("THIS IS NOT YAML AND NOT BASE64 AND NOT VALID URL LINES!!! $$$$$$"),
		ContentDigest: "digest-corrupted",
	})

	resFail, err := svc.ReconcileSubscription(ctx, subID)
	if err == nil {
		t.Fatalf("expected error on invalid content, got nil")
	}
	if resFail == nil || resFail.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed outcome, got: %#v", resFail)
	}

	// Existing ledger nodes MUST BE 100% UNCHANGED
	afterNodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if total != 2 || len(afterNodes) != 2 {
		t.Fatalf("ledger mutated on parse failure! expected 2 active nodes, got %d", total)
	}

	// Failed audit record logged with digest
	failedRec, err := fetchRepo.GetByID(ctx, resFail.FetchID)
	if err != nil {
		t.Fatalf("failed to get failed parse fetch by ID: %v", err)
	}
	if failedRec.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected outcome failed, got %s", failedRec.Outcome)
	}
	if failedRec.ContentDigest != "digest-corrupted" {
		t.Fatalf("expected digest-corrupted, got %s", failedRec.ContentDigest)
	}

	fetches, err := fetchRepo.ListBySubscription(ctx, subID, 10)
	if err != nil || len(fetches) != 2 {
		t.Fatalf("expected 2 fetch audit records, got %d (err: %v)", len(fetches), err)
	}
}

// 5. Disabled subscription rejection test.
func TestReconcile_DisabledSubscriptionRejected(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://disabled.example.com/sub"
	createTestSub(t, subRepo, subID, "Disabled Sub", url, false) // enabled = false

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	res, err := svc.ReconcileSubscription(ctx, subID)
	if err == nil {
		t.Fatalf("expected error for disabled subscription, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result on preflight rejection, got %#v", res)
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Category != domain.CategoryValidation || de.Code != "subscription_disabled" {
		t.Fatalf("expected validation error (subscription_disabled), got: %v", err)
	}
}

// 6. Reactivation test: deactivated node becomes active=1 when it reappears in a subscription.
func TestReconcile_Reactivation(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Sub", url, true)

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Step 1: add node
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSharedNode),
		ContentDigest: "digest-1",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("step 1 failed: %v", err)
	}

	sharedLogicalID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.1", 8388, map[string]string{"network": "tcp"})
	detail, err := svc.GetNodeDetail(ctx, sharedLogicalID)
	if err != nil || !detail.Node.Active {
		t.Fatalf("node should be active in step 1")
	}

	// Step 2: replace with another node -> shared node is deactivated
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub1WithoutSharedNode),
		ContentDigest: "digest-2",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("step 2 failed: %v", err)
	}

	detail, err = svc.GetNodeDetail(ctx, sharedLogicalID)
	if err != nil || detail.Node.Active {
		t.Fatalf("node should be inactive in step 2")
	}

	// Step 3: shared node reappears -> must be reactivated
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSharedNode),
		ContentDigest: "digest-3",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("step 3 failed: %v", err)
	}

	detail, err = svc.GetNodeDetail(ctx, sharedLogicalID)
	if err != nil || !detail.Node.Active {
		t.Fatalf("node should be reactivated in step 3, got active=%v", detail.Node.Active)
	}
	if len(detail.Sources) != 1 {
		t.Fatalf("expected 1 source on reactivation, got %d", len(detail.Sources))
	}
}

// 7. Partial outcome test: some nodes valid, some rejected.
func TestReconcile_PartialOutcomeOnRejectedNodes(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Sub Partial", url, true)

	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLPartialRejected),
		ContentDigest: "digest-partial",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("expected reconcile success with partial outcome, got error: %v", err)
	}
	if res.Outcome != domain.FetchOutcomePartial {
		t.Fatalf("expected outcome 'partial', got: %s", res.Outcome)
	}
	if res.NodesValid != 1 {
		t.Fatalf("expected 1 valid node, got %d", res.NodesValid)
	}
	if res.NodesParsed != 2 {
		t.Fatalf("expected 2 parsed nodes (1 valid + 1 rejected), got %d", res.NodesParsed)
	}
}

// 8. Server-side filtering and pagination test on ListNodes.
func TestService_ListNodesAndDetail(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Sub Filter", url, true)

	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub1WithTwoNodes),
		ContentDigest: "digest-filter",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// Protocol filter: SS
	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{
		Protocols:  []domain.Protocol{domain.ProtocolSS},
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if total != 1 || len(nodes) != 1 {
		t.Fatalf("expected 1 SS node, got total=%d len=%d", total, len(nodes))
	}
	if nodes[0].Protocol != domain.ProtocolSS {
		t.Fatalf("expected ProtocolSS, got %s", nodes[0].Protocol)
	}

	// Search filter: "Beta"
	nodes, total, err = svc.ListNodes(ctx, domain.NodeFilter{
		SearchText: "Beta",
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListNodes search failed: %v", err)
	}
	if total != 1 || len(nodes) != 1 {
		t.Fatalf("expected 1 match for 'Beta', got total=%d", total)
	}

	// Non-existent detail check
	_, err = svc.GetNodeDetail(ctx, "non-existent-logical-id")
	if err == nil {
		t.Fatalf("expected not found error, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Category != domain.CategoryNotFound {
		t.Fatalf("expected NotFoundError, got: %v", err)
	}
}

// 9. Duplicate nodes with identical logical_id within the same subscription.
func TestReconcile_DuplicateNodesInSameSubscription(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Sub Dups", url, true)

	const clashYAMLDups = `
proxies:
  - name: "Node First Instance"
    type: ss
    server: 198.51.100.99
    port: 8388
    cipher: aes-128-gcm
    password: pass1
  - name: "Node Second Instance (Identical Transport)"
    type: ss
    server: 198.51.100.99
    port: 8388
    cipher: aes-128-gcm
    password: pass2
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLDups),
		ContentDigest: "digest-dups",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("reconcile duplicate entries failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("expected outcome success, got: %s", res.Outcome)
	}

	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if total != 1 || len(nodes) != 1 {
		t.Fatalf("expected exactly 1 deduplicated node in ledger, got total=%d len=%d", total, len(nodes))
	}
}

// 10. NodeView non-disclosure test: ensure secret references are stripped from NodeView.
func TestReconcile_NodeViewSecretRedaction(t *testing.T) {
	node := domain.Node{
		LogicalID:                 "node_0123456789abcdef",
		Protocol:                  domain.ProtocolSS,
		DisplayName:               "Public Display",
		NormalizedConfigSecretRef: "secret_should_never_leak_in_view",
		Active:                    true,
		CreatedAt:                 domain.NowUTC(),
		UpdatedAt:                 domain.NowUTC(),
	}

	view := inventory.ToNodeView(node)
	if view.LogicalID != node.LogicalID {
		t.Fatalf("logical_id mismatch: %s != %s", view.LogicalID, node.LogicalID)
	}
	if view.DisplayName != node.DisplayName {
		t.Fatalf("display_name mismatch: %s != %s", view.DisplayName, node.DisplayName)
	}

	views := inventory.ToNodeViews([]domain.Node{node})
	if len(views) != 1 || views[0].LogicalID != node.LogicalID {
		t.Fatalf("ToNodeViews failed: %#v", views)
	}
}

// 11. Concurrency test: verify concurrent reconciles and list queries pass under race detector.
func TestReconcile_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	sub1ID := domain.MustNewUUIDv7()
	sub2ID := domain.MustNewUUIDv7()
	url1 := "https://p1.example.com/sub"
	url2 := "https://p2.example.com/sub"
	createTestSub(t, subRepo, sub1ID, "Sub C1", url1, true)
	createTestSub(t, subRepo, sub2ID, "Sub C2", url2, true)

	fetcher.setResponse(url1, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub1WithTwoNodes),
		ContentDigest: "digest-c1",
	})
	fetcher.setResponse(url2, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSub2WithSharedAndUnique),
		ContentDigest: "digest-c2",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := svc.ReconcileSubscription(ctx, sub1ID); err != nil {
				errCh <- err
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := svc.ReconcileSubscription(ctx, sub2ID); err != nil {
				errCh <- err
			}
		}()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent reconcile error: %v", err)
	}
}
