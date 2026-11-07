package compiler_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func makeSSPluginSnapshot(nodes ...resolver.ResolvedNode) resolver.ResolvedPolicySnapshot {
	members := make([]resolver.ResolvedGroupMember, len(nodes))
	nodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		members[i] = resolver.ResolvedGroupMember{
			Kind:        resolver.MemberKindNode,
			TargetID:    n.LogicalID,
			DisplayName: n.DisplayName,
			Position:    i,
		}
		nodeIDs[i] = n.LogicalID
	}
	return resolver.ResolvedPolicySnapshot{
		Nodes:          nodes,
		NodeLogicalIDs: nodeIDs,
		Groups: []resolver.ResolvedGroup{
			{
				ID:                "grp-proxy",
				Name:              "Proxy",
				GroupType:         domain.GroupTypeSelect,
				Members:           members,
				NodeLogicalIDs:    nodeIDs,
				AllNodeLogicalIDs: nodeIDs,
			},
		},
		Rules: []resolver.ResolvedRule{
			{
				ID:              "rule-terminal",
				TargetGroupID:   "grp-proxy",
				TargetGroupName: "Proxy",
				Expression:      "MATCH",
				Position:        0,
				IsTerminal:      true,
			},
		},
	}
}

func testMihomoConfigValidity(t *testing.T, yamlContent []byte) {
	t.Helper()
	mihomoBin, err := exec.LookPath("mihomo")
	if err != nil {
		return // official mihomo binary not installed in test environment
	}
	tmpDir := t.TempDir()
	writeDeterministicMihomoGeodataFixtures(t, tmpDir)
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, yamlContent, 0o600); err != nil {
		t.Fatalf("write temp mihomo config: %v", err)
	}
	cmd := exec.Command(mihomoBin, "-t", "-d", tmpDir, "-f", cfgPath)
	output, execErr := cmd.CombinedOutput()
	if execErr != nil {
		t.Fatalf("official mihomo -t validation failed: %v\nOutput:\n%s\nConfig:\n%s", execErr, string(output), string(yamlContent))
	}
}

