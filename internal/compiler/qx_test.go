package compiler_test

import (
	"context"
	"errors"
	"net"
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
	res, err := compiler.Compile(ctx, fixtureSnapshot(), domain.TargetQuantumultX)
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
	qxRes, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetQuantumultX)
	if err != nil {
		t.Fatalf("compile QuantumultX failed: %v", err)
	}
	qxStr := string(qxRes.Content)
	for _, unexpectedSection := range []string{"[general]", "[server_local]", "[policy]", "[filter_local]"} {
		if strings.Contains(qxStr, unexpectedSection) {
			t.Fatalf("node-only QuantumultX output must not contain section %s:\n%s", unexpectedSection, qxStr)
		}
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
			{
				LogicalID:   "node-ss-tcp",
				DisplayName: "SS-TCP",
				Protocol:    domain.ProtocolSS,
				Server:      "198.51.100.10",
				Port:        8388,
				Credentials: domain.InboundProtocolCredential{
					Method:   "2022-blake3-aes-128-gcm",
					Password: "ss-tcp-password",
				},
				Active:   true,
				Position: 0,
			},
			{
				LogicalID:   "node-ss-obfs",
				DisplayName: "SS-Obfs",
				Protocol:    domain.ProtocolSS,
				Server:      "198.51.100.11",
				Port:        8080,
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
				Active:   true,
				Position: 1,
			},
			{
				LogicalID:   "node-ss-wss",
				DisplayName: "SS-WSS",
				Protocol:    domain.ProtocolSS,
				Server:      "198.51.100.12",
				Port:        8443,
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
				Active:   true,
				Position: 2,
			},
			{
				LogicalID:   "node-vmess-tls",
				DisplayName: "VMess-TLS",
				Protocol:    domain.ProtocolVMess,
				Server:      "198.51.100.20",
				Port:        443,
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
				Active:   true,
				Position: 3,
			},
			{
				LogicalID:   "node-vmess-wss",
				DisplayName: "VMess-WSS",
				Protocol:    domain.ProtocolVMess,
				Server:      "2001:db8::21",
				Port:        2053,
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
				Active:   true,
				Position: 4,
			},
			{
				LogicalID:   "node-trojan-tcp",
				DisplayName: "Trojan-TCP",
				Protocol:    domain.ProtocolTrojan,
				Server:      "trojan.example.com",
				Port:        443,
				Credentials: domain.InboundProtocolCredential{
					Password: "trojan-tcp-password",
					Transport: map[string]string{
						"network": "tcp",
						"tls":     "true",
						"sni":     "trojan.example.com",
						"alpn":    "h2,http/1.1",
					},
				},
				Active:   true,
				Position: 5,
			},
			{
				LogicalID:   "node-trojan-wss",
				DisplayName: "Trojan-WSS",
				Protocol:    domain.ProtocolTrojan,
				Server:      "198.51.100.31",
				Port:        9443,
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
				Active:   true,
				Position: 6,
			},
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

	res, err := compiler.Compile(ctx, snapshot, domain.TargetQuantumultX)
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
	}
	for _, want := range expectedLines {
		if !strings.Contains(out, want) {
			t.Errorf("QuantumultX output missing expected line %q.\nFull output:\n%s", want, out)
		}
	}
	for _, unexpected := range []string{"static =", "host,", "host-suffix,", "final,"} {
		if strings.Contains(out, unexpected) {
			t.Errorf("node-only QuantumultX output must not contain policy/rule line %q.\nFull output:\n%s", unexpected, out)
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

	res, err := compiler.Compile(context.Background(), snapshot, domain.TargetQuantumultX)
	if err != nil {
		t.Fatalf("expected Quantumult-X node-only export to ignore PROCESS-NAME rule cleanly: %v", err)
	}
	if strings.Contains(string(res.Content), "PROCESS-NAME") {
		t.Fatalf("expected PROCESS-NAME rule to be omitted in Quantumult-X output:\n%s", string(res.Content))
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
			_, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX)
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

	t.Run("IgnoresAllGroupTypesInNodeOnlyExport", func(t *testing.T) {
		groupTypes := []domain.GroupType{
			domain.GroupTypeURLTest,
			domain.GroupTypeFallback,
			domain.GroupTypeLoadBalance,
		}
		for _, gt := range groupTypes {
			snap := fixtureSnapshot()
			snap.Groups[0].GroupType = gt
			res, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX)
			if err != nil {
				t.Fatalf("expected QuantumultX node-only export to ignore group type %s, got %v", gt, err)
			}
			assertValidQuantumultXConfig(t, string(res.Content))
		}
	})

	t.Run("IgnoresRulesInNodeOnlyExport", func(t *testing.T) {
		cases := []struct {
			name        string
			expr        string
			targetGroup string
		}{
			{name: "GEOSITE", expr: "GEOSITE,category-ads-all", targetGroup: "proxy"},
			{name: "RULE-SET", expr: "RULE-SET,apple", targetGroup: "proxy"},
			{name: "SRC-IP-CIDR", expr: "SRC-IP-CIDR,10.0.0.0/8", targetGroup: "proxy"},
			{name: "DST-PORT", expr: "DST-PORT,443", targetGroup: "proxy"},
			{name: "PORT", expr: "PORT,80", targetGroup: "proxy"},
			{name: "InvalidIPv4CIDR", expr: "IP-CIDR,2001:db8::/32", targetGroup: "proxy"},
			{name: "InvalidIPv6CIDR", expr: "IP-CIDR6,198.51.100.0/24", targetGroup: "proxy"},
			{name: "ExtraRuleModifier", expr: "DOMAIN-SUFFIX,example.com,no-resolve", targetGroup: "proxy"},
			{name: "UnsupportedBuiltInTargetPass", expr: "DOMAIN,example.com", targetGroup: "PASS"},
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
				res, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX)
				if err != nil {
					t.Fatalf("expected rule %s (%q) to be ignored in node-only QuantumultX export, got %v", tc.name, tc.expr, err)
				}
				assertValidQuantumultXConfig(t, string(res.Content))
			})
		}

		t.Run("NonTerminalMatchRuleIgnored", func(t *testing.T) {
			snap := fixtureSnapshot()
			snap.Rules = []resolver.ResolvedRule{
				{ID: "r-match-first", TargetGroupID: snap.Groups[0].ID, TargetGroupName: "proxy", Expression: "MATCH", Position: 0, IsTerminal: true},
				{ID: "r-after-match", TargetGroupID: snap.Groups[0].ID, TargetGroupName: "proxy", Expression: "DOMAIN,example.com", Position: 1},
			}
			res, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX)
			if err != nil {
				t.Fatalf("expected non-terminal MATCH rule to be ignored in node-only QuantumultX export, got %v", err)
			}
			assertValidQuantumultXConfig(t, string(res.Content))
		})
	})

	t.Run("RejectsMissingCredentialsAndUnsupportedTransportsWithoutSecretLeak", func(t *testing.T) {
		// 1. Missing credentials on non-empty snapshot must fail closed
		snapMissing := fixtureSnapshot()
		snapMissing.Nodes[0].Credentials = domain.InboundProtocolCredential{}
		_, err := compiler.Compile(ctx, snapMissing, domain.TargetQuantumultX)
		if err == nil {
			t.Fatal("expected Compile without credentials to fail closed on non-empty snapshot")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "nodes[0]" {
			t.Fatalf("expected nodes[0] CapabilityError for empty credentials, got %v", err)
		}

		// 2. Unsupported transports / ciphers / injection characters must fail without leaking secrets
		type transportCase struct {
			name         string
			nodeIndex    int
			mutate       func(*resolver.ResolvedPolicySnapshot)
			secretMarker string
			wantFeature  string
		}

		tCases := []transportCase{
			{
				name:      "VMess_GRPC_Unsupported",
				nodeIndex: 0,
				mutate: func(s *resolver.ResolvedPolicySnapshot) {
					s.Nodes[0].Credentials.UUID = "SECRET-VMESS-UUID-101"
					s.Nodes[0].Credentials.Transport = map[string]string{
						"network": "grpc",
					}
				},
				secretMarker: "SECRET-VMESS-UUID-101",
				wantFeature:  "vmess",
			},
			{
				name:      "SS_GRPC_Unsupported",
				nodeIndex: 1,
				mutate: func(s *resolver.ResolvedPolicySnapshot) {
					s.Nodes[1].Credentials.Password = "SECRET-SS-PASS-202"
					s.Nodes[1].Credentials.Transport = map[string]string{
						"network": "grpc",
					}
				},
				secretMarker: "SECRET-SS-PASS-202",
				wantFeature:  "ss",
			},
			{
				name:      "SS_UnsupportedCipher",
				nodeIndex: 1,
				mutate: func(s *resolver.ResolvedPolicySnapshot) {
					s.Nodes[1].Credentials.Method = "SECRET-UNKNOWN-CIPHER-303"
					s.Nodes[1].Credentials.Password = "SECRET-SS-PASS-303"
				},
				secretMarker: "SECRET-UNKNOWN-CIPHER-303",
				wantFeature:  "ss",
			},
			{
				name:      "PasswordContainsCommaInjection",
				nodeIndex: 1,
				mutate: func(s *resolver.ResolvedPolicySnapshot) {
					s.Nodes[1].Credentials.Password = "SECRET-INJECT-404, over-tls=false"
				},
				secretMarker: "SECRET-INJECT-404",
				wantFeature:  "ss",
			},
			{
				name:      "PlaceholderLogicalIDServerAddress",
				nodeIndex: 0,
				mutate: func(s *resolver.ResolvedPolicySnapshot) {
					s.Nodes[0].Server = "0123456789abcdef0123456789abcdef"
					s.Nodes[0].Credentials.UUID = "SECRET-VMESS-UUID-505"
				},
				secretMarker: "SECRET-VMESS-UUID-505",
				wantFeature:  "vmess",
			},
		}

		for _, tc := range tCases {
			t.Run(tc.name, func(t *testing.T) {
				snap := fixtureSnapshot()
				tc.mutate(snap)
				_, err := compiler.Compile(ctx, snap, domain.TargetQuantumultX)
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

// assertValidQuantumultXConfig validates rendered Quantumult X node-only lines against
// Cross Utility's official Quantumult X server_local grammar.
func assertValidQuantumultXConfig(t *testing.T, content string) {
	t.Helper()

	definedTags := make(map[string]bool)

	lines := strings.Split(content, "\n")
	for lineNo, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			t.Fatalf("line %d: unexpected INI section %q in node-only Quantumult X output", lineNo+1, line)
		}

		eqIdx := strings.IndexByte(line, '=')
		if eqIdx <= 0 {
			t.Fatalf("line %d: node entry missing '=': %q", lineNo+1, line)
		}
		proto := strings.TrimSpace(line[:eqIdx])
		if proto != "shadowsocks" && proto != "vmess" && proto != "trojan" {
			t.Fatalf("line %d: unsupported Quantumult X protocol %q in %q", lineNo+1, proto, line)
		}

		fields := strings.Split(line[eqIdx+1:], ",")
		if len(fields) < 3 {
			t.Fatalf("line %d: node entry has too few fields: %q", lineNo+1, line)
		}
		endpoint := strings.TrimSpace(fields[0])
		host, portStr, err := net.SplitHostPort(endpoint)
		if err != nil || host == "" {
			t.Fatalf("line %d: invalid host:port %q: %v", lineNo+1, endpoint, err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			t.Fatalf("line %d: invalid port %q", lineNo+1, portStr)
		}

		kv := make(map[string]string, len(fields)-1)
		for _, rawField := range fields[1:] {
			kvParts := strings.SplitN(strings.TrimSpace(rawField), "=", 2)
			if len(kvParts) != 2 || strings.TrimSpace(kvParts[0]) == "" || strings.TrimSpace(kvParts[1]) == "" {
				t.Fatalf("line %d: invalid parameter %q in %q", lineNo+1, rawField, line)
			}
			kv[strings.TrimSpace(kvParts[0])] = strings.TrimSpace(kvParts[1])
		}

		tag := kv["tag"]
		if tag == "" {
			t.Fatalf("line %d: node entry missing tag: %q", lineNo+1, line)
		}
		if definedTags[tag] {
			t.Fatalf("line %d: duplicate tag %q", lineNo+1, tag)
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
	}
}
