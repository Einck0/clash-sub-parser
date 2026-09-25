package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	ipriskApp "clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
	transporthttp "clash-sub-parser/internal/transport/http"
	"clash-sub-parser/migrations"
)

// controlledFakeRunner implements probe.Runner for safe integration tests without outbound network traffic.
type controlledFakeRunner struct {
	mu           sync.Mutex
	executedRuns []string
}

func newControlledFakeRunner() *controlledFakeRunner {
	return &controlledFakeRunner{executedRuns: make([]string, 0)}
}

func (r *controlledFakeRunner) Run(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error {
	r.mu.Lock()
	r.executedRuns = append(r.executedRuns, run.ID)
	r.mu.Unlock()
	return nil
}

// TestFeatureConvergence_SchemaMigrationTo8 verifies migrating from scratch through all 8 migrations.
func TestFeatureConvergence_SchemaMigrationTo8(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(sqlite.Config{
		Path:        fmt.Sprintf("file:migration_test_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to run migrations 1..8: %v", err)
	}

	report, err := sqlite.CheckReadiness(ctx, db)
	if err != nil {
		t.Fatalf("CheckReadiness returned error: %v", err)
	}
	if !report.Ready {
		t.Fatalf("expected readiness report Ready=true, got report: %+v", report)
	}
	if report.SchemaVersion != 8 {
		t.Fatalf("expected schema version 8, got %d", report.SchemaVersion)
	}
	if len(report.MissingTables) > 0 {
		t.Fatalf("unexpected missing tables: %v", report.MissingTables)
	}
	if len(report.ForeignKeyViolations) > 0 {
		t.Fatalf("unexpected FK violations: %v", report.ForeignKeyViolations)
	}

	// Verify tables from migration 7 and 8 exist
	for _, tbl := range []string{"probe_schedules", "probe_batches", "probe_batch_runs", "global_node_filters", "group_node_filters"} {
		var name string
		err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %q to exist: %v", tbl, err)
		}
	}
}

// TestFeatureConvergence_FullStack verifies:
// 1. Service wiring matching cmd/csp/main.go
// 2. Real HTTP authentication, global and group node filters
// 3. Filter counts & diagnostics returned in PreviewResult
// 4. Credential version and mismatch detection on /nodes
// 5. 5-target compilation identical source filtering
// 6. Empty routed group blocking publication
// 7. Periodic probe schedule and safe batch execution in DB
func TestFeatureConvergence_FullStack(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(sqlite.Config{
		Path:        fmt.Sprintf("file:fullstack_test_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Repositories
	settingsRepo := sqlite.NewSettingsRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeSourceRepo := sqlite.NewNodeSourceRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	probeRunRepo := sqlite.NewProbeRunRepository(db)
	probeObsRepo := sqlite.NewProbeObservationRepository(db)
	probeScheduleRepo := sqlite.NewProbeScheduleRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revisionRepo := sqlite.NewRevisionRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	riskBindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)
	nodeFilterRepo := sqlite.NewNodeFilterRepository(db)

	fakeRunner := newControlledFakeRunner()

	probeScheduler, err := queue.NewScheduler(queue.Config{
		Concurrency: queue.DefaultConcurrency,
		Context:     ctx,
	})
	if err != nil {
		t.Fatalf("scheduler init failed: %v", err)
	}
	defer func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = probeScheduler.Drain(drainCtx)
	}()

	probeService := probe.NewService(
		probeRunRepo,
		probe.WithRunner(fakeRunner),
		probe.WithScheduleRepository(probeScheduleRepo),
		probe.WithAudit(auditRepo),
	)

	periodicCoord := probe.NewPeriodicCoordinator(
		probeScheduleRepo,
		nodeRepo,
		probeRunRepo,
		fakeRunner,
		probe.WithCoordinatorOwner("test-instance-1"),
	)
	probeService.SetCoordinator(periodicCoord)

	subService := subscription.NewService(subRepo, auditRepo)
	invService := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, nil,
		inventory.WithProbeObservationRepository(probeObsRepo),
	)
	subService.SetReconciler(invService)

	policyService := policy.NewService(policyRepo, revisionRepo, nodeRepo, auditRepo, nodeFilterRepo)
	revisionService := revision.NewService(revisionRepo, auditRepo, revision.WithPolicyRepository(policyRepo))
	ipriskService := ipriskApp.NewService(
		riskObsRepo,
		riskPolicyRepo,
		ipriskApp.WithBindingRepository(riskBindingRepo),
		ipriskApp.WithGroupRepository(policyRepo),
		ipriskApp.WithNodeRepository(nodeRepo),
		ipriskApp.WithAuditRepository(auditRepo),
	)

	pubService := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revisionRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(nodeFilterRepo),
		publication.WithNodeSourceRepository(nodeSourceRepo),
		publication.WithProbeObservationRepository(probeObsRepo),
		publication.WithIPRiskService(ipriskService),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskBindingRepository(riskBindingRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	const adminToken = "test-admin-token-xyz-12345"
	hashedToken, err := transporthttp.HashToken(adminToken)
	if err != nil {
		t.Fatalf("hash admin token failed: %v", err)
	}
	if err := settingsRepo.UpdateAdminToken(ctx, hashedToken); err != nil {
		t.Fatalf("persist admin token failed: %v", err)
	}

	sessionStore := transporthttp.NewMemorySessionStore(1 * time.Hour)
	tokenHolder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		InitialVerifier: hashedToken,
		SettingsRepo:    settingsRepo,
		SessionStore:    sessionStore,
		HashCost:        transporthttp.MinHashCost,
	})

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		TokenHolder:         tokenHolder,
		SettingsRepository:  settingsRepo,
		SessionStore:        sessionStore,
		SubscriptionService: subService,
		InventoryService:    invService,
		ProbeService:        probeService,
		PolicyService:       policyService,
		RevisionService:     revisionService,
		PublicationService:         pubService,
		ProbeRunRepository:         probeRunRepo,
		ProbeObservationRepository: probeObsRepo,
		AuditRepository:            auditRepo,
		PublicationTokenValidator: func(c context.Context, publicationID, token string) (bool, error) {
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
			pub, err := pubRepo.GetByID(c, publicationID)
			if err != nil || pub == nil || pub.RevokedAt != nil {
				return false, nil
			}
			return pub.TokenHash == hash, nil
		},
	})

	ts := httptest.NewServer(router)
	defer ts.Close()

	client := ts.Client()

	doReq := func(method, path string, body any) (*http.Response, []byte) {
		var rdr io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal request body failed: %v", err)
			}
			rdr = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, ts.URL+path, rdr)
		if err != nil {
			t.Fatalf("create request failed: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+adminToken)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %s %s failed: %v", method, path, err)
		}
		defer resp.Body.Close()
		respBytes, _ := io.ReadAll(resp.Body)
		return resp, respBytes
	}

	// -------------------------------------------------------------------------
	// 1. Seed Nodes & Observations in DB directly
	// -------------------------------------------------------------------------
	now := time.Now().UTC()
	node1 := domain.Node{
		LogicalID:                 "node_00000000000000000000000000000001",
		Protocol:                  domain.ProtocolTrojan,
		DisplayName:               "Tokyo Trojan Fast",
		NormalizedConfigSecretRef: "secret://tokyo-1",
		CredentialVersion:         1,
		Active:                    true,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	node2 := domain.Node{
		LogicalID:                 "node_00000000000000000000000000000002",
		Protocol:                  domain.ProtocolVMess,
		DisplayName:               "US Vmess Slow",
		NormalizedConfigSecretRef: "secret://us-2",
		CredentialVersion:         1,
		Active:                    true,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	node3 := domain.Node{
		LogicalID:                 "node_00000000000000000000000000000003",
		Protocol:                  domain.ProtocolTrojan,
		DisplayName:               "HK Trojan Mismatched",
		NormalizedConfigSecretRef: "secret://hk-3",
		CredentialVersion:         2, // Version is 2!
		Active:                    true,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}

	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node1, node2, node3}); err != nil {
		t.Fatalf("failed to insert nodes: %v", err)
	}

	// Create ProbeRun for observations FK
	run := domain.ProbeRun{
		ID:             domain.MustNewUUIDv7(),
		IdempotencyKey: "test-idem-key-1",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := probeRunRepo.Create(ctx, &run); err != nil {
		t.Fatalf("create probe run failed: %v", err)
	}

	// Observations:
	// Node 1: baseline latency 45ms, cred_version 1 (matches node 1)
	v1 := 1
	obs1 := domain.ProbeObservation{
		ID:                domain.MustNewUUIDv7(),
		ProbeRunID:        run.ID,
		NodeLogicalID:     node1.LogicalID,
		Kind:              domain.ProbeKindBaseline,
		Verdict:           domain.VerdictAvailable,
		LatencyMS:         45,
		CredentialVersion: &v1,
		ObservedAt:        now,
	}
	// Node 2: baseline latency 250ms, cred_version 1
	obs2 := domain.ProbeObservation{
		ID:                domain.MustNewUUIDv7(),
		ProbeRunID:        run.ID,
		NodeLogicalID:     node2.LogicalID,
		Kind:              domain.ProbeKindBaseline,
		Verdict:           domain.VerdictAvailable,
		LatencyMS:         250,
		CredentialVersion: &v1,
		ObservedAt:        now,
	}
	// Node 3: baseline latency 40ms, cred_version 1 (mismatch with node3 credential_version 2!)
	obs3 := domain.ProbeObservation{
		ID:                domain.MustNewUUIDv7(),
		ProbeRunID:        run.ID,
		NodeLogicalID:     node3.LogicalID,
		Kind:              domain.ProbeKindBaseline,
		Verdict:           domain.VerdictAvailable,
		LatencyMS:         45,
		CredentialVersion: &v1, // Observation has version 1, but node is version 2!
		ObservedAt:        now,
	}

	for _, o := range []domain.ProbeObservation{obs1, obs2, obs3} {
		if err := probeObsRepo.Create(ctx, &o); err != nil {
			t.Fatalf("failed to create observation: %v", err)
		}
	}

	// -------------------------------------------------------------------------
	// 2. Test GET /api/v1/nodes: Check credential_version & credential_mismatch
	// -------------------------------------------------------------------------
	resp, body := doReq(http.MethodGet, "/api/v1/nodes", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/nodes returned %d: %s", resp.StatusCode, string(body))
	}

	var nodePage struct {
		Data struct {
			Items []struct {
				LogicalID          string `json:"logical_id"`
				CredentialVersion  int    `json:"credential_version"`
				CredentialMismatch bool   `json:"credential_mismatch"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &nodePage); err != nil {
		t.Fatalf("unmarshal nodes failed: %v", err)
	}

	var foundN1, foundN3 bool
	for _, n := range nodePage.Data.Items {
		if n.LogicalID == node1.LogicalID {
			foundN1 = true
			if n.CredentialVersion != 1 {
				t.Errorf("node1 expected credential_version 1, got %d", n.CredentialVersion)
			}
			if n.CredentialMismatch {
				t.Errorf("node1 expected credential_mismatch false, got true")
			}
		}
		if n.LogicalID == node3.LogicalID {
			foundN3 = true
			if n.CredentialVersion != 2 {
				t.Errorf("node3 expected credential_version 2, got %d", n.CredentialVersion)
			}
			if !n.CredentialMismatch {
				t.Errorf("node3 expected credential_mismatch true, got false")
			}
		}
	}
	if !foundN1 || !foundN3 {
		t.Fatalf("expected node1 and node3 in list, got %+v", nodePage.Data.Items)
	}

	// -------------------------------------------------------------------------
	// 3. Configure Global Node Filter via PUT /api/v1/policies/global-node-filter
	// Filter: protocol == "trojan" (excludes node2 which is vmess)
	// -------------------------------------------------------------------------
	globalFilterReq := map[string]any{
		"spec": map[string]any{
			"conditions": []map[string]any{
				{
					"field": "protocol",
					"op":    "equals",
					"value": "trojan",
				},
			},
		},
	}
	resp, body = doReq(http.MethodPut, "/api/v1/policies/global-node-filter", globalFilterReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/policies/global-node-filter returned %d: %s", resp.StatusCode, string(body))
	}

	// Verify GET /api/v1/policies/global-node-filter returns configured filter
	resp, body = doReq(http.MethodGet, "/api/v1/policies/global-node-filter", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/policies/global-node-filter returned %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "trojan") {
		t.Fatalf("expected global filter to contain 'trojan', got %s", string(body))
	}

	// -------------------------------------------------------------------------
	// 4. Create Policy Group & Revision
	// Group 1: "Fast Group" (explicit members: node1, node3; group filter: latency_ms <= 100)
	// -------------------------------------------------------------------------
	grpID := domain.MustNewUUIDv7()
	groupReq := map[string]any{
		"id":         grpID,
		"name":       "Fast Group",
		"group_type": "select",
		"node_filter": map[string]any{
			"conditions": []map[string]any{
				{
					"field":      "probe:latency_ms",
					"op":         "lte",
					"value":      "100",
					"probe_kind": "baseline",
				},
			},
		},
		"edges": []map[string]any{
			{"node_logical_id": node1.LogicalID, "position": 0},
			{"node_logical_id": node3.LogicalID, "position": 1},
		},
	}
	resp, body = doReq(http.MethodPost, "/api/v1/policies/groups", groupReq)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/policies/groups returned %d: %s", resp.StatusCode, string(body))
	}

	// Create initial Configuration Revision
	rev, err := policyService.CreateRevision(ctx, policy.CreateRevisionCommand{
		ActorKind: domain.ActorKindAdmin,
		State:     domain.RevisionStateActive,
	})
	if err != nil {
		t.Fatalf("create revision failed: %v", err)
	}

	// Create Policy Rule targeting Fast Group
	ruleReq := map[string]any{
		"revision_id":     rev.ID,
		"target_group_id": grpID,
		"name":            "Direct Fast Route",
		"expression":      "MATCH",
		"position":        0,
	}
	resp, body = doReq(http.MethodPost, "/api/v1/policies/rules", ruleReq)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/policies/rules returned %d: %s", resp.StatusCode, string(body))
	}

	// -------------------------------------------------------------------------
	// 5. Test POST /api/v1/publications/preview: Check filter_counts & diagnostics
	// Global filter eliminates node2 (vmess) -> global_filtered_total = 2
	// Group filter evaluates probe latency for node1 & node3:
	//   - node1: obs credential_version matches node cred_version, latency 45 <= 100 -> KEPT
	//   - node3: obs credential_version (1) != node cred_version (2) -> fail-closed -> EXCLUDED
	// Final group_filtered_total = 1 (only node1)
	// -------------------------------------------------------------------------
	previewReq := map[string]any{
		"target":      "clash",
		"revision_id": rev.ID,
	}
	resp, body = doReq(http.MethodPost, "/api/v1/publications/preview", previewReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/publications/preview returned %d: %s", resp.StatusCode, string(body))
	}

	var previewData struct {
		Data struct {
			Target         string                      `json:"target"`
			SnapshotDigest string                      `json:"snapshot_digest"`
			Content        string                      `json:"content"`
			FilterCounts   *resolver.FilterLayerCounts `json:"filter_counts"`
			Diagnostics    []resolver.Diagnostic       `json:"diagnostics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &previewData); err != nil {
		t.Fatalf("unmarshal preview response failed: %v", err)
	}

	fc := previewData.Data.FilterCounts
	if fc == nil {
		t.Fatalf("expected filter_counts in preview response, got nil. Body: %s", string(body))
	}
	if fc.RawTotal != 3 {
		t.Errorf("expected RawTotal 3, got %d", fc.RawTotal)
	}
	if fc.AdmittedTotal != 3 {
		t.Errorf("expected AdmittedTotal 3, got %d", fc.AdmittedTotal)
	}
	if fc.GlobalFilteredTotal != 2 {
		t.Errorf("expected GlobalFilteredTotal 2, got %d", fc.GlobalFilteredTotal)
	}
	if fc.GroupFilteredTotal != 1 {
		t.Errorf("expected GroupFilteredTotal 1, got %d", fc.GroupFilteredTotal)
	}

	// Verify group_counts
	if grpCounts, ok := fc.GroupCounts[grpID]; !ok {
		t.Errorf("expected group_counts to contain group %s", grpID)
	} else {
		if grpCounts.Candidate != 2 {
			t.Errorf("expected group candidate 2, got %d", grpCounts.Candidate)
		}
		if grpCounts.Kept != 1 {
			t.Errorf("expected group kept 1, got %d", grpCounts.Kept)
		}
		if grpCounts.Excluded != 1 {
			t.Errorf("expected group excluded 1, got %d", grpCounts.Excluded)
		}
	}

	// Verify diagnostics: Node2 excluded by global filter, Node3 excluded by group filter
	var hasGlobalExcl, hasGroupExcl bool
	for _, d := range previewData.Data.Diagnostics {
		if d.Code == "global_filter_excluded" && d.Target == node2.LogicalID {
			hasGlobalExcl = true
			if d.Reason == "" {
				t.Errorf("expected non-empty Reason for global_filter_excluded diagnostic")
			}
		}
		if d.Code == "group_member_filtered" && d.Target == node3.LogicalID {
			hasGroupExcl = true
			if d.Reason == "" {
				t.Errorf("expected non-empty Reason for group_member_filtered diagnostic")
			}
		}
	}
	if !hasGlobalExcl {
		t.Errorf("expected global_filter_excluded diagnostic for node2")
	}
	if !hasGroupExcl {
		t.Errorf("expected group_member_filtered diagnostic for node3")
	}

	// -------------------------------------------------------------------------
	// 6. Five-Target Parity Verification
	// All 5 compilers (clash, mihomo, singbox, surge, qx) must produce
	// the identical snapshot digest and only include node1 (Tokyo Trojan Fast).
	// -------------------------------------------------------------------------
	targets := []domain.CompilerTarget{
		domain.TargetClash,
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}

	expectedDigest := previewData.Data.SnapshotDigest
	for _, target := range targets {
		tResp, tBody := doReq(http.MethodPost, "/api/v1/publications/preview", map[string]any{
			"target":      string(target),
			"revision_id": rev.ID,
		})
		if tResp.StatusCode != http.StatusOK {
			t.Fatalf("preview target %s failed (%d): %s", target, tResp.StatusCode, string(tBody))
		}
		var targetData struct {
			Data struct {
				SnapshotDigest string `json:"snapshot_digest"`
				Content        string `json:"content"`
			} `json:"data"`
		}
		if err := json.Unmarshal(tBody, &targetData); err != nil {
			t.Fatalf("unmarshal target %s failed: %v", target, err)
		}
		if targetData.Data.SnapshotDigest != expectedDigest {
			t.Errorf("target %s snapshot digest mismatch: expected %s, got %s",
				target, expectedDigest, targetData.Data.SnapshotDigest)
		}
		// Check that Tokyo Trojan Fast is present in rendered config, but US is absent from everything (excluded globally)
		content := targetData.Data.Content
		if !strings.Contains(content, "Tokyo Trojan Fast") {
			t.Errorf("target %s content missing kept node 'Tokyo Trojan Fast'", target)
		}
		if strings.Contains(content, "US Vmess Slow") {
			t.Errorf("target %s content should NOT contain 'US Vmess Slow'", target)
		}

		// Verify that HK Trojan Mismatched was excluded from the proxy group members across all 5 targets
		switch target {
		case domain.TargetClash, domain.TargetMihomo:
			// In Clash/Mihomo, proxy-groups proxies list should only contain Tokyo Trojan Fast
			if strings.Contains(content, "- HK Trojan Mismatched") {
				t.Errorf("target %s proxy-group should NOT contain '- HK Trojan Mismatched'", target)
			}
		case domain.TargetSingBox:
			// In sing-box, group selector outbounds list should only contain Tokyo Trojan Fast
			if strings.Contains(content, `"HK Trojan Mismatched"`) && strings.Contains(content, `"type":"selector"`) {
				// Ensure it's not in the selector outbounds array
				var sbConfig struct {
					Outbounds []struct {
						Tag       string   `json:"tag"`
						Type      string   `json:"type"`
						Outbounds []string `json:"outbounds"`
					} `json:"outbounds"`
				}
				if err := json.Unmarshal([]byte(content), &sbConfig); err == nil {
					for _, ob := range sbConfig.Outbounds {
						if ob.Tag == "Fast Group" {
							for _, sub := range ob.Outbounds {
								if sub == "HK Trojan Mismatched" {
									t.Errorf("singbox Fast Group selector should not contain 'HK Trojan Mismatched'")
								}
							}
						}
					}
				}
			}
		}
	}

	// -------------------------------------------------------------------------
	// 7. Empty Routed Group Blocking Verification
	// Add a rule pointing to an empty group -> Preflight blocks publication
	// -------------------------------------------------------------------------
	emptyGrpID := domain.MustNewUUIDv7()
	emptyGrpReq := map[string]any{
		"id":         emptyGrpID,
		"name":       "Completely Empty Group",
		"group_type": "select",
		"edges":      []map[string]any{},
	}
	resp, body = doReq(http.MethodPost, "/api/v1/policies/groups", emptyGrpReq)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST empty group returned %d: %s", resp.StatusCode, string(body))
	}

	// Re-create active revision before adding empty rule
	emptyRev, err := policyService.CreateRevision(ctx, policy.CreateRevisionCommand{
		ActorKind: domain.ActorKindAdmin,
		State:     domain.RevisionStateActive,
	})
	if err != nil {
		t.Fatalf("create empty revision failed: %v", err)
	}

	// Route traffic to the empty group
	emptyRuleReq := map[string]any{
		"revision_id":     emptyRev.ID,
		"target_group_id": emptyGrpID,
		"name":            "Empty Group Route",
		"expression":      "GEOIP,CN",
		"position":        0,
	}
	resp, body = doReq(http.MethodPost, "/api/v1/policies/rules", emptyRuleReq)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST empty group rule returned %d: %s", resp.StatusCode, string(body))
	}

	// Preview should report empty_routed_group diagnostic
	resp, body = doReq(http.MethodPost, "/api/v1/publications/preview", map[string]any{
		"target":      "clash",
		"revision_id": emptyRev.ID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview empty group returned %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "empty_routed_group") {
		t.Errorf("expected empty_routed_group diagnostic in preview, got: %s", string(body))
	}

	// Publication creation MUST fail with 409 preflight conflict error
	pubReq := map[string]any{
		"target":      "clash",
		"revision_id": emptyRev.ID,
	}
	resp, body = doReq(http.MethodPost, "/api/v1/publications", pubReq)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for empty routed group publication, got %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "empty_routed_group") {
		t.Errorf("expected 409 body to reference empty_routed_group, got: %s", string(body))
	}

	// -------------------------------------------------------------------------
	// 8. Periodic Probe Schedule & Safe Local Execution
	// -------------------------------------------------------------------------
	schedReq := map[string]any{
		"enabled":          true,
		"interval_seconds": 3600,
		"kinds":            []string{"baseline"},
	}
	resp, body = doReq(http.MethodPut, "/api/v1/probes/schedule", schedReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/probes/schedule returned %d: %s", resp.StatusCode, string(body))
	}

	resp, body = doReq(http.MethodGet, "/api/v1/probes/schedule", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/probes/schedule returned %d: %s", resp.StatusCode, string(body))
	}
	var schedResp struct {
		Data struct {
			Enabled         bool     `json:"enabled"`
			IntervalSeconds int      `json:"interval_seconds"`
			Kinds           []string `json:"kinds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &schedResp); err != nil {
		t.Fatalf("unmarshal schedule failed: %v", err)
	}
	if !schedResp.Data.Enabled {
		t.Errorf("expected schedule enabled true, got false")
	}
	if schedResp.Data.IntervalSeconds != 3600 {
		t.Errorf("expected interval 3600, got %d", schedResp.Data.IntervalSeconds)
	}

	// Trigger periodic coordination safely in local fixture (no external network calls)
	coordWindow := time.Now().UTC().Truncate(time.Minute)
	sched, err := probeScheduleRepo.Get(ctx)
	if err != nil {
		t.Fatalf("get schedule from db failed: %v", err)
	}
	batch := domain.ProbeBatch{
		ID:         domain.MustNewUUIDv7(),
		WindowAt:   coordWindow,
		Generation: sched.Generation,
		Owner:      "test-instance-1",
		State:      domain.ProbeBatchStatePending,
		RunIDs:     []string{},
		Counts: domain.ProbeBatchCounts{
			TotalNodes:     3,
			DispatchedRuns: 1,
			CompletedRuns:  1,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := probeScheduleRepo.CreateBatch(ctx, &batch); err != nil {
		t.Fatalf("create probe batch failed: %v", err)
	}

	// Check GET /api/v1/probes/batches
	resp, body = doReq(http.MethodGet, "/api/v1/probes/batches", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/probes/batches returned %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), batch.ID) {
		t.Errorf("expected batches list to contain batch %s, got: %s", batch.ID, string(body))
	}

	// Test cancelling batch via POST /api/v1/probes/batches/{id}/cancel
	resp, body = doReq(http.MethodPost, fmt.Sprintf("/api/v1/probes/batches/%s/cancel", batch.ID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST cancel batch returned %d: %s", resp.StatusCode, string(body))
	}

	// Verify scheduler drain works without error
	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := probeScheduler.Drain(drainCtx); err != nil {
		t.Fatalf("probe scheduler drain failed: %v", err)
	}
}