func TestMihomoShadowsocksPlugin_SupportedSerialization(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name           string
		transport      map[string]string
		wantPlugin     string
		wantOpts       map[string]any
		expectNoPlugin bool
	}{
		{
			name: "PlainSSWithoutPlugin",
			transport: map[string]string{
				"network": "tcp",
			},
			expectNoPlugin: true,
		},
		{
			name: "ObfsTLS_JSON",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin-opts": `{"mode":"tls","host":"bing.com"}`,
			},
			wantPlugin: "obfs",
			wantOpts: map[string]any{
				"mode": "tls",
				"host": "bing.com",
			},
		},
		{
			name: "ObfsHTTP_Semicolon",
			transport: map[string]string{
				"plugin":      "simple-obfs",
				"plugin-opts": "mode=http;host=example.com",
			},
			wantPlugin: "obfs",
			wantOpts: map[string]any{
				"mode": "http",
				"host": "example.com",
			},
		},
		{
			name: "Obfs_LegacySIP003Mapping",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin_opts": "obfs=tls;obfs-host=bing.com",
			},
			wantPlugin: "obfs",
			wantOpts: map[string]any{
				"mode": "tls",
				"host": "bing.com",
			},
		},
		{
			name: "V2RayPlugin_JSON",
			transport: map[string]string{
				"plugin": "v2ray-plugin",
				"plugin-opts": `{
					"mode": "websocket",
					"host": "example.com",
					"path": "/ws",
					"tls": true,
					"skip-cert-verify": true,
					"mux": true,
					"headers": {"Host": "example.com", "User-Agent": "test"}
				}`,
			},
			wantPlugin: "v2ray-plugin",
			wantOpts: map[string]any{
				"mode":             "websocket",
				"host":             "example.com",
				"path":             "/ws",
				"tls":              true,
				"skip-cert-verify": true,
				"mux":              true,
				"headers":          map[string]any{"Host": "example.com", "User-Agent": "test"},
			},
		},
		{
			name: "V2RayPlugin_SemicolonWithBareFlags",
			transport: map[string]string{
				"plugin":      "v2ray",
				"plugin-opts": "mode=websocket;host=example.com;path=/ws;tls;skip-cert-verify;mux",
			},
			wantPlugin: "v2ray-plugin",
			wantOpts: map[string]any{
				"mode":             "websocket",
				"host":             "example.com",
				"path":             "/ws",
				"tls":              true,
				"skip-cert-verify": true,
				"mux":              true,
			},
		},
		{
			name: "V2RayPlugin_HttpUpgrade",
			transport: map[string]string{
				"plugin":      "v2ray-plugin",
				"plugin-opts": "mode=websocket;host=example.com;v2ray-http-upgrade=true;v2ray-http-upgrade-fast-open=true",
			},
			wantPlugin: "v2ray-plugin",
			wantOpts: map[string]any{
				"mode":                         "websocket",
				"host":                         "example.com",
				"v2ray-http-upgrade":           true,
				"v2ray-http-upgrade-fast-open": true,
			},
		},
		{
			name: "ShadowTLS_JSON",
			transport: map[string]string{
				"plugin":      "shadow-tls",
				"plugin-opts": `{"host":"cloud.tencent.com","password":"shadow_tls_password","version":2,"fingerprint":"chrome"}`,
			},
			wantPlugin: "shadow-tls",
			wantOpts: map[string]any{
				"host":        "cloud.tencent.com",
				"password":    "shadow_tls_password",
				"version":     2,
				"fingerprint": "chrome",
			},
		},
		{
			name: "Restls_JSON",
			transport: map[string]string{
				"plugin": "restls",
				"plugin-opts": `{
					"host": "www.microsoft.com",
					"password": "restls_password",
					"version-hint": "tls13",
					"restls-script": "300?100<1,400~100"
				}`,
			},
			wantPlugin: "restls",
			wantOpts: map[string]any{
				"host":          "www.microsoft.com",
				"password":      "restls_password",
				"version-hint":  "tls13",
				"restls-script": "300?100<1,400~100",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.2.3.4", 8388, tc.transport)
			node := resolver.ResolvedNode{
				LogicalID:   nodeID,
				DisplayName: "SS-Node-1",
				Protocol:    domain.ProtocolSS,
				Server:      "1.2.3.4",
				Port:        8388,
				Credentials: domain.InboundProtocolCredential{
					Method:    "aes-128-gcm",
					Password:  "secret-ss-password",
					Transport: tc.transport,
				},
				Active: true,
			}
			snap := makeSSPluginSnapshot(node)

			res, err := compiler.CompileMihomo(ctx, &snap)
			if err != nil {
				t.Fatalf("unexpected CompileMihomo error: %v", err)
			}

			// Validate with official mihomo CLI if installed
			testMihomoConfigValidity(t, res.Content)

			var parsed struct {
				Proxies []map[string]any `yaml:"proxies"`
			}
			if unmarshalErr := yaml.Unmarshal(res.Content, &parsed); unmarshalErr != nil {
				t.Fatalf("failed to unmarshal generated Mihomo YAML: %v", unmarshalErr)
			}
			if len(parsed.Proxies) != 1 {
				t.Fatalf("expected 1 proxy, got %d", len(parsed.Proxies))
			}
			proxy := parsed.Proxies[0]

			if tc.expectNoPlugin {
				if plugin, exists := proxy["plugin"]; exists && plugin != "" {
					t.Fatalf("expected no plugin, got %v", plugin)
				}
				if opts, exists := proxy["plugin-opts"]; exists && opts != nil {
					t.Fatalf("expected no plugin-opts, got %v", opts)
				}
				return
			}

			gotPlugin, _ := proxy["plugin"].(string)
			if gotPlugin != tc.wantPlugin {
				t.Errorf("plugin mismatch: got %q, want %q", gotPlugin, tc.wantPlugin)
			}

			gotOpts, _ := proxy["plugin-opts"].(map[string]any)
			if gotOpts == nil {
				t.Fatalf("expected plugin-opts, got nil")
			}

			for wantK, wantV := range tc.wantOpts {
				gotV, ok := gotOpts[wantK]
				if !ok {
					t.Errorf("missing expected plugin-opts key %q", wantK)
					continue
				}
				// Verify exact typed scalar types
				switch expected := wantV.(type) {
				case bool:
					b, ok := gotV.(bool)
					if !ok || b != expected {
						t.Errorf("key %q: got %v (%T), want bool %v", wantK, gotV, gotV, expected)
					}
				case int:
					i, ok := gotV.(int)
					if !ok || i != expected {
						t.Errorf("key %q: got %v (%T), want int %v", wantK, gotV, gotV, expected)
					}
				case string:
					s, ok := gotV.(string)
					if !ok || s != expected {
						t.Errorf("key %q: got %v (%T), want string %v", wantK, gotV, gotV, expected)
					}
				case map[string]any:
					m, ok := gotV.(map[string]any)
					if !ok {
						t.Errorf("key %q: got %v (%T), want map[string]any", wantK, gotV, gotV)
						continue
					}
					for subK, subV := range expected {
						if fmt.Sprintf("%v", m[subK]) != fmt.Sprintf("%v", subV) {
							t.Errorf("key %q[%q]: got %v, want %v", wantK, subK, m[subK], subV)
						}
					}
				}
			}
		})
	}
}

