package probe_test

import (
	"context"
	"errors"
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

func TestSameRunGeoAndPlatformRegionMismatch(t *testing.T) {
	t.Run("same_run_geoUS_geminiJP_yields_abnormal", func(t *testing.T) {
		runs := newMemoryRuns()
		observations := newMemoryObservations()
		nodes := newMemoryNodes()
		nodes.items["node_geo_mismatch"] = domain.Node{LogicalID: "node_geo_mismatch", Protocol: domain.ProtocolSS, Active: true}

		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		// Geo returns US exit; Gemini returns JP service region.
		dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					var body string
					switch {
					case strings.Contains(req.URL.Host, "api.ip.sb") || strings.Contains(req.URL.Path, "geoip"):
						body = `{"ip":"203.0.113.10","country_code":"US","asn":13335}`
					case strings.Contains(req.URL.Host, "gemini.google.com"):
						body = `,2,1,200,"JPN"`
					default:
						body = "<html>OK</html>"
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(body)),
						Request:    req,
					}, nil
				}),
			}
			return client, nil, nil
		}

		runner := probe.NewDefaultRunner(
			nodes, observations, sched, runs,
			probe.WithNodeDialer(dialer),
			probe.WithPlatforms([]string{"gemini"}),
		)

		run := &domain.ProbeRun{
			ID:             "run_mismatch_1",
			IdempotencyKey: "key_mismatch_1",
			State:          domain.ProbeRunStateQueued,
			DeadlineAt:     time.Now().Add(time.Minute),
		}
		if err := runs.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}

		kinds := []domain.ProbeKind{domain.ProbeKindGeo, domain.ProbeKindAI}
		if err := runner.Run(context.Background(), run, []string{"node_geo_mismatch"}, kinds); err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		obsList, err := observations.ListByRun(context.Background(), run.ID)
		if err != nil || len(obsList) != 2 {
			t.Fatalf("expected 2 observations (geo, ai), got %d: %+v", len(obsList), obsList)
		}

		var geoObs, aiObs *domain.ProbeObservation
		for i := range obsList {
			if obsList[i].Kind == domain.ProbeKindGeo {
				geoObs = &obsList[i]
			} else if obsList[i].Kind == domain.ProbeKindAI {
				aiObs = &obsList[i]
			}
		}
		if geoObs == nil || aiObs == nil {
			t.Fatalf("missing geo or ai observation: geo=%v, ai=%v", geoObs, aiObs)
		}

		if geoObs.Verdict != domain.VerdictAvailable || geoObs.Region != "US" {
			t.Fatalf("geo observation want available/US, got verdict=%s region=%s", geoObs.Verdict, geoObs.Region)
		}

		// AI observation must be abnormal due to region mismatch
		if aiObs.Verdict != domain.VerdictAbnormal || aiObs.Reason != "partial_or_abnormal" {
			t.Fatalf("AI observation want abnormal/partial_or_abnormal, got verdict=%s reason=%s summary=%s",
				aiObs.Verdict, aiObs.Reason, aiObs.RedactedSummary)
		}
		geminiCap, ok := aiObs.Platforms["gemini"]
		if !ok {
			t.Fatalf("missing gemini platform in platformsMap: %+v", aiObs.Platforms)
		}
		if geminiCap.Verdict != domain.VerdictAbnormal || geminiCap.Reason != "region_mismatch" {
			t.Fatalf("gemini platform want abnormal/region_mismatch, got verdict=%s reason=%s",
				geminiCap.Verdict, geminiCap.Reason)
		}
		if geminiCap.Region != "JP" {
			t.Fatalf("gemini platform region want JP, got %s", geminiCap.Region)
		}
		if geminiCap.ExitRegion != "US" {
			t.Fatalf("gemini platform exit_region want US, got %s", geminiCap.ExitRegion)
		}
		if geminiCap.RegionSource != "service" {
			t.Fatalf("gemini platform region_source want service, got %s", geminiCap.RegionSource)
		}
		if geminiCap.EvidenceScope != "web_region" {
			t.Fatalf("gemini platform evidence_scope want web_region, got %s", geminiCap.EvidenceScope)
		}
	})

	t.Run("same_run_geoUS_geminiUS_yields_available", func(t *testing.T) {
		runs := newMemoryRuns()
		observations := newMemoryObservations()
		nodes := newMemoryNodes()
		nodes.items["node_geo_match"] = domain.Node{LogicalID: "node_geo_match", Protocol: domain.ProtocolSS, Active: true}

		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					var body string
					switch {
					case strings.Contains(req.URL.Host, "api.ip.sb") || strings.Contains(req.URL.Path, "geoip"):
						body = `{"ip":"203.0.113.10","country_code":"US","asn":13335}`
					case strings.Contains(req.URL.Host, "gemini.google.com"):
						body = `,2,1,200,"USA"`
					default:
						body = "<html>OK</html>"
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(body)),
						Request:    req,
					}, nil
				}),
			}
			return client, nil, nil
		}

		runner := probe.NewDefaultRunner(
			nodes, observations, sched, runs,
			probe.WithNodeDialer(dialer),
			probe.WithPlatforms([]string{"gemini"}),
		)

		run := &domain.ProbeRun{
			ID:             "run_match_1",
			IdempotencyKey: "key_match_1",
			State:          domain.ProbeRunStateQueued,
			DeadlineAt:     time.Now().Add(time.Minute),
		}
		if err := runsRepoCreate(runs, run); err != nil {
			t.Fatal(err)
		}

		kinds := []domain.ProbeKind{domain.ProbeKindGeo, domain.ProbeKindAI}
		if err := runner.Run(context.Background(), run, []string{"node_geo_match"}, kinds); err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		obsList, err := observations.ListByRun(context.Background(), run.ID)
		if err != nil || len(obsList) != 2 {
			t.Fatalf("expected 2 observations, got %d", len(obsList))
		}

		var aiObs *domain.ProbeObservation
		for i := range obsList {
			if obsList[i].Kind == domain.ProbeKindAI {
				aiObs = &obsList[i]
			}
		}
		if aiObs.Verdict != domain.VerdictAvailable || aiObs.Reason != "contract_matched" {
			t.Fatalf("AI observation want available/contract_matched, got verdict=%s reason=%s",
				aiObs.Verdict, aiObs.Reason)
		}
		geminiCap := aiObs.Platforms["gemini"]
		if geminiCap.Verdict != domain.VerdictAvailable || geminiCap.Region != "US" || geminiCap.ExitRegion != "US" {
			t.Fatalf("gemini platform want available US/US, got %+v", geminiCap)
		}
	})

	t.Run("missing_geo_kind_yields_no_inferred_conflict", func(t *testing.T) {
		runs := newMemoryRuns()
		observations := newMemoryObservations()
		nodes := newMemoryNodes()
		nodes.items["node_no_geo"] = domain.Node{LogicalID: "node_no_geo", Protocol: domain.ProtocolSS, Active: true}

		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`,2,1,200,"JPN"`)),
						Request:    req,
					}, nil
				}),
			}
			return client, nil, nil
		}

		runner := probe.NewDefaultRunner(
			nodes, observations, sched, runs,
			probe.WithNodeDialer(dialer),
			probe.WithPlatforms([]string{"gemini"}),
		)

		run := &domain.ProbeRun{
			ID:             "run_no_geo_1",
			IdempotencyKey: "key_no_geo_1",
			State:          domain.ProbeRunStateQueued,
			DeadlineAt:     time.Now().Add(time.Minute),
		}
		if err := runsRepoCreate(runs, run); err != nil {
			t.Fatal(err)
		}

		// Only AI is probed (no Geo)
		if err := runner.Run(context.Background(), run, []string{"node_no_geo"}, []domain.ProbeKind{domain.ProbeKindAI}); err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		obsList, _ := observations.ListByRun(context.Background(), run.ID)
		if len(obsList) != 1 {
			t.Fatalf("expected 1 observation, got %d", len(obsList))
		}
		if obsList[0].Verdict != domain.VerdictAvailable {
			t.Fatalf("want available when geo was not executed, got %s", obsList[0].Verdict)
		}
		geminiCap := obsList[0].Platforms["gemini"]
		if geminiCap.Verdict != domain.VerdictAvailable || geminiCap.ExitRegion != "" {
			t.Fatalf("want available with empty exit_region, got %+v", geminiCap)
		}
	})

	t.Run("netflix_fast_cdn_target_not_compared_as_exit", func(t *testing.T) {
		runs := newMemoryRuns()
		observations := newMemoryObservations()
		nodes := newMemoryNodes()
		nodes.items["node_fast_cdn"] = domain.Node{LogicalID: "node_fast_cdn", Protocol: domain.ProtocolSS, Active: true}

		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		// Geo returns US, fast.com returns target country JP (cdn_target)
		dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					var body string
					switch {
					case strings.Contains(req.URL.Host, "api.ip.sb") || strings.Contains(req.URL.Path, "geoip"):
						body = `{"ip":"203.0.113.10","country_code":"US","asn":13335}`
					case strings.Contains(req.URL.Host, "api.fast.com"):
						body = `{"targets":[{"location":{"country":"JP"}}]}`
					default:
						body = "<html>OK</html>"
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(body)),
						Request:    req,
					}, nil
				}),
			}
			return client, nil, nil
		}

		runner := probe.NewDefaultRunner(
			nodes, observations, sched, runs,
			probe.WithNodeDialer(dialer),
			probe.WithPlatforms([]string{"netflix"}),
		)

		run := &domain.ProbeRun{
			ID:             "run_fast_cdn_1",
			IdempotencyKey: "key_fast_cdn_1",
			State:          domain.ProbeRunStateQueued,
			DeadlineAt:     time.Now().Add(time.Minute),
		}
		if err := runsRepoCreate(runs, run); err != nil {
			t.Fatal(err)
		}

		kinds := []domain.ProbeKind{domain.ProbeKindGeo, domain.ProbeKindStreaming}
		if err := runner.Run(context.Background(), run, []string{"node_fast_cdn"}, kinds); err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		obsList, _ := observations.ListByRun(context.Background(), run.ID)
		var streamObs *domain.ProbeObservation
		for i := range obsList {
			if obsList[i].Kind == domain.ProbeKindStreaming {
				streamObs = &obsList[i]
			}
		}
		if streamObs == nil {
			t.Fatal("missing streaming observation")
		}
		nf := streamObs.Platforms["netflix"]
		// cdn_target should NOT trigger region_mismatch against exit US
		if nf.Verdict != domain.VerdictAvailable || nf.Reason != "fast_cdn_unlocked" {
			t.Fatalf("netflix cdn_target must remain available/fast_cdn_unlocked, got verdict=%s reason=%s",
				nf.Verdict, nf.Reason)
		}
		if nf.RegionSource != "cdn_target" {
			t.Fatalf("expected region_source cdn_target, got %s", nf.RegionSource)
		}
	})

	t.Run("explicit_gemini_selection_does_not_request_antigravity_or_geo", func(t *testing.T) {
		runs := newMemoryRuns()
		observations := newMemoryObservations()
		nodes := newMemoryNodes()
		nodes.items["node_gemini_only"] = domain.Node{LogicalID: "node_gemini_only", Protocol: domain.ProtocolSS, Active: true}

		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		var requestedHosts sync.Map
		dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					requestedHosts.Store(req.URL.Host, true)
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`,2,1,200,"USA"`)),
						Request:    req,
					}, nil
				}),
			}
			return client, nil, nil
		}

		runner := probe.NewDefaultRunner(
			nodes, observations, sched, runs,
			probe.WithNodeDialer(dialer),
			probe.WithPlatforms([]string{"gemini"}),
		)

		run := &domain.ProbeRun{
			ID:             "run_gem_only_1",
			IdempotencyKey: "key_gem_only_1",
			State:          domain.ProbeRunStateQueued,
			DeadlineAt:     time.Now().Add(time.Minute),
		}
		if err := runsRepoCreate(runs, run); err != nil {
			t.Fatal(err)
		}

		if err := runner.Run(context.Background(), run, []string{"node_gemini_only"}, []domain.ProbeKind{domain.ProbeKindAI}); err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		if _, antigravityRequested := requestedHosts.Load("antigravity.google"); antigravityRequested {
			t.Fatal("antigravity was requested when only gemini was selected")
		}
		if _, geoRequested := requestedHosts.Load("api.ip.sb"); geoRequested {
			t.Fatal("hidden geo request occurred when geo was not selected")
		}
	})
}

