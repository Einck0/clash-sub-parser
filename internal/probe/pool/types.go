package pool

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrPoolClosed is returned when attempting to submit tasks to a closed pool.
	ErrPoolClosed = errors.New("worker pool is closed")
	// ErrPoolCancelled is returned when the pool or operation was cancelled.
	ErrPoolCancelled = errors.New("worker pool context cancelled")
)

// Task encapsulates an isolated unit of execution.
type Task[T any] struct {
	ID      string
	Fn      func(ctx context.Context) (T, error)
	Timeout time.Duration
}

// Result encapsulates the outcome of a Task execution.
type Result[T any] struct {
	ID       string
	Value    T
	Error    error
	Duration time.Duration
}

// Options configures the sliding window pool and memory guard.
type Options struct {
	// Concurrency is the initial active worker limit (100~500).
	Concurrency int `json:"concurrency"`
	// MinConcurrency is the lowest bound concurrency under dynamic backpressure.
	MinConcurrency int `json:"min_concurrency"`
	// MaxConcurrency is the upper bound concurrency permitted.
	MaxConcurrency int `json:"max_concurrency"`

	// TaskTimeout is the default deadline applied if a Task does not specify its own.
	TaskTimeout time.Duration `json:"task_timeout"`
	// GlobalTimeout is an optional deadline for the entire pool lifetime.
	GlobalTimeout time.Duration `json:"global_timeout"`

	// MemoryLimitBytes specifies the soft memory limit (e.g. 150MB = 150*1024*1024).
	MemoryLimitBytes int64 `json:"memory_limit_bytes"`
	// HighWatermarkRatio is the memory usage ratio that triggers backpressure (e.g. 0.80).
	HighWatermarkRatio float64 `json:"high_watermark_ratio"`
	// LowWatermarkRatio is the memory usage ratio below which backpressure is released (e.g. 0.60).
	LowWatermarkRatio float64 `json:"low_watermark_ratio"`
	// MonitorInterval is the sampling period for checking heap usage.
	MonitorInterval time.Duration `json:"monitor_interval"`
	// BackpressureDelay is the dispatch pause duration injected under active backpressure.
	BackpressureDelay time.Duration `json:"backpressure_delay"`
	// AutoGC specifies whether to invoke runtime.GC() when memory exceeds high watermark.
	AutoGC bool `json:"auto_gc"`

	// MemStatsFunc allows injecting custom heap alloc probe for deterministic testing.
	MemStatsFunc func() uint64 `json:"-"`
}

// DefaultOptions returns production-ready default settings for high-concurrency probing.
func DefaultOptions() Options {
	return Options{
		Concurrency:        100,
		MinConcurrency:     10,
		MaxConcurrency:     500,
		TaskTimeout:        5 * time.Second,
		GlobalTimeout:      0,
		MemoryLimitBytes:   150 * 1024 * 1024, // 150MB soft memory limit
		HighWatermarkRatio: 0.80,              // 80% watermark (120MB)
		LowWatermarkRatio:  0.60,              // 60% watermark (90MB)
		MonitorInterval:    100 * time.Millisecond,
		BackpressureDelay:  10 * time.Millisecond,
		AutoGC:             true,
	}
}

// Stats holds real-time telemetry metrics for the pool.
type Stats struct {
	ConfiguredConcurrency int    `json:"configured_concurrency"`
	CurrentConcurrency    int    `json:"current_concurrency"`
	ActiveTasks           int    `json:"active_tasks"`
	Waiters               int    `json:"waiters"`
	CompletedTasks        int64  `json:"completed_tasks"`
	FailedTasks           int64  `json:"failed_tasks"`
	TimedOutTasks         int64  `json:"timed_out_tasks"`
	PanicTasks            int64  `json:"panic_tasks"`
	BackpressureActive    bool   `json:"backpressure_active"`
	BackpressureEvents    int64  `json:"backpressure_events"`
	CurrentAllocBytes     uint64 `json:"current_alloc_bytes"`
	MemoryLimitBytes      int64  `json:"memory_limit_bytes"`
}
