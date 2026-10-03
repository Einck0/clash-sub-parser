package probe_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

// setupPipelineSQLite initializes a real SQLite database with all migrations applied.
func setupPipelineSQLite(t *testing.T) *sql.DB {
	t.Helper()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:test_pipeline_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to run migrations on test db: %v", err)
	}
	return db
}

// 1. alive failure skips media & speed
func TestPipelineAliveFailureSkipsMediaAndSpeed(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_pass"] = domain.Node{LogicalID: "node_pass", Protocol: domain.ProtocolSS, Active: true}
	nodesRepo.items["node_dead"] = domain.Node{LogicalID: "node_dead", Protocol: domain.ProtocolVMess, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 20})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var reqMu sync.Mutex
	nodeRequests := make(map[string][]string)

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					reqMu.Lock()
					nodeRequests[node.LogicalID] = append(nodeRequests[node.LogicalID], req.URL.String())
					reqMu.Unlock()

					// node_dead returns 503 for generate_204 baseline
					if node.LogicalID == "node_dead" {
						return &http.Response{
							StatusCode: http.StatusServiceUnavailable,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader("service unavailable")),
							Request:    req,
						}, nil
					}

					// node_pass returns valid 204 for baseline, 200 for others
					if strings.Contains(req.URL.Path, "generate_204") {
						return &http.Response{
							StatusCode: http.StatusNoContent,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader("")),
							Request:    req,
						}, nil
					}
					if strings.Contains(req.URL.Path, "__down") {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 4096))),
							Request:    req,
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader("<html>ok</html>")),
						Request:    req,
					}, nil
				}),
			}
			return client, nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:         "run_alive_funnel",
		State:      domain.ProbeRunStateQueued,
		DeadlineAt: time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	kinds := []domain.ProbeKind{
		domain.ProbeKindBaseline,
		domain.ProbeKindStreaming,
		domain.ProbeKindSpeed,
	}
	if err := runner.Run(context.Background(), run, []string{"node_pass", "node_dead"}, kinds); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}

	reqMu.Lock()
	deadReqs := len(nodeRequests["node_dead"])
	passReqs := len(nodeRequests["node_pass"])
	reqMu.Unlock()

	// node_dead must only execute baseline (1 request) and NEVER enter stage 2 or stage 3
	if deadReqs != 1 {
		t.Fatalf("expected node_dead to have exactly 1 request (baseline), got %d: %v", deadReqs, nodeRequests["node_dead"])
	}
	if passReqs < 3 {
		t.Fatalf("expected node_pass to have at least 3 requests, got %d", passReqs)
	}

	obs, err := obsRepo.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var deadObsCount, passObsCount int
	for _, o := range obs {
		if o.NodeLogicalID == "node_dead" {
			deadObsCount++
			if o.Verdict == domain.VerdictAvailable {
				t.Fatalf("dead node observation should not be available: %+v", o)
			}
		}
		if o.NodeLogicalID == "node_pass" {
			passObsCount++
		}
	}
	if deadObsCount != 1 {
		t.Fatalf("expected 1 observation for node_dead, got %d", deadObsCount)
	}
	if passObsCount != 3 {
		t.Fatalf("expected 3 observations for node_pass (baseline, streaming, speed), got %d", passObsCount)
	}
}

// 2. 阶段2结束才阶段3 (Stage 2 finishes completely before Stage 3 begins)
func TestPipelinePhase2FinishesBeforePhase3Starts(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_sync"] = domain.Node{LogicalID: "node_sync", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var (
		stage2Active atomic.Bool
		stage2Done   atomic.Bool
		violation    atomic.Bool
	)

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if strings.Contains(req.URL.Path, "generate_204") {
						return &http.Response{
							StatusCode: http.StatusNoContent,
							Body:       io.NopCloser(strings.NewReader("")),
							Request:    req,
						}, nil
					}
					// Streaming stage: mark active, simulate work, then mark done
					if strings.Contains(req.URL.Host, "netflix") || strings.Contains(req.URL.Host, "fast.com") ||
						strings.Contains(req.URL.Host, "youtube") || strings.Contains(req.URL.Host, "disney") {
						stage2Active.Store(true)
						time.Sleep(30 * time.Millisecond)
						stage2Active.Store(false)
						stage2Done.Store(true)
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader("ok")),
							Request:    req,
						}, nil
					}
					// Speed stage: must not execute while stage 2 is active, and stage 2 must be done
					if strings.Contains(req.URL.Path, "__down") {
						if stage2Active.Load() || !stage2Done.Load() {
							violation.Store(true)
						}
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 4096))),
							Request:    req,
						}, nil
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
				}),
			}
			return client, nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:         "run_phase_ordering",
		State:      domain.ProbeRunStateQueued,
		DeadlineAt: time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	kinds := []domain.ProbeKind{
		domain.ProbeKindBaseline,
		domain.ProbeKindStreaming,
		domain.ProbeKindSpeed,
	}
	if err := runner.Run(context.Background(), run, []string{"node_sync"}, kinds); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}

	if violation.Load() {
		t.Fatal("Stage 3 (Speed) executed before Stage 2 (Streaming) completed!")
	}
	if !stage2Done.Load() {
		t.Fatal("Stage 2 never completed")
	}
}

