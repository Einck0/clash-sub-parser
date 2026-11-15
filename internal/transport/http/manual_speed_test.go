package http_test

import (
	"context"
	"encoding/json"
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

// Compatibility tests stop at the runner boundary: no dialer or real traffic.
type capturedProbeRequest struct {
	nodeIDs []string
	kinds   []domain.ProbeKind
}

type capturingProbeRunner struct {
	requests chan capturedProbeRequest
}

func (r capturingProbeRunner) Run(_ context.Context, _ *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error {
	r.requests <- capturedProbeRequest{nodeIDs: nodeIDs, kinds: kinds}
	return nil
}

func TestLegacySpeedOnlyHTTPRequestsPreserveFleetSemantics(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		nodeIDs []string
	}{
		{
			name:    "multiple explicit nodes",
			payload: `{"node_logical_ids":["one","two"],"kinds":["speed"]}`,
			nodeIDs: []string{"one", "two"},
		},
		{
			name:    "implicit fleet",
			payload: `{"kinds":["speed"]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs := &probeRunMemory{data: map[string]domain.ProbeRun{}}
			runner := capturingProbeRunner{requests: make(chan capturedProbeRequest, 1)}
			service := probe.NewService(runs, probe.WithRunner(runner))
			router := transporthttp.NewRouter(transporthttp.RouterConfig{
				AdminToken: probeTestAdminToken, ProbeService: service,
				ProbeRunRepository: runs, ProbeObservationRepository: &observationMemory{},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs", strings.NewReader(tc.payload))
			req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
			req.Header.Set("Idempotency-Key", "legacy-speed-"+tc.name)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("legacy speed-only request rejected: status=%d body=%s", rec.Code, rec.Body.String())
			}
			select {
			case got := <-runner.requests:
				if !reflect.DeepEqual(got.nodeIDs, tc.nodeIDs) || !reflect.DeepEqual(got.kinds, []domain.ProbeKind{domain.ProbeKindSpeed}) {
					t.Fatalf("legacy request rewritten: got=%+v want targets=%v kinds=[speed]", got, tc.nodeIDs)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("legacy speed-only request never reached controlled runner")
			}
		})
	}
}

func TestMixedSpeedHTTPRequestsPreserveLegacyTargets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		nodeIDs []string
		kinds   []domain.ProbeKind
	}{
		{
			name:    "single mixed formerly rejected",
			payload: `{"node_logical_ids":["manual-speed-node"],"kinds":["speed","baseline"]}`,
			nodeIDs: []string{"manual-speed-node"}, kinds: []domain.ProbeKind{domain.ProbeKindSpeed, domain.ProbeKindBaseline},
		},
		{
			name:    "omitted targets retain fleet semantics",
			payload: `{"kinds":["baseline","speed"]}`,
			kinds:   []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindSpeed},
		},
		{
			name:    "empty targets retain fleet semantics",
			payload: `{"node_logical_ids":[],"kinds":["baseline","speed"]}`,
			nodeIDs: []string{}, kinds: []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindSpeed},
		},
		{
			name:    "multiple mixed targets",
			payload: `{"node_logical_ids":["one","two"],"kinds":["baseline","speed"]}`,
			nodeIDs: []string{"one", "two"}, kinds: []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindSpeed},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs := &probeRunMemory{data: map[string]domain.ProbeRun{}}
			runner := capturingProbeRunner{requests: make(chan capturedProbeRequest, 1)}
			service := probe.NewService(runs, probe.WithRunner(runner))
			router := transporthttp.NewRouter(transporthttp.RouterConfig{
				AdminToken: probeTestAdminToken, ProbeService: service,
				ProbeRunRepository: runs, ProbeObservationRepository: &observationMemory{},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs", strings.NewReader(tc.payload))
			req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
			req.Header.Set("Idempotency-Key", "mixed")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("mixed request rejected: status=%d body=%s", rec.Code, rec.Body.String())
			}
			select {
			case got := <-runner.requests:
				if !reflect.DeepEqual(got.nodeIDs, tc.nodeIDs) || !reflect.DeepEqual(got.kinds, tc.kinds) {
					t.Fatalf("mixed request rewritten: got=%+v want targets=%v kinds=%v", got, tc.nodeIDs, tc.kinds)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("mixed request never reached controlled runner")
			}
		})
	}
}

type speedRoundTripper func(*http.Request) (*http.Response, error)

func (f speedRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise the real HTTP -> service -> runner -> SQLite -> inventory path without
// network traffic. Reopen the file DB to distinguish persisted status from run memory.
func TestManualSpeedPersistsIntoNodeStatus(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{Path: filepath.Join(t.TempDir(), "speed.db"), ForeignKeys: true, BusyTimeout: 5 * time.Second}
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatal(err)
	}
	nodes := sqlite.NewNodeRepository(db)
	node := domain.Node{LogicalID: "manual-speed-node", DisplayName: "Synthetic node", Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388, Active: true, Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "synthetic"}}
	if err := nodes.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	runs := sqlite.NewProbeRunRepository(db)
	observations := sqlite.NewProbeObservationRepository(db)
	scheduler, err := queue.NewScheduler(queue.Config{Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Close()
	var requests atomic.Int32
	runner := probe.NewDefaultRunner(nodes, observations, scheduler, runs,
		probe.WithNodeScope(domain.NodeScopeAllAssets),
		probe.WithNodeDialer(func(context.Context, domain.Node) (*http.Client, func() error, error) {
			return &http.Client{Transport: speedRoundTripper(func(req *http.Request) (*http.Response, error) {
				requests.Add(1)
				if req.URL.Host != "speed.cloudflare.com" || req.URL.Query().Get("bytes") != "5242880" {
					t.Errorf("unexpected speed request: %s", req.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 4096))), Request: req}, nil
			})}, nil, nil
		}),
	)
	service := probe.NewService(runs, probe.WithRunner(runner))
	router := transporthttp.NewRouter(transporthttp.RouterConfig{AdminToken: probeTestAdminToken, ProbeService: service, ProbeRunRepository: runs, ProbeObservationRepository: observations})
	create := func(key, payload string, want int) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/probes/runs", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+probeTestAdminToken)
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("create status %d, want %d: %s", rec.Code, want, rec.Body.String())
		}
		var resp struct {
			Data struct {
				RunID string `json:"run_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.Data.RunID
	}
	for _, payload := range []string{
		`{"manual_speed":true,"kinds":["speed"]}`,
		`{"manual_speed":true,"node_logical_ids":[],"kinds":["speed"]}`,
		`{"manual_speed":true,"node_logical_ids":["manual-speed-node","other"],"kinds":["speed"]}`,
		`{"manual_speed":true,"node_logical_ids":["manual-speed-node"],"kinds":["speed","speed"]}`,
		`{"manual_speed":true,"node_logical_ids":["missing"],"kinds":["speed"]}`,
		`{"manual_speed":true,"node_logical_ids":[" "],"kinds":["speed"]}`,
	} {
		create(domain.MustNewUUIDv7(), payload, http.StatusUnprocessableEntity)
	}
	if _, total, err := runs.List(ctx, nil, 1, 10); err != nil || total != 0 || requests.Load() != 0 {
		t.Fatalf("rejected request created work: total=%d requests=%d err=%v", total, requests.Load(), err)
	}
	payload := `{"manual_speed":true,"node_logical_ids":["manual-speed-node"],"kinds":["speed"]}`
	node.Active = false
	if err := nodes.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	create("inactive-speed", payload, http.StatusUnprocessableEntity)
	node.Active = true
	if err := nodes.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	currentNode, err := nodes.GetByLogicalID(ctx, node.LogicalID)
	if err != nil {
		t.Fatal(err)
	}
	currentRevision := currentNode.ConnectionRevision
	runID := create("manual-speed-once", payload, http.StatusCreated)
	wait := func(id string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			run, err := runs.GetByID(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if run.IsTerminal() {
				if run.State != domain.ProbeRunStateSucceeded {
					t.Fatalf("run failed: %+v", run)
				}
				return
			}
			select {
			case <-deadline:
				t.Fatal("manual speed did not complete")
			case <-tick.C:
			}
		}
	}
	wait(runID)
	if retry := create("manual-speed-once", payload, http.StatusCreated); retry != runID || requests.Load() != 1 {
		t.Fatalf("retry duplicated speed request: first=%s retry=%s requests=%d", runID, retry, requests.Load())
	}
	persisted, err := observations.ListByRun(ctx, runID)
	if err != nil || len(persisted) != 1 || persisted[0].Kind != domain.ProbeKindSpeed || persisted[0].Throughput == nil || *persisted[0].Throughput <= 0 {
		t.Fatalf("missing persisted speed: %+v err=%v", persisted, err)
	}
	throughput := *persisted[0].Throughput
	if strings.Count(persisted[0].RedactedSummary, "throughput_kbps=") != 1 || !strings.Contains(persisted[0].RedactedSummary, "throughput_kib_per_second=") {
		t.Fatalf("ambiguous speed units in summary: %s", persisted[0].RedactedSummary)
	}
	if persisted[0].ConnectionRevision == nil || *persisted[0].ConnectionRevision != currentRevision {
		t.Fatalf("missing connection revision: %+v", persisted[0])
	}
	// The runner asynchronously prunes history using the old handle (best effort).
	// Close that handle, then reopen the file for independent inventory reads.
	scheduler.Close()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	inventoryService := inventory.NewService(reopened, sqlite.NewSubscriptionRepository(reopened), sqlite.NewSubscriptionFetchRepository(reopened), sqlite.NewNodeRepository(reopened), sqlite.NewNodeSourceRepository(reopened), nil)
	assertView := func(v inventory.NodeView) {
		t.Helper()
		cap, ok := v.Capabilities["speed"]
		if !ok || cap.Verdict != domain.VerdictAvailable || cap.Stale || v.HealthStatus != "unknown" || v.LatencyMS != nil || v.ProbeState != "idle" {
			t.Fatalf("speed was confused with baseline/task status: %+v", v)
		}
		if cap.Throughput == nil || *cap.Throughput != throughput {
			t.Fatalf("speed not projected: %+v", cap)
		}
	}
	views, total, err := inventoryService.ListNodesReadModel(ctx, domain.NodeFilter{Scope: domain.NodeScopeAllAssets, Pagination: domain.Pagination{Page: 1, PageSize: 10}})
	if err != nil || total != 1 || len(views) != 1 {
		t.Fatalf("list: %+v total=%d err=%v", views, total, err)
	}
	assertView(views[0])
	detail, err := inventoryService.GetNodeDetailWithRisk(ctx, node.LogicalID, "")
	if err != nil {
		t.Fatal(err)
	}
	assertView(detail.ToNodeView())

	// A connection change preserves historical speed evidence but cannot certify
	// the new credentials; the same invalidation contract applies as other probes.
	node.Credentials.Password = "synthetic-revised"
	if err := sqlite.NewNodeRepository(reopened).UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	detail, err = inventoryService.GetNodeDetailWithRisk(ctx, node.LogicalID, "")
	if err != nil {
		t.Fatal(err)
	}
	if v := detail.ToNodeView(); v.ConnectionRevision != currentRevision+1 || !v.Capabilities["speed"].Stale || v.HealthStatus != "unknown" {
		t.Fatalf("old speed certified changed connection: %+v", v)
	}
}
