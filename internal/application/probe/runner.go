package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/identity"
	"clash-sub-parser/internal/probe/profiles"
	"clash-sub-parser/internal/probe/queue"
)

// Runner defines the execution contract to run probe tasks for a ProbeRun.
type Runner interface {
	Run(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error
}

// NodeDialer establishes an HTTP client routed through a node's in-memory proxy outbound.
type NodeDialer func(ctx context.Context, node domain.Node) (*http.Client, func() error, error)

// DefaultRunnerOption configures DefaultRunner parameters.
type DefaultRunnerOption func(*DefaultRunner)

// WithNodeDialer overrides the default sing-box dialer.
func WithNodeDialer(dialer NodeDialer) DefaultRunnerOption {
	return func(r *DefaultRunner) {
		r.dialer = dialer
	}
}

// WithRunnerClock configures a deterministic clock for testing.
func WithRunnerClock(clock func() time.Time) DefaultRunnerOption {
	return func(r *DefaultRunner) {
		r.clock = clock
	}
}

// DefaultRunner coordinates probe task dispatching, singbox outbound dial, and observation persistence.
type RunBudget struct {
	MaxTasks         int
	MaxResponseBytes int64
	TaskTimeout      time.Duration
}

// MaxResponseBytes is enforced independently for each probe task, not across a run.
var DefaultRunBudget = RunBudget{MaxTasks: 10000, MaxResponseBytes: 16 << 20, TaskTimeout: 30 * time.Second}

var errResponseTooLarge = errors.New("probe response exceeds per-task byte limit")

func readBoundedResponse(body io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		limit = 0
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return data, err
	}
	if int64(len(data)) > limit {
		return data[:limit], errResponseTooLarge
	}
	return data, nil
}

func WithRunBudget(budget RunBudget) DefaultRunnerOption {
	return func(r *DefaultRunner) { r.budget = budget }
}

type DefaultRunner struct {
	budget       RunBudget
	nodes        domain.NodeRepository
	observations domain.ProbeObservationRepository
	scheduler    *queue.Scheduler
	runs         domain.ProbeRunRepository
	dialer       NodeDialer
	clock        func() time.Time
}

// NewDefaultRunner constructs a DefaultRunner.
func NewDefaultRunner(
	nodes domain.NodeRepository,
	observations domain.ProbeObservationRepository,
	scheduler *queue.Scheduler,
	runs domain.ProbeRunRepository,
	opts ...DefaultRunnerOption,
) *DefaultRunner {
	r := &DefaultRunner{
		nodes:        nodes,
		observations: observations,
		scheduler:    scheduler,
		runs:         runs,
		dialer:       NewSafeNodeDialer(),
		clock:        time.Now,
		budget:       DefaultRunBudget,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type tasksEnqueuedCtxKey struct{}

// WithTasksEnqueuedHook attaches a one-shot callback to ctx that is invoked as
// soon as DefaultRunner.Run has finished submitting initial tasks to the queue scheduler.
func WithTasksEnqueuedHook(ctx context.Context, fn func()) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return ctx
	}
	var once sync.Once
	return context.WithValue(ctx, tasksEnqueuedCtxKey{}, func() {
		once.Do(fn)
	})
}

func notifyTasksEnqueued(ctx context.Context) {
	if ctx == nil {
		return
	}
	if fn, ok := ctx.Value(tasksEnqueuedCtxKey{}).(func()); ok && fn != nil {
		fn()
	}
}

// Scheduler returns the underlying queue scheduler used by this runner.
func (r *DefaultRunner) Scheduler() *queue.Scheduler {
	if r == nil {
		return nil
	}
	return r.scheduler
}

// IsNodeInPool returns true if the node is currently probing or queued in the node pool.
func (r *DefaultRunner) IsNodeInPool(logicalID string) bool {
	if r == nil || r.scheduler == nil {
		return false
	}
	return r.scheduler.IsNodeInPool(logicalID)
}

