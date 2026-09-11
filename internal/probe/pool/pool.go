package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Pool coordinates dynamic sliding window concurrency and soft memory backpressure.
type Pool[T any] struct {
	opts      Options
	limiter   *DynamicLimiter
	guard     *MemoryGuard
	ctx       context.Context
	cancel    context.CancelFunc

	mu                sync.RWMutex
	targetConcurrency int
	closed            bool

	resultsChan chan Result[T]
	doneChan    chan struct{}
	wg          sync.WaitGroup

	// Telemetry counters
	completedTasks atomic.Int64
	failedTasks    atomic.Int64
	timedOutTasks  atomic.Int64
	panicTasks     atomic.Int64
}

// New constructs a new worker Pool for the specified return type T.
func New[T any](opts Options) *Pool[T] {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 100
	}
	if opts.MinConcurrency <= 0 {
		opts.MinConcurrency = 10
	}
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 500
	}
	if opts.Concurrency < opts.MinConcurrency {
		opts.Concurrency = opts.MinConcurrency
	}
	if opts.Concurrency > opts.MaxConcurrency {
		opts.Concurrency = opts.MaxConcurrency
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = 5 * time.Second
	}
	if opts.MemoryLimitBytes <= 0 {
		opts.MemoryLimitBytes = 150 * 1024 * 1024
	}

	ctx, cancel := context.WithCancel(context.Background())
	if opts.GlobalTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.GlobalTimeout)
	}

	bufSize := opts.Concurrency * 4
	if bufSize < 1024 {
		bufSize = 1024
	}

	p := &Pool[T]{
		opts:              opts,
		limiter:           NewLimiter(opts.Concurrency),
		ctx:               ctx,
		cancel:            cancel,
		targetConcurrency: opts.Concurrency,
		resultsChan:       make(chan Result[T], bufSize),
		doneChan:          make(chan struct{}),
	}

	p.guard = NewMemoryGuard(opts, func(active bool, ratio float64) {
		p.handleBackpressureTrigger(active, ratio)
	})

	return p
}

func (p *Pool[T]) handleBackpressureTrigger(active bool, ratio float64) {
	p.mu.RLock()
	target := p.targetConcurrency
	minC := p.opts.MinConcurrency
	p.mu.RUnlock()

	if active {
		reduced := target / 2
		if reduced < minC {
			reduced = minC
		}
		p.limiter.SetLimit(reduced)
	} else {
		p.limiter.SetLimit(target)
	}
}

// Concurrency returns the configured target concurrency.
func (p *Pool[T]) Concurrency() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.targetConcurrency
}

// SetConcurrency dynamically updates the target concurrency window.
func (p *Pool[T]) SetConcurrency(n int) {
	p.mu.Lock()
	if n < p.opts.MinConcurrency {
		n = p.opts.MinConcurrency
	}
	if n > p.opts.MaxConcurrency {
		n = p.opts.MaxConcurrency
	}
	p.targetConcurrency = n
	isBP := p.guard != nil && p.guard.IsBackpressureActive()
	p.mu.Unlock()

	if isBP {
		reduced := n / 2
		if reduced < p.opts.MinConcurrency {
			reduced = p.opts.MinConcurrency
		}
		p.limiter.SetLimit(reduced)
	} else {
		p.limiter.SetLimit(n)
	}
}

// ActiveCount returns the number of workers currently running tasks.
func (p *Pool[T]) ActiveCount() int {
	return p.limiter.Active()
}

// Results returns the receive-only channel for results submitted via Submit.
func (p *Pool[T]) Results() <-chan Result[T] {
	return p.resultsChan
}

// Done returns a channel that closes when the pool is stopped and all tasks finish.
func (p *Pool[T]) Done() <-chan struct{} {
	return p.doneChan
}

// Cancel immediately cancels all running and pending tasks.
func (p *Pool[T]) Cancel() {
	p.cancel()
}

// Stop initiates a graceful shutdown of the pool, preventing new submissions and
// waiting for in-flight tasks to terminate before closing results and done channels.
func (p *Pool[T]) Stop() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	p.wg.Wait()
	if p.guard != nil {
		p.guard.Close()
	}
	p.cancel()

	close(p.resultsChan)
	close(p.doneChan)
}

// Wait blocks until all currently in-flight tasks have finished.
func (p *Pool[T]) Wait() {
	p.wg.Wait()
}

// Submit enqueues a single Task into the pool, waiting for concurrency slot allocation.
func (p *Pool[T]) Submit(ctx context.Context, task Task[T]) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return ErrPoolClosed
	}
	p.mu.RUnlock()

	// Apply backpressure throttle delay if active
	if p.guard != nil && p.guard.IsBackpressureActive() && p.opts.BackpressureDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.ctx.Done():
			return ErrPoolCancelled
		case <-time.After(p.opts.BackpressureDelay):
		}
	}

	combinedCtx, cancelCombined := combineContexts(ctx, p.ctx)
	defer cancelCombined()

	if err := p.limiter.Acquire(combinedCtx); err != nil {
		if errors.Is(err, context.Canceled) && p.ctx.Err() != nil {
			return ErrPoolCancelled
		}
		return err
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.executeTask(task, p.resultsChan, true)
	}()
	return nil
}

