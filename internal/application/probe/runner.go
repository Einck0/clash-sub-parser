package probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
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
var DefaultRunBudget = RunBudget{MaxTasks: 512, MaxResponseBytes: 16 << 20, TaskTimeout: 30 * time.Second}

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
		dialer:       defaultNodeDialer,
		clock:        time.Now,
		budget:       DefaultRunBudget,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
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
		nodes, _, err := r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true})
		if err != nil {
			_ = run.TransitionTo(domain.ProbeRunStateFailed)
			_ = r.runs.UpdateState(ctx, run.ID, domain.ProbeRunStateFailed)
			return err
		}
		targetNodes = nodes
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

	isMixedTwoPhase := hasBaseline && len(expensiveKinds) > 0

	if !isMixedTwoPhase {
		// Single-stage dispatch: original behavior for baseline-only or non-baseline requests
		var (
			wg        sync.WaitGroup
			submitErr error
		)

		for _, node := range targetNodes {
			for _, kind := range validKinds {
				n := node
				k := kind
				wg.Add(1)

				task := queue.Task{
					ID:        domain.MustNewUUIDv7(),
					RunID:     run.ID,
					LogicalID: n.LogicalID,
					Kind:      k,
					Context:   runCtx,
					Execute: func(tCtx context.Context) error {
						err := r.executeTask(tCtx, run, n, k)
						if err != nil {
							recordTaskErr(err)
						}
						return err
					},
					OnComplete: func(_ error) {
						wg.Done()
					},
				}

				if err := r.scheduler.SubmitWithContext(runCtx, task); err != nil {
					wg.Done()
					submitErr = err
					break
				}
			}
			if submitErr != nil {
				break
			}
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-runCtx.Done():
			return handleCancelOrDeadline(done)
		case <-done:
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
		stage1WG.Add(1)

		task := queue.Task{
			ID:        domain.MustNewUUIDv7(),
			RunID:     run.ID,
			LogicalID: n.LogicalID,
			Kind:      k,
			Context:   runCtx,
			Execute: func(tCtx context.Context) error {
				verdict, err := r.executeTaskWithVerdict(tCtx, run, n, k)
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

	stage1Done := make(chan struct{})
	go func() {
		stage1WG.Wait()
		close(stage1Done)
	}()

	select {
	case <-runCtx.Done():
		return handleCancelOrDeadline(stage1Done)
	case <-stage1Done:
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
		for _, kind := range expensiveKinds {
			n := node
			k := kind
			stage2WG.Add(1)

			task := queue.Task{
				ID:        domain.MustNewUUIDv7(),
				RunID:     run.ID,
				LogicalID: n.LogicalID,
				Kind:      k,
				Context:   runCtx,
				Execute: func(tCtx context.Context) error {
					err := r.executeTask(tCtx, run, n, k)
					if err != nil {
						recordTaskErr(err)
					}
					return err
				},
				OnComplete: func(_ error) {
					stage2WG.Done()
				},
			}

			if err := r.scheduler.Submit(task); err != nil {
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

func (r *DefaultRunner) executeTask(ctx context.Context, run *domain.ProbeRun, node domain.Node, kind domain.ProbeKind) error {
	_, err := r.executeTaskWithVerdict(ctx, run, node, kind)
	return err
}

func (r *DefaultRunner) executeTaskWithVerdict(ctx context.Context, run *domain.ProbeRun, node domain.Node, kind domain.ProbeKind) (domain.ProbeVerdict, error) {
	prof := profileForKind(kind)
	start := r.clock()
	taskCtx, taskCancel := context.WithTimeout(ctx, r.budget.TaskTimeout)
	defer taskCancel()
	ctx = taskCtx

	var (
		result  profiles.Result
		latency int64
	)

	if kind == domain.ProbeKindSpeed {
		result = profiles.Result{}
		eval := prof.Evaluate(result)
		now := r.clock().UTC()
		obs := &domain.ProbeObservation{
			ID:              domain.MustNewUUIDv7(),
			ProbeRunID:      run.ID,
			NodeLogicalID:   node.LogicalID,
			Kind:            kind,
			Verdict:         eval.Verdict,
			EvidenceDigest:  evidenceDigest(run.ID, node.LogicalID, prof.Version, eval.Verdict, 0, eval.Reason),
			ObservedAt:      now,
			RedactedSummary: fmt.Sprintf("profile=%s version=%s verdict=%s reason=%s status=0 latency_ms=0", prof.Kind, prof.Version, eval.Verdict, eval.Reason),
		}
		if createErr := r.observations.Create(ctx, obs); createErr != nil {
			return eval.Verdict, createErr
		}
		return eval.Verdict, ErrSpeedProbeOptInRequired
	}

	client, cleanup, dialErr := r.dialer(ctx, node)
	if cleanup != nil {
		defer cleanup()
	}

	if dialErr != nil {
		result = profiles.Result{NetworkError: true}
		latency = r.clock().Sub(start).Milliseconds()
	} else if client == nil {
		result = profiles.Result{NetworkError: true}
		latency = r.clock().Sub(start).Milliseconds()
	} else {
		reqURL := probeURLForKind(kind)
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if reqErr != nil {
			result = profiles.Result{NetworkError: true}
			latency = r.clock().Sub(start).Milliseconds()
		} else {
			req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CSP-Probe/1.0)")
			reqStart := r.clock()
			if client.CheckRedirect == nil {
				client = cloneClient(client)
				client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
			}
			req = req.WithContext(ctx)
			resp, doErr := client.Do(req)
			latency = r.clock().Sub(reqStart).Milliseconds()

			if doErr != nil {
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
			} else {
				defer resp.Body.Close()
				body, readErr := readBoundedResponse(resp.Body, r.budget.MaxResponseBytes)
				if readErr != nil {
					result.NetworkError = true
					result.BytesRead = int64(len(body))
				} else {
					result.StatusCode = resp.StatusCode
					result.Body = body
					result.BytesRead = int64(len(body))
					result.ContractMatched = resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent
					result.ContractVersion = prof.Contract
				}
			}
		}
	}

	eval := prof.Evaluate(result)
	now := r.clock().UTC()
	summary := fmt.Sprintf("profile=%s version=%s verdict=%s reason=%s status=%d latency_ms=%d",
		prof.Kind, prof.Version, eval.Verdict, eval.Reason, result.StatusCode, latency)
	if dialErr != nil && (errors.Is(dialErr, ErrCredentialsUnavailable) || strings.Contains(dialErr.Error(), "credentials_unavailable")) {
		summary = summary + " error=credentials_unavailable"
	}
	obs := &domain.ProbeObservation{
		ID:              domain.MustNewUUIDv7(),
		ProbeRunID:      run.ID,
		NodeLogicalID:   node.LogicalID,
		Kind:            kind,
		Verdict:         eval.Verdict,
		EvidenceDigest:  evidenceDigest(run.ID, node.LogicalID, prof.Version, eval.Verdict, result.StatusCode, eval.Reason),
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

func evidenceDigest(runID, nodeID, version string, verdict domain.ProbeVerdict, statusCode int, reason string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%d\x00%s", runID, nodeID, version, verdict, statusCode, reason)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
