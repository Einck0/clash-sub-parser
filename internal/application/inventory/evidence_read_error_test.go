package inventory_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

type failingNodeSourceRepository struct {
	domain.NodeSourceRepository
	err         error
	batchIDs    []string
	batchCalls  int
	singleCalls int
}

func (r *failingNodeSourceRepository) ListByNodes(ctx context.Context, logicalIDs []string) (map[string][]domain.NodeSource, error) {
	r.batchCalls++
	r.batchIDs = append([]string(nil), logicalIDs...)
	if r.err != nil {
		return nil, r.err
	}
	return r.NodeSourceRepository.ListByNodes(ctx, logicalIDs)
}

func (r *failingNodeSourceRepository) ListByNode(ctx context.Context, logicalID string) ([]domain.NodeSource, error) {
	r.singleCalls++
	return r.NodeSourceRepository.ListByNode(ctx, logicalID)
}

type failingProbeObservationRepository struct {
	domain.ProbeObservationRepository
	err        error
	batchIDs   []string
	batchCalls int
}

func (r *failingProbeObservationRepository) ListLatestByNodes(ctx context.Context, logicalIDs []string, kinds []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	r.batchCalls++
	r.batchIDs = append([]string(nil), logicalIDs...)
	if r.err != nil {
		return nil, r.err
	}
	return r.ProbeObservationRepository.ListLatestByNodes(ctx, logicalIDs, kinds)
}

func assertNodeEvidenceUnavailable(t *testing.T, err error) {
	t.Helper()
	var domErr *domain.DomainError
	if err == nil || !errors.As(err, &domErr) {
		t.Fatalf("expected node evidence domain error, got %v", err)
	}
	if domErr.Code != "node_evidence_unavailable" || domErr.Message != "Unable to read node evidence; refresh and try again" || domErr.Category != domain.CategoryInternal || len(domErr.Details) != 0 {
		t.Fatalf("unexpected domain error: %+v", domErr)
	}
}

func TestInventoryService_NodeEvidenceReadFailuresFailClosed(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	node := domain.Node{LogicalID: "node-evidence-error", DisplayName: "Evidence Error", Protocol: domain.ProtocolSS, Server: "192.0.2.10", Port: 8388, Active: true}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}

	for _, failure := range []string{"source", "probe"} {
		t.Run(failure, func(t *testing.T) {
			sources := &failingNodeSourceRepository{NodeSourceRepository: sourceRepo}
			observations := &failingProbeObservationRepository{ProbeObservationRepository: sqlite.NewProbeObservationRepository(db)}
			// These are synthetic failure markers, not secrets or live providers.
			injected := errors.New("SQL SELECT private_evidence: password=synthetic-read-failure")
			if failure == "source" {
				sources.err = injected
			} else {
				observations.err = injected
			}
			svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sources, nil, inventory.WithProbeObservationRepository(observations))
			for _, healthStatuses := range [][]string{nil, {"unknown"}} {
				views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
					Pagination:     domain.Pagination{Page: 1, PageSize: 10},
					Scope:          domain.NodeScopeAllAssets,
					HealthStatuses: healthStatuses,
				})
				assertNodeEvidenceUnavailable(t, err)
				if views != nil || total != 0 {
					t.Fatalf("evidence failure returned a partial page: views=%+v total=%d", views, total)
				}
			}
			detail, err := svc.GetNodeDetailWithRisk(ctx, node.LogicalID, "")
			assertNodeEvidenceUnavailable(t, err)
			if detail != nil {
				t.Fatalf("evidence failure returned partial detail: %+v", detail)
			}
		})
	}
}

func TestInventoryService_NodeEvidenceEmptyIsUnknownAndBatchScoped(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	nodes := make([]domain.Node, 5)
	for i := range nodes {
		nodes[i] = domain.Node{LogicalID: "node-evidence-page-" + string(rune('a'+i)), DisplayName: "Evidence Page " + string(rune('a'+i)), Protocol: domain.ProtocolSS, Server: "192.0.2.20", Port: 8388, Active: true}
	}
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatal(err)
	}

	recordingSources := &failingNodeSourceRepository{NodeSourceRepository: sourceRepo}
	recordingObservations := &failingProbeObservationRepository{ProbeObservationRepository: sqlite.NewProbeObservationRepository(db)}
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, recordingSources, nil, inventory.WithProbeObservationRepository(recordingObservations))
	filter := domain.NodeFilter{Pagination: domain.Pagination{Page: 2, PageSize: 2}, Scope: domain.NodeScopeAllAssets, SortBy: "display_name", SortOrder: "asc"}
	wantIDs := []string{nodes[2].LogicalID, nodes[3].LogicalID}
	views, total, err := svc.ListNodesReadModel(ctx, filter)
	if err != nil || len(views) != 2 || total != 5 {
		t.Fatalf("expected a two-node page of five: views=%+v total=%d err=%v", views, total, err)
	}
	gotIDs := make([]string, 0, len(views))
	for _, view := range views {
		gotIDs = append(gotIDs, view.LogicalID)
		if view.HealthStatus != "unknown" || !view.ProbeMissing || len(view.Sources) != 0 {
			t.Fatalf("empty evidence should remain unknown/missing: %+v", view)
		}
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("page IDs changed: got %v want %v", gotIDs, wantIDs)
	}
	if !reflect.DeepEqual(recordingSources.batchIDs, wantIDs) || !reflect.DeepEqual(recordingObservations.batchIDs, wantIDs) || recordingSources.batchCalls != 1 || recordingObservations.batchCalls != 1 {
		t.Fatalf("evidence reads escaped page: sources=%v observations=%v page=%v", recordingSources.batchIDs, recordingObservations.batchIDs, wantIDs)
	}

	detail, err := svc.GetNodeDetailWithRisk(ctx, nodes[0].LogicalID, "")
	if err != nil || detail == nil {
		t.Fatalf("empty evidence detail failed: detail=%+v err=%v", detail, err)
	}
	if detail.HealthStatus != "unknown" || !detail.ProbeMissing || detail.Sources == nil || len(detail.Sources) != 0 {
		t.Fatalf("empty detail evidence should remain unknown/missing: %+v", detail)
	}
	if !reflect.DeepEqual(recordingSources.batchIDs, []string{nodes[0].LogicalID}) || !reflect.DeepEqual(recordingObservations.batchIDs, []string{nodes[0].LogicalID}) || recordingSources.singleCalls != 0 {
		t.Fatalf("detail should use a single-node batch without a redundant source read: sources=%+v observations=%+v", recordingSources, recordingObservations)
	}

	// Both failure injection points must remain untouched for an empty page,
	// even when the overall result set has nodes on earlier pages.
	recordingSources.err = errors.New("unexpected source read on empty page")
	recordingObservations.err = errors.New("unexpected probe read on empty page")
	sourceCalls, probeCalls := recordingSources.batchCalls, recordingObservations.batchCalls
	views, total, err = svc.ListNodesReadModel(ctx, domain.NodeFilter{Pagination: domain.Pagination{Page: 4, PageSize: 2}, Scope: domain.NodeScopeAllAssets})
	if err != nil || len(views) != 0 || total != 5 {
		t.Fatalf("out-of-range page should succeed without evidence: views=%+v total=%d err=%v", views, total, err)
	}
	if recordingSources.batchCalls != sourceCalls || recordingObservations.batchCalls != probeCalls {
		t.Fatalf("empty page queried evidence: source calls=%d probe calls=%d", recordingSources.batchCalls, recordingObservations.batchCalls)
	}
}