// 3. 阶段并发 (Stage concurrency bounds independently)
func TestPipelineStageConcurrencyLimits(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("node_%02d", i)
		nodesRepo.items[id] = domain.Node{LogicalID: id, Protocol: domain.ProtocolSS, Active: true}
	}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 50})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var (
		curAlive, maxAlive atomic.Int32
		curMedia, maxMedia atomic.Int32
		curSpeed, maxSpeed atomic.Int32
	)

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithStageConcurrency(probe.StageConcurrency{
			Alive: 2,
			Media: 1,
			Speed: 1,
		}),
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					switch {
					case strings.Contains(req.URL.Path, "generate_204"):
						cur := curAlive.Add(1)
						for prev := maxAlive.Load(); cur > prev && !maxAlive.CompareAndSwap(prev, cur); prev = maxAlive.Load() {
						}
						time.Sleep(20 * time.Millisecond)
						curAlive.Add(-1)
						return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil

					case strings.Contains(req.URL.Host, "netflix") || strings.Contains(req.URL.Host, "fast.com") ||
						strings.Contains(req.URL.Host, "youtube") || strings.Contains(req.URL.Host, "disney"):
						cur := curMedia.Add(1)
						for prev := maxMedia.Load(); cur > prev && !maxMedia.CompareAndSwap(prev, cur); prev = maxMedia.Load() {
						}
						time.Sleep(20 * time.Millisecond)
						curMedia.Add(-1)
						return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil

					case strings.Contains(req.URL.Path, "__down"):
						cur := curSpeed.Add(1)
						for prev := maxSpeed.Load(); cur > prev && !maxSpeed.CompareAndSwap(prev, cur); prev = maxSpeed.Load() {
						}
						time.Sleep(20 * time.Millisecond)
						curSpeed.Add(-1)
						return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 4096))), Request: req}, nil
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
				}),
			}
			return client, nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:         "run_stage_concurrency",
		State:      domain.ProbeRunStateQueued,
		DeadlineAt: time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	kinds := []domain.ProbeKind{
		domain.ProbeKindBaseline,
		domain.ProbeKindStreaming,
		domain.ProbeKindSpeed,
	}
	if err := runner.Run(context.Background(), run, nil, kinds); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}

	if maxAlive.Load() > 2 {
		t.Fatalf("expected Alive peak concurrency <= 2, got %d", maxAlive.Load())
	}
	// Media stage concurrency limit is 1 node. With parallel sub-requests (Netflix, YouTube, Disney),
	// peak in-flight HTTP requests for that 1 node reaches up to 3.
	if maxMedia.Load() > 3 {
		t.Fatalf("expected Media peak concurrency <= 3 (1 node x 3 sub-requests), got %d", maxMedia.Load())
	}
	if maxSpeed.Load() > 1 {
		t.Fatalf("expected Speed peak concurrency <= 1, got %d", maxSpeed.Load())
	}
}

