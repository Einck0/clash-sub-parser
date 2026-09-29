package probe

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
)

const (
	defaultLeaseDuration   = 60 * time.Second
	defaultRunTimeout      = 10 * time.Minute
	defaultMaxTasks        = 512
	defaultSweepInterval   = 10 * time.Minute
	defaultFailureCooldown = 5 * time.Minute
	defaultSweepQuota      = 512
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

// WithCoordinatorSweepInterval sets the lightweight sweep interval (default: 10 minutes).
func WithCoordinatorSweepInterval(d time.Duration) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		if d > 0 {
			c.sweepInterval = d
		}
	}
}

// WithCoordinatorFailureCooldown sets the cooldown period after a failed probe before retrying.
func WithCoordinatorFailureCooldown(d time.Duration) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		if d > 0 {
			c.failureCooldown = d
		}
	}
}

// WithCoordinatorSweepQuota sets the maximum number of probe tasks scheduled in a single periodic sweep.
func WithCoordinatorSweepQuota(quota int) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		if quota > 0 {
			c.sweepQuota = quota
		}
	}
}

// WithCoordinatorObservations sets the observation repository for incremental expiry checks.
func WithCoordinatorObservations(observations domain.ProbeObservationRepository) PeriodicCoordinatorOption {
	return func(c *PeriodicCoordinator) {
		c.observations = observations
	}
}

