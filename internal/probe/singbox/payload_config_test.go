package singbox_test

import (
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/probe/singbox"
	"github.com/sagernet/sing-box/option"
)

func TestNodeConfigFromPayload(t *testing.T) {
	norm := parser.NormalizedNode{
		Node:   domain.Node{LogicalID: "test-logical-id", DisplayName: "Test Node", Protocol: domain.ProtocolSS},
		Server: "1.2.3.4", Port: 8388, Transport: map[string]string{"network": "tcp"},
	}
	payload := &domain.NodeCredentialPayload{
		LogicalID: "test-logical-id", Protocol: domain.ProtocolSS,
		Credentials: domain.InboundProtocolCredential{Password: "my-password", Method: "aes-128-gcm"},
	}
	cfg := singbox.NodeConfigFromPayload(norm, payload)
	if cfg.LogicalID != "test-logical-id" || cfg.Password != "my-password" || cfg.Method != "aes-128-gcm" {
		t.Fatalf("config does not preserve identity and credentials: %+v", cfg)
	}
	options, tag, err := singbox.BuildOptions(cfg)
	if err != nil {
		t.Fatalf("BuildOptions error: %v", err)
	}
	if tag != "test-logical-id" || len(options.Outbounds) != 1 {
		t.Fatalf("unexpected options: tag=%s, outbounds=%d", tag, len(options.Outbounds))
	}
}

func TestNodeConfigFromPayloadPreservesUnsafeFlagsForSafeNodeDialerRejection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		protocol  domain.Protocol
		transport map[string]string
		check     func(singbox.NodeConfig) bool
	}{
		{name: "Hy2 ports", protocol: domain.ProtocolHysteria2, transport: map[string]string{"server_ports": "443,8443"}, check: func(c singbox.NodeConfig) bool { return c.Hy2Ports == "443,8443" }},
		{name: "TUIC disable SNI", protocol: domain.ProtocolTUIC, transport: map[string]string{"disable_sni": "true"}, check: func(c singbox.NodeConfig) bool { return c.TUICDisableSNI }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := singbox.NodeConfigFromPayload(parser.NormalizedNode{Node: domain.Node{LogicalID: "n", Protocol: tc.protocol}, Server: "node.example.test", Port: 443}, &domain.NodeCredentialPayload{Protocol: tc.protocol, Credentials: domain.InboundProtocolCredential{Transport: tc.transport}})
			if !tc.check(cfg) {
				t.Fatalf("unsafe flags lost before SafeNodeDialer can reject: %+v", cfg)
			}
		})
	}
}

func TestBuildOptionsRejectsUnsafeProtocolOptions(t *testing.T) {
	cases := []struct {
		name string
		cfg  singbox.NodeConfig
	}{
		{name: "explicit TLS bypass", cfg: singbox.NodeConfig{Protocol: domain.ProtocolTrojan, Server: "93.184.216.34", Port: 443, Password: "secret", SkipCertVerify: true}},
		{name: "transport TLS bypass", cfg: singbox.NodeConfig{Protocol: domain.ProtocolVLESS, Server: "93.184.216.34", Port: 443, UUID: "00000000-0000-0000-0000-000000000001", TLS: true, Transport: map[string]string{"insecure": "true"}}},
		{name: "hysteria port hopping field", cfg: singbox.NodeConfig{Protocol: domain.ProtocolHysteria2, Server: "93.184.216.34", Port: 443, Password: "secret", Hy2Ports: "443,8443"}},
		{name: "hysteria port hopping transport", cfg: singbox.NodeConfig{Protocol: domain.ProtocolHysteria2, Server: "93.184.216.34", Port: 443, Password: "secret", Transport: map[string]string{"server_ports": "443,8443"}}},
		{name: "tuic disable SNI field", cfg: singbox.NodeConfig{Protocol: domain.ProtocolTUIC, Server: "93.184.216.34", Port: 443, UUID: "00000000-0000-0000-0000-000000000001", Password: "secret", TUICDisableSNI: true}},
		{name: "tuic disable SNI transport", cfg: singbox.NodeConfig{Protocol: domain.ProtocolTUIC, Server: "93.184.216.34", Port: 443, UUID: "00000000-0000-0000-0000-000000000001", Password: "secret", Transport: map[string]string{"disable_sni": "true"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := singbox.BuildOptions(tc.cfg); err == nil {
				t.Fatal("unsafe protocol option accepted")
			}
		})
	}
}

func TestNodeConfigFromPayloadPreservesHostnameIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, network, explicitHost, wantHost string
	}{
		{name: "websocket", network: "ws", wantHost: "edge.example.test"},
		{name: "http", network: "http", explicitHost: "route.example.test", wantHost: "route.example.test"},
		{name: "http upgrade", network: "httpupgrade", wantHost: "edge.example.test"},
		{name: "grpc", network: "grpc", wantHost: "edge.example.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := map[string]string{"network": tc.network, "tls": "true"}
			if tc.explicitHost != "" {
				transport["host"] = tc.explicitHost
			}
			cfg := singbox.NodeConfigFromPayload(parser.NormalizedNode{
				Node:   domain.Node{LogicalID: "test", Protocol: domain.ProtocolVLESS},
				Server: "edge.example.test", Port: 443, Transport: transport,
			}, &domain.NodeCredentialPayload{Protocol: domain.ProtocolVLESS, Credentials: domain.InboundProtocolCredential{UUID: "00000000-0000-0000-0000-000000000001"}})
			if cfg.SNI != "edge.example.test" {
				t.Fatalf("SNI=%q", cfg.SNI)
			}
			if cfg.Headers["Host"] != tc.wantHost {
				t.Fatalf("Host=%q, want %q", cfg.Headers["Host"], tc.wantHost)
			}
			cfg.Server = "93.184.216.34" // same socket target after SafeNodeDialer validation/pinning
			options, _, err := singbox.BuildOptions(cfg)
			if err != nil {
				t.Fatal(err)
			}
			out, ok := options.Outbounds[0].Options.(*option.VLESSOutboundOptions)
			if !ok {
				t.Fatalf("unexpected outbound options %T", options.Outbounds[0].Options)
			}
			if out.Server != "93.184.216.34" {
				t.Fatalf("socket server=%q", out.Server)
			}
			if out.TLS == nil || out.TLS.ServerName != "edge.example.test" || out.TLS.Insecure {
				t.Fatalf("TLS identity not strict/preserved: %+v", out.TLS)
			}
		})
	}
}
