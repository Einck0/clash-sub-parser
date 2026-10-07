package compiler_test

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	probeMihomo "clash-sub-parser/internal/probe/mihomo"
	"clash-sub-parser/internal/resolver"
	officialAdapter "github.com/metacubex/mihomo/adapter"
	"gopkg.in/yaml.v3"
)

func TestMihomoGoldenFixture(t *testing.T) {
	assertTargetGoldenFixture(t, domain.TargetMihomo)
}

func writeDeterministicMihomoGeodataFixtures(t *testing.T, dir string) {
	t.Helper()

	// Minimal valid MaxMind DB (node_count=1, record_size=24) for offline mihomo -t GEOIP validation.
	const mmdbMetaHex = "abcdef4d61784d696e642e636f6de95b62696e6172795f666f726d61745f6d616a6f725f76657273696f6ea1025b62696e6172795f666f726d61745f6d696e6f725f76657273696f6ea04b6275696c645f65706f63680402693a0ecf4d64617461626173655f747970655047656f4c697465322d436f756e7472794b6465736372697074696f6ee142656e5d07437573746f6d697a65642047656f4c6974653220436f756e7472792064617461626173654a69705f76657273696f6ea106496c616e67756167657300044a6e6f64655f636f756e74c1014b7265636f72645f73697a65a118"
	metaBytes, err := hex.DecodeString(mmdbMetaHex)
	if err != nil {
		t.Fatalf("decode mmdb meta hex: %v", err)
	}
	mmdb := make([]byte, 0, 6+16+len(metaBytes))
	mmdb = append(mmdb, 0x00, 0x00, 0x01, 0x00, 0x00, 0x01)
	mmdb = append(mmdb, make([]byte, 16)...)
	mmdb = append(mmdb, metaBytes...)

	for _, name := range []string{"country.mmdb", "Country.mmdb", "geoip.metadb"} {
		if writeErr := os.WriteFile(filepath.Join(dir, name), mmdb, 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", name, writeErr)
		}
	}

	// Minimal valid V2Ray GeoSiteList protobuf for offline mihomo -t GEOSITE validation.
	var geosite []byte
	for _, code := range []string{"CN", "CATEGORY-ADS-ALL", "GOOGLE", "GITHUB"} {
		codeBytes := []byte(code)
		dom := []byte("\x08\x02\x12\x0bexample.com")
		entry := make([]byte, 0, 2+len(codeBytes)+2+len(dom))
		entry = append(entry, 0x0a, byte(len(codeBytes)))
		entry = append(entry, codeBytes...)
		entry = append(entry, 0x12, byte(len(dom)))
		entry = append(entry, dom...)
		geosite = append(geosite, 0x0a, byte(len(entry)))
		geosite = append(geosite, entry...)
	}

	for _, name := range []string{"geosite.dat", "GeoSite.dat"} {
		if writeErr := os.WriteFile(filepath.Join(dir, name), geosite, 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", name, writeErr)
		}
	}
}

func TestMihomoCapabilityMatrix_DeclaresOnlyRenderedCapabilities(t *testing.T) {
	capMatrix := compiler.CapabilityMatrix()[domain.TargetMihomo]

	wantProtocols := map[domain.Protocol]bool{
		domain.ProtocolSS:        true,
		domain.ProtocolVMess:     true,
		domain.ProtocolVLESS:     true,
		domain.ProtocolTrojan:    true,
		domain.ProtocolHysteria2: true,
		domain.ProtocolWireGuard: true,
		domain.ProtocolTUIC:      true,
		domain.ProtocolHTTP:      true,
		domain.ProtocolSocks5:    true,
		domain.ProtocolAnyTLS:    true,
	}
	if !reflect.DeepEqual(capMatrix.Protocols, wantProtocols) {
		t.Fatalf("Mihomo protocol capabilities mismatch:\ngot:  %#v\nwant: %#v", capMatrix.Protocols, wantProtocols)
	}

	wantGroups := map[domain.GroupType]bool{
		domain.GroupTypeSelect:      true,
		domain.GroupTypeURLTest:     true,
		domain.GroupTypeFallback:    true,
		domain.GroupTypeLoadBalance: true,
	}
	if !reflect.DeepEqual(capMatrix.GroupTypes, wantGroups) {
		t.Fatalf("Mihomo group capabilities mismatch:\ngot:  %#v\nwant: %#v", capMatrix.GroupTypes, wantGroups)
	}

	wantRules := map[string]bool{
		"DOMAIN":         true,
		"DOMAIN-SUFFIX":  true,
		"DOMAIN-KEYWORD": true,
		"IP-CIDR":        true,
		"IP-CIDR6":       true,
		"GEOIP":          true,
		"GEOSITE":        true,
		"RULE-SET":       true,
		"SRC-IP-CIDR":    true,
		"SRC-PORT":       true,
		"DST-PORT":       true,
		"PORT":           true,
		"PROCESS-NAME":   true,
		"MATCH":          true,
	}
	if !reflect.DeepEqual(capMatrix.RuleKinds, wantRules) {
		t.Fatalf("Mihomo rule capabilities mismatch:\ngot:  %#v\nwant: %#v", capMatrix.RuleKinds, wantRules)
	}
}

