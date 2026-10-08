package probe

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
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
	existing, ok := m.batches[b.ID]
	if !ok {
		return domain.NewNotFoundError("probe_batch_not_found", "not found")
	}
	existing.State = b.State
	existing.Counts = b.Counts
	existing.RedactedError = b.RedactedError
	existing.UpdatedAt = b.UpdatedAt
	existing.RunIDs = append([]string{}, b.RunIDs...)
	m.batches[b.ID] = existing
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
	idSet := make(map[string]bool, len(filter.LogicalIDs))
	for _, id := range filter.LogicalIDs {
		idSet[id] = true
	}
	for _, n := range m.nodes {
		if len(filter.LogicalIDs) > 0 && !idSet[n.LogicalID] {
			continue
		}
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

func (m *memoryNodeRepo) ListAll(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, error) {
	unpaginated := filter
	unpaginated.Pagination.PageSize = 0
	nodes, _, err := m.List(ctx, unpaginated)
	return nodes, err
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
	mu              sync.Mutex
	executedRuns    []*domain.ProbeRun
	executedNodeIDs map[string][]string
	runFn           func(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error
}

func (m *mockRunner) Run(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error {
	m.mu.Lock()
	m.executedRuns = append(m.executedRuns, run)
	if m.executedNodeIDs == nil {
		m.executedNodeIDs = make(map[string][]string)
	}
	m.executedNodeIDs[run.ID] = nodeIDs
	fn := m.runFn
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, run, nodeIDs, kinds)
	}
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

	// Setup 5 active nodes (all eligible without credential version gate)
	nodes := []domain.Node{
		{LogicalID: "node-1", DisplayName: "Node 1", Active: true},
		{LogicalID: "node-2", DisplayName: "Node 2", Active: true},
		{LogicalID: "node-3", DisplayName: "Node 3", Active: true},
		{LogicalID: "node-4", DisplayName: "Node 4", Active: true},
		{LogicalID: "node-5", DisplayName: "Node 5", Active: true},
	}
	nodeRepo := &memoryNodeRepo{nodes: nodes}

	// Set maxTasksPerRun = 2, so 5 active nodes will be sharded into 3 runs: [2 nodes] + [2 nodes] + [1 node]
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
	if b.Counts.SkippedNodes != 0 {
		t.Fatalf("expected skipped_nodes 0, got %d", b.Counts.SkippedNodes)
	}
	if b.Counts.DispatchedRuns != 3 {
		t.Fatalf("expected dispatched_runs 3, got %d", b.Counts.DispatchedRuns)
	}
	if b.Counts.CompletedRuns != 3 {
		t.Fatalf("expected completed_runs 3, got %d", b.Counts.CompletedRuns)
	}
	if len(b.RunIDs) != 3 {
		t.Fatalf("expected 3 run IDs in batch, got %d", len(b.RunIDs))
	}

	// Verify runner executed 3 runs
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.executedRuns) != 3 {
		t.Fatalf("expected 3 executed runs, got %d", len(runner.executedRuns))
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
		{LogicalID: "n1", DisplayName: "N1", Active: true},
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

	// 3. Verify schedule next_due_at corrected to now for immediate sweep (not delayed to future)
	sched, err := schedRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get recovered schedule: %v", err)
	}
	if sched.NextDueAt == nil || !sched.NextDueAt.Equal(now) {
		t.Fatalf("expected next_due_at corrected to now for immediate sweep, got %v (expected %v)", sched.NextDueAt, now)
	}

	// 4. Verify schedule with nil NextDueAt is also corrected to now
	sched.NextDueAt = nil
	_ = schedRepo.Update(ctx, sched)
	if err := coord.Recover(ctx); err != nil {
		t.Fatalf("Recover failed for nil NextDueAt: %v", err)
	}
	sched, _ = schedRepo.Get(ctx)
	if sched.NextDueAt == nil || !sched.NextDueAt.Equal(now) {
		t.Fatalf("expected nil next_due_at corrected to now, got %v", sched.NextDueAt)
	}

	// 5. Verify schedule with legacy far-future NextDueAt is corrected to now
	farFuture := now.Add(2 * time.Hour)
	sched.NextDueAt = &farFuture
	_ = schedRepo.Update(ctx, sched)
	if err := coord.Recover(ctx); err != nil {
		t.Fatalf("Recover failed for far-future NextDueAt: %v", err)
	}
	sched, _ = schedRepo.Get(ctx)
	if sched.NextDueAt == nil || !sched.NextDueAt.Equal(now) {
		t.Fatalf("expected far-future next_due_at corrected to now, got %v", sched.NextDueAt)
	}

	// 6. Verify valid near-future NextDueAt (e.g. now + 30s within 1m sweep interval) is preserved
	nearFuture := now.Add(30 * time.Second)
	sched.NextDueAt = &nearFuture
	_ = schedRepo.Update(ctx, sched)
	if err := coord.Recover(ctx); err != nil {
		t.Fatalf("Recover failed for near-future NextDueAt: %v", err)
	}
	sched, _ = schedRepo.Get(ctx)
	if sched.NextDueAt == nil || !sched.NextDueAt.Equal(nearFuture) {
		t.Fatalf("expected near-future next_due_at preserved, got %v (expected %v)", sched.NextDueAt, nearFuture)
	}
}

