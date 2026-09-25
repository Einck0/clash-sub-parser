package probe

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
)

type memoryScheduleRepo struct {
	mu       sync.Mutex
	schedule domain.ProbeSchedule
	batches  map[string]domain.ProbeBatch
	runs     map[string][]string // batchID -> runIDs
}

func newMemoryScheduleRepo(initial domain.ProbeSchedule) *memoryScheduleRepo {
	return &memoryScheduleRepo{
		schedule: initial,
		batches:  make(map[string]domain.ProbeBatch),
		runs:     make(map[string][]string),
	}
}

func (m *memoryScheduleRepo) Get(_ context.Context) (*domain.ProbeSchedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := m.schedule
	return &copy, nil
}

func (m *memoryScheduleRepo) Update(_ context.Context, s *domain.ProbeSchedule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schedule = *s
	return nil
}

func (m *memoryScheduleRepo) GetBatchByID(_ context.Context, id string) (*domain.ProbeBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.batches[id]
	if !ok {
		return nil, domain.NewNotFoundError("probe_batch_not_found", "not found")
	}
	copy := b
	copy.RunIDs = append([]string{}, m.runs[id]...)
	return &copy, nil
}

func (m *memoryScheduleRepo) ListBatches(_ context.Context, page, pageSize int) ([]domain.ProbeBatch, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]domain.ProbeBatch, 0, len(m.batches))
	for id, b := range m.batches {
		copy := b
		copy.RunIDs = append([]string{}, m.runs[id]...)
		res = append(res, copy)
	}
	return res, len(res), nil
}

func (m *memoryScheduleRepo) GetBatchByWindow(_ context.Context, generation int64, windowAt time.Time) (*domain.ProbeBatch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, b := range m.batches {
		if b.Generation == generation && b.WindowAt.Equal(windowAt) {
			copy := b
			copy.RunIDs = append([]string{}, m.runs[id]...)
			return &copy, nil
		}
	}
	return nil, domain.NewNotFoundError("probe_batch_not_found", "not found")
}

func (m *memoryScheduleRepo) CreateBatch(_ context.Context, b *domain.ProbeBatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.batches {
		if existing.Generation == b.Generation && existing.WindowAt.Equal(b.WindowAt) {
			return domain.NewConflictError("probe_batch_already_exists", "already exists")
		}
	}
	m.batches[b.ID] = *b
	m.runs[b.ID] = append([]string{}, b.RunIDs...)
	return nil
}

func (m *memoryScheduleRepo) UpdateBatch(_ context.Context, b *domain.ProbeBatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.batches[b.ID]; !ok {
		return domain.NewNotFoundError("probe_batch_not_found", "not found")
	}
	m.batches[b.ID] = *b
	m.runs[b.ID] = append([]string{}, b.RunIDs...)
	return nil
}

func (m *memoryScheduleRepo) AcquireLease(_ context.Context, batchID string, owner string, leaseDuration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.batches[batchID]
	if !ok {
		return false, domain.NewNotFoundError("probe_batch_not_found", "not found")
	}
	if b.State.IsTerminal() {
		return false, nil
	}
	now := time.Now().UTC()
	if b.Owner != "" && b.Owner != owner && b.LeaseUntil != nil && b.LeaseUntil.After(now) {
		return false, nil
	}
	until := now.Add(leaseDuration)
	b.Owner = owner
	b.LeaseUntil = &until
	m.batches[batchID] = b
	return true, nil
}

func (m *memoryScheduleRepo) HeartbeatLease(_ context.Context, batchID string, owner string, leaseDuration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.batches[batchID]
	if !ok {
		return domain.NewNotFoundError("probe_batch_not_found", "not found")
	}
	if b.Owner != owner || b.State.IsTerminal() {
		return domain.NewConflictError("lost_lease", "lost lease")
	}
	until := time.Now().UTC().Add(leaseDuration)
	b.LeaseUntil = &until
	m.batches[batchID] = b
	return nil
}

func (m *memoryScheduleRepo) ReleaseLease(_ context.Context, batchID string, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.batches[batchID]
	if !ok || b.Owner != owner {
		return nil
	}
	b.Owner = ""
	b.LeaseUntil = nil
	m.batches[batchID] = b
	return nil
}

