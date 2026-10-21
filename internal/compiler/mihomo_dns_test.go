package compiler_test

import (
	"context"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func TestCompileMihomo_DNSEnabledWithUnknownKeys(t *testing.T) {
	ctx := context.Background()
	snap := fixtureSnapshot()
	snap.DNS = resolver.DNSConfig{
		Enabled:     true,
		Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		Fallback:    []string{"1.0.0.1"},
		Config: map[string]any{
			"enable":        true,
			"nameserver":    []any{"1.1.1.1", "8.8.8.8"},
			"fallback":      []any{"1.0.0.1"},
			"enhanced-mode": "fake-ip",
			"fake-ip-range": "198.18.0.1/16",
			"use-hosts":     true,
			"custom-unknown-option": "preserved_value",
		},
	}

	res, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(res.Content, &parsed); err != nil {
		t.Fatalf("failed to unmarshal compiled YAML: %v", err)
	}

	dnsRaw, ok := parsed["dns"]
	if !ok {
		t.Fatalf("expected 'dns' section in compiled Mihomo configuration, got:\n%s", string(res.Content))
	}

	dnsMap, ok := dnsRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected 'dns' to be a mapping, got %T", dnsRaw)
	}

	if dnsMap["enable"] != true {
		t.Fatalf("expected dns.enable=true, got %v", dnsMap["enable"])
	}
	if dnsMap["enhanced-mode"] != "fake-ip" {
		t.Fatalf("expected enhanced-mode fake-ip, got %v", dnsMap["enhanced-mode"])
	}
	if dnsMap["custom-unknown-option"] != "preserved_value" {
		t.Fatalf("expected custom-unknown-option to be preserved, got %v", dnsMap["custom-unknown-option"])
	}

	// Verify exact YAML text output
	yamlText := string(res.Content)
	if !strings.Contains(yamlText, "custom-unknown-option: preserved_value") {
		t.Fatalf("expected raw YAML to contain unknown option, got:\n%s", yamlText)
	}
}

func TestCompileMihomo_DNSDisabled(t *testing.T) {
	ctx := context.Background()
	snap := fixtureSnapshot()
	snap.DNS = resolver.DNSConfig{
		Enabled:     false,
		Nameservers: []string{"1.1.1.1"},
		Config: map[string]any{
			"enable":     false,
			"nameserver": []any{"1.1.1.1"},
			"ipv6":       true,
		},
	}

	res, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(res.Content, &parsed); err != nil {
		t.Fatalf("failed to unmarshal compiled YAML: %v", err)
	}

	dnsRaw, ok := parsed["dns"]
	if !ok {
		t.Fatalf("expected 'dns' section even when disabled, got:\n%s", string(res.Content))
	}

	dnsMap, ok := dnsRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected 'dns' to be a mapping, got %T", dnsRaw)
	}

	if dnsMap["enable"] != false {
		t.Fatalf("expected dns.enable=false, got %v", dnsMap["enable"])
	}
	if dnsMap["ipv6"] != true {
		t.Fatalf("expected ipv6: true preserved, got %v", dnsMap["ipv6"])
	}
}

func TestResolver_DNSChangesDigest(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	baseInput := func(dns resolver.DNSConfig) resolver.ResolveInput {
		return resolver.ResolveInput{
			RevisionID:         "rev-digest-1",
			InventoryWatermark: "wm-1",
			CompilerVersion:    "1.0.0",
			Nodes: []domain.Node{
				{
					LogicalID:   "0123456789abcdef0123456789abcdef",
					DisplayName: "Node A",
					Protocol:    domain.ProtocolTrojan,
					Active:      true,
				},
			},
			Groups: []domain.NodeGroup{
				{
					ID:        "018f0b6e-4d7a-7abc-8def-0123456789ab",
					Name:      "proxy",
					GroupType: domain.GroupTypeSelect,
				},
			},
			GroupSources: map[string][]domain.MemberSource{
				"018f0b6e-4d7a-7abc-8def-0123456789ab": {
					{
						ID:            "018f0b6e-4d7a-7abc-8def-0123456789ac",
						GroupID:       "018f0b6e-4d7a-7abc-8def-0123456789ab",
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						NodeLogicalID: func() *string { s := "0123456789abcdef0123456789abcdef"; return &s }(),
						Position:      0,
					},
				},
			},
			PolicyRules: []domain.PolicyRule{
				{
					ID:            "rule-1",
					TargetGroupID: "018f0b6e-4d7a-7abc-8def-0123456789ab",
					Expression:    "MATCH",
					Position:      0,
				},
			},
			DNS: dns,
		}
	}

	snap1, err := r.Resolve(ctx, baseInput(resolver.DNSConfig{
		Enabled:     true,
		Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		Config: map[string]any{
			"enable":     true,
			"nameserver": []any{"1.1.1.1", "8.8.8.8"},
		},
	}))
	if err != nil {
		t.Fatalf("resolve snap1 failed: %v", err)
	}

	// 1. Changing enabled changes digest
	snap2, err := r.Resolve(ctx, baseInput(resolver.DNSConfig{
		Enabled:     false,
		Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		Config: map[string]any{
			"enable":     false,
			"nameserver": []any{"1.1.1.1", "8.8.8.8"},
		},
	}))
	if err != nil {
		t.Fatalf("resolve snap2 failed: %v", err)
	}

	if snap1.SnapshotDigest == snap2.SnapshotDigest {
		t.Fatalf("expected different snapshot digest when DNS enabled state changes")
	}

	// 2. Changing nameserver changes digest
	snap3, err := r.Resolve(ctx, baseInput(resolver.DNSConfig{
		Enabled:     true,
		Nameservers: []string{"9.9.9.9"},
		Config: map[string]any{
			"enable":     true,
			"nameserver": []any{"9.9.9.9"},
		},
	}))
	if err != nil {
		t.Fatalf("resolve snap3 failed: %v", err)
	}

	if snap1.SnapshotDigest == snap3.SnapshotDigest {
		t.Fatalf("expected different snapshot digest when nameserver changes")
	}

	// 3. Changing custom option in Config changes digest
	snap4, err := r.Resolve(ctx, baseInput(resolver.DNSConfig{
		Enabled:     true,
		Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		Config: map[string]any{
			"enable":        true,
			"nameserver":    []any{"1.1.1.1", "8.8.8.8"},
			"enhanced-mode": "fake-ip",
		},
	}))
	if err != nil {
		t.Fatalf("resolve snap4 failed: %v", err)
	}

	if snap1.SnapshotDigest == snap4.SnapshotDigest {
		t.Fatalf("expected different snapshot digest when Config map changes")
	}
}
