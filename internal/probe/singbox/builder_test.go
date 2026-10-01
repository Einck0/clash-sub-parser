package singbox_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/singbox"
	"github.com/sagernet/sing-box/option"
)

// Task 3.1: TestSingboxBuilder_L4TransportDecoupling_UDP
func TestSingboxBuilder_L4TransportDecoupling_UDP(t *testing.T) {
	// 1. Shadowsocks: default includes UDP support and never injects transport names into L4 Network
	t.Run("shadowsocks_l4_udp", func(t *testing.T) {
		cfg := singbox.NodeConfig{
			LogicalID: "ss-node-1",
			Protocol:  domain.ProtocolSS,
			Server:    "198.51.100.1",
			Port:      8388,
			Method:    "aes-128-gcm",
			Password:  "password",
		}
		out, err := singbox.BuildOutbound(cfg, "ss-tag")
		if err != nil {
			t.Fatalf("BuildOutbound failed: %v", err)
		}
		ssOpt, ok := out.Options.(*option.ShadowsocksOutboundOptions)
		if !ok || ssOpt == nil {
			t.Fatalf("expected Shadowsocks outbound options")
		}
		netStr := string(ssOpt.Network)
		if !strings.Contains(netStr, "udp") || !strings.Contains(netStr, "tcp") {
			t.Fatalf("expected Shadowsocks to support tcp and udp, got: %s", netStr)
		}
	})

	// 2. VLESS with WebSocket: Network is L4 (tcp), Transport is ws
	t.Run("vless_ws_transport_decoupled", func(t *testing.T) {
		cfg := singbox.NodeConfig{
			LogicalID: "vless-ws-node",
			Protocol:  domain.ProtocolVLESS,
			Server:    "198.51.100.2",
			Port:      443,
			UUID:      "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			Network:   "ws", // Old-style input where user or subscription wrote ws in Network
			Transport: map[string]string{
				"network": "ws",
				"path":    "/ws-path",
				"host":    "vless.example.com",
			},
			TLS: true,
		}
		out, err := singbox.BuildOutbound(cfg, "vless-tag")
		if err != nil {
			t.Fatalf("BuildOutbound failed: %v", err)
		}
		vlessOpt, ok := out.Options.(*option.VLESSOutboundOptions)
		if !ok || vlessOpt == nil {
			t.Fatalf("expected VLESS outbound options")
		}
		netStr := string(vlessOpt.Network)
		if strings.Contains(netStr, "ws") {
			t.Fatalf("Network field must NOT contain transport name 'ws', got: %s", netStr)
		}
		if !strings.Contains(netStr, "tcp") {
			t.Fatalf("expected L4 Network 'tcp', got: %s", netStr)
		}
		if vlessOpt.Transport == nil || vlessOpt.Transport.Type != "ws" {
			t.Fatalf("expected Transport Type 'ws', got: %#v", vlessOpt.Transport)
		}
	})

	// 3. VMess with gRPC: Network is L4 (tcp), Transport is grpc
	t.Run("vmess_grpc_transport_decoupled", func(t *testing.T) {
		cfg := singbox.NodeConfig{
			LogicalID: "vmess-grpc-node",
			Protocol:  domain.ProtocolVMess,
			Server:    "198.51.100.3",
			Port:      443,
			UUID:      "b2c3d4e5-f6a1-7890-abcd-ef1234567890",
			Network:   "grpc",
			Transport: map[string]string{
				"network":      "grpc",
				"service_name": "grpc-service",
			},
			TLS: true,
		}
		out, err := singbox.BuildOutbound(cfg, "vmess-tag")
		if err != nil {
			t.Fatalf("BuildOutbound failed: %v", err)
		}
		vmessOpt, ok := out.Options.(*option.VMessOutboundOptions)
		if !ok || vmessOpt == nil {
			t.Fatalf("expected VMess outbound options")
		}
		netStr := string(vmessOpt.Network)
		if strings.Contains(netStr, "grpc") {
			t.Fatalf("Network field must NOT contain transport name 'grpc', got: %s", netStr)
		}
		if !strings.Contains(netStr, "tcp") {
			t.Fatalf("expected L4 Network 'tcp', got: %s", netStr)
		}
		if vmessOpt.Transport == nil || vmessOpt.Transport.Type != "grpc" {
			t.Fatalf("expected Transport Type 'grpc', got: %#v", vmessOpt.Transport)
		}
	})

	// 4. Trojan: supports L4 tcp and udp
	t.Run("trojan_l4_udp", func(t *testing.T) {
		cfg := singbox.NodeConfig{
			LogicalID: "trojan-node",
			Protocol:  domain.ProtocolTrojan,
			Server:    "198.51.100.4",
			Port:      443,
			Password:  "trojan-password",
			Network:   "tcp,udp",
			TLS:       true,
		}
		out, err := singbox.BuildOutbound(cfg, "trojan-tag")
		if err != nil {
			t.Fatalf("BuildOutbound failed: %v", err)
		}
		trojanOpt, ok := out.Options.(*option.TrojanOutboundOptions)
		if !ok || trojanOpt == nil {
			t.Fatalf("expected Trojan outbound options")
		}
		netStr := string(trojanOpt.Network)
		if !strings.Contains(netStr, "tcp") || !strings.Contains(netStr, "udp") {
			t.Fatalf("expected Trojan to support tcp and udp, got: %s", netStr)
		}
	})

	// 5. Hysteria2: Network is L4 udp
	t.Run("hysteria2_l4_udp", func(t *testing.T) {
		cfg := singbox.NodeConfig{
			LogicalID: "hy2-node",
			Protocol:  domain.ProtocolHysteria2,
			Server:    "198.51.100.5",
			Port:      443,
			Password:  "hy2-password",
		}
		out, err := singbox.BuildOutbound(cfg, "hy2-tag")
		if err != nil {
			t.Fatalf("BuildOutbound failed: %v", err)
		}
		hy2Opt, ok := out.Options.(*option.Hysteria2OutboundOptions)
		if !ok || hy2Opt == nil {
			t.Fatalf("expected Hysteria2 outbound options")
		}
		netStr := string(hy2Opt.Network)
		if !strings.Contains(netStr, "udp") {
			t.Fatalf("expected Hysteria2 L4 Network 'udp', got: %s", netStr)
		}
	})
}

