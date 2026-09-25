package integration_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/import/legacy"
	"clash-sub-parser/internal/probe/profiles"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
	"clash-sub-parser/migrations"
)

// ==============================================================================
// Critic Adversarial Verification Suite: 10 Non-UI Dimensions & 7 Core Scenarios
// ==============================================================================

// Helper to open a clean isolated in-memory SQLite DB with all migrations applied.
func openAdversarialDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(sqlite.Config{
		Path:        fmt.Sprintf("file:adversarial_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("open adversarial database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(context.Background()); err != nil {
		t.Fatalf("apply adversarial migrations: %v", err)
	}
	return db
}

// ------------------------------------------------------------------------------
// Dimension 1: 零订阅边界与冷启动极限 (Zero Subscription & Empty Boundary)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension1_ZeroSubscriptionAndEmptyBoundary(t *testing.T) {
	ctx := context.Background()
	db := openAdversarialDB(t)

	// Verify Readiness on clean empty DB
	readiness, err := sqlite.CheckReadiness(ctx, db)
	if err != nil || !readiness.Ready {
		t.Fatalf("clean DB must be ready: ready=%v err=%v report=%+v", readiness.Ready, err, readiness)
	}

	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	invSvc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, nil)

	// 1. List on zero subscriptions from repo
	subs, totalSubs, err := subRepo.List(ctx, domain.SubscriptionFilter{Pagination: domain.Pagination{Page: 1, PageSize: 10}})
	if err != nil || totalSubs != 0 || len(subs) != 0 {
		t.Fatalf("expected 0 subscriptions on cold start, got total=%d len=%d err=%v", totalSubs, len(subs), err)
	}

	// 2. List on zero nodes
	nodes, totalNodes, err := invSvc.ListNodes(ctx, domain.NodeFilter{Pagination: domain.Pagination{Page: 1, PageSize: 10}})
	if err != nil || totalNodes != 0 || len(nodes) != 0 {
		t.Fatalf("expected 0 nodes on cold start, got total=%d len=%d err=%v", totalNodes, len(nodes), err)
	}

	// 3. Resolve policy snapshot on zero nodes
	groupID := domain.MustNewUUIDv7()
	emptyInput := resolver.ResolveInput{
		RevisionID:      "empty-revision",
		CompilerVersion: "1.0.0",
		Nodes:           []domain.Node{},
		Groups: []domain.NodeGroup{
			{ID: groupID, Name: "PROXY", GroupType: domain.GroupTypeSelect},
		},
		Edges:       map[string][]domain.GroupEdge{groupID: {}},
		PolicyRules: []domain.PolicyRule{{ID: "rule-empty", TargetGroupID: groupID, Expression: "MATCH", Position: 0}},
	}
	snap, err := resolver.New().Resolve(ctx, emptyInput)
	if err != nil {
		t.Fatalf("resolver must not fail on 0 nodes: %v", err)
	}
	if snap == nil || snap.SnapshotDigest == "" {
		t.Fatalf("resolver must produce valid snapshot digest for 0 nodes, got nil or empty")
	}
	if len(snap.NodeLogicalIDs) != 0 {
		t.Fatalf("expected 0 active nodes in snapshot, got %d", len(snap.NodeLogicalIDs))
	}

	// 4. Compile 5 targets on empty snapshot
	targets := []domain.CompilerTarget{
		domain.TargetClash,
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}
	for _, target := range targets {
		out, err := compiler.Compile(ctx, snap, target)
		if err != nil {
			t.Fatalf("compiler must cleanly handle 0 nodes for target %s: %v", target, err)
		}
		if len(out.Content) == 0 {
			t.Fatalf("compiled content must not be empty for target %s", target)
		}
	}
}

