package publication_test

import (
	"context"
	"errors"
	"testing"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

type draftSourceBatchRepo struct {
	domain.NodeSourceRepository
	calls int
	ids   [][]string
	err   error
}

func (r *draftSourceBatchRepo) ListByNodes(_ context.Context, ids []string) (map[string][]domain.NodeSource, error) {
	r.calls++
	r.ids = append(r.ids, append([]string(nil), ids...))
	if r.err != nil {
		return nil, r.err
	}
	result := make(map[string][]domain.NodeSource, len(ids))
	for _, id := range ids {
		result[id] = []domain.NodeSource{{NodeLogicalID: id, SubscriptionID: "sub-current"}}
	}
	return result, nil
}

type draftObservationBatchRepo struct {
	domain.ProbeObservationRepository
	calls int
	ids   [][]string
	err   error
}

func (r *draftObservationBatchRepo) ListLatestByNodes(_ context.Context, ids []string, _ []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	r.calls++
	r.ids = append(r.ids, append([]string(nil), ids...))
	if r.err != nil {
		return nil, r.err
	}
	return map[string]map[domain.ProbeKind]domain.ProbeObservation{}, nil
}

func draftFilterService(t *testing.T, policy *mockPolicyRepo, sources domain.NodeSourceRepository, observations domain.ProbeObservationRepository) *publication.Service {
	t.Helper()
	ctx := context.Background()
	revisions := &mockRevisionRepo{}
	if err := revisions.CreateActive(ctx, &domain.ConfigurationRevision{ID: "rev-draft-dependencies", ContentDigest: "sha256:draft-dependencies"}); err != nil {
		t.Fatal(err)
	}
	if policy == nil {
		policy = newMockPolicyRepo()
	}
	return publication.NewService(
		newMockPublicationRepo(), &mockAuditRepo{},
		publication.WithPolicyRepository(policy),
		publication.WithRevisionRepository(revisions),
		publication.WithNodeRepository(setupSampleNodeSource()),
		publication.WithResolver(resolver.New()),
		publication.WithNodeSourceRepository(sources),
		publication.WithProbeObservationRepository(observations),
	)
}

func draftSourceFilter() *domain.NodeFilterSpec {
	return &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{
		Field: domain.FilterFieldSourceSubscriptions,
		Op:    domain.FilterOpContains,
		Value: "sub-current",
	}}}
}

func draftProbeFilter() *domain.NodeFilterSpec {
	kind := domain.ProbeKindBaseline
	return &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{
		Field: domain.FilterFieldProbeVerdict,
		Op:    domain.FilterOpEquals,
		Value: string(domain.VerdictAvailable), ProbeKind: &kind,
	}}}
}

func TestAssembleResolveInputForPreviewLoadsDraftDependenciesOnce(t *testing.T) {
	ctx := context.Background()
	sources := &draftSourceBatchRepo{}
	observations := &draftObservationBatchRepo{}
	svc := draftFilterService(t, nil, sources, observations)

	// A regex-only, unsaved draft has no evidence dependency; dependency loading remains lazy.
	noEvidence := &domain.NodeGroup{ID: "draft-no-evidence", Sources: []domain.MemberSource{{
		ID: "source-regex", GroupID: "draft-no-evidence", Operation: domain.OperationAdd,
		Kind: domain.SourceKindRegex, Pattern: draftTestStringPtr(".*"),
	}}}
	input, err := svc.AssembleResolveInputForPreview(ctx, "", noEvidence)
	if err != nil {
		t.Fatalf("assemble dependency-free draft: %v", err)
	}
	if len(input.Groups) != 1 || input.Groups[0].ID != noEvidence.ID {
		t.Fatalf("draft overlay missing from assembled groups: %+v", input.Groups)
	}
	if sources.calls != 0 || observations.calls != 0 {
		t.Fatalf("dependency-free draft caused evidence reads: source=%d observations=%d", sources.calls, observations.calls)
	}

	// Adding source and probe conditions must cause one batch read per required evidence type.
	combined := draftSourceFilter()
	combined.Conditions = append(combined.Conditions, draftProbeFilter().Conditions...)
	withEvidence := &domain.NodeGroup{ID: "draft-with-evidence", Sources: []domain.MemberSource{{
		ID: "source-filter", GroupID: "draft-with-evidence", Operation: domain.OperationAdd,
		Kind: domain.SourceKindFilter, FilterSpec: combined,
	}}}
	input, err = svc.AssembleResolveInputForPreview(ctx, "", withEvidence)
	if err != nil {
		t.Fatalf("assemble dependency-bearing draft: %v", err)
	}
	if got := input.NodeSources["node-hk-01"]; len(got) != 1 || got[0].SubscriptionID != "sub-current" {
		t.Fatalf("draft source-filter facts missing for active candidate: %+v", input.NodeSources)
	}
	if sources.calls != 1 || observations.calls != 1 {
		t.Fatalf("draft dependencies were not batched once: source=%d observations=%d", sources.calls, observations.calls)
	}
	if len(sources.ids[0]) != 3 || len(observations.ids[0]) != 3 {
		t.Fatalf("batches should contain only the three active inventory candidates: source=%v observations=%v", sources.ids[0], observations.ids[0])
	}
}