// Sentinel errors for fatal probe dial configuration issues.
var (
	ErrCredentialsUnavailable    = errors.New("credentials_unavailable")
	ErrProbeDialingNotConfigured = errors.New("probe dialing not configured")
	ErrSpeedProbeOptInRequired   = errors.New("speed probe opt-in required")
)

func defaultNodeDialer(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
	return nil, nil, fmt.Errorf("%w: probe dialing not configured for node %s (%s): %w", ErrProbeDialingNotConfigured, node.DisplayName, node.LogicalID, ErrCredentialsUnavailable)
}

func profileForKind(kind domain.ProbeKind) profiles.Profile {
	switch kind {
	case domain.ProbeKindBaseline:
		return profiles.Baseline()
	case domain.ProbeKindGeo:
		return profiles.Geo()
	case domain.ProbeKindStreaming:
		return profiles.Streaming()
	case domain.ProbeKindAI:
		return profiles.AI()
	case domain.ProbeKindSpeed:
		return profiles.Speed()
	case domain.ProbeKindIPRisk:
		return profiles.IPRisk()
	default:
		return profiles.Baseline()
	}
}

func probeURLForKind(kind domain.ProbeKind) string {
	switch kind {
	case domain.ProbeKindBaseline:
		return "http://cp.cloudflare.com/generate_204"
	case domain.ProbeKindGeo:
		return "https://api.ip.sb/geoip"
	case domain.ProbeKindStreaming:
		return "https://www.netflix.com"
	case domain.ProbeKindAI:
		return "https://api.openai.com"
	case domain.ProbeKindSpeed:
		return "http://speed.cloudflare.com/__down?bytes=1048576"
	case domain.ProbeKindIPRisk:
		return "https://cloudflare.com/cdn-cgi/trace"
	default:
		return "http://cp.cloudflare.com/generate_204"
	}
}

