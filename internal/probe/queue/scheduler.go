package queue

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"clash-sub-parser/internal/domain"
)

var (
	ErrInvalidConcurrency = errors.New("concurrency must be between 1 and 256")
	ErrCapacityExceeded   = errors.New("queue capacity exceeded")
	ErrNodeConflict       = errors.New("node has active or recent probe for this kind")
	ErrSchedulerClosed    = errors.New("scheduler is closed")
)

const (
	DefaultConcurrency = 50
	MinConcurrency     = 1
	MaxConcurrency     = 64
	HardMaxConcurrency = 256
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

// EnqueueMode controls how a task is handled during submission.
type EnqueueMode int

const (
	// EnqueueStrictConflict (default zero value) preserves the original Submit
	// behaviour: if the node+kind is active or within NodeTTL, ErrNodeConflict
	// is returned.  This keeps all existing tests intact.
	EnqueueStrictConflict EnqueueMode = 0

	// EnqueuePeriodicDedupe is used by the periodic coordinator.  If the node
	// is already in the pool (probing or queued), the task is silently skipped
	// (OnComplete(nil) is called and nil is returned).  Otherwise the task is
	// appended to the queue tail.
	EnqueuePeriodicDedupe EnqueueMode = 1

	// EnqueueManualPreemptFront is used by manual probe triggers.  NodeTTL is
	// ignored; the task is inserted at the front of the queue for its run so
	// that the next idle worker picks it up first.  If the node is already
	// queued it is promoted to the front.  If it is currently probing, the
	// manual task is still queued at the front (no conflict error).
	EnqueueManualPreemptFront EnqueueMode = 2
)

type Task struct {
	ID         string
	RunID      string
	LogicalID  string
	Kind       domain.ProbeKind
	Mode       EnqueueMode
	Context    context.Context
	Execute    func(ctx context.Context) error
	OnComplete func(err error)
}

type activeRecord struct {
	active    bool
	count     int
	until     time.Time
	lastRunID string
}

type Scheduler struct {
	cfg                    Config
	explicitRunConcurrency bool
	mu                     sync.Mutex
	closed         bool
	queues         map[string][]Task
	runOrder       []string
	runCursor      int
	queued         int
	activeByRun    map[string]int
	activeNodes    map[string]activeRecord
	probingNodes   map[string]int
	probingTasks   map[int64]Task
	nextExecID     int64
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
	explicitRunConcurrency := cfg.RunConcurrency > 0
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
	s := &Scheduler{
		cfg:                    cfg,
		explicitRunConcurrency: explicitRunConcurrency,
		queues:                 make(map[string][]Task),
		activeByRun:    make(map[string]int),
		activeNodes:    make(map[string]activeRecord),
		probingNodes:   make(map[string]int),
		probingTasks:   make(map[int64]Task),
		runCancels:     make(map[string]map[int64]context.CancelFunc),
		cancelledRuns:  make(map[string]bool),
		changed:        make(chan struct{}),
		dispatcherDone: make(chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
	}
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
		s.nextExecID++
		execID := s.nextExecID
		s.probingNodes[task.LogicalID]++
		s.probingTasks[execID] = task
		current := s.activeCount.Add(1)
		for prev := s.peakActive.Load(); current > prev && !s.peakActive.CompareAndSwap(prev, current); prev = s.peakActive.Load() {
		}
		s.mu.Unlock()
		go s.processTask(task, execID)
	}
}

// activeCompetingRunsLocked returns the number of distinct runs that either have queued tasks
// or currently active in-flight tasks.
// Caller must hold s.mu.
func (s *Scheduler) activeCompetingRunsLocked() int {
	runs := len(s.queues)
	for runID := range s.activeByRun {
		if _, ok := s.queues[runID]; !ok {
			runs++
		}
	}
	return runs
}

func (s *Scheduler) nextEligibleLocked() (Task, bool) {
	if len(s.runOrder) == 0 {
		return Task{}, false
	}
	// Priority pass: always dispatch EnqueueManualPreemptFront tasks at the front first.
	for i, run := range s.runOrder {
		q := s.queues[run]
		if len(q) > 0 && q[0].Mode == EnqueueManualPreemptFront {
			t := q[0]
			s.queues[run] = q[1:]
			s.queued--
			if len(s.queues[run]) == 0 {
				delete(s.queues, run)
				s.runOrder = append(s.runOrder[:i], s.runOrder[i+1:]...)
				if len(s.runOrder) > 0 {
					s.runCursor %= len(s.runOrder)
				} else {
					s.runCursor = 0
				}
			}
			s.signalLocked()
			return t, true
		}
	}
	competing := s.activeCompetingRunsLocked()
	for n := 0; n < len(s.runOrder); n++ {
		i := (s.runCursor + n) % len(s.runOrder)
		run := s.runOrder[i]
		q := s.queues[run]
		limit := s.cfg.RunConcurrency
		if !s.explicitRunConcurrency && competing <= 1 {
			limit = s.cfg.Concurrency
		}
		if len(q) > 0 && s.activeByRun[run] < limit {
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

	// Dispatch to mode-specific logic.
	switch task.Mode {
	case EnqueuePeriodicDedupe:
		return s.submitPeriodicDedupe(ctx, task, wait)
	case EnqueueManualPreemptFront:
		return s.submitManualPreemptFront(ctx, task, wait)
	default:
		return s.submitStrictConflict(ctx, task, wait)
	}
}

// submitStrictConflict is the original Submit behaviour (Mode == 0).
func (s *Scheduler) submitStrictConflict(ctx context.Context, task Task, wait bool) error {
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
			s.enqueueTaskLocked(key, task)
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

func (s *Scheduler) enqueueTaskLocked(key string, task Task) {
	s.activeNodes[key] = activeRecord{active: true, count: 1, lastRunID: task.RunID}
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
}

// isNodeInPoolForPeriodicLocked checks whether this node is already in the pool
// (either actively probing or waiting in queue) for periodic deduplication.
// Caller must hold s.mu.
func (s *Scheduler) isNodeInPoolForPeriodicLocked(task Task) bool {
	if task.LogicalID == "" {
		return false
	}
	for _, pt := range s.probingTasks {
		if pt.LogicalID == task.LogicalID && (pt.RunID != task.RunID || task.RunID == "" || pt.Kind == task.Kind) {
			return true
		}
	}
	for _, q := range s.queues {
		for _, t := range q {
			if t.LogicalID == task.LogicalID && (t.RunID != task.RunID || task.RunID == "" || t.Kind == task.Kind) {
				return true
			}
		}
	}
	return false
}

// submitPeriodicDedupe silently skips if the node is already in the pool.
func (s *Scheduler) submitPeriodicDedupe(ctx context.Context, task Task, wait bool) error {
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

		// If already in pool (probing or queued), silently complete.
		if s.isNodeInPoolForPeriodicLocked(task) {
			s.mu.Unlock()
			if task.OnComplete != nil {
				task.OnComplete(nil)
			}
			return nil
		}

		now := s.cfg.Clock()
		s.pruneActiveNodesLocked(now)

		if s.queued < s.cfg.QueueSize {
			rec := s.activeNodes[key]
			rec.count++
			rec.active = true
			rec.until = time.Time{}
			rec.lastRunID = task.RunID
			s.activeNodes[key] = rec
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

// submitManualPreemptFront inserts the task at the front of the node pool queue,
// bypassing NodeTTL and never returning ErrNodeConflict. If the node is already
// queued, it is promoted to the front.
func (s *Scheduler) submitManualPreemptFront(ctx context.Context, task Task, wait bool) error {
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

		// Reject duplicate (LogicalID, Kind) within the exact same non-empty RunID.
		if task.RunID != "" {
			if rec, ok := s.activeNodes[key]; ok && rec.lastRunID == task.RunID && (rec.active || now.Before(rec.until)) {
				s.mu.Unlock()
				return ErrNodeConflict
			}
			for _, pt := range s.probingTasks {
				if pt.RunID == task.RunID && pt.LogicalID == task.LogicalID && pt.Kind == task.Kind {
					s.mu.Unlock()
					return ErrNodeConflict
				}
			}
			for _, qt := range s.queues[task.RunID] {
				if qt.LogicalID == task.LogicalID && qt.Kind == task.Kind {
					s.mu.Unlock()
					return ErrNodeConflict
				}
			}
		}

		// Check if the node+kind is already queued in another run; if so, remove the old queued entry
		// so we can promote it to the front. If the node is queued for a different kind, promote that
		// task to the front as well so all pending kinds for this node run first.
		var promotedOld []Task
		for runID, q := range s.queues {
			if runID == task.RunID && task.RunID != "" {
				continue
			}
			for i := 0; i < len(q); i++ {
				t := q[i]
				if t.LogicalID != task.LogicalID {
					continue
				}
				if t.Kind == task.Kind || t.Kind == "" || task.Kind == "" {
					oldKey := makeNodeKey(t.LogicalID, t.Kind)
					if rec, ok := s.activeNodes[oldKey]; ok {
						rec.count--
						if rec.count <= 0 {
							delete(s.activeNodes, oldKey)
						} else {
							rec.active = true
							s.activeNodes[oldKey] = rec
						}
					}
					q = append(q[:i], q[i+1:]...)
					i--
					s.queues[runID] = q
					s.queued--
					s.taskCount--
					promotedOld = append(promotedOld, t)
				} else if t.Mode != EnqueueManualPreemptFront {
					q[i].Mode = EnqueueManualPreemptFront
					promotedTask := q[i]
					q = append(q[:i], q[i+1:]...)
					insertAt := 0
					for insertAt < len(q) && q[insertAt].Mode == EnqueueManualPreemptFront {
						insertAt++
					}
					updatedQ := make([]Task, 0, len(q)+1)
					updatedQ = append(updatedQ, q[:insertAt]...)
					updatedQ = append(updatedQ, promotedTask)
					updatedQ = append(updatedQ, q[insertAt:]...)
					q = updatedQ
					s.queues[runID] = q
				}
			}
			if len(s.queues[runID]) == 0 {
				delete(s.queues, runID)
				for j, r := range s.runOrder {
					if r == runID {
						s.runOrder = append(s.runOrder[:j], s.runOrder[j+1:]...)
						if len(s.runOrder) > 0 {
							s.runCursor %= len(s.runOrder)
						} else {
							s.runCursor = 0
						}
						break
					}
				}
			}
		}

		if s.queued < s.cfg.QueueSize {
			rec := s.activeNodes[key]
			rec.count++
			rec.active = true
			rec.until = time.Time{}
			rec.lastRunID = task.RunID
			s.activeNodes[key] = rec
			s.taskCount++
			s.callbacks++

			// Place task.RunID at the front of s.runOrder.
			found := false
			for j, r := range s.runOrder {
				if r == task.RunID {
					found = true
					if j > 0 {
						s.runOrder = append(s.runOrder[:j], s.runOrder[j+1:]...)
						s.runOrder = append([]string{task.RunID}, s.runOrder...)
					}
					break
				}
			}
			if !found {
				s.runOrder = append([]string{task.RunID}, s.runOrder...)
			}
			s.runCursor = 0

			// Insert right after any existing EnqueueManualPreemptFront tasks in this run's queue
			// so a manual batch ["node-18", "node-99"] stays in order ["node-18", "node-99"]
			// while still preempting all periodic/strict tasks in the queue.
			existingQ := s.queues[task.RunID]
			insertIdx := 0
			for insertIdx < len(existingQ) && existingQ[insertIdx].Mode == EnqueueManualPreemptFront {
				insertIdx++
			}
			newQ := make([]Task, 0, len(existingQ)+1)
			newQ = append(newQ, existingQ[:insertIdx]...)
			newQ = append(newQ, task)
			newQ = append(newQ, existingQ[insertIdx:]...)
			s.queues[task.RunID] = newQ

			s.queued++
			for prev := s.peakQueued.Load(); int32(s.queued) > prev && !s.peakQueued.CompareAndSwap(prev, int32(s.queued)); prev = s.peakQueued.Load() {
			}
			s.signalLocked()
			s.mu.Unlock()

			for _, oldTask := range promotedOld {
				go s.completeCallback(oldTask, nil)
			}
			return nil
		}
		if !wait {
			s.mu.Unlock()
			for _, oldTask := range promotedOld {
				go s.completeCallback(oldTask, nil)
			}
			return ErrCapacityExceeded
		}
		wake := s.changed
		s.mu.Unlock()
		for _, oldTask := range promotedOld {
			go s.completeCallback(oldTask, nil)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ctx.Done():
			return ErrSchedulerClosed
		case <-wake:
		}
	}
}

func (s *Scheduler) processTask(task Task, execID int64) {
	var (
		err       error
		completed bool
		seq       int64
		hasSeq    bool
	)
	defer func() {
		if !completed {
			s.mu.Lock()
			s.finishTaskExecutionLocked(task, execID, seq, hasSeq)
			s.mu.Unlock()
			s.completeCallback(task, err)
		}
	}()

	s.mu.Lock()
	if s.closed || s.cancelledRuns[task.RunID] {
		completed = true
		s.finishTaskExecutionLocked(task, execID, 0, false)
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
	s.finishTaskExecutionLocked(task, execID, seq, hasSeq)
	s.mu.Unlock()

	// Completion is no longer outstanding before invoking user code, permitting
	// callbacks to call Wait without waiting on themselves.
	s.completeCallback(task, err)
}

func (s *Scheduler) finishTaskExecutionLocked(task Task, execID int64, seq int64, hasSeq bool) {
	if hasSeq {
		if m := s.runCancels[task.RunID]; m != nil {
			delete(m, seq)
			if len(m) == 0 {
				delete(s.runCancels, task.RunID)
			}
		}
	}
	delete(s.probingTasks, execID)
	if s.probingNodes[task.LogicalID] > 1 {
		s.probingNodes[task.LogicalID]--
	} else {
		delete(s.probingNodes, task.LogicalID)
	}
	s.activeCount.Add(-1)
	s.activeByRun[task.RunID]--
	if s.activeByRun[task.RunID] == 0 {
		delete(s.activeByRun, task.RunID)
	}
	key := makeNodeKey(task.LogicalID, task.Kind)
	if rec, stillActive := s.activeNodes[key]; stillActive {
		rec.count--
		if rec.count <= 0 {
			rec.count = 0
			rec.active = false
			rec.until = s.cfg.Clock().Add(s.cfg.NodeTTL)
		} else {
			rec.active = true
		}
		s.activeNodes[key] = rec
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
		key := makeNodeKey(t.LogicalID, t.Kind)
		rec := s.activeNodes[key]
		rec.count--
		if rec.count <= 0 {
			rec.count = 0
			rec.active = false
			rec.until = s.cfg.Clock().Add(s.cfg.NodeTTL)
		} else {
			rec.active = true
		}
		s.activeNodes[key] = rec
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
		key := makeNodeKey(t.LogicalID, t.Kind)
		rec := s.activeNodes[key]
		rec.count--
		if rec.count <= 0 {
			rec.count = 0
			rec.active = false
			rec.until = s.cfg.Clock().Add(s.cfg.NodeTTL)
		} else {
			rec.active = true
		}
		s.activeNodes[key] = rec
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

// nodeIDFromKey extracts the logicalID from a key produced by makeNodeKey.
func nodeIDFromKey(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '#' {
			return key[:i]
		}
	}
	return key
}

// PoolQueueSnapshot is a point-in-time snapshot of the node probe pool.
type PoolQueueSnapshot struct {
	ProbingNodeIDs     []string `json:"probing_node_ids"`
	QueuedNodeIDs      []string `json:"queued_node_ids"`
	ProbingCount       int      `json:"probing_count"`
	QueuedWaitingCount int      `json:"queued_waiting_count"`
	QueueNodesCount    int      `json:"queue_nodes_count"` // ProbingCount + QueuedWaitingCount
}

// SnapshotNodePool returns a thread-safe snapshot of the current node pool state.
func (s *Scheduler) SnapshotNodePool() PoolQueueSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	probingSet := make(map[string]bool, len(s.probingNodes))
	probingIDs := make([]string, 0, len(s.probingNodes))
	for nid, count := range s.probingNodes {
		if count > 0 && nid != "" {
			probingSet[nid] = true
			probingIDs = append(probingIDs, nid)
		}
	}
	sort.Strings(probingIDs)

	queuedSet := make(map[string]bool)
	queuedOrdered := make([]string, 0)

	// First collect EnqueueManualPreemptFront tasks in runOrder, then remaining tasks,
	// so manual front-of-queue nodes always appear at the front of QueuedNodeIDs.
	for _, runID := range s.runOrder {
		for _, t := range s.queues[runID] {
			if t.Mode != EnqueueManualPreemptFront {
				continue
			}
			nid := t.LogicalID
			if nid != "" && !queuedSet[nid] && !probingSet[nid] {
				queuedSet[nid] = true
				queuedOrdered = append(queuedOrdered, nid)
			}
		}
	}
	for _, runID := range s.runOrder {
		for _, t := range s.queues[runID] {
			nid := t.LogicalID
			if nid != "" && !queuedSet[nid] && !probingSet[nid] {
				queuedSet[nid] = true
				queuedOrdered = append(queuedOrdered, nid)
			}
		}
	}

	snap := PoolQueueSnapshot{
		ProbingNodeIDs:     probingIDs,
		QueuedNodeIDs:      queuedOrdered,
		ProbingCount:       len(probingIDs),
		QueuedWaitingCount: len(queuedOrdered),
	}
	snap.QueueNodesCount = snap.ProbingCount + snap.QueuedWaitingCount
	return snap
}

// PoolQueueSnapshot is an alias for SnapshotNodePool.
func (s *Scheduler) PoolQueueSnapshot() PoolQueueSnapshot {
	return s.SnapshotNodePool()
}

// GetNodePoolState returns "probing", "queued", or "idle" for a given logicalID.
func (s *Scheduler) GetNodePoolState(logicalID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.probingNodes[logicalID] > 0 {
		return "probing"
	}
	for _, q := range s.queues {
		for _, t := range q {
			if t.LogicalID == logicalID {
				return "queued"
			}
		}
	}
	return "idle"
}

// GetNodeProbeState is an alias for GetNodePoolState.
func (s *Scheduler) GetNodeProbeState(logicalID string) string {
	return s.GetNodePoolState(logicalID)
}

// IsNodeInPool returns true if the node is currently probing or queued in the node pool.
func (s *Scheduler) IsNodeInPool(logicalID string) bool {
	return s.GetNodePoolState(logicalID) != "idle"
}