func TestMihomoShadowsocksPlugin_UnsupportedPluginsFailClosed(t *testing.T) {
	ctx := context.Background()

	const secretMarker = "sensitive-secret-pw-888"

	cases := []struct {
		name      string
		transport map[string]string
	}{
		{
			name: "UnknownPluginName",
			transport: map[string]string{
				"plugin": "unknown-cipher-plugin",
			},
		},
		{
			name: "GostPlugin",
			transport: map[string]string{
				"plugin":      "gost-plugin",
				"plugin-opts": `{"mode":"websocket","host":"example.com"}`,
			},
		},
		{
			name: "Kcptun",
			transport: map[string]string{
				"plugin":      "kcptun",
				"plugin-opts": "key=secret;crypt=aes",
			},
		},
		{
			name: "Jls",
			transport: map[string]string{
				"plugin":      "jls",
				"plugin-opts": "host=example.com;username=user;password=pwd",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.2.3.4", 8388, tc.transport)
			node := resolver.ResolvedNode{
				LogicalID:   nodeID,
				DisplayName: "SS-Unsupported-Node",
				Protocol:    domain.ProtocolSS,
				Server:      "1.2.3.4",
				Port:        8388,
				Credentials: domain.InboundProtocolCredential{
					Method:    "aes-128-gcm",
					Password:  secretMarker,
					Transport: tc.transport,
				},
				Active: true,
			}
			snap := makeSSPluginSnapshot(node)

			_, err := compiler.CompileMihomo(ctx, &snap)
			if err == nil {
				t.Fatalf("expected CompileMihomo to fail, but succeeded")
			}

			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("expected error to be *compiler.CapabilityError, got %T: %v", err, err)
			}
			if capErr.Target != domain.TargetMihomo {
				t.Errorf("expected Target %s, got %s", domain.TargetMihomo, capErr.Target)
			}
			if capErr.Feature != "ss" {
				t.Errorf("expected Feature ss, got %s", capErr.Feature)
			}
			if !strings.Contains(capErr.Reason, "unsupported shadowsocks plugin") {
				t.Errorf("error reason %q does not contain expected %q", capErr.Reason, "unsupported shadowsocks plugin")
			}
			if strings.Contains(capErr.Error(), secretMarker) {
				t.Fatalf("CapabilityError leaked secret marker %q: %s", secretMarker, capErr.Error())
			}

			diags := compiler.ValidateTargetCapabilities(&snap, domain.TargetMihomo)
			if len(diags) == 0 {
				t.Fatalf("expected ValidateTargetCapabilities diagnostics, got none")
			}
			foundDiag := false
			for _, d := range diags {
				if d.NodeID == node.LogicalID && strings.Contains(d.Message, "unsupported shadowsocks plugin") {
					foundDiag = true
				}
				if strings.Contains(d.Message, secretMarker) {
					t.Fatalf("diagnostic leaked secret marker: %s", d.Message)
				}
			}
			if !foundDiag {
				t.Errorf("diagnostics did not include expected failure: %+v", diags)
			}
		})
	}
}

