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

// TestSchedulerConcurrentSubmitCancelCloseStress uses only controlled in-process
// tasks. Every blocked execution is released by cancellation or the test itself.
func TestSchedulerConcurrentSubmitCancelCloseStress(t *testing.T) {
	const submitters = 24
	const tasksPerSubmitter = 40
	const workers = 10
	const queueCapacity = 16
	const deadline = 5 * time.Second

	baseline := runtime.NumGoroutine()
	s, err := NewScheduler(Config{Concurrency: workers, QueueSize: queueCapacity, NodeTTL: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{}, workers)
	completed := make(chan struct{}, submitters*tasksPerSubmitter)
	var accepted, rejected, cancelled atomic.Int64
	var submitWG sync.WaitGroup
	for submitter := 0; submitter < submitters; submitter++ {
		submitWG.Add(1)
		go func(submitter int) {
			defer submitWG.Done()
			for n := 0; n < tasksPerSubmitter; n++ {
				ctx := context.Background()
				var cancel context.CancelFunc
				if n%4 == 0 {
					ctx, cancel = context.WithCancel(ctx)
					time.AfterFunc(2*time.Millisecond, cancel)
				}
				err := s.SubmitWithContext(ctx, Task{
					RunID: fmt.Sprintf("run-%d", submitter), LogicalID: fmt.Sprintf("node-%d-%d", submitter, n), Kind: domain.ProbeKindBaseline,
					Context: ctx,
					Execute: func(ctx context.Context) error {
						select {
						case started <- struct{}{}:
						default:
						}
						<-ctx.Done()
						return ctx.Err()
					},

					OnComplete: func(err error) {
						if errors.Is(err, context.Canceled) {
							cancelled.Add(1)
						}
						completed <- struct{}{}
					},
				})
				if cancel != nil {
					cancel()
				}
				switch {
				case err == nil:
					accepted.Add(1)
				case errors.Is(err, context.Canceled):
					rejected.Add(1)
				case errors.Is(err, ErrSchedulerClosed):
					rejected.Add(1)
				default:
					t.Errorf("unexpected submit error: %v", err)
					return
				}
			}
		}(submitter)
	}

	// Ensure work is flowing before racing shutdown against remaining submitters.
	select {
	case <-started:
	case <-time.After(deadline):
		t.Fatal("workers did not start")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- s.Close() }()
	submitDone := make(chan struct{})
	go func() { submitWG.Wait(); close(submitDone) }()
	select {
	case <-submitDone:
	case <-time.After(deadline):
		t.Fatal("submitters did not finish")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(deadline):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("Close did not finish; goroutine dump:\n%s", buf[:n])
	}
	drainCtx, drainCancel := context.WithTimeout(context.Background(), deadline)
	defer drainCancel()
	if err := s.Drain(drainCtx); err != nil {
		t.Fatalf("Drain did not observe all accepted callbacks: %v", err)
	}

	// Close completes/cancels each accepted task exactly once, including queued tasks.
	if got, want := int64(len(completed)), accepted.Load(); got != want {
		t.Fatalf("completion count=%d accepted=%d", got, want)
	}
	if accepted.Load()+rejected.Load() != submitters*tasksPerSubmitter {
		t.Fatalf("submissions accounted=%d want=%d", accepted.Load()+rejected.Load(), submitters*tasksPerSubmitter)
	}
	if active := s.ActiveCount(); active != 0 {
		t.Fatalf("active count after Close=%d", active)
	}
	if queued := s.QueuedCount(); queued != 0 {
		t.Fatalf("queued count after Close=%d", queued)
	}
	s.mu.Lock()
	if len(s.activeNodes) != 0 { // completed records are intentionally TTL-retained; only no active reservations are allowed.
		for key, record := range s.activeNodes {
			if record.active {
				s.mu.Unlock()
				t.Fatalf("active key leaked: %s", key)
			}
		}
	}
	if len(s.runCancels) != 0 {
		s.mu.Unlock()
		t.Fatalf("run cancellation entries leaked: %d", len(s.runCancels))
	}
	s.mu.Unlock()
	if cancelled.Load() == 0 {
		t.Fatal("expected cancellation completions during shutdown")
	}
	t.Logf("stress submitters=%d requested=%d accepted=%d rejected_or_cancelled=%d cancelled=%d workers=%d queue_cap=%d", submitters, submitters*tasksPerSubmitter, accepted.Load(), rejected.Load(), cancelled.Load(), workers, queueCapacity)

	// Workers and callback/task goroutines must settle after shutdown.
	until := time.Now().Add(deadline)
	for runtime.NumGoroutine() > baseline+2 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > baseline+2 {
		t.Fatalf("goroutine count did not settle: baseline=%d after=%d", baseline, got)
	}
}
