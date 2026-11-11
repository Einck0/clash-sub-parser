package inventory_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

type cancelAfterHistoryRead struct {
	domain.NodeSourceHistoryRepository
	cancel context.CancelFunc
}

func (r cancelAfterHistoryRead) ListByNodeLogicalID(ctx context.Context, nodeID string) ([]domain.NodeSourceHistory, error) {
	records, err := r.NodeSourceHistoryRepository.ListByNodeLogicalID(ctx, nodeID)
	if err == nil {
		r.cancel()
	}
	return records, err
}

func TestSourceHistoryReadErrorsAreReturned(t *testing.T) {
	for _, failure := range []string{"current_query", "current_scan", "subscription_query"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			db, subs, fetches, nodes, sources := setupTestEnv(t)
			fetcher := newMockFetcher()
			id, url := domain.MustNewUUIDv7(), "https://source-history.invalid/feed"
			createTestSub(t, subs, id, "History", url, true)
			fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(clashYAMLSharedNode), ContentDigest: "history"})
			svc := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
			if _, err := svc.ReconcileSubscription(ctx, id); err != nil {
				t.Fatal(err)
			}
			active, _, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
			if err != nil || len(active) != 1 {
				t.Fatalf("expected one active node: %v", err)
			}

			var want string
			switch failure {
			case "current_query":
				if _, err := db.ExecContext(ctx, "DROP TABLE node_sources"); err != nil {
					t.Fatal(err)
				}
				want = "failed to query current node sources"
			case "current_scan":
				if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET enabled='invalid' WHERE id=?", id); err != nil {
					t.Fatal(err)
				}
				want = "failed to scan current node source"
			case "subscription_query":
				if err := sqlite.NewNodeSourceHistoryRepository(db).InsertBatch(ctx, []domain.NodeSourceHistory{{
					ID: domain.MustNewUUIDv7(), NodeLogicalID: active[0].LogicalID,
					SubscriptionID: &id, SourceIdentity: "error:source", SourceLabel: "History",
					RelationState: domain.RelationStateVerified, Cause: domain.CauseRefreshRemoved,
					EvidenceKind: "refresh_snapshot", EvidenceKey: "error:history", EvidenceJSON: "{}",
					CreatedAt: domain.NowUTC(),
				}}); err != nil {
					t.Fatal(err)
				}
				// Cancel after the history read, at the actual database query boundary.
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				repo := cancelAfterHistoryRead{NodeSourceHistoryRepository: sqlite.NewNodeSourceHistoryRepository(db), cancel: cancel}
				svc = inventory.NewService(db, subs, fetches, nodes, sources, fetcher, inventory.WithNodeSourceHistoryRepository(repo))
				want = "failed to query subscriptions for node source history"
			}
			result, err := svc.GetNodeSourceHistory(ctx, active[0].LogicalID)
			if result != nil || err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("result=%+v error=%v; expected %s", result, err, want)
			}
			if failure == "subscription_query" && !errors.Is(err, context.Canceled) {
				t.Fatalf("subscription query error was not preserved: %v", err)
			}
		})
	}
}

func TestSourceHistorySubscriptionSnapshotOverHundred(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	fetcher := newMockFetcher()
	id, url := domain.MustNewUUIDv7(), "https://source-history.invalid/feed"
	createTestSub(t, subs, id, "History", url, true)
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(clashYAMLSharedNode), ContentDigest: "history"})
	svc := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, id); err != nil {
		t.Fatal(err)
	}
	active, _, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(active) != 1 {
		t.Fatalf("expected one active node, got %d: %v", len(active), err)
	}

	var history []domain.NodeSourceHistory
	for i := 0; i < 125; i++ {
		subID := domain.MustNewUUIDv7()
		createTestSub(t, subs, subID, fmt.Sprintf("History %d", i), fmt.Sprintf("https://source-history.invalid/%d", i), i%2 == 0)
		history = append(history, domain.NodeSourceHistory{
			ID: domain.MustNewUUIDv7(), NodeLogicalID: active[0].LogicalID,
			SubscriptionID: &subID, SourceIdentity: fmt.Sprintf("snapshot:source:%d", i), SourceLabel: fmt.Sprintf("History %d", i),
			RelationState: domain.RelationStateVerified, Cause: domain.CauseRefreshRemoved,
			EvidenceKind: "refresh_snapshot", EvidenceKey: fmt.Sprintf("snapshot:history:%d", i), EvidenceJSON: "{}",
			CreatedAt: domain.NowUTC(),
		})
	}
	if err := sqlite.NewNodeSourceHistoryRepository(db).InsertBatch(ctx, history); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.GetNodeSourceHistory(ctx, active[0].LogicalID)
	if err != nil || len(resp.History) != len(history) {
		t.Fatalf("expected %d history items, response=%+v err=%v", len(history), resp, err)
	}
	for _, item := range resp.History {
		if item.SourceDeleted || item.SourceUnmapped {
			t.Errorf("existing source misclassified: %+v", item)
		}
	}
}

