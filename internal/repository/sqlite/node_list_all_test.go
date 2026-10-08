package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestNodeRepository_ListAll_AndHealthPredicatesConservation(t *testing.T) {
	db := setupTestDB(t)
	nodeRepo := NewNodeRepository(db)
	obsRepo := NewProbeObservationRepository(db)
	ctx := context.Background()

	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	nowStr := now.Format(time.RFC3339)

	// 1. Create two subscriptions: one enabled, one disabled
	enabledSubID := "sub-enabled-001"
	disabledSubID := "sub-disabled-002"
	fetchID := "fetch-001"

	_, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'Enabled Sub', 'secret-1', 1, 'rev-1', ?, ?),
		       (?, 'Disabled Sub', 'secret-2', 0, 'rev-2', ?, ?);
	`, enabledSubID, nowStr, nowStr, disabledSubID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert subscriptions: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, finished_at, outcome)
		VALUES (?, ?, ?, ?, 'success');
	`, fetchID, enabledSubID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert fetch: %v", err)
	}

	// 2. Insert 250 active nodes into enabled subscription
	// Taiwan nodes at index 70-72, Canada nodes at index 78-80, and extra targets > 100
	const totalActive = 250
	activeNodes := make([]domain.Node, 0, totalActive)
	for i := 0; i < totalActive; i++ {
		logicalID := fmt.Sprintf("node-active-%03d", i)
		name := fmt.Sprintf("HK-Node-%03d", i)
		if i >= 70 && i <= 72 {
			name = fmt.Sprintf("TW-Node-Taiwan-%03d", i)
		} else if i >= 78 && i <= 80 {
			name = fmt.Sprintf("CA-Node-Canada-%03d", i)
		} else if i >= 150 && i <= 155 {
			name = fmt.Sprintf("US-Node-America-%03d", i)
		}

		createdAt := now.Add(-time.Duration(300-i) * time.Minute)
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

	// Link all 250 active nodes to enabled subscription
	for _, n := range activeNodes {
		_, err := db.ExecContext(ctx, `
			INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
			VALUES (?, ?, ?);
		`, n.LogicalID, enabledSubID, fetchID)
		if err != nil {
			t.Fatalf("failed to insert node source for %s: %v", n.LogicalID, err)
		}
	}

	// 3. Insert inactive nodes (should be excluded when ActiveOnly: true)
	inactiveNodes := []domain.Node{
		{
			LogicalID:          "node-inactive-01",
			Protocol:           domain.ProtocolSS,
			DisplayName:        "Inactive-Node-01",
			Server:             "198.51.100.250",
			Port:               8388,
			Active:             false,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := nodeRepo.UpsertBatch(ctx, inactiveNodes); err != nil {
		t.Fatalf("failed to upsert inactive nodes: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES ('node-inactive-01', ?, ?);
	`, enabledSubID, fetchID)
	if err != nil {
		t.Fatalf("failed to insert inactive node source: %v", err)
	}

	// 4. Insert notice-type nodes (should be excluded when ExcludeNotices: true via confirmedNoticeExclusionSubquery)
	noticeNodes := []domain.Node{
		{
			LogicalID:          "node-notice-01",
			Protocol:           domain.ProtocolSS,
			DisplayName:        "剩余流量：100GB",
			Server:             "198.51.100.251",
			Port:               8388,
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := nodeRepo.UpsertBatch(ctx, noticeNodes); err != nil {
		t.Fatalf("failed to upsert notice nodes: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES ('node-notice-01', ?, ?);
	`, enabledSubID, fetchID)
	if err != nil {
		t.Fatalf("failed to insert notice node source: %v", err)
	}
	payloadID := "payload-notice-01"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, pinned, created_at)
		VALUES (?, ?, ?, 'digest-notice', X'deadbeef', 1, ?);
	`, payloadID, enabledSubID, fetchID, nowStr)
	if err != nil {
		t.Fatalf("failed to insert notice payload: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_entries (id, payload_id, subscription_id, ordinal, source_key, raw_name, protocol, entry_kind, node_logical_id, created_at)
		VALUES ('entry-notice-01', ?, ?, 0, 'key-notice', '剩余流量：100GB', 'ss', 'notice', 'node-notice-01', ?);
	`, payloadID, enabledSubID, nowStr)
	if err != nil {
		t.Fatalf("failed to insert notice entry: %v", err)
	}

	// 5. Insert nodes belonging ONLY to disabled subscription (should be excluded under NodeScopeEnabledSubscriptions)
	disabledNodes := []domain.Node{
		{
			LogicalID:          "node-disabled-sub-01",
			Protocol:           domain.ProtocolSS,
			DisplayName:        "Disabled-Sub-Node-01",
			Server:             "198.51.100.252",
			Port:               8388,
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := nodeRepo.UpsertBatch(ctx, disabledNodes); err != nil {
		t.Fatalf("failed to upsert disabled sub nodes: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES ('node-disabled-sub-01', ?, ?);
	`, disabledSubID, fetchID)
	if err != nil {
		t.Fatalf("failed to insert disabled sub node source: %v", err)
	}

	// -------------------------------------------------------------
	// Verification A: NodeRepository.List vs NodeRepository.ListAll
	// -------------------------------------------------------------
	baseFilter := domain.NodeFilter{
		Scope:          domain.NodeScopeEnabledSubscriptions,
		ActiveOnly:     true,
		ExcludeNotices: true,
	}

	// List without explicit pagination defaults to 50 items
	paginatedNodes, totalCount, err := nodeRepo.List(ctx, baseFilter)
	if err != nil {
		t.Fatalf("nodeRepo.List failed: %v", err)
	}
	if totalCount != totalActive {
		t.Fatalf("expected totalCount=%d, got %d", totalActive, totalCount)
	}
	if len(paginatedNodes) != 50 {
		t.Fatalf("expected default List to return 50 items, got %d", len(paginatedNodes))
	}

	// ListAll MUST return ALL 250 active nodes unpaginated without negative magic
	allNodes, err := nodeRepo.ListAll(ctx, baseFilter)
	if err != nil {
		t.Fatalf("nodeRepo.ListAll failed: %v", err)
	}
	if len(allNodes) != totalActive {
		t.Fatalf("expected ListAll to return exactly %d nodes, got %d", totalActive, len(allNodes))
	}

	// Verify excluded nodes are NOT present in ListAll
	for _, n := range allNodes {
		if n.LogicalID == "node-inactive-01" {
			t.Fatalf("inactive node was not excluded")
		}
		if n.LogicalID == "node-notice-01" {
			t.Fatalf("notice node was not excluded")
		}
		if n.LogicalID == "node-disabled-sub-01" {
			t.Fatalf("disabled sub node was not excluded")
		}
	}

	// -------------------------------------------------------------
	// Verification B: Health Predicates & Mutual Exclusion Conservation
	// -------------------------------------------------------------
	// Setup probe observations with varied health categories:
	rev1 := int64(1)
	revOld := int64(0)

	// 10 healthy nodes (node-active-000 .. node-active-009)
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("node-active-%03d", i)
		_ = obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                 fmt.Sprintf("obs-h-%03d", i),
			ProbeRunID:         "run-001",
			NodeLogicalID:      id,
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			LatencyMS:          35,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-5 * time.Minute),
		})
	}

	// 5 degraded nodes (node-active-010 .. node-active-014)
	for i := 10; i < 15; i++ {
		id := fmt.Sprintf("node-active-%03d", i)
		_ = obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                 fmt.Sprintf("obs-d-%03d", i),
			ProbeRunID:         "run-001",
			NodeLogicalID:      id,
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictRestricted,
			LatencyMS:          120,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-5 * time.Minute),
		})
	}

	// 5 unhealthy nodes (node-active-015 .. node-active-019)
	for i := 15; i < 20; i++ {
		id := fmt.Sprintf("node-active-%03d", i)
		_ = obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                 fmt.Sprintf("obs-u-%03d", i),
			ProbeRunID:         "run-001",
			NodeLogicalID:      id,
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictError,
			RedactedSummary:    "profile=baseline reason=node_connect_failed",
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-5 * time.Minute),
		})
	}

	// 15 undetermined nodes with various causes:
	// - 3 with AI only (node-active-020 .. 022)
	for i := 20; i <= 22; i++ {
		id := fmt.Sprintf("node-active-%03d", i)
		_ = obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                 fmt.Sprintf("obs-ai-%03d", i),
			ProbeRunID:         "run-001",
			NodeLogicalID:      id,
			Kind:               domain.ProbeKindAI,
			Verdict:            domain.VerdictAvailable,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-5 * time.Minute),
		})
	}
	// - 3 stale baseline > 1h (node-active-023 .. 025)
	for i := 23; i <= 25; i++ {
		id := fmt.Sprintf("node-active-%03d", i)
		_ = obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                 fmt.Sprintf("obs-stale-%03d", i),
			ProbeRunID:         "run-001",
			NodeLogicalID:      id,
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-2 * time.Hour),
		})
	}
	// - 3 future dated baseline (node-active-026 .. 028)
	for i := 26; i <= 28; i++ {
		id := fmt.Sprintf("node-active-%03d", i)
		_ = obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                 fmt.Sprintf("obs-fut-%03d", i),
			ProbeRunID:         "run-001",
			NodeLogicalID:      id,
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(30 * time.Minute),
		})
	}
	// - 3 old connection revision (node-active-029 .. 031)
	for i := 29; i <= 31; i++ {
		id := fmt.Sprintf("node-active-%03d", i)
		_ = obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                 fmt.Sprintf("obs-revold-%03d", i),
			ProbeRunID:         "run-001",
			NodeLogicalID:      id,
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			ConnectionRevision: &revOld,
			ObservedAt:         now.Add(-5 * time.Minute),
		})
	}
	// - 3 security / config rejection error (node-active-032 .. 034)
	for i := 32; i <= 34; i++ {
		id := fmt.Sprintf("node-active-%03d", i)
		_ = obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                 fmt.Sprintf("obs-rej-%03d", i),
			ProbeRunID:         "run-001",
			NodeLogicalID:      id,
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictError,
			RedactedSummary:    "reason=unsafe_tls_rejected",
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-5 * time.Minute),
		})
	}

	// Remaining active nodes (035 .. 249) = 215 nodes have 0 observations -> untested!

	queryHealth := func(statuses ...string) int {
		t.Helper()
		f := baseFilter
		f.Now = &now
		f.HealthStatuses = statuses
		nodes, err := nodeRepo.ListAll(ctx, f)
		if err != nil {
			t.Fatalf("ListAll with HealthStatuses %v failed: %v", statuses, err)
		}
		// Also verify List with large page gives matching total
		f.Pagination = domain.Pagination{Page: 1, PageSize: 100}
		_, listTotal, err := nodeRepo.List(ctx, f)
		if err != nil {
			t.Fatalf("List with HealthStatuses %v failed: %v", statuses, err)
		}
		if listTotal != len(nodes) {
			t.Fatalf("List total (%d) != ListAll count (%d) for statuses %v", listTotal, len(nodes), statuses)
		}
		return len(nodes)
	}

	healthyCount := queryHealth("healthy")
	degradedCount := queryHealth("degraded")
	unhealthyCount := queryHealth("unhealthy")
	undeterminedCount := queryHealth("undetermined")
	untestedCount := queryHealth("untested")

	if healthyCount != 10 {
		t.Fatalf("expected healthy=10, got %d", healthyCount)
	}
	if degradedCount != 5 {
		t.Fatalf("expected degraded=5, got %d", degradedCount)
	}
	if unhealthyCount != 5 {
		t.Fatalf("expected unhealthy=5, got %d", unhealthyCount)
	}
	if undeterminedCount != 15 {
		t.Fatalf("expected undetermined=15, got %d", undeterminedCount)
	}
	if untestedCount != 215 {
		t.Fatalf("expected untested=215, got %d", untestedCount)
	}

	// Conservation check: sum of all 5 disjoint categories MUST equal total active nodes (250)
	sum := healthyCount + degradedCount + unhealthyCount + undeterminedCount + untestedCount
	if sum != totalActive {
		t.Fatalf("conservation violated! healthy(%d) + degraded(%d) + unhealthy(%d) + undetermined(%d) + untested(%d) = %d != %d",
			healthyCount, degradedCount, unhealthyCount, undeterminedCount, untestedCount, sum, totalActive)
	}

	// Multi-value check: healthy + degraded = 15
	multiCount := queryHealth("healthy", "degraded")
	if multiCount != 15 {
		t.Fatalf("expected multi healthy+degraded=15, got %d", multiCount)
	}

	// Multi-value check: undetermined + untested = 230
	multiUndetUntested := queryHealth("undetermined", "untested")
	if multiUndetUntested != 230 {
		t.Fatalf("expected multi undetermined+untested=230, got %d", multiUndetUntested)
	}
}
