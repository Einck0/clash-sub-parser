package publication_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
)

// Embed the existing ports so the test overrides only the read path under test.
// All other methods are deliberately unreachable in these service-level tests.
type filterReadRepo struct {
	domain.NodeFilterRepository
	global              domain.NodeFilterSpec
	groups              map[string]domain.NodeFilterSpec
	globalErr, groupErr error
}

func (r *filterReadRepo) GetGlobalFilter(context.Context) (*domain.GlobalNodeFilter, error) {
	if r.globalErr != nil {
		return nil, r.globalErr
	}
	return &domain.GlobalNodeFilter{Spec: r.global}, nil
}
func (r *filterReadRepo) ListGroupFilters(context.Context) (map[string]domain.NodeFilterSpec, error) {
	return r.groups, r.groupErr
}

type observationReadRepo struct {
	domain.ProbeObservationRepository
	latest map[string]map[domain.ProbeKind]domain.ProbeObservation
	err    error
}

func (r *observationReadRepo) ListLatestByNodes(context.Context, []string, []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	return r.latest, r.err
}

type sourceReadRepo struct {
	domain.NodeSourceRepository
	err error
}

func (r *sourceReadRepo) ListByNodes(context.Context, []string) (map[string][]domain.NodeSource, error) {
	return nil, r.err
}

func probeFilter() domain.NodeFilterSpec {
	kind := domain.ProbeKindBaseline
	return domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{
		Field: domain.FilterFieldProbeVerdict, Op: domain.FilterOpEquals,
		Value: string(domain.VerdictAvailable), ProbeKind: &kind,
	}}}
}

func exportFilterFixture(t *testing.T, withGroup bool, filter *filterReadRepo, observations domain.ProbeObservationRepository, sources domain.NodeSourceRepository) (*publication.Service, *mockPublicationRepo) {
	t.Helper()
	ctx := context.Background()
	revision := newMockRevisionRepo()
	if err := revision.CreateActive(ctx, &domain.ConfigurationRevision{ID: "01944aa0-0000-7000-8000-000000000001", ContentDigest: "sha256:fixture"}); err != nil {
		t.Fatal(err)
	}
	policy := newMockPolicyRepo()
	if withGroup {
		groupID := "01944aa0-0000-7000-8000-000000000002"
		if err := policy.CreateGroup(ctx, &domain.NodeGroup{ID: groupID, Name: "PROXY", GroupType: domain.GroupTypeSelect}); err != nil {
			t.Fatal(err)
		}
		if err := policy.SetEdgesForGroup(ctx, groupID, []domain.GroupEdge{
			{ID: "01944aa0-0000-7000-8000-000000000003", ParentGroupID: groupID, NodeLogicalID: ptr(hkNodeID)},
			{ID: "01944aa0-0000-7000-8000-000000000004", ParentGroupID: groupID, NodeLogicalID: ptr(usNodeID), Position: 1},
		}); err != nil {
			t.Fatal(err)
		}
		if err := policy.CreatePolicyRule(ctx, &domain.PolicyRule{ID: "01944aa0-0000-7000-8000-000000000005", RevisionID: "01944aa0-0000-7000-8000-000000000001", TargetGroupID: groupID, Expression: "MATCH"}); err != nil {
			t.Fatal(err)
		}
	}
	pubs := newMockPublicationRepo()
	nodes := setupSampleNodeSource()
	nodes.mu.Lock()
	for oldID, newID := range map[string]string{"node-hk-01": hkNodeID, "node-us-01": usNodeID} {
		n := *nodes.nodes[oldID]
		delete(nodes.nodes, oldID)
		n.LogicalID = newID
		nodes.nodes[newID] = &n
	}
	delete(nodes.nodes, "node-hy2-01") // QX does not support Hysteria2.
	nodes.mu.Unlock()
	svc := publication.NewService(pubs, &mockAuditRepo{}, publication.WithPolicyRepository(policy), publication.WithRevisionRepository(revision), publication.WithNodeRepository(nodes), publication.WithNodeFilterRepository(filter), publication.WithProbeObservationRepository(observations), publication.WithNodeSourceRepository(sources))
	return svc, pubs
}
func ptr(s string) *string { return &s }

const (
	hkNodeID = "node_0123456789abcdef0123456789abcdef"
	usNodeID = "node_abcdef0123456789abcdef0123456789"
)