func TestCompileMihomo_AllTenProtocolsFourGroupsFourteenRules_AndOfficialValidation(t *testing.T) {
	ctx := context.Background()

	snapshot := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "snap-mihomo-full-10p-4g-14r",
		CompilerVersion: "1.0.0",
		Nodes: []resolver.ResolvedNode{
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
		},
		Groups: []resolver.ResolvedGroup{
			{
				ID:        "grp-select",
				Name:      "proxy",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindGroup, TargetID: "grp-urltest", DisplayName: "auto-urltest", Position: 0},
					{Kind: resolver.MemberKindGroup, TargetID: "grp-fallback", DisplayName: "auto-fallback", Position: 1},
					{Kind: resolver.MemberKindGroup, TargetID: "grp-lb", DisplayName: "auto-lb", Position: 2},
					{Kind: resolver.MemberKindNode, TargetID: "node-ss", DisplayName: "ss-edge", Position: 3},
					{Kind: resolver.MemberKindNode, TargetID: "node-http", DisplayName: "http-edge", Position: 4},
				},
				Position: 0,
			},
			{
				ID:        "grp-urltest",
				Name:      "auto-urltest",
				GroupType: domain.GroupTypeURLTest,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "node-vmess", DisplayName: "vmess-edge", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "node-vless", DisplayName: "vless-reality-edge", Position: 1},
					{Kind: resolver.MemberKindNode, TargetID: "node-socks5", DisplayName: "socks5-edge", Position: 2},
				},
				Position: 1,
			},
			{
				ID:        "grp-fallback",
				Name:      "auto-fallback",
				GroupType: domain.GroupTypeFallback,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "node-trojan", DisplayName: "trojan-grpc-edge", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "node-hy2", DisplayName: "hy2-edge", Position: 1},
				},
				Position: 2,
			},
			{
				ID:        "grp-lb",
				Name:      "auto-lb",
				GroupType: domain.GroupTypeLoadBalance,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "node-wg", DisplayName: "wg-edge", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "node-tuic", DisplayName: "tuic-edge", Position: 1},
					{Kind: resolver.MemberKindNode, TargetID: "node-anytls", DisplayName: "anytls-edge", Position: 2},
				},
				Position: 3,
			},
		},
		Rules: []resolver.ResolvedRule{
			{ID: "r1", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "DOMAIN,api.example.com", Position: 0},
			{ID: "r2", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "DOMAIN-SUFFIX,example.com", Position: 1},
			{ID: "r3", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "DOMAIN-KEYWORD,example", Position: 2},
			{ID: "r4", TargetGroupID: "grp-urltest", TargetGroupName: "auto-urltest", Expression: "IP-CIDR,198.51.100.0/24,no-resolve", Position: 3},
			{ID: "r5", TargetGroupID: "grp-urltest", TargetGroupName: "auto-urltest", Expression: "IP-CIDR6,2001:db8::/32,no-resolve", Position: 4},
			{ID: "r6", TargetGroupID: "grp-fallback", TargetGroupName: "auto-fallback", Expression: "GEOIP,CN,no-resolve", Position: 5},
			{ID: "r7", TargetGroupID: "grp-fallback", TargetGroupName: "auto-fallback", Expression: "GEOSITE,category-ads-all", Position: 6},
			{ID: "r8", TargetGroupID: "grp-lb", TargetGroupName: "auto-lb", Expression: "RULE-SET,ads-ruleset,https://ruleset.example.com/ads.yaml,no-resolve", Position: 7},
			{ID: "r9", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "SRC-IP-CIDR,192.168.1.0/24", Position: 8},
			{ID: "r10", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "SRC-PORT,8080", Position: 9},
			{ID: "r11", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "DST-PORT,443", Position: 10},
			{ID: "r12", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "PORT,8443", Position: 11},
			{ID: "r13", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "PROCESS-NAME,curl", Position: 12},
			{ID: "r14", TargetGroupID: "grp-select", TargetGroupName: "proxy", Expression: "MATCH", Position: 13, IsTerminal: true},
		},
	}

	res1, err := compiler.CompileMihomo(ctx, snapshot)
	if err != nil {
		t.Fatalf("CompileMihomo failed: %v", err)
	}
	res2, err := compiler.Compile(ctx, snapshot, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("Compile(TargetMihomo) failed: %v", err)
	}
	if string(res1.Content) != string(res2.Content) || res1.ContentDigest != res2.ContentDigest {
		t.Fatal("CompileMihomo and Compile(TargetMihomo) produced non-deterministic output")
	}

	out := string(res1.Content)
	for _, wantSubstr := range []string{
		"type: wireguard",
		"ip: 10.0.0.2/32",
		"ipv6: fd00::2/128",
		"private-key: aGVsbG8td29ybGQtdGVzdC1wcml2YXRlLWtleS0xMjM=",
		"public-key: aGVsbG8td29ybGQtdGVzdC1wdWJsaWMta2V5LTEyMzQ=",
		"pre-shared-key: aGVsbG8td29ybGQtdGVzdC1wc2sta2V5LTEyMzQ1Njc=",
		"reserved: [12, 34, 56]",
		"mtu: 1420",
		"type: tuic",
		"congestion-controller: bbr",
		"udp-relay-mode: native",
		"disable-sni: true",
		"reality-opts:",
		"public-key: jNXHt1yRo0vD5_1N6p2W3x4Y5z6A7b8C9d0E1f2G3h4",
		"short-id: 01ab",
		"client-fingerprint: chrome",
		"flow: xtls-rprx-vision",
		"obfs: salamander",
		"obfs-password: hy2-obfs-secret",
		"ports: 20000-30000",
		"type: url-test",
		"tolerance: 50",
		"type: fallback",
		"type: load-balance",
		"strategy: consistent-hashing",
		"url: https://www.gstatic.com/generate_204",
		"interval: 300",
		"rule-providers:",
		"ads-ruleset:",
		"GEOSITE,category-ads-all,auto-fallback",
		"RULE-SET,ads-ruleset,auto-lb,no-resolve",
		"DST-PORT,8443,proxy",
		"type: http",
		"username: http-user",
		"password: http-password",
		"X-Custom-Header: custom-http-val",
		"type: socks5",
		"username: socks-user",
		"password: socks-password",
		"type: anytls",
		"password: anytls-secret-password",
		"idle-session-check-interval: 30",
		"idle-session-timeout: 60",
		"min-idle-session: 1",
	} {
		if !strings.Contains(out, wantSubstr) {
			t.Errorf("Mihomo output missing expected substring %q:\n%s", wantSubstr, out)
		}
	}

	for _, forbidden := range []string{
		"ruleset.invalid",
		"./ruleset/",
	} {
		if strings.Contains(out, forbidden) {
			t.Errorf("Mihomo output must not contain placeholder %q:\n%s", forbidden, out)
		}
	}

	// Round-trip through parser.ExtractWithCredentials to verify all 10 protocols and credentials survive cleanly.
	extracted, err := parser.ExtractWithCredentials(res1.Content)
	if err != nil {
		t.Fatalf("ExtractWithCredentials on rendered Mihomo output failed: %v", err)
	}
	if extracted.Rejected != 0 || len(extracted.Items) != 10 {
		t.Fatalf("expected 10 extracted nodes and 0 rejected, got %d items and %d rejected", len(extracted.Items), extracted.Rejected)
	}

	// Verify each node can be mapped and parsed by official mihomo adapter.ParseProxy
	for _, n := range snapshot.Nodes {
		domainNode := domain.Node{
			LogicalID:   n.LogicalID,
			DisplayName: n.DisplayName,
			Protocol:    n.Protocol,
			Server:      n.Server,
			Port:        n.Port,
			Credentials: n.Credentials,
		}
		mapping, mapErr := probeMihomo.NodeToMapping(domainNode)
		if mapErr != nil {
			t.Errorf("NodeToMapping failed for node %s (%s): %v", n.DisplayName, n.Protocol, mapErr)
			continue
		}
		proxy, parseErr := officialAdapter.ParseProxy(mapping)
		if parseErr != nil {
			t.Errorf("officialAdapter.ParseProxy failed for node %s (%s): %v", n.DisplayName, n.Protocol, parseErr)
			continue
		}
		if proxy == nil {
			t.Errorf("expected non-nil proxy for node %s", n.DisplayName)
		}
	}

	mihomoBin := findMihomoBinary()
	if mihomoBin == "" {
		t.Fatal("official mihomo binary not found on system")
	}

	tmpDir := t.TempDir()
	writeDeterministicMihomoGeodataFixtures(t, tmpDir)

	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if writeErr := os.WriteFile(cfgPath, res1.Content, 0o600); writeErr != nil {
		t.Fatalf("write temp config: %v", writeErr)
	}

	cmd := exec.Command(mihomoBin, "-t", "-d", tmpDir, "-f", cfgPath)
	output, execErr := cmd.CombinedOutput()
	if execErr != nil {
		t.Fatalf("official mihomo -t failed on 7-protocol 4-group 14-rule config: %v\nOutput:\n%s\nConfig:\n%s", execErr, string(output), out)
	}
}