// Run executes all node probing tasks for a ProbeRun.
func (r *DefaultRunner) Run(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error {
	if run == nil {
		return domain.NewValidationError("nil_run", "probe run is nil")
	}
	if r.scheduler == nil {
		return errors.New("scheduler is nil")
	}

	if run.IsTerminal() {
		return nil
	}
	if r.budget.MaxTasks <= 0 || r.budget.MaxResponseBytes <= 0 || r.budget.TaskTimeout <= 0 {
		return errors.New("invalid probe run budget")
	}

	if run.State == domain.ProbeRunStateQueued {
		if err := run.TransitionTo(domain.ProbeRunStateRunning); err != nil {
			return err
		}
		if err := r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateRunning); err != nil {
			return err
		}
	}

	var targetNodes []domain.Node
	if len(nodeIDs) > 0 {
		for _, id := range nodeIDs {
			node, err := r.nodes.GetByLogicalID(ctx, id)
			if err != nil || node == nil {
				continue
			}
			targetNodes = append(targetNodes, *node)
		}
	} else {
		const fetchPageSize = 100
		seen := make(map[string]struct{})
		for fetchPage := 1; ; fetchPage++ {
			chunk, total, err := r.nodes.List(ctx, domain.NodeFilter{
				ActiveOnly: true,
				Pagination: domain.Pagination{Page: fetchPage, PageSize: fetchPageSize},
			})
			if err != nil {
				_ = run.TransitionTo(domain.ProbeRunStateFailed)
				_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
				return err
			}
			added := 0
			for _, n := range chunk {
				if _, exists := seen[n.LogicalID]; !exists {
					seen[n.LogicalID] = struct{}{}
					targetNodes = append(targetNodes, n)
					added++
				}
			}
			if len(targetNodes) >= total || len(chunk) == 0 || added == 0 {
				break
			}
		}
	}

	if len(targetNodes) == 0 {
		_ = run.TransitionTo(domain.ProbeRunStateFailed)
		_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
		return errors.New("no target nodes available to probe")
	}

	var validKinds []domain.ProbeKind
	for _, k := range kinds {
		if k.IsValid() {
			validKinds = append(validKinds, k)
		}
	}
	if len(validKinds) == 0 {
		validKinds = []domain.ProbeKind{domain.ProbeKindBaseline}
	}
	// Bound the complete node × profile expansion before changing run state or dispatching.
	taskCount := len(targetNodes) * len(validKinds)
	if taskCount > r.budget.MaxTasks {
		_ = run.TransitionTo(domain.ProbeRunStateFailed)
		_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
		return fmt.Errorf("probe run task budget exceeded: %d > %d", taskCount, r.budget.MaxTasks)
	}
	runCtx := ctx
	if !run.DeadlineAt.IsZero() && run.DeadlineAt.After(r.clock().UTC()) {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithDeadline(ctx, run.DeadlineAt)
		defer cancel()
	}

	pool := newNodeSessionPool(runCtx)
	defer pool.closeAll()

	// Classify probe kinds: check if mixed (baseline + expensive)
	var hasBaseline bool
	var expensiveKinds []domain.ProbeKind
	for _, k := range validKinds {
		if k == domain.ProbeKindBaseline {
			hasBaseline = true
		} else {
			expensiveKinds = append(expensiveKinds, k)
		}
	}

	var (
		taskErrMu       sync.Mutex
		taskFatalErr    error
		hasFatalDialErr bool
	)

	recordTaskErr := func(err error) {
		if err == nil {
			return
		}
		taskErrMu.Lock()
		if taskFatalErr == nil {
			taskFatalErr = err
		}
		if isFatalDialError(err) {
			hasFatalDialErr = true
		}
		taskErrMu.Unlock()
	}

	handleCancelOrDeadline := func(done chan struct{}) error {
		r.scheduler.CancelRun(run.ID)
		<-done
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			_ = run.TransitionTo(domain.ProbeRunStateExpired)
			if err := r.runs.UpdateState(persistCtx, run.ID, domain.ProbeRunStateExpired); err != nil {
				return errors.Join(runCtx.Err(), err)
			}
			return runCtx.Err()
		}
		_ = run.TransitionTo(domain.ProbeRunStateCancelled)
		if err := r.runs.UpdateState(persistCtx, run.ID, domain.ProbeRunStateCancelled); err != nil {
			return errors.Join(runCtx.Err(), err)
		}
		return runCtx.Err()
	}

	enqueueMode := queue.EnqueueManualPreemptFront
	if run.ActorScope == "system:periodic-probe" {
		enqueueMode = queue.EnqueuePeriodicDedupe
	}

	isMixedTwoPhase := hasBaseline && len(expensiveKinds) > 0

	if !isMixedTwoPhase {
		// Single-stage dispatch: original behavior for baseline-only or non-baseline requests
		var (
			wg        sync.WaitGroup
			submitErr error
		)

		for _, node := range targetNodes {
			session := pool.sessionFor(node)
			session.retain(len(validKinds))
			for idx, kind := range validKinds {
				n := node
				k := kind
				s := session
				wg.Add(1)

				task := queue.Task{
					ID:        domain.MustNewUUIDv7(),
					RunID:     run.ID,
					LogicalID: n.LogicalID,
					Kind:      k,
					Mode:      enqueueMode,
					Context:   runCtx,
					Execute: func(tCtx context.Context) error {
						err := r.executeTask(tCtx, run, n, k, s)
						if err != nil {
							recordTaskErr(err)
						}
						return err
					},
					OnComplete: func(_ error) {
						s.release(1)
						wg.Done()
					},
				}

				if err := r.scheduler.SubmitWithContext(runCtx, task); err != nil {
					s.release(len(validKinds) - idx)
					wg.Done()
					submitErr = err
					break
				}
			}
			if submitErr != nil {
				break
			}
		}
		notifyTasksEnqueued(ctx)

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-runCtx.Done():
			return handleCancelOrDeadline(done)
		case <-done:
			if runCtx.Err() != nil {
				return handleCancelOrDeadline(done)
			}
			if submitErr != nil {
				_ = run.TransitionTo(domain.ProbeRunStateFailed)
				_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
				return submitErr
			}
			taskErrMu.Lock()
			fatalErr := taskFatalErr
			fatalDial := hasFatalDialErr
			taskErrMu.Unlock()

			if fatalDial {
				_ = run.TransitionTo(domain.ProbeRunStateFailed)
				_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
				return fatalErr
			}

			_ = run.TransitionTo(domain.ProbeRunStateSucceeded)
			_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateSucceeded)
			return nil
		}
	}

	// Two-phase execution: Phase 1 (Baseline) -> Collect Available -> Phase 2 (Expensive)
	var (
		stage1WG        sync.WaitGroup
		stage1SubmitErr error
		availMu         sync.Mutex
		availableMap    = make(map[string]domain.Node)
	)

	for _, node := range targetNodes {
		n := node
		k := domain.ProbeKindBaseline
		s := pool.sessionFor(n)
		stage1WG.Add(1)

		task := queue.Task{
			ID:        domain.MustNewUUIDv7(),
			RunID:     run.ID,
			LogicalID: n.LogicalID,
			Kind:      k,
			Mode:      enqueueMode,
			Context:   runCtx,
			Execute: func(tCtx context.Context) error {
				verdict, err := r.executeTaskWithVerdict(tCtx, run, n, k, s)
				if err != nil {
					recordTaskErr(err)
				}
				if verdict == domain.VerdictAvailable {
					availMu.Lock()
					availableMap[n.LogicalID] = n
					availMu.Unlock()
				}
				return err
			},
			OnComplete: func(_ error) {
				stage1WG.Done()
			},
		}

		if err := r.scheduler.SubmitWithContext(runCtx, task); err != nil {
			stage1WG.Done()
			stage1SubmitErr = err
			break
		}
	}
	notifyTasksEnqueued(ctx)

	stage1Done := make(chan struct{})
	go func() {
		stage1WG.Wait()
		close(stage1Done)
	}()

	select {
	case <-runCtx.Done():
		return handleCancelOrDeadline(stage1Done)
	case <-stage1Done:
		if runCtx.Err() != nil {
			return handleCancelOrDeadline(stage1Done)
		}
		if stage1SubmitErr != nil {
			_ = run.TransitionTo(domain.ProbeRunStateFailed)
			_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
			return stage1SubmitErr
		}
		taskErrMu.Lock()
		fatalDial := hasFatalDialErr
		fatalErr := taskFatalErr
		taskErrMu.Unlock()

		if fatalDial {
			_ = run.TransitionTo(domain.ProbeRunStateFailed)
			_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
			return fatalErr
		}
	}

	// Filter nodes for Phase 2: only those that passed baseline with VerdictAvailable
	availMu.Lock()
	var stage2Nodes []domain.Node
	for _, n := range targetNodes {
		if _, ok := availableMap[n.LogicalID]; ok {
			stage2Nodes = append(stage2Nodes, n)
		} else {
			_ = pool.sessionFor(n).close()
		}
	}
	availMu.Unlock()

	if len(stage2Nodes) == 0 {
		// An unavailable or restricted baseline is a successful observation, not
		// an execution failure. The run is complete once every baseline result
		// has been persisted and there is no submission, fatal dial, or context error.
		_ = run.TransitionTo(domain.ProbeRunStateSucceeded)
		_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateSucceeded)
		return nil
	}

	// Phase 2: submit expensive kinds only for available nodes
	var (
		stage2WG        sync.WaitGroup
		stage2SubmitErr error
	)

	for _, node := range stage2Nodes {
		session := pool.sessionFor(node)
		session.retain(len(expensiveKinds))
		for idx, kind := range expensiveKinds {
			n := node
			k := kind
			s := session
			stage2WG.Add(1)

			task := queue.Task{
				ID:        domain.MustNewUUIDv7(),
				RunID:     run.ID,
				LogicalID: n.LogicalID,
				Kind:      k,
				Mode:      enqueueMode,
				Context:   runCtx,
				Execute: func(tCtx context.Context) error {
					err := r.executeTask(tCtx, run, n, k, s)
					if err != nil {
						recordTaskErr(err)
					}
					return err
				},
				OnComplete: func(_ error) {
					s.release(1)
					stage2WG.Done()
				},
			}

			if err := r.scheduler.SubmitWithContext(runCtx, task); err != nil {
				s.release(len(expensiveKinds) - idx)
				stage2WG.Done()
				stage2SubmitErr = err
				break
			}
		}
		if stage2SubmitErr != nil {
			break
		}
	}

	stage2Done := make(chan struct{})
	go func() {
		stage2WG.Wait()
		close(stage2Done)
	}()

	select {
	case <-runCtx.Done():
		return handleCancelOrDeadline(stage2Done)
	case <-stage2Done:
		if runCtx.Err() != nil {
			return handleCancelOrDeadline(stage2Done)
		}
		if stage2SubmitErr != nil {
			_ = run.TransitionTo(domain.ProbeRunStateFailed)
			_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
			return stage2SubmitErr
		}
		taskErrMu.Lock()
		fatalErr := taskFatalErr
		fatalDial := hasFatalDialErr
		taskErrMu.Unlock()

		if fatalDial {
			_ = run.TransitionTo(domain.ProbeRunStateFailed)
			_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
			return fatalErr
		}

		_ = run.TransitionTo(domain.ProbeRunStateSucceeded)
		_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateSucceeded)
		return nil
	}
}