func TestPeriodicCoordinator_StartupRecovery_ImmediateSweepWithFakeClock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now().UTC().Truncate(time.Second)
	overdue := now.Add(-1 * time.Hour)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &overdue,
		Generation:      1,
		UpdatedAt:       overdue,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{nodes: []domain.Node{
		{LogicalID: "node-sweep-1", DisplayName: "Sweep Node 1", Active: true},
	}}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	runner := &mockRunner{}

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorOwner("recovery-worker"),
		WithCoordinatorClock(func() time.Time { return now }),
	)

	if err := coord.Recover(ctx); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	// Verify next_due_at was corrected to now
	recoveredSched, _ := schedRepo.Get(ctx)
	if recoveredSched.NextDueAt == nil || !recoveredSched.NextDueAt.Equal(now) {
		t.Fatalf("expected next_due_at set to now, got %v", recoveredSched.NextDueAt)
	}

	// Start coordinator loop and verify immediate sweep triggers without sleeping
	sweepDone := make(chan struct{})
	runner.runFn = func(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error {
		select {
		case <-sweepDone:
		default:
			close(sweepDone)
		}
		return nil
	}

	coord.Start(ctx)
	defer coord.Stop()

	select {
	case <-sweepDone:
		// Succeeded immediately on startup
	case <-time.After(2 * time.Second):
		t.Fatal("expected startup sweep to trigger immediately, timed out")
	}
}

func TestPeriodicCoordinator_MultiInstanceLeaseWithMultipleUpdateBatches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now().UTC().Truncate(time.Second)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	// 5 nodes, maxTasksPerRun = 1 -> will produce 5 separate runs and multiple UpdateBatch calls during execution
	nodes := make([]domain.Node, 5)
	for i := 0; i < 5; i++ {
		nodes[i] = domain.Node{
			LogicalID:   fmt.Sprintf("node-lease-%d", i),
			DisplayName: fmt.Sprintf("Node %d", i),
			Active:      true,
		}
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{nodes: nodes}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}

	var initialOwner string
	var initialOwnerMu sync.Mutex
	var leaseChecksFailed int32
	runner := &mockRunner{}
	runner.runFn = func(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error {
		t.Logf("RUN %s nodeIDs=%v", run.ID, nodeIDs)
		time.Sleep(10 * time.Millisecond)
		// Verify batch lease is still held by the winning instance in repo
		batches, _, _ := schedRepo.ListBatches(ctx, 1, 10)
		for _, b := range batches {
			initialOwnerMu.Lock()
			if initialOwner == "" {
				initialOwner = b.Owner
			}
			expectedOwner := initialOwner
			initialOwnerMu.Unlock()

			if b.Owner != expectedOwner || b.LeaseUntil == nil || !b.LeaseUntil.After(now) {
				t.Logf("CHECK FAILED: batch ID=%s, owner=%q (expected %q), state=%s, lease_until=%v, now=%v", b.ID, b.Owner, expectedOwner, b.State, b.LeaseUntil, now)
				atomic.AddInt32(&leaseChecksFailed, 1)
			}
		}
		return nil
	}

	coord1 := NewPeriodicCoordinator(
		schedRepo, nodeRepo, runRepo, runner,
		WithCoordinatorOwner("instance-1"),
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorMaxTasks(1),
		WithCoordinatorLeaseDuration(1*time.Second),
	)

	coord2 := NewPeriodicCoordinator(
		schedRepo, nodeRepo, runRepo, runner,
		WithCoordinatorOwner("instance-2"),
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorMaxTasks(1),
		WithCoordinatorLeaseDuration(1*time.Second),
	)

	coord1Done := make(chan error, 1)
	go func() {
		coord1Done <- coord1.TriggerWindow()
	}()

	// Concurrently, instance-2 attempts TriggerWindow repeatedly while coord1 executes multiple runs
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = coord2.TriggerWindow()
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}
	wg.Wait()

	if err := <-coord1Done; err != nil {
		t.Fatalf("coord1 TriggerWindow failed: %v", err)
	}

	if atomic.LoadInt32(&leaseChecksFailed) > 0 {
		t.Fatalf("detected lease corruption during batch execution: %d checks failed", leaseChecksFailed)
	}

	// Verify only 1 batch was created and executed to terminal state
	batches, _, err := schedRepo.ListBatches(ctx, 1, 10)
	if err != nil || len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d (err: %v)", len(batches), err)
	}
	if batches[0].State != domain.ProbeBatchStateSucceeded {
		t.Fatalf("expected batch state succeeded, got %s", batches[0].State)
	}
	if batches[0].Counts.CompletedRuns != 5 {
		t.Fatalf("expected 5 completed runs, got %d", batches[0].Counts.CompletedRuns)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.executedRuns) != 5 {
		t.Fatalf("expected exactly 5 runs executed (all by coord1), got %d", len(runner.executedRuns))
	}
}

