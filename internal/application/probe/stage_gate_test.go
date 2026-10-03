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

func stageRun(id string) *domain.ProbeRun {
	return &domain.ProbeRun{ID: id, State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Minute)}
}

func TestStageGateSpeedExplicitOptInExecutesAndEnforcesBudget(t *testing.T) {
	t.Run("explicit_speed_kind_opts_in_and_succeeds", func(t *testing.T) {
		runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
		nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		var dials, requests atomic.Int32
		runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
			probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
				dials.Add(1)
				return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					requests.Add(1)
					payload := strings.Repeat("x", 4096)
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(payload)),
						Request:    req,
					}, nil
				})}, nil, nil
			}),
		)
		run := stageRun("speed_opt_in")
		if err := runs.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		if err := runner.Run(context.Background(), run, []string{"node"}, []domain.ProbeKind{domain.ProbeKindSpeed}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if dials.Load() != 1 || requests.Load() != 1 {
			t.Fatalf("expected speed probe to dial and request once: dials=%d requests=%d", dials.Load(), requests.Load())
		}
		got, err := observations.ListByRun(context.Background(), run.ID)
		if err != nil || len(got) != 1 || got[0].Kind != domain.ProbeKindSpeed {
			t.Fatalf("speed observation missing: %#v, err=%v", got, err)
		}
		if got[0].Verdict != domain.VerdictAvailable {
			t.Fatalf("expected speed verdict available, got %s (summary=%s)", got[0].Verdict, got[0].RedactedSummary)
		}
		if !strings.Contains(got[0].RedactedSummary, "bytes_read=4096") {
			t.Fatalf("expected summary to include bytes_read=4096, got %s", got[0].RedactedSummary)
		}
	})

	t.Run("speed_rejects_204_empty_and_tiny_payloads_below_1024_bytes", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			statusCode int
			payload    string
		}{
			{name: "status_204_empty", statusCode: http.StatusNoContent, payload: ""},
			{name: "status_200_empty", statusCode: http.StatusOK, payload: ""},
			{name: "status_200_tiny_ok", statusCode: http.StatusOK, payload: "OK"},
			{name: "status_200_1023_bytes", statusCode: http.StatusOK, payload: strings.Repeat("x", 1023)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
				nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
				sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
				if err != nil {
					t.Fatal(err)
				}
				defer sched.Close()

				runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
					probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
						return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: tc.statusCode,
								Header:     make(http.Header),
								Body:       io.NopCloser(strings.NewReader(tc.payload)),
								Request:    req,
							}, nil
						})}, nil, nil
					}),
				)
				run := stageRun("speed_tiny_" + tc.name)
				if err := runs.Create(context.Background(), run); err != nil {
					t.Fatal(err)
				}
				if err := runner.Run(context.Background(), run, []string{"node"}, []domain.ProbeKind{domain.ProbeKindSpeed}); err != nil {
					t.Fatalf("Run: %v", err)
				}
				got, err := observations.ListByRun(context.Background(), run.ID)
				if err != nil || len(got) != 1 {
					t.Fatalf("speed observation missing: %#v, err=%v", got, err)
				}
				if got[0].Verdict != domain.VerdictUnknown || !strings.Contains(got[0].RedactedSummary, "reason=contract_drift") {
					t.Fatalf("expected speed tiny payload verdict=unknown reason=contract_drift, got %#v", got[0])
				}
			})
		}
	})

	t.Run("speed_measures_full_body_transfer_elapsed_time_and_throughput", func(t *testing.T) {
		runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
		nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		var clockMu sync.Mutex
		now := time.Now().UTC()
		clockFn := func() time.Time {
			clockMu.Lock()
			defer clockMu.Unlock()
			return now
		}
		advanceClock := func(d time.Duration) {
			clockMu.Lock()
			now = now.Add(d)
			clockMu.Unlock()
		}

		// 4 chunks of 1024 bytes = 4096 bytes total; each chunk read advances clock by 20ms (total 80ms in body read).
		// Header response itself takes 0ms, so any non-zero latency_ms proves timing stopped AFTER readBoundedResponse.
		runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
			probe.WithRunnerClock(clockFn),
			probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
				return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body: &slowChunkReader{
							remaining: 4096,
							maxRead:   1024,
							onRead: func() {
								advanceClock(20 * time.Millisecond)
							},
						},
						Request: req,
					}, nil
				})}, nil, nil
			}),
		)
		run := &domain.ProbeRun{
			ID:         "speed_slow_reader",
			State:      domain.ProbeRunStateQueued,
			DeadlineAt: now.Add(time.Minute),
		}
		if err := runs.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		if err := runner.Run(context.Background(), run, []string{"node"}, []domain.ProbeKind{domain.ProbeKindSpeed}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		got, err := observations.ListByRun(context.Background(), run.ID)
		if err != nil || len(got) != 1 {
			t.Fatalf("speed observation missing: %#v, err=%v", got, err)
		}
		if got[0].Verdict != domain.VerdictAvailable {
			t.Fatalf("expected speed verdict available, got %s (%s)", got[0].Verdict, got[0].RedactedSummary)
		}
		if got[0].LatencyMS != 80 {
			t.Fatalf("expected latency_ms=80 from full body read, got %d (summary=%s)", got[0].LatencyMS, got[0].RedactedSummary)
		}
		// throughput_kbps = (4096 * 8) / 80 = 409
		if !strings.Contains(got[0].RedactedSummary, "bytes_read=4096 throughput_kbps=409") {
			t.Fatalf("expected bytes_read=4096 throughput_kbps=409 in summary, got %s", got[0].RedactedSummary)
		}
	})

	t.Run("speed_caps_at_application_byte_budget_without_error", func(t *testing.T) {
		runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
		nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		// Server offers 8 MB of payload, exceeding 5 MB application read budget
		totalOffered := 8 * 1024 * 1024
		var serverBytesRead atomic.Int64
		runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
			probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
				return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
				})}, nil, nil
			}),
		)
		run := stageRun("speed_over_budget")
		if err := runs.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		if err := runner.Run(context.Background(), run, []string{"node"}, []domain.ProbeKind{domain.ProbeKindSpeed}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		got, err := observations.ListByRun(context.Background(), run.ID)
		if err != nil || len(got) != 1 {
			t.Fatalf("speed observation missing: %#v, err=%v", got, err)
		}
		if got[0].Verdict != domain.VerdictAvailable {
			t.Fatalf("expected speed verdict available, got %#v", got[0])
		}
		expectedBudget := int64(platform.DefaultDownloadMB * 1024 * 1024)
		if serverBytesRead.Load() > expectedBudget+64*1024 {
			t.Fatalf("expected application read to be bounded by budget %d, got %d", expectedBudget, serverBytesRead.Load())
		}
		if got[0].Throughput == nil || *got[0].Throughput <= 0 {
			t.Fatalf("expected valid throughput calculation, got %#v", got[0].Throughput)
		}
	})
}

