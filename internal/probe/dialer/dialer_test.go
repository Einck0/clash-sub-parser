package dialer_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/dialer"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

func TestBuildOutbound_Shadowsocks(t *testing.T) {
	node := &domain.Node{
		ID:        1,
		LogicalID: "ss-node-1",
		Name:      "SS-Test",
		Protocol:  domain.ProtocolShadowsocks,
		Server:    "192.0.2.1",
		Port:      8388,
		NormalizedPayload: map[string]any{
			"cipher":   "aes-128-gcm",
			"password": "secret-password",
			"udp":      true,
		},
	}

	out, err := dialer.BuildOutbound(node)
	if err != nil {
		t.Fatalf("BuildOutbound failed: %v", err)
	}

	if out.Type != C.TypeShadowsocks {
		t.Errorf("expected type %q, got %q", C.TypeShadowsocks, out.Type)
	}

	ssOpts, ok := out.Options.(*option.ShadowsocksOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.ShadowsocksOutboundOptions, got %T", out.Options)
	}
	if ssOpts.Server != "192.0.2.1" || ssOpts.ServerPort != 8388 {
		t.Errorf("unexpected server/port: %s:%d", ssOpts.Server, ssOpts.ServerPort)
	}
	if ssOpts.Method != "aes-128-gcm" {
		t.Errorf("expected method aes-128-gcm, got %q", ssOpts.Method)
	}
	if ssOpts.Password != "secret-password" {
		t.Errorf("expected password secret-password, got %q", ssOpts.Password)
	}
}

func TestBuildOutbound_Shadowsocks_2022(t *testing.T) {
	node := &domain.Node{
		ID:        11,
		LogicalID: "ss-2022-node",
		Name:      "SS-2022-Test",
		Protocol:  domain.ProtocolShadowsocks,
		Server:    "192.0.2.2",
		Port:      8389,
		NormalizedPayload: map[string]any{
			"cipher":   "2022-blake3-aes-128-gcm",
			"password": "uFk+3W129Q86x4M5L7pA==",
		},
	}

	out, err := dialer.BuildOutbound(node)
	if err != nil {
		t.Fatalf("BuildOutbound failed: %v", err)
	}

	ssOpts, ok := out.Options.(*option.ShadowsocksOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.ShadowsocksOutboundOptions, got %T", out.Options)
	}
	if ssOpts.Method != "2022-blake3-aes-128-gcm" {
		t.Errorf("expected 2022-blake3-aes-128-gcm, got %q", ssOpts.Method)
	}
}

func TestBuildOutbound_VMess(t *testing.T) {
	node := &domain.Node{
		ID:        2,
		LogicalID: "vmess-node-1",
		Name:      "VMess-Test",
		Protocol:  domain.ProtocolVMess,
		Server:    "example.com",
		Port:      443,
		NormalizedPayload: map[string]any{
			"uuid":             "b831381d-6324-4d53-ad4f-8cda48b30811",
			"alterId":          0,
			"cipher":           "auto",
			"network":          "ws",
			"tls":              true,
			"servername":       "vmess.example.com",
			"skip-cert-verify": true,
			"ws-opts": map[string]any{
				"path": "/chat",
				"headers": map[string]string{
					"Host": "vmess.example.com",
				},
			},
		},
	}

	out, err := dialer.BuildOutbound(node)
	if err != nil {
		t.Fatalf("BuildOutbound failed: %v", err)
	}

	if out.Type != C.TypeVMess {
		t.Errorf("expected type %q, got %q", C.TypeVMess, out.Type)
	}

	vOpts, ok := out.Options.(*option.VMessOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.VMessOutboundOptions, got %T", out.Options)
	}
	if vOpts.Server != "example.com" || vOpts.ServerPort != 443 {
		t.Errorf("unexpected server/port: %s:%d", vOpts.Server, vOpts.ServerPort)
	}
	if vOpts.UUID != "b831381d-6324-4d53-ad4f-8cda48b30811" {
		t.Errorf("unexpected uuid: %s", vOpts.UUID)
	}
	if vOpts.TLS == nil || !vOpts.TLS.Enabled || vOpts.TLS.ServerName != "vmess.example.com" || !vOpts.TLS.Insecure {
		t.Errorf("unexpected TLS options: %+v", vOpts.TLS)
	}
	if vOpts.Transport == nil || vOpts.Transport.Type != C.V2RayTransportTypeWebsocket {
		t.Fatalf("expected websocket transport, got %+v", vOpts.Transport)
	}
	if vOpts.Transport.WebsocketOptions.Path != "/chat" {
		t.Errorf("expected ws path /chat, got %q", vOpts.Transport.WebsocketOptions.Path)
	}
}

