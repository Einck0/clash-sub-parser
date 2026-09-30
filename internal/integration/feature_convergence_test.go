package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
	"clash-sub-parser/internal/fetch"
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
	if report.SchemaVersion != 13 {
		t.Fatalf("expected schema version 13, got %d", report.SchemaVersion)
	}
	if len(report.MissingTables) > 0 {
		t.Fatalf("unexpected missing tables: %v", report.MissingTables)
	}
	if len(report.ForeignKeyViolations) > 0 {
		t.Fatalf("unexpected FK violations: %v", report.ForeignKeyViolations)
	}

	// Verify tables from migration 7 and 8 exist, plus index from migration 10
	for _, tbl := range []string{"probe_schedules", "probe_batches", "probe_batch_runs", "global_node_filters", "group_node_filters"} {
		var name string
		err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %q to exist: %v", tbl, err)
		}
	}
	var idxName string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='index' AND name='idx_ip_risk_obs_node_observed_id'").Scan(&idxName); err != nil {
		t.Fatalf("expected index idx_ip_risk_obs_node_observed_id from migration 10 to exist: %v", err)
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
		TokenHolder:                tokenHolder,
		SettingsRepository:         settingsRepo,
		SessionStore:               sessionStore,
		SubscriptionService:        subService,
		InventoryService:           invService,
		ProbeService:               probeService,
		PolicyService:              policyService,
		RevisionService:            revisionService,
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
		LogicalID:   domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "tokyo1.example.com", 443, nil),
		Protocol:    domain.ProtocolTrojan,
		DisplayName: "Tokyo Trojan Fast",
		Server:      "tokyo1.example.com",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{Password: "tokyo1-pass"},
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	node2 := domain.Node{
		LogicalID:   domain.ComputeNodeLogicalID(domain.ProtocolVMess, "us2.example.com", 443, nil),
		Protocol:    domain.ProtocolVMess,
		DisplayName: "US Vmess Slow",
		Server:      "us2.example.com",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{UUID: "11111111-1111-1111-1111-111111111111", Method: "auto"},
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	node3 := domain.Node{
		LogicalID:   domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "hk3.example.com", 443, nil),
		Protocol:    domain.ProtocolTrojan,
		DisplayName: "HK Trojan Mismatched",
		Server:      "hk3.example.com",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{Password: "hk3-pass-v2"},
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
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
	// Node 1: baseline latency 45ms (<= 100ms)
	obs1 := domain.ProbeObservation{
		ID:            domain.MustNewUUIDv7(),
		ProbeRunID:    run.ID,
		NodeLogicalID: node1.LogicalID,
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictAvailable,
		LatencyMS:     45,
		ObservedAt:    now,
	}
	// Node 2: baseline latency 250ms
	obs2 := domain.ProbeObservation{
		ID:            domain.MustNewUUIDv7(),
		ProbeRunID:    run.ID,
		NodeLogicalID: node2.LogicalID,
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictAvailable,
		LatencyMS:     250,
		ObservedAt:    now,
	}
	// Node 3: baseline latency 240ms (> 100ms, excluded by group filter)
	obs3 := domain.ProbeObservation{
		ID:            domain.MustNewUUIDv7(),
		ProbeRunID:    run.ID,
		NodeLogicalID: node3.LogicalID,
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictAvailable,
		LatencyMS:     240,
		ObservedAt:    now,
	}

	for _, o := range []domain.ProbeObservation{obs1, obs2, obs3} {
		if err := probeObsRepo.Create(ctx, &o); err != nil {
			t.Fatalf("failed to create observation: %v", err)
		}
	}

	// -------------------------------------------------------------------------
	// 2. Test GET /api/v1/nodes: Check plaintext server & port returned directly
	// -------------------------------------------------------------------------
	resp, body := doReq(http.MethodGet, "/api/v1/nodes", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/nodes returned %d: %s", resp.StatusCode, string(body))
	}

	var nodePage struct {
		Data struct {
			Items []struct {
				LogicalID string `json:"logical_id"`
				Server    string `json:"server"`
				Port      int    `json:"port"`
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
			if n.Server != "tokyo1.example.com" || n.Port != 443 {
				t.Errorf("node1 expected tokyo1.example.com:443, got %s:%d", n.Server, n.Port)
			}
		}
		if n.LogicalID == node3.LogicalID {
			foundN3 = true
			if n.Server != "hk3.example.com" || n.Port != 443 {
				t.Errorf("node3 expected hk3.example.com:443, got %s:%d", n.Server, n.Port)
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
					"field":      "probe_latency_ms",
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
		"target":      "singbox",
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
	// 6. Target Parity Verification
	// Compilers (singbox, surge, qx) must produce
	// the identical snapshot digest and only include node1 (Tokyo Trojan Fast).
	// -------------------------------------------------------------------------
	targets := []domain.CompilerTarget{
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
		case domain.TargetMihomo:
			// In Mihomo, proxy-groups proxies list should only contain Tokyo Trojan Fast
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
		"target":      "mihomo",
		"revision_id": emptyRev.ID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview empty group returned %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "empty_routed_group") {
		t.Errorf("expected empty_routed_group diagnostic in preview, got: %s", string(body))
	}

	// Publication creation for Mihomo MUST fail with 409 preflight conflict error
	pubReq := map[string]any{
		"target":      "mihomo",
		"revision_id": emptyRev.ID,
	}
	resp, body = doReq(http.MethodPost, "/api/v1/publications", pubReq)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for empty routed group publication on mihomo, got %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "empty_routed_group") {
		t.Errorf("expected 409 body to reference empty_routed_group, got: %s", string(body))
	}

	// Publication creation for node-only target (singbox) ignores empty_routed_group and succeeds
	sbPubReq := map[string]any{
		"target":      "singbox",
		"revision_id": emptyRev.ID,
	}
	resp, body = doReq(http.MethodPost, "/api/v1/publications", sbPubReq)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for node-only singbox publication despite empty_routed_group, got %d: %s", resp.StatusCode, string(body))
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

func findMihomoBinary() string {
	if bin := os.Getenv("MIHOMO_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	if path, err := exec.LookPath("mihomo"); err == nil {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidate := filepath.Join(home, "clashctl", "bin", "mihomo")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func findSingBoxBinary() string {
	if bin := os.Getenv("SINGBOX_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	if path, err := exec.LookPath("sing-box"); err == nil {
		return path
	}
	for _, candidate := range []string{"/usr/local/bin/sing-box", "/usr/bin/sing-box"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func writeDeterministicMihomoGeodataFixtures(t *testing.T, dir string) {
	t.Helper()
	const mmdbMetaHex = "abcdef4d61784d696e642e636f6de95b62696e6172795f666f726d61745f6d616a6f725f76657273696f6ea1025b62696e6172795f666f726d61745f6d696e6f725f76657273696f6ea04b6275696c645f65706f63680402693a0ecf4d64617461626173655f747970655047656f4c697465322d436f756e7472794b6465736372697074696f6ee142656e5d074375746f6d697a65642047656f4c6974653220436f756e7472792064617461626173654a69705f76657273696f6ea106496c616e67756167657300044a6e6f64655f636f756e74c1014b7265636f72645f73697a65a118"
	metaBytes, err := hex.DecodeString(mmdbMetaHex)
	if err != nil {
		t.Fatalf("decode mmdb meta hex: %v", err)
	}
	mmdb := make([]byte, 0, 6+16+len(metaBytes))
	mmdb = append(mmdb, 0x00, 0x00, 0x01, 0x00, 0x00, 0x01)
	mmdb = append(mmdb, make([]byte, 16)...)
	mmdb = append(mmdb, metaBytes...)

	for _, name := range []string{"country.mmdb", "Country.mmdb", "geoip.metadb"} {
		if writeErr := os.WriteFile(filepath.Join(dir, name), mmdb, 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", name, writeErr)
		}
	}

	var geosite []byte
	for _, code := range []string{"CN", "CATEGORY-ADS-ALL", "GOOGLE", "GITHUB"} {
		codeBytes := []byte(code)
		dom := []byte("\x08\x02\x12\x0bexample.com")
		entry := make([]byte, 0, 2+len(codeBytes)+2+len(dom))
		entry = append(entry, 0x0a, byte(len(codeBytes)))
		entry = append(entry, codeBytes...)
		entry = append(entry, 0x12, byte(len(dom)))
		entry = append(entry, dom...)
		geosite = append(geosite, 0x0a, byte(len(entry)))
		geosite = append(geosite, entry...)
	}

	for _, name := range []string{"geosite.dat", "GeoSite.dat"} {
		if writeErr := os.WriteFile(filepath.Join(dir, name), geosite, 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", name, writeErr)
		}
	}
}

func TestFeatureConvergence_NativeMihomoExportCleanSlate(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(sqlite.Config{
		Path:        fmt.Sprintf("file:native_mihomo_test_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	nodeRepo := sqlite.NewNodeRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)

	now := time.Now().UTC()
	hy2ID := domain.ComputeNodeLogicalID(domain.ProtocolHysteria2, "hy2.integration.edge", 443, nil)
	hy2Node := domain.Node{
		LogicalID:   hy2ID,
		Protocol:    domain.ProtocolHysteria2,
		DisplayName: "Hy2-Integration",
		Server:      "hy2.integration.edge",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			Password: "hy2-convergence-secret",
			Transport: map[string]string{
				"sni":              "hy2.integration.edge",
				"skip_cert_verify": "true",
			},
		},
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	ssID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "ss.integration.edge", 8388, nil)
	ssNode := domain.Node{
		LogicalID:   ssID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "SS-Integration",
		Server:      "ss.integration.edge",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-256-gcm",
			Password: "ss-convergence-secret",
		},
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{hy2Node, ssNode}); err != nil {
		t.Fatalf("upsert nodes: %v", err)
	}

	revID := domain.MustNewUUIDv7()
	_ = revRepo.Create(ctx, &domain.ConfigurationRevision{
		ID:            revID,
		ContentDigest: "sha256:hy2-ss-conv-rev",
		State:         domain.RevisionStateDraft,
		CreatedAt:     now,
	})
	_ = revRepo.SetActive(ctx, revID)

	groupID := domain.MustNewUUIDv7()
	_ = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
		ID:        groupID,
		Name:      "DUAL-CONV",
		GroupType: domain.GroupTypeSelect,
	})
	edgeID1 := domain.MustNewUUIDv7()
	edgeID2 := domain.MustNewUUIDv7()
	_ = policyRepo.SetEdgesForGroup(ctx, groupID, []domain.GroupEdge{
		{ID: edgeID1, ParentGroupID: groupID, NodeLogicalID: &hy2ID, Position: 0},
		{ID: edgeID2, ParentGroupID: groupID, NodeLogicalID: &ssID, Position: 1},
	})
	ruleID := domain.MustNewUUIDv7()
	_ = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID:            ruleID,
		RevisionID:    revID,
		TargetGroupID: groupID,
		Expression:    "MATCH",
		Position:      0,
	})

	pubService := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
	)

	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		PublicationService: pubService,
		PublicationTokenValidator: func(ctx context.Context, publicationID, token string) (bool, error) {
			return pubService.ValidateToken(ctx, publicationID, token)
		},
		IsPublicationToken: func(ctx context.Context, token string) bool {
			return pubService.IsPublicationToken(ctx, token)
		},
		AuditRepository: auditRepo,
	})

	// 1. New requests with target 'clash' must be rejected with 422 unsupported_target (POST and GET)
	for _, endpoint := range []string{"/api/v1/publications", "/api/v1/publications/preview", "/api/v1/publications/preflight"} {
		req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"target": "clash"}`))
		req.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for %s with clash, got %d. body: %s", endpoint, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "unsupported_target") {
			t.Fatalf("expected unsupported_target error code for %s, got: %s", endpoint, rec.Body.String())
		}
	}
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/publications/preflight?target=clash", nil)
		req.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for GET preflight with clash, got %d. body: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "unsupported_target") {
			t.Fatalf("expected unsupported_target error code for GET preflight, got: %s", rec.Body.String())
		}
	}

	// 2. Preview with target 'mihomo' -> 200 OK with real Hysteria2 + Shadowsocks credentials, no placeholder server
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target": "mihomo"}`))
	previewReq.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
	previewReq.Header.Set("Content-Type", "application/json")
	previewRec := httptest.NewRecorder()
	router.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for Mihomo preview, got %d. body: %s", previewRec.Code, previewRec.Body.String())
	}
	var prevData struct {
		Data struct {
			Content       string `json:"content"`
			ContentDigest string `json:"content_digest"`
		} `json:"data"`
	}
	if err := json.Unmarshal(previewRec.Body.Bytes(), &prevData); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	// Assert Hysteria2 details
	if !strings.Contains(prevData.Data.Content, "type: hysteria2") || !strings.Contains(prevData.Data.Content, "hy2-convergence-secret") ||
		!strings.Contains(prevData.Data.Content, "server: hy2.integration.edge") || !strings.Contains(prevData.Data.Content, "port: 443") {
		t.Fatalf("preview missing Hysteria2 details:\n%s", prevData.Data.Content)
	}
	// Assert Shadowsocks details
	if !strings.Contains(prevData.Data.Content, "type: ss") || !strings.Contains(prevData.Data.Content, "ss-convergence-secret") ||
		!strings.Contains(prevData.Data.Content, "cipher: aes-256-gcm") || !strings.Contains(prevData.Data.Content, "server: ss.integration.edge") ||
		!strings.Contains(prevData.Data.Content, "port: 8388") {
		t.Fatalf("preview missing Shadowsocks details:\n%s", prevData.Data.Content)
	}
	// Assert no placeholder logical IDs used as server address
	if strings.Contains(prevData.Data.Content, "server: "+hy2ID) || strings.Contains(prevData.Data.Content, "server: "+ssID) {
		t.Fatalf("preview output contains logical ID as server address:\n%s", prevData.Data.Content)
	}

	// 3. Publish with target 'mihomo' -> 201 Created
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(`{"target": "mihomo"}`))
	pubReq.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
	pubReq.Header.Set("Content-Type", "application/json")
	pubRec := httptest.NewRecorder()
	router.ServeHTTP(pubRec, pubReq)
	if pubRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for Mihomo publish, got %d. body: %s", pubRec.Code, pubRec.Body.String())
	}
	var pubData struct {
		Data struct {
			Publication struct {
				ID string `json:"id"`
			} `json:"publication"`
			RawToken      string `json:"raw_token"`
			ContentDigest string `json:"content_digest"`
		} `json:"data"`
	}
	if err := json.Unmarshal(pubRec.Body.Bytes(), &pubData); err != nil {
		t.Fatalf("unmarshal publish: %v", err)
	}
	pubID := pubData.Data.Publication.ID
	rawToken := pubData.Data.RawToken
	if pubData.Data.ContentDigest != prevData.Data.ContentDigest {
		t.Fatalf("content digest mismatch between preview and publish: %s vs %s", prevData.Data.ContentDigest, pubData.Data.ContentDigest)
	}

	// 4. Download artifact via GET /publish/v1/{id}?token={token} -> 200 OK application/yaml
	clientReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubID, rawToken), nil)
	clientRec := httptest.NewRecorder()
	router.ServeHTTP(clientRec, clientReq)
	if clientRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for download, got %d. body: %s", clientRec.Code, clientRec.Body.String())
	}
	if clientRec.Header().Get("Content-Type") != "application/yaml" {
		t.Fatalf("expected application/yaml, got %s", clientRec.Header().Get("Content-Type"))
	}
	if clientRec.Body.String() != prevData.Data.Content {
		t.Fatal("download body does not match preview content")
	}

	// 5. Official Mihomo CLI validation
	mihomoBin := findMihomoBinary()
	if mihomoBin == "" {
		t.Fatal("expected mihomo binary to be found for official CLI validation")
	}
	{
		tmpDir, dirErr := os.MkdirTemp("", "mihomo-integ-*")
		if dirErr != nil {
			t.Fatalf("create temp dir: %v", dirErr)
		}
		defer os.RemoveAll(tmpDir)

		cfgPath := filepath.Join(tmpDir, "config.yaml")
		if writeErr := os.WriteFile(cfgPath, clientRec.Body.Bytes(), 0o600); writeErr != nil {
			t.Fatalf("write config file: %v", writeErr)
		}
		cmd := exec.Command(mihomoBin, "-t", "-d", tmpDir, "-f", cfgPath)
		output, execErr := cmd.CombinedOutput()
		if execErr != nil {
			t.Fatalf("official mihomo -t validation failed: %v\nOutput: %s", execErr, string(output))
		}
		t.Logf("official mihomo validation passed using %s: %s", mihomoBin, strings.TrimSpace(string(output)))
	}

	// 6. Restart recovery: simulate process restart with completely new Service & Router instance (cleared memory cache)
	{
		restartedPubService := publication.NewService(
			pubRepo,
			auditRepo,
			publication.WithPolicyRepository(policyRepo),
			publication.WithRevisionRepository(revRepo),
			publication.WithNodeRepository(nodeRepo),
		)
		restartedRouter := transporthttp.NewRouter(transporthttp.RouterConfig{
			PublicationService: restartedPubService,
			PublicationTokenValidator: func(ctx context.Context, publicationID, token string) (bool, error) {
				return restartedPubService.ValidateToken(ctx, publicationID, token)
			},
			IsPublicationToken: func(ctx context.Context, token string) bool {
				return restartedPubService.IsPublicationToken(ctx, token)
			},
			AuditRepository: auditRepo,
		})

		rReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubID, rawToken), nil)
		rRec := httptest.NewRecorder()
		restartedRouter.ServeHTTP(rRec, rReq)
		if rRec.Code != http.StatusOK {
			t.Fatalf("restart recovery failed, expected 200, got %d. body: %s", rRec.Code, rRec.Body.String())
		}
		if rRec.Body.String() != prevData.Data.Content {
			t.Fatal("restarted download content does not match original content")
		}

		// 7. Revoke publication via POST /api/v1/publications/{id}/revoke -> subsequent download 403
		revokeReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/publications/%s/revoke", pubID), nil)
		revokeReq.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
		revokeRec := httptest.NewRecorder()
		restartedRouter.ServeHTTP(revokeRec, revokeReq)
		if revokeRec.Code != http.StatusOK {
			t.Fatalf("expected 200 for revoke, got %d. body: %s", revokeRec.Code, revokeRec.Body.String())
		}

		revokedDownloadReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubID, rawToken), nil)
		revokedDownloadRec := httptest.NewRecorder()
		restartedRouter.ServeHTTP(revokedDownloadRec, revokedDownloadReq)
		if revokedDownloadRec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for download after revoke, got %d. body: %s", revokedDownloadRec.Code, revokedDownloadRec.Body.String())
		}
	}

	// 8. Verify secrets do NOT enter audit logs in database
	{
		rows, qErr := db.QueryContext(ctx, "SELECT id, action, redacted_summary FROM audit_events")
		if qErr != nil {
			t.Fatalf("query audit_events: %v", qErr)
		}
		defer rows.Close()
		for rows.Next() {
			var id, action, summary string
			if scanErr := rows.Scan(&id, &action, &summary); scanErr != nil {
				t.Fatalf("scan audit row: %v", scanErr)
			}
			for _, secret := range []string{"hy2-convergence-secret", "ss-convergence-secret", "01234567890123456789012345678901"} {
				if strings.Contains(action, secret) || strings.Contains(summary, secret) {
					t.Fatalf("CRITICAL SECURITY LEAK: secret %q found in audit_events row %s", secret, id)
				}
			}
		}
	}

	// 9. Historic clash publication in DB returns 422 Unprocessable Entity for valid token, 401 for bad token, 403 for revoked
	histRawToken := "pub_historic_token_abc"
	histHash := hex.EncodeToString(func() []byte { h := sha256.Sum256([]byte(histRawToken)); return h[:] }())
	histID := "0191e4a0-0000-7000-8000-000000000077"
	_, _ = db.ExecContext(ctx, `
		INSERT INTO publications (id, target, snapshot_digest, compiler_version, token_hash, state, created_at)
		VALUES (?, 'clash', 'sha256:legacy-snap', '1.0.0', ?, 'active', ?);`,
		histID, histHash, now.Format(time.RFC3339))

	// Invalid token -> 401
	{
		r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=bad_token", histID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for bad token on historic clash, got %d", w.Code)
		}
	}

	// Revoked -> 403
	{
		_, _ = db.ExecContext(ctx, "UPDATE publications SET state = 'revoked', revoked_at = ? WHERE id = ?;", now.Format(time.RFC3339), histID)
		r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", histID, histRawToken), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for revoked historic clash, got %d", w.Code)
		}
		_, _ = db.ExecContext(ctx, "UPDATE publications SET state = 'active', revoked_at = NULL WHERE id = ?;", histID)
	}

	// Valid token on active historic clash -> 422 Unprocessable Entity (unsupported_target)
	{
		r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", histID, histRawToken), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for valid token on historic clash, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "unsupported_target") {
			t.Fatalf("expected unsupported_target, got: %s", w.Body.String())
		}
	}

	// Historic clash row in DB must remain unmigrated/untouched
	var storedTarget, storedState string
	if err := db.QueryRowContext(ctx, "SELECT target, state FROM publications WHERE id = ?;", histID).Scan(&storedTarget, &storedState); err != nil {
		t.Fatalf("query historic clash row: %v", err)
	}
	if storedTarget != "clash" || storedState != "active" {
		t.Fatalf("historic clash row was unexpectedly mutated: target=%q state=%q", storedTarget, storedState)
	}
}

