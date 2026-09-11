package pool_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/pool"
)

func TestPool_ChaosAndZeroGoroutineLeak(t *testing.T) {
	// Baseline goroutine count before starting pool
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	opts := pool.DefaultOptions()
	opts.Concurrency = 250
	opts.MinConcurrency = 20
	opts.MaxConcurrency = 500
	opts.TaskTimeout = 30 * time.Millisecond
	opts.MemoryLimitBytes = 150 * 1024 * 1024

	p := pool.New[string](opts)

	const totalTasks = 1200
	tasks := make([]pool.Task[string], totalTasks)

	var (
		normalCount  atomic.Int64
		timeoutCount atomic.Int64
		panicCount   atomic.Int64
	)

	for i := 0; i < totalTasks; i++ {
		idx := i
		var timeout time.Duration
		if idx%4 == 1 {
			// Deliberately short timeout to test timeout interception under load
			timeout = 10 * time.Millisecond
		}

		tasks[i] = pool.Task[string]{
			ID:      fmt.Sprintf("chaos-%d", idx),
			Timeout: timeout,
			Fn: func(ctx context.Context) (string, error) {
				switch idx % 4 {
				case 0:
					// Normal fast task
					normalCount.Add(1)
					return "ok", nil
				case 1:
					// Deliberately slow task that triggers timeout
					select {
					case <-time.After(80 * time.Millisecond):
						return "slow-done", nil
					case <-ctx.Done():
						timeoutCount.Add(1)
						return "", ctx.Err()
					}
				case 2:
					// Panic task
					panicCount.Add(1)
					panic("chaos panic simulated")
				case 3:
					// Variable delay task
					time.Sleep(2 * time.Millisecond)
					normalCount.Add(1)
					return "delayed-ok", nil
				default:
					return "default", nil
				}
			},
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Concurrently dynamically resize pool while batch is executing
	go func() {
		time.Sleep(10 * time.Millisecond)
		p.SetConcurrency(500)
		time.Sleep(15 * time.Millisecond)
		p.SetConcurrency(150)
	}()

	resultsChan := p.DispatchBatch(ctx, tasks)

	var totalReceived int
	for res := range resultsChan {
		totalReceived++
		if res.Error != nil && errors.Is(res.Error, context.DeadlineExceeded) {
			// verified deadline exceeded
		}
	}

	if totalReceived != totalTasks {
		t.Fatalf("expected %d total results, received %d", totalTasks, totalReceived)
	}

	stats := p.Stats()
	if stats.PanicTasks == 0 {
		t.Fatalf("expected recorded panic tasks > 0, got %d", stats.PanicTasks)
	}
	if stats.TimedOutTasks == 0 {
		t.Fatalf("expected recorded timed out tasks > 0, got %d", stats.TimedOutTasks)
	}

	// Graceful shutdown
	p.Stop()

	select {
	case <-p.Done():
		// Confirmed cleanly terminated
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for pool to finish Stop")
	}

	// Check for Goroutine leaks: all spawned worker goroutines must have exited
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()

	// Allow a small tolerance (<= 3) for standard runtime/testing goroutines
	diff := finalGoroutines - initialGoroutines
	if diff > 3 {
		t.Fatalf("potential goroutine leak detected: initial=%d, final=%d, diff=%d",
			initialGoroutines, finalGoroutines, diff)
	}
}

func TestPool_RapidContextCancellationChaos(t *testing.T) {
	opts := pool.DefaultOptions()
	opts.Concurrency = 200
	p := pool.New[int](opts)
	defer p.Stop()

	const tasksPerRound = 150
	for round := 0; round < 5; round++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)

		tasks := make([]pool.Task[int], tasksPerRound)
		for i := 0; i < tasksPerRound; i++ {
			val := i
			tasks[i] = pool.Task[int]{
				ID: fmt.Sprintf("round-%d-task-%d", round, i),
				Fn: func(taskCtx context.Context) (int, error) {
					select {
					case <-time.After(50 * time.Millisecond):
						return val, nil
					case <-taskCtx.Done():
						return 0, taskCtx.Err()
					}
				},
			}
		}

		results := p.DispatchBatch(ctx, tasks)
		received := 0
		for range results {
			received++
		}
		cancel()

		if received != tasksPerRound {
			t.Fatalf("round %d: expected %d results, got %d", round, tasksPerRound, received)
		}
	}
}