// ------------------------------------------------------------------------------
// Dimension 2: 节点突发海量下线与最后源收敛 (Mass Sudden Offline & Tombstone Convergence)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension2_MassSuddenNodeOfflineAndConvergence(t *testing.T) {
	ctx := context.Background()
	db := openAdversarialDB(t)

	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)

	subA := &domain.Subscription{
		ID:                 "sub-mass-a",
		Name:               "Mass Provider A",
		SourceURLSecretRef: "https://provider-a.invalid/sub.yaml",
		Enabled:            true,
		RefreshPolicy:      domain.RefreshPolicy{IntervalSeconds: 3600, TimeoutSeconds: 10, MaxResponseBytes: 1024 * 1024},
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	subB := &domain.Subscription{
		ID:                 "sub-mass-b",
		Name:               "Mass Provider B (Shared Survivor)",
		SourceURLSecretRef: "https://provider-b.invalid/sub.yaml",
		Enabled:            true,
		RefreshPolicy:      domain.RefreshPolicy{IntervalSeconds: 3600, TimeoutSeconds: 10, MaxResponseBytes: 1024 * 1024},
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, subA); err != nil {
		t.Fatalf("create subA: %v", err)
	}
	if err := subRepo.Create(ctx, subB); err != nil {
		t.Fatalf("create subB: %v", err)
	}

	// Generate 100 nodes for Sub A
	var yamlA strings.Builder
	yamlA.WriteString("proxies:\n")
	for i := 1; i <= 100; i++ {
		yamlA.WriteString(fmt.Sprintf("  - name: Node-%03d\n    type: ss\n    server: 198.51.100.%d\n    port: 8388\n    cipher: aes-128-gcm\n    password: secret-%03d\n", i, (i%250)+1, i))
	}

	// Sub B shares Node-100 and has Node-B01
	yamlB := "proxies:\n" +
		"  - name: Node-100\n    type: ss\n    server: 198.51.100.100\n    port: 8388\n    cipher: aes-128-gcm\n    password: secret-100\n" +
		"  - name: Node-B01\n    type: ss\n    server: 198.51.100.201\n    port: 8388\n    cipher: aes-128-gcm\n    password: secret-b01\n"

	fetcher := &dynamicFetcher{responses: make(map[string]*fetch.Response)}
	fetcher.responses[subA.SourceURLSecretRef] = &fetch.Response{
		StatusCode: 200, ContentType: "text/yaml", ContentDigest: "digest-a1", Body: []byte(yamlA.String()),
	}
	fetcher.responses[subB.SourceURLSecretRef] = &fetch.Response{
		StatusCode: 200, ContentType: "text/yaml", ContentDigest: "digest-b1", Body: []byte(yamlB),
	}

	invSvc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// Ingest Sub A (100 nodes)
	resA, err := invSvc.ReconcileSubscription(ctx, subA.ID)
	if err != nil || resA.NodesValid != 100 {
		t.Fatalf("reconcile subA initial: err=%v res=%+v", err, resA)
	}

	// Ingest Sub B (2 nodes: Node-100 shared, Node-B01 exclusive)
	resB, err := invSvc.ReconcileSubscription(ctx, subB.ID)
	if err != nil || resB.NodesValid != 2 {
		t.Fatalf("reconcile subB initial: err=%v res=%+v", err, resB)
	}

	// Check total active nodes: 100 + 1 = 101 (MaxPageSize is 100, so page 1 has 100 items, total has 101)
	activeNodesPage1, totalActive, err := invSvc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly: true,
		Pagination: domain.Pagination{Page: 1, PageSize: 100},
	})
	if err != nil || totalActive != 101 || len(activeNodesPage1) != 100 {
		t.Fatalf("expected 101 active nodes total (page 1 len 100), got total=%d len=%d err=%v", totalActive, len(activeNodesPage1), err)
	}
	activeNodesPage2, _, err := invSvc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly: true,
		Pagination: domain.Pagination{Page: 2, PageSize: 100},
	})
	if err != nil || len(activeNodesPage2) != 1 {
		t.Fatalf("expected 1 node on page 2, got len=%d err=%v", len(activeNodesPage2), err)
	}

	// --- MASS SUDDEN OFFLINE ADVERSARIAL EVENT (Case 2A: Valid parse, 99 nodes dropped) ---
	// Sub A upstream suddenly drops 99 nodes, returning only Node-100 (shared) and a new Node-A-Solo
	yamlADropped := "proxies:\n" +
		"  - name: Node-100\n    type: ss\n    server: 198.51.100.100\n    port: 8388\n    cipher: aes-128-gcm\n    password: secret-100\n"
	fetcher.responses[subA.SourceURLSecretRef] = &fetch.Response{
		StatusCode: 200, ContentType: "text/yaml", ContentDigest: "digest-a-dropped", Body: []byte(yamlADropped),
	}
	resA2, err := invSvc.ReconcileSubscription(ctx, subA.ID)
	if err != nil || resA2.NodesValid != 1 {
		t.Fatalf("reconcile subA dropped: err=%v res=%+v", err, resA2)
	}

	// Verify Convergence:
	// Exclusive nodes 1..99 must be tombstoned (active = 0)
	// Node-100 was also in Sub B, so it MUST stay active!
	// Node-B01 was in Sub B, so it stays active!
	// Total active nodes MUST be exactly 2 (Node-100 and Node-B01)!
	activeAfter, totalAfter, err := invSvc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly: true,
		Pagination: domain.Pagination{Page: 1, PageSize: 200},
	})
	if err != nil || totalAfter != 2 || len(activeAfter) != 2 {
		t.Fatalf("expected exactly 2 active nodes surviving mass offline, got total=%d len=%d err=%v", totalAfter, len(activeAfter), err)
	}

	// Historical all-nodes query (ActiveOnly=false) must still preserve all 101 nodes for audit/evidence!
	allNodes, totalAll, err := invSvc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly: false,
		Pagination: domain.Pagination{Page: 1, PageSize: 200},
	})
	if err != nil || totalAll != 101 || len(allNodes) != 100 { // Page 1 has 100 items out of 101
		t.Fatalf("historical node ledger must preserve all 101 nodes, got total=%d len=%d err=%v", totalAll, len(allNodes), err)
	}

	// --- UPSTREAM EMPTY / INVALID PROTECTION TEST (Case 2B: Empty proxies: [] rejected by parser) ---
	// If upstream returns empty proxies: [], parser returns no_supported_nodes, which fails fetch but PROTECTS inventory from wipeout!
	fetcher.responses[subA.SourceURLSecretRef] = &fetch.Response{
		StatusCode: 200, ContentType: "text/yaml", ContentDigest: "digest-a-empty", Body: []byte("proxies: []\n"),
	}
	_, err = invSvc.ReconcileSubscription(ctx, subA.ID)
	if err == nil {
		t.Fatalf("expected parser rejection on empty proxies: [], got nil")
	}

	// Active nodes must STILL remain 2 (protected against empty-body wipeout!)
	activeAfterEmpty, totalAfterEmpty, err := invSvc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly: true,
		Pagination: domain.Pagination{Page: 1, PageSize: 200},
	})
	if err != nil || totalAfterEmpty != 2 || len(activeAfterEmpty) != 2 {
		t.Fatalf("empty response parse failure must preserve existing inventory: total=%d err=%v", totalAfterEmpty, err)
	}

	// --- NETWORK FAILURE INVARIANCE ADVERSARIAL TEST ---
	// Upstream Sub B fails with network error / 502. Inventory MUST NOT change!
	fetcher.responses[subB.SourceURLSecretRef] = nil
	fetcher.errors = map[string]error{subB.SourceURLSecretRef: fmt.Errorf("HTTP 502 Bad Gateway")}
	_, err = invSvc.ReconcileSubscription(ctx, subB.ID)
	if err == nil {
		t.Fatalf("expected reconcile error on 502, got nil")
	}

	// Active nodes must STILL remain 2 (last known good state protected)
	activeAfterErr, totalAfterErr, err := invSvc.ListNodes(ctx, domain.NodeFilter{
		ActiveOnly: true,
		Pagination: domain.Pagination{Page: 1, PageSize: 200},
	})
	if err != nil || totalAfterErr != 2 || len(activeAfterErr) != 2 {
		t.Fatalf("failed fetch must preserve last known good inventory: total=%d err=%v", totalAfterErr, err)
	}
}