func TestCompileMihomo_FailsClosedWithoutCredentials(t *testing.T) {
	ctx := context.Background()

	t.Run("CompileTargetMihomo_WithoutCredentials_FailsClosed", func(t *testing.T) {
		snapshot := fixtureSnapshot()
		snapshot.Nodes[0].Credentials = domain.InboundProtocolCredential{}
		_, err := compiler.Compile(ctx, snapshot, domain.TargetMihomo)
		if err == nil {
			t.Fatal("expected TargetMihomo without credentials to fail closed")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) {
			t.Fatalf("expected CapabilityError, got %v", err)
		}
		if capErr.Target != domain.TargetMihomo || capErr.Location != "nodes[0]" {
			t.Fatalf("unexpected capability error: %#v", capErr)
		}
	})

	t.Run("CompileMihomo_PartialCredentials_FailsClosedAtLocation", func(t *testing.T) {
		snapshot := fixtureSnapshot()
		snapshot.Nodes[1].Credentials = domain.InboundProtocolCredential{}
		_, err := compiler.CompileMihomo(ctx, snapshot)
		if err == nil {
			t.Fatal("expected missing node credentials to fail closed")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) {
			t.Fatalf("expected CapabilityError, got %v", err)
		}
		if capErr.Location != "nodes[1]" {
			t.Fatalf("expected location nodes[1], got %s", capErr.Location)
		}
	})

	t.Run("CompileMihomo_MissingPassword_FailsClosed", func(t *testing.T) {
		snapshot := fixtureSnapshot()
		snapshot.Nodes[1].Credentials.Password = ""
		_, err := compiler.CompileMihomo(ctx, snapshot)
		if err == nil {
			t.Fatal("expected empty password to fail closed")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) {
			t.Fatalf("expected CapabilityError, got %v", err)
		}
		if capErr.Location != "nodes[1]" || capErr.Feature != "ss" {
			t.Fatalf("expected ss failure at nodes[1], got %#v", capErr)
		}
	})

	t.Run("CompileMihomo_MissingServer_FailsClosed", func(t *testing.T) {
		snapshot := fixtureSnapshot()
		snapshot.Nodes[0].Server = "  "
		_, err := compiler.CompileMihomo(ctx, snapshot)
		if err == nil {
			t.Fatal("expected blank server to fail closed")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) {
			t.Fatalf("expected CapabilityError, got %v", err)
		}
		if capErr.Location != "nodes[0]" {
			t.Fatalf("expected nodes[0], got %s", capErr.Location)
		}
	})
}

