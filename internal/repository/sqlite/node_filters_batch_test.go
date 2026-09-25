// Package sqlite_test contains tests for node_filters repository.
package sqlite_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestNodeFilterRepository_BatchAndTransactionConsistency(t *testing.T) {
	db, _ := setupTestDB(t)
	repo := sqlite.NewNodeFilterRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	runRepo := sqlite.NewProbeRunRepository(db)
	ctx := context.Background()

	// 1. Create a batch of nodes (10 nodes)
	var nodes []domain.Node
	var nodeIDs []string
	for i := 0; i < 10; i++ {
		nid := fmt.Sprintf("0195c100-0000-7000-8000-%012d", i+1)
		nodeIDs = append(nodeIDs, nid)
		nodes = append(nodes, domain.Node{
			LogicalID:         nid,
			DisplayName:       fmt.Sprintf("Node-%02d", i+1),
			Protocol:          domain.ProtocolSS,
			Active:            true,
			CredentialVersion: 1,
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		})
	}
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatalf("UpsertBatch failed: %v", err)
	}

	// 2. Associate sources with nodes
	subRepo := sqlite.NewSubscriptionRepository(db)
	subID := "0195c200-0000-7000-8000-000000000001"
	if err := subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "Test Sub",
		SourceURLSecretRef: "secret-ref",
		Enabled:            true,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Create subscription failed: %v", err)
	}
	for _, nid := range nodeIDs[:5] {
		err := sourceRepo.Upsert(ctx, &domain.NodeSource{
			NodeLogicalID:    nid,
			SubscriptionID:   subID,
			LastSeenFetchID: "fetch-1",
		})
		if err != nil {
			t.Fatalf("Upsert source failed: %v", err)
		}
	}

	// Verify ListByNodes for sources does not exceed N+1 (single/chunked query)
	sourcesMap, err := sourceRepo.ListByNodes(ctx, nodeIDs)
	if err != nil {
		t.Fatalf("ListByNodes failed: %v", err)
	}
	if len(sourcesMap) != 10 {
		t.Fatalf("expected 10 entries in sourcesMap, got %d", len(sourcesMap))
	}
	for i, nid := range nodeIDs {
		if i < 5 {
			if len(sourcesMap[nid]) != 1 {
				t.Fatalf("expected 1 source for %s, got %d", nid, len(sourcesMap[nid]))
			}
		} else {
			if len(sourcesMap[nid]) != 0 {
				t.Fatalf("expected 0 sources for %s, got %d", nid, len(sourcesMap[nid]))
			}
		}
	}

	// 3. Insert probe run and observations
	runID := "0195c300-0000-7000-8000-000000000001"
	if err := runRepo.Create(ctx, &domain.ProbeRun{
		ID:        runID,
		State:     domain.ProbeRunStateSucceeded,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Create probe run failed: %v", err)
	}

	for i, nid := range nodeIDs {
		ver := 1
		if i == 0 {
			// One node has mismatched credential version
			ver = 2
		}
		obsID := fmt.Sprintf("obs-bulk-%02d", i+1)
		err := obsRepo.Create(ctx, &domain.ProbeObservation{
			ID:                obsID,
			ProbeRunID:        runID,
			NodeLogicalID:     nid,
			Kind:              domain.ProbeKindBaseline,
			Verdict:           domain.VerdictAvailable,
			EvidenceDigest:    "digest",
			LatencyMS:         50,
			ObservedAt:        time.Now().UTC(),
			CredentialVersion: &ver,
		})
		if err != nil {
			t.Fatalf("Create probe observation failed: %v", err)
		}
	}

	// 4. Batch query latest observations
	obsMap, err := obsRepo.ListLatestByNodes(ctx, nodeIDs, []domain.ProbeKind{domain.ProbeKindBaseline})
	if err != nil {
		t.Fatalf("ListLatestByNodes failed: %v", err)
	}
	if len(obsMap) != 10 {
		t.Fatalf("expected 10 entries in obsMap, got %d", len(obsMap))
	}

	// 5. Test group filter batch list
	var groupIDs []string
	for i := 0; i < 3; i++ {
		gid := fmt.Sprintf("0195c400-0000-7000-8000-%012d", i+1)
		groupIDs = append(groupIDs, gid)
		err := policyRepo.CreateGroup(ctx, &domain.NodeGroup{
			ID:        gid,
			Name:      fmt.Sprintf("Group-%d", i+1),
			GroupType: domain.GroupTypeSelect,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("CreateGroup failed: %v", err)
		}
		err = repo.SetGroupFilter(ctx, &domain.GroupNodeFilter{
			GroupID: gid,
			Spec: domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{
						Field: domain.FilterFieldDisplayName,
						Op:    domain.FilterOpContains,
						Value: fmt.Sprintf("Node-%02d", i+1),
					},
				},
			},
			UpdatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("SetGroupFilter failed: %v", err)
		}
	}

	groupFilters, err := repo.ListGroupFilters(ctx)
	if err != nil {
		t.Fatalf("ListGroupFilters failed: %v", err)
	}
	if len(groupFilters) != 3 {
		t.Fatalf("expected 3 group filters, got %d", len(groupFilters))
	}
	for _, gid := range groupIDs {
		if _, ok := groupFilters[gid]; !ok {
			t.Fatalf("missing group filter for %s", gid)
		}
	}
}

