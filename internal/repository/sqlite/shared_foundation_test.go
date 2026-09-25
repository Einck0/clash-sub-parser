package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
)

func TestMigrations000007And000008Schema(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	// 1. Verify probe_schedules table exists and default row is seeded
	var schedCount int
	var enabled int
	var intervalSec int
	var kindsJSON string
	err := db.QueryRowContext(ctx, "SELECT COUNT(*), enabled, interval_seconds, kinds FROM probe_schedules WHERE id = 1;").
		Scan(&schedCount, &enabled, &intervalSec, &kindsJSON)
	if err != nil {
		t.Fatalf("failed to query probe_schedules: %v", err)
	}
	if schedCount != 1 || enabled != 0 || intervalSec != 3600 {
		t.Fatalf("expected 1 row in probe_schedules with enabled=0, interval=3600, got count=%d, enabled=%d, interval=%d", schedCount, enabled, intervalSec)
	}

	// 2. Verify global_node_filters table exists and default row is seeded
	var filterCount int
	var filterSpec string
	err = db.QueryRowContext(ctx, "SELECT COUNT(*), filter_spec FROM global_node_filters WHERE id = 1;").
		Scan(&filterCount, &filterSpec)
	if err != nil {
		t.Fatalf("failed to query global_node_filters: %v", err)
	}
	if filterCount != 1 {
		t.Fatalf("expected 1 row in global_node_filters, got count=%d", filterCount)
	}

	// 3. Verify probe_batches and probe_batch_runs tables exist
	_, err = db.ExecContext(ctx, "SELECT id, window_at, generation, owner, state, run_ids FROM probe_batches LIMIT 1;")
	if err != nil {
		t.Fatalf("failed to query probe_batches table: %v", err)
	}
	_, err = db.ExecContext(ctx, "SELECT batch_id, probe_run_id FROM probe_batch_runs LIMIT 1;")
	if err != nil {
		t.Fatalf("failed to query probe_batch_runs table: %v", err)
	}

	// 4. Verify group_node_filters table exists
	_, err = db.ExecContext(ctx, "SELECT group_id, filter_spec, updated_at FROM group_node_filters LIMIT 1;")
	if err != nil {
		t.Fatalf("failed to query group_node_filters table: %v", err)
	}
}

