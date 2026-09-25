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
	"clash-sub-parser/internal/probe/queue"
)

type memoryRuns struct {
	mu    sync.RWMutex
	items map[string]domain.ProbeRun
}

func newMemoryRuns() *memoryRuns {
	return &memoryRuns{items: make(map[string]domain.ProbeRun)}
}

func (m *memoryRuns) GetByID(_ context.Context, id string) (*domain.ProbeRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.items[id]
	if !ok {
		return nil, domain.NewNotFoundError("run_not_found", "run not found")
	}
	return &run, nil
}

func (m *memoryRuns) GetByIdempotencyKey(_ context.Context, actor, key string) (*domain.ProbeRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.items {
		if r.ActorScope == actor && r.IdempotencyKey == key {
			return &r, nil
		}
	}
	return nil, domain.NewNotFoundError("run_not_found", "run not found")
}

func (m *memoryRuns) Create(_ context.Context, run *domain.ProbeRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.items {
		if existing.ActorScope == run.ActorScope && existing.IdempotencyKey == run.IdempotencyKey {
			return domain.NewConflictError("run_already_exists", "duplicate idempotency key")
		}
	}
	m.items[run.ID] = *run
	return nil
}

func (m *memoryRuns) UpdateState(_ context.Context, id string, state domain.ProbeRunState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.items[id]
	if !ok {
		return domain.NewNotFoundError("run_not_found", "run not found")
	}
	if err := run.TransitionTo(state); err != nil {
		return err
	}
	m.items[id] = run
	return nil
}

func (m *memoryRuns) List(_ context.Context, state *domain.ProbeRunState, page, pageSize int) ([]domain.ProbeRun, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []domain.ProbeRun
	for _, r := range m.items {
		if state == nil || r.State == *state {
			res = append(res, r)
		}
	}
	return res, len(res), nil
}

func (m *memoryRuns) ListActive(_ context.Context) ([]domain.ProbeRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []domain.ProbeRun
	for _, r := range m.items {
		if !r.IsTerminal() {
			res = append(res, r)
		}
	}
	return res, nil
}

type memoryObservations struct {
	mu    sync.RWMutex
	items []domain.ProbeObservation
}

func newMemoryObservations() *memoryObservations {
	return &memoryObservations{items: make([]domain.ProbeObservation, 0)}
}

func (m *memoryObservations) GetByID(_ context.Context, id string) (*domain.ProbeObservation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, obs := range m.items {
		if obs.ID == id {
			return &obs, nil
		}
	}
	return nil, domain.NewNotFoundError("obs_not_found", "observation not found")
}

func (m *memoryObservations) ListByRun(_ context.Context, runID string) ([]domain.ProbeObservation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []domain.ProbeObservation
	for _, obs := range m.items {
		if obs.ProbeRunID == runID {
			res = append(res, obs)
		}
	}
	return res, nil
}

func (m *memoryObservations) ListByNode(_ context.Context, nodeLogicalID string, limit int) ([]domain.ProbeObservation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []domain.ProbeObservation
	for _, obs := range m.items {
		if obs.NodeLogicalID == nodeLogicalID {
			res = append(res, obs)
			if limit > 0 && len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

func (m *memoryObservations) ListLatestByNodes(_ context.Context, nodeLogicalIDs []string, kinds []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]map[domain.ProbeKind]domain.ProbeObservation)
	for _, id := range nodeLogicalIDs {
		res[id] = make(map[domain.ProbeKind]domain.ProbeObservation)
	}
	return res, nil
}

func (m *memoryObservations) Create(_ context.Context, obs *domain.ProbeObservation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, *obs)
	return nil
}

type memoryNodes struct {
	mu    sync.RWMutex
	items map[string]domain.Node
}

func newMemoryNodes() *memoryNodes {
	return &memoryNodes{items: make(map[string]domain.Node)}
}

func (m *memoryNodes) GetByLogicalID(_ context.Context, logicalID string) (*domain.Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	node, ok := m.items[logicalID]
	if !ok {
		return nil, domain.NewNotFoundError("node_not_found", "node not found")
	}
	return &node, nil
}

