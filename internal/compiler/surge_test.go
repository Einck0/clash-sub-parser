package compiler_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

type parsedSurgeProxy struct {
	Name   string
	Proto  string
	Server string
	Port   int
	Params map[string]string
}

type parsedSurgeGroup struct {
	Name    string
	Type    string
	Members []string
	Params  map[string]string
}

type parsedSurgeRule struct {
	Kind    string
	Value   string
	Policy  string
	Options []string
}

type parsedSurgeConfig struct {
	General map[string]string
	Proxies map[string]parsedSurgeProxy
	Groups  map[string]parsedSurgeGroup
	Rules   []parsedSurgeRule
}

// parseSurge5Config validates pure Surge 5 node lines (without [General]/[Proxy]/[Proxy Group]/[Rule] sections).
func parseSurge5Config(raw string, disallowedLogicalIDs map[string]bool) (*parsedSurgeConfig, error) {
	cfg := &parsedSurgeConfig{
		General: make(map[string]string),
		Proxies: make(map[string]parsedSurgeProxy),
		Groups:  make(map[string]parsedSurgeGroup),
	}

	lines := strings.Split(raw, "\n")
	for lineNo, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			return nil, fmt.Errorf("line %d: unexpected INI section %q in node-only Surge output", lineNo+1, line)
		}

		name, rest, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: invalid proxy definition %q", lineNo+1, line)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("line %d: empty proxy name", lineNo+1)
		}
		tokens := splitTrimmedCSV(rest)
		if len(tokens) < 4 {
			return nil, fmt.Errorf("line %d: proxy %q requires at least type, server, port, and credential params", lineNo+1, name)
		}
		proto := tokens[0]
		server := tokens[1]
		if disallowedLogicalIDs[server] {
			return nil, fmt.Errorf("line %d: proxy %q uses LogicalID %q as server address", lineNo+1, name, server)
		}
		port, err := strconv.Atoi(tokens[2])
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("line %d: proxy %q has invalid port %q", lineNo+1, name, tokens[2])
		}
		params := make(map[string]string, len(tokens)-3)
		for _, kv := range tokens[3:] {
			pk, pv, hasEq := strings.Cut(kv, "=")
			if !hasEq || strings.TrimSpace(pk) == "" || strings.TrimSpace(pv) == "" {
				return nil, fmt.Errorf("line %d: proxy %q has malformed parameter %q", lineNo+1, name, kv)
			}
			params[strings.TrimSpace(pk)] = strings.TrimSpace(pv)
		}
		switch proto {
		case "ss":
			if params["encrypt-method"] == "" || params["password"] == "" {
				return nil, fmt.Errorf("line %d: ss proxy %q missing encrypt-method or password", lineNo+1, name)
			}
		case "vmess":
			if params["username"] == "" || (params["vmess-aead"] != "true" && params["vmess-aead"] != "false") {
				return nil, fmt.Errorf("line %d: vmess proxy %q missing username or vmess-aead", lineNo+1, name)
			}
		case "trojan", "hysteria2":
			if params["password"] == "" {
				return nil, fmt.Errorf("line %d: %s proxy %q missing password", lineNo+1, proto, name)
			}
		case "tuic":
			if params["uuid"] == "" || params["password"] == "" {
				return nil, fmt.Errorf("line %d: tuic proxy %q missing uuid or password", lineNo+1, name)
			}
		case "wireguard":
			if params["private-key"] == "" || params["peer-public-key"] == "" {
				return nil, fmt.Errorf("line %d: wireguard proxy %q missing private-key or peer-public-key", lineNo+1, name)
			}
		default:
			return nil, fmt.Errorf("line %d: unsupported Surge 5 proxy type %q", lineNo+1, proto)
		}
		cfg.Proxies[name] = parsedSurgeProxy{
			Name:   name,
			Proto:  proto,
			Server: server,
			Port:   port,
			Params: params,
		}
	}

	return cfg, nil
}

