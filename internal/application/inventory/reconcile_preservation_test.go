package inventory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

// Task 1.1: TestReconcile_FetchOrParseFailure_PreservesNodes
func TestReconcile_FetchOrParseFailure_PreservesNodes(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Preserve Sub", url, true)

	// Initial successful populate
	const initialYAML = `
proxies:
  - name: "Node Alpha"
    type: ss
    server: 198.51.100.10
    port: 8388
    cipher: aes-128-gcm
    password: pass-alpha
  - name: "Node Beta"
    type: trojan
    server: 198.51.100.11
    port: 443
    password: pass-beta
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(initialYAML),
		ContentDigest: "digest-initial",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomeSuccess || res.NodesValid != 2 {
		t.Fatalf("expected 2 valid nodes, got %d", res.NodesValid)
	}

	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 || len(nodes) != 2 {
		t.Fatalf("expected 2 active nodes initially, got %d", total)
	}

	// Subtest 1: Network / HTTP failure
	t.Run("network_fetch_failure", func(t *testing.T) {
		fetcher.setError(url, fmt.Errorf("http 502 bad gateway or connection timeout"))

		rFail, err := svc.ReconcileSubscription(ctx, subID)
		if err == nil {
			t.Fatalf("expected error on network failure, got nil")
		}
		if rFail == nil || rFail.Outcome != domain.FetchOutcomeFailed {
			t.Fatalf("expected failed outcome, got: %#v", rFail)
		}

		afterNodes, afterTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
		if err != nil {
			t.Fatalf("ListNodes failed: %v", err)
		}
		if afterTotal != 2 || len(afterNodes) != 2 {
			t.Fatalf("nodes mutated after network failure: got %d, want 2", afterTotal)
		}
		for _, n := range afterNodes {
			if !n.Active {
				t.Fatalf("node %s was deactivated on fetch failure", n.LogicalID)
			}
		}
	})

	// Subtest 2: Content parsing failure
	t.Run("content_parse_failure", func(t *testing.T) {
		fetcher.setResponse(url, &fetch.Response{
			StatusCode:    200,
			Body:          []byte("corrupted invalid binary/garbage content %%%$$$"),
			ContentDigest: "digest-corrupt",
		})

		rFail, err := svc.ReconcileSubscription(ctx, subID)
		if err == nil {
			t.Fatalf("expected error on parse failure, got nil")
		}
		if rFail == nil || rFail.Outcome != domain.FetchOutcomeFailed {
			t.Fatalf("expected failed outcome, got: %#v", rFail)
		}

		afterNodes, afterTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
		if err != nil {
			t.Fatalf("ListNodes failed: %v", err)
		}
		if afterTotal != 2 || len(afterNodes) != 2 {
			t.Fatalf("nodes mutated after parse failure: got %d, want 2", afterTotal)
		}
	})

	// Subtest 3: Empty extraction result
	t.Run("empty_extraction_preserves_nodes", func(t *testing.T) {
		fetcher.setResponse(url, &fetch.Response{
			StatusCode:    200,
			Body:          []byte("proxies: []\n"),
			ContentDigest: "digest-empty",
		})

		rFail, err := svc.ReconcileSubscription(ctx, subID)
		if err == nil {
			t.Fatalf("expected error on empty extracted proxies, got nil")
		}
		if rFail == nil || rFail.Outcome != domain.FetchOutcomeFailed {
			t.Fatalf("expected failed outcome, got: %#v", rFail)
		}

		afterNodes, afterTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
		if err != nil {
			t.Fatalf("ListNodes failed: %v", err)
		}
		if afterTotal != 2 || len(afterNodes) != 2 {
			t.Fatalf("nodes mutated after empty extraction: got %d, want 2", afterTotal)
		}
	})
}

// Task 1.2: TestReconcile_PruneScoped_ProtectsManualAndOtherSourceNodes
func TestReconcile_PruneScoped_ProtectsManualAndOtherSourceNodes(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	// 1. Create a manual node directly in nodes table without any node_sources
	manualNode := domain.Node{
		LogicalID:   "node_manual_9999999999999999",
		Protocol:    domain.ProtocolSS,
		DisplayName: "Independent Manual Node",
		Server:      "203.0.113.50",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "manual-pass"},
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{manualNode}); err != nil {
		t.Fatalf("failed to insert manual node: %v", err)
	}

	// 2. Create Sub A and Sub B
	subAID := domain.MustNewUUIDv7()
	subBID := domain.MustNewUUIDv7()
	urlA := "https://provider-a.example.com/sub"
	urlB := "https://provider-b.example.com/sub"
	createTestSub(t, subRepo, subAID, "Subscription A", urlA, true)
	createTestSub(t, subRepo, subBID, "Subscription B", urlB, true)

	const yamlSubA = `
proxies:
  - name: "Node A1"
    type: ss
    server: 198.51.100.21
    port: 8388
    cipher: aes-128-gcm
    password: pass-a1
`
	const yamlSubB = `
proxies:
  - name: "Node B1"
    type: trojan
    server: 198.51.100.22
    port: 443
    password: pass-b1
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubA), ContentDigest: "digest-a1"})
	fetcher.setResponse(urlB, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubB), ContentDigest: "digest-b1"})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("reconcile Sub A failed: %v", err)
	}
	if _, err := svc.ReconcileSubscription(ctx, subBID); err != nil {
		t.Fatalf("reconcile Sub B failed: %v", err)
	}

	// Verify all 3 nodes (Manual + A1 + B1) are active
	nodesBefore, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 3 {
		t.Fatalf("expected 3 active nodes, got total=%d err=%v", total, err)
	}
	var b1ID string
	for _, n := range nodesBefore {
		if n.DisplayName == "Node B1" {
			b1ID = n.LogicalID
			break
		}
	}
	if b1ID == "" {
		t.Fatalf("Node B1 not found in ledger")
	}

	// 3. Now Sub A refreshes with a completely new node A2 (Node A1 is replaced/omitted)
	const yamlSubAUpdated = `
proxies:
  - name: "Node A2 (Replacement)"
    type: ss
    server: 198.51.100.23
    port: 8388
    cipher: aes-128-gcm
    password: pass-a2
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubAUpdated), ContentDigest: "digest-a2"})

	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("reconcile Sub A update failed: %v", err)
	}

	// 4. Assert non-destructive merge:
	// - Manual node MUST STILL be active!
	// - Sub B's Node B1 MUST STILL be active!
	// - Node A2 is active!
	// - Node A1 (unseen in this fetch) MUST ALSO STILL be active (non-destructive merge)!
	manualDetail, err := svc.GetNodeDetail(ctx, manualNode.LogicalID)
	if err != nil {
		t.Fatalf("failed to get manual node: %v", err)
	}
	if !manualDetail.Node.Active {
		t.Fatalf("MANUAL NODE WAS ERRONEOUSLY DEACTIVATED! Must remain active!")
	}

	b1Detail, err := svc.GetNodeDetail(ctx, b1ID)
	if err != nil {
		t.Fatalf("failed to get node B1: %v", err)
	}
	if !b1Detail.Node.Active {
		t.Fatalf("SUB B NODE WAS ERRONEOUSLY DEACTIVATED! Must remain active!")
	}

	allNodesAfter, totalAfter, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalAfter != 4 {
		t.Fatalf("expected 4 active nodes (Manual + B1 + A1 + A2) due to non-destructive merge, got %d", totalAfter)
	}
	var a1Found, a2Found bool
	for _, n := range allNodesAfter {
		if n.DisplayName == "Node A1" && n.Active {
			a1Found = true
		}
		if n.DisplayName == "Node A2 (Replacement)" && n.Active {
			a2Found = true
		}
	}
	if !a1Found {
		t.Fatalf("Node A1 was erroneously deactivated/deleted on refresh with fewer/different nodes!")
	}
	if !a2Found {
		t.Fatalf("Node A2 was not activated!")
	}
}

// Task 1.3: TestReconcile_IdentityMatching_DisambiguationAndIsolation
func TestReconcile_IdentityMatching_DisambiguationAndIsolation(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subAID := domain.MustNewUUIDv7()
	subBID := domain.MustNewUUIDv7()
	urlA := "https://sub-a.com"
	urlB := "https://sub-b.com"
	createTestSub(t, subRepo, subAID, "Sub A", urlA, true)
	createTestSub(t, subRepo, subBID, "Sub B", urlB, true)

	// 1. Same subscription stable identity matching: "Tokyo Node" changes server IP
	const subAInitial = `
