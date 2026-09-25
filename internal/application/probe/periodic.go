package probe

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
)

const (
	defaultLeaseDuration = 60 * time.Second
	defaultRunTimeout    = 10 * time.Minute
	defaultMaxTasks      = 512
)

// PeriodicCoordinatorOption configures PeriodicCoordinator options.
type PeriodicCoordinatorOption func(*PeriodicCoordinator)

// WithCoordinatorClock sets a deterministic clock function for testing.
func WithCoordinatorClock(clock func() time.Time) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		c.clock = clock
	}
}

// WithCoordinatorSleep overrides the sleep channel generator for deterministic timer testing.
func WithCoordinatorSleep(sleep func(d time.Duration) <-chan time.Time) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		c.sleep = sleep
	}
}

// WithCoordinatorOwner sets the unique instance owner identifier.
func WithCoordinatorOwner(owner string) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		c.ownerID = owner
	}
}

// WithCoordinatorLeaseDuration configures lease duration for coordination.
func WithCoordinatorLeaseDuration(d time.Duration) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		c.leaseDuration = d
	}
}

// WithCoordinatorRunTimeout sets the timeout applied to periodic probe runs.
func WithCoordinatorRunTimeout(d time.Duration) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		c.runTimeout = d
	}
}

// WithCoordinatorMaxTasks sets the maximum number of probe tasks per single run chunk.
func WithCoordinatorMaxTasks(maxTasks int) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		c.maxTasksPerRun = maxTasks
	}
}

// PeriodicCoordinator supervises periodic probe scheduling, lease coordination,
// active node sharding, and graceful drain.
type PeriodicCoordinator struct {
	schedules      domain.ProbeScheduleRepository
	nodes          domain.NodeRepository
	runs           domain.ProbeRunRepository
	runner         Runner
	ownerID        string
	leaseDuration  time.Duration
	runTimeout     time.Duration
	maxTasksPerRun int
	clock          func() time.Time
	sleep          func(d time.Duration) <-chan time.Time

	mu            sync.Mutex
	activeBatches map[string]context.CancelFunc
	wakeCh        chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	started       bool
}

// NewPeriodicCoordinator creates a new PeriodicCoordinator.
func NewPeriodicCoordinator(
	schedules domain.ProbeScheduleRepository,
	nodes domain.NodeRepository,
	runs domain.ProbeRunRepository,
	runner Runner,
	opts ...PeriodicCoordinatorOption,
) *PeriodicCoordinator {
	c := &PeriodicCoordinator{
		schedules:      schedules,
		nodes:          nodes,
		runs:           runs,
		runner:         runner,
		ownerID:        "csp-instance-" + domain.MustNewUUIDv7(),
		leaseDuration:  defaultLeaseDuration,
		runTimeout:     defaultRunTimeout,
		maxTasksPerRun: defaultMaxTasks,
		clock:          time.Now,
		sleep:          time.After,
		activeBatches:  make(map[string]context.CancelFunc),
		wakeCh:         make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Start launches the periodic scheduling loop in a background goroutine.
func (c *PeriodicCoordinator) Start(parentCtx context.Context) {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	c.ctx, c.cancel = context.WithCancel(parentCtx)
	c.started = true
	c.wg.Add(1)
	c.mu.Unlock()

	go c.runLoop()
}

// Stop gracefully signals the scheduling loop to terminate and cancels active periodic tasks.
func (c *PeriodicCoordinator) Stop() {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return
	}
	c.cancel()
	// Cancel all active in-flight batch contexts
	for _, cancel := range c.activeBatches {
		cancel()
	}
	c.mu.Unlock()

	c.wg.Wait()
}

// Wake signals the loop to re-evaluate the schedule immediately.
func (c *PeriodicCoordinator) Wake() {
	select {
	case c.wakeCh <- struct{}{}:
	default:
	}
}

// CancelBatch cancels an in-flight batch execution context if running on this instance.
func (c *PeriodicCoordinator) CancelBatch(batchID string) {
	c.mu.Lock()
	if cancel, ok := c.activeBatches[batchID]; ok {
		cancel()
	}
	c.mu.Unlock()
}

// Recover handles startup inspection of legacy/abandoned batches and advances overdue schedules.
func (c *PeriodicCoordinator) Recover(ctx context.Context) error {
	if c.schedules == nil {
		return nil
	}
	now := c.clock().UTC()

	// 1. Recover unfinalized batches whose lease has expired
	page := 1
	for {
		batches, total, err := c.schedules.ListBatches(ctx, page, 50)
		if err != nil || len(batches) == 0 {
			break
		}
		for _, b := range batches {
			if !b.State.IsTerminal() {
				isExpired := b.LeaseUntil == nil || b.LeaseUntil.Before(now)
				if isExpired {
					b.State = domain.ProbeBatchStateExpired
					b.RedactedError = "batch lease expired during startup recovery"
					_ = c.schedules.UpdateBatch(ctx, &b)

					for _, runID := range b.RunIDs {
						run, err := c.runs.GetByID(ctx, runID)
						if err == nil && !run.IsTerminal() {
							_ = run.TransitionTo(domain.ProbeRunStateExpired)
							_ = c.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateExpired)
						}
					}
				}
			}
		}
		if page*50 >= total {
			break
		}
		page++
	}

	// 2. Advance schedule next_due_at if overdue to avoid historical replay storm
	sched, err := c.schedules.Get(ctx)
	if err == nil && sched != nil && sched.Enabled {
		if sched.NextDueAt == nil || sched.NextDueAt.Before(now) {
			futureDue := now.Add(time.Duration(sched.IntervalSeconds) * time.Second)
			sched.NextDueAt = &futureDue
			_ = c.schedules.Update(ctx, sched)
		}
	}

	return nil
}