func TestBuildOutbound_VMess_GRPC(t *testing.T) {
	node := &domain.Node{
		ID:        22,
		LogicalID: "vmess-grpc",
		Name:      "VMess-gRPC",
		Protocol:  domain.ProtocolVMess,
		Server:    "grpc.example.com",
		Port:      443,
		NormalizedPayload: map[string]any{
			"uuid":    "b831381d-6324-4d53-ad4f-8cda48b30811",
			"network": "grpc",
			"grpc-opts": map[string]any{
				"service-name": "GunService",
			},
		},
	}

	out, err := dialer.BuildOutbound(node)
	if err != nil {
		t.Fatalf("BuildOutbound failed: %v", err)
	}

	vOpts := out.Options.(*option.VMessOutboundOptions)
	if vOpts.Transport == nil || vOpts.Transport.Type != C.V2RayTransportTypeGRPC {
		t.Fatalf("expected grpc transport, got %+v", vOpts.Transport)
	}
	if vOpts.Transport.GRPCOptions.ServiceName != "GunService" {
		t.Errorf("expected service name GunService, got %q", vOpts.Transport.GRPCOptions.ServiceName)
	}
}

func TestBuildOutbound_VLESS_Reality(t *testing.T) {
	node := &domain.Node{
		ID:        3,
		LogicalID: "vless-reality-1",
		Name:      "VLESS-Reality-Test",
		Protocol:  domain.ProtocolVLESS,
		Server:    "reality.example.com",
		Port:      8443,
		NormalizedPayload: map[string]any{
			"uuid":               "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
			"flow":               "xtls-rprx-vision",
			"tls":                true,
			"servername":         "yahoo.com",
			"client-fingerprint": "chrome",
			"reality-opts": map[string]any{
				"public-key": "abcd1234efgh5678ijkl9012mnop3456qrst7890",
				"short-id":   "0123456789abcdef",
			},
		},
	}

	out, err := dialer.BuildOutbound(node)
	if err != nil {
		t.Fatalf("BuildOutbound failed: %v", err)
	}

	if out.Type != C.TypeVLESS {
		t.Errorf("expected type %q, got %q", C.TypeVLESS, out.Type)
	}

	vlOpts, ok := out.Options.(*option.VLESSOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.VLESSOutboundOptions, got %T", out.Options)
	}
	if vlOpts.Server != "reality.example.com" || vlOpts.ServerPort != 8443 {
		t.Errorf("unexpected server/port: %s:%d", vlOpts.Server, vlOpts.ServerPort)
	}
	if vlOpts.UUID != "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d" {
		t.Errorf("unexpected uuid: %s", vlOpts.UUID)
	}
	if vlOpts.Flow != "xtls-rprx-vision" {
		t.Errorf("expected flow xtls-rprx-vision, got %q", vlOpts.Flow)
	}
	if vlOpts.TLS == nil || !vlOpts.TLS.Enabled {
		t.Fatalf("expected TLS enabled")
	}
	if vlOpts.TLS.ServerName != "yahoo.com" {
		t.Errorf("expected SNI yahoo.com, got %q", vlOpts.TLS.ServerName)
	}
	if vlOpts.TLS.UTLS == nil || !vlOpts.TLS.UTLS.Enabled || vlOpts.TLS.UTLS.Fingerprint != "chrome" {
		t.Errorf("expected uTLS chrome fingerprint, got %+v", vlOpts.TLS.UTLS)
	}
	if vlOpts.TLS.Reality == nil || !vlOpts.TLS.Reality.Enabled {
		t.Fatalf("expected Reality enabled")
	}
	if vlOpts.TLS.Reality.PublicKey != "abcd1234efgh5678ijkl9012mnop3456qrst7890" {
		t.Errorf("unexpected reality public key: %q", vlOpts.TLS.Reality.PublicKey)
	}
	if vlOpts.TLS.Reality.ShortID != "0123456789abcdef" {
		t.Errorf("unexpected reality short ID: %q", vlOpts.TLS.Reality.ShortID)
	}
}