type memoryNodeRepo struct {
	mu    sync.Mutex
	nodes []domain.Node
}

func (m *memoryNodeRepo) List(_ context.Context, filter domain.NodeFilter) ([]domain.Node, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []domain.Node
	for _, n := range m.nodes {
		if filter.ActiveOnly && !n.Active {
			continue
		}
		filtered = append(filtered, n)
	}
	total := len(filtered)
	if filter.Pagination.PageSize <= 0 {
		return filtered, total, nil
	}
	pageSize := filter.Pagination.PageSize
	page := filter.Pagination.Page
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * pageSize
	if start >= total {
		return []domain.Node{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return filtered[start:end], total, nil
}

func (m *memoryNodeRepo) GetByLogicalID(_ context.Context, id string) (*domain.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.LogicalID == id {
			copy := n
			return &copy, nil
		}
	}
	return nil, domain.NewNotFoundError("node_not_found", "not found")
}

func (m *memoryNodeRepo) UpsertBatch(_ context.Context, nodes []domain.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes = append(m.nodes, nodes...)
	return nil
}

func (m *memoryNodeRepo) ListReadModel(_ context.Context, _ domain.NodeFilter) ([]domain.NodeReadModel, int, error) {
	return nil, 0, nil
}

func (m *memoryNodeRepo) GetReadModel(_ context.Context, _ string, _ string) (*domain.NodeReadModel, error) {
	return nil, nil
}

func (m *memoryNodeRepo) DeactivateNodesNotIn(_ context.Context, _ []string) error {
	return nil
}

type mockRunner struct {
	mu           sync.Mutex
	executedRuns []*domain.ProbeRun
	executedNodeIDs map[string][]string
}

func (m *mockRunner) Run(_ context.Context, run *domain.ProbeRun, nodeIDs []string, _ []domain.ProbeKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executedRuns = append(m.executedRuns, run)
	if m.executedNodeIDs == nil {
		m.executedNodeIDs = make(map[string][]string)
	}
	m.executedNodeIDs[run.ID] = nodeIDs
	return nil
}

func TestPeriodicCoordinator_TriggerWindow_ShardingAndCounts(t *testing.T) {
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	runner := &mockRunner{}

	// Setup 5 nodes:
	// 3 nodes with CredentialVersion > 0 (eligible)
	// 2 nodes with CredentialVersion = 0 (fail-closed skip)
	nodes := []domain.Node{
		{LogicalID: "node-1", DisplayName: "Node 1", Active: true, CredentialVersion: 1},
		{LogicalID: "node-2", DisplayName: "Node 2", Active: true, CredentialVersion: 0}, // skipped
		{LogicalID: "node-3", DisplayName: "Node 3", Active: true, CredentialVersion: 2},
		{LogicalID: "node-4", DisplayName: "Node 4", Active: true, CredentialVersion: -1}, // skipped
		{LogicalID: "node-5", DisplayName: "Node 5", Active: true, CredentialVersion: 1},
	}
	nodeRepo := &memoryNodeRepo{nodes: nodes}

	// Set maxTasksPerRun = 2, so 3 eligible nodes will be sharded into 2 runs: [2 nodes] + [1 node]
	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorMaxTasks(2),
		WithCoordinatorOwner("test-coord-1"),
	)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}

	// Verify batch status
	batches, _, err := schedRepo.ListBatches(ctx, 1, 10)
	if err != nil || len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d (err: %v)", len(batches), err)
	}

	b := batches[0]
	if b.State != domain.ProbeBatchStateSucceeded {
		t.Fatalf("expected batch state succeeded, got %s", b.State)
	}
	if b.Counts.TotalNodes != 5 {
		t.Fatalf("expected total_nodes 5, got %d", b.Counts.TotalNodes)
	}
	if b.Counts.SkippedNodes != 2 {
		t.Fatalf("expected skipped_nodes 2, got %d", b.Counts.SkippedNodes)
	}
	if b.Counts.DispatchedRuns != 2 {
		t.Fatalf("expected dispatched_runs 2, got %d", b.Counts.DispatchedRuns)
	}
	if b.Counts.CompletedRuns != 2 {
		t.Fatalf("expected completed_runs 2, got %d", b.Counts.CompletedRuns)
	}
	if len(b.RunIDs) != 2 {
		t.Fatalf("expected 2 run IDs in batch, got %d", len(b.RunIDs))
	}

	// Verify runner executed 2 runs
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.executedRuns) != 2 {
		t.Fatalf("expected 2 executed runs, got %d", len(runner.executedRuns))
	}
}

