package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"clash-sub-parser/internal/domain"
)

var (
	ErrInvalidConcurrency = errors.New("concurrency must be between 10 and 32")
	ErrCapacityExceeded   = errors.New("queue capacity exceeded")
	ErrNodeConflict       = errors.New("node has active or recent probe for this kind")
	ErrSchedulerClosed    = errors.New("scheduler is closed")
)

const (
	DefaultConcurrency = 16
	MinConcurrency     = 10
	MaxConcurrency     = 20
	HardMaxConcurrency = 32
	DefaultQueueSize   = 1024
	DefaultNodeTTL     = 30 * time.Second
)

// Config controls scheduler bounds. RunConcurrency defaults to half of the global
// concurrency (rounded up), so one run cannot monopolize all execution slots.
type Config struct {
	Concurrency    int
	RunConcurrency int
	QueueSize      int
	NodeTTL        time.Duration
	Clock          func() time.Time
	Context        context.Context
}

type Task struct {
	ID         string
	RunID      string
	LogicalID  string
	Kind       domain.ProbeKind
	Context    context.Context
	Execute    func(ctx context.Context) error
	OnComplete func(err error)
}

type activeRecord struct {
	active bool
	until  time.Time
}

type Scheduler struct {
	cfg            Config
	mu             sync.Mutex
	closed         bool
	queues         map[string][]Task
	runOrder       []string
	runCursor      int
	queued         int
	activeByRun    map[string]int
	activeNodes    map[string]activeRecord
	runCancels     map[string]map[int64]context.CancelFunc
	cancelledRuns  map[string]bool
	cancelSequence int64
	changed        chan struct{}
	activeCount    atomic.Int32
	peakActive     atomic.Int32
	peakQueued     atomic.Int32
	taskCount      int
	callbacks      int
	dispatcherDone chan struct{}
	workerWg       sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewScheduler(cfg Config) (*Scheduler, error) {
	if cfg.Concurrency == 0 {
		cfg.Concurrency = DefaultConcurrency
	}
	if cfg.Concurrency < MinConcurrency || cfg.Concurrency > HardMaxConcurrency {
		return nil, ErrInvalidConcurrency
	}
	if cfg.RunConcurrency < 0 || cfg.RunConcurrency > cfg.Concurrency {
		return nil, errors.New("run concurrency must be between zero and global concurrency")
	}
	if cfg.RunConcurrency == 0 {
		cfg.RunConcurrency = (cfg.Concurrency + 1) / 2
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = DefaultQueueSize
	}
	if cfg.NodeTTL <= 0 {
		cfg.NodeTTL = DefaultNodeTTL
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	parent := cfg.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Scheduler{cfg: cfg, queues: make(map[string][]Task), activeByRun: make(map[string]int), activeNodes: make(map[string]activeRecord), runCancels: make(map[string]map[int64]context.CancelFunc), cancelledRuns: make(map[string]bool), changed: make(chan struct{}), dispatcherDone: make(chan struct{}), ctx: ctx, cancel: cancel}
	s.workerWg.Add(1)
	go s.dispatch()
	return s, nil
}

func (s *Scheduler) signalLocked() { close(s.changed); s.changed = make(chan struct{}) }

// Keep TTL history bounded even when callers continuously submit unique nodes.
func (s *Scheduler) pruneActiveNodesLocked(now time.Time) {
	for key, record := range s.activeNodes {
		if !record.active && !now.Before(record.until) {
			delete(s.activeNodes, key)
		}
	}
	limit := s.cfg.QueueSize + s.cfg.Concurrency
	for len(s.activeNodes) > limit {
		var oldestKey string
		var oldest time.Time
		for key, record := range s.activeNodes {
			if !record.active && (oldestKey == "" || record.until.Before(oldest)) {
				oldestKey, oldest = key, record.until
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.activeNodes, oldestKey)
	}
}

func (s *Scheduler) dispatch() {
	defer s.workerWg.Done()
	defer close(s.dispatcherDone)
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		if int(s.activeCount.Load()) >= s.cfg.Concurrency || s.queued == 0 {
			wake := s.changed
			s.mu.Unlock()
			select {
			case <-wake:
			case <-s.ctx.Done():
				return
			}
			continue
		}
		task, ok := s.nextEligibleLocked()
		if !ok {
			wake := s.changed
			s.mu.Unlock()
			select {
			case <-wake:
			case <-s.ctx.Done():
				return
			}
			continue
		}
		s.activeByRun[task.RunID]++
		current := s.activeCount.Add(1)
		for prev := s.peakActive.Load(); current > prev && !s.peakActive.CompareAndSwap(prev, current); prev = s.peakActive.Load() {
		}
		s.mu.Unlock()
		go s.processTask(task)
	}
}

func (s *Scheduler) nextEligibleLocked() (Task, bool) {
	if len(s.runOrder) == 0 {
		return Task{}, false
	}
	for n := 0; n < len(s.runOrder); n++ {
		i := (s.runCursor + n) % len(s.runOrder)
		run := s.runOrder[i]
		q := s.queues[run]
		if len(q) > 0 && s.activeByRun[run] < s.cfg.RunConcurrency {
			t := q[0]
			s.queues[run] = q[1:]
			s.queued--
			if len(s.queues[run]) == 0 {
				delete(s.queues, run)
				s.runOrder = append(s.runOrder[:i], s.runOrder[i+1:]...)
				if len(s.runOrder) > 0 {
					s.runCursor = i % len(s.runOrder)
				} else {
					s.runCursor = 0
				}
			} else {
				s.runCursor = (i + 1) % len(s.runOrder)
			}
			s.signalLocked()
			return t, true
		}
	}
	return Task{}, false
}

func (s *Scheduler) Submit(task Task) error { return s.submit(context.Background(), task, false) }
func (s *Scheduler) SubmitWithContext(ctx context.Context, task Task) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.submit(ctx, task, true)
}
func (s *Scheduler) submit(ctx context.Context, task Task, wait bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := makeNodeKey(task.LogicalID, task.Kind)
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return ErrSchedulerClosed
		}
		if s.cancelledRuns[task.RunID] {
			s.mu.Unlock()
			return context.Canceled
		}
		now := s.cfg.Clock()
		s.pruneActiveNodesLocked(now)
		if rec, ok := s.activeNodes[key]; ok {
			if rec.active || now.Before(rec.until) {
				s.mu.Unlock()
				return ErrNodeConflict
			}
			delete(s.activeNodes, key)
		}
		if s.queued < s.cfg.QueueSize {
			s.activeNodes[key] = activeRecord{active: true}
			s.taskCount++
			s.callbacks++
			if len(s.queues[task.RunID]) == 0 {
				s.runOrder = append(s.runOrder, task.RunID)
			}
			s.queues[task.RunID] = append(s.queues[task.RunID], task)
			s.queued++
			for prev := s.peakQueued.Load(); int32(s.queued) > prev && !s.peakQueued.CompareAndSwap(prev, int32(s.queued)); prev = s.peakQueued.Load() {
			}
			s.signalLocked()
			s.mu.Unlock()
			return nil
		}
		if !wait {
			s.mu.Unlock()
			return ErrCapacityExceeded
		}
		wake := s.changed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ctx.Done():
			return ErrSchedulerClosed
		case <-wake:
		}
	}
}

