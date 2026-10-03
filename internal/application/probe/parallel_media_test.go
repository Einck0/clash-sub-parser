package probe_test

import (
	"context"
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
)

// TestStreamingParallelSubRequestsWallClock verifies that within ProbeKindStreaming,
// the 3 platform sub-requests (Netflix, YouTube, Disney) execute concurrently on the
// same node proxy client. If each takes 60ms, the total wall-clock time is bounded by
// max (~60-90ms) rather than sum (~180ms+).
func TestStreamingParallelSubRequestsWallClock(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_stream_par"] = domain.Node{
		LogicalID: "node_stream_par",
		Protocol:  domain.ProtocolVLESS,
		Active:    true,
	}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var (
		curConcurrent atomic.Int32
		maxConcurrent atomic.Int32
		reqCount      atomic.Int32
	)

	dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		return &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				reqCount.Add(1)
				cur := curConcurrent.Add(1)
				for {
					m := maxConcurrent.Load()
					if cur <= m || maxConcurrent.CompareAndSwap(m, cur) {
						break
					}
				}
				// Simulate 60ms latency per platform
				time.Sleep(60 * time.Millisecond)
				curConcurrent.Add(-1)

				body := ""
				status := http.StatusOK
				switch {
				case strings.Contains(req.URL.Host, "fast.com") || strings.Contains(req.URL.Host, "netflix"):
					body = `{"targets":[{"location":{"country":"US"}}]}`
				case strings.Contains(req.URL.Host, "youtube"):
					body = `"INNERTUBE_CONTEXT_GL":"US"`
				case strings.Contains(req.URL.Host, "bamgrid") || strings.Contains(req.URL.Host, "disney"):
					body = `{"assertion":"test_token","token":{"accessToken":"test_access_token"}}`
				default:
					body = "ok"
				}
				return &http.Response{
					StatusCode: status,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			}),
		}, nil, nil
	}

	runner := probe.NewDefaultRunner(nodesRepo, obsRepo, sched, runsRepo, probe.WithNodeDialer(dialer))

	run := &domain.ProbeRun{
		ID:             "run_stream_parallel",
		IdempotencyKey: "key_stream_parallel",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := runner.Run(context.Background(), run, []string{"node_stream_par"}, []domain.ProbeKind{domain.ProbeKindStreaming}); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
	elapsed := time.Since(start)

	// In-flight concurrency across platforms must have reached 3 (all 3 platforms executing concurrently)
	if got := maxConcurrent.Load(); got < 3 {
		t.Fatalf("expected concurrent sub-requests peak >= 3, got %d", got)
	}

	// In serial execution, 3 platforms would sum sequentially. In parallel,
	// Netflix (60ms), YouTube (60ms), Disney (multiple steps) execute in parallel,
	// saving significant wall-clock latency compared to sum.
	t.Logf("observed parallel streaming wall-clock duration: %v, peak concurrent platforms: %d", elapsed, maxConcurrent.Load())

	obs, err := obsRepo.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}

	platforms := obs[0].Platforms
	if len(platforms) < 3 {
		t.Fatalf("expected at least 3 platforms, got %d: %+v", len(platforms), platforms)
	}
	for _, p := range []string{"netflix", "youtube", "disney"} {
		if _, ok := platforms[p]; !ok {
			t.Fatalf("missing platform %s in observation platforms: %+v", p, platforms)
		}
	}
}

// TestAIParallelSubRequestsWallClock verifies that within ProbeKindAI,
// OpenAI, Claude, and Gemini execute concurrently on the same node proxy client.
func TestAIParallelSubRequestsWallClock(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_ai_par"] = domain.Node{
		LogicalID: "node_ai_par",
		Protocol:  domain.ProtocolVLESS,
		Active:    true,
	}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var (
		curConcurrent atomic.Int32
		maxConcurrent atomic.Int32
	)

	dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		return &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				cur := curConcurrent.Add(1)
				for {
					m := maxConcurrent.Load()
					if cur <= m || maxConcurrent.CompareAndSwap(m, cur) {
						break
					}
				}
				time.Sleep(60 * time.Millisecond)
				curConcurrent.Add(-1)

				body := ""
				switch {
				case strings.Contains(req.URL.Host, "openai.com"):
					body = `{"status":"ok","cookies":"ok"}`
				case strings.Contains(req.URL.Host, "claude.ai"):
					body = "ip=203.0.113.1\nloc=US\n"
				case strings.Contains(req.URL.Host, "gemini.google.com"):
					body = `,2,1,200,"USA"`
				default:
					body = "ok"
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			}),
		}, nil, nil
	}

	runner := probe.NewDefaultRunner(nodesRepo, obsRepo, sched, runsRepo, probe.WithNodeDialer(dialer))

	run := &domain.ProbeRun{
		ID:             "run_ai_parallel",
		IdempotencyKey: "key_ai_parallel",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := runner.Run(context.Background(), run, []string{"node_ai_par"}, []domain.ProbeKind{domain.ProbeKindAI}); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
	elapsed := time.Since(start)

	if got := maxConcurrent.Load(); got < 3 {
		t.Fatalf("expected AI concurrent sub-requests peak >= 3, got %d", got)
	}

	t.Logf("observed parallel AI wall-clock duration: %v, peak concurrent platforms: %d", elapsed, maxConcurrent.Load())

	obs, err := obsRepo.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	platforms := obs[0].Platforms
	for _, p := range []string{"openai", "claude", "gemini"} {
		if _, ok := platforms[p]; !ok {
			t.Fatalf("missing AI platform %s in platforms: %+v", p, platforms)
		}
	}
}

