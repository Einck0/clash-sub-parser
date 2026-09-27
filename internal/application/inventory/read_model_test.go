package inventory_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestInventoryService_ReadModelWithRisk(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	ctx := context.Background()
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, nil)

	// 1. Create a parent subscription and node
	sub := &domain.Subscription{
		ID:                 "sub-readmodel-01",
		Name:               "ReadModel Test Sub",
		SourceURLSecretRef: "secret://sub/test",
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds:  86400,
			TimeoutSeconds:   30,
			MaxResponseBytes: 10485760,
		},
		Revision:  "rev-1",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("create sub: %v", err)
	}

	nodeID1 := "node_0000000000000001"
	nodeID2 := "node_0000000000000002"

	nodes := []domain.Node{
		{
			LogicalID:   nodeID1,
			Protocol:    domain.ProtocolSS,
			DisplayName: "Node-Allow",
			Server:      "203.0.113.10",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "pw1"},
			Active:      true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		{
			LogicalID:   nodeID2,
			Protocol:    domain.ProtocolVMess,
			DisplayName: "Node-Block",
			Server:      "203.0.113.20",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{UUID: "00000000-0000-0000-0000-000000000002"},
			Active:      true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	}
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatalf("upsert batch: %v", err)
	}

	// 2. Setup provider settings
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	if err := providerRepo.Upsert(ctx, &domain.IPRiskProviderSettings{
		Provider:           "scamalytics",
		SchemaVersion:      "v1",
		Enabled:            true,
		SecretReference:    "secret://iprisk/key",
		MaxConcurrency:     5,
		RequestsPerMinute:  60,
		DailyRequestBudget: 1000,
		PerRequestTimeout:  time.Second,
		MaxResponseBytes:   1024,
	}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}

	// 3. Setup risk policy
	policyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	policyRevID := domain.MustNewUUIDv7()
	if err := policyRepo.Create(ctx, &domain.RiskPolicyRevision{
		RiskPolicy: domain.RiskPolicy{
			RevisionID: policyRevID,
			ProviderSelection: domain.RiskProviderSelection{
				Mode:      domain.RiskFusionSingleProvider,
				Providers: []domain.ProviderRef{{Provider: "scamalytics", SchemaVersion: "v1"}},
			},
			MaxObservationAge: 24 * time.Hour,
			MinimumConfidence: 50,
			ScoreBands: []domain.ScoreBand{
				{Min: 0, Max: 49, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
				{Min: 50, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
			},
			UnknownAction: domain.RiskActionReview,
		},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if err := policyRepo.SetActive(ctx, policyRevID, true); err != nil {
		t.Fatalf("set active policy: %v", err)
	}

	// 4. Create observations for nodeID1 (score 10 -> allow) and nodeID2 (score 90 -> block)
	obsRepo := sqlite.NewIPRiskObservationRepository(db)
	score1, score2 := 10, 90
	conf := 90
	now := time.Now().UTC()

	if err := obsRepo.Create(ctx, &domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         nodeID1,
		ExitIdentityDigest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Provider:              "scamalytics",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score1,
		Confidence:            &conf,
		NetworkClass:          domain.NetworkClassResidential,
		AnonymizerTraits:      []domain.AnonymizerTrait{},
		EvidenceDigest:        "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		RedactedSummary:       "safe residential score 10",
	}); err != nil {
		t.Fatalf("create obs 1: %v", err)
	}

	if err := obsRepo.Create(ctx, &domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         nodeID2,
		ExitIdentityDigest:    "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Provider:              "scamalytics",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score2,
		Confidence:            &conf,
		NetworkClass:          domain.NetworkClassDatacenter,
		AnonymizerTraits:      []domain.AnonymizerTrait{domain.TraitTor},
		EvidenceDigest:        "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		RedactedSummary:       "safe datacenter tor score 90",
	}); err != nil {
		t.Fatalf("create obs 2: %v", err)
	}

	t.Run("list_nodes_read_model_without_filters", func(t *testing.T) {
		views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 50},
		})
		if err != nil {
			t.Fatalf("ListNodesReadModel failed: %v", err)
		}
		if total != 2 || len(views) != 2 {
			t.Fatalf("expected 2 views, got %d, total %d", len(views), total)
		}

		for _, v := range views {
			if v.IPRiskSummary == nil {
				t.Fatalf("expected IPRiskSummary for node %s, got nil", v.LogicalID)
			}
			if err := v.IPRiskSummary.Validate(); err != nil {
				t.Errorf("summary validation failed for node %s: %v", v.LogicalID, err)
			}
		}
	})

	t.Run("filter_by_decision_block", func(t *testing.T) {
		views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 50},
			RiskDecisions: []domain.RiskAction{domain.RiskActionBlock},
		})
		if err != nil {
			t.Fatalf("ListNodesReadModel failed: %v", err)
		}
		if total != 1 || len(views) != 1 {
			t.Fatalf("expected 1 view, got %d, total %d", len(views), total)
		}
		if views[0].LogicalID != nodeID2 {
			t.Errorf("expected %s, got %s", nodeID2, views[0].LogicalID)
		}
		if views[0].IPRiskSummary.Decision != domain.RiskActionBlock {
			t.Errorf("expected block decision, got %s", views[0].IPRiskSummary.Decision)
		}
	})

	t.Run("get_node_detail_with_risk", func(t *testing.T) {
		detail, err := svc.GetNodeDetailWithRisk(ctx, nodeID1, "")
		if err != nil {
			t.Fatalf("GetNodeDetailWithRisk failed: %v", err)
		}
		if detail.Node.LogicalID != nodeID1 {
			t.Errorf("expected logical_id %s, got %s", nodeID1, detail.Node.LogicalID)
		}
		if detail.IPRiskSummary == nil {
			t.Fatalf("expected IPRiskSummary, got nil")
		}
		if detail.IPRiskSummary.Decision != domain.RiskActionAllow {
			t.Errorf("expected allow decision, got %s", detail.IPRiskSummary.Decision)
		}
	})

	t.Run("zero_sensitive_data_in_api_view", func(t *testing.T) {
		views, _, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 50},
		})
		if err != nil {
			t.Fatalf("ListNodesReadModel failed: %v", err)
		}

		raw, err := json.Marshal(views)
		if err != nil {
			t.Fatalf("marshal views: %v", err)
		}
		jsonStr := string(raw)

		for _, forbidden := range []string{"normalized_config_secret_ref", "secret://", "198.51.100."} {
			if strings.Contains(jsonStr, forbidden) {
				t.Fatalf("JSON contains forbidden substring %q: %s", forbidden, jsonStr)
			}
		}
	})
}

