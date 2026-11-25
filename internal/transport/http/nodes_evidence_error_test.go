package http_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
)

type nodeEvidenceSourceFailure struct {
	domain.NodeSourceRepository
}

func (r nodeEvidenceSourceFailure) ListByNodes(context.Context, []string) (map[string][]domain.NodeSource, error) {
	return nil, errors.New("SELECT private_evidence FROM node_sources: password=synthetic-evidence-secret")
}

func nodeEvidenceTestRouter(db *sql.DB, sources domain.NodeSourceRepository, observations domain.ProbeObservationRepository) http.Handler {
	cfg := newTestRouterConfig(true)
	cfg.InventoryService = inventory.NewService(
		db, sqlite.NewSubscriptionRepository(db), sqlite.NewSubscriptionFetchRepository(db), sqlite.NewNodeRepository(db), sources, nil,
		inventory.WithProbeObservationRepository(observations),
	)
	return transporthttp.NewRouter(cfg)
}

func TestNodesEvidenceReadFailureEnvelope(t *testing.T) {
	db := newCleanSQLiteDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)
	nodes := sqlite.NewNodeRepository(db)
	const nodeID = "node_0000000000000009"
	if err := nodes.UpsertBatch(ctx, []domain.Node{{LogicalID: nodeID, DisplayName: "Evidence read fixture", Protocol: domain.ProtocolSS, Server: "192.0.2.9", Port: 8388, Active: true}}); err != nil {
		t.Fatal(err)
	}
	node, err := nodes.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: "evidence-read-run", IdempotencyKey: "evidence-read-run", State: domain.ProbeRunStateSucceeded, DeadlineAt: now}); err != nil {
		t.Fatal(err)
	}
	speed := 100.0
	observations := sqlite.NewProbeObservationRepository(db)
	if err := observations.Create(ctx, &domain.ProbeObservation{ID: "evidence-read-speed", ProbeRunID: "evidence-read-run", NodeLogicalID: nodeID, Kind: domain.ProbeKindSpeed, Verdict: domain.VerdictAvailable, ConnectionRevision: &node.ConnectionRevision, ObservedAt: now, Throughput: &speed}); err != nil {
		t.Fatal(err)
	}
	sources := sqlite.NewNodeSourceRepository(db)
	base := "/api/v1/nodes?scope=all_assets"
	speedPath := base + "&speed_min_kib_s=100"
	control := nodeEvidenceTestRouter(db, sources, observations)
	rec, page := getNodesPaginated(t, control, speedPath, testAdminToken)
	if rec.Code != http.StatusOK || page.Data.Total != 1 || len(page.Data.Items) != 1 || page.Data.Items[0].LogicalID != nodeID {
		t.Fatalf("speed fixture must match before injecting an enrichment failure: %d %s", rec.Code, rec.Body.String())
	}

	for _, failure := range []string{"source", "probe"} {
		t.Run(failure, func(t *testing.T) {
			sourceRepo, observationRepo := sources, observations
			if failure == "source" {
				sourceRepo = nodeEvidenceSourceFailure{NodeSourceRepository: sources}
			} else {
				observationRepo = speedReadBoundary{ProbeObservationRepository: observations, beforeRead: func() error {
					return errors.New("SELECT private_evidence FROM probe_observations: password=synthetic-evidence-secret")
				}}
			}
			router := nodeEvidenceTestRouter(db, sourceRepo, observationRepo)
			for _, tc := range []struct {
				path, code, message string
			}{
				{base, "node_evidence_unavailable", "Unable to read node evidence; refresh and try again"},
				{base + "&health_status=unknown", "node_evidence_unavailable", "Unable to read node evidence; refresh and try again"},
				{"/api/v1/nodes/" + nodeID, "node_evidence_unavailable", "Unable to read node evidence; refresh and try again"},
				{speedPath, "speed_evidence_unavailable", "Unable to read speed evidence; refresh the node list"},
			} {
				rec, response := getNodesError(t, router, tc.path, testAdminToken)
				if rec.Code != http.StatusInternalServerError || response.Code != tc.code || response.Message != tc.message || response.RequestID == "" {
					t.Fatalf("%s: expected safe 500/%s/request ID, got %d %+v", tc.path, tc.code, rec.Code, response)
				}
				var body map[string]json.RawMessage
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body) != 3 || body["data"] != nil {
					t.Fatalf("failure must return only error fields, not a partial success: %s (err=%v)", rec.Body.String(), err)
				}
				for _, forbidden := range []string{"synthetic-evidence-secret", "private_evidence", "SELECT", "node_sources", "probe_observations"} {
					if strings.Contains(rec.Body.String(), forbidden) {
						t.Fatalf("public error leaked %q: %s", forbidden, rec.Body.String())
					}
				}
			}
		})
	}
}

func TestNodesMissingEvidenceStillSucceeds(t *testing.T) {
	db := newCleanSQLiteDB(t)
	const nodeID = "node_0000000000000008"
	if err := sqlite.NewNodeRepository(db).UpsertBatch(context.Background(), []domain.Node{{LogicalID: nodeID, DisplayName: "No evidence fixture", Protocol: domain.ProtocolSS, Server: "192.0.2.8", Port: 8388, Active: true}}); err != nil {
		t.Fatal(err)
	}
	router := nodeEvidenceTestRouter(db, sqlite.NewNodeSourceRepository(db), sqlite.NewProbeObservationRepository(db))
	rec, page := getNodesPaginated(t, router, "/api/v1/nodes?scope=all_assets", testAdminToken)
	if rec.Code != http.StatusOK || page.Data.Total != 1 || len(page.Data.Items) != 1 {
		t.Fatalf("missing evidence list failed: %d %s", rec.Code, rec.Body.String())
	}
	assertUnknown := func(view inventory.NodeView) {
		t.Helper()
		if view.LogicalID != nodeID || view.HealthStatus != "unknown" || !view.ProbeMissing || len(view.Sources) != 0 {
			t.Fatalf("missing evidence must remain unknown/missing: %+v", view)
		}
	}
	assertUnknown(page.Data.Items[0])

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+nodeID, nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var detail nodeDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("missing evidence detail failed: status=%d body=%s err=%v", rec.Code, rec.Body.String(), err)
	}
	assertUnknown(detail.Data.Node)
	if detail.Data.Sources == nil || len(detail.Data.Sources) != 0 {
		t.Fatalf("missing evidence detail should have empty sources: %+v", detail)
	}
}