func TestCompileMihomo_SevenProtocolsNegativeValidationAndZeroSecretLeakage(t *testing.T) {
	ctx := context.Background()

	makeSingleNodeSnap := func(proto domain.Protocol) *resolver.ResolvedPolicySnapshot {
		return &resolver.ResolvedPolicySnapshot{
			SnapshotDigest:  "snap-neg-" + string(proto),
			CompilerVersion: "1.0.0",
			Nodes: []resolver.ResolvedNode{
				{LogicalID: "node-0", DisplayName: "edge-0", Protocol: proto, Active: true, Position: 0},
			},
			Groups: []resolver.ResolvedGroup{
				{
					ID:        "grp-0",
					Name:      "proxy",
					GroupType: domain.GroupTypeSelect,
					Members: []resolver.ResolvedGroupMember{
						{Kind: resolver.MemberKindNode, TargetID: "node-0", DisplayName: "edge-0", Position: 0},
					},
					Position: 0,
				},
			},
			Rules: []resolver.ResolvedRule{
				{ID: "r-0", TargetGroupID: "grp-0", TargetGroupName: "proxy", Expression: "MATCH", Position: 0, IsTerminal: true},
			},
		}
	}

	cases := []struct {
		name         string
		proto        domain.Protocol
		cred         domain.InboundProtocolCredential
		secretMarker string
		wantReason   string
	}{
		{
			name:  "VMess_UnsupportedNetworkKCP",
			proto: domain.ProtocolVMess,
			cred: domain.InboundProtocolCredential{
				UUID:      "secret-vmess-uuid-001",
				Transport: map[string]string{"network": "kcp"},
			},
			secretMarker: "secret-vmess-uuid-001",
			wantReason:   "unsupported network",
		},
		{
			name:  "VLESS_RealitySidWithoutPbk",
			proto: domain.ProtocolVLESS,
			cred: domain.InboundProtocolCredential{
				UUID:      "secret-vless-uuid-002",
				Transport: map[string]string{"sid": "01ab"},
			},
			secretMarker: "secret-vless-uuid-002",
			wantReason:   "missing required reality public key",
		},
		{
			name:  "VLESS_UnsupportedFlow",
			proto: domain.ProtocolVLESS,
			cred: domain.InboundProtocolCredential{
				UUID:      "secret-vless-uuid-003",
				Transport: map[string]string{"flow": "xtls-rprx-direct"},
			},
			secretMarker: "secret-vless-uuid-003",
			wantReason:   "unsupported flow",
		},
		{
			name:  "Hysteria2_UnsupportedObfsMode",
			proto: domain.ProtocolHysteria2,
			cred: domain.InboundProtocolCredential{
				Password:  "secret-hy2-password-004",
				Transport: map[string]string{"obfs": "faketls", "obfs-password": "secret-obfs-password-004"},
			},
			secretMarker: "secret-hy2-password-004",
			wantReason:   "unsupported obfs mode",
		},
		{
			name:  "Hysteria2_ObfsWithoutObfsPassword",
			proto: domain.ProtocolHysteria2,
			cred: domain.InboundProtocolCredential{
				Password:  "secret-hy2-password-005",
				Transport: map[string]string{"obfs": "salamander"},
			},
			secretMarker: "secret-hy2-password-005",
			wantReason:   "missing required obfs-password",
		},
		{
			name:  "Hysteria2_ObfsPasswordWithoutObfs",
			proto: domain.ProtocolHysteria2,
			cred: domain.InboundProtocolCredential{
				Password:  "secret-hy2-password-006",
				Transport: map[string]string{"obfs-password": "secret-obfs-password-006"},
			},
			secretMarker: "secret-obfs-password-006",
			wantReason:   "obfs-password requires obfs mode",
		},
		{
			name:  "WireGuard_MissingPrivateKey",
			proto: domain.ProtocolWireGuard,
			cred: domain.InboundProtocolCredential{
				PublicKey:    "secret-wg-pub-007",
				LocalAddress: []string{"10.0.0.2/32"},
			},
			secretMarker: "secret-wg-pub-007",
			wantReason:   "missing required private_key",
		},
		{
			name:  "WireGuard_MissingPublicKey",
			proto: domain.ProtocolWireGuard,
			cred: domain.InboundProtocolCredential{
				PrivateKey:   "secret-wg-priv-008",
				LocalAddress: []string{"10.0.0.2/32"},
			},
			secretMarker: "secret-wg-priv-008",
			wantReason:   "missing required public_key",
		},
		{
			name:  "WireGuard_MissingLocalAddress",
			proto: domain.ProtocolWireGuard,
			cred: domain.InboundProtocolCredential{
				PrivateKey: "secret-wg-priv-009",
				PublicKey:  "secret-wg-pub-009",
			},
			secretMarker: "secret-wg-priv-009",
			wantReason:   "missing required local_address",
		},
		{
			name:  "WireGuard_MultipleIPv4AddressesRejected",
			proto: domain.ProtocolWireGuard,
			cred: domain.InboundProtocolCredential{
				PrivateKey:   "secret-wg-priv-010",
				PublicKey:    "secret-wg-pub-010",
				LocalAddress: []string{"10.0.0.2/32", "10.0.0.3/32"},
			},
			secretMarker: "secret-wg-priv-010",
			wantReason:   "multiple IPv4 local_address CIDRs",
		},
		{
			name:  "TUIC_MissingUUID",
			proto: domain.ProtocolTUIC,
			cred: domain.InboundProtocolCredential{
				Password: "secret-tuic-password-011",
			},
			secretMarker: "secret-tuic-password-011",
			wantReason:   "missing required uuid",
		},
		{
			name:  "TUIC_MissingPassword",
			proto: domain.ProtocolTUIC,
			cred: domain.InboundProtocolCredential{
				UUID: "secret-tuic-uuid-012",
			},
			secretMarker: "secret-tuic-uuid-012",
			wantReason:   "missing required password",
		},
		{
			name:  "TUIC_UnsupportedCongestionController",
			proto: domain.ProtocolTUIC,
			cred: domain.InboundProtocolCredential{
				UUID:              "secret-tuic-uuid-013",
				Password:          "secret-tuic-password-013",
				CongestionControl: "reno-invalid",
			},
			secretMarker: "secret-tuic-password-013",
			wantReason:   "unsupported congestion controller",
		},
		{
			name:  "TUIC_UnsupportedUDPRelayMode",
			proto: domain.ProtocolTUIC,
			cred: domain.InboundProtocolCredential{
				UUID:         "secret-tuic-uuid-014",
				Password:     "secret-tuic-password-014",
				UDPRelayMode: "udp-invalid",
			},
			secretMarker: "secret-tuic-password-014",
			wantReason:   "unsupported udp relay mode",
		},
		{
			name:  "UnknownTransportKeyFailsClosed",
			proto: domain.ProtocolTrojan,
			cred: domain.InboundProtocolCredential{
				Password:  "secret-trojan-password-015",
				Transport: map[string]string{"custom_unrecognized_flag": "1"},
			},
			secretMarker: "secret-trojan-password-015",
			wantReason:   "unsupported transport parameter",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := makeSingleNodeSnap(tc.proto)
			snap.Nodes[0].Server = "198.51.100.99"
			snap.Nodes[0].Port = 443
			snap.Nodes[0].Credentials = tc.cred
			_, err := compiler.CompileMihomo(ctx, snap)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("expected CapabilityError, got %T: %v", err, err)
			}
			if capErr.Target != domain.TargetMihomo || capErr.Location != "nodes[0]" {
				t.Fatalf("unexpected CapabilityError target/location: %#v", capErr)
			}
			if !strings.Contains(capErr.Reason, tc.wantReason) {
				t.Fatalf("expected reason containing %q, got %q", tc.wantReason, capErr.Reason)
			}
			if tc.secretMarker != "" && strings.Contains(err.Error(), tc.secretMarker) {
				t.Fatalf("error leaked secret %q: %v", tc.secretMarker, err)
			}
		})
	}
}

