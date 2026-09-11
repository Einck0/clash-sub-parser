package template_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/compiler/template"
	"clash-sub-parser/internal/domain"
)

func createSampleBundle() *compiler.Bundle {
	nodes := []*domain.Node{
		{
			ID:        1,
			LogicalID: "node_ss_1",
			Name:      "HK-Shadowsocks",
			Protocol:  domain.ProtocolShadowsocks,
			Server:    "hk.example.com",
			Port:      8388,
			NormalizedPayload: map[string]any{
				"cipher":           "aes-256-gcm",
				"password":         "secret-password-1",
				"source_id":        "secret-source-123",
				"admin_token":      "leak-prevention-token-xyz",
				"management_token": "leak-prevention-token-abc",
			},
		},
		{
			ID:        2,
			LogicalID: "node_vmess_1",
			Name:      "US-VMess",
			Protocol:  domain.ProtocolVMess,
			Server:    "us.example.com",
			Port:      443,
			NormalizedPayload: map[string]any{
				"uuid":             "00000000-0000-4000-8000-000000000001",
				"alterId":          0,
				"cipher":           "auto",
				"tls":              true,
				"network":          "ws",
				"ws-opts":          map[string]any{"path": "/ws-path"},
				"management_token": "should-not-be-in-export",
			},
		},
		{
			ID:        3,
			LogicalID: "node_vless_1",
			Name:      "JP-VLESS-Reality",
			Protocol:  domain.ProtocolVLESS,
			Server:    "jp.example.com",
			Port:      443,
			NormalizedPayload: map[string]any{
				"uuid": "00000000-0000-4000-8000-000000000002",
				"tls":  true,
				"reality-opts": map[string]any{
					"public-key": "test-pubkey-reality",
					"short-id":   "815458e4",
				},
				"client-fingerprint": "chrome",
			},
		},
		{
			ID:        4,
			LogicalID: "node_trojan_1",
			Name:      "SG-Trojan",
			Protocol:  domain.ProtocolTrojan,
			Server:    "sg.example.com",
			Port:      443,
			NormalizedPayload: map[string]any{
				"password": "trojan-pass-123",
				"sni":      "sg.example.com",
			},
		},
		{
			ID:        5,
			LogicalID: "node_hy2_1",
			Name:      "TW-Hysteria2",
			Protocol:  domain.ProtocolHysteria2,
			Server:    "tw.example.com",
			Port:      8443,
			NormalizedPayload: map[string]any{
				"password":      "hy2-password-abc",
				"sni":           "tw.example.com",
				"obfs":          "salamander",
				"obfs-password": "obfs-pass-xyz",
			},
		},
		{
			ID:        6,
			LogicalID: "node_tuic_1",
			Name:      "KR-TUIC",
			Protocol:  domain.ProtocolTUIC,
			Server:    "kr.example.com",
			Port:      8443,
			NormalizedPayload: map[string]any{
				"uuid":                  "00000000-0000-4000-8000-000000000003",
				"password":              "tuic-pass-456",
				"congestion-controller": "bbr",
				"sni":                   "kr.example.com",
			},
		},
		{
			ID:        7,
			LogicalID: "node_wg_1",
			Name:      "EU-WireGuard",
			Protocol:  domain.ProtocolWireGuard,
			Server:    "eu.example.com",
			Port:      51820,
			NormalizedPayload: map[string]any{
				"private-key": "private-key-test-base64",
				"public-key":  "public-key-test-base64",
				"ip":          "10.0.0.2",
			},
		},
	}

	groups := []*domain.NodeGroup{
		{
			ID:                1,
			Name:              "PROXY",
			Kind:              domain.GroupKindManual,
			GroupType:         domain.GroupTypeSelect,
			SortOrder:         1,
			ResolvedNodeNames: []string{"HK-Shadowsocks", "US-VMess", "JP-VLESS-Reality", "SG-Trojan", "TW-Hysteria2"},
		},
		{
			ID:        2,
			Name:      "AUTO",
			Kind:      domain.GroupKindAuto,
			GroupType: domain.GroupTypeURLTest,
			SortOrder: 2,
			URLTestConfig: domain.URLTestConfig{
				URL:       "https://cp.cloudflare.com/generate_204",
				Interval:  300,
				Tolerance: 50,
			},
			ResolvedNodeNames: []string{"HK-Shadowsocks", "US-VMess"},
		},
	}

	rules := []*domain.Rule{
		{
			ID:        1,
			Type:      domain.RuleTypeDomainSuffix,
			Value:     "google.com",
			Proxy:     "PROXY",
			SortOrder: 1,
			Enabled:   true,
		},
		{
			ID:        2,
			Type:      domain.RuleTypeGeoSite,
			Value:     "cn",
			Proxy:     "DIRECT",
			SortOrder: 2,
			Enabled:   true,
		},
		{
			ID:        3,
			Type:      domain.RuleTypeIPCIDR,
			Value:     "192.168.0.0/16",
			Proxy:     "DIRECT",
			Options:   []string{"no-resolve"},
			SortOrder: 3,
			Enabled:   true,
		},
		{
			ID:        4,
			Type:      domain.RuleTypeMatch,
			Value:     "",
			Proxy:     "PROXY",
			SortOrder: 4,
			Enabled:   true,
		},
	}

	dns := &domain.DNSConfig{
		Enabled:     true,
		Listen:      "0.0.0.0:1053",
		IPv6:        false,
		Nameservers: []string{"223.5.5.5", "119.29.29.29"},
		Fallback:    []string{"https://1.1.1.1/dns-query"},
	}

	genCfg := domain.DefaultGenerateConfig()

	return &compiler.Bundle{
		Nodes:          nodes,
		Groups:         groups,
		Rules:          rules,
		DNS:            dns,
		GenerateConfig: genCfg,
	}
}

