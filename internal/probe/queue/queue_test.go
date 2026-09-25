package queue

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestSchedulerBoundsConcurrencyAndRejectsNodeConflict(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, QueueSize: 5, NodeTTL: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var active, maxActive atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	task := Task{
		RunID:     "run-1",
		LogicalID: "node-a",
		Kind:      domain.ProbeKindBaseline,
		Execute: func(context.Context) error {
			current := active.Add(1)
			for {
				prev := maxActive.Load()
				if current <= prev || maxActive.CompareAndSwap(prev, current) {
					break
				}
			}
			close(started)
			<-release
			active.Add(-1)
			return nil
		},
	}

	if err := s.Submit(task); err != nil {
		t.Fatal(err)
	}
	<-started

	// Second task for same node and kind while active must be rejected with ErrNodeConflict
	if err := s.Submit(task); !errors.Is(err, ErrNodeConflict) {
		t.Fatalf("expected ErrNodeConflict, got %v", err)
	}

	close(release)
	s.Wait()

	if got := maxActive.Load(); got > 10 {
		t.Fatalf("concurrency exceeded bound: %d", got)
	}
}

func TestSchedulerReportsCapacityWithoutUnboundedQueue(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, RunConcurrency: 10, QueueSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	var startedCount atomic.Int32

	// Submit 10 tasks to occupy all 10 concurrency workers
	for i := 0; i < 10; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		err := s.Submit(Task{
			RunID:     "run-block",
			LogicalID: nodeID,
			Kind:      domain.ProbeKindGeo,
			Execute: func(context.Context) error {
				if startedCount.Add(1) == 10 {
					close(started)
				}
				<-release
				return nil
			},
		})
		if err != nil {
			t.Fatalf("failed to submit worker task %d: %v", i, err)
		}
	}

	<-started

	// Fill the pending queue buffer of size 2
	for i := 0; i < 12; i++ {
		nodeID := fmt.Sprintf("queue-node-%d", i)
		err := s.Submit(Task{
			RunID:     "run-queue",
			LogicalID: nodeID,
			Kind:      domain.ProbeKindGeo,
			Execute:   func(context.Context) error { return nil },
		})
		if err != nil {
			t.Fatalf("failed to submit queued task %d: %v", i, err)
		}
	}

	// The 13th task exceeds capacity
	err = s.Submit(Task{
		RunID:     "run-overflow",
		LogicalID: "overflow-node",
		Kind:      domain.ProbeKindGeo,
		Execute:   func(context.Context) error { return nil },
	})
	if !errors.Is(err, ErrCapacityExceeded) {
		t.Fatalf("expected ErrCapacityExceeded, got %v", err)
	}

	close(release)
	s.Wait()
}

func TestSubmitWithContextWaitsCancelsAndRecoversCapacity(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, RunConcurrency: 10, QueueSize: 11})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	release := make(chan struct{})
	started := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		err := s.Submit(Task{RunID: "blocked", LogicalID: fmt.Sprintf("active-%d", i), Kind: domain.ProbeKindGeo,
			Execute: func(context.Context) error { started <- struct{}{}; <-release; return nil }})
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		<-started
	}
	for i := 0; i < 11; i++ {
		if err := s.Submit(Task{RunID: "queued", LogicalID: fmt.Sprintf("queued-%d", i), Kind: domain.ProbeKindGeo}); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- s.SubmitWithContext(ctx, Task{RunID: "cancelled", LogicalID: "cancel-me", Kind: domain.ProbeKindGeo})
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, ok := s.activeNodes[makeNodeKey("cancel-me", domain.ProbeKindGeo)]; ok {
		t.Fatal("cancelled reservation was not rolled back")
	}

	close(release)
	if err := s.SubmitWithContext(context.Background(), Task{RunID: "recovered", LogicalID: "after-release", Kind: domain.ProbeKindGeo}); err != nil {
		t.Fatalf("capacity did not recover: %v", err)
	}
	s.Wait()
	if s.PeakActiveCount() > 10 || s.PeakQueuedCount() > 11 {
		t.Fatalf("bounds exceeded: active=%d queued=%d", s.PeakActiveCount(), s.PeakQueuedCount())
	}
}

func TestSubmitWithContextPreCancelledNeverEnqueues(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, QueueSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = s.SubmitWithContext(ctx, Task{LogicalID: "pre-cancelled", Kind: domain.ProbeKindGeo})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if s.QueuedCount() != 0 {
		t.Fatalf("unexpected queued task: %d", s.QueuedCount())
	}
}

