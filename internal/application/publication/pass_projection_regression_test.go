package publication_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func TestProjectedPassSemanticsAcrossPublicationEntrypoints(t *testing.T) {
	for _, scenario := range []string{"global-pass", "local-pass", "builtin-pass", "no-pass", "all-unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			facts := incompatibleMihomoFacts()
			if scenario != "global-pass" {
				off := false
				facts.GlobalEmptyFallbackPass = &off
			}
			switch scenario {
			case "local-pass":
				facts.GlobalEmptyFallbackPass = nil
				facts.Groups[1].EmptyFallbackPass = true
			case "builtin-pass":
				pass := "PASS"
				groupID := facts.Groups[1].ID
				facts.GroupSources[groupID] = append(facts.GroupSources[groupID], domain.MemberSource{ID: "pass-source", GroupID: groupID, Kind: domain.SourceKindTypedBuiltin, Operation: domain.OperationAdd, BuiltinName: &pass, Position: 1})
				facts.Groups[1].Sources = facts.GroupSources[groupID]
			case "all-unavailable":
				on := true
				facts.GlobalEmptyFallbackPass = &on
				facts.Nodes[0].Credentials.Transport = map[string]string{"skip_cert_verify": "true"}
			}
			// Match the live pipeline's default DNS and resolution metadata before comparing artifacts.
			dnsSettings := domain.DefaultDNSSettings()
			facts.DNSSettings = &dnsSettings
			dnsConfig, err := domain.ParseDNSSettings(dnsSettings.Enabled, dnsSettings.RawYAML)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := resolver.New().Resolve(ctx, resolver.ResolveInput{
				Nodes: facts.Nodes, Groups: facts.Groups, GroupSources: facts.GroupSources,
				PolicyRules: facts.PolicyRules, GlobalEmptyFallbackPass: facts.GlobalEmptyFallbackPass, RevisionID: facts.Revision.ID,
				InventoryWatermark: "v1", CompilerVersion: "1.0.0", AsOf: facts.AsOf,
				DNS: resolver.DNSConfig{Enabled: dnsSettings.Enabled, Config: dnsConfig},
			})
			if err != nil {
				t.Fatal(err)
			}
			svc := publication.NewService(newMockPublicationRepo(), nil)
			allowed := scenario != "no-pass" && scenario != "all-unavailable"
			preflight, err := svc.Preflight(ctx, publication.PreflightCommand{Target: domain.TargetMihomo, Snapshot: snapshot})
			if err != nil || preflight.Allowed != allowed {
				t.Fatalf("preflight=%+v err=%v", preflight, err)
			}
			preview, previewErr := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo, Snapshot: snapshot})
			published, publishErr := svc.Publish(ctx, publication.PublishCommand{Target: domain.TargetMihomo, Snapshot: snapshot})
			live, _, liveErr := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
			if !allowed {
				if previewErr == nil || publishErr == nil || liveErr == nil {
					t.Fatalf("unusable topology accepted: preview=%v publish=%v live=%v", previewErr, publishErr, liveErr)
				}
				return
			}
			if previewErr != nil || publishErr != nil || liveErr != nil {
				t.Fatalf("PASS rejected: preview=%v publish=%v live=%v", previewErr, publishErr, liveErr)
			}
			served, err := svc.ResolveAndServe(ctx, published.Publication.ID, published.RawToken)
			if err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string][]byte{"preview": preview.Content, "live": live.Content, "saved": served.Content} {
				if !strings.Contains(string(content), "- PASS") || strings.Contains(string(content), "Legacy") {
					t.Fatalf("%s lost PASS or included rejected node: %s", name, content)
				}
			}
			if string(preview.Content) != string(live.Content) {
				t.Fatalf("preview/live content differs:\npreview:\n%s\nlive:\n%s", preview.Content, live.Content)
			}
			if string(preview.Content) != string(served.Content) {
				t.Fatalf("preview/saved content differs:\npreview:\n%s\nsaved:\n%s", preview.Content, served.Content)
			}
			if preflight.SnapshotDigest != preview.SnapshotDigest || preview.SnapshotDigest != live.SnapshotDigest || preview.SnapshotDigest != served.SnapshotDigest {
				t.Fatalf("projection digests disagree: preflight=%s preview=%s live=%s saved=%s", preflight.SnapshotDigest, preview.SnapshotDigest, live.SnapshotDigest, served.SnapshotDigest)
			}
			if preview.ContentDigest != live.ContentDigest || preview.ContentDigest != served.ContentDigest {
				t.Fatalf("content digests disagree: preview=%s live=%s saved=%s", preview.ContentDigest, live.ContentDigest, served.ContentDigest)
			}
			if preview.Manifest.NodeCount != 1 || preview.Manifest.ExcludedCount != 1 || len(preview.Manifest.Included) != 1 || len(preview.Manifest.Excluded) != 1 || preview.Manifest.Included[0].NodeID != facts.Nodes[0].LogicalID || preview.Manifest.Excluded[0].NodeID != facts.Nodes[1].LogicalID || preview.Manifest.Excluded[0].Code != "unsupported_target_capability" {
				t.Fatalf("unexpected target projection manifest: %+v", preview.Manifest)
			}
			if live.Manifest == nil || served.Manifest == nil || !reflect.DeepEqual(preview.Manifest.Included, live.Manifest.Included) || !reflect.DeepEqual(preview.Manifest.Included, served.Manifest.Included) || !reflect.DeepEqual(preview.Manifest.Excluded, live.ExcludedNodes) || !reflect.DeepEqual(preview.Manifest.Excluded, served.ExcludedNodes) {
				t.Fatalf("projection manifests disagree: preview=%+v live=%+v saved=%+v", preview.Manifest, live.Manifest, served.Manifest)
			}
		})
	}
}