func TestAssembleResolveInputForPreviewReplacedDraftDropsRemovedDependency(t *testing.T) {
	ctx := context.Background()
	persisted := newMockPolicyRepo()
	if err := persisted.CreateGroup(ctx, &domain.NodeGroup{ID: "group-edit", Name: "Edit me", GroupType: domain.GroupTypeSelect, Sources: []domain.MemberSource{{Kind: domain.SourceKindFilter, Operation: domain.OperationAdd, FilterSpec: draftProbeFilter()}}}); err != nil {
		t.Fatal(err)
	}
	sources := &draftSourceBatchRepo{}
	observations := &draftObservationBatchRepo{}
	svc := draftFilterService(t, persisted, sources, observations)

	updated := &domain.NodeGroup{ID: "group-edit", Name: "Edit me", GroupType: domain.GroupTypeSelect,
		Sources: []domain.MemberSource{{ID: "manual-source", GroupID: "group-edit", Operation: domain.OperationAdd,
			Kind: domain.SourceKindManualNode, NodeLogicalID: draftTestStringPtr("node-hk-01")}}}
	input, err := svc.AssembleResolveInputForPreview(ctx, "", updated)
	if err != nil {
		t.Fatalf("assemble replacement draft without former dependency: %v", err)
	}
	if len(input.Groups) != 1 || len(input.Groups[0].Sources) != 1 || input.Groups[0].Sources[0].Kind != domain.SourceKindManualNode {
		t.Fatalf("removed persisted filter survived draft replacement: %+v", input.Groups)
	}
	if sources.calls != 0 || observations.calls != 0 {
		t.Fatalf("removed draft dependency still caused reads: source=%d observations=%d", sources.calls, observations.calls)
	}
}

func TestAssembleResolveInputForPreviewPropagatesDraftDependencyReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  *draftSourceBatchRepo
		obs  *draftObservationBatchRepo
		code string
	}{
		{name: "sources", src: &draftSourceBatchRepo{err: errors.New("source backend unavailable")}, obs: &draftObservationBatchRepo{}, code: "node_sources_unavailable"},
		{name: "observations", src: &draftSourceBatchRepo{}, obs: &draftObservationBatchRepo{err: errors.New("observation backend unavailable")}, code: "probe_observations_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := draftFilterService(t, nil, tc.src, tc.obs)
			combined := draftSourceFilter()
			combined.Conditions = append(combined.Conditions, draftProbeFilter().Conditions...)
			draft := &domain.NodeGroup{ID: "draft-error", Sources: []domain.MemberSource{{
				ID: "source-error", GroupID: "draft-error", Operation: domain.OperationAdd,
				Kind: domain.SourceKindFilter, FilterSpec: combined,
			}}}
			_, err := svc.AssembleResolveInputForPreview(context.Background(), "", draft)
			if err == nil {
				t.Fatal("expected dependency batch read failure")
			}
			domainErr, ok := domain.AsDomainError(err)
			if !ok || domainErr.Code != tc.code {
				t.Fatalf("expected domain error %q, got %v", tc.code, err)
			}
			if tc.src != nil && tc.src.calls != 1 {
				t.Fatalf("source batch calls = %d, want 1", tc.src.calls)
			}
			wantObsCalls := 1
			if tc.name == "sources" {
				wantObsCalls = 0
			}
			if tc.obs.calls != wantObsCalls {
				t.Fatalf("observation batch calls = %d, want %d", tc.obs.calls, wantObsCalls)
			}
		})
	}
}

func draftTestStringPtr(value string) *string { return &value }