func TestPeriodicCoordinator_LeaseLostAbortsExecution(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	nodes := []domain.Node{
		{LogicalID: "node-lost-1", DisplayName: "N1", Active: true},
		{LogicalID: "node-lost-2", DisplayName: "N2", Active: true},
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{nodes: nodes}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}

	// Runner steals lease on first run execution to simulate another instance taking over expired lease
	runner := &mockRunner{}
	runner.runFn = func(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error {
		// Steal lease by force-setting owner to instance-thief
		schedRepo.mu.Lock()
		for id, b := range schedRepo.batches {
			b.Owner = "instance-thief"
			until := now.Add(10 * time.Minute)
			b.LeaseUntil = &until
			schedRepo.batches[id] = b
		}
		schedRepo.mu.Unlock()
		// Sleep so the heartbeat ticker (5ms) fires, notices owner changed, and triggers leaseLost
		time.Sleep(30 * time.Millisecond)
		return nil
	}

	coord := NewPeriodicCoordinator(
		schedRepo, nodeRepo, runRepo, runner,
		WithCoordinatorOwner("instance-victim"),
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorMaxTasks(1),
		WithCoordinatorLeaseDuration(15*time.Millisecond), // Fast heartbeat: ticker every 5ms
	)

	err := coord.TriggerWindow()
	if err == nil {
		t.Fatalf("expected error due to lost lease, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Category != domain.CategoryConflict {
		t.Fatalf("expected conflict error (lease_lost), got %v", err)
	}

	// Verify that instance-victim did NOT overwrite owner back or set batch to terminal Failed/Cancelled
	schedRepo.mu.Lock()
	defer schedRepo.mu.Unlock()
	for _, b := range schedRepo.batches {
		if b.Owner != "instance-thief" {
			t.Fatalf("expected batch owner to remain instance-thief, got %s", b.Owner)
		}
	}
}

func TestObservationWrittenForActiveNode(t *testing.T) {
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
			{LogicalID: "node-v3", DisplayName: "Node V3", Active: true},
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
		IdempotencyKey: "test-obs",
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
	if recordedObs[0].NodeLogicalID != "node-v3" {
		t.Fatalf("expected NodeLogicalID=node-v3, got %s", recordedObs[0].NodeLogicalID)
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
			LogicalID:   fmt.Sprintf("node-%03d", i),
			DisplayName: fmt.Sprintf("Node %d", i),
			Active:      true,
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
		{LogicalID: "node-1", DisplayName: "Node 1", Active: true},
		{LogicalID: "node-2", DisplayName: "Node 2", Active: true},
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

func TestPeriodicCoordinator_DeduplicatesNodesAlreadyInPool(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	pastDue := now.Add(-time.Minute)

	schedRepo := newMemoryScheduleRepo(domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 300,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &pastDue,
		Generation:      1,
		UpdatedAt:       now,
	})
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-hk-01", DisplayName: "HK 01", Protocol: domain.ProtocolVMess, Active: true},
			{LogicalID: "node-sg-02", DisplayName: "SG 02", Protocol: domain.ProtocolVMess, Active: true},
		},
	}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := &testObsRepo{}

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10, QueueSize: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	// Pre-occupy node-hk-01 in the node pool (currently probing)
	hkStarted := make(chan struct{})
	releaseHK := make(chan struct{})
	if err := sched.Submit(queue.Task{
		RunID:     "manual-run-hk",
		LogicalID: "node-hk-01",
		Kind:      domain.ProbeKindBaseline,
		Mode:      queue.EnqueueManualPreemptFront,
		Execute: func(context.Context) error {
			close(hkStarted)
			<-releaseHK
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	<-hkStarted

	var probedNodesMu sync.Mutex
	probedByPeriodic := make([]string, 0)
	runner := NewDefaultRunner(
		nodeRepo,
		obsRepo,
		sched,
		runRepo,
		WithNodeDialer(func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
			probedNodesMu.Lock()
			probedByPeriodic = append(probedByPeriodic, node.LogicalID)
			probedNodesMu.Unlock()
			return nil, nil, nil
		}),
	)

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return now }),
	)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}

	close(releaseHK)
	sched.Wait()

	probedNodesMu.Lock()
	if len(probedByPeriodic) != 1 || probedByPeriodic[0] != "node-sg-02" {
		t.Fatalf("expected only node-sg-02 to be probed by periodic batch, got %v", probedByPeriodic)
	}
	probedNodesMu.Unlock()

	batches, total, err := schedRepo.ListBatches(ctx, 1, 10)
	if err != nil || total != 1 {
		t.Fatalf("expected 1 batch, got total=%d err=%v", total, err)
	}
	if batches[0].Counts.TotalNodes != 2 || batches[0].Counts.SkippedNodes != 1 {
		t.Fatalf("expected TotalNodes=2 and SkippedNodes=1, got %+v", batches[0].Counts)
	}
}

