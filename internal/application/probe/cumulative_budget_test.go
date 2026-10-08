package probe_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
	"clash-sub-parser/internal/probe/queue"
)

func TestRunnerSharedExperimentBudgetPersistsEveryTerminalStage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, strings.Repeat("b", 1000)) }))
	defer server.Close()
	budget := platform.NewBodyBudget(1500)
	for _, side := range []string{"A", "B"} {
		runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
		nodes.items["one"] = domain.Node{LogicalID: "one", Protocol: domain.ProtocolHTTP, Active: true, ConnectionRevision: 3, Credentials: domain.InboundProtocolCredential{Password: "private-secret"}}
		sched, err := queue.NewScheduler(queue.Config{Concurrency: 8})
		if err != nil {
			t.Fatal(err)
		}
		runner := probe.NewDefaultRunner(nodes, observations, sched, runs, probe.WithBodyBudget(budget), probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
			client := server.Client()
			client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				copy := req.Clone(req.Context())
				u := *req.URL
				copy.URL = &u
				copy.URL.Scheme = "http"
				copy.URL.Host = strings.TrimPrefix(server.URL, "http://")
				return http.DefaultTransport.RoundTrip(copy)
			})
			return client, nil, nil
		}))
		run := &domain.ProbeRun{ID: side, State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Second), ConfigRevision: "frozen"}
		if err = runs.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		_ = runner.Run(context.Background(), run, []string{"one"}, []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindGeo, domain.ProbeKindStreaming, domain.ProbeKindSpeed})
		sched.Close()
		rows, err := observations.ListByRun(context.Background(), run.ID)
		if err != nil || len(rows) != 4 {
			t.Fatalf("side %s terminal rows=%d err=%v", side, len(rows), err)
		}
		for _, row := range rows {
			if row.Attempt == nil || row.Attempt.ConfigFingerprint == "" || strings.Contains(row.EvidenceData, "private-secret") {
				t.Fatalf("missing/unsafe attempt: %+v", row)
			}
			if side == "B" {
				// Baseline never started on B: do not falsely infer a dependency failure.
				want := "budget_not_executed"
				if row.Attempt.Category != want {
					t.Fatalf("B terminal reason: got %s want %s", row.Attempt.Category, want)
				}
			}
		}
	}
	_, used := budget.Snapshot()
	if used != 1500 {
		t.Fatalf("aggregate AB body consumption=%d", used)
	}
}