// 4. 用户单选与混选 (Single-select and mixed-select probe kinds)
func TestPipelineUserSelectionsSingleAndMixed(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_sel"] = domain.Node{LogicalID: "node_sel", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			return contractAwareMockHTTPClient(), nil, nil
		}),
	)

	t.Run("single_select_speed_only", func(t *testing.T) {
		run := &domain.ProbeRun{ID: "run_single_speed", IdempotencyKey: "key_single_speed", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Minute)}
		if err := runsRepo.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		if err := runner.Run(context.Background(), run, []string{"node_sel"}, []domain.ProbeKind{domain.ProbeKindSpeed}); err != nil {
			t.Fatalf("Run speed: %v", err)
		}
		obs, _ := obsRepo.ListByRun(context.Background(), run.ID)
		if len(obs) != 1 || obs[0].Kind != domain.ProbeKindSpeed {
			t.Fatalf("expected 1 speed observation, got: %+v", obs)
		}
		if obs[0].Verdict != domain.VerdictAvailable || obs[0].Throughput == nil || *obs[0].Throughput <= 0 {
			t.Fatalf("expected valid speed observation with positive throughput, got: %+v", obs[0])
		}
	})

	t.Run("mixed_select_streaming_and_speed_without_baseline", func(t *testing.T) {
		run := &domain.ProbeRun{ID: "run_mixed_stream_speed", IdempotencyKey: "key_mixed_stream_speed", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Minute)}
		if err := runsRepo.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		kinds := []domain.ProbeKind{domain.ProbeKindStreaming, domain.ProbeKindSpeed}
		if err := runner.Run(context.Background(), run, []string{"node_sel"}, kinds); err != nil {
			t.Fatalf("Run mixed: %v", err)
		}
		obs, _ := obsRepo.ListByRun(context.Background(), run.ID)
		if len(obs) != 2 {
			t.Fatalf("expected 2 observations (streaming, speed), got %d", len(obs))
		}
	})

	t.Run("mixed_select_baseline_and_speed_skips_stage2", func(t *testing.T) {
		run := &domain.ProbeRun{ID: "run_mixed_base_speed", IdempotencyKey: "key_mixed_base_speed", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Minute)}
		if err := runsRepo.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		kinds := []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindSpeed}
		if err := runner.Run(context.Background(), run, []string{"node_sel"}, kinds); err != nil {
			t.Fatalf("Run mixed base+speed: %v", err)
		}
		obs, _ := obsRepo.ListByRun(context.Background(), run.ID)
		if len(obs) != 2 {
			t.Fatalf("expected 2 observations (baseline, speed), got %d", len(obs))
		}
	})
}

// 5. 连接复用与优雅关闭 (Client reuse across all stages and single close)
func TestPipelineClientReuseAndGracefulClose(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_reuse"] = domain.Node{LogicalID: "node_reuse", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var (
		dials, cleanups atomic.Int32
		clientClosed    atomic.Bool
		closedPremature atomic.Bool
	)

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(dialCtx context.Context, node domain.Node) (*http.Client, func() error, error) {
			dials.Add(1)
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if clientClosed.Load() {
						closedPremature.Store(true)
						return nil, fmt.Errorf("client already closed")
					}
					return contractAwareMockHTTPClient().Transport.RoundTrip(req)
				}),
			}
			cleanup := func() error {
				clientClosed.Store(true)
				cleanups.Add(1)
				return nil
			}
			return client, cleanup, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:         "run_reuse_test",
		State:      domain.ProbeRunStateQueued,
		DeadlineAt: time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	kinds := []domain.ProbeKind{
		domain.ProbeKindBaseline,
		domain.ProbeKindGeo,
		domain.ProbeKindStreaming,
		domain.ProbeKindAI,
		domain.ProbeKindSpeed,
	}
	if err := runner.Run(context.Background(), run, []string{"node_reuse"}, kinds); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if dials.Load() != 1 {
		t.Fatalf("expected 1 dial across all 5 kinds, got %d", dials.Load())
	}
	if cleanups.Load() != 1 {
		t.Fatalf("expected 1 cleanup after all 5 kinds completed, got %d", cleanups.Load())
	}
	if closedPremature.Load() {
		t.Fatal("client was closed prematurely while requests were still executing")
	}
}

// 6. 任务取消 (Run cancellation halts in-flight tasks and cleans up)
func TestPipelineCancellationHaltsInFlight(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_cancel"] = domain.Node{LogicalID: "node_cancel", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var cleanups atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(dialCtx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					// Cancel when request arrives
					cancel()
					<-req.Context().Done()
					return nil, req.Context().Err()
				}),
			}
			return client, func() error {
				cleanups.Add(1)
				return nil
			}, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:         "run_cancel_test",
		State:      domain.ProbeRunStateQueued,
		DeadlineAt: time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	err = runner.Run(ctx, run, []string{"node_cancel"}, []domain.ProbeKind{domain.ProbeKindBaseline})
	if err == nil {
		t.Fatal("expected cancellation error from runner.Run")
	}

	updated, _ := runsRepo.GetByID(context.Background(), run.ID)
	if updated.State != domain.ProbeRunStateCancelled {
		t.Fatalf("expected run state cancelled, got %s", updated.State)
	}
	if cleanups.Load() != 1 {
		t.Fatalf("expected session to be cleaned up on cancel, cleanups=%d", cleanups.Load())
	}
}

