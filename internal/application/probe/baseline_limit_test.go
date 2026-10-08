package probe_test

import (
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
	"clash-sub-parser/internal/probe/queue"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActualRunnerBaseline64KiBNoExtraProbeByte(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, strings.Repeat("b", 100000)) }))
	defer server.Close()
	nodes := newMemoryNodes()
	nodes.items["one"] = domain.Node{LogicalID: "one", Active: true, Protocol: domain.ProtocolHTTP}
	obs := newMemoryObservations()
	runs := newMemoryRuns()
	sched, _ := queue.NewScheduler(queue.Config{Concurrency: 1})
	defer sched.Close()
	budget := platform.NewBodyBudget(1 << 20)
	runner := probe.NewDefaultRunner(nodes, obs, sched, runs, probe.WithBodyBudget(budget), probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
		c := server.Client()
		c.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			u := *req.URL
			u.Scheme = "http"
			u.Host = strings.TrimPrefix(server.URL, "http://")
			clone.URL = &u
			return http.DefaultTransport.RoundTrip(clone)
		})
		return c, nil, nil
	}))
	run := &domain.ProbeRun{ID: "limit", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Second)}
	_ = runs.Create(context.Background(), run)
	if err := runner.Run(context.Background(), run, nil, []domain.ProbeKind{domain.ProbeKindBaseline}); err != nil {
		t.Fatal(err)
	}
	_, used := budget.Snapshot()
	if used != 65536 {
		t.Fatalf("actual runner body=%d", used)
	}
	rows, _ := obs.ListByRun(context.Background(), run.ID)
	if len(rows) != 1 || rows[0].Verdict != domain.VerdictAvailable || rows[0].Attempt.BodyBytes != 65536 {
		t.Fatalf("runner normalized baseline=%+v", rows)
	}
}
