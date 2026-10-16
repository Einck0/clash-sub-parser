package inventory_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

func TestPrefix_PrependingAndRetroactiveRenaming(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://prefix-test.example.com/sub"
	payload := `
proxies:
  - name: "Node 1"
    type: ss
    server: 198.51.100.10
    port: 8388
    cipher: aes-128-gcm
    password: pass1
  - name: "[HK] Node 2"
    type: ss
    server: 198.51.100.11
    port: 8388
    cipher: aes-128-gcm
    password: pass2
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(payload),
		ContentType:   "text/yaml",
		ContentDigest: "sha256:prefix_test_1",
	})

	now := time.Now().UTC()
	sub := &domain.Subscription{
		ID:                 subID,
		Name:               "Prefix Sub",
		SourceURLSecretRef: url,
		Enabled:            true,
		Prefix:             "[HK] ",
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds: 86400,
			TimeoutSeconds:  30,
		},
		Revision:  domain.MustNewUUIDv7(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("failed to create sub: %v", err)
	}

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("expected success, got: %s", res.Outcome)
	}

	nodes, err := nodeRepo.ListAll(ctx, domain.NodeFilter{})
	if err != nil {
		t.Fatalf("failed to list nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	for _, n := range nodes {
		if n.Server == "198.51.100.10" && n.DisplayName != "[HK] Node 1" {
			t.Fatalf("expected display name '[HK] Node 1', got %q", n.DisplayName)
		}
		if n.Server == "198.51.100.11" && n.DisplayName != "[HK] Node 2" {
			t.Fatalf("expected display name '[HK] Node 2' without duplicate prefix, got %q", n.DisplayName)
		}
	}

	// Now retroactively change prefix to "[SG] "
	sub.Prefix = "[SG] "
	if err := subRepo.Update(ctx, sub); err != nil {
		t.Fatalf("UpdateSubscriptionPrefix failed: %v", err)
	}

	nodesAfterUpdate, err := nodeRepo.ListAll(ctx, domain.NodeFilter{})
	if err != nil {
		t.Fatalf("failed to list nodes after update: %v", err)
	}
	for _, n := range nodesAfterUpdate {
		if n.Server == "198.51.100.10" && n.DisplayName != "[SG] Node 1" {
			t.Fatalf("expected updated display name '[SG] Node 1', got %q", n.DisplayName)
		}
	}

	// Now clear prefix to "" (should restore raw_name)
	sub.Prefix = ""
	if err := subRepo.Update(ctx, sub); err != nil {
		t.Fatalf("UpdateSubscriptionPrefix empty failed: %v", err)
	}

	nodesAfterClear, err := nodeRepo.ListAll(ctx, domain.NodeFilter{})
	if err != nil {
		t.Fatalf("failed to list nodes after clear: %v", err)
	}
	for _, n := range nodesAfterClear {
		if n.Server == "198.51.100.10" && n.DisplayName != "Node 1" {
			t.Fatalf("expected clean raw display name 'Node 1', got %q", n.DisplayName)
		}
	}
}

func TestManualNodes_ProtectedFromSubscriptionDeletionAndInScope(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	// 1. Insert a manual node with NO node_sources
	manualNodeID := "manual_node_warp_test"
	manualNode := domain.Node{
		LogicalID:   manualNodeID,
		Protocol:    domain.ProtocolWireGuard,
		DisplayName: "Manual-WARP",
		Server:      "engage.cloudflareclient.com",
		Port:        2408,
		Active:      true,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{manualNode}); err != nil {
		t.Fatalf("failed to insert manual node: %v", err)
	}

	// 2. Ingest subscription with 1 node
	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(clashYAMLSharedNode),
		ContentType:   "text/yaml",
		ContentDigest: "sha256:sub_manual_test",
	})
	now := time.Now().UTC()
	sub := &domain.Subscription{
		ID:                 subID,
		Name:               "Sub A",
		SourceURLSecretRef: url,
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds: 86400,
			TimeoutSeconds:  30,
		},
		Revision:  domain.MustNewUUIDv7(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("failed to create sub: %v", err)
	}

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// 3. Verify NodeScopeEnabledSubscriptions returns BOTH the subscription node and the manual node
	scopedNodes, err := nodeRepo.ListAll(ctx, domain.NodeFilter{
		Scope:      domain.NodeScopeEnabledSubscriptions,
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListAll with scope failed: %v", err)
	}
	if len(scopedNodes) != 2 {
		t.Fatalf("expected 2 nodes under NodeScopeEnabledSubscriptions, got %d", len(scopedNodes))
	}
	hasManual := false
	for _, n := range scopedNodes {
		if n.LogicalID == manualNodeID {
			hasManual = true
		}
	}
	if !hasManual {
		t.Fatalf("manual node missing from NodeScopeEnabledSubscriptions")
	}

	// 4. Delete the subscription
	if err := subRepo.Delete(ctx, subID); err != nil {
		t.Fatalf("failed to delete subscription: %v", err)
	}

	// 5. Verify manual node is STILL active, while subscription node was deactivated
	manNode, err := nodeRepo.GetByLogicalID(ctx, manualNodeID)
	if err != nil {
		t.Fatalf("failed to get manual node: %v", err)
	}
	if !manNode.Active {
		t.Fatalf("manual node was deactivated after subscription delete!")
	}

	// Verify NodeScopeEnabledSubscriptions still returns the manual node
	remainingScoped, err := nodeRepo.ListAll(ctx, domain.NodeFilter{
		Scope:      domain.NodeScopeEnabledSubscriptions,
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListAll after delete failed: %v", err)
	}
	if len(remainingScoped) != 1 || remainingScoped[0].LogicalID != manualNodeID {
		t.Fatalf("expected only manual node under NodeScopeEnabledSubscriptions, got: %+v", remainingScoped)
	}
}
