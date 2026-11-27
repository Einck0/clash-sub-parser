package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
	"clash-sub-parser/migrations"
)

type claudeCancelledBody struct{}

func (claudeCancelledBody) Read([]byte) (int, error) { return 0, context.Canceled }

// Real runner -> file SQLite -> new read handle -> HTTP list/detail/history and
// exact policy predicate. Only the node HTTP transport is synthetic; no sockets,
// provider credentials, production data, baseline or account checks are used.
func TestClaudeRunnerEvidencePersistsToAPIAndExactFilter(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{Path: filepath.Join(t.TempDir(), "claude.db"), ForeignKeys: true, BusyTimeout: 5 * time.Second}
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		id, body, mitigated string
		cancelRead          bool
		verdict             domain.ProbeVerdict
		reason, region      string
	}{
		{id: "claude-match", body: "loc=US\n", verdict: domain.VerdictAvailable, reason: "claude_available", region: "US"},
		{id: "claude-blocked", body: "uag=loc=US\nloc=CN\n", verdict: domain.VerdictRestricted, reason: "sanctioned_region", region: "CN"},
		{id: "claude-mismatch", body: "loc=JP\n", verdict: domain.VerdictAbnormal, reason: "region_mismatch", region: "JP"},
		{id: "claude-conflict", body: "loc=US\nloc=CN\n", verdict: domain.VerdictError, reason: "conflicting_region_markers"},
		{id: "claude-oversized", body: "loc=US\n" + strings.Repeat("x", 1<<20), verdict: domain.VerdictError, reason: "response_body_too_large"},
		{id: "claude-challenge", body: "loc=US\n", mitigated: "challenge", verdict: domain.VerdictRestricted, reason: "challenge_or_login"},
		{id: "claude-control", body: "loc=US\n", cancelRead: true, verdict: domain.VerdictSkipped, reason: "cancelled"},
	}
	nodes := sqlite.NewNodeRepository(db)
	var nodeIDs []string
	caseByID := make(map[string]int)
	for i, tc := range cases {
		nodeIDs = append(nodeIDs, tc.id)
		caseByID[tc.id] = i
		node := domain.Node{LogicalID: tc.id, DisplayName: tc.id, Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388, Active: true, Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "synthetic"}}
		if err := nodes.UpsertBatch(ctx, []domain.Node{node}); err != nil {
			t.Fatal(err)
		}
	}
	runs := sqlite.NewProbeRunRepository(db)
	observations := sqlite.NewProbeObservationRepository(db)
	scheduler, err := queue.NewScheduler(queue.Config{Concurrency: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Close()
	var sessions, closed, geoCalls, claudeCalls atomic.Int32
	runner := probe.NewDefaultRunner(nodes, observations, scheduler, runs,
		probe.WithNodeScope(domain.NodeScopeAllAssets), probe.WithPlatforms([]string{"claude"}),
		probe.WithNodeDialer(func(_ context.Context, node domain.Node) (*http.Client, func() error, error) {
			index, ok := caseByID[node.LogicalID]
			if !ok {
				return nil, nil, fmt.Errorf("unexpected fixture node %q", node.LogicalID)
			}
			tc := cases[index]
			sessions.Add(1)
			client := &http.Client{Transport: speedRoundTripper(func(req *http.Request) (*http.Response, error) {
				var reader io.Reader
				header := make(http.Header)
				switch req.URL.String() {
				case "https://api.ip.sb/geoip":
					geoCalls.Add(1)
					reader = strings.NewReader(`{"ip":"203.0.113.10","country_code":"US","asn":13335}`)
				case "https://claude.ai/cdn-cgi/trace":
					claudeCalls.Add(1)
					header.Set("Cf-Mitigated", tc.mitigated)
					reader = strings.NewReader(tc.body)
					if tc.cancelRead {
						reader = io.MultiReader(reader, claudeCancelledBody{})
					}
				default:
					return nil, fmt.Errorf("unexpected fixture request %s", req.URL)
				}
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(reader), Request: req}, nil
			})}
			return client, func() error { closed.Add(1); return nil }, nil
		}),
	)
	run := &domain.ProbeRun{ID: domain.MustNewUUIDv7(), IdempotencyKey: "claude-consumer", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Minute)}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx, run, nodeIDs, []domain.ProbeKind{domain.ProbeKindGeo, domain.ProbeKindAI}); err != nil {
		t.Fatal(err)
	}
	wantCalls := int32(len(cases))
	if sessions.Load() != wantCalls || closed.Load() != wantCalls || geoCalls.Load() != wantCalls || claudeCalls.Load() != wantCalls {
		t.Fatalf("node sessions/selection changed: opened=%d closed=%d geo=%d claude=%d", sessions.Load(), closed.Load(), geoCalls.Load(), claudeCalls.Load())
	}

	// An independent database handle/repository proves consumers read the stored
	// evidence, not runner-owned maps. The original handle stays open for the
	// runner's existing best-effort asynchronous history pruning.
	readDB, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer readDB.Close()
	readNodes := sqlite.NewNodeRepository(readDB)
	readRuns := sqlite.NewProbeRunRepository(readDB)
	readObservations := sqlite.NewProbeObservationRepository(readDB)
	stored, err := readObservations.ListByRun(ctx, run.ID)
	if err != nil || len(stored) != 2*len(cases) {
		t.Fatalf("stored observations=%d err=%v", len(stored), err)
	}
	storedAI := make(map[string]domain.ProbeObservation)
	for _, obs := range stored {
		if obs.Kind == domain.ProbeKindAI {
			storedAI[obs.NodeLogicalID] = obs
		}
	}
	inv := inventory.NewService(readDB, sqlite.NewSubscriptionRepository(readDB), sqlite.NewSubscriptionFetchRepository(readDB), readNodes, sqlite.NewNodeSourceRepository(readDB), nil)
	router := transporthttp.NewRouter(transporthttp.RouterConfig{
		AdminToken: probeTestAdminToken, InventoryService: inv, ProbeService: probe.NewService(readRuns),
		ProbeRunRepository: readRuns, ProbeObservationRepository: readObservations,
	})
	get := func(t *testing.T, path string, target any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
			t.Fatal(err)
		}
	}
	var list struct {
		Data struct {
			Items []inventory.NodeView `json:"items"`
			Total int                  `json:"total"`
		} `json:"data"`
	}
	get(t, "/api/v1/nodes?scope=all_assets&page_size=100", &list)
	if list.Data.Total != len(cases) || len(list.Data.Items) != len(cases) {
		t.Fatalf("node list count=%d total=%d", len(list.Data.Items), list.Data.Total)
	}
	views := make(map[string]inventory.NodeView)
	for _, view := range list.Data.Items {
		views[view.LogicalID] = view
	}
	checkHistory := func(t *testing.T, path string, wantCount int) {
		t.Helper()
		var response struct {
			Data struct {
				Items []domain.ProbeObservation `json:"items"`
			} `json:"data"`
		}
		get(t, path, &response)
		if len(response.Data.Items) != wantCount {
			t.Fatalf("GET %s: observation count=%d want=%d", path, len(response.Data.Items), wantCount)
		}
		for _, obs := range response.Data.Items {
			if obs.Kind == domain.ProbeKindAI && !reflect.DeepEqual(obs, storedAI[obs.NodeLogicalID]) {
				t.Fatalf("history changed stored evidence: %s %+v", path, obs)
			}
		}
	}
	checkHistory(t, "/api/v1/probes/runs/"+run.ID+"/observations?page_size=100", len(stored))
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			obs := storedAI[tc.id]
			cap := obs.Platforms["claude"]
			node, err := readNodes.GetByLogicalID(ctx, tc.id)
			if err != nil {
				t.Fatal(err)
			}
			if len(obs.Platforms) != 1 || obs.ProbeRunID != run.ID || obs.ConnectionRevision == nil || *obs.ConnectionRevision != node.ConnectionRevision || obs.Verdict != tc.verdict {
				t.Fatalf("identity/aggregate/selection mismatch: %+v", obs)
			}
			if cap.Verdict != tc.verdict || cap.Reason != tc.reason || cap.Region != tc.region || cap.ExitRegion != "US" {
				t.Fatalf("detector or same-run geo evidence changed: %+v", cap)
			}
			if tc.region == "" && (cap.RegionSource != "" || cap.EvidenceScope != "") || tc.region != "" && (cap.RegionSource != "cdn_trace" || cap.EvidenceScope != "cdn_trace") {
				t.Fatalf("trace scope changed: %+v", cap)
			}
			assertView := func(view inventory.NodeView) {
				t.Helper()
				status := view.Capabilities["ai"]
				if view.HealthStatus != "unknown" || view.LatencyMS != nil || status.Stale || status.Verdict != tc.verdict || !reflect.DeepEqual(status.Platforms["claude"], cap) {
					t.Fatalf("view lost evidence or confused AI with baseline health: id=%s health=%s status=%+v", view.LogicalID, view.HealthStatus, status)
				}
			}
			assertView(views[tc.id])
			var detail struct {
				Data transporthttp.NodeDetailResponse `json:"data"`
			}
			get(t, "/api/v1/nodes/"+tc.id, &detail)
			assertView(detail.Data.Node)
			checkHistory(t, "/api/v1/nodes/"+tc.id+"/observations?page_size=100", 2)
			kind := domain.ProbeKindAI
			for _, verdict := range []domain.ProbeVerdict{domain.VerdictAvailable, domain.VerdictRestricted, domain.VerdictError, domain.VerdictAbnormal, domain.VerdictSkipped, domain.VerdictUnknown} {
				condition := domain.FilterCondition{Field: domain.FilterFieldPlatformVerdict, Op: domain.FilterOpEquals, Value: string(verdict), ProbeKind: &kind, Platform: "claude"}
				got, reason := domain.MatchesCondition(condition, *node, nil, map[domain.ProbeKind]domain.ProbeObservation{kind: obs}, time.Now().UTC())
				if got != (verdict == tc.verdict) {
					t.Fatalf("exact filter %s matched=%t reason=%s", verdict, got, reason)
				}
				changed := *node
				changed.ConnectionRevision++
				if matched, _ := domain.MatchesCondition(condition, changed, nil, map[domain.ProbeKind]domain.ProbeObservation{kind: obs}, time.Now().UTC()); matched {
					t.Fatal("old observation certified a different connection revision")
				}
			}
			t.Logf("CLAUDE_CONSUMER case=%s verdict=%s reason=%s region=%s exit=US scope=%s health=unknown history=exact filter=exact revision=isolated", tc.id, cap.Verdict, cap.Reason, cap.Region, cap.EvidenceScope)
		})
	}
}
