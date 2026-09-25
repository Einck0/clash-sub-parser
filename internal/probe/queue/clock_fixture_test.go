package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

// deterministicClock provides thread-safe repeatable time progression for queue tests.
type deterministicClock struct {
	mu  sync.Mutex
	now time.Time
}

func newDeterministicClock(start time.Time) *deterministicClock {
	return &deterministicClock{now: start}
}

func (c *deterministicClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *deterministicClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return c.now
}

func (c *deterministicClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// TestDeterministicRepeatableClockNodeTTL verifies that after s.Wait() returns,
// the node is strictly linearized into TTL state with zero race or defer latency,
// and expires deterministically upon clock advancement.
func TestDeterministicRepeatableClockNodeTTL(t *testing.T) {
	baseTime := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	clock := newDeterministicClock(baseTime)

	s, err := NewScheduler(Config{
		Concurrency:    10,
		RunConcurrency: 5,
		NodeTTL:        3 * time.Minute,
		Clock:          clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Drain(context.Background()) }()

	const generations = 10
	for gen := 0; gen < generations; gen++ {
		nodeID := fmt.Sprintf("repeatable-node-%d", gen%3)
		var executed atomic.Bool

		err := s.Submit(Task{
			RunID:     fmt.Sprintf("run-gen-%d", gen),
			LogicalID: nodeID,
			Kind:      domain.ProbeKindSpeed,
			Execute: func(context.Context) error {
				executed.Store(true)
				return nil
			},
		})
		if err != nil {
			t.Fatalf("generation %d: submit failed: %v", gen, err)
		}

		// Wait must strictly guarantee execution is complete and node TTL is recorded.
		s.Wait()
		if !executed.Load() {
			t.Fatalf("generation %d: task not executed after Wait", gen)
		}

		// Immediate re-submit with same clock MUST return ErrNodeConflict deterministically.
		err = s.Submit(Task{
			RunID:     fmt.Sprintf("run-conflict-%d", gen),
			LogicalID: nodeID,
			Kind:      domain.ProbeKindSpeed,
			Execute:   func(context.Context) error { return nil },
		})
		if !errors.Is(err, ErrNodeConflict) {
			t.Fatalf("generation %d: expected ErrNodeConflict before clock advance, got %v", gen, err)
		}

		// Advancing clock past NodeTTL (3 min) MUST allow subsequent submission.
		clock.Advance(4 * time.Minute)

		err = s.Submit(Task{
			RunID:     fmt.Sprintf("run-post-ttl-%d", gen),
			LogicalID: nodeID,
			Kind:      domain.ProbeKindSpeed,
			Execute:   func(context.Context) error { return nil },
		})
		if err != nil {
			t.Fatalf("generation %d: expected success after TTL expiration, got %v", gen, err)
		}
		s.Wait()
	}
}

// TestDeterministicClockFairnessUnderPerRunLimit verifies that multiple runs
// share execution slots fairly according to RunConcurrency limits under deterministic timing.
func TestDeterministicClockFairnessUnderPerRunLimit(t *testing.T) {
	clock := newDeterministicClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	s, err := NewScheduler(Config{
		Concurrency:    10,
		RunConcurrency: 2,
		QueueSize:      64,
		Clock:          clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Drain(context.Background()) }()

	var runAProgress, runBProgress atomic.Int32
	var peakA, peakB atomic.Int32
	var currentA, currentB atomic.Int32

	blockA := make(chan struct{})

	// Submit 8 tasks for run A
	for i := 0; i < 8; i++ {
		taskID := fmt.Sprintf("node-a-%d", i)
		err := s.Submit(Task{
			RunID:     "run-A",
			LogicalID: taskID,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				c := currentA.Add(1)
				for prev := peakA.Load(); c > prev && !peakA.CompareAndSwap(prev, c); prev = peakA.Load() {
				}
				<-blockA
				currentA.Add(-1)
				runAProgress.Add(1)
				return nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Submit 4 tasks for run B
	runBStarted := make(chan struct{}, 4)
	for i := 0; i < 4; i++ {
		taskID := fmt.Sprintf("node-b-%d", i)
		err := s.Submit(Task{
			RunID:     "run-B",
			LogicalID: taskID,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				c := currentB.Add(1)
				for prev := peakB.Load(); c > prev && !peakB.CompareAndSwap(prev, c); prev = peakB.Load() {
				}
				defer currentB.Add(-1)
				runBProgress.Add(1)
				runBStarted <- struct{}{}
				return nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Run B should make progress even though Run A is blocked, proving fairness and per-run limits.
	for i := 0; i < 2; i++ {
		select {
		case <-runBStarted:
		case <-time.After(2 * time.Second):
			t.Fatal("run B starved behind blocked run A")
		}
	}

	if p := peakA.Load(); p > 2 {
		t.Fatalf("run A peak concurrency exceeded run limit: %d > 2", p)
	}

	close(blockA)
	s.Wait()

	if pA := runAProgress.Load(); pA != 8 {
		t.Fatalf("run A progress = %d, want 8", pA)
	}
	if pB := runBProgress.Load(); pB != 4 {
		t.Fatalf("run B progress = %d, want 4", pB)
	}
}

// TestRepeatedCloseAndDrainIdempotence verifies that repeated sequential and
// concurrent Close and Drain calls succeed idempotently without deadlocks or panics.
func TestRepeatedCloseAndDrainIdempotence(t *testing.T) {
	clock := newDeterministicClock(time.Date(2026, 9, 24, 14, 0, 0, 0, time.UTC))
	s, err := NewScheduler(Config{
		Concurrency: 10,
		QueueSize:   32,
		Clock:       clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}

	var completed atomic.Int32
	for i := 0; i < 5; i++ {
		err := s.Submit(Task{
			RunID:     "run-idemp",
			LogicalID: fmt.Sprintf("node-%d", i),
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				return nil
			},
			OnComplete: func(error) {
				completed.Add(1)
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	s.Wait()

	// 1. Sequential repeated Close calls must return nil
	for i := 0; i < 5; i++ {
		if err := s.Close(); err != nil {
			t.Fatalf("sequential close %d failed: %v", i, err)
		}
	}

	// 2. Sequential repeated Drain calls must return nil
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := s.Drain(ctx); err != nil {
			t.Fatalf("sequential drain %d failed: %v", i, err)
		}
	}

	// 3. Concurrent Close and Drain calls
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := s.Close(); err != nil {
				t.Errorf("concurrent close failed: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := s.Drain(ctx); err != nil {
				t.Errorf("concurrent drain failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if c := completed.Load(); c != 5 {
		t.Fatalf("completed callbacks = %d, want 5", c)
	}
}

// TestCallbackReenteringWaitAndCloseWithDeterministicClock ensures callbacks
// reentering Wait or Close never deadlock and cleanly complete.
func TestCallbackReenteringWaitAndCloseWithDeterministicClock(t *testing.T) {
	clock := newDeterministicClock(time.Date(2026, 9, 24, 16, 0, 0, 0, time.UTC))
	s, err := NewScheduler(Config{
		Concurrency: 10,
		QueueSize:   16,
		Clock:       clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	err = s.Submit(Task{
		RunID:     "reenter-run",
		LogicalID: "reenter-node",
		Kind:      domain.ProbeKindBaseline,
		Execute: func(context.Context) error {
			return nil
		},
		OnComplete: func(error) {
			s.Wait()
			_ = s.Close()
			close(done)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("callback deadlocked reentering Wait/Close")
	}

	if err := s.Drain(context.Background()); err != nil {
		t.Fatalf("drain failed: %v", err)
	}
}
