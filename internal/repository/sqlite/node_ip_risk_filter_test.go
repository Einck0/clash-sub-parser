package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestNodeIPRiskPredicatesDomainParityAndPagination(t *testing.T) {
	db := setupNodeProjectionDB(t)
	ctx := context.Background()
	repo := NewNodeRepository(db)
	obsRepo := NewProbeObservationRepository(db)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	rev, old := int64(1), int64(0)
	if err := NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: "risk-run", IdempotencyKey: "risk-run", State: domain.ProbeRunStateSucceeded, DeadlineAt: now}); err != nil {
		t.Fatal(err)
	}
	rawScores := []string{"0", "24", "25", "64", "65", "100", "15% low", "15%", "15 % low", "\t15%\tlow\n", "\u200315\u00a0", "", "garbage", "00", "01", "-1", "+1", "1.0", "101", "15junk", "15%low", "15%% low", "15% low%", "15%\u00a0low", "15%\u0085low", "0\x00garbage", "15% label\x00tail", "15% label\x00%", "15%\x00"}
	type fixture struct {
		node   domain.Node
		latest map[domain.ProbeKind]domain.ProbeObservation
	}
	var fixtures []fixture
	add := func(score, scenario string) {
		t.Helper()
		id := fmt.Sprintf("risk-%03d", len(fixtures))
		node := domain.Node{LogicalID: id, DisplayName: id, Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
			t.Fatal(err)
		}
		stored, err := repo.GetByLogicalID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		obs := domain.ProbeObservation{ID: "obs-" + id, ProbeRunID: "risk-run", NodeLogicalID: id, Kind: domain.ProbeKindIPRisk, Verdict: domain.VerdictAvailable, ConnectionRevision: &rev, ObservedAt: now.Add(-24 * time.Hour), RiskScore: score}
		switch scenario {
		case "error":
			obs.Verdict = domain.VerdictError
		case "restricted":
			obs.Verdict = domain.VerdictRestricted
		case "old":
			obs.ConnectionRevision = &old
		case "nil":
			obs.ConnectionRevision = nil
		case "future":
			obs.ObservedAt = now.Add(time.Hour)
		case "missing":
			fixtures = append(fixtures, fixture{node: *stored})
			return
		}
		if scenario == "latest_error" {
			prior := obs
			prior.ID += "-prior"
			prior.ObservedAt = obs.ObservedAt.Add(-time.Hour)
			if err := obsRepo.Create(ctx, &prior); err != nil {
				t.Fatal(err)
			}
			obs.Verdict = domain.VerdictError
		}
		if err := obsRepo.Create(ctx, &obs); err != nil {
			t.Fatal(err)
		}
		if scenario == "changed_connection" {
			node.Server = "192.0.2.2"
			if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
				t.Fatal(err)
			}
			stored, err = repo.GetByLogicalID(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
		}
		if scenario == "invalid_json" || scenario == "number_json" || scenario == "zero_time" || scenario == "unrelated_json_type" {
			evidence, stamp := obs.EvidenceData, obs.ObservedAt.Format(time.RFC3339)
			if scenario == "invalid_json" {
				evidence = "{"
				obs.RiskScore = ""
			}
			if scenario == "number_json" {
				evidence = `{"risk_score":0}`
				obs.RiskScore = ""
			}
			if scenario == "unrelated_json_type" {
				evidence = `{"risk_score":"0","throughput":"invalid"}`
			}
			if scenario == "zero_time" {
				stamp = "0001-01-01T00:00:00Z"
				obs.ObservedAt = time.Time{}
			}
			obs.EvidenceData = evidence
			if _, err := db.ExecContext(ctx, `UPDATE probe_observations SET evidence_data=?, observed_at=? WHERE id=?`, evidence, stamp, obs.ID); err != nil {
				t.Fatal(err)
			}
		}
		fixtures = append(fixtures, fixture{node: *stored, latest: map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindIPRisk: obs}})
	}
	for _, score := range rawScores {
		add(score, "")
	}
	for _, scenario := range []string{"missing", "error", "restricted", "old", "nil", "future", "latest_error", "invalid_json", "number_json", "zero_time", "changed_connection", "unrelated_json_type"} {
		add("0", scenario)
	}

	for _, band := range []string{"low", "medium", "high", "unknown"} {
		want := map[string]bool{}
		for _, f := range fixtures {
			ok, _ := domain.MatchesCondition(domain.FilterCondition{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: band}, f.node, nil, f.latest, now)
			if ok {
				want[f.node.LogicalID] = true
			}
		}
		filter := domain.NodeFilter{IPRiskBands: []string{band}, Now: &now, SortBy: "display_name", Pagination: domain.Pagination{Page: 1, PageSize: 100}}
		items, total, err := repo.List(ctx, filter)
		if err != nil {
			t.Fatalf("band %s: %v", band, err)
		}
		if total != len(want) || len(items) != len(want) {
			t.Fatalf("band %s: total=%d len=%d want=%v", band, total, len(items), want)
		}
		for _, item := range items {
			if !want[item.LogicalID] {
				t.Errorf("band %s unexpectedly matched %s", band, item.LogicalID)
			}
		}
	}

	// Both the ordinary query and read-model query must filter BEFORE count and
	// pagination, including the risk-policy fast path. Legacy admission bands
	// intentionally differ from these probe score bands.
	policy := domain.RiskPolicy{RevisionID: domain.MustNewUUIDv7(), ProviderSelection: domain.RiskProviderSelection{Mode: domain.RiskFusionSingleProvider, Providers: []domain.ProviderRef{{Provider: "scamalytics", SchemaVersion: "v1"}}}, MaxObservationAge: time.Hour, MinimumConfidence: 50, ScoreBands: []domain.ScoreBand{{Min: 0, Max: 100, Band: domain.RiskBandHigh, Action: domain.RiskActionReview}}, UnknownAction: domain.RiskActionReview}
	for _, activePolicy := range []bool{false, true} {
		if activePolicy {
			if err := NewIPRiskProviderSettingsRepository(db).Upsert(ctx, &domain.IPRiskProviderSettings{
				Provider: "scamalytics", SchemaVersion: "v1", Enabled: true,
				SecretReference: "secret://test/risk", MaxConcurrency: 1, RequestsPerMinute: 60,
				DailyRequestBudget: 100, PerRequestTimeout: time.Second, MaxResponseBytes: 1024,
			}); err != nil {
				t.Fatal(err)
			}
			policyRepo := NewRiskPolicyRevisionRepository(db)
			if err := policyRepo.Create(ctx, &domain.RiskPolicyRevision{RiskPolicy: policy, CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			if err := policyRepo.SetActive(ctx, policy.RevisionID, true); err != nil {
				t.Fatal(err)
			}
		}
		for _, max := range []int{0, 24, 25, 64, 65, 100} {
			var want []string
			for _, f := range fixtures {
				ok, _ := domain.MatchesCondition(domain.FilterCondition{Field: domain.FilterFieldIPRiskScore, Op: domain.FilterOpLTE, Value: fmt.Sprint(max)}, f.node, nil, f.latest, now)
				if ok {
					want = append(want, f.node.LogicalID)
				}
			}
			filter := domain.NodeFilter{IPRiskScoreMax: &max, IPRiskBands: []string{"low", "medium", "high"}, Now: &now, SearchText: "risk-", Protocols: []domain.Protocol{domain.ProtocolSS}, ActiveOnly: true, SortBy: "display_name", Pagination: domain.Pagination{Page: 2, PageSize: 1}}
			items, total, err := repo.ListReadModel(ctx, filter)
			if err != nil {
				t.Fatalf("policy=%v max=%d: %v", activePolicy, max, err)
			}
			if total != len(want) {
				t.Fatalf("policy=%v max=%d total=%d want=%d", activePolicy, max, total, len(want))
			}
			if len(want) > 1 && (len(items) != 1 || items[0].Node.LogicalID != want[1]) {
				encoded, _ := json.Marshal(items)
				t.Fatalf("page 2 max=%d got=%s want=%s", max, encoded, want[1])
			}
			if len(want) <= 1 && len(items) != 0 {
				t.Fatalf("expected empty page 2: %+v", items)
			}
			// Combining the existing admission contract switches away from the
			// fast path, without changing probe-score matches or totals.
			filter.RiskBands = []domain.RiskBand{domain.RiskBandUnknown}
			combined, combinedTotal, err := repo.ListReadModel(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			if combinedTotal != total || len(combined) != len(items) {
				t.Fatalf("legacy and probe risk combination changed pagination: %d/%d vs %d/%d", combinedTotal, len(combined), total, len(items))
			}
		}
	}
}