func TestInventoryService_ReadModelWithProbeObservationsAndSources(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	ctx := context.Background()
	probeObsRepo := sqlite.NewProbeObservationRepository(db)
	probeRunRepo := sqlite.NewProbeRunRepository(db)
	svc := inventory.NewService(
		db,
		subRepo,
		fetchRepo,
		nodeRepo,
		sourceRepo,
		nil,
		inventory.WithProbeObservationRepository(probeObsRepo),
	)

	now := time.Now().UTC().Truncate(time.Second)
	sub := &domain.Subscription{
		ID:                 "sub-probe-01",
		Name:               "Probe ReadModel Sub",
		SourceURLSecretRef: "secret://sub/probe",
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds:  86400,
			TimeoutSeconds:   30,
			MaxResponseBytes: 10485760,
		},
		Revision:  "rev-1",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("create sub: %v", err)
	}

	fetchRec := &domain.SubscriptionFetch{
		ID:             "fetch-probe-01",
		SubscriptionID: sub.ID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		NodesParsed:    3,
		NodesValid:     3,
	}
	if err := fetchRepo.Create(ctx, fetchRec); err != nil {
		t.Fatalf("create fetch: %v", err)
	}

	nodeHealthy := "node_probe_healthy_01"
	nodeStale := "node_probe_stale_02"
	nodeMissing := "node_probe_missing_03"

	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{
		{
			LogicalID:   nodeHealthy,
			Protocol:    domain.ProtocolSS,
			DisplayName: "HK Healthy",
			Server:      "203.0.113.11",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "pw"},
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			LogicalID:   nodeStale,
			Protocol:    domain.ProtocolVMess,
			DisplayName: "US Stale Restricted",
			Server:      "203.0.113.12",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{UUID: "00000000-0000-0000-0000-000000000012"},
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			LogicalID:   nodeMissing,
			Protocol:    domain.ProtocolTrojan,
			DisplayName: "JP Unprobed",
			Server:      "203.0.113.13",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{Password: "pw3"},
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}); err != nil {
		t.Fatalf("upsert nodes: %v", err)
	}

	if err := sourceRepo.Upsert(ctx, &domain.NodeSource{
		NodeLogicalID:   nodeHealthy,
		SubscriptionID:  sub.ID,
		LastSeenFetchID: fetchRec.ID,
	}); err != nil {
		t.Fatalf("upsert source: %v", err)
	}

	run := &domain.ProbeRun{
		ID:             domain.MustNewUUIDv7(),
		IdempotencyKey: "probe-rm-run-1",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     now.Add(10 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := probeRunRepo.Create(ctx, run); err != nil {
		t.Fatalf("create probe run: %v", err)
	}

	// Healthy node: baseline 42ms available + streaming 85ms available (fresh)
	for _, obs := range []*domain.ProbeObservation{
		{
			ID:              domain.MustNewUUIDv7(),
			ProbeRunID:      run.ID,
			NodeLogicalID:   nodeHealthy,
			Kind:            domain.ProbeKindBaseline,
			Verdict:         domain.VerdictAvailable,
			EvidenceDigest:  domain.ComputeProbeEvidenceDigest(run.ID, nodeHealthy, "baseline-v1", domain.VerdictAvailable, 204, "contract_matched"),
			ObservedAt:      now.Add(-5 * time.Minute),
			LatencyMS:       42,
			RedactedSummary: "profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=42",
		},
		{
			ID:              domain.MustNewUUIDv7(),
			ProbeRunID:      run.ID,
			NodeLogicalID:   nodeHealthy,
			Kind:            domain.ProbeKindStreaming,
			Verdict:         domain.VerdictAvailable,
			EvidenceDigest:  domain.ComputeProbeEvidenceDigest(run.ID, nodeHealthy, "streaming-v1", domain.VerdictAvailable, 200, "contract_matched"),
			ObservedAt:      now.Add(-4 * time.Minute),
			LatencyMS:       85,
			RedactedSummary: "profile=streaming version=streaming-v1 verdict=available reason=contract_matched status=200 latency_ms=85",
		},
		// Stale node: restricted observation 2 hours ago without baseline
		{
			ID:              domain.MustNewUUIDv7(),
			ProbeRunID:      run.ID,
			NodeLogicalID:   nodeStale,
			Kind:            domain.ProbeKindAI,
			Verdict:         domain.VerdictRestricted,
			EvidenceDigest:  domain.ComputeProbeEvidenceDigest(run.ID, nodeStale, "ai-v1", domain.VerdictRestricted, 403, "access_restricted"),
			ObservedAt:      now.Add(-2 * time.Hour),
			LatencyMS:       210,
			RedactedSummary: "profile=ai version=ai-v1 verdict=restricted reason=access_restricted status=403 latency_ms=210",
		},
	} {
		if err := probeObsRepo.Create(ctx, obs); err != nil {
			t.Fatalf("create probe obs: %v", err)
		}
	}

	views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 50},
	})
	if err != nil {
		t.Fatalf("ListNodesReadModel: %v", err)
	}
	if total != 3 || len(views) != 3 {
		t.Fatalf("expected 3 views, got len=%d total=%d", len(views), total)
	}

	byID := make(map[string]inventory.NodeView, len(views))
	for _, v := range views {
		byID[v.LogicalID] = v
	}

	// Verify nodeHealthy
	vh := byID[nodeHealthy]
	if vh.LatencyMS == nil || *vh.LatencyMS != 42 {
		t.Fatalf("expected nodeHealthy latency_ms=42 (from baseline), got %v", vh.LatencyMS)
	}
	if vh.HealthStatus != "healthy" || vh.ProbeMissing || vh.ProbeStale {
		t.Fatalf("unexpected nodeHealthy status: health=%s missing=%v stale=%v", vh.HealthStatus, vh.ProbeMissing, vh.ProbeStale)
	}
	if len(vh.Sources) != 1 || vh.Sources[0].SubscriptionID != sub.ID {
		t.Fatalf("expected nodeHealthy sources=[%s], got %+v", sub.ID, vh.Sources)
	}
	if vh.Capabilities["baseline"].Verdict != domain.VerdictAvailable || vh.Capabilities["streaming"].Verdict != domain.VerdictAvailable {
		t.Fatalf("unexpected nodeHealthy capabilities: %+v", vh.Capabilities)
	}

	// Verify nodeStale (no baseline, AI restricted 2h ago)
	vs := byID[nodeStale]
	if vs.LatencyMS == nil || *vs.LatencyMS != 210 {
		t.Fatalf("expected nodeStale fallback latency_ms=210, got %v", vs.LatencyMS)
	}
	if vs.HealthStatus != "degraded" || vs.ProbeMissing || !vs.ProbeStale {
		t.Fatalf("expected nodeStale health=degraded missing=false stale=true, got health=%s missing=%v stale=%v", vs.HealthStatus, vs.ProbeMissing, vs.ProbeStale)
	}
	if !vs.Capabilities["ai"].Stale {
		t.Fatalf("expected nodeStale ai capability stale=true, got %+v", vs.Capabilities["ai"])
	}

	// Verify nodeMissing
	vm := byID[nodeMissing]
	if vm.LatencyMS != nil || vm.LastProbedAt != nil || vm.HealthStatus != "unknown" || !vm.ProbeMissing || vm.ProbeStale {
		t.Fatalf("unexpected nodeMissing view: %+v", vm)
	}

	// Verify GetNodeDetailWithRisk also enriches NodeView
	detail, err := svc.GetNodeDetailWithRisk(ctx, nodeHealthy, "")
	if err != nil {
		t.Fatalf("GetNodeDetailWithRisk: %v", err)
	}
	dv := detail.ToNodeView()
	if dv.LatencyMS == nil || *dv.LatencyMS != 42 || dv.HealthStatus != "healthy" || len(dv.Sources) != 1 {
		t.Fatalf("unexpected detail NodeView: %+v", dv)
	}
}