func (m *memoryNodes) List(_ context.Context, filter domain.NodeFilter) ([]domain.Node, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []domain.Node
	for _, node := range m.items {
		if filter.ActiveOnly && !node.Active {
			continue
		}
		res = append(res, node)
	}
	return res, len(res), nil
}

func (m *memoryNodes) ListReadModel(_ context.Context, filter domain.NodeFilter) ([]domain.NodeReadModel, int, error) {
	return nil, 0, nil
}

func (m *memoryNodes) GetReadModel(_ context.Context, logicalID string, policyRevisionID string) (*domain.NodeReadModel, error) {
	return nil, nil
}

func (m *memoryNodes) UpsertBatch(_ context.Context, nodes []domain.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range nodes {
		m.items[n.LogicalID] = n
	}
	return nil
}

func (m *memoryNodes) DeactivateNodesNotIn(_ context.Context, activeLogicalIDs []string) error {
	return nil
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func mockHTTPClient(statusCode int, body string, err error) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}),
	}
}

func TestProbeRunnerLifecycleSucceeded(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_1"] = domain.Node{LogicalID: "node_1", DisplayName: "HK 01", Protocol: domain.ProtocolSS, Active: true}
	nodesRepo.items["node_2"] = domain.Node{LogicalID: "node_2", DisplayName: "US 01", Protocol: domain.ProtocolVMess, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	var dialCount atomic.Int32
	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			dialCount.Add(1)
			client := mockHTTPClient(200, "OK", nil)
			return client, func() error { return nil }, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:             "run_test_01",
		IdempotencyKey: "test_key_01",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatalf("failed to create run: %v", err)
	}

	err = runner.Run(context.Background(), run, nil, []domain.ProbeKind{domain.ProbeKindBaseline})
	if err != nil {
		t.Fatalf("runner.Run returned unexpected error: %v", err)
	}

	updatedRun, err := runsRepo.GetByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("failed to fetch updated run: %v", err)
	}
	if updatedRun.State != domain.ProbeRunStateSucceeded {
		t.Fatalf("expected run state %s, got %s", domain.ProbeRunStateSucceeded, updatedRun.State)
	}

	observations, err := obsRepo.ListByRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("failed to list observations: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(observations))
	}
	for _, obs := range observations {
		if obs.Verdict != domain.VerdictAvailable {
			t.Errorf("expected observation verdict available, got %s", obs.Verdict)
		}
	}
}

func TestProbeRunnerSpecificNodesAndKinds(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_1"] = domain.Node{LogicalID: "node_1", DisplayName: "HK 01", Protocol: domain.ProtocolSS, Active: true}
	nodesRepo.items["node_2"] = domain.Node{LogicalID: "node_2", DisplayName: "US 01", Protocol: domain.ProtocolVMess, Active: true}
	nodesRepo.items["node_3"] = domain.Node{LogicalID: "node_3", DisplayName: "JP 01", Protocol: domain.ProtocolTrojan, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			return mockHTTPClient(200, "OK", nil), nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:             "run_test_02",
		IdempotencyKey: "test_key_02",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	_ = runsRepo.Create(context.Background(), run)

	// Target only node_1 and node_3, with 2 kinds: Baseline and Geo
	err = runner.Run(context.Background(), run, []string{"node_1", "node_3"}, []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindGeo})
	if err != nil {
		t.Fatalf("runner.Run returned unexpected error: %v", err)
	}

	observations, _ := obsRepo.ListByRun(context.Background(), run.ID)
	if len(observations) != 4 {
		t.Fatalf("expected 4 observations (2 nodes * 2 kinds), got %d", len(observations))
	}
}