proxies:
  - name: "Tokyo Stable Node"
    type: ss
    server: 198.51.100.31
    port: 8388
    cipher: aes-128-gcm
    password: pass-old
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(subAInitial), ContentDigest: "dig-1"})
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	resA1, err := svc.ReconcileSubscription(ctx, subAID)
	if err != nil {
		t.Fatalf("sub A initial failed: %v", err)
	}
	if resA1.NodesValid != 1 {
		t.Fatalf("expected 1 valid node, got %d", resA1.NodesValid)
	}

	nodesA1, _, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(nodesA1) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodesA1))
	}
	initialLogicalID := nodesA1[0].LogicalID
	if nodesA1[0].ConnectionRevision != 1 {
		t.Fatalf("expected revision 1, got %d", nodesA1[0].ConnectionRevision)
	}

	// Now Sub A updates: "Tokyo Stable Node" has updated server IP and password!
	const subAUpdated = `
proxies:
  - name: "Tokyo Stable Node"
    type: ss
    server: 198.51.100.32
    port: 8388
    cipher: aes-128-gcm
    password: pass-new
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(subAUpdated), ContentDigest: "dig-2"})
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("sub A update failed: %v", err)
	}

	nodesA2, _, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(nodesA2) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodesA2))
	}
	// Stable identity MUST be preserved!
	if nodesA2[0].LogicalID != initialLogicalID {
		t.Fatalf("logical ID changed on endpoint update! want %s, got %s", initialLogicalID, nodesA2[0].LogicalID)
	}
	if nodesA2[0].Server != "198.51.100.32" {
		t.Fatalf("expected server updated to 198.51.100.32, got %s", nodesA2[0].Server)
	}
	if nodesA2[0].ConnectionRevision != 2 {
		t.Fatalf("expected connection revision to increment to 2, got %d", nodesA2[0].ConnectionRevision)
	}

	// 2. Cross-source credential isolation: Sub B provides a node with name "Tokyo Stable Node" but different protocol/creds
	const subBContent = `
proxies:
  - name: "Tokyo Stable Node"
    type: trojan
    server: 198.51.100.99
    port: 443
    password: trojan-sub-b-pass
`
	fetcher.setResponse(urlB, &fetch.Response{StatusCode: 200, Body: []byte(subBContent), ContentDigest: "dig-b"})
	if _, err := svc.ReconcileSubscription(ctx, subBID); err != nil {
		t.Fatalf("sub B reconcile failed: %v", err)
	}

	// Sub B must NOT merge credentials or overwrite Sub A's node!
	nodeAAfter, err := svc.GetNode(ctx, initialLogicalID)
	if err != nil {
		t.Fatalf("failed to get Sub A node: %v", err)
	}
	if nodeAAfter.Protocol != domain.ProtocolSS || nodeAAfter.Server != "198.51.100.32" {
		t.Fatalf("Sub A node was corrupted by Sub B! %+v", nodeAAfter)
	}

	allNodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 || len(allNodes) != 2 {
		t.Fatalf("expected 2 distinct active nodes across isolated subscriptions, got %d", total)
	}
}

// Task 1.4: TestReconcile_ConnectionRevision_IncrementOnRealChange
func TestReconcile_ConnectionRevision_IncrementOnRealChange(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Revision Sub", url, true)

	const yamlInitial = `
