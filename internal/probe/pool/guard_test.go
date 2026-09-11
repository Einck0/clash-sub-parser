package pool_test

import (
	"runtime/debug"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/pool"
)

func TestMemoryGuard_WatermarkTriggerAndRecovery(t *testing.T) {
	var currentMockAlloc atomic.Uint64
	// Initial alloc: 50MB (within 150MB limit, ratio = 33.3%)
	currentMockAlloc.Store(50 * 1024 * 1024)

	opts := pool.DefaultOptions()
	opts.MemoryLimitBytes = 150 * 1024 * 1024 // 150MB
	opts.HighWatermarkRatio = 0.80             // 120MB
	opts.LowWatermarkRatio = 0.60              // 90MB
	opts.MonitorInterval = 10 * time.Millisecond
	opts.MemStatsFunc = func() uint64 {
		return currentMockAlloc.Load()
	}

	var triggeredActive atomic.Bool
	var triggerCount atomic.Int64

	guard := pool.NewMemoryGuard(opts, func(active bool, ratio float64) {
		triggeredActive.Store(active)
		triggerCount.Add(1)
	})
	defer guard.Close()

	// Initially not active
	time.Sleep(30 * time.Millisecond)
	if guard.IsBackpressureActive() {
		t.Fatalf("expected backpressure to be inactive initially")
	}

	// Spike memory to 130MB (ratio = 86.7% >= 80%)
	currentMockAlloc.Store(130 * 1024 * 1024)
	guard.CheckMemory()

	for i := 0; i < 20 && !triggeredActive.Load(); i++ {
		time.Sleep(5 * time.Millisecond)
	}

	if !guard.IsBackpressureActive() {
		t.Fatalf("expected backpressure to become active when exceeding high watermark")
	}
	if !triggeredActive.Load() {
		t.Fatalf("expected onTrigger callback with active=true")
	}
	if guard.BackpressureEvents() < 1 {
		t.Fatalf("expected at least 1 backpressure event, got %d", guard.BackpressureEvents())
	}
	if guard.LastAllocBytes() != 130*1024*1024 {
		t.Fatalf("expected last alloc to be 130MB, got %d", guard.LastAllocBytes())
	}

	// Memory drops to 70MB (ratio = 46.7% <= 60%)
	currentMockAlloc.Store(70 * 1024 * 1024)
	guard.CheckMemory()

	for i := 0; i < 20 && triggeredActive.Load(); i++ {
		time.Sleep(5 * time.Millisecond)
	}

	if guard.IsBackpressureActive() {
		t.Fatalf("expected backpressure to be deactivated below low watermark")
	}
	if triggeredActive.Load() {
		t.Fatalf("expected onTrigger callback with active=false")
	}
}

func TestMemoryGuard_SetMemoryLimitApplied(t *testing.T) {
	limit := int64(150 * 1024 * 1024) // 150MB
	opts := pool.DefaultOptions()
	opts.MemoryLimitBytes = limit

	guard := pool.NewMemoryGuard(opts, nil)
	defer guard.Close()

	// Verify runtime/debug.SetMemoryLimit has been set
	// Reading current limit without altering it
	currentLimit := debug.SetMemoryLimit(-1)
	if currentLimit != limit {
		t.Fatalf("expected runtime memory limit %d, got %d", limit, currentLimit)
	}
}
