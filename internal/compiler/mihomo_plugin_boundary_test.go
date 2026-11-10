package compiler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func pluginBoundarySnapshot(plugin, opts string) resolver.ResolvedPolicySnapshot {
	return makeSSPluginSnapshot(resolver.ResolvedNode{
		LogicalID: "plugin-boundary-node", DisplayName: "Plugin boundary", Protocol: domain.ProtocolSS,
		Server: "192.0.2.60", Port: 8388, Active: true,
		Credentials: domain.InboundProtocolCredential{
			Method: "aes-128-gcm", Password: "SYNTHETIC_CREDENTIAL_SENTINEL",
			Transport: map[string]string{"plugin": plugin, "plugin-opts": opts},
		},
	})
}

func TestMihomoPluginBoundaryRejectsCompositeAliasesWithoutPanic(t *testing.T) {
	cases := []struct{ plugin, opts string }{
		{"obfs", `{"mode":{"x":1},"obfs":{"x":1}}`},
		{"simple-obfs", `{"mode":["tls"],"obfs":["tls"]}`},
		{"obfs", `{"mode":"tls","host":{"x":1},"obfs-host":{"x":1}}`},
		{"obfs", `{"mode":"tls","host":["a"],"obfs_host":["a"]}`},
		{"obfs", `{"mode":{"x":1},"obfs":{"x":2}}`},
		{"obfs", `{"mode":"tls","host":["a"],"obfs-host":["b"]}`},
		{"v2ray", `{"mode":"websocket","skip-cert-verify":{},"skip_cert_verify":{}}`},
		{"v2ray-plugin", `{"mode":"websocket","tls":[],"TLS":[]}`},
		{"shadowtls", `{"host":"cdn.example.org","password":"pw","skip-cert-verify":{},"skip_cert_verify":{}}`},
		{"shadow-tls", `{"host":"cdn.example.org","password":"pw","version":[],"VERSION":[]}`},
		{"restls", `{"host":"cdn.example.org","password":"pw","version-hint":{},"version_hint":{}}`},
		{"restls", `{"host":"cdn.example.org","password":"pw","restls-script":[],"restls_script":[]}`},
		{"v2ray-plugin", `{"mode":"websocket","headers":{"Host":"a"},"HEADERS":{"Host":"b"}}`},
		{"v2ray-plugin", `{"mode":"websocket","headers":{"X-Test":["a"]}}`},
		{"v2ray-plugin", `{"mode":"websocket","headers":{"X-Test":{"nested":"a"}}}`},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%02d_%s", i, tc.plugin), func(t *testing.T) {
			snapshot := pluginBoundarySnapshot(tc.plugin, tc.opts)
			var first string
			for attempt := 0; attempt < 12; attempt++ {
				result, err := compiler.CompileMihomo(context.Background(), &snapshot)
				if err == nil || len(result.Content) != 0 {
					t.Fatal("invalid composite options must fail closed, not strip plugin")
				}
				var capability *compiler.CapabilityError
				if !errors.As(err, &capability) {
					t.Fatalf("expected capability error, got %T", err)
				}
				if attempt == 0 {
					first = err.Error()
				} else if first != err.Error() {
					t.Fatalf("non-deterministic error: %q versus %q", first, err.Error())
				}
				if diagnostics := compiler.ValidateTargetCapabilities(&snapshot, domain.TargetMihomo); len(diagnostics) == 0 {
					t.Fatal("invalid plugin missing capability diagnostic")
				}
			}
		})
	}
}