var exportTargets = []domain.CompilerTarget{domain.TargetMihomo, domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX}

func assertDeniedNewExport(t *testing.T, svc *publication.Service, pubs *mockPublicationRepo, target domain.CompilerTarget, expected string) {
	t.Helper()
	ctx := context.Background()
	before := len(pubs.publications)
	pre, err := svc.Preflight(ctx, publication.PreflightCommand{Target: target})
	if err == nil {
		if pre == nil || pre.Allowed {
			t.Fatalf("%s preflight allowed unsafe export: %+v", target, pre)
		}
		found := false
		for _, d := range pre.Diagnostics {
			if d.Code == expected {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s preflight missing %s: %+v", target, expected, pre.Diagnostics)
		}
	} else if !strings.Contains(err.Error(), expected) {
		t.Fatalf("%s preflight: expected %s, got %v", target, expected, err)
	}
	assertExportError := func(err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s export succeeded despite %s", target, expected)
		}
		if expected == "filtered_nodes_empty" {
			var preflight *publication.PreflightError
			if !errors.As(err, &preflight) {
				t.Fatalf("%s expected preflight error, got %v", target, err)
			}
			for _, d := range preflight.Result.Diagnostics {
				if d.Code == expected {
					return
				}
			}
			t.Fatalf("%s missing %s diagnostic: %+v", target, expected, preflight.Result.Diagnostics)
		}
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("%s expected %s, got %v", target, expected, err)
		}
	}
	_, err = svc.Preview(ctx, publication.PreviewQuery{Target: target})
	assertExportError(err)
	_, err = svc.Publish(ctx, publication.PublishCommand{Target: target})
	assertExportError(err)
	if len(pubs.publications) != before {
		t.Fatalf("%s created a publication on failed filter", target)
	}
}

func TestExportFilterReadFailuresFailClosedAcrossTargetsAndEntrypoints(t *testing.T) {
	secret := "secret-password-read-failure"
	for _, tc := range []struct {
		name    string
		filter  *filterReadRepo
		obs     domain.ProbeObservationRepository
		sources domain.NodeSourceRepository
		group   bool
		code    string
	}{
		{"global", &filterReadRepo{globalErr: errors.New(secret)}, nil, nil, false, "global_filter_unavailable"},
		{"group", &filterReadRepo{groupErr: errors.New(secret)}, nil, nil, false, "group_filters_unavailable"},
		{"probe", &filterReadRepo{global: probeFilter()}, &observationReadRepo{err: errors.New(secret)}, nil, false, "probe_observations_unavailable"},
		{"missing probe dependency", &filterReadRepo{global: probeFilter()}, nil, nil, false, "probe_observations_unavailable"},
		{"group probe", &filterReadRepo{groups: map[string]domain.NodeFilterSpec{"01944aa0-0000-7000-8000-000000000002": probeFilter()}}, &observationReadRepo{err: errors.New(secret)}, nil, true, "probe_observations_unavailable"},
		{"source", &filterReadRepo{global: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldSourceSubscriptions, Op: domain.FilterOpContains, Value: "01944aa0-0000-7000-8000-000000000006"}}}}, nil, &sourceReadRepo{err: errors.New(secret)}, false, "node_sources_unavailable"},
		{"missing source dependency", &filterReadRepo{global: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldSourceSubscriptions, Op: domain.FilterOpNotContains, Value: "01944aa0-0000-7000-8000-000000000006"}}}}, nil, nil, false, "node_sources_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, pubs := exportFilterFixture(t, tc.group, tc.filter, tc.obs, tc.sources)
			for _, target := range exportTargets {
				assertDeniedNewExport(t, svc, pubs, target, tc.code)
			}
			if _, err := svc.Preview(context.Background(), publication.PreviewQuery{Target: domain.TargetSingBox}); err != nil && strings.Contains(err.Error(), secret) {
				t.Fatalf("leaked repo error: %v", err)
			}
		})
	}
}