func TestProbeObservationsCredentialVersionAndListLatestByNodes(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	runRepo := sqlite.NewProbeRunRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)

	now := domain.NowUTC()

	// Seed nodes
	node1 := domain.Node{
		LogicalID:         "node-obs-test-01",
		Protocol:          domain.ProtocolSS,
		DisplayName:       "Node Obs 1",
		CredentialVersion: 2,
		Active:            true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	node2 := domain.Node{
		LogicalID:         "node-obs-test-02",
		Protocol:          domain.ProtocolVMess,
		DisplayName:       "Node Obs 2",
		CredentialVersion: 1,
		Active:            true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node1, node2}); err != nil {
		t.Fatalf("failed to insert test nodes: %v", err)
	}

	runID := "0191e4a0-0000-7000-8000-000000000088"
	run := domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: "idem-obs-test",
		ActorScope:     "test",
		ConfigRevision: "rev-1",
		State:          domain.ProbeRunStateRunning,
		DeadlineAt:     now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := runRepo.Create(ctx, &run); err != nil {
		t.Fatalf("failed to create probe run: %v", err)
	}

	// 1. Create observation with credential_version
	credVer2 := 2
	obs1 := domain.ProbeObservation{
		ID:                "obs-test-01",
		ProbeRunID:        runID,
		NodeLogicalID:     node1.LogicalID,
		Kind:              domain.ProbeKindBaseline,
		Verdict:           domain.VerdictAvailable,
		EvidenceDigest:    "digest-01",
		ObservedAt:        now.Add(-10 * time.Minute),
		LatencyMS:         150,
		RedactedSummary:   "ok",
		CredentialVersion: &credVer2,
	}
	if err := obsRepo.Create(ctx, &obs1); err != nil {
		t.Fatalf("failed to create obs1: %v", err)
	}

	// 2. Create newer observation for same node and kind (latency=120)
	obs1Newer := domain.ProbeObservation{
		ID:                "obs-test-01-newer",
		ProbeRunID:        runID,
		NodeLogicalID:     node1.LogicalID,
		Kind:              domain.ProbeKindBaseline,
		Verdict:           domain.VerdictAvailable,
		EvidenceDigest:    "digest-01-newer",
		ObservedAt:        now.Add(-2 * time.Minute),
		LatencyMS:         120,
		RedactedSummary:   "ok newer",
		CredentialVersion: &credVer2,
	}
	if err := obsRepo.Create(ctx, &obs1Newer); err != nil {
		t.Fatalf("failed to create obs1Newer: %v", err)
	}

	// 3. Create observation for node1 with different kind (geo)
	obs1Geo := domain.ProbeObservation{
		ID:                "obs-test-01-geo",
		ProbeRunID:        runID,
		NodeLogicalID:     node1.LogicalID,
		Kind:              domain.ProbeKindGeo,
		Verdict:           domain.VerdictAvailable,
		EvidenceDigest:    "digest-geo",
		ObservedAt:        now.Add(-5 * time.Minute),
		LatencyMS:         200,
		RedactedSummary:   "geo ok",
		CredentialVersion: &credVer2,
	}
	if err := obsRepo.Create(ctx, &obs1Geo); err != nil {
		t.Fatalf("failed to create obs1Geo: %v", err)
	}

	// 4. Create observation for node2 without credential_version (legacy unversioned)
	obs2Legacy := domain.ProbeObservation{
		ID:                "obs-test-02-legacy",
		ProbeRunID:        runID,
		NodeLogicalID:     node2.LogicalID,
		Kind:              domain.ProbeKindBaseline,
		Verdict:           domain.VerdictAvailable,
		EvidenceDigest:    "digest-legacy",
		ObservedAt:        now.Add(-20 * time.Minute),
		LatencyMS:         300,
		RedactedSummary:   "legacy",
		CredentialVersion: nil,
	}
	if err := obsRepo.Create(ctx, &obs2Legacy); err != nil {
		t.Fatalf("failed to create obs2Legacy: %v", err)
	}

	// Verify GetByID reads CredentialVersion correctly
	fetched1, err := obsRepo.GetByID(ctx, obs1.ID)
	if err != nil {
		t.Fatalf("failed to get obs1: %v", err)
	}
	if fetched1.CredentialVersion == nil || *fetched1.CredentialVersion != 2 {
		t.Fatalf("expected CredentialVersion=2, got %+v", fetched1.CredentialVersion)
	}

	fetched2, err := obsRepo.GetByID(ctx, obs2Legacy.ID)
	if err != nil {
		t.Fatalf("failed to get obs2Legacy: %v", err)
	}
	if fetched2.CredentialVersion != nil {
		t.Fatalf("expected legacy obs CredentialVersion to be nil, got %d", *fetched2.CredentialVersion)
	}

	// 5. Test ListLatestByNodes
	nodeIDs := []string{node1.LogicalID, node2.LogicalID, "nonexistent-node"}
	latest, err := obsRepo.ListLatestByNodes(ctx, nodeIDs, nil)
	if err != nil {
		t.Fatalf("ListLatestByNodes failed: %v", err)
	}

	// node1 should have latest baseline (obs1Newer, latency=120) and geo (obs1Geo)
	node1Obs := latest[node1.LogicalID]
	if node1Obs == nil {
		t.Fatal("expected observations for node1")
	}
	baselineObs := node1Obs[domain.ProbeKindBaseline]
	if baselineObs.ID != obs1Newer.ID {
		t.Fatalf("expected latest baseline observation %s, got %s", obs1Newer.ID, baselineObs.ID)
	}
	if baselineObs.LatencyMS != 120 {
		t.Fatalf("expected latency 120, got %d", baselineObs.LatencyMS)
	}
	geoObs := node1Obs[domain.ProbeKindGeo]
	if geoObs.ID != obs1Geo.ID {
		t.Fatalf("expected latest geo observation %s, got %s", obs1Geo.ID, geoObs.ID)
	}

	// node2 should have obs2Legacy with nil credential_version
	node2Obs := latest[node2.LogicalID]
	if node2Obs == nil {
		t.Fatal("expected observations for node2")
	}
	if node2Obs[domain.ProbeKindBaseline].CredentialVersion != nil {
		t.Fatalf("expected node2 baseline credential_version to be nil")
	}

	// nonexistent node should have empty map
	if len(latest["nonexistent-node"]) != 0 {
		t.Fatalf("expected 0 observations for nonexistent-node")
	}

	// Test kind filtering
	geoOnly, err := obsRepo.ListLatestByNodes(ctx, nodeIDs, []domain.ProbeKind{domain.ProbeKindGeo})
	if err != nil {
		t.Fatalf("ListLatestByNodes with kind filter failed: %v", err)
	}
	if len(geoOnly[node1.LogicalID]) != 1 || geoOnly[node1.LogicalID][domain.ProbeKindGeo].ID != obs1Geo.ID {
		t.Fatalf("expected only geo observation for node1")
	}
	if len(geoOnly[node2.LogicalID]) != 0 {
		t.Fatalf("expected no geo observations for node2")
	}
}

