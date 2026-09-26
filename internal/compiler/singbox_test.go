package compiler_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	probesingbox "clash-sub-parser/internal/probe/singbox"
	"clash-sub-parser/internal/resolver"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
)

func findSingBoxBinary() string {
	if bin := os.Getenv("SINGBOX_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	if path, err := exec.LookPath("sing-box"); err == nil {
		return path
	}
	for _, candidate := range []string{"/usr/local/bin/sing-box", "/usr/bin/sing-box"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func assertValidSingBoxConfig(t *testing.T, content []byte) option.Options {
	t.Helper()
	ctx := probesingbox.ExportOptionContext(context.Background())
	opts, err := singjson.UnmarshalExtendedContext[option.Options](ctx, content)
	if err != nil {
		t.Fatalf("official sing-box option unmarshal failed: %v\nContent:\n%s", err, string(content))
	}

	bin := findSingBoxBinary()
	if bin == "" {
		t.Fatal("expected official sing-box binary to be available for configuration check")
	}
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "sing-box.json")
	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write temp sing-box config: %v", err)
	}
	cmd := exec.Command(bin, "check", "-c", cfgPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("official sing-box check CLI (%s) failed: %v\nOutput: %s\nConfig:\n%s", bin, err, string(out), string(content))
	}
	return opts
}

func TestSingBoxGoldenFixture(t *testing.T) {
	assertTargetGoldenFixture(t, domain.TargetSingBox)

	res, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetSingBox)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	assertValidSingBoxConfig(t, res.Content)
}

func TestSingBoxOutputIsValidJSON(t *testing.T) {
	result, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetSingBox)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(result.Content, &value); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	assertValidSingBoxConfig(t, result.Content)

	raw := string(result.Content)
	for _, node := range fixtureSnapshot().Nodes {
		if strings.Contains(raw, node.LogicalID) {
			t.Fatalf("sing-box output must not contain fake LogicalID server %q:\n%s", node.LogicalID, raw)
		}
	}
	if !strings.Contains(raw, `"198.51.100.1"`) || !strings.Contains(raw, `"198.51.100.2"`) {
		t.Fatalf("sing-box output missing real node server addresses:\n%s", raw)
	}
}

func TestSingBoxSupportsGeositeAndProcessNameRules(t *testing.T) {
	snapshot := fixtureSnapshot()
	snapshot.Rules = []resolver.ResolvedRule{
		{
			ID:              "rule-domain-suffix",
			TargetGroupID:   snapshot.Groups[0].ID,
			TargetGroupName: snapshot.Groups[0].Name,
			Expression:      "DOMAIN-SUFFIX,example.com",
			Position:        0,
		},
		{
			ID:              "rule-geosite",
			TargetGroupID:   snapshot.Groups[0].ID,
			TargetGroupName: snapshot.Groups[0].Name,
			Expression:      "GEOSITE,category-ads-all",
			Position:        1,
		},
		{
			ID:              "rule-process-name",
			TargetGroupID:   snapshot.Groups[0].ID,
			TargetGroupName: snapshot.Groups[0].Name,
			Expression:      "PROCESS-NAME,curl",
			Position:        2,
		},
		{
			ID:              "rule-match",
			TargetGroupID:   snapshot.Groups[0].ID,
			TargetGroupName: snapshot.Groups[0].Name,
			Expression:      "MATCH",
			Position:        3,
			IsTerminal:      true,
		},
	}

	res, err := compiler.Compile(context.Background(), snapshot, domain.TargetSingBox)
	if err != nil {
		t.Fatalf("expected SingBox to support GEOSITE and PROCESS-NAME rules: %v", err)
	}
	assertValidSingBoxConfig(t, res.Content)
}

