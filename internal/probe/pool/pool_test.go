package pool_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/pool"
)

func TestPool_SubmitAndResults(t *testing.T) {
	opts := pool.DefaultOptions()
	opts.Concurrency = 10
	p := pool.New[int](opts)
	defer p.Stop()

	ctx := context.Background()
	const numTasks = 20

	for i := 0; i < numTasks; i++ {
		val := i
		err := p.Submit(ctx, pool.Task[int]{
			ID: fmt.Sprintf("task-%d", i),
			Fn: func(taskCtx context.Context) (int, error) {
				return val * 2, nil
			},
		})
		if err != nil {
			t.Fatalf("submit failed: %v", err)
		}
	}

	results := make(map[string]int)
	for i := 0; i < numTasks; i++ {
		select {
		case res := <-p.Results():
			if res.Error != nil {
				t.Fatalf("unexpected error for %s: %v", res.ID, res.Error)
			}
			results[res.ID] = res.Value
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout waiting for results")
		}
	}

	if len(results) != numTasks {
		t.Fatalf("expected %d results, got %d", numTasks, len(results))
	}
	for i := 0; i < numTasks; i++ {
		expected := i * 2
		id := fmt.Sprintf("task-%d", i)
		if results[id] != expected {
			t.Fatalf("expected %s to have value %d, got %d", id, expected, results[id])
		}
	}
}