type memoryObsRepo struct {
	mu           sync.Mutex
	observations map[string]map[domain.ProbeKind]domain.ProbeObservation
}

func newMemoryObsRepo() *memoryObsRepo {
	return &memoryObsRepo{
		observations: make(map[string]map[domain.ProbeKind]domain.ProbeObservation),
	}
}

func (m *memoryObsRepo) SetObservation(nodeID string, kind domain.ProbeKind, obs domain.ProbeObservation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.observations[nodeID]; !ok {
		m.observations[nodeID] = make(map[domain.ProbeKind]domain.ProbeObservation)
	}
	m.observations[nodeID][kind] = obs
}

func (m *memoryObsRepo) GetByID(_ context.Context, _ string) (*domain.ProbeObservation, error) {
	return nil, nil
}
func (m *memoryObsRepo) ListByRun(_ context.Context, _ string) ([]domain.ProbeObservation, error) {
	return nil, nil
}
func (m *memoryObsRepo) ListByNode(_ context.Context, _ string, _ int) ([]domain.ProbeObservation, error) {
	return nil, nil
}
func (m *memoryObsRepo) ListLatestByNodes(_ context.Context, nodeLogicalIDs []string, kinds []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make(map[string]map[domain.ProbeKind]domain.ProbeObservation)
	for _, id := range nodeLogicalIDs {
		res[id] = make(map[domain.ProbeKind]domain.ProbeObservation)
		if nodeMap, ok := m.observations[id]; ok {
			if len(kinds) == 0 {
				for k, obs := range nodeMap {
					res[id][k] = obs
				}
			} else {
				for _, k := range kinds {
					if obs, exists := nodeMap[k]; exists {
						res[id][k] = obs
					}
				}
			}
		}
	}
	return res, nil
}
func (m *memoryObsRepo) Create(_ context.Context, obs *domain.ProbeObservation) error {
	m.SetObservation(obs.NodeLogicalID, obs.Kind, *obs)
	return nil
}

func TestPeriodicCoordinator_IncrementalSweep_EnableImmediateAndNextDueAt(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	initialSched := domain.DefaultProbeSchedule()
	initialSched.Enabled = false
	initialSched.IntervalSeconds = 7200 // 2 hours

	schedRepo := newMemoryScheduleRepo(initialSched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-1", DisplayName: "Node 1", Active: true},
		},
	}
	runner := &mockRunner{}
	obsRepo := newMemoryObsRepo()

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorSweepInterval(10*time.Minute),
	)

	service := NewService(
		runRepo,
		WithScheduleRepository(schedRepo),
		WithCoordinator(coord),
		WithClock(func() time.Time { return now }),
	)

	// 1. Enable schedule: NextDueAt should be set immediately to now, not now + 2h
	enabled := true
	updated, err := service.UpdateSchedule(ctx, domain.UpdateProbeScheduleRequest{
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("UpdateSchedule failed: %v", err)
	}
	if updated.NextDueAt == nil || !updated.NextDueAt.Equal(now) {
		t.Fatalf("expected NextDueAt to be immediately now (%v), got %v", now, updated.NextDueAt)
	}

	// 2. TriggerWindow executes: next_due_at advances by 10 minutes (sweepInterval), NOT 2 hours
	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}

	afterSched, err := schedRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get schedule: %v", err)
	}
	expectedNextDue := now.Add(10 * time.Minute)
	if afterSched.NextDueAt == nil || !afterSched.NextDueAt.Equal(expectedNextDue) {
		t.Fatalf("expected next_due_at to be %v (10min sweep interval), got %v", expectedNextDue, afterSched.NextDueAt)
	}
}

