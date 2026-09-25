package singbox_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/probe/singbox"
)

func TestBuildOptionsSupportsNormalizedProtocols(t *testing.T) {
	protocols := []domain.Protocol{
		domain.ProtocolSS, domain.ProtocolVMess, domain.ProtocolVLESS,
		domain.ProtocolTrojan, domain.ProtocolHysteria2, domain.ProtocolWireGuard,
		domain.ProtocolTUIC,
	}
	for _, protocol := range protocols {
		t.Run(string(protocol), func(t *testing.T) {
			config := testNode(protocol)
			options, tag, err := singbox.BuildOptions(config)
			if err != nil {
				t.Fatalf("BuildOptions() error = %v", err)
			}
			if tag != config.LogicalID || len(options.Outbounds)+len(options.Endpoints) != 1 {
				t.Fatalf("unexpected runtime options: tag=%q outbounds=%d endpoints=%d", tag, len(options.Outbounds), len(options.Endpoints))
			}
			if options.Route == nil || options.Route.Final != tag {
				t.Fatalf("runtime does not route final traffic through node outbound")
			}
		})
	}
}

func TestBuildOptionsRejectsInvalidEndpoint(t *testing.T) {
	config := testNode(domain.ProtocolSS)
	config.Server = ""
	if _, _, err := singbox.BuildOptions(config); err == nil {
		t.Fatal("BuildOptions() accepted an empty server")
	}
	config.Server = "127.0.0.1"
	config.Port = 0
	if _, _, err := singbox.BuildOptions(config); err == nil {
		t.Fatal("BuildOptions() accepted an invalid port")
	}
	config.Port = 70000
	if _, _, err := singbox.BuildOptions(config); err == nil {
		t.Fatal("BuildOptions() accepted an out-of-range port")
	}
	config.Port = 8388
	config.Protocol = domain.Protocol("unknown_proto")
	if _, _, err := singbox.BuildOptions(config); err == nil {
		t.Fatal("BuildOptions() accepted an unknown protocol")
	}
}

func TestNodeConfigFromNormalized(t *testing.T) {
	testCases := []struct {
		protocol domain.Protocol
		secrets  []string
		network  string
		tls      bool
	}{
		{domain.ProtocolSS, []string{"ss-pass", "aes-128-gcm"}, "tcp", false},
		{domain.ProtocolVMess, []string{"11111111-1111-1111-1111-111111111111"}, "ws", true},
		{domain.ProtocolVLESS, []string{"22222222-2222-2222-2222-222222222222"}, "grpc", true},
		{domain.ProtocolTrojan, []string{"trojan-pass"}, "tcp", true},
		{domain.ProtocolHysteria2, []string{"hy2-pass"}, "quic", true},
		{domain.ProtocolTUIC, []string{"33333333-3333-3333-3333-333333333333", "tuic-pass"}, "quic", true},
		{domain.ProtocolWireGuard, []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="}, "wireguard", false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.protocol), func(t *testing.T) {
			norm := parser.NormalizedNode{
				Node: domain.Node{
					LogicalID:   fmt.Sprintf("node_%s_id", tc.protocol),
					DisplayName: fmt.Sprintf("Node %s", tc.protocol),
					Protocol:    tc.protocol,
					Active:      true,
				},
				Server: "198.51.100.1",
				Port:   8443,
				Transport: map[string]string{
					"network": tc.network,
					"sni":     "node.example.com",
				},
			}
			if tc.tls {
				norm.Transport["tls"] = "true"
			}
			if tc.network == "ws" {
				norm.Transport["path"] = "/ws"
				norm.Transport["host"] = "ws.example.com"
			}
			if tc.network == "grpc" {
				norm.Transport["service_name"] = "grpc-svc"
			}

			cfg := singbox.NodeConfigFromNormalized(norm, tc.secrets...)
			if cfg.LogicalID != norm.Node.LogicalID {
				t.Fatalf("LogicalID = %q, want %q", cfg.LogicalID, norm.Node.LogicalID)
			}
			if cfg.Protocol != tc.protocol {
				t.Fatalf("Protocol = %q, want %q", cfg.Protocol, tc.protocol)
			}
			if cfg.Server != norm.Server || cfg.Port != norm.Port {
				t.Fatalf("endpoint mismatch: %s:%d vs %s:%d", cfg.Server, cfg.Port, norm.Server, norm.Port)
			}

			// Validate singbox options build succeeds
			options, tag, err := singbox.BuildOptions(cfg)
			if err != nil {
				t.Fatalf("BuildOptions(fromNormalized) error = %v", err)
			}
			if tag != cfg.LogicalID {
				t.Fatalf("tag = %q, want %q", tag, cfg.LogicalID)
			}
			if options.Route.Final != tag {
				t.Fatalf("route.final = %q, want %q", options.Route.Final, tag)
			}
		})
	}
}

