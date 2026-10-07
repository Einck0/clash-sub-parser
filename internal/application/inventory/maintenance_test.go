package inventory_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestParseProbeKinds(t *testing.T) {
	t.Run("default empty kinds", func(t *testing.T) {
		kinds, err := inventory.ParseProbeKinds("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(kinds) != 6 {
			t.Fatalf("expected 6 default kinds, got %d", len(kinds))
		}
	})

	t.Run("aliases and deduplication", func(t *testing.T) {
		kinds, err := inventory.ParseProbeKinds("alive,baseline,media,streaming,ai,speed,ip_risk,geo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(kinds) != 6 {
			t.Fatalf("expected 6 deduplicated kinds, got %d", len(kinds))
		}
		expectedMap := map[domain.ProbeKind]bool{
			domain.ProbeKindBaseline:  true,
			domain.ProbeKindStreaming: true,
			domain.ProbeKindAI:        true,
			domain.ProbeKindSpeed:     true,
			domain.ProbeKindIPRisk:    true,
			domain.ProbeKindGeo:       true,
		}
		for _, k := range kinds {
			if !expectedMap[k] {
				t.Errorf("unexpected kind: %s", k)
			}
		}
	})

	t.Run("invalid kind rejected", func(t *testing.T) {
		_, err := inventory.ParseProbeKinds("alive,invalid_dimension")
		if err == nil {
			t.Fatalf("expected error for invalid kind, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported probe kind") {
			t.Errorf("expected descriptive error, got %v", err)
		}
	})
}

func setupTestMaintenanceOrchestrator(t *testing.T, db *sql.DB, dialer probe.NodeDialer, fetcher fetch.Fetcher, extraRunnerOpts ...probe.DefaultRunnerOption) (*inventory.MaintenanceOrchestrator, *inventory.Service, *probe.Service, *publication.Service) {
	t.Helper()
	ctx := context.Background()

	subRepo := sqlite.NewSubscriptionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeSourceRepo := sqlite.NewNodeSourceRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	probeRunRepo := sqlite.NewProbeRunRepository(db)
	probeObsRepo := sqlite.NewProbeObservationRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revisionRepo := sqlite.NewRevisionRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	riskBindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)
	probeScheduleRepo := sqlite.NewProbeScheduleRepository(db)
	nodeFilterRepo := sqlite.NewNodeFilterRepository(db)
	pubPayloadRefRepo := sqlite.NewPublicationPayloadRefRepository(db)

	subService := subscription.NewService(subRepo, auditRepo)
	invOpts := []inventory.Option{
		inventory.WithProbeObservationRepository(probeObsRepo),
	}
	invService := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher, invOpts...)
	subService.SetReconciler(invService)

	sched, err := queue.NewScheduler(queue.Config{
		Concurrency: queue.DefaultConcurrency,
		Context:     ctx,
	})
	if err != nil {
		t.Fatalf("failed to init scheduler: %v", err)
	}
	t.Cleanup(func() {
		_ = sched.Close()
	})
	invService.SetNodePoolStateProvider(sched)

	var runnerOpts []probe.DefaultRunnerOption
	runnerOpts = append(runnerOpts, probe.WithIPRiskObservationRepository(riskObsRepo))
	if dialer != nil {
		runnerOpts = append(runnerOpts, probe.WithNodeDialer(dialer))
	}
	if len(extraRunnerOpts) > 0 {
		runnerOpts = append(runnerOpts, extraRunnerOpts...)
	}
	probeRunner := probe.NewDefaultRunner(nodeRepo, probeObsRepo, sched, probeRunRepo, runnerOpts...)

	probeService := probe.NewService(
		probeRunRepo,
		probe.WithRunner(probeRunner),
		probe.WithNodeRepository(nodeRepo),
		probe.WithObservationRepository(probeObsRepo),
		probe.WithScheduler(sched),
		probe.WithScheduleRepository(probeScheduleRepo),
		probe.WithAudit(auditRepo),
	)

	policyService := policy.NewService(policyRepo, revisionRepo, nodeRepo, auditRepo, nodeFilterRepo)
	_ = revision.NewService(revisionRepo, auditRepo, revision.WithPolicyRepository(policyRepo))
	if _, err := policyService.EnsureActiveRevision(ctx); err != nil {
		t.Fatalf("ensure active revision: %v", err)
	}

	ipriskService := iprisk.NewService(
		riskObsRepo,
		riskPolicyRepo,
		iprisk.WithBindingRepository(riskBindingRepo),
		iprisk.WithGroupRepository(policyRepo),
		iprisk.WithNodeRepository(nodeRepo),
		iprisk.WithAuditRepository(auditRepo),
	)

	pubOpts := []publication.Option{
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
		publication.WithPayloadRefRepository(pubPayloadRefRepo),
	}
	pubService := publication.NewService(
		pubRepo,
		auditRepo,
		pubOpts...,
	)

	orchestrator := inventory.NewMaintenanceOrchestrator(
		db,
		subRepo,
		nodeRepo,
		probeObsRepo,
		probeRunRepo,
		invService,
		probeRunner,
		probeService,
		pubService,
		auditRepo,
	)

	return orchestrator, invService, probeService, pubService
}