func TestBuildOutbound_Trojan(t *testing.T) {
	node := &domain.Node{
		ID:        4,
		LogicalID: "trojan-1",
		Name:      "Trojan-Test",
		Protocol:  domain.ProtocolTrojan,
		Server:    "trojan.example.com",
		Port:      443,
		NormalizedPayload: map[string]any{
			"password":         "trojan-password",
			"sni":              "trojan.example.com",
			"skip-cert-verify": false,
			"alpn":             []string{"h2", "http/1.1"},
			"network":          "ws",
			"ws-opts": map[string]any{
				"path": "/trojan-ws",
			},
		},
	}

	out, err := dialer.BuildOutbound(node)
	if err != nil {
		t.Fatalf("BuildOutbound failed: %v", err)
	}

	if out.Type != C.TypeTrojan {
		t.Errorf("expected type %q, got %q", C.TypeTrojan, out.Type)
	}

	trOpts, ok := out.Options.(*option.TrojanOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.TrojanOutboundOptions, got %T", out.Options)
	}
	if trOpts.Password != "trojan-password" {
		t.Errorf("expected password trojan-password, got %q", trOpts.Password)
	}
	if trOpts.TLS == nil || !trOpts.TLS.Enabled || trOpts.TLS.ServerName != "trojan.example.com" {
		t.Errorf("unexpected TLS options: %+v", trOpts.TLS)
	}
	if len(trOpts.TLS.ALPN) != 2 || trOpts.TLS.ALPN[0] != "h2" {
		t.Errorf("unexpected ALPN: %+v", trOpts.TLS.ALPN)
	}
	if trOpts.Transport == nil || trOpts.Transport.Type != C.V2RayTransportTypeWebsocket {
		t.Errorf("expected ws transport, got %+v", trOpts.Transport)
	}
}

func TestBuildOutbound_Hysteria2(t *testing.T) {
	node := &domain.Node{
		ID:        5,
		LogicalID: "hy2-1",
		Name:      "Hy2-Test",
		Protocol:  domain.ProtocolHysteria2,
		Server:    "hy2.example.com",
		Port:      443,
		NormalizedPayload: map[string]any{
			"password":      "hy2-secret",
			"sni":           "hy2.example.com",
			"ports":         "20000-30000",
			"obfs":          "salamander",
			"obfs-password": "obfs-password-123",
		},
	}

	out, err := dialer.BuildOutbound(node)
	if err != nil {
		t.Fatalf("BuildOutbound failed: %v", err)
	}

	if out.Type != C.TypeHysteria2 {
		t.Errorf("expected type %q, got %q", C.TypeHysteria2, out.Type)
	}

	hyOpts, ok := out.Options.(*option.Hysteria2OutboundOptions)
	if !ok {
		t.Fatalf("expected *option.Hysteria2OutboundOptions, got %T", out.Options)
	}
	if hyOpts.Password != "hy2-secret" {
		t.Errorf("expected password hy2-secret, got %q", hyOpts.Password)
	}
	if hyOpts.TLS == nil || !hyOpts.TLS.Enabled || hyOpts.TLS.ServerName != "hy2.example.com" {
		t.Errorf("unexpected TLS options: %+v", hyOpts.TLS)
	}
	if hyOpts.Obfs == nil || hyOpts.Obfs.Type != "salamander" || hyOpts.Obfs.Password != "obfs-password-123" {
		t.Errorf("unexpected obfs options: %+v", hyOpts.Obfs)
	}
	if len(hyOpts.ServerPorts) != 1 || hyOpts.ServerPorts[0] != "20000-30000" {
		t.Errorf("unexpected server ports: %+v", hyOpts.ServerPorts)
	}
}