proxies:
  - name: "Target Node"
    type: vmess
    server: 198.51.100.40
    port: 443
    uuid: 22222222-3333-4444-5555-666666666666
    alterId: 0
    cipher: auto
    network: ws
    ws-opts:
      path: /ws
      headers:
        Host: vmess.example.com
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlInitial), ContentDigest: "dig-v1"})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	nodes, _, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	node := nodes[0]
	if node.ConnectionRevision != 1 {
		t.Fatalf("expected initial revision 1, got %d", node.ConnectionRevision)
	}

	// Record a baseline healthy probe observation at revision 1
	obsRepo := sqlite.NewProbeObservationRepository(db)
	runRepo := sqlite.NewProbeRunRepository(db)
	run := &domain.ProbeRun{
		ID:             "run-rev-1",
		IdempotencyKey: "run-rev-1",
		ActorScope:     "test",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     time.Now().Add(time.Hour),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("failed to create probe run: %v", err)
	}

	rev1 := int64(1)
	obs := &domain.ProbeObservation{
		ID:                 "obs-rev-1",
		ProbeRunID:         run.ID,
		NodeLogicalID:      node.LogicalID,
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictAvailable,
		LatencyMS:          120,
		ConnectionRevision: &rev1,
		ObservedAt:         time.Now().UTC(),
	}
	if err := obsRepo.Create(ctx, obs); err != nil {
		t.Fatalf("failed to create observation: %v", err)
	}

	// Verify NodeDetail reports healthy at revision 1
	detail1, err := svc.GetNodeDetail(ctx, node.LogicalID)
	if err != nil {
		t.Fatalf("GetNodeDetail failed: %v", err)
	}
	if detail1.HealthStatus != "healthy" {
		t.Fatalf("expected healthStatus 'healthy' at revision 1, got: %s", detail1.HealthStatus)
	}

	// Subtest 1: display name rename only -> ConnectionRevision MUST NOT change!
	const yamlRenamed = `
proxies:
  - name: "Target Node Renamed Label"
    type: vmess
    server: 198.51.100.40
    port: 443
    uuid: 22222222-3333-4444-5555-666666666666
    alterId: 0
    cipher: auto
    network: ws
    ws-opts:
      path: /ws
      headers:
        Host: vmess.example.com
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlRenamed), ContentDigest: "dig-renamed"})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("reconcile renamed failed: %v", err)
	}

	detail2, err := svc.GetNodeDetail(ctx, node.LogicalID)
	if err != nil {
		t.Fatalf("GetNodeDetail failed: %v", err)
	}
	if detail2.Node.ConnectionRevision != 1 {
		t.Fatalf("ConnectionRevision incremented on rename! expected 1, got %d", detail2.Node.ConnectionRevision)
	}
	if detail2.HealthStatus != "healthy" {
		t.Fatalf("HealthStatus invalidated on rename! expected 'healthy', got %s", detail2.HealthStatus)
	}

	// Subtest 2: Real connection change (port updated from 443 to 8443) -> ConnectionRevision MUST increment!
	const yamlPortChange = `
proxies:
  - name: "Target Node Renamed Label"
    type: vmess
    server: 198.51.100.40
    port: 8443
    uuid: 22222222-3333-4444-5555-666666666666
    alterId: 0
    cipher: auto
    network: ws
    ws-opts:
      path: /ws
      headers:
        Host: vmess.example.com
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlPortChange), ContentDigest: "dig-port"})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("reconcile port change failed: %v", err)
	}

	detail3, err := svc.GetNodeDetail(ctx, node.LogicalID)
	if err != nil {
		t.Fatalf("GetNodeDetail failed: %v", err)
	}
	if detail3.Node.ConnectionRevision != 2 {
		t.Fatalf("expected ConnectionRevision to increment to 2 on port change, got %d", detail3.Node.ConnectionRevision)
	}
	// Old revision 1 observation must NO LONGER certify this revision 2 connection!
	if detail3.HealthStatus == "healthy" {
		t.Fatalf("HealthStatus must NOT be healthy after connection changed! got: %s", detail3.HealthStatus)
	}
	if detail3.HealthStatus != "unknown" {
		t.Fatalf("expected health status 'unknown', got %s", detail3.HealthStatus)
	}
}

// Regression: Success fetch with fewer nodes (2 nodes shrink to 1) must preserve older nodes
func TestReconcile_SuccessShrink2To1_PreservesOldNode(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://shrink.example.com/sub"
	createTestSub(t, subRepo, subID, "Shrink Sub", url, true)

	const yamlTwoNodes = `
proxies:
  - name: "Node First"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: pass-1
  - name: "Node Second"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-2
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlTwoNodes), ContentDigest: "digest-two"})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	_, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 {
		t.Fatalf("expected 2 active nodes initially, got %d", total)
	}

	// Shrink: second fetch only contains Node Second (Node First missing from remote)
	const yamlOneNode = `
proxies:
  - name: "Node Second"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-2
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlOneNode), ContentDigest: "digest-one"})

	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("shrink reconcile failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomeSuccess || res.NodesValid != 1 {
		t.Fatalf("expected successful outcome with 1 valid node, got outcome=%s valid=%d", res.Outcome, res.NodesValid)
	}

	// Invariant: Node First MUST still be active! Both nodes remain in ledger!
	nodesAfter, totalAfter, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalAfter != 2 || len(nodesAfter) != 2 {
		t.Fatalf("shrink destroyed inactive nodes! expected 2 active nodes preserved, got %d", totalAfter)
	}
	for _, n := range nodesAfter {
		if !n.Active {
			t.Fatalf("node %s became inactive after shrink fetch", n.DisplayName)
		}
	}
}

