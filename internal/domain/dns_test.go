package domain_test

import (
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestDefaultDNSSettings(t *testing.T) {
	def := domain.DefaultDNSSettings()
	if !def.Enabled {
		t.Fatalf("expected DefaultDNSSettings().Enabled to be true")
	}
	if def.Source != "default" {
		t.Fatalf("expected source default, got %q", def.Source)
	}
	if def.Revision != 0 {
		t.Fatalf("expected revision 0, got %d", def.Revision)
	}
	cfg, err := domain.ParseDNSSettings(def.Enabled, def.RawYAML)
	if err != nil {
		t.Fatalf("expected default raw YAML to parse: %v", err)
	}
	if cfg["enable"] != true {
		t.Fatalf("expected enable: true in parsed config")
	}
	servers, ok := cfg["nameserver"].([]any)
	if !ok || len(servers) != 2 || servers[0] != "1.1.1.1" || servers[1] != "8.8.8.8" {
		t.Fatalf("expected default nameservers [1.1.1.1, 8.8.8.8], got %#v", cfg["nameserver"])
	}
}

func TestParseDNSSettings_WrapperAndUnknownKeysPreserved(t *testing.T) {
	raw := `
dns:
  enable: true
  ipv6: false
  enhanced-mode: fake-ip
  nameserver:
    - 1.1.1.1
    - 8.8.8.8
  custom-future-option:
    nested-flag: true
    items:
      - alpha
      - beta
`
	cfg, err := domain.ParseDNSSettings(true, raw)
	if err != nil {
		t.Fatalf("expected parse to succeed: %v", err)
	}
	if cfg["enable"] != true {
		t.Fatalf("expected enable true")
	}
	if cfg["ipv6"] != false {
		t.Fatalf("expected ipv6 false")
	}
	custom, ok := cfg["custom-future-option"].(map[string]any)
	if !ok || custom["nested-flag"] != true {
		t.Fatalf("expected preserved nested custom mapping, got %#v", cfg["custom-future-option"])
	}

	norm, err := domain.NormalizeDNSSettings(true, raw)
	if err != nil {
		t.Fatalf("expected normalize to succeed: %v", err)
	}
	if !strings.Contains(norm, "custom-future-option") {
		t.Fatalf("expected normalized YAML to contain custom option, got %s", norm)
	}
}

func TestParseDNSSettings_EnableSwitchOverridesYAML(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		raw := "enable: " + map[bool]string{true: "false", false: "true"}[enabled] + "\nnameserver:\n  - 1.1.1.1\n"
		cfg, err := domain.ParseDNSSettings(enabled, raw)
		if err != nil {
			t.Fatalf("enabled=%t: expected external switch to override YAML: %v", enabled, err)
		}
		if cfg["enable"] != enabled {
			t.Fatalf("enabled=%t: parsed enable = %#v", enabled, cfg["enable"])
		}
		normalized, err := domain.NormalizeDNSSettings(enabled, raw)
		if err != nil || !strings.Contains(normalized, "enable: "+map[bool]string{true: "true", false: "false"}[enabled]) {
			t.Fatalf("enabled=%t: normalized YAML does not match switch: %q, %v", enabled, normalized, err)
		}
	}
	if _, err := domain.ParseDNSSettings(true, "enable: yes\n"); err == nil {
		t.Fatal("non-boolean YAML enable must be rejected")
	}
}

func TestParseDNSSettings_DisabledState(t *testing.T) {
	raw := `
enable: false
nameserver:
  - 1.1.1.1
`
	cfg, err := domain.ParseDNSSettings(false, raw)
	if err != nil {
		t.Fatalf("expected parse to succeed for disabled DNS: %v", err)
	}
	if cfg["enable"] != false {
		t.Fatalf("expected enable false in parsed result")
	}

	norm, err := domain.NormalizeDNSSettings(false, raw)
	if err != nil {
		t.Fatalf("expected normalize to succeed: %v", err)
	}
	if !strings.Contains(norm, "enable: false") {
		t.Fatalf("expected normalized YAML to have enable: false, got %s", norm)
	}
}

