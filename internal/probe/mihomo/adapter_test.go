package mihomo_test

import (
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/mihomo"
)

func TestNodeToMapping_SupportsSkipCertVerify(t *testing.T) {
	protocols := []struct {
		proto domain.Protocol
		creds domain.InboundProtocolCredential
	}{
		{
			proto: domain.ProtocolVMess,
			creds: domain.InboundProtocolCredential{
				UUID:      "a67dd449-34ba-449e-b924-4f9342738a14",
				Method:    "auto",
				Transport: map[string]string{"tls": "true", "skip-cert-verify": "true"},
			},
		},
		{
			proto: domain.ProtocolVLESS,
			creds: domain.InboundProtocolCredential{
				UUID:      "a67dd449-34ba-449e-b924-4f9342738a14",
				Transport: map[string]string{"tls": "true", "insecure": "1"},
			},
		},
		{
			proto: domain.ProtocolTrojan,
			creds: domain.InboundProtocolCredential{
				Password:  "password123",
				Transport: map[string]string{"skip_cert_verify": "true"},
			},
		},
		{
			proto: domain.ProtocolHysteria2,
			creds: domain.InboundProtocolCredential{
				Password:  "password123",
				Transport: map[string]string{"skip-cert-verify": "true"},
			},
		},
		{
			proto: domain.ProtocolTUIC,
			creds: domain.InboundProtocolCredential{
				UUID:      "a67dd449-34ba-449e-b924-4f9342738a14",
				Password:  "password123",
				Transport: map[string]string{"skip-cert-verify": "true"},
			},
		},
	}

	for _, tc := range protocols {
		t.Run(string(tc.proto), func(t *testing.T) {
			node := domain.Node{
				LogicalID:   "node-test-" + string(tc.proto),
				DisplayName: "Test " + string(tc.proto),
				Protocol:    tc.proto,
				Server:      "1.1.1.1",
				Port:        443,
				Credentials: tc.creds,
			}

			mapping, err := mihomo.NodeToMapping(node)
			if err != nil {
				t.Fatalf("NodeToMapping failed: %v", err)
			}

			skipCert, ok := mapping["skip-cert-verify"].(bool)
			if !ok || !skipCert {
				t.Fatalf("expected skip-cert-verify=true in mapping, got: %v", mapping["skip-cert-verify"])
			}

			proxy, err := mihomo.ParseProxy(node)
			if err != nil {
				t.Fatalf("ParseProxy failed: %v", err)
			}
			if proxy == nil {
				t.Fatal("expected non-nil proxy instance")
			}
		})
	}
}

func TestNodeToMapping_Hysteria2Ports(t *testing.T) {
	node := domain.Node{
		LogicalID:   "node-hy2-ports",
		DisplayName: "Hy2 Ports Test",
		Protocol:    domain.ProtocolHysteria2,
		Server:      "1.1.1.1",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			Password: "secret-hy2-password",
			Transport: map[string]string{
				"server_ports": "20000-30000,443",
				"sni":          "hy2.example.com",
			},
		},
	}

	mapping, err := mihomo.NodeToMapping(node)
	if err != nil {
		t.Fatalf("NodeToMapping failed: %v", err)
	}

	ports, ok := mapping["ports"].(string)
	if !ok || ports != "20000-30000,443" {
		t.Fatalf("expected ports='20000-30000,443', got: %v", mapping["ports"])
	}

	proxy, err := mihomo.ParseProxy(node)
	if err != nil {
		t.Fatalf("ParseProxy failed: %v", err)
	}
	if proxy == nil {
		t.Fatal("expected non-nil proxy instance")
	}
}

func TestNodeToMapping_TUICDisableSNI(t *testing.T) {
	node := domain.Node{
		LogicalID:   "node-tuic-disable-sni",
		DisplayName: "TUIC Disable SNI Test",
		Protocol:    domain.ProtocolTUIC,
		Server:      "1.1.1.1",
		Port:        8443,
		Credentials: domain.InboundProtocolCredential{
			UUID:     "a67dd449-34ba-449e-b924-4f9342738a14",
			Password: "tuic-secret-password",
			Transport: map[string]string{
				"disable-sni": "1",
			},
		},
	}

	mapping, err := mihomo.NodeToMapping(node)
	if err != nil {
		t.Fatalf("NodeToMapping failed: %v", err)
	}

	disableSNI, ok := mapping["disable-sni"].(bool)
	if !ok || !disableSNI {
		t.Fatalf("expected disable-sni=true, got: %v", mapping["disable-sni"])
	}

	proxy, err := mihomo.ParseProxy(node)
	if err != nil {
		t.Fatalf("ParseProxy failed: %v", err)
	}
	if proxy == nil {
		t.Fatal("expected non-nil proxy instance")
	}
}

func TestNodeToMapping_SupportsVLESSXHTTP(t *testing.T) {
	node := domain.Node{
		DisplayName: "VLESS-xhttp",
		Protocol:    domain.ProtocolVLESS,
		Server:      "edge.example.com",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			UUID: "a67dd449-34ba-449e-b924-4f9342738a14",
			Transport: map[string]string{
				"network": "xhttp",
				"path":    "/xhttp-path",
				"host":    "edge.example.com",
				"mode":    "auto",
				"headers": `{"X-Test":"custom"}`,
			},
		},
	}

	mapping, err := mihomo.NodeToMapping(node)
	if err != nil {
		t.Fatalf("NodeToMapping failed: %v", err)
	}

	if mapping["network"] != "xhttp" {
		t.Fatalf("expected network=xhttp, got: %v", mapping["network"])
	}

	xhttpOpts, ok := mapping["xhttp-opts"].(map[string]any)
	if !ok || xhttpOpts == nil {
		t.Fatalf("expected xhttp-opts map in mapping, got: %v", mapping["xhttp-opts"])
	}
	if xhttpOpts["path"] != "/xhttp-path" || xhttpOpts["host"] != "edge.example.com" || xhttpOpts["mode"] != "auto" {
		t.Fatalf("xhttp-opts values mismatch: %v", xhttpOpts)
	}
}
