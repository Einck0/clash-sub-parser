package policy_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestPolicyValidation_250Nodes_FullInventoryResolutionWithoutTruncation(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	nodeRepo := sqlite.NewNodeRepository(db)
	ctx := context.Background()

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
		logicalID := fmt.Sprintf("node-inv-%03d", i)
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
			LogicalID:          logicalID,
			Protocol:           domain.ProtocolSS,
			DisplayName:        name,
			Server:             fmt.Sprintf("198.51.100.%d", (i%200)+1),
			Port:               8388,
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

	// 3. Insert inactive nodes, notice nodes, disabled sub nodes (must not be resolved)
	inactiveNode := domain.Node{
		LogicalID:          "node-inactive-tw",
		Protocol:           domain.ProtocolSS,
		DisplayName:        "TW-Taiwan-Inactive",
		Server:             "198.51.100.250",
		Port:               8388,
		Active:             false,
		ConnectionRevision: 1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_ = nodeRepo.UpsertBatch(ctx, []domain.Node{inactiveNode})
	_, _ = db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('node-inactive-tw', ?, ?);`, subID, fetchID)

	disabledSubNode := domain.Node{
		LogicalID:          "node-disabled-ca",
		Protocol:           domain.ProtocolSS,
		DisplayName:        "CA-Canada-DisabledSub",
		Server:             "198.51.100.251",
		Port:               8388,
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_ = nodeRepo.UpsertBatch(ctx, []domain.Node{disabledSubNode})
	_, _ = db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('node-disabled-ca', ?, ?);`, disabledSubID, fetchID)

	// 4. Create Policy Groups:
	// - Group Taiwan: dynamic pool filter matching "Taiwan"
	// - Group Canada: dynamic pool filter matching "Canada"
	// - Group Proxy: Select group pointing to Taiwan and Canada child groups
	taiwanFilter := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: "display_name", Op: "contains", Value: "Taiwan"},
		},
	}
	grpTaiwan, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
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
	grpCanada, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
		Name:       "Canada-Auto",
		GroupType:  domain.GroupTypeURLTest,
		NodeFilter: &canadaFilter,
		RequestID:  "req-grp-ca",
		ActorKind:  domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create Canada group: %v", err)
	}

	grpProxy, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
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
	rev, err := svc.CreateRevision(ctx, policy.CreateRevisionCommand{
		State:     domain.RevisionStateActive,
		RequestID: "req-rev-1",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("failed to create active revision: %v", err)
	}

	_, err = svc.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
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

	// 6. Execute ValidateGraph:
	// Because Taiwan and Canada nodes are situated beyond index 100, if repository had
	// a 50 or 100 limit, both Taiwan and Canada groups would be completely empty, causing validation errors.
	valRes, err := svc.ValidateGraph(ctx)
	if err != nil {
		t.Fatalf("ValidateGraph failed: %v", err)
	}
	if !valRes.Valid {
		t.Fatalf("expected graph to be valid, but got invalid with errors: %+v, issues: %+v", valRes.Errors, valRes.Issues)
	}
	if len(valRes.Errors) > 0 {
		t.Fatalf("unexpected error diagnostics: %+v", valRes.Errors)
	}

	// 7. Global Node Filter Verification:
	// If a global node filter excludes all Canada nodes, Canada group should become empty and validation should catch it!
	_, err = svc.SetGlobalNodeFilter(ctx, policy.SetGlobalNodeFilterCommand{
		Spec: domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: "display_name", Op: "not_contains", Value: "Canada"},
			},
		},
		RequestID: "req-set-gf",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("SetGlobalNodeFilter failed: %v", err)
	}

	valResAfterGF, err := svc.ValidateGraph(ctx)
	if err != nil {
		t.Fatalf("ValidateGraph after global filter failed: %v", err)
	}
	// With Canada nodes globally excluded, Canada-Auto has 0 matching candidates and should produce an empty group issue!
	hasCanadaEmptyDiag := false
	for _, iss := range valResAfterGF.Issues {
		if iss.TargetGroupID == grpCanada.ID {
			hasCanadaEmptyDiag = true
			break
		}
	}
	if !hasCanadaEmptyDiag {
		t.Fatalf("expected empty group diagnostic for Canada group after global filter excluded Canada nodes, got issues: %+v", valResAfterGF.Issues)
	}
}