func TestMihomoShadowsocksPlugin_InvalidConfigurationsFailClosed(t *testing.T) {
	ctx := context.Background()

	const secretMarker = "sensitive-secret-pw-999"

	cases := []struct {
		name       string
		transport  map[string]string
		wantReason string
	}{
		{
			name: "PluginOptsWithoutPlugin",
			transport: map[string]string{
				"plugin-opts": "mode=tls;host=bing.com",
			},
			wantReason: "plugin-opts configured without plugin",
		},
		{
			name: "ObfsMissingMode",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin-opts": "host=bing.com",
			},
			wantReason: "missing required mode in obfs plugin-opts",
		},
		{
			name: "ObfsUnsupportedMode",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin-opts": "mode=quic;host=bing.com",
			},
			wantReason: "unsupported obfs mode",
		},
		{
			name: "ObfsUnknownOption",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin-opts": "mode=tls;unknown_extra_param=1",
			},
			wantReason: "unsupported option",
		},
		{
			name: "V2RayPluginMissingMode",
			transport: map[string]string{
				"plugin":      "v2ray-plugin",
				"plugin-opts": "host=example.com;path=/ws",
			},
			wantReason: "missing required mode in v2ray-plugin plugin-opts",
		},
		{
			name: "V2RayPluginUnsupportedMode",
			transport: map[string]string{
				"plugin":      "v2ray-plugin",
				"plugin-opts": "mode=quic;host=example.com",
			},
			wantReason: "unsupported v2ray-plugin mode",
		},
		{
			name: "V2RayPluginUnknownOption",
			transport: map[string]string{
				"plugin":      "v2ray-plugin",
				"plugin-opts": "mode=websocket;invalid_v2ray_flag=true",
			},
			wantReason: "unsupported option",
		},
		{
			name: "V2RayPluginInvalidBoolean",
			transport: map[string]string{
				"plugin":      "v2ray-plugin",
				"plugin-opts": "mode=websocket;tls=not_a_valid_bool",
			},
			wantReason: "invalid boolean value",
		},
		{
			name: "ShadowTLSMissingHost",
			transport: map[string]string{
				"plugin":      "shadow-tls",
				"plugin-opts": "password=mypassword;version=2",
			},
			wantReason: "missing required host",
		},
		{
			name: "ShadowTLSMissingPassword",
			transport: map[string]string{
				"plugin":      "shadow-tls",
				"plugin-opts": "host=cloud.tencent.com;version=2",
			},
			wantReason: "missing required password",
		},
		{
			name: "ShadowTLSInvalidVersion",
			transport: map[string]string{
				"plugin":      "shadow-tls",
				"plugin-opts": "host=cloud.tencent.com;password=mypassword;version=99",
			},
			wantReason: "unsupported shadow-tls version",
		},
		{
			name: "RestlsMissingHost",
			transport: map[string]string{
				"plugin":      "restls",
				"plugin-opts": "password=pwd;version-hint=tls13",
			},
			wantReason: "missing required host",
		},
		{
			name: "RestlsMissingPassword",
			transport: map[string]string{
				"plugin":      "restls",
				"plugin-opts": "host=www.microsoft.com;version-hint=tls13",
			},
			wantReason: "missing required password",
		},
		{
			name: "RestlsMissingVersionHint",
			transport: map[string]string{
				"plugin":      "restls",
				"plugin-opts": "host=www.microsoft.com;password=pwd",
			},
			wantReason: "missing required version-hint",
		},
		{
			name: "ConflictingPluginOptsKeys",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin-opts": "mode=tls;host=bing.com",
				"plugin_opts": "mode=http;host=other.com",
			},
			wantReason: "conflicting plugin-opts and plugin_opts",
		},
		{
			name: "ConflictingObfsModeOptions",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin-opts": "mode=tls;obfs=http",
			},
			wantReason: "conflicting mode and obfs",
		},
		{
			name: "InvalidJSONInPluginOpts",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin-opts": "{invalid-json-content",
			},
			wantReason: "invalid JSON syntax",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.2.3.4", 8388, tc.transport)
			node := resolver.ResolvedNode{
				LogicalID:   nodeID,
				DisplayName: "SS-Fail-Node",
				Protocol:    domain.ProtocolSS,
				Server:      "1.2.3.4",
				Port:        8388,
				Credentials: domain.InboundProtocolCredential{
					Method:    "aes-128-gcm",
					Password:  secretMarker,
					Transport: tc.transport,
				},
				Active: true,
			}
			snap := makeSSPluginSnapshot(node)

			_, err := compiler.CompileMihomo(ctx, &snap)
			if err == nil {
				t.Fatalf("expected CompileMihomo to fail, but succeeded")
			}

			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("expected error to be *compiler.CapabilityError, got %T: %v", err, err)
			}
			if capErr.Target != domain.TargetMihomo {
				t.Errorf("expected Target %s, got %s", domain.TargetMihomo, capErr.Target)
			}
			if capErr.Feature != "ss" {
				t.Errorf("expected Feature ss, got %s", capErr.Feature)
			}
			if !strings.Contains(capErr.Reason, tc.wantReason) {
				t.Errorf("error reason %q does not contain expected %q", capErr.Reason, tc.wantReason)
			}
			if strings.Contains(capErr.Error(), secretMarker) {
				t.Fatalf("CapabilityError leaked secret marker %q: %s", secretMarker, capErr.Error())
			}

			// Also verify ValidateTargetCapabilities reports diagnosis
			diags := compiler.ValidateTargetCapabilities(&snap, domain.TargetMihomo)
			if len(diags) == 0 {
				t.Fatalf("expected ValidateTargetCapabilities to report diagnostics, got none")
			}
			foundDiag := false
			for _, d := range diags {
				if d.NodeID == node.LogicalID && strings.Contains(d.Message, tc.wantReason) {
					foundDiag = true
				}
				if strings.Contains(d.Message, secretMarker) {
					t.Fatalf("ValidateTargetCapabilities diagnostic leaked secret marker: %s", d.Message)
				}
			}
			if !foundDiag {
				t.Errorf("diagnostics did not include failure for node: %+v", diags)
			}
		})
	}
}

