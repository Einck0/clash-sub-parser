package probe

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
)

type memoryRuns struct {
	mu    sync.Mutex
	items map[string]domain.ProbeRun
}

func (m *memoryRuns) GetByID(_ context.Context, id string) (*domain.ProbeRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.items[id]
	if !ok {
		return nil, domain.NewNotFoundError("probe_run_not_found", "not found")
	}
	copy := run
	return &copy, nil
}

func (m *memoryRuns) GetByIdempotencyKey(_ context.Context, actor, key string) (*domain.ProbeRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.items {
		if run.ActorScope == actor && run.IdempotencyKey == key {
			copy := run
			return &copy, nil
		}
	}
	return nil, domain.NewNotFoundError("probe_run_not_found", "not found")
}

func (m *memoryRuns) Create(_ context.Context, run *domain.ProbeRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[run.ID] = *run
	return nil
}

func (m *memoryRuns) UpdateState(_ context.Context, id string, state domain.ProbeRunState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.items[id]
	if !ok {
		return domain.NewNotFoundError("probe_run_not_found", "not found")
	}
	run.State = state
	run.UpdatedAt = time.Now().UTC()
	m.items[id] = run
	return nil
}

func (m *memoryRuns) ListActive(_ context.Context) ([]domain.ProbeRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var active []domain.ProbeRun
	for _, run := range m.items {
		if run.State == domain.ProbeRunStateQueued || run.State == domain.ProbeRunStateRunning {
			active = append(active, run)
		}
	}
	return active, nil
}

func (m *memoryRuns) List(_ context.Context, state *domain.ProbeRunState, page, pageSize int) ([]domain.ProbeRun, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]domain.ProbeRun, 0)
	for _, run := range m.items {
		if state == nil || run.State == *state {
			items = append(items, run)
		}
	}
	if pageSize == 0 {
		return items, len(items), nil
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []domain.ProbeRun{}, len(items), nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], len(items), nil
}

func TestServiceCreateIsIdempotentWithinTTL(t *testing.T) {
	repo := &memoryRuns{items: map[string]domain.ProbeRun{}}
	svc := NewService(repo)

	command := CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "request-1",
		ConfigRevision: "rev-1",
		Deadline:       time.Now().Add(time.Hour),
	}

	first, err := svc.Create(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Create(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}

	if first.ID != second.ID {
		t.Fatalf("idempotent create returned different IDs: %s, %s", first.ID, second.ID)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected one run in repo, got %d", len(repo.items))
	}
}

func TestServiceCreateRejectsExpiredIdempotencyKey(t *testing.T) {
	currentTime := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakeClock := func() time.Time {
		return currentTime
	}

	repo := &memoryRuns{items: map[string]domain.ProbeRun{
		"old-run": {
			ID:             "old-run",
			IdempotencyKey: "expired-key",
			ActorScope:     "admin",
			State:          domain.ProbeRunStateSucceeded,
			CreatedAt:      currentTime.Add(-25 * time.Hour), // 25 hours old
			DeadlineAt:     currentTime.Add(-24 * time.Hour),
		},
	}}

	svc := NewService(repo, WithClock(fakeClock))

	_, err := svc.Create(context.Background(), CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "expired-key",
		Deadline:       currentTime.Add(time.Hour),
	})

	var conflict *domain.DomainError
	if !errors.As(err, &conflict) || conflict.Code != "idempotency_key_expired" {
		t.Fatalf("expected idempotency_key_expired conflict error, got %v", err)
	}
}

