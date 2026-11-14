package probe_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
	"clash-sub-parser/internal/probe/queue"
)

type countingProbeSettings struct {
	domain.SettingsRepository
	reads   atomic.Int32
	seconds atomic.Int32
}

func (s *countingProbeSettings) Get(context.Context) (*domain.Settings, error) {
	s.reads.Add(1)
	return &domain.Settings{ProbeTimeoutSeconds: int(s.seconds.Load())}, nil
}

func TestRunnerFreezesTimeoutOncePerRun(t *testing.T) {
	runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("node-%d", i)
		nodes.items[id] = domain.Node{LogicalID: id, Protocol: domain.ProtocolSS, Active: true}
	}
	sched, err := queue.NewScheduler(queue.Config{Concurrency: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()
	settings := &countingProbeSettings{}
	settings.seconds.Store(4)
	var requests atomic.Int32
	runner := probe.NewDefaultRunner(nodes, observations, sched, runs, probe.WithSettingsRepository(settings), probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
		// A construction-time timeout must not cap the run's frozen setting.
		return &http.Client{Timeout: time.Second, Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			requests.Add(1)
			// Changing the setting while requests execute only affects the next run.
			settings.seconds.Store(9)
			deadline, ok := req.Context().Deadline()
			remaining := time.Until(deadline)
			want := 4 * time.Second
			if settings.reads.Load() == 2 {
				want = 9 * time.Second
			}
			if !ok || remaining > want || remaining < want-time.Second {
				t.Errorf("task timeout=%s want approximately %s", remaining, want)
			}
			body := ""
			if strings.Contains(req.URL.Path, "geoip") {
				body = `{"ip":"203.0.113.1","country_code":"US"}`
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		})}, nil, nil
	}))
	for i := 0; i < 2; i++ {
		run := stageRun(fmt.Sprintf("timeout-%d", i))
		run.IdempotencyKey = run.ID
		if err := runs.Create(context.Background(), run); err != nil {
			t.Fatal(err)
		}
		if err := runner.Run(context.Background(), run, nil, []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindGeo}); err != nil {
			t.Fatal(err)
		}
		if got := settings.reads.Load(); got != int32(i+1) {
			t.Fatalf("settings reads=%d, runs=%d", got, i+1)
		}
	}
	if requests.Load() != 32 {
		t.Fatalf("requests=%d want 32", requests.Load())
	}
	t.Logf("settings_reads=%d tasks=%d", settings.reads.Load(), requests.Load())
}

type failingEvidenceRepository struct {
	*memoryObservations
	failKind domain.ProbeKind
	err      error
	calls    atomic.Int32
}

func (r *failingEvidenceRepository) Create(ctx context.Context, obs *domain.ProbeObservation) error {
	if obs.Kind == r.failKind {
		r.calls.Add(1)
		return r.err
	}
	return r.memoryObservations.Create(ctx, obs)
}

type failingRiskEvidence struct {
	domain.IPRiskObservationRepository
	err error
}

func (r *failingRiskEvidence) Create(context.Context, *domain.IPRiskObservation) error { return r.err }

func TestRunnerDoesNotSucceedWhenEvidenceCannotBeSaved(t *testing.T) {
	for _, scenario := range []string{"executed", "dependency_skipped", "normalized_risk"} {
		t.Run(scenario, func(t *testing.T) {
			runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
			nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
			sched, err := queue.NewScheduler(queue.Config{Concurrency: 4})
			if err != nil {
				t.Fatal(err)
			}
			defer sched.Close()
			saveErr := errors.New("synthetic evidence write failure")
			failed := &failingEvidenceRepository{memoryObservations: observations, failKind: domain.ProbeKindBaseline, err: saveErr}
			var repo domain.ProbeObservationRepository = failed
			kinds := []domain.ProbeKind{domain.ProbeKindBaseline}
			opts := []probe.DefaultRunnerOption{probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
				return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					code := 200
					if scenario == "dependency_skipped" {
						code = 503
					}
					return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
				})}, nil, nil
			})}
			if scenario == "dependency_skipped" {
				failed.failKind = domain.ProbeKindAI
				kinds = append(kinds, domain.ProbeKindAI)
			}
			if scenario == "normalized_risk" {
				repo = observations
				kinds = []domain.ProbeKind{domain.ProbeKindIPRisk}
				opts = append(opts, probe.WithIPRiskObservationRepository(&failingRiskEvidence{err: saveErr}))
			}
			runner := probe.NewDefaultRunner(nodes, repo, sched, runs, opts...)
			run := stageRun("write-failure")
			if err := runs.Create(context.Background(), run); err != nil {
				t.Fatal(err)
			}
			if err := runner.Run(context.Background(), run, []string{"node"}, kinds); !errors.Is(err, saveErr) {
				t.Fatalf("Run error=%v want persistence failure", err)
			}
			stored, err := runs.GetByID(context.Background(), run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.State != domain.ProbeRunStateFailed {
				t.Fatalf("persisted run state=%s", stored.State)
			}
			if scenario == "executed" && failed.calls.Load() != 1 {
				t.Fatalf("executed result was replaced with synthetic skipped evidence: writes=%d", failed.calls.Load())
			}
		})
	}
}