// ------------------------------------------------------------------------------
// Dimension 3: 探测队列过载拥塞与并发滑动窗口 (Probe Queue Overload & Concurrency Bounding)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension3_ProbeQueueOverloadAndBackpressure(t *testing.T) {
	// 1. Concurrency boundary check (Must be between 10 and 32)
	_, errLow := queue.NewScheduler(queue.Config{Concurrency: 4})
	if errLow != queue.ErrInvalidConcurrency {
		t.Fatalf("concurrency < 10 must return ErrInvalidConcurrency, got %v", errLow)
	}
	_, errHigh := queue.NewScheduler(queue.Config{Concurrency: 40})
	if errHigh != queue.ErrInvalidConcurrency {
		t.Fatalf("concurrency > 32 must return ErrInvalidConcurrency, got %v", errHigh)
	}

	// 2. Queue capacity and backpressure
	const qSize = 10
	const concurrency = 10
	sched, err := queue.NewScheduler(queue.Config{
		Concurrency: concurrency,
		// Use the scheduler's default per-run budget; do not disable fairness in
		// an integration test that exercises the global concurrency boundary.
		QueueSize: qSize,
		NodeTTL:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	defer sched.Close()

	// Fill workers with slow blocking tasks
	blockCh := make(chan struct{})
	var activeCount atomic.Int32
	var maxObservedConcurrency atomic.Int32

	for i := 1; i <= concurrency; i++ {
		nodeID := fmt.Sprintf("node-block-%d", i)
		err := sched.Submit(queue.Task{
			ID:        fmt.Sprintf("task-block-%d", i),
			RunID:     fmt.Sprintf("active-run-%d", i),
			LogicalID: nodeID,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(ctx context.Context) error {
				curr := activeCount.Add(1)
				for {
					old := maxObservedConcurrency.Load()
					if curr <= old || maxObservedConcurrency.CompareAndSwap(old, curr) {
						break
					}
				}
				defer activeCount.Add(-1)
				select {
				case <-blockCh:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		})
		if err != nil {
			t.Fatalf("failed submitting initial concurrency task %d: %v", i, err)
		}
	}

	// Wait for workers to claim tasks
	time.Sleep(50 * time.Millisecond)

	// Now fill the waiting queue up to qSize
	for i := 1; i <= qSize; i++ {
		nodeID := fmt.Sprintf("node-queue-%d", i)
		err := sched.Submit(queue.Task{
			ID:        fmt.Sprintf("task-queue-%d", i),
			RunID:     fmt.Sprintf("queued-run-%d", i),
			LogicalID: nodeID, // allow the default per-run cap to exercise queue capacity
			Kind:      domain.ProbeKindBaseline,
			Execute: func(ctx context.Context) error {
				return nil
			},
		})
		if err != nil {
			t.Fatalf("submitting task %d within capacity failed: %v", i, err)
		}
	}

	// The queue is now full. Enqueuing one more task MUST trigger ErrCapacityExceeded!
	overflowErr := sched.Submit(queue.Task{
		ID:        "task-overflow",
		RunID:     "overflow-run",
		LogicalID: "node-overflow",
		Kind:      domain.ProbeKindBaseline,
		Execute:   func(ctx context.Context) error { return nil },
	})
	if overflowErr != queue.ErrCapacityExceeded {
		t.Fatalf("expected ErrCapacityExceeded on saturated queue, got: %v", overflowErr)
	}

	// 3. Node conflict check within TTL
	conflictErr := sched.Submit(queue.Task{
		ID:        "task-conflict",
		RunID:     "run-overload-1",
		LogicalID: "node-block-1", // already active
		Kind:      domain.ProbeKindBaseline,
		Execute:   func(ctx context.Context) error { return nil },
	})
	if conflictErr != queue.ErrNodeConflict {
		t.Fatalf("expected ErrNodeConflict for concurrently active node, got: %v", conflictErr)
	}

	// Release blocking tasks and verify max concurrency never exceeded configured bound
	close(blockCh)
	time.Sleep(100 * time.Millisecond)

	if maxObservedConcurrency.Load() > concurrency {
		t.Fatalf("max observed concurrency %d exceeded hard limit %d", maxObservedConcurrency.Load(), concurrency)
	}
}

// ------------------------------------------------------------------------------
// Dimension 4: 协程生命周期与上下文取消级联防泄漏 (Context Cancellation Cascade)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension4_ContextCancellationCascade(t *testing.T) {
	sched, err := queue.NewScheduler(queue.Config{
		Concurrency: 10,
		QueueSize:   50,
	})
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	var cancelledTasks atomic.Int32
	var completedTasks atomic.Int32

	const taskCount = 10
	started := make(chan struct{}, taskCount)
	for i := 0; i < taskCount; i++ {
		nodeID := fmt.Sprintf("node-cancel-%d", i)
		err := sched.Submit(queue.Task{
			ID:        fmt.Sprintf("task-cancel-%d", i),
			RunID:     "run-cascade-test",
			LogicalID: nodeID,
			Kind:      domain.ProbeKindBaseline,
			Context:   runCtx,
			Execute: func(ctx context.Context) error {
				started <- struct{}{}
				select {
				case <-time.After(2 * time.Second):
					completedTasks.Add(1)
					return nil
				case <-ctx.Done():
					cancelledTasks.Add(1)
					return ctx.Err()
				}
			},
		})
		if err != nil {
			t.Fatalf("submit task %d: %v", i, err)
		}
	}

	// Synchronize on actual execution rather than assuming dispatch timing.
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no task started before cancellation")
	}
	cancelRun()

	// Cancel run in scheduler
	sched.CancelRun("run-cascade-test")

	// Close scheduler cleanly
	closeDone := make(chan struct{})
	go func() {
		_ = sched.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		// Succeeded cleanly
	case <-time.After(3 * time.Second):
		t.Fatalf("scheduler.Close() hung after cancellation; goroutine leak detected")
	}
	sched.Wait()

	if cancelledTasks.Load() == 0 {
		t.Fatalf("expected tasks to be cancelled by context cascade, got cancelled=%d completed=%d", cancelledTasks.Load(), completedTasks.Load())
	}
}

// ------------------------------------------------------------------------------
// Dimension 5: 上游 Provider 契约漂移与假阳性拦截 (Provider Contract Drift & Anti-False-Positive)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension5_ProviderContractDriftAndAntiFalsePositive(t *testing.T) {
	profile := profiles.Profile{
		Kind:     domain.ProbeKindStreaming,
		Version:  "streaming-v1",
		Contract: "netflix-v1",
		MaxAge:   10 * time.Minute,
	}
	if err := profile.Validate(); err != nil {
		t.Fatalf("validate profile: %v", err)
	}

	// Case 1: Cloudflare Challenge / Turnstile with HTTP 200 (Common stealth block)
	resChallenge := profiles.Result{
		StatusCode:      200,
		Body:            []byte("<html><title>Just a moment...</title><body><div class=\"cf-turnstile\">verify you are human</div></body></html>"),
		ContractMatched: false,
		ContractVersion: "netflix-v1",
	}
	evalChallenge := profile.Evaluate(resChallenge)
	if evalChallenge.Verdict != domain.VerdictRestricted || evalChallenge.Reason != "access_restricted" {
		t.Fatalf("challenge body with HTTP 200 must yield VerdictRestricted/access_restricted, got: %+v", evalChallenge)
	}

	// Case 2: Upstream contract drift (HTTP 200, but schema/contract mismatch)
	resDrift := profiles.Result{
		StatusCode:      200,
		Body:            []byte(`{"unexpected":"payload","format":"changed"}`),
		ContractMatched: false, // Contract matcher rejected unexpected structure
		ContractVersion: "unknown-v2",
	}
	evalDrift := profile.Evaluate(resDrift)
	if evalDrift.Verdict != domain.VerdictUnknown || evalDrift.Reason != "contract_drift" {
		t.Fatalf("contract drift must yield VerdictUnknown/contract_drift, got: %+v", evalDrift)
	}

	// Case 3: HTTP 200 without contract matching must NEVER be VerdictAvailable!
	resFake200 := profiles.Result{
		StatusCode:      200,
		Body:            []byte("Random 200 OK without valid evidence"),
		ContractMatched: false,
	}
	evalFake200 := profile.Evaluate(resFake200)
	if evalFake200.Verdict == domain.VerdictAvailable {
		t.Fatalf("HTTP 200 alone must NEVER produce VerdictAvailable!")
	}

	// Case 4: IPRisk Missing Exit Identity
	ipRiskProfile := profiles.Profile{
		Kind:     domain.ProbeKindIPRisk,
		Version:  "ip-risk-v1",
		Contract: "ip-risk-v1",
		MaxAge:   10 * time.Minute,
	}
	evalIPRiskMissing := ipRiskProfile.Evaluate(profiles.Result{
		StatusCode:          200,
		ExitIdentityMissing: true,
	})
	if evalIPRiskMissing.Verdict != domain.VerdictUnknown || evalIPRiskMissing.Reason != "missing_exit_identity" {
		t.Fatalf("IPRisk missing exit identity must be VerdictUnknown/missing_exit_identity, got: %+v", evalIPRiskMissing)
	}

	// Case 5: Speed Probe Budget Overrun
	speedProfile := profiles.Profile{
		Kind:     domain.ProbeKindSpeed,
		Version:  "speed-v1",
		Contract: "speed-v1",
		MaxAge:   10 * time.Minute,
		SpeedBudget: profiles.SpeedBudget{
			OptInRequired:      true,
			MaxBytesPerRequest: 10 * 1024 * 1024,
			MaxBytesPerRun:     20 * 1024 * 1024,
			Deadline:           5 * time.Second,
		},
	}
	evalSpeedExceeded := speedProfile.Evaluate(profiles.Result{
		StatusCode: 200,
		OptIn:      true,
		BytesRead:  25 * 1024 * 1024, // Exceeds MaxBytesPerRun!
	})
	if evalSpeedExceeded.Verdict != domain.VerdictError || evalSpeedExceeded.Reason != "speed_budget_exceeded" {
		t.Fatalf("speed test budget overrun must yield VerdictError/speed_budget_exceeded, got: %+v", evalSpeedExceeded)
	}
}