func TestProbeRunnerDialerFailureProducesVerdictError(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_broken"] = domain.Node{LogicalID: "node_broken", DisplayName: "Broken", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			return nil, nil, errors.New("dial error: connection refused")
		}),
	)

	run := &domain.ProbeRun{
		ID:             "run_test_broken",
		IdempotencyKey: "test_key_broken",
		ActorScope:     "admin",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	_ = runsRepo.Create(context.Background(), run)

	err = runner.Run(context.Background(), run, []string{"node_broken"}, []domain.ProbeKind{domain.ProbeKindBaseline})
	if err != nil {
		t.Fatalf("runner.Run returned unexpected error: %v", err)
	}

	updatedRun, _ := runsRepo.GetByID(context.Background(), run.ID)
	if updatedRun.State != domain.ProbeRunStateSucceeded {
		t.Fatalf("run should still reach succeeded even if individual probes failed, got %s", updatedRun.State)
	}

	observations, _ := obsRepo.ListByRun(context.Background(), run.ID)
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	if observations[0].Verdict != domain.VerdictError {
		t.Fatalf("expected verdict error, got %s", observations[0].Verdict)
	}
}

func TestProbeServiceTriggerRunIntegration(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_1"] = domain.Node{LogicalID: "node_1", DisplayName: "Node 1", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			return mockHTTPClient(200, "OK", nil), nil, nil
		}),
	)

	svc := probe.NewService(runsRepo, probe.WithRunner(runner))

	run, err := svc.Create(context.Background(), probe.CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "trig-run-key",
		Deadline:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("failed to create run: %v", err)
	}

	// Trigger run with default runner configured in service
	if err := svc.TriggerRun(context.Background(), run.ID, nil, nil, nil); err != nil {
		t.Fatalf("TriggerRun failed: %v", err)
	}

	finalRun, _ := runsRepo.GetByID(context.Background(), run.ID)
	if finalRun.State != domain.ProbeRunStateSucceeded {
		t.Fatalf("final state = %s, want succeeded", finalRun.State)
	}
}

func TestProbeRunnerEmptyNodesFails(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(nodesRepo, obsRepo, sched, runsRepo)

	run := &domain.ProbeRun{
		ID:             "run_empty",
		IdempotencyKey: "key_empty",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	_ = runsRepo.Create(context.Background(), run)

	err = runner.Run(context.Background(), run, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty nodes run, got nil")
	}

	updated, _ := runsRepo.GetByID(context.Background(), run.ID)
	if updated.State != domain.ProbeRunStateFailed {
		t.Fatalf("expected state failed, got %s", updated.State)
	}
}

func TestProbeRunnerDefaultDialerFailsClosedWithoutPlaceholderLoopback(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_opaque"] = domain.Node{
		LogicalID:                 "node_opaque",
		DisplayName:               "Opaque Node",
		Protocol:                  domain.ProtocolSS,
		NormalizedConfigSecretRef: "secret_sha256_mock_hash",
		Active:                    true,
	}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	// Use default dialer - NO WithNodeDialer option
	runner := probe.NewDefaultRunner(nodesRepo, obsRepo, sched, runsRepo)

	run := &domain.ProbeRun{
		ID:             "run_fail_closed",
		IdempotencyKey: "key_fail_closed",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	_ = runsRepo.Create(context.Background(), run)

	err = runner.Run(context.Background(), run, []string{"node_opaque"}, []domain.ProbeKind{domain.ProbeKindBaseline})
	if err == nil {
		t.Fatal("expected runner.Run to fail when credentials are unavailable, got nil")
	}

	updated, _ := runsRepo.GetByID(context.Background(), run.ID)
	if updated.State != domain.ProbeRunStateFailed {
		t.Fatalf("expected run state failed, got %s", updated.State)
	}

	obs, _ := obsRepo.ListByRun(context.Background(), run.ID)
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	if obs[0].Verdict == domain.VerdictAvailable {
		t.Fatalf("verdict must NOT be available, got %s", obs[0].Verdict)
	}
	if obs[0].Verdict != domain.VerdictError {
		t.Fatalf("expected verdict error, got %s", obs[0].Verdict)
	}
	if !strings.Contains(obs[0].RedactedSummary, "credentials_unavailable") {
		t.Fatalf("expected summary to contain credentials_unavailable, got %s", obs[0].RedactedSummary)
	}
}

func TestProbeRunnerBaselineUnavailableCompletesWithoutMediaStage(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()
	nodesRepo.items["node_down"] = domain.Node{LogicalID: "node_down", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	var dialKinds atomic.Int32
	runner := probe.NewDefaultRunner(nodesRepo, obsRepo, sched, runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			dialKinds.Add(1)
			return mockHTTPClient(503, "unavailable", nil), nil, nil
		}),
	)
	run := &domain.ProbeRun{ID: "baseline_down", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Hour)}
	if err := runsRepo.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(context.Background(), run, []string{"node_down"}, []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindStreaming}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	updated, _ := runsRepo.GetByID(context.Background(), run.ID)
	if updated.State != domain.ProbeRunStateSucceeded {
		t.Fatalf("state = %s", updated.State)
	}
	observations, _ := obsRepo.ListByRun(context.Background(), run.ID)
	if len(observations) != 1 || observations[0].Verdict == domain.VerdictAvailable {
		t.Fatalf("observations = %#v", observations)
	}
	if dialKinds.Load() != 1 {
		t.Fatalf("dial count = %d, want baseline only", dialKinds.Load())
	}
}

