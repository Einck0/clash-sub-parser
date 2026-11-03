package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/proxychain"
	"clash-sub-parser/internal/domain"
	_ "modernc.org/sqlite"
)

func TestApplicationServices_ProxyChainWiring(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}

	if appServices.chainRepo == nil {
		t.Fatal("expected non-nil chainRepo in applicationServices")
	}
	if appServices.chainService == nil {
		t.Fatal("expected non-nil chainService in applicationServices")
	}

	cfg, err := appServices.chainService.Get(ctx)
	if err != nil {
		t.Fatalf("chainService.Get failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil ProxyChainConfig")
	}
	if len(cfg.Bindings) != 0 {
		t.Fatalf("expected 0 initial bindings, got %d", len(cfg.Bindings))
	}
	if cfg.Version == "" {
		t.Fatal("expected non-empty deterministic initial version")
	}

	opts, err := appServices.chainService.Options(ctx)
	if err != nil {
		t.Fatalf("chainService.Options failed: %v", err)
	}
	if opts == nil || opts.Nodes == nil || opts.Groups == nil || opts.Subscriptions == nil {
		t.Fatal("expected non-nil OptionsResult and slices")
	}
}

func TestApplicationServices_ProxyChain_RepairInvalidPersistedSubscription(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)

	// 1. Setup active revision
	revID := domain.MustNewUUIDv7()
	_, err := db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-1', 'active', ?);
	`, revID, nowStr)
	if err != nil {
		t.Fatalf("insert revision failed: %v", err)
	}

	// 2. Setup admission rule
	arID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO admission_rules (id, revision_id, name, expression, action, position)
		VALUES (?, ?, 'Allow All', 'true', 'allow', 0);
	`, arID, revID)
	if err != nil {
		t.Fatalf("insert admission rule failed: %v", err)
	}

	// 3. Setup policy group and rule for Mihomo target
	groupID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'Proxy Group', 'select', ?, ?, 0, 1);
	`, groupID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert group failed: %v", err)
	}

	// 4. Construct valid SS nodes using domain constructors
	node1ID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	node2ID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)

	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'Node A', '1.1.1.1', 8388, '{"method":"aes-128-gcm","password":"pwd1"}', 1, ?, ?, 1),
		       (? , 'ss', 'Node B', '1.1.1.2', 8388, '{"method":"aes-128-gcm","password":"pwd2"}', 1, ?, ?, 1);
	`, node1ID, nowStr, nowStr, node2ID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert nodes failed: %v", err)
	}

	pgs1ID := domain.MustNewUUIDv7()
	pgs2ID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, created_at, updated_at)
		VALUES (?, ?, 'add', 'manual_node', 0, ?, ?, ?),
		       (?, ?, 'add', 'manual_node', 1, ?, ?, ?);
	`, pgs1ID, groupID, node1ID, nowStr, nowStr, pgs2ID, groupID, node2ID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert group sources failed: %v", err)
	}

	prID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES (?, ?, ?, 'MATCH', 0);
	`, prID, revID, groupID)
	if err != nil {
		t.Fatalf("insert policy rule failed: %v", err)
	}

	// 5. Construct subscriptions using UUIDv7.
	// subAnchorID models shared stable assets with a successful fetch linking both node1 and node2.
	// This ensures both nodes remain exportable and inventory stays non-empty across the entire
	// lifecycle, isolating invalid binding repair from unrelated empty inventory errors.
	sub1ID := domain.MustNewUUIDv7()
	sub2ID := domain.MustNewUUIDv7()
	subAnchorID := domain.MustNewUUIDv7()

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'Sub 1', 'http://example.com/sub1', 1, 'rev1', ?, ?),
		       (?, 'Sub 2', 'http://example.com/sub2', 1, 'rev1', ?, ?),
		       (?, 'Sub Anchor', 'http://example.com/anchor', 1, 'rev1', ?, ?);
	`, sub1ID, nowStr, nowStr, sub2ID, nowStr, nowStr, subAnchorID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert subscriptions failed: %v", err)
	}

	fetch1ID := domain.MustNewUUIDv7()
	fetchAnchorID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'hash-1', 1, 1),
		       (?, ?, ?, 'success', 'hash-anchor', 2, 2);
	`, fetch1ID, sub1ID, nowStr, fetchAnchorID, subAnchorID, nowStr)
	if err != nil {
		t.Fatalf("insert subscription_fetches failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?),
		       (?, ?, ?),
		       (?, ?, ?);
	`, node1ID, sub1ID, fetch1ID, node1ID, subAnchorID, fetchAnchorID, node2ID, subAnchorID, fetchAnchorID)
	if err != nil {
		t.Fatalf("insert node_sources failed: %v", err)
	}

	// 6. Insert initial proxy chain binding targeting sub1ID with dialer node2ID
	bindingID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO proxy_chain_bindings (id, target_type, target_id, dialer_type, dialer_id, enabled, sort_order, note, revision, updated_at)
		VALUES (?, 'subscription', ?, 'node', ?, 1, 0, 'initial chain', 1, ?);
	`, bindingID, sub1ID, node2ID, nowStr)
	if err != nil {
		t.Fatalf("insert proxy_chain_binding failed: %v", err)
	}

	// 7. Initialize applicationServices with real wiring
	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}

	// Initial export MUST succeed and emit dialer-proxy: Node B for Node A
	artInitial, err := appServices.pubService.GenerateTargetArtifact(ctx, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("initial GenerateTargetArtifact failed: %v", err)
	}
	if !strings.Contains(string(artInitial.Content), "dialer-proxy") {
		t.Fatalf("expected dialer-proxy in initial artifact:\n%s", string(artInitial.Content))
	}

	// 8. Invalidate sub1ID (disable subscription in database)
	_, err = db.ExecContext(ctx, `UPDATE subscriptions SET enabled = 0 WHERE id = ?;`, sub1ID)
	if err != nil {
		t.Fatalf("disable subscription failed: %v", err)
	}

	// Actual export MUST now fail with chain_reference_missing
	_, err = appServices.pubService.GenerateTargetArtifact(ctx, domain.TargetMihomo)
	if err == nil {
		t.Fatal("expected GenerateTargetArtifact to fail closed when persisted subscription is disabled")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing, got: %v", err)
	}

	// 9. REPAIR STEP A: Retarget binding to enabled sub2ID via chainService.Update
	// Insert subscription_fetch for sub2 so that its node qualifies as active inventory
	fetch2ID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'hash-2', 1, 1);
	`, fetch2ID, sub2ID, nowStr)
	if err != nil {
		t.Fatalf("insert subscription_fetch for sub2 failed: %v", err)
	}

	// Link node1ID to sub2ID
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, node1ID, sub2ID, fetch2ID)
	if err != nil {
		t.Fatalf("insert node_sources for sub2 failed: %v", err)
	}

	cfg, err := appServices.chainService.Get(ctx)
	if err != nil {
		t.Fatalf("chainService.Get failed: %v", err)
	}

	// Updating the binding to target sub2ID succeeds because AssembleResolveInput does NOT fail on old sub1ID!
	upRes, err := appServices.chainService.Update(ctx, bindingID, proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			ID:         bindingID,
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   sub2ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   node2ID,
			Enabled:    true,
			SortOrder:  0,
			Note:       "retargeted to sub2",
			Revision:   cfg.Bindings[0].Revision,
		},
		ConfigVersion:    cfg.Version,
		ExpectedRevision: cfg.Bindings[0].Revision,
	})
	if err != nil {
		t.Fatalf("chainService.Update (retarget) failed: %v", err)
	}

	// Export now SUCCEEDS again after retarget repair
	artRetarget, err := appServices.pubService.GenerateTargetArtifact(ctx, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("GenerateTargetArtifact after retarget failed: %v", err)
	}
	if !strings.Contains(string(artRetarget.Content), "dialer-proxy") {
		t.Fatalf("expected dialer-proxy in retargeted artifact:\n%s", string(artRetarget.Content))
	}

	// 10. REPAIR STEP B: Invalidate sub2ID and verify disabling binding via chainService.Update repairs export
	_, err = db.ExecContext(ctx, `UPDATE subscriptions SET enabled = 0 WHERE id = ?;`, sub2ID)
	if err != nil {
		t.Fatalf("disable sub2 failed: %v", err)
	}

	// Export fails again
	_, err = appServices.pubService.GenerateTargetArtifact(ctx, domain.TargetMihomo)
	if err == nil {
		t.Fatal("expected export to fail when sub2 is disabled")
	}

	// Disable the binding
	upRes2, err := appServices.chainService.Update(ctx, bindingID, proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			ID:         bindingID,
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   sub2ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   node2ID,
			Enabled:    false, // disabled!
			SortOrder:  0,
			Note:       "disabled binding",
			Revision:   upRes.Binding.Revision,
		},
		ConfigVersion:    upRes.Version,
		ExpectedRevision: upRes.Binding.Revision,
	})
	if err != nil {
		t.Fatalf("chainService.Update (disable) failed: %v", err)
	}

	// Export now SUCCEEDS because disabled binding does not block export
	artDisabled, err := appServices.pubService.GenerateTargetArtifact(ctx, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("GenerateTargetArtifact after disable failed: %v", err)
	}
	if strings.Contains(string(artDisabled.Content), "dialer-proxy") {
		t.Fatalf("expected no dialer-proxy when binding is disabled:\n%s", string(artDisabled.Content))
	}

	// 11. REPAIR STEP C: Delete the binding via chainService.Delete
	_, err = appServices.chainService.Delete(ctx, bindingID, proxychain.DeleteCommand{
		ConfigVersion:    upRes2.Version,
		ExpectedRevision: upRes2.Binding.Revision,
	})
	if err != nil {
		t.Fatalf("chainService.Delete failed: %v", err)
	}

	cfgFinal, err := appServices.chainService.Get(ctx)
	if err != nil {
		t.Fatalf("chainService.Get after delete failed: %v", err)
	}
	if len(cfgFinal.Bindings) != 0 {
		t.Fatalf("expected 0 bindings after delete, got %d", len(cfgFinal.Bindings))
	}

	// Export SUCCEEDS
	_, err = appServices.pubService.GenerateTargetArtifact(ctx, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("GenerateTargetArtifact after delete failed: %v", err)
	}
}
