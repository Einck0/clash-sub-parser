package compiler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func TestPluginCredentialNormalizationMatchesRenderedPassword(t *testing.T) {
	cases := []struct {
		name, form, value, want string
	}{
		{"semicolon_unquoted", ";", "password=UNQUOTED_PLUGIN_SECRET", "UNQUOTED_PLUGIN_SECRET"},
		{"semicolon_single_quoted", ";", "password='SINGLE_QUOTED_PLUGIN_SECRET'", "SINGLE_QUOTED_PLUGIN_SECRET"},
		{"semicolon_double_quoted", ";", `password="DOUBLE_QUOTED_PLUGIN_SECRET"`, "DOUBLE_QUOTED_PLUGIN_SECRET"},
		{"ampersand_fallback", "&", "password='AMPERSAND_PLUGIN_SECRET'", "AMPERSAND_PLUGIN_SECRET"},
		{"json_string", "json", `"  JSON_STRING_PLUGIN_SECRET  "`, "JSON_STRING_PLUGIN_SECRET"},
		{"json_escaped_string", "json", `"ESCAPED_\u0050LUGIN_\"SECRET\""`, `ESCAPED_PLUGIN_"SECRET"`},
		{"json_large_number", "json", "9007199254740993123456789", "9007199254740993123456789"},
		{"json_exponent", "json", "1.234567890123456789e+42", "1.234567890123456789e+42"},
		// These assert the current compiler's scalar-to-string behavior, not an
		// independently verified external ShadowTLS/Restls runtime contract.
		{"json_boolean_true", "json", "true", "true"},
		{"json_boolean_false", "json", "false", "false"},
		{"bare_password_flag", ";", "password", "true"},
	}
	for _, plugin := range []string{"shadow-tls", "shadowtls", "restls"} {
		for _, alias := range []string{"plugin-opts", "plugin_opts"} {
			for _, tc := range cases {
				t.Run(plugin+"/"+alias+"/"+tc.name, func(t *testing.T) {
					var opts string
					if tc.form == "json" {
						required := ""
						if plugin == "restls" {
							required = `,"version-hint":"tls13"`
						}
						opts = fmt.Sprintf(`{"host":"cdn.example.org","password":%s%s}`, tc.value, required)
					} else {
						opts = "host=cdn.example.org" + tc.form + tc.value
						if plugin == "restls" {
							opts += tc.form + "version-hint=tls13"
						}
					}
					snapshot := pluginBoundarySnapshot(plugin, opts)
					snapshot.Nodes[0].Credentials.Transport = map[string]string{"plugin": plugin, alias: opts}
					assertPluginNormalizationInputUnchanged(t, &snapshot)

					result, err := compiler.CompileMihomo(context.Background(), &snapshot)
					if err != nil {
						t.Fatalf("supported password representation rejected: %v", err)
					}
					var rendered struct {
						Proxies []struct {
							Plugin string         `yaml:"plugin"`
							Opts   map[string]any `yaml:"plugin-opts"`
						} `yaml:"proxies"`
					}
					if err := yaml.Unmarshal(result.Content, &rendered); err != nil {
						t.Fatal(err)
					}
					if len(rendered.Proxies) != 1 {
						t.Fatalf("expected one plugin proxy, got %d", len(rendered.Proxies))
					}
					wantPlugin := "shadow-tls"
					if plugin == "restls" {
						wantPlugin = plugin
					}
					proxy := rendered.Proxies[0]
					if proxy.Plugin != wantPlugin {
						t.Fatalf("plugin = %q, want %q", proxy.Plugin, wantPlugin)
					}
					password, ok := proxy.Opts["password"].(string)
					if !ok || password != tc.want {
						t.Fatalf("password = %v (%T), want exact string %q", proxy.Opts["password"], proxy.Opts["password"], tc.want)
					}
					if plugin == "restls" && proxy.Opts["version-hint"] != "tls13" {
						t.Fatal("required restls version-hint was lost")
					}

					secrets, complete := compiler.CollectCredentialSecrets(snapshot.Nodes[0].Credentials)
					if !complete || !slices.Contains(secrets, password) {
						t.Fatal("collector must completely cover the exact rendered password")
					}
					if !slices.Contains(secrets, snapshot.Nodes[0].Credentials.Password) {
						t.Fatal("outer Shadowsocks credential disappeared from collector")
					}
					if diagnostics := compiler.ValidateTargetCapabilities(&snapshot, domain.TargetMihomo); len(diagnostics) != 0 {
						t.Fatalf("supported plugin rejected by capability validation: %+v", diagnostics)
					}

					// Exercise the real non-Mihomo rejection, without injecting the
					// secret into an unrelated name or error field.
					assertPluginNormalizationRejected(t, &snapshot, domain.TargetSingBox, "plugin", password)
				})
			}
		}
	}
}

