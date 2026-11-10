package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// TestSubscriptionFetchOrder_EqualStartedAt_RowIDTieBreaksNewest tests that when multiple
// fetches share the exact same started_at timestamp, the repository queries break ties by
// insertion order (rowid DESC) rather than lexical UUID string order. This ensures that
// when an older fetch has a lexically larger UUID than a newer fetch, the newer fetch is
// consistently selected for active node scope membership and subscription metadata.
func TestSubscriptionFetchOrder_EqualStartedAt_RowIDTieBreaksNewest(t *testing.T) {
	ctx := context.Background()
	db, subRepo := newSubscriptionsTestDB(t)
	nodeRepo := sqlite.NewNodeRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)

	subID := "01920000-0000-7000-8000-000000000001"
	sub := &domain.Subscription{
		ID:                 subID,
		Name:               "Fetch Order Tie-Break Sub",
		SourceURLSecretRef: "secret://order-regression",
		Enabled:            true,
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          domain.NowUTC().Add(-1 * time.Hour),
		UpdatedAt:          domain.NowUTC().Add(-1 * time.Hour),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("Create sub failed: %v", err)
	}

	// Force same started_at timestamp.
	sameStartedAt := "2026-11-08T12:00:00Z"
	olderFinishedAt := "2026-11-08T12:00:01Z"
	newerFinishedAt := "2026-11-08T12:00:02Z"

	// Older fetch has a lexically HIGH UUID.
	// Newer fetch has a lexically LOW UUID.
	olderFetchID := "0192ffff-ffff-7fff-bfff-ffffffffffff"
	newerFetchID := "01920000-0000-7000-8000-000000000000"

	if olderFetchID <= newerFetchID {
		t.Fatalf("invariant broken: olderFetchID %s must be > newerFetchID %s", olderFetchID, newerFetchID)
	}

	// 1. Persist older fetch first (rowid 1, outcome success)
	insertTestFetch(t, db, olderFetchID, sub.ID, sameStartedAt, olderFinishedAt, "success")

	// 2. Persist newer fetch second (rowid 2, outcome success)
	insertTestFetch(t, db, newerFetchID, sub.ID, sameStartedAt, newerFinishedAt, "success")

	// 3. Seed active nodes: one manual node, one current subscription node, one pruned subscription node.
	now := domain.NowUTC()
	manualNode := domain.Node{
		LogicalID:   "node-manual-warp",
		Protocol:    domain.ProtocolWireGuard,
		DisplayName: "WARP-manual-fixture",
		Server:      "198.51.100.8",
		Port:        2408,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	currentNode := domain.Node{
		LogicalID:   "node-current-fixture",
		Protocol:    domain.ProtocolSS,
		DisplayName: "A WARP-current-fixture",
		Server:      "198.51.100.11",
		Port:        8388,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	prunedNode := domain.Node{
		LogicalID:   "node-pruned-fixture",
		Protocol:    domain.ProtocolSS,
		DisplayName: "A WARP-pruned-fixture",
		Server:      "198.51.100.10",
		Port:        8388,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{manualNode, currentNode, prunedNode}); err != nil {
		t.Fatalf("UpsertBatch failed: %v", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO node_source_history(id,node_logical_id,source_identity,source_label,relation_state,cause,evidence_kind,evidence_key,created_at) VALUES ('manual-fixture',?,'manual','manual','verified','manual_confirmed','fixture','manual-fixture',?)`, manualNode.LogicalID, now); err != nil {
		t.Fatal(err)
	}

	// Current node sources points to newerFetchID (rowid 2).
	// Pruned node sources points to olderFetchID (rowid 1).
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?), (?, ?, ?);
	`, currentNode.LogicalID, sub.ID, newerFetchID, prunedNode.LogicalID, sub.ID, olderFetchID)
	if err != nil {
		t.Fatalf("insert node_sources failed: %v", err)
	}

	// 4. Verify Node Inventory Scope Membership (enabledSubscriptionsInventorySubquery)
	// Under rowid DESC tie-break, the latest fetch is newerFetchID.
	// Therefore, currentNode (last_seen_fetch_id = newerFetchID) and manualNode (standalone)
	// must be included. prunedNode (last_seen_fetch_id = olderFetchID) must be excluded.
	scopeNodes, err := nodeRepo.ListAll(ctx, domain.NodeFilter{
		Scope:          domain.NodeScopeEnabledSubscriptions,
		ActiveOnly:     true,
		ExcludeNotices: true,
	})
	if err != nil {
		t.Fatalf("nodeRepo.ListAll in enabled_subscriptions scope failed: %v", err)
	}

	foundManual := false
	foundCurrent := false
	foundPruned := false
	for _, n := range scopeNodes {
		switch n.LogicalID {
		case manualNode.LogicalID:
			foundManual = true
		case currentNode.LogicalID:
			foundCurrent = true
		case prunedNode.LogicalID:
			foundPruned = true
		}
	}

	if !foundManual {
		t.Errorf("manual node %s should be included in enabled subscription scope", manualNode.LogicalID)
	}
	if !foundCurrent {
		t.Errorf("current node %s (last_seen=%s) MUST be included in enabled subscription scope under rowid tie-break", currentNode.LogicalID, newerFetchID)
	}
	if foundPruned {
		t.Errorf("pruned node %s (last_seen=%s) MUST be excluded from enabled subscription scope", prunedNode.LogicalID, olderFetchID)
	}

	// 5. Verify Subscription Detail (GetByID -> attachMetadata -> attachLatestFetches & attachNodeCounts)
	detailSub, err := subRepo.GetByID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("subRepo.GetByID failed: %v", err)
	}
	if detailSub.LastRefreshedAt == nil {
		t.Fatal("expected non-nil LastRefreshedAt on detail subscription")
	}
	if detailSub.LastRefreshedAt.Format(time.RFC3339) != newerFinishedAt {
		t.Errorf("detailSub.LastRefreshedAt mismatch: want %s (newer rowid), got %s", newerFinishedAt, detailSub.LastRefreshedAt.Format(time.RFC3339))
	}
	if detailSub.LastRefreshOutcome == nil || *detailSub.LastRefreshOutcome != domain.FetchOutcomeSuccess {
		t.Errorf("detailSub.LastRefreshOutcome mismatch: want success, got %v", detailSub.LastRefreshOutcome)
	}
	if detailSub.SourceNodeCount != 1 {
		t.Errorf("detailSub.SourceNodeCount mismatch: want 1 (only current node matches latest fetch), got %d", detailSub.SourceNodeCount)
	}
	if detailSub.NodeCount != 1 {
		t.Errorf("detailSub.NodeCount mismatch: want 1, got %d", detailSub.NodeCount)
	}

	// 6. Verify Subscription List (List -> attachMetadata)
	listSubs, total, err := subRepo.List(ctx, domain.SubscriptionFilter{})
	if err != nil || total != 1 || len(listSubs) != 1 {
		t.Fatalf("subRepo.List failed: %v (total=%d, count=%d)", err, total, len(listSubs))
	}
	listSub := listSubs[0]
	if listSub.LastRefreshedAt == nil {
		t.Fatal("expected non-nil LastRefreshedAt on listed subscription")
	}
	if listSub.LastRefreshedAt.Format(time.RFC3339) != newerFinishedAt {
		t.Errorf("listSub.LastRefreshedAt mismatch: want %s (newer rowid), got %s", newerFinishedAt, listSub.LastRefreshedAt.Format(time.RFC3339))
	}
	if listSub.SourceNodeCount != 1 {
		t.Errorf("listSub.SourceNodeCount mismatch: want 1, got %d", listSub.SourceNodeCount)
	}
	if listSub.NodeCount != 1 {
		t.Errorf("listSub.NodeCount mismatch: want 1, got %d", listSub.NodeCount)
	}

	// 7. Verify SubscriptionFetchRepository ListBySubscription ordering
	fetches, err := fetchRepo.ListBySubscription(ctx, sub.ID, 10)
	if err != nil {
		t.Fatalf("fetchRepo.ListBySubscription failed: %v", err)
	}
	if len(fetches) != 2 {
		t.Fatalf("expected 2 fetches, got %d", len(fetches))
	}
	if fetches[0].ID != newerFetchID {
		t.Errorf("expected first fetch in ListBySubscription to be newerFetchID (%s, higher rowid), got %s", newerFetchID, fetches[0].ID)
	}
	if fetches[1].ID != olderFetchID {
		t.Errorf("expected second fetch in ListBySubscription to be olderFetchID (%s, lower rowid), got %s", olderFetchID, fetches[1].ID)
	}
}
