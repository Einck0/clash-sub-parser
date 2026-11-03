package compiler_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func allTenProtocolNodesFixture() []resolver.ResolvedNode {
	return []resolver.ResolvedNode{
		{
			LogicalID:   "node-ss",
			DisplayName: "ss-edge",
			Protocol:    domain.ProtocolSS,
			Server:      "198.51.100.10",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{
				Method:   "aes-256-gcm",
				Password: "ss-secret-password",
			},
			Active:   true,
			Position: 0,
		},
		{
			LogicalID:   "node-vmess",
			DisplayName: "vmess-edge",
			Protocol:    domain.ProtocolVMess,
			Server:      "198.51.100.11",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID:    "11111111-1111-1111-1111-111111111111",
				AlterID: 0,
				Method:  "auto",
				Transport: map[string]string{
					"network": "ws",
					"tls":     "true",
					"sni":     "vmess.example.com",
					"path":    "/vmess-ws",
					"host":    "vmess.example.com",
				},
			},
			Active:   true,
			Position: 1,
		},
		{
			LogicalID:   "node-vless",
			DisplayName: "vless-reality-edge",
			Protocol:    domain.ProtocolVLESS,
			Server:      "198.51.100.12",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID: "22222222-2222-2222-2222-222222222222",
				Transport: map[string]string{
					"network": "tcp",
					"tls":     "true",
					"sni":     "reality.example.com",
					"flow":    "xtls-rprx-vision",
					"fp":      "chrome",
					"pbk":     "jNXHt1yRo0vD5_1N6p2W3x4Y5z6A7b8C9d0E1f2G3h4",
					"sid":     "01ab",
				},
			},
			Active:   true,
			Position: 2,
		},
		{
			LogicalID:   "node-trojan",
			DisplayName: "trojan-grpc-edge",
			Protocol:    domain.ProtocolTrojan,
			Server:      "198.51.100.13",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				Password: "trojan-secret-password",
				Transport: map[string]string{
					"network":          "grpc",
					"sni":              "trojan.example.com",
					"skip_cert_verify": "true",
					"alpn":             "h2,http/1.1",
					"service_name":     "trojan-grpc-svc",
				},
			},
			Active:   true,
			Position: 3,
		},
		{
			LogicalID:   "node-hy2",
			DisplayName: "hy2-edge",
			Protocol:    domain.ProtocolHysteria2,
			Server:      "198.51.100.14",
			Port:        8443,
			Credentials: domain.InboundProtocolCredential{
				Password: "hysteria2-secret-password",
				Transport: map[string]string{
					"sni":              "hy2.example.com",
					"skip_cert_verify": "true",
					"up":               "100 Mbps",
					"down":             "500 Mbps",
					"obfs":             "salamander",
					"obfs-password":    "hy2-obfs-secret",
					"ports":            "20000-30000",
				},
			},
			Active:   true,
			Position: 4,
		},
		{
			LogicalID:   "node-wg",
			DisplayName: "wg-edge",
			Protocol:    domain.ProtocolWireGuard,
			Server:      "198.51.100.15",
			Port:        51820,
			Credentials: domain.InboundProtocolCredential{
				PrivateKey:   "aGVsbG8td29ybGQtdGVzdC1wcml2YXRlLWtleS0xMjM=",
				PublicKey:    "aGVsbG8td29ybGQtdGVzdC1wdWJsaWMta2V5LTEyMzQ=",
				PreSharedKey: "aGVsbG8td29ybGQtdGVzdC1wc2sta2V5LTEyMzQ1Njc=",
				LocalAddress: []string{"10.0.0.2/32", "fd00::2/128"},
				Reserved:     []uint8{12, 34, 56},
				MTU:          1420,
				DNS:          []string{"1.1.1.1", "8.8.8.8"},
			},
			Active:   true,
			Position: 5,
		},
		{
			LogicalID:   "node-tuic",
			DisplayName: "tuic-edge",
			Protocol:    domain.ProtocolTUIC,
			Server:      "198.51.100.16",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID:              "33333333-3333-3333-3333-333333333333",
				Password:          "tuic-secret-password",
				CongestionControl: "bbr",
				UDPRelayMode:      "native",
				ALPN:              []string{"h3"},
				SNI:               "tuic.example.com",
				DisableSNI:        true,
				Transport: map[string]string{
					"skip_cert_verify": "true",
				},
			},
			Active:   true,
			Position: 6,
		},
		{
			LogicalID:   "node-http",
			DisplayName: "http-edge",
			Protocol:    domain.ProtocolHTTP,
			Server:      "198.51.100.17",
			Port:        8080,
			Credentials: domain.InboundProtocolCredential{
				Username: "http-user",
				Password: "http-password",
				Transport: map[string]string{
					"tls":              "true",
					"sni":              "http.example.com",
					"skip_cert_verify": "true",
					"headers":          `{"X-Custom-Header":"custom-http-val"}`,
				},
			},
			Active:   true,
			Position: 7,
		},
		{
			LogicalID:   "node-socks5",
			DisplayName: "socks5-edge",
			Protocol:    domain.ProtocolSocks5,
			Server:      "198.51.100.18",
			Port:        1080,
			Credentials: domain.InboundProtocolCredential{
				Username: "socks-user",
				Password: "socks-password",
				Transport: map[string]string{
					"udp":              "true",
					"tls":              "true",
					"sni":              "socks.example.com",
					"skip_cert_verify": "true",
				},
			},
			Active:   true,
			Position: 8,
		},
		{
			LogicalID:   "node-anytls",
			DisplayName: "anytls-edge",
			Protocol:    domain.ProtocolAnyTLS,
			Server:      "198.51.100.19",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				Password: "anytls-secret-password",
				Transport: map[string]string{
					"sni":                         "anytls.example.com",
					"alpn":                        "h2,http/1.1",
					"fp":                          "chrome",
					"skip_cert_verify":            "true",
					"udp":                         "true",
					"idle-session-check-interval": "30",
					"idle-session-timeout":        "60",
					"min-idle-session":            "1",
				},
			},
			Active:   true,
			Position: 9,
		},
	}
}

