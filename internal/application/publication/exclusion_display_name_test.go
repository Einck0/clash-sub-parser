package publication_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestPublicationExclusionDisplayNameIsSafeSnapshotMetadata(t *testing.T) {
	ctx := context.Background()
	svc := publication.NewService(newMockPublicationRepo(), nil)
	baseline, _, err := svc.BuildCurrentArtifact(ctx, incompatibleMihomoFacts(), domain.TargetMihomo)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, label, want string
	}{
		{"readable", "香港 · Legacy", "香港 · Legacy"},
		{"controls", "香港\x00\n\u202eLegacy", "香港Legacy"},
		{"bounded_unicode", strings.Repeat("港", 200), strings.Repeat("港", 127) + "…"},
		{"http_userinfo", "https://fixture-user:fixture-pass@example.invalid/feed?token=fixture-token", ""},
		{"proxy_uri", "ss://fixture-base64-credential@example.invalid:443", ""},
		{"known_password", "Region synthetic-secret", ""},
		{"bare_assignment", "Region password=fixture-secret", "Region password=***"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := incompatibleMihomoFacts()
			facts.Nodes[1].DisplayName = tc.label
			artifact, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.Manifest == nil || len(artifact.Manifest.Excluded) != 1 {
				t.Fatalf("missing excluded identity: %+v", artifact.Manifest)
			}
			item := artifact.Manifest.Excluded[0]
			if item.NodeID != facts.Nodes[1].LogicalID || item.DisplayName != tc.want || item.Code != "unsupported_target_capability" || item.Reason == "" {
				t.Fatalf("unexpected safe exclusion: %+v", item)
			}
			if !utf8.ValidString(item.DisplayName) || utf8.RuneCountInString(item.DisplayName) > 128 {
				t.Fatalf("invalid or unbounded display name: %q", item.DisplayName)
			}
			encoded, err := json.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"fixture-user", "fixture-pass", "fixture-token", "fixture-base64-credential", "synthetic-secret", "fixture-secret"} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatalf("diagnostic leaks %q: %s", forbidden, encoded)
				}
			}
			if tc.want == "" && strings.Contains(string(encoded), "display_name") {
				t.Fatalf("unsafe display name must be omitted: %s", encoded)
			}
			if string(artifact.Content) != string(baseline.Content) || artifact.ContentDigest != baseline.ContentDigest {
				t.Fatal("excluded label changed compiled bytes or content digest")
			}
			if facts.Nodes[1].DisplayName != tc.label {
				t.Fatal("diagnostic sanitization changed inventory facts")
			}
		})
	}
}

