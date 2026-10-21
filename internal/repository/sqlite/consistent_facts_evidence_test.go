package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestConsistentFactsLoader_EvidenceDataSyncAndFiltering(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	loader := sqlite.NewConsistentFactsLoader(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	runRepo := sqlite.NewProbeRunRepository(db)

	baseTime := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	asOf := baseTime

	// 1. Insert active configuration revision
	revID := "rev-evidence-test"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-ev', 'active', ?);
	`, revID, baseTime.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert revision failed: %v", err)
	}

	// 2. Insert/update global node filter with exit_region condition so inspectFilterDependencies requires observations
	if _, err := db.ExecContext(ctx, `
		INSERT INTO global_node_filters (id, filter_spec, updated_at)
		VALUES (1, '{"conditions":[{"field":"exit_region","op":"equals","value":"HK"}]}', ?)
		ON CONFLICT(id) DO UPDATE SET filter_spec = excluded.filter_spec, updated_at = excluded.updated_at;
	`, baseTime.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert global_node_filters failed: %v", err)
	}

	// 3. Insert test nodes
	nodes := []domain.Node{
		{
			LogicalID:   "node-hk",
			Protocol:    domain.ProtocolSS,
			DisplayName: "HK Healthy Node",
			Server:      "198.51.100.1",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret"},
			Active:      true,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			LogicalID:   "node-future",
			Protocol:    domain.ProtocolSS,
			DisplayName: "Future Obs Node",
			Server:      "198.51.100.2",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret"},
			Active:      true,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			LogicalID:   "node-wrong-rev",
			Protocol:    domain.ProtocolSS,
			DisplayName: "Wrong Revision Node",
			Server:      "198.51.100.3",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret"},
			Active:      true,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			LogicalID:   "node-shadowed",
			Protocol:    domain.ProtocolSS,
			DisplayName: "Shadowed By Failure Node",
			Server:      "198.51.100.4",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret"},
			Active:      true,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			LogicalID:   "node-malformed-json",
			Protocol:    domain.ProtocolSS,
			DisplayName: "Malformed Evidence Node",
			Server:      "198.51.100.5",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret"},
			Active:      true,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
		{
			LogicalID:   "node-missing-obs",
			Protocol:    domain.ProtocolSS,
			DisplayName: "Missing Observations Node",
			Server:      "198.51.100.6",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret"},
			Active:      true,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		},
	}
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatalf("UpsertBatch nodes failed: %v", err)
	}

	// Bump node-wrong-rev connection_revision by changing server
	nodes[2].Server = "198.51.100.33"
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{nodes[2]}); err != nil {
		t.Fatalf("UpsertBatch bump revision failed: %v", err)
	}

	// 4. Create probe run
	runID := "run-ev-001"
	if err := runRepo.Create(ctx, &domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: "idem-ev-run",
		ActorScope:     "test",
		ConfigRevision: "rev-evidence-test",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     baseTime.Add(time.Hour),
		CreatedAt:      baseTime,
		UpdatedAt:      baseTime,
	}); err != nil {
		t.Fatalf("Create probe run failed: %v", err)
	}

	// 5. Insert probe observations
	insertObsSQL := `
		INSERT INTO probe_observations (
			id, probe_run_id, node_logical_id, kind, verdict,
			evidence_digest, observed_at, latency_ms, redacted_summary, connection_revision, evidence_data
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	// node-hk: baseline (120ms), geo (HK), streaming (netflix: available, openai: restricted)
	tPast := baseTime.Add(-10 * time.Minute).Format(time.RFC3339)
	rev1 := int64(1)
	if _, err := db.ExecContext(ctx, insertObsSQL,
		"obs-hk-baseline", runID, "node-hk", string(domain.ProbeKindBaseline), string(domain.VerdictAvailable),
		"dig-hk-base", tPast, 120, "baseline ok", rev1, "",
	); err != nil {
		t.Fatalf("insert baseline obs failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, insertObsSQL,
		"obs-hk-geo", runID, "node-hk", string(domain.ProbeKindGeo), string(domain.VerdictAvailable),
		"dig-hk-geo", tPast, 150, "geo HK", rev1, `{"region":"HK"}`,
	); err != nil {
		t.Fatalf("insert geo obs failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, insertObsSQL,
		"obs-hk-stream", runID, "node-hk", string(domain.ProbeKindStreaming), string(domain.VerdictAvailable),
		"dig-hk-stream", tPast, 200, "streaming ok", rev1,
		`{"platforms":{"netflix":{"verdict":"available"},"openai":{"verdict":"restricted"}}}`,
	); err != nil {
		t.Fatalf("insert streaming obs failed: %v", err)
	}

	// node-future: geo observation with timestamp in the future (baseTime + 1h)
	tFuture := baseTime.Add(1 * time.Hour).Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, insertObsSQL,
		"obs-future-geo", runID, "node-future", string(domain.ProbeKindGeo), string(domain.VerdictAvailable),
		"dig-fut-geo", tFuture, 150, "future geo", rev1, `{"region":"HK"}`,
	); err != nil {
		t.Fatalf("insert future obs failed: %v", err)
	}

	// node-wrong-rev: node connection_revision is 2, observation connection_revision is 1
	if _, err := db.ExecContext(ctx, insertObsSQL,
		"obs-wrongrev-geo", runID, "node-wrong-rev", string(domain.ProbeKindGeo), string(domain.VerdictAvailable),
		"dig-wrongrev-geo", tPast, 150, "wrong rev geo", rev1, `{"region":"HK"}`,
	); err != nil {
		t.Fatalf("insert wrong rev obs failed: %v", err)
	}

	// node-shadowed: old success (tPast), newer failure (tRecent)
	tOld := baseTime.Add(-2 * time.Hour).Format(time.RFC3339)
	tRecent := baseTime.Add(-5 * time.Minute).Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, insertObsSQL,
		"obs-shad-old", runID, "node-shadowed", string(domain.ProbeKindGeo), string(domain.VerdictAvailable),
		"dig-shad-old", tOld, 150, "old geo success", rev1, `{"region":"HK"}`,
	); err != nil {
		t.Fatalf("insert shadowed old obs failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, insertObsSQL,
		"obs-shad-new", runID, "node-shadowed", string(domain.ProbeKindGeo), string(domain.VerdictError),
		"dig-shad-new", tRecent, 0, "recent geo error", rev1, `{"reason":"timeout"}`,
	); err != nil {
		t.Fatalf("insert shadowed new obs failed: %v", err)
	}

	// node-malformed-json: geo observation with invalid JSON in evidence_data
	if _, err := db.ExecContext(ctx, insertObsSQL,
		"obs-malformed-geo", runID, "node-malformed-json", string(domain.ProbeKindGeo), string(domain.VerdictAvailable),
		"dig-malformed-geo", tPast, 150, "malformed geo", rev1, `{"region": malformed...`,
	); err != nil {
		t.Fatalf("insert malformed obs failed: %v", err)
	}

	// 6. Load consistent facts through LoadConsistentFacts
	facts, err := loader.LoadConsistentFacts(ctx, revID, domain.TargetMihomo, asOf)
	if err != nil {
		t.Fatalf("LoadConsistentFacts failed: %v", err)
	}

	// Map loaded nodes by ID for test convenience
	nodesByID := make(map[string]domain.Node)
	for _, n := range facts.Nodes {
		nodesByID[n.LogicalID] = n
	}

	// Define filter specs for domain evaluation
	filterExitRegionHK := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: domain.FilterFieldExitRegion, Op: domain.FilterOpEquals, Value: "HK"},
		},
	}
	filterExitRegionUS := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: domain.FilterFieldExitRegion, Op: domain.FilterOpEquals, Value: "US"},
		},
	}
	filterExitRegionNotEqualsUS := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: domain.FilterFieldExitRegion, Op: domain.FilterOpNotEquals, Value: "US"},
		},
	}

	streamingKind := domain.ProbeKindStreaming
	filterPlatformNetflixAvailable := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "available",
				ProbeKind: &streamingKind,
				Platform:  "netflix",
			},
		},
	}
	filterPlatformOpenAIRestricted := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "restricted",
				ProbeKind: &streamingKind,
				Platform:  "openai",
			},
		},
	}
	filterPlatformOpenAIAvailable := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpEquals,
				Value:     "available",
				ProbeKind: &streamingKind,
				Platform:  "openai",
			},
		},
	}
	filterPlatformNetflixNotEqualsRestricted := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{
				Field:     domain.FilterFieldPlatformVerdict,
				Op:        domain.FilterOpNotEquals,
				Value:     "restricted",
				ProbeKind: &streamingKind,
				Platform:  "netflix",
			},
		},
	}

	// Test 1: node-hk Geo HK matches, Platform available/restricted matches, baseline & latency preserved
	t.Run("node-hk evidence synced and matches domain filters", func(t *testing.T) {
		nodeHK := nodesByID["node-hk"]
		obsMap := facts.LatestObservations["node-hk"]
		if obsMap == nil {
			t.Fatal("expected observations for node-hk, got nil")
		}

		// Verify structured fields unmarshalled from EvidenceData
		geoObs := obsMap[domain.ProbeKindGeo]
		if geoObs.Region != "HK" {
			t.Errorf("expected geoObs.Region to be 'HK', got %q", geoObs.Region)
		}

		streamObs := obsMap[domain.ProbeKindStreaming]
		if streamObs.Platforms == nil {
			t.Fatal("expected streamObs.Platforms to be populated, got nil")
		}
		if netflixCap, ok := streamObs.Platforms["netflix"]; !ok || netflixCap.Verdict != domain.VerdictAvailable {
			t.Errorf("expected netflix platform verdict available, got %+v", netflixCap)
		}
		if openaiCap, ok := streamObs.Platforms["openai"]; !ok || openaiCap.Verdict != domain.VerdictRestricted {
			t.Errorf("expected openai platform verdict restricted, got %+v", openaiCap)
		}

		// Baseline observation checks: latency and healthy status preserved
		baseObs := obsMap[domain.ProbeKindBaseline]
		if baseObs.LatencyMS != 120 {
			t.Errorf("expected baseline latency 120ms, got %d", baseObs.LatencyMS)
		}
		health := domain.ClassifyNodeHealth(obsMap, nodeHK.ConnectionRevision, asOf, 0)
		if health != domain.HealthCategoryHealthy {
			t.Errorf("expected health healthy, got %s", health)
		}

		// Domain filter matching: exit_region == HK -> true
		matched, reason := domain.MatchesFilter(&filterExitRegionHK, nodeHK, nil, obsMap, asOf)
		if !matched {
			t.Errorf("expected node-hk to match exit_region=HK, failed: %s", reason)
		}

		// Domain filter matching: exit_region == US -> false
		matched, _ = domain.MatchesFilter(&filterExitRegionUS, nodeHK, nil, obsMap, asOf)
		if matched {
			t.Errorf("expected node-hk not to match exit_region=US")
		}

		// Domain filter matching: platform netflix available -> true
		matched, reason = domain.MatchesFilter(&filterPlatformNetflixAvailable, nodeHK, nil, obsMap, asOf)
		if !matched {
			t.Errorf("expected node-hk to match netflix=available, failed: %s", reason)
		}

		// Domain filter matching: platform openai restricted -> true
		matched, reason = domain.MatchesFilter(&filterPlatformOpenAIRestricted, nodeHK, nil, obsMap, asOf)
		if !matched {
			t.Errorf("expected node-hk to match openai=restricted, failed: %s", reason)
		}

		// Domain filter matching: platform openai available -> false
		matched, _ = domain.MatchesFilter(&filterPlatformOpenAIAvailable, nodeHK, nil, obsMap, asOf)
		if matched {
			t.Errorf("expected node-hk not to match openai=available")
		}
	})

	// Test 2: Future observation rejected
	t.Run("future observation does not match", func(t *testing.T) {
		nodeFuture := nodesByID["node-future"]
		obsMap := facts.LatestObservations["node-future"]
		matched, _ := domain.MatchesFilter(&filterExitRegionHK, nodeFuture, nil, obsMap, asOf)
		if matched {
			t.Errorf("expected future observation NOT to match exit_region=HK at asOf")
		}
	})

	// Test 3: Wrong revision does not match
	t.Run("wrong revision does not match", func(t *testing.T) {
		nodeWrongRev := nodesByID["node-wrong-rev"]
		obsMap := facts.LatestObservations["node-wrong-rev"]
		matched, _ := domain.MatchesFilter(&filterExitRegionHK, nodeWrongRev, nil, obsMap, asOf)
		if matched {
			t.Errorf("expected wrong revision observation NOT to match exit_region=HK")
		}
	})

	// Test 4: Latest failure shadows older success
	t.Run("latest failure shadows older success", func(t *testing.T) {
		nodeShadowed := nodesByID["node-shadowed"]
		obsMap := facts.LatestObservations["node-shadowed"]
		geoObs := obsMap[domain.ProbeKindGeo]
		if geoObs.Verdict != domain.VerdictError {
			t.Errorf("expected latest loaded obs verdict to be 'error', got %s", geoObs.Verdict)
		}
		matched, _ := domain.MatchesFilter(&filterExitRegionHK, nodeShadowed, nil, obsMap, asOf)
		if matched {
			t.Errorf("expected shadowed node NOT to match exit_region=HK")
		}
	})

	// Test 5: Malformed JSON in evidence_data fails closed (unknown/empty region, does not match)
	t.Run("malformed json fails closed", func(t *testing.T) {
		nodeMalformed := nodesByID["node-malformed-json"]
		obsMap := facts.LatestObservations["node-malformed-json"]
		geoObs := obsMap[domain.ProbeKindGeo]
		if geoObs.Region != "" {
			t.Errorf("expected malformed json to result in empty Region, got %q", geoObs.Region)
		}
		matched, _ := domain.MatchesFilter(&filterExitRegionHK, nodeMalformed, nil, obsMap, asOf)
		if matched {
			t.Errorf("expected malformed json observation NOT to match exit_region=HK")
		}
	})

	// Test 6: Missing evidence does not match even with not_equals
	t.Run("missing observations fail closed on not_equals", func(t *testing.T) {
		nodeMissing := nodesByID["node-missing-obs"]
		obsMap := facts.LatestObservations["node-missing-obs"] // may be nil or empty map

		// Missing geo: not_equals US should still be false
		matched, _ := domain.MatchesFilter(&filterExitRegionNotEqualsUS, nodeMissing, nil, obsMap, asOf)
		if matched {
			t.Errorf("expected missing geo observation NOT to match exit_region != US (fail closed)")
		}

		// Missing streaming: not_equals restricted should still be false
		matched, _ = domain.MatchesFilter(&filterPlatformNetflixNotEqualsRestricted, nodeMissing, nil, obsMap, asOf)
		if matched {
			t.Errorf("expected missing platform observation NOT to match netflix != restricted (fail closed)")
		}
	})
}