type nodeSession struct {
	node          domain.Node
	sessionCtx    context.Context
	sessionCancel context.CancelFunc

	dialOnce sync.Once
	client   *http.Client
	cleanup  func() error
	dialErr  error
	dialed   bool

	mu           sync.Mutex
	pendingTasks int

	closeOnce sync.Once
	closeErr  error
}

func (s *nodeSession) retain(n int) {
	if s == nil || n <= 0 {
		return
	}
	s.mu.Lock()
	s.pendingTasks += n
	s.mu.Unlock()
}

func (s *nodeSession) release(n int) {
	if s == nil || n <= 0 {
		return
	}
	s.mu.Lock()
	s.pendingTasks -= n
	shouldClose := s.pendingTasks <= 0
	if s.pendingTasks < 0 {
		s.pendingTasks = 0
	}
	s.mu.Unlock()
	if shouldClose {
		_ = s.close()
	}
}

func (s *nodeSession) getOrCreateClient(dialer NodeDialer) (*http.Client, error) {
	if s == nil {
		return nil, errors.New("nil node session")
	}
	s.dialOnce.Do(func() {
		if dialer == nil {
			dialer = defaultNodeDialer
		}
		client, cleanup, err := dialer(s.sessionCtx, s.node)
		if err == nil && client != nil && client.CheckRedirect == nil {
			client = cloneClient(client)
			client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			}
		}
		s.client = client
		s.cleanup = cleanup
		s.dialErr = err
		s.dialed = true
	})
	if !s.dialed {
		return nil, context.Canceled
	}
	return s.client, s.dialErr
}