func TestPeriodicCoordinator_IncrementalSweep_ShortenedInterval(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	futureDue := now.Add(10 * time.Minute)
	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 7200, // 2 hours
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &futureDue,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	coord := NewPeriodicCoordinator(
		schedRepo,
		&memoryNodeRepo{},
		runRepo,
		&mockRunner{},
		WithCoordinatorClock(func() time.Time { return now }),
	)

	service := NewService(
		runRepo,
		WithScheduleRepository(schedRepo),
		WithCoordinator(coord),
		WithClock(func() time.Time { return now }),
	)

	// Shorten interval to 600s: NextDueAt should be reset to now for immediate re-evaluation
	newInterval := 600
	updated, err := service.UpdateSchedule(ctx, domain.UpdateProbeScheduleRequest{
		IntervalSeconds: &newInterval,
	})
	if err != nil {
		t.Fatalf("UpdateSchedule failed: %v", err)
	}
	if updated.NextDueAt == nil || !updated.NextDueAt.Equal(now) {
		t.Fatalf("expected NextDueAt to be reset to now (%v), got %v", now, updated.NextDueAt)
	}
}

func TestPeriodicCoordinator_IncrementalSweep_ValidExpiredAndUnobservedNodes(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 7200, // 2 hours
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindStreaming},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo()

	// 5 active nodes:
	// node-unobs: no observations -> expired!
	// node-fresh: observed 1 hour ago (valid, interval is 2h) -> skip!
	// node-stale: observed 3 hours ago -> expired!
	// node-cooldown: failed 2 minutes ago, failure cooldown 5m -> suppressed!
	// node-failed-expired: failed 10 minutes ago, interval 5m, but here interval is 2h -> wait, if interval is 2h and failed 10m ago, not expired
	// node-fail-due: observed 3 hours ago with error, cooldown 5m -> expired!
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-unobs", DisplayName: "Unobserved", Active: true},
			{LogicalID: "node-fresh", DisplayName: "Fresh", Active: true},
			{LogicalID: "node-stale", DisplayName: "Stale", Active: true},
			{LogicalID: "node-cooldown", DisplayName: "Cooldown", Active: true},
			{LogicalID: "node-fail-due", DisplayName: "Fail Due", Active: true},
		},
	}

	// Setup observations:
	// Fresh: both kinds observed 1h ago
	obsRepo.SetObservation("node-fresh", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID: "node-fresh",
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictAvailable,
		ObservedAt:    now.Add(-1 * time.Hour),
	})
	obsRepo.SetObservation("node-fresh", domain.ProbeKindStreaming, domain.ProbeObservation{
		NodeLogicalID: "node-fresh",
		Kind:          domain.ProbeKindStreaming,
		Verdict:       domain.VerdictAvailable,
		ObservedAt:    now.Add(-1 * time.Hour),
	})

	// Stale: baseline observed 3h ago, streaming unobserved -> both expired!
	obsRepo.SetObservation("node-stale", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID: "node-stale",
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictAvailable,
		ObservedAt:    now.Add(-3 * time.Hour),
	})

	// Cooldown: failed 2m ago -> suppressed by failure cooldown (5m)
	obsRepo.SetObservation("node-cooldown", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID: "node-cooldown",
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictError,
		ObservedAt:    now.Add(-2 * time.Minute),
	})
	obsRepo.SetObservation("node-cooldown", domain.ProbeKindStreaming, domain.ProbeObservation{
		NodeLogicalID: "node-cooldown",
		Kind:          domain.ProbeKindStreaming,
		Verdict:       domain.VerdictError,
		ObservedAt:    now.Add(-2 * time.Minute),
	})

	// Fail Due: failed 3h ago -> exceeds cooldown (5m) and exceeds interval (2h) -> expired!
	obsRepo.SetObservation("node-fail-due", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID: "node-fail-due",
		Kind:          domain.ProbeKindBaseline,
		Verdict:       domain.VerdictError,
		ObservedAt:    now.Add(-3 * time.Hour),
	})
	obsRepo.SetObservation("node-fail-due", domain.ProbeKindStreaming, domain.ProbeObservation{
		NodeLogicalID: "node-fail-due",
		Kind:          domain.ProbeKindStreaming,
		Verdict:       domain.VerdictError,
		ObservedAt:    now.Add(-3 * time.Hour),
	})

	runner := &mockRunner{}
	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorFailureCooldown(5*time.Minute),
		WithCoordinatorSweepInterval(10*time.Minute),
	)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}

	// Verify executed nodes
	executedNodeSet := make(map[string]bool)
	for _, nodeIDs := range runner.executedNodeIDs {
		for _, id := range nodeIDs {
			executedNodeSet[id] = true
		}
	}

	// Expected to be probed: node-unobs, node-stale, node-fail-due
	if !executedNodeSet["node-unobs"] {
		t.Errorf("expected node-unobs to be probed")
	}
	if !executedNodeSet["node-stale"] {
		t.Errorf("expected node-stale to be probed")
	}
	if !executedNodeSet["node-fail-due"] {
		t.Errorf("expected node-fail-due to be probed")
	}

	// Expected NOT to be probed: node-fresh (still valid), node-cooldown (in failure cooldown)
	if executedNodeSet["node-fresh"] {
		t.Errorf("expected node-fresh NOT to be probed (still valid)")
	}
	if executedNodeSet["node-cooldown"] {
		t.Errorf("expected node-cooldown NOT to be probed (within failure cooldown)")
	}
}