func TestCompileMihomo_ModernRulesAndFailClosedValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("SupportsGEOSITE_RULESET_PROCESSNAME_PORT", func(t *testing.T) {
		snapshot := fixtureSnapshot()
		snapshot.Rules = []resolver.ResolvedRule{
			{ID: "r1", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "GEOSITE,category-ads-all", Position: 0},
			{ID: "r2", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "RULE-SET,https://example.com/rules/reject.yaml,no-resolve", Position: 1},
			{ID: "r3", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "PROCESS-NAME,curl", Position: 2},
			{ID: "r4", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "PORT,8000-8080", Position: 3},
			{ID: "r5", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "MATCH", Position: 4, IsTerminal: true},
		}
		res, err := compiler.CompileMihomo(ctx, snapshot)
		if err != nil {
			t.Fatalf("expected modern rules to compile cleanly on Mihomo: %v", err)
		}
		out := string(res.Content)
		if !strings.Contains(out, "GEOSITE,category-ads-all,proxy") ||
			!strings.Contains(out, "RULE-SET,reject,proxy,no-resolve") ||
			!strings.Contains(out, "PROCESS-NAME,curl,proxy") ||
			!strings.Contains(out, "DST-PORT,8000-8080,proxy") {
			t.Fatalf("unexpected rendered rules in Mihomo output:\n%s", out)
		}
		if strings.Contains(out, "ruleset.invalid") || strings.Contains(out, "./ruleset/") {
			t.Fatalf("Mihomo RULE-SET output must not contain placeholder URL or relative path:\n%s", out)
		}
	})

	t.Run("RuleSet_ConfiguredProviderAndExplicitHTTPSURL_PassesOfficialMihomo", func(t *testing.T) {
		snapshot := fixtureSnapshot()
		snapshot.Rules = []resolver.ResolvedRule{
			{ID: "r1", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "RULE-SET,corp-direct,https://rules.example.com/corp/direct.yaml", Position: 0},
			{ID: "r2", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "RULE-SET,corp-direct,no-resolve", Position: 1},
			{ID: "r3", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "RULE-SET,https://rules.example.com/ads/reject.yaml,no-resolve", Position: 2},
			{ID: "r4", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "MATCH", Position: 3, IsTerminal: true},
		}
		res, err := compiler.CompileMihomo(ctx, snapshot)
		if err != nil {
			t.Fatalf("expected valid https RULE-SET rules to compile: %v", err)
		}
		out := string(res.Content)
		for _, want := range []string{
			"corp-direct:",
			"url: https://rules.example.com/corp/direct.yaml",
			"reject:",
			"url: https://rules.example.com/ads/reject.yaml",
			"RULE-SET,corp-direct,proxy",
			"RULE-SET,corp-direct,proxy,no-resolve",
			"RULE-SET,reject,proxy,no-resolve",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("expected Mihomo output to contain %q, got:\n%s", want, out)
			}
		}
		if strings.Contains(out, "ruleset.invalid") || strings.Contains(out, "./ruleset/") {
			t.Fatalf("Mihomo output must not contain fake placeholder URL or path:\n%s", out)
		}

		if mihomoBin := findMihomoBinary(); mihomoBin != "" {
			tmpDir := t.TempDir()
			cfgPath := filepath.Join(tmpDir, "config.yaml")
			if writeErr := os.WriteFile(cfgPath, res.Content, 0o600); writeErr != nil {
				t.Fatalf("write temp config: %v", writeErr)
			}
			cmd := exec.Command(mihomoBin, "-t", "-d", tmpDir, "-f", cfgPath)
			if output, execErr := cmd.CombinedOutput(); execErr != nil {
				t.Fatalf("official mihomo -t failed on RULE-SET config: %v\nOutput:\n%s\nConfig:\n%s", execErr, string(output), out)
			}
		}
	})

	t.Run("RuleSet_BareProviderAtNonZeroIndex_FailsClosedWithExactRuleLocation", func(t *testing.T) {
		snapshot := fixtureSnapshot()
		snapshot.Rules = []resolver.ResolvedRule{
			{ID: "r1", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "DOMAIN-SUFFIX,example.com", Position: 0},
			{ID: "r2", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "RULE-SET,unconfigured-provider", Position: 1},
			{ID: "r3", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "MATCH", Position: 2, IsTerminal: true},
		}
		_, err := compiler.CompileMihomo(ctx, snapshot)
		if err == nil {
			t.Fatal("expected bare RULE-SET provider without URL at rules[1] to fail closed")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) {
			t.Fatalf("expected *compiler.CapabilityError, got %T: %v", err, err)
		}
		if capErr.Target != domain.TargetMihomo || capErr.Location != "rules[1]" || capErr.Feature != "RULE-SET" {
			t.Fatalf("unexpected CapabilityError fields: %#v", capErr)
		}
	})

	t.Run("RuleSet_ConflictingProviderURLs_FailsClosedAtSecondRule", func(t *testing.T) {
		snapshot := fixtureSnapshot()
		snapshot.Rules = []resolver.ResolvedRule{
			{ID: "r1", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "RULE-SET,ads,https://rules.example.com/v1/ads.yaml", Position: 0},
			{ID: "r2", TargetGroupID: snapshot.Groups[0].ID, TargetGroupName: "proxy", Expression: "RULE-SET,ads,https://rules.example.com/v2/ads.yaml", Position: 1},
		}
		_, err := compiler.CompileMihomo(ctx, snapshot)
		if err == nil {
			t.Fatal("expected conflicting RULE-SET provider URLs to fail closed")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) {
			t.Fatalf("expected *compiler.CapabilityError, got %T: %v", err, err)
		}
		if capErr.Target != domain.TargetMihomo || capErr.Location != "rules[1]" || capErr.Feature != "RULE-SET" {
			t.Fatalf("unexpected CapabilityError fields: %#v", capErr)
		}
		if !strings.Contains(capErr.Reason, "conflicting URLs") {
			t.Fatalf("expected conflicting URLs reason, got %q", capErr.Reason)
		}
	})

	invalidRules := []struct {
		name       string
		expression string
		wantReason string
	}{
		{
			name:       "UnsupportedRuleKind_USER_AGENT",
			expression: "USER-AGENT,Instagram*",
			wantReason: "routing rule kind is not supported",
		},
		{
			name:       "UnsupportedRuleKind_URL_REGEX",
			expression: "URL-REGEX,^https://ads\\.example\\.com",
			wantReason: "routing rule kind is not supported",
		},
		{
			name:       "DomainRule_UnknownParameterFailsClosed",
			expression: "DOMAIN,example.com,no-resolve",
			wantReason: "unsupported parameter",
		},
		{
			name:       "GeositeRule_UnknownParameterFailsClosed",
			expression: "GEOSITE,cn,no-resolve",
			wantReason: "unsupported parameter",
		},
		{
			name:       "GeoipRule_UnknownParameterFailsClosed",
			expression: "GEOIP,CN,unknown-opt",
			wantReason: "unsupported parameter",
		},
		{
			name:       "IPCIDR_InvalidCIDRFailsClosed",
			expression: "IP-CIDR,not-a-cidr",
			wantReason: "invalid CIDR",
		},
		{
			name:       "IPCIDR_IPv6InIPCIDR4FailsClosed",
			expression: "IP-CIDR,2001:db8::/32",
			wantReason: "IPv4 CIDR prefix",
		},
		{
			name:       "IPCIDR6_IPv4InIPCIDR6FailsClosed",
			expression: "IP-CIDR6,198.51.100.0/24",
			wantReason: "IPv6 CIDR prefix",
		},
		{
			name:       "IPCIDR_UnknownParameterFailsClosed",
			expression: "IP-CIDR,198.51.100.0/24,bad-flag",
			wantReason: "unsupported parameter",
		},
		{
			name:       "DstPort_OutOfRangeFailsClosed",
			expression: "DST-PORT,70000",
			wantReason: "out of bounds",
		},
		{
			name:       "Port_InvertedRangeFailsClosed",
			expression: "PORT,9000-8000",
			wantReason: "out of bounds",
		},
		{
			name:       "RuleSet_InvalidProviderNameFailsClosed",
			expression: "RULE-SET,../bad-provider",
			wantReason: "invalid RULE-SET provider name",
		},
		{
			name:       "RuleSet_BareProviderWithoutURLFailsClosed",
			expression: "RULE-SET,ads-only",
			wantReason: "missing explicit https:// URL",
		},
		{
			name:       "RuleSet_BareProviderWithNoResolveWithoutURLFailsClosed",
			expression: "RULE-SET,ads-only,no-resolve",
			wantReason: "missing explicit https:// URL",
		},
		{
			name:       "RuleSet_InvalidURLSchemeFailsClosed",
			expression: "RULE-SET,ftp://example.com/rules.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_PlainHTTPURLFailsClosed",
			expression: "RULE-SET,http://example.com/rules.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_ProviderWithPlainHTTPURLFailsClosed",
			expression: "RULE-SET,ads,http://example.com/rules.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_DotInvalidDomainURLFailsClosed",
			expression: "RULE-SET,https://ruleset.invalid/ads.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_ProviderWithDotInvalidDomainURLFailsClosed",
			expression: "RULE-SET,ads,https://ruleset.invalid/ads.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_EmptyHostURLFailsClosed",
			expression: "RULE-SET,https:///ads.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_ProviderWithEmptyHostPortURLFailsClosed",
			expression: "RULE-SET,ads,https://:443/ads.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_EmptyResourcePathURLFailsClosed",
			expression: "RULE-SET,https://example.com/",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_RelativeFakePathSecondArgFailsClosed",
			expression: "RULE-SET,ads,./ruleset/ads.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_TraversalPathInHTTPSURLFailsClosed",
			expression: "RULE-SET,ads,https://example.com/../ads.yaml",
			wantReason: "invalid RULE-SET URL",
		},
		{
			name:       "RuleSet_UnknownParameterFailsClosed",
			expression: "RULE-SET,ads,unknown-flag",
			wantReason: "unsupported parameter",
		},
	}

	for _, tc := range invalidRules {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := fixtureSnapshot()
			snapshot.Rules = []resolver.ResolvedRule{
				{
					ID:              "r-bad",
					TargetGroupID:   snapshot.Groups[0].ID,
					TargetGroupName: snapshot.Groups[0].Name,
					Expression:      tc.expression,
					Position:        0,
				},
			}
			_, err := compiler.CompileMihomo(ctx, snapshot)
			if err == nil {
				t.Fatalf("expected rule %q to fail closed, got nil", tc.expression)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("expected CapabilityError, got %T: %v", err, err)
			}
			if capErr.Target != domain.TargetMihomo || capErr.Location != "rules[0]" {
				t.Fatalf("unexpected CapabilityError location/target: %#v", capErr)
			}
			if strings.HasPrefix(tc.name, "RuleSet_") && capErr.Feature != "RULE-SET" {
				t.Fatalf("expected Feature RULE-SET, got %q", capErr.Feature)
			}
			if !strings.Contains(capErr.Reason, tc.wantReason) {
				t.Fatalf("expected reason containing %q, got %q", tc.wantReason, capErr.Reason)
			}
		})
	}
}