func TestNodeFilter_UnknownOrMissingCredentialVersion_FailClosed(t *testing.T) {
	node := domain.Node{
		LogicalID:         "0195c500-0000-7000-8000-000000000001",
		DisplayName:       "Node-Version-Test",
		Protocol:          domain.ProtocolSS,
		CredentialVersion: 3,
	}

	kind := domain.ProbeKindBaseline
	freshness := 3600
	cond := domain.FilterCondition{
		Field:            domain.FilterFieldProbeVerdict,
		Op:               domain.FilterOpEquals,
		Value:            "available",
		ProbeKind:        &kind,
		FreshnessSeconds: &freshness,
	}

	// 1. Observation with nil credential_version (legacy/unknown)
	obsNil := domain.ProbeObservation{
		ID:                "obs-unknown-ver",
		NodeLogicalID:     node.LogicalID,
		Kind:              kind,
		Verdict:           domain.VerdictAvailable,
		ObservedAt:        time.Now().UTC(),
		CredentialVersion: nil, // Unknown / unversioned
	}

	matched, reason := domain.MatchesCondition(cond, node, nil, map[domain.ProbeKind]domain.ProbeObservation{kind: obsNil}, time.Now().UTC())
	if matched {
		t.Fatalf("observation with nil CredentialVersion must fail-closed, got matched=true")
	}
	if reason == "" {
		t.Fatalf("expected failure reason for unversioned observation")
	}

	// 2. Observation with mismatched version (version 2 vs node version 3)
	ver2 := 2
	obsMismatch := domain.ProbeObservation{
		ID:                "obs-mismatch-ver",
		NodeLogicalID:     node.LogicalID,
		Kind:              kind,
		Verdict:           domain.VerdictAvailable,
		ObservedAt:        time.Now().UTC(),
		CredentialVersion: &ver2,
	}
	matched, _ = domain.MatchesCondition(cond, node, nil, map[domain.ProbeKind]domain.ProbeObservation{kind: obsMismatch}, time.Now().UTC())
	if matched {
		t.Fatalf("observation with mismatched CredentialVersion must fail-closed, got matched=true")
	}

	// 3. Negated condition OpNotEquals with unknown version MUST ALSO fail-closed!
	condNeg := domain.FilterCondition{
		Field:            domain.FilterFieldProbeVerdict,
		Op:               domain.FilterOpNotEquals,
		Value:            "error",
		ProbeKind:        &kind,
		FreshnessSeconds: &freshness,
	}
	matched, _ = domain.MatchesCondition(condNeg, node, nil, map[domain.ProbeKind]domain.ProbeObservation{kind: obsNil}, time.Now().UTC())
	if matched {
		t.Fatalf("negated probe condition with unversioned observation must fail-closed, got matched=true")
	}

	// 4. Matching version 3 passes
	ver3 := 3
	obsMatching := domain.ProbeObservation{
		ID:                "obs-matching-ver",
		NodeLogicalID:     node.LogicalID,
		Kind:              kind,
		Verdict:           domain.VerdictAvailable,
		ObservedAt:        time.Now().UTC(),
		CredentialVersion: &ver3,
	}
	matched, _ = domain.MatchesCondition(cond, node, nil, map[domain.ProbeKind]domain.ProbeObservation{kind: obsMatching}, time.Now().UTC())
	if !matched {
		t.Fatalf("observation with matching CredentialVersion should pass")
	}
}

func TestNodeFilter_OldConfigDefaultPassThrough(t *testing.T) {
	node := domain.Node{
		LogicalID:   "0195c600-0000-7000-8000-000000000001",
		DisplayName: "Node-PassThrough-Test",
		Protocol:    domain.ProtocolSS,
	}

	// Nil spec or empty conditions spec evaluates to true (pass-through)
	matched, _ := domain.MatchesFilter(nil, node, nil, nil, time.Now().UTC())
	if !matched {
		t.Fatalf("nil spec must be pass-through")
	}

	emptySpec := &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{}}
	matched, _ = domain.MatchesFilter(emptySpec, node, nil, nil, time.Now().UTC())
	if !matched {
		t.Fatalf("empty spec must be pass-through")
	}
}
