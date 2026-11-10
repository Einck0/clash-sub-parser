package sqlite

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestNodeFilterPredicates_CanonicalUnknownParity(t *testing.T) {
	db := setupNodeProjectionDB(t)
	nodeRepo := NewNodeRepository(db)
	obsRepo := NewProbeObservationRepository(db)
	ctx := context.Background()

	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	rev1 := int64(1)
	rev2 := int64(2)

	// Create 5 test nodes:
	// node-untested: no observations at all
	// node-ai-only: only AI observations (no baseline)
	// node-rev-mismatch: baseline observation on old revision (rev1 vs node's rev2)
	// node-healthy: baseline available matching rev1
	// node-unhealthy: baseline error matching rev1
	nodes := []domain.Node{
		{
			LogicalID:          "node-untested",
			DisplayName:        "Untested Node",
			Protocol:           domain.ProtocolSS,
			Server:             "192.0.2.1",
			Port:               8388,
			Active:             true,
			ConnectionRevision: rev1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			LogicalID:          "node-ai-only",
			DisplayName:        "AI Only Node",
			Protocol:           domain.ProtocolSS,
			Server:             "192.0.2.2",
			Port:               8388,
			Active:             true,
			ConnectionRevision: rev1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			LogicalID:          "node-rev-mismatch",
			DisplayName:        "Rev Mismatch Node",
			Protocol:           domain.ProtocolSS,
			Server:             "192.0.2.3",
			Port:               8388,
			Active:             true,
			ConnectionRevision: rev1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			LogicalID:          "node-healthy",
			DisplayName:        "Healthy Node",
			Protocol:           domain.ProtocolSS,
			Server:             "192.0.2.4",
			Port:               8388,
			Active:             true,
			ConnectionRevision: rev1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			LogicalID:          "node-unhealthy",
			DisplayName:        "Unhealthy Node",
			Protocol:           domain.ProtocolSS,
			Server:             "192.0.2.5",
			Port:               8388,
			Active:             true,
			ConnectionRevision: rev1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatalf("failed to upsert nodes: %v", err)
	}

	// Bump node-rev-mismatch connection_revision to rev2 by changing server via supported repository operation
	nodes[2].Server = "192.0.2.33"
	nodes[2].ConnectionRevision = rev2
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{nodes[2]}); err != nil {
		t.Fatalf("failed to bump node revision: %v", err)
	}

	// Create observations
	observations := []*domain.ProbeObservation{
		{
			ID:                 "obs-ai-1",
			ProbeRunID:         "run-1",
			NodeLogicalID:      "node-ai-only",
			Kind:               domain.ProbeKindAI,
			Verdict:            domain.VerdictAvailable,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-time.Minute),
		},
		{
			ID:                 "obs-mismatch-1",
			ProbeRunID:         "run-1",
			NodeLogicalID:      "node-rev-mismatch",
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			LatencyMS:          50,
			ConnectionRevision: &rev1, // node is rev2
			ObservedAt:         now.Add(-time.Minute),
		},
		{
			ID:                 "obs-healthy-1",
			ProbeRunID:         "run-1",
			NodeLogicalID:      "node-healthy",
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictAvailable,
			LatencyMS:          50,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-time.Minute),
		},
		{
			ID:                 "obs-unhealthy-1",
			ProbeRunID:         "run-1",
			NodeLogicalID:      "node-unhealthy",
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictError,
			ConnectionRevision: &rev1,
			ObservedAt:         now.Add(-time.Minute),
		},
	}

	if err := NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: "run-1", IdempotencyKey: "run-1", State: domain.ProbeRunStateSucceeded, DeadlineAt: now}); err != nil {
		t.Fatalf("create parent probe run: %v", err)
	}
	for _, obs := range observations {
		if err := obsRepo.Create(ctx, obs); err != nil {
			t.Fatalf("failed to create observation %s: %v", obs.ID, err)
		}
	}

	query := func(statuses ...string) []string {
		t.Helper()
		f := domain.NodeFilter{
			ActiveOnly:     true,
			HealthStatuses: statuses,
			Now:            &now,
		}
		res, err := nodeRepo.ListAll(ctx, f)
		if err != nil {
			t.Fatalf("ListAll(%v) failed: %v", statuses, err)
		}
		ids := make([]string, len(res))
		for i, n := range res {
			ids[i] = n.LogicalID
		}
		return ids
	}

	// 1. Query 'unknown' must match node-untested, node-ai-only, node-rev-mismatch (3 nodes)
	unknownIDs := query("unknown")
	if len(unknownIDs) != 3 {
		t.Fatalf("expected 3 unknown nodes, got %d: %v", len(unknownIDs), unknownIDs)
	}

	// 2. Query 'untested' must match 0 nodes (canonical SQL output is unknown, no dual track compatibility)
	untestedIDs := query("untested")
	if len(untestedIDs) != 0 {
		t.Fatalf("expected 0 untested nodes for canonical unknown SQL output, got %d: %v", len(untestedIDs), untestedIDs)
	}

	// 3. Query 'healthy' must match node-healthy (1 node)
	healthyIDs := query("healthy")
	if len(healthyIDs) != 1 || healthyIDs[0] != "node-healthy" {
		t.Fatalf("expected [node-healthy], got %v", healthyIDs)
	}

	// 4. Query 'unhealthy' must match node-unhealthy (1 node)
	unhealthyIDs := query("unhealthy")
	if len(unhealthyIDs) != 1 || unhealthyIDs[0] != "node-unhealthy" {
		t.Fatalf("expected [node-unhealthy], got %v", unhealthyIDs)
	}

	// 5. Verify domain.ClassifyNodeHealth parity
	for _, id := range []string{"node-untested", "node-ai-only", "node-rev-mismatch"} {
		obsList, err := obsRepo.ListByNode(ctx, id, 10)
		if err != nil {
			t.Fatalf("ListByNode(%s) failed: %v", id, err)
		}
		var targetNode domain.Node
		for _, n := range nodes {
			if n.LogicalID == id {
				targetNode = n
				break
			}
		}
		cat := domain.ClassifyNodeHealthFromSlice(obsList, targetNode.ConnectionRevision, now, 0)
		if cat != domain.HealthCategoryUnknown {
			t.Errorf("domain.ClassifyNodeHealthFromSlice for %s = %s, expected %s", id, cat, domain.HealthCategoryUnknown)
		}
	}
}
