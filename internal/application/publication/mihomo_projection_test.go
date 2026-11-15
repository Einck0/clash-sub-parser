package publication_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
)

func incompatibleMihomoFacts() *domain.ConsistentFacts {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	goodID, badID := "node_0123456789abcdef", "node_abcdef0123456789"
	good := domain.Node{LogicalID: goodID, DisplayName: "Good", Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388, Active: true, Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "synthetic-secret"}, CreatedAt: now, UpdatedAt: now}
	bad := good
	bad.LogicalID, bad.DisplayName, bad.Server = badID, "Legacy", "192.0.2.2"
	bad.Credentials.Transport = map[string]string{"skip_cert_verify": "true"}
	groups := []domain.NodeGroup{
		{ID: "main", Name: "Main", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true},
		{ID: "fallback", Name: "Fallback", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true},
	}
	sources := map[string][]domain.MemberSource{
		"main":     {{ID: "good-source", GroupID: "main", Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &goodID}},
		"fallback": {{ID: "legacy-source", GroupID: "fallback", Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &badID}},
	}
	for i := range groups {
		groups[i].Sources = sources[groups[i].ID]
	}
	pass := true
	return &domain.ConsistentFacts{
		AsOf: now, Revision: &domain.ConfigurationRevision{ID: "rev-projection", State: domain.RevisionStateActive, CreatedAt: now},
		Nodes: []domain.Node{good, bad}, Groups: groups, GroupSources: sources,
		GlobalEmptyFallbackPass: &pass,
		PolicyRules: []domain.PolicyRule{
			{ID: "specific", RevisionID: "rev-projection", TargetGroupID: "fallback", Expression: "DOMAIN,fixture.invalid", Enabled: true},
			{ID: "terminal", RevisionID: "rev-projection", TargetGroupID: "main", Expression: "MATCH", Position: 1, Enabled: true},
		},
	}
}

func TestMihomoProjectionPreviewLiveSaveAndPublicExportAgree(t *testing.T) {
	ctx := context.Background()
	facts := incompatibleMihomoFacts()
	db := newCleanSQLiteDB(t)
	repo := sqlite.NewPublicationRepository(db)
	loader := &memoryFactsLoader{facts: facts}
	svc := publication.NewService(repo, nil, publication.WithConsistentFactsLoader(loader))
	live, projected, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Nodes) != 1 || len(live.ExcludedNodes) != 1 || live.ExcludedNodes[0].NodeID != facts.Nodes[1].LogicalID {
		t.Fatalf("wrong projection: %+v %+v", projected.Nodes, live.ExcludedNodes)
	}
	if !strings.Contains(string(live.Content), "empty-fallback: PASS") || !strings.Contains(string(live.Content), "- PASS") || strings.Contains(string(live.Content), "Legacy") {
		t.Fatalf("missing PASS or incompatible node leaked: %s", live.Content)
	}
	if len(facts.Nodes) != 2 || len(facts.Groups[1].Sources) != 1 {
		t.Fatal("projection mutated inventory facts")
	}
	preview, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo})
	if err != nil {
		t.Fatal(err)
	}
	if string(preview.Content) != string(live.Content) || preview.SnapshotDigest != live.SnapshotDigest || !reflect.DeepEqual(preview.Manifest.Excluded, live.ExcludedNodes) {
		t.Fatalf("preview/live divergence: %+v %+v", preview.Manifest, live)
	}
	if preview.Manifest.NodeCount != 1 || preview.Manifest.ExcludedCount != 1 || preview.Manifest.Included[0].NodeID != facts.Nodes[0].LogicalID {
		t.Fatalf("wrong manifest: %+v", preview.Manifest)
	}
	foundWarning := false
	for _, d := range preview.Diagnostics {
		if d.Code == "unsupported_target_capability" && d.Target == "mihomo" && d.Severity == resolver.DiagnosticSeverityWarning {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatal("exclusion not diagnosed")
	}
	draft, err := repo.GetByID(ctx, preview.SnapshotID)
	if err != nil || draft.Manifest == nil || !reflect.DeepEqual(*draft.Manifest, preview.Manifest) {
		t.Fatalf("manifest not frozen: %+v %v", draft, err)
	}
	published, err := svc.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo, SnapshotID: preview.SnapshotID})
	if err != nil {
		t.Fatal(err)
	}
	// A new service represents a restart. Immutable serving must not consult current facts.
	restarted := publication.NewService(sqlite.NewPublicationRepository(db), nil)
	served, err := restarted.ResolveAndServe(ctx, published.Publication.ID, published.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	if string(served.Content) != string(preview.Content) || !reflect.DeepEqual(served.ExcludedNodes, preview.Manifest.Excluded) {
		t.Fatalf("public artifact lost frozen projection: %+v", served)
	}
	detail, err := restarted.Get(ctx, preview.SnapshotID)
	if err != nil || detail.Publication.Manifest == nil || !reflect.DeepEqual(detail.Publication.Manifest.Excluded, preview.Manifest.Excluded) {
		t.Fatalf("read lost manifest: %+v %v", detail, err)
	}
	if len(compiler.ValidateTargetCapabilities(&resolver.ResolvedPolicySnapshot{Nodes: []resolver.ResolvedNode{{LogicalID: facts.Nodes[1].LogicalID, DisplayName: "Legacy", Protocol: domain.ProtocolSS, Server: "192.0.2.2", Port: 8388, Credentials: facts.Nodes[1].Credentials}}}, domain.TargetMihomo)) == 0 {
		t.Fatal("capability validator no longer diagnoses original node")
	}
}