func TestMihomoShadowsocksPlugin_SourceSnapshotImmutability(t *testing.T) {
	ctx := context.Background()

	nodeID1 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.2.3.4", 8388, map[string]string{"plugin": "obfs"})
	nodeID2 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.2.3.4", 8389, map[string]string{"plugin": "v2ray-plugin"})

	originalSnap := makeSSPluginSnapshot(
		resolver.ResolvedNode{
			LogicalID:   nodeID1,
			DisplayName: "SS-Obfs",
			Protocol:    domain.ProtocolSS,
			Server:      "1.2.3.4",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{
				Method:   "aes-128-gcm",
				Password: "password-1",
				Transport: map[string]string{
					"plugin":      "obfs",
					"plugin-opts": "mode=tls;host=bing.com",
				},
			},
			Active: true,
		},
		resolver.ResolvedNode{
			LogicalID:   nodeID2,
			DisplayName: "SS-V2Ray",
			Protocol:    domain.ProtocolSS,
			Server:      "1.2.3.4",
			Port:        8389,
			Credentials: domain.InboundProtocolCredential{
				Method:   "aes-128-gcm",
				Password: "password-2",
				Transport: map[string]string{
					"plugin":      "v2ray-plugin",
					"plugin-opts": `{"mode":"websocket","host":"example.com","tls":true}`,
				},
			},
			Active: true,
		},
	)

	beforeJSON, err := json.Marshal(originalSnap)
	if err != nil {
		t.Fatalf("json.Marshal before: %v", err)
	}

	_, compileErr := compiler.CompileMihomo(ctx, &originalSnap)
	if compileErr != nil {
		t.Fatalf("CompileMihomo failed: %v", compileErr)
	}

	afterJSON, err := json.Marshal(originalSnap)
	if err != nil {
		t.Fatalf("json.Marshal after: %v", err)
	}

	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("snapshot mutated after CompileMihomo:\nbefore: %s\nafter:  %s", string(beforeJSON), string(afterJSON))
	}
	if !reflect.DeepEqual(originalSnap.Nodes[0].Credentials.Transport, map[string]string{
		"plugin":      "obfs",
		"plugin-opts": "mode=tls;host=bing.com",
	}) {
		t.Fatalf("node[0] Transport was mutated")
	}
}