func TestBuildOutbound_TUIC(t *testing.T) {
	node := &domain.Node{
		ID:        55,
		LogicalID: "tuic-1",
		Name:      "TUIC-Test",
		Protocol:  domain.ProtocolTUIC,
		Server:    "tuic.example.com",
		Port:      8443,
		NormalizedPayload: map[string]any{
			"uuid":                  "c9d8e7f6-a5b4-3210-9876-fedcba098765",
			"password":              "tuic-pass",
			"congestion_controller": "bbr",
			"udp_relay_mode":        "quic",
			"sni":                   "tuic.example.com",
			"alpn":                  []string{"h3"},
		},
	}

	out, err := dialer.BuildOutbound(node)
	if err != nil {
		t.Fatalf("BuildOutbound failed: %v", err)
	}

	if out.Type != C.TypeTUIC {
		t.Errorf("expected type %q, got %q", C.TypeTUIC, out.Type)
	}

	tuicOpts, ok := out.Options.(*option.TUICOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.TUICOutboundOptions, got %T", out.Options)
	}
	if tuicOpts.UUID != "c9d8e7f6-a5b4-3210-9876-fedcba098765" {
		t.Errorf("unexpected uuid: %s", tuicOpts.UUID)
	}
	if tuicOpts.CongestionControl != "bbr" {
		t.Errorf("expected congestion control bbr, got %q", tuicOpts.CongestionControl)
	}
	if tuicOpts.UDPRelayMode != "quic" {
		t.Errorf("expected udp relay mode quic, got %q", tuicOpts.UDPRelayMode)
	}
}

func TestBuildEndpoint_WireGuard(t *testing.T) {
	node := &domain.Node{
		ID:        6,
		LogicalID: "wg-1",
		Name:      "WG-Test",
		Protocol:  domain.ProtocolWireGuard,
		Server:    "wg.example.com",
		Port:      51820,
		NormalizedPayload: map[string]any{
			"private-key": "aW52YWxpZF9wcml2YXRlX2tleV9mb3JfdGVzdF8xMjM=",
			"public-key":  "aW52YWxpZF9wdWJsaWNfa2V5X2Zvcl90ZXN0XzEyMw==",
			"ip":          "10.0.0.2",
			"mtu":         1420,
			"reserved":    []int{1, 2, 3},
		},
	}

	ep, err := dialer.BuildEndpoint(node)
	if err != nil {
		t.Fatalf("BuildEndpoint failed: %v", err)
	}

	if ep.Type != C.TypeWireGuard {
		t.Errorf("expected endpoint type %q, got %q", C.TypeWireGuard, ep.Type)
	}

	wgOpts, ok := ep.Options.(*option.WireGuardEndpointOptions)
	if !ok {
		t.Fatalf("expected *option.WireGuardEndpointOptions, got %T", ep.Options)
	}
	if wgOpts.PrivateKey != "aW52YWxpZF9wcml2YXRlX2tleV9mb3JfdGVzdF8xMjM=" {
		t.Errorf("unexpected private key: %q", wgOpts.PrivateKey)
	}
	if wgOpts.MTU != 1420 {
		t.Errorf("expected MTU 1420, got %d", wgOpts.MTU)
	}
	if len(wgOpts.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(wgOpts.Peers))
	}
	peer := wgOpts.Peers[0]
	if peer.Address != "wg.example.com" || peer.Port != 51820 {
		t.Errorf("unexpected peer addr: %s:%d", peer.Address, peer.Port)
	}
	if len(peer.Reserved) != 3 || peer.Reserved[0] != 1 || peer.Reserved[1] != 2 || peer.Reserved[2] != 3 {
		t.Errorf("unexpected reserved: %+v", peer.Reserved)
	}
}