func (s *nodeSession) close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if s.sessionCancel != nil {
			s.sessionCancel()
		}
		s.dialOnce.Do(func() {})
		if s.cleanup != nil {
			s.closeErr = s.cleanup()
		}
	})
	return s.closeErr
}

type nodeSessionPool struct {
	runCtx   context.Context
	mu       sync.Mutex
	sessions map[string]*nodeSession
}

func newNodeSessionPool(runCtx context.Context) *nodeSessionPool {
	if runCtx == nil {
		runCtx = context.Background()
	}
	return &nodeSessionPool{
		runCtx:   runCtx,
		sessions: make(map[string]*nodeSession),
	}
}

func (p *nodeSessionPool) sessionFor(node domain.Node) *nodeSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.sessions[node.LogicalID]; ok {
		return s
	}
	sessionCtx, sessionCancel := context.WithCancel(p.runCtx)
	s := &nodeSession{
		node:          node,
		sessionCtx:    sessionCtx,
		sessionCancel: sessionCancel,
	}
	p.sessions[node.LogicalID] = s
	return s
}

func (p *nodeSessionPool) closeAll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	sessions := make([]*nodeSession, 0, len(p.sessions))
	for _, s := range p.sessions {
		sessions = append(sessions, s)
	}
	p.mu.Unlock()

	for _, s := range sessions {
		_ = s.close()
	}
}

