package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// TestSubscriptionRepository_NodeCount_NoticeExclusionRegressions verifies that
// subscription node counts respect canonical notice exclusions, durable overrides,
// and enabled/disabled count semantics.
func TestSubscriptionRepository_NodeCount_NoticeExclusionRegressions(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	ctx := context.Background()

	subID := "sub-count-test-1"
	sub := &domain.Subscription{
		ID:                 subID,
		Name:               "Count Test Sub",
		SourceURLSecretRef: "https://example.com/count",
		Enabled:            true,
		Revision:           "rev-count-1",
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds: 3600,
		},
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	fetchID := "fetch-count-good-1"
	payloadID := "payload-count-good-1"
	nowStr := time.Now().UTC().Format(time.RFC3339)

	// Insert successful fetch
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'dig-count', 4, 4);
	`, fetchID, subID, nowStr)
	if err != nil {
		t.Fatalf("insert fetch failed: %v", err)
	}

	// Insert payload
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, http_status, headers_json, pinned, created_at)
		VALUES (?, ?, ?, 'dig-count', X'', 200, '{}', 0, ?);
	`, payloadID, subID, fetchID, nowStr)
	if err != nil {
		t.Fatalf("insert payload failed: %v", err)
	}

	// Insert nodes and node_sources
	nodes := []struct {
		logicalID   string
		displayName string
		entryKind   string
		override    string // "" or "proxy" or "notice"
	}{
		// 1. Regular proxy node -> included
		{logicalID: "node-normal-proxy", displayName: "Normal Proxy", entryKind: "proxy", override: ""},
		// 2. Regular notice node -> excluded
		{logicalID: "node-normal-notice", displayName: "Notice Node", entryKind: "notice", override: ""},
		// 3. Entry is notice, but explicit override ledger overrides to 'proxy' -> included!
		{logicalID: "node-notice-overridden-to-proxy", displayName: "Notice Overridden", entryKind: "notice", override: "proxy"},
		// 4. Entry is proxy, but explicit override ledger overrides to 'notice' -> excluded!
		{logicalID: "node-proxy-overridden-to-notice", displayName: "Proxy Overridden", entryKind: "proxy", override: "notice"},
	}

	for ordinal, n := range nodes {
		// Insert node
		_, err = db.ExecContext(ctx, `
			INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, connection_revision, created_at, updated_at)
			VALUES (?, 'ss', ?, '1.2.3.4', 8388, '{}', 1, 1, ?, ?);
		`, n.logicalID, n.displayName, nowStr, nowStr)
		if err != nil {
			t.Fatalf("insert node %s failed: %v", n.logicalID, err)
		}

		// Insert node_source linked to latest good fetch
		_, err = db.ExecContext(ctx, `
			INSERT INTO node_sources (subscription_id, node_logical_id, last_seen_fetch_id)
			VALUES (?, ?, ?);
		`, subID, n.logicalID, fetchID)
		if err != nil {
			t.Fatalf("insert node_source for %s failed: %v", n.logicalID, err)
		}

		// Insert subscription_entry
		entryID := "entry-" + n.logicalID
		overrideAnchor := "anchor-" + n.logicalID
		_, err = db.ExecContext(ctx, `
			INSERT INTO subscription_entries (
				id, subscription_id, payload_id, ordinal, raw_name, entry_kind, protocol,
				node_logical_id, source_key, override_anchor, created_at
			) VALUES (?, ?, ?, ?, ?, ?, 'ss', ?, ?, ?, ?);
		`, entryID, subID, payloadID, ordinal+1, n.displayName, n.entryKind, n.logicalID, "key-"+n.logicalID, overrideAnchor, nowStr)
		if err != nil {
			t.Fatalf("insert entry for %s failed: %v", n.logicalID, err)
		}

		// If explicit override is specified, insert into subscription_entry_overrides ledger
		if n.override != "" {
			_, err = db.ExecContext(ctx, `
				INSERT INTO subscription_entry_overrides (
					subscription_id, override_anchor, source_key, raw_name, user_kind_override, override_reason, override_at
				) VALUES (?, ?, ?, ?, ?, 'test override', ?);
			`, subID, overrideAnchor, "key-"+n.logicalID, n.displayName, n.override, nowStr)
			if err != nil {
				t.Fatalf("insert override for %s failed: %v", n.logicalID, err)
			}
		}
	}

	// Verify counts when subscription is Enabled:
	// Expected included nodes:
	// - node-normal-proxy (proxy, no override) -> YES
	// - node-normal-notice (notice, no override) -> NO
	// - node-notice-overridden-to-proxy (notice, override=proxy) -> YES
	// - node-proxy-overridden-to-notice (proxy, override=notice) -> NO
	// Total expected: 2
	gotSub, err := subRepo.GetByID(ctx, subID)
	if err != nil {
		t.Fatalf("subRepo.GetByID failed: %v", err)
	}
	if gotSub.SourceNodeCount != 2 {
		t.Errorf("expected SourceNodeCount 2, got %d", gotSub.SourceNodeCount)
	}
	if gotSub.NodeCount != 2 {
		t.Errorf("expected NodeCount 2 when enabled, got %d", gotSub.NodeCount)
	}

	// Verify counts when subscription is Disabled:
	// SourceNodeCount remains 2, but NodeCount becomes 0
	gotSub.Enabled = false
	if err := subRepo.Update(ctx, gotSub); err != nil {
		t.Fatalf("subRepo.Update disabled failed: %v", err)
	}

	gotDisabledSub, err := subRepo.GetByID(ctx, subID)
	if err != nil {
		t.Fatalf("subRepo.GetByID after disable failed: %v", err)
	}
	if gotDisabledSub.SourceNodeCount != 2 {
		t.Errorf("expected SourceNodeCount 2 when disabled, got %d", gotDisabledSub.SourceNodeCount)
	}
	if gotDisabledSub.NodeCount != 0 {
		t.Errorf("expected NodeCount 0 when disabled, got %d", gotDisabledSub.NodeCount)
	}
}