func TestPluginCredentialNormalizationFailsClosed(t *testing.T) {
	const first = "FIRST_INVALID_PLUGIN_SECRET"
	const second = "SECOND_INVALID_PLUGIN_SECRET"
	cases := []struct {
		name, opts     string
		wantIncomplete bool
	}{
		{"invalid_json", `{"host":"cdn.example.org","password":"` + first + `",}`, true},
		{"json_trailing_data", `{"host":"cdn.example.org","password":"` + first + `"} trailing`, true},
		{"json_array", `["` + first + `"]`, true},
		{"password_object", `{"host":"cdn.example.org","password":{"token":"` + first + `"}}`, true},
		{"password_array", `{"host":"cdn.example.org","password":["` + first + `"]}`, true},
		{"password_null", `{"host":"cdn.example.org","password":null}`, true},
		{"semicolon_composite", `host=cdn.example.org;password={"token":"` + first + `"}`, true},
		{"semicolon_invalid_composite", `host=cdn.example.org;password={"token":"` + first + `"`, true},
		{"duplicate_conflict", "host=cdn.example.org;password='" + first + "';password=\"" + second + "\"", true},
		// Both values are understandable scalars, but the renderer must still
		// reject a conflicting canonical option instead of silently choosing one.
		{"canonical_key_conflict", `{"host":"cdn.example.org","password":"` + first + `","PASSWORD":"` + second + `"}`, false},
	}
	for _, plugin := range []string{"shadow-tls", "shadowtls", "restls"} {
		for _, alias := range []string{"plugin-opts", "plugin_opts"} {
			for _, tc := range cases {
				t.Run(plugin+"/"+alias+"/"+tc.name, func(t *testing.T) {
					opts := tc.opts
					if plugin == "restls" {
						if strings.HasPrefix(opts, "{") {
							opts = strings.Replace(opts, `"host":`, `"version-hint":"tls13","host":`, 1)
						} else if !strings.HasPrefix(opts, "[") {
							opts = "version-hint=tls13;" + opts
						}
					}
					snapshot := pluginBoundarySnapshot(plugin, opts)
					snapshot.Nodes[0].Credentials.Transport = map[string]string{"plugin": plugin, alias: opts}
					assertPluginNormalizationInputUnchanged(t, &snapshot)
					secrets, complete := compiler.CollectCredentialSecrets(snapshot.Nodes[0].Credentials)
					if tc.wantIncomplete && complete {
						t.Fatal("unparseable or composite password cannot be marked safe for display")
					}
					if !tc.wantIncomplete && complete && (!slices.Contains(secrets, first) || !slices.Contains(secrets, second)) {
						t.Fatal("collector marked conflicting options complete without covering both passwords")
					}
					assertPluginNormalizationRejected(t, &snapshot, domain.TargetMihomo, "ss", first, second)
				})
			}
		}
	}
}

func TestPluginCredentialNormalizationTransportAliasConflictStillRejected(t *testing.T) {
	snapshot := pluginBoundarySnapshot("shadow-tls", `host=cdn.example.org;password='FIRST_ALIAS_PLUGIN_SECRET'`)
	snapshot.Nodes[0].Credentials.Transport["plugin_opts"] = `{"host":"cdn.example.org","password":"SECOND_ALIAS_PLUGIN_SECRET"}`
	assertPluginNormalizationInputUnchanged(t, &snapshot)
	// Even fully understood credentials do not make conflicting aliases valid.
	secrets, complete := compiler.CollectCredentialSecrets(snapshot.Nodes[0].Credentials)
	if complete && (!slices.Contains(secrets, "FIRST_ALIAS_PLUGIN_SECRET") || !slices.Contains(secrets, "SECOND_ALIAS_PLUGIN_SECRET")) {
		t.Fatal("collector marked conflicting transport complete without covering both normalized passwords")
	}
	assertPluginNormalizationRejected(t, &snapshot, domain.TargetMihomo, "ss", "FIRST_ALIAS_PLUGIN_SECRET", "SECOND_ALIAS_PLUGIN_SECRET")
}