// ------------------------------------------------------------------------------
// Dimension 6: 数据库脏状态、坏版本与外键断裂拦截 (Dirty DB, Corrupt Schema & FK Violation Interception)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension6_StartupSchemaAnomalyAndFKCheck(t *testing.T) {
	ctx := context.Background()

	// 1. Missing table anomaly
	dbMissing := openAdversarialDB(t)
	if _, err := dbMissing.ExecContext(ctx, "DROP TABLE publications;"); err != nil {
		t.Fatalf("drop table publications: %v", err)
	}
	repMissing, err := sqlite.CheckReadiness(ctx, dbMissing)
	if err != nil {
		t.Fatalf("check readiness error: %v", err)
	}
	if repMissing.Ready {
		t.Fatalf("database missing publications table must NOT be ready")
	}
	var hasPubTable bool
	for _, tbl := range repMissing.MissingTables {
		if tbl == "publications" {
			hasPubTable = true
		}
	}
	if !hasPubTable {
		t.Fatalf("expected 'publications' in MissingTables, got: %v", repMissing.MissingTables)
	}

	// 2. Foreign Key Violation Detection
	dbFK := openAdversarialDB(t)
	// Temporarily bypass FK enforcement to inject a broken relation, then re-enable
	if _, err := dbFK.ExecContext(ctx, "PRAGMA foreign_keys = OFF;"); err != nil {
		t.Fatalf("disable fk: %v", err)
	}
	// Insert orphaned node_sources referencing non-existent node
	_, err = dbFK.ExecContext(ctx, "INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('phantom-node', 'sub-x', 'fetch-x');")
	if err != nil {
		t.Fatalf("insert orphaned relation: %v", err)
	}
	if _, err := dbFK.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("enable fk: %v", err)
	}

	repFK, err := sqlite.CheckReadiness(ctx, dbFK)
	if err != nil {
		t.Fatalf("check readiness fk error: %v", err)
	}
	if repFK.Ready {
		t.Fatalf("database with FK violation must NOT be ready")
	}
	if len(repFK.ForeignKeyViolations) == 0 {
		t.Fatalf("expected PRAGMA foreign_key_check to report violations, got 0")
	}
	if repFK.ForeignKeyViolations[0].Table != "node_sources" {
		t.Fatalf("expected FK violation on node_sources, got: %+v", repFK.ForeignKeyViolations[0])
	}
}

