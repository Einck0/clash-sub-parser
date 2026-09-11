package template_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler/template"
	"clash-sub-parser/internal/domain"
)

func TestShadowrocket_LinkSerialization(t *testing.T) {
	nodeSS := &domain.Node{
		ID:        1,
		LogicalID: "node_ss_1",
		Name:      "HK-SS",
		Protocol:  domain.ProtocolShadowsocks,
		Server:    "hk.example.com",
		Port:      8388,
		NormalizedPayload: map[string]any{
			"cipher":           "aes-256-gcm",
			"password":         "secret123",
			"admin_token":      "leak-token",
			"management_token": "leak-mgmt",
		},
	}

	linkSS := template.NodeToShadowrocketLink(nodeSS)
	if !strings.HasPrefix(linkSS, "ss://") {
		t.Fatalf("expected ss:// prefix, got %q", linkSS)
	}
	if strings.Contains(linkSS, "leak-token") || strings.Contains(linkSS, "leak-mgmt") {
		t.Errorf("shadowrocket link contains leaked token: %s", linkSS)
	}

	nodeVMess := &domain.Node{
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
			"ws-opts":          map[string]any{"path": "/ws"},
			"management_token": "leak",
		},
	}

	linkVMess := template.NodeToShadowrocketLink(nodeVMess)
	if !strings.HasPrefix(linkVMess, "vmess://") {
		t.Fatalf("expected vmess:// prefix, got %q", linkVMess)
	}

	nodeVLESS := &domain.Node{
		ID:        3,
		LogicalID: "node_vless_1",
		Name:      "JP-VLESS",
		Protocol:  domain.ProtocolVLESS,
		Server:    "jp.example.com",
		Port:      443,
		NormalizedPayload: map[string]any{
			"uuid": "00000000-0000-4000-8000-000000000002",
			"tls":  true,
			"reality-opts": map[string]any{
				"public-key": "test-pubkey",
				"short-id":   "815458e4",
			},
			"flow": "xtls-rprx-vision",
		},
	}

	linkVLESS := template.NodeToShadowrocketLink(nodeVLESS)
	if !strings.HasPrefix(linkVLESS, "vless://") {
		t.Fatalf("expected vless:// prefix, got %q", linkVLESS)
	}
	if !strings.Contains(linkVLESS, "security=reality") || !strings.Contains(linkVLESS, "pbk=test-pubkey") {
		t.Errorf("vless link missing reality params: %s", linkVLESS)
	}

	nodeTrojan := &domain.Node{
		ID:        4,
		LogicalID: "node_trojan_1",
		Name:      "SG-Trojan",
		Protocol:  domain.ProtocolTrojan,
		Server:    "sg.example.com",
		Port:      443,
		NormalizedPayload: map[string]any{
			"password": "trojan-password",
			"sni":      "sg.example.com",
		},
	}

	linkTrojan := template.NodeToShadowrocketLink(nodeTrojan)
	if !strings.HasPrefix(linkTrojan, "trojan://") {
		t.Fatalf("expected trojan:// prefix, got %q", linkTrojan)
	}

	nodeHy2 := &domain.Node{
		ID:        5,
		LogicalID: "node_hy2_1",
		Name:      "TW-Hysteria2",
		Protocol:  domain.ProtocolHysteria2,
		Server:    "tw.example.com",
		Port:      8443,
		NormalizedPayload: map[string]any{
			"password": "hy2-password",
			"sni":      "tw.example.com",
		},
	}

	linkHy2 := template.NodeToShadowrocketLink(nodeHy2)
	if !strings.HasPrefix(linkHy2, "hysteria2://") {
		t.Fatalf("expected hysteria2:// prefix, got %q", linkHy2)
	}
}

func TestTemplateCompiler_ShadowrocketCompile(t *testing.T) {
	comp, err := template.NewCompiler()
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	bundle := createSampleBundle()
	res, err := comp.Compile(context.Background(), bundle, domain.TargetShadowrocket)
	if err != nil {
		t.Fatalf("failed to compile shadowrocket target: %v", err)
	}

	if res.ContentType != "text/plain; charset=utf-8" {
		t.Errorf("expected ContentType text/plain; charset=utf-8, got %s", res.ContentType)
	}
	if res.Filename != "sub.txt" {
		t.Errorf("expected Filename sub.txt, got %s", res.Filename)
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(res.Content)))
	if err != nil {
		t.Fatalf("res.Content is not valid base64: %v", err)
	}
	decodedText := string(decodedBytes)
	if !strings.Contains(decodedText, "ss://") {
		t.Errorf("decoded shadowrocket missing ss://: %s", decodedText)
	}
	if !strings.Contains(decodedText, "vmess://") {
		t.Errorf("decoded shadowrocket missing vmess://: %s", decodedText)
	}
	if !strings.Contains(decodedText, "vless://") {
		t.Errorf("decoded shadowrocket missing vless://: %s", decodedText)
	}
}