func TestSubmitWithContextReturnsOnCloseWhileCapacityFull(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, RunConcurrency: 10, QueueSize: 11})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	started := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		if err := s.Submit(Task{LogicalID: fmt.Sprintf("close-active-%d", i), Kind: domain.ProbeKindGeo, Execute: func(context.Context) error { started <- struct{}{}; <-release; return nil }}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		<-started
	}
	for i := 0; i < 11; i++ {
		if err := s.Submit(Task{LogicalID: fmt.Sprintf("close-queued-%d", i), Kind: domain.ProbeKindGeo}); err != nil {
			t.Fatal(err)
		}
	}
	result := make(chan error, 1)
	go func() {
		result <- s.SubmitWithContext(context.Background(), Task{LogicalID: "close-waiter", Kind: domain.ProbeKindGeo})
	}()
	time.Sleep(10 * time.Millisecond)
	closeDone := make(chan error, 1)
	go func() { closeDone <- s.Close() }()
	select {
	case err := <-result:
		if !errors.Is(err, ErrSchedulerClosed) {
			t.Fatalf("expected closed, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("submit did not return on close")
	}
	close(release)
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close deadlocked")
	}
	s.Wait()
}

func TestSchedulerPropagatesTaskCancellation(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	if err := s.Submit(Task{
		RunID:     "run-cancel",
		LogicalID: "node-c",
		Kind:      domain.ProbeKindAI,
		Context:   ctx,
		Execute: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
		OnComplete: func(err error) {
			done <- err
		},
	}); err != nil {
		t.Fatal(err)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	s.Wait()
}

func TestSchedulerPropagatesRunCancellation(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	started := make(chan struct{})
	done := make(chan error, 1)

	if err := s.Submit(Task{
		RunID:     "run-bulk",
		LogicalID: "node-active",
		Kind:      domain.ProbeKindStreaming,
		Execute: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
		OnComplete: func(err error) {
			done <- err
		},
	}); err != nil {
		t.Fatal(err)
	}

	<-started
	s.CancelRun("run-bulk")

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from CancelRun, got %v", err)
	}
	s.Wait()
}

func TestSchedulerConfigRejectsUnsafeConcurrency(t *testing.T) {
	if _, err := NewScheduler(Config{Concurrency: 9}); !errors.Is(err, ErrInvalidConcurrency) {
		t.Fatalf("expected invalid lower bound, got %v", err)
	}
	if _, err := NewScheduler(Config{Concurrency: 33}); !errors.Is(err, ErrInvalidConcurrency) {
		t.Fatalf("expected invalid upper bound, got %v", err)
	}
	valid, err := NewScheduler(Config{Concurrency: 16})
	if err != nil {
		t.Fatalf("expected 16 to be valid, got %v", err)
	}
	_ = valid.Close()
}

func TestSchedulerFakeClockNodeTTL(t *testing.T) {
	var clockMu sync.Mutex
	currentTime := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	fakeClock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return currentTime
	}

	s, err := NewScheduler(Config{
		Concurrency: 10,
		NodeTTL:     5 * time.Minute,
		Clock:       fakeClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	executed := make(chan struct{})
	task := Task{
		RunID:     "run-ttl",
		LogicalID: "node-time",
		Kind:      domain.ProbeKindSpeed,
		Execute: func(context.Context) error {
			close(executed)
			return nil
		},
	}

	if err := s.Submit(task); err != nil {
		t.Fatal(err)
	}
	<-executed
	s.Wait()

	// Immediately re-submitting with same clock should conflict
	err = s.Submit(Task{
		RunID:     "run-ttl-2",
		LogicalID: "node-time",
		Kind:      domain.ProbeKindSpeed,
		Execute:   func(context.Context) error { return nil },
	})
	if !errors.Is(err, ErrNodeConflict) {
		t.Fatalf("expected ErrNodeConflict during TTL, got %v", err)
	}

	// Advance clock by 6 minutes (past 5 minute TTL)
	clockMu.Lock()
	currentTime = currentTime.Add(6 * time.Minute)
	clockMu.Unlock()

	// Now re-submitting should succeed
	err = s.Submit(Task{
		RunID:     "run-ttl-3",
		LogicalID: "node-time",
		Kind:      domain.ProbeKindSpeed,
		Execute:   func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatalf("expected submission to succeed after TTL expiration, got %v", err)
	}
	s.Wait()
}

func TestSchedulerZeroGoroutineLeak(t *testing.T) {
	beforeGoroutines := runtime.NumGoroutine()

	s, err := NewScheduler(Config{Concurrency: 10, QueueSize: 10})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		nodeID := fmt.Sprintf("leak-node-%d", i)
		_ = s.Submit(Task{
			RunID:     "leak-run",
			LogicalID: nodeID,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				time.Sleep(2 * time.Millisecond)
				return nil
			},
		})
	}

	s.Wait()
	if err := s.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Wait briefly for runtime goroutines to settle
	time.Sleep(10 * time.Millisecond)
	afterGoroutines := runtime.NumGoroutine()

	// Goroutine count must return to baseline (allowing at most 1 for background GC/runtime jitter)
	if afterGoroutines > beforeGoroutines+1 {
		t.Fatalf("goroutine leak detected: before=%d, after=%d", beforeGoroutines, afterGoroutines)
	}
}