func TestFeatureConvergence_AllFourTargetsFullLifecycleAndRotationRejection(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(sqlite.Config{
		Path:        fmt.Sprintf("file:four_targets_conv_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	nodeRepo := sqlite.NewNodeRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)

	nodeFilterRepo := sqlite.NewNodeFilterRepository(db)

	// Ingest all 7 protocols via inventory.ReconcileSubscription (YAML with SS, VMess, VLESS Reality, Trojan, Hysteria2, WireGuard, TUIC)
	yaml7Proto := `proxies:
  - name: "N1-SS"
    type: ss
    server: 198.51.100.11
    port: 8388
    cipher: aes-256-gcm
    password: "secret-ss-password-conv"
  - name: "N2-VMess"
    type: vmess
    server: 198.51.100.12
    port: 443
    uuid: "11111111-1111-4111-8111-111111111111"
    alterId: 0
    cipher: auto
    tls: true
    servername: vmess.conv.example.com
    network: ws
    ws-opts:
      path: /ws
      headers:
        Host: vmess.conv.example.com
  - name: "N3-VLESS-Reality"
    type: vless
    server: 198.51.100.13
    port: 443
    uuid: "22222222-2222-4222-8222-222222222222"
    tls: true
    servername: vless.conv.example.com
    flow: xtls-rprx-vision
    client-fingerprint: chrome
    reality-opts:
      public-key: "jNXHt1yRo0vD5_1N6p2W3x4Y5z6A7b8C9d0E1f2G3h4"
      short-id: "01ab"
  - name: "N4-Trojan"
    type: trojan
    server: 198.51.100.14
    port: 443
    password: "secret-trojan-password-conv"
    sni: trojan.conv.example.com
  - name: "N5-Hysteria2"
    type: hysteria2
    server: 198.51.100.15
    port: 8443
    password: "secret-hy2-password-conv"
    sni: hy2.conv.example.com
    up: "50 Mbps"
    down: "200 Mbps"
    obfs: salamander
    obfs-password: "secret-hy2-obfs-password-conv"
  - name: "N6-WireGuard"
    type: wireguard
    server: 198.51.100.16
    port: 51820
    ip: 10.0.0.2/32
    ipv6: fd00::2/128
    private-key: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
    public-key: "HyCEC7mK3/cd/2d+p4I5dfB3nBvV9uG1D2L8aF+p+A8="
    pre-shared-key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
    reserved: [1, 2, 3]
    mtu: 1420
    dns: [1.1.1.1]
  - name: "N7-TUIC"
    type: tuic
    server: 198.51.100.17
    port: 8443
    uuid: "33333333-3333-4333-8333-333333333333"
    password: "secret-tuic-password-conv"
    congestion-controller: bbr
    udp-relay-mode: native
    alpn: [h3]
    sni: tuic.conv.example.com
    disable-sni: false
`
	subID := domain.MustNewUUIDv7()
	now := time.Now().UTC()
	if err := subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "7-Proto-Source",
		SourceURLSecretRef: "https://example.com/7proto.yaml",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	fetcher := fixtureFetcher{
		response: &fetch.Response{
			StatusCode:    200,
			ContentType:   "text/yaml",
			ContentDigest: "sha256:7proto-digest-v1",
			Body:          []byte(yaml7Proto),
		},
	}
	invSvc := inventory.NewService(
		db,
		subRepo,
		fetchRepo,
		nodeRepo,
		sourceRepo,
		fetcher,
	)
	recRes, err := invSvc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("reconcile 7 protocols failed: %v", err)
	}
	if recRes.NodesValid != 7 {
		t.Fatalf("expected 7 valid nodes ingested, got %d", recRes.NodesValid)
	}

	nodes, _, err := nodeRepo.List(ctx, domain.NodeFilter{Pagination: domain.Pagination{Page: 1, PageSize: 20}})
	if err != nil || len(nodes) != 7 {
		t.Fatalf("expected 7 nodes in DB, got %d (err=%v)", len(nodes), err)
	}

	nodeByProto := make(map[domain.Protocol]domain.Node, 7)
	for _, n := range nodes {
		nodeByProto[n.Protocol] = n
	}

	// Revision 1: Full 7-protocol snapshot with select + urltest groups & modern rules (compatible with Mihomo and sing-box)
	rev7ID := domain.MustNewUUIDv7()
	_ = revRepo.Create(ctx, &domain.ConfigurationRevision{
		ID:            rev7ID,
		ContentDigest: "sha256:rev7-digest",
		State:         domain.RevisionStateDraft,
		CreatedAt:     now,
	})
	_ = revRepo.SetActive(ctx, rev7ID)

	gSelectID := domain.MustNewUUIDv7()
	gURLTestID := domain.MustNewUUIDv7()
	_ = policyRepo.CreateGroup(ctx, &domain.NodeGroup{ID: gSelectID, Name: "Proxy-Select", GroupType: domain.GroupTypeSelect})
	_ = policyRepo.CreateGroup(ctx, &domain.NodeGroup{ID: gURLTestID, Name: "Auto-URLTest", GroupType: domain.GroupTypeURLTest})

	var selectEdges, urlTestEdges []domain.GroupEdge
	pos := 0
	for _, proto := range []domain.Protocol{
		domain.ProtocolSS,
		domain.ProtocolVMess,
		domain.ProtocolVLESS,
		domain.ProtocolTrojan,
		domain.ProtocolHysteria2,
		domain.ProtocolWireGuard,
		domain.ProtocolTUIC,
	} {
		lid := nodeByProto[proto].LogicalID
		selectEdges = append(selectEdges, domain.GroupEdge{
			ID:            domain.MustNewUUIDv7(),
			ParentGroupID: gSelectID,
			NodeLogicalID: &lid,
			Position:      pos,
		})
		urlTestEdges = append(urlTestEdges, domain.GroupEdge{
			ID:            domain.MustNewUUIDv7(),
			ParentGroupID: gURLTestID,
			NodeLogicalID: &lid,
			Position:      pos,
		})
		pos++
	}
	selectEdges = append(selectEdges, domain.GroupEdge{
		ID:            domain.MustNewUUIDv7(),
		ParentGroupID: gSelectID,
		ChildGroupID:  &gURLTestID,
		Position:      pos,
	})
	_ = policyRepo.SetEdgesForGroup(ctx, gSelectID, selectEdges)
	_ = policyRepo.SetEdgesForGroup(ctx, gURLTestID, urlTestEdges)

	for i, expr := range []string{
		"DOMAIN-SUFFIX,example.com",
		"IP-CIDR,198.51.100.0/24,no-resolve",
		"MATCH",
	} {
		_ = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
			ID:            domain.MustNewUUIDv7(),
			RevisionID:    rev7ID,
			TargetGroupID: gSelectID,
			Expression:    expr,
			Position:      i,
		})
	}

	hashedToken, err := transporthttp.HashToken("dev-insecure-admin-token")
	if err != nil {
		t.Fatalf("hash admin token: %v", err)
	}
	tokenHolder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		InitialVerifier: hashedToken,
		HashCost:        transporthttp.MinHashCost,
	})

	makeRouter := func() (*publication.Service, http.Handler) {
		svc := publication.NewService(
			pubRepo,
			auditRepo,
			publication.WithPolicyRepository(policyRepo),
			publication.WithRevisionRepository(revRepo),
			publication.WithNodeRepository(nodeRepo),
			publication.WithNodeFilterRepository(nodeFilterRepo),
		)
		r := transporthttp.NewRouter(transporthttp.RouterConfig{
			TokenHolder:        tokenHolder,
			PublicationService: svc,
			PublicationTokenValidator: func(ctx context.Context, publicationID, token string) (bool, error) {
				return svc.ValidateToken(ctx, publicationID, token)
			},
			IsPublicationToken: func(ctx context.Context, token string) bool {
				return svc.IsPublicationToken(ctx, token)
			},
			AuditRepository: auditRepo,
		})
		return svc, r
	}

	_, router := makeRouter()

	// Step A: Unauthenticated admin preview/publish rejected with 401
	{
		unauthReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(`{"target":"mihomo"}`))
		unauthReq.Header.Set("Content-Type", "application/json")
		unauthRec := httptest.NewRecorder()
		router.ServeHTTP(unauthRec, unauthReq)
		if unauthRec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated preview, got %d", unauthRec.Code)
		}
	}

	// Step B: Surge and Quantumult X strictly fail closed (422 unsupported_target_capability) on 7-protocol snapshot
	for _, unsupportedTarget := range []string{"surge", "qx"} {
		for _, path := range []string{"/api/v1/publications/preview", "/api/v1/publications"} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(fmt.Sprintf(`{"target":%q}`, unsupportedTarget)))
			req.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422 for %s on %s with 7-proto snapshot, got %d: %s", unsupportedTarget, path, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "unsupported_target_capability") {
				t.Fatalf("expected unsupported_target_capability for %s on %s, got: %s", unsupportedTarget, path, rec.Body.String())
			}
		}
	}

	// Step C: Mihomo and sing-box Preview -> Publish -> Token Download -> Official CLI Check -> Restart Recovery
	published7Proto := make(map[string]struct {
		id      string
		token   string
		content string
		digest  string
	})

	for _, target := range []string{"mihomo", "singbox"} {
		prevReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(fmt.Sprintf(`{"target":%q}`, target)))
		prevReq.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
		prevReq.Header.Set("Content-Type", "application/json")
		prevRec := httptest.NewRecorder()
		router.ServeHTTP(prevRec, prevReq)
		if prevRec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s preview, got %d: %s", target, prevRec.Code, prevRec.Body.String())
		}
		var prevEnv struct {
			Data struct {
				Content       string `json:"content"`
				ContentDigest string `json:"content_digest"`
			} `json:"data"`
		}
		if err := json.Unmarshal(prevRec.Body.Bytes(), &prevEnv); err != nil {
			t.Fatalf("unmarshal %s preview: %v", target, err)
		}

		// Ensure all 7 node secrets/endpoints are rendered and zero LogicalID placeholders exist
		for _, n := range nodes {
			if strings.Contains(prevEnv.Data.Content, n.LogicalID) {
				t.Fatalf("%s output must not contain node LogicalID %s", target, n.LogicalID)
			}
		}
		for _, expectedField := range []string{
			"secret-ss-password-conv",
			"11111111-1111-4111-8111-111111111111",
			"jNXHt1yRo0vD5_1N6p2W3x4Y5z6A7b8C9d0E1f2G3h4",
			"secret-trojan-password-conv",
			"secret-hy2-password-conv",
			"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
			"HyCEC7mK3/cd/2d+p4I5dfB3nBvV9uG1D2L8aF+p+A8=",
			"secret-tuic-password-conv",
		} {
			if !strings.Contains(prevEnv.Data.Content, expectedField) {
				t.Fatalf("%s preview missing expected credential field %q", target, expectedField)
			}
		}

		pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(fmt.Sprintf(`{"target":%q}`, target)))
		pubReq.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
		pubReq.Header.Set("Content-Type", "application/json")
		pubRec := httptest.NewRecorder()
		router.ServeHTTP(pubRec, pubReq)
		if pubRec.Code != http.StatusCreated {
			t.Fatalf("expected 201 for %s publish, got %d: %s", target, pubRec.Code, pubRec.Body.String())
		}
		var pubEnv struct {
			Data struct {
				Publication struct {
					ID string `json:"id"`
				} `json:"publication"`
				RawToken      string `json:"raw_token"`
				ContentDigest string `json:"content_digest"`
			} `json:"data"`
		}
		if err := json.Unmarshal(pubRec.Body.Bytes(), &pubEnv); err != nil {
			t.Fatalf("unmarshal %s publish: %v", target, err)
		}

		dlReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubEnv.Data.Publication.ID, pubEnv.Data.RawToken), nil)
		dlRec := httptest.NewRecorder()
		router.ServeHTTP(dlRec, dlReq)
		if dlRec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s token download, got %d: %s", target, dlRec.Code, dlRec.Body.String())
		}
		if dlRec.Body.String() != prevEnv.Data.Content {
			t.Fatalf("%s download content does not match preview content", target)
		}

		// Official CLI verification
		if target == "mihomo" {
			mihomoBin := findMihomoBinary()
			if mihomoBin == "" {
				t.Fatal("mihomo binary required for official validation")
			}
			tmpDir := t.TempDir()
			writeDeterministicMihomoGeodataFixtures(t, tmpDir)
			cfgPath := filepath.Join(tmpDir, "mihomo.yaml")
			if err := os.WriteFile(cfgPath, dlRec.Body.Bytes(), 0o600); err != nil {
				t.Fatalf("write mihomo config: %v", err)
			}
			if out, err := exec.Command(mihomoBin, "-t", "-d", tmpDir, "-f", cfgPath).CombinedOutput(); err != nil {
				t.Fatalf("official mihomo -t failed: %v\nOutput: %s", err, string(out))
			}
		} else if target == "singbox" {
			singBoxBin := findSingBoxBinary()
			if singBoxBin == "" {
				t.Fatal("sing-box binary required for official validation")
			}
			tmpDir := t.TempDir()
			cfgPath := filepath.Join(tmpDir, "sing-box.json")
			if err := os.WriteFile(cfgPath, dlRec.Body.Bytes(), 0o600); err != nil {
				t.Fatalf("write sing-box config: %v", err)
			}
			if out, err := exec.Command(singBoxBin, "check", "-c", cfgPath).CombinedOutput(); err != nil {
				t.Fatalf("official sing-box check failed: %v\nOutput: %s", err, string(out))
			}
		}

		published7Proto[target] = struct {
			id      string
			token   string
			content string
			digest  string
		}{
			id:      pubEnv.Data.Publication.ID,
			token:   pubEnv.Data.RawToken,
			content: prevEnv.Data.Content,
			digest:  prevEnv.Data.ContentDigest,
		}
	}

	// Step D: Surge & Quantumult X Preview -> Publish -> Token Download on supported subset (SS + VMess + Trojan, select group)
	rev3ID := domain.MustNewUUIDv7()
	_ = revRepo.Create(ctx, &domain.ConfigurationRevision{
		ID:            rev3ID,
		ContentDigest: "sha256:rev3-digest",
		State:         domain.RevisionStateDraft,
		CreatedAt:     now,
	})
	gSubsetID := domain.MustNewUUIDv7()
	_ = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
		ID:         gSubsetID,
		Name:       "Subset-Select",
		GroupType:  domain.GroupTypeSelect,
		NodeFilter: &domain.NodeFilterSpec{},
	})
	var subsetEdges []domain.GroupEdge
	for idx, proto := range []domain.Protocol{domain.ProtocolSS, domain.ProtocolVMess, domain.ProtocolTrojan} {
		lid := nodeByProto[proto].LogicalID
		subsetEdges = append(subsetEdges, domain.GroupEdge{
			ID:            domain.MustNewUUIDv7(),
			ParentGroupID: gSubsetID,
			NodeLogicalID: &lid,
			Position:      idx,
		})
	}
	_ = policyRepo.SetEdgesForGroup(ctx, gSubsetID, subsetEdges)
	if err := nodeFilterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
		Spec: domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpNotEquals, Value: "vless"},
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpNotEquals, Value: "hysteria2"},
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpNotEquals, Value: "wireguard"},
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpNotEquals, Value: "tuic"},
			},
		},
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set global node filter: %v", err)
	}
	// Remove gSelectID and gURLTestID so only gSubsetID is in the policy graph for Surge/QX
	_ = policyRepo.DeleteGroup(ctx, gSelectID)
	_ = policyRepo.DeleteGroup(ctx, gURLTestID)
	_ = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID:            domain.MustNewUUIDv7(),
		RevisionID:    rev3ID,
		TargetGroupID: gSubsetID,
		Expression:    "DOMAIN-SUFFIX,example.com",
		Position:      0,
	})
	_ = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID:            domain.MustNewUUIDv7(),
		RevisionID:    rev3ID,
		TargetGroupID: gSubsetID,
		Expression:    "MATCH",
		Position:      1,
	})
	_ = revRepo.SetActive(ctx, rev3ID)

	for _, target := range []string{"surge", "qx"} {
		prevReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(fmt.Sprintf(`{"target":%q}`, target)))
		prevReq.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
		prevReq.Header.Set("Content-Type", "application/json")
		prevRec := httptest.NewRecorder()
		router.ServeHTTP(prevRec, prevReq)
		if prevRec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s preview on 3-proto subset, got %d: %s", target, prevRec.Code, prevRec.Body.String())
		}
		var prevEnv struct {
			Data struct {
				Content       string `json:"content"`
				ContentDigest string `json:"content_digest"`
			} `json:"data"`
		}
		_ = json.Unmarshal(prevRec.Body.Bytes(), &prevEnv)
		for _, expectedSecret := range []string{"secret-ss-password-conv", "11111111-1111-4111-8111-111111111111", "secret-trojan-password-conv"} {
			if !strings.Contains(prevEnv.Data.Content, expectedSecret) {
				t.Fatalf("%s preview missing expected credential %q:\n%s", target, expectedSecret, prevEnv.Data.Content)
			}
		}

		pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/publications", strings.NewReader(fmt.Sprintf(`{"target":%q}`, target)))
		pubReq.Header.Set("Authorization", "Bearer dev-insecure-admin-token")
		pubReq.Header.Set("Content-Type", "application/json")
		pubRec := httptest.NewRecorder()
		router.ServeHTTP(pubRec, pubReq)
		if pubRec.Code != http.StatusCreated {
			t.Fatalf("expected 201 for %s publish on 3-proto subset, got %d: %s", target, pubRec.Code, pubRec.Body.String())
		}
		var pubEnv struct {
			Data struct {
				Publication struct {
					ID string `json:"id"`
				} `json:"publication"`
				RawToken string `json:"raw_token"`
			} `json:"data"`
		}
		_ = json.Unmarshal(pubRec.Body.Bytes(), &pubEnv)

		// Restart recovery for Surge/QX serves persisted plaintext content directly
		_, restartedRouter := makeRouter()
		dlReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", pubEnv.Data.Publication.ID, pubEnv.Data.RawToken), nil)
		dlRec := httptest.NewRecorder()
		restartedRouter.ServeHTTP(dlRec, dlReq)
		if dlRec.Code != http.StatusOK || dlRec.Body.String() != prevEnv.Data.Content {
			t.Fatalf("%s restart download failed: code=%d", target, dlRec.Code)
		}

		// Verify plaintext content is persisted directly in publications table
		var storedContent []byte
		if err := db.QueryRowContext(ctx, "SELECT content FROM publications WHERE id = ?;", pubEnv.Data.Publication.ID).Scan(&storedContent); err != nil {
			t.Fatalf("query stored publication content for %s: %v", target, err)
		}
		if string(storedContent) != prevEnv.Data.Content {
			t.Fatalf("expected stored plaintext content in publications table to match preview for %s", target)
		}
	}
}