func TestDialer_LifecycleAndInterface(t *testing.T) {
	node := &domain.Node{
		ID:        7,
		LogicalID: "ss-node-lifecycle",
		Name:      "SS-Lifecycle",
		Protocol:  domain.ProtocolShadowsocks,
		Server:    "127.0.0.1",
		Port:      18388,
		NormalizedPayload: map[string]any{
			"cipher":   "aes-128-gcm",
			"password": "test-password-12345",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d, err := dialer.NewDialer(ctx, node)
	if err != nil {
		t.Fatalf("NewDialer failed: %v", err)
	}
	defer d.Close()

	if d.Outbound() == nil {
		t.Fatalf("expected non-nil Outbound()")
	}

	if d.Outbound().Type() != C.TypeShadowsocks {
		t.Errorf("expected outbound type %q, got %q", C.TypeShadowsocks, d.Outbound().Type())
	}

	// Idempotent close check
	if err := d.Close(); err != nil {
		t.Errorf("first Close() returned error: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Errorf("second Close() returned error: %v", err)
	}
}

func TestDialer_EndToEnd_HTTPTunnel(t *testing.T) {
	// 1. Start target HTTP service
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for target: %v", err)
	}
	defer targetListener.Close()
	targetPort := targetListener.Addr().(*net.TCPAddr).Port

	targetMux := http.NewServeMux()
	targetMux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Egress-Check", "passed")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong-from-target"))
	})
	targetServer := &http.Server{Handler: targetMux}
	go func() {
		_ = targetServer.Serve(targetListener)
	}()
	defer targetServer.Close()

	// 2. Start mock HTTP CONNECT proxy server
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for proxy: %v", err)
	}
	defer proxyListener.Close()
	proxyPort := proxyListener.Addr().(*net.TCPAddr).Port

	proxyServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect {
				http.Error(w, "only CONNECT supported", http.StatusBadRequest)
				return
			}
			destConn, err := net.DialTimeout("tcp", r.Host, 2*time.Second)
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				destConn.Close()
				http.Error(w, "hijacking not supported", http.StatusInternalServerError)
				return
			}
			clientConn, _, err := hijacker.Hijack()
			if err != nil {
				destConn.Close()
				return
			}
			go func() {
				defer destConn.Close()
				defer clientConn.Close()
				_, _ = io.Copy(destConn, clientConn)
			}()
			go func() {
				defer destConn.Close()
				defer clientConn.Close()
				_, _ = io.Copy(clientConn, destConn)
			}()
		}),
	}
	go func() {
		_ = proxyServer.Serve(proxyListener)
	}()
	defer proxyServer.Close()

	// 3. Construct sing-box HTTP proxy node
	node := &domain.Node{
		ID:        80,
		LogicalID: "http-proxy-test",
		Name:      "HTTP-Proxy-Test",
		Protocol:  domain.ProtocolHTTP,
		Server:    "127.0.0.1",
		Port:      proxyPort,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d, err := dialer.NewDialer(ctx, node)
	if err != nil {
		t.Fatalf("NewDialer failed: %v", err)
	}
	defer d.Close()

	// 4. Dial through sing-box pure in-memory dialer to target server
	targetAddr := fmt.Sprintf("127.0.0.1:%d", targetPort)
	conn, err := d.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		t.Fatalf("DialContext failed through proxy: %v", err)
	}
	defer conn.Close()

	// Write HTTP request
	reqStr := fmt.Sprintf("GET /ping HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", targetAddr)
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
	if string(bodyBytes) != "pong-from-target" {
		t.Errorf("unexpected body: %q", string(bodyBytes))
	}
	if resp.Header.Get("X-Egress-Check") != "passed" {
		t.Errorf("missing or unexpected header X-Egress-Check: %q", resp.Header.Get("X-Egress-Check"))
	}
}

func TestDialer_Concurrency_NoLeaks(t *testing.T) {
	// Create and close 50 dialers concurrently to stress lifecycle management
	const concurrency = 50
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			node := &domain.Node{
				ID:        int64(100 + idx),
				LogicalID: fmt.Sprintf("concurrency-node-%d", idx),
				Name:      fmt.Sprintf("Concurrent-Node-%d", idx),
				Protocol:  domain.ProtocolShadowsocks,
				Server:    "127.0.0.1",
				Port:      10000 + idx,
				NormalizedPayload: map[string]any{
					"cipher":   "aes-128-gcm",
					"password": "concurrency-pass",
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			d, err := dialer.NewDialer(ctx, node)
			if err != nil {
				errCh <- fmt.Errorf("worker %d NewDialer: %w", idx, err)
				return
			}
			if d.Outbound() == nil {
				errCh <- fmt.Errorf("worker %d nil outbound", idx)
			}
			if err := d.Close(); err != nil {
				errCh <- fmt.Errorf("worker %d Close: %w", idx, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrency error: %v", err)
	}
}

func TestDialer_UnsupportedProtocol(t *testing.T) {
	node := &domain.Node{
		ID:        9,
		LogicalID: "unknown-proto",
		Name:      "Unknown",
		Protocol:  "nonexistent-proto",
		Server:    "1.2.3.4",
		Port:      1080,
	}

	ctx := context.Background()
	_, err := dialer.NewDialer(ctx, node)
	if err == nil {
		t.Fatal("expected error for unsupported protocol, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported protocol") {
		t.Errorf("expected unsupported protocol error, got %v", err)
	}
}
