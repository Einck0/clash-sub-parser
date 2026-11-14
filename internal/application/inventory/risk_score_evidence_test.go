package inventory_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// Traverse persistence, SQL filtering, repository hydration and both API view
// projections: a valid risk score must survive an unrelated malformed field.
func TestRiskScoreEvidenceListAndDetailParity(t *testing.T) {
	for _, tc := range []struct {
		name, evidence, wantScore string
		verdict                   domain.ProbeVerdict
		oldRevision               bool
		known                     bool
	}{
		{"unrelated_invalid_field", `{"risk_score":"0","throughput":"invalid"}`, "0", domain.VerdictAvailable, false, true},
		{"invalid_score_type", `{"risk_score":0,"throughput":"invalid"}`, "", domain.VerdictAvailable, false, false},
		{"invalid_score_string", `{"risk_score":"invalid","throughput":"invalid"}`, "invalid", domain.VerdictAvailable, false, false},
		{"invalid_json", `{"risk_score":"0",`, "", domain.VerdictAvailable, false, false},
		{"latest_failed", `{"risk_score":"0","throughput":"invalid"}`, "0", domain.VerdictError, false, false},
		{"old_revision", `{"risk_score":"0","throughput":"invalid"}`, "0", domain.VerdictAvailable, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, subs, fetches, nodes, sources := setupTestEnv(t)
			ctx := context.Background()
			obsRepo := sqlite.NewProbeObservationRepository(db)
			svc := inventory.NewService(db, subs, fetches, nodes, sources, nil, inventory.WithProbeObservationRepository(obsRepo))
			now := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
			node := domain.Node{LogicalID: "risk-evidence", DisplayName: "Risk evidence", Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 443, Active: true, CreatedAt: now, UpdatedAt: now, Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "fixture"}}
			if err := nodes.UpsertBatch(ctx, []domain.Node{node}); err != nil {
				t.Fatal(err)
			}
			stored, err := nodes.GetByLogicalID(ctx, node.LogicalID)
			if err != nil {
				t.Fatal(err)
			}
			revision := stored.ConnectionRevision
			if revision < 1 {
				t.Fatal("fixture must have a current connection revision")
			}
			runID := domain.MustNewUUIDv7()
			if err := sqlite.NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: runID, IdempotencyKey: runID, State: domain.ProbeRunStateSucceeded, DeadlineAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatal(err)
			}
			if tc.verdict == domain.VerdictError {
				if err := obsRepo.Create(ctx, &domain.ProbeObservation{ID: domain.MustNewUUIDv7(), ProbeRunID: runID, NodeLogicalID: node.LogicalID, Kind: domain.ProbeKindIPRisk, Verdict: domain.VerdictAvailable, ConnectionRevision: &revision, ObservedAt: now.Add(-time.Minute), RiskScore: "0"}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.oldRevision {
				revision--
			}
			if err := obsRepo.Create(ctx, &domain.ProbeObservation{ID: domain.MustNewUUIDv7(), ProbeRunID: runID, NodeLogicalID: node.LogicalID, Kind: domain.ProbeKindIPRisk, Verdict: tc.verdict, ConnectionRevision: &revision, ObservedAt: now, EvidenceData: tc.evidence}); err != nil {
				t.Fatal(err)
			}
			latest, err := obsRepo.ListLatestByNodes(ctx, []string{node.LogicalID}, nil)
			if err != nil {
				t.Fatal(err)
			}
			band := domain.IPRiskScoreBand(0, tc.known)
			matched, reason := domain.MatchesCondition(domain.FilterCondition{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: band}, *stored, nil, latest[node.LogicalID], time.Now().UTC())
			if !matched {
				t.Fatalf("policy condition disagrees: %s", reason)
			}
			filter := domain.NodeFilter{Pagination: domain.Pagination{Page: 1, PageSize: 10}, IPRiskBands: []string{band}}
			views, total, err := svc.ListNodesReadModel(ctx, filter)
			if err != nil || total != 1 || len(views) != 1 {
				t.Fatalf("list by %s: views=%d total=%d err=%v", band, len(views), total, err)
			}
			detail, err := svc.GetNodeDetailWithRisk(ctx, node.LogicalID, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, view := range []inventory.NodeView{views[0], detail.ToNodeView()} {
				// Check the serialized API projection consumed by the risk badge.
				raw, err := json.Marshal(view)
				if err != nil {
					t.Fatal(err)
				}
				var response struct {
					Capabilities map[string]struct {
						RiskScore  string              `json:"risk_score"`
						Verdict    domain.ProbeVerdict `json:"verdict"`
						Stale      bool                `json:"stale"`
						Throughput *float64            `json:"throughput"`
					} `json:"capabilities"`
				}
				if err := json.Unmarshal(raw, &response); err != nil {
					t.Fatal(err)
				}
				cap := response.Capabilities["ip_risk"]
				_, validScore := domain.ParseIPRiskScore(cap.RiskScore)
				badgeKnown := cap.Verdict == domain.VerdictAvailable && !cap.Stale && validScore
				if cap.RiskScore != tc.wantScore || badgeKnown != tc.known || cap.Throughput != nil {
					t.Fatalf("API risk evidence disagrees with filter or changed throughput semantics: %+v", cap)
				}
			}
			zero := 0
			filter.IPRiskBands = nil
			filter.IPRiskScoreMax = &zero
			_, total, err = svc.ListNodesReadModel(ctx, filter)
			wantTotal := 0
			if tc.known {
				wantTotal = 1
			}
			if err != nil || total != wantTotal {
				t.Fatalf("score0 filter: total=%d want=%d err=%v", total, wantTotal, err)
			}
		})
	}
}
