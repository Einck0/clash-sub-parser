package pool_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/pool"
)

func TestPool_DynamicBackpressure_WindowScaling(t *testing.T) {
	var mockAlloc atomic.Uint64
	mockAlloc.Store(50 * 1024 * 1024) // 50MB

	opts := pool.DefaultOptions()
	opts.Concurrency = 100
	opts.MinConcurrency = 10
	opts.MaxConcurrency = 500
	opts.MemoryLimitBytes = 150 * 1024 * 1024 // 150MB
	opts.HighWatermarkRatio = 0.80             // 120MB
	opts.LowWatermarkRatio = 0.60              // 90MB
	opts.BackpressureDelay = 5 * time.Millisecond
	opts.MonitorInterval = 10 * time.Millisecond
	opts.MemStatsFunc = func() uint64 {
		return mockAlloc.Load()
	}

	p := pool.New[int](opts)
	defer p.Stop()

	if p.Concurrency() != 100 {
		t.Fatalf("expected initial concurrency 100, got %d", p.Concurrency())
	}
	stats := p.Stats()
	if stats.BackpressureActive {
		t.Fatalf("expected backpressure to be initially false")
	}

	// Inject high memory pressure (135MB > 120MB threshold)
	mockAlloc.Store(135 * 1024 * 1024)
	p.Guard().CheckMemory()

	stats = p.Stats()
	if !stats.BackpressureActive {
		t.Fatalf("expected backpressure to be active")
	}
	if stats.CurrentConcurrency != 50 {
		t.Fatalf("expected concurrency window to be halved to 50 under backpressure, got %d", stats.CurrentConcurrency)
	}
	if stats.BackpressureEvents < 1 {
		t.Fatalf("expected at least 1 backpressure event")
	}

	// Dispatch batch under backpressure to verify smooth throttled execution
	const batchSize = 60
	tasks := make([]pool.Task[int], batchSize)
	for i := 0; i < batchSize; i++ {
		val := i
		tasks[i] = pool.Task[int]{
			ID: fmt.Sprintf("bp-task-%d", i),
			Fn: func(ctx context.Context) (int, error) {
				return val + 1, nil
			},
		}
	}

	ctx := context.Background()
	resultsChan := p.DispatchBatch(ctx, tasks)
	count := 0
	for res := range resultsChan {
		if res.Error != nil {
			t.Fatalf("unexpected error in throttled task: %v", res.Error)
		}
		count++
	}
	if count != batchSize {
		t.Fatalf("expected %d results, got %d", batchSize, count)
	}

	// Recover memory to 70MB (< 90MB low watermark)
	mockAlloc.Store(70 * 1024 * 1024)
	p.Guard().CheckMemory()

	stats = p.Stats()
	if stats.BackpressureActive {
		t.Fatalf("expected backpressure to recover to false")
	}
	if stats.CurrentConcurrency != 100 {
		t.Fatalf("expected concurrency to restore to 100, got %d", stats.CurrentConcurrency)
	}
}