func TestPeriodicCoordinator_EmptyInventory(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{nodes: nil}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	runner := &mockRunner{}

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return now }),
	)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow on empty inventory failed: %v", err)
	}

	batches, _, _ := schedRepo.ListBatches(context.Background(), 1, 10)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	b := batches[0]
	if b.State != domain.ProbeBatchStateSucceeded {
		t.Fatalf("expected batch state succeeded, got %s", b.State)
	}
	if b.Counts.TotalNodes != 0 || b.Counts.DispatchedRuns != 0 {
		t.Fatalf("expected 0 total/dispatched nodes, got %+v", b.Counts)
	}
}

func TestPeriodicCoordinator_MultiInstanceCASContention(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	sharedSchedRepo := newMemoryScheduleRepo(sched)
	sharedNodeRepo := &memoryNodeRepo{nodes: []domain.Node{
		{LogicalID: "n1", DisplayName: "N1", Active: true, CredentialVersion: 1},
	}}
	sharedRunRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	runner := &mockRunner{}

	coord1 := NewPeriodicCoordinator(
		sharedSchedRepo, sharedNodeRepo, sharedRunRepo, runner,
		WithCoordinatorOwner("instance-alpha"),
		WithCoordinatorClock(func() time.Time { return now }),
	)
	coord2 := NewPeriodicCoordinator(
		sharedSchedRepo, sharedNodeRepo, sharedRunRepo, runner,
		WithCoordinatorOwner("instance-beta"),
		WithCoordinatorClock(func() time.Time { return now }),
	)

	// Both trigger the same window concurrently
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = coord1.TriggerWindow()
	}()
	go func() {
		defer wg.Done()
		_ = coord2.TriggerWindow()
	}()
	wg.Wait()

	batches, _, _ := sharedSchedRepo.ListBatches(context.Background(), 1, 10)
	if len(batches) != 1 {
		t.Fatalf("expected exactly 1 batch created across instances, got %d", len(batches))
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	// Only 1 instance should execute the run
	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected exactly 1 run dispatched across instances, got %d", len(runner.executedRuns))
	}
}

func TestPeriodicCoordinator_StartupRecovery(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	pastWindow := now.Add(-2 * time.Hour)
	pastLease := now.Add(-1 * time.Hour)

	abandonedRun := domain.ProbeRun{
		ID:         "run-abandoned",
		State:      domain.ProbeRunStateRunning,
		DeadlineAt: pastWindow.Add(10 * time.Minute),
		CreatedAt:  pastWindow,
		UpdatedAt:  pastWindow,
	}
	runRepo := &memoryRuns{items: map[string]domain.ProbeRun{
		abandonedRun.ID: abandonedRun,
	}}

	abandonedBatch := domain.ProbeBatch{
		ID:         "batch-abandoned",
		WindowAt:   pastWindow,
		Generation: 1,
		Owner:      "crashed-worker",
		LeaseUntil: &pastLease,
		State:      domain.ProbeBatchStateRunning,
		RunIDs:     []string{abandonedRun.ID},
		CreatedAt:  pastWindow,
		UpdatedAt:  pastWindow,
	}

	overdueSched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &pastWindow, // Overdue
		Generation:      1,
		UpdatedAt:       pastWindow,
	}

	schedRepo := newMemoryScheduleRepo(overdueSched)
	_ = schedRepo.CreateBatch(ctx, &abandonedBatch)

	coord := NewPeriodicCoordinator(
		schedRepo,
		&memoryNodeRepo{},
		runRepo,
		&mockRunner{},
		WithCoordinatorClock(func() time.Time { return now }),
	)

	// Run recovery
	if err := coord.Recover(ctx); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	// 1. Verify batch marked expired
	b, err := schedRepo.GetBatchByID(ctx, abandonedBatch.ID)
	if err != nil {
		t.Fatalf("failed to get recovered batch: %v", err)
	}
	if b.State != domain.ProbeBatchStateExpired {
		t.Fatalf("expected abandoned batch state expired, got %s", b.State)
	}

	// 2. Verify run marked expired
	r, err := runRepo.GetByID(ctx, abandonedRun.ID)
	if err != nil {
		t.Fatalf("failed to get recovered run: %v", err)
	}
	if r.State != domain.ProbeRunStateExpired {
		t.Fatalf("expected abandoned run state expired, got %s", r.State)
	}

	// 3. Verify schedule next_due_at advanced to future (not overdue)
	sched, err := schedRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get recovered schedule: %v", err)
	}
	if sched.NextDueAt == nil || !sched.NextDueAt.After(now) {
		t.Fatalf("expected next_due_at advanced to future, got %v (now: %v)", sched.NextDueAt, now)
	}
}

