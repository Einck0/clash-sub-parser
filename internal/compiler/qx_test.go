package compiler_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func TestQuantumultXGoldenFixture(t *testing.T) {
	assertTargetGoldenFixture(t, domain.TargetQuantumultX)

	ctx := context.Background()
	res, err := compiler.Compile(ctx, fixtureSnapshot(), domain.TargetQuantumultX, compiler.WithCredentials(fixtureCredentials()))
	if err != nil {
		t.Fatalf("compile QuantumultX failed: %v", err)
	}

	altGoldenPath := filepath.Join("testdata", "golden", "quantumult-x.golden")
	if os.Getenv("UPDATE_GOLDENS") == "1" {
		if writeErr := os.WriteFile(altGoldenPath, res.Content, 0o644); writeErr != nil {
			t.Fatalf("write quantumult-x.golden: %v", writeErr)
		}
	}
	altGolden, err := os.ReadFile(altGoldenPath)
	if err != nil {
		t.Fatalf("read quantumult-x.golden: %v", err)
	}
	if string(res.Content) != string(altGolden) {
		t.Fatalf("output differs from quantumult-x.golden:\nGot:\n%s\nExpected:\n%s", string(res.Content), string(altGolden))
	}

	out := string(res.Content)
	for _, node := range fixtureSnapshot().Nodes {
		if strings.Contains(out, node.LogicalID) {
			t.Fatalf("QuantumultX output must not contain node LogicalID %q:\n%s", node.LogicalID, out)
		}
	}
	if strings.Contains(out, "address=") {
		t.Fatalf("QuantumultX output must not use fake placeholder address= syntax:\n%s", out)
	}

	assertValidQuantumultXConfig(t, out)
}

func TestQuantumultXOutputFormat(t *testing.T) {
	qxRes, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetQuantumultX, compiler.WithCredentials(fixtureCredentials()))
	if err != nil {
		t.Fatalf("compile QuantumultX failed: %v", err)
	}
	qxStr := string(qxRes.Content)
	if !strings.Contains(qxStr, "[general]") || !strings.Contains(qxStr, "[server_local]") ||
		!strings.Contains(qxStr, "[policy]") || !strings.Contains(qxStr, "[filter_local]") {
		t.Fatalf("QuantumultX output missing expected sections: %s", qxStr)
	}
	assertValidQuantumultXConfig(t, qxStr)
}

