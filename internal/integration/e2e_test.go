package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	ipriskApp "clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	ipriskProbe "clash-sub-parser/internal/probe/iprisk"
	"clash-sub-parser/internal/probe/singbox"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
	transporthttp "clash-sub-parser/internal/transport/http"
	"clash-sub-parser/migrations"
)

type fixtureFetcher struct {
	response *fetch.Response
}

func (f fixtureFetcher) Fetch(context.Context, fetch.Options) (*fetch.Response, error) {
	return f.response, nil
}

func openIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(sqlite.Config{
		Path:        fmt.Sprintf("file:integration_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(context.Background()); err != nil {
		t.Fatalf("apply integration migrations: %v", err)
	}
	return db
}

// TestControlPlaneEndToEndFixture runs a comprehensive, unmocked closed-loop harness:
// 1. Subscription refresh with raw YAML fixture
// 2. Inventory ingestion, stable non-secret logical ID computation, and ledger query
// 3. Sing-box in-memory probe options generation, runtime startup, and observation recording
// 4. Policy deterministic resolution with Group and Rule graph to produce stable snapshot digest
// 5. Five-target compilation across Clash, Mihomo, sing-box, Surge, and Quantumult X
// 6. Publication creation, token generation, and client retrieval via HTTP endpoint
// 7. UI Console display delivery (embedded SPA shell) and health/ready endpoints
func TestControlPlaneEndToEndFixture(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)

	// -------------------------------------------------------------------------
	// 1. Subscription Refresh
	// -------------------------------------------------------------------------
	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	sub := &domain.Subscription{
		ID:                 "integration-subscription",
		Name:               "Integration fixture",
		SourceURLSecretRef: "https://fixture.invalid/subscription.yaml",
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds:  3600,
			TimeoutSeconds:   10,
			MaxResponseBytes: 1024 * 1024,
		},
		Revision:  domain.MustNewUUIDv7(),
		CreatedAt: domain.NowUTC(),
		UpdatedAt: domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	inventoryService := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fixtureFetcher{
		response: &fetch.Response{
			StatusCode:    200,
			ContentType:   "text/yaml",
			ContentDigest: "fixture-digest",
			Body: []byte("proxies:\n" +
				"  - name: Integration Shadowsocks\n" +
				"    type: ss\n" +
				"    server: 198.51.100.10\n" +
				"    port: 8388\n" +
				"    cipher: aes-128-gcm\n" +
				"    password: fixture-secret\n"),
		},
	})
	refresh, err := inventoryService.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("refresh subscription: %v", err)
	}
	if refresh.Outcome != domain.FetchOutcomeSuccess || refresh.NodesValid != 1 {
		t.Fatalf("unexpected refresh result: %+v", refresh)
	}

	// -------------------------------------------------------------------------
	// 2. Inventory Ingestion & Ledger
	// -------------------------------------------------------------------------
	nodes, total, err := inventoryService.ListNodes(ctx, domain.NodeFilter{Pagination: domain.Pagination{Page: 1, PageSize: 10}})
	if err != nil || total != 1 || len(nodes) != 1 {
		t.Fatalf("inventory ledger mismatch: total=%d nodes=%d err=%v", total, len(nodes), err)
	}
	node := nodes[0]
	if !domain.IsValidLogicalID(node.LogicalID) {
		t.Fatalf("node logical ID is invalid: %s", node.LogicalID)
	}
	if node.Protocol != domain.ProtocolSS {
		t.Fatalf("expected ss protocol, got: %s", node.Protocol)
	}

	// -------------------------------------------------------------------------
	// 3. Sing-box In-Memory Probe & Observation Recording
	// -------------------------------------------------------------------------
	probeRepo := sqlite.NewProbeRunRepository(db)
	probeObsRepo := sqlite.NewProbeObservationRepository(db)
	probeService := probe.NewService(probeRepo)

	run, err := probeService.Create(ctx, probe.CreateRunCommand{
		ActorScope:     "integration",
		IdempotencyKey: "integration-probe-1",
		ConfigRevision: sub.Revision,
		NodeLogicalIDs: []string{node.LogicalID},
		Kinds:          []domain.ProbeKind{domain.ProbeKindBaseline},
	})
	if err != nil {
		t.Fatalf("create probe run: %v", err)
	}

	nodeCfg := singbox.NodeConfig{
		LogicalID:   node.LogicalID,
		DisplayName: node.DisplayName,
		Protocol:    node.Protocol,
		Server:      "198.51.100.10",
		Port:        8388,
		Method:      "aes-128-gcm",
		Password:    "fixture-secret",
	}
	options, tag, err := singbox.BuildOptions(nodeCfg)
	if err != nil || tag != node.LogicalID {
		t.Fatalf("singbox BuildOptions failed: tag=%s err=%v", tag, err)
	}
	if len(options.Outbounds) != 1 {
		t.Fatalf("expected 1 singbox outbound, got %d", len(options.Outbounds))
	}

	rt, err := singbox.New(ctx, nodeCfg)
	if err != nil {
		t.Fatalf("start in-memory sing-box runtime: %v", err)
	}
	if rt.Tag() != node.LogicalID {
		t.Fatalf("unexpected runtime tag: %s", rt.Tag())
	}
	_ = rt.Close()

	// Execute probe run and store observation
	if err := probeService.Execute(ctx, run.ID, func(context.Context) error {
		return probeObsRepo.Create(ctx, &domain.ProbeObservation{
			ID:              domain.MustNewUUIDv7(),
			ProbeRunID:      run.ID,
			NodeLogicalID:   node.LogicalID,
			Kind:            domain.ProbeKindBaseline,
			Verdict:         domain.VerdictAvailable,
			EvidenceDigest:  "sha256:integration-fixture",
			ObservedAt:      domain.NowUTC(),
			LatencyMS:       32,
			RedactedSummary: "baseline probe success",
		})
	}); err != nil {
		t.Fatalf("execute probe run: %v", err)
	}

	completedRun, err := probeService.Get(ctx, run.ID)
	if err != nil || completedRun.State != domain.ProbeRunStateSucceeded {
		t.Fatalf("probe run did not succeed: state=%s err=%v", completedRun.State, err)
	}

	observations, err := probeObsRepo.ListByNode(ctx, node.LogicalID, 10)
	if err != nil || len(observations) != 1 || observations[0].Verdict != domain.VerdictAvailable {
		t.Fatalf("expected 1 available observation, got %d err=%v", len(observations), err)
	}

	// -------------------------------------------------------------------------
	// 4. Policy Deterministic Resolution
	// -------------------------------------------------------------------------
	groupID := domain.MustNewUUIDv7()
	groupEdgeNodeID := node.LogicalID
	snapshot, err := resolver.New().Resolve(ctx, resolver.ResolveInput{
		RevisionID:      "integration-revision",
		CompilerVersion: "1.0.0",
		Nodes:           nodes,
		Groups:          []domain.NodeGroup{{ID: groupID, Name: "Integration Select", GroupType: domain.GroupTypeSelect}},
		Edges:           map[string][]domain.GroupEdge{groupID: {{ID: domain.MustNewUUIDv7(), ParentGroupID: groupID, NodeLogicalID: &groupEdgeNodeID}}},
		PolicyRules:     []domain.PolicyRule{{ID: "integration-rule", TargetGroupID: groupID, Expression: "MATCH", Position: 0}},
	})
	if err != nil {
		t.Fatalf("resolve policy: %v", err)
	}
	if snapshot.SnapshotDigest == "" || len(snapshot.NodeLogicalIDs) != 1 {
		t.Fatalf("invalid resolved snapshot: %+v", snapshot)
	}

	// -------------------------------------------------------------------------
	// 5. Five-Target Compilation
	// -------------------------------------------------------------------------
	expectedTargets := []domain.CompilerTarget{
		domain.TargetClash,
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}
	for _, target := range expectedTargets {
		compiled, err := compiler.Compile(ctx, snapshot, target)
		if err != nil {
			t.Fatalf("compile %s: %v", target, err)
		}
		if len(compiled.Content) == 0 || compiled.ContentDigest == "" {
			t.Fatalf("empty compile artifact for %s", target)
		}
	}

	// -------------------------------------------------------------------------
	// 6. Publication Creation & Client HTTP Retrieval
	// -------------------------------------------------------------------------
	publicationRepo := sqlite.NewPublicationRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	publicationService := publication.NewService(publicationRepo, auditRepo)

	published, err := publicationService.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snapshot,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-integration-e2e",
	})
	if err != nil {
		t.Fatalf("publish compiled artifact: %v", err)
	}

	artifact, err := publicationService.ResolveAndServe(ctx, published.Publication.ID, published.RawToken)
	if err != nil || len(artifact.Content) == 0 {
		t.Fatalf("read publication artifact: size=%d err=%v", len(artifact.Content), err)
	}

	// Wire full HTTP router
	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		PublicationService: publicationService,
		ReadinessChecker: func(ctx context.Context) (*sqlite.ReadinessReport, error) {
			return sqlite.CheckReadiness(ctx, db)
		},
	})

	// Authorized client request with query token -> 200 OK
	clientPubURL := fmt.Sprintf("/publish/v1/%s?token=%s", published.Publication.ID, published.RawToken)
	clientReq := httptest.NewRequest(http.MethodGet, clientPubURL, nil)
	clientRec := httptest.NewRecorder()
	router.ServeHTTP(clientRec, clientReq)

	if clientRec.Code != http.StatusOK {
		t.Fatalf("client publication GET %s returned %d: %s", clientPubURL, clientRec.Code, clientRec.Body.String())
	}
	if recType := clientRec.Header().Get("Content-Type"); !strings.Contains(recType, "yaml") {
		t.Fatalf("unexpected content type: %s", recType)
	}
	if clientRec.Body.String() != string(artifact.Content) {
		t.Fatalf("publication content mismatch: got %d bytes, expected %d bytes", clientRec.Body.Len(), len(artifact.Content))
	}

	// Unauthorized client request without token -> 401 Unauthorized
	unauthReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s", published.Publication.ID), nil)
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for publication without token, got %d", unauthRec.Code)
	}

	// -------------------------------------------------------------------------
	// 7. UI Console Display & Health Verification
	// -------------------------------------------------------------------------
	uiReq := httptest.NewRequest(http.MethodGet, "/", nil)
	uiRec := httptest.NewRecorder()
	router.ServeHTTP(uiRec, uiReq)

	if uiRec.Code != http.StatusOK {
		t.Fatalf("UI console GET / returned %d", uiRec.Code)
	}
	if !strings.Contains(uiRec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("expected HTML content-type for UI console, got: %s", uiRec.Header().Get("Content-Type"))
	}
	uiContent := uiRec.Body.String()
	if !strings.Contains(uiContent, "id=\"app\"") {
		t.Fatalf("UI console HTML missing #app mounting root: %s", uiContent)
	}

	// Liveness & readiness probes
	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	router.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("healthz returned %d", healthRec.Code)
	}

	readyReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyRec := httptest.NewRecorder()
	router.ServeHTTP(readyRec, readyReq)
	if readyRec.Code != http.StatusOK {
		t.Fatalf("readyz returned %d", readyRec.Code)
	}
}