func TestPluginCredentialNormalizationOpaqueHeadersKeepRenderingButOmitLabels(t *testing.T) {
	const header = "Bearer OPAQUE_PLUGIN_SECRET"
	for _, alias := range []string{"plugin-opts", "plugin_opts"} {
		for _, opts := range []string{
			`{"mode":"websocket","headers":{"Authorization":"` + header + `"}}`,
			`{"mode":"websocket","headers":"{\"Authorization\":\"` + header + `\"}"}`,
			`mode=websocket;headers='{"Authorization":"` + header + `"}'`,
		} {
			t.Run(alias+"/"+opts, func(t *testing.T) {
				snapshot := pluginBoundarySnapshot("v2ray-plugin", opts)
				snapshot.Nodes[0].Credentials.Transport = map[string]string{"plugin": "v2ray-plugin", alias: opts}
				assertPluginNormalizationInputUnchanged(t, &snapshot)
				result, err := compiler.CompileMihomo(context.Background(), &snapshot)
				if err != nil {
					t.Fatal(err)
				}
				var rendered struct {
					Proxies []struct {
						Opts struct {
							Headers map[string]string `yaml:"headers"`
						} `yaml:"plugin-opts"`
					} `yaml:"proxies"`
				}
				if err := yaml.Unmarshal(result.Content, &rendered); err != nil {
					t.Fatal(err)
				}
				if len(rendered.Proxies) != 1 || rendered.Proxies[0].Opts.Headers["Authorization"] != header {
					t.Fatal("header normalization changed actual rendered credentials")
				}
				secrets, complete := compiler.CollectCredentialSecrets(snapshot.Nodes[0].Credentials)
				if complete || !slices.Contains(secrets, header) {
					t.Fatal("opaque header values must be collected without authorizing display labels")
				}
				assertPluginNormalizationRejected(t, &snapshot, domain.TargetSingBox, "plugin", "OPAQUE_PLUGIN_SECRET")
			})
		}
	}
}

func assertPluginNormalizationInputUnchanged(t *testing.T, snapshot *resolver.ResolvedPolicySnapshot) {
	t.Helper()
	before, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		after, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Error("compilation, diagnostics or secret collection mutated the original snapshot/credentials")
		}
	})
}

func assertPluginNormalizationRejected(t *testing.T, snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget, feature string, secrets ...string) {
	t.Helper()
	result, err := compiler.Compile(context.Background(), snapshot, target)
	if err == nil || len(result.Content) != 0 {
		t.Fatal("invalid or unsupported plugin must fail closed without output")
	}
	var capability *compiler.CapabilityError
	if !errors.As(err, &capability) || capability.Target != target || capability.Location != "nodes[0]" || capability.Feature != feature {
		t.Fatalf("wrong capability rejection: %v", err)
	}
	diagnostics := compiler.ValidateTargetCapabilities(snapshot, target)
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_target_capability" || diagnostics[0].NodeID != snapshot.Nodes[0].LogicalID || diagnostics[0].Target != string(target) {
		t.Fatalf("node-specific capability rejection changed: %+v", diagnostics)
	}
	if target == domain.TargetSingBox {
		wantReason := "target singbox does not support plugin or plugin-opts"
		if capability.Reason != wantReason || diagnostics[0].Message != wantReason {
			t.Fatal("fixed unsupported-plugin message changed")
		}
	}
	encoded, marshalErr := json.Marshal(diagnostics)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	secrets = append(secrets, snapshot.Nodes[0].Credentials.Password)
	for _, secret := range secrets {
		if secret != "" && (strings.Contains(err.Error(), secret) || strings.Contains(string(encoded), secret)) {
			t.Fatal("capability error or public diagnostic leaked a credential")
		}
	}
}