// 7. 测速 float64 KB/s 及 5MB 应用读取预算截断
func TestPipelineSpeedThroughputFloat64KBpsAndBudget(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_speed_bgt"] = domain.Node{LogicalID: "node_speed_bgt", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	// Server returns 8 MB payload, which exceeds the 5 MB application read budget
	totalOffered := 8 * 1024 * 1024
	var serverBytesRead atomic.Int64

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body: io.NopCloser(&slowChunkReader{
							remaining: totalOffered,
							maxRead:   32 * 1024,
							onRead: func() {
								serverBytesRead.Add(32 * 1024)
							},
						}),
						Request: req,
					}, nil
				}),
			}
			return client, nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:         "run_speed_bgt",
		State:      domain.ProbeRunStateQueued,
		DeadlineAt: time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	if err := runner.Run(context.Background(), run, []string{"node_speed_bgt"}, []domain.ProbeKind{domain.ProbeKindSpeed}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	obs, err := obsRepo.ListByRun(context.Background(), run.ID)
	if err != nil || len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d: %v", len(obs), err)
	}
	if obs[0].Verdict != domain.VerdictAvailable {
		t.Fatalf("expected VerdictAvailable with budget truncation, got %s", obs[0].Verdict)
	}
	if obs[0].Throughput == nil || *obs[0].Throughput <= 0 {
		t.Fatalf("expected non-nil, positive Throughput (float64 KB/s), got %v", obs[0].Throughput)
	}

	// Application read must be truncated around the 5MB budget (5 * 1024 * 1024)
	expectedCap := int64(platform.DefaultDownloadMB * 1024 * 1024)
	if serverBytesRead.Load() > expectedCap+64*1024 {
		t.Fatalf("expected server read to be bounded around 5MB (%d), got %d", expectedCap, serverBytesRead.Load())
	}
}

// 8. SQLite 真实持久化与多平台数据链路 (SQLite to Run and Node view integration)
func TestPipelineSQLiteEndToEndWithNodeView(t *testing.T) {
	db := setupPipelineSQLite(t)
	runsRepo := sqlite.NewProbeRunRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)

	ctx := context.Background()
	rev := int64(1)
	node := domain.Node{
		LogicalID:          "node_sql_01",
		DisplayName:        "SQLite Test Node",
		Protocol:           domain.ProtocolVMess,
		Active:             true,
		ConnectionRevision: rev,
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatalf("failed to insert test node: %v", err)
	}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(
		nodeRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			return contractAwareMockHTTPClient(), nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:             "run_sql_subcheck",
		IdempotencyKey: "key_sql_subcheck",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(ctx, run); err != nil {
		t.Fatalf("runsRepo.Create: %v", err)
	}

	kinds := []domain.ProbeKind{
		domain.ProbeKindBaseline,
		domain.ProbeKindStreaming,
		domain.ProbeKindAI,
		domain.ProbeKindSpeed,
	}
	if err := runner.Run(ctx, run, []string{"node_sql_01"}, kinds); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}

	// Verify ProbeRun state updated to Succeeded in SQLite
	persistedRun, err := runsRepo.GetByID(ctx, run.ID)
	if err != nil {
		t.Fatalf("runsRepo.GetByID: %v", err)
	}
	if persistedRun.State != domain.ProbeRunStateSucceeded {
		t.Fatalf("expected run state Succeeded, got %s", persistedRun.State)
	}

	// Verify observations persisted in SQLite with structured evidence data
	persistedObs, err := obsRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("obsRepo.ListByRun: %v", err)
	}
	if len(persistedObs) != 4 {
		t.Fatalf("expected 4 observations, got %d", len(persistedObs))
	}

	for _, o := range persistedObs {
		if o.NodeLogicalID != "node_sql_01" {
			t.Fatalf("unexpected node_logical_id: %s", o.NodeLogicalID)
		}
		if o.ConnectionRevision == nil || *o.ConnectionRevision != rev {
			t.Fatalf("connection revision not persisted: %+v", o.ConnectionRevision)
		}
		switch o.Kind {
		case domain.ProbeKindSpeed:
			if o.Throughput == nil || *o.Throughput <= 0 {
				t.Fatalf("speed observation missing positive throughput: %+v", o)
			}
		case domain.ProbeKindStreaming:
			if len(o.Platforms) == 0 {
				t.Fatalf("streaming observation missing platforms map: %+v", o)
			}
		case domain.ProbeKindAI:
			if len(o.Platforms) == 0 {
				t.Fatalf("AI observation missing platforms map: %+v", o)
			}
		}
	}
}