func TestQuantumultXSupportedProtocolsAndRules(t *testing.T) {
	ctx := context.Background()
	groupID := "018f0b6e-4d7a-7abc-8def-0123456789ab"
	subGroupID := "018f0b6e-4d7a-7abc-8def-0123456789ac"

	snapshot := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "snap-qx-full",
		CompilerVersion: "1.0.0",
		Nodes: []resolver.ResolvedNode{
			{LogicalID: "node-ss-tcp", DisplayName: "SS-TCP", Protocol: domain.ProtocolSS, Active: true, Position: 0},
			{LogicalID: "node-ss-obfs", DisplayName: "SS-Obfs", Protocol: domain.ProtocolSS, Active: true, Position: 1},
			{LogicalID: "node-ss-wss", DisplayName: "SS-WSS", Protocol: domain.ProtocolSS, Active: true, Position: 2},
			{LogicalID: "node-vmess-tls", DisplayName: "VMess-TLS", Protocol: domain.ProtocolVMess, Active: true, Position: 3},
			{LogicalID: "node-vmess-wss", DisplayName: "VMess-WSS", Protocol: domain.ProtocolVMess, Active: true, Position: 4},
			{LogicalID: "node-trojan-tcp", DisplayName: "Trojan-TCP", Protocol: domain.ProtocolTrojan, Active: true, Position: 5},
			{LogicalID: "node-trojan-wss", DisplayName: "Trojan-WSS", Protocol: domain.ProtocolTrojan, Active: true, Position: 6},
		},
		Groups: []resolver.ResolvedGroup{
			{
				ID:        subGroupID,
				Name:      "HK-Nodes",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "node-ss-tcp", DisplayName: "SS-TCP", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "node-ss-obfs", DisplayName: "SS-Obfs", Position: 1},
					{Kind: resolver.MemberKindNode, TargetID: "node-ss-wss", DisplayName: "SS-WSS", Position: 2},
				},
				Position: 0,
			},
			{
				ID:        groupID,
				Name:      "Proxy",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindGroup, TargetID: subGroupID, DisplayName: "HK-Nodes", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "node-vmess-tls", DisplayName: "VMess-TLS", Position: 1},
					{Kind: resolver.MemberKindNode, TargetID: "node-vmess-wss", DisplayName: "VMess-WSS", Position: 2},
					{Kind: resolver.MemberKindNode, TargetID: "node-trojan-tcp", DisplayName: "Trojan-TCP", Position: 3},
					{Kind: resolver.MemberKindNode, TargetID: "node-trojan-wss", DisplayName: "Trojan-WSS", Position: 4},
					{Kind: "", DisplayName: "DIRECT", Position: 5},
				},
				Position: 1,
			},
		},
		Rules: []resolver.ResolvedRule{
			{ID: "r1", TargetGroupID: groupID, TargetGroupName: "Proxy", Expression: "DOMAIN,api.example.com", Position: 0},
			{ID: "r2", TargetGroupID: groupID, TargetGroupName: "Proxy", Expression: "DOMAIN-SUFFIX,example.com", Position: 1},
			{ID: "r3", TargetGroupID: groupID, TargetGroupName: "Proxy", Expression: "DOMAIN-KEYWORD,google", Position: 2},
			{ID: "r4", TargetGroupID: "", TargetGroupName: "DIRECT", Expression: "IP-CIDR,198.51.100.0/24", Position: 3},
			{ID: "r5", TargetGroupID: "", TargetGroupName: "REJECT", Expression: "IP-CIDR6,2001:db8::/32", Position: 4},
			{ID: "r6", TargetGroupID: "", TargetGroupName: "DIRECT", Expression: "GEOIP,CN", Position: 5},
			{ID: "r7", TargetGroupID: groupID, TargetGroupName: "Proxy", Expression: "MATCH", Position: 6, IsTerminal: true},
		},
	}

	creds := map[string]*domain.NodeCredentialPayload{
		"node-ss-tcp": {
			LogicalID: "node-ss-tcp",
			Protocol:  domain.ProtocolSS,
			Server:    "198.51.100.10",
			Port:      8388,
			Version:   1,
			Credentials: domain.InboundProtocolCredential{
				Method:   "2022-blake3-aes-128-gcm",
				Password: "ss-tcp-password",
			},
		},
		"node-ss-obfs": {
			LogicalID: "node-ss-obfs",
			Protocol:  domain.ProtocolSS,
			Server:    "198.51.100.11",
			Port:      8080,
			Version:   1,
			Credentials: domain.InboundProtocolCredential{
				Method:   "chacha20-ietf-poly1305",
				Password: "ss-obfs-password",
				Transport: map[string]string{
					"network": "tcp",
					"obfs":    "http",
					"host":    "cdn.example.com",
					"path":    "/download",
				},
			},
		},
		"node-ss-wss": {
			LogicalID: "node-ss-wss",
			Protocol:  domain.ProtocolSS,
			Server:    "198.51.100.12",
			Port:      8443,
			Version:   1,
			Credentials: domain.InboundProtocolCredential{
				Method:   "aes-256-gcm",
				Password: "ss-wss-password",
				Transport: map[string]string{
					"network":          "ws",
					"tls":              "true",
					"host":             "ws.example.com",
					"sni":              "sni.example.com",
					"path":             "/ss-ws",
					"skip_cert_verify": "true",
				},
			},
		},
		"node-vmess-tls": {
			LogicalID: "node-vmess-tls",
			Protocol:  domain.ProtocolVMess,
			Server:    "198.51.100.20",
			Port:      443,
			Version:   1,
			Credentials: domain.InboundProtocolCredential{
				UUID:   "11111111-2222-3333-4444-555555555555",
				Method: "aes-128-gcm",
				Transport: map[string]string{
					"network":          "tcp",
					"tls":              "true",
					"sni":              "vmess-tls.example.com",
					"skip_cert_verify": "true",
				},
			},
		},
		"node-vmess-wss": {
			LogicalID: "node-vmess-wss",
			Protocol:  domain.ProtocolVMess,
			Server:    "2001:db8::21",
			Port:      2053,
			Version:   1,
			Credentials: domain.InboundProtocolCredential{
				UUID:   "66666666-7777-8888-9999-000000000000",
				Method: "chacha20-poly1305",
				Transport: map[string]string{
					"network": "ws",
					"tls":     "true",
					"host":    "vmess-ws.example.com",
					"path":    "/ws-path",
				},
			},
		},
		"node-trojan-tcp": {
			LogicalID: "node-trojan-tcp",
			Protocol:  domain.ProtocolTrojan,
			Server:    "trojan.example.com",
			Port:      443,
			Version:   1,
			Credentials: domain.InboundProtocolCredential{
				Password: "trojan-tcp-password",
				Transport: map[string]string{
					"network": "tcp",
					"tls":     "true",
					"sni":     "trojan.example.com",
					"alpn":    "h2,http/1.1",
				},
			},
		},
		"node-trojan-wss": {
			LogicalID: "node-trojan-wss",
			Protocol:  domain.ProtocolTrojan,
			Server:    "198.51.100.31",
			Port:      9443,
			Version:   1,
			Credentials: domain.InboundProtocolCredential{
				Password: "trojan-wss-password",
				Transport: map[string]string{
					"network":          "ws",
					"tls":              "true",
					"host":             "trojan-cdn.example.com",
					"sni":              "trojan-sni.example.com",
					"path":             "/trojan-ws",
					"skip_cert_verify": "true",
				},
			},
		},
	}

	res, err := compiler.Compile(ctx, snapshot, domain.TargetQuantumultX, compiler.WithCredentials(creds))
	if err != nil {
		t.Fatalf("expected QuantumultX full supported compile to succeed, got: %v", err)
	}

	out := string(res.Content)
	assertValidQuantumultXConfig(t, out)

	expectedLines := []string{
		"shadowsocks = 198.51.100.10:8388, method=2022-blake3-aes-128-gcm, password=ss-tcp-password, tag=SS-TCP",
		"shadowsocks = 198.51.100.11:8080, method=chacha20-ietf-poly1305, password=ss-obfs-password, obfs=http, obfs-host=cdn.example.com, obfs-uri=/download, tag=SS-Obfs",
		"shadowsocks = 198.51.100.12:8443, method=aes-256-gcm, password=ss-wss-password, obfs=wss, obfs-host=ws.example.com, obfs-uri=/ss-ws, tls-host=sni.example.com, tls-verification=false, tag=SS-WSS",
		"vmess = 198.51.100.20:443, method=aes-128-gcm, password=11111111-2222-3333-4444-555555555555, obfs=over-tls, tls-host=vmess-tls.example.com, tls-verification=false, tag=VMess-TLS",
		"vmess = [2001:db8::21]:2053, method=chacha20-poly1305, password=66666666-7777-8888-9999-000000000000, obfs=wss, obfs-host=vmess-ws.example.com, obfs-uri=/ws-path, tls-host=vmess-ws.example.com, tag=VMess-WSS",
		"trojan = trojan.example.com:443, password=trojan-tcp-password, over-tls=true, tls-host=trojan.example.com, tls-verification=true, tag=Trojan-TCP",
		"trojan = 198.51.100.31:9443, password=trojan-wss-password, obfs=wss, obfs-host=trojan-cdn.example.com, obfs-uri=/trojan-ws, tls-host=trojan-sni.example.com, tls-verification=false, tag=Trojan-WSS",
		"static = HK-Nodes, SS-TCP, SS-Obfs, SS-WSS",
		"static = Proxy, HK-Nodes, VMess-TLS, VMess-WSS, Trojan-TCP, Trojan-WSS, DIRECT",
		"host, api.example.com, Proxy",
		"host-suffix, example.com, Proxy",
		"host-keyword, google, Proxy",
		"ip-cidr, 198.51.100.0/24, DIRECT",
		"ip6-cidr, 2001:db8::/32, REJECT",
		"geoip, CN, DIRECT",
		"final, Proxy",
	}
	for _, want := range expectedLines {
		if !strings.Contains(out, want) {
			t.Errorf("QuantumultX output missing expected line %q.\nFull output:\n%s", want, out)
		}
	}
}

