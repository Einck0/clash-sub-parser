package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/transport/http"
)

// This exercises the router's production publication-provider wiring through
// PolicyService.PreviewGroup and the real resolver, using only an in-memory DB.
func TestPolicyPreview_UnsavedPlatformFilterUsesDraftEvidenceDependencies(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	now := domain.NowUTC().UTC().Truncate(time.Second)

	subRepo := sqlite.NewSubscriptionRepository(db)
	sub := &domain.Subscription{
		ID: "018bcfe5-7b13-7cc4-8f11-111111111111", Name: "preview fixture",
		SourceURLSecretRef: "https://fixture.invalid/preview", Enabled: true,
		Revision: domain.MustNewUUIDv7(), CreatedAt: now, UpdatedAt: now,
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("create synthetic subscription: %v", err)
	}

	// The precise acceptance boundary is seven enabled candidates with valid,
	// connection-current evidence: exactly one has antigravity=abnormal.
	const nodesTableInsert = `INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
VALUES (?, 'ss', ?, '198.51.100.1', 8388, '{}', 1, ?, ?, 1)`
	obsRepo := sqlite.NewProbeObservationRepository(db)
	nodeIDs := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		id := fmt.Sprintf("preview-node-%02d", i)
		name := fmt.Sprintf("Synthetic %02d", i)
		if _, err := db.ExecContext(ctx, nodesTableInsert, id, name, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
			t.Fatalf("insert synthetic node %s: %v", id, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id) VALUES (?, ?)`, id, sub.ID); err != nil {
			t.Fatalf("associate synthetic node %s: %v", id, err)
		}
		nodeIDs = append(nodeIDs, id)

		runID := fmt.Sprintf("preview-run-%02d", i)
		if _, err := db.ExecContext(ctx, `INSERT INTO probe_runs (id, idempotency_key, actor_scope, config_revision, state, deadline_at, created_at, updated_at) VALUES (?, ?, 'fixture', 'preview-revision', 'completed', ?, ?, ?)`, runID, runID, now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
			t.Fatalf("insert synthetic probe run %s: %v", runID, err)
		}
		verdict := domain.VerdictSkipped
		if i == 3 {
			verdict = domain.VerdictAbnormal
		}
		obs := &domain.ProbeObservation{
			ID: fmt.Sprintf("preview-observation-%02d", i), ProbeRunID: runID,
			NodeLogicalID: id, Kind: domain.ProbeKindAI, Verdict: verdict,
			EvidenceDigest: fmt.Sprintf("fixture-%02d", i), ObservedAt: now,
			ConnectionRevision: int64Ptr(1),
			Platforms: map[string]domain.PlatformCapability{
				"antigravity": {Verdict: verdict, ObservedAt: timePtr(now)},
			},
		}
		if err := obsRepo.Create(ctx, obs); err != nil {
			t.Fatalf("insert synthetic observation for %s: %v", id, err)
		}
	}

	// An inactive node has matching evidence-independent attributes but must never
	// enter the enabled active candidate inventory.
	if _, err := db.ExecContext(ctx, `INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision) VALUES ('preview-inactive', 'ss', 'Synthetic inactive', '198.51.100.2', 8388, '{}', 0, ?, ?, 1)`, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert inactive candidate: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id, subscription_id) VALUES (?, ?)`, "preview-inactive", sub.ID); err != nil {
		t.Fatalf("associate inactive candidate: %v", err)
	}

	revisions := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{ID: "preview-revision", ContentDigest: "preview-fixture", State: domain.RevisionStateActive, CreatedAt: now}
	if err := revisions.Create(ctx, rev); err != nil {
		t.Fatalf("create active revision: %v", err)
	}
	if err := revisions.SetActive(ctx, rev.ID); err != nil {
		t.Fatalf("activate revision: %v", err)
	}

	policyRepo := sqlite.NewPolicyRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	filterRepo := sqlite.NewNodeFilterRepository(db)
	policySvc := policy.NewService(policyRepo, revisions, nodeRepo, sqlite.NewAuditRepository(db), filterRepo)
	publicationSvc := publication.NewService(
		sqlite.NewPublicationRepository(db), sqlite.NewAuditRepository(db),
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revisions),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(filterRepo),
		publication.WithNodeSourceRepository(sqlite.NewNodeSourceRepository(db)),
		publication.WithProbeObservationRepository(obsRepo),
	)
	// NewRouter installs the same publication input provider used by HTTP preview.
	_ = http.NewRouter(http.RouterConfig{PolicyService: policySvc, PublicationService: publicationSvc})
	assertGroupCount := func(t *testing.T, want int) {
		t.Helper()
		groups, err := policyRepo.ListGroups(ctx)
		if err != nil {
			t.Fatalf("list groups after preview: %v", err)
		}
		if len(groups) != want {
			t.Fatalf("preview changed persisted group count: got %d, want %d; groups=%+v", len(groups), want, groups)
		}
	}

	kind := domain.ProbeKindAI
	condition := domain.FilterCondition{
		Field: domain.FilterFieldPlatformVerdict, Op: domain.FilterOpEquals,
		Value: string(domain.VerdictAbnormal), ProbeKind: &kind, Platform: "antigravity",
	}
	source := domain.MemberSource{
		Operation: domain.OperationAdd, Kind: domain.SourceKindFilter, Position: 0,
		FilterSpec: &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{condition}},
	}
	preview := func(t *testing.T, op domain.FilterOp, value string) *policy.GroupPreviewResponse {
		t.Helper()
		condition.Op = op
		condition.Value = value
		cmdSource := source
		filter := *source.FilterSpec
		filter.Conditions = []domain.FilterCondition{condition}
		cmdSource.FilterSpec = &filter
		returnPreview, err := policySvc.PreviewGroup(ctx, policy.PreviewGroupCommand{
			Name:    stringPtr("Unsaved evidence preview"),
			Sources: []domain.MemberSource{cmdSource},
		})
		if err != nil {
			t.Fatalf("preview unsaved platform filter (%s %s): %v", op, value, err)
		}
		return returnPreview
	}
	assertIDs := func(t *testing.T, got *policy.GroupPreviewResponse, want []string) {
		t.Helper()
		if got.CandidateCount != 7 || got.MemberCount != len(want) || got.ExcludedCount != 7-len(want) {
			t.Fatalf("counts: candidates=%d members=%d excluded=%d; want 7/%d/%d", got.CandidateCount, got.MemberCount, got.ExcludedCount, len(want), 7-len(want))
		}
		actual := make(map[string]bool, len(got.Members))
		for _, member := range got.Members {
			if member.NodeLogicalID == nil {
				t.Fatalf("expected node member, got %+v", member)
			}
			actual[*member.NodeLogicalID] = true
		}
		for _, id := range want {
			if !actual[id] {
				t.Errorf("missing expected member %s; got %v", id, actual)
			}
			delete(actual, id)
		}
		if len(actual) != 0 {
			t.Errorf("unexpected members: %v", actual)
		}
	}

	t.Run("equals abnormal keeps the unique matching candidate", func(t *testing.T) {
		got := preview(t, domain.FilterOpEquals, string(domain.VerdictAbnormal))
		assertIDs(t, got, []string{nodeIDs[3]})
		assertGroupCount(t, 0)
	})
	t.Run("not equals abnormal keeps the six other observed candidates", func(t *testing.T) {
		got := preview(t, domain.FilterOpNotEquals, string(domain.VerdictAbnormal))
		want := append([]string(nil), nodeIDs[:3]...)
		want = append(want, nodeIDs[4:]...)
		assertIDs(t, got, want)
		assertGroupCount(t, 0)
	})
	t.Run("equals skipped keeps six candidates", func(t *testing.T) {
		got := preview(t, domain.FilterOpEquals, string(domain.VerdictSkipped))
		want := append([]string(nil), nodeIDs[:3]...)
		want = append(want, nodeIDs[4:]...)
		assertIDs(t, got, want)
		assertGroupCount(t, 0)
	})
	t.Run("not equals skipped keeps only abnormal candidate", func(t *testing.T) {
		got := preview(t, domain.FilterOpNotEquals, string(domain.VerdictSkipped))
		assertIDs(t, got, []string{nodeIDs[3]})
		assertGroupCount(t, 0)
	})

	previewSources := func(t *testing.T, groupID *string, sources []domain.MemberSource) *policy.GroupPreviewResponse {
		t.Helper()
		returnPreview, err := policySvc.PreviewGroup(ctx, policy.PreviewGroupCommand{
			GroupID: groupID,
			Name:    stringPtr("Draft evidence preview"),
			Sources: sources,
		})
		if err != nil {
			t.Fatalf("preview sources: %v", err)
		}
		return returnPreview
	}
	platformSource := func(op domain.FilterOp, value string) domain.MemberSource {
		kind := domain.ProbeKindAI
		c := domain.FilterCondition{
			Field: domain.FilterFieldPlatformVerdict, ProbeKind: &kind, Platform: "antigravity",
			Op: op, Value: value,
		}
		return domain.MemberSource{
			Operation: domain.OperationAdd, Kind: domain.SourceKindFilter,
			FilterSpec: &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{c}},
		}
	}
	displaySource := domain.MemberSource{
		Operation: domain.OperationAdd, Kind: domain.SourceKindFilter,
		FilterSpec: &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{
			Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "Synthetic",
		}}},
	}

	t.Run("saved and unsaved identical conditions resolve to the same member", func(t *testing.T) {
		sources := []domain.MemberSource{platformSource(domain.FilterOpEquals, string(domain.VerdictAbnormal))}
		created, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
			Name: "Saved evidence comparison", GroupType: domain.GroupTypeSelect, Sources: sources,
		})
		if err != nil {
			t.Fatalf("save comparison group: %v", err)
		}
		groupID := created.ID
		saved := previewSources(t, &groupID, sources)
		assertIDs(t, saved, []string{nodeIDs[3]})
		if err := policySvc.DeleteGroup(ctx, policy.DeleteGroupCommand{ID: groupID}); err != nil {
			t.Fatalf("remove saved control group before unsaved comparison: %v", err)
		}
		unsaved := previewSources(t, nil, sources)
		assertIDs(t, unsaved, []string{nodeIDs[3]})
		assertGroupCount(t, 0)
	})

	t.Run("editing in and out of an evidence dependency updates the preview", func(t *testing.T) {
		created, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
			Name: "Editable dependency", GroupType: domain.GroupTypeSelect, Sources: []domain.MemberSource{displaySource},
		})
		if err != nil {
			t.Fatalf("save editable group: %v", err)
		}
		groupID := created.ID
		evidenceSources := []domain.MemberSource{platformSource(domain.FilterOpEquals, string(domain.VerdictAbnormal))}
		assertIDs(t, previewSources(t, &groupID, evidenceSources), []string{nodeIDs[3]})
		if _, err := policySvc.UpdateGroup(ctx, policy.UpdateGroupCommand{ID: groupID, Sources: &evidenceSources}); err != nil {
			t.Fatalf("save evidence-dependent sources: %v", err)
		}
		assertIDs(t, previewSources(t, &groupID, []domain.MemberSource{displaySource}), nodeIDs)
		assertGroupCount(t, 1)
		if err := policySvc.DeleteGroup(ctx, policy.DeleteGroupCommand{ID: groupID}); err != nil {
			t.Fatalf("remove editable group: %v", err)
		}
	})

	t.Run("draft group reference loads evidence dependencies from the referenced group", func(t *testing.T) {
		child, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{
			Name: "Evidence child", GroupType: domain.GroupTypeSelect,
			Sources: []domain.MemberSource{platformSource(domain.FilterOpEquals, string(domain.VerdictAbnormal))},
		})
		if err != nil {
			t.Fatalf("save evidence child: %v", err)
		}
		childID := child.ID
		draftRef := domain.MemberSource{
			Operation: domain.OperationAdd, Kind: domain.SourceKindGroupRef, TargetGroupID: &childID,
		}
		got := previewSources(t, nil, []domain.MemberSource{draftRef})
		if got.CandidateCount != 1 || got.MemberCount != 1 || got.NodeCount != 1 {
			t.Fatalf("draft subgroup resolution: candidates=%d members=%d nodes=%d, want 1/1/1", got.CandidateCount, got.MemberCount, got.NodeCount)
		}
		if got.Members[0].GroupID == nil || *got.Members[0].GroupID != childID {
			t.Fatalf("expected reference to evidence child %s, got %+v", childID, got.Members)
		}
		assertGroupCount(t, 1)
		if err := policySvc.DeleteGroup(ctx, policy.DeleteGroupCommand{ID: childID}); err != nil {
			t.Fatalf("remove evidence child: %v", err)
		}
	})

	t.Run("source membership and global conditions both constrain effective candidates", func(t *testing.T) {
		// All fixture nodes belong to this subscription. The source condition
		// proves source facts are available; the global display condition
		// independently narrows the candidate inventory before the draft filter.
		subscriptionID := sub.ID
		global := &domain.GlobalNodeFilter{Spec: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{
			Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "Synthetic 03",
		}}}}
		if err := filterRepo.SetGlobalFilter(ctx, global); err != nil {
			t.Fatalf("set global candidate filter: %v", err)
		}
		sourceCondition := domain.FilterCondition{Field: domain.FilterFieldSourceSubscriptions, Op: domain.FilterOpContains, Value: subscriptionID}
		platformCondition := domain.FilterCondition{
			Field: domain.FilterFieldPlatformVerdict, Op: domain.FilterOpEquals,
			Value: string(domain.VerdictAbnormal), ProbeKind: &kind, Platform: "antigravity",
		}
		source := domain.MemberSource{Operation: domain.OperationAdd, Kind: domain.SourceKindFilter,
			FilterSpec: &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{sourceCondition, platformCondition}}}
		got := previewSources(t, nil, []domain.MemberSource{source})
		if got.CandidateCount != 1 || got.MemberCount != 1 || got.ExcludedCount != 0 || len(got.Members) != 1 || got.Members[0].NodeLogicalID == nil || *got.Members[0].NodeLogicalID != nodeIDs[3] {
			t.Fatalf("global AND draft filter should keep only node %s, got %+v", nodeIDs[3], got)
		}
		global.Spec.Conditions[0].Value = "Synthetic 00"
		if err := filterRepo.SetGlobalFilter(ctx, global); err != nil {
			t.Fatalf("change global candidate filter: %v", err)
		}
		got = previewSources(t, nil, []domain.MemberSource{source})
		if got.CandidateCount != 1 || got.MemberCount != 0 || got.ExcludedCount != 1 {
			t.Fatalf("global and draft filters must both match: %+v", got)
		}
		assertGroupCount(t, 0)
	})
}

func int64Ptr(v int64) *int64        { return &v }
func timePtr(v time.Time) *time.Time { return &v }
func stringPtr(v string) *string     { return &v }