// PeriodicCoordinator supervises periodic probe scheduling, lease coordination,
// active node sharding, and graceful drain.
type PeriodicCoordinator struct {
	schedules       domain.ProbeScheduleRepository
	nodes           domain.NodeRepository
	runs            domain.ProbeRunRepository
	observations    domain.ProbeObservationRepository
	runner          Runner
	ownerID         string
	leaseDuration   time.Duration
	runTimeout      time.Duration
	maxTasksPerRun  int
	sweepInterval   time.Duration
	failureCooldown time.Duration
	sweepQuota      int
	clock           func() time.Time
	sleep           func(d time.Duration) <-chan time.Time

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
		schedules:       schedules,
		nodes:           nodes,
		runs:            runs,
		runner:          runner,
		ownerID:         "csp-instance-" + domain.MustNewUUIDv7(),
		leaseDuration:   defaultLeaseDuration,
		runTimeout:      defaultRunTimeout,
		maxTasksPerRun:  defaultMaxTasks,
		sweepInterval:   defaultSweepInterval,
		failureCooldown: defaultFailureCooldown,
		sweepQuota:      defaultSweepQuota,
		clock:           time.Now,
		sleep:           time.After,
		activeBatches:   make(map[string]context.CancelFunc),
		wakeCh:          make(chan struct{}, 1),
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

	// 2. Advance schedule next_due_at if overdue or legacy far future to avoid delay
	sched, err := c.schedules.Get(ctx)
	if err == nil && sched != nil && sched.Enabled {
		sweepInterval := c.sweepInterval
		if sweepInterval <= 0 {
			sweepInterval = defaultSweepInterval
		}
		// If schedule next_due_at is nil, in the past (overdue), or set far into the future (legacy whole-interval schedule, e.g. 2 hours),
		// correct it to now so that incremental sweep takes effect immediately without waiting.
		if sched.NextDueAt == nil || sched.NextDueAt.Before(now) || sched.NextDueAt.After(now.Add(sweepInterval)) {
			sched.NextDueAt = &now
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
		} else if sched.NextDueAt.Before(now) || sched.NextDueAt.Equal(now) {
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

// TriggerImmediate forces an immediate periodic probe batch execution, deduplicating
// nodes that are already in the probe pool.
func (c *PeriodicCoordinator) TriggerImmediate(ctx context.Context) error {
	if ctx == nil {
		if c.ctx != nil {
			ctx = c.ctx
		} else {
			ctx = context.Background()
		}
	}

	var sched *domain.ProbeSchedule
	if c.schedules != nil {
		if s, err := c.schedules.Get(ctx); err == nil && s != nil {
			copySched := *s
			sched = &copySched
		}
	}
	if sched == nil {
		def := domain.DefaultProbeSchedule()
		sched = &def
	}
	if len(sched.Kinds) == 0 {
		sched.Kinds = []domain.ProbeKind{domain.ProbeKindBaseline}
	}

	now := c.clock().UTC()
	windowAt := now.Truncate(time.Millisecond)
	newBatch := &domain.ProbeBatch{
		ID:         domain.MustNewUUIDv7(),
		WindowAt:   windowAt,
		Generation: sched.Generation,
		Owner:      c.ownerID,
		State:      domain.ProbeBatchStatePending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if c.schedules != nil {
		if err := c.schedules.CreateBatch(ctx, newBatch); err != nil {
			if existing, getErr := c.schedules.GetBatchByWindow(ctx, sched.Generation, windowAt); getErr == nil && existing != nil {
				newBatch = existing
			} else {
				return err
			}
		}
		if sched.Enabled {
			sweepInterval := c.sweepInterval
			if sweepInterval <= 0 {
				sweepInterval = defaultSweepInterval
			}
			nextDue := now.Add(sweepInterval)
			sched.NextDueAt = &nextDue
			_ = c.schedules.Update(ctx, sched)
		}
	}

	if newBatch.State.IsTerminal() {
		return nil
	}

	return c.executeBatch(ctx, newBatch, sched)
}

func (c *PeriodicCoordinator) triggerWindow() error {
	c.mu.Lock()
	ctx := c.ctx
	c.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	sched, err := c.schedules.Get(ctx)
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

	// Advance next_due_at to the next lightweight sweep clock tick before execution
	// to prevent catch-up storms. NextDueAt reflects the next sweep, not the full interval.
	sweepInterval := c.sweepInterval
	if sweepInterval <= 0 {
		sweepInterval = defaultSweepInterval
	}
	nextDue := now.Add(sweepInterval)
	sched.NextDueAt = &nextDue
	if err := c.schedules.Update(ctx, sched); err != nil {
		return fmt.Errorf("failed to advance probe schedule next_due_at: %w", err)
	}

	// Find or create batch for (generation, windowAt)
	batch, err := c.schedules.GetBatchByWindow(ctx, sched.Generation, windowAt)
	if err != nil {
		if de, ok := domain.AsDomainError(err); ok && de.Category == domain.CategoryNotFound {
			newBatch := &domain.ProbeBatch{
				ID:         domain.MustNewUUIDv7(),
				WindowAt:   windowAt,
				Generation: sched.Generation,
				Owner:      "",
				State:      domain.ProbeBatchStatePending,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if createErr := c.schedules.CreateBatch(ctx, newBatch); createErr != nil {
				// Duplicate / race from another instance
				existing, getErr := c.schedules.GetBatchByWindow(ctx, sched.Generation, windowAt)
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

	c.mu.Lock()
	if _, active := c.activeBatches[batch.ID]; active {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	return c.executeBatch(ctx, batch, sched)
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
	batch.Owner = c.ownerID

	batchCtx, batchCancel := context.WithCancel(parentCtx)
	defer batchCancel()

	c.mu.Lock()
	if _, active := c.activeBatches[batch.ID]; active {
		c.mu.Unlock()
		return nil
	}
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
	var allActiveNodes []domain.Node
	fetchPage := 1
	const fetchPageSize = 100
	for {
		chunk, total, err := c.nodes.List(batchCtx, domain.NodeFilter{
			ActiveOnly: true,
			Pagination: domain.Pagination{Page: fetchPage, PageSize: fetchPageSize},
		})
		if err != nil {
			select {
			case <-leaseLost:
				return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
			default:
			}
			batch.RedactedError = "failed to list active nodes"
			_ = batch.TransitionTo(domain.ProbeBatchStateFailed)
			_ = c.schedules.UpdateBatch(context.Background(), batch)
			_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
			return err
		}
		allActiveNodes = append(allActiveNodes, chunk...)
		if len(allActiveNodes) >= total || len(chunk) == 0 {
			break
		}
		fetchPage++
	}

	// Deterministic ordering by LogicalID
	sort.Slice(allActiveNodes, func(i, j int) bool {
		return allActiveNodes[i].LogicalID < allActiveNodes[j].LogicalID
	})

	batch.Counts.TotalNodes = len(allActiveNodes)

	// Filter nodes currently active in the probe pool
	var candidateNodes []domain.Node
	poolFilter, hasPoolFilter := c.runner.(interface{ IsNodeInPool(logicalID string) bool })
	for _, n := range allActiveNodes {
		if hasPoolFilter && poolFilter != nil && poolFilter.IsNodeInPool(n.LogicalID) {
			continue
		}
		candidateNodes = append(candidateNodes, n)
	}

	if len(candidateNodes) == 0 {
		select {
		case <-leaseLost:
			return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
		default:
		}
		batch.Counts.SkippedNodes = len(allActiveNodes)
		notifyTasksEnqueued(parentCtx)
		_ = batch.TransitionTo(domain.ProbeBatchStateSucceeded)
		_ = c.schedules.UpdateBatch(context.Background(), batch)
		_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
		return nil
	}

	// 5. Query latest observations and evaluate per-(node, kind) expiry with failure cooldown
	configuredKinds := sched.Kinds
	if len(configuredKinds) == 0 {
		configuredKinds = []domain.ProbeKind{domain.ProbeKindBaseline}
	}

	interval := time.Duration(sched.IntervalSeconds) * time.Second
	cooldown := c.failureCooldown
	if cooldown <= 0 {
		cooldown = defaultFailureCooldown
	}

	var latestObsMap map[string]map[domain.ProbeKind]domain.ProbeObservation
	if c.observations != nil {
		nodeIDs := make([]string, len(candidateNodes))
		for i, n := range candidateNodes {
			nodeIDs[i] = n.LogicalID
		}
		obsMap, err := c.observations.ListLatestByNodes(batchCtx, nodeIDs, configuredKinds)
		if err != nil {
			select {
			case <-leaseLost:
				return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
			default:
			}
			batch.RedactedError = "failed to query latest observations: " + domain.RedactSensitiveInfo(err.Error())
			_ = batch.TransitionTo(domain.ProbeBatchStateFailed)
			_ = c.schedules.UpdateBatch(context.Background(), batch)
			_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
			return err
		}
		latestObsMap = obsMap
	}

	now := c.clock().UTC()
	type nodeExpiredCandidate struct {
		node             domain.Node
		expiredKinds     []domain.ProbeKind
		hasUnobserved    bool
		oldestObservedAt time.Time
	}

	var candidates []nodeExpiredCandidate
	for _, n := range candidateNodes {
		var expiredKinds []domain.ProbeKind
		hasUnobserved := false
		var oldestObs time.Time
		hasAnyObs := false

		var nodeObs map[domain.ProbeKind]domain.ProbeObservation
		if latestObsMap != nil {
			nodeObs = latestObsMap[n.LogicalID]
		}

		for _, k := range configuredKinds {
			if latestObsMap == nil {
				// No observation repository provided (e.g. legacy/mock tests)
				expiredKinds = append(expiredKinds, k)
				hasUnobserved = true
				continue
			}

			obs, exists := nodeObs[k]
			if !exists {
				// No observation record: due for probe
				expiredKinds = append(expiredKinds, k)
				hasUnobserved = true
				continue
			}

			obsAge := now.Sub(obs.ObservedAt)
			if obsAge < 0 {
				obsAge = 0
			}

			// Cooldown on failure: don't retry failed/error probes before cooldown expires
			isFailure := obs.Verdict == domain.VerdictError || obs.Verdict == domain.VerdictUnknown
			if isFailure && obsAge < cooldown {
				continue
			}

			if obsAge >= interval {
				expiredKinds = append(expiredKinds, k)
				if !hasAnyObs || obs.ObservedAt.Before(oldestObs) {
					oldestObs = obs.ObservedAt
					hasAnyObs = true
				}
			}
		}

		if len(expiredKinds) > 0 {
			candidates = append(candidates, nodeExpiredCandidate{
				node:             n,
				expiredKinds:     expiredKinds,
				hasUnobserved:    hasUnobserved,
				oldestObservedAt: oldestObs,
			})
		}
	}

	// 6. Fair sorting & Quota budgeting to prevent bursts (e.g. 960 nodes * 5 kinds)
	// Sort candidates:
	// 1. Unobserved nodes first
	// 2. Oldest observed_at ascending (starvation prevention)
	// 3. LogicalID ascending (deterministic tie-breaker)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].hasUnobserved != candidates[j].hasUnobserved {
			return candidates[i].hasUnobserved
		}
		if !candidates[i].hasUnobserved {
			if !candidates[i].oldestObservedAt.Equal(candidates[j].oldestObservedAt) {
				return candidates[i].oldestObservedAt.Before(candidates[j].oldestObservedAt)
			}
		}
		return candidates[i].node.LogicalID < candidates[j].node.LogicalID
	})

	sweepQuota := c.sweepQuota
	if sweepQuota <= 0 {
		sweepQuota = defaultSweepQuota
	}

	var selectedCandidates []nodeExpiredCandidate
	accumulatedTasks := 0
	for _, cand := range candidates {
		candTasks := len(cand.expiredKinds)
		if accumulatedTasks+candTasks > sweepQuota && len(selectedCandidates) > 0 {
			break
		}
		selectedCandidates = append(selectedCandidates, cand)
		accumulatedTasks += candTasks
	}

	batch.Counts.SkippedNodes = len(allActiveNodes) - len(selectedCandidates)

	if len(selectedCandidates) == 0 {
		select {
		case <-leaseLost:
			return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
		default:
		}
		notifyTasksEnqueued(parentCtx)
		_ = batch.TransitionTo(domain.ProbeBatchStateSucceeded)
		_ = c.schedules.UpdateBatch(context.Background(), batch)
		_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
		return nil
	}

	// 7. Group selected candidates by expired kinds signature and partition into run chunks
	maxTasksPerRun := c.maxTasksPerRun
	if maxTasksPerRun <= 0 {
		maxTasksPerRun = defaultMaxTasks
	}

	type candidateGroup struct {
		kinds []domain.ProbeKind
		nodes []domain.Node
	}

	groupMap := make(map[string]*candidateGroup)
	var groupKeys []string

	for _, cand := range selectedCandidates {
		kindNames := make([]string, len(cand.expiredKinds))
		for i, k := range cand.expiredKinds {
			kindNames[i] = string(k)
		}
		sort.Strings(kindNames)
		key := strings.Join(kindNames, ",")

		grp, exists := groupMap[key]
		if !exists {
			grp = &candidateGroup{
				kinds: cand.expiredKinds,
			}
			groupMap[key] = grp
			groupKeys = append(groupKeys, key)
		}
		grp.nodes = append(grp.nodes, cand.node)
	}
	sort.Strings(groupKeys)

	type runChunk struct {
		nodes []domain.Node
		kinds []domain.ProbeKind
	}
	var chunks []runChunk

	for _, key := range groupKeys {
		grp := groupMap[key]
		kindsCount := len(grp.kinds)
		if kindsCount == 0 {
			kindsCount = 1
		}
		nodesPerChunk := maxTasksPerRun / kindsCount
		if nodesPerChunk <= 0 {
			nodesPerChunk = 1
		}

		for i := 0; i < len(grp.nodes); i += nodesPerChunk {
			end := i + nodesPerChunk
			if end > len(grp.nodes) {
				end = len(grp.nodes)
			}
			chunks = append(chunks, runChunk{
				nodes: grp.nodes[i:end],
				kinds: grp.kinds,
			})
		}
	}

	batch.Counts.DispatchedRuns = len(chunks)

	// Create and persist runs for each chunk
	runs := make([]*domain.ProbeRun, 0, len(chunks))
	runIDs := make([]string, 0, len(chunks))

	for chunkIdx, chunk := range chunks {
		nodeIDs := make([]string, len(chunk.nodes))
		for idx, n := range chunk.nodes {
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
				select {
				case <-leaseLost:
					return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
				default:
				}
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
	select {
	case <-leaseLost:
		return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
	default:
	}
	if err := c.schedules.UpdateBatch(batchCtx, batch); err != nil {
		select {
		case <-leaseLost:
			return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
		default:
		}
		_ = c.schedules.ReleaseLease(context.Background(), batch.ID, c.ownerID)
		return err
	}

	// 8. Execute each run chunk
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
			select {
			case <-leaseLost:
				return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
			default:
			}
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
		nodeIDs := make([]string, len(chunk.nodes))
		for idx, n := range chunk.nodes {
			nodeIDs[idx] = n.LogicalID
		}

		runErr := c.runner.Run(batchCtx, run, nodeIDs, chunk.kinds)
		if runErr != nil {
			hasRunError = true
			batch.RedactedError = domain.RedactSensitiveInfo(runErr.Error())
		}
		completedRuns++
		batch.Counts.CompletedRuns = completedRuns

		select {
		case <-leaseLost:
			return domain.NewConflictError("lease_lost", "batch lease lost to another instance")
		default:
		}
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