func TestExportFilterProbeFreshnessAndFrozenPublication(t *testing.T) {
	ctx := context.Background()
	filter := &filterReadRepo{}
	obs := &observationReadRepo{}
	svc, pubs := exportFilterFixture(t, false, filter, obs, nil)
	for _, target := range exportTargets {
		old, err := svc.Publish(ctx, publication.PublishCommand{Target: target})
		if err != nil {
			t.Fatalf("unfiltered %s publication: %v", target, err)
		}
		filter.global = probeFilter()
		obs.latest = map[string]map[domain.ProbeKind]domain.ProbeObservation{
			hkNodeID: {domain.ProbeKindBaseline: {NodeLogicalID: hkNodeID, Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ObservedAt: time.Now().UTC()}},
			usNodeID: {domain.ProbeKindBaseline: {NodeLogicalID: usNodeID, Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ObservedAt: time.Now().UTC().Add(-48 * time.Hour)}},
		}
		preview, err := svc.Preview(ctx, publication.PreviewQuery{Target: target})
		if err != nil {
			t.Fatalf("filtered %s preview: %v", target, err)
		}
		if preview.FilterCounts == nil || preview.FilterCounts.GlobalFilteredTotal != 1 || strings.Contains(string(preview.Content), "United States 01") || !strings.Contains(string(preview.Content), "Hong Kong 01") {
			t.Fatalf("%s failed to exclude stale observation: counts=%+v content=%s", target, preview.FilterCounts, preview.Content)
		}
		fresh, err := svc.Publish(ctx, publication.PublishCommand{Target: target})
		if err != nil || fresh.ContentDigest != preview.ContentDigest {
			t.Fatalf("%s filtered publish mismatch: %v", target, err)
		}
		delete(obs.latest, hkNodeID) // one stale, one untested: no trusted candidates
		assertDeniedNewExport(t, svc, pubs, target, "filtered_nodes_empty")
		artifact, err := svc.ResolveAndServe(ctx, old.Publication.ID, old.RawToken)
		if err != nil || artifact.ContentDigest != old.ContentDigest || !strings.Contains(string(artifact.Content), "United States 01") {
			t.Fatalf("%s historical artifact changed: %v", target, err)
		}
		filter.global = domain.NodeFilterSpec{}
	}
}

func TestExportFilterUnconfiguredDependenciesRemainOptional(t *testing.T) {
	filter := &filterReadRepo{}
	svc, _ := exportFilterFixture(t, false, filter, &observationReadRepo{err: errors.New("unneeded probe read")}, &sourceReadRepo{err: errors.New("unneeded source read")})
	for _, target := range exportTargets {
		preview, err := svc.Preview(context.Background(), publication.PreviewQuery{Target: target})
		if err != nil || preview.FilterCounts.GlobalFilteredTotal != preview.FilterCounts.AdmittedTotal {
			t.Fatalf("%s empty filter should pass through unchanged: %v, %+v", target, err, preview)
		}
	}
}

func TestExportFilterGroupOnlyZeroBlocksAllTargets(t *testing.T) {
	filter := &filterReadRepo{groups: map[string]domain.NodeFilterSpec{
		"01944aa0-0000-7000-8000-000000000002": probeFilter(),
	}}
	svc, pubs := exportFilterFixture(t, true, filter, &observationReadRepo{latest: map[string]map[domain.ProbeKind]domain.ProbeObservation{}}, nil)
	for _, target := range exportTargets {
		assertDeniedNewExport(t, svc, pubs, target, "filtered_nodes_empty")
	}
}

func TestExportFilterEmptyRoutedGroupAndGroupAND(t *testing.T) {
	filter := &filterReadRepo{global: probeFilter(), groups: map[string]domain.NodeFilterSpec{
		"01944aa0-0000-7000-8000-000000000002": {Conditions: []domain.FilterCondition{{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "United States"}}},
	}}
	obs := &observationReadRepo{latest: map[string]map[domain.ProbeKind]domain.ProbeObservation{
		hkNodeID: {domain.ProbeKindBaseline: {NodeLogicalID: hkNodeID, Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ObservedAt: time.Now().UTC()}},
	}}
	svc, pubs := exportFilterFixture(t, true, filter, obs, nil)
	for _, target := range exportTargets {
		assertDeniedNewExport(t, svc, pubs, target, "filtered_nodes_empty")
		if target == domain.TargetMihomo {
			pre, err := svc.Preflight(context.Background(), publication.PreflightCommand{Target: target})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range pre.Diagnostics {
				if d.Code == "empty_routed_group" {
					found = true
				}
			}
			if !found {
				t.Fatalf("lost empty_routed_group diagnostic: %+v", pre.Diagnostics)
			}
		}
	}
}
