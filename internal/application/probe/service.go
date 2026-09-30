// Package probe provides the application service and state machine for probe runs.
package probe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
)

const (
	// DefaultRunTimeout is the default deadline duration if not specified.
	DefaultRunTimeout = 10 * time.Minute
	// IdempotencyTTL is the duration within which identical submissions return the existing run.
	IdempotencyTTL = 24 * time.Hour
	// DefaultProbeFreshnessTTL defines the baseline freshness window used for health determination.
	DefaultProbeFreshnessTTL = time.Hour
)

// ScheduleTriggerResult embeds domain.ProbePoolStatus to preserve 100% backward compatibility
// while providing explicit incremental sweep feedback for manual status refresh.
type ScheduleTriggerResult struct {
	domain.ProbePoolStatus
	BatchID        string                 `json:"batch_id,omitempty"`
	BatchState     domain.ProbeBatchState `json:"batch_state,omitempty"`
	DispatchedRuns int                    `json:"dispatched_runs"`
	ScheduledNodes int                    `json:"scheduled_nodes"`
	ScheduledTasks int                    `json:"scheduled_tasks"`
	SkippedNodes   int                    `json:"skipped_nodes"`
	NoDueTasks     bool                   `json:"no_due_tasks"`
	RunIDs         []string               `json:"run_ids,omitempty"`
}

// CreateRunCommand specifies parameters for scheduling a probe run.
type CreateRunCommand struct {
	ActorScope     string
	IdempotencyKey string
	ConfigRevision string
	Deadline       time.Time
	NodeLogicalIDs []string
	Kinds          []domain.ProbeKind
}

type runController struct {
	ctx        context.Context
	cancel     context.CancelFunc
	deadlineAt time.Time
}

// Service orchestrates probe run lifecycle, state transitions, idempotency, and cancellation.
type Service struct {
	runs         domain.ProbeRunRepository
	nodes        domain.NodeRepository
	observations domain.ProbeObservationRepository
	scheduler    *queue.Scheduler
	audit        domain.AuditRepository
	schedules    domain.ProbeScheduleRepository
	coordinator  *PeriodicCoordinator
	clock        func() time.Time
	mu           sync.RWMutex
	createMu     sync.Mutex
	active       map[string]*runController
	runner       Runner
}

// Option configures Service dependencies.
type Option func(*Service)

// WithRunner sets a default Runner for probe execution.
func WithRunner(r Runner) Option {
	return func(s *Service) {
		s.runner = r
	}
}

// WithNodeRepository sets the node repository for computing node pool and inventory status metrics.
func WithNodeRepository(nodes domain.NodeRepository) Option {
	return func(s *Service) {
		s.nodes = nodes
	}
}

// WithObservationRepository sets the probe observation repository for computing node health metrics.
func WithObservationRepository(observations domain.ProbeObservationRepository) Option {
	return func(s *Service) {
		s.observations = observations
	}
}

// WithScheduler sets the queue scheduler for real-time node probe pool introspection.
func WithScheduler(scheduler *queue.Scheduler) Option {
	return func(s *Service) {
		s.scheduler = scheduler
	}
}

// WithScheduleRepository sets the probe schedule repository for periodic probe coordination.
func WithScheduleRepository(schedules domain.ProbeScheduleRepository) Option {
	return func(s *Service) {
		s.schedules = schedules
	}
}

// WithCoordinator sets the periodic probe coordinator.
func WithCoordinator(coordinator *PeriodicCoordinator) Option {
	return func(s *Service) {
		s.coordinator = coordinator
	}
}

// WithAudit sets an audit repository for probe run events.
func WithAudit(audit domain.AuditRepository) Option {
	return func(s *Service) {
		s.audit = audit
	}
}

// WithClock sets a custom clock function for deterministic time testing.
func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		s.clock = clock
	}
}