func TestMihomoOutputIsValidYAML(t *testing.T) {
	res, err := compiler.CompileMihomo(context.Background(), fixtureSnapshot())
	if err != nil {
		t.Fatalf("compile Mihomo failed: %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(res.Content, &parsed); err != nil {
		t.Fatalf("Mihomo output invalid YAML: %v", err)
	}
	proxies, ok := parsed["proxies"].([]any)
	if !ok || len(proxies) != 2 {
		t.Fatalf("Mihomo missing expected proxies array: %#v", parsed["proxies"])
	}
	groups, ok := parsed["proxy-groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("Mihomo missing expected proxy-groups array: %#v", parsed["proxy-groups"])
	}
}

func TestMihomo_RejectsEmptyProxyGroups(t *testing.T) {
	ctx := context.Background()
	snap := fixtureSnapshot()
	// Add an empty proxy group (e.g. 加拿大 with 0 members)
	snap.Groups = append(snap.Groups, resolver.ResolvedGroup{
		ID:        "grp-canada",
		Name:      "加拿大",
		GroupType: domain.GroupTypeURLTest,
		Members:   []resolver.ResolvedGroupMember{},
	})

	// 1. ValidateTargetCapabilities must flag required_nonempty
	diags := compiler.ValidateTargetCapabilities(snap, domain.TargetMihomo)
	foundRequiredNonEmpty := false
	for _, d := range diags {
		if d.Code == "required_nonempty" && strings.Contains(d.Message, "加拿大") {
			foundRequiredNonEmpty = true
			break
		}
	}
	if !foundRequiredNonEmpty {
		t.Fatalf("expected required_nonempty diagnostic for group 加拿大, got: %+v", diags)
	}

	// 2. CompileMihomo must fail closed with CapabilityError
	_, err := compiler.CompileMihomo(ctx, snap)
	if err == nil {
		t.Fatalf("expected CompileMihomo to fail on empty group 加拿大, got nil")
	}
	var capErr *compiler.CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected CapabilityError, got %T: %v", err, err)
	}
	if !strings.Contains(capErr.Reason, "required_nonempty") {
		t.Fatalf("expected reason containing required_nonempty, got: %s", capErr.Reason)
	}
}