func TestPeriodicCoordinator_IncrementalSweep_QuotaAndStarvationPrevention(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 3600, // 1 hour
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo()

	// 20 nodes total:
	// node-00 .. node-09: unobserved (priority 1)
	// node-10 .. node-19: observed 3 hours ago (priority 2, oldest observed)
	nodes := make([]domain.Node, 20)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("node-%02d", i)
		nodes[i] = domain.Node{LogicalID: id, DisplayName: id, Active: true}
		if i >= 10 {
			// Observed in the past (node-10 oldest, node-19 newest of the old ones)
			obsRepo.SetObservation(id, domain.ProbeKindBaseline, domain.ProbeObservation{
				NodeLogicalID: id,
				Kind:          domain.ProbeKindBaseline,
				Verdict:       domain.VerdictAvailable,
				ObservedAt:    now.Add(time.Duration(-(180 - i)) * time.Minute),
			})
		}
	}
	nodeRepo := &memoryNodeRepo{nodes: nodes}

	// Sweep quota = 8 tasks per sweep
	runner := &mockRunner{}
	currentClock := now
	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return currentClock }),
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorSweepQuota(8),
		WithCoordinatorSweepInterval(10*time.Minute),
	)

	// Round 1: quota is 8 tasks -> should pick first 8 unobserved nodes (node-00 .. node-07)
	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("Round 1 TriggerWindow failed: %v", err)
	}

	var round1Nodes []string
	for _, nids := range runner.executedNodeIDs {
		round1Nodes = append(round1Nodes, nids...)
	}
	sort.Strings(round1Nodes)
	if len(round1Nodes) != 8 {
		t.Fatalf("expected 8 nodes in round 1, got %d: %v", len(round1Nodes), round1Nodes)
	}
	for i := 0; i < 8; i++ {
		expectedID := fmt.Sprintf("node-%02d", i)
		if round1Nodes[i] != expectedID {
			t.Fatalf("expected round 1 node %d to be %s, got %s", i, expectedID, round1Nodes[i])
		}
		// Record fresh observation for probed nodes
		obsRepo.SetObservation(expectedID, domain.ProbeKindBaseline, domain.ProbeObservation{
			NodeLogicalID: expectedID,
			Kind:          domain.ProbeKindBaseline,
			Verdict:       domain.VerdictAvailable,
			ObservedAt:    currentClock,
		})
	}

	// Advance clock by 10 minutes for Round 2
	currentClock = currentClock.Add(10 * time.Minute)
	runner.executedNodeIDs = make(map[string][]string)

	// Round 2: should pick remaining 2 unobserved nodes (node-08, node-09) + 6 oldest observed nodes (node-10 .. node-15)
	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("Round 2 TriggerWindow failed: %v", err)
	}

	var round2Nodes []string
	for _, nids := range runner.executedNodeIDs {
		round2Nodes = append(round2Nodes, nids...)
	}
	sort.Strings(round2Nodes)
	if len(round2Nodes) != 8 {
		t.Fatalf("expected 8 nodes in round 2, got %d: %v", len(round2Nodes), round2Nodes)
	}

	expectedRound2 := []string{"node-08", "node-09", "node-10", "node-11", "node-12", "node-13", "node-14", "node-15"}
	for i, exp := range expectedRound2 {
		if round2Nodes[i] != exp {
			t.Fatalf("expected round 2 node %d to be %s, got %s", i, exp, round2Nodes[i])
		}
	}
}

func TestPeriodicCoordinator_IncrementalSweep_RecoveryLegacyFarDueCorrection(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Legacy schedule in DB had NextDueAt set to 2 hours in the future
	legacyDue := now.Add(2 * time.Hour)
	legacySched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 7200,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &legacyDue,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(legacySched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	coord := NewPeriodicCoordinator(
		schedRepo,
		&memoryNodeRepo{},
		runRepo,
		&mockRunner{},
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorSweepInterval(10*time.Minute),
	)

	// Run recovery on startup
	if err := coord.Recover(ctx); err != nil {
		t.Fatalf("Recover failed: %v", err)
	}

	// Verify NextDueAt was corrected to now (or <= now) for immediate sweep, eliminating the 2-hour delay
	recoveredSched, err := schedRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get recovered schedule: %v", err)
	}
	if recoveredSched.NextDueAt == nil || recoveredSched.NextDueAt.After(now) {
		t.Fatalf("expected NextDueAt to be corrected to <= now (%v), got %v", now, recoveredSched.NextDueAt)
	}
}

