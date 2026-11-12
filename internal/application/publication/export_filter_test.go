package publication_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

// Embed the existing ports so the test overrides only the read path under test.
// All other methods are deliberately unreachable in these service-level tests.
type filterReadRepo struct {
	domain.NodeFilterRepository
	global              domain.NodeFilterSpec
	emptyFallbackPass   *bool
	groups              map[string]domain.NodeFilterSpec
	globalErr, groupErr error
}

func (r *filterReadRepo) GetGlobalFilter(context.Context) (*domain.GlobalNodeFilter, error) {
	if r.globalErr != nil {
		return nil, r.globalErr
	}
	return &domain.GlobalNodeFilter{Spec: r.global, EmptyFallbackPass: r.emptyFallbackPass}, nil
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

func exportFilterFixture(t *testing.T, withGroup bool, filter *filterReadRepo, observations domain.ProbeObservationRepository, sources domain.NodeSourceRepository, configure ...func(*mockPolicyRepo)) (*publication.Service, *mockPublicationRepo) {
	t.Helper()
	ctx := context.Background()
	revision := newMockRevisionRepo()
	if err := revision.CreateActive(ctx, &domain.ConfigurationRevision{ID: "01944aa0-0000-7000-8000-000000000001", ContentDigest: "sha256:fixture"}); err != nil {
		t.Fatal(err)
	}
	policy := newMockPolicyRepo()
	if withGroup {
		groupID := "01944aa0-0000-7000-8000-000000000002"
		if err := policy.CreateGroup(ctx, &domain.NodeGroup{ID: groupID, Name: "PROXY", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true}); err != nil {
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
	// Group conditions live on sources, not the obsolete group filter repository.
	if filter.groupErr != nil {
		policy.listErr = filter.groupErr
	}
	for id, spec := range filter.groups {
		if g := policy.groups[id]; g != nil {
			if g.NodeFilter != nil {
				spec.Conditions = append(spec.Conditions, g.NodeFilter.Conditions...)
				g.NodeFilter = nil
			}
			for i := range g.Sources {
				g.Sources[i].FilterSpec = &spec
			}
		}
	}
	for _, configurePolicy := range configure {
		configurePolicy(policy)
	}
	pubs := newMockPublicationRepo()
	nodes := setupSampleNodeSource()
	nodes.mu.Lock()
	for oldID, newID := range map[string]string{"node-hk-01": hkNodeID, "node-us-01": usNodeID} {
		n := *nodes.nodes[oldID]
		delete(nodes.nodes, oldID)
		n.LogicalID = newID
		n.ConnectionRevision = 1
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
		{"group", &filterReadRepo{groupErr: errors.New("group_sources_unavailable")}, nil, nil, false, "group_sources_unavailable"},
		{"probe", &filterReadRepo{global: probeFilter()}, &observationReadRepo{err: errors.New(secret)}, nil, false, "probe_observations_unavailable"},
		{"missing probe dependency", &filterReadRepo{global: probeFilter()}, nil, nil, false, "probe_observations_unavailable"},
		{"group probe", &filterReadRepo{groups: map[string]domain.NodeFilterSpec{"01944aa0-0000-7000-8000-000000000002": probeFilter()}}, &observationReadRepo{err: errors.New(secret)}, nil, true, "probe_observations_unavailable"},
		{"source", &filterReadRepo{global: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldSourceSubscriptions, Op: domain.FilterOpContains, Value: "01944aa0-0000-7000-8000-000000000006"}}}}, nil, &sourceReadRepo{err: errors.New(secret)}, false, "node_sources_unavailable"},
		{"missing source dependency", &filterReadRepo{global: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldSourceSubscriptions, Op: domain.FilterOpNotContains, Value: "01944aa0-0000-7000-8000-000000000006"}}}}, nil, nil, false, "node_sources_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, pubs := exportFilterFixture(t, tc.group, tc.filter, tc.obs, tc.sources)
			for _, target := range exportTargets {
				if target != domain.TargetMihomo && (tc.name == "group" || tc.name == "group probe") {
					assertPureNodeExportAllowed(t, svc, target)
					continue
				}
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
	rev := int64(1)
	for _, target := range exportTargets {
		old, err := svc.Publish(ctx, publication.PublishCommand{Target: target})
		if err != nil {
			t.Fatalf("unfiltered %s publication: %v", target, err)
		}
		filter.global = probeFilter()
		obs.latest = map[string]map[domain.ProbeKind]domain.ProbeObservation{
			hkNodeID: {domain.ProbeKindBaseline: {NodeLogicalID: hkNodeID, Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ObservedAt: domain.NowUTC().Add(-time.Second), ConnectionRevision: &rev}},
			usNodeID: {domain.ProbeKindBaseline: {NodeLogicalID: usNodeID, Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ObservedAt: domain.NowUTC().Add(-48 * time.Hour), ConnectionRevision: &rev}},
		}
		preview, err := svc.Preview(ctx, publication.PreviewQuery{Target: target})
		if err != nil {
			t.Fatalf("filtered %s preview: %v", target, err)
		}
		if preview.FilterCounts == nil || preview.FilterCounts.GlobalFilteredTotal != 2 || !strings.Contains(string(preview.Content), "United States 01") || !strings.Contains(string(preview.Content), "Hong Kong 01") {
			t.Fatalf("%s should accept current and old observations from the current connection revision: counts=%+v content=%s", target, preview.FilterCounts, preview.Content)
		}
		fresh, err := svc.Publish(ctx, publication.PublishCommand{Target: target})
		if err != nil || fresh.ContentDigest != preview.ContentDigest {
			t.Fatalf("%s filtered publish mismatch: %v", target, err)
		}
		delete(obs.latest, hkNodeID)
		delete(obs.latest, usNodeID) // missing evidence is not synthesized as success
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

func assertPureNodeExportAllowed(t *testing.T, svc *publication.Service, target domain.CompilerTarget) {
	t.Helper()
	ctx := context.Background()
	pre, err := svc.Preflight(ctx, publication.PreflightCommand{Target: target})
	if err != nil || !pre.Allowed {
		t.Fatalf("pure-node graph independence: %+v %v", pre, err)
	}
	if _, err := svc.Preview(ctx, publication.PreviewQuery{Target: target}); err != nil {
		t.Fatal(err)
	}
	pub, err := svc.Publish(ctx, publication.PublishCommand{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveAndServe(ctx, pub.Publication.ID, pub.RawToken); err != nil {
		t.Fatal(err)
	}
}

func TestExportFilterGroupOnlyZeroBlocksMihomoNotPureNodes(t *testing.T) {
	strict := false
	filter := &filterReadRepo{emptyFallbackPass: &strict, groups: map[string]domain.NodeFilterSpec{
		"01944aa0-0000-7000-8000-000000000002": probeFilter(),
	}}
	svc, pubs := exportFilterFixture(t, true, filter, &observationReadRepo{latest: map[string]map[domain.ProbeKind]domain.ProbeObservation{}}, nil)
	for _, target := range exportTargets {
		if target == domain.TargetMihomo {
			assertDeniedNewExport(t, svc, pubs, target, "filtered_nodes_empty")
		} else {
			assertPureNodeExportAllowed(t, svc, target)
		}
	}
}

func TestExportFilterEmptyRoutedGroupAndGroupAND(t *testing.T) {
	strict := false
	filter := &filterReadRepo{global: probeFilter(), emptyFallbackPass: &strict, groups: map[string]domain.NodeFilterSpec{
		"01944aa0-0000-7000-8000-000000000002": {Conditions: []domain.FilterCondition{{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "United States"}}},
	}}
	rev := int64(1)
	obs := &observationReadRepo{latest: map[string]map[domain.ProbeKind]domain.ProbeObservation{
		hkNodeID: {domain.ProbeKindBaseline: {NodeLogicalID: hkNodeID, Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ObservedAt: domain.NowUTC().Add(-time.Second), ConnectionRevision: &rev}},
	}}
	svc, pubs := exportFilterFixture(t, true, filter, obs, nil)
	for _, target := range exportTargets {
		if target != domain.TargetMihomo {
			assertPureNodeExportAllowed(t, svc, target)
			continue
		}
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

func TestExportFilterGlobalExhaustWithRoutedEmptyPassAllowsMihomoWithPass(t *testing.T) {
	ctx := context.Background()

	globalFilter := domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{{
			Field: domain.FilterFieldDisplayName,
			Op:    domain.FilterOpContains,
			Value: "__guaranteed_absent_display_name__",
		}},
	}

	t.Run("default empty pass allows mihomo with PASS and denies pure nodes", func(t *testing.T) {
		filter := &filterReadRepo{global: globalFilter}
		svc, pubs := exportFilterFixture(t, true, filter, nil, nil)

		pre, err := svc.Preflight(ctx, publication.PreflightCommand{Target: domain.TargetMihomo})
		if err != nil || pre == nil || !pre.Allowed {
			t.Fatalf("expected mihomo preflight allowed with routed empty PASS: %+v, %v", pre, err)
		}

		prev, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo})
		if err != nil {
			t.Fatalf("expected mihomo preview allowed: %v", err)
		}
		if prev.FilterCounts == nil || prev.FilterCounts.AdmittedTotal <= 0 || prev.FilterCounts.GlobalFilteredTotal != 0 {
			t.Fatalf("expected admitted nodes and zero globally filtered nodes: %+v", prev.FilterCounts)
		}
		if !strings.Contains(string(prev.Content), "empty-fallback: PASS") {
			t.Fatalf("expected empty-fallback: PASS in mihomo content: %s", prev.Content)
		}
		var content struct {
			Proxies     []map[string]any `yaml:"proxies"`
			ProxyGroups []struct {
				Proxies []string `yaml:"proxies"`
			} `yaml:"proxy-groups"`
			Rules []string `yaml:"rules"`
		}
		if err := yaml.Unmarshal(prev.Content, &content); err != nil {
			t.Fatalf("decode mihomo preview YAML: %v", err)
		}
		if len(content.Proxies) != 0 {
			t.Fatalf("expected no top-level proxy servers when all nodes are filtered out, got %+v", content.Proxies)
		}
		passInGroup := false
		for _, group := range content.ProxyGroups {
			if containsString(group.Proxies, "PASS") {
				passInGroup = true
				break
			}
		}
		if !passInGroup {
			t.Fatalf("expected PASS in a proxy group's proxies list: %+v", content.ProxyGroups)
		}
		if !containsString(content.Rules, "MATCH,PROXY") {
			t.Fatalf("expected rule evaluation to continue through the PROXY group: %+v", content.Rules)
		}

		pub, err := svc.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo})
		if err != nil || pub == nil || pub.ContentDigest == "" {
			t.Fatalf("expected mihomo publish allowed: %v, %+v", err, pub)
		}

		for _, target := range exportTargets {
			if target == domain.TargetMihomo {
				continue
			}
			assertDeniedNewExport(t, svc, pubs, target, "filtered_nodes_empty")
		}
	})

	t.Run("strict off rejects mihomo on global exhaust", func(t *testing.T) {
		strict := false
		filter := &filterReadRepo{global: globalFilter, emptyFallbackPass: &strict}
		svc, pubs := exportFilterFixture(t, true, filter, nil, nil)
		assertDeniedNewExport(t, svc, pubs, domain.TargetMihomo, "filtered_nodes_empty")
	})

	t.Run("independent errors stay errors even with empty pass enabled", func(t *testing.T) {
		filter := &filterReadRepo{
			global:    globalFilter,
			globalErr: errors.New("global_filter_unavailable"),
		}
		svc, pubs := exportFilterFixture(t, true, filter, nil, nil)
		assertDeniedNewExport(t, svc, pubs, domain.TargetMihomo, "global_filter_unavailable")
	})
}

func TestExportFilterMixedRoutedGroupsApplyEmptyPassIndependently(t *testing.T) {
	ctx := context.Background()
	filter := &filterReadRepo{groups: map[string]domain.NodeFilterSpec{
		"01944aa0-0000-7000-8000-000000000002": {Conditions: []domain.FilterCondition{{
			Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "Tokyo",
		}}},
	}}
	svc, _ := exportFilterFixture(t, true, filter, nil, nil, func(policy *mockPolicyRepo) {
		groupID := "01944aa0-0000-7000-8000-000000000006"
		for i := range policy.policyRules["01944aa0-0000-7000-8000-000000000001"] {
			if policy.policyRules["01944aa0-0000-7000-8000-000000000001"][i].Expression == "MATCH" {
				policy.policyRules["01944aa0-0000-7000-8000-000000000001"][i].Position = 1
			}
		}
		if err := policy.CreateGroup(ctx, &domain.NodeGroup{ID: groupID, Name: "USABLE", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true}); err != nil {
			t.Fatal(err)
		}
		if err := policy.SetEdgesForGroup(ctx, groupID, []domain.GroupEdge{{ID: "01944aa0-0000-7000-8000-000000000007", ParentGroupID: groupID, NodeLogicalID: ptr(usNodeID)}}); err != nil {
			t.Fatal(err)
		}
		if err := policy.CreatePolicyRule(ctx, &domain.PolicyRule{ID: "01944aa0-0000-7000-8000-000000000008", RevisionID: "01944aa0-0000-7000-8000-000000000001", TargetGroupID: groupID, Expression: "DOMAIN-SUFFIX,example.org", Position: 0}); err != nil {
			t.Fatal(err)
		}
	})

	pre, err := svc.Preflight(ctx, publication.PreflightCommand{Target: domain.TargetMihomo})
	if err != nil || pre == nil || !pre.Allowed {
		t.Fatalf("expected Mihomo preflight allowed: %+v, %v", pre, err)
	}
	preview, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo})
	if err != nil {
		t.Fatalf("expected preview to succeed: %v", err)
	}
	var content struct {
		ProxyGroups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(preview.Content, &content); err != nil {
		t.Fatalf("decode preview YAML: %v", err)
	}
	groups := map[string][]string{}
	for _, group := range content.ProxyGroups {
		groups[group.Name] = group.Proxies
	}
	if len(groups["PROXY"]) != 1 || groups["PROXY"][0] != "PASS" {
		t.Fatalf("expected PROXY to contain only PASS, got %+v", groups["PROXY"])
	}
	if !containsString(groups["USABLE"], "United States 01") || containsString(groups["USABLE"], "PASS") {
		t.Fatalf("expected USABLE to contain its retained node and no PASS, got %+v", groups["USABLE"])
	}
	if preview.FilterCounts == nil {
		t.Fatal("expected filter counts")
	}
	counts := preview.FilterCounts.GroupCounts
	original, originalOK := counts["01944aa0-0000-7000-8000-000000000002"]
	second, secondOK := counts["01944aa0-0000-7000-8000-000000000006"]
	if !originalOK || !secondOK || original.Candidate == 0 || original.Kept != 0 || second.Kept == 0 || preview.FilterCounts.AdmittedTotal == 0 {
		t.Fatalf("expected isolated group filtering and global nodes retained; counts=%+v", preview.FilterCounts)
	}
	if _, err := svc.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo}); err != nil {
		t.Fatalf("expected publish to succeed: %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestExportFilterPostResolutionErrorNotMaskedByEmptyPass(t *testing.T) {
	ctx := context.Background()
	svc, pubs, _ := setupService()
	snapshot := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "sha256:test-post-resolution-error",
		CompilerVersion: "1.0.0",
		Groups: []resolver.ResolvedGroup{{
			ID: "empty-pass-group", Name: "EMPTY", GroupType: domain.GroupTypeSelect,
			EmptyFallbackPass: true, EffectiveEmptyPass: true, AllNodeLogicalIDs: []string{},
		}},
		Rules:       []resolver.ResolvedRule{{ID: "empty-rule", TargetGroupID: "empty-pass-group", TargetGroupName: "EMPTY", Expression: "MATCH", IsTerminal: true}},
		Diagnostics: []resolver.Diagnostic{{Severity: resolver.DiagnosticSeverityError, Code: "risk_blocked", Message: "risk policy blocked publication"}},
	}
	pubs.mu.RLock()
	before := len(pubs.publications)
	pubs.mu.RUnlock()
	pre, err := svc.Preflight(ctx, publication.PreflightCommand{Target: domain.TargetMihomo, Snapshot: snapshot})
	if err != nil || pre == nil || pre.Allowed || !hasDiagnosticCode(pre.Diagnostics, "risk_blocked") {
		t.Fatalf("expected risk_blocked to deny preflight: %+v, %v", pre, err)
	}
	for _, run := range []struct {
		name string
		call func() error
	}{
		{"preview", func() error {
			_, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo, Snapshot: snapshot})
			return err
		}},
		{"publish", func() error {
			_, err := svc.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo, Snapshot: snapshot})
			return err
		}},
	} {
		t.Run(run.name, func(t *testing.T) {
			err := run.call()
			var preflightErr *publication.PreflightError
			if !errors.As(err, &preflightErr) || !hasDiagnosticCode(preflightErr.Result.Diagnostics, "risk_blocked") {
				t.Fatalf("expected PreflightError carrying risk_blocked, got %v", err)
			}
		})
	}
	pubs.mu.RLock()
	after := len(pubs.publications)
	pubs.mu.RUnlock()
	if after != before {
		t.Fatalf("expected denied publication not to persist; before=%d after=%d", before, after)
	}
}

