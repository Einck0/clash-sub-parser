package probe_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
)

func TestStageAdmissionDoesNotOccupySchedulerSlotsWhileWaiting(t *testing.T) {
	runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
	firstRunNodes := []string{"media-a", "media-b", "media-c", "media-d", "media-e"}
	for _, id := range append(firstRunNodes, "unrelated") {
		nodes.items[id] = domain.Node{LogicalID: id, Protocol: domain.ProtocolSS, Active: true}
	}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 4, RunConcurrency: 4, QueueSize: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	firstMediaStarted := make(chan struct{})
	unrelatedStarted := make(chan struct{})
	releaseFirstMedia := make(chan struct{})
	var firstRunMediaRequests atomic.Int32
	runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
		probe.WithStageConcurrency(probe.StageConcurrency{Media: 1}),
		probe.WithNodeDialer(func(_ context.Context, node domain.Node) (*http.Client, func() error, error) {
			return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.HasPrefix(node.LogicalID, "media-"):
					firstRunMediaRequests.Add(1)
					if node.LogicalID == "media-a" {
						close(firstMediaStarted)
					}
					select {
					case <-releaseFirstMedia:
					case <-req.Context().Done():
						return nil, req.Context().Err()
					}
				case node.LogicalID == "unrelated":
					close(unrelatedStarted)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"country_code":"US"}`)), Request: req}, nil
			})}, nil, nil
		}),
	)

	firstRun := stageRun("admission_blocked")
	if err := runs.Create(context.Background(), firstRun); err != nil {
		t.Fatal(err)
	}
	firstRunDone := make(chan error, 1)
	go func() {
		firstRunDone <- runner.Run(context.Background(), firstRun, firstRunNodes, []domain.ProbeKind{domain.ProbeKindGeo})
	}()
	select {
	case <-firstMediaStarted:
	case <-time.After(time.Second):
		t.Fatal("first media task did not reach its blocked request")
	}

	// The first run has a single stage permit. Its next task is waiting for that
	// permit, not submitted as a scheduler callback consuming another global slot.
	otherRun := stageRun("admission_unrelated")
	if err := runs.Create(context.Background(), otherRun); err != nil {
		t.Fatal(err)
	}
	otherRunDone := make(chan error, 1)
	go func() {
		otherRunDone <- runner.Run(context.Background(), otherRun, []string{"unrelated"}, []domain.ProbeKind{domain.ProbeKindGeo})
	}()
	select {
	case <-unrelatedStarted:
	case <-time.After(time.Second):
		t.Fatal("unrelated run could not progress while the first run's stage was blocked")
	}
	if got := firstRunMediaRequests.Load(); got != 1 {
		t.Fatalf("stage limit=1 admitted %d first-run media requests, want exactly one", got)
	}

	if err := <-otherRunDone; err != nil {
		t.Fatalf("unrelated run: %v", err)
	}
	// Release the held request so its permit can advance through the first run.
	close(releaseFirstMedia)
	select {
	case err := <-firstRunDone:
		if err != nil {
			t.Fatalf("blocked run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked run did not finish after its request was released")
	}
}

func TestStageAdmissionCancellationWhileWaitingDoesNotExecuteWaitingTask(t *testing.T) {
	runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
	for _, id := range []string{"cancel-a", "cancel-b"} {
		nodes.items[id] = domain.Node{LogicalID: id, Protocol: domain.ProtocolSS, Active: true}
	}
	sched, err := queue.NewScheduler(queue.Config{Concurrency: 4, RunConcurrency: 4, QueueSize: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	started := make(chan string, 2)
	var cleanups atomic.Int32
	runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
		probe.WithStageConcurrency(probe.StageConcurrency{Media: 1}),
		probe.WithNodeDialer(func(_ context.Context, node domain.Node) (*http.Client, func() error, error) {
			return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				started <- node.LogicalID
				<-req.Context().Done()
				return nil, req.Context().Err()
			})}, func() error { cleanups.Add(1); return nil }, nil
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	run := stageRun("admission_cancel_wait")
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() {
		runDone <- runner.Run(ctx, run, []string{"cancel-a", "cancel-b"}, []domain.ProbeKind{domain.ProbeKindGeo})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first media task did not start")
	}
	cancel()
	select {
	case err := <-runDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error=%v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not promptly finish after cancellation while awaiting stage admission")
	}

	sched.Wait()
	ctxDrain, cancelDrain := context.WithTimeout(context.Background(), time.Second)
	defer cancelDrain()
	if err := sched.Drain(ctxDrain); err != nil {
		t.Fatalf("scheduler callbacks/tasks did not drain: %v", err)
	}
	select {
	case node := <-started:
		t.Fatalf("waiting stage task unexpectedly executed for %q", node)
	default:
	}
	if got := cleanups.Load(); got != 1 {
		t.Fatalf("dialer cleanup count=%d, want one closed active session", got)
	}
	got, err := observations.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, observation := range got {
		if observation.NodeLogicalID == "cancel-b" && observation.Attempt != nil && observation.Attempt.Executed {
			t.Fatalf("waiting task was recorded as executed: %+v", observation)
		}
	}
}

func TestStageAdmissionSubmissionFailureReleasesResources(t *testing.T) {
	runs, observations, nodes := newMemoryRuns(), newMemoryObservations(), newMemoryNodes()
	nodes.items["submit-failure"] = domain.Node{LogicalID: "submit-failure", Protocol: domain.ProtocolSS, Active: true}
	sched, err := queue.NewScheduler(queue.Config{Concurrency: 4, RunConcurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	if err := sched.Close(); err != nil {
		t.Fatal(err)
	}

	var dials atomic.Int32
	runner := probe.NewDefaultRunner(nodes, observations, sched, runs,
		probe.WithStageConcurrency(probe.StageConcurrency{Media: 1}),
		probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
			dials.Add(1)
			return &http.Client{}, func() error { return nil }, nil
		}),
	)
	run := stageRun("admission_submit_failure")
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(context.Background(), run, []string{"submit-failure"}, []domain.ProbeKind{domain.ProbeKindGeo}); err == nil {
		t.Fatal("Run succeeded despite scheduler submission failure")
	}
	if run.State != domain.ProbeRunStateFailed {
		t.Fatalf("run state=%s, want failed", run.State)
	}
	if got := dials.Load(); got != 0 {
		t.Fatalf("dialer calls=%d, want no execution after rejected submit", got)
	}
	ctxDrain, cancelDrain := context.WithTimeout(context.Background(), time.Second)
	defer cancelDrain()
	if err := sched.Drain(ctxDrain); err != nil {
		t.Fatalf("scheduler did not drain after rejected submit: %v", err)
	}
}
