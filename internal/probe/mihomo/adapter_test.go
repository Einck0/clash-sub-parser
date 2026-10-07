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

func TestNodeToMapping_HTTP(t *testing.T) {
	node := domain.Node{
		LogicalID:   "node-synthetic-http-01",
		DisplayName: "Synthetic HTTP Node",
		Protocol:    domain.ProtocolHTTP,
		Server:      "http-proxy.synthetic.test",
		Port:        8080,
		Credentials: domain.InboundProtocolCredential{
			Username: "synthetic-user",
			Password: "synthetic-pass",
			Transport: map[string]string{
				"tls":              "true",
				"sni":              "sni.synthetic.test",
				"skip_cert_verify": "true",
				"headers":          `{"X-Custom-Header":"synthetic-val"}`,
			},
		},
	}

	mapping, err := mihomo.NodeToMapping(node)
	if err != nil {
		t.Fatalf("NodeToMapping failed: %v", err)
	}

	if mapping["type"] != "http" {
		t.Errorf("expected type=http, got %v", mapping["type"])
	}
	if mapping["username"] != "synthetic-user" || mapping["password"] != "synthetic-pass" {
		t.Errorf("credentials mismatch in mapping: user=%v pass=%v", mapping["username"], mapping["password"])
	}
	if mapping["tls"] != true {
		t.Errorf("expected tls=true, got %v", mapping["tls"])
	}
	if mapping["sni"] != "sni.synthetic.test" {
		t.Errorf("expected sni=sni.synthetic.test, got %v", mapping["sni"])
	}
	if mapping["skip-cert-verify"] != true {
		t.Errorf("expected skip-cert-verify=true, got %v", mapping["skip-cert-verify"])
	}

	proxy, err := mihomo.ParseProxy(node)
	if err != nil {
		t.Fatalf("ParseProxy failed: %v", err)
	}
	if proxy.Type().String() != "Http" {
		t.Errorf("expected proxy.Type()=Http, got %s", proxy.Type())
	}
}

func TestNodeToMapping_Socks5(t *testing.T) {
	node := domain.Node{
		LogicalID:   "node-synthetic-socks5-01",
		DisplayName: "Synthetic SOCKS5 Node",
		Protocol:    domain.ProtocolSocks5,
		Server:      "socks5-proxy.synthetic.test",
		Port:        1080,
		Credentials: domain.InboundProtocolCredential{
			Username: "synthetic-user",
			Password: "synthetic-pass",
			Transport: map[string]string{
				"udp":              "true",
				"tls":              "true",
				"sni":              "sni.synthetic.test",
				"skip_cert_verify": "true",
			},
		},
	}

	mapping, err := mihomo.NodeToMapping(node)
	if err != nil {
		t.Fatalf("NodeToMapping failed: %v", err)
	}

	if mapping["type"] != "socks5" {
		t.Errorf("expected type=socks5, got %v", mapping["type"])
	}
	if mapping["udp"] != true {
		t.Errorf("expected udp=true, got %v", mapping["udp"])
	}

	proxy, err := mihomo.ParseProxy(node)
	if err != nil {
		t.Fatalf("ParseProxy failed: %v", err)
	}
	if proxy.Type().String() != "Socks5" {
		t.Errorf("expected proxy.Type()=Socks5, got %s", proxy.Type())
	}
}

func TestNodeToMapping_AnyTLS_Roundtrip(t *testing.T) {
	node := domain.Node{
		LogicalID:   "node-synthetic-anytls-01",
		DisplayName: "Synthetic AnyTLS Node",
		Protocol:    domain.ProtocolAnyTLS,
		Server:      "anytls-proxy.synthetic.test",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			Password: "synthetic-anytls-password",
			Transport: map[string]string{
				"sni":                         "sni.synthetic.test",
				"alpn":                        "h2,http/1.1",
				"fp":                          "chrome",
				"udp":                         "true",
				"skip_cert_verify":            "true",
				"idle-session-check-interval": "30",
				"idle-session-timeout":        "60",
				"min-idle-session":            "2",
				"disable-reuse":               "false",
				"ech-opts":                    `{"enable":true,"query-server-name":"cloudflarechallenge.com"}`,
			},
		},
	}

	mapping, err := mihomo.NodeToMapping(node)
	if err != nil {
		t.Fatalf("NodeToMapping failed: %v", err)
	}

	if mapping["type"] != "anytls" {
		t.Errorf("expected type=anytls, got %v", mapping["type"])
	}
	if mapping["password"] != "synthetic-anytls-password" {
		t.Errorf("password mismatch: %v", mapping["password"])
	}
	if mapping["client-fingerprint"] != "chrome" {
		t.Errorf("fingerprint mismatch: %v", mapping["client-fingerprint"])
	}
	if mapping["idle-session-check-interval"] != 30 {
		t.Errorf("idle-session-check-interval mismatch: %v", mapping["idle-session-check-interval"])
	}

	proxy, err := mihomo.ParseProxy(node)
	if err != nil {
		t.Fatalf("ParseProxy failed: %v", err)
	}
	if proxy.Type().String() != "AnyTLS" {
		t.Errorf("expected proxy.Type()=AnyTLS, got %s", proxy.Type())
	}
}

func TestNodeToMapping_UnsupportedMappingFailureKind(t *testing.T) {
	node := domain.Node{
		LogicalID:   "node-invalid-endpoint",
		DisplayName: "Invalid Endpoint Node",
		Protocol:    domain.ProtocolAnyTLS,
		Server:      "", // Invalid server
		Port:        0,
		Credentials: domain.InboundProtocolCredential{
			Password: "synthetic-pass",
		},
	}

	_, err := mihomo.ParseProxy(node)
	if err == nil {
		t.Fatal("expected error for invalid endpoint")
	}
	// Verify it fails with credentials unavailable (mapping failure), NOT transport timeout
	if err != domain.ErrCredentialsUnavailable {
		t.Errorf("expected ErrCredentialsUnavailable, got: %v", err)
	}
}