func TestMaintenanceOrchestrator_Refresh_Hermetic(t *testing.T) {
	ctx := context.Background()
	db := setupIsolatedWorkflowDB(t)
	defer db.Close()

	subRepo := sqlite.NewSubscriptionRepository(db)
	enabledIDs, _ := seedSubscriptionsWorkflow(t, subRepo)

	fetcher := newMockWorkflowFetcher()
	// Source 1 (7li): Success with 2 nodes
	sub1, _ := subRepo.GetByID(ctx, enabledIDs[0])
	fetcher.setResponse(sub1.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		Body:          []byte("proxies:\n  - name: Node1\n    type: ss\n    server: 1.1.1.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: p1\n  - name: Node2\n    type: ss\n    server: 1.1.1.2\n    port: 8388\n    cipher: aes-128-gcm\n    password: p2\n"),
		ContentDigest: "digest-sub1",
	})

	// Source 2 (魔戒): Partial with 1 valid node and 1 rejected node
	sub2, _ := subRepo.GetByID(ctx, enabledIDs[1])
	fetcher.setResponse(sub2.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		Body:          []byte("proxies:\n  - name: Node3\n    type: ss\n    server: 1.1.1.3\n    port: 8388\n    cipher: aes-128-gcm\n    password: p3\n  - name: InvalidNode\n    type: ss\n    server: 1.1.1.4\n    port: 9999999\n    cipher: aes-128-gcm\n    password: p\n"),
		ContentDigest: "digest-sub2",
	})

	// Source 3 (Dogegg): Empty (0 nodes) -> should fail without fake success
	sub3, _ := subRepo.GetByID(ctx, enabledIDs[2])
	fetcher.setResponse(sub3.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		Body:          []byte("proxies: []\n"),
		ContentDigest: "digest-sub3",
	})

	// Source 4 (einck-qzz): Network error (500)
	sub4, _ := subRepo.GetByID(ctx, enabledIDs[3])
	fetcher.setError(sub4.SourceURLSecretRef, fmt.Errorf("HTTP 500: Internal Server Error"))

	reportDir := filepath.Join(t.TempDir(), "reports")
	orchestrator, _, _, _ := setupTestMaintenanceOrchestrator(t, db, nil, fetcher)

	cfg := inventory.MaintenanceConfig{
		TargetDBPath: ":memory:",
		Operation:    inventory.OperationRefresh,
		ReportDir:    reportDir,
	}

	report, err := orchestrator.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("orchestrator.Run refresh: %v", err)
	}

	if report.Status != "PARTIAL" {
		t.Errorf("expected PARTIAL status due to 2 failed sources, got %s", report.Status)
	}

	if report.Refresh == nil {
		t.Fatalf("expected RefreshSummary in report")
	}

	if report.Refresh.EnabledSources != 4 {
		t.Errorf("expected 4 enabled sources, got %d", report.Refresh.EnabledSources)
	}
	if report.Refresh.SuccessfulSources != 2 {
		t.Errorf("expected 2 successful sources (1 success, 1 partial), got %d", report.Refresh.SuccessfulSources)
	}
	if report.Refresh.FailedSources != 2 {
		t.Errorf("expected 2 failed sources (empty and 500 err), got %d", report.Refresh.FailedSources)
	}
	if report.Refresh.TotalNodesValid != 3 {
		t.Errorf("expected 3 total valid nodes, got %d", report.Refresh.TotalNodesValid)
	}

	// Verify report file existence and permissions
	if fi, err := os.Stat(reportDir); err != nil || fi.Mode().Perm() != 0700 {
		t.Errorf("expected report dir perm 0700, got %v (err: %v)", fi.Mode().Perm(), err)
	}
	if fi, err := os.Stat(report.ReportFilePath); err != nil || fi.Mode().Perm() != 0600 {
		t.Errorf("expected report file perm 0600, got %v (err: %v)", fi.Mode().Perm(), err)
	}

	// Verify report file does not leak secret tokens
	reportBytes, err := os.ReadFile(report.ReportFilePath)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}
	reportStr := string(reportBytes)
	if strings.Contains(reportStr, "tok1") || strings.Contains(reportStr, "tok2") {
		t.Errorf("report file leaked raw token parameters")
	}
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func mockHermeticHTTPClient(statusCode int, body string) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}),
	}
}