// ------------------------------------------------------------------------------
// Dimension 7: 高并发发布撤销竞争与零竞态 (High Concurrency Publication Revocation Race)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension7_PublishRevokeConcurrencyRace(t *testing.T) {
	ctx := context.Background()
	db := openAdversarialDB(t)

	pubRepo := sqlite.NewPublicationRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	pubSvc := publication.NewService(pubRepo, auditRepo)

	groupID := domain.MustNewUUIDv7()
	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.1", 8388, nil)
	snap, err := resolver.New().Resolve(ctx, resolver.ResolveInput{
		RevisionID:      "rev-race-test",
		CompilerVersion: "1.0.0",
		Nodes: []domain.Node{{
			LogicalID:   nodeID,
			Protocol:    domain.ProtocolSS,
			DisplayName: "Race Node",
			Active:      true,
		}},
		Groups:      []domain.NodeGroup{{ID: groupID, Name: "PROXY", GroupType: domain.GroupTypeSelect}},
		Edges:       map[string][]domain.GroupEdge{groupID: {{ID: domain.MustNewUUIDv7(), ParentGroupID: groupID, NodeLogicalID: &nodeID}}},
		PolicyRules: []domain.PolicyRule{{ID: "rule-race", TargetGroupID: groupID, Expression: "MATCH", Position: 0}},
	})
	if err != nil {
		t.Fatalf("resolve policy: %v", err)
	}

	// Create an active publication
	published, err := pubSvc.Publish(ctx, publication.PublishCommand{
		Target:     domain.TargetClash,
		RevisionID: "rev-race-test",
		Snapshot:   snap,
		ActorKind:  domain.ActorKindAdmin,
		RequestID:  "req-race-create",
	})
	if err != nil {
		t.Fatalf("create publication: %v", err)
	}

	// Verify it can be served initially
	artifact, err := pubSvc.ResolveAndServe(ctx, published.Publication.ID, published.RawToken)
	if err != nil || artifact == nil {
		t.Fatalf("initial serve failed: %v", err)
	}

	// Run concurrent read workers and concurrent revokers
	const readerCount = 20
	const iterations = 50
	var wg sync.WaitGroup
	var revokedSuccess atomic.Int32
	var rejectedAfterRevoke atomic.Int32

	// Readers
	for i := 0; i < readerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := pubSvc.ResolveAndServe(ctx, published.Publication.ID, published.RawToken)
				if err == publication.ErrRevoked {
					rejectedAfterRevoke.Add(1)
				}
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	// Revoker (will trigger mid-flight)
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		_, err := pubSvc.Revoke(ctx, publication.RevokeCommand{
			ID:        published.Publication.ID,
			ActorKind: domain.ActorKindAdmin,
			RequestID: "req-race-revoke",
		})
		if err == nil {
			revokedSuccess.Add(1)
		}
	}()

	wg.Wait()

	if revokedSuccess.Load() == 0 {
		t.Fatalf("revocation failed during concurrent race")
	}

	// After revocation completes, EVERY call must be rejected with ErrRevoked
	_, finalErr := pubSvc.ResolveAndServe(ctx, published.Publication.ID, published.RawToken)
	if finalErr != publication.ErrRevoked {
		t.Fatalf("expected ErrRevoked after revocation, got: %v", finalErr)
	}
}