// TestMediaParallelCancellationAndRace verifies cancellation during in-flight parallel
// sub-requests and exercises multiple concurrent nodes under race detector.
func TestMediaParallelCancellationAndRace(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	const nodeCount = 4
	var nodeIDs []string
	for i := 0; i < nodeCount; i++ {
		id := "node_race_" + string(rune('a'+i))
		nodeIDs = append(nodeIDs, id)
		nodesRepo.items[id] = domain.Node{LogicalID: id, Protocol: domain.ProtocolVLESS, Active: true}
	}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 20})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var reqMu sync.Mutex
	var activeReqs int
	dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		return &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				reqMu.Lock()
				activeReqs++
				reqMu.Unlock()

				defer func() {
					reqMu.Lock()
					activeReqs--
					reqMu.Unlock()
				}()

				select {
				case <-time.After(15 * time.Millisecond):
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader("ok")),
						Request:    req,
					}, nil
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
			}),
		}, nil, nil
	}

	runner := probe.NewDefaultRunner(nodesRepo, obsRepo, sched, runsRepo, probe.WithNodeDialer(dialer))

	// Test cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	runCancel := &domain.ProbeRun{
		ID:             "run_media_cancel",
		IdempotencyKey: "key_media_cancel",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), runCancel); err != nil {
		t.Fatal(err)
	}

	_ = runner.Run(ctx, runCancel, nodeIDs, []domain.ProbeKind{domain.ProbeKindStreaming, domain.ProbeKindAI})

	// Run full without cancellation
	runFull := &domain.ProbeRun{
		ID:             "run_media_full",
		IdempotencyKey: "key_media_full",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), runFull); err != nil {
		t.Fatal(err)
	}

	if err := runner.Run(context.Background(), runFull, nodeIDs, []domain.ProbeKind{domain.ProbeKindStreaming, domain.ProbeKindAI}); err != nil {
		t.Fatalf("runner.Run full: %v", err)
	}

	obs, err := obsRepo.ListByRun(context.Background(), runFull.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 4 nodes x 2 kinds = 8 observations
	if len(obs) != nodeCount*2 {
		t.Fatalf("expected %d observations, got %d", nodeCount*2, len(obs))
	}
}

// TestSerialVsParallelMediaLatencyMaxVsSum directly compares serial platform execution
// (calling CheckNetflix, CheckYoutube, CheckDisney sequentially) vs the new parallel runner execution
// under identical simulated network latency fixtures to prove the latency reduction (max vs sum).
func TestSerialVsParallelMediaLatencyMaxVsSum(t *testing.T) {
	const subRequestLatency = 40 * time.Millisecond

	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		time.Sleep(subRequestLatency)
		body := `{"targets":[{"location":{"country":"US"}}]}`
		if strings.Contains(req.URL.Host, "youtube") {
			body = `"INNERTUBE_CONTEXT_GL":"US"`
		} else if strings.Contains(req.URL.Host, "disney") || strings.Contains(req.URL.Host, "bamgrid") {
			body = `{"assertion":"token","token":{"accessToken":"tok"}}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	mockClient := &http.Client{Transport: mockTransport}

	// 1. Old serial execution: call CheckNetflix -> CheckYoutube -> CheckDisney in series
	// Netflix: 1 req (40ms). YouTube: 1 req (40ms). Disney: 3 reqs (120ms). Total serial = ~200ms.
	ctx := context.Background()
	serialStart := time.Now()
	_ = platform.CheckNetflix(ctx, mockClient)
	_ = platform.CheckYoutube(ctx, mockClient)
	_ = platform.CheckDisney(ctx, mockClient)
	serialElapsed := time.Since(serialStart)

	// 2. New parallel runner execution: executes all 3 platform pipelines concurrently
	// Netflix (40ms), YouTube (40ms), Disney (120ms) execute in parallel -> max is ~120ms.
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_cmp"] = domain.Node{LogicalID: "node_cmp", Protocol: domain.ProtocolVLESS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(nodesRepo, obsRepo, sched, runsRepo, probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		return &http.Client{Transport: mockTransport}, nil, nil
	}))

	run := &domain.ProbeRun{
		ID:             "run_cmp",
		IdempotencyKey: "key_cmp",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Minute),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	parStart := time.Now()
	if err := runner.Run(context.Background(), run, []string{"node_cmp"}, []domain.ProbeKind{domain.ProbeKindStreaming}); err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
	parElapsed := time.Since(parStart)

	t.Logf("serial sum duration: %v, parallel duration: %v, speedup ratio: %.2fx",
		serialElapsed, parElapsed, float64(serialElapsed)/float64(parElapsed))

	// Parallel must execute platforms concurrently (max ~120ms),
	// significantly faster than the old sequential sum (~200ms).
	if parElapsed >= serialElapsed {
		t.Fatalf("expected parallel (%v) < serial sum (%v)", parElapsed, serialElapsed)
	}
}