// TestSubscriptionRepository_NodeCount_StaleNoticeCurrentProxy verifies that
// when an older payload had a notice entry, but the current useful payload has a proxy entry,
// the node is correctly counted as a proxy.
func TestSubscriptionRepository_NodeCount_StaleNoticeCurrentProxy(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	ctx := context.Background()

	subID := "sub-stale-notice-test"
	sub := &domain.Subscription{
		ID:                 subID,
		Name:               "Stale Notice Sub",
		SourceURLSecretRef: "https://example.com/stale",
		Enabled:            true,
		Revision:           "rev-stale-1",
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds: 3600,
		},
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	oldTime := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	curTime := time.Now().UTC().Format(time.RFC3339)

	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}

	// Fetch 1: Old fetch where node was marked 'notice'
	fetch1 := "fetch-old-1"
	payload1 := "payload-old-1"
	mustExec(`
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'dig-1', 1, 1);
	`, fetch1, subID, oldTime)
	mustExec(`
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, http_status, headers_json, pinned, created_at)
		VALUES (?, ?, ?, 'dig-1', X'', 200, '{}', 0, ?);
	`, payload1, subID, fetch1, oldTime)

	// Fetch 2: Current latest good fetch where node is marked 'proxy'
	fetch2 := "fetch-current-2"
	payload2 := "payload-current-2"
	mustExec(`
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'dig-2', 1, 1);
	`, fetch2, subID, curTime)
	mustExec(`
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, http_status, headers_json, pinned, created_at)
		VALUES (?, ?, ?, 'dig-2', X'', 200, '{}', 0, ?);
	`, payload2, subID, fetch2, curTime)

	nodeID := "node-evolving-notice-proxy"
	mustExec(`
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, connection_revision, created_at, updated_at)
		VALUES (?, 'ss', 'Evolving Node', '8.8.8.8', 8388, '{}', 1, 1, ?, ?);
	`, nodeID, curTime, curTime)

	// node_source links to latest fetch2
	mustExec(`
		INSERT INTO node_sources (subscription_id, node_logical_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, subID, nodeID, fetch2)

	// Old entry was notice
	mustExec(`
		INSERT INTO subscription_entries (id, subscription_id, payload_id, ordinal, raw_name, entry_kind, protocol, node_logical_id, source_key, override_anchor, created_at)
		VALUES ('entry-old', ?, ?, 1, 'Evolving Node', 'notice', 'ss', ?, 'key-evolving', 'anchor-ev', ?);
	`, subID, payload1, nodeID, oldTime)

	// Current entry is proxy
	mustExec(`
		INSERT INTO subscription_entries (id, subscription_id, payload_id, ordinal, raw_name, entry_kind, protocol, node_logical_id, source_key, override_anchor, created_at)
		VALUES ('entry-cur', ?, ?, 1, 'Evolving Node', 'proxy', 'ss', ?, 'key-evolving', 'anchor-ev', ?);
	`, subID, payload2, nodeID, curTime)

	// Since current entry is proxy, it must be counted!
	gotSub, err := subRepo.GetByID(ctx, subID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if gotSub.SourceNodeCount != 1 {
		t.Errorf("expected SourceNodeCount 1 for current proxy, got %d", gotSub.SourceNodeCount)
	}
}
