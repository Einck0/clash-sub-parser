package publication_test

import (
	"context"
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
				facts.GroupSources["fallback"] = append(facts.GroupSources["fallback"], domain.MemberSource{ID: "pass-source", GroupID: "fallback", Kind: domain.SourceKindBuiltin, Operation: domain.OperationAdd, BuiltinName: &pass, Position: 1})
				facts.Groups[1].Sources = facts.GroupSources["fallback"]
			case "all-unavailable":
				on := true
				facts.GlobalEmptyFallbackPass = &on
				facts.Nodes[0].Credentials.Transport = map[string]string{"skip_cert_verify": "true"}
			}
			snapshot, err := resolver.New().Resolve(ctx, resolver.ResolveInput{
				Nodes: facts.Nodes, Groups: facts.Groups, GroupSources: facts.GroupSources,
				PolicyRules: facts.PolicyRules, GlobalEmptyFallbackPass: facts.GlobalEmptyFallbackPass, RevisionID: facts.Revision.ID,
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
			if string(preview.Content) != string(live.Content) || string(preview.Content) != string(served.Content) {
				t.Fatal("export entrypoints disagree")
			}
		})
	}
}