func TestQuantumultXRejectsProcessNameRule(t *testing.T) {
	snapshot := fixtureSnapshot()
	snapshot.Rules = append(snapshot.Rules, resolver.ResolvedRule{
		ID:              "rule-process-name",
		TargetGroupID:   snapshot.Groups[0].ID,
		TargetGroupName: snapshot.Groups[0].Name,
		Expression:      "PROCESS-NAME,curl",
		Position:        2,
	})

	_, err := compiler.Compile(context.Background(), snapshot, domain.TargetQuantumultX, compiler.WithCredentials(fixtureCredentials()))
	if err == nil {
		t.Fatal("expected Quantumult-X to reject PROCESS-NAME")
	}
	var capErr *compiler.CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected CapabilityError, got %v", err)
	}
	if capErr.Feature != "PROCESS-NAME" || capErr.Location != "rules[2]" {
		t.Fatalf("expected feature PROCESS-NAME at rules[2], got %#v", capErr)
	}
}

func TestQuantumultXRejectsUnsupportedCapabilitiesAndTransports(t *testing.T) {
	ctx := context.Background()

	t.Run("RejectsUnsupportedProtocols_VLESS_Hy2_WG_TUIC", func(t *testing.T) {
		unsupported := []domain.Protocol{
			domain.ProtocolVLESS,
			domain.ProtocolHysteria2,
			domain.ProtocolWireGuard,
			domain.ProtocolTUIC,
		}
		for _, proto := range unsupported {
			snap := fixtureSnapshot()
			snap.Nodes[0].Protocol = proto
			_, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX, compiler.WithCredentials(fixtureCredentials()))
			if err == nil {
				t.Fatalf("expected QuantumultX to reject protocol %s", proto)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("expected CapabilityError for %s, got %T: %v", proto, err, err)
			}
			if capErr.Target != domain.TargetQuantumultX || capErr.Location != "nodes[0]" || capErr.Feature != string(proto) {
				t.Fatalf("unexpected CapabilityError for %s: %#v", proto, capErr)
			}
		}
	})

	t.Run("RejectsUnsupportedGroupTypesWithoutSilentStaticDowngrade", func(t *testing.T) {
		unsupportedGroups := []domain.GroupType{
			domain.GroupTypeURLTest,
			domain.GroupTypeFallback,
			domain.GroupTypeLoadBalance,
		}
		for _, gt := range unsupportedGroups {
			snap := fixtureSnapshot()
			snap.Groups[0].GroupType = gt
			_, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX, compiler.WithCredentials(fixtureCredentials()))
			if err == nil {
				t.Fatalf("expected QuantumultX to reject group type %s", gt)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("expected CapabilityError for %s, got %T: %v", gt, err, err)
			}
			if capErr.Target != domain.TargetQuantumultX || capErr.Location != "groups[0]" || capErr.Feature != string(gt) {
				t.Fatalf("unexpected CapabilityError for %s: %#v", gt, capErr)
			}
		}
	})

	t.Run("RejectsUnsupportedRulesAndInvalidCIDRs", func(t *testing.T) {
		cases := []struct {
			name        string
			expr        string
			targetGroup string
			wantFeature string
		}{
			{name: "GEOSITE", expr: "GEOSITE,category-ads-all", targetGroup: "proxy", wantFeature: "GEOSITE"},
			{name: "RULE-SET", expr: "RULE-SET,apple", targetGroup: "proxy", wantFeature: "RULE-SET"},
			{name: "SRC-IP-CIDR", expr: "SRC-IP-CIDR,10.0.0.0/8", targetGroup: "proxy", wantFeature: "SRC-IP-CIDR"},
			{name: "DST-PORT", expr: "DST-PORT,443", targetGroup: "proxy", wantFeature: "DST-PORT"},
			{name: "PORT", expr: "PORT,80", targetGroup: "proxy", wantFeature: "PORT"},
			{name: "InvalidIPv4CIDR", expr: "IP-CIDR,2001:db8::/32", targetGroup: "proxy", wantFeature: "IP-CIDR"},
			{name: "InvalidIPv6CIDR", expr: "IP-CIDR6,198.51.100.0/24", targetGroup: "proxy", wantFeature: "IP-CIDR6"},
			{name: "ExtraRuleModifier", expr: "DOMAIN-SUFFIX,example.com,no-resolve", targetGroup: "proxy", wantFeature: "DOMAIN-SUFFIX"},
			{name: "UnsupportedBuiltInTargetPass", expr: "DOMAIN,example.com", targetGroup: "PASS", wantFeature: "PASS"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				snap := fixtureSnapshot()
				targetID := snap.Groups[0].ID
				if tc.targetGroup != "proxy" {
					targetID = ""
				}
				snap.Rules = []resolver.ResolvedRule{
					{ID: "r-test", TargetGroupID: targetID, TargetGroupName: tc.targetGroup, Expression: tc.expr, Position: 0},
				}
				_, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX, compiler.WithCredentials(fixtureCredentials()))
				if err == nil {
					t.Fatalf("expected error for %s (%q)", tc.name, tc.expr)
				}
				var capErr *compiler.CapabilityError
				if !errors.As(err, &capErr) {
					t.Fatalf("expected CapabilityError for %s, got %T: %v", tc.name, err, err)
				}
				if capErr.Location != "rules[0]" || capErr.Feature != tc.wantFeature {
					t.Fatalf("unexpected CapabilityError for %s: %#v", tc.name, capErr)
				}
			})
		}

		t.Run("NonTerminalMatchRuleRejected", func(t *testing.T) {
			snap := fixtureSnapshot()
			snap.Rules = []resolver.ResolvedRule{
				{ID: "r-match-first", TargetGroupID: snap.Groups[0].ID, TargetGroupName: "proxy", Expression: "MATCH", Position: 0, IsTerminal: true},
				{ID: "r-after-match", TargetGroupID: snap.Groups[0].ID, TargetGroupName: "proxy", Expression: "DOMAIN,example.com", Position: 1},
			}
			_, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX, compiler.WithCredentials(fixtureCredentials()))
			if err == nil {
				t.Fatal("expected non-terminal MATCH rule to be rejected")
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) || capErr.Location != "rules[0]" || capErr.Feature != "MATCH" {
				t.Fatalf("expected CapabilityError at rules[0]/MATCH, got %#v (%v)", capErr, err)
			}
		})
	})

	t.Run("RejectsMissingCredentialsAndUnsupportedTransportsWithoutSecretLeak", func(t *testing.T) {
		// 1. Missing credentials on non-empty snapshot must fail closed
		_, err := compiler.Compile(ctx, fixtureSnapshot(), domain.TargetQuantumultX)
		if err == nil {
			t.Fatal("expected Compile without credentials to fail closed on non-empty snapshot")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "nodes[0]" {
			t.Fatalf("expected nodes[0] CapabilityError for nil credentials, got %v", err)
		}

		// 2. Unsupported transports / ciphers / injection characters must fail without leaking secrets
		type transportCase struct {
			name         string
			nodeIndex    int
			mutate       func(map[string]*domain.NodeCredentialPayload)
			secretMarker string
			wantFeature  string
		}

		tCases := []transportCase{
			{
				name:      "VMess_GRPC_Unsupported",
				nodeIndex: 0,
				mutate: func(c map[string]*domain.NodeCredentialPayload) {
					c["0123456789abcdef0123456789abcdef"].Credentials.UUID = "SECRET-VMESS-UUID-101"
					c["0123456789abcdef0123456789abcdef"].Credentials.Transport = map[string]string{
						"network": "grpc",
					}
				},
				secretMarker: "SECRET-VMESS-UUID-101",
				wantFeature:  "vmess",
			},
			{
				name:      "SS_GRPC_Unsupported",
				nodeIndex: 1,
				mutate: func(c map[string]*domain.NodeCredentialPayload) {
					c["abcdef0123456789abcdef0123456789"].Credentials.Password = "SECRET-SS-PASS-202"
					c["abcdef0123456789abcdef0123456789"].Credentials.Transport = map[string]string{
						"network": "grpc",
					}
				},
				secretMarker: "SECRET-SS-PASS-202",
				wantFeature:  "ss",
			},
			{
				name:      "SS_UnsupportedCipher",
				nodeIndex: 1,
				mutate: func(c map[string]*domain.NodeCredentialPayload) {
					c["abcdef0123456789abcdef0123456789"].Credentials.Method = "SECRET-UNKNOWN-CIPHER-303"
					c["abcdef0123456789abcdef0123456789"].Credentials.Password = "SECRET-SS-PASS-303"
				},
				secretMarker: "SECRET-UNKNOWN-CIPHER-303",
				wantFeature:  "ss",
			},
			{
				name:      "PasswordContainsCommaInjection",
				nodeIndex: 1,
				mutate: func(c map[string]*domain.NodeCredentialPayload) {
					c["abcdef0123456789abcdef0123456789"].Credentials.Password = "SECRET-INJECT-404, over-tls=false"
				},
				secretMarker: "SECRET-INJECT-404",
				wantFeature:  "ss",
			},
			{
				name:      "PlaceholderLogicalIDServerAddress",
				nodeIndex: 0,
				mutate: func(c map[string]*domain.NodeCredentialPayload) {
					c["0123456789abcdef0123456789abcdef"].Server = "0123456789abcdef0123456789abcdef"
					c["0123456789abcdef0123456789abcdef"].Credentials.UUID = "SECRET-VMESS-UUID-505"
				},
				secretMarker: "SECRET-VMESS-UUID-505",
				wantFeature:  "vmess",
			},
		}

		for _, tc := range tCases {
			t.Run(tc.name, func(t *testing.T) {
				creds := fixtureCredentials()
				tc.mutate(creds)
				_, err := compiler.Compile(ctx, fixtureSnapshot(), domain.TargetQuantumultX, compiler.WithCredentials(creds))
				if err == nil {
					t.Fatalf("expected error for %s", tc.name)
				}
				var ce *compiler.CapabilityError
				if !errors.As(err, &ce) {
					t.Fatalf("expected CapabilityError for %s, got %T: %v", tc.name, err, err)
				}
				wantLoc := "nodes[" + strconv.Itoa(tc.nodeIndex) + "]"
				if ce.Location != wantLoc || ce.Feature != tc.wantFeature {
					t.Fatalf("expected %s/%s, got %#v", wantLoc, tc.wantFeature, ce)
				}
				if strings.Contains(err.Error(), tc.secretMarker) {
					t.Fatalf("error leaked secret marker %q: %v", tc.secretMarker, err)
				}
			})
		}
	})
}

