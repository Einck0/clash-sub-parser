package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
	"gopkg.in/yaml.v3"
)

// Every fixture enters the real subscription parser, persists credentials, loads
// consistent facts and reaches the authenticated HTTP preview consumer. No
// supplied snapshot, production database, real fetch or listening port is used.
func TestParserPluginBoundaryFactsAndHTTPPreview(t *testing.T) {
	const secret = "PRIVATE_PLUGIN_SENTINEL"
	cases := []struct {
		name, plugin, opts string
		valid              bool
	}{
		{"valid_nested_headers", "v2ray", `{"mode":"websocket","headers":{"Host":"cdn.example.org","X-Test":"preserved"},"HEADERS":{"X-Test":"preserved","Host":"cdn.example.org"}}`, true},
		{"obfs_map_alias", "obfs", `{"mode":{"x":"` + secret + `"},"obfs":{"x":"` + secret + `"}}`, false},
		{"obfs_slice_alias", "obfs", `{"mode":"tls","host":["` + secret + `"],"obfs-host":["` + secret + `"]}`, false},
		{"normalized_map_alias", "v2ray-plugin", `{"mode":"websocket","skip_cert_verify":{},"skip-cert-verify":{}}`, false},
		{"unknown_plugin", secret, "", false},
		{"unknown_key", "v2ray-plugin", `{"mode":"websocket","` + secret + `":true}`, false},
		{"unknown_token", "v2ray-plugin", "mode=websocket;" + secret, false},
		{"invalid_mode", "obfs", "mode=" + secret, false},
		{"invalid_bool", "v2ray-plugin", "mode=websocket;tls=" + secret, false},
		{"invalid_integer", "shadow-tls", "host=cdn.example.org;password=pw;version=" + secret, false},
		{"empty_key", "obfs", "mode=tls;=" + secret, false},
		{"malformed_json", "obfs", `{"` + secret + `":}`, false},
		{"malformed_headers", "v2ray-plugin", `{"mode":"websocket","headers":"{\"` + secret + `\":}"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := openIntegrationDB(t)
			subs := sqlite.NewSubscriptionRepository(db)
			fetches := sqlite.NewSubscriptionFetchRepository(db)
			nodes := sqlite.NewNodeRepository(db)
			sources := sqlite.NewNodeSourceRepository(db)
			now := domain.NowUTC()
			sub := &domain.Subscription{ID: "plugin-boundary-sub", Name: "Plugin boundary", Enabled: true, SourceURLSecretRef: "https://fixture.invalid/plugin", Revision: domain.MustNewUUIDv7(), CreatedAt: now, UpdatedAt: now}
			if err := subs.Create(ctx, sub); err != nil {
				t.Fatal(err)
			}
			// JSON is also valid YAML, and quotes the malformed plugin options as
			// a scalar so they reach the compiler rather than breaking the fixture.
			body, err := json.Marshal(map[string]any{"proxies": []map[string]any{
				{"name": "Plugin", "type": "ss", "server": "192.0.2.61", "port": 8388, "cipher": "aes-128-gcm", "password": "SYNTHETIC_PASSWORD_SENTINEL", "plugin": tc.plugin, "plugin-opts": tc.opts},
				{"name": "Plain", "type": "ss", "server": "192.0.2.62", "port": 8388, "cipher": "aes-128-gcm", "password": "SYNTHETIC_PASSWORD_SENTINEL"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			inv := inventory.NewService(db, subs, fetches, nodes, sources, fixtureFetcher{response: &fetch.Response{StatusCode: 200, Body: body, ContentDigest: "plugin-boundary-" + tc.name}})
			result, err := inv.ReconcileSubscription(ctx, sub.ID)
			if err != nil || result.Outcome != domain.FetchOutcomeSuccess || result.NodesValid != 2 {
				t.Fatalf("parser must preserve both nodes for target capability evaluation: %+v, %v", result, err)
			}
			revs := sqlite.NewRevisionRepository(db)
			rev := &domain.ConfigurationRevision{ID: "plugin-boundary-revision", ContentDigest: "plugin-boundary-revision", State: domain.RevisionStateActive, CreatedAt: now}
			if err := revs.Create(ctx, rev); err != nil {
				t.Fatal(err)
			}
			loader := sqlite.NewConsistentFactsLoader(db)
			facts, err := loader.LoadConsistentFacts(ctx, rev.ID, domain.TargetMihomo, domain.NowUTC())
			if err != nil || facts == nil || len(facts.Nodes) != 2 {
				t.Fatalf("facts missing nodes: %v", err)
			}
			var pluginID string
			members := make([]domain.MemberSource, 0, 2)
			for _, node := range facts.Nodes {
				nodeID := node.LogicalID
				members = append(members, domain.MemberSource{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &nodeID})
				if node.Server == "192.0.2.61" {
					pluginID = nodeID
					if node.Credentials.Transport["plugin"] != tc.plugin || node.Credentials.Transport["plugin-opts"] != tc.opts {
						t.Fatal("facts silently lost plugin configuration")
					}
				}
			}
			if pluginID == "" {
				t.Fatal("plugin node absent from facts")
			}
			policy := sqlite.NewPolicyRepository(db)
			group := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: "Plugin Select", GroupType: domain.GroupTypeSelect, CreatedAt: now, UpdatedAt: now}
			if _, err := policy.SaveGroupWithSources(ctx, group, 0, members); err != nil {
				t.Fatal(err)
			}
			if err := policy.CreatePolicyRule(ctx, &domain.PolicyRule{ID: domain.MustNewUUIDv7(), RevisionID: rev.ID, TargetGroupID: group.ID, Expression: "MATCH", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			pub := publication.NewService(sqlite.NewPublicationRepository(db), nil, publication.WithConsistentFactsLoader(loader))
			const token = "plugin-boundary-admin"
			router := transporthttp.NewRouter(transporthttp.RouterConfig{AdminToken: token, PublicationService: pub})
			preview := func(target, mode string) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/publications/preview", strings.NewReader(fmt.Sprintf(`{"target":%q,"revision_id":%q,"compat_mode":%q}`, target, rev.ID, mode)))
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			strict := preview("mihomo", "strict")
			if tc.valid {
				if strict.Code != http.StatusOK {
					t.Fatalf("valid plugin rejected: %d %s", strict.Code, strict.Body.String())
				}
				res, err := pub.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo, RevisionID: rev.ID, CompatMode: "compatible"})
				if err != nil || res.Manifest.NodeCount != 2 {
					t.Fatalf("valid compatible preview: %v", err)
				}
				var rendered struct {
					Proxies []map[string]any `yaml:"proxies"`
				}
				if err := yaml.Unmarshal(res.Content, &rendered); err != nil {
					t.Fatal(err)
				}
				var found bool
				for _, proxy := range rendered.Proxies {
					if proxy["server"] != "192.0.2.61" {
						continue
					}
					found = true
					opts, ok := proxy["plugin-opts"].(map[string]any)
					if !ok || proxy["plugin"] != "v2ray-plugin" {
						t.Fatal("plugin stripped from valid preview")
					}
					headers, ok := opts["headers"].(map[string]any)
					if !ok || headers["Host"] != "cdn.example.org" || headers["X-Test"] != "preserved" {
						t.Fatalf("nested headers changed: %+v", opts)
					}
				}
				if !found {
					t.Fatal("valid plugin node omitted")
				}
			} else {
				if strict.Code != http.StatusUnprocessableEntity || !strings.Contains(strict.Body.String(), "unsupported_target_capability") {
					t.Fatalf("invalid plugin did not fail closed: %d %s", strict.Code, strict.Body.String())
				}
				if !strings.Contains(strict.Body.String(), pluginID) {
					t.Fatal("strict failure missing affected node")
				}
				if strings.Contains(strict.Body.String(), secret) || strings.Contains(strict.Body.String(), "SYNTHETIC_PASSWORD_SENTINEL") {
					t.Fatal("HTTP capability error leaked private plugin input")
				}
				if rec := preview("mihomo", "compatible"); rec.Code == http.StatusOK || rec.Code == http.StatusInternalServerError {
					t.Fatalf("invalid plugin must neither panic nor become plain SS: %d %s", rec.Code, rec.Body.String())
				}
			}
			// Targets without SS plugin support must reject strictly or exclude
			// the whole node compatibly, never emit it as plain Shadowsocks.
			if rec := preview("singbox", "strict"); rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("sing-box strict accepted plugin: %d", rec.Code)
			}
			compatible, err := pub.Preview(ctx, publication.PreviewQuery{Target: domain.TargetSingBox, RevisionID: rev.ID, CompatMode: "compatible"})
			if err != nil || compatible.Manifest.NodeCount != 1 || compatible.Manifest.ExcludedCount != 1 || compatible.Manifest.Excluded[0].NodeID != pluginID {
				t.Fatalf("compatible target must exclude plugin node explicitly: %v", err)
			}
			if strings.Contains(string(compatible.Content), "192.0.2.61") || !strings.Contains(string(compatible.Content), "192.0.2.62") {
				t.Fatal("compatible target silently stripped plugin instead of excluding node")
			}
		})
	}
}
