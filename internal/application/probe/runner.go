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
	"clash-sub-parser/internal/probe/mihomo"
	"clash-sub-parser/internal/probe/platform"
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
// StageConcurrency defines per-stage concurrency bounds for the probe pipeline,
// aligned with upstream subs-check defaults (alive: 50, media: 20, speed: 8).
type StageConcurrency struct {
	Alive int
	Media int
	Speed int
}

// DefaultStageConcurrency matches upstream subs-check configuration defaults.
var DefaultStageConcurrency = StageConcurrency{
	Alive: 50,
	Media: 20,
	Speed: 8,
}

// WithStageConcurrency configures independent stage concurrency limits.
func WithStageConcurrency(sc StageConcurrency) DefaultRunnerOption {
	return func(r *DefaultRunner) {
		r.stageConcurrency = sc
	}
}

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

// WithIPRiskObservationRepository configures an optional IP risk repository for recording 1:N risk observations.
func WithIPRiskObservationRepository(repo domain.IPRiskObservationRepository) DefaultRunnerOption {
	return func(r *DefaultRunner) { r.riskObs = repo }
}

// WithNodeScope configures the node scope for selecting target nodes (default: enabled_subscriptions).
func WithNodeScope(scope domain.NodeScope) DefaultRunnerOption {
	return func(r *DefaultRunner) { r.scope = scope }
}

// WithSpeedBudget overrides the default speed probe per-node byte and deadline limits.
func WithSpeedBudget(maxBytesPerNode int64, deadline time.Duration) DefaultRunnerOption {
	return func(r *DefaultRunner) {
		r.speedBudget = &profiles.SpeedBudget{
			OptInRequired:      true,
			MaxBytesPerRequest: maxBytesPerNode,
			Deadline:           deadline,
		}
	}
}

