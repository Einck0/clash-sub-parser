package queue

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func newLifecycleScheduler(t *testing.T) *Scheduler {
	t.Helper()
	s, err := NewScheduler(Config{Concurrency: 10, RunConcurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Drain(context.Background()) })
	return s
}

func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("barrier timed out")
	}
}

func TestWaitDoesNotWaitForCallbackButDrainDoes(t *testing.T) {
	s := newLifecycleScheduler(t)
	entered, release := make(chan struct{}), make(chan struct{})
	if err := s.Submit(Task{RunID: "r", LogicalID: "n", Kind: domain.ProbeKindBaseline, Execute: func(context.Context) error { return nil }, OnComplete: func(error) { close(entered); <-release }}); err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	waited := make(chan struct{})
	go func() { s.Wait(); close(waited) }()
	await(t, waited)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	drained := make(chan error, 1)
	go func() { drained <- s.Drain(ctx) }()
	select {
	case err := <-drained:
		t.Fatalf("drain returned early: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedCloseIsIdempotentAndDrainCompletesPendingCallbacks(t *testing.T) {
	s := newLifecycleScheduler(t)
	started, release := make(chan struct{}), make(chan struct{})
	var callbacks atomic.Int32
	if err := s.Submit(Task{RunID: "close-race", LogicalID: "active", Execute: func(context.Context) error { close(started); <-release; return nil }, OnComplete: func(error) { callbacks.Add(1) }}); err != nil {
		t.Fatal(err)
	}
	await(t, started)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if got := callbacks.Load(); got != 1 {
		t.Fatalf("callbacks=%d want 1", got)
	}
}

func TestSubmitRejectedAfterClose(t *testing.T) {
	s := newLifecycleScheduler(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Submit(Task{LogicalID: "late"}); !errors.Is(err, ErrSchedulerClosed) {
		t.Fatalf("Submit error=%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDrainDeadlineWithUncooperativeExecute(t *testing.T) {
	s := newLifecycleScheduler(t)
	started, release := make(chan struct{}), make(chan struct{})
	if err := s.Submit(Task{LogicalID: "stuck", Execute: func(context.Context) error { close(started); <-release; return nil }}); err != nil {
		t.Fatal(err)
	}
	await(t, started)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Drain(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain error=%v", err)
	}
	if err := s.Submit(Task{LogicalID: "after-close"}); !errors.Is(err, ErrSchedulerClosed) {
		t.Fatalf("Submit error=%v", err)
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := s.Drain(ctx2); err != nil {
		t.Fatal(err)
	}
}

func TestReentrantWaitAndClose(t *testing.T) {
	s := newLifecycleScheduler(t)
	done := make(chan struct{})
	if err := s.Submit(Task{LogicalID: "reentrant", Execute: func(context.Context) error { _ = s.Close(); return nil }, OnComplete: func(error) { _ = s.Close(); s.Wait(); close(done) }}); err != nil {
		t.Fatal(err)
	}
	await(t, done)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCancelRunCloseRaceCompletesEachAcceptedTaskOnce(t *testing.T) {
	s := newLifecycleScheduler(t)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var callbacks atomic.Int32
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("race-%d", i)
		if err := s.Submit(Task{RunID: "r", LogicalID: id, Execute: func(context.Context) error { started <- struct{}{}; <-release; return nil }, OnComplete: func(error) { callbacks.Add(1) }}); err != nil {
			t.Fatal(err)
		}
	}
	await(t, started)
	gate := make(chan struct{})
	done := make(chan struct{}, 2)
	go func() { <-gate; s.CancelRun("r"); done <- struct{}{} }()
	go func() { <-gate; _ = s.Close(); done <- struct{}{} }()
	close(gate)
	await(t, done)
	await(t, done)
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if got := callbacks.Load(); got != 2 {
		t.Fatalf("callbacks=%d want 2", got)
	}
}