func splitTrimmedCSV(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, strings.TrimSpace(item))
	}
	return out
}

func TestSurgeGoldenFixture(t *testing.T) {
	assertTargetGoldenFixture(t, domain.TargetSurge)

	snap := fixtureSnapshot()
	res, err := compiler.Compile(context.Background(), snap, domain.TargetSurge)
	if err != nil {
		t.Fatalf("compile Surge failed: %v", err)
	}
	out := string(res.Content)

	disallowedIDs := map[string]bool{
		"0123456789abcdef0123456789abcdef": true,
		"abcdef0123456789abcdef0123456789": true,
	}
	for id := range disallowedIDs {
		if strings.Contains(out, id) {
			t.Fatalf("Surge output must never contain fake LogicalID %s:\n%s", id, out)
		}
	}

	parsed, err := parseSurge5Config(out, disallowedIDs)
	if err != nil {
		t.Fatalf("independent Surge 5 parser rejected golden output: %v\nOutput:\n%s", err, out)
	}

	edgeA, ok := parsed.Proxies["edge-a"]
	if !ok || edgeA.Proto != "vmess" || edgeA.Server != "198.51.100.1" || edgeA.Port != 443 ||
		edgeA.Params["username"] != "b831381d-6324-4d53-ad4f-8cda48b30811" ||
		edgeA.Params["vmess-aead"] != "true" ||
		edgeA.Params["ws"] != "true" ||
		edgeA.Params["ws-path"] != "/vmess" ||
		edgeA.Params["ws-headers"] != "Host:example.com" ||
		edgeA.Params["tls"] != "true" {
		t.Fatalf("unexpected parsed edge-a proxy: %+v", edgeA)
	}

	edgeB, ok := parsed.Proxies["edge-b"]
	if !ok || edgeB.Proto != "ss" || edgeB.Server != "198.51.100.2" || edgeB.Port != 8388 ||
		edgeB.Params["encrypt-method"] != "aes-256-gcm" ||
		edgeB.Params["password"] != "test-ss-password" {
		t.Fatalf("unexpected parsed edge-b proxy: %+v", edgeB)
	}

	if len(parsed.Groups) != 0 || len(parsed.Rules) != 0 {
		t.Fatalf("expected node-only Surge export to have 0 groups and 0 rules, got groups=%d rules=%d", len(parsed.Groups), len(parsed.Rules))
	}
}

func TestSurgeOutputFormat(t *testing.T) {
	surgeRes, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetSurge)
	if err != nil {
		t.Fatalf("compile Surge failed: %v", err)
	}
	surgeStr := string(surgeRes.Content)
	for _, unexpectedSection := range []string{"[General]", "[Proxy]", "[Proxy Group]", "[Rule]"} {
		if strings.Contains(surgeStr, unexpectedSection) {
			t.Fatalf("node-only Surge output must not contain section %s:\n%s", unexpectedSection, surgeStr)
		}
	}
	if !strings.Contains(surgeStr, "edge-a = vmess,") || !strings.Contains(surgeStr, "edge-b = ss,") {
		t.Fatalf("Surge output missing expected node lines:\n%s", surgeStr)
	}
}

func TestSurgeSupportsProcessNameRule(t *testing.T) {
	snapshot := fixtureSnapshot()
	snapshot.Rules = []resolver.ResolvedRule{
		snapshot.Rules[0],
		{
			ID:              "rule-process-name",
			TargetGroupID:   snapshot.Groups[0].ID,
			TargetGroupName: snapshot.Groups[0].Name,
			Expression:      "PROCESS-NAME,curl",
			Position:        1,
		},
		{
			ID:              snapshot.Rules[1].ID,
			TargetGroupID:   snapshot.Rules[1].TargetGroupID,
			TargetGroupName: snapshot.Rules[1].TargetGroupName,
			Expression:      snapshot.Rules[1].Expression,
			Position:        2,
			IsTerminal:      true,
		},
	}
	res, err := compiler.Compile(context.Background(), snapshot, domain.TargetSurge)
	if err != nil {
		t.Fatalf("expected Surge node-only export to ignore PROCESS-NAME rule cleanly: %v", err)
	}
	if strings.Contains(string(res.Content), "PROCESS-NAME") {
		t.Fatalf("expected PROCESS-NAME rule to be omitted in node-only Surge output:\n%s", string(res.Content))
	}
}

