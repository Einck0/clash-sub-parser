package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestNodeSpeedFilterLatestRevisionAndPagination(t *testing.T) {
	db := setupNodeProjectionDB(t)
	ctx := context.Background()
	nodes := NewNodeRepository(db)
	observations := NewProbeObservationRepository(db)
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	if err := NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: "speed-filter-run", IdempotencyKey: "speed-filter-run", State: domain.ProbeRunStateSucceeded, DeadlineAt: now}); err != nil {
		t.Fatal(err)
	}
	speed := 100.0
	for i := 0; i < 110; i++ {
		id := fmt.Sprintf("speed-filter-%03d", i)
		node := domain.Node{LogicalID: id, DisplayName: id, Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := nodes.UpsertBatch(ctx, []domain.Node{node}); err != nil {
			t.Fatal(err)
		}
		saved, err := nodes.GetByLogicalID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		obs := domain.ProbeObservation{ID: "obs-" + id, ProbeRunID: "speed-filter-run", NodeLogicalID: id, Kind: domain.ProbeKindSpeed, Verdict: domain.VerdictAvailable, ConnectionRevision: &saved.ConnectionRevision, ObservedAt: now.Add(-time.Hour), Throughput: &speed}
		if err := observations.Create(ctx, &obs); err != nil {
			t.Fatal(err)
		}
	}
	// Invalid structured data and a newer failure are not eligible, and a
	// revision change invalidates a previously successful measurement.
	for id, evidence := range map[string]string{"speed-filter-000": "{", "speed-filter-001": `{"throughput":"100"}`, "speed-filter-002": `{"throughput":1e999}`, "speed-filter-003": `{"throughput":0}`} {
		if _, err := db.ExecContext(ctx, `UPDATE probe_observations SET evidence_data=? WHERE id=?`, evidence, "obs-"+id); err != nil {
			t.Fatal(err)
		}
	}
	var revision int64 = 1
	if err := observations.Create(ctx, &domain.ProbeObservation{ID: "new-error", ProbeRunID: "speed-filter-run", NodeLogicalID: "speed-filter-004", Kind: domain.ProbeKindSpeed, Verdict: domain.VerdictError, ConnectionRevision: &revision, ObservedAt: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE probe_observations SET observed_at=? WHERE id=?`, "0001-01-01T00:00:00Z", "obs-speed-filter-005"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE probe_observations SET observed_at=? WHERE id=?`, now.Add(time.Hour).Format(time.RFC3339), "obs-speed-filter-006"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE probe_observations SET connection_revision=NULL WHERE id=?`, "obs-speed-filter-007"); err != nil {
		t.Fatal(err)
	}
	node, err := nodes.GetByLogicalID(ctx, "speed-filter-008")
	if err != nil {
		t.Fatal(err)
	}
	node.Server = "192.0.2.2"
	if err := nodes.UpsertBatch(ctx, []domain.Node{*node}); err != nil {
		t.Fatal(err)
	}
	filter := domain.NodeFilter{SpeedMinKiBS: &speed, Now: &now, SearchText: "speed-filter-", Protocols: []domain.Protocol{domain.ProtocolSS}, ActiveOnly: true, SortBy: "display_name", Pagination: domain.Pagination{Page: 2, PageSize: 100}}
	// Several measurements per node force latest-row selection. The second
	// page starts beyond 100 matching nodes, not merely 100 raw candidates.
	for i := 0; i < 4; i++ {
		_, err := db.ExecContext(ctx, `INSERT INTO probe_observations
			(id, probe_run_id, node_logical_id, kind, verdict, observed_at, connection_revision, evidence_data)
			SELECT ? || logical_id, 'speed-filter-run', logical_id, 'speed', 'available', ?, 1, '{"throughput":50}'
			FROM nodes WHERE logical_id >= 'speed-filter-009'`, fmt.Sprintf("history-%d-", i), now.AddDate(0, 0, -i-2).Format(time.RFC3339))
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range enabledDiagnosticQueries(filter) {
		t.Run("indexed_latest_"+q.name, func(t *testing.T) {
			rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+q.query, q.args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			indexed := false
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				t.Log(detail)
				if strings.HasPrefix(detail, "SCAN po ") || detail == "SCAN po" {
					t.Fatal("full observation scan:", detail)
				}
				if strings.HasPrefix(detail, "SEARCH po USING") && strings.Contains(detail, "node_logical_id=? AND kind=?") && (strings.Contains(detail, "idx_probe_obs_node_kind_observed_id") || strings.Contains(detail, "idx_probe_obs_latest_lookup")) {
					indexed = true
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !indexed {
				t.Fatal("latest speed lookup did not use a node/kind/time/id index")
			}
		})
	}
	plain, total, err := nodes.List(ctx, filter)
	if err != nil || total != 101 || len(plain) != 1 || plain[0].LogicalID != "speed-filter-109" {
		t.Fatalf("list: total=%d page=%v err=%v", total, plain, err)
	}
	for page, wantLen := range []int{100, 1} {
		filter.Pagination.Page = page + 1
		items, total, err := nodes.ListReadModel(ctx, filter)
		if err != nil || total != 101 || len(items) != wantLen {
			t.Fatalf("page %d: total=%d len=%d err=%v", page+1, total, len(items), err)
		}
		for i, item := range items {
			if want := fmt.Sprintf("speed-filter-%03d", 9+page*100+i); item.Node.LogicalID != want {
				t.Fatalf("page %d row %d: got %s, want %s", page+1, i, item.Node.LogicalID, want)
			}
		}
	}
	filter.HealthStatuses = []string{"unknown"}
	filter.IPRiskBands = []string{"unknown"}
	_, total, err = nodes.ListReadModel(ctx, filter)
	if err != nil || total != 101 {
		t.Fatalf("health and risk intersection: total=%d err=%v", total, err)
	}
	filter.HealthStatuses = nil
	filter.IPRiskBands = nil
	above := 100.01
	filter.SpeedMinKiBS = &above
	_, total, err = nodes.List(ctx, filter)
	if err != nil || total != 0 {
		t.Fatalf("above boundary: total=%d err=%v", total, err)
	}
	filter.SpeedMinKiBS = nil
	_, total, err = nodes.List(ctx, filter)
	if err != nil || total != 110 {
		t.Fatalf("no speed condition: total=%d err=%v", total, err)
	}

	// The active-policy fast projection reads from nodes, while the full CTE
	// resolves the head. Both must expose the revision that admitted the speed.
	if _, err := db.Exec(`UPDATE nodes SET connection_revision=2 WHERE logical_id='speed-filter-109'`); err != nil {
		t.Fatal(err)
	}
	if err := NewIPRiskProviderSettingsRepository(db).Upsert(ctx, &domain.IPRiskProviderSettings{Provider: "scamalytics", SchemaVersion: "v1", Enabled: true, SecretReference: "secret://test/risk", MaxConcurrency: 1, RequestsPerMinute: 60, DailyRequestBudget: 100, PerRequestTimeout: time.Second, MaxResponseBytes: 1024}); err != nil {
		t.Fatal(err)
	}
	policy := domain.RiskPolicy{RevisionID: domain.MustNewUUIDv7(), ProviderSelection: domain.RiskProviderSelection{Mode: domain.RiskFusionSingleProvider, Providers: []domain.ProviderRef{{Provider: "scamalytics", SchemaVersion: "v1"}}}, MaxObservationAge: time.Hour, MinimumConfidence: 50, ScoreBands: []domain.ScoreBand{{Min: 0, Max: 100, Band: domain.RiskBandHigh, Action: domain.RiskActionReview}}, UnknownAction: domain.RiskActionReview}
	policies := NewRiskPolicyRevisionRepository(db)
	if err := policies.Create(ctx, &domain.RiskPolicyRevision{RiskPolicy: policy, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := policies.SetActive(ctx, policy.RevisionID, true); err != nil {
		t.Fatal(err)
	}
	filter.SpeedMinKiBS = &speed
	for _, bands := range [][]domain.RiskBand{nil, {domain.RiskBandUnknown}} {
		filter.RiskBands = bands
		items, total, err := nodes.ListReadModel(ctx, filter)
		if err != nil || total != 101 || len(items) != 1 || items[0].Node.LogicalID != "speed-filter-109" || items[0].Node.ConnectionRevision != 1 {
			t.Fatalf("risk bands=%v: total=%d page=%v err=%v", bands, total, items, err)
		}
	}
}