func TestMihomoPluginBoundaryPreservesValidNestedHeadersAndAliases(t *testing.T) {
	cases := []struct {
		plugin, opts, wantPlugin string
		want                     map[string]any
	}{
		{"simple-obfs", `{"mode":"tls","obfs":"tls","host":"cdn.example.org","obfs-host":"cdn.example.org","obfs_host":"cdn.example.org"}`, "obfs", map[string]any{"mode": "tls", "host": "cdn.example.org"}},
		{"v2ray", `{"mode":"websocket","skip-cert-verify":true,"skip_cert_verify":true,"headers":{"Host":"cdn.example.org","X-Test":"value"},"HEADERS":{"X-Test":"value","Host":"cdn.example.org"}}`, "v2ray-plugin", map[string]any{"mode": "websocket", "skip-cert-verify": true, "headers": map[string]any{"Host": "cdn.example.org", "X-Test": "value"}}},
		{"v2ray-plugin", `mode=websocket;headers={"Host":"cdn.example.org"}`, "v2ray-plugin", map[string]any{"mode": "websocket", "headers": map[string]any{"Host": "cdn.example.org"}}},
		{"shadowtls", `{"host":"cdn.example.org","password":"pw","skip_cert_verify":false,"skip-cert-verify":false}`, "shadow-tls", map[string]any{"host": "cdn.example.org", "password": "pw", "skip-cert-verify": false}},
		{"restls", `{"host":"cdn.example.org","password":"pw","version_hint":"tls13","version-hint":"tls13"}`, "restls", map[string]any{"host": "cdn.example.org", "password": "pw", "version-hint": "tls13"}},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%02d_%s", i, tc.plugin), func(t *testing.T) {
			snapshot := pluginBoundarySnapshot(tc.plugin, tc.opts)
			result, err := compiler.CompileMihomo(context.Background(), &snapshot)
			if err != nil {
				t.Fatal(err)
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
			if len(rendered.Proxies) != 1 || rendered.Proxies[0].Plugin != tc.wantPlugin || !reflect.DeepEqual(rendered.Proxies[0].Opts, tc.want) {
				t.Fatalf("plugin configuration changed: %+v", rendered.Proxies)
			}
		})
	}
}

func TestMihomoPluginBoundaryDiagnosticsNeverEchoInputs(t *testing.T) {
	const secret = "PRIVATE_PLUGIN_SENTINEL"
	cases := []struct{ plugin, opts string }{
		{secret, ""},
		{"obfs", `mode=tls;` + secret + `=true`},
		{"v2ray-plugin", `mode=websocket;` + secret},
		{"shadow-tls", `host=cdn.example.org;password=pw;` + secret + `=value`},
		{"restls", `host=cdn.example.org;password=pw;version-hint=tls13;` + secret},
		{"obfs", `mode=` + secret},
		{"v2ray-plugin", `mode=` + secret},
		{"v2ray-plugin", `mode=websocket;tls=` + secret},
		{"shadow-tls", `host=cdn.example.org;password=pw;version=` + secret},
		{"shadow-tls", `host=cdn.example.org;password=pw;version=987654321`},
		{"obfs", `mode=tls;=` + secret},
		{"obfs", `{"` + secret + `":}`},
		{"obfs", `{"mode":"tls","` + secret + `"`},
		{"obfs", `{"mode":"tls"} ` + secret},
		{"obfs", `["` + secret + `"]`},
		{"v2ray-plugin", `mode=websocket;headers={"` + secret + `":}`},
		{"v2ray-plugin", `{"mode":"websocket","headers":"{\"` + secret + `\":}"}`},
		{"v2ray-plugin", `{"mode":"websocket","headers":{"` + secret + `":{"` + secret + `":"value"}}}`},
		{"v2ray-plugin", `mode=websocket;` + secret + `=1;` + secret + `=2`},
		{"obfs", `mode=tls;mode=` + secret},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%02d_%s", i, tc.plugin), func(t *testing.T) {
			snapshot := pluginBoundarySnapshot(tc.plugin, tc.opts)
			_, err := compiler.CompileMihomo(context.Background(), &snapshot)
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			diagnostics := compiler.ValidateTargetCapabilities(&snapshot, domain.TargetMihomo)
			if len(diagnostics) != 1 || diagnostics[0].NodeID != snapshot.Nodes[0].LogicalID {
				t.Fatalf("missing node-specific rejection: %+v", diagnostics)
			}
			encoded, marshalErr := json.Marshal(diagnostics)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			for _, value := range []string{err.Error(), string(encoded)} {
				if strings.Contains(value, secret) || strings.Contains(value, "SYNTHETIC_CREDENTIAL_SENTINEL") || strings.Contains(value, "987654321") {
					t.Fatalf("error leaked input: %s", value)
				}
			}
		})
	}
}