func TestProbeRunnerAccessRestrictedVerdict(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_cf"] = domain.Node{LogicalID: "node_cf", DisplayName: "Cloudflare Block", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			return mockHTTPClient(200, "<html><title>Just a moment...</title><body><div class=\"cf-turnstile\">verify you are human</div></body></html>", nil), nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:             "run_cf",
		IdempotencyKey: "key_cf",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	_ = runsRepo.Create(context.Background(), run)

	if err := runner.Run(context.Background(), run, []string{"node_cf"}, []domain.ProbeKind{domain.ProbeKindBaseline}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	obs, _ := obsRepo.ListByRun(context.Background(), run.ID)
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	if obs[0].Verdict != domain.VerdictRestricted {
		t.Fatalf("expected verdict restricted, got %s", obs[0].Verdict)
	}
}

func TestProbeRunnerContextCancellation(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_slow"] = domain.Node{LogicalID: "node_slow", DisplayName: "Slow Node", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	ctx, cancel := context.WithCancel(context.Background())

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(dCtx context.Context, node domain.Node) (*http.Client, func() error, error) {
			cancel() // cancel immediately
			return mockHTTPClient(200, "OK", nil), nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:             "run_cancel",
		IdempotencyKey: "key_cancel",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	_ = runsRepo.Create(context.Background(), run)

	_ = runner.Run(ctx, run, []string{"node_slow"}, []domain.ProbeKind{domain.ProbeKindBaseline})

	updated, _ := runsRepo.GetByID(context.Background(), run.ID)
	if updated.State != domain.ProbeRunStateCancelled {
		t.Fatalf("expected state cancelled, got %s", updated.State)
	}
}

func TestProbeRunnerPartialSubmitFailureFailsRun(t *testing.T) {
	runsRepo := newMemoryRuns()
	obsRepo := newMemoryObservations()
	nodesRepo := newMemoryNodes()

	nodesRepo.items["node_1"] = domain.Node{LogicalID: "node_1", DisplayName: "N1", Protocol: domain.ProtocolSS, Active: true}
	nodesRepo.items["node_2"] = domain.Node{LogicalID: "node_2", DisplayName: "N2", Protocol: domain.ProtocolSS, Active: true}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}

	firstDialStarted := make(chan struct{})
	blockDial := make(chan struct{})

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(func(dCtx context.Context, node domain.Node) (*http.Client, func() error, error) {
			select {
			case <-firstDialStarted:
			default:
				close(firstDialStarted)
			}
			<-blockDial
			return mockHTTPClient(200, "OK", nil), nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:             "run_partial_submit",
		IdempotencyKey: "key_partial_submit",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	_ = runsRepo.Create(context.Background(), run)

	// Duplicate node submission deterministically fails while the first task is
	// active; release it so Runner can receive OnComplete and return.
	go func() {
		<-firstDialStarted
		close(blockDial)
	}()

	err = runner.Run(context.Background(), run, []string{"node_1", "node_1"}, []domain.ProbeKind{domain.ProbeKindBaseline})
	if err == nil {
		t.Fatal("expected runner.Run to fail on partial submit error, got nil")
	}

	updated, _ := runsRepo.GetByID(context.Background(), run.ID)
	if updated.State != domain.ProbeRunStateFailed {
		t.Fatalf("expected run state failed, got %s", updated.State)
	}
}