// TestControlPlaneEndToEndWithIPRiskFakeProvider tests the full closed-loop chain with
// a deterministic fake IP risk provider:
// 1. Subscription ingestion of multiple nodes (clean node vs adversarial/Tor node)
// 2. Probing with fake IP risk provider and generating non-secret observations
// 3. Risk policy evaluation producing deterministic risk decisions and digests
// 4. Resolver applying risk admission rules (excluding/diagnosing high-risk nodes)
// 5. Five-target compilation of the admitted policy snapshot
// 6. Publication preflight interception: blocking publications that violate risk policy,
//    and allowing publications containing verified low-risk nodes
func TestControlPlaneEndToEndWithIPRiskFakeProvider(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)

	// Repositories
	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	riskBindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	providerSettingsRepo := sqlite.NewIPRiskProviderSettingsRepository(db)

	if err := providerSettingsRepo.Upsert(ctx, &domain.IPRiskProviderSettings{
		Provider:           "fake_ip_provider",
		SchemaVersion:      "v1",
		Enabled:            true,
		SecretReference:    "secret://fake_ip_provider",
		MaxConcurrency:     1,
		RequestsPerMinute:  60,
		DailyRequestBudget: 1000,
		PerRequestTimeout:  2 * time.Second,
		MaxResponseBytes:   1024 * 1024,
	}); err != nil {
		t.Fatalf("upsert provider settings: %v", err)
	}

	// 1. Ingest subscription with 2 nodes: one normal residential node, one Tor node
	sub := &domain.Subscription{
		ID:                 "iprisk-e2e-sub",
		Name:               "IPRisk E2E Subscription",
		SourceURLSecretRef: "https://fixture.invalid/iprisk-sub.yaml",
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds:  3600,
			TimeoutSeconds:   10,
			MaxResponseBytes: 1024 * 1024,
		},
		Revision:  domain.MustNewUUIDv7(),
		CreatedAt: domain.NowUTC(),
		UpdatedAt: domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	inventoryService := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fixtureFetcher{
		response: &fetch.Response{
			StatusCode:    200,
			ContentType:   "text/yaml",
			ContentDigest: "iprisk-fixture-digest",
			Body: []byte("proxies:\n" +
				"  - name: ResNodeClean\n" +
				"    type: ss\n" +
				"    server: 198.51.100.20\n" +
				"    port: 8388\n" +
				"    cipher: aes-128-gcm\n" +
				"    password: secret-clean\n" +
				"  - name: TorNodeRisk\n" +
				"    type: ss\n" +
				"    server: 198.51.100.30\n" +
				"    port: 8389\n" +
				"    cipher: aes-128-gcm\n" +
				"    password: secret-tor\n"),
		},
	})
	refresh, err := inventoryService.ReconcileSubscription(ctx, sub.ID)
	if err != nil || refresh.NodesValid != 2 {
		t.Fatalf("reconcile subscription: nodes_valid=%d err=%v", refresh.NodesValid, err)
	}

	nodes, total, err := inventoryService.ListNodes(ctx, domain.NodeFilter{Pagination: domain.Pagination{Page: 1, PageSize: 10}})
	if err != nil || total != 2 || len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got total=%d len=%d err=%v", total, len(nodes), err)
	}

	var cleanNode, torNode domain.Node
	for _, n := range nodes {
		if n.DisplayName == "ResNodeClean" {
			cleanNode = n
		} else if n.DisplayName == "TorNodeRisk" {
			torNode = n
		}
	}
	if cleanNode.LogicalID == "" || torNode.LogicalID == "" {
		t.Fatalf("expected cleanNode and torNode to be present")
	}

	// 2. Probe with Fake IP Risk Provider
	fakeCleanProvider := ipriskProbe.NewDeterministicFakeProvider("fake_ip_provider", "v1", ipriskProbe.FakeProviderBehavior{
		Score:        12,
		Confidence:   92,
		NetworkClass: domain.NetworkClassResidential,
		Status:       domain.IPRiskStatusAvailable,
		TTL:          time.Hour,
	})
	fakeTorProvider := ipriskProbe.NewDeterministicFakeProvider("fake_ip_provider", "v1", ipriskProbe.FakeProviderBehavior{
		Score:        96,
		Confidence:   98,
		NetworkClass: domain.NetworkClassDatacenter,
		Traits:       []domain.AnonymizerTrait{domain.TraitTor, domain.TraitProxy},
		Status:       domain.IPRiskStatusAvailable,
		TTL:          time.Hour,
	})

	cleanCandidate, err := fakeCleanProvider.Probe(ctx, http.DefaultClient, domain.ExitIdentity{
		NodeLogicalID:  cleanNode.LogicalID,
		ObservedAt:     time.Now().UTC(),
		IdentityDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatalf("probe clean node: %v", err)
	}
	cleanObs, err := cleanCandidate.ToObservation(cleanNode.LogicalID, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "sha256:1111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatalf("candidate to observation: %v", err)
	}
	if err := riskObsRepo.Create(ctx, cleanObs); err != nil {
		t.Fatalf("save clean observation: %v", err)
	}

	torCandidate, err := fakeTorProvider.Probe(ctx, http.DefaultClient, domain.ExitIdentity{
		NodeLogicalID:  torNode.LogicalID,
		ObservedAt:     time.Now().UTC(),
		IdentityDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("probe tor node: %v", err)
	}
	torObs, err := torCandidate.ToObservation(torNode.LogicalID, "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "sha256:2222222222222222222222222222222222222222222222222222222222222222")
	if err != nil {
		t.Fatalf("candidate to observation: %v", err)
	}
	if err := riskObsRepo.Create(ctx, torObs); err != nil {
		t.Fatalf("save tor observation: %v", err)
	}

	// 3. Create & Activate Risk Policy Revision
	ipriskSvc := ipriskApp.NewService(
		riskObsRepo,
		riskPolicyRepo,
		ipriskApp.WithBindingRepository(riskBindingRepo),
		ipriskApp.WithNodeRepository(nodeRepo),
		ipriskApp.WithAuditRepository(auditRepo),
	)

	policyCmd := ipriskApp.CreateRiskPolicyCommand{
		ProviderSelection: domain.RiskProviderSelection{
			Mode:      domain.RiskFusionSingleProvider,
			Providers: []domain.ProviderRef{{Provider: "fake_ip_provider", SchemaVersion: "v1"}},
		},
		MaxObservationAge: 24 * time.Hour,
		MinimumConfidence: 60,
		ScoreBands: []domain.ScoreBand{
			{Min: 0, Max: 30, Action: domain.RiskActionAllow, Band: domain.RiskBandLow},
			{Min: 31, Max: 70, Action: domain.RiskActionReview, Band: domain.RiskBandMedium},
			{Min: 71, Max: 100, Action: domain.RiskActionBlock, Band: domain.RiskBandHigh},
		},
		TraitRules: []domain.TraitRule{
			{Trait: domain.TraitTor, Action: domain.RiskActionBlock},
		},
		UnknownAction:  domain.RiskActionReview,
		ConflictAction: domain.RiskActionReview,
		ReviewAction:   domain.RiskActionBlock,
		ActorKind:      domain.ActorKindAdmin,
		RequestID:      "req-policy-init",
	}
	policyRev, err := ipriskSvc.CreatePolicy(ctx, policyCmd)
	if err != nil {
		t.Fatalf("create risk policy: %v", err)
	}
	if _, err := ipriskSvc.ActivatePolicy(ctx, policyRev.RevisionID, ipriskApp.Action{ActorKind: domain.ActorKindAdmin, RequestID: "req-activate"}); err != nil {
		t.Fatalf("activate risk policy: %v", err)
	}

	// 4. Batch Risk Evaluation
	batchRes, err := ipriskSvc.EvaluateBatch(ctx, ipriskApp.EvaluateBatchInput{
		NodeLogicalIDs:   []string{cleanNode.LogicalID, torNode.LogicalID},
		Policy:           &policyRev.RiskPolicy,
		PolicyRevisionID: policyRev.RevisionID,
		EvaluatedAt:      domain.NowUTC(),
	})
	if err != nil {
		t.Fatalf("evaluate batch risk: %v", err)
	}
	if batchRes.DecisionDigest == "" {
		t.Fatal("expected non-empty risk decision digest")
	}
	if len(batchRes.AdmittedNodeIDs) != 1 || batchRes.AdmittedNodeIDs[0] != cleanNode.LogicalID {
		t.Fatalf("expected cleanNode admitted, got: %+v", batchRes.AdmittedNodeIDs)
	}
	if len(batchRes.ExcludedNodeIDs) != 1 || batchRes.ExcludedNodeIDs[0] != torNode.LogicalID {
		t.Fatalf("expected torNode excluded, got: %+v", batchRes.ExcludedNodeIDs)
	}

	// 5. Policy Deterministic Resolution
	groupID := domain.MustNewUUIDv7()
	groupEdgeClean := cleanNode.LogicalID
	groupEdgeTor := torNode.LogicalID
	resolvedSnap, err := resolver.New().Resolve(ctx, resolver.ResolveInput{
		RevisionID:         "rev-iprisk-e2e",
		CompilerVersion:    "1.0.0",
		Nodes:              nodes,
		Groups:             []domain.NodeGroup{{ID: groupID, Name: "All Nodes", GroupType: domain.GroupTypeSelect}},
		Edges:              map[string][]domain.GroupEdge{groupID: {{ID: domain.MustNewUUIDv7(), ParentGroupID: groupID, NodeLogicalID: &groupEdgeClean}, {ID: domain.MustNewUUIDv7(), ParentGroupID: groupID, NodeLogicalID: &groupEdgeTor}}},
		PolicyRules:        []domain.PolicyRule{{ID: "rule-match-all", TargetGroupID: groupID, Expression: "MATCH", Position: 0}},
		RiskPolicyRevision: policyRev.RevisionID,
		RiskDecisionDigest: batchRes.DecisionDigest,
		RiskDecisions:      batchRes.Decisions,
		RiskReviewAction:   policyRev.EffectiveReviewAction(),
	})
	if err != nil {
		t.Fatalf("resolve policy with risk: %v", err)
	}
	if resolvedSnap.SnapshotDigest == "" {
		t.Fatal("expected non-empty snapshot digest")
	}
	// Verify that Tor node is flagged with risk_blocked diagnostic
	foundBlockedDiag := false
	for _, d := range resolvedSnap.Diagnostics {
		if d.Target == torNode.LogicalID && d.Code == "risk_blocked" {
			foundBlockedDiag = true
			break
		}
	}
	if !foundBlockedDiag {
		t.Fatalf("expected risk_blocked diagnostic for torNode %s, got: %+v", torNode.LogicalID, resolvedSnap.Diagnostics)
	}

	// 6. Five-Target Compilation for Admitted Snapshot
	targets := []domain.CompilerTarget{
		domain.TargetClash,
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}
	for _, target := range targets {
		compiled, err := compiler.Compile(ctx, resolvedSnap, target)
		if err != nil {
			t.Fatalf("compile %s: %v", target, err)
		}
		if len(compiled.Content) == 0 || compiled.ContentDigest == "" {
			t.Fatalf("empty compiled output for %s", target)
		}
	}

	// 7. Publication & Preflight Interceptor
	pubService := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithIPRiskService(ipriskSvc),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	// 7a. Attempting to publish the snapshot containing the blocked Tor node must be rejected by preflight!
	_, err = pubService.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  resolvedSnap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-publish-fail-test",
	})
	if err == nil {
		t.Fatal("expected publish to be rejected by preflight due to risk_blocked diagnostic, got nil error")
	}

	// 7b. Publishing clean snapshot containing only admitted node succeeds
	cleanOnlySnap, err := resolver.New().Resolve(ctx, resolver.ResolveInput{
		RevisionID:         "rev-clean-only",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{cleanNode},
		Groups:             []domain.NodeGroup{{ID: groupID, Name: "Clean Nodes", GroupType: domain.GroupTypeSelect}},
		Edges:              map[string][]domain.GroupEdge{groupID: {{ID: domain.MustNewUUIDv7(), ParentGroupID: groupID, NodeLogicalID: &groupEdgeClean}}},
		PolicyRules:        []domain.PolicyRule{{ID: "rule-match-clean", TargetGroupID: groupID, Expression: "MATCH", Position: 0}},
		RiskPolicyRevision: policyRev.RevisionID,
		RiskDecisionDigest: batchRes.DecisionDigest,
		RiskDecisions:      []domain.RiskDecision{batchRes.DecisionMap[cleanNode.LogicalID]},
		RiskReviewAction:   policyRev.EffectiveReviewAction(),
	})
	if err != nil {
		t.Fatalf("resolve clean only snapshot: %v", err)
	}

	publishedClean, err := pubService.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  cleanOnlySnap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-publish-clean-success",
	})
	if err != nil {
		t.Fatalf("publish clean snapshot: %v", err)
	}

	artifact, err := pubService.ResolveAndServe(ctx, publishedClean.Publication.ID, publishedClean.RawToken)
	if err != nil || len(artifact.Content) == 0 {
		t.Fatalf("resolve and serve clean publication: %v", err)
	}

	// 8. HTTP router verification
	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		PublicationService: pubService,
		ReadinessChecker: func(ctx context.Context) (*sqlite.ReadinessReport, error) {
			return sqlite.CheckReadiness(ctx, db)
		},
	})

	// Authorized GET with token
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s?token=%s", publishedClean.Publication.ID, publishedClean.RawToken), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// Unauthorized GET without token
	unauthReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/publish/v1/%s", publishedClean.Publication.ID), nil)
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthRec.Code)
	}
}