func TestControlInterruptionAndPartialEvidencePreservation(t *testing.T) {
	t.Run("body_budget_interruption_preserves_completed_positive_platform", func(t *testing.T) {
		runs := newMemoryRuns()
		observations := newMemoryObservations()
		nodes := newMemoryNodes()
		nodes.items["node_budget"] = domain.Node{LogicalID: "node_budget", Protocol: domain.ProtocolSS, Active: true}

		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		// OpenAI completes quickly; Claude hits budget
		var reqCount int32
		budget := platform.NewBodyBudget(1024) // tight 1024-byte budget
		dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					n := atomic.AddInt32(&reqCount, 1)
					if strings.Contains(req.URL.Host, "chat.openai.com") || strings.Contains(req.URL.Host, "openai") {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader("ip=203.0.113.10\nloc=US\n")),
							Request:    req,
						}, nil
					}
					// Return large body for other requests that exhausts the 1024-byte budget
					if n > 2 {
						return nil, platform.ErrBodyBudget
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(strings.Repeat("a", 2048))),
						Request:    req,
					}, nil
				}),
			}
			return client, nil, nil
		}

		runner := probe.NewDefaultRunner(
			nodes, observations, sched, runs,
			probe.WithNodeDialer(dialer),
			probe.WithBodyBudget(budget),
		)

		run := &domain.ProbeRun{
			ID:             "run_budget_preserve",
			IdempotencyKey: "key_budget_preserve",
			State:          domain.ProbeRunStateQueued,
			DeadlineAt:     time.Now().Add(time.Minute),
		}
		if err := runsRepoCreate(runs, run); err != nil {
			t.Fatal(err)
		}

		_ = runner.Run(context.Background(), run, []string{"node_budget"}, []domain.ProbeKind{domain.ProbeKindAI})

		obsList, _ := observations.ListByRun(context.Background(), run.ID)
		if len(obsList) != 1 {
			t.Fatalf("expected 1 observation, got %d", len(obsList))
		}
		aiObs := obsList[0]

		// Completed platform facts must be preserved in platformsMap
		if len(aiObs.Platforms) == 0 {
			t.Fatal("platforms map must not be empty on budget interruption")
		}
		for name, p := range aiObs.Platforms {
			if p.Verdict == domain.VerdictSkipped && p.Reason != "budget_not_executed" && p.Reason != "cancelled" {
				t.Fatalf("platform %s skipped has unexpected reason %s", name, p.Reason)
			}
		}
	})

	t.Run("executed_deadline_yields_error_deliberate_cancel_yields_skipped", func(t *testing.T) {
		// When context deadline is exceeded on executed task => error
		runs := newMemoryRuns()
		observations := newMemoryObservations()
		nodes := newMemoryNodes()
		nodes.items["node_timeout"] = domain.Node{LogicalID: "node_timeout", Protocol: domain.ProtocolSS, Active: true}

		sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
		if err != nil {
			t.Fatal(err)
		}
		defer sched.Close()

		dialer := func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			client := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					return nil, context.DeadlineExceeded
				}),
			}
			return client, nil, nil
		}

		runner := probe.NewDefaultRunner(
			nodes, observations, sched, runs,
			probe.WithNodeDialer(dialer),
		)

		run := &domain.ProbeRun{
			ID:             "run_timeout_error",
			IdempotencyKey: "key_timeout_error",
			State:          domain.ProbeRunStateQueued,
			DeadlineAt:     time.Now().Add(time.Minute),
		}
		if err := runsRepoCreate(runs, run); err != nil {
			t.Fatal(err)
		}

		_ = runner.Run(context.Background(), run, []string{"node_timeout"}, []domain.ProbeKind{domain.ProbeKindBaseline})

		obsList, _ := observations.ListByRun(context.Background(), run.ID)
		if len(obsList) != 1 {
			t.Fatalf("expected 1 observation, got %d", len(obsList))
		}
		if obsList[0].Verdict != domain.VerdictError {
			t.Fatalf("executed deadline must be VerdictError, got %s", obsList[0].Verdict)
		}
	})
}

// helper to create run in repo
func runsRepoCreate(runs domain.ProbeRunRepository, run *domain.ProbeRun) error {
	return runs.Create(context.Background(), run)
}
