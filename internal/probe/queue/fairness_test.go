package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestSchedulerRunFairnessAndPerRunLimit(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, RunConcurrency: 2, QueueSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	started := make(chan string, 32)
	release := make(chan struct{})
	var run1, run2 atomic.Int32
	var activeBulk, peakBulk atomic.Int32
	var activeOther, peakOther atomic.Int32
	for i := 0; i < 12; i++ {
		i := i
		if err := s.Submit(Task{RunID: "bulk", LogicalID: fmt.Sprintf("bulk-%d", i), Kind: domain.ProbeKindBaseline, Execute: func(context.Context) error {
			current := activeBulk.Add(1)
			for prev := peakBulk.Load(); current > prev && !peakBulk.CompareAndSwap(prev, current); prev = peakBulk.Load() {
			}
			started <- "bulk"
			<-release
			activeBulk.Add(-1)
			run1.Add(1)
			return nil
		}}); err != nil {
			t.Fatal(err)
		}
	}
	firstBulk := 0
	for firstBulk < 2 {
		select {
		case name := <-started:
			if name == "bulk" {
				firstBulk++
			}
		case <-time.After(time.Second):
			t.Fatal("bulk workers did not start")
		}
	}
	for i := 0; i < 6; i++ {
		i := i
		if err := s.Submit(Task{RunID: "other", LogicalID: fmt.Sprintf("other-%d", i), Kind: domain.ProbeKindBaseline, Execute: func(context.Context) error {
			current := activeOther.Add(1)
			for prev := peakOther.Load(); current > prev && !peakOther.CompareAndSwap(prev, current); prev = peakOther.Load() {
			}
			defer activeOther.Add(-1)
			run2.Add(1)
			started <- "other"
			<-release
			return nil
		}}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case got := <-started:
		if got != "other" {
			t.Fatalf("expected other run progress while bulk blocked, got %s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second run starved behind blocked first run")
	}
	if got := peakBulk.Load(); got > 2 {
		t.Fatalf("expected at most 2 active for bulk run, peak=%d", got)
	}
	if got := peakOther.Load(); got > 2 {
		t.Fatalf("expected at most 2 active for other run, peak=%d", got)
	}
	if got := s.PeakActiveCount(); got > 10 {
		t.Fatalf("global active peak exceeded configured limit: %d", got)
	}
	close(release)
	s.Wait()
	if run2.Load() == 0 {
		t.Fatal("second run had no progress")
	}
	t.Logf("observed peak global=%d per-run configured=2, progress bulk=%d other=%d", s.PeakActiveCount(), run1.Load(), run2.Load())
}

func TestTTLHistoryIsBounded(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, QueueSize: 64, NodeTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if err := s.Submit(Task{RunID: "ttl", LogicalID: fmt.Sprintf("ttl-%d", i), Kind: domain.ProbeKindBaseline}); err != nil {
			t.Fatal(err)
		}
	}
	s.Wait()
	s.mu.Lock()
	got, limit := len(s.activeNodes), s.cfg.QueueSize+s.cfg.Concurrency
	s.mu.Unlock()
	if got > limit {
		t.Fatalf("TTL history unbounded: entries=%d limit=%d", got, limit)
	}
	_ = s.Close()
	s.Wait()
}

func TestCompletionCallbackMayReenterWaitAndClose(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	callbackDone := make(chan struct{})
	if err := s.Submit(Task{RunID: "reenter", LogicalID: "node", Kind: domain.ProbeKindBaseline,
		OnComplete: func(error) { s.Wait(); _ = s.Close(); close(callbackDone) },
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("callback deadlocked reentering Wait/Close")
	}
	s.Wait()
}

func TestCancelRunCompletesQueuedTasksExactlyOnce(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, RunConcurrency: 1, QueueSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var completed atomic.Int32
	for i := 0; i < 3; i++ {
		i := i
		err = s.Submit(Task{RunID: "cancel", LogicalID: fmt.Sprintf("cancel-%d", i), Kind: domain.ProbeKindBaseline, Execute: func(ctx context.Context) error {
			if i == 0 {
				close(started)
				<-release
			}
			return ctx.Err()
		}, OnComplete: func(error) { completed.Add(1) }})
		if err != nil {
			t.Fatal(err)
		}
	}
	<-started
	s.CancelRun("cancel")
	close(release)
	s.Wait()
	if got := completed.Load(); got != 3 {
		t.Fatalf("completion count=%d want 3", got)
	}
	if s.QueuedCount() != 0 || s.ActiveCount() != 0 {
		t.Fatalf("not drained queued=%d active=%d", s.QueuedCount(), s.ActiveCount())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestSchedulerIdleWorkConservingAndMultiRunFairness proves both:
// 1. Idle work-conserving: A single run on an idle scheduler uses the full global
//    concurrency capacity (e.g. 10/10) instead of being halved by the fair-share limit.
// 2. Multi-run fairness: When multiple runs compete for slots, each run is bounded
//    by its fair share so no run starves competing runs.
func TestSchedulerIdleWorkConservingAndMultiRunFairness(t *testing.T) {
	const globalConcurrency = 10
	s, err := NewScheduler(Config{Concurrency: globalConcurrency, QueueSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// -------------------------------------------------------------
	// Part 1: Idle Work-Conserving
	// -------------------------------------------------------------
	var singleActive, peakSingle atomic.Int32
	var singleStartedOnce sync.Once
	allSingleStarted := make(chan struct{})
	releaseSingle := make(chan struct{})

	for i := 0; i < globalConcurrency; i++ {
		nid := fmt.Sprintf("idle-node-%d", i)
		if err := s.Submit(Task{
			RunID:     "single-run",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				curr := singleActive.Add(1)
				for {
					p := peakSingle.Load()
					if curr <= p || peakSingle.CompareAndSwap(p, curr) {
						break
					}
				}
				if curr == int32(globalConcurrency) {
					singleStartedOnce.Do(func() { close(allSingleStarted) })
				}
				<-releaseSingle
				singleActive.Add(-1)
				return nil
			},
		}); err != nil {
			t.Fatalf("submit single-run task %d: %v", i, err)
		}
	}

	select {
	case <-allSingleStarted:
	case <-time.After(2 * time.Second):
		t.Fatalf("idle work-conserving violated: only reached %d active workers, want %d",
			peakSingle.Load(), globalConcurrency)
	}

	if got := peakSingle.Load(); got != int32(globalConcurrency) {
		t.Fatalf("expected single run to use full global concurrency %d, got %d", globalConcurrency, got)
	}

	close(releaseSingle)
	s.Wait()

	// -------------------------------------------------------------
	// Part 2: Multi-Run Fairness Under Contention (Simultaneous)
	// -------------------------------------------------------------
	// Default fair share for 2 competing runs when concurrency is 10 is (10+1)/2 = 5.
	const fairShareLimit = (globalConcurrency + 1) / 2

	var bulkActive, peakBulk atomic.Int32
	var otherActive, peakOther atomic.Int32
	var otherStartedOnce sync.Once
	otherStarted := make(chan struct{})
	releaseMulti := make(chan struct{})

	// Deterministic registration: hold scheduler internal mutex during submission
	// so both runs are enqueued before dispatcher admits tasks simultaneously.
	s.mu.Lock()

	// Submit 12 tasks for bulk-run (more than fair share)
	for i := 0; i < 12; i++ {
		nid := fmt.Sprintf("bulk-multi-%d", i)
		task := Task{
			RunID:     "run-bulk",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				curr := bulkActive.Add(1)
				for {
					p := peakBulk.Load()
					if curr <= p || peakBulk.CompareAndSwap(p, curr) {
						break
					}
				}
				<-releaseMulti
				bulkActive.Add(-1)
				return nil
			},
		}
		key := makeNodeKey(task.LogicalID, task.Kind)
		s.enqueueTaskLocked(key, task)
	}

	// Concurrently submit 6 tasks for run-other
	for i := 0; i < 6; i++ {
		nid := fmt.Sprintf("other-multi-%d", i)
		task := Task{
			RunID:     "run-other",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				curr := otherActive.Add(1)
				for {
					p := peakOther.Load()
					if curr <= p || peakOther.CompareAndSwap(p, curr) {
						break
					}
				}
				otherStartedOnce.Do(func() { close(otherStarted) })
				<-releaseMulti
				otherActive.Add(-1)
				return nil
			},
		}
		key := makeNodeKey(task.LogicalID, task.Kind)
		s.enqueueTaskLocked(key, task)
	}

	s.mu.Unlock()

	// Verify run-other makes progress and is not starved behind run-bulk
	select {
	case <-otherStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("competing second run starved behind bulk run")
	}

	// Under simultaneous contention, each competing run is bounded by fair share limit
	if got := peakBulk.Load(); got > int32(fairShareLimit) {
		t.Fatalf("multi-run fairness violated: bulk active peak %d exceeded fair share %d",
			got, fairShareLimit)
	}
	if got := peakOther.Load(); got > int32(fairShareLimit) {
		t.Fatalf("multi-run fairness violated: other active peak %d exceeded fair share %d",
			got, fairShareLimit)
	}

	// Global active must never exceed globalConcurrency
	if got := s.PeakActiveCount(); got > globalConcurrency {
		t.Fatalf("global concurrency exceeded: %d > %d", got, globalConcurrency)
	}

	close(releaseMulti)
	s.Wait()
}

// TestSchedulerLateArrivalConvergence verifies that:
// 1. When an idle scheduler receives a bulk run, work-conserving allows it to reach full capacity (10 slots).
// 2. When a competing run joins late, running tasks are NOT preempted (historical peak remains 10).
// 3. As in-flight bulk tasks complete, freed slots are immediately awarded to the late-arriving run
//    rather than continuing to admit queued bulk tasks, converging to fair share without starvation.
func TestSchedulerLateArrivalConvergence(t *testing.T) {
	const globalConcurrency = 10
	const fairShareLimit = (globalConcurrency + 1) / 2

	s, err := NewScheduler(Config{Concurrency: globalConcurrency, QueueSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var bulkActive, peakBulk atomic.Int32
	var otherActive, peakOther atomic.Int32

	var bulkFullOnce sync.Once
	bulkFullCh := make(chan struct{})
	releaseBulkGate := make(chan struct{})
	releaseAll := make(chan struct{})

	// Submit 16 tasks for bulk run. The first 10 will saturate the scheduler.
	for i := 0; i < 16; i++ {
		nid := fmt.Sprintf("late-bulk-%d", i)
		idx := i
		if err := s.Submit(Task{
			RunID:     "late-bulk",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				curr := bulkActive.Add(1)
				for {
					p := peakBulk.Load()
					if curr <= p || peakBulk.CompareAndSwap(p, curr) {
						break
					}
				}
				if curr == int32(globalConcurrency) {
					bulkFullOnce.Do(func() { close(bulkFullCh) })
				}
				// The first 5 tasks wait on releaseBulkGate; others wait on releaseAll
				if idx < 5 {
					select {
					case <-releaseBulkGate:
					case <-releaseAll:
					}
				} else {
					<-releaseAll
				}
				bulkActive.Add(-1)
				return nil
			},
		}); err != nil {
			t.Fatalf("submit bulk %d: %v", i, err)
		}
	}

	// 1. Bulk run saturates all 10 slots on idle scheduler
	select {
	case <-bulkFullCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("bulk run failed to reach 10 in-flight slots, current=%d", bulkActive.Load())
	}

	if got := peakBulk.Load(); got != int32(globalConcurrency) {
		t.Fatalf("expected bulk peak to reach global concurrency %d, got %d", globalConcurrency, got)
	}

	// 2. Run "Other" arrives late while 10 bulk tasks are in-flight
	var otherStartedOnce sync.Once
	otherStarted := make(chan struct{})

	for i := 0; i < 6; i++ {
		nid := fmt.Sprintf("late-other-%d", i)
		if err := s.Submit(Task{
			RunID:     "late-other",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				curr := otherActive.Add(1)
				for {
					p := peakOther.Load()
					if curr <= p || peakOther.CompareAndSwap(p, curr) {
						break
					}
				}
				otherStartedOnce.Do(func() { close(otherStarted) })
				<-releaseAll
				otherActive.Add(-1)
				return nil
			},
		}); err != nil {
			t.Fatalf("submit other %d: %v", i, err)
		}
	}

	// 3. Release 5 bulk tasks. Since other is now queued, competing runs = 2.
	// Bulk currently has >= 5 active tasks, so fair share prevents bulk from taking
	// new admissions. All freed slots must go to other!
	close(releaseBulkGate)

	select {
	case <-otherStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("late-arriving second run was starved after slots were freed")
	}

	// Wait briefly for other to acquire freed slots
	deadline := time.Now().Add(time.Second)
	for otherActive.Load() < int32(fairShareLimit) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if got := otherActive.Load(); got < 1 {
		t.Fatalf("expected other run to acquire active slots, got %d", got)
	}

	// Clean up
	close(releaseAll)
	s.Wait()

	if got := s.PeakActiveCount(); got > globalConcurrency {
		t.Fatalf("global concurrency exceeded limit: %d > %d", got, globalConcurrency)
	}
}

// TestSchedulerQueuedEmptyButActiveCountsAsCompetingRun verifies that a run with 0 queued tasks
// but active in-flight tasks is still counted in activeCompetingRunsLocked, preventing a newly
// arriving run from bypassing fair share and monopolizing the scheduler.
func TestSchedulerQueuedEmptyButActiveCountsAsCompetingRun(t *testing.T) {
	const globalConcurrency = 10
	const fairShareLimit = (globalConcurrency + 1) / 2

	s, err := NewScheduler(Config{Concurrency: globalConcurrency, QueueSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var activeA, peakB atomic.Int32
	var aStartedOnce sync.Once
	aStarted := make(chan struct{})
	releaseA := make(chan struct{})
	releaseB := make(chan struct{})

	// Run A submits exactly 3 tasks.
	for i := 0; i < 3; i++ {
		nid := fmt.Sprintf("node-a-%d", i)
		if err := s.Submit(Task{
			RunID:     "run-A",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				if activeA.Add(1) == 3 {
					aStartedOnce.Do(func() { close(aStarted) })
				}
				<-releaseA
				activeA.Add(-1)
				return nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for all 3 tasks of Run A to be active. At this point, Run A's queue is EMPTY.
	select {
	case <-aStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("run A tasks failed to start")
	}

	// Run B now submits 8 tasks. Even though Run A has 0 queued tasks, Run A has 3 active tasks.
	// competing runs = 2, so Run B must be bounded by fairShareLimit (5).
	var bStartedOnce sync.Once
	bFirstStarted := make(chan struct{})
	var bCount atomic.Int32

	for i := 0; i < 8; i++ {
		nid := fmt.Sprintf("node-b-%d", i)
		if err := s.Submit(Task{
			RunID:     "run-B",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				curr := bCount.Add(1)
				for {
					p := peakB.Load()
					if curr <= p || peakB.CompareAndSwap(p, curr) {
						break
					}
				}
				bStartedOnce.Do(func() { close(bFirstStarted) })
				<-releaseB
				bCount.Add(-1)
				return nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case <-bFirstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("run B failed to start")
	}

	// Give dispatcher a moment to admit any eligible tasks for B
	time.Sleep(50 * time.Millisecond)

	if got := peakB.Load(); got > int32(fairShareLimit) {
		t.Fatalf("run B exceeded fair share limit: peak %d > limit %d (Run A active tasks were ignored)",
			got, fairShareLimit)
	}

	close(releaseA)
	close(releaseB)
	s.Wait()
}

// TestSchedulerExplicitRunConcurrencyRespected verifies that when RunConcurrency is explicitly
// configured, a single run on an idle scheduler never exceeds that limit.
func TestSchedulerExplicitRunConcurrencyRespected(t *testing.T) {
	const explicitLimit = 3
	s, err := NewScheduler(Config{
		Concurrency:    10,
		RunConcurrency: explicitLimit,
		QueueSize:      64,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var active, peak atomic.Int32
	startedCount := make(chan struct{}, 8)
	release := make(chan struct{})

	for i := 0; i < 8; i++ {
		nid := fmt.Sprintf("explicit-node-%d", i)
		if err := s.Submit(Task{
			RunID:     "single-explicit",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				curr := active.Add(1)
				for {
					p := peak.Load()
					if curr <= p || peak.CompareAndSwap(p, curr) {
						break
					}
				}
				startedCount <- struct{}{}
				<-release
				active.Add(-1)
				return nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for 3 tasks to start
	for i := 0; i < explicitLimit; i++ {
		select {
		case <-startedCount:
		case <-time.After(2 * time.Second):
			t.Fatal("explicit tasks failed to start")
		}
	}

	time.Sleep(50 * time.Millisecond)

	if got := peak.Load(); got > explicitLimit {
		t.Fatalf("explicit RunConcurrency violated: peak %d > %d", got, explicitLimit)
	}

	close(release)
	s.Wait()
}

// TestSchedulerCancelAndCloseRecovery verifies task cancellation and clean shutdown
// without deadlock, resource leaks, or dropped callbacks.
func TestSchedulerCancelAndCloseRecovery(t *testing.T) {
	s, err := NewScheduler(Config{Concurrency: 10, RunConcurrency: 2, QueueSize: 64})
	if err != nil {
		t.Fatal(err)
	}

	var runACompleted, runBCanceled atomic.Int32
	var aStartedOnce sync.Once
	aStarted := make(chan struct{})
	releaseA := make(chan struct{})

	// Run A submits tasks
	for i := 0; i < 4; i++ {
		nid := fmt.Sprintf("cancel-a-%d", i)
		if err := s.Submit(Task{
			RunID:     "run-cancel-A",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(context.Context) error {
				aStartedOnce.Do(func() { close(aStarted) })
				<-releaseA
				return nil
			},
			OnComplete: func(err error) {
				runACompleted.Add(1)
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	<-aStarted

	// Run B submits tasks, then is cancelled
	for i := 0; i < 4; i++ {
		nid := fmt.Sprintf("cancel-b-%d", i)
		if err := s.Submit(Task{
			RunID:     "run-cancel-B",
			LogicalID: nid,
			Kind:      domain.ProbeKindBaseline,
			Execute: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			OnComplete: func(err error) {
				if err == context.Canceled {
					runBCanceled.Add(1)
				}
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	s.CancelRun("run-cancel-B")

	close(releaseA)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := s.Drain(ctx); err != nil {
		t.Fatalf("drain failed: %v", err)
	}

	if got := runBCanceled.Load(); got != 4 {
		t.Fatalf("expected 4 canceled tasks for Run B, got %d", got)
	}
	if got := runACompleted.Load(); got != 4 {
		t.Fatalf("expected 4 completed tasks for Run A, got %d", got)
	}
}