func TestSingBoxAllSevenProtocolsGroupsAndFourteenRules_OfficialValidation(t *testing.T) {
	ctx := context.Background()

	nodes := []resolver.ResolvedNode{
		{
			LogicalID:   "id-ss",
			DisplayName: "Node-SS",
			Protocol:    domain.ProtocolSS,
			Server:      "198.51.100.10",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{
				Method:   "2022-blake3-aes-128-gcm",
				Password: "AAAAAAAAAAAAAAAAAAAAAA==",
			},
			Active:   true,
			Position: 0,
		},
		{
			LogicalID:   "id-vmess",
			DisplayName: "Node-VMess",
			Protocol:    domain.ProtocolVMess,
			Server:      "vmess.example.com",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID:    "b831381d-6324-4d53-ad4f-8cda48b30811",
				Method:  "aes-128-gcm",
				AlterID: 0,
				Transport: map[string]string{
					"network": "ws",
					"tls":     "true",
					"sni":     "vmess.example.com",
					"host":    "cdn.example.com",
					"path":    "/vmess-ws",
					"alpn":    "h2,http/1.1",
					"fp":      "chrome",
				},
			},
			Active:   true,
			Position: 1,
		},
		{
			LogicalID:   "id-vless",
			DisplayName: "Node-VLESS-Reality",
			Protocol:    domain.ProtocolVLESS,
			Server:      "198.51.100.12",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID: "b831381d-6324-4d53-ad4f-8cda48b30812",
				Transport: map[string]string{
					"network": "tcp",
					"tls":     "true",
					"sni":     "www.microsoft.com",
					"flow":    "xtls-rprx-vision",
					"pbk":     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
					"sid":     "0123456789abcdef",
					"fp":      "safari",
				},
			},
			Active:   true,
			Position: 2,
		},
		{
			LogicalID:   "id-trojan",
			DisplayName: "Node-Trojan",
			Protocol:    domain.ProtocolTrojan,
			Server:      "trojan.example.com",
			Port:        8443,
			Credentials: domain.InboundProtocolCredential{
				Password: "trojan-secret-password",
				Transport: map[string]string{
					"network":      "grpc",
					"tls":          "true",
					"sni":          "trojan.example.com",
					"service_name": "trojan-grpc-svc",
				},
			},
			Active:   true,
			Position: 3,
		},
		{
			LogicalID:   "id-hy2",
			DisplayName: "Node-Hysteria2",
			Protocol:    domain.ProtocolHysteria2,
			Server:      "hy2.example.com",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				Password: "hy2-secret-password",
				Transport: map[string]string{
					"network":       "quic",
					"sni":           "hy2.example.com",
					"obfs":          "salamander",
					"obfs-password": "hy2-obfs-password",
					"up":            "100 Mbps",
					"down":          "500 Mbps",
				},
			},
			Active:   true,
			Position: 4,
		},
		{
			LogicalID:   "id-wg",
			DisplayName: "Node-WireGuard",
			Protocol:    domain.ProtocolWireGuard,
			Server:      "198.51.100.15",
			Port:        51820,
			Credentials: domain.InboundProtocolCredential{
				PrivateKey:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				PublicKey:    "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
				PreSharedKey: "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=",
				LocalAddress: []string{"10.0.0.2/32", "fd00::2/128"},
				Reserved:     []uint8{10, 20, 30},
				MTU:          1400,
				DNS:          []string{"1.1.1.1"},
			},
			Active:   true,
			Position: 5,
		},
		{
			LogicalID:   "id-tuic",
			DisplayName: "Node-TUIC",
			Protocol:    domain.ProtocolTUIC,
			Server:      "tuic.example.com",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID:              "b831381d-6324-4d53-ad4f-8cda48b30813",
				Password:          "tuic-secret-password",
				CongestionControl: "bbr",
				UDPRelayMode:      "native",
				ALPN:              []string{"h3"},
				SNI:               "tuic.example.com",
				DisableSNI:        true,
			},
			Active:   true,
			Position: 6,
		},
	}

	groupSelectID := "018f0b6e-4d7a-7abc-8def-012345678901"
	groupAutoID := "018f0b6e-4d7a-7abc-8def-012345678902"

	snapshot := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "snap-all-7-protocols",
		CompilerVersion: "1.0.0",
		Nodes:           nodes,
		Groups: []resolver.ResolvedGroup{
			{
				ID:        groupAutoID,
				Name:      "Auto-Fast",
				GroupType: domain.GroupTypeURLTest,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "id-ss", DisplayName: "Node-SS", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "id-vmess", DisplayName: "Node-VMess", Position: 1},
					{Kind: resolver.MemberKindNode, TargetID: "id-vless", DisplayName: "Node-VLESS-Reality", Position: 2},
					{Kind: resolver.MemberKindNode, TargetID: "id-trojan", DisplayName: "Node-Trojan", Position: 3},
					{Kind: resolver.MemberKindNode, TargetID: "id-hy2", DisplayName: "Node-Hysteria2", Position: 4},
					{Kind: resolver.MemberKindNode, TargetID: "id-wg", DisplayName: "Node-WireGuard", Position: 5},
					{Kind: resolver.MemberKindNode, TargetID: "id-tuic", DisplayName: "Node-TUIC", Position: 6},
				},
				Position: 0,
			},
			{
				ID:        groupSelectID,
				Name:      "Proxy",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindGroup, TargetID: groupAutoID, DisplayName: "Auto-Fast", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "id-wg", DisplayName: "Node-WireGuard", Position: 1},
					{Kind: resolver.MemberKindNode, TargetID: "id-tuic", DisplayName: "Node-TUIC", Position: 2},
					{Kind: "", DisplayName: "DIRECT", Position: 3},
				},
				Position: 1,
			},
		},
		Rules: []resolver.ResolvedRule{
			{ID: "r1", TargetGroupID: groupSelectID, TargetGroupName: "Proxy", Expression: "DOMAIN,api.example.com", Position: 0},
			{ID: "r2", TargetGroupID: groupSelectID, TargetGroupName: "Proxy", Expression: "DOMAIN-SUFFIX,google.com", Position: 1},
			{ID: "r3", TargetGroupID: groupSelectID, TargetGroupName: "Proxy", Expression: "DOMAIN-KEYWORD,github", Position: 2},
			{ID: "r4", TargetGroupID: groupSelectID, TargetGroupName: "Proxy", Expression: "IP-CIDR,198.51.100.0/24,no-resolve", Position: 3},
			{ID: "r5", TargetGroupID: groupSelectID, TargetGroupName: "Proxy", Expression: "IP-CIDR6,2001:db8::/32,no-resolve", Position: 4},
			{ID: "r6", TargetGroupID: "", TargetGroupName: "DIRECT", Expression: "GEOIP,CN", Position: 5},
			{ID: "r7", TargetGroupID: "", TargetGroupName: "REJECT", Expression: "GEOSITE,category-ads-all", Position: 6},
			{ID: "r8", TargetGroupID: "", TargetGroupName: "DIRECT", Expression: "SRC-IP-CIDR,192.168.1.0/24", Position: 7},
			{ID: "r9", TargetGroupID: groupAutoID, TargetGroupName: "Auto-Fast", Expression: "SRC-PORT,8080", Position: 8},
			{ID: "r10", TargetGroupID: groupAutoID, TargetGroupName: "Auto-Fast", Expression: "DST-PORT,443", Position: 9},
			{ID: "r11", TargetGroupID: groupAutoID, TargetGroupName: "Auto-Fast", Expression: "PORT,8000-8090", Position: 10},
			{ID: "r12", TargetGroupID: groupSelectID, TargetGroupName: "Proxy", Expression: "PROCESS-NAME,curl", Position: 11},
			{ID: "r13", TargetGroupID: "", TargetGroupName: "REJECT-DROP", Expression: "RULE-SET,ads,https://example.com/rules/ads.srs", Position: 12},
			{ID: "r14", TargetGroupID: groupSelectID, TargetGroupName: "Proxy", Expression: "MATCH", Position: 13, IsTerminal: true},
		},
	}

	res, err := compiler.Compile(ctx, snapshot, domain.TargetSingBox)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	opts := assertValidSingBoxConfig(t, res.Content)

	// WireGuard must be emitted as an Endpoint, not an Outbound
	if len(opts.Endpoints) != 1 || opts.Endpoints[0].Type != "wireguard" || opts.Endpoints[0].Tag != "Node-WireGuard" {
		t.Fatalf("expected 1 wireguard endpoint 'Node-WireGuard', got %#v", opts.Endpoints)
	}
	wgOpts, ok := opts.Endpoints[0].Options.(*option.WireGuardEndpointOptions)
	if !ok {
		t.Fatalf("unexpected wireguard options type: %T", opts.Endpoints[0].Options)
	}
	if wgOpts.MTU != 1400 || len(wgOpts.Address) != 2 || len(wgOpts.Peers) != 1 || len(wgOpts.Peers[0].Reserved) != 3 {
		t.Fatalf("unexpected wireguard endpoint fields: %#v", wgOpts)
	}

	// Verify VLESS Reality, Hysteria2, and TUIC fields in outbounds
	raw := string(res.Content)
	for _, expected := range []string{
		`"flow": "xtls-rprx-vision"`,
		`"public_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"`,
		`"short_id": "0123456789abcdef"`,
		`"up_mbps": 100`,
		`"down_mbps": 500`,
		`"type": "salamander"`,
		`"congestion_control": "bbr"`,
		`"udp_relay_mode": "native"`,
		`"disable_sni": true`,
		`"type": "urltest"`,
		`"type": "selector"`,
		`"final": "Proxy"`,
	} {
		if !strings.Contains(raw, expected) {
			t.Errorf("expected rendered sing-box JSON to contain %s, got:\n%s", expected, raw)
		}
	}
}