func (s *Scheduler) processTask(task Task) {
	var (
		err       error
		completed bool
		seq       int64
		hasSeq    bool
	)
	defer func() {
		if !completed {
			s.mu.Lock()
			s.finishTaskExecutionLocked(task, seq, hasSeq)
			s.mu.Unlock()
			s.completeCallback(task, err)
		}
	}()

	s.mu.Lock()
	if s.closed || s.cancelledRuns[task.RunID] {
		completed = true
		s.finishTaskExecutionLocked(task, 0, false)
		s.mu.Unlock()
		s.completeCallback(task, context.Canceled)
		return
	}
	taskCtx, cancel := context.WithCancel(s.ctx)
	if task.Context != nil {
		context.AfterFunc(task.Context, cancel)
	}
	seq = s.cancelSequence
	s.cancelSequence++
	hasSeq = true
	if s.runCancels[task.RunID] == nil {
		s.runCancels[task.RunID] = make(map[int64]context.CancelFunc)
	}
	s.runCancels[task.RunID][seq] = cancel
	s.mu.Unlock()

	if task.Execute != nil {
		err = task.Execute(taskCtx)
	}
	cancel()

	s.mu.Lock()
	completed = true
	s.finishTaskExecutionLocked(task, seq, hasSeq)
	s.mu.Unlock()

	// Completion is no longer outstanding before invoking user code, permitting
	// callbacks to call Wait without waiting on themselves.
	s.completeCallback(task, err)
}