type mockPoolStateProvider struct {
	states map[string]string
}

func (m mockPoolStateProvider) GetNodePoolState(logicalID string) string {
	if st, ok := m.states[logicalID]; ok {
		return st
	}
	return "idle"
}

func TestInventoryService_NodeViewProbeStateProbingAndQueued(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	ctx := context.Background()

	now := domain.NowUTC()
	n1 := "node_probing_01"
	n2 := "node_queued_02"
	n3 := "node_idle_03"
	for _, id := range []string{n1, n2, n3} {
		if err := nodeRepo.UpsertBatch(ctx, []domain.Node{{
			LogicalID:   id,
			Protocol:    domain.ProtocolVMess,
			DisplayName: id,
			Server:      "198.51.100.1",
			Port:        443,
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}}); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}

	probeRunRepo := sqlite.NewProbeRunRepository(db)
	probeObsRepo := sqlite.NewProbeObservationRepository(db)
	runID := domain.MustNewUUIDv7()
	_ = probeRunRepo.Create(ctx, &domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: "pool-state-run",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     now.Add(10 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	_ = probeObsRepo.Create(ctx, &domain.ProbeObservation{
		ID:            domain.MustNewUUIDv7(),
		ProbeRunID:    runID,
		NodeLogicalID: n1,
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictAvailable,
		EvidenceDigest: domain.ComputeProbeEvidenceDigest(
			runID, n1, "v1", domain.VerdictAvailable, 204, "ok",
		),
		ObservedAt:      now,
		LatencyMS:       55,
		RedactedSummary: "ok",
	})

	provider := mockPoolStateProvider{
		states: map[string]string{
			n1: "probing",
			n2: "queued",
			n3: "idle",
		},
	}

	svc := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, sourceRepo, nil,
		inventory.WithProbeObservationRepository(probeObsRepo),
		inventory.WithNodePoolStateProvider(provider),
	)

	views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 20},
	})
	if err != nil || total != 3 {
		t.Fatalf("ListNodesReadModel: total=%d err=%v", total, err)
	}

	byID := make(map[string]inventory.NodeView, len(views))
	for _, v := range views {
		byID[v.LogicalID] = v
	}

	if byID[n1].ProbeState != "probing" || byID[n1].HealthStatus != "probing" {
		t.Fatalf("expected %s probe_state=probing and health_status=probing, got %+v", n1, byID[n1])
	}
	if byID[n1].LatencyMS == nil || *byID[n1].LatencyMS != 55 {
		t.Fatalf("expected %s historical latency_ms=55 preserved while probing, got %v", n1, byID[n1].LatencyMS)
	}
	if byID[n2].ProbeState != "queued" {
		t.Fatalf("expected %s probe_state=queued, got %+v", n2, byID[n2])
	}
	if byID[n3].ProbeState != "idle" {
		t.Fatalf("expected %s probe_state=idle, got %+v", n3, byID[n3])
	}

	detail, err := svc.GetNodeDetailWithRisk(ctx, n1, "")
	if err != nil {
		t.Fatalf("GetNodeDetailWithRisk: %v", err)
	}
	if detail.ProbeState != "probing" || detail.HealthStatus != "probing" || detail.ToNodeView().ProbeState != "probing" {
		t.Fatalf("expected detail probe_state=probing and health_status=probing, got %+v", detail)
	}

	// Verify failed/unhealthy observation (verdict=error, latency_ms=0 or >0) never populates valid LatencyMS
	_ = probeObsRepo.Create(ctx, &domain.ProbeObservation{
		ID:            domain.MustNewUUIDv7(),
		ProbeRunID:    runID,
		NodeLogicalID: n3,
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictError,
		EvidenceDigest: domain.ComputeProbeEvidenceDigest(
			runID, n3, "v1", domain.VerdictError, 0, "dial_timeout",
		),
		ObservedAt:      now,
		LatencyMS:       0,
		RedactedSummary: "profile=baseline version=v1 verdict=error reason=dial_timeout status=0 latency_ms=0",
	})
	detailFailed, err := svc.GetNodeDetailWithRisk(ctx, n3, "")
	if err != nil {
		t.Fatalf("GetNodeDetailWithRisk(%s): %v", n3, err)
	}
	if detailFailed.HealthStatus != "unhealthy" || detailFailed.LatencyMS != nil {
		t.Fatalf("expected unhealthy node %s to have nil LatencyMS, got health=%s latency=%v", n3, detailFailed.HealthStatus, detailFailed.LatencyMS)
	}
}