func TestObservationCredentialVersionWritten(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	var recordedObs []*domain.ProbeObservation
	var obsMu sync.Mutex

	obsRepo := &testObsRepo{
		createFn: func(obs *domain.ProbeObservation) error {
			obsMu.Lock()
			defer obsMu.Unlock()
			recordedObs = append(recordedObs, obs)
			return nil
		},
	}

	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-v3", DisplayName: "Node V3", Active: true, CredentialVersion: 3},
		},
	}

	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	sched, err := queue.NewScheduler(queue.Config{})
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}
	defer sched.Close()

	runner := NewDefaultRunner(
		nodeRepo,
		obsRepo,
		sched,
		runRepo,
		WithNodeDialer(func(_ context.Context, _ domain.Node) (*http.Client, func() error, error) {
			return nil, nil, nil
		}),
	)

	run := &domain.ProbeRun{
		ID:             domain.MustNewUUIDv7(),
		IdempotencyKey: "test-cred-ver",
		ActorScope:     "test",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     now.Add(time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_ = runRepo.Create(ctx, run)

	if err := runner.Run(ctx, run, []string{"node-v3"}, []domain.ProbeKind{domain.ProbeKindBaseline}); err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	obsMu.Lock()
	defer obsMu.Unlock()
	if len(recordedObs) != 1 {
		t.Fatalf("expected 1 recorded observation, got %d", len(recordedObs))
	}

	obs := recordedObs[0]
	if obs.CredentialVersion == nil {
		t.Fatalf("expected CredentialVersion to be non-nil, got nil")
	}
	if *obs.CredentialVersion != 3 {
		t.Fatalf("expected CredentialVersion=3, got %d", *obs.CredentialVersion)
	}
}

type testObsRepo struct {
	createFn func(*domain.ProbeObservation) error
}

func (t *testObsRepo) GetByID(_ context.Context, _ string) (*domain.ProbeObservation, error) {
	return nil, nil
}
func (t *testObsRepo) ListByRun(_ context.Context, _ string) ([]domain.ProbeObservation, error) {
	return nil, nil
}
func (t *testObsRepo) ListByNode(_ context.Context, _ string, _ int) ([]domain.ProbeObservation, error) {
	return nil, nil
}
func (t *testObsRepo) ListLatestByNodes(_ context.Context, _ []string, _ []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	return nil, nil
}
func (t *testObsRepo) Create(_ context.Context, obs *domain.ProbeObservation) error {
	if t.createFn != nil {
		return t.createFn(obs)
	}
	return nil
}

func TestPeriodicCoordinator_LargeInventoryPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	runner := &mockRunner{}

	// Setup 250 active nodes (which exceeds single page size of 100)
	nodes := make([]domain.Node, 250)
	for i := 0; i < 250; i++ {
		nodes[i] = domain.Node{
			LogicalID:         fmt.Sprintf("node-%03d", i),
			DisplayName:       fmt.Sprintf("Node %d", i),
			Active:            true,
			CredentialVersion: 1,
		}
	}
	nodeRepo := &memoryNodeRepo{nodes: nodes}

	// maxTasks = 50, so 250 nodes with 1 kind should partition into 5 runs
	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorMaxTasks(50),
		WithCoordinatorOwner("test-coord-large"),
	)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed for large inventory: %v", err)
	}

	batches, _, err := schedRepo.ListBatches(ctx, 1, 10)
	if err != nil || len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d (err: %v)", len(batches), err)
	}

	b := batches[0]
	if b.State != domain.ProbeBatchStateSucceeded {
		t.Fatalf("expected batch state succeeded, got %s", b.State)
	}
	if b.Counts.TotalNodes != 250 {
		t.Fatalf("expected total_nodes 250, got %d", b.Counts.TotalNodes)
	}
	if b.Counts.DispatchedRuns != 5 {
		t.Fatalf("expected dispatched_runs 5, got %d", b.Counts.DispatchedRuns)
	}
	if b.Counts.CompletedRuns != 5 {
		t.Fatalf("expected completed_runs 5, got %d", b.Counts.CompletedRuns)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.executedRuns) != 5 {
		t.Fatalf("expected runner to execute 5 runs, got %d", len(runner.executedRuns))
	}
}