// Task 3.2: TestSingboxBuilder_RealityParametersValidation
func TestSingboxBuilder_RealityParametersValidation(t *testing.T) {
	// Standard 32-byte key: 32 bytes of 0x01
	validRaw32 := make([]byte, 32)
	for i := range validRaw32 {
		validRaw32[i] = byte(i + 1)
	}
	validStdB64 := base64.StdEncoding.EncodeToString(validRaw32)
	validURLB64 := base64.RawURLEncoding.EncodeToString(validRaw32)

	baseCfg := singbox.NodeConfig{
		LogicalID: "vless-reality-node",
		Protocol:  domain.ProtocolVLESS,
		Server:    "198.51.100.10",
		Port:      443,
		UUID:      "c3d4e5f6-a1b2-7890-abcd-ef1234567890",
		TLS:       true,
		SNI:       "reality.example.com",
	}

	t.Run("valid_32_byte_keys", func(t *testing.T) {
		// Test StdEncoding
		cfg1 := baseCfg
		cfg1.RealityPublicKey = validStdB64
		cfg1.RealityShortID = "0123456789abcdef" // 16 hex chars = 8 bytes
		out1, err := singbox.BuildOutbound(cfg1, "r-tag-1")
		if err != nil {
			t.Fatalf("expected valid standard base64 key to succeed, got: %v", err)
		}
		vless1 := out1.Options.(*option.VLESSOutboundOptions)
		if vless1.TLS.Reality == nil || !vless1.TLS.Reality.Enabled {
			t.Fatalf("expected Reality enabled")
		}

		// Test RawURLEncoding
		cfg2 := baseCfg
		cfg2.RealityPublicKey = validURLB64
		cfg2.RealityShortID = "0a" // 2 hex chars = 1 byte
		out2, err := singbox.BuildOutbound(cfg2, "r-tag-2")
		if err != nil {
			t.Fatalf("expected valid url-safe base64 key to succeed, got: %v", err)
		}
		vless2 := out2.Options.(*option.VLESSOutboundOptions)
		if vless2.TLS.Reality == nil || vless2.TLS.Reality.ShortID != "0a" {
			t.Fatalf("expected short ID preserved")
		}
	})

	t.Run("invalid_public_key_rejected", func(t *testing.T) {
		// Short key (16 bytes)
		shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
		cfgShort := baseCfg
		cfgShort.RealityPublicKey = shortKey
		if _, err := singbox.BuildOutbound(cfgShort, "r-short"); err == nil {
			t.Fatalf("expected error for 16-byte reality public key, got nil")
		}

		// Long key (33 bytes)
		longKey := base64.StdEncoding.EncodeToString(make([]byte, 33))
		cfgLong := baseCfg
		cfgLong.RealityPublicKey = longKey
		if _, err := singbox.BuildOutbound(cfgLong, "r-long"); err == nil {
			t.Fatalf("expected error for 33-byte reality public key, got nil")
		}

		// Corrupted non-base64 key
		cfgCorrupt := baseCfg
		cfgCorrupt.RealityPublicKey = "not-a-valid-base64-string!@#$%"
		if _, err := singbox.BuildOutbound(cfgCorrupt, "r-corrupt"); err == nil {
			t.Fatalf("expected error for non-base64 reality public key, got nil")
		}
	})

	t.Run("invalid_short_id_rejected_no_strip_or_pad", func(t *testing.T) {
		// Odd length hex string (7 chars) MUST be rejected, NOT padded!
		cfgOdd := baseCfg
		cfgOdd.RealityPublicKey = validURLB64
		cfgOdd.RealityShortID = "0123456"
		if _, err := singbox.BuildOutbound(cfgOdd, "r-odd"); err == nil {
			t.Fatalf("expected error for odd-length short ID, got nil")
		}

		// Invalid hex characters (g-z) MUST be rejected, NOT stripped!
		cfgInvalidHex := baseCfg
		cfgInvalidHex.RealityPublicKey = validURLB64
		cfgInvalidHex.RealityShortID = "0123456g"
		if _, err := singbox.BuildOutbound(cfgInvalidHex, "r-invalid-hex"); err == nil {
			t.Fatalf("expected error for non-hex short ID, got nil")
		}

		// Exceeds 8 bytes (18 hex characters > 16) MUST be rejected!
		cfgTooLong := baseCfg
		cfgTooLong.RealityPublicKey = validURLB64
		cfgTooLong.RealityShortID = "0123456789abcdef01"
		if _, err := singbox.BuildOutbound(cfgTooLong, "r-too-long"); err == nil {
			t.Fatalf("expected error for short ID exceeding 8 bytes (16 hex chars), got nil")
		}
	})
}