// Regression: Partial outcome with rejected nodes preserves all existing nodes
func TestReconcile_PartialRejected_PreservesExistingNodes(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://partial.example.com/sub"
	createTestSub(t, subRepo, subID, "Partial Sub", url, true)

	const yamlInitial = `
proxies:
  - name: "Stable Node 1"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: pass-1
  - name: "Stable Node 2"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-2
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlInitial), ContentDigest: "d-init"})
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Second fetch: contains Node 3, but also an invalid proxy with missing port / malformed config
	const yamlPartial = `
proxies:
  - name: "Stable Node 3 (New)"
    type: ss
    server: 198.51.100.3
    port: 8388
    cipher: aes-128-gcm
    password: pass-3
  - name: "Malformed Invalid Proxy"
    type: ss
    server: ""
    port: 0
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlPartial), ContentDigest: "d-partial"})

	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("reconcile partial failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomePartial {
		t.Fatalf("expected FetchOutcomePartial, got %s", res.Outcome)
	}

	// Invariant: Node 1, Node 2, and Node 3 are all active!
	nodesAfter, totalAfter, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalAfter != 3 || len(nodesAfter) != 3 {
		t.Fatalf("expected 3 active nodes (Node 1, 2, 3), got %d", totalAfter)
	}
}

// Regression: Overlapping sources and independent manual nodes are never corrupted
func TestReconcile_OverlappingSourcesAndManualNodes(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	// 1. Create a manual node
	manualNode := domain.Node{
		LogicalID:   "manual_node_protected_001",
		Protocol:    domain.ProtocolSS,
		DisplayName: "Manual Sovereign Node",
		Server:      "203.0.113.10",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "manual-pass"},
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{manualNode}); err != nil {
		t.Fatalf("failed to insert manual node: %v", err)
	}

	// 2. Sub A and Sub B both import an identical shared node (same server, port, credentials)
	subAID := domain.MustNewUUIDv7()
	subBID := domain.MustNewUUIDv7()
	urlA := "https://overlap-a.example.com/sub"
	urlB := "https://overlap-b.example.com/sub"
	createTestSub(t, subRepo, subAID, "Overlap Sub A", urlA, true)
	createTestSub(t, subRepo, subBID, "Overlap Sub B", urlB, true)

	const yamlOverlap = `
proxies:
  - name: "Shared Provider Node"
    type: ss
    server: 198.51.100.50
    port: 8388
    cipher: aes-128-gcm
    password: shared-secret
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlOverlap), ContentDigest: "d-oa"})
	fetcher.setResponse(urlB, &fetch.Response{StatusCode: 200, Body: []byte(yamlOverlap), ContentDigest: "d-ob"})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("sub A reconcile failed: %v", err)
	}
	if _, err := svc.ReconcileSubscription(ctx, subBID); err != nil {
		t.Fatalf("sub B reconcile failed: %v", err)
	}

	// Assert: exactly 2 active nodes (manual + shared)
	_, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 {
		t.Fatalf("expected 2 active nodes (1 manual + 1 shared), got %d", total)
	}

	// Sub A refreshes with a totally new node
	const yamlSubANew = `
proxies:
  - name: "Sub A New Node"
    type: ss
    server: 198.51.100.51
    port: 8388
    cipher: aes-128-gcm
    password: a-new-secret
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubANew), ContentDigest: "d-oa2"})
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("sub A second reconcile failed: %v", err)
	}

	// Assert: manual node is active, shared node is active, new node is active!
	_, totalAfter, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalAfter != 3 {
		t.Fatalf("expected 3 active nodes (manual + shared + sub A new), got %d", totalAfter)
	}

	manualCheck, _ := svc.GetNodeDetail(ctx, manualNode.LogicalID)
	if !manualCheck.Node.Active {
		t.Fatalf("manual node was deactivated!")
	}
}