// ------------------------------------------------------------------------------
// Dimension 8: 离线导入只读物理隔离与隔离区审查 (Offline Import ReadOnly Isolation & Quarantine)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension8_OfflineImportReadOnlyIsolationAndQuarantine(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	sourceDBPath := filepath.Join(tempDir, "legacy_source.db")

	// 1. Create a legacy SQLite database with credentials and raw fields
	legacyDB, err := sql.Open("sqlite", sourceDBPath)
	if err != nil {
		t.Fatalf("create legacy sqlite: %v", err)
	}
	initSQL := `
	CREATE TABLE subscriptions (
		id TEXT PRIMARY KEY,
		name TEXT,
		url TEXT,
		token TEXT,
		raw_nodes TEXT
	);
	CREATE TABLE nodes (
		id INTEGER PRIMARY KEY,
		name TEXT,
		type TEXT,
		server TEXT,
		port INTEGER,
		password TEXT,
		uuid TEXT,
		private_key TEXT
	);
	INSERT INTO subscriptions VALUES ('sub-1', 'Legacy Sub', 'https://example.com/sub?token=secret123', 'bearer-token-abc', 'raw-blob-secret');
	INSERT INTO nodes VALUES (1, 'Legacy-SS-01', 'ss', '198.51.100.1', 8388, 'top-secret-pwd', '', '');
	INSERT INTO nodes VALUES (2, 'Legacy-Vmess-02', 'vmess', '198.51.100.2', 443, '', 'secret-uuid-xyz', 'priv-key-123');
	`
	if _, err := legacyDB.Exec(initSQL); err != nil {
		t.Fatalf("populate legacy sqlite: %v", err)
	}
	_ = legacyDB.Close()

	// Compute initial hash of legacy source file
	initialHash := fileSHA256(t, sourceDBPath)

	// 2. Verify strict read-only mode rejection: opening mode=ro MUST reject writes
	roDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", sourceDBPath))
	if err != nil {
		t.Fatalf("open ro db: %v", err)
	}
	_, writeErr := roDB.Exec("INSERT INTO subscriptions VALUES ('sub-evil', 'Evil', 'url', 'tok', 'blob');")
	_ = roDB.Close()
	if writeErr == nil {
		t.Fatalf("expected write error on mode=ro database, got nil!")
	}

	// 3. Run legacy allowlist importer into target clean DB
	targetDBPath := filepath.Join(tempDir, "target_clean.db")
	targetDB, err := sqlite.Open(sqlite.Config{
		Path:        targetDBPath,
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("open target db: %v", err)
	}
	if err := sqlite.NewMigrationRunner(targetDB, migrations.FS).Run(ctx); err != nil {
		t.Fatalf("migrate target db: %v", err)
	}

	importer := legacy.NewImporter()
	report, err := importer.Import(ctx, legacy.Options{
		SourcePath: sourceDBPath,
		TargetPath: targetDBPath,
		DryRun:     false,
	})
	if err != nil {
		t.Fatalf("legacy import execution failed: %v", err)
	}

	// Verify source hash remains 100% untouched
	afterHash := fileSHA256(t, sourceDBPath)
	if initialHash != afterHash {
		t.Fatalf("legacy source database was mutated! initial=%s after=%s", initialHash, afterHash)
	}

	// Verify Quarantine captures:
	// raw_nodes (blob), token, password, uuid, private_key
	if report.Counts.QuarantineCount == 0 {
		t.Fatalf("expected secrets/blobs to be quarantined, got count 0")
	}

	// Verify Target DB contains NO plain-text secrets
	var leakCount int
	err = targetDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes WHERE display_name LIKE '%secret%' OR normalized_config_secret_ref LIKE '%top-secret%'").Scan(&leakCount)
	if err != nil || leakCount != 0 {
		t.Fatalf("plain-text secrets found in target nodes table! leakCount=%d err=%v", leakCount, err)
	}
	_ = targetDB.Close()
}