func TestSourceHistoryWriteChain_RefreshPruning(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Provider Alpha", url, true)

	// Fetch 1: 2 nodes (Node 1 and Node 2)
	const yaml1 = `
proxies:
  - name: "Node 1"
    type: ss
    server: 1.1.1.1
    port: 8388
    cipher: aes-128-gcm
    password: pass-1
  - name: "Node 2"
    type: ss
    server: 1.1.1.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-2
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(yaml1),
		ContentDigest: "digest-1",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	res1, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res1.NodesValid != 2 {
		t.Fatalf("reconcile 1 failed: %v", err)
	}

	nodes, _, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(nodes) != 2 {
		t.Fatalf("expected 2 active nodes initially, got %d", len(nodes))
	}
	var node1ID, node2ID string
	for _, n := range nodes {
		if n.DisplayName == "Node 1" {
			node1ID = n.LogicalID
		} else if n.DisplayName == "Node 2" {
			node2ID = n.LogicalID
		}
	}

	// Verify initially 0 history records
	hRepo := sqlite.NewNodeSourceHistoryRepository(db)
	hList1, err := hRepo.ListByNodeLogicalID(ctx, node1ID)
	if err != nil || len(hList1) != 0 {
		t.Fatalf("expected 0 history records initially, got %d", len(hList1))
	}

	// Fetch 2: Node 1 is removed upstream, only Node 2 remains
	const yaml2 = `
proxies:
  - name: "Node 2"
    type: ss
    server: 1.1.1.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-2
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(yaml2),
		ContentDigest: "digest-2",
	})

	res2, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || res2.NodesValid != 1 {
		t.Fatalf("reconcile 2 failed: %v", err)
	}

	// Verify Node 1 is pruned from live node_sources
	liveSources1, err := sourceRepo.ListByNode(ctx, node1ID)
	if err != nil || len(liveSources1) != 0 {
		t.Fatalf("expected Node 1 to be pruned from live node_sources, got %d", len(liveSources1))
	}

	// Verify Node 1 active status remains TRUE (non-destructive invariant)
	node1, err := svc.GetNode(ctx, node1ID)
	if err != nil || !node1.Active {
		t.Fatalf("expected Node 1 active to remain true, got active=%v", node1.Active)
	}

	// Verify Node 1 provenance is captured in node_source_history
	hist1, err := hRepo.ListByNodeLogicalID(ctx, node1ID)
	if err != nil || len(hist1) != 1 {
		t.Fatalf("expected 1 history record for Node 1, got %d", len(hist1))
	}
	if hist1[0].Cause != domain.CauseRefreshRemoved {
		t.Errorf("expected cause refresh_removed, got %s", hist1[0].Cause)
	}
	if hist1[0].RelationState != domain.RelationStateVerified {
		t.Errorf("expected relation_state verified, got %s", hist1[0].RelationState)
	}
	if hist1[0].SourceLabel != "Provider Alpha" {
		t.Errorf("expected source_label 'Provider Alpha', got %s", hist1[0].SourceLabel)
	}

	// Verify Node 2 still has live source and 0 history records
	liveSources2, err := sourceRepo.ListByNode(ctx, node2ID)
	if err != nil || len(liveSources2) != 1 {
		t.Fatalf("expected Node 2 to have 1 live source, got %d", len(liveSources2))
	}
	hist2, err := hRepo.ListByNodeLogicalID(ctx, node2ID)
	if err != nil || len(hist2) != 0 {
		t.Fatalf("expected 0 history records for Node 2, got %d", len(hist2))
	}

	// Verify API response for Node 1
	apiResp1, err := svc.GetNodeSourceHistory(ctx, node1ID)
	if err != nil {
		t.Fatalf("GetNodeSourceHistory failed: %v", err)
	}
	if len(apiResp1.CurrentSources) != 0 {
		t.Errorf("expected 0 current sources for Node 1, got %d", len(apiResp1.CurrentSources))
	}
	if len(apiResp1.History) != 1 {
		t.Errorf("expected 1 history item for Node 1, got %d", len(apiResp1.History))
	}
	if apiResp1.AttributionStatus != domain.AttributionStatusHistoricalVerified {
		t.Errorf("expected historical_verified status for Node 1, got %s", apiResp1.AttributionStatus)
	}
	if apiResp1.History[0].SourceDeleted {
		t.Errorf("expected SourceDeleted=false because subscription still exists, got true")
	}

	// Verify API response for Node 2
	apiResp2, err := svc.GetNodeSourceHistory(ctx, node2ID)
	if err != nil {
		t.Fatalf("GetNodeSourceHistory failed: %v", err)
	}
	if len(apiResp2.CurrentSources) != 1 {
		t.Errorf("expected 1 current source for Node 2, got %d", len(apiResp2.CurrentSources))
	}
	if apiResp2.AttributionStatus != domain.AttributionStatusCurrent {
		t.Errorf("expected current status for Node 2, got %s", apiResp2.AttributionStatus)
	}
}