func TestPool_PanicRecovery(t *testing.T) {
	opts := pool.DefaultOptions()
	opts.Concurrency = 5
	p := pool.New[string](opts)
	defer p.Stop()

	ctx := context.Background()

	err := p.Submit(ctx, pool.Task[string]{
		ID: "panicking-task",
		Fn: func(taskCtx context.Context) (string, error) {
			panic("something went terribly wrong")
		},
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	select {
	case res := <-p.Results():
		if res.Error == nil {
			t.Fatalf("expected error from panicking task, got nil")
		}
		if !strings.Contains(res.Error.Error(), "panic") {
			t.Fatalf("expected error to mention panic, got %v", res.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for panic result")
	}

	// Verify pool is still alive and accepting new tasks
	err = p.Submit(ctx, pool.Task[string]{
		ID: "healthy-task",
		Fn: func(taskCtx context.Context) (string, error) {
			return "healthy", nil
		},
	})
	if err != nil {
		t.Fatalf("submit healthy failed: %v", err)
	}

	select {
	case res := <-p.Results():
		if res.Value != "healthy" || res.Error != nil {
			t.Fatalf("unexpected result: value=%s, err=%v", res.Value, res.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for healthy task")
	}
}

func TestPool_TaskTimeout(t *testing.T) {
	opts := pool.DefaultOptions()
	opts.Concurrency = 5
	p := pool.New[string](opts)
	defer p.Stop()

	ctx := context.Background()

	start := time.Now()
	err := p.Submit(ctx, pool.Task[string]{
		ID:      "slow-task",
		Timeout: 50 * time.Millisecond,
		Fn: func(taskCtx context.Context) (string, error) {
			select {
			case <-time.After(500 * time.Millisecond):
				return "done", nil
			case <-taskCtx.Done():
				return "", taskCtx.Err()
			}
		},
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	select {
	case res := <-p.Results():
		elapsed := time.Since(start)
		if elapsed > 300*time.Millisecond {
			t.Fatalf("task took %v, expected timeout around 50ms", elapsed)
		}
		if !errors.Is(res.Error, context.DeadlineExceeded) && !strings.Contains(res.Error.Error(), "deadline exceeded") {
			t.Fatalf("expected DeadlineExceeded, got %v", res.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for result")
	}
}

func TestPool_DispatchBatch(t *testing.T) {
	opts := pool.DefaultOptions()
	opts.Concurrency = 50
	p := pool.New[int](opts)
	defer p.Stop()

	const totalTasks = 300
	tasks := make([]pool.Task[int], totalTasks)
	for i := 0; i < totalTasks; i++ {
		val := i
		tasks[i] = pool.Task[int]{
			ID: fmt.Sprintf("batch-%d", i),
			Fn: func(taskCtx context.Context) (int, error) {
				time.Sleep(2 * time.Millisecond)
				return val + 1, nil
			},
		}
	}

	ctx := context.Background()
	resultsChan := p.DispatchBatch(ctx, tasks)

	count := 0
	received := make(map[string]int)
	for res := range resultsChan {
		if res.Error != nil {
			t.Fatalf("unexpected error in batch: %v", res.Error)
		}
		received[res.ID] = res.Value
		count++
	}

	if count != totalTasks {
		t.Fatalf("expected %d results, got %d", totalTasks, count)
	}
	if len(received) != totalTasks {
		t.Fatalf("expected %d unique items, got %d", totalTasks, len(received))
	}
}

func TestPool_CancelAndStop(t *testing.T) {
	opts := pool.DefaultOptions()
	opts.Concurrency = 10
	p := pool.New[int](opts)

	ctx := context.Background()

	// Submit tasks that wait on cancellation
	for i := 0; i < 5; i++ {
		_ = p.Submit(ctx, pool.Task[int]{
			ID: fmt.Sprintf("task-%d", i),
			Fn: func(taskCtx context.Context) (int, error) {
				<-taskCtx.Done()
				return 0, taskCtx.Err()
			},
		})
	}

	// Cancel pool
	p.Cancel()

	// All tasks should return with context cancelled
	for i := 0; i < 5; i++ {
		select {
		case res := <-p.Results():
			if res.Error == nil {
				t.Fatalf("expected cancellation error, got nil")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for cancelled task")
		}
	}

	// Stop pool
	p.Stop()
	select {
	case <-p.Done():
		// Stopped cleanly
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for Done channel")
	}

	// Submitting after stop should return error
	err := p.Submit(ctx, pool.Task[int]{
		ID: "after-stop",
		Fn: func(taskCtx context.Context) (int, error) { return 1, nil },
	})
	if err == nil {
		t.Fatalf("expected error submitting to stopped pool")
	}
}

func TestPool_DynamicConcurrency_100_to_500(t *testing.T) {
	opts := pool.DefaultOptions()
	opts.Concurrency = 100
	opts.MinConcurrency = 10
	opts.MaxConcurrency = 500
	p := pool.New[int](opts)
	defer p.Stop()

	if p.Concurrency() != 100 {
		t.Fatalf("expected concurrency 100, got %d", p.Concurrency())
	}

	// Tune to 500
	p.SetConcurrency(500)
	if p.Concurrency() != 500 {
		t.Fatalf("expected concurrency 500, got %d", p.Concurrency())
	}

	// Verify clamping to MinConcurrency and MaxConcurrency
	p.SetConcurrency(5)
	if p.Concurrency() != 10 {
		t.Fatalf("expected clamped min concurrency 10, got %d", p.Concurrency())
	}

	p.SetConcurrency(1000)
	if p.Concurrency() != 500 {
		t.Fatalf("expected clamped max concurrency 500, got %d", p.Concurrency())
	}
}

func TestPool_ConcurrentStress_500(t *testing.T) {
	opts := pool.DefaultOptions()
	opts.Concurrency = 500
	opts.MinConcurrency = 10
	opts.MaxConcurrency = 500
	p := pool.New[int](opts)
	defer p.Stop()

	const totalTasks = 1000
	var executed atomic.Int64

	tasks := make([]pool.Task[int], totalTasks)
	for i := 0; i < totalTasks; i++ {
		tasks[i] = pool.Task[int]{
			ID: fmt.Sprintf("stress-%d", i),
			Fn: func(taskCtx context.Context) (int, error) {
				time.Sleep(500 * time.Microsecond)
				executed.Add(1)
				return 1, nil
			},
		}
	}

	ctx := context.Background()
	resultsChan := p.DispatchBatch(ctx, tasks)

	count := 0
	for res := range resultsChan {
		if res.Error != nil {
			t.Fatalf("unexpected error in stress: %v", res.Error)
		}
		count++
	}

	if count != totalTasks {
		t.Fatalf("expected %d results, got %d", totalTasks, count)
	}
	if executed.Load() != int64(totalTasks) {
		t.Fatalf("expected %d tasks executed, got %d", totalTasks, executed.Load())
	}
}