func TestMihomo_RejectsDanglingRuleTarget(t *testing.T) {
	ctx := context.Background()
	snap := fixtureSnapshot()
	// Add rule targeting non-existent group
	snap.Rules = append(snap.Rules, resolver.ResolvedRule{
		ID:              "rule-dangling",
		TargetGroupName: "NonExistentGroup",
		Expression:      "DOMAIN-SUFFIX,example.com",
	})

	// 1. ValidateTargetCapabilities must flag dangling_rule_target
	diags := compiler.ValidateTargetCapabilities(snap, domain.TargetMihomo)
	foundDangling := false
	for _, d := range diags {
		if d.Code == "dangling_rule_target" && strings.Contains(d.Message, "NonExistentGroup") {
			foundDangling = true
			break
		}
	}
	if !foundDangling {
		t.Fatalf("expected dangling_rule_target diagnostic, got: %+v", diags)
	}

	// 2. CompileMihomo must fail closed
	_, err := compiler.CompileMihomo(ctx, snap)
	if err == nil {
		t.Fatalf("expected CompileMihomo to fail on dangling rule target, got nil")
	}
}

func TestMihomo_RejectsDanglingGroupReference(t *testing.T) {
	ctx := context.Background()
	snap := fixtureSnapshot()
	// Add group referencing non-existent child group
	snap.Groups[0].Members = append(snap.Groups[0].Members, resolver.ResolvedGroupMember{
		Kind:        resolver.MemberKindGroup,
		TargetID:    "missing-child-id",
		DisplayName: "MissingChildGroup",
	})

	// 1. ValidateTargetCapabilities must flag dangling_group_reference
	diags := compiler.ValidateTargetCapabilities(snap, domain.TargetMihomo)
	foundDangling := false
	for _, d := range diags {
		if d.Code == "dangling_group_reference" && strings.Contains(d.Message, "MissingChildGroup") {
			foundDangling = true
			break
		}
	}
	if !foundDangling {
		t.Fatalf("expected dangling_group_reference diagnostic, got: %+v", diags)
	}

	// 2. CompileMihomo must fail closed
	_, err := compiler.CompileMihomo(ctx, snap)
	if err == nil {
		t.Fatalf("expected CompileMihomo to fail on dangling group reference, got nil")
	}
}