// NewService constructs a new probe application service.
func NewService(runs domain.ProbeRunRepository, opts ...Option) *Service {
	s := &Service{
		runs:   runs,
		clock:  time.Now,
		active: make(map[string]*runController),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Create validates and initiates a probe run with 24h idempotency enforcement.
func (s *Service) Create(ctx context.Context, cmd CreateRunCommand) (*domain.ProbeRun, error) {
	s.createMu.Lock()
	defer s.createMu.Unlock()
	key := strings.TrimSpace(cmd.IdempotencyKey)
	if key == "" {
		return nil, domain.NewValidationError("missing_idempotency_key", "idempotency_key is required")
	}

	actor := strings.TrimSpace(cmd.ActorScope)
	if actor == "" {
		actor = "default"
	}

	now := s.clock().UTC()
	deadline := cmd.Deadline
	if deadline.IsZero() {
		deadline = now.Add(DefaultRunTimeout)
	} else if deadline.Before(now) {
		return nil, domain.NewValidationError("invalid_deadline", "deadline cannot be in the past")
	}

	// 24-hour Idempotency key evaluation
	existing, err := s.runs.GetByIdempotencyKey(ctx, actor, key)
	if err == nil && existing != nil {
		age := now.Sub(existing.CreatedAt)
		if age < IdempotencyTTL {
			// In TTL: return existing run without scheduling duplicate jobs
			return existing, nil
		}
		if existing.IsTerminal() {
			return nil, domain.NewConflictError("idempotency_key_expired",
				fmt.Sprintf("idempotency key %s expired after 24 hours", key))
		}
		return existing, nil
	}
	if err != nil {
		if de, ok := domain.AsDomainError(err); ok {
			if de.Category != domain.CategoryNotFound {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	runID := domain.MustNewUUIDv7()
	run := domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: key,
		ActorScope:     actor,
		ConfigRevision: strings.TrimSpace(cmd.ConfigRevision),
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     deadline,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.runs.Create(ctx, &run); err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithDeadline(context.Background(), deadline)
	s.mu.Lock()
	s.active[runID] = &runController{
		ctx:        runCtx,
		cancel:     cancel,
		deadlineAt: deadline,
	}
	s.mu.Unlock()

	return &run, nil
}

// Get returns the probe run by ID.
func (s *Service) Get(ctx context.Context, id string) (*domain.ProbeRun, error) {
	return s.runs.GetByID(ctx, id)
}

// Cancel transitions a probe run to cancelled and broadcasts cancellation to in-flight tasks.
func (s *Service) Cancel(ctx context.Context, id string) error {
	run, err := s.runs.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if run.IsTerminal() {
		if run.State == domain.ProbeRunStateCancelled {
			return nil
		}
		return domain.NewConflictError("terminal_state",
			fmt.Sprintf("cannot cancel probe run %s in terminal state %s", id, run.State))
	}

	wasQueued := run.State == domain.ProbeRunStateQueued
	if err := run.TransitionTo(domain.ProbeRunStateCancelled); err != nil {
		return err
	}

	if err := s.runs.UpdateState(ctx, id, domain.ProbeRunStateCancelled); err != nil {
		return err
	}

	s.mu.Lock()
	if ctrl, ok := s.active[id]; ok {
		ctrl.cancel()
		if wasQueued {
			delete(s.active, id)
		}
	}
	s.mu.Unlock()

	if sched := s.resolveScheduler(); sched != nil {
		sched.CancelRun(id)
	}

	return nil
}

// Execute marks the run running and executes the provided workload with cancellation and deadline supervision.
func (s *Service) Execute(ctx context.Context, id string, fn func(ctx context.Context) error) error {
	run, err := s.runs.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := run.TransitionTo(domain.ProbeRunStateRunning); err != nil {
		return err
	}

	if err := s.runs.UpdateState(ctx, id, domain.ProbeRunStateRunning); err != nil {
		return err
	}

	s.mu.Lock()
	ctrl, ok := s.active[id]
	if !ok {
		runCtx, cancel := context.WithDeadline(context.Background(), run.DeadlineAt)
		ctrl = &runController{ctx: runCtx, cancel: cancel, deadlineAt: run.DeadlineAt}
		s.active[id] = ctrl
	}
	s.mu.Unlock()

	execCtx, execCancel := context.WithCancel(ctrl.ctx)
	if ctx != nil && ctx.Done() != nil {
		stop := context.AfterFunc(ctx, func() {
			execCancel()
		})
		defer stop()
	}
	defer execCancel()

	defer func() {
		s.mu.Lock()
		delete(s.active, id)
		s.mu.Unlock()
	}()

	execErr := fn(execCtx)

	// Check if already cancelled asynchronously by Cancel()
	current, _ := s.runs.GetByID(ctx, id)
	if current != nil && current.State == domain.ProbeRunStateCancelled {
		return execErr
	}

	if execErr == nil {
		_ = s.runs.UpdateState(ctx, id, domain.ProbeRunStateSucceeded)
		return nil
	}

	if errors.Is(execErr, context.Canceled) || execCtx.Err() == context.Canceled {
		_ = s.runs.UpdateState(ctx, id, domain.ProbeRunStateCancelled)
		return execErr
	}

	if errors.Is(execErr, context.DeadlineExceeded) || execCtx.Err() == context.DeadlineExceeded {
		_ = s.runs.UpdateState(ctx, id, domain.ProbeRunStateExpired)
		return execErr
	}

	_ = s.runs.UpdateState(ctx, id, domain.ProbeRunStateFailed)
	return execErr
}

// ExecuteTasks runs a set of probe tasks through a bounded sliding window queue.
func (s *Service) ExecuteTasks(ctx context.Context, id string, tasks []queue.Task, sched *queue.Scheduler) error {
	if sched == nil {
		return errors.New("scheduler cannot be nil")
	}

	return s.Execute(ctx, id, func(runCtx context.Context) error {
		var wg sync.WaitGroup
		var firstErr error
		var errMu sync.Mutex

		for _, task := range tasks {
			task.RunID = id
			task.Context = runCtx

			origExecute := task.Execute
			origComplete := task.OnComplete

			wg.Add(1)
			task.Execute = func(tCtx context.Context) error {
				if origExecute != nil {
					return origExecute(tCtx)
				}
				return nil
			}
			task.OnComplete = func(err error) {
				defer wg.Done()
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
				}
				if origComplete != nil {
					origComplete(err)
				}
			}

			if err := sched.Submit(task); err != nil {
				wg.Done()
				return err
			}
		}

		// Wait for run completion or context done
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-runCtx.Done():
			sched.CancelRun(id)
			<-done
			return runCtx.Err()
		case <-done:
			return firstErr
		}
	})
}

// ExpireStaleRuns finds active runs past their deadline and transitions them to expired.
func (s *Service) ExpireStaleRuns(ctx context.Context) (int, error) {
	active, err := s.runs.ListActive(ctx)
	if err != nil {
		return 0, err
	}

	now := s.clock().UTC()
	expiredCount := 0

	for _, run := range active {
		if now.After(run.DeadlineAt) {
			if err := s.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateExpired); err == nil {
				expiredCount++
				s.mu.Lock()
				if ctrl, ok := s.active[run.ID]; ok {
					ctrl.cancel()
					delete(s.active, run.ID)
				}
				s.mu.Unlock()
			}
		}
	}

	return expiredCount, nil
}

// Runner returns the configured Runner if any.
func (s *Service) Runner() Runner {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runner
}

// SetRunner sets or replaces the Runner.
func (s *Service) SetRunner(r Runner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runner = r
}

// TriggerRun triggers execution of a probe run using the provided or default runner.
func (s *Service) TriggerRun(ctx context.Context, runID string, nodeIDs []string, kinds []domain.ProbeKind, runner Runner) error {
	if runner == nil {
		s.mu.RLock()
		runner = s.runner
		s.mu.RUnlock()
	}
	if runner == nil {
		runnerErr := errors.New("probe runner is required")
		run, err := s.runs.GetByID(ctx, runID)
		if err != nil {
			return errors.Join(runnerErr, err)
		}
		if run.State == domain.ProbeRunStateQueued {
			if err := run.TransitionTo(domain.ProbeRunStateRunning); err != nil {
				return errors.Join(runnerErr, err)
			}
		}
		if err := run.TransitionTo(domain.ProbeRunStateFailed); err != nil {
			return errors.Join(runnerErr, err)
		}
		if err := s.runs.UpdateState(ctx, runID, domain.ProbeRunStateFailed); err != nil {
			return errors.Join(runnerErr, err)
		}
		return runnerErr
	}

	run, err := s.runs.GetByID(ctx, runID)
	if err != nil {
		return err
	}

	return runner.Run(ctx, run, nodeIDs, kinds)
}

// SetCoordinator configures or updates the periodic probe coordinator.
func (s *Service) SetCoordinator(coordinator *PeriodicCoordinator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coordinator = coordinator
}

// GetSchedule returns the current probe schedule configuration.
func (s *Service) GetSchedule(ctx context.Context) (*domain.ProbeSchedule, error) {
	s.mu.RLock()
	repo := s.schedules
	s.mu.RUnlock()
	if repo == nil {
		return nil, domain.NewNotFoundError("probe_schedule_not_available", "probe schedule repository not configured")
	}
	return repo.Get(ctx)
}

// UpdateSchedule updates probe schedule parameters and re-arms the periodic coordinator.
func (s *Service) UpdateSchedule(ctx context.Context, req domain.UpdateProbeScheduleRequest) (*domain.ProbeSchedule, error) {
	s.mu.RLock()
	repo := s.schedules
	coord := s.coordinator
	s.mu.RUnlock()

	if repo == nil {
		return nil, domain.NewNotFoundError("probe_schedule_not_available", "probe schedule repository not configured")
	}

	current, err := repo.Get(ctx)
	if err != nil {
		return nil, err
	}

	updated := *current
	changed := false

	if req.Enabled != nil && *req.Enabled != updated.Enabled {
		updated.Enabled = *req.Enabled
		changed = true
	}
	if req.IntervalSeconds != nil && *req.IntervalSeconds != updated.IntervalSeconds {
		updated.IntervalSeconds = *req.IntervalSeconds
		changed = true
	}
	if req.Kinds != nil {
		updated.Kinds = *req.Kinds
		changed = true
	}

	if err := updated.Validate(); err != nil {
		return nil, err
	}

	now := s.clock().UTC()
	if changed {
		updated.Generation++
		if updated.Enabled {
			// Trigger immediate lightweight sweep upon enabling or configuration change
			updated.NextDueAt = &now
		} else {
			updated.NextDueAt = nil
		}
	} else if updated.Enabled && (updated.NextDueAt == nil || updated.NextDueAt.Before(now)) {
		updated.NextDueAt = &now
	}

	if err := repo.Update(ctx, &updated); err != nil {
		return nil, err
	}

	if coord != nil {
		coord.Wake()
	}

	return &updated, nil
}

// ListBatches returns paginated probe execution batches.
func (s *Service) ListBatches(ctx context.Context, page, pageSize int) ([]domain.ProbeBatch, int, error) {
	s.mu.RLock()
	repo := s.schedules
	s.mu.RUnlock()
	if repo == nil {
		return nil, 0, domain.NewNotFoundError("probe_schedule_not_available", "probe schedule repository not configured")
	}
	return repo.ListBatches(ctx, page, pageSize)
}

// GetBatch returns a specific probe batch by ID.
func (s *Service) GetBatch(ctx context.Context, id string) (*domain.ProbeBatch, error) {
	s.mu.RLock()
	repo := s.schedules
	s.mu.RUnlock()
	if repo == nil {
		return nil, domain.NewNotFoundError("probe_schedule_not_available", "probe schedule repository not configured")
	}
	return repo.GetBatchByID(ctx, id)
}

// CancelBatch cancels an in-flight periodic probe batch and cascades cancellation to all associated runs.
func (s *Service) CancelBatch(ctx context.Context, id string) error {
	s.mu.RLock()
	repo := s.schedules
	coord := s.coordinator
	s.mu.RUnlock()

	if repo == nil {
		return domain.NewNotFoundError("probe_schedule_not_available", "probe schedule repository not configured")
	}

	batch, err := repo.GetBatchByID(ctx, id)
	if err != nil {
		return err
	}

	if batch.State.IsTerminal() {
		if batch.State == domain.ProbeBatchStateCancelled {
			return nil
		}
		return domain.NewConflictError("terminal_state",
			fmt.Sprintf("cannot cancel probe batch %s in terminal state %s", id, batch.State))
	}

	if err := batch.TransitionTo(domain.ProbeBatchStateCancelled); err != nil {
		return err
	}

	if err := repo.UpdateBatch(ctx, batch); err != nil {
		return err
	}

	if coord != nil {
		coord.CancelBatch(id)
	}

	for _, runID := range batch.RunIDs {
		_ = s.Cancel(ctx, runID)
	}

	return nil
}

// SetNodeRepository sets or replaces the node repository used for pool status metrics.
func (s *Service) SetNodeRepository(nodes domain.NodeRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = nodes
}

// SetObservationRepository sets or replaces the probe observation repository used for pool status metrics.
func (s *Service) SetObservationRepository(observations domain.ProbeObservationRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observations = observations
}

// SetScheduler sets or replaces the queue scheduler used for pool status introspection.
func (s *Service) SetScheduler(scheduler *queue.Scheduler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduler = scheduler
}

func (s *Service) resolveScheduler() *queue.Scheduler {
	s.mu.RLock()
	sched := s.scheduler
	runner := s.runner
	s.mu.RUnlock()
	if sched != nil {
		return sched
	}
	if dr, ok := runner.(*DefaultRunner); ok && dr != nil {
		return dr.scheduler
	}
	if sp, ok := runner.(interface{ Scheduler() *queue.Scheduler }); ok && sp != nil {
		return sp.Scheduler()
	}
	return nil
}

func (s *Service) resolveNodesRepo() domain.NodeRepository {
	s.mu.RLock()
	nodes := s.nodes
	runner := s.runner
	coord := s.coordinator
	s.mu.RUnlock()
	if nodes != nil {
		return nodes
	}
	if dr, ok := runner.(*DefaultRunner); ok && dr != nil && dr.nodes != nil {
		return dr.nodes
	}
	if coord != nil && coord.nodes != nil {
		return coord.nodes
	}
	return nil
}

func (s *Service) resolveObservationsRepo() domain.ProbeObservationRepository {
	s.mu.RLock()
	obs := s.observations
	runner := s.runner
	s.mu.RUnlock()
	if obs != nil {
		return obs
	}
	if dr, ok := runner.(*DefaultRunner); ok && dr != nil && dr.observations != nil {
		return dr.observations
	}
	return nil
}

// GetNodePoolState returns the real-time probe pool state ("probing", "queued", or "idle") for a node.
func (s *Service) GetNodePoolState(logicalID string) string {
	sched := s.resolveScheduler()
	if sched == nil {
		return "idle"
	}
	return sched.GetNodePoolState(logicalID)
}

func classifyNodeHealthFromObservations(obsByKind map[domain.ProbeKind]domain.ProbeObservation) string {
	if len(obsByKind) == 0 {
		return "unknown"
	}
	var (
		hasAvailable bool
		hasDegraded  bool
		hasUnhealthy bool
	)
	mapVerdict := func(verdict domain.ProbeVerdict) string {
		switch strings.ToLower(strings.TrimSpace(string(verdict))) {
		case string(domain.VerdictAvailable), "healthy":
			return "healthy"
		case string(domain.VerdictRestricted), string(domain.VerdictStale), "degraded":
			return "degraded"
		case string(domain.VerdictError), "unreachable", "unhealthy":
			return "unhealthy"
		default:
			return "unknown"
		}
	}
	for _, obs := range obsByKind {
		switch mapVerdict(obs.Verdict) {
		case "healthy":
			hasAvailable = true
		case "degraded":
			hasDegraded = true
		case "unhealthy":
			hasUnhealthy = true
		}
	}
	if baselineObs, hasBaseline := obsByKind[domain.ProbeKindBaseline]; hasBaseline {
		status := mapVerdict(baselineObs.Verdict)
		if status != "unknown" {
			return status
		}
	}
	if hasUnhealthy && !hasAvailable {
		return "unhealthy"
	}
	if hasDegraded {
		return "degraded"
	}
	if hasAvailable {
		return "healthy"
	}
	return "unknown"
}

// GetPoolStatus aggregates real-time node probe pool counts and active node health statistics.
func (s *Service) GetPoolStatus(ctx context.Context) (*domain.ProbePoolStatus, error) {
	status := &domain.ProbePoolStatus{
		ProbingNodeIDs: []string{},
		QueuedNodeIDs:  []string{},
		UpdatedAt:      s.clock().UTC(),
	}

	if sched := s.resolveScheduler(); sched != nil {
		snap := sched.SnapshotNodePool()
		if snap.ProbingNodeIDs != nil {
			status.ProbingNodeIDs = snap.ProbingNodeIDs
		}
		if snap.QueuedNodeIDs != nil {
			status.QueuedNodeIDs = snap.QueuedNodeIDs
		}
		status.ProbingCount = snap.ProbingCount
		status.QueuedWaitingCount = snap.QueuedWaitingCount
		status.QueueNodesCount = snap.QueueNodesCount
	}

	nodesRepo := s.resolveNodesRepo()
	if nodesRepo == nil {
		return status, nil
	}

	const fetchPageSize = 100
	seen := make(map[string]struct{})
	activeNodes := make([]domain.Node, 0)
	activeIDs := make([]string, 0)
	for fetchPage := 1; ; fetchPage++ {
		chunk, total, err := nodesRepo.List(ctx, domain.NodeFilter{
			ActiveOnly: true,
			Pagination: domain.Pagination{Page: fetchPage, PageSize: fetchPageSize},
		})
		if err != nil {
			return nil, err
		}
		added := 0
		for _, n := range chunk {
			if n.LogicalID == "" {
				continue
			}
			if _, exists := seen[n.LogicalID]; !exists {
				seen[n.LogicalID] = struct{}{}
				activeNodes = append(activeNodes, n)
				activeIDs = append(activeIDs, n.LogicalID)
				added++
			}
		}
		if len(activeIDs) >= total || len(chunk) == 0 || added == 0 {
			break
		}
	}

	status.TotalCount = len(activeIDs)
	if status.TotalCount == 0 {
		return status, nil
	}

	obsRepo := s.resolveObservationsRepo()
	if obsRepo == nil {
		status.UntestedCount = status.TotalCount
		return status, nil
	}

	latestByNode, err := obsRepo.ListLatestByNodes(ctx, activeIDs, nil)
	if err != nil {
		return nil, err
	}

	now := s.clock().UTC()
	for _, n := range activeNodes {
		var baselinePtr *domain.ProbeObservation
		if nodeObs, ok := latestByNode[n.LogicalID]; ok {
			if baselineObs, hasBaseline := nodeObs[domain.ProbeKindBaseline]; hasBaseline {
				obsCopy := baselineObs
				baselinePtr = &obsCopy
			}
		}
		health, _ := domain.EvaluateBaselineHealth(baselinePtr, n.ConnectionRevision, now, DefaultProbeFreshnessTTL)
		switch health {
		case domain.BaselineHealthy:
			status.HealthyCount++
			status.AvailableCount++
		case domain.BaselineDegraded:
			status.DegradedCount++
			status.AvailableCount++
		case domain.BaselineUnhealthy:
			status.UnavailableCount++
		default:
			status.UntestedCount++
		}
	}

	return status, nil
}

// TriggerScheduleNow triggers an immediate periodic deduplicated node pool enqueue
// across active nodes and returns the updated ProbePoolStatus.
func (s *Service) TriggerScheduleNow(ctx context.Context) (*ScheduleTriggerResult, error) {
	s.mu.RLock()
	coord := s.coordinator
	schedRepo := s.schedules
	runner := s.runner
	runs := s.runs
	clock := s.clock
	s.mu.RUnlock()

	if coord == nil {
		coord = NewPeriodicCoordinator(
			schedRepo,
			s.resolveNodesRepo(),
			runs,
			runner,
			WithCoordinatorObservations(s.resolveObservationsRepo()),
			WithCoordinatorClock(clock),
		)
	}

	enqueuedCh := make(chan struct{})
	doneCh := make(chan error, 1)
	var summaryMu sync.Mutex
	var sweepSummary SweepSummary

	bgCtx := WithTasksEnqueuedHook(context.Background(), func() {
		close(enqueuedCh)
	})
	bgCtx = withSweepSummaryHook(bgCtx, func(summary SweepSummary) {
		summaryMu.Lock()
		sweepSummary = summary
		summaryMu.Unlock()
	})

	go func() {
		doneCh <- coord.TriggerImmediate(bgCtx)
	}()

	select {
	case <-enqueuedCh:
		select {
		case err := <-doneCh:
			if err != nil {
				return nil, err
			}
		default:
		}
	case err := <-doneCh:
		if err != nil {
			return nil, err
		}
	case <-time.After(2 * time.Second):
	}

	poolStatus, err := s.GetPoolStatus(ctx)
	if err != nil {
		return nil, err
	}

	summaryMu.Lock()
	summary := sweepSummary
	summaryMu.Unlock()

	return &ScheduleTriggerResult{
		ProbePoolStatus: *poolStatus,
		BatchID:         summary.BatchID,
		BatchState:      summary.BatchState,
		DispatchedRuns:  summary.DispatchedRuns,
		ScheduledNodes:  summary.ScheduledNodes,
		ScheduledTasks:  summary.ScheduledTasks,
		SkippedNodes:    summary.SkippedNodes,
		NoDueTasks:      summary.NoDueTasks,
		RunIDs:          summary.RunIDs,
	}, nil
}
