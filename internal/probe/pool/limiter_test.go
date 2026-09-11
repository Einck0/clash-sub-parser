package pool_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/pool"
)

func TestDynamicLimiter_BasicAndLimit(t *testing.T) {
	lim := pool.NewLimiter(2)
	if lim.Limit() != 2 {
		t.Fatalf("expected limit 2, got %d", lim.Limit())
	}
	if lim.Active() != 0 {
		t.Fatalf("expected active 0, got %d", lim.Active())
	}

	ctx := context.Background()

	// Acquire 1
	if err := lim.Acquire(ctx); err != nil {
		t.Fatalf("acquire 1 failed: %v", err)
	}
	if lim.Active() != 1 {
		t.Fatalf("expected active 1, got %d", lim.Active())
	}

	// Acquire 2
	if err := lim.Acquire(ctx); err != nil {
		t.Fatalf("acquire 2 failed: %v", err)
	}
	if lim.Active() != 2 {
		t.Fatalf("expected active 2, got %d", lim.Active())
	}

	// Acquire 3 should block until release or context cancel
	ctxTimeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err := lim.Acquire(ctxTimeout)
	if err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if lim.Active() != 2 {
		t.Fatalf("expected active 2 after cancelled acquire, got %d", lim.Active())
	}

	// Release 1
	lim.Release()
	if lim.Active() != 1 {
		t.Fatalf("expected active 1 after release, got %d", lim.Active())
	}

	// Now acquire should succeed immediately
	if err := lim.Acquire(ctx); err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	if lim.Active() != 2 {
		t.Fatalf("expected active 2, got %d", lim.Active())
	}

	lim.Release()
	lim.Release()
	if lim.Active() != 0 {
		t.Fatalf("expected active 0, got %d", lim.Active())
	}
}

func TestDynamicLimiter_DynamicResize(t *testing.T) {
	lim := pool.NewLimiter(2)
	ctx := context.Background()

	// Acquire 2 slots
	if err := lim.Acquire(ctx); err != nil {
		t.Fatalf("acquire 1 failed: %v", err)
	}
	if err := lim.Acquire(ctx); err != nil {
		t.Fatalf("acquire 2 failed: %v", err)
	}

	// Queue up 2 waiters in background
	var acquired3, acquired4 atomic.Bool
	go func() {
		if err := lim.Acquire(ctx); err == nil {
			acquired3.Store(true)
		}
	}()
	go func() {
		if err := lim.Acquire(ctx); err == nil {
			acquired4.Store(true)
		}
	}()

	time.Sleep(50 * time.Millisecond)
	if acquired3.Load() || acquired4.Load() {
		t.Fatalf("waiters should not have acquired slots yet")
	}

	// Dynamically expand limit to 4
	lim.SetLimit(4)
	if lim.Limit() != 4 {
		t.Fatalf("expected limit 4, got %d", lim.Limit())
	}

	// Both waiters should now acquire without any Release calls
	time.Sleep(50 * time.Millisecond)
	if !acquired3.Load() || !acquired4.Load() {
		t.Fatalf("expected both waiters to acquire after limit increased to 4")
	}
	if lim.Active() != 4 {
		t.Fatalf("expected active 4, got %d", lim.Active())
	}

	// Dynamically shrink limit to 1
	lim.SetLimit(1)
	if lim.Limit() != 1 {
		t.Fatalf("expected limit 1, got %d", lim.Limit())
	}

	// Releasing 3 should bring active to 1 without waking new waiters
	lim.Release()
	lim.Release()
	lim.Release()
	if lim.Active() != 1 {
		t.Fatalf("expected active 1 after 3 releases with limit 1, got %d", lim.Active())
	}

	lim.Release()
	if lim.Active() != 0 {
		t.Fatalf("expected active 0, got %d", lim.Active())
	}
}

func TestDynamicLimiter_ConcurrentStress(t *testing.T) {
	const (
		concurrency = 50
		iterations  = 1000
	)
	lim := pool.NewLimiter(concurrency)
	var activeMax atomic.Int64
	var currentActive atomic.Int64
	var wg sync.WaitGroup

	ctx := context.Background()

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lim.Acquire(ctx); err != nil {
				return
			}
			cur := currentActive.Add(1)
			for {
				max := activeMax.Load()
				if cur <= max || activeMax.CompareAndSwap(max, cur) {
					break
				}
			}
			time.Sleep(100 * time.Microsecond)
			currentActive.Add(-1)
			lim.Release()
		}()
	}

	wg.Wait()

	if activeMax.Load() > int64(concurrency) {
		t.Fatalf("activeMax %d exceeded concurrency limit %d", activeMax.Load(), concurrency)
	}
	if lim.Active() != 0 {
		t.Fatalf("expected active 0 at end, got %d", lim.Active())
	}
}