func hasDiagnosticCode(diagnostics []publication.PreflightDiagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestExportFilterGroupExhaustWithRoutedEmptyPassAllowsMihomoWithPass(t *testing.T) {
	ctx := context.Background()

	tokyoGroupFilter := map[string]domain.NodeFilterSpec{
		"01944aa0-0000-7000-8000-000000000002": {
			Conditions: []domain.FilterCondition{{
				Field: domain.FilterFieldDisplayName,
				Op:    domain.FilterOpContains,
				Value: "Tokyo",
			}},
		},
	}

	t.Run("default empty pass allows mihomo with PASS in group", func(t *testing.T) {
		filter := &filterReadRepo{groups: tokyoGroupFilter}
		svc, _ := exportFilterFixture(t, true, filter, nil, nil)

		pre, err := svc.Preflight(ctx, publication.PreflightCommand{Target: domain.TargetMihomo})
		if err != nil || pre == nil || !pre.Allowed {
			t.Fatalf("expected mihomo preflight allowed with routed empty PASS: %+v, %v", pre, err)
		}

		prev, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo})
		if err != nil {
			t.Fatalf("expected mihomo preview allowed: %v", err)
		}
		if prev.FilterCounts == nil || prev.FilterCounts.AdmittedTotal <= 0 || prev.FilterCounts.GlobalFilteredTotal != prev.FilterCounts.AdmittedTotal {
			t.Fatalf("expected group filter to exhaust sourced nodes while preserving globally admitted nodes: %+v", prev.FilterCounts)
		}
		groupCount, ok := prev.FilterCounts.GroupCounts["01944aa0-0000-7000-8000-000000000002"]
		if !ok || groupCount.Candidate != prev.FilterCounts.AdmittedTotal || groupCount.Kept != 0 || groupCount.Excluded != groupCount.Candidate {
			t.Fatalf("expected group filter to exclude its complete routed source set: %+v, counts=%+v", groupCount, prev.FilterCounts)
		}
		if !strings.Contains(string(prev.Content), "empty-fallback: PASS") {
			t.Fatalf("expected empty-fallback: PASS in mihomo content: %s", prev.Content)
		}
		if !strings.Contains(string(prev.Content), "- PASS") {
			t.Fatalf("expected proxy group proxies list to contain PASS: %s", prev.Content)
		}
		if !strings.Contains(string(prev.Content), "Hong Kong 01") || !strings.Contains(string(prev.Content), "United States 01") {
			t.Fatalf("expected normal nodes preserved in proxies list: %s", prev.Content)
		}

		pub, err := svc.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo})
		if err != nil || pub == nil || pub.ContentDigest == "" {
			t.Fatalf("expected mihomo publish allowed: %v, %+v", err, pub)
		}

		for _, target := range exportTargets {
			if target != domain.TargetMihomo {
				assertPureNodeExportAllowed(t, svc, target)
			}
		}
	})

	t.Run("strict off rejects mihomo on group exhaust", func(t *testing.T) {
		strict := false
		filter := &filterReadRepo{groups: tokyoGroupFilter, emptyFallbackPass: &strict}
		svc, pubs := exportFilterFixture(t, true, filter, nil, nil)
		assertDeniedNewExport(t, svc, pubs, domain.TargetMihomo, "filtered_nodes_empty")
	})

	t.Run("independent errors stay errors even with empty pass enabled", func(t *testing.T) {
		filter := &filterReadRepo{
			groups:   tokyoGroupFilter,
			groupErr: errors.New("group_sources_unavailable"),
		}
		svc, pubs := exportFilterFixture(t, true, filter, nil, nil)
		assertDeniedNewExport(t, svc, pubs, domain.TargetMihomo, "group_sources_unavailable")
	})
}