// Exercise the parser-supported plugin representations through actual target
// exclusion and immutable preview persistence, not only a label helper.
func TestPublicationExclusionDisplayNameOmitsNestedPluginSecrets(t *testing.T) {
	ctx := context.Background()
	// These are actual option representations, not a test-side normalizer.
	// Password scalar acceptance is separately checked against emitted Mihomo
	// YAML in compiler/plugin_secret_normalization_test.go.
	forms := []struct{ name, template, secret string }{
		{"semicolon", "<key>=PLUGIN_SECRET;host=plugin.example.invalid", "PLUGIN_SECRET"},
		{"single_quoted", "<key>='PLUGIN_SECRET';host=plugin.example.invalid", "PLUGIN_SECRET"},
		{"double_quoted", `<key>="PLUGIN_SECRET";host=plugin.example.invalid`, "PLUGIN_SECRET"},
		{"ampersand", "<key>='PLUGIN_SECRET'&host=plugin.example.invalid", "PLUGIN_SECRET"},
		{"json_string", `{"<key>":"PLUGIN_SECRET","host":"plugin.example.invalid"}`, "PLUGIN_SECRET"},
		{"json_escaped", `{"<key>":"PLUGIN_\u0053ECRET","host":"plugin.example.invalid"}`, "PLUGIN_SECRET"},
		{"json_large_number", `{"<key>":9007199254740993123456789,"host":"plugin.example.invalid"}`, "9007199254740993123456789"},
		{"json_exponent", `{"<key>":1.234567890123456789e+42,"host":"plugin.example.invalid"}`, "1.234567890123456789e+42"},
		{"json_true", `{"<key>":true,"host":"plugin.example.invalid"}`, "true"},
		{"json_false", `{"<key>":false,"host":"plugin.example.invalid"}`, "false"},
		{"bare_flag", "<key>;host=plugin.example.invalid", "true"},
	}
	for _, optionKey := range []string{"plugin-opts", "plugin_opts"} {
		for _, form := range forms {
			for _, secretKey := range []string{"password", "token"} {
				t.Run(optionKey+"/"+form.name+"/"+secretKey, func(t *testing.T) {
					secret := form.secret
					options := strings.ReplaceAll(form.template, "<key>", secretKey)
					parsed, credentials, err := parser.ExtractProxyMap(map[string]any{
						"name": "Plugin node", "type": "ss", "server": "192.0.2.2", "port": 8388,
						"cipher": "aes-128-gcm", "password": "outer-ss-credential",
						"plugin": "shadow-tls", optionKey: options,
					})
					if err != nil || credentials.Transport[optionKey] != options {
						t.Fatalf("fixture must retain supported plugin options: %v", err)
					}
					facts := incompatibleMihomoFacts()
					badID := facts.Nodes[1].LogicalID
					facts.Nodes[1] = parsed.Node
					facts.Nodes[1].LogicalID = badID
					facts.Nodes[1].Credentials = credentials
					facts.Nodes[1].Active = true
					// Keep a usable group on every target; the plugin node, not an
					// unrelated empty-group/PASS condition, must trigger exclusion.
					facts.Groups = facts.Groups[:1]
					groupID := facts.Groups[0].ID
					facts.GroupSources = map[string][]domain.MemberSource{groupID: append(facts.GroupSources[groupID], domain.MemberSource{
						ID: "plugin-source", GroupID: groupID, Operation: domain.OperationAdd,
						Kind: domain.SourceKindManualNode, NodeLogicalID: &badID,
					})}
					facts.Groups[0].Sources = facts.GroupSources[groupID]
					facts.PolicyRules = facts.PolicyRules[1:]
					repo := sqlite.NewPublicationRepository(newCleanSQLiteDB(t))
					svc := publication.NewService(repo, nil, publication.WithConsistentFactsLoader(&memoryFactsLoader{facts: facts}))
					for _, target := range []domain.CompilerTarget{domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX} {
						t.Run(string(target), func(t *testing.T) {
							facts.Nodes[1].DisplayName = "Plugin node"
							control, _, err := svc.BuildCurrentArtifact(ctx, facts, target)
							if err != nil {
								t.Fatal(err)
							}
							if len(control.ExcludedNodes) != 1 || control.ExcludedNodes[0].DisplayName != "Plugin node" {
								t.Fatalf("safe control must retain one identifiable exclusion: %+v", control.ExcludedNodes)
							}
							facts.Nodes[1].DisplayName = "Region " + secret // No password=/token= marker for generic redaction.
							before, err := json.Marshal(facts)
							if err != nil {
								t.Fatal(err)
							}
							preview, err := svc.Preview(ctx, publication.PreviewQuery{Target: target})
							if err != nil {
								t.Fatal(err)
							}
							if len(preview.Manifest.Excluded) != 1 {
								t.Fatalf("expected plugin exclusion: %+v", preview.Manifest)
							}
							item := preview.Manifest.Excluded[0]
							if item.NodeID != badID || item.Code != "unsupported_target_capability" || !strings.Contains(item.Reason, "plugin") || item.DisplayName != "" {
								t.Fatalf("plugin secret must fall back to ID: %+v", item)
							}
							encoded, err := json.Marshal(preview)
							needle := secret
							if secret == "true" || secret == "false" {
								// Boolean words can legitimately occur elsewhere in JSON;
								// the forbidden value here is the raw credential label.
								needle = "Region " + secret
							}
							if err != nil || strings.Contains(string(encoded), needle) {
								t.Fatalf("management preview retained nested plugin secret (marshal error: %v)", err)
							}
							stored, err := repo.GetByID(ctx, preview.SnapshotID)
							if err != nil || stored.Manifest == nil {
								t.Fatalf("preview manifest not persisted: %v", err)
							}
							encoded, err = json.Marshal(stored.Manifest)
							if err != nil || strings.Contains(string(encoded), needle) || strings.Contains(string(encoded), "display_name") {
								t.Fatalf("immutable manifest retained nested plugin label (marshal error: %v)", err)
							}
							if string(preview.Content) != string(control.Content) || preview.ContentDigest != control.ContentDigest || preview.SnapshotDigest != control.SnapshotDigest || string(stored.Content) != string(control.Content) {
								t.Fatal("label sanitization changed compiled bytes or digests")
							}
							after, err := json.Marshal(facts)
							if err != nil || string(before) != string(after) {
								t.Fatal("label sanitization mutated input facts")
							}
						})
					}
				})
			}
		}
	}
}