func TestPeriodicCoordinator_MinuteSweep_NoSpinAndConnectionRevisionChange(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	rev1 := int64(1)
	rev2 := int64(2)

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 3600, // 1 hour validity
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo()

	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-1", DisplayName: "Node 1", Active: true, ConnectionRevision: 1},
			{LogicalID: "node-2", DisplayName: "Node 2", Active: true, ConnectionRevision: 1},
			{LogicalID: "node-cooldown", DisplayName: "Node Cooldown", Active: true, ConnectionRevision: 1},
		},
	}

	// Both node-1 and node-2 have fresh observations matching ConnectionRevision=1
	obsRepo.SetObservation("node-1", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "node-1",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictAvailable,
		ObservedAt:         now.Add(-30 * time.Second),
		ConnectionRevision: &rev1,
	})
	obsRepo.SetObservation("node-2", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "node-2",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictAvailable,
		ObservedAt:         now.Add(-30 * time.Second),
		ConnectionRevision: &rev1,
	})
	// node-cooldown failed 2 minutes ago (< 5m default failure cooldown)
	obsRepo.SetObservation("node-cooldown", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "node-cooldown",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictError,
		ObservedAt:         now.Add(-2 * time.Minute),
		ConnectionRevision: &rev1,
	})

	runner := &mockRunner{}
	currentClock := now
	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return currentClock }),
		WithCoordinatorObservations(obsRepo),
	)

	// 1. Minute 0: No changes, all valid or in cooldown -> must NOT spin / dispatch any runs
	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("Minute 0 TriggerWindow failed: %v", err)
	}
	if len(runner.executedRuns) != 0 {
		t.Fatalf("expected 0 dispatched runs when nothing changed, got %d", len(runner.executedRuns))
	}

	// Verify default sweep interval is 1 minute
	afterM0, err := schedRepo.Get(ctx)
	if err != nil {
		t.Fatalf("Get schedule failed: %v", err)
	}
	expectedM1 := now.Add(1 * time.Minute)
	if afterM0.NextDueAt == nil || !afterM0.NextDueAt.Equal(expectedM1) {
		t.Fatalf("expected NextDueAt=%v (1-minute default sweep), got %v", expectedM1, afterM0.NextDueAt)
	}

	// 2. Minute 1: node-2 connection configuration changes (ConnectionRevision increments 1 -> 2)
	currentClock = expectedM1
	nodeRepo.nodes[1].ConnectionRevision = rev2

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("Minute 1 TriggerWindow failed: %v", err)
	}
	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected 1 dispatched run after ConnectionRevision increment, got %d", len(runner.executedRuns))
	}
	var probedM1 []string
	for _, ids := range runner.executedNodeIDs {
		probedM1 = append(probedM1, ids...)
	}
	if len(probedM1) != 1 || probedM1[0] != "node-2" {
		t.Fatalf("expected only node-2 (revision changed) to be probed at Minute 1, got %v", probedM1)
	}

	// Record fresh observation for node-2 with rev2
	obsRepo.SetObservation("node-2", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "node-2",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictAvailable,
		ObservedAt:         currentClock,
		ConnectionRevision: &rev2,
	})

	// 3. Manual TriggerImmediate must NOT postpone NextDueAt
	schedBeforeManual, _ := schedRepo.Get(ctx)
	if schedBeforeManual.NextDueAt == nil {
		t.Fatalf("expected NextDueAt to be set")
	}
	expectedAutoDue := *schedBeforeManual.NextDueAt

	runner.executedRuns = nil
	runner.executedNodeIDs = make(map[string][]string)
	if err := coord.TriggerImmediate(ctx); err != nil {
		t.Fatalf("TriggerImmediate failed: %v", err)
	}
	if len(runner.executedRuns) != 0 {
		t.Fatalf("expected 0 runs on manual trigger when all nodes fresh/cooldown, got %d", len(runner.executedRuns))
	}
	schedAfterManual, _ := schedRepo.Get(ctx)
	if schedAfterManual.NextDueAt == nil || !schedAfterManual.NextDueAt.Equal(expectedAutoDue) {
		t.Fatalf("manual TriggerImmediate must not alter NextDueAt: want %v, got %v", expectedAutoDue, schedAfterManual.NextDueAt)
	}
}