// Regression: Cross-source credential isolation prevents credential overwriting
// P0 Root Cause: Sub A and Sub B have EXACT SAME host, port, protocol, and display_name, but DIFFERENT passwords.
// Neither node may overwrite the other; subsequent refreshes preserve isolation and independent revisions.
func TestReconcile_NameCollisionAndCrossSourceCredentialsIsolation(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subAID := domain.MustNewUUIDv7()
	subBID := domain.MustNewUUIDv7()
	urlA := "https://col-a.example.com/sub"
	urlB := "https://col-b.example.com/sub"
	createTestSub(t, subRepo, subAID, "Collision Sub A", urlA, true)
	createTestSub(t, subRepo, subBID, "Collision Sub B", urlB, true)

	// Sub A has node with server 198.51.100.71 and pass-sub-a-secret
	const yamlSubA = `
proxies:
  - name: "Collision Name"
    type: ss
    server: 198.51.100.71
    port: 8388
    cipher: aes-128-gcm
    password: pass-sub-a-secret
`
	// Sub B has node with EXACT SAME server 198.51.100.71 and port 8388 and name, but DIFFERENT password pass-sub-b-secret
	const yamlSubB = `
proxies:
  - name: "Collision Name"
    type: ss
    server: 198.51.100.71
    port: 8388
    cipher: aes-128-gcm
    password: pass-sub-b-secret
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubA), ContentDigest: "d-ca-1"})
	fetcher.setResponse(urlB, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubB), ContentDigest: "d-cb-1"})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("reconcile Sub A failed: %v", err)
	}
	if _, err := svc.ReconcileSubscription(ctx, subBID); err != nil {
		t.Fatalf("reconcile Sub B failed: %v", err)
	}

	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 || len(nodes) != 2 {
		t.Fatalf("expected 2 isolated nodes across subscriptions with different creds, got total=%d len=%d", total, len(nodes))
	}

	// Verify both nodes exist and their credentials are NOT merged or overwritten
	var nodeAID, nodeBID string
	for _, n := range nodes {
		det, err := svc.GetNode(ctx, n.LogicalID)
		if err != nil {
			t.Fatalf("GetNode failed: %v", err)
		}
		if det.Credentials.Password == "pass-sub-a-secret" {
			nodeAID = n.LogicalID
		}
		if det.Credentials.Password == "pass-sub-b-secret" {
			nodeBID = n.LogicalID
		}
	}
	if nodeAID == "" || nodeBID == "" || nodeAID == nodeBID {
		t.Fatalf("credentials were merged or overwritten! nodeAID=%q nodeBID=%q", nodeAID, nodeBID)
	}

	// Step 2: Refresh Sub A again -> Sub B's credentials and revision must be UNCHANGED!
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubA), ContentDigest: "d-ca-2"})
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("re-reconcile Sub A failed: %v", err)
	}

	detBAfterA, err := svc.GetNode(ctx, nodeBID)
	if err != nil || detBAfterA.Credentials.Password != "pass-sub-b-secret" || detBAfterA.ConnectionRevision != 1 {
		t.Fatalf("Sub B was corrupted after Sub A refresh! got pass=%q, rev=%d", detBAfterA.Credentials.Password, detBAfterA.ConnectionRevision)
	}

	// Step 3: Refresh Sub B again -> Sub A's credentials and revision must be UNCHANGED!
	fetcher.setResponse(urlB, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubB), ContentDigest: "d-cb-2"})
	if _, err := svc.ReconcileSubscription(ctx, subBID); err != nil {
		t.Fatalf("re-reconcile Sub B failed: %v", err)
	}

	detAAfterB, err := svc.GetNode(ctx, nodeAID)
	if err != nil || detAAfterB.Credentials.Password != "pass-sub-a-secret" || detAAfterB.ConnectionRevision != 1 {
		t.Fatalf("Sub A was corrupted after Sub B refresh! got pass=%q, rev=%d", detAAfterB.Credentials.Password, detAAfterB.ConnectionRevision)
	}

	// Step 4: Sub A updates its password to pass-sub-a-updated -> Sub A's revision increments to 2, Sub B stays 1!
	const yamlSubAUpdated = `
proxies:
  - name: "Collision Name"
    type: ss
    server: 198.51.100.71
    port: 8388
    cipher: aes-128-gcm
    password: pass-sub-a-updated
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubAUpdated), ContentDigest: "d-ca-3"})
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("reconcile Sub A updated failed: %v", err)
	}

	detAUpdated, err := svc.GetNode(ctx, nodeAID)
	if err != nil {
		t.Fatalf("failed to get Sub A after update: %v", err)
	}
	if detAUpdated.Credentials.Password != "pass-sub-a-updated" || detAUpdated.ConnectionRevision != 2 {
		t.Fatalf("Sub A revision should be 2 and pass updated, got rev=%d pass=%s", detAUpdated.ConnectionRevision, detAUpdated.Credentials.Password)
	}

	detBFinal, err := svc.GetNode(ctx, nodeBID)
	if err != nil || detBFinal.Credentials.Password != "pass-sub-b-secret" || detBFinal.ConnectionRevision != 1 {
		t.Fatalf("Sub B was modified by Sub A connection update! rev=%d, pass=%s", detBFinal.ConnectionRevision, detBFinal.Credentials.Password)
	}
}