type closedEvidenceBody struct {
	io.Reader
	closed chan struct{}
}

func (b *closedEvidenceBody) Close() error { close(b.closed); return nil }

func TestRunnerPreservesCompletedPlatformEvidenceOnSiblingInterruption(t *testing.T) {
	for _, interruption := range []error{platform.ErrBodyBudget, context.Canceled, context.DeadlineExceeded} {
		t.Run(interruption.Error(), func(t *testing.T) {
			runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
			nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
			sched, err := queue.NewScheduler(queue.Config{Concurrency: 4})
			if err != nil {
				t.Fatal(err)
			}
			defer sched.Close()
			restrictedComplete := make(chan struct{})
			runner := probe.NewDefaultRunner(nodes, observations, sched, runs, probe.WithPlatforms([]string{"gemini", "claude"}), probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
				return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host == "claude.ai" {
						select {
						case <-restrictedComplete:
							return nil, interruption
						case <-req.Context().Done():
							return nil, req.Context().Err()
						}
					}
					return &http.Response{StatusCode: 403, Header: make(http.Header), Body: &closedEvidenceBody{Reader: strings.NewReader(""), closed: restrictedComplete}, Request: req}, nil
				})}, nil, nil
			}))
			run := stageRun("sibling-interrupted")
			if err := runs.Create(context.Background(), run); err != nil {
				t.Fatal(err)
			}
			_ = runner.Run(context.Background(), run, []string{"node"}, []domain.ProbeKind{domain.ProbeKindAI})
			rows, _ := observations.ListByRun(context.Background(), run.ID)
			if len(rows) != 1 {
				t.Fatalf("observations=%d", len(rows))
			}
			gemini := rows[0].Platforms["gemini"]
			if gemini.Verdict != domain.VerdictRestricted || gemini.Reason != "access_restricted" {
				t.Fatalf("completed restriction overwritten: %+v", gemini)
			}
			if rows[0].Verdict != domain.VerdictRestricted {
				t.Fatalf("aggregate=%s want restricted", rows[0].Verdict)
			}
		})
	}
}

func TestRunnerNetflixEmptyCDNPersistsInterruptedFallback(t *testing.T) {
	for _, tc := range []struct {
		name            string
		err             error
		verdict         domain.ProbeVerdict
		reason          string
		aggregateReason string
	}{
		{"cancel", context.Canceled, domain.VerdictSkipped, "cancelled", "not_executed"},
		{"budget", platform.ErrBodyBudget, domain.VerdictSkipped, "budget_not_executed", "not_executed"},
		{"deadline", context.DeadlineExceeded, domain.VerdictError, "deadline_exceeded", "detection_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
			nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
			sched, err := queue.NewScheduler(queue.Config{Concurrency: 4})
			if err != nil {
				t.Fatal(err)
			}
			defer sched.Close()
			runner := probe.NewDefaultRunner(nodes, observations, sched, runs, probe.WithPlatforms([]string{"netflix"}), probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
				return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host == "api.fast.com" {
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"targets":[]}`)), Request: req}, nil
					}
					return nil, tc.err
				})}, nil, nil
			}))
			run := stageRun("netflix-interruption")
			if err := runs.Create(context.Background(), run); err != nil {
				t.Fatal(err)
			}
			_ = runner.Run(context.Background(), run, []string{"node"}, []domain.ProbeKind{domain.ProbeKindStreaming})
			rows, err := observations.ListByRun(context.Background(), run.ID)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%d err=%v", len(rows), err)
			}
			row := rows[0]
			cap := row.Platforms["netflix"]
			if cap.Verdict != tc.verdict || cap.Reason != tc.reason {
				t.Fatalf("platform=%+v want %s/%s", cap, tc.verdict, tc.reason)
			}
			if row.Verdict != tc.verdict || row.Attempt == nil || row.Attempt.Reason != tc.aggregateReason {
				t.Fatalf("persisted aggregate=%+v want %s/%s", row, tc.verdict, tc.aggregateReason)
			}
		})
	}
}

func TestRunnerEmptyPlatformSelectionIsSkipped(t *testing.T) {
	runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
	nodes.items["node"] = domain.Node{LogicalID: "node", Protocol: domain.ProtocolSS, Active: true}
	sched, err := queue.NewScheduler(queue.Config{Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()
	runner := probe.NewDefaultRunner(nodes, observations, sched, runs, probe.WithPlatforms([]string{}), probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
		return &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			t.Error("unselected platform requested")
			return nil, errors.New("unexpected request")
		})}, nil, nil
	}))
	run := stageRun("empty-selection")
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(context.Background(), run, []string{"node"}, []domain.ProbeKind{domain.ProbeKindAI, domain.ProbeKindStreaming}); err != nil {
		t.Fatal(err)
	}
	rows, _ := observations.ListByRun(context.Background(), run.ID)
	if len(rows) != 2 {
		t.Fatalf("observations=%d", len(rows))
	}
	for _, row := range rows {
		if row.Verdict != domain.VerdictSkipped || row.Attempt.Reason != "no_platforms_selected" {
			t.Fatalf("empty selection: %+v", row)
		}
	}
}
