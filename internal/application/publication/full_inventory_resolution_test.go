package publication_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
)

func TestPublication_250Nodes_FullInventoryResolutionAndExportRegression(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)

	// Set up repos
	subRepo := sqlite.NewSubscriptionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	filterRepo := sqlite.NewNodeFilterRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	auditRepo := &dummyAuditRepo{}

	// Set up services
	policySvc := policy.NewService(policyRepo, revRepo, nodeRepo, auditRepo, filterRepo)
	pubSvc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(filterRepo),
		publication.WithNodeSourceRepository(sourceRepo),
	)

	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	nowStr := now.Format(time.RFC3339)

	// 1. Setup enabled subscription & fetch
	subID := "sub-main-001"
	disabledSubID := "sub-disabled-002"
	fetchID := "fetch-main-001"

	_, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'Main Subscription', 'secret-main', 1, 'rev-1', ?, ?),
		       (?, 'Disabled Subscription', 'secret-disabled', 0, 'rev-1', ?, ?);
	`, subID, nowStr, nowStr, disabledSubID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert subscriptions: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, finished_at, outcome)
		VALUES (?, ?, ?, ?, 'success');
	`, fetchID, subID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert subscription fetch: %v", err)
	}

	// 2. Insert 250 active nodes
	// 000..099: 100 HK nodes
	// 100..104: 5 Taiwan nodes (indices 100-104 > 100)
	// 105..109: 5 Canada nodes (indices 105-109 > 100)
	// 110..249: 140 US nodes
	const totalActive = 250
	activeNodes := make([]domain.Node, 0, totalActive)
	for i := 0; i < totalActive; i++ {
		logicalID := fmt.Sprintf("node-pub-inv-%03d", i)
		name := fmt.Sprintf("HK-Node-%03d", i)
		if i >= 100 && i <= 104 {
			name = fmt.Sprintf("TW-Taiwan-Node-%03d", i)
		} else if i >= 105 && i <= 109 {
			name = fmt.Sprintf("CA-Canada-Node-%03d", i)
		} else if i >= 110 {
			name = fmt.Sprintf("US-America-Node-%03d", i)
		}

		createdAt := now.Add(-time.Duration(500-i) * time.Minute)
		node := domain.Node{
			LogicalID:   logicalID,
			Protocol:    domain.ProtocolSS,
			DisplayName: name,
			Server:      fmt.Sprintf("198.51.100.%d", (i%200)+1),
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{
				Password: "secret-cipher-password",
				Method:   "aes-256-gcm",
			},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          createdAt,
			UpdatedAt:          createdAt,
		}
		activeNodes = append(activeNodes, node)
	}

	if err := nodeRepo.UpsertBatch(ctx, activeNodes); err != nil {
		t.Fatalf("failed to upsert 250 active nodes: %v", err)
	}

	for _, n := range activeNodes {
		_, err := db.ExecContext(ctx, `
			INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
			VALUES (?, ?, ?);
		`, n.LogicalID, subID, fetchID)
		if err != nil {
			t.Fatalf("failed to insert node source for %s: %v", n.LogicalID, err)
		}
	}

	// 3. Insert inactive nodes, disabled sub nodes, notice nodes (must not be resolved)
	inactiveNode := domain.Node{
		LogicalID:   "node-inactive-tw",
		Protocol:    domain.ProtocolSS,
		DisplayName: "TW-Taiwan-Inactive",
		Server:      "198.51.100.250",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Password: "secret-cipher-password",
			Method:   "aes-256-gcm",
		},
		Active:             false,
		ConnectionRevision: 1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_ = nodeRepo.UpsertBatch(ctx, []domain.Node{inactiveNode})
	_, _ = db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('node-inactive-tw', ?, ?);`, subID, fetchID)

	disabledSubNode := domain.Node{
		LogicalID:   "node-disabled-ca",
		Protocol:    domain.ProtocolSS,
		DisplayName: "CA-Canada-DisabledSub",
		Server:      "198.51.100.251",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Password: "secret-cipher-password",
			Method:   "aes-256-gcm",
		},
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_ = nodeRepo.UpsertBatch(ctx, []domain.Node{disabledSubNode})
	_, _ = db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('node-disabled-ca', ?, ?);`, disabledSubID, fetchID)

	noticeNode := domain.Node{
		LogicalID:   "node-notice-tw",
		Protocol:    domain.ProtocolSS,
		DisplayName: "TW-Taiwan-Notice-Traffic",
		Server:      "198.51.100.252",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Password: "secret-cipher-password",
			Method:   "aes-256-gcm",
		},
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_ = nodeRepo.UpsertBatch(ctx, []domain.Node{noticeNode})
	_, _ = db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('node-notice-tw', ?, ?);`, subID, fetchID)
	payloadID := "payload-notice-01"
	_, _ = db.ExecContext(ctx, `
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, pinned, created_at)
		VALUES (?, ?, ?, 'digest-notice', X'deadbeef', 1, ?);
	`, payloadID, subID, fetchID, nowStr)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO subscription_entries (id, payload_id, subscription_id, ordinal, source_key, raw_name, protocol, server, port, entry_kind, classification_reason, node_logical_id, created_at)
		VALUES ('entry-notice-01', ?, ?, 0, 'k-notice', 'TW-Taiwan-Notice-Traffic', 'ss', '198.51.100.252', 8388, 'notice', 'Notice pattern match', 'node-notice-tw', ?);
	`, payloadID, subID, nowStr)

	_ = subRepo

	// 4. Create Policy Groups:
	// - Group Taiwan: dynamic pool filter matching "Taiwan"
	// - Group Canada: dynamic pool filter matching "Canada"
	// - Group Proxy: Select group pointing to Taiwan and Canada child groups
	taiwanFilter := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: "display_name", Op: "contains", Value: "Taiwan"},
		},
	}
	grpTaiwan, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:       "Taiwan-Auto",
		GroupType:  domain.GroupTypeURLTest,
		NodeFilter: &taiwanFilter,
		RequestID:  "req-grp-tw",
		ActorKind:  domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create Taiwan group: %v", err)
	}

	canadaFilter := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: "display_name", Op: "contains", Value: "Canada"},
		},
	}
	grpCanada, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:       "Canada-Auto",
		GroupType:  domain.GroupTypeURLTest,
		NodeFilter: &canadaFilter,
		RequestID:  "req-grp-ca",
		ActorKind:  domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create Canada group: %v", err)
	}

	grpProxy, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:      "Proxy",
		GroupType: domain.GroupTypeSelect,
		Edges: []policy.EdgeInput{
			{ChildGroupID: &grpTaiwan.ID, Position: 0},
			{ChildGroupID: &grpCanada.ID, Position: 1},
		},
		RequestID: "req-grp-proxy",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create Proxy group: %v", err)
	}

	// 5. Create Active Configuration Revision with MATCH rule
	rev, err := policySvc.CreateRevision(ctx, policy.CreateRevisionCommand{
		State:     domain.RevisionStateActive,
		RequestID: "req-rev-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create active revision: %v", err)
	}

	_, err = policySvc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		RevisionID:    rev.ID,
		TargetGroupID: grpProxy.ID,
		Expression:    "MATCH",
		Position:      0,
		RequestID:     "req-rule-match",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create MATCH policy rule: %v", err)
	}

	// 6. Test ResolvePolicySnapshot:
	// Verify that publication service's resolveSnapshot calls nodeRepo.ListAll,
	// returning all 250 active nodes and populating Taiwan and Canada groups (which are at >100).
	snapshot, err := pubSvc.ResolvePolicySnapshot(ctx, rev.ID)
	if err != nil {
		t.Fatalf("ResolvePolicySnapshot failed: %v", err)
	}

	if len(snapshot.Nodes) != totalActive {
		t.Fatalf("expected exactly %d nodes in snapshot, got %d", totalActive, len(snapshot.Nodes))
	}

	// Verify group membership resolution:
	// Find Taiwan and Canada groups in snapshot
	var resolvedTaiwan, resolvedCanada, resolvedProxy *resolver.ResolvedGroup
	for i := range snapshot.Groups {
		g := &snapshot.Groups[i]
		if g.ID == grpTaiwan.ID {
			resolvedTaiwan = g
		} else if g.ID == grpCanada.ID {
			resolvedCanada = g
		} else if g.ID == grpProxy.ID {
			resolvedProxy = g
		}
	}

	if resolvedTaiwan == nil {
		t.Fatal("expected Taiwan group in resolved snapshot")
	}
	if len(resolvedTaiwan.NodeLogicalIDs) != 5 {
		t.Fatalf("expected Taiwan group to have exactly 5 nodes, got %d (nodes: %v)", len(resolvedTaiwan.NodeLogicalIDs), resolvedTaiwan.NodeLogicalIDs)
	}

	if resolvedCanada == nil {
		t.Fatal("expected Canada group in resolved snapshot")
	}
	if len(resolvedCanada.NodeLogicalIDs) != 5 {
		t.Fatalf("expected Canada group to have exactly 5 nodes, got %d (nodes: %v)", len(resolvedCanada.NodeLogicalIDs), resolvedCanada.NodeLogicalIDs)
	}

	if resolvedProxy == nil {
		t.Fatal("expected Proxy group in resolved snapshot")
	}
	if len(resolvedProxy.ChildGroupIDs) != 2 {
		t.Fatalf("expected Proxy group to have 2 child groups, got %d", len(resolvedProxy.ChildGroupIDs))
	}

	// Verify that inactive, disabled-sub, and notice nodes were excluded
	for _, n := range snapshot.Nodes {
		if n.LogicalID == "node-inactive-tw" {
			t.Fatalf("inactive node %s should have been excluded from snapshot", n.LogicalID)
		}
		if n.LogicalID == "node-disabled-ca" {
			t.Fatalf("disabled sub node %s should have been excluded from snapshot", n.LogicalID)
		}
		if n.LogicalID == "node-notice-tw" {
			t.Fatalf("notice node %s should have been excluded from snapshot", n.LogicalID)
		}
	}

	// 7. Test Preflight for SingBox & Mihomo:
	// Preflight must pass without empty-group errors.
	preflightSingBox, err := pubSvc.Preflight(ctx, publication.PreflightCommand{
		Target:     domain.TargetSingBox,
		RevisionID: rev.ID,
		ActorKind:  domain.ActorKindAdmin,
		RequestID:  "req-preflight-sb",
	})
	if err != nil {
		t.Fatalf("Preflight SingBox error: %v", err)
	}
	if !preflightSingBox.Allowed {
		t.Fatalf("expected Preflight SingBox to be allowed, got diags: %+v", preflightSingBox.Diagnostics)
	}

	preflightMihomo, err := pubSvc.Preflight(ctx, publication.PreflightCommand{
		Target:     domain.TargetMihomo,
		RevisionID: rev.ID,
		ActorKind:  domain.ActorKindAdmin,
		RequestID:  "req-preflight-mi",
	})
	if err != nil {
		t.Fatalf("Preflight Mihomo error: %v", err)
	}
	if !preflightMihomo.Allowed {
		t.Fatalf("expected Preflight Mihomo to be allowed, got diags: %+v", preflightMihomo.Diagnostics)
	}

	// 8. Test Publish & Export (Compile Artifact) for SingBox
	pubResultSB, err := pubSvc.Publish(ctx, publication.PublishCommand{
		Target:     domain.TargetSingBox,
		RevisionID: rev.ID,
		ActorKind:  domain.ActorKindAdmin,
		RequestID:  "req-pub-sb-250",
	})
	if err != nil {
		t.Fatalf("Publish SingBox error: %v", err)
	}
	if pubResultSB.Publication.State != domain.PublicationStateActive {
		t.Fatalf("expected publication state active, got %s", pubResultSB.Publication.State)
	}
	if !strings.HasPrefix(pubResultSB.RawToken, "pub_") {
		t.Fatalf("expected raw token to start with pub_, got %s", pubResultSB.RawToken)
	}

	// Retrieve exported artifact via ResolveAndServeAuthorized
	artifactSB, err := pubSvc.ResolveAndServeAuthorized(ctx, pubResultSB.Publication.ID, func(h string) bool { return true })
	if err != nil {
		t.Fatalf("ResolveAndServeAuthorized SingBox error: %v", err)
	}
	if len(artifactSB.Content) == 0 {
		t.Fatal("expected non-empty artifact content for SingBox")
	}
	sbContent := string(artifactSB.Content)
	if !strings.Contains(sbContent, "TW-Taiwan-Node-100") {
		t.Fatalf("expected SingBox artifact to contain Taiwan node 100 (> 100), but missing")
	}
	if !strings.Contains(sbContent, "CA-Canada-Node-105") {
		t.Fatalf("expected SingBox artifact to contain Canada node 105 (> 100), but missing")
	}

	// 9. Test Publish & Export for Mihomo (Clash Meta)
	pubResultMihomo, err := pubSvc.Publish(ctx, publication.PublishCommand{
		Target:     domain.TargetMihomo,
		RevisionID: rev.ID,
		ActorKind:  domain.ActorKindAdmin,
		RequestID:  "req-pub-mihomo-250",
	})
	if err != nil {
		t.Fatalf("Publish Mihomo error: %v", err)
	}
	artifactMihomo, err := pubSvc.ResolveAndServeAuthorized(ctx, pubResultMihomo.Publication.ID, func(h string) bool { return true })
	if err != nil {
		t.Fatalf("ResolveAndServeAuthorized Mihomo error: %v", err)
	}
	if len(artifactMihomo.Content) == 0 {
		t.Fatal("expected non-empty artifact content for Mihomo")
	}
	mihomoContent := string(artifactMihomo.Content)
	if !strings.Contains(mihomoContent, "TW-Taiwan-Node-100") {
		t.Fatalf("expected Mihomo artifact to contain Taiwan node 100 (> 100), but missing")
	}
	if !strings.Contains(mihomoContent, "CA-Canada-Node-105") {
		t.Fatalf("expected Mihomo artifact to contain Canada node 105 (> 100), but missing")
	}
	if !strings.Contains(mihomoContent, "Taiwan-Auto") {
		t.Fatalf("expected Mihomo artifact to contain Taiwan-Auto proxy group")
	}
	if !strings.Contains(mihomoContent, "Canada-Auto") {
		t.Fatalf("expected Mihomo artifact to contain Canada-Auto proxy group")
	}
}