// Invariant: 2 -> 1 -> 2 same-name connection change preserves identity, provenance, and increments revision
func TestReconcile_TwoOneTwo_SameNameConnectionChange(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://cycle.example.com/sub"
	createTestSub(t, subRepo, subID, "Cycle Sub", url, true)
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// --- Phase 1: 2 nodes present (Alpha port 8388, Beta port 8388) ---
	const yamlPhase1 = `
proxies:
  - name: "Node Alpha"
    type: ss
    server: 198.51.100.11
    port: 8388
    cipher: aes-128-gcm
    password: pass-alpha-1
  - name: "Node Beta"
    type: ss
    server: 198.51.100.12
    port: 8388
    cipher: aes-128-gcm
    password: pass-beta-1
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlPhase1), ContentDigest: "digest-p1"})
	res1, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res1.NodesValid != 2 {
		t.Fatalf("Phase 1 reconcile failed: %v", err)
	}

	nodesP1, totalP1, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalP1 != 2 {
		t.Fatalf("Phase 1 expected 2 nodes, got total=%d", totalP1)
	}
	var alphaID string
	var alphaRev1 int64
	for _, n := range nodesP1 {
		if n.DisplayName == "Node Alpha" {
			alphaID = n.LogicalID
			alphaRev1 = n.ConnectionRevision
		}
	}
	if alphaID == "" || alphaRev1 != 1 {
		t.Fatalf("Phase 1 Node Alpha not found or rev != 1 (id=%q, rev=%d)", alphaID, alphaRev1)
	}

	// --- Phase 2: 1 node present (Alpha temporarily omitted, only Beta) ---
	const yamlPhase2 = `
proxies:
  - name: "Node Beta"
    type: ss
    server: 198.51.100.12
    port: 8388
    cipher: aes-128-gcm
    password: pass-beta-1
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlPhase2), ContentDigest: "digest-p2"})
	res2, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res2.NodesValid != 1 {
		t.Fatalf("Phase 2 reconcile failed: %v", err)
	}

	// Invariant: Non-destructive merge preserves Node Alpha, and does NOT prune its source edge!
	_, totalP2, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalP2 != 2 {
		t.Fatalf("Phase 2 expected both nodes preserved, got %d", totalP2)
	}
	alphaDetailP2, err := svc.GetNodeDetail(ctx, alphaID)
	if err != nil {
		t.Fatalf("failed to get alpha detail in phase 2: %v", err)
	}
	if !alphaDetailP2.Node.Active {
		t.Fatalf("Node Alpha must remain active in Phase 2")
	}
	if len(alphaDetailP2.Sources) != 0 {
		t.Fatalf("Node Alpha live source edge must be pruned in Phase 2, got %#v", alphaDetailP2.Sources)
	}
	alphaHistP2, err := svc.GetNodeSourceHistory(ctx, alphaID)
	if err != nil || alphaHistP2.AttributionStatus != domain.AttributionStatusHistoricalVerified {
		t.Fatalf("Node Alpha provenance must be preserved as historical_verified in Phase 2, got %#v", alphaHistP2)
	}

	// --- Phase 3: 2 nodes present again, but Node Alpha has changed connection port to 8389! ---
	const yamlPhase3 = `
proxies:
  - name: "Node Alpha"
    type: ss
    server: 198.51.100.11
    port: 8389
    cipher: aes-128-gcm
    password: pass-alpha-1
  - name: "Node Beta"
    type: ss
    server: 198.51.100.12
    port: 8388
    cipher: aes-128-gcm
    password: pass-beta-1
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlPhase3), ContentDigest: "digest-p3"})
	res3, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res3.NodesValid != 2 {
		t.Fatalf("Phase 3 reconcile failed: %v", err)
	}

	_, totalP3, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalP3 != 2 {
		t.Fatalf("Phase 3 expected exactly 2 nodes (no duplicate created!), got %d", totalP3)
	}

	alphaDetailP3, err := svc.GetNodeDetail(ctx, alphaID)
	if err != nil {
		t.Fatalf("failed to get alpha detail in phase 3: %v", err)
	}
	if alphaDetailP3.Node.Port != 8389 {
		t.Fatalf("expected Node Alpha port updated to 8389, got %d", alphaDetailP3.Node.Port)
	}
	if alphaDetailP3.Node.ConnectionRevision != 2 {
		t.Fatalf("expected Node Alpha ConnectionRevision incremented to 2, got %d", alphaDetailP3.Node.ConnectionRevision)
	}
	if len(alphaDetailP3.Sources) != 1 || alphaDetailP3.Sources[0].LastSeenFetchID != res3.FetchID {
		t.Fatalf("expected source edge updated to Phase 3 fetch ID, got %#v", alphaDetailP3.Sources)
	}
}

// Invariant: Failure -> Success retains inventory during failure and seamlessly updates upon success
func TestReconcile_FailureThenSuccess_PreservesInventory(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://fail-recover.example.com/sub"
	createTestSub(t, subRepo, subID, "Fail Recover Sub", url, true)
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	const yamlInitial = `
proxies:
  - name: "Node Resilient"
    type: ss
    server: 198.51.100.30
    port: 8388
    cipher: aes-128-gcm
    password: pass-resilient
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlInitial), ContentDigest: "d-init"})
	res1, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res1.NodesValid != 1 {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Intermediate failure: network 503 error
	fetcher.setResponse(url, &fetch.Response{StatusCode: 503, Body: []byte("Service Unavailable"), ContentDigest: ""})
	res2, err := svc.ReconcileSubscription(ctx, subID)
	if err == nil || res2.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed reconcile on 503, got err=%v res=%+v", err, res2)
	}

	// Inventory must be completely preserved!
	nodesDuringFail, totalFail, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalFail != 1 || len(nodesDuringFail) != 1 {
		t.Fatalf("expected inventory preserved during failure, got total=%d", totalFail)
	}
	if nodesDuringFail[0].DisplayName != "Node Resilient" || !nodesDuringFail[0].Active {
		t.Fatalf("Node Resilient must remain active during failure")
	}

	// Subsequent recovery: 200 OK
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlInitial), ContentDigest: "d-init-recovered"})
	res3, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res3.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("expected recovery to succeed, got err=%v res=%+v", err, res3)
	}

	nodesRecovered, totalRec, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalRec != 1 || len(nodesRecovered) != 1 {
		t.Fatalf("expected inventory intact after recovery, got total=%d", totalRec)
	}
}

