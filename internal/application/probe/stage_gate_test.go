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
	"clash-sub-parser/internal/probe/queue"
)

func stageRun(id string) *domain.ProbeRun {
	return &domain.ProbeRun{ID: id, State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Minute)}
}

func TestStageGateSpeedRejectedBeforeDialOrHTTP(t *testing.T) {
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
			return &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return nil, nil
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
	if dials.Load() != 0 || requests.Load() != 0 {
		t.Fatalf("speed unexpectedly performed work: dials=%d requests=%d", dials.Load(), requests.Load())
	}
	got, err := observations.ListByRun(context.Background(), run.ID)
	if err != nil || len(got) != 1 || got[0].Kind != domain.ProbeKindSpeed {
		t.Fatalf("speed observation missing: %#v, err=%v", got, err)
	}
}

func TestStageGateBaselineAvailabilityControlsStreaming(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		wantRequests int
	}{
		{name: "unavailable", status: http.StatusServiceUnavailable, wantRequests: 1},
		{name: "available", status: http.StatusOK, wantRequests: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
			nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
			sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
			if err != nil {
				t.Fatal(err)
			}
			defer sched.Close()
			var requests atomic.Int32
			var urlsMu sync.Mutex
			var urls []string
			client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				requests.Add(1)
				urlsMu.Lock()
				urls = append(urls, req.URL.String())
				urlsMu.Unlock()
				body := "ok"
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})}
			runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
				probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) { return client, nil, nil }),
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
			got, err := observations.ListByRun(context.Background(), run.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantObs := 1
			if tc.wantRequests == 2 {
				wantObs = 2
			}
			if len(got) != wantObs {
				t.Fatalf("observations=%d, want %d: %#v", len(got), wantObs, got)
			}
			if tc.wantRequests == 1 && (got[0].Kind != domain.ProbeKindBaseline || got[0].Verdict == domain.VerdictAvailable) {
				t.Fatalf("503 baseline observation not retained: %#v", got[0])
			}
			if tc.wantRequests == 2 {
				urlsMu.Lock()
				defer urlsMu.Unlock()
				if !strings.Contains(urls[1], "netflix.com") {
					t.Fatalf("second-stage request not streaming: %q", urls[1])
				}
			}
		})
	}
}
