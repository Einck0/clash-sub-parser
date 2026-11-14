package probe_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
)

// One selected geo result may serve the same node/session's risk request, but
// must never leak across runs or get persisted as a raw address.
func TestRunnerReusesOnlySameRunSelectedGeoIdentityForRisk(t *testing.T) {
	runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
	nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true, ConnectionRevision: 7}
	sched, err := queue.NewScheduler(queue.Config{Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()
	var geoCalls, traceCalls, scoreCalls atomic.Int32
	var generation atomic.Int32
	var expectedIP string
	runner := probe.NewDefaultRunner(nodes, observations, sched, runs, probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
		return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body := ""
			switch req.URL.Host {
			case "api.ip.sb":
				geoCalls.Add(1)
				body = fmt.Sprintf(`{"ip":"203.0.113.%d","country_code":"US"}`, generation.Load())
			case "cloudflare.com":
				traceCalls.Add(1)
				body = "ip=203.0.113.9\nloc=US\n"
			case "scamalytics.com":
				scoreCalls.Add(1)
				if req.URL.Path != "/ip/"+expectedIP {
					t.Errorf("risk identity=%s want %s", req.URL.Path, expectedIP)
				}
				body = "IP Fraud Risk API\n1\n2\n3\n\"score\": \"12\",\n\"risk\": \"low\"\n6"
			default:
				t.Errorf("unexpected target=%s", req.URL.Host)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		})}, nil, nil
	}))
	for i := 1; i <= 3; i++ {
		generation.Store(int32(i))
		expectedIP = fmt.Sprintf("203.0.113.%d", i)
		kinds := []domain.ProbeKind{domain.ProbeKindGeo, domain.ProbeKindIPRisk}
		if i == 3 {
			kinds = []domain.ProbeKind{domain.ProbeKindIPRisk}
			expectedIP = "203.0.113.9"
		}
		run := stageRun(fmt.Sprintf("geo-risk-%d", i))
		run.IdempotencyKey = run.ID
		if err := runs.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		if err := runner.Run(context.Background(), run, []string{"node"}, kinds); err != nil {
			t.Fatal(err)
		}
		rows, _ := observations.ListByRun(context.Background(), run.ID)
		if len(rows) != len(kinds) {
			t.Fatalf("observations=%d want %d", len(rows), len(kinds))
		}
		for _, row := range rows {
			if row.Verdict != domain.VerdictAvailable {
				t.Fatalf("check failed: kind=%s verdict=%s summary=%s", row.Kind, row.Verdict, row.RedactedSummary)
			}
			if strings.Contains(row.EvidenceData, expectedIP) || strings.Contains(row.RedactedSummary, expectedIP) {
				t.Fatal("raw geo IP persisted")
			}
			if row.ConnectionRevision == nil || *row.ConnectionRevision != 7 {
				t.Fatal("connection revision lost")
			}
		}
	}
	if geoCalls.Load() != 2 || traceCalls.Load() != 1 || scoreCalls.Load() != 3 {
		t.Fatalf("geo=%d trace=%d score=%d", geoCalls.Load(), traceCalls.Load(), scoreCalls.Load())
	}
	t.Logf("geo_requests=2 identity_fallback_requests=1 risk_requests=3 runs=3")
}
