// Isolated adapter: only the production runner, mapper and in-memory repositories.
package main

import (
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/mihomo"
	"clash-sub-parser/internal/probe/platform"
	"clash-sub-parser/internal/probe/queue"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

type inventory struct {
	domain.NodeRepository
	nodes []domain.Node
}

func (r *inventory) List(context.Context, domain.NodeFilter) ([]domain.Node, int, error) {
	return r.nodes, len(r.nodes), nil
}

type observations struct {
	domain.ProbeObservationRepository
	mu   sync.Mutex
	rows []domain.ProbeObservation
}

func (r *observations) Create(_ context.Context, row *domain.ProbeObservation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, *row)
	return nil
}
func (r *observations) ListByRun(context.Context, string) ([]domain.ProbeObservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.ProbeObservation(nil), r.rows...), nil
}

type runs struct{ domain.ProbeRunRepository }

func (*runs) UpdateState(context.Context, string, domain.ProbeRunState) error { return nil }

type quota struct {
	Scope string `json:"scope"`
	Limit int64  `json:"limit"`
}
type input struct {
	Mode        string           `json:"mode"`
	Nodes       []domain.Node    `json:"nodes"`
	Deadline    time.Time        `json:"deadline"`
	Stage       string           `json:"stage"`
	Platforms   []string         `json:"platforms"`
	TimeoutMS   int              `json:"timeout_ms"`
	Quotas      map[string]quota `json:"quotas"`
	LedgerURL   string           `json:"ledger_url"`
	LedgerToken string           `json:"ledger_token"`
	Attempt     string           `json:"attempt"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "comparison rejected:", err)
		os.Exit(1)
	}
}
func run() error {
	var in input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		return fmt.Errorf("invalid input")
	}
	if in.Mode == "freeze" {
		maps := []map[string]any{}
		for _, n := range in.Nodes {
			m, err := mihomo.NodeToMapping(n)
			if err != nil {
				return fmt.Errorf("mapping rejected")
			}
			m["name"] = n.LogicalID
			maps = append(maps, m)
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"mappings": maps, "engine": mihomo.CoreVersion()})
	}
	if in.TimeoutMS != 15000 || time.Until(in.Deadline) <= 0 || time.Until(in.Deadline) > 30*time.Minute {
		return fmt.Errorf("invalid deadline/timeout")
	}
	kind := domain.ProbeKindBaseline
	concurrency := 8
	switch in.Stage {
	case "baseline":
	case "streaming":
		kind = domain.ProbeKindStreaming
		concurrency = 2
	case "ai":
		kind = domain.ProbeKindAI
		concurrency = 2
	case "ip_risk":
		kind = domain.ProbeKindIPRisk
		concurrency = 2
	default:
		return fmt.Errorf("no-speed stage required")
	}
	for _, n := range in.Nodes {
		q := in.Quotas[n.LogicalID]
		if q.Limit <= 0 || q.Limit > 128<<20 || q.Scope == "" {
			return fmt.Errorf("missing quota")
		}
		if kind == domain.ProbeKindBaseline && q.Limit > 64<<10 {
			return fmt.Errorf("baseline exceeds 64KiB")
		}
	}
	ctx, cancel := context.WithDeadline(context.Background(), in.Deadline)
	defer cancel()
	obs := &observations{}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var used int64
	var fatal error
	for _, node := range in.Nodes {
		if ctx.Err() != nil {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(n domain.Node) {
			defer wg.Done()
			defer func() { <-sem }()
			q := in.Quotas[n.LogicalID]
			b, err := platform.NewLedgerBodyBudget(q.Limit, in.LedgerURL, in.LedgerToken, q.Scope)
			if err != nil {
				mu.Lock()
				fatal = err
				mu.Unlock()
				return
			}
			sched, _ := queue.NewScheduler(queue.Config{Concurrency: 1})
			defer sched.Close()
			dialer := func(_ context.Context, node domain.Node) (*http.Client, func() error, error) {
				pc, err := mihomo.NewProxyClient(node, 15*time.Second)
				if err != nil {
					return nil, nil, err
				}
				tr := pc.Transport.(*http.Transport)
				tr.DisableKeepAlives = true
				tr.ForceAttemptHTTP2 = false
				pc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
				return pc.Client, pc.Close, nil
			}
			r := probe.NewDefaultRunner(&inventory{nodes: []domain.Node{n}}, obs, sched, &runs{}, probe.WithBodyBudget(b), probe.WithPlatforms(in.Platforms), probe.WithNodeDialer(dialer), probe.WithRunBudget(probe.RunBudget{MaxTasks: 1, MaxResponseBytes: 64 << 10, MaxTotalBytes: q.Limit, TaskTimeout: 15 * time.Second}))
			pr := &domain.ProbeRun{ID: in.Attempt + "/" + n.LogicalID, State: domain.ProbeRunStateQueued, ConfigRevision: "frozen", DeadlineAt: in.Deadline}
			_ = r.Run(ctx, pr, nil, []domain.ProbeKind{kind})
			_, v := b.Snapshot()
			mu.Lock()
			used += v
			mu.Unlock()
		}(node)
	}
	wg.Wait()
	if fatal != nil {
		return fatal
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"side": "csp", "engine": mihomo.CoreVersion(), "rows": obs.rows, "body_bytes": used, "incomplete": ctx.Err() != nil, "transport_conditions": "http1/redirect-reject/verify-target-tls/keepalive-off", "stage": in.Stage})
}