func TestParseDNSSettings_NonMappingRejected(t *testing.T) {
	_, err := domain.ParseDNSSettings(true, "- item1\n- item2\n")
	if err == nil {
		t.Fatalf("expected error for top-level sequence")
	}
	_, err = domain.ParseDNSSettings(true, "just a scalar string\n")
	if err == nil {
		t.Fatalf("expected error for top-level scalar")
	}
}

func TestParseDNSSettings_DisallowedWrappersWithUnrelatedKeys(t *testing.T) {
	raw := `
dns:
  enable: true
proxies:
  - name: dummy
`
	_, err := domain.ParseDNSSettings(true, raw)
	if err == nil {
		t.Fatalf("expected error on wrapper containing unrelated top-level keys")
	}

	rulesRaw := `
dns:
  enable: true
rules:
  - MATCH,DIRECT
`
	_, err = domain.ParseDNSSettings(true, rulesRaw)
	if err == nil {
		t.Fatalf("expected error on wrapper containing rules")
	}
}

func TestParseDNSSettings_MultipleDocumentsRejected(t *testing.T) {
	raw := "enable: true\nnameserver:\n  - 1.1.1.1\n---\nenable: true\n"
	_, err := domain.ParseDNSSettings(true, raw)
	if err == nil {
		t.Fatalf("expected error for multi-document YAML")
	}
}

func TestParseDNSSettings_DuplicateKeysRejected(t *testing.T) {
	raw := "enable: true\nnameserver:\n  - 1.1.1.1\nnameserver:\n  - 8.8.8.8\n"
	_, err := domain.ParseDNSSettings(true, raw)
	if err == nil {
		t.Fatalf("expected error for duplicate YAML keys")
	}
}

func TestParseDNSSettings_SizeLimit(t *testing.T) {
	huge := "nameserver:\n" + strings.Repeat("  - 1.1.1.1\n", 6000) // 6000 * 12 + 12 = 72012 bytes > 64 KiB
	if len(huge) <= 64*1024 {
		t.Fatalf("fixture size must exceed 64KiB, got %d", len(huge))
	}
	_, err := domain.ParseDNSSettings(true, huge)
	if err == nil {
		t.Fatalf("expected error for YAML exceeding 64KiB")
	}
}

func TestParseDNSSettings_AliasesRejected(t *testing.T) {
	for _, raw := range []string{
		"a: &anchor [one, two]\nb: *anchor\n",
		"a: &anchor [*anchor]\n",
	} {
		if _, err := domain.ParseDNSSettings(true, raw); err == nil {
			t.Fatalf("expected aliases to be rejected: %q", raw)
		}
	}
}

func TestParseDNSSettings_InvalidFieldTypes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"enhanced-mode-not-string", "enhanced-mode: [fake-ip]\n"},
		{"ipv6-not-bool", "ipv6: maybe\n"},
		{"nameserver-not-list-of-strings", "nameserver:\n  - 1.1.1.1\n  - 12345\n"},
		{"store-fake-ip-not-bool", "store-fake-ip: yes\n"},
		{"use-system-hosts-not-bool", "use-system-hosts: maybe\n"},
		{"direct-nameserver-follow-policy-not-bool", "direct-nameserver-follow-policy: 1\n"},
		{"fake-ip-filter-not-string-list", "fake-ip-filter: {example.com: true}\n"},
		{"fallback-filter-not-map", "fallback-filter: [geoip]\n"},
		{"nameserver-policy-not-map", "nameserver-policy: [1.1.1.1]\n"},
		{"proxy-server-nameserver-policy-not-map", "proxy-server-nameserver-policy: [1.1.1.1]\n"},
		{"hosts-not-map", "hosts: [1.1.1.1]\n"},
		{"fake-ip-range-not-string", "fake-ip-range: [198.18.0.1/16]\n"},
		{"listen-not-string", "listen: 5353\n"},
		{"cache-algorithm-not-string", "cache-algorithm: true\n"},
		{"store-fake-ip-not-misclassified-as-string", "store-fake-ip: \"true\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.ParseDNSSettings(true, tc.raw)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}