// DispatchBatch streams execution of a collection of tasks over the sliding window.
// It returns a dedicated receive channel that yields results in real time and closes
// when all tasks in the batch have resolved.
func (p *Pool[T]) DispatchBatch(ctx context.Context, tasks []Task[T]) <-chan Result[T] {
	bufLen := len(tasks)
	if bufLen > 2048 {
		bufLen = 2048
	}
	outChan := make(chan Result[T], bufLen)

	go func() {
		defer close(outChan)

		var batchWg sync.WaitGroup
		for _, task := range tasks {
			select {
			case <-ctx.Done():
				outChan <- Result[T]{
					ID:    task.ID,
					Error: fmt.Errorf("task %s cancelled before dispatch: %w", task.ID, ctx.Err()),
				}
				continue
			case <-p.ctx.Done():
				outChan <- Result[T]{
					ID:    task.ID,
					Error: fmt.Errorf("task %s cancelled by pool shutdown: %w", task.ID, ErrPoolCancelled),
				}
				continue
			default:
			}

			if p.guard != nil && p.guard.IsBackpressureActive() && p.opts.BackpressureDelay > 0 {
				time.Sleep(p.opts.BackpressureDelay)
			}

			combinedCtx, cancelCombined := combineContexts(ctx, p.ctx)
			if err := p.limiter.Acquire(combinedCtx); err != nil {
				cancelCombined()
				outChan <- Result[T]{
					ID:    task.ID,
					Error: fmt.Errorf("task %s acquire failed: %w", task.ID, err),
				}
				continue
			}
			cancelCombined()

			batchWg.Add(1)
			go func(t Task[T]) {
				defer batchWg.Done()
				p.executeTask(t, outChan, true)
			}(task)
		}

		batchWg.Wait()
	}()

	return outChan
}

func (p *Pool[T]) executeTask(task Task[T], out chan<- Result[T], releaseLimiter bool) {
	if releaseLimiter {
		defer p.limiter.Release()
	}

	taskTimeout := task.Timeout
	if taskTimeout <= 0 {
		taskTimeout = p.opts.TaskTimeout
	}

	var taskCtx context.Context
	var cancel context.CancelFunc
	if taskTimeout > 0 {
		taskCtx, cancel = context.WithTimeout(p.ctx, taskTimeout)
	} else {
		taskCtx, cancel = context.WithCancel(p.ctx)
	}
	defer cancel()

	start := time.Now()

	type execRes struct {
		val T
		err error
	}
	resCh := make(chan execRes, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				p.panicTasks.Add(1)
				resCh <- execRes{err: fmt.Errorf("task panicked: %v", r)}
			}
		}()
		v, e := task.Fn(taskCtx)
		resCh <- execRes{val: v, err: e}
	}()

	var res Result[T]
	res.ID = task.ID

	select {
	case r := <-resCh:
		res.Value = r.val
		res.Error = r.err
		res.Duration = time.Since(start)
	case <-taskCtx.Done():
		res.Duration = time.Since(start)
		if errors.Is(taskCtx.Err(), context.DeadlineExceeded) {
			p.timedOutTasks.Add(1)
			res.Error = fmt.Errorf("task %s: %w", task.ID, context.DeadlineExceeded)
		} else {
			res.Error = fmt.Errorf("task %s: %w", task.ID, taskCtx.Err())
		}
	}

	if res.Error != nil {
		p.failedTasks.Add(1)
	} else {
		p.completedTasks.Add(1)
	}

	select {
	case out <- res:
	case <-p.ctx.Done():
		select {
		case out <- res:
		default:
		}
	}
}

// Stats returns a snapshot of pool telemetry and backpressure state.
func (p *Pool[T]) Stats() Stats {
	p.mu.RLock()
	target := p.targetConcurrency
	p.mu.RUnlock()

	var alloc uint64
	var bpActive bool
	var bpEvents int64
	var memLimit int64 = p.opts.MemoryLimitBytes

	if p.guard != nil {
		alloc = p.guard.LastAllocBytes()
		bpActive = p.guard.IsBackpressureActive()
		bpEvents = p.guard.BackpressureEvents()
	}

	return Stats{
		ConfiguredConcurrency: target,
		CurrentConcurrency:    p.limiter.Limit(),
		ActiveTasks:           p.limiter.Active(),
		Waiters:               p.limiter.Waiters(),
		CompletedTasks:        p.completedTasks.Load(),
		FailedTasks:           p.failedTasks.Load(),
		TimedOutTasks:         p.timedOutTasks.Load(),
		PanicTasks:            p.panicTasks.Load(),
		BackpressureActive:    bpActive,
		BackpressureEvents:    bpEvents,
		CurrentAllocBytes:     alloc,
		MemoryLimitBytes:      memLimit,
	}
}

// Guard returns the internal memory guard.
func (p *Pool[T]) Guard() *MemoryGuard {
	return p.guard
}

func combineContexts(ctx1, ctx2 context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx1)
	stop := context.AfterFunc(ctx2, func() {
		cancel()
	})
	return ctx, func() {
		stop()
		cancel()
	}
}