func (s *Scheduler) finishTaskExecutionLocked(task Task, seq int64, hasSeq bool) {
	if hasSeq {
		if m := s.runCancels[task.RunID]; m != nil {
			delete(m, seq)
			if len(m) == 0 {
				delete(s.runCancels, task.RunID)
			}
		}
	}
	s.activeCount.Add(-1)
	s.activeByRun[task.RunID]--
	if s.activeByRun[task.RunID] == 0 {
		delete(s.activeByRun, task.RunID)
	}
	key := makeNodeKey(task.LogicalID, task.Kind)
	if _, stillActive := s.activeNodes[key]; stillActive {
		s.activeNodes[key] = activeRecord{until: s.cfg.Clock().Add(s.cfg.NodeTTL)}
	}
	s.taskCount--
	s.signalLocked()
}

func (s *Scheduler) completeCallback(task Task, err error) {
	defer func() {
		s.mu.Lock()
		s.callbacks--
		s.signalLocked()
		s.mu.Unlock()
	}()
	if task.OnComplete != nil {
		task.OnComplete(err)
	}
}

func (s *Scheduler) CancelRun(runID string) {
	s.mu.Lock()
	s.cancelledRuns[runID] = true
	for _, cancel := range s.runCancels[runID] {
		cancel()
	}
	q := s.queues[runID]
	delete(s.queues, runID)
	for _, t := range q {
		s.queued--
		s.activeNodes[makeNodeKey(t.LogicalID, t.Kind)] = activeRecord{until: s.cfg.Clock().Add(s.cfg.NodeTTL)}
	}
	for i, r := range s.runOrder {
		if r == runID {
			s.runOrder = append(s.runOrder[:i], s.runOrder[i+1:]...)
			if len(s.runOrder) > 0 {
				s.runCursor %= len(s.runOrder)
			} else {
				s.runCursor = 0
			}
			break
		}
	}
	s.taskCount -= len(q)
	s.signalLocked()
	s.mu.Unlock()
	for _, t := range q {
		s.completeCallback(t, context.Canceled)
	}
}
func (s *Scheduler) ActiveCount() int     { return int(s.activeCount.Load()) }
func (s *Scheduler) QueuedCount() int     { s.mu.Lock(); defer s.mu.Unlock(); return s.queued }
func (s *Scheduler) PeakActiveCount() int { return int(s.peakActive.Load()) }
func (s *Scheduler) PeakQueuedCount() int { return int(s.peakQueued.Load()) }

// Wait observes task execution counts, not completion callbacks. Submissions
// may proceed after it returns; use Drain for complete shutdown.
func (s *Scheduler) Wait() {
	for {
		s.mu.Lock()
		if s.taskCount == 0 {
			s.mu.Unlock()
			return
		}
		wake := s.changed
		s.mu.Unlock()
		<-wake
	}
}

// Drain closes admission and waits for accepted callbacks and the dispatcher/workers.
// It must only be called by an external coordinator, never from Execute or OnComplete.
func (s *Scheduler) Drain(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_ = s.Close()
	for {
		s.mu.Lock()
		if s.callbacks == 0 && s.activeCount.Load() == 0 && s.queued == 0 {
			s.mu.Unlock()
			select {
			case <-s.dispatcherDone:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		wake := s.changed
		s.mu.Unlock()
		select {
		case <-wake:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Scheduler) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cancel()
	var pending []Task
	for _, q := range s.queues {
		pending = append(pending, q...)
	}
	s.queues = make(map[string][]Task)
	s.runOrder = nil
	s.queued = 0
	for _, t := range pending {
		s.activeNodes[makeNodeKey(t.LogicalID, t.Kind)] = activeRecord{until: s.cfg.Clock().Add(s.cfg.NodeTTL)}
	}
	for _, m := range s.runCancels {
		for _, cancel := range m {
			cancel()
		}
	}
	s.taskCount -= len(pending)
	s.signalLocked()
	s.mu.Unlock()
	for _, t := range pending {
		go s.completeCallback(t, context.Canceled)
	}
	// Close initiates cancellation but deliberately does not join workers: it is
	// safe to call from Execute/OnComplete. Call Wait after Close to join tasks.
	return nil
}
func makeNodeKey(logicalID string, kind domain.ProbeKind) string {
	return logicalID + "#" + string(kind)
}
