package queue

import (
	"context"
	"fmt"
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