func makeSnapshotWithNodes(nodes []resolver.ResolvedNode) *resolver.ResolvedPolicySnapshot {
	nodeIDs := make([]string, len(nodes))
	members := make([]resolver.ResolvedGroupMember, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.LogicalID
		members[i] = resolver.ResolvedGroupMember{
			Kind:        resolver.MemberKindNode,
			TargetID:    n.LogicalID,
			DisplayName: n.DisplayName,
			Position:    i,
		}
	}
	return &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "snap-proxy-chain-test",
		CompilerVersion: "1.0.0",
		Nodes:           nodes,
		Groups: []resolver.ResolvedGroup{
			{
				ID:                "grp-proxy",
				Name:              "proxy",
				GroupType:         domain.GroupTypeSelect,
				Members:           members,
				NodeLogicalIDs:    nodeIDs,
				AllNodeLogicalIDs: nodeIDs,
				Position:          0,
			},
		},
		Rules: []resolver.ResolvedRule{
			{
				ID:              "r-match",
				TargetGroupID:   "grp-proxy",
				TargetGroupName: "proxy",
				Expression:      "MATCH",
				Position:        0,
				IsTerminal:      true,
			},
		},
	}
}

func TestMihomoCompiler_ProxyChain_AllTenProtocols_PresentAndOmitted(t *testing.T) {
	ctx := context.Background()

	t.Run("omitted when DialerProxy is empty", func(t *testing.T) {
		nodes := allTenProtocolNodesFixture()
		for i := range nodes {
			nodes[i].DialerProxy = ""
		}
		snap := makeSnapshotWithNodes(nodes)

		res, err := compiler.CompileMihomo(ctx, snap)
		if err != nil {
			t.Fatalf("CompileMihomo failed: %v", err)
		}

		var raw struct {
			Proxies []map[string]any `yaml:"proxies"`
		}
		if err := yaml.Unmarshal(res.Content, &raw); err != nil {
			t.Fatalf("failed to unmarshal Mihomo YAML: %v", err)
		}
		if len(raw.Proxies) != 10 {
			t.Fatalf("expected 10 proxies, got %d", len(raw.Proxies))
		}
		for i, p := range raw.Proxies {
			if _, exists := p["dialer-proxy"]; exists {
				t.Errorf("proxy[%d] (%s) unexpectedly has dialer-proxy: %v", i, p["name"], p["dialer-proxy"])
			}
		}
		if strings.Contains(string(res.Content), "dialer-proxy") {
			t.Errorf("rendered YAML contains 'dialer-proxy' substring when all nodes have empty DialerProxy:\n%s", res.Content)
		}
	})

	t.Run("present for all 10 protocols when DialerProxy is populated", func(t *testing.T) {
		nodes := allTenProtocolNodesFixture()
		expectedDialers := make(map[string]string, len(nodes))
		for i := range nodes {
			dp := fmt.Sprintf("springboard-%s", nodes[i].DisplayName)
			nodes[i].DialerProxy = dp
			expectedDialers[nodes[i].DisplayName] = dp
		}
		snap := makeSnapshotWithNodes(nodes)

		res, err := compiler.CompileMihomo(ctx, snap)
		if err != nil {
			t.Fatalf("CompileMihomo failed: %v", err)
		}

		var raw struct {
			Proxies []map[string]any `yaml:"proxies"`
		}
		if err := yaml.Unmarshal(res.Content, &raw); err != nil {
			t.Fatalf("failed to unmarshal Mihomo YAML: %v", err)
		}
		if len(raw.Proxies) != 10 {
			t.Fatalf("expected 10 proxies, got %d", len(raw.Proxies))
		}

		for i, p := range raw.Proxies {
			name, _ := p["name"].(string)
			wantDialer, ok := expectedDialers[name]
			if !ok {
				t.Errorf("proxy[%d] has unexpected name %q", i, name)
				continue
			}
			gotDialer, exists := p["dialer-proxy"]
			if !exists {
				t.Errorf("proxy[%d] (%s) missing dialer-proxy in YAML output", i, name)
			} else if gotDialer != wantDialer {
				t.Errorf("proxy[%d] (%s) dialer-proxy = %v, want %q", i, name, gotDialer, wantDialer)
			}
		}
	})

	t.Run("mixed: present only for nodes with DialerProxy", func(t *testing.T) {
		nodes := allTenProtocolNodesFixture()
		// Only even indices have DialerProxy set
		for i := range nodes {
			if i%2 == 0 {
				nodes[i].DialerProxy = fmt.Sprintf("chain-target-%d", i)
			} else {
				nodes[i].DialerProxy = ""
			}
		}
		snap := makeSnapshotWithNodes(nodes)

		res, err := compiler.CompileMihomo(ctx, snap)
		if err != nil {
			t.Fatalf("CompileMihomo failed: %v", err)
		}

		var raw struct {
			Proxies []map[string]any `yaml:"proxies"`
		}
		if err := yaml.Unmarshal(res.Content, &raw); err != nil {
			t.Fatalf("failed to unmarshal Mihomo YAML: %v", err)
		}
		if len(raw.Proxies) != 10 {
			t.Fatalf("expected 10 proxies, got %d", len(raw.Proxies))
		}

		for i, p := range raw.Proxies {
			name, _ := p["name"].(string)
			gotDialer, exists := p["dialer-proxy"]
			if i%2 == 0 {
				want := fmt.Sprintf("chain-target-%d", i)
				if !exists {
					t.Errorf("proxy[%d] (%s) should have dialer-proxy %q, but key is missing", i, name, want)
				} else if gotDialer != want {
					t.Errorf("proxy[%d] (%s) dialer-proxy = %v, want %q", i, name, gotDialer, want)
				}
			} else {
				if exists {
					t.Errorf("proxy[%d] (%s) should omit dialer-proxy, but found: %v", i, name, gotDialer)
				}
			}
		}
	})
}