func TestSurgeAllSupportedProtocolsGroupsAndRules(t *testing.T) {
	ctx := context.Background()
	snapshot := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "surge-full-digest",
		CompilerVersion: "1.0.0",
		Nodes: []resolver.ResolvedNode{
			{
				LogicalID:   "id-ss-obfs",
				DisplayName: "HK-SS-Obfs",
				Protocol:    domain.ProtocolSS,
				Server:      "hk.ss.example.com",
				Port:        8388,
				Credentials: domain.InboundProtocolCredential{
					Method:   "2022-blake3-aes-128-gcm",
					Password: "ss-2022-secret-password",
					Transport: map[string]string{
						"obfs": "tls",
						"host": "obfs.example.com",
						"path": "/obfs",
					},
				},
				Active:   true,
				Position: 0,
			},
			{
				LogicalID:   "id-vmess-legacy",
				DisplayName: "US-VMess-Legacy",
				Protocol:    domain.ProtocolVMess,
				Server:      "us.vmess.example.com",
				Port:        8443,
				Credentials: domain.InboundProtocolCredential{
					UUID:    "11111111-2222-3333-4444-555555555555",
					AlterID: 2,
					Method:  "chacha20-poly1305",
					Transport: map[string]string{
						"network":          "tcp",
						"tls":              "true",
						"sni":              "sni.vmess.example.com",
						"skip_cert_verify": "true",
					},
				},
				Active:   true,
				Position: 1,
			},
			{
				LogicalID:   "id-trojan-ws",
				DisplayName: "JP-Trojan-WS",
				Protocol:    domain.ProtocolTrojan,
				Server:      "jp.trojan.example.com",
				Port:        443,
				Credentials: domain.InboundProtocolCredential{
					Password: "trojan-secret-password",
					SNI:      "sni.trojan.example.com",
					Transport: map[string]string{
						"network":          "ws",
						"path":             "/trojan-ws",
						"host":             "cdn.trojan.example.com",
						"skip-cert-verify": "true",
					},
				},
				Active:   true,
				Position: 2,
			},
			{
				LogicalID:   "id-hy2",
				DisplayName: "SG-Hysteria2",
				Protocol:    domain.ProtocolHysteria2,
				Server:      "sg.hy2.example.com",
				Port:        443,
				Credentials: domain.InboundProtocolCredential{
					Password: "hy2-secret-password",
					SNI:      "sni.hy2.example.com",
					Transport: map[string]string{
						"down":             "500 Mbps",
						"skip-cert-verify": "true",
					},
				},
				Active:   true,
				Position: 3,
			},
			{
				LogicalID:   "id-tuic",
				DisplayName: "DE-TUIC",
				Protocol:    domain.ProtocolTUIC,
				Server:      "de.tuic.example.com",
				Port:        8443,
				Credentials: domain.InboundProtocolCredential{
					UUID:     "22222222-3333-4444-5555-666666666666",
					Password: "tuic-secret-password",
					SNI:      "sni.tuic.example.com",
					ALPN:     []string{"h3"},
				},
				Active:   true,
				Position: 4,
			},
			{
				LogicalID:   "id-wg",
				DisplayName: "UK-WireGuard",
				Protocol:    domain.ProtocolWireGuard,
				Server:      "198.51.100.77",
				Port:        51820,
				Credentials: domain.InboundProtocolCredential{
					PrivateKey:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
					PublicKey:    "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
					PreSharedKey: "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=",
					LocalAddress: []string{"10.0.0.2/32", "fd00::2/128"},
					Reserved:     []uint8{1, 2, 3},
					MTU:          1400,
				},
				Active:   true,
				Position: 5,
			},
		},
		Groups: []resolver.ResolvedGroup{
			{
				ID:        "g-select",
				Name:      "Proxy",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindGroup, TargetID: "g-urltest", DisplayName: "Auto", Position: 0},
					{Kind: resolver.MemberKindGroup, TargetID: "g-fallback", DisplayName: "FallbackGroup", Position: 1},
					{Kind: resolver.MemberKindNode, TargetID: "id-ss-obfs", DisplayName: "HK-SS-Obfs", Position: 2},
				},
				Position: 0,
			},
			{
				ID:        "g-urltest",
				Name:      "Auto",
				GroupType: domain.GroupTypeURLTest,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "id-ss-obfs", DisplayName: "HK-SS-Obfs", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "id-vmess-legacy", DisplayName: "US-VMess-Legacy", Position: 1},
				},
				Position: 1,
			},
			{
				ID:        "g-fallback",
				Name:      "FallbackGroup",
				GroupType: domain.GroupTypeFallback,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "id-trojan-ws", DisplayName: "JP-Trojan-WS", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "id-ss-obfs", DisplayName: "HK-SS-Obfs", Position: 1},
				},
				Position: 2,
			},
		},
		Rules: []resolver.ResolvedRule{
			{ID: "r1", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "DOMAIN,api.example.com", Position: 0},
			{ID: "r2", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "DOMAIN-SUFFIX,example.org,extended-matching", Position: 1},
			{ID: "r3", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "DOMAIN-KEYWORD,google", Position: 2},
			{ID: "r4", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "IP-CIDR,198.51.100.0/24,no-resolve", Position: 3},
			{ID: "r5", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "IP-CIDR6,2001:db8::/32,no-resolve", Position: 4},
			{ID: "r6", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "GEOIP,CN,no-resolve", Position: 5},
			{ID: "r7", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "PROCESS-NAME,Telegram", Position: 6},
			{ID: "r8", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "SRC-IP,192.168.1.10", Position: 7},
			{ID: "r9", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "DEST-PORT,80-443", Position: 8},
			{ID: "r10", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "IN-PORT,7890", Position: 9},
			{ID: "r11", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "RULE-SET,https://rules.example.com/direct.list,no-resolve", Position: 10},
			{ID: "r12", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "RULE-SET,LAN", Position: 11},
			{ID: "r13", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "USER-AGENT,Instagram*", Position: 12},
			{ID: "r14", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "URL-REGEX,^https://example\\.com/api", Position: 13},
			{ID: "r15", TargetGroupID: "g-select", TargetGroupName: "Proxy", Expression: "MATCH,dns-failed", Position: 14, IsTerminal: true},
		},
	}

	res, err := compiler.Compile(ctx, snapshot, domain.TargetSurge)
	if err != nil {
		t.Fatalf("compile full Surge profile failed: %v", err)
	}
	out := string(res.Content)

	disallowed := map[string]bool{
		"id-ss-obfs":      true,
		"id-vmess-legacy": true,
		"id-trojan-ws":    true,
	}
	parsed, err := parseSurge5Config(out, disallowed)
	if err != nil {
		t.Fatalf("independent Surge 5 parser failed on full profile: %v\nOutput:\n%s", err, out)
	}

	// Verify SS obfs
	ss := parsed.Proxies["HK-SS-Obfs"]
	if ss.Params["obfs"] != "tls" || ss.Params["obfs-host"] != "obfs.example.com" || ss.Params["obfs-uri"] != "/obfs" {
		t.Fatalf("unexpected SS obfs parameters: %+v", ss.Params)
	}

	// Verify VMess legacy alterId > 0 sets vmess-aead=false
	vm := parsed.Proxies["US-VMess-Legacy"]
	if vm.Params["vmess-aead"] != "false" || vm.Params["sni"] != "sni.vmess.example.com" || vm.Params["skip-cert-verify"] != "true" {
		t.Fatalf("unexpected VMess parameters: %+v", vm.Params)
	}

	// Verify Trojan WS
	tr := parsed.Proxies["JP-Trojan-WS"]
	if tr.Params["ws"] != "true" || tr.Params["ws-path"] != "/trojan-ws" || tr.Params["ws-headers"] != "Host:cdn.trojan.example.com" ||
		tr.Params["sni"] != "sni.trojan.example.com" || tr.Params["skip-cert-verify"] != "true" {
		t.Fatalf("unexpected Trojan WS parameters: %+v", tr.Params)
	}

	// Verify Hysteria2, TUIC, and WireGuard
	hy2 := parsed.Proxies["SG-Hysteria2"]
	if hy2.Proto != "hysteria2" || hy2.Params["password"] != "hy2-secret-password" || hy2.Params["download-bandwidth"] != "500" || hy2.Params["sni"] != "sni.hy2.example.com" {
		t.Fatalf("unexpected Hysteria2 parameters: %+v", hy2)
	}
	tuic := parsed.Proxies["DE-TUIC"]
	if tuic.Proto != "tuic" || tuic.Params["uuid"] != "22222222-3333-4444-5555-666666666666" || tuic.Params["password"] != "tuic-secret-password" || tuic.Params["alpn"] != "h3" {
		t.Fatalf("unexpected TUIC parameters: %+v", tuic)
	}
	wg := parsed.Proxies["UK-WireGuard"]
	if wg.Proto != "wireguard" || wg.Params["self-ip"] != "10.0.0.2" || wg.Params["self-ip-v6"] != "fd00::2" || wg.Params["mtu"] != "1400" || wg.Params["client-id"] != "1/2/3" {
		t.Fatalf("unexpected WireGuard parameters: %+v", wg)
	}

	// Verify groups and rules are ignored in node-only Surge output
	if len(parsed.Groups) != 0 || len(parsed.Rules) != 0 {
		t.Fatalf("expected 0 groups and 0 rules in node-only Surge output, got groups=%+v rules=%+v", parsed.Groups, parsed.Rules)
	}
}