func TestSingBoxNegativeCases_FailsClosedWithPreciseDiagnostics(t *testing.T) {
	ctx := context.Background()

	t.Run("MissingCredentialsFailsClosed", func(t *testing.T) {
		snap := fixtureSnapshot()
		snap.Nodes[0].Credentials = domain.InboundProtocolCredential{}
		_, err := compiler.Compile(ctx, snap, domain.TargetSingBox)
		if err == nil {
			t.Fatal("expected compilation without credentials to fail closed")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "nodes[0]" {
			t.Fatalf("expected CapabilityError at nodes[0], got %v", err)
		}
	})

	t.Run("UnsupportedGroupTypesFallbackAndLoadBalanceRejected", func(t *testing.T) {
		for _, gt := range []domain.GroupType{domain.GroupTypeFallback, domain.GroupTypeLoadBalance} {
			snap := fixtureSnapshot()
			snap.Groups[0].GroupType = gt
			_, err := compiler.Compile(ctx, snap, domain.TargetSingBox)
			if err == nil {
				t.Fatalf("expected group type %s to be rejected on sing-box", gt)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) || capErr.Location != "groups[0]" || capErr.Feature != string(gt) {
				t.Fatalf("expected CapabilityError at groups[0] for %s, got %#v", gt, err)
			}
		}
	})

	t.Run("EmptyGroupMembersRejected", func(t *testing.T) {
		snap := fixtureSnapshot()
		snap.Groups[0].Members = nil
		snap.Groups[0].NodeLogicalIDs = nil
		snap.Groups[0].ChildGroupIDs = nil
		_, err := compiler.Compile(ctx, snap, domain.TargetSingBox)
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "groups[0]" {
			t.Fatalf("expected CapabilityError at groups[0] for empty group, got %v", err)
		}
	})

	t.Run("InvalidNodeProtocolOptionsRejected", func(t *testing.T) {
		cases := []struct {
			name     string
			mutate   func(snap *resolver.ResolvedPolicySnapshot)
			wantFeat string
		}{
			{
				name: "UnsupportedSSCipher",
				mutate: func(snap *resolver.ResolvedPolicySnapshot) {
					snap.Nodes[1].Credentials.Method = "unsupported-cipher"
				},
				wantFeat: "ss",
			},
			{
				name: "UnsupportedVMessSecurity",
				mutate: func(snap *resolver.ResolvedPolicySnapshot) {
					snap.Nodes[0].Credentials.Method = "chacha20-invalid"
				},
				wantFeat: "vmess",
			},
			{
				name: "UnsupportedTransportNetwork",
				mutate: func(snap *resolver.ResolvedPolicySnapshot) {
					snap.Nodes[0].Credentials.Transport["network"] = "kcp"
				},
				wantFeat: "vmess",
			},
			{
				name: "UnsupportedVLESSFlow",
				mutate: func(snap *resolver.ResolvedPolicySnapshot) {
					snap.Nodes[0].Protocol = domain.ProtocolVLESS
					snap.Nodes[0].Credentials.Transport = map[string]string{"flow": "xtls-rprx-direct"}
				},
				wantFeat: "vless",
			},
			{
				name: "Hysteria2ObfsMissingPassword",
				mutate: func(snap *resolver.ResolvedPolicySnapshot) {
					snap.Nodes[0].Protocol = domain.ProtocolHysteria2
					snap.Nodes[0].Credentials.Password = "hy2-pass"
					snap.Nodes[0].Credentials.Transport = map[string]string{"obfs": "salamander"}
				},
				wantFeat: "hysteria2",
			},
			{
				name: "Hysteria2InvalidBandwidth",
				mutate: func(snap *resolver.ResolvedPolicySnapshot) {
					snap.Nodes[0].Protocol = domain.ProtocolHysteria2
					snap.Nodes[0].Credentials.Password = "hy2-pass"
					snap.Nodes[0].Credentials.Transport = map[string]string{"up": "invalid-bw"}
				},
				wantFeat: "hysteria2",
			},
			{
				name: "TUICUnsupportedCongestionControl",
				mutate: func(snap *resolver.ResolvedPolicySnapshot) {
					snap.Nodes[0].Protocol = domain.ProtocolTUIC
					snap.Nodes[0].Credentials.Password = "tuic-pass"
					snap.Nodes[0].Credentials.CongestionControl = "reno-invalid"
					snap.Nodes[0].Credentials.Transport = nil
				},
				wantFeat: "tuic",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				snap := fixtureSnapshot()
				tc.mutate(snap)
				_, err := compiler.Compile(ctx, snap, domain.TargetSingBox)
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tc.name)
				}
				var capErr *compiler.CapabilityError
				if !errors.As(err, &capErr) || capErr.Feature != tc.wantFeat {
					t.Fatalf("expected CapabilityError with feature %q, got %#v", tc.wantFeat, err)
				}
			})
		}
	})

	t.Run("InvalidAndUnsupportedRulesRejected", func(t *testing.T) {
		badRules := []struct {
			name     string
			expr     string
			target   string
			wantFeat string
		}{
			{name: "UnsupportedUserAgent", expr: "USER-AGENT,Instagram*", target: "proxy", wantFeat: "USER-AGENT"},
			{name: "InvalidIPCIDR", expr: "IP-CIDR,2001:db8::/32", target: "proxy", wantFeat: "IP-CIDR"},
			{name: "InvalidIPCIDR6", expr: "IP-CIDR6,198.51.100.0/24", target: "proxy", wantFeat: "IP-CIDR6"},
			{name: "InvalidSrcIPCIDR", expr: "SRC-IP-CIDR,not-a-cidr", target: "proxy", wantFeat: "SRC-IP-CIDR"},
			{name: "InvalidPortNumber", expr: "DST-PORT,70000", target: "proxy", wantFeat: "DST-PORT"},
			{name: "InvalidPortRange", expr: "PORT,9000-8000", target: "proxy", wantFeat: "PORT"},
			{name: "InvalidRuleSetURL", expr: "RULE-SET,http://", target: "proxy", wantFeat: "RULE-SET"},
			{name: "UnsupportedBuiltInPass", expr: "DOMAIN,example.com", target: "PASS", wantFeat: "PASS"},
		}

		for _, tc := range badRules {
			t.Run(tc.name, func(t *testing.T) {
				snap := fixtureSnapshot()
				targetID := snap.Groups[0].ID
				if tc.target != "proxy" {
					targetID = ""
				}
				snap.Rules[0] = resolver.ResolvedRule{
					ID:              "bad-rule",
					TargetGroupID:   targetID,
					TargetGroupName: tc.target,
					Expression:      tc.expr,
					Position:        0,
				}
				_, err := compiler.Compile(ctx, snap, domain.TargetSingBox)
				if err == nil {
					t.Fatalf("expected error for %s (%s), got nil", tc.name, tc.expr)
				}
				var capErr *compiler.CapabilityError
				if !errors.As(err, &capErr) || capErr.Location != "rules[0]" || capErr.Feature != tc.wantFeat {
					t.Fatalf("expected CapabilityError at rules[0] feature %q, got %#v", tc.wantFeat, err)
				}
			})
		}
	})
}