// assertValidQuantumultXConfig validates rendered Quantumult X configuration against
// Cross Utility's official Quantumult X sample.conf grammar for [general], [server_local],
// [policy], and [filter_local].
func assertValidQuantumultXConfig(t *testing.T, content string) {
	t.Helper()

	var currentSection string
	seenSections := make(map[string]bool)
	definedTags := make(map[string]bool)
	definedPolicies := make(map[string]bool)
	seenFinalRule := false

	type policyEntry struct {
		name       string
		candidates []string
	}
	var policies []policyEntry

	lines := strings.Split(content, "\n")
	for lineNo, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line
			seenSections[currentSection] = true
			continue
		}

		switch currentSection {
		case "[general]":
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				t.Fatalf("line %d: invalid [general] entry: %q", lineNo+1, line)
			}

		case "[server_local]":
			eqIdx := strings.IndexByte(line, '=')
			if eqIdx <= 0 {
				t.Fatalf("line %d: [server_local] entry missing '=': %q", lineNo+1, line)
			}
			proto := strings.TrimSpace(line[:eqIdx])
			if proto != "shadowsocks" && proto != "vmess" && proto != "trojan" {
				t.Fatalf("line %d: unsupported [server_local] protocol %q in %q", lineNo+1, proto, line)
			}

			fields := strings.Split(line[eqIdx+1:], ",")
			if len(fields) < 3 {
				t.Fatalf("line %d: [server_local] entry has too few fields: %q", lineNo+1, line)
			}
			endpoint := strings.TrimSpace(fields[0])
			host, portStr, err := net.SplitHostPort(endpoint)
			if err != nil || host == "" {
				t.Fatalf("line %d: invalid [server_local] host:port %q: %v", lineNo+1, endpoint, err)
			}
			port, err := strconv.Atoi(portStr)
			if err != nil || port < 1 || port > 65535 {
				t.Fatalf("line %d: invalid [server_local] port %q", lineNo+1, portStr)
			}

			kv := make(map[string]string, len(fields)-1)
			for _, rawField := range fields[1:] {
				kvParts := strings.SplitN(strings.TrimSpace(rawField), "=", 2)
				if len(kvParts) != 2 || strings.TrimSpace(kvParts[0]) == "" || strings.TrimSpace(kvParts[1]) == "" {
					t.Fatalf("line %d: invalid [server_local] parameter %q in %q", lineNo+1, rawField, line)
				}
				kv[strings.TrimSpace(kvParts[0])] = strings.TrimSpace(kvParts[1])
			}

			tag := kv["tag"]
			if tag == "" {
				t.Fatalf("line %d: [server_local] entry missing tag: %q", lineNo+1, line)
			}
			if definedTags[tag] {
				t.Fatalf("line %d: duplicate [server_local] tag %q", lineNo+1, tag)
			}
			definedTags[tag] = true

			switch proto {
			case "shadowsocks", "vmess":
				if kv["method"] == "" || kv["password"] == "" {
					t.Fatalf("line %d: %s entry missing method or password: %q", lineNo+1, proto, line)
				}
			case "trojan":
				if kv["password"] == "" {
					t.Fatalf("line %d: trojan entry missing password: %q", lineNo+1, line)
				}
				if kv["over-tls"] != "true" && kv["obfs"] != "wss" {
					t.Fatalf("line %d: trojan entry must specify over-tls=true or obfs=wss: %q", lineNo+1, line)
				}
			}

		case "[policy]":
			eqIdx := strings.IndexByte(line, '=')
			if eqIdx <= 0 {
				t.Fatalf("line %d: [policy] entry missing '=': %q", lineNo+1, line)
			}
			policyType := strings.TrimSpace(line[:eqIdx])
			if policyType != "static" {
				t.Fatalf("line %d: unexpected [policy] type %q in %q", lineNo+1, policyType, line)
			}
			fields := strings.Split(line[eqIdx+1:], ",")
			if len(fields) < 2 {
				t.Fatalf("line %d: [policy] entry must contain group name and at least one candidate: %q", lineNo+1, line)
			}
			groupName := strings.TrimSpace(fields[0])
			if groupName == "" {
				t.Fatalf("line %d: empty [policy] group name in %q", lineNo+1, line)
			}
			definedPolicies[groupName] = true
			candidates := make([]string, 0, len(fields)-1)
			for _, c := range fields[1:] {
				candidates = append(candidates, strings.TrimSpace(c))
			}
			policies = append(policies, policyEntry{name: groupName, candidates: candidates})

		case "[filter_local]":
			if seenFinalRule {
				t.Fatalf("line %d: rule %q appears after terminal final rule", lineNo+1, line)
			}
			fields := strings.Split(line, ",")
			kind := strings.TrimSpace(fields[0])
			switch kind {
			case "final":
				if len(fields) != 2 {
					t.Fatalf("line %d: final rule must have 2 fields: %q", lineNo+1, line)
				}
				target := strings.TrimSpace(fields[1])
				if !definedPolicies[target] && target != "DIRECT" && target != "REJECT" {
					t.Fatalf("line %d: final rule references unknown policy %q", lineNo+1, target)
				}
				seenFinalRule = true
			case "host", "host-suffix", "host-keyword", "geoip", "ip-cidr", "ip6-cidr":
				if len(fields) != 3 {
					t.Fatalf("line %d: %s rule must have 3 fields: %q", lineNo+1, kind, line)
				}
				val := strings.TrimSpace(fields[1])
				target := strings.TrimSpace(fields[2])
				if val == "" {
					t.Fatalf("line %d: empty value in %s rule: %q", lineNo+1, kind, line)
				}
				if kind == "ip-cidr" {
					if p, err := netip.ParsePrefix(val); err != nil || !p.Addr().Is4() {
						t.Fatalf("line %d: invalid IPv4 CIDR %q", lineNo+1, val)
					}
				}
				if kind == "ip6-cidr" {
					if p, err := netip.ParsePrefix(val); err != nil || !p.Addr().Is6() {
						t.Fatalf("line %d: invalid IPv6 CIDR %q", lineNo+1, val)
					}
				}
				if !definedPolicies[target] && target != "DIRECT" && target != "REJECT" {
					t.Fatalf("line %d: %s rule references unknown policy %q", lineNo+1, kind, target)
				}
			default:
				t.Fatalf("line %d: unsupported [filter_local] rule kind %q in %q", lineNo+1, kind, line)
			}
		}
	}

	for _, reqSection := range []string{"[general]", "[server_local]", "[policy]", "[filter_local]"} {
		if !seenSections[reqSection] {
			t.Fatalf("missing required Quantumult X section %s", reqSection)
		}
	}

	for _, p := range policies {
		for _, cand := range p.candidates {
			if !definedTags[cand] && !definedPolicies[cand] && cand != "DIRECT" && cand != "REJECT" {
				t.Fatalf("policy %q references undefined candidate %q", p.name, cand)
			}
		}
	}
}