func cloneClient(client *http.Client) *http.Client {
	copy := *client
	return &copy
}

func isFatalDialError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrCredentialsUnavailable) || errors.Is(err, ErrProbeDialingNotConfigured)
}

func (r *DefaultRunner) executeTask(ctx context.Context, run *domain.ProbeRun, node domain.Node, kind domain.ProbeKind, session *nodeSession) error {
	_, err := r.executeTaskWithVerdict(ctx, run, node, kind, session)
	return err
}

func (r *DefaultRunner) executeTaskWithVerdict(ctx context.Context, run *domain.ProbeRun, node domain.Node, kind domain.ProbeKind, session *nodeSession) (domain.ProbeVerdict, error) {
	prof := profileForKind(kind)
	start := r.clock()
	timeout := r.budget.TaskTimeout
	if kind == domain.ProbeKindSpeed && prof.SpeedBudget.Deadline > 0 && prof.SpeedBudget.Deadline < timeout {
		timeout = prof.SpeedBudget.Deadline
	}
	taskCtx, taskCancel := context.WithTimeout(ctx, timeout)
	defer taskCancel()
	ctx = taskCtx

	var (
		result     profiles.Result
		latency    int64
		geoCountry string
	)
	if kind == domain.ProbeKindSpeed {
		result.OptIn = true
	}

	var (
		client  *http.Client
		dialErr error
	)
	if session != nil {
		client, dialErr = session.getOrCreateClient(r.dialer)
	} else {
		var cleanup func() error
		client, cleanup, dialErr = r.dialer(ctx, node)
		if cleanup != nil {
			defer cleanup()
		}
		if dialErr == nil && client != nil && client.CheckRedirect == nil {
			client = cloneClient(client)
			client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
		}
	}

	if dialErr != nil {
		result.NetworkError = true
		latency = r.clock().Sub(start).Milliseconds()
	} else if client == nil {
		result.NetworkError = true
		latency = r.clock().Sub(start).Milliseconds()
	} else {
		reqURL := probeURLForKind(kind)
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if reqErr != nil {
			result.NetworkError = true
			latency = r.clock().Sub(start).Milliseconds()
		} else {
			req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CSP-Probe/1.0)")
			reqStart := r.clock()
			req = req.WithContext(ctx)
			resp, doErr := client.Do(req)

			if doErr != nil {
				latency = r.clock().Sub(reqStart).Milliseconds()
				if errors.Is(doErr, context.DeadlineExceeded) || (ctx.Err() == context.DeadlineExceeded) {
					result.DeadlineExceeded = true
				} else {
					var dnsErr *net.DNSError
					if errors.As(doErr, &dnsErr) {
						result.DNSError = true
					} else {
						result.NetworkError = true
					}
				}
			} else if resp == nil || resp.Body == nil {
				latency = r.clock().Sub(reqStart).Milliseconds()
				result.NetworkError = true
			} else {
				readLimit := r.budget.MaxResponseBytes
				speedCapEnforced := false
				if kind == domain.ProbeKindSpeed && prof.SpeedBudget.MaxBytesPerRequest > 0 && prof.SpeedBudget.MaxBytesPerRequest < readLimit {
					readLimit = prof.SpeedBudget.MaxBytesPerRequest
					speedCapEnforced = true
				}
				body, readErr := readBoundedResponse(resp.Body, readLimit)
				_ = resp.Body.Close()
				latency = r.clock().Sub(reqStart).Milliseconds()

				if readErr != nil {
					if speedCapEnforced && errors.Is(readErr, errResponseTooLarge) {
						result.StatusCode = resp.StatusCode
						result.Body = body
						result.BytesRead = prof.SpeedBudget.MaxBytesPerRequest + 1
						result.ContractVersion = prof.Contract
					} else if errors.Is(readErr, context.DeadlineExceeded) || (ctx.Err() == context.DeadlineExceeded) {
						result.DeadlineExceeded = true
						result.BytesRead = int64(len(body))
					} else {
						result.NetworkError = true
						result.BytesRead = int64(len(body))
					}
				} else {
					result.StatusCode = resp.StatusCode
					result.Body = body
					result.BytesRead = int64(len(body))
					result.ContractVersion = prof.Contract

					switch kind {
					case domain.ProbeKindBaseline:
						result.ContractMatched = resp.StatusCode == http.StatusNoContent && len(body) == 0 && result.BytesRead == 0
					case domain.ProbeKindGeo:
						if resp.StatusCode == http.StatusOK {
							if cand, err := identity.ExtractCandidate(body); err == nil {
								result.ContractMatched = true
								geoCountry = cand.NormalizedCountryCode()
							}
						}
					case domain.ProbeKindStreaming, domain.ProbeKindAI:
						result.ContractMatched = false
					case domain.ProbeKindIPRisk:
						result.ContractMatched = false
						if resp.StatusCode >= 200 && resp.StatusCode < 400 {
							if resp.StatusCode != http.StatusOK {
								if prof.Evaluate(result).Reason != "access_restricted" {
									result.ExitIdentityMissing = true
								}
							} else if _, err := identity.ExtractCandidate(body); err != nil {
								if prof.Evaluate(result).Reason != "access_restricted" {
									result.ExitIdentityMissing = true
								}
							}
						}
					case domain.ProbeKindSpeed:
						result.ContractMatched = resp.StatusCode == http.StatusOK && result.BytesRead >= profiles.MinValidSpeedBytes
					default:
						result.ContractMatched = false
					}
				}
			}
		}
	}

	eval := prof.Evaluate(result)
	now := r.clock().UTC()
	summary := fmt.Sprintf("profile=%s version=%s verdict=%s reason=%s status=%d latency_ms=%d",
		prof.Kind, prof.Version, eval.Verdict, eval.Reason, result.StatusCode, latency)
	if kind == domain.ProbeKindGeo && geoCountry != "" {
		summary = fmt.Sprintf("%s country=%s", summary, geoCountry)
	}
	if kind == domain.ProbeKindSpeed && result.BytesRead > 0 {
		throughputKbps := int64(0)
		if latency > 0 {
			throughputKbps = (result.BytesRead * 8) / latency
		}
		summary = fmt.Sprintf("%s bytes_read=%d throughput_kbps=%d", summary, result.BytesRead, throughputKbps)
	}
	if dialErr != nil && (errors.Is(dialErr, ErrCredentialsUnavailable) || strings.Contains(dialErr.Error(), "credentials_unavailable")) {
		summary = summary + " error=credentials_unavailable"
	}
	obs := &domain.ProbeObservation{
		ID:              domain.MustNewUUIDv7(),
		ProbeRunID:      run.ID,
		NodeLogicalID:   node.LogicalID,
		Kind:            kind,
		Verdict:         eval.Verdict,
		EvidenceDigest:  domain.ComputeProbeEvidenceDigest(run.ID, node.LogicalID, prof.Version, eval.Verdict, result.StatusCode, eval.Reason),
		ObservedAt:      now,
		LatencyMS:       latency,
		RedactedSummary: summary,
	}

	if createErr := r.observations.Create(ctx, obs); createErr != nil {
		return eval.Verdict, createErr
	}
	if dialErr != nil && isFatalDialError(dialErr) {
		return eval.Verdict, dialErr
	}
	return eval.Verdict, nil
}