func TestMihomoProjectionRejectsUnusableTopologyAndAllExcluded(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []string{"no-pass", "all-excluded"} {
		t.Run(scenario, func(t *testing.T) {
			facts := incompatibleMihomoFacts()
			if scenario == "no-pass" {
				disabled := false
				facts.GlobalEmptyFallbackPass = &disabled
			} else {
				facts.Nodes = facts.Nodes[1:]
			}
			svc := publication.NewService(newMockPublicationRepo(), nil, publication.WithConsistentFactsLoader(&memoryFactsLoader{facts: facts}))
			if _, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo); err == nil {
				t.Fatal("unusable live export succeeded")
			}
			if _, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo}); err == nil {
				t.Fatal("unusable preview succeeded")
			}
		})
	}
}

func TestMihomoSnapshotPreflightPreviewAndDirectPublishUseProjection(t *testing.T) {
	ctx := context.Background()
	facts := incompatibleMihomoFacts()
	input := resolver.ResolveInput{Nodes: facts.Nodes, Groups: facts.Groups, GroupSources: facts.GroupSources, PolicyRules: facts.PolicyRules, GlobalEmptyFallbackPass: facts.GlobalEmptyFallbackPass, RevisionID: facts.Revision.ID}
	snapshot, err := resolver.New().Resolve(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	svc := publication.NewService(newMockPublicationRepo(), nil)
	preflight, err := svc.Preflight(ctx, publication.PreflightCommand{Target: domain.TargetMihomo, Snapshot: snapshot})
	if err != nil || !preflight.Allowed || preflight.SnapshotDigest == snapshot.SnapshotDigest {
		t.Fatalf("preflight rejected projected snapshot: %+v %v", preflight, err)
	}
	preview, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo, Snapshot: snapshot})
	if err != nil || preview.Manifest.ExcludedCount != 1 {
		t.Fatalf("snapshot preview failed: %+v %v", preview, err)
	}
	published, err := svc.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo, Snapshot: snapshot})
	if err != nil || published.Publication.Manifest == nil || published.Publication.Manifest.ExcludedCount != 1 {
		t.Fatalf("direct publish bypassed projection: %+v %v", published, err)
	}
	if len(snapshot.Nodes) != 2 {
		t.Fatal("preview mutated original snapshot")
	}
	for _, group := range snapshot.Groups {
		if group.EffectiveEmptyPass {
			t.Fatal("preview mutated original group PASS state")
		}
	}
}
