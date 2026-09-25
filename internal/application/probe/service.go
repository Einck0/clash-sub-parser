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
)

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
	runs     domain.ProbeRunRepository
	audit    domain.AuditRepository
	clock    func() time.Time
	mu       sync.RWMutex
	createMu sync.Mutex
	active   map[string]*runController
	runner   Runner
}

// Option configures Service dependencies.
type Option func(*Service)

// WithRunner sets a default Runner for probe execution.
func WithRunner(r Runner) Option {
	return func(s *Service) {
		s.runner = r
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