// ------------------------------------------------------------------------------
// Dimension 9: 未激活 Draft 配置版本隔离 (Unactivated Draft Revision Isolation)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension9_UnactivatedDraftRevisionIsolation(t *testing.T) {
	ctx := context.Background()
	db := openAdversarialDB(t)

	auditRepo := sqlite.NewAuditRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	revSvc := revision.NewService(revRepo, auditRepo)

	// Insert unactivated draft revision
	draftRev := &domain.ConfigurationRevision{
		ID:            "draft-quarantine-rev-01",
		State:         domain.RevisionStateDraft,
		ContentDigest: "digest-draft-01",
		CreatedAt:     domain.NowUTC(),
	}
	if err := revRepo.Create(ctx, draftRev); err != nil {
		t.Fatalf("create draft revision: %v", err)
	}

	// 1. GetActive MUST NOT return draft revision!
	active, err := revSvc.GetActive(ctx)
	if err != nil {
		de, ok := domain.AsDomainError(err)
		if !ok || de.Category != domain.CategoryNotFound {
			t.Fatalf("expected CategoryNotFound error querying active revision, got: %v", err)
		}
	}
	if active != nil && active.ID == draftRev.ID {
		t.Fatalf("unactivated draft revision was returned as active runtime configuration!")
	}

	// 2. Draft review maintains draft state without activating
	reviewed, err := revSvc.Review(ctx, draftRev.ID, revision.Action{
		RequestID: "req-rev-test",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil || reviewed.State != domain.RevisionStateDraft {
		t.Fatalf("review must preserve draft state: err=%v rev=%+v", err, reviewed)
	}

	// Still not active
	activeAfterReview, _ := revSvc.GetActive(ctx)
	if activeAfterReview != nil && activeAfterReview.ID == draftRev.ID {
		t.Fatalf("draft became active after review without explicit activation!")
	}
}

// ------------------------------------------------------------------------------
// Dimension 10: SSRF、本地环回与私有网段全方位防御 (Full Spectrum SSRF & Loopback Defense)
// ------------------------------------------------------------------------------
func TestAdversarial_Dimension10_FullSpectrumSSRFDefense(t *testing.T) {
	ctx := context.Background()
	policy := fetch.DefaultPolicy()

	adversarialTargets := []string{
		"http://127.0.0.1:8080/evil",
		"http://127.0.0.2:8080/evil",
		"http://localhost:8080/evil",
		"http://[::1]:8080/evil",
		"http://10.0.0.1/internal",
		"http://10.254.254.254/internal",
		"http://172.16.0.1/admin",
		"http://172.31.255.255/admin",
		"http://192.168.1.1/router",
		"http://169.254.169.254/latest/meta-data", // Cloud metadata
		"http://0.0.0.0:8080/",
	}

	for _, target := range adversarialTargets {
		u, err := url.Parse(target)
		if err != nil {
			t.Fatalf("failed parsing test url %s: %v", target, err)
		}
		err = policy.ValidateTarget(ctx, u)
		if err == nil {
			t.Fatalf("CRITICAL SECURITY HOLE: SSRF target %s was NOT blocked by policy!", target)
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "ssrf_blocked" {
			t.Fatalf("expected DomainError(ssrf_blocked) for target %s, got: %v", target, err)
		}
	}
}

// ==============================================================================
// Test Helpers & Mocks
// ==============================================================================

type dynamicFetcher struct {
	responses map[string]*fetch.Response
	errors    map[string]error
}

func (f *dynamicFetcher) Fetch(ctx context.Context, opts fetch.Options) (*fetch.Response, error) {
	if f.errors != nil {
		if err, ok := f.errors[opts.URL]; ok && err != nil {
			return nil, err
		}
	}
	if resp, ok := f.responses[opts.URL]; ok && resp != nil {
		return resp, nil
	}
	return &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/yaml",
		ContentDigest: "default-digest",
		Body:          []byte("proxies: []\n"),
	}, nil
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
