package inventory_test

import (
	"context"
	"fmt"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

// 1. End-to-End Retention Wiring: Reconciles automatically trigger bounded lifecycle prune
// Using stable repeated nodes, intermediate payloads 2 and 3 are pruned, while newest 2 (4 and 5)
// and initial connection version provenance payload (1) are preserved.
func TestReconcile_RetentionWiring_EndToEnd(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := "sub-retention-wiring"
	url := "https://provider.example.com/wiring-sub"
	createTestSub(t, subRepo, subID, "Retention Wiring Sub", url, true)

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Perform 5 successive reconciles with stable repeated nodes
	const stableYAML = `
proxies:
  - name: "Node Alpha"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: pass-alpha
  - name: "Node Beta"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-beta
`
	for i := 1; i <= 5; i++ {
		fetcher.setResponse(url, &fetch.Response{
			StatusCode:    200,
			Body:          []byte(stableYAML),
			ContentDigest: fmt.Sprintf("digest-wire-%d", i),
		})

		res, err := svc.ReconcileSubscription(ctx, subID)
		if err != nil {
			t.Fatalf("reconcile %d failed: %v", i, err)
		}
		if res.Outcome != domain.FetchOutcomeSuccess {
			t.Fatalf("reconcile %d expected success, got %v", i, res.Outcome)
		}
	}

	// Payloads 2 and 3 must have been pruned as unprotected intermediate snapshots!
	// Surviving payloads:
	// - Payload 1 (protected by node_connection_versions.source_entry_id provenance)
	// - Payload 4 and Payload 5 (newest 2 success payloads)
	var survivingPayloads []string
	rows, err := db.QueryContext(ctx, "SELECT id FROM subscription_payloads WHERE subscription_id = ? ORDER BY created_at ASC;", subID)
	if err != nil {
		t.Fatalf("failed to query payloads: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		survivingPayloads = append(survivingPayloads, id)
	}

	// Qualify bound test: stable identical nodes retain the first snapshot as connection version provenance
	// exception + newest 2 success snapshots (count unprotected latest 2 + connection exceptions = <=3 payloads total).
	// Intermediate unprotected snapshots (2 and 3) must be pruned.
	if len(survivingPayloads) > 3 || len(survivingPayloads) == 0 {
		t.Fatalf("expected <= 3 payloads to survive for identical nodes (1 connection provenance + newest 2), found %d: %v", len(survivingPayloads), survivingPayloads)
	}

	// Prove connection source-entry provenance for the first snapshot:
	var connSourceEntryPayload string
	err = db.QueryRowContext(ctx, `
		SELECT se.payload_id
		FROM node_connection_versions ncv
		JOIN subscription_entries se ON ncv.source_entry_id = se.id
		WHERE se.subscription_id = ?
		LIMIT 1;
	`, subID).Scan(&connSourceEntryPayload)
	if err != nil {
		t.Fatalf("failed to query connection version source entry payload: %v", err)
	}
	if survivingPayloads[0] != connSourceEntryPayload {
		t.Fatalf("expected first surviving payload %s to match connection provenance payload %s", survivingPayloads[0], connSourceEntryPayload)
	}

	if len(survivingPayloads) != 3 {
		t.Fatalf("expected 3 payloads to survive (1 connection provenance + 2 newest success), found %d: %v", len(survivingPayloads), survivingPayloads)
	}
}

// 1b. Provenance Exception: Payloads without connection version provenance keep exactly newest 2
func TestReconcile_RetentionWiring_WithoutConnectionProvenance_KeepsExactTwo(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := "sub-retention-notices"
	url := "https://provider.example.com/dogegg-notices-sub"
	createTestSub(t, subRepo, subID, "Dogegg Retention Notices Sub", url, true)

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Feeds containing informational notices (do not create active node connection heads)
	for i := 1; i <= 5; i++ {
		noticeYAML := fmt.Sprintf(`
proxies:
  - name: "通知 Item %d"
    type: ss
    server: 127.0.0.1
    port: 1
    cipher: aes-128-gcm
    password: 00000000-0000-4000-8000-000000000000
`, i)
		fetcher.setResponse(url, &fetch.Response{
			StatusCode:    200,
			Body:          []byte(noticeYAML),
			ContentDigest: fmt.Sprintf("digest-notice-%d", i),
		})

		res, err := svc.ReconcileSubscription(ctx, subID)
		if err != nil {
			t.Fatalf("reconcile %d failed: %v", i, err)
		}
		if res.Outcome != domain.FetchOutcomeSuccess {
			t.Fatalf("reconcile %d expected success, got %v", i, res.Outcome)
		}
	}

	// Without connection version pins, exactly newest 2 payloads remain
	var survivingPayloads []string
	rows, err := db.QueryContext(ctx, "SELECT id FROM subscription_payloads WHERE subscription_id = ? ORDER BY created_at ASC;", subID)
	if err != nil {
		t.Fatalf("failed to query payloads: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		survivingPayloads = append(survivingPayloads, id)
	}

	if len(survivingPayloads) != 2 {
		t.Fatalf("expected exactly 2 payloads to survive without connection provenance, found %d: %v", len(survivingPayloads), survivingPayloads)
	}
}

// 2. Failure and Zero-Node Refreshes Do Not Destroy Last Usable Inventory
func TestReconcile_FailureAndZeroNode_PreservesLastUsableInventory(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := "sub-fail-preserve"
	url := "https://provider.example.com/preserve-sub"
	createTestSub(t, subRepo, subID, "Preserve Usable Sub", url, true)

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Step 1: Successful initial population with 2 valid nodes
	const initialYAML = `
proxies:
  - name: "Alpha Node"
    type: ss
    server: 198.51.100.10
    port: 8388
    cipher: aes-128-gcm
    password: pass-alpha
  - name: "Beta Node"
    type: trojan
    server: 198.51.100.11
    port: 443
    password: pass-beta
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(initialYAML),
		ContentDigest: "digest-usable-init",
	})

	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomeSuccess || res.NodesValid != 2 {
		t.Fatalf("expected 2 valid nodes, got %d", res.NodesValid)
	}

	// Verify 2 active nodes exist
	nodesInit, totalInit, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalInit != 2 || len(nodesInit) != 2 {
		t.Fatalf("expected 2 active nodes initially, got %d", totalInit)
	}

	// Step 2: Network / HTTP failure
	fetcher.setError(url, fmt.Errorf("http 503 service unavailable"))
	rFail, fErr := svc.ReconcileSubscription(ctx, subID)
	if fErr == nil {
		t.Fatalf("expected error on network failure, got nil")
	}
	if rFail == nil || rFail.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed outcome, got: %#v", rFail)
	}

	// Usable inventory MUST NOT be destroyed
	nodesAfterFail, totalAfterFail, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalAfterFail != 2 || len(nodesAfterFail) != 2 {
		t.Fatalf("expected 2 active nodes preserved after network failure, got %d", totalAfterFail)
	}

	// Step 3: Zero-node empty payload
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte("proxies: []\n"),
		ContentDigest: "digest-empty",
	})
	rEmpty, eErr := svc.ReconcileSubscription(ctx, subID)
	if eErr == nil {
		t.Fatalf("expected error on empty feed, got nil")
	}
	if rEmpty == nil || rEmpty.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed outcome on empty feed, got: %#v", rEmpty)
	}

	// Usable inventory MUST STILL be intact
	nodesAfterEmpty, totalAfterEmpty, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || totalAfterEmpty != 2 || len(nodesAfterEmpty) != 2 {
		t.Fatalf("expected 2 active nodes preserved after zero-node reconcile, got %d", totalAfterEmpty)
	}

	// Payloads and sources from the initial success must still exist
	var payloadCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads WHERE subscription_id = ?;", subID).Scan(&payloadCount)
	if payloadCount < 1 {
		t.Fatalf("last usable payload was destroyed! count=%d", payloadCount)
	}
}

// 3. Durable Override Ledger: Carries forward across refreshes and reappears after absence
func TestReconcile_DurableOverride_CarriesForwardAndReappears(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := "sub-durable-override"
	url := "https://provider.example.com/durable-override"
	createTestSub(t, subRepo, subID, "Durable Override Sub", url, true)

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Feed 1: Contains Node Alpha and Node Beta
	const feed1 = `
proxies:
  - name: "Node Alpha"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: pass-a
  - name: "Node Beta"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-b
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(feed1),
		ContentDigest: "digest-feed-1",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("feed 1 reconcile failed: %v", err)
	}

	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	entries, _, err := entryRepo.ListBySubscription(ctx, subID, 10, 0)
	if err != nil || len(entries) < 2 {
		t.Fatalf("failed to list entries: %v", err)
	}

	var alphaEntryID string
	for _, e := range entries {
		if e.RawName == "Node Alpha" {
			alphaEntryID = e.ID
			break
		}
	}
	if alphaEntryID == "" {
		t.Fatalf("could not find entry for Node Alpha")
	}

	noticeKind := domain.EntryKindNotice
	updated, err := svc.OverrideEntryKind(ctx, alphaEntryID, &noticeKind, "mark notice", "admin")
	if err != nil {
		t.Fatalf("OverrideEntryKind failed: %v", err)
	}
	if updated.UserKindOverride == nil || *updated.UserKindOverride != domain.EntryKindNotice {
		t.Fatalf("expected updated entry to have notice override, got %v", updated.UserKindOverride)
	}

	// Verify written to durable ledger
	var ledgerKind string
	err = db.QueryRowContext(ctx, "SELECT user_kind_override FROM subscription_entry_overrides WHERE subscription_id = ? AND override_anchor = ?;", subID, updated.OverrideAnchor).Scan(&ledgerKind)
	if err != nil {
		t.Fatalf("failed to read from subscription_entry_overrides: %v", err)
	}
	if ledgerKind != "notice" {
		t.Fatalf("expected ledger kind notice, got %s", ledgerKind)
	}

	// Feed 2: Node Alpha is ABSENT (feed only has Node Beta)
	const feed2 = `
proxies:
  - name: "Node Beta"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-b
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(feed2),
		ContentDigest: "digest-feed-2",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("feed 2 reconcile failed: %v", err)
	}

	// Prune old payload from feed 1
	_, err = sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("retention prune failed: %v", err)
	}

	// Feed 3: Node Alpha REAPPEARS in upstream feed!
	const feed3 = `
proxies:
  - name: "Node Alpha"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: pass-a
  - name: "Node Beta"
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: pass-b
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(feed3),
		ContentDigest: "digest-feed-3",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("feed 3 reconcile failed: %v", err)
	}

	// Verify Node Alpha's new entry has UserKindOverride = notice carried forward from durable ledger!
	latestEntries, err := entryRepo.ListLatestBySubscription(ctx, subID)
	if err != nil {
		t.Fatalf("ListLatestBySubscription failed: %v", err)
	}
	var reappearedAlpha *domain.SubscriptionEntry
	for _, e := range latestEntries {
		if e.RawName == "Node Alpha" {
			reappearedAlpha = &e
			break
		}
	}
	if reappearedAlpha == nil {
		t.Fatalf("Node Alpha not found in latest entries")
	}
	if reappearedAlpha.UserKindOverride == nil || *reappearedAlpha.UserKindOverride != domain.EntryKindNotice {
		t.Fatalf("expected Node Alpha override to be reapplied from durable ledger, got %v", reappearedAlpha.UserKindOverride)
	}
}

// 4. Cleared Override Does Not Resurrect
func TestReconcile_ClearedOverride_DoesNotResurrect(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := "sub-clear-override"
	url := "https://provider.example.com/clear-override"
	createTestSub(t, subRepo, subID, "Clear Override Sub", url, true)

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	const feedContent = `
proxies:
  - name: "Node Alpha"
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: pass-a
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(feedContent),
		ContentDigest: "digest-clear-1",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	entries, _, _ := entryRepo.ListBySubscription(ctx, subID, 10, 0)
	if len(entries) == 0 {
		t.Fatalf("no entries found")
	}
	entryID := entries[0].ID

	// Set override to notice
	noticeKind := domain.EntryKindNotice
	_, err := svc.OverrideEntryKind(ctx, entryID, &noticeKind, "mark notice", "admin")
	if err != nil {
		t.Fatalf("OverrideEntryKind failed: %v", err)
	}

	// Now clear override
	cleared, err := svc.OverrideEntryKind(ctx, entryID, nil, "", "admin")
	if err != nil {
		t.Fatalf("clear OverrideEntryKind failed: %v", err)
	}
	if cleared.UserKindOverride != nil {
		t.Fatalf("expected cleared override to be nil, got %v", cleared.UserKindOverride)
	}

	// Reconcile again with new digest
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(feedContent),
		ContentDigest: "digest-clear-2",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	// The cleared override must NOT resurrect!
	latestEntries, err := entryRepo.ListLatestBySubscription(ctx, subID)
	if err != nil || len(latestEntries) == 0 {
		t.Fatalf("failed to query latest entries: %v", err)
	}
	if latestEntries[0].UserKindOverride != nil {
		t.Fatalf("cleared override resurrected! got %v", latestEntries[0].UserKindOverride)
	}
	if latestEntries[0].EffectiveKind() != domain.EntryKindProxy {
		t.Fatalf("expected effective kind proxy, got %v", latestEntries[0].EffectiveKind())
	}
}

// 5. Notice Predicates Use Current Useful Snapshot and Overrides, Not Stale Historical Entries
// Also verifies explicit proxy override suppresses notice exclusion, whereas plain parsed proxy does not.
func TestReconcile_NoticePredicates_UseCurrentUsefulSnapshot(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := "sub-notice-useful"
	url := "https://provider.example.com/dogegg-notice-useful"
	createTestSub(t, subRepo, subID, "Dogegg Notice Useful Sub", url, true)

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Feed 1: Node had loopback notice combo in historical past
	// 127.0.0.1:1, pass 00000000-0000-4000-8000-000000000000, name '剩余流量'
	const historicalNoticeFeed = `
proxies:
  - name: "剩余流量：100GB"
    type: ss
    server: 127.0.0.1
    port: 1
    cipher: aes-128-gcm
    password: 00000000-0000-4000-8000-000000000000
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(historicalNoticeFeed),
		ContentDigest: "digest-hist-notice",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("historical notice reconcile failed: %v", err)
	}

	// Feed 2: Current snapshot changes Node to a regular proxy!
	const currentProxyFeed = `
proxies:
  - name: "Regular Proxy"
    type: ss
    server: 198.51.100.99
    port: 8388
    cipher: aes-128-gcm
    password: pass-real-proxy
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(currentProxyFeed),
		ContentDigest: "digest-curr-proxy",
	})
	if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("current proxy reconcile failed: %v", err)
	}

	// Query with ExcludeNotices: true
	// The current proxy node MUST be included, not excluded by stale historical entries!
	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly:     true,
		ExcludeNotices: true,
	})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if total != 1 || len(nodes) != 1 {
		t.Fatalf("expected 1 active proxy node, got %d (nodes: %v)", total, nodes)
	}
	if nodes[0].DisplayName != "Regular Proxy" {
		t.Fatalf("expected Regular Proxy, got %s", nodes[0].DisplayName)
	}

	// Subtest: Explicit proxy override suppresses notice exclusion across current sources
	// Mark the regular proxy entry as notice via override
	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	currEntries, err := entryRepo.ListLatestBySubscription(ctx, subID)
	if err != nil || len(currEntries) == 0 {
		t.Fatalf("failed to list latest entries: %v", err)
	}
	noticeKind := domain.EntryKindNotice
	_, err = svc.OverrideEntryKind(ctx, currEntries[0].ID, &noticeKind, "user mark notice", "admin")
	if err != nil {
		t.Fatalf("override to notice failed: %v", err)
	}

	// Now it should be excluded as a confirmed notice
	nodesExcluded, totalExcluded, err := svc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly:     true,
		ExcludeNotices: true,
	})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if totalExcluded != 0 || len(nodesExcluded) != 0 {
		t.Fatalf("expected node to be excluded after notice override, got %d", totalExcluded)
	}

	// Now explicitly override back to proxy
	proxyKind := domain.EntryKindProxy
	_, err = svc.OverrideEntryKind(ctx, currEntries[0].ID, &proxyKind, "user explicitly override proxy", "admin")
	if err != nil {
		t.Fatalf("override to proxy failed: %v", err)
	}

	// The explicit proxy override in durable ledger MUST un-exclude the node
	nodesRecovered, totalRecovered, err := svc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly:     true,
		ExcludeNotices: true,
	})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if totalRecovered != 1 || len(nodesRecovered) != 1 {
		t.Fatalf("expected node to be un-excluded by explicit proxy override, got %d", totalRecovered)
	}
}
