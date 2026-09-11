package pool

import (
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryGuard monitors runtime memory pressure and coordinates dynamic backpressure
// against soft memory boundaries configured via runtime/debug.SetMemoryLimit.
type MemoryGuard struct {
	mu                 sync.RWMutex
	opts               Options
	backpressureActive atomic.Bool
	backpressureEvents atomic.Int64
	lastAllocBytes     atomic.Uint64
	stopCh             chan struct{}
	doneCh             chan struct{}
	onTrigger          func(active bool, ratio float64)
}

// NewMemoryGuard initializes the soft memory limit and starts the telemetry loop.
func NewMemoryGuard(opts Options, onTrigger func(active bool, ratio float64)) *MemoryGuard {
	if opts.MemoryLimitBytes > 0 {
		debug.SetMemoryLimit(opts.MemoryLimitBytes)
	}
	if opts.MonitorInterval <= 0 {
		opts.MonitorInterval = 100 * time.Millisecond
	}
	if opts.HighWatermarkRatio <= 0 {
		opts.HighWatermarkRatio = 0.80
	}
	if opts.LowWatermarkRatio <= 0 {
		opts.LowWatermarkRatio = 0.60
	}

	g := &MemoryGuard{
		opts:      opts,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
		onTrigger: onTrigger,
	}

	go g.monitorLoop()
	return g
}

func (g *MemoryGuard) readAlloc() uint64 {
	if g.opts.MemStatsFunc != nil {
		return g.opts.MemStatsFunc()
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func (g *MemoryGuard) monitorLoop() {
	defer close(g.doneCh)
	ticker := time.NewTicker(g.opts.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.checkMemory()
		}
	}
}

// CheckMemory evaluates current memory usage against watermarks.
func (g *MemoryGuard) CheckMemory() {
	g.checkMemory()
}

func (g *MemoryGuard) checkMemory() {
	if g.opts.MemoryLimitBytes <= 0 {
		return
	}

	alloc := g.readAlloc()
	g.lastAllocBytes.Store(alloc)

	ratio := float64(alloc) / float64(g.opts.MemoryLimitBytes)

	if ratio >= g.opts.HighWatermarkRatio {
		if !g.backpressureActive.Swap(true) {
			g.backpressureEvents.Add(1)
			if g.onTrigger != nil {
				g.onTrigger(true, ratio)
			}
			if g.opts.AutoGC {
				runtime.GC()
			}
		}
	} else if ratio <= g.opts.LowWatermarkRatio {
		if g.backpressureActive.Swap(false) {
			if g.onTrigger != nil {
				g.onTrigger(false, ratio)
			}
		}
	}
}

// IsBackpressureActive returns true if dynamic backpressure is currently triggered.
func (g *MemoryGuard) IsBackpressureActive() bool {
	return g.backpressureActive.Load()
}

// BackpressureEvents returns the total number of backpressure activations.
func (g *MemoryGuard) BackpressureEvents() int64 {
	return g.backpressureEvents.Load()
}

// LastAllocBytes returns the most recently sampled heap allocation in bytes.
func (g *MemoryGuard) LastAllocBytes() uint64 {
	return g.lastAllocBytes.Load()
}

// Close stops the background monitoring goroutine.
func (g *MemoryGuard) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	select {
	case <-g.stopCh:
		// already closed
	default:
		close(g.stopCh)
		<-g.doneCh
	}
}
