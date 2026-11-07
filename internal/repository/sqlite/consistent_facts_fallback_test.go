package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestConsistentFactsLoader_PrimarySubscriptionFallbackFields(t *testing.T) {
	db, _ := setupTestDB(t)
	loader := sqlite.NewConsistentFactsLoader(db)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Revision
	revID := "rev-primary-test"
	_, err := db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-rev', 'active', ?);
	`, revID, now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert revision failed: %v", err)
	}

	// 2. Subscriptions: sub-primary (primary, enabled), sub-secondary (non-primary, enabled)
	subPrimaryID := "sub-primary-1"
	expectedUserInfo := "upload=100; download=200; total=1000; expire=1800000000"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, is_primary, userinfo, revision, created_at, updated_at)
		VALUES (?, 'Primary Sub', 'secret://p', 1, 1, ?, 1, ?, ?);
	`, subPrimaryID, expectedUserInfo, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert primary subscription failed: %v", err)
	}

	subSecondaryID := "sub-secondary-2"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, is_primary, userinfo, revision, created_at, updated_at)
		VALUES (?, 'Secondary Sub', 'secret://s', 1, 0, 'other-info', 1, ?, ?);
	`, subSecondaryID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert secondary subscription failed: %v", err)
	}

	// 3. Subscription fetches
	fetchP := "fetch-p1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, outcome, started_at, finished_at)
		VALUES (?, ?, 'success', ?, ?);
	`, fetchP, subPrimaryID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert primary fetch failed: %v", err)
	}

	fetchS := "fetch-s1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, outcome, started_at, finished_at)
		VALUES (?, ?, 'success', ?, ?);
	`, fetchS, subSecondaryID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert secondary fetch failed: %v", err)
	}

	// 4. Nodes:
	// node-p1 (active, primary)
	// node-p2 (active, primary)
	// node-p3-inactive (inactive, primary -> must be excluded)
	// node-p4-notice (active, primary, notice -> must be excluded)
	// node-s1 (active, secondary -> must be in Nodes, but NOT in PrimaryNodeIDs)
	nodeP1 := "node-p1"
	nodeP2 := "node-p2"
	nodeP3Inactive := "node-p3-inactive"
	nodeP4Notice := "node-p4-notice"
	nodeS1 := "node-s1"

	t1 := now.Add(-10 * time.Minute)
	t2 := now.Add(-5 * time.Minute)

	insertNode := func(id, name string, active int, created time.Time) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
			VALUES (?, 'ss', ?, '1.1.1.1', 8388, '{"method":"aes-128-gcm","password":"sec"}', ?, ?, ?, 1);
		`, id, name, active, created.Format(time.RFC3339Nano), created.Format(time.RFC3339Nano))
		if err != nil {
			t.Fatalf("insert node %s failed: %v", id, err)
		}
	}

	insertNode(nodeP1, "Node Primary 1", 1, t2)
	insertNode(nodeP2, "Node Primary 2", 1, t1)
	insertNode(nodeP3Inactive, "Node Primary Inactive", 0, t1)
	insertNode(nodeP4Notice, "Node Primary Notice", 1, t1)
	insertNode(nodeS1, "Node Secondary 1", 1, t2)

	// Node sources
	insertSource := func(nodeID, subID, fetchID string) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
			VALUES (?, ?, ?);
		`, nodeID, subID, fetchID)
		if err != nil {
			t.Fatalf("insert node_sources for %s failed: %v", nodeID, err)
		}
	}
	insertSource(nodeP1, subPrimaryID, fetchP)
	insertSource(nodeP2, subPrimaryID, fetchP)
	insertSource(nodeP3Inactive, subPrimaryID, fetchP)
	insertSource(nodeP4Notice, subPrimaryID, fetchP)
	insertSource(nodeS1, subSecondaryID, fetchS)

	// Notice payload and entry for nodeP4Notice
	payloadP := "payload-p1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, pinned, created_at)
		VALUES (?, ?, ?, 'digest-notice', X'deadbeef', 1, ?);
	`, payloadP, subPrimaryID, fetchP, now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert notice payload failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_entries (id, payload_id, subscription_id, ordinal, source_key, raw_name, protocol, entry_kind, node_logical_id, created_at)
		VALUES ('entry-notice', ?, ?, 1, 'key-notice', 'Notice Node', 'ss', 'notice', ?, ?);
	`, payloadP, subPrimaryID, nodeP4Notice, now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert notice entry failed: %v", err)
	}

	// 5. Load facts without any filters
	facts, err := loader.LoadConsistentFacts(ctx, revID, domain.TargetSingBox, now)
	if err != nil {
		t.Fatalf("LoadConsistentFacts failed: %v", err)
	}

	// Verify PrimarySubscriptionID
	if facts.PrimarySubscriptionID != subPrimaryID {
		t.Errorf("expected PrimarySubscriptionID %q, got %q", subPrimaryID, facts.PrimarySubscriptionID)
	}

	// Verify PrimaryUserInfo
	if facts.PrimaryUserInfo != expectedUserInfo {
		t.Errorf("expected PrimaryUserInfo %q, got %q", expectedUserInfo, facts.PrimaryUserInfo)
	}

	// Verify PrimaryNodeIDs contains only nodeP1 and nodeP2, in facts.Nodes order (created_at DESC)
	if len(facts.PrimaryNodeIDs) != 2 {
		t.Fatalf("expected 2 PrimaryNodeIDs, got %d: %+v", len(facts.PrimaryNodeIDs), facts.PrimaryNodeIDs)
	}
	if facts.PrimaryNodeIDs[0] != nodeP1 || facts.PrimaryNodeIDs[1] != nodeP2 {
		t.Errorf("expected PrimaryNodeIDs [%s, %s], got %+v", nodeP1, nodeP2, facts.PrimaryNodeIDs)
	}

	// Verify facts.Nodes has nodeP1, nodeS1, nodeP2 (all 3 active eligible nodes across all subscriptions)
	if len(facts.Nodes) != 3 {
		t.Errorf("expected 3 total facts.Nodes, got %d", len(facts.Nodes))
	}

	// 6. Test Disabled Primary Subscription
	_, err = db.ExecContext(ctx, `UPDATE subscriptions SET enabled = 0 WHERE id = ?;`, subPrimaryID)
	if err != nil {
		t.Fatalf("disable primary subscription failed: %v", err)
	}

	factsDisabled, err := loader.LoadConsistentFacts(ctx, revID, domain.TargetSingBox, now)
	if err != nil {
		t.Fatalf("LoadConsistentFacts with disabled primary failed: %v", err)
	}
	if factsDisabled.PrimarySubscriptionID != "" {
		t.Errorf("expected empty PrimarySubscriptionID when disabled, got %q", factsDisabled.PrimarySubscriptionID)
	}
	if len(factsDisabled.PrimaryNodeIDs) != 0 {
		t.Errorf("expected 0 PrimaryNodeIDs when primary disabled, got %d", len(factsDisabled.PrimaryNodeIDs))
	}
}

func TestConsistentFactsLoader_NoPrimarySubscription(t *testing.T) {
	db, _ := setupTestDB(t)
	loader := sqlite.NewConsistentFactsLoader(db)
	ctx := context.Background()
	now := time.Now().UTC()

	revID := "rev-no-primary"
	_, err := db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-rev', 'active', ?);
	`, revID, now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert revision failed: %v", err)
	}

	subID := "sub-only-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, is_primary, userinfo, revision, created_at, updated_at)
		VALUES (?, 'Non-Primary Sub', 'secret://np', 1, 0, 'info', 1, ?, ?);
	`, subID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert subscription failed: %v", err)
	}

	facts, err := loader.LoadConsistentFacts(ctx, revID, domain.TargetSingBox, now)
	if err != nil {
		t.Fatalf("LoadConsistentFacts failed: %v", err)
	}
	if facts.PrimarySubscriptionID != "" {
		t.Errorf("expected empty PrimarySubscriptionID when is_primary=0, got %q", facts.PrimarySubscriptionID)
	}
	if len(facts.PrimaryNodeIDs) != 0 {
		t.Errorf("expected empty PrimaryNodeIDs, got %d", len(facts.PrimaryNodeIDs))
	}
}
