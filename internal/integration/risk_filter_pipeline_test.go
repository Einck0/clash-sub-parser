package integration_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
	"gopkg.in/yaml.v3"
)

// A risk-only filter must load probe_observations for draft preview and export;
// it must not accidentally introduce baseline-health admission as a side effect.
func TestRiskOnlyFilterPreviewSaveAndExport(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	now := domain.NowUTC().Truncate(time.Second)
	nodes := sqlite.NewNodeRepository(db)
	subs := sqlite.NewSubscriptionRepository(db)
	sub := &domain.Subscription{ID: domain.MustNewUUIDv7(), Name: "risk fixture", SourceURLSecretRef: "https://fixture.invalid/risk", Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := subs.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}
	// Native reconcile supplies valid connection credentials and current-source
	// provenance. Three nodes are sufficient: low, high, and missing risk data.
	body := `proxies:
  - {name: LowRisk, type: ss, server: 192.0.2.11, port: 8388, cipher: aes-256-gcm, password: fixture-low}
  - {name: HighRisk, type: ss, server: 192.0.2.12, port: 8388, cipher: aes-256-gcm, password: fixture-high}
  - {name: UnknownRisk, type: ss, server: 192.0.2.13, port: 8388, cipher: aes-256-gcm, password: fixture-unknown}
`
	inv := inventory.NewService(db, subs, sqlite.NewSubscriptionFetchRepository(db), nodes, sqlite.NewNodeSourceRepository(db), fixtureFetcher{response: &fetch.Response{StatusCode: 200, ContentType: "text/yaml", ContentDigest: "sha256:risk-fixture", Body: []byte(body)}})
	if result, err := inv.ReconcileSubscription(ctx, sub.ID); err != nil || result.NodesValid != 3 {
		t.Fatalf("reconcile: %+v %v", result, err)
	}
	all, err := nodes.ListAll(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(all) != 3 {
		t.Fatalf("nodes: %+v %v", all, err)
	}
	byName := map[string]domain.Node{}
	for _, node := range all {
		byName[node.DisplayName] = node
	}
	low := byName["LowRisk"]
	if low.LogicalID == "" {
		t.Fatal("low-risk fixture missing")
	}
	revisions := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{ID: "risk-filter-revision", ContentDigest: "risk-fixture", State: domain.RevisionStateActive, CreatedAt: now}
	if err := revisions.Create(ctx, rev); err != nil {
		t.Fatal(err)
	}
	if err := revisions.SetActive(ctx, rev.ID); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewProbeRunRepository(db).Create(ctx, &domain.ProbeRun{ID: "risk-filter-run", IdempotencyKey: "risk-filter-run", State: domain.ProbeRunStateSucceeded, DeadlineAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	obs := sqlite.NewProbeObservationRepository(db)
	for _, pair := range []struct{ name, score string }{{"LowRisk", "24% low"}, {"HighRisk", "65% high"}} {
		node := byName[pair.name]
		if err := obs.Create(ctx, &domain.ProbeObservation{ID: "risk-" + pair.name, ProbeRunID: "risk-filter-run", NodeLogicalID: node.LogicalID, Kind: domain.ProbeKindIPRisk, Verdict: domain.VerdictAvailable, ConnectionRevision: &node.ConnectionRevision, ObservedAt: now, RiskScore: pair.score}); err != nil {
			t.Fatal(err)
		}
	}
	// Low risk with an explicitly failing baseline must still match a risk-only
	// filter. UnknownRisk has no baseline or risk evidence at all.
	if err := obs.Create(ctx, &domain.ProbeObservation{ID: "risk-low-baseline", ProbeRunID: "risk-filter-run", NodeLogicalID: low.LogicalID, Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictError, ConnectionRevision: &low.ConnectionRevision, ObservedAt: now}); err != nil {
		t.Fatal(err)
	}
	policies := sqlite.NewPolicyRepository(db)
	filters := sqlite.NewNodeFilterRepository(db)
	policySvc := policy.NewService(policies, revisions, nodes, nil, filters)
	pub := publication.NewService(sqlite.NewPublicationRepository(db), nil,
		publication.WithPolicyRepository(policies), publication.WithRevisionRepository(revisions), publication.WithNodeRepository(nodes),
		publication.WithNodeFilterRepository(filters), publication.WithNodeSourceRepository(sqlite.NewNodeSourceRepository(db)),
		publication.WithProbeObservationRepository(obs), publication.WithConsistentFactsLoader(sqlite.NewConsistentFactsLoader(db)))
	_ = transporthttp.NewRouter(transporthttp.RouterConfig{PolicyService: policySvc, PublicationService: pub})
	loader := sqlite.NewConsistentFactsLoader(db)
	for _, condition := range []domain.FilterCondition{
		{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: "low"},
		{Field: domain.FilterFieldIPRiskScore, Op: domain.FilterOpLTE, Value: "24"},
	} {
		t.Run(string(condition.Field), func(t *testing.T) {
			spec := domain.NodeFilterSpec{Conditions: []domain.FilterCondition{condition}}
			sources := []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindFilter, FilterSpec: &spec}}
			assertPreview := func(groupID *string) {
				t.Helper()
				got, err := policySvc.PreviewGroup(ctx, policy.PreviewGroupCommand{GroupID: groupID, Name: stringPtr("Risk Only"), Sources: sources})
				if err != nil {
					t.Fatal(err)
				}
				if got.MemberCount != 1 || len(got.Members) != 1 || got.Members[0].NodeLogicalID == nil || *got.Members[0].NodeLogicalID != low.LogicalID {
					t.Fatalf("risk-only preview must retain low risk despite baseline error: %+v", got)
				}
			}
			assertPreview(nil)
			group, err := policySvc.CreateGroup(ctx, policy.CreateGroupCommand{Name: "Risk Only", GroupType: domain.GroupTypeSelect, Sources: sources})
			if err != nil {
				t.Fatal(err)
			}
			persisted, err := policies.GetGroupByID(ctx, group.ID)
			if err != nil || len(persisted.Sources) != 1 || persisted.Sources[0].FilterSpec == nil || len(persisted.Sources[0].FilterSpec.Conditions) != 1 || persisted.Sources[0].FilterSpec.Conditions[0].Field != condition.Field {
				t.Fatalf("save lost risk-only condition: %+v %v", persisted, err)
			}
			sources = persisted.Sources
			assertPreview(&group.ID)
			ruleID := domain.MustNewUUIDv7()
			if err := policies.CreatePolicyRule(ctx, &domain.PolicyRule{ID: ruleID, RevisionID: rev.ID, TargetGroupID: group.ID, Expression: "MATCH", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			facts, err := loader.LoadConsistentFacts(ctx, rev.ID, domain.TargetMihomo, now)
			if err != nil {
				t.Fatal(err)
			}
			if got := facts.LatestObservations[low.LogicalID][domain.ProbeKindIPRisk]; got.RiskScore != "24% low" || got.Verdict != domain.VerdictAvailable {
				t.Fatalf("risk-only persisted source did not load probe evidence: %+v", got)
			}
			artifact, snapshot, err := pub.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Groups) != 1 || len(snapshot.Groups[0].AllNodeLogicalIDs) != 1 || snapshot.Groups[0].AllNodeLogicalIDs[0] != low.LogicalID {
				t.Fatalf("export group differs from preview: %+v", snapshot.Groups)
			}
			// Group filters constrain group membership, not the global proxy
			// ledger. Verify emitted group references instead of incorrectly
			// requiring unrelated global proxies to disappear.
			var emitted struct {
				Groups []struct {
					Name    string   `yaml:"name"`
					Proxies []string `yaml:"proxies"`
				} `yaml:"proxy-groups"`
			}
			if err := yaml.Unmarshal(artifact.Content, &emitted); err != nil {
				t.Fatal(err)
			}
			if len(emitted.Groups) != 1 || emitted.Groups[0].Name != "Risk Only" || len(emitted.Groups[0].Proxies) != 1 || emitted.Groups[0].Proxies[0] != low.DisplayName {
				t.Fatalf("emitted group differs from risk-only preview: %+v", emitted.Groups)
			}
			// The same risk condition alone at the global boundary exercises the
			// non-Mihomo facts path without relying on a group dependency.
			if err := filters.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{Spec: spec}); err != nil {
				t.Fatal(err)
			}
			globalFacts, err := loader.LoadConsistentFacts(ctx, rev.ID, domain.TargetSingBox, now)
			if err != nil {
				t.Fatal(err)
			}
			if globalFacts.LatestObservations[low.LogicalID][domain.ProbeKindIPRisk].RiskScore != "24% low" {
				t.Fatal("global risk-only filter did not load probe_observations")
			}
			globalArtifact, globalSnapshot, err := pub.BuildCurrentArtifact(ctx, globalFacts, domain.TargetSingBox)
			if err != nil {
				t.Fatal(err)
			}
			if len(globalSnapshot.Nodes) != 1 || globalSnapshot.Nodes[0].LogicalID != low.LogicalID || !strings.Contains(string(globalArtifact.Content), "192.0.2.11") {
				t.Fatalf("global export differs from source filter: %+v", globalSnapshot.Nodes)
			}
			if err := filters.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{}); err != nil {
				t.Fatal(err)
			}
			if err := policies.DeletePolicyRule(ctx, ruleID); err != nil {
				t.Fatal(err)
			}
			if err := policySvc.DeleteGroup(ctx, policy.DeleteGroupCommand{ID: group.ID}); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Missing evidence is selectable by unknown without an implicit health gate.
	unknown := domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: "unknown"}}}
	got, err := policySvc.PreviewGroup(ctx, policy.PreviewGroupCommand{Name: stringPtr("Unknown risk"), Sources: []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindFilter, FilterSpec: &unknown}}})
	if err != nil || got.MemberCount != 1 || len(got.Members) != 1 || got.Members[0].NodeLogicalID == nil || *got.Members[0].NodeLogicalID != byName["UnknownRisk"].LogicalID {
		t.Fatalf("unknown risk preview: %+v %v", got, err)
	}
}

