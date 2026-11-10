package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/probe/mihomo"
	"clash-sub-parser/internal/repository/sqlite"
	"gopkg.in/yaml.v3"
)

// The fixture enters through reconciliation and exits through the real facts,
// preview/compiler and probe mapping consumers; no supplied snapshot shortcut.
func TestParserSemanticsRecoveredNodesReachFactsPreviewAndProbe(t *testing.T) {
	for _, name := range []string{"yaml", "base64_yaml", "url"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := openIntegrationDB(t)
			subs := sqlite.NewSubscriptionRepository(db)
			fetches := sqlite.NewSubscriptionFetchRepository(db)
			nodes := sqlite.NewNodeRepository(db)
			sources := sqlite.NewNodeSourceRepository(db)
			sub := &domain.Subscription{ID: "parse-recovery", Name: "Recovery", Enabled: true, SourceURLSecretRef: "https://fixture.invalid/recovery", Revision: domain.MustNewUUIDv7(), CreatedAt: domain.NowUTC(), UpdatedAt: domain.NowUTC()}
			if err := subs.Create(ctx, sub); err != nil {
				t.Fatal(err)
			}
			body := "proxies:\n - SECRET_SCALAR_SENTINEL\n" +
				" - {name: Plugin, type: ss, server: 192.0.2.41, port: 8388, cipher: aes-128-gcm, password: SYNTHETIC_SS_PASSWORD, plugin: obfs, plugin-opts: {mode: tls, host: cdn.example.org}}\n" +
				" - {name: VMess, type: vmess, server: 192.0.2.42, port: 443, uuid: 11111111-1111-4111-8111-111111111111, cipher: auto, tls: true}\n" +
				" - {name: SECRET_NAME_SENTINEL, type: SECRET_PROTOCOL_SENTINEL}\n"
			if name == "base64_yaml" {
				body = base64.StdEncoding.EncodeToString([]byte(body))
			} else if name == "url" {
				legacy := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:SYNTHETIC_SS_PASSWORD@192.0.2.41:8388"))
				vmess := base64.StdEncoding.EncodeToString([]byte(`{"add":"192.0.2.42","port":"443","id":"11111111-1111-4111-8111-111111111111","security":"tls"}`))
				body = "SECRET_SCALAR_SENTINEL\nss://" + legacy + "?plugin=obfs%3Bmode%3Dtls%3Bhost%3Dcdn.example.org#Plugin\nvmess://" + vmess + "\nSECRET_PROTOCOL_SENTINEL://SECRET_NAME_SENTINEL"
			}
			svc := inventory.NewService(db, subs, fetches, nodes, sources, fixtureFetcher{response: &fetch.Response{StatusCode: 200, Body: []byte(body), ContentDigest: "parse-recovery-" + name}})
			result, err := svc.ReconcileSubscription(ctx, sub.ID)
			if err != nil || result.Outcome != domain.FetchOutcomePartial || result.NodesParsed != 4 || result.NodesValid != 2 {
				t.Fatalf("reconcile=%+v error=%v", result, err)
			}
			if strings.Contains(result.RedactedError, "SENTINEL") || strings.Contains(result.RedactedError, "SYNTHETIC_SS_PASSWORD") {
				t.Fatal("secret in partial diagnostic")
			}
			original, err := svc.GetOriginalConfig(ctx, sub.ID)
			if err != nil || string(original.Body) != body {
				t.Fatalf("original bytes changed: %v", err)
			}
			revs := sqlite.NewRevisionRepository(db)
			rev := &domain.ConfigurationRevision{ID: "parse-recovery-revision", ContentDigest: "parse-recovery-revision", State: domain.RevisionStateActive, CreatedAt: domain.NowUTC()}
			if err := revs.Create(ctx, rev); err != nil {
				t.Fatal(err)
			}
			loader := sqlite.NewConsistentFactsLoader(db)
			facts, err := loader.LoadConsistentFacts(ctx, rev.ID, domain.TargetMihomo, domain.NowUTC())
			if err != nil || facts == nil || len(facts.Nodes) != 2 {
				t.Fatalf("facts count/error: %v", err)
			}
			var pluginID string
			for _, node := range facts.Nodes {
				if node.Protocol != domain.ProtocolSS {
					continue
				}
				pluginID = node.LogicalID
				if node.Credentials.Password != "SYNTHETIC_SS_PASSWORD" || node.Credentials.Transport["plugin"] != "obfs" {
					t.Fatal("facts lost credentials/plugin")
				}
				mapping, err := mihomo.NodeToMapping(node)
				if err != nil {
					t.Fatalf("probe mapping: %v", err)
				}
				opts, ok := mapping["plugin-opts"].(map[string]any)
				if mapping["plugin"] != "obfs" || !ok || opts["mode"] != "tls" || opts["host"] != "cdn.example.org" {
					t.Fatalf("probe lost YAML plugin options: %+v", mapping["plugin-opts"])
				}
			}
			if pluginID == "" {
				t.Fatal("plugin node missing from facts")
			}
			// Mihomo requires effective routing. With neither a route nor a
			// primary subscription this fixture would fail preflight before
			// reaching the compiler. Persist the real policy graph, as in the
			// export facts matrix; do not bypass preflight or supply a snapshot.
			policy := sqlite.NewPolicyRepository(db)
			now := domain.NowUTC()
			group := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: "Recovery Select", GroupType: domain.GroupTypeSelect, CreatedAt: now, UpdatedAt: now}
			members := make([]domain.MemberSource, 0, len(facts.Nodes))
			for _, node := range facts.Nodes {
				nodeID := node.LogicalID
				members = append(members, domain.MemberSource{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &nodeID})
			}
			if _, err := policy.SaveGroupWithSources(ctx, group, 0, members); err != nil {
				t.Fatalf("create recovery group: %v", err)
			}
			if err := policy.CreatePolicyRule(ctx, &domain.PolicyRule{ID: domain.MustNewUUIDv7(), RevisionID: rev.ID, TargetGroupID: group.ID, Expression: "MATCH", Enabled: true}); err != nil {
				t.Fatalf("create recovery route: %v", err)
			}
			assertPreviewOK := func(operation string, err error) {
				t.Helper()
				if err == nil {
					return
				}
				var preflight *publication.PreflightError
				if errors.As(err, &preflight) {
					t.Fatalf("%s: %v; preflight: %+v", operation, err, preflight.Result)
				}
				var capability *publication.StrictCapabilityError
				if errors.As(err, &capability) {
					t.Fatalf("%s: %v; capabilities: %+v", operation, err, capability.Diagnostics)
				}
				t.Fatalf("%s: %v", operation, err)
			}
			pub := publication.NewService(sqlite.NewPublicationRepository(db), nil, publication.WithConsistentFactsLoader(loader))
			preview, err := pub.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo, RevisionID: rev.ID})
			assertPreviewOK("Mihomo preview", err)
			if preview == nil || preview.Manifest.NodeCount != 2 {
				t.Fatalf("Mihomo preview missing recovered nodes: %+v", preview)
			}
			var rendered struct {
				Proxies []map[string]any `yaml:"proxies"`
				Groups  []struct {
					Name    string   `yaml:"name"`
					Proxies []string `yaml:"proxies"`
				} `yaml:"proxy-groups"`
				Rules []string `yaml:"rules"`
			}
			if err := yaml.Unmarshal(preview.Content, &rendered); err != nil {
				t.Fatal(err)
			}
			if len(rendered.Groups) != 1 || rendered.Groups[0].Name != group.Name || len(rendered.Groups[0].Proxies) != 2 || len(rendered.Rules) != 1 || rendered.Rules[0] != "MATCH,"+group.Name {
				t.Fatalf("persisted recovery graph not rendered: groups=%+v rules=%v", rendered.Groups, rendered.Rules)
			}
			var found bool
			for _, proxy := range rendered.Proxies {
				if proxy["type"] != "ss" {
					continue
				}
				found = true
				opts, ok := proxy["plugin-opts"].(map[string]any)
				if proxy["plugin"] != "obfs" || !ok || opts["mode"] != "tls" || opts["host"] != "cdn.example.org" {
					t.Fatalf("preview lost plugin options: %+v", proxy)
				}
			}
			if !found {
				t.Fatal("rendered SS missing")
			}
			compatible, err := pub.Preview(ctx, publication.PreviewQuery{Target: domain.TargetSingBox, RevisionID: rev.ID, CompatMode: "compatible"})
			assertPreviewOK("compatible sing-box preview", err)
			if compatible == nil || compatible.Manifest.NodeCount != 1 || compatible.Manifest.ExcludedCount != 1 || compatible.Manifest.Excluded[0].NodeID != pluginID {
				t.Fatalf("unsupported plugin must be excluded, not silently stripped: %v", err)
			}
			var singbox struct {
				Outbounds []struct {
					Type     string `json:"type"`
					Security string `json:"security"`
				} `json:"outbounds"`
			}
			if err := json.Unmarshal(compatible.Content, &singbox); err != nil {
				t.Fatal(err)
			}
			var vmessFound bool
			for _, out := range singbox.Outbounds {
				if out.Type == "vmess" {
					vmessFound = true
					if out.Security != "auto" {
						t.Fatalf("bad VMess cipher %q", out.Security)
					}
				}
			}
			if !vmessFound {
				t.Fatal("VMess absent from sing-box preview")
			}
		})
	}
}
