package http_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

func TestNodesSpeedFilterValidation(t *testing.T) {
	db := newCleanSQLiteDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)
	nodes := sqlite.NewNodeRepository(db)
	if err := nodes.UpsertBatch(ctx, []domain.Node{{LogicalID: "speed-http", DisplayName: "speed-http", Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388, Active: true, CreatedAt: now, UpdatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	node, err := nodes.GetByLogicalID(ctx, "speed-http")
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: "speed-http-run", IdempotencyKey: "speed-http-run", State: domain.ProbeRunStateSucceeded, DeadlineAt: now}); err != nil {
		t.Fatal(err)
	}
	speed := 12.5
	if err := sqlite.NewProbeObservationRepository(db).Create(ctx, &domain.ProbeObservation{ID: "speed-http-obs", ProbeRunID: "speed-http-run", NodeLogicalID: node.LogicalID, Kind: domain.ProbeKindSpeed, Verdict: domain.VerdictAvailable, ConnectionRevision: &node.ConnectionRevision, ObservedAt: now, Throughput: &speed}); err != nil {
		t.Fatal(err)
	}
	router := setupRegressionTestRouter(t, db, nil)
	for _, query := range []string{
		"speed_min_kib_s=", "speed_min_kib_s=-1", "speed_min_kib_s=-0", "speed_min_kib_s=NaN", "speed_min_kib_s=Inf", "speed_min_kib_s=1e999", "speed_min_kib_s=nope", "speed_min_kib_s=1&speed_min_kib_s=2",
		"speed_min_kib_s=1e-400", "speed_min_kib_s=1e-324", "speed_min_kib_s=0." + strings.Repeat("0", 324) + "1",
	} {
		rec, failure := getNodesError(t, router, "/api/v1/nodes?"+query, testAdminToken)
		if rec.Code != http.StatusBadRequest || failure.Code != "invalid_speed_min_kib_s" {
			t.Errorf("%s: got %d/%s", query, rec.Code, failure.Code)
		}
	}
	for _, tc := range []struct {
		query string
		total int
	}{
		{"speed_min_kib_s=0", 1}, {"speed_min_kib_s=0.00E-400", 1}, {"speed_min_kib_s=5e-324", 1},
		{"speed_min_kib_s=12.5", 1}, {"speed_min_kib_s=12.51", 0}, {"speed_min_kib_s=1.3e1", 0}, {"", 1},
	} {
		rec, response := getNodesPaginated(t, router, "/api/v1/nodes?scope=all_assets&"+tc.query, testAdminToken)
		if rec.Code != http.StatusOK || response.Data.Total != tc.total {
			t.Errorf("%s: status=%d total=%d", tc.query, rec.Code, response.Data.Total)
		}
	}
}

func speedTestRouter(db *sql.DB, opts ...inventory.Option) http.Handler {
	cfg := newTestRouterConfig(true)
	cfg.InventoryService = inventory.NewService(db, sqlite.NewSubscriptionRepository(db), sqlite.NewSubscriptionFetchRepository(db), sqlite.NewNodeRepository(db), sqlite.NewNodeSourceRepository(db), nil, opts...)
	return transporthttp.NewRouter(cfg)
}