type DefaultRunner struct {
	budget           RunBudget
	stageConcurrency StageConcurrency
	speedBudget      *profiles.SpeedBudget
	nodes            domain.NodeRepository
	observations     domain.ProbeObservationRepository
	riskObs          domain.IPRiskObservationRepository
	scheduler        *queue.Scheduler
	runs             domain.ProbeRunRepository
	dialer           NodeDialer
	clock            func() time.Time
	scope            domain.NodeScope
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
		nodes:            nodes,
		observations:     observations,
		scheduler:        scheduler,
		runs:             runs,
		dialer:           mihomo.NewNodeDialer(15 * time.Second),
		clock:            time.Now,
		budget:           DefaultRunBudget,
		stageConcurrency: DefaultStageConcurrency,
		scope:            domain.NodeScopeEnabledSubscriptions,
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

// Sentinel errors for fatal probe dial configuration issues and legacy failure classification.
var (
	ErrCredentialsUnavailable    = domain.ErrCredentialsUnavailable
	ErrProbeDialingNotConfigured = errors.New("probe dialing not configured")
	ErrSpeedProbeOptInRequired   = errors.New("speed probe opt-in required")
	ErrTargetUnresolvable        = errors.New("target_unresolvable")
	ErrPrivateTargetRejected     = errors.New("private_target_rejected")
	ErrUnsafeTLSRejected         = errors.New("unsafe_tls_rejected")
	ErrUnsafeOptionRejected      = errors.New("unsafe_option_rejected")
	ErrClientBuildFailed         = domain.ErrClientBuildFailed
)

func defaultNodeDialer(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
	return nil, nil, ErrProbeDialingNotConfigured
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
		return platform.DefaultSpeedTestURL
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

	targetScope := r.scope
	if targetScope == "" {
		targetScope = domain.NodeScopeEnabledSubscriptions
	}

	var targetNodes []domain.Node
	if len(nodeIDs) > 0 {
		chunk, _, err := r.nodes.List(ctx, domain.NodeFilter{
			Scope:          targetScope,
			ActiveOnly:     true,
			LogicalIDs:     nodeIDs,
			ExcludeNotices: true,
			Pagination:     domain.Pagination{Page: 1, PageSize: len(nodeIDs)},
		})
		if err != nil {
			_ = run.TransitionTo(domain.ProbeRunStateFailed)
			_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
			return err
		}
		chunkMap := make(map[string]domain.Node, len(chunk))
		for _, n := range chunk {
			chunkMap[n.LogicalID] = n
		}
		for _, id := range nodeIDs {
			if n, ok := chunkMap[id]; ok {
				targetNodes = append(targetNodes, n)
			}
		}
	} else {
		const fetchPageSize = 100
		seen := make(map[string]struct{})
		for fetchPage := 1; ; fetchPage++ {
			chunk, total, err := r.nodes.List(ctx, domain.NodeFilter{
				Scope:          targetScope,
				ActiveOnly:     true,
				ExcludeNotices: true,
				Pagination:     domain.Pagination{Page: fetchPage, PageSize: fetchPageSize},
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

	// Classify probe kinds into 3 pipeline stages:
	// Stage 1 (Alive): Baseline
	// Stage 2 (Media): Geo, Streaming, AI, IPRisk
	// Stage 3 (Speed): Speed
	var (
		stage1Kinds []domain.ProbeKind
		stage2Kinds []domain.ProbeKind
		stage3Kinds []domain.ProbeKind
	)
	for _, k := range validKinds {
		switch k {
		case domain.ProbeKindBaseline:
			stage1Kinds = append(stage1Kinds, k)
		case domain.ProbeKindSpeed:
			stage3Kinds = append(stage3Kinds, k)
		default:
			stage2Kinds = append(stage2Kinds, k)
		}
	}

	var (
		taskErrMu          sync.Mutex
		taskFatalErr       error
		hasSystemicDialErr bool
		fatalDialNodes     = make(map[string]struct{})
	)

	recordTaskErr := func(nodeLogicalID string, err error) {
		if err == nil {
			return
		}
		taskErrMu.Lock()
		if taskFatalErr == nil {
			taskFatalErr = err
		}
		if errors.Is(err, ErrProbeDialingNotConfigured) {
			hasSystemicDialErr = true
		}
		if isFatalDialError(err) {
			fatalDialNodes[nodeLogicalID] = struct{}{}
		}
		taskErrMu.Unlock()
	}

	checkFatalDial := func() (bool, error) {
		taskErrMu.Lock()
		defer taskErrMu.Unlock()
		if hasSystemicDialErr || (len(targetNodes) > 0 && len(fatalDialNodes) == len(targetNodes)) {
			return true, taskFatalErr
		}
		return false, nil
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

	var enqueuedOnce sync.Once
	triggerNotify := func() {
		enqueuedOnce.Do(func() {
			notifyTasksEnqueued(ctx)
		})
	}

	// ---------------------------------------------------------
	// Stage 1: Alive (Baseline)
	// ---------------------------------------------------------
	var (
		availMu      sync.Mutex
		availableMap = make(map[string]domain.Node)
	)

	if len(stage1Kinds) > 0 {
		var (
			stage1WG        sync.WaitGroup
			stage1SubmitErr error
		)
		stage1Limit := r.stageConcurrency.Alive
		if stage1Limit <= 0 {
			stage1Limit = DefaultStageConcurrency.Alive
		}
		stage1Sem := make(chan struct{}, stage1Limit)

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
					select {
					case stage1Sem <- struct{}{}:
						defer func() { <-stage1Sem }()
					case <-tCtx.Done():
						return tCtx.Err()
					}
					verdict, err := r.executeTaskWithVerdict(tCtx, run, n, k, s)
					if err != nil {
						recordTaskErr(n.LogicalID, err)
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
		triggerNotify()

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
			if fatalDial, fatalErr := checkFatalDial(); fatalDial {
				_ = run.TransitionTo(domain.ProbeRunStateFailed)
				_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
				return fatalErr
			}
		}
	}

	// Filter nodes eligible for Stage 2 & Stage 3:
	// If Baseline was executed, only nodes with VerdictAvailable advance.
	// Nodes that failed baseline are closed immediately.
	var eligibleNodes []domain.Node
	if len(stage1Kinds) > 0 {
		availMu.Lock()
		for _, n := range targetNodes {
			if _, ok := availableMap[n.LogicalID]; ok {
				eligibleNodes = append(eligibleNodes, n)
			} else {
				_ = pool.sessionFor(n).close()
			}
		}
		availMu.Unlock()
	} else {
		eligibleNodes = targetNodes
	}

	if len(eligibleNodes) == 0 || (len(stage2Kinds) == 0 && len(stage3Kinds) == 0) {
		_ = run.TransitionTo(domain.ProbeRunStateSucceeded)
		_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateSucceeded)
		return nil
	}

	// Retain sessions for all tasks across subsequent stages (Stage 2 + Stage 3)
	tasksPerNode := len(stage2Kinds) + len(stage3Kinds)
	for _, n := range eligibleNodes {
		pool.sessionFor(n).retain(tasksPerNode)
	}

	// ---------------------------------------------------------
	// Stage 2: Media / AI (Geo, Streaming, AI, IPRisk)
	// ---------------------------------------------------------
	if len(stage2Kinds) > 0 {
		var (
			stage2WG        sync.WaitGroup
			stage2SubmitErr error
		)
		stage2Limit := r.stageConcurrency.Media
		if stage2Limit <= 0 {
			stage2Limit = DefaultStageConcurrency.Media
		}
		stage2Sem := make(chan struct{}, stage2Limit)

		for _, node := range eligibleNodes {
			session := pool.sessionFor(node)
			for idx, kind := range stage2Kinds {
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
						select {
						case stage2Sem <- struct{}{}:
							defer func() { <-stage2Sem }()
						case <-tCtx.Done():
							return tCtx.Err()
						}
						err := r.executeTask(tCtx, run, n, k, s)
						if err != nil {
							recordTaskErr(n.LogicalID, err)
						}
						return err
					},
					OnComplete: func(_ error) {
						s.release(1)
						stage2WG.Done()
					},
				}

				if err := r.scheduler.SubmitWithContext(runCtx, task); err != nil {
					s.release(tasksPerNode - idx)
					stage2WG.Done()
					stage2SubmitErr = err
					break
				}
			}
			if stage2SubmitErr != nil {
				break
			}
		}
		triggerNotify()

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
			if fatalDial, fatalErr := checkFatalDial(); fatalDial {
				_ = run.TransitionTo(domain.ProbeRunStateFailed)
				_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
				return fatalErr
			}
		}
	}

	// ---------------------------------------------------------
	// Stage 3: Speed (Opt-in throughput test, after Stage 2 finishes)
	// ---------------------------------------------------------
	if len(stage3Kinds) > 0 {
		var (
			stage3WG        sync.WaitGroup
			stage3SubmitErr error
		)
		stage3Limit := r.stageConcurrency.Speed
		if stage3Limit <= 0 {
			stage3Limit = DefaultStageConcurrency.Speed
		}
		stage3Sem := make(chan struct{}, stage3Limit)

		for _, node := range eligibleNodes {
			session := pool.sessionFor(node)
			for idx, kind := range stage3Kinds {
				n := node
				k := kind
				s := session
				stage3WG.Add(1)

				task := queue.Task{
					ID:        domain.MustNewUUIDv7(),
					RunID:     run.ID,
					LogicalID: n.LogicalID,
					Kind:      k,
					Mode:      enqueueMode,
					Context:   runCtx,
					Execute: func(tCtx context.Context) error {
						select {
						case stage3Sem <- struct{}{}:
							defer func() { <-stage3Sem }()
						case <-tCtx.Done():
							return tCtx.Err()
						}
						err := r.executeTask(tCtx, run, n, k, s)
						if err != nil {
							recordTaskErr(n.LogicalID, err)
						}
						return err
					},
					OnComplete: func(_ error) {
						s.release(1)
						stage3WG.Done()
					},
				}

				if err := r.scheduler.SubmitWithContext(runCtx, task); err != nil {
					s.release(len(stage3Kinds) - idx)
					stage3WG.Done()
					stage3SubmitErr = err
					break
				}
			}
			if stage3SubmitErr != nil {
				break
			}
		}
		triggerNotify()

		stage3Done := make(chan struct{})
		go func() {
			stage3WG.Wait()
			close(stage3Done)
		}()

		select {
		case <-runCtx.Done():
			return handleCancelOrDeadline(stage3Done)
		case <-stage3Done:
			if runCtx.Err() != nil {
				return handleCancelOrDeadline(stage3Done)
			}
			if stage3SubmitErr != nil {
				_ = run.TransitionTo(domain.ProbeRunStateFailed)
				_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
				return stage3SubmitErr
			}
			if fatalDial, fatalErr := checkFatalDial(); fatalDial {
				_ = run.TransitionTo(domain.ProbeRunStateFailed)
				_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
				return fatalErr
			}
		}
	}

	_ = run.TransitionTo(domain.ProbeRunStateSucceeded)
	_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateSucceeded)
	return nil
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

// classifyDialFailure intentionally never formats the underlying error: runtime errors may
// contain server names, socket addresses or authentication material.
func classifyDialFailure(err error) string {
	switch {
	case errors.Is(err, ErrUnsafeTLSRejected):
		return "unsafe_tls_rejected"
	case errors.Is(err, ErrUnsafeOptionRejected):
		return "unsafe_option_rejected"
	case errors.Is(err, ErrPrivateTargetRejected):
		return "private_target_rejected"
	case errors.Is(err, ErrTargetUnresolvable):
		return "target_unresolvable"
	case errors.Is(err, ErrCredentialsUnavailable):
		return "credentials_unavailable"
	case errors.Is(err, ErrProbeDialingNotConfigured):
		return "probe_dialing_not_configured"
	case errors.Is(err, ErrClientBuildFailed):
		return "client_build_failed"
	default:
		return "client_build_failed"
	}
}

func isFatalDialError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrTargetUnresolvable) || errors.Is(err, ErrPrivateTargetRejected) {
		return false
	}
	return errors.Is(err, ErrCredentialsUnavailable) || errors.Is(err, ErrProbeDialingNotConfigured) || errors.Is(err, ErrClientBuildFailed)
}

