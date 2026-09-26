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

// parseSurge5Config is an independent structural and semantic validator for Surge 5 INI profiles
// based on the Surge 5 Official Manual (https://kb.nssurge.com/surge-knowledge-base/manual/configuration).
func parseSurge5Config(raw string, disallowedLogicalIDs map[string]bool) (*parsedSurgeConfig, error) {
	cfg := &parsedSurgeConfig{
		General: make(map[string]string),
		Proxies: make(map[string]parsedSurgeProxy),
		Groups:  make(map[string]parsedSurgeGroup),
	}

	var section string
	lines := strings.Split(raw, "\n")
	for lineNo, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line
			continue
		}

		switch section {
		case "[General]":
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				return nil, fmt.Errorf("line %d: invalid [General] assignment %q", lineNo+1, line)
			}
			cfg.General[strings.TrimSpace(k)] = strings.TrimSpace(v)

		case "[Proxy]":
			name, rest, ok := strings.Cut(line, "=")
			if !ok {
				return nil, fmt.Errorf("line %d: invalid [Proxy] definition %q", lineNo+1, line)
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
			case "trojan":
				if params["password"] == "" {
					return nil, fmt.Errorf("line %d: trojan proxy %q missing password", lineNo+1, name)
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

		case "[Proxy Group]":
			name, rest, ok := strings.Cut(line, "=")
			if !ok {
				return nil, fmt.Errorf("line %d: invalid [Proxy Group] definition %q", lineNo+1, line)
			}
			name = strings.TrimSpace(name)
			tokens := splitTrimmedCSV(rest)
			if len(tokens) < 2 {
				return nil, fmt.Errorf("line %d: group %q requires type and at least one policy member", lineNo+1, name)
			}
			gType := tokens[0]
			var members []string
			params := make(map[string]string)
			for _, tok := range tokens[1:] {
				if k, v, hasEq := strings.Cut(tok, "="); hasEq {
					params[strings.TrimSpace(k)] = strings.TrimSpace(v)
				} else {
					members = append(members, tok)
				}
			}
			if len(members) == 0 {
				return nil, fmt.Errorf("line %d: group %q has 0 policy members", lineNo+1, name)
			}
			switch gType {
			case "select":
			case "url-test":
				if params["url"] == "" || params["interval"] == "" || params["timeout"] == "" || params["tolerance"] == "" {
					return nil, fmt.Errorf("line %d: url-test group %q missing url/interval/timeout/tolerance: %v", lineNo+1, name, params)
				}
			case "fallback":
				if params["url"] == "" || params["interval"] == "" || params["timeout"] == "" {
					return nil, fmt.Errorf("line %d: fallback group %q missing url/interval/timeout: %v", lineNo+1, name, params)
				}
			default:
				return nil, fmt.Errorf("line %d: unsupported Surge 5 group type %q", lineNo+1, gType)
			}
			cfg.Groups[name] = parsedSurgeGroup{
				Name:    name,
				Type:    gType,
				Members: members,
				Params:  params,
			}

		case "[Rule]":
			tokens := splitTrimmedCSV(line)
			if len(tokens) < 2 {
				return nil, fmt.Errorf("line %d: invalid [Rule] line %q", lineNo+1, line)
			}
			kind := tokens[0]
			if kind == "FINAL" {
				cfg.Rules = append(cfg.Rules, parsedSurgeRule{
					Kind:    "FINAL",
					Policy:  tokens[1],
					Options: tokens[2:],
				})
			} else {
				if len(tokens) < 3 {
					return nil, fmt.Errorf("line %d: rule %q requires kind, value, and policy", lineNo+1, line)
				}
				cfg.Rules = append(cfg.Rules, parsedSurgeRule{
					Kind:    kind,
					Value:   tokens[1],
					Policy:  tokens[2],
					Options: tokens[3:],
				})
			}

		default:
			return nil, fmt.Errorf("line %d: content outside known Surge section (%q): %q", lineNo+1, section, line)
		}
	}

	builtInPolicies := map[string]bool{
		"DIRECT":      true,
		"REJECT":      true,
		"REJECT-DROP": true,
	}

	for _, grp := range cfg.Groups {
		for _, m := range grp.Members {
			if _, isProxy := cfg.Proxies[m]; !isProxy {
				if _, isGroup := cfg.Groups[m]; !isGroup && !builtInPolicies[m] {
					return nil, fmt.Errorf("group %q references undefined policy or proxy %q", grp.Name, m)
				}
			}
		}
	}

	for idx, r := range cfg.Rules {
		if r.Kind == "FINAL" && idx != len(cfg.Rules)-1 {
			return nil, fmt.Errorf("FINAL rule at index %d is not the last rule", idx)
		}
		if _, isGroup := cfg.Groups[r.Policy]; !isGroup {
			if _, isProxy := cfg.Proxies[r.Policy]; !isProxy && !builtInPolicies[r.Policy] {
				return nil, fmt.Errorf("rule %d (%s) references undefined policy %q", idx, r.Kind, r.Policy)
			}
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

	if len(parsed.Rules) != 2 || parsed.Rules[1].Kind != "FINAL" || parsed.Rules[1].Policy != "proxy" {
		t.Fatalf("expected terminal FINAL, proxy rule, got %+v", parsed.Rules)
	}
}

func TestSurgeOutputFormat(t *testing.T) {
	surgeRes, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetSurge)
	if err != nil {
		t.Fatalf("compile Surge failed: %v", err)
	}
	surgeStr := string(surgeRes.Content)
	if !strings.Contains(surgeStr, "[General]") || !strings.Contains(surgeStr, "[Proxy]") ||
		!strings.Contains(surgeStr, "[Proxy Group]") || !strings.Contains(surgeStr, "[Rule]") {
		t.Fatalf("Surge output missing expected sections: %s", surgeStr)
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
		t.Fatalf("expected Surge to support PROCESS-NAME rule: %v", err)
	}
	if !strings.Contains(string(res.Content), "PROCESS-NAME,curl, proxy") {
		t.Fatalf("expected PROCESS-NAME,curl, proxy in output:\n%s", string(res.Content))
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

	// Verify url-test and fallback groups
	autoGrp := parsed.Groups["Auto"]
	if autoGrp.Type != "url-test" || autoGrp.Params["url"] != "http://www.gstatic.com/generate_204" ||
		autoGrp.Params["interval"] != "300" || autoGrp.Params["timeout"] != "5" || autoGrp.Params["tolerance"] != "50" {
		t.Fatalf("unexpected Auto url-test group: %+v", autoGrp)
	}
	fbGrp := parsed.Groups["FallbackGroup"]
	if fbGrp.Type != "fallback" || fbGrp.Params["url"] != "http://www.gstatic.com/generate_204" ||
		fbGrp.Params["interval"] != "300" || fbGrp.Params["timeout"] != "5" {
		t.Fatalf("unexpected FallbackGroup fallback group: %+v", fbGrp)
	}
}

func TestSurgeCapabilityAndFailClosedRejections(t *testing.T) {
	ctx := context.Background()

	// 1. Unsupported protocols (VLESS, Hysteria2, WireGuard, TUIC) must fail at exact node index
	unsupportedProtocols := []domain.Protocol{
		domain.ProtocolVLESS,
		domain.ProtocolHysteria2,
		domain.ProtocolWireGuard,
		domain.ProtocolTUIC,
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

	// 2. Unsupported group type (loadbalance) must fail at exact group index
	t.Run("UnsupportedGroupType_LoadBalance", func(t *testing.T) {
		snap := fixtureSnapshot()
		snap.Groups = append(snap.Groups, resolver.ResolvedGroup{
			ID:        "g-lb",
			Name:      "LB",
			GroupType: domain.GroupTypeLoadBalance,
			Members:   snap.Groups[0].Members,
			Position:  1,
		})
		_, err := compiler.Compile(ctx, snap, domain.TargetSurge)
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "groups[1]" || capErr.Feature != string(domain.GroupTypeLoadBalance) {
			t.Fatalf("expected CapabilityError at groups[1] for loadbalance, got %v", err)
		}
	})

	// 3. Unsupported rules and malformed rule values must fail at exact rule index
	badRuleCases := []struct {
		name       string
		expression string
		wantFeat   string
	}{
		{name: "UnsupportedGEOSITE", expression: "GEOSITE,category-ads-all", wantFeat: "GEOSITE"},
		{name: "UnsupportedSRCIPCIDR", expression: "SRC-IP-CIDR,192.168.1.0/24", wantFeat: "SRC-IP-CIDR"},
		{name: "UnsupportedDSTPORT", expression: "DST-PORT,443", wantFeat: "DST-PORT"},
		{name: "BareRuleSetProviderName", expression: "RULE-SET,my-clash-provider", wantFeat: "RULE-SET"},
		{name: "InvalidIPCIDR", expression: "IP-CIDR,2001:db8::/32", wantFeat: "IP-CIDR"},
		{name: "InvalidIPCIDR6", expression: "IP-CIDR6,10.0.0.0/8", wantFeat: "IP-CIDR6"},
		{name: "InvalidDestPortRange", expression: "DEST-PORT,443-80", wantFeat: "DEST-PORT"},
		{name: "InvalidSrcIP", expression: "SRC-IP,not-an-ip", wantFeat: "SRC-IP"},
		{name: "UnsupportedRuleOption", expression: "IP-CIDR,10.0.0.0/8,unknown-opt", wantFeat: "IP-CIDR"},
	}
	for _, tc := range badRuleCases {
		t.Run("RuleRejection_"+tc.name, func(t *testing.T) {
			snap := fixtureSnapshot()
			snap.Rules = []resolver.ResolvedRule{
				{
					ID:              "bad-rule",
					TargetGroupID:   snap.Groups[0].ID,
					TargetGroupName: snap.Groups[0].Name,
					Expression:      tc.expression,
					Position:        0,
				},
				snap.Rules[1],
			}
			_, err := compiler.Compile(ctx, snap, domain.TargetSurge)
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) || capErr.Location != "rules[0]" || capErr.Feature != tc.wantFeat {
				t.Fatalf("expected CapabilityError at rules[0] feature=%s, got %v", tc.wantFeat, err)
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