func TestMaintenanceOrchestrator_ProbeAndGating_Hermetic(t *testing.T) {
	ctx := context.Background()
	db := setupIsolatedWorkflowDB(t)
	defer db.Close()

	subRepo := sqlite.NewSubscriptionRepository(db)
	now := domain.NowUTC()
	subID := domain.MustNewUUIDv7()
	subURL := "https://example.com/sub.yaml"
	if err := subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "TestSub",
		SourceURLSecretRef: subURL,
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds: 86400,
			TimeoutSeconds:  30,
		},
		Revision:  "rev1",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create sub: %v", err)
	}

	fetcher := newMockWorkflowFetcher()
	yamlContent := `proxies:
  - name: HealthyNode
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: p1
  - name: FailingNode
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: p2
`
	fetcher.setResponse(subURL, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(yamlContent),
		ContentDigest: "digest-nodes",
	})

	// Mock dialer: server 198.51.100.1 succeeds, 198.51.100.2 fails
	mockDialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		if node.Server == "198.51.100.1" {
			client := mockHermeticHTTPClient(http.StatusNoContent, "")
			return client, func() error { return nil }, nil
		}
		return nil, nil, context.DeadlineExceeded
	}

	orchestrator, invService, _, _ := setupTestMaintenanceOrchestrator(t, db, mockDialer, fetcher,
		probe.WithRunBudget(probe.RunBudget{
			MaxTasks:         1000,
			MaxResponseBytes: 1024 * 1024,
			TaskTimeout:      5 * time.Second,
		}),
	)

	// Reconcile subscription to seed nodes canonically
	if _, err := invService.ReconcileSubscription(ctx, subID); err != nil {
		t.Fatalf("reconcile seed subscription: %v", err)
	}

	reconciledNodes, _, _ := invService.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	for _, n := range reconciledNodes {
		t.Logf("Reconciled node: id=%s name=%s server=%s port=%d", n.LogicalID, n.DisplayName, n.Server, n.Port)
	}

	reportDir := filepath.Join(t.TempDir(), "reports")
	cfg := inventory.MaintenanceConfig{
		TargetDBPath:     ":memory:",
		Operation:        inventory.OperationProbe,
		ReportDir:        reportDir,
		Kinds:            []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindStreaming},
		AliveConcurrency: 2,
		AliveTimeout:     5 * time.Second,
		MediaConcurrency: 2,
	}

	report, err := orchestrator.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("orchestrator.Run probe: %v", err)
	}

	if report.Probe != nil && report.Probe.Summary != "" {
		t.Logf("Probe summary: %s", report.Probe.Summary)
	}
	var countInDB int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM probe_observations;").Scan(&countInDB)
	t.Logf("Count of probe_observations in DB: %d", countInDB)

	if report.Probe == nil {
		t.Fatalf("expected ProbeSummary in report")
	}

	if report.Probe.TotalNodesTargeted != 2 {
		t.Errorf("expected 2 nodes targeted, got %d", report.Probe.TotalNodesTargeted)
	}
	if report.Probe.AvailableNodes != 1 {
		t.Logf("report.Probe summary: %s", report.Probe.Summary)
		for i, r := range report.Probe.Rows {
			t.Logf("row %d: %+v", i, r)
		}
		t.Errorf("expected 1 available node (HealthyNode), got %d", report.Probe.AvailableNodes)
	}

	// Verify gating row coverage:
	// HealthyNode: baseline available, streaming completed/attempted
	// FailingNode: baseline failed/unavailable, streaming skipped with alive_gating_dependency_failed
	var foundSkippedDependency bool
	for _, row := range report.Probe.Rows {
		if row.Kind == string(domain.ProbeKindStreaming) && row.Status == "skipped" && row.SkipReason == "alive_gating_dependency_failed" {
			foundSkippedDependency = true
		}
	}
	if !foundSkippedDependency {
		t.Errorf("expected failing node streaming task to be skipped due to alive gating dependency failure; rows: %+v", report.Probe.Rows)
	}
}