func (c *PeriodicCoordinator) runLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		sched, err := c.schedules.Get(c.ctx)
		if err != nil || !sched.Enabled {
			// Schedule disabled or unconfigured; wait for wake signal or periodic check
			select {
			case <-c.ctx.Done():
				return
			case <-c.wakeCh:
				continue
			case <-c.sleep(30 * time.Second):
				continue
			}
		}

		now := c.clock().UTC()
		var delay time.Duration
		if sched.NextDueAt == nil || sched.NextDueAt.IsZero() {
			delay = 0
		} else if sched.NextDueAt.Before(now) {
			delay = 0
		} else {
			delay = sched.NextDueAt.Sub(now)
		}

		if delay > 0 {
			select {
			case <-c.ctx.Done():
				return
			case <-c.wakeCh:
				continue
			case <-c.sleep(delay):
			}
		}

		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.triggerWindow()
	}
}

// TriggerWindow executes a single due window if schedule is enabled.
func (c *PeriodicCoordinator) TriggerWindow() error {
	return c.triggerWindow()
}

func (c *PeriodicCoordinator) triggerWindow() error {
	if c.ctx == nil {
		c.ctx = context.Background()
	}

	sched, err := c.schedules.Get(c.ctx)
	if err != nil {
		return err
	}
	if !sched.Enabled {
		return nil
	}

	now := c.clock().UTC()
	if sched.NextDueAt != nil && sched.NextDueAt.After(now) {
		// Not due yet; another instance may have already claimed/advanced this window
		return nil
	}

	var windowAt time.Time
	if sched.NextDueAt != nil && !sched.NextDueAt.IsZero() {
		windowAt = sched.NextDueAt.UTC().Truncate(time.Second)
	} else {
		windowAt = now.Truncate(time.Second)
	}

	// Advance next_due_at before execution to prevent catch-up storms
	nextDue := now.Add(time.Duration(sched.IntervalSeconds) * time.Second)
	sched.NextDueAt = &nextDue
	if err := c.schedules.Update(c.ctx, sched); err != nil {
		return fmt.Errorf("failed to advance probe schedule next_due_at: %w", err)
	}

	// Find or create batch for (generation, windowAt)
	batch, err := c.schedules.GetBatchByWindow(c.ctx, sched.Generation, windowAt)
	if err != nil {
		if de, ok := domain.AsDomainError(err); ok && de.Category == domain.CategoryNotFound {
			newBatch := &domain.ProbeBatch{
				ID:         domain.MustNewUUIDv7(),
				WindowAt:   windowAt,
				Generation: sched.Generation,
				Owner:      c.ownerID,
				State:      domain.ProbeBatchStatePending,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if createErr := c.schedules.CreateBatch(c.ctx, newBatch); createErr != nil {
				// Duplicate / race from another instance
				existing, getErr := c.schedules.GetBatchByWindow(c.ctx, sched.Generation, windowAt)
				if getErr != nil {
					return fmt.Errorf("failed to create or get batch for window %s: %w", windowAt, createErr)
				}
				batch = existing
			} else {
				batch = newBatch
			}
		} else {
			return err
		}
	}

	if batch.State.IsTerminal() {
		return nil
	}

	return c.executeBatch(c.ctx, batch, sched)
}

func (c *PeriodicCoordinator) executeBatch(parentCtx context.Context, batch *domain.ProbeBatch, sched *domain.ProbeSchedule) error {
	// 1. CAS Lease Acquisition
	acquired, err := c.schedules.AcquireLease(parentCtx, batch.ID, c.ownerID, c.leaseDuration)
	if err != nil {
		return fmt.Errorf("failed to acquire lease for batch %s: %w", batch.ID, err)
	}
	if !acquired {
		// Another instance holds active lease; yield execution
		return nil
	}

	batchCtx, batchCancel := context.WithCancel(parentCtx)
	defer batchCancel()

	c.mu.Lock()
	c.activeBatches[batch.ID] = batchCancel
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.activeBatches, batch.ID)
		c.mu.Unlock()
	}()

	// 2. Heartbeat supervision goroutine
	leaseLost := make(chan struct{})
	heartbeatDone := make(chan struct{})
	defer func() {
		close(heartbeatDone)
	}()

	go func() {
		ticker := time.NewTicker(c.leaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-batchCtx.Done():
				return
			case <-ticker.C:
				hbErr := c.schedules.HeartbeatLease(batchCtx, batch.ID, c.ownerID, c.leaseDuration)
				if hbErr != nil {
					select {
					case <-leaseLost:
					default:
						close(leaseLost)
					}
					batchCancel()
					return
				}
			}
		}
	}()

	// 3. Mark batch running
	if batch.CanTransitionTo(domain.ProbeBatchStateRunning) {
		_ = batch.TransitionTo(domain.ProbeBatchStateRunning)
		if err := c.schedules.UpdateBatch(batchCtx, batch); err != nil {
			_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
			return err
		}
	}

	// 4. Query active inventory with pagination to retrieve full inventory
	var nodes []domain.Node
	fetchPage := 1
	const fetchPageSize = 100
	for {
		chunk, total, err := c.nodes.List(batchCtx, domain.NodeFilter{
			ActiveOnly: true,
			Pagination: domain.Pagination{Page: fetchPage, PageSize: fetchPageSize},
		})
		if err != nil {
			batch.RedactedError = "failed to list active nodes"
			_ = batch.TransitionTo(domain.ProbeBatchStateFailed)
			_ = c.schedules.UpdateBatch(context.Background(), batch)
			_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
			return err
		}
		nodes = append(nodes, chunk...)
		if len(nodes) >= total || len(chunk) == 0 {
			break
		}
		fetchPage++
	}

	// Deterministic ordering by LogicalID
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].LogicalID < nodes[j].LogicalID
	})

	batch.Counts.TotalNodes = len(nodes)

	// Fail-closed for nodes with unconfigured/invalid credential versions
	var eligibleNodes []domain.Node
	for _, n := range nodes {
		if n.CredentialVersion <= 0 {
			batch.Counts.SkippedNodes++
			continue
		}
		eligibleNodes = append(eligibleNodes, n)
	}

	if len(eligibleNodes) == 0 {
		if batch.Counts.SkippedNodes > 0 {
			batch.RedactedError = fmt.Sprintf("%d nodes skipped: missing or invalid credentials (fail-closed)", batch.Counts.SkippedNodes)
		}
		_ = batch.TransitionTo(domain.ProbeBatchStateSucceeded)
		_ = c.schedules.UpdateBatch(context.Background(), batch)
		_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
		return nil
	}
	if batch.Counts.SkippedNodes > 0 && batch.RedactedError == "" {
		batch.RedactedError = fmt.Sprintf("%d nodes skipped: missing or invalid credentials (fail-closed)", batch.Counts.SkippedNodes)
	}

	// 5. Partition active nodes into run chunks within task budget
	maxTasks := c.maxTasksPerRun
	if maxTasks <= 0 {
		maxTasks = defaultMaxTasks
	}
	kindsCount := len(sched.Kinds)
	if kindsCount == 0 {
		kindsCount = 1
	}
	nodesPerChunk := maxTasks / kindsCount
	if nodesPerChunk <= 0 {
		nodesPerChunk = 1
	}

	var chunks [][]domain.Node
	for i := 0; i < len(eligibleNodes); i += nodesPerChunk {
		end := i + nodesPerChunk
		if end > len(eligibleNodes) {
			end = len(eligibleNodes)
		}
		chunks = append(chunks, eligibleNodes[i:end])
	}

	batch.Counts.DispatchedRuns = len(chunks)

	// Create and persist runs for each chunk
	runs := make([]*domain.ProbeRun, 0, len(chunks))
	runIDs := make([]string, 0, len(chunks))
	now := c.clock().UTC()

	for chunkIdx, chunk := range chunks {
		nodeIDs := make([]string, len(chunk))
		for idx, n := range chunk {
			nodeIDs[idx] = n.LogicalID
		}

		idempotencyKey := fmt.Sprintf("periodic:%d:%s:%d", sched.Generation, batch.WindowAt.Format(time.RFC3339), chunkIdx)
		var run *domain.ProbeRun
		existing, getErr := c.runs.GetByIdempotencyKey(batchCtx, "system:periodic-probe", idempotencyKey)
		if getErr == nil && existing != nil {
			run = existing
		} else {
			runID := domain.MustNewUUIDv7()
			run = &domain.ProbeRun{
				ID:             runID,
				IdempotencyKey: idempotencyKey,
				ActorScope:     "system:periodic-probe",
				State:          domain.ProbeRunStateQueued,
				DeadlineAt:     now.Add(c.runTimeout),
				CreatedAt:      now,
				UpdatedAt:      now,
			}

			if err := c.runs.Create(batchCtx, run); err != nil {
				// Idempotency fallback
				fallback, fErr := c.runs.GetByIdempotencyKey(batchCtx, "system:periodic-probe", idempotencyKey)
				if fErr == nil && fallback != nil {
					run = fallback
				} else {
					batch.RedactedError = "failed to create probe run chunk"
					_ = batch.TransitionTo(domain.ProbeBatchStateFailed)
					_ = c.schedules.UpdateBatch(context.Background(), batch)
					_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
					return fmt.Errorf("failed to create run for chunk %d: %w", chunkIdx, err)
				}
			}
		}
		runs = append(runs, run)
		runIDs = append(runIDs, run.ID)
	}

	batch.RunIDs = runIDs
	if err := c.schedules.UpdateBatch(batchCtx, batch); err != nil {
		_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
		return err
	}

	// 6. Execute each run chunk
	var hasRunError bool
	completedRuns := 0
	for _, r := range runs {
		if r.IsTerminal() {
			completedRuns++
		}
	}
	batch.Counts.CompletedRuns = completedRuns

	for chunkIdx, run := range runs {
		select {
		case <-leaseLost:
			// Lost lease; cease execution and do NOT overwrite state
			return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
		case <-batchCtx.Done():
			if batch.CanTransitionTo(domain.ProbeBatchStateCancelled) {
				_ = batch.TransitionTo(domain.ProbeBatchStateCancelled)
			}
			_ = c.schedules.UpdateBatch(context.Background(), batch)
			_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
			return batchCtx.Err()
		default:
		}

		if run.IsTerminal() {
			continue
		}

		chunk := chunks[chunkIdx]
		nodeIDs := make([]string, len(chunk))
		for idx, n := range chunk {
			nodeIDs[idx] = n.LogicalID
		}

		runErr := c.runner.Run(batchCtx, run, nodeIDs, sched.Kinds)
		if runErr != nil {
			hasRunError = true
			batch.RedactedError = domain.RedactSensitiveInfo(runErr.Error())
		}
		completedRuns++
		batch.Counts.CompletedRuns = completedRuns
		_ = c.schedules.UpdateBatch(batchCtx, batch)
	}

	select {
	case <-leaseLost:
		return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
	default:
	}

	targetState := domain.ProbeBatchStateSucceeded
	if hasRunError && batch.Counts.CompletedRuns == 0 {
		targetState = domain.ProbeBatchStateFailed
	}

	if batch.CanTransitionTo(targetState) {
		_ = batch.TransitionTo(targetState)
	}
	_ = c.schedules.UpdateBatch(context.Background(), batch)
	_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)

	return nil
}