func TestSurgeCapabilityAndFailClosedRejections(t *testing.T) {
	ctx := context.Background()

	// 1. Unsupported protocol (VLESS) must fail at exact node index
	unsupportedProtocols := []domain.Protocol{
		domain.ProtocolVLESS,
	}
	for _, proto := range unsupportedProtocols {
		t.Run("UnsupportedProtocol_"+string(proto), func(t *testing.T) {
			snap := fixtureSnapshot()
			snap.Nodes = append(snap.Nodes, resolver.ResolvedNode{
				LogicalID:   "node-unsupported-" + string(proto),
				DisplayName: "Unsupported-" + string(proto),
				Protocol:    proto,
				Active:      true,
				Position:    2,
			})
			_, err := compiler.Compile(ctx, snap, domain.TargetSurge)
			if err == nil {
				t.Fatalf("expected Surge to reject unsupported protocol %s", proto)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("expected CapabilityError, got %T: %v", err, err)
			}
			if capErr.Target != domain.TargetSurge || capErr.Location != "nodes[2]" || capErr.Feature != string(proto) {
				t.Fatalf("unexpected CapabilityError fields: %#v", capErr)
			}
		})
	}

	// 2. Group types (including loadbalance) are ignored in node-only Surge export
	t.Run("GroupType_LoadBalance_Ignored", func(t *testing.T) {
		snap := fixtureSnapshot()
		snap.Groups = append(snap.Groups, resolver.ResolvedGroup{
			ID:        "g-lb",
			Name:      "LB",
			GroupType: domain.GroupTypeLoadBalance,
			Members:   snap.Groups[0].Members,
			Position:  1,
		})
		res, err := compiler.Compile(ctx, snap, domain.TargetSurge)
		if err != nil {
			t.Fatalf("expected loadbalance group to be ignored in node-only Surge export, got %v", err)
		}
		if strings.Contains(string(res.Content), "LB") {
			t.Fatalf("expected group LB to be omitted from Surge output, got:\n%s", string(res.Content))
		}
	})

	// 3. Routing rules are ignored in node-only Surge export
	ignoredRuleCases := []struct {
		name       string
		expression string
	}{
		{name: "GEOSITE", expression: "GEOSITE,category-ads-all"},
		{name: "SRCIPCIDR", expression: "SRC-IP-CIDR,192.168.1.0/24"},
		{name: "DSTPORT", expression: "DST-PORT,443"},
		{name: "BareRuleSetProviderName", expression: "RULE-SET,my-clash-provider"},
	}
	for _, tc := range ignoredRuleCases {
		t.Run("RuleIgnored_"+tc.name, func(t *testing.T) {
			snap := fixtureSnapshot()
			snap.Rules = []resolver.ResolvedRule{
				{
					ID:              "ignored-rule",
					TargetGroupID:   snap.Groups[0].ID,
					TargetGroupName: snap.Groups[0].Name,
					Expression:      tc.expression,
					Position:        0,
				},
				snap.Rules[1],
			}
			res, err := compiler.Compile(ctx, snap, domain.TargetSurge)
			if err != nil {
				t.Fatalf("expected rule %s to be ignored in node-only Surge export, got %v", tc.name, err)
			}
			if strings.Contains(string(res.Content), tc.expression) {
				t.Fatalf("expected rule %s to be omitted from Surge output, got:\n%s", tc.name, string(res.Content))
			}
		})
	}

	// 4. Missing credentials on node must fail closed without fake LogicalID:443
	t.Run("NilCredentialsFailClosed", func(t *testing.T) {
		snap := fixtureSnapshot()
		snap.Nodes[0].Credentials = domain.InboundProtocolCredential{}
		_, err := compiler.Compile(ctx, snap, domain.TargetSurge)
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "nodes[0]" {
			t.Fatalf("expected CapabilityError at nodes[0] when credentials empty, got %v", err)
		}
	})

	// 5. Unsupported transport / cipher / secret non-leakage on error
	t.Run("UnsupportedTransportAndNoSecretLeak", func(t *testing.T) {
		snap := fixtureSnapshot()
		secretUUID := "secret-vmess-uuid-999999"
		snap.Nodes[0].Credentials.UUID = secretUUID
		snap.Nodes[0].Credentials.Transport["network"] = "grpc"
		snap.Nodes[0].Credentials.Transport["service_name"] = "grpc-svc"

		_, err := compiler.Compile(ctx, snap, domain.TargetSurge)
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "nodes[0]" || capErr.Feature != "vmess" {
			t.Fatalf("expected CapabilityError at nodes[0] for vmess grpc, got %v", err)
		}
		if strings.Contains(err.Error(), secretUUID) {
			t.Fatalf("error leaked secret UUID: %v", err)
		}

		// SS unsupported cipher does not leak password
		snap2 := fixtureSnapshot()
		secretPass := "super-secret-ss-password-value"
		snap2.Nodes[1].Credentials.Password = secretPass
		snap2.Nodes[1].Credentials.Method = "unsupported-cipher-xyz"

		_, err = compiler.Compile(ctx, snap2, domain.TargetSurge)
		if !errors.As(err, &capErr) || capErr.Location != "nodes[1]" || capErr.Feature != "ss" {
			t.Fatalf("expected CapabilityError at nodes[1] for ss cipher, got %v", err)
		}
		if strings.Contains(err.Error(), secretPass) {
			t.Fatalf("error leaked secret password: %v", err)
		}
	})
}
