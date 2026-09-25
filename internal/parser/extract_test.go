package parser_test

import (
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
)

func TestExtractWithCredentialsYAML(t *testing.T) {
	content := readFixture(t, "testdata/clash.yaml")
	res, err := parser.ExtractWithCredentials(content)
	if err != nil {
		t.Fatalf("ExtractWithCredentials error: %v", err)
	}
	if len(res.Items) != 7 {
		t.Fatalf("expected 7 items, got %d", len(res.Items))
	}

	for _, item := range res.Items {
		switch item.Normalized.Node.Protocol {
		case domain.ProtocolSS:
			if item.Credentials.Password != "ss-password" || item.Credentials.Method != "aes-256-gcm" {
				t.Errorf("SS credentials mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolVMess:
			if item.Credentials.UUID != "11111111-1111-1111-1111-111111111111" {
				t.Errorf("VMess UUID mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolVLESS:
			if item.Credentials.UUID != "22222222-2222-2222-2222-222222222222" {
				t.Errorf("VLESS UUID mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolTrojan:
			if item.Credentials.Password != "trojan-password" {
				t.Errorf("Trojan password mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolHysteria2:
			if item.Credentials.Password != "hy2-password" {
				t.Errorf("Hysteria2 password mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolWireGuard:
			if item.Credentials.PrivateKey != "wireguard-private-key" || item.Credentials.PublicKey != "wireguard-public-key" {
				t.Errorf("WireGuard mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolTUIC:
			if item.Credentials.UUID != "33333333-3333-3333-3333-333333333333" || item.Credentials.Password != "tuic-password" {
				t.Errorf("TUIC mismatch: %+v", item.Credentials)
			}
		}
	}
}

func TestExtractWithCredentialsURLLines(t *testing.T) {
	content := readFixture(t, "testdata/subscription.txt")
	res, err := parser.ExtractWithCredentials(content)
	if err != nil {
		t.Fatalf("ExtractWithCredentials error: %v", err)
	}
	if len(res.Items) != 7 {
		t.Fatalf("expected 7 items, got %d", len(res.Items))
	}
	if res.Rejected != 1 {
		t.Fatalf("expected 1 rejected (unsupported://), got %d", res.Rejected)
	}

	for _, item := range res.Items {
		switch item.Normalized.Node.Protocol {
		case domain.ProtocolSS:
			if item.Credentials.Password != "ss-password" || item.Credentials.Method != "aes-256-gcm" {
				t.Errorf("SS credentials mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolVMess:
			if item.Credentials.UUID != "11111111-1111-1111-1111-111111111111" {
				t.Errorf("VMess UUID mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolVLESS:
			if item.Credentials.UUID != "22222222-2222-2222-2222-222222222222" {
				t.Errorf("VLESS UUID mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolTrojan:
			if item.Credentials.Password != "trojan-password" {
				t.Errorf("Trojan password mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolHysteria2:
			if item.Credentials.Password != "hy2-password" {
				t.Errorf("Hysteria2 password mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolWireGuard:
			if item.Credentials.PrivateKey != "wireguard-private-key" || item.Credentials.PublicKey != "wireguard-public-key" {
				t.Errorf("WireGuard mismatch: %+v", item.Credentials)
			}
		case domain.ProtocolTUIC:
			if item.Credentials.UUID != "33333333-3333-3333-3333-333333333333" || item.Credentials.Password != "tuic-password" {
				t.Errorf("TUIC mismatch: %+v", item.Credentials)
			}
		}
	}
}