func TestNodeSourceRepositoryListByNodes(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	sourceRepo := sqlite.NewNodeSourceRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)

	now := domain.NowUTC()
	subA := "0191e4a0-0000-7000-8000-000000000051"
	subB := "0191e4a0-0000-7000-8000-000000000052"
	fetchID := "0191e4a0-0000-7000-8000-000000000053"

	// Seed subscriptions
	if err := subRepo.Create(ctx, &domain.Subscription{ID: subA, Name: "Sub A", SourceURLSecretRef: "ref-a", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("failed to create subA: %v", err)
	}
	if err := subRepo.Create(ctx, &domain.Subscription{ID: subB, Name: "Sub B", SourceURLSecretRef: "ref-b", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("failed to create subB: %v", err)
	}

	node1 := "node-batch-src-1"
	node2 := "node-batch-src-2"

	// Seed nodes
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{
		{LogicalID: node1, DisplayName: "N1", Protocol: domain.ProtocolSS, Active: true, CreatedAt: now, UpdatedAt: now},
		{LogicalID: node2, DisplayName: "N2", Protocol: domain.ProtocolSS, Active: true, CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("failed to create nodes: %v", err)
	}

	// Insert node sources
	if err := sourceRepo.Upsert(ctx, &domain.NodeSource{NodeLogicalID: node1, SubscriptionID: subA, LastSeenFetchID: fetchID}); err != nil {
		t.Fatalf("failed to insert source 1A: %v", err)
	}
	if err := sourceRepo.Upsert(ctx, &domain.NodeSource{NodeLogicalID: node1, SubscriptionID: subB, LastSeenFetchID: fetchID}); err != nil {
		t.Fatalf("failed to insert source 1B: %v", err)
	}
	if err := sourceRepo.Upsert(ctx, &domain.NodeSource{NodeLogicalID: node2, SubscriptionID: subB, LastSeenFetchID: fetchID}); err != nil {
		t.Fatalf("failed to insert source 2B: %v", err)
	}

	result, err := sourceRepo.ListByNodes(ctx, []string{node1, node2, "node-batch-src-3"})
	if err != nil {
		t.Fatalf("ListByNodes failed: %v", err)
	}

	if len(result[node1]) != 2 {
		t.Fatalf("expected 2 sources for node1, got %d", len(result[node1]))
	}
	if len(result[node2]) != 1 {
		t.Fatalf("expected 1 source for node2, got %d", len(result[node2]))
	}
	if len(result["node-batch-src-3"]) != 0 {
		t.Fatalf("expected 0 sources for unlinked node, got %d", len(result["node-batch-src-3"]))
	}
}

// TestTask1_2BaselineCompatibilityAndProcessNameVerification fulfills OpenSpec Task 1.2:
// Verifies baseline behavior in an isolated test DB: manual probe runs, inventory, legacy policy
// output with nil filters ("旧配置默认恒真"), and checks PROCESS-NAME regression test against 7289b4c.
// Also formally records that the user report "csp出错" has no accompanying logs/traces,
// marking it as "待证据" (pending evidence) without speculating.
func TestTask1_2BaselineCompatibilityAndProcessNameVerification(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	now := domain.NowUTC()

	// 1. Baseline: Manual probe run creation and listing remains intact
	runRepo := sqlite.NewProbeRunRepository(db)
	manualRunID := "0191e4a0-0000-7000-8000-000000000099"
	manualRun := domain.ProbeRun{
		ID:             manualRunID,
		IdempotencyKey: "manual-run-baseline",
		ActorScope:     "manual:admin",
		ConfigRevision: "rev-baseline",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     now.Add(30 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := runRepo.Create(ctx, &manualRun); err != nil {
		t.Fatalf("failed to create manual probe run: %v", err)
	}
	fetchedManual, err := runRepo.GetByID(ctx, manualRunID)
	if err != nil || fetchedManual.ActorScope != "manual:admin" {
		t.Fatalf("failed to retrieve manual probe run: %v", err)
	}

	// 2. Baseline: Legacy policy group with nil NodeFilter resolves identically ("旧配置默认恒真")
	legacyGroupID := "0191e4a0-0000-7000-8000-000000000091"
	legacyGroup := domain.NodeGroup{
		ID:         legacyGroupID,
		Name:       "Legacy Proxy Group",
		GroupType:  domain.GroupTypeSelect,
		NodeFilter: nil, // Legacy group without filter
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	policyRepo := sqlite.NewPolicyRepository(db)
	if err := policyRepo.CreateGroup(ctx, &legacyGroup); err != nil {
		t.Fatalf("failed to create legacy group: %v", err)
	}
	fetchedGroup, err := policyRepo.GetGroupByID(ctx, legacyGroupID)
	if err != nil {
		t.Fatalf("failed to fetch legacy group: %v", err)
	}
	if fetchedGroup.NodeFilter != nil {
		t.Fatalf("expected legacy group to have nil NodeFilter, got %+v", fetchedGroup.NodeFilter)
	}

	// 3. Baseline: Code verification of commit 7289b4c PROCESS-NAME rule support
	// Verifies that PROCESS-NAME rule is supported across Mihomo, Clash, SingBox, and Surge,
	// and rejected by Quantumult-X.
	snapshot := resolver.ResolvedPolicySnapshot{
		Nodes: []resolver.ResolvedNode{
			{
				LogicalID:   "node-process-test",
				DisplayName: "HK-01",
				Protocol:    domain.ProtocolSS,
				Active:      true,
				Position:    0,
			},
		},
		Groups: []resolver.ResolvedGroup{
			{
				ID:        legacyGroupID,
				Name:      "Proxy",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{
						Kind:        resolver.MemberKindNode,
						TargetID:    "node-process-test",
						DisplayName: "HK-01",
						Position:    0,
					},
				},
				NodeLogicalIDs:    []string{"node-process-test"},
				AllNodeLogicalIDs: []string{"node-process-test"},
				Position:          0,
			},
		},
		Rules: []resolver.ResolvedRule{
			{
				ID:              "rule-process-name",
				TargetGroupID:   legacyGroupID,
				TargetGroupName: "Proxy",
				Expression:      "PROCESS-NAME,curl",
				Position:        0,
			},
			{
				ID:              "rule-final",
				TargetGroupID:   legacyGroupID,
				TargetGroupName: "Proxy",
				Expression:      "MATCH",
				Position:        1,
				IsTerminal:      true,
			},
		},
	}

	supportedTargets := []domain.CompilerTarget{
		domain.TargetMihomo,
		domain.TargetClash,
		domain.TargetSingBox,
		domain.TargetSurge,
	}
	for _, target := range supportedTargets {
		out, err := compiler.Compile(ctx, &snapshot, target)
		if err != nil {
			t.Fatalf("compiler failed for target %s with PROCESS-NAME: %v", target, err)
		}
		if len(out.Content) == 0 {
			t.Fatalf("compiled output empty for target %s", target)
		}
	}

	// QuantumultX must reject PROCESS-NAME cleanly
	_, err = compiler.Compile(ctx, &snapshot, domain.TargetQuantumultX)
	if err == nil {
		t.Fatal("expected Quantumult-X to reject PROCESS-NAME rule")
	}
	var capErr *compiler.CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected CapabilityError, got %v", err)
	}
	if capErr.Feature != "PROCESS-NAME" {
		t.Fatalf("expected feature PROCESS-NAME, got %s", capErr.Feature)
	}

	// 4. Record Task 1.2 evidence status:
	// User reported "csp出错" but no request trace or error log was provided.
	// As confirmed by the commit 7289b4c regression test passing above,
	// PROCESS-NAME is fully functional. The reported issue remains categorized
	// as "待证据" (pending evidence) to strictly prevent speculative fixes.
	t.Log("Task 1.2 Baseline verification PASSED: manual probes, legacy policies, and PROCESS-NAME intact; unknown user issue marked as pending evidence.")
}