func TestPublicationExclusionDisplayNameOmitsUnparsedPluginOptions(t *testing.T) {
	ctx := context.Background()
	const secret = "UNPARSED_PLUGIN_SECRET"
	for _, alias := range []string{"plugin-opts", "plugin_opts"} {
		for _, raw := range []string{
			`{"password":"` + secret + `",}`,
			`{"password":"` + secret + `"} trailing`,
			`{"password":null}`,
			`{"password":{"token":"` + secret + `"}}`,
			`{"password":["` + secret + `"]}`,
			`password="` + secret + `";password=other-secret`,
			`{"mode":"websocket","headers":{"Authorization":"Bearer ` + secret + `"}}`,
		} {
			t.Run(alias+"/"+raw, func(t *testing.T) {
				facts := incompatibleMihomoFacts()
				facts.Nodes[1].DisplayName = "Region " + secret
				facts.Nodes[1].Credentials.Transport = map[string]string{"plugin": "v2ray-plugin", alias: raw}
				// Even a malformed/opaque option cannot be labelled safely on a
				// target that excludes all plugins before their schema is consumed.
				facts.Groups = facts.Groups[:1]
				groupID := facts.Groups[0].ID
				badID := facts.Nodes[1].LogicalID
				facts.GroupSources[groupID] = append(facts.GroupSources[groupID], domain.MemberSource{
					ID: "opaque-plugin", GroupID: groupID, Operation: domain.OperationAdd,
					Kind: domain.SourceKindManualNode, NodeLogicalID: &badID,
				})
				facts.Groups[0].Sources = facts.GroupSources[groupID]
				facts.PolicyRules = facts.PolicyRules[1:]
				svc := publication.NewService(newMockPublicationRepo(), nil, publication.WithConsistentFactsLoader(&memoryFactsLoader{facts: facts}))
				preview, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetSingBox})
				if err != nil {
					t.Fatal(err)
				}
				if len(preview.Manifest.Excluded) != 1 || preview.Manifest.Excluded[0].NodeID != badID || preview.Manifest.Excluded[0].DisplayName != "" {
					t.Fatal("unparsed plugin must use node ID, never an unproven display name")
				}
				encoded, err := json.Marshal(preview)
				if err != nil || strings.Contains(string(encoded), secret) {
					t.Fatal("unparsed credential leaked into management preview")
				}
			})
		}
	}
}

func TestPublicationExclusionLegacyNameRemainsOptional(t *testing.T) {
	var manifest domain.PublicationManifest
	if err := json.Unmarshal([]byte(`{"excluded":[{"node_id":"old-node","code":"format_error","reason":"invalid entry"}]}`), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Excluded) != 1 || manifest.Excluded[0].DisplayName != "" || manifest.Excluded[0].NodeID != "old-node" {
		t.Fatalf("legacy manifest changed: %+v", manifest)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil || strings.Contains(string(encoded), "display_name") {
		t.Fatalf("legacy manifest invented a label: %s, %v", encoded, err)
	}
}