func TestTemplateCompiler_ClashMihomo(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := createSampleBundle()
	ctx := context.Background()

	// Test Mihomo
	res, err := c.Compile(ctx, bundle, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("Compile mihomo error: %v", err)
	}
	if !strings.Contains(res.ContentType, "yaml") {
		t.Errorf("expected yaml content type, got %s", res.ContentType)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(res.Content, &parsed); err != nil {
		t.Fatalf("invalid YAML output: %v\nOutput was:\n%s", err, string(res.Content))
	}

	proxies, ok := parsed["proxies"].([]any)
	if !ok || len(proxies) != len(bundle.Nodes) {
		t.Fatalf("expected %d proxies, got %v", len(bundle.Nodes), proxies)
	}

	groups, ok := parsed["proxy-groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("expected 2 proxy-groups, got %v", groups)
	}

	rules, ok := parsed["rules"].([]any)
	if !ok || len(rules) != 4 {
		t.Fatalf("expected 4 rules, got %v", rules)
	}

	// Verify no secrets leaked
	rawStr := string(res.Content)
	if strings.Contains(rawStr, "secret-source-123") || strings.Contains(rawStr, "leak-prevention-token-xyz") {
		t.Errorf("found leaked management token in output!")
	}
}

func TestTemplateCompiler_SingBox(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := createSampleBundle()
	ctx := context.Background()

	res, err := c.Compile(ctx, bundle, domain.TargetSingBox)
	if err != nil {
		t.Fatalf("Compile sing-box error: %v", err)
	}
	if !strings.Contains(res.ContentType, "json") {
		t.Errorf("expected json content type, got %s", res.ContentType)
	}

	var parsed map[string]any
	if err := json.Unmarshal(res.Content, &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v\nOutput was:\n%s", err, string(res.Content))
	}

	inbounds, ok := parsed["inbounds"].([]any)
	if !ok || len(inbounds) == 0 {
		t.Errorf("expected inbounds in sing-box config")
	}

	outbounds, ok := parsed["outbounds"].([]any)
	if !ok || len(outbounds) < len(bundle.Nodes)+2 {
		t.Errorf("expected outbounds for nodes and groups, got %d", len(outbounds))
	}

	route, ok := parsed["route"].(map[string]any)
	if !ok {
		t.Fatalf("missing route section in sing-box config")
	}
	if route["final"] != "PROXY" {
		t.Errorf("expected final outbound PROXY, got %v", route["final"])
	}

	// Verify no secrets leaked
	rawStr := string(res.Content)
	if strings.Contains(rawStr, "secret-source-123") || strings.Contains(rawStr, "leak-prevention-token-xyz") {
		t.Errorf("found leaked management token in singbox output!")
	}
}

func TestTemplateCompiler_Surge(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := createSampleBundle()
	ctx := context.Background()

	res, err := c.Compile(ctx, bundle, domain.TargetSurge)
	if err != nil {
		t.Fatalf("Compile surge error: %v", err)
	}

	out := string(res.Content)
	if !strings.Contains(out, "[General]") {
		t.Errorf("missing [General] section in Surge config")
	}
	if !strings.Contains(out, "[Proxy]") {
		t.Errorf("missing [Proxy] section in Surge config")
	}
	if !strings.Contains(out, "[Proxy Group]") {
		t.Errorf("missing [Proxy Group] section in Surge config")
	}
	if !strings.Contains(out, "[Rule]") {
		t.Errorf("missing [Rule] section in Surge config")
	}

	// Verify node proxies exist in Surge syntax
	if !strings.Contains(out, "HK-Shadowsocks = ss, hk.example.com, 8388") {
		t.Errorf("missing or invalid Surge shadowsocks line:\n%s", out)
	}
	if !strings.Contains(out, "US-VMess = vmess, us.example.com, 443") {
		t.Errorf("missing or invalid Surge vmess line:\n%s", out)
	}

	// Verify rules
	if !strings.Contains(out, "DOMAIN-SUFFIX,google.com,PROXY") {
		t.Errorf("missing DOMAIN-SUFFIX rule in Surge config")
	}
	if !strings.Contains(out, "FINAL,PROXY") {
		t.Errorf("missing FINAL,PROXY in Surge config")
	}

	// Verify no secrets leaked
	if strings.Contains(out, "secret-source-123") || strings.Contains(out, "leak-prevention-token-xyz") {
		t.Errorf("found leaked management token in surge output!")
	}
}

func TestTemplateCompiler_Loon(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := createSampleBundle()
	ctx := context.Background()

	res, err := c.Compile(ctx, bundle, domain.TargetLoon)
	if err != nil {
		t.Fatalf("Compile loon error: %v", err)
	}

	out := string(res.Content)
	if !strings.Contains(out, "[General]") {
		t.Errorf("missing [General] section in Loon config")
	}
	if !strings.Contains(out, "[Proxy]") {
		t.Errorf("missing [Proxy] section in Loon config")
	}
	if !strings.Contains(out, "[Proxy Group]") {
		t.Errorf("missing [Proxy Group] section in Loon config")
	}
	if !strings.Contains(out, "[Rule]") {
		t.Errorf("missing [Rule] section in Loon config")
	}

	if !strings.Contains(out, "HK-Shadowsocks = Shadowsocks,hk.example.com,8388") {
		t.Errorf("missing or invalid Loon shadowsocks line:\n%s", out)
	}

	if !strings.Contains(out, "FINAL,PROXY") {
		t.Errorf("missing FINAL,PROXY in Loon config")
	}

	// Verify no secrets leaked
	if strings.Contains(out, "secret-source-123") || strings.Contains(out, "leak-prevention-token-xyz") {
		t.Errorf("found leaked management token in loon output!")
	}
}

func TestTemplateCompiler_QuantumultX(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := createSampleBundle()
	ctx := context.Background()

	res, err := c.Compile(ctx, bundle, domain.TargetQuantumultX)
	if err != nil {
		t.Fatalf("Compile QuantumultX error: %v", err)
	}

	out := string(res.Content)
	if !strings.Contains(out, "[general]") {
		t.Errorf("missing [general] section in QX config")
	}
	if !strings.Contains(out, "[server_local]") {
		t.Errorf("missing [server_local] section in QX config")
	}
	if !strings.Contains(out, "[policy]") {
		t.Errorf("missing [policy] section in QX config")
	}
	if !strings.Contains(out, "[filter_local]") {
		t.Errorf("missing [filter_local] section in QX config")
	}

	if !strings.Contains(out, "shadowsocks=hk.example.com:8388") {
		t.Errorf("missing or invalid QX shadowsocks line:\n%s", out)
	}
	if !strings.Contains(out, "tag=HK-Shadowsocks") {
		t.Errorf("missing QX tag in shadowsocks line:\n%s", out)
	}

	if !strings.Contains(out, "final, PROXY") {
		t.Errorf("missing final, PROXY in QX config:\n%s", out)
	}

	// Verify no secrets leaked
	if strings.Contains(out, "secret-source-123") || strings.Contains(out, "leak-prevention-token-xyz") {
		t.Errorf("found leaked management token in QX output!")
	}
}

func TestTemplateEngine_CustomFilters(t *testing.T) {
	engine, err := template.NewEngine()
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 1. base64 filter
	res, err := engine.RenderString(`{{ "hello world"|base64 }}`, nil)
	if err != nil {
		t.Fatalf("render base64 error: %v", err)
	}
	expectedB64 := base64.StdEncoding.EncodeToString([]byte("hello world"))
	if strings.TrimSpace(res) != expectedB64 {
		t.Errorf("base64 filter expected %s, got %s", expectedB64, res)
	}

	// 2. urlencode filter
	res, err = engine.RenderString(`{{ "a b&c=d"|urlencode }}`, nil)
	if err != nil {
		t.Fatalf("render urlencode error: %v", err)
	}
	if strings.TrimSpace(res) != "a+b%26c%3Dd" && strings.TrimSpace(res) != "a%20b%26c%3Dd" {
		t.Errorf("urlencode filter unexpected: %s", res)
	}

	// 3. json filter
	data := map[string]any{"key": "value", "num": 42}
	res, err = engine.RenderString(`{{ data|json }}`, map[string]any{"data": data})
	if err != nil {
		t.Fatalf("render json error: %v", err)
	}
	var unmarshaled map[string]any
	if err := json.Unmarshal([]byte(res), &unmarshaled); err != nil {
		t.Fatalf("json filter produced invalid JSON: %v", err)
	}
	if unmarshaled["key"] != "value" {
		t.Errorf("json filter mismatch: %v", unmarshaled)
	}

	// 4. regex_filter filter
	nodes := []*domain.Node{
		{Name: "HK-01", Server: "hk1.com", Port: 443},
		{Name: "US-01", Server: "us1.com", Port: 443},
		{Name: "HK-02", Server: "hk2.com", Port: 443},
	}
	res, err = engine.RenderString(`{% for n in nodes|regex_filter:"^HK" %}{{ n.Name }},{% endfor %}`, map[string]any{"nodes": nodes})
	if err != nil {
		t.Fatalf("render regex_filter error: %v", err)
	}
	if strings.TrimSpace(res) != "HK-01,HK-02," {
		t.Errorf("regex_filter expected 'HK-01,HK-02,', got %q", res)
	}
}

func TestTemplateCompiler_StashAndClashTargets(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := createSampleBundle()
	ctx := context.Background()

	// Test Stash
	resStash, err := c.Compile(ctx, bundle, domain.TargetStash)
	if err != nil {
		t.Fatalf("Compile stash error: %v", err)
	}
	if resStash.Filename != "stash.yaml" {
		t.Errorf("expected stash.yaml filename, got %s", resStash.Filename)
	}

	// Test Clash
	resClash, err := c.Compile(ctx, bundle, domain.TargetClash)
	if err != nil {
		t.Fatalf("Compile clash error: %v", err)
	}
	if resClash.Filename != "clash.yaml" {
		t.Errorf("expected clash.yaml filename, got %s", resClash.Filename)
	}
	if !strings.Contains(string(resClash.Content), "port: 7890") {
		t.Errorf("expected standard port: 7890 in clash config")
	}
}

func TestTemplateCompiler_UnsupportedTarget(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := createSampleBundle()
	ctx := context.Background()

	_, err = c.Compile(ctx, bundle, domain.ExportTarget("unsupported_xyz"))
	if err == nil {
		t.Fatalf("expected error for unsupported target, got nil")
	}

	_, err = c.Compile(ctx, bundle, domain.ExportTarget("script"))
	if err == nil {
		t.Fatalf("expected error for deprecated script target, got nil")
	}
}

func TestTemplateCompiler_NilBundle(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	ctx := context.Background()
	_, err = c.Compile(ctx, nil, domain.TargetMihomo)
	if err == nil {
		t.Fatalf("expected error for nil bundle, got nil")
	}
}

func TestTemplateCompiler_NilDNSAndEmptyGroups(t *testing.T) {
	c, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := &compiler.Bundle{
		Nodes: []*domain.Node{
			{
				Name:     "Simple-SS",
				Protocol: domain.ProtocolShadowsocks,
				Server:   "1.2.3.4",
				Port:     8388,
				NormalizedPayload: map[string]any{
					"cipher":   "aes-128-gcm",
					"password": "pass",
				},
			},
		},
		Groups: []*domain.NodeGroup{
			{
				Name:              "EmptyGroup",
				GroupType:         domain.GroupTypeSelect,
				ResolvedNodeNames: nil, // empty proxies
			},
		},
		Rules: []*domain.Rule{
			{
				Type:    domain.RuleTypeMatch,
				Proxy:   "EmptyGroup",
				Enabled: true,
			},
		},
		DNS: nil, // nil DNS
	}

	ctx := context.Background()

	for _, target := range []domain.ExportTarget{
		domain.TargetClash,
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetLoon,
		domain.TargetQuantumultX,
	} {
		res, err := c.Compile(ctx, bundle, target)
		if err != nil {
			t.Fatalf("Compile %s with nil DNS failed: %v", target, err)
		}
		if len(res.Content) == 0 {
			t.Errorf("Compile %s returned empty content", target)
		}
	}
}