func TestServiceExecuteTransitionsToSucceeded(t *testing.T) {
	repo := &memoryRuns{items: map[string]domain.ProbeRun{}}
	svc := NewService(repo)

	run, err := svc.Create(context.Background(), CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "request-2",
		Deadline:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Execute(context.Background(), run.ID, func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := repo.GetByID(context.Background(), run.ID)
	if got.State != domain.ProbeRunStateSucceeded {
		t.Fatalf("state = %s, want succeeded", got.State)
	}
}

func TestServiceCancelQueuedRunDirectly(t *testing.T) {
	repo := &memoryRuns{items: map[string]domain.ProbeRun{}}
	svc := NewService(repo)

	run, err := svc.Create(context.Background(), CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "req-queued-cancel",
		Deadline:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Cancel(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := repo.GetByID(context.Background(), run.ID)
	if got.State != domain.ProbeRunStateCancelled {
		t.Fatalf("state = %s, want cancelled directly from queued", got.State)
	}

	// Idempotent second cancel must succeed without conflict
	if err := svc.Cancel(context.Background(), run.ID); err != nil {
		t.Fatalf("second cancel should be idempotent, got %v", err)
	}
}

func TestServiceCancelPropagatesToRunningWorker(t *testing.T) {
	repo := &memoryRuns{items: map[string]domain.ProbeRun{}}
	svc := NewService(repo)

	run, err := svc.Create(context.Background(), CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "request-3",
		Deadline:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	var observed atomic.Bool

	go func() {
		done <- svc.Execute(context.Background(), run.ID, func(ctx context.Context) error {
			<-ctx.Done()
			observed.Store(true)
			return ctx.Err()
		})
	}()

	for i := 0; i < 100; i++ {
		current, _ := repo.GetByID(context.Background(), run.ID)
		if current.State == domain.ProbeRunStateRunning {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	if err := svc.Cancel(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}

	err = <-done
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if !observed.Load() {
		t.Fatal("worker did not observe cancellation")
	}

	got, _ := repo.GetByID(context.Background(), run.ID)
	if got.State != domain.ProbeRunStateCancelled {
		t.Fatalf("state = %s, want cancelled", got.State)
	}
}

func TestServiceDeadlineExceededTransitionsToExpired(t *testing.T) {
	repo := &memoryRuns{items: map[string]domain.ProbeRun{}}
	svc := NewService(repo)

	// Short deadline of 5ms
	run, err := svc.Create(context.Background(), CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "req-deadline",
		Deadline:       time.Now().Add(5 * time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Execute(context.Background(), run.ID, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded error, got %v", err)
	}

	got, _ := repo.GetByID(context.Background(), run.ID)
	if got.State != domain.ProbeRunStateExpired {
		t.Fatalf("state = %s, want expired", got.State)
	}
}

func TestServiceExecuteTasksWithQueueScheduler(t *testing.T) {
	repo := &memoryRuns{items: map[string]domain.ProbeRun{}}
	svc := NewService(repo)

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer sched.Close()

	run, err := svc.Create(context.Background(), CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "req-tasks",
		Deadline:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	var completedTasks atomic.Int32
	tasks := []queue.Task{
		{
			LogicalID: "node-1",
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				completedTasks.Add(1)
				return nil
			},
		},
		{
			LogicalID: "node-2",
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				completedTasks.Add(1)
				return nil
			},
		},
	}

	if err := svc.ExecuteTasks(context.Background(), run.ID, tasks, sched); err != nil {
		t.Fatal(err)
	}

	if got := completedTasks.Load(); got != 2 {
		t.Fatalf("expected 2 completed tasks, got %d", got)
	}

	runDB, _ := repo.GetByID(context.Background(), run.ID)
	if runDB.State != domain.ProbeRunStateSucceeded {
		t.Fatalf("run state = %s, want succeeded", runDB.State)
	}
}

func TestServiceExpireStaleRuns(t *testing.T) {
	currentTime := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakeClock := func() time.Time {
		return currentTime
	}

	repo := &memoryRuns{items: map[string]domain.ProbeRun{
		"stale-run-1": {
			ID:         "stale-run-1",
			State:      domain.ProbeRunStateRunning,
			DeadlineAt: currentTime.Add(-10 * time.Minute),
		},
		"active-run-2": {
			ID:         "active-run-2",
			State:      domain.ProbeRunStateRunning,
			DeadlineAt: currentTime.Add(10 * time.Minute),
		},
	}}

	svc := NewService(repo, WithClock(fakeClock))

	count, err := svc.ExpireStaleRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 expired run, got %d", count)
	}

	r1, _ := repo.GetByID(context.Background(), "stale-run-1")
	if r1.State != domain.ProbeRunStateExpired {
		t.Fatalf("stale run state = %s, want expired", r1.State)
	}

	r2, _ := repo.GetByID(context.Background(), "active-run-2")
	if r2.State != domain.ProbeRunStateRunning {
		t.Fatalf("active run state = %s, want running", r2.State)
	}
}

func TestServiceRejectsPastDeadline(t *testing.T) {
	repo := &memoryRuns{items: map[string]domain.ProbeRun{}}
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "request-past",
		Deadline:       time.Now().Add(-time.Second),
	})
	if err == nil {
		t.Fatal("expected deadline validation error")
	}

	var validation *domain.DomainError
	if !errors.As(err, &validation) || validation.Code != "invalid_deadline" {
		t.Fatalf("expected invalid_deadline error, got %v", err)
	}
}

func TestProbeServiceTriggerRunWithoutRunnerFailsRun(t *testing.T) {
	repo := &memoryRuns{items: map[string]domain.ProbeRun{}}
	svc := NewService(repo) // No runner configured

	run, err := svc.Create(context.Background(), CreateRunCommand{
		ActorScope:     "admin",
		IdempotencyKey: "key-no-runner",
		Deadline:       time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = svc.TriggerRun(context.Background(), run.ID, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error from TriggerRun with no runner, got nil")
	}

	updated, err := repo.GetByID(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.State != domain.ProbeRunStateFailed {
		t.Fatalf("expected run state failed, got %s", updated.State)
	}
}