type slowChunkReader struct {
	remaining int
	maxRead   int
	advances  int
	onRead    func()
}

func (r *slowChunkReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	if r.onRead != nil && r.advances < 4 {
		r.advances++
		r.onRead()
	}
	n := r.remaining
	if r.maxRead > 0 && n > r.maxRead {
		n = r.maxRead
	}
	if n > len(p) {
		n = len(p)
	}
	for i := 0; i < n; i++ {
		p[i] = 's'
	}
	r.remaining -= n
	return n, nil
}

func (r *slowChunkReader) Close() error { return nil }

func TestStageGateBaselineAvailabilityControlsStreaming(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		body         string
		wantRequests int
	}{
		{name: "unavailable_503", status: http.StatusServiceUnavailable, body: "unavailable", wantRequests: 1},
		{name: "http_200_ok_rejected_as_drift", status: http.StatusOK, body: "ok", wantRequests: 1},
		{name: "http_204_non_empty_rejected_as_drift", status: http.StatusNoContent, body: "unexpected", wantRequests: 1},
		{name: "available_204_empty", status: http.StatusNoContent, body: "", wantRequests: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
			nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
			sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
			if err != nil {
				t.Fatal(err)
			}
			defer sched.Close()
			var requests, dials, cleanups atomic.Int32
			var urlsMu sync.Mutex
			var urls []string
			client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				requests.Add(1)
				urlsMu.Lock()
				urls = append(urls, req.URL.String())
				urlsMu.Unlock()
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
			})}
			runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
				probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
					dials.Add(1)
					return client, func() error {
						cleanups.Add(1)
						return nil
					}, nil
				}),
			)
			run := stageRun("stage_" + tc.name)
			if err := runs.Create(context.Background(), run); err != nil {
				t.Fatal(err)
			}
			if err := runner.Run(context.Background(), run, []string{"node"}, []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindStreaming}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := requests.Load(); got != int32(tc.wantRequests) {
				t.Fatalf("HTTP requests=%d, want %d", got, tc.wantRequests)
			}
			if gotDials := dials.Load(); gotDials != 1 {
				t.Fatalf("node dial count=%d, want 1", gotDials)
			}
			if gotCleanups := cleanups.Load(); gotCleanups != 1 {
				t.Fatalf("node cleanup count=%d, want 1", gotCleanups)
			}
			got, err := observations.ListByRun(context.Background(), run.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantObs := 1
			if tc.wantRequests > 1 {
				wantObs = 2
			}
			if len(got) != wantObs {
				t.Fatalf("observations=%d, want %d: %#v", len(got), wantObs, got)
			}
			if tc.wantRequests == 1 && (got[0].Kind != domain.ProbeKindBaseline || got[0].Verdict == domain.VerdictAvailable) {
				t.Fatalf("non-available baseline observation not retained: %#v", got[0])
			}
			if tc.wantRequests > 1 {
				urlsMu.Lock()
				defer urlsMu.Unlock()
				// With concurrent streaming sub-requests (Netflix, YouTube, Disney), urls[1] can be any of the streaming endpoints.
				isStreaming := strings.Contains(urls[1], "fast.com") || strings.Contains(urls[1], "netflix") ||
					strings.Contains(urls[1], "youtube") || strings.Contains(urls[1], "disney")
				if !isStreaming {
					t.Fatalf("second-stage request not streaming: %q", urls[1])
				}
			}
		})
	}
}