func TestNodesSpeedFilterEvidenceViewParity(t *testing.T) {
	db := newCleanSQLiteDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 12, 0, 0, 123456789, time.UTC)
	const good = `{"throughput":100}`
	at := now.Format(time.RFC3339Nano)
	cases := []struct {
		id, evidence, stamp, verdict string
		revision                     any
		visible                      bool
	}{
		{"exact", good, at, "available", 1, true},
		{"earlier", good, now.Add(-time.Nanosecond).Format(time.RFC3339Nano), "available", 1, true},
		{"offset", good, "2026-11-18T18:00:00.123456789+06:00", "available", 1, true},
		{"old-extension", `{"throughput":100,"future_extension":true}`, "2024-01-01T00:00:00Z", "available", 1, true},
		{"region", `{"throughput":100,"region":3}`, at, "available", 1, false},
		{"nested", `{"throughput":100,"attempt":{"executed":"yes"}}`, at, "available", 1, false},
		{"future", good, now.Add(time.Nanosecond).Format(time.RFC3339Nano), "available", 1, false},
		{"date-only", good, "2026-11-17", "available", 1, false},
		{"space", good, "2026-11-17 00:00:00Z", "available", 1, false},
		{"calendar", good, "2026-02-30T00:00:00Z", "available", 1, false},
		{"zero-time", good, "0001-01-01T00:00:00Z", "available", 1, false},
		{"failed", good, at, "error", 1, false}, {"stale", good, at, "stale", 1, false}, {"skipped", good, at, "skipped", 1, false},
		{"version", good, at, "available", 2, false}, {"legacy", good, at, "available", nil, false},
		{"string", `{"throughput":"100"}`, at, "available", 1, false}, {"bad-json", "{", at, "available", 1, false},
		{"overflow", `{"throughput":1e999}`, at, "available", 1, false}, {"zero-speed", `{"throughput":0}`, at, "available", 1, false},
		{"negative", `{"throughput":-1}`, at, "available", 1, false}, {"summary-only", `{}`, at, "available", 1, false},
		{"untested", "", "", "", nil, false},
	}
	if err := sqlite.NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: "parity", IdempotencyKey: "parity", State: domain.ProbeRunStateSucceeded, DeadlineAt: now}); err != nil {
		t.Fatal(err)
	}
	const insertObs = `INSERT INTO probe_observations (id,probe_run_id,node_logical_id,kind,verdict,connection_revision,observed_at,evidence_data,redacted_summary) VALUES (?,'parity',?,'speed',?,?,?,?, 'throughput_kbps=9999')`
	var wantAll, wantValid []string
	for _, tc := range cases {
		_, err := db.Exec(`INSERT INTO nodes (logical_id,protocol,display_name,server,port,config_json,active,connection_revision,created_at,updated_at) VALUES (?,'ss',?,'192.0.2.1',8388,'{}',1,1,?,?)`, tc.id, tc.id, at, at)
		if err != nil {
			t.Fatal(err)
		}
		wantAll = append(wantAll, tc.id)
		if tc.visible {
			wantValid = append(wantValid, tc.id)
		}
		if tc.verdict == "" {
			continue
		}
		// Same stored timestamp: id DESC must select the invalid row, not fall
		// back to the valid lower id even if the timestamp itself is valid.
		if tc.stamp == at {
			if _, err := db.Exec(insertObs, "a-"+tc.id, tc.id, "available", 1, at, good); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec(insertObs, "z-"+tc.id, tc.id, tc.verdict, tc.revision, tc.stamp, tc.evidence); err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(wantAll)
	slices.Sort(wantValid)
	clockReads := 0
	router := speedTestRouter(db, inventory.WithClock(func() time.Time { clockReads++; return now }))
	for _, query := range []string{"", "&speed_min_kib_s=0", "&speed_min_kib_s=100", "&speed_min_kib_s=100.0001"} {
		before := clockReads
		rec, resp := getNodesPaginated(t, router, "/api/v1/nodes?scope=all_assets&sort_by=display_name"+query, testAdminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", query, rec.Code, rec.Body.String())
		}
		if query != "" && clockReads-before != 1 {
			t.Errorf("%s: read as-of clock %d times", query, clockReads-before)
		}
		want := wantValid
		if query == "" {
			want = wantAll
		} else if query == "&speed_min_kib_s=100.0001" {
			want = nil
		}
		var ids []string
		for _, view := range resp.Data.Items {
			ids = append(ids, view.LogicalID)
			cap := view.Capabilities["speed"]
			displayable := cap.Verdict == domain.VerdictAvailable && !cap.Stale && cap.Throughput != nil && *cap.Throughput > 0
			if expected := slices.Contains(wantValid, view.LogicalID); expected != displayable || (expected && *cap.Throughput != 100) || view.HealthStatus != "unknown" {
				t.Fatalf("%s %s: unexpected speed=%+v baseline=%s", query, view.LogicalID, cap, view.HealthStatus)
			}
		}
		if !slices.Equal(ids, want) || resp.Data.Total != len(want) {
			t.Fatalf("%s: IDs=%v total=%d want=%v", query, ids, resp.Data.Total, want)
		}
	}
}

// Interpose only between the real page query and its real enrichment read.
// This makes concurrent supersession deterministic without timing or goroutines.
type speedReadBoundary struct {
	domain.ProbeObservationRepository
	beforeRead func() error
}

func (r speedReadBoundary) ListLatestByNodes(ctx context.Context, ids []string, kinds []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	if err := r.beforeRead(); err != nil {
		return nil, err
	}
	return r.ProbeObservationRepository.ListLatestByNodes(ctx, ids, kinds)
}

func TestNodesSpeedFilterPageReadFailure(t *testing.T) {
	db := newCleanSQLiteDB(t)
	seedPaginationRegressionNodes(t, db)
	now := time.Now().UTC().Add(-time.Minute)
	const id = "node-reg-0121"
	insert := func(obsID, verdict string, stamp time.Time) error {
		_, err := db.Exec(`INSERT INTO probe_observations (id,probe_run_id,node_logical_id,kind,verdict,connection_revision,observed_at,evidence_data) VALUES (?,'run-reg-001',?,'speed',?,1,?,'{"throughput":100}')`, obsID, id, verdict, stamp.Format(time.RFC3339))
		return err
	}
	if err := insert("speed-ok", "available", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/nodes?search=Combined&health_status=unknown&ip_risk_band=unknown&protocol=vmess&subscription_id=sub-reg-001&speed_min_kib_s=100"
	rec, resp := getNodesPaginated(t, setupRegressionTestRouter(t, db, nil), base, testAdminToken)
	if rec.Code != 200 || resp.Data.Total != 1 || len(resp.Data.Items) != 1 || resp.Data.Items[0].LogicalID != id {
		t.Fatalf("control: status=%d total=%d items=%v", rec.Code, resp.Data.Total, resp.Data.Items)
	}
	for _, condition := range []string{"protocol=ss", "subscription_id=other", "health_status=healthy", "ip_risk_band=high", "search=HongKong"} {
		rec, page := getNodesPaginated(t, setupRegressionTestRouter(t, db, nil), "/api/v1/nodes?speed_min_kib_s=100&"+condition, testAdminToken)
		if rec.Code != 200 || page.Data.Total != 0 || len(page.Data.Items) != 0 {
			t.Fatalf("intersection %s: status=%d total=%d", condition, rec.Code, page.Data.Total)
		}
	}
	for _, tc := range []struct {
		code   string
		status int
		before func() error
	}{
		{"speed_evidence_unavailable", http.StatusInternalServerError, func() error { return errors.New("fixture read error") }},
		{"speed_evidence_changed", http.StatusConflict, func() error { return insert("new-failure", "error", now) }},
	} {
		router := speedTestRouter(db, inventory.WithProbeObservationRepository(speedReadBoundary{sqlite.NewProbeObservationRepository(db), tc.before}))
		rec, failure := getNodesError(t, router, base, testAdminToken)
		if rec.Code != tc.status || failure.Code != tc.code {
			t.Fatalf("want %d/%s got %d/%s", tc.status, tc.code, rec.Code, failure.Code)
		}
	}
}