func aggregateGroupVerdict(platforms map[string]domain.PlatformCapability) (domain.ProbeVerdict, string) {
	if len(platforms) == 0 {
		return domain.VerdictUnknown, "contract_drift"
	}
	hasAvailable := false
	allRestricted := true
	allError := true
	for _, p := range platforms {
		if p.Verdict == domain.VerdictAvailable {
			hasAvailable = true
		}
		if p.Verdict != domain.VerdictRestricted {
			allRestricted = false
		}
		if p.Verdict != domain.VerdictError {
			allError = false
		}
	}
	if hasAvailable {
		return domain.VerdictAvailable, "contract_matched"
	}
	if allRestricted {
		return domain.VerdictRestricted, "access_restricted"
	}
	if allError {
		return domain.VerdictError, "transport_error"
	}
	return domain.VerdictUnknown, "contract_drift"
}

func (r *DefaultRunner) executeTask(ctx context.Context, run *domain.ProbeRun, node domain.Node, kind domain.ProbeKind, session *nodeSession) error {
	_, err := r.executeTaskWithVerdict(ctx, run, node, kind, session)
	return err
}

func (r *DefaultRunner) executeTaskWithVerdict(ctx context.Context, run *domain.ProbeRun, node domain.Node, kind domain.ProbeKind, session *nodeSession) (domain.ProbeVerdict, error) {
	prof := profileForKind(kind)
	if kind == domain.ProbeKindSpeed && r.speedBudget != nil {
		if r.speedBudget.MaxBytesPerRequest > 0 {
			prof.SpeedBudget.MaxBytesPerRequest = r.speedBudget.MaxBytesPerRequest
		}
		if r.speedBudget.Deadline > 0 {
			prof.SpeedBudget.Deadline = r.speedBudget.Deadline
		}
	}
	start := r.clock()
	timeout := r.budget.TaskTimeout
	if kind == domain.ProbeKindSpeed && prof.SpeedBudget.Deadline > 0 {
		speedDeadline := prof.SpeedBudget.Deadline
		if speedDeadline+2*time.Second < timeout {
			timeout = speedDeadline + 2*time.Second
		}
	}
	taskCtx, taskCancel := context.WithTimeout(ctx, timeout)
	defer taskCancel()
	ctx = taskCtx

	var (
		result          profiles.Result
		latency         int64
		geoCountry      string
		failureReason   string
		platformsMap    map[string]domain.PlatformCapability
		speedCap        domain.PlatformCapability
		speedThroughput *float64
		ipRiskScore     string
		ipRiskCap       domain.PlatformCapability
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
		failureReason = classifyDialFailure(dialErr)
		result.NetworkError = true
		latency = r.clock().Sub(start).Milliseconds()
	} else if client == nil {
		failureReason = "client_build_failed"
		result.NetworkError = true
		latency = r.clock().Sub(start).Milliseconds()
	} else if kind == domain.ProbeKindStreaming {
		reqStart := r.clock()
		var (
			wg  sync.WaitGroup
			nf  domain.PlatformCapability
			yt  domain.PlatformCapability
			dis domain.PlatformCapability
		)
		wg.Add(3)
		go func() {
			defer wg.Done()
			nf = platform.CheckNetflix(ctx, client)
		}()
		go func() {
			defer wg.Done()
			yt = platform.CheckYoutube(ctx, client)
		}()
		go func() {
			defer wg.Done()
			dis = platform.CheckDisney(ctx, client)
		}()
		wg.Wait()
		latency = r.clock().Sub(reqStart).Milliseconds()
		platformsMap = map[string]domain.PlatformCapability{
			"netflix": nf,
			"youtube": yt,
			"disney":  dis,
		}
		result.ContractMatched = true
		result.ContractVersion = prof.Contract
	} else if kind == domain.ProbeKindAI {
		reqStart := r.clock()
		var (
			wg     sync.WaitGroup
			oai    domain.PlatformCapability
			claude domain.PlatformCapability
			gem    domain.PlatformCapability
		)
		wg.Add(3)
		go func() {
			defer wg.Done()
			oai = platform.CheckOpenAI(ctx, client)
		}()
		go func() {
			defer wg.Done()
			claude = platform.CheckClaude(ctx, client)
		}()
		go func() {
			defer wg.Done()
			gem = platform.CheckGemini(ctx, client)
		}()
		wg.Wait()
		latency = r.clock().Sub(reqStart).Milliseconds()
		platformsMap = map[string]domain.PlatformCapability{
			"openai": oai,
			"claude": claude,
			"gemini": gem,
		}
		result.ContractMatched = true
		result.ContractVersion = prof.Contract
	} else if kind == domain.ProbeKindSpeed {
		reqStart := r.clock()
		limitBytes := uint64(platform.DefaultDownloadMB * 1024 * 1024)
		if prof.SpeedBudget.MaxBytesPerRequest > 0 {
			limitBytes = uint64(prof.SpeedBudget.MaxBytesPerRequest)
		}
		deadline := platform.DefaultSpeedTimeout
		if prof.SpeedBudget.Deadline > 0 {
			deadline = prof.SpeedBudget.Deadline
		}
		var speedErr error
		speedCap, speedErr = platform.CheckSpeed(ctx, client, nil, "", limitBytes, deadline)
		latency = r.clock().Sub(reqStart).Milliseconds()
		if latency <= 0 && speedCap.LatencyMS != nil {
			latency = *speedCap.LatencyMS
		}
		var actualBytes int64
		if idx := strings.Index(speedCap.Summary, "bytes_read="); idx != -1 {
			_, _ = fmt.Sscanf(speedCap.Summary[idx:], "bytes_read=%d", &actualBytes)
		}
		speedThroughput = speedCap.Throughput
		if actualBytes > 0 {
			result.BytesRead = actualBytes
			if latency > 0 {
				kbps := (float64(actualBytes) / 1024.0) * 1000.0 / float64(latency)
				speedThroughput = &kbps
				throughputKbps := (actualBytes * 8) / latency
				budgetTag := ""
				if strings.Contains(speedCap.Summary, "[speed_budget_limited]") {
					budgetTag = " [speed_budget_limited]"
				}
				speedCap.Summary = fmt.Sprintf("%.1f KB/s (read %d bytes in %d ms) bytes_read=%d throughput_kbps=%d%s",
					kbps, actualBytes, latency, actualBytes, throughputKbps, budgetTag)
			}
		}
		if speedErr != nil {
			failureReason = speedCap.Reason
			if errors.Is(speedErr, context.DeadlineExceeded) || (ctx.Err() == context.DeadlineExceeded) {
				result.DeadlineExceeded = true
			} else {
				result.NetworkError = true
			}
		} else if speedCap.Verdict != domain.VerdictAvailable && speedCap.Reason != "" {
			failureReason = speedCap.Reason
		}
		result.ContractMatched = (speedCap.Verdict == domain.VerdictAvailable)
		result.ContractVersion = prof.Contract
		if speedCap.Verdict == domain.VerdictAvailable {
			result.StatusCode = http.StatusOK
		} else if idx := strings.Index(speedCap.Summary, "HTTP "); idx != -1 {
			var code int
			if _, err := fmt.Sscanf(speedCap.Summary[idx:], "HTTP %d", &code); err == nil {
				result.StatusCode = code
			}
		}
	} else if kind == domain.ProbeKindIPRisk {
		reqStart := r.clock()
		ipRiskCap = platform.CheckIPRisk(ctx, client, "")
		latency = r.clock().Sub(reqStart).Milliseconds()
		if latency <= 0 && ipRiskCap.LatencyMS != nil {
			latency = *ipRiskCap.LatencyMS
		}
		ipRiskScore = ipRiskCap.RiskScore
		if ipRiskCap.Verdict == domain.VerdictAvailable {
			result.ContractMatched = true
			result.StatusCode = http.StatusOK
		} else {
			result.ContractMatched = false
			failureReason = ipRiskCap.Reason
			if ipRiskCap.Verdict == domain.VerdictError {
				result.NetworkError = true
			}
		}
	} else {
		reqURL := probeURLForKind(kind)
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if reqErr != nil {
			failureReason = "request_build_failed"
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
					failureReason = "timeout"
					result.DeadlineExceeded = true
				} else {
					var dnsErr *net.DNSError
					if errors.As(doErr, &dnsErr) {
						failureReason = "dns_error"
						result.DNSError = true
					} else {
						failureReason = "transport_error"
						result.NetworkError = true
					}
				}
			} else if resp == nil || resp.Body == nil {
				latency = r.clock().Sub(reqStart).Milliseconds()
				failureReason = "transport_error"
				result.NetworkError = true
			} else {
				readLimit := r.budget.MaxResponseBytes
				body, readErr := readBoundedResponse(resp.Body, readLimit)
				_ = resp.Body.Close()
				latency = r.clock().Sub(reqStart).Milliseconds()

				if readErr != nil {
					if errors.Is(readErr, context.DeadlineExceeded) || (ctx.Err() == context.DeadlineExceeded) {
						failureReason = "timeout"
						result.DeadlineExceeded = true
						result.BytesRead = int64(len(body))
					} else {
						failureReason = "transport_error"
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
					case domain.ProbeKindIPRisk:
						if resp.StatusCode >= 200 && resp.StatusCode < 300 {
							cand, err := identity.ExtractCandidate(body)
							if err != nil || cand.CountryCode == "" {
								result.ExitIdentityMissing = true
								result.ContractMatched = false
							} else {
								result.ContractMatched = true
								ipRiskScore = cand.CountryCode
							}
						}
					default:
						result.ContractMatched = false
					}
				}
			}
		}
	}

	var eval profiles.Evaluation
	if kind == domain.ProbeKindSpeed {
		eval.Verdict = speedCap.Verdict
		eval.Reason = speedCap.Reason
	} else if kind == domain.ProbeKindIPRisk {
		eval.Verdict = ipRiskCap.Verdict
		eval.Reason = ipRiskCap.Reason
	} else {
		eval = prof.Evaluate(result)
		if len(platformsMap) > 0 {
			eval.Verdict, eval.Reason = aggregateGroupVerdict(platformsMap)
		}
	}
	if failureReason != "" {
		eval.Reason = failureReason
	}
	now := r.clock().UTC()
	summary := fmt.Sprintf("profile=%s version=%s verdict=%s reason=%s status=%d latency_ms=%d",
		prof.Kind, prof.Version, eval.Verdict, eval.Reason, result.StatusCode, latency)
	if kind == domain.ProbeKindGeo && geoCountry != "" {
		summary = fmt.Sprintf("%s country=%s", summary, geoCountry)
	}
	if kind == domain.ProbeKindSpeed {
		if speedCap.Summary != "" {
			summary = fmt.Sprintf("%s %s", summary, speedCap.Summary)
		}
		if speedThroughput != nil {
			summary = fmt.Sprintf("%s throughput_kbps=%.1f", summary, *speedThroughput)
		}
	}
	if kind == domain.ProbeKindIPRisk && ipRiskCap.Summary != "" {
		summary = fmt.Sprintf("%s summary=%s", summary, ipRiskCap.Summary)
	}
	if dialErr != nil {
		summary += " error=" + failureReason
	}
	obsSubTier := ""
	obsRegion := geoCountry
	if kind == domain.ProbeKindStreaming {
		if nf, ok := platformsMap["netflix"]; ok && nf.SubTier != "" {
			obsSubTier = nf.SubTier
			if nf.Region != "" {
				obsRegion = nf.Region
			}
		}
	} else if kind == domain.ProbeKindAI {
		if oai, ok := platformsMap["openai"]; ok && oai.SubTier != "" {
			obsSubTier = oai.SubTier
			if oai.Region != "" {
				obsRegion = oai.Region
			}
		}
	}

	var safeDetail *domain.SafeDetail
	if eval.Verdict != domain.VerdictAvailable || dialErr != nil || failureReason != "" {
		code := eval.Reason
		if code == "" {
			code = failureReason
		}
		stage := string(kind)
		if dialErr != nil {
			stage = "dial"
			if code == "" {
				code = "dial_failed"
			}
		}
		rawMap := map[string]any{
			"stage":     stage,
			"code":      code,
			"transport": string(node.Protocol),
			"status":    result.StatusCode,
			"timeout":   int(latency),
		}
		if coreVer := mihomo.CoreVersion(); coreVer != "" {
			rawMap["core"] = coreVer
		}
		sd := domain.SanitizeSafeDetail(rawMap)
		safeDetail = &sd
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
		Region:          obsRegion,
		SubTier:         obsSubTier,
		Throughput:      speedThroughput,
		RiskScore:       ipRiskScore,
		Platforms:       platformsMap,
		SafeDetail:      safeDetail,
	}

	if node.ConnectionRevision > 0 {
		revision := node.ConnectionRevision
		obs.ConnectionRevision = &revision
	}
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer persistCancel()
	if createErr := r.observations.Create(persistCtx, obs); createErr != nil {
		return eval.Verdict, createErr
	}
	if r.riskObs != nil && kind == domain.ProbeKindIPRisk {
		ipRiskStatus := domain.IPRiskStatusAvailable
		if eval.Verdict != domain.VerdictAvailable {
			ipRiskStatus = domain.IPRiskStatusError
		}
		var parsedScore *int
		if ipRiskScore != "" {
			var sc int
			if n, _ := fmt.Sscanf(ipRiskScore, "%d%%", &sc); n > 0 && sc >= 0 && sc <= 100 {
				parsedScore = &sc
			}
		}
		conf := 80
		ipRiskObs := &domain.IPRiskObservation{
			ID:                    domain.MustNewUUIDv7(),
			NodeLogicalID:         node.LogicalID,
			ExitIdentityDigest:    obs.EvidenceDigest,
			Provider:              "scamalytics",
			ProviderSchemaVersion: "v1",
			ObservedAt:            now,
			ExpiresAt:             now.Add(24 * time.Hour),
			Status:                ipRiskStatus,
			Score:                 parsedScore,
			Confidence:            &conf,
			NetworkClass:          domain.NetworkClassDatacenter,
			EvidenceDigest:        obs.EvidenceDigest,
			RedactedSummary:       obs.RedactedSummary,
			ProbeObservationID:    &obs.ID,
		}
		_ = r.riskObs.Create(persistCtx, ipRiskObs)
	}
	if dialErr != nil && isFatalDialError(dialErr) {
		return eval.Verdict, dialErr
	}
	return eval.Verdict, nil
}
