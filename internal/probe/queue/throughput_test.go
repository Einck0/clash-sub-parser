package queue

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

// TestSchedulerThroughputFixture exercises bounded submission, completion, and
// shutdown using only in-process no-op tasks; it never performs network I/O.
func TestSchedulerThroughputFixture(t *testing.T) {
	const taskCount = 2000
	const concurrency = 16
	const queueSize = 64

	s, err := NewScheduler(Config{Concurrency: concurrency, QueueSize: queueSize})
	if err != nil {
		t.Fatal(err)
	}
	var completed atomic.Int64
	start := time.Now()
	for i := 0; i < taskCount; i++ {
		i := i
		err := s.SubmitWithContext(context.Background(), Task{
			RunID: "fixture", LogicalID: fmt.Sprintf("fixture-%d", i), Kind: domain.ProbeKindBaseline,
			Execute:    func(context.Context) error { return nil },
			OnComplete: func(error) { completed.Add(1) },
		})
		if err != nil {
			t.Fatalf("submit task %d: %v", i, err)
		}
	}
	s.Wait()
	elapsed := time.Since(start)
	peakActive, peakQueued := s.PeakActiveCount(), s.PeakQueuedCount()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Drain(ctx); err != nil {
		t.Fatalf("drain scheduler: %v", err)
	}
	if got := completed.Load(); got != taskCount {
		t.Fatalf("completed=%d, want %d", got, taskCount)
	}
	if peakActive < 1 || peakActive > concurrency || peakQueued > queueSize {
		t.Fatalf("observed bounds violated: peak_active=%d (cap %d), peak_queued=%d (cap %d)", peakActive, concurrency, peakQueued, queueSize)
	}
	t.Logf("fixture tasks=%d elapsed=%s tasks_per_second=%.1f peak_active=%d peak_queued=%d capacity_rejections=0 concurrency=%d queue_capacity=%d", taskCount, elapsed, float64(taskCount)/elapsed.Seconds(), peakActive, peakQueued, concurrency, queueSize)
}

// BenchmarkSchedulerThroughput reports scheduler throughput and observed
// high-water marks for a fixed, network-free no-op task workload.
func BenchmarkSchedulerThroughput(b *testing.B) {
	const concurrency = 16
	const queueSize = 64
	s, err := NewScheduler(Config{Concurrency: concurrency, QueueSize: queueSize})
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	var rejected atomic.Int64
	start := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := s.SubmitWithContext(context.Background(), Task{
			RunID: "benchmark", LogicalID: fmt.Sprintf("bench-%d", i), Kind: domain.ProbeKindBaseline,
			Execute: func(context.Context) error { return nil },
		})
		if err != nil {
			rejected.Add(1)
			b.Fatalf("submit task %d: %v", i, err)
		}
	}
	s.Wait()
	b.StopTimer()
	elapsed := time.Since(start)
	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "tasks/s")
	b.ReportMetric(float64(s.PeakActiveCount()), "peak-active")
	b.ReportMetric(float64(s.PeakQueuedCount()), "peak-queued")
	b.ReportMetric(float64(rejected.Load()), "capacity-rejections")
	b.ReportMetric(float64(concurrency), "worker-cap")
	b.ReportMetric(float64(queueSize), "queue-cap")
}