// Invariant: Cross-source duplicate names and same-source duplicate name ambiguity are conservatively isolated
func TestReconcile_CrossSourceAndDuplicateNameAmbiguity(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subAID := domain.MustNewUUIDv7()
	subBID := domain.MustNewUUIDv7()
	urlA := "https://dup-a.example.com/sub"
	urlB := "https://dup-b.example.com/sub"
	createTestSub(t, subRepo, subAID, "Dup Sub A", urlA, true)
	createTestSub(t, subRepo, subBID, "Dup Sub B", urlB, true)
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Sub A contains two nodes with the SAME display name but different ports in the same incoming payload
	const yamlSubA = `
proxies:
  - name: "Tokyo Edge"
    type: ss
    server: 198.51.100.51
    port: 8001
    cipher: aes-128-gcm
    password: pass-a1
  - name: "Tokyo Edge"
    type: ss
    server: 198.51.100.51
    port: 8002
    cipher: aes-128-gcm
    password: pass-a2
`
	// Sub B contains a node with the same name "Tokyo Edge" but completely different server
	const yamlSubB = `
proxies:
  - name: "Tokyo Edge"
    type: ss
    server: 198.51.100.52
    port: 8003
    cipher: aes-128-gcm
    password: pass-b1
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubA), ContentDigest: "d-da"})
	fetcher.setResponse(urlB, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubB), ContentDigest: "d-db"})

	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("Sub A reconcile failed: %v", err)
	}
	if _, err := svc.ReconcileSubscription(ctx, subBID); err != nil {
		t.Fatalf("Sub B reconcile failed: %v", err)
	}

	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 3 || len(nodes) != 3 {
		t.Fatalf("expected exactly 3 distinct nodes without cross-source merging or collision, got total=%d len=%d", total, len(nodes))
	}
}

// Invariant: User manually disabled node (active=0) is preserved and NOT automatically reactivated on refresh
func TestReconcile_UserDisabledPreservation_NotReactivated(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://user-disable.example.com/sub"
	createTestSub(t, subRepo, subID, "User Disable Sub", url, true)
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	const yamlSub = `
proxies:
  - name: "Node User Disabled"
    type: ss
    server: 198.51.100.60
    port: 8388
    cipher: aes-128-gcm
    password: pass-user-disable
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlSub), ContentDigest: "d-ud-1"})
	res1, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res1.NodesValid != 1 {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 1 {
		t.Fatalf("expected 1 node, got %d", total)
	}
	nodeID := nodes[0].LogicalID

	// User explicitly deactivates this node
	_, err = db.ExecContext(ctx, "UPDATE nodes SET active = 0 WHERE logical_id = ?", nodeID)
	if err != nil {
		t.Fatalf("failed to update node active=0: %v", err)
	}

	// Verify node is currently inactive
	det, err := svc.GetNodeDetail(ctx, nodeID)
	if err != nil || det.Node.Active {
		t.Fatalf("node should be inactive before refresh")
	}

	// Subscription refreshes with the same node
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(yamlSub), ContentDigest: "d-ud-2"})
	res2, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res2.NodesValid != 1 {
		t.Fatalf("second reconcile failed: %v", err)
	}

	// INVARIANT: Node MUST STILL be active=0 (user disabled state preserved!)
	detAfter, err := svc.GetNodeDetail(ctx, nodeID)
	if err != nil {
		t.Fatalf("failed to get node after refresh: %v", err)
	}
	if detAfter.Node.Active {
		t.Fatalf("CRITICAL: User disabled node was erroneously reactivated to active=1 on subscription refresh!")
	}
}

// P0 Root Cause 1: Sub A and Sub B have SAME host+port+protocol, DIFFERENT display_name, and DIFFERENT UUIDs.
// Both nodes must be isolated and revisions independent.
func TestReconcile_SameHostPortProtocol_DifferentDisplayName_CrossSourceIsolation(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subAID := domain.MustNewUUIDv7()
	subBID := domain.MustNewUUIDv7()
	urlA := "https://vless-a.example.com/sub"
	urlB := "https://vless-b.example.com/sub"
	createTestSub(t, subRepo, subAID, "VLESS Sub A", urlA, true)
	createTestSub(t, subRepo, subBID, "VLESS Sub B", urlB, true)

	const yamlSubA = `
proxies:
  - name: "Tokyo Edge Gateway A"
    type: vless
    server: 203.0.113.88
    port: 443
    uuid: 11111111-1111-1111-1111-111111111111
`
	const yamlSubB = `
proxies:
  - name: "Tokyo Edge Gateway B"
    type: vless
    server: 203.0.113.88
    port: 443
    uuid: 22222222-2222-2222-2222-222222222222
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubA), ContentDigest: "d-va"})
	fetcher.setResponse(urlB, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubB), ContentDigest: "d-vb"})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("reconcile Sub A failed: %v", err)
	}
	if _, err := svc.ReconcileSubscription(ctx, subBID); err != nil {
		t.Fatalf("reconcile Sub B failed: %v", err)
	}

	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 || len(nodes) != 2 {
		t.Fatalf("expected 2 isolated nodes, got total=%d len=%d", total, len(nodes))
	}

	var uuidA, uuidB string
	for _, n := range nodes {
		det, err := svc.GetNode(ctx, n.LogicalID)
		if err != nil {
			t.Fatalf("GetNode failed: %v", err)
		}
		if det.DisplayName == "Tokyo Edge Gateway A" {
			uuidA = det.Credentials.UUID
		}
		if det.DisplayName == "Tokyo Edge Gateway B" {
			uuidB = det.Credentials.UUID
		}
	}
	if uuidA != "11111111-1111-1111-1111-111111111111" || uuidB != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("UUID mismatch or overwrite: uuidA=%s, uuidB=%s", uuidA, uuidB)
	}
}

// P0 Root Cause 1: Manual node candidate conflict is NEVER overwritten.
func TestReconcile_ManualNodeCandidateConflict_NeverOverwritten(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	// 1. User created a manual node directly with server 203.0.113.99:8388
	candidateID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "203.0.113.99", 8388, map[string]string{"network": "tcp"})
	nowStr := domain.NowUTC().Format("2006-01-02T15:04:05.000Z")
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'Manual Core Node', '203.0.113.99', 8388, '{"method":"aes-128-gcm","password":"manual-secret-do-not-overwrite"}', 1, ?, ?, 1);
	`, candidateID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert manual node: %v", err)
	}

	// 2. A subscription is fetched containing a proxy on the EXACT SAME server and port, but with different secret
	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub-manual-conflict"
	createTestSub(t, subRepo, subID, "Sub Conflict", url, true)

	const subYAML = `
proxies:
  - name: "Incoming Sub Node"
    type: ss
    server: 203.0.113.99
    port: 8388
    cipher: aes-128-gcm
    password: sub-different-secret
`
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(subYAML), ContentDigest: "d-mc"})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("reconcile subscription failed: %v", err)
	}

	// Assert: Manual node must be preserved completely intact!
	manualNode, err := svc.GetNode(ctx, candidateID)
	if err != nil {
		t.Fatalf("failed to get manual node: %v", err)
	}
	if manualNode.DisplayName != "Manual Core Node" || manualNode.Credentials.Password != "manual-secret-do-not-overwrite" {
		t.Fatalf("CRITICAL: manual node was overwritten by subscription! got name=%q pass=%q", manualNode.DisplayName, manualNode.Credentials.Password)
	}

	// Assert: Total nodes must be 2 (manual node + subscription isolated node)
	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 || len(nodes) != 2 {
		t.Fatalf("expected 2 active nodes (manual + sub isolated), got total=%d len=%d", total, len(nodes))
	}
}