func TestSourceHistoryWriteChain_FetchFailureAndSubscriptionToggle(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Toggle Sub", url, true)

	const yaml = `
proxies:
  - name: "Node Static"
    type: ss
    server: 1.1.1.10
    port: 8388
    cipher: aes-128-gcm
    password: pass-static
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(yaml),
		ContentDigest: "digest-static",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	hRepo := sqlite.NewNodeSourceHistoryRepository(db)

	// Fetch failure: should NOT create history and NOT prune node_sources
	fetcher.setError(url, fmt.Errorf("network connection refused"))
	if _, err := svc.ReconcileSubscription(ctx, subID); err == nil {
		t.Fatalf("expected error on fetch failure")
	}

	nodes, _, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("node should remain active on fetch failure")
	}
	hList, err := hRepo.ListByNodeLogicalID(ctx, nodes[0].LogicalID)
	if err != nil || len(hList) != 0 {
		t.Fatalf("no history records should be created on fetch failure, got %d", len(hList))
	}

	// Toggle subscription disabled / enabled: should NOT prune node_sources and NOT create history
	auditRepo := sqlite.NewAuditRepository(db)
	subSvc := subscription.NewService(subRepo, auditRepo)
	subObj, err := subRepo.GetByID(ctx, subID)
	if err != nil {
		t.Fatalf("get sub failed: %v", err)
	}

	// Disable
	disabled := false
	_, err = subSvc.Update(ctx, subscription.UpdateSubscriptionCommand{
		ID:        subID,
		Revision:  subObj.Revision,
		Enabled:   &disabled,
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("disable subscription failed: %v", err)
	}

	// Check node_sources still intact
	sourcesAfterDisable, err := sourceRepo.ListByNode(ctx, nodes[0].LogicalID)
	if err != nil || len(sourcesAfterDisable) != 1 {
		t.Fatalf("node sources should remain intact after sub disable, got %d", len(sourcesAfterDisable))
	}
	hAfterDisable, _ := hRepo.ListByNodeLogicalID(ctx, nodes[0].LogicalID)
	if len(hAfterDisable) != 0 {
		t.Fatalf("no history should be created on sub disable, got %d", len(hAfterDisable))
	}
}

func TestSourceHistoryWriteChain_SubscriptionDeleteCascade(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub"
	createTestSub(t, subRepo, subID, "Ephemeral Sub", url, true)

	const yaml = `
proxies:
  - name: "Node Ephemeral"
    type: ss
    server: 2.2.2.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-ephemeral
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(yaml),
		ContentDigest: "digest-ephemeral",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	nodes, _, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	nodeID := nodes[0].LogicalID

	// Delete subscription
	if err := subRepo.Delete(ctx, subID); err != nil {
		t.Fatalf("subRepo.Delete failed: %v", err)
	}

	// Verify subscription is deleted
	_, err = subRepo.GetByID(ctx, subID)
	if err == nil {
		t.Fatalf("expected subscription to be deleted")
	}

	// Verify history record was created and subscription_id was SET NULL by database cascade
	hRepo := sqlite.NewNodeSourceHistoryRepository(db)
	history, err := hRepo.ListByNodeLogicalID(ctx, nodeID)
	if err != nil || len(history) != 1 {
		t.Fatalf("expected 1 history record after sub delete, got %d (err: %v)", len(history), err)
	}

	rec := history[0]
	if rec.Cause != domain.CauseSubscriptionDeleted {
		t.Errorf("expected cause subscription_deleted, got %s", rec.Cause)
	}
	if rec.SubscriptionID != nil {
		t.Errorf("expected subscription_id to be NULL after ON DELETE SET NULL, got %s", *rec.SubscriptionID)
	}
	if rec.SourceLabel != "Ephemeral Sub" {
		t.Errorf("expected source_label 'Ephemeral Sub' to be preserved, got %s", rec.SourceLabel)
	}
	if rec.SourceIdentity != "sub:"+subID {
		t.Errorf("expected source_identity 'sub:%s', got %s", subID, rec.SourceIdentity)
	}

	// Verify API response reports SourceDeleted = true
	apiResp, err := svc.GetNodeSourceHistory(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetNodeSourceHistory failed: %v", err)
	}
	if len(apiResp.CurrentSources) != 0 {
		t.Errorf("expected 0 current sources, got %d", len(apiResp.CurrentSources))
	}
	if len(apiResp.History) != 1 {
		t.Fatalf("expected 1 history record in API response, got %d", len(apiResp.History))
	}
	if !apiResp.History[0].SourceDeleted {
		t.Errorf("expected SourceDeleted=true after subscription delete")
	}
	if apiResp.AttributionStatus != domain.AttributionStatusHistoricalVerified {
		t.Errorf("expected attribution_status historical_verified, got %s", apiResp.AttributionStatus)
	}
}