func TestRiskOnlyStandalonePreviewAndValidationRequireEvidenceProvider(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	now := domain.NowUTC()
	revisions := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{ID: "risk-standalone", ContentDigest: "risk-standalone", State: domain.RevisionStateActive, CreatedAt: now}
	if err := revisions.Create(ctx, rev); err != nil {
		t.Fatal(err)
	}
	if err := revisions.SetActive(ctx, rev.ID); err != nil {
		t.Fatal(err)
	}
	policies := sqlite.NewPolicyRepository(db)
	filters := sqlite.NewNodeFilterRepository(db)
	svc := policy.NewService(policies, revisions, sqlite.NewNodeRepository(db), nil, filters)
	check := func(err error, code string) {
		t.Helper()
		var de *domain.DomainError
		if !errors.As(err, &de) || de.Code != code {
			t.Fatalf("want %s, got %v", code, err)
		}
	}
	for _, condition := range []domain.FilterCondition{{Field: domain.FilterFieldIPRiskScore, Op: domain.FilterOpLTE, Value: "24"}, {Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: "low"}} {
		spec := domain.NodeFilterSpec{Conditions: []domain.FilterCondition{condition}}
		_, err := svc.PreviewGroup(ctx, policy.PreviewGroupCommand{Name: stringPtr("Standalone risk"), Sources: []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindFilter, FilterSpec: &spec}}})
		check(err, "preview_incomplete")
		if err := filters.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{Spec: spec}); err != nil {
			t.Fatal(err)
		}
		_, err = svc.PreviewGroup(ctx, policy.PreviewGroupCommand{Name: stringPtr("Standalone global")})
		check(err, "preview_incomplete")
		_, err = svc.ValidateGraph(ctx)
		check(err, "validation_incomplete")
		if err := filters.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{}); err != nil {
			t.Fatal(err)
		}
		group := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: fmt.Sprintf("Standalone %s", condition.Field), GroupType: domain.GroupTypeSelect, CreatedAt: now, UpdatedAt: now}
		if err := policies.CreateGroup(ctx, group); err != nil {
			t.Fatal(err)
		}
		if err := filters.SetGroupFilter(ctx, &domain.GroupNodeFilter{GroupID: group.ID, Spec: spec}); err != nil {
			t.Fatal(err)
		}
		_, err = svc.PreviewGroup(ctx, policy.PreviewGroupCommand{GroupID: &group.ID})
		check(err, "preview_incomplete")
		_, err = svc.ValidateGraph(ctx)
		check(err, "validation_incomplete")
		if err := svc.DeleteGroup(ctx, policy.DeleteGroupCommand{ID: group.ID}); err != nil {
			t.Fatal(err)
		}
	}
}