func TestCompiler_NonMihomoTargets_RejectDialerProxy(t *testing.T) {
	ctx := context.Background()
	nonMihomoTargets := []domain.CompilerTarget{
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}

	for _, target := range nonMihomoTargets {
		t.Run(fmt.Sprintf("%s rejects non-empty DialerProxy", target), func(t *testing.T) {
			// Use a standard SS node which all non-Mihomo targets support
			nodes := []resolver.ResolvedNode{
				{
					LogicalID:   "node-ss-1",
					DisplayName: "ss-target-test",
					Protocol:    domain.ProtocolSS,
					Server:      "198.51.100.10",
					Port:        8388,
					Credentials: domain.InboundProtocolCredential{
						Method:   "aes-256-gcm",
						Password: "ss-password-123",
					},
					DialerProxy: "springboard-node",
					Active:      true,
					Position:    0,
				},
			}
			snap := makeSnapshotWithNodes(nodes)

			// 1. Compile must fail with CapabilityError
			_, err := compiler.Compile(ctx, snap, target)
			if err == nil {
				t.Fatalf("target %s expected error for non-empty DialerProxy, got nil", target)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("target %s expected *CapabilityError, got %T: %v", target, err, err)
			}
			if capErr.Target != target {
				t.Errorf("target mismatch: got %s, want %s", capErr.Target, target)
			}
			if capErr.Feature != "dialer-proxy" {
				t.Errorf("feature mismatch: got %q, want 'dialer-proxy'", capErr.Feature)
			}

			// 2. ValidateTargetCapabilities must report unsupported_target_capability diagnostic
			diags := compiler.ValidateTargetCapabilities(snap, target)
			foundDialerDiag := false
			for _, d := range diags {
				if d.Code == "unsupported_target_capability" && strings.Contains(d.Message, "dialer-proxy") {
					foundDialerDiag = true
					break
				}
			}
			if !foundDialerDiag {
				t.Errorf("target %s ValidateTargetCapabilities missing dialer-proxy diagnostic: %+v", target, diags)
			}
		})

		t.Run(fmt.Sprintf("%s accepts empty DialerProxy for supported protocols", target), func(t *testing.T) {
			nodes := []resolver.ResolvedNode{
				{
					LogicalID:   "node-ss-1",
					DisplayName: "ss-target-test",
					Protocol:    domain.ProtocolSS,
					Server:      "198.51.100.10",
					Port:        8388,
					Credentials: domain.InboundProtocolCredential{
						Method:   "aes-256-gcm",
						Password: "ss-password-123",
					},
					DialerProxy: "",
					Active:      true,
					Position:    0,
				},
			}
			snap := makeSnapshotWithNodes(nodes)

			// ValidateTargetCapabilities should not report any dialer-proxy diagnostics
			diags := compiler.ValidateTargetCapabilities(snap, target)
			for _, d := range diags {
				if strings.Contains(d.Message, "dialer-proxy") {
					t.Errorf("target %s unexpectedly reported dialer-proxy diagnostic when DialerProxy is empty: %+v", target, d)
				}
			}

			// Compile should succeed without error
			res, err := compiler.Compile(ctx, snap, target)
			if err != nil {
				t.Fatalf("target %s failed to compile node with empty DialerProxy: %v", target, err)
			}
			if len(res.Content) == 0 {
				t.Fatalf("target %s rendered empty content", target)
			}
		})
	}
}