// P0 Root Cause 1: Shared legacy node connection update performs copy-on-write,
// updating ONLY the modifying subscription while leaving other subscriptions and policy group edges untouched.
func TestReconcile_SharedLegacyNode_CopyOnWriteOnConnectionChange(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subAID := domain.MustNewUUIDv7()
	subBID := domain.MustNewUUIDv7()
	urlA := "https://cow-a.example.com/sub"
	urlB := "https://cow-b.example.com/sub"
	createTestSub(t, subRepo, subAID, "COW Sub A", urlA, true)
	createTestSub(t, subRepo, subBID, "COW Sub B", urlB, true)

	// Step 1: Sub A and Sub B initially share identical node "Shared Gateway" on 203.0.113.50:8388
	const yamlInitial = `
proxies:
  - name: "Shared Gateway"
    type: ss
    server: 203.0.113.50
    port: 8388
    cipher: aes-128-gcm
    password: shared-secret
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlInitial), ContentDigest: "d-init-a"})
	fetcher.setResponse(urlB, &fetch.Response{StatusCode: 200, Body: []byte(yamlInitial), ContentDigest: "d-init-b"})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("sub A initial reconcile failed: %v", err)
	}
	if _, err := svc.ReconcileSubscription(ctx, subBID); err != nil {
		t.Fatalf("sub B initial reconcile failed: %v", err)
	}

	// Verify ledger has 1 shared node with 2 sources
	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 1 {
		t.Fatalf("expected 1 shared node initially, got %d", total)
	}
	sharedNodeID := nodes[0].LogicalID

	// Create a policy group edge pointing to this shared node to verify policy edge preservation
	nowStr := domain.NowUTC().Format("2006-01-02T15:04:05.000Z")
	policyGroupID := domain.MustNewUUIDv7()
	edgeID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `INSERT INTO node_groups (id, name, group_type, created_at, updated_at) VALUES (?, 'Test Group', 'select', ?, ?);`, policyGroupID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert test group: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO group_edges (id, parent_group_id, node_logical_id, position) VALUES (?, ?, ?, 0);`, edgeID, policyGroupID, sharedNodeID)
	if err != nil {
		t.Fatalf("failed to insert test group edge: %v", err)
	}

	// Step 2: Sub A updates its connection: port changed from 8388 to 8389!
	const yamlSubAChanged = `
proxies:
  - name: "Shared Gateway"
    type: ss
    server: 203.0.113.50
    port: 8389
    cipher: aes-128-gcm
    password: shared-secret
`
	fetcher.setResponse(urlA, &fetch.Response{StatusCode: 200, Body: []byte(yamlSubAChanged), ContentDigest: "d-changed-a"})
	if _, err := svc.ReconcileSubscription(ctx, subAID); err != nil {
		t.Fatalf("sub A updated reconcile failed: %v", err)
	}

	// Assert: Copy-on-Write occurred!
	// Total active nodes must now be 2: Sub B's legacy node (port 8388) + Sub A's new isolated node (port 8389)
	allNodes, totalAfter, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalAfter != 2 || len(allNodes) != 2 {
		t.Fatalf("expected 2 active nodes after copy-on-write, got total=%d len=%d", totalAfter, len(allNodes))
	}

	// Sub B's legacy node MUST remain on port 8388, revision 1!
	legacyDet, err := svc.GetNodeDetail(ctx, sharedNodeID)
	if err != nil {
		t.Fatalf("failed to get legacy node detail: %v", err)
	}
	if legacyDet.Node.Port != 8388 || legacyDet.Node.ConnectionRevision != 1 {
		t.Fatalf("CRITICAL: legacy node was corrupted by Sub A! port=%d rev=%d", legacyDet.Node.Port, legacyDet.Node.ConnectionRevision)
	}
	if len(legacyDet.Sources) != 1 || legacyDet.Sources[0].SubscriptionID != subBID {
		t.Fatalf("legacy node must belong only to Sub B now, got sources: %#v", legacyDet.Sources)
	}

	// Policy edge MUST still point to legacyNodeID (conservative preservation, not deleted)
	var edgeTargetNode string
	err = db.QueryRowContext(ctx, "SELECT node_logical_id FROM group_edges WHERE id = ?;", edgeID).Scan(&edgeTargetNode)
	if err != nil || edgeTargetNode != sharedNodeID {
		t.Fatalf("policy group edge was corrupted! want %s, got %s (err: %v)", sharedNodeID, edgeTargetNode, err)
	}
}