func TestPeriodicCoordinator_CrashTakeoverIdempotency(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	runner := &mockRunner{}

	nodes := []domain.Node{
		{LogicalID: "node-1", DisplayName: "Node 1", Active: true, CredentialVersion: 1},
		{LogicalID: "node-2", DisplayName: "Node 2", Active: true, CredentialVersion: 1},
	}
	nodeRepo := &memoryNodeRepo{nodes: nodes}

	// Instance 1 creates batch with 2 chunks (maxTasks = 1)
	// But it simulates crashing after chunk 0 succeeds:
	batchID := domain.MustNewUUIDv7()
	runID0 := domain.MustNewUUIDv7()
	runID1 := domain.MustNewUUIDv7()
	key0 := fmt.Sprintf("periodic:1:%s:0", now.Format(time.RFC3339))
	key1 := fmt.Sprintf("periodic:1:%s:1", now.Format(time.RFC3339))

	run0 := domain.ProbeRun{
		ID:             runID0,
		IdempotencyKey: key0,
		ActorScope:     "system:periodic-probe",
		State:          domain.ProbeRunStateSucceeded, // Already completed
		DeadlineAt:     now.Add(10 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	run1 := domain.ProbeRun{
		ID:             runID1,
		IdempotencyKey: key1,
		ActorScope:     "system:periodic-probe",
		State:          domain.ProbeRunStateQueued, // Needs to run
		DeadlineAt:     now.Add(10 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_ = runRepo.Create(ctx, &run0)
	_ = runRepo.Create(ctx, &run1)

	pastLease := now.Add(-time.Second) // Lease expired
	preexistingBatch := domain.ProbeBatch{
		ID:         batchID,
		WindowAt:   now,
		Generation: 1,
		Owner:      "crashed-worker",
		LeaseUntil: &pastLease,
		State:      domain.ProbeBatchStateRunning,
		RunIDs:     []string{runID0, runID1},
		Counts: domain.ProbeBatchCounts{
			TotalNodes:     2,
			DispatchedRuns: 2,
			CompletedRuns:  1,
			SkippedNodes:   0,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = schedRepo.CreateBatch(ctx, &preexistingBatch)

	// Instance 2 takes over the batch
	coord2 := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorMaxTasks(1),
		WithCoordinatorOwner("takeover-worker"),
	)

	if err := coord2.TriggerWindow(); err != nil {
		t.Fatalf("takeover TriggerWindow failed: %v", err)
	}

	// Verify that runner only executed run1 (chunk 1), NOT run0
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected exactly 1 run re-executed on takeover, got %d", len(runner.executedRuns))
	}
	if runner.executedRuns[0].IdempotencyKey != key1 {
		t.Fatalf("expected run1 to be executed, got %s", runner.executedRuns[0].IdempotencyKey)
	}

	finalBatch, _ := schedRepo.GetBatchByID(ctx, batchID)
	if finalBatch.State != domain.ProbeBatchStateSucceeded {
		t.Fatalf("expected final batch state succeeded, got %s", finalBatch.State)
	}
	if finalBatch.Counts.CompletedRuns != 2 {
		t.Fatalf("expected completed_runs 2, got %d", finalBatch.Counts.CompletedRuns)
	}
}