func TestMaintenanceOrchestrator_Validate_Hermetic(t *testing.T) {
	ctx := context.Background()
	db := setupIsolatedWorkflowDB(t)
	defer db.Close()

	subRepo := sqlite.NewSubscriptionRepository(db)
	now := domain.NowUTC()
	subID := domain.MustNewUUIDv7()
	subURL := "https://example.com/sub.yaml"
	if err := subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "TestSub",
		SourceURLSecretRef: subURL,
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds: 86400,
			TimeoutSeconds:  30,
		},
		Revision:  "rev1",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create sub: %v", err)
	}

	fetcher := newMockWorkflowFetcher()
	yamlContent := `proxies:
  - name: NodeA
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: p1
`
	fetcher.setResponse(subURL, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(yamlContent),
		ContentDigest: "digest-validate",
	})

	mockDialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		return mockHermeticHTTPClient(http.StatusNoContent, ""), func() error { return nil }, nil
	}

	reportDir := filepath.Join(t.TempDir(), "reports")
	orchestrator, _, _, _ := setupTestMaintenanceOrchestrator(t, db, mockDialer, fetcher,
		probe.WithRunBudget(probe.RunBudget{
			MaxTasks:         1000,
			MaxResponseBytes: 1024 * 1024,
			TaskTimeout:      5 * time.Second,
		}),
	)

	cfg := inventory.MaintenanceConfig{
		TargetDBPath:     ":memory:",
		Operation:        inventory.OperationValidate,
		ReportDir:        reportDir,
		Kinds:            []domain.ProbeKind{domain.ProbeKindBaseline},
		AliveConcurrency: 2,
		AliveTimeout:     5 * time.Second,
	}

	report, err := orchestrator.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("orchestrator.Run validate: %v", err)
	}

	if report.Status != "PASS" {
		t.Errorf("expected PASS status, got %s", report.Status)
	}

	if report.Refresh == nil || report.Refresh.SuccessfulSources != 1 {
		t.Errorf("expected 1 successful source refresh, got %+v", report.Refresh)
	}
	if report.Probe == nil || report.Probe.AvailableNodes != 1 {
		t.Errorf("expected 1 available probed node, got %+v", report.Probe)
	}
	if report.Preview == nil || !report.Preview.Allowed {
		t.Errorf("expected allowed preview, got %+v", report.Preview)
	}
}

func TestMaintenanceOrchestrator_DryRun_NoMutation(t *testing.T) {
	ctx := context.Background()
	db := setupIsolatedWorkflowDB(t)
	defer db.Close()

	subRepo := sqlite.NewSubscriptionRepository(db)
	seedSubscriptionsWorkflow(t, subRepo)

	reportDir := filepath.Join(t.TempDir(), "reports")
	orchestrator, _, _, _ := setupTestMaintenanceOrchestrator(t, db, nil, nil)

	cfg := inventory.MaintenanceConfig{
		TargetDBPath: ":memory:",
		Operation:    inventory.OperationValidate,
		DryRun:       true,
		ReportDir:    reportDir,
	}

	report, err := orchestrator.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("orchestrator.Run dry-run: %v", err)
	}

	if !report.DryRun {
		t.Errorf("expected report.DryRun=true")
	}
	if report.Refresh == nil || report.Refresh.Sources == nil {
		t.Fatalf("expected dry-run refresh sources inspected")
	}
	for _, s := range report.Refresh.Sources {
		if s.Status != "dry_run_inspected" {
			t.Errorf("expected status 'dry_run_inspected', got %s", s.Status)
		}
	}

	// Verify no nodes were added
	var nodeCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes;").Scan(&nodeCount)
	if nodeCount != 0 {
		t.Errorf("expected 0 nodes in dry-run, got %d", nodeCount)
	}
}