func TestPeriodicCoordinator_Default512TaskBudget(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 3600,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &now,
		Generation:      1,
		UpdatedAt:       now,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo()

	// Create 600 unobserved active nodes
	nodes := make([]domain.Node, 600)
	for i := 0; i < 600; i++ {
		id := fmt.Sprintf("node-%03d", i)
		nodes[i] = domain.Node{LogicalID: id, DisplayName: id, Active: true, ConnectionRevision: 1}
	}
	nodeRepo := &memoryNodeRepo{nodes: nodes}

	runner := &mockRunner{}
	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorObservations(obsRepo),
	)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}

	var totalProbed int
	for _, nids := range runner.executedNodeIDs {
		totalProbed += len(nids)
	}
	if totalProbed != 512 {
		t.Fatalf("expected default 512 task budget to cap scheduled nodes at 512, got %d", totalProbed)
	}
}

func TestPoolStatus_BaselineIsolationAndSecurityRejectionsUnknown(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	rev1 := int64(1)

	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "n-healthy", Active: true, ConnectionRevision: 1},
			{LogicalID: "n-unhealthy", Active: true, ConnectionRevision: 1},
			{LogicalID: "n-nonbase-err", Active: true, ConnectionRevision: 1},
			{LogicalID: "n-unsafe-tls", Active: true, ConnectionRevision: 1},
			{LogicalID: "n-private-ip", Active: true, ConnectionRevision: 1},
			{LogicalID: "n-creds-missing", Active: true, ConnectionRevision: 1},
			{LogicalID: "n-build-failed", Active: true, ConnectionRevision: 1},
		},
	}

	obsRepo := newMemoryObsRepo()
	// 1. Healthy baseline
	obsRepo.SetObservation("n-healthy", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "n-healthy",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictAvailable,
		LatencyMS:          35,
		ObservedAt:         now,
		ConnectionRevision: &rev1,
	})
	// 2. Unhealthy baseline (confirmed node_connect_failed)
	obsRepo.SetObservation("n-unhealthy", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "n-unhealthy",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictError,
		RedactedSummary:    "reason=node_connect_failed",
		ObservedAt:         now,
		ConnectionRevision: &rev1,
	})
	// 3. Non-baseline error without baseline -> must be counted as Untested/Unknown, NOT Unavailable
	obsRepo.SetObservation("n-nonbase-err", domain.ProbeKindStreaming, domain.ProbeObservation{
		NodeLogicalID:      "n-nonbase-err",
		Kind:               domain.ProbeKindStreaming,
		Verdict:            domain.VerdictError,
		RedactedSummary:    "reason=node_connect_failed",
		ObservedAt:         now,
		ConnectionRevision: &rev1,
	})
	// 4. Security / build rejections -> must be counted as Untested/Unknown, NOT Unavailable
	for _, tc := range []struct {
		id     string
		reason string
	}{
		{"n-unsafe-tls", "unsafe_tls_rejected"},
		{"n-private-ip", "private_target_rejected"},
		{"n-creds-missing", "credentials_unavailable"},
		{"n-build-failed", "client_build_failed"},
	} {
		obsRepo.SetObservation(tc.id, domain.ProbeKindBaseline, domain.ProbeObservation{
			NodeLogicalID:      tc.id,
			Kind:               domain.ProbeKindBaseline,
			Verdict:            domain.VerdictError,
			RedactedSummary:    "reason=" + tc.reason,
			ObservedAt:         now,
			ConnectionRevision: &rev1,
		})
	}

	svc := NewService(
		&memoryRuns{items: make(map[string]domain.ProbeRun)},
		WithNodeRepository(nodeRepo),
		WithObservationRepository(obsRepo),
		WithClock(func() time.Time { return now }),
	)

	pool, err := svc.GetPoolStatus(ctx)
	if err != nil {
		t.Fatalf("GetPoolStatus failed: %v", err)
	}
	if pool.TotalCount != 7 {
		t.Fatalf("expected total_count=7, got %d", pool.TotalCount)
	}
	if pool.HealthyCount != 1 || pool.AvailableCount != 1 {
		t.Fatalf("expected healthy=1 available=1, got healthy=%d available=%d", pool.HealthyCount, pool.AvailableCount)
	}
	if pool.UnavailableCount != 1 {
		t.Fatalf("expected unavailable_count=1 (only n-unhealthy), got %d", pool.UnavailableCount)
	}
	if pool.UndeterminedCount != 5 {
		t.Fatalf("expected undetermined_count=5 (non-baseline + 4 security/build rejections), got %d", pool.UndeterminedCount)
	}
	if pool.UntestedCount != 0 {
		t.Fatalf("expected untested_count=0, got %d", pool.UntestedCount)
	}
	if !pool.ValidateConservation() {
		t.Fatalf("pool conservation violated: %+v", pool)
	}
}