func TestHTTPClientNeverUsesEnvironmentProxy(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, "http://127.0.0.1:1")
	}
	client, closeRuntime, err := singbox.NewHTTPClient(context.Background(), testNode(domain.ProtocolSS), singbox.HTTPClientOptions{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}
	defer closeRuntime()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("HTTP client inherited an environment proxy")
	}
}

func TestHTTPClientRoutesTrafficThroughConfiguredOutbound(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"198.51.100.1","status":"ok"}`))
	}))
	defer target.Close()

	hostProxy := startMockHostProxyServer()
	defer hostProxy.Close()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, hostProxy.URL())
	}

	config := testNode(domain.Protocol("socks5"))
	config.Server = proxy.Address()
	config.Port = proxy.Port()
	client, closeRuntime, err := singbox.NewHTTPClient(context.Background(), config, singbox.HTTPClientOptions{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}
	defer closeRuntime()

	response, err := client.Get(fixturePublicURL(t, target.URL))
	if err != nil {
		t.Fatalf("GET through configured outbound error = %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response error = %v", err)
	}
	if string(body) != `{"ip":"198.51.100.1","status":"ok"}` {
		t.Fatalf("response body = %q, want target server response", body)
	}
	if proxy.HandledConnections() == 0 {
		t.Fatalf("configured SOCKS5 fixture did not handle probe traffic: destinations=%v", proxy.Destinations())
	}
	deadline := time.Now().Add(time.Second)
	for proxy.BytesRelayed() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if proxy.BytesRelayed() == 0 {
		t.Fatalf("configured SOCKS5 fixture relayed no bytes: destinations=%v", proxy.Destinations())
	}
	if hostProxy.RequestCount() != 0 {
		t.Fatalf("host proxy handled %d requests, expected 0", hostProxy.RequestCount())
	}
	if destinations := proxy.Destinations(); len(destinations) == 0 || destinations[0] == "" {
		t.Fatalf("SOCKS5 fixture recorded no destination: %v", destinations)
	}
}

func TestHTTPClientPreservesHostAndPinsApprovedAddress(t *testing.T) {
	var calls int
	resolver := rebindingResolverFunc(func(context.Context, string) ([]net.IPAddr, error) {
		calls++
		if calls == 1 {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
	})
	cfg := testNode(domain.Protocol("socks5"))
	cfg.Server, cfg.Port = "192.0.2.2", 1080
	// A capturing HTTP outbound verifies the destination passed to sing-box and does not access network.
	client, closeFn, err := singbox.NewHTTPClient(context.Background(), cfg, singbox.HTTPClientOptions{Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	transport := client.Transport.(*http.Transport)
	if transport.DialContext == nil {
		t.Fatal("missing protected dialer")
	}
	if _, err := transport.DialContext(context.Background(), "tcp", "rebind.example:443"); err == nil {
		t.Fatal("expected outbound dial failure")
	}
	if calls != 1 {
		t.Fatalf("resolver calls=%d, expected one lookup followed by pinned IP dial", calls)
	}
}

func TestHTTPClientRejectsPrivateDestinationBeforeOutboundDial(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	cfg := testNode(domain.Protocol("socks5"))
	cfg.Server, cfg.Port = proxy.Address(), proxy.Port()
	client, closeFn, err := singbox.NewHTTPClient(context.Background(), cfg, singbox.HTTPClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	_, err = client.Get("http://127.0.0.1:8080/")
	if err == nil {
		t.Fatal("private destination unexpectedly accepted")
	}
	if proxy.HandledConnections() != 0 {
		t.Fatalf("outbound was dialed %d times", proxy.HandledConnections())
	}
}

func TestHTTPClientReportsConfiguredOutboundFailureWithoutUsingHostProxy(t *testing.T) {
	// Scenario: Host proxy is available and functional, but tested node fails.
	// Expected: Request through tested node fails; host proxy MUST NOT be touched.
	hostProxy := startMockHostProxyServer()
	defer hostProxy.Close()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, hostProxy.URL())
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("target-should-not-be-reached"))
	}))
	defer target.Close()

	// Pick an unreachable local port where no proxy is listening
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	unreachablePort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // Immediately closed so connections will fail

	config := testNode(domain.Protocol("socks5"))
	config.Server = "127.0.0.1"
	config.Port = unreachablePort
	client, closeRuntime, err := singbox.NewHTTPClient(context.Background(), config, singbox.HTTPClientOptions{Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewHTTPClient() error = %v", err)
	}
	defer closeRuntime()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("request through unreachable node unexpectedly succeeded")
	}

	// Crucial assertion: Host proxy MUST NOT have received any traffic!
	if hostProxy.RequestCount() != 0 {
		t.Fatalf("host proxy leaked/handled %d requests when node failed", hostProxy.RequestCount())
	}
}

func TestHTTPClientRejectsDNSRebindingSetContainingPrivateIP(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	cfg := testNode(domain.Protocol("socks5"))
	cfg.Server, cfg.Port = proxy.Address(), proxy.Port()
	resolver := &rebindingResolver{ips: []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("169.254.169.254")}}}
	client, closeFn, err := singbox.NewHTTPClient(context.Background(), cfg, singbox.HTTPClientOptions{Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	_, err = client.Get("http://rebind.example/")
	if err == nil {
		t.Fatal("DNS result containing metadata address unexpectedly accepted")
	}
	if proxy.HandledConnections() != 0 {
		t.Fatalf("outbound was dialed %d times", proxy.HandledConnections())
	}
}

type rebindingResolver struct{ ips []net.IPAddr }

func (r *rebindingResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.ips, nil
}

type rebindingResolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (f rebindingResolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

func TestRuntimeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runtime, err := singbox.New(ctx, testNode(domain.ProtocolSS))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if runtime.IsClosed() {
		t.Fatal("runtime should be open before cancellation")
	}

	// Trigger context cancellation
	cancel()

	// Wait briefly for AfterFunc hook to execute
	deadline := time.Now().Add(time.Second)
	for !runtime.IsClosed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond * 5)
	}

	if !runtime.IsClosed() {
		t.Fatal("runtime was not closed after context cancellation")
	}

	// Operations after context cancellation should return ErrRuntimeClosed or context error
	_, err = runtime.DialContext(context.Background(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("DialContext on cancelled runtime succeeded")
	}
}

func TestRuntimeCloseIsIdempotent(t *testing.T) {
	runtime, err := singbox.New(context.Background(), testNode(domain.ProtocolSS))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if !runtime.IsClosed() {
		t.Fatal("runtime remains open after Close()")
	}
	if _, err := runtime.DialContext(context.Background(), "tcp", "example.com:80"); err == nil {
		t.Fatal("closed runtime accepted a dial")
	}
	if _, err := runtime.ListenPacket(context.Background(), "example.com:80"); err == nil {
		t.Fatal("closed runtime accepted ListenPacket")
	}
}

func TestRuntimeAdapterInterface(t *testing.T) {
	config := testNode(domain.ProtocolSS)
	config.LogicalID = "test_adapter_interface"
	rt, err := singbox.New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer rt.Close()

	var adapter singbox.Adapter = rt
	if adapter.LogicalID() != "test_adapter_interface" {
		t.Fatalf("LogicalID = %q, want test_adapter_interface", adapter.LogicalID())
	}
	if adapter.Protocol() != domain.ProtocolSS {
		t.Fatalf("Protocol = %q, want %q", adapter.Protocol(), domain.ProtocolSS)
	}
	if adapter.Tag() != "test_adapter_interface" {
		t.Fatalf("Tag = %q, want test_adapter_interface", adapter.Tag())
	}
	if adapter.IsClosed() {
		t.Fatal("adapter should not be closed yet")
	}
	if adapter.Outbound() == nil {
		t.Fatal("adapter.Outbound() is nil")
	}
}

func TestConcurrentProbesWithRace(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	const numWorkers = 16
	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			cfg := testNode(domain.Protocol("socks5"))
			cfg.LogicalID = fmt.Sprintf("worker_%d", workerID)
			cfg.Server = proxy.Address()
			cfg.Port = proxy.Port()

			client, closeFn, err := singbox.NewHTTPClient(context.Background(), cfg, singbox.HTTPClientOptions{
				Timeout: 5 * time.Second,
			})
			if err != nil {
				errCh <- fmt.Errorf("worker %d NewHTTPClient: %w", workerID, err)
				return
			}
			defer closeFn()

			resp, err := client.Get(fixturePublicURL(t, target.URL))
			if err != nil {
				errCh <- fmt.Errorf("worker %d Get: %w", workerID, err)
				return
			}
			defer resp.Body.Close()
			_, _ = io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("worker %d status = %d", workerID, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent worker error: %v", err)
	}

	if proxy.HandledConnections() < numWorkers {
		t.Fatalf("handled connections = %d, want at least %d", proxy.HandledConnections(), numWorkers)
	}
}

func TestRuntimeResourceReclamationZeroGoroutineLeak(t *testing.T) {
	// Baseline goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	}))
	defer target.Close()

	for i := 0; i < 20; i++ {
		cfg := testNode(domain.Protocol("socks5"))
		cfg.LogicalID = fmt.Sprintf("leak_test_%d", i)
		cfg.Server = proxy.Address()
		cfg.Port = proxy.Port()

		client, closeFn, err := singbox.NewHTTPClient(context.Background(), cfg, singbox.HTTPClientOptions{
			Timeout: 2 * time.Second,
		})
		if err != nil {
			t.Fatalf("iteration %d NewHTTPClient error: %v", i, err)
		}

		resp, err := client.Get(fixturePublicURL(t, target.URL))
		if err != nil {
			closeFn()
			t.Fatalf("iteration %d Get error: %v", i, err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()

		if err := closeFn(); err != nil {
			t.Fatalf("iteration %d closeFn error: %v", i, err)
		}
	}

	// Allow background closures and GC to complete
	deadline := time.Now().Add(2 * time.Second)
	var finalGoroutines int
	for time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
		finalGoroutines = runtime.NumGoroutine()
		// Allow a small margin (<= initialGoroutines + 3) for test framework/http transport cleanup
		if finalGoroutines <= initialGoroutines+3 {
			break
		}
	}

	if finalGoroutines > initialGoroutines+5 {
		t.Fatalf("goroutine leak detected: initial=%d final=%d", initialGoroutines, finalGoroutines)
	}
}

func fixturePublicURL(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse fixture URL: %v", err)
	}
	_, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split fixture listener: %v", err)
	}
	parsed.Host = net.JoinHostPort("8.8.8.8", port)
	return parsed.String()
}

func testNode(protocol domain.Protocol) singbox.NodeConfig {
	return singbox.NodeConfig{
		LogicalID:  "node_test_01",
		Protocol:   protocol,
		Server:     "127.0.0.1",
		Port:       1,
		Method:     "aes-256-gcm",
		Password:   "fixture-password",
		UUID:       "11111111-1111-1111-1111-111111111111",
		PrivateKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		PublicKey:  "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
		SNI:        "fixture.invalid",
		Network:    "tcp",
		TLS:        true,
		Transport:  map[string]string{"network": "tcp"},
	}
}
