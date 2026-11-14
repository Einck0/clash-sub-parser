package http_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestNodesIPRiskQueryValidationAndPagination(t *testing.T) {
	db := newCleanSQLiteDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)
	repo := sqlite.NewNodeRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	if err := sqlite.NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: "http-risk-run", IdempotencyKey: "http-risk-run", State: domain.ProbeRunStateSucceeded, DeadlineAt: now}); err != nil {
		t.Fatal(err)
	}
	for i, score := range []string{"0", "24", "25", "64", "65", "100", "malformed"} {
		id := fmt.Sprintf("http-risk-%d", i)
		if err := repo.UpsertBatch(ctx, []domain.Node{{LogicalID: id, Protocol: domain.ProtocolSS, DisplayName: id, Server: "192.0.2.1", Port: 8388, Active: true, CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatal(err)
		}
		node, err := repo.GetByLogicalID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := obsRepo.Create(ctx, &domain.ProbeObservation{ID: "obs-" + id, ProbeRunID: "http-risk-run", NodeLogicalID: id, Kind: domain.ProbeKindIPRisk, Verdict: domain.VerdictAvailable, ConnectionRevision: &node.ConnectionRevision, ObservedAt: now, RiskScore: score}); err != nil {
			t.Fatal(err)
		}
	}
	router := setupRegressionTestRouter(t, db, nil)
	for _, query := range []string{"ip_risk_score_max=-1", "ip_risk_score_max=101", "ip_risk_score_max=1.5", "ip_risk_score_max=garbage", "ip_risk_score_max=", "ip_risk_score_max=0&ip_risk_score_max=1", "ip_risk_band=critical", "ip_risk_band=", "ip_risk_band=low,garbage"} {
		rec, _ := getNodesError(t, router, "/api/v1/nodes?"+query, testAdminToken)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", query, rec.Code)
		}
	}
	for _, tc := range []struct {
		query string
		total int
		id    string
	}{
		{"ip_risk_score_max=0", 1, "http-risk-0"},
		{"ip_risk_band=low", 2, "http-risk-0"},
		{"ip_risk_band=medium", 2, "http-risk-2"},
		{"ip_risk_band=high", 2, "http-risk-4"},
		{"ip_risk_band=unknown", 1, "http-risk-6"},
		{"ip_risk_band=low,medium&ip_risk_score_max=25&page=2", 3, "http-risk-1"},
	} {
		rec, resp := getNodesPaginated(t, router, "/api/v1/nodes?scope=all_assets&active_only=true&protocol=ss&search=http-risk-&sort_by=display_name&page_size=1&"+tc.query, testAdminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", tc.query, rec.Code, rec.Body.String())
		}
		if resp.Data.Total != tc.total || len(resp.Data.Items) != 1 || resp.Data.Items[0].LogicalID != tc.id {
			t.Errorf("%s: total=%d items=%+v", tc.query, resp.Data.Total, resp.Data.Items)
		}
	}
}