// Task 3.3: TestSingboxLibrary_ActualFeatureSupport
func TestSingboxLibrary_ActualFeatureSupport(t *testing.T) {
	// 1. Test xhttp: Sing-box core library does NOT support xhttp transport, returns truthful diagnostic
	t.Run("xhttp_unsupported_diagnostic", func(t *testing.T) {
		cfg := singbox.NodeConfig{
			LogicalID: "vless-xhttp-node",
			Protocol:  domain.ProtocolVLESS,
			Server:    "198.51.100.20",
			Port:      443,
			UUID:      "d4e5f6a1-b2c3-7890-abcd-ef1234567890",
			Network:   "xhttp",
			Transport: map[string]string{
				"network": "xhttp",
				"type":    "xhttp",
				"path":    "/xhttp-path",
			},
			TLS: true,
		}

		_, err := singbox.BuildOutbound(cfg, "xhttp-tag")
		if err == nil {
			t.Fatalf("expected error for unsupported xhttp transport, got nil")
		}
		if !strings.Contains(err.Error(), "xhttp") || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("expected diagnostic mentioning xhttp unsupported, got: %v", err)
		}
	})

	// 2. Test verified supported transports: ws, grpc, http
	t.Run("supported_transports_baseline", func(t *testing.T) {
		supported := []string{"ws", "grpc", "http"}
		for _, tr := range supported {
			cfg := singbox.NodeConfig{
				LogicalID: "vless-" + tr,
				Protocol:  domain.ProtocolVLESS,
				Server:    "198.51.100.21",
				Port:      443,
				UUID:      "e5f6a1b2-c3d4-7890-abcd-ef1234567890",
				Network:   tr,
				Transport: map[string]string{
					"network": tr,
					"path":    "/" + tr,
				},
				TLS: true,
			}
			out, err := singbox.BuildOutbound(cfg, "tag-"+tr)
			if err != nil {
				t.Fatalf("expected transport %q to be supported by sing-box, got err: %v", tr, err)
			}
			vlessOpt := out.Options.(*option.VLESSOutboundOptions)
			if vlessOpt == nil || vlessOpt.Transport == nil {
				t.Fatalf("expected transport options for %q", tr)
			}
		}
	})
}
