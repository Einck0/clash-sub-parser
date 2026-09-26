package probe_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/probe/singbox"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

// 1. TestSafeNodeDialerMissingServerOrPort tests missing LogicalID, empty server, and invalid port
func TestSafeNodeDialerMissingServerOrPort(t *testing.T) {
	dialer := probe.NewSafeNodeDialer()
	ctx := context.Background()

	// 1.1 Empty LogicalID
	nodeEmptyID := domain.Node{
		LogicalID: "",
		Protocol:  domain.ProtocolSS,
		Server:    "198.51.100.1",
		Port:      8388,
	}
	client, cleanup, err := dialer(ctx, nodeEmptyID)
	if err == nil {
		if cleanup != nil {
			_ = cleanup()
		}
		t.Fatal("expected error for empty LogicalID, got nil")
	}
	if !errors.Is(err, probe.ErrCredentialsUnavailable) || client != nil {
		t.Fatalf("expected ErrCredentialsUnavailable and nil client, got err=%v client=%v", err, client)
	}

	// 1.2 Empty Server
	nodeEmptyServer := domain.Node{
		LogicalID: "node-empty-server",
		Protocol:  domain.ProtocolSS,
		Server:    "",
		Port:      8388,
	}
	client, cleanup, err = dialer(ctx, nodeEmptyServer)
	if err == nil {
		if cleanup != nil {
			_ = cleanup()
		}
		t.Fatal("expected error for empty Server, got nil")
	}
	if !errors.Is(err, probe.ErrCredentialsUnavailable) || client != nil {
		t.Fatalf("expected ErrCredentialsUnavailable and nil client, got err=%v client=%v", err, client)
	}

	// 1.3 Invalid Port
	nodeInvalidPort := domain.Node{
		LogicalID: "node-bad-port",
		Protocol:  domain.ProtocolSS,
		Server:    "198.51.100.1",
		Port:      0,
	}
	client, cleanup, err = dialer(ctx, nodeInvalidPort)
	if err == nil {
		if cleanup != nil {
			_ = cleanup()
		}
		t.Fatal("expected error for Port=0, got nil")
	}
	if !errors.Is(err, probe.ErrCredentialsUnavailable) || client != nil {
		t.Fatalf("expected ErrCredentialsUnavailable and nil client, got err=%v client=%v", err, client)
	}
}

// 2. TestSafeNodeDialerRejectsPrivateAndCarrierGradeIPs tests rejection of 100.64.0.0/10, 10.0.0.0/8, 127.0.0.1, etc.
func TestSafeNodeDialerRejectsPrivateAndCarrierGradeIPs(t *testing.T) {
	dialer := probe.NewSafeNodeDialer()
	ctx := context.Background()

	testIPs := []struct {
		name string
		ip   string
	}{
		{"CGNAT 100.64.0.1", "100.64.0.1"},
		{"CGNAT 100.127.255.254", "100.127.255.254"},
		{"Private 10.0.0.1", "10.0.0.1"},
		{"Private 172.16.0.1", "172.16.0.1"},
		{"Private 192.168.1.1", "192.168.1.1"},
		{"Loopback 127.0.0.1", "127.0.0.1"},
		{"IPv6 Loopback ::1", "::1"},
		{"IPv6 Loopback in brackets [::1]", "[::1]"},
		{"Link local 169.254.1.1", "169.254.1.1"},
	}

	for i, tc := range testIPs {
		node := domain.Node{
			LogicalID: fmt.Sprintf("node-ip-%d", i),
			Protocol:  domain.ProtocolSS,
			Server:    tc.ip,
			Port:      8388,
			Credentials: domain.InboundProtocolCredential{
				Method:   "aes-128-gcm",
				Password: "password",
			},
		}

		client, cleanup, err := dialer(ctx, node)
		if err == nil {
			if cleanup != nil {
				_ = cleanup()
			}
			t.Fatalf("[%s] expected IP %s to be rejected, but dialer succeeded", tc.name, tc.ip)
		}
		if !errors.Is(err, probe.ErrCredentialsUnavailable) {
			t.Fatalf("[%s] expected ErrCredentialsUnavailable, got %v", tc.name, err)
		}
		if client != nil {
			t.Fatalf("[%s] client must be nil", tc.name)
		}
	}
}

// 3. TestSafeNodeDialerErrorSanitization tests that secret passwords do not leak in error messages
func TestSafeNodeDialerErrorSanitization(t *testing.T) {
	dialer := probe.NewSafeNodeDialer()
	ctx := context.Background()

	secretPassword := "super-sensitive-secret-token-do-not-leak"
	node := domain.Node{
		LogicalID: "node-private-ip",
		Protocol:  domain.ProtocolSS,
		Server:    "10.0.0.1",
		Port:      8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: secretPassword,
		},
	}

	client, cleanup, err := dialer(ctx, node)
	if err == nil {
		if cleanup != nil {
			_ = cleanup()
		}
		t.Fatal("expected error on private IP, got nil")
	}
	if !errors.Is(err, probe.ErrCredentialsUnavailable) || client != nil {
		t.Fatalf("expected ErrCredentialsUnavailable and nil client, got %v", err)
	}
	if strings.Contains(err.Error(), secretPassword) {
		t.Fatalf("error message leaks secret password: %s", err.Error())
	}
}

// 4. TestSafeNodeDialerRejectsDomainNames verifies unsafe or unresolvable domain entries remain fail-closed.
func TestSafeNodeDialerRejectsDomainNames(t *testing.T) {
	dialer := probe.NewSafeNodeDialer()
	ctx := context.Background()

	domains := []string{
		"example.com",
		"internal.service.local",
		"node.proxy.org",
	}

	for i, domainName := range domains {
		node := domain.Node{
			LogicalID: fmt.Sprintf("node-dom-%d", i),
			Protocol:  domain.ProtocolSS,
			Server:    domainName,
			Port:      8388,
			Credentials: domain.InboundProtocolCredential{
				Method:   "aes-128-gcm",
				Password: "password",
			},
		}

		client, cleanup, err := dialer(ctx, node)
		if err == nil {
			if cleanup != nil {
				_ = cleanup()
			}
			if client != nil {
				client.CloseIdleConnections()
			}
			continue
		}
		if !errors.Is(err, probe.ErrCredentialsUnavailable) {
			t.Fatalf("[%s] expected ErrCredentialsUnavailable, got %v", domainName, err)
		}
		if client != nil {
			t.Fatalf("[%s] client must be nil", domainName)
		}
	}
}

// 5. TestSafeNodeDialerPublicIPPreparationDoesNotConnect verifies that preparing a dialer for a public IP succeeds
// in constructing an in-memory client and setting CheckRedirect, but does NOT initiate any network dial or open connections.
func TestSafeNodeDialerPublicIPPreparationDoesNotConnect(t *testing.T) {
	dialer := probe.NewSafeNodeDialer()
	ctx := context.Background()

	node := domain.Node{
		LogicalID: "node-public-prep",
		Protocol:  domain.ProtocolSS,
		Server:    "1.1.1.1",
		Port:      8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "safe-password-test",
		},
	}

	client, cleanup, err := dialer(ctx, node)
	if err != nil {
		t.Fatalf("expected dialer to successfully prepare client for public IP, got err: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup function")
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup returned error: %v", err)
		}
	}()

	if client.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://1.1.1.1/test", nil)
	if err != nil {
		t.Fatalf("http.NewRequest failed: %v", err)
	}
	if err := client.CheckRedirect(req, []*http.Request{req}); err != http.ErrUseLastResponse {
		t.Fatalf("expected CheckRedirect to return http.ErrUseLastResponse, got %v", err)
	}
}

// 6. TestRunnerSafeNodeDialerWiring verifies wiring between SafeNodeDialer and Runner
func TestRunnerSafeNodeDialerWiring(t *testing.T) {
	safeDialer := probe.NewSafeNodeDialer()

	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("queue.NewScheduler: %v", err)
	}
	defer sched.Close()

	node := domain.Node{
		LogicalID:   "node-wire-test",
		DisplayName: "Wire Test Node",
		Protocol:    domain.ProtocolSS,
		Server:      "127.0.0.1", // loopback rejected by safe dialer!
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass",
		},
		Active: true,
	}

	_, _, err = safeDialer(context.Background(), node)
	if err == nil {
		t.Fatal("expected safe dialer to reject 127.0.0.1, got nil")
	}
	if !errors.Is(err, probe.ErrCredentialsUnavailable) {
		t.Fatalf("expected ErrCredentialsUnavailable, got %v", err)
	}

	nodesRepo := newMemoryNodes()
	nodesRepo.items[node.LogicalID] = node
	obsRepo := newMemoryObservations()
	runsRepo := newMemoryRuns()

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(safeDialer),
	)

	run := &domain.ProbeRun{
		ID:             "run_safe_wire_test",
		IdempotencyKey: "key_safe_wire_test",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     time.Now().Add(time.Hour),
	}
	_ = runsRepo.Create(context.Background(), run)

	runErr := runner.Run(context.Background(), run, []string{node.LogicalID}, []domain.ProbeKind{domain.ProbeKindBaseline})
	if runErr == nil {
		t.Fatal("expected runner.Run to fail when safe dialer rejects private IP, got nil")
	}

	updatedRun, _ := runsRepo.GetByID(context.Background(), run.ID)
	if updatedRun.State != domain.ProbeRunStateFailed {
		t.Fatalf("expected run state failed, got %s", updatedRun.State)
	}

	observations, _ := obsRepo.ListByRun(context.Background(), run.ID)
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	if observations[0].Verdict != domain.VerdictError {
		t.Fatalf("expected verdict error, got %s", observations[0].Verdict)
	}
	if !strings.Contains(observations[0].RedactedSummary, "credentials_unavailable") {
		t.Fatalf("expected summary to contain credentials_unavailable, got %s", observations[0].RedactedSummary)
	}
}

type fixtureResolver struct {
	mu        sync.Mutex
	responses map[string][]net.IPAddr
	errors    map[string]error
}

func newFixtureResolver() *fixtureResolver {
	return &fixtureResolver{
		responses: make(map[string][]net.IPAddr),
		errors:    make(map[string]error),
	}
}

func (r *fixtureResolver) SetIPs(host string, ips ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clean := strings.ToLower(strings.TrimSpace(host))
	var addrs []net.IPAddr
	for _, ipStr := range ips {
		parsed := net.ParseIP(ipStr)
		if parsed != nil {
			addrs = append(addrs, net.IPAddr{IP: parsed})
		}
	}
	r.responses[clean] = addrs
}

func (r *fixtureResolver) SetError(host string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clean := strings.ToLower(strings.TrimSpace(host))
	r.errors[clean] = err
}

func (r *fixtureResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clean := strings.ToLower(strings.TrimSpace(host))
	if err, ok := r.errors[clean]; ok && err != nil {
		return nil, err
	}
	if addrs, ok := r.responses[clean]; ok {
		return addrs, nil
	}
	return nil, fmt.Errorf("no fixture record for %s", host)
}

func TestSafeNodeDialerRejectsProtocolEndpointBypasses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		protocol  domain.Protocol
		server    string
		port      int
		transport map[string]string
	}{
		{name: "Hysteria2 port hopping", protocol: domain.ProtocolHysteria2, server: "1.1.1.1", port: 443, transport: map[string]string{"ports": "443-444"}},
		{name: "TUIC disabled SNI", protocol: domain.ProtocolTUIC, server: "1.1.1.1", port: 443, transport: map[string]string{"disable_sni": "true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialer := probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{
				ClientFactory: func(context.Context, singbox.NodeConfig, singbox.HTTPClientOptions) (*http.Client, func() error, error) {
					t.Fatal("client factory must not be called for an unsafe endpoint configuration")
					return nil, nil, nil
				},
			})
			node := domain.Node{
				LogicalID: "endpoint-bypass-" + string(tc.protocol),
				Protocol:  tc.protocol,
				Server:    tc.server,
				Port:      tc.port,
				Credentials: domain.InboundProtocolCredential{
					UUID:      "11111111-1111-1111-1111-111111111111",
					Password:  "fixture",
					Transport: tc.transport,
				},
			}
			_, _, err := dialer(context.Background(), node)
			if !errors.Is(err, probe.ErrCredentialsUnavailable) {
				t.Fatalf("expected fail-closed credential error, got %v", err)
			}
		})
	}
}

func TestSafeNodeDialerTrojanDomainResolutionAndIPPinning(t *testing.T) {
	resolver := newFixtureResolver()
	ctx := context.Background()

	resolver.SetIPs("trojan.public.com", "1.1.1.1", "1.0.0.1")
	resolver.SetIPs("trojan.single.com", "93.184.216.34")
	resolver.SetIPs("trojan.ipv6.com", "2606:4700:4700::1111")
	resolver.SetIPs("trojan.explicit-sni.com", "1.1.1.1")

	testCases := []struct {
		name         string
		domainName   string
		expectedIP   string
		expectedSNI  string
		transport    map[string]string
		testOutbound bool
	}{
		{
			name:         "Multiple public IPs pins first IP and domain as SNI",
			domainName:   "trojan.public.com",
			expectedIP:   "1.1.1.1",
			expectedSNI:  "trojan.public.com",
			testOutbound: true,
		},
		{
			name:         "Single public IPv4 pins IP and domain as SNI",
			domainName:   "trojan.single.com",
			expectedIP:   "93.184.216.34",
			expectedSNI:  "trojan.single.com",
			testOutbound: true,
		},
		{
			name:         "Public IPv6 pins IPv6 literal and domain as SNI",
			domainName:   "trojan.ipv6.com",
			expectedIP:   "2606:4700:4700::1111",
			expectedSNI:  "trojan.ipv6.com",
			testOutbound: true,
		},
		{
			name:        "Explicit SNI in transport preserved over domain",
			domainName:  "trojan.explicit-sni.com",
			expectedIP:  "1.1.1.1",
			expectedSNI: "custom.sni.org",
			transport: map[string]string{
				"network": "tcp",
				"sni":     "custom.sni.org",
			},
			testOutbound: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logicalID := "node-" + strings.ReplaceAll(tc.domainName, ".", "-")
			node := domain.Node{
				LogicalID: logicalID,
				Protocol:  domain.ProtocolTrojan,
				Server:    tc.domainName,
				Port:      443,
				Credentials: domain.InboundProtocolCredential{
					Password:  "trojan-test-password",
					Transport: tc.transport,
				},
			}

			var capturedCfg singbox.NodeConfig
			factoryCalled := false
			capturingFactory := func(fCtx context.Context, config singbox.NodeConfig, opts singbox.HTTPClientOptions) (*http.Client, func() error, error) {
				factoryCalled = true
				capturedCfg = config
				return &http.Client{}, func() error { return nil }, nil
			}

			dialer := probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{
				Resolver:      resolver,
				ClientFactory: capturingFactory,
			})

			client, cleanup, err := dialer(ctx, node)
			if err != nil {
				t.Fatalf("dialer failed unexpectedly: %v", err)
			}
			if client == nil || cleanup == nil {
				t.Fatal("expected non-nil client and cleanup")
			}
			if !factoryCalled {
				t.Fatal("expected capturing client factory to be called")
			}

			if capturedCfg.Server != tc.expectedIP {
				t.Fatalf("expected pinned cfg.Server=%s, got %s", tc.expectedIP, capturedCfg.Server)
			}
			if capturedCfg.SNI != tc.expectedSNI {
				t.Fatalf("expected cfg.SNI=%s, got %s", tc.expectedSNI, capturedCfg.SNI)
			}
			if capturedCfg.LogicalID != node.LogicalID {
				t.Fatalf("logical ID mismatch: %s != %s", capturedCfg.LogicalID, node.LogicalID)
			}
			if capturedCfg.Port != 443 {
				t.Fatalf("port mismatch: %d != 443", capturedCfg.Port)
			}
			if capturedCfg.Password != "trojan-test-password" {
				t.Fatalf("password mismatch: %s", capturedCfg.Password)
			}
			if capturedCfg.SkipCertVerify {
				t.Fatal("expected SkipCertVerify to be false")
			}

			if tc.testOutbound {
				opts, tag, buildErr := singbox.BuildOptions(capturedCfg)
				if buildErr != nil {
					t.Fatalf("singbox.BuildOptions failed: %v", buildErr)
				}
				if tag != node.LogicalID {
					t.Fatalf("singbox tag mismatch: %s", tag)
				}
				if len(opts.Outbounds) != 1 {
					t.Fatalf("expected 1 outbound, got %d", len(opts.Outbounds))
				}
				ob := opts.Outbounds[0]
				if ob.Type != C.TypeTrojan {
					t.Fatalf("expected outbound type %s, got %s", C.TypeTrojan, ob.Type)
				}
				trojanOpts, ok := ob.Options.(*option.TrojanOutboundOptions)
				if !ok || trojanOpts == nil {
					t.Fatalf("failed to cast outbound options to TrojanOutboundOptions")
				}
				if trojanOpts.Server != tc.expectedIP {
					t.Fatalf("singbox trojanOpts.Server=%s, expected pinned IP %s", trojanOpts.Server, tc.expectedIP)
				}
				if trojanOpts.ServerPort != 443 {
					t.Fatalf("singbox trojanOpts.ServerPort=%d, expected 443", trojanOpts.ServerPort)
				}
				if trojanOpts.TLS == nil || trojanOpts.TLS.ServerName != tc.expectedSNI {
					t.Fatalf("singbox trojanOpts.TLS.ServerName=%v, expected %s", trojanOpts.TLS, tc.expectedSNI)
				}
				if trojanOpts.TLS.Insecure {
					t.Fatal("singbox trojanOpts.TLS.Insecure must be false")
				}
			}
		})
	}

	t.Run("DefaultSingboxHTTPClientInstantiatedWithoutNetworkCalls", func(t *testing.T) {
		defaultDialer := probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{
			Resolver: resolver,
		})
		node := domain.Node{
			LogicalID: "node-trojan-public-com",
			Protocol:  domain.ProtocolTrojan,
			Server:    "trojan.public.com",
			Port:      443,
			Credentials: domain.InboundProtocolCredential{
				Password: "trojan-test-password",
			},
		}

		client, cleanup, err := defaultDialer(ctx, node)
		if err != nil {
			t.Fatalf("expected default singbox client creation to succeed, got %v", err)
		}
		if client == nil || cleanup == nil {
			t.Fatal("expected non-nil client and cleanup")
		}
		defer func() {
			if err := cleanup(); err != nil {
				t.Errorf("cleanup returned error: %v", err)
			}
		}()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://cp.cloudflare.com/generate_204", nil)
		if err := client.CheckRedirect(req, []*http.Request{req}); err != http.ErrUseLastResponse {
			t.Fatalf("expected http.ErrUseLastResponse, got %v", err)
		}

		tr, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("client transport is not *http.Transport: %T", client.Transport)
		}
		if tr.Proxy != nil {
			t.Fatal("transport.Proxy must be nil to guarantee no host proxy is consulted")
		}
		if tr.DialContext == nil {
			t.Fatal("transport.DialContext must be configured to sing-box runtime dialer")
		}
	})
}

func TestSafeNodeDialerTrojanPrivateAndRebindingRejections(t *testing.T) {
	resolver := newFixtureResolver()
	ctx := context.Background()

	resolver.SetIPs("trojan.rfc6598-1.com", "100.64.0.1")
	resolver.SetIPs("trojan.rfc6598-2.com", "100.127.255.254")
	resolver.SetIPs("trojan.ipv6-loopback.com", "::1")
	resolver.SetIPs("trojan.ipv6-ula-1.com", "fc00::1")
	resolver.SetIPs("trojan.ipv6-ula-2.com", "fd12:3456:789a::1")
	resolver.SetIPs("trojan.ipv6-linklocal.com", "fe80::1")
	resolver.SetIPs("trojan.ipv4-private10.com", "10.0.0.1")
	resolver.SetIPs("trojan.ipv4-private172.com", "172.16.0.1")
	resolver.SetIPs("trojan.ipv4-private192.com", "192.168.1.1")
	resolver.SetIPs("trojan.ipv4-loopback.com", "127.0.0.1")
	resolver.SetIPs("trojan.rebind-loopback.com", "1.1.1.1", "127.0.0.1")
	resolver.SetIPs("trojan.rebind-rfc6598.com", "1.1.1.1", "100.64.1.1")
	resolver.SetIPs("trojan.rebind-ipv6-ula.com", "1.1.1.1", "fc00::1")
	resolver.SetIPs("trojan.rebind-ipv6-linklocal.com", "1.1.1.1", "fe80::1")
	resolver.SetIPs("trojan.rebind-ipv4-private.com", "1.1.1.1", "10.10.10.10")
	resolver.SetError("trojan.nxdomain.com", errors.New("no such host"))
	resolver.SetIPs("trojan.empty-ips.com")

	testCases := []struct {
		name       string
		domainName string
	}{
		{"RFC6598 CGNAT lower boundary", "trojan.rfc6598-1.com"},
		{"RFC6598 CGNAT upper boundary", "trojan.rfc6598-2.com"},
		{"IPv6 loopback ::1", "trojan.ipv6-loopback.com"},
		{"IPv6 ULA fc00::1", "trojan.ipv6-ula-1.com"},
		{"IPv6 ULA fd12::1", "trojan.ipv6-ula-2.com"},
		{"IPv6 link-local fe80::1", "trojan.ipv6-linklocal.com"},
		{"IPv4 private 10.0.0.1", "trojan.ipv4-private10.com"},
		{"IPv4 private 172.16.0.1", "trojan.ipv4-private172.com"},
		{"IPv4 private 192.168.1.1", "trojan.ipv4-private192.com"},
		{"IPv4 loopback 127.0.0.1", "trojan.ipv4-loopback.com"},
		{"DNS Rebinding public + loopback", "trojan.rebind-loopback.com"},
		{"DNS Rebinding public + RFC6598", "trojan.rebind-rfc6598.com"},
		{"DNS Rebinding public + IPv6 ULA", "trojan.rebind-ipv6-ula.com"},
		{"DNS Rebinding public + IPv6 link-local", "trojan.rebind-ipv6-linklocal.com"},
		{"DNS Rebinding public + RFC1918", "trojan.rebind-ipv4-private.com"},
		{"DNS resolver NXDOMAIN error", "trojan.nxdomain.com"},
		{"DNS resolver empty IP set", "trojan.empty-ips.com"},
	}

	dialer := probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{
		Resolver: resolver,
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logicalID := "node-reject-" + strings.ReplaceAll(tc.domainName, ".", "-")
			node := domain.Node{
				LogicalID: logicalID,
				Protocol:  domain.ProtocolTrojan,
				Server:    tc.domainName,
				Port:      443,
				Credentials: domain.InboundProtocolCredential{
					Password: "password",
				},
			}

			client, cleanup, err := dialer(ctx, node)
			if err == nil {
				if cleanup != nil {
					_ = cleanup()
				}
				t.Fatalf("[%s] expected fail-closed rejection for %s, got nil error", tc.name, tc.domainName)
			}
			if !errors.Is(err, probe.ErrCredentialsUnavailable) {
				t.Fatalf("[%s] expected ErrCredentialsUnavailable, got %v", tc.name, err)
			}
			if client != nil {
				t.Fatalf("[%s] client must be nil on rejection", tc.name)
			}
		})
	}
}

func TestSafeNodeDialerUnsafeTransportAndCertVerificationRejected(t *testing.T) {
	resolver := newFixtureResolver()
	resolver.SetIPs("trojan.valid.com", "1.1.1.1")
	ctx := context.Background()

	dialer := probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{
		Resolver: resolver,
	})

	testCases := []struct {
		name      string
		protocol  domain.Protocol
		server    string
		port      int
		transport map[string]string
	}{
		{"Trojan domain with skip_cert_verify=true rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "skip_cert_verify": "true"}},
		{"Trojan domain with skip_cert_verify=1 rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "skip_cert_verify": "1"}},
		{"Trojan domain with skip-cert-verify=true rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "skip-cert-verify": "true"}},
		{"Trojan domain with skip-cert-verify=1 rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "skip-cert-verify": "1"}},
		{"Trojan domain with skipcert=true rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "skipcert": "true"}},
		{"Trojan domain with insecure=true rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "insecure": "true"}},
		{"Trojan domain with insecure=1 rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "insecure": "1"}},
		{"Trojan domain with allow_insecure=true rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "allow_insecure": "true"}},
		{"Trojan domain with allow-insecure=1 rejected", domain.ProtocolTrojan, "trojan.valid.com", 443, map[string]string{"network": "tcp", "allow-insecure": "1"}},

		{"Trojan IP literal with skip_cert_verify=true rejected", domain.ProtocolTrojan, "1.1.1.1", 443, map[string]string{"skip_cert_verify": "true"}},
		{"Trojan IP literal with insecure=1 rejected", domain.ProtocolTrojan, "1.1.1.1", 443, map[string]string{"insecure": "1"}},
		{"Trojan IP literal with skip-cert-verify=true rejected", domain.ProtocolTrojan, "1.1.1.1", 443, map[string]string{"skip-cert-verify": "true"}},

		{"Trojan with port 0 rejected", domain.ProtocolTrojan, "1.1.1.1", 0, nil},
		{"Trojan with port -1 rejected", domain.ProtocolTrojan, "1.1.1.1", -1, nil},
		{"Trojan with port 65536 rejected", domain.ProtocolTrojan, "1.1.1.1", 65536, nil},
	}

	for idx, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node := domain.Node{
				LogicalID: fmt.Sprintf("node-unsafe-%d", idx),
				Protocol:  tc.protocol,
				Server:    tc.server,
				Port:      tc.port,
				Credentials: domain.InboundProtocolCredential{
					Password:  "pass",
					Transport: tc.transport,
				},
			}

			client, cleanup, err := dialer(ctx, node)
			if err == nil {
				if cleanup != nil {
					_ = cleanup()
				}
				if client != nil {
					client.CloseIdleConnections()
				}
				t.Fatalf("[%s] expected rejection, got nil error", tc.name)
			}
			if !errors.Is(err, probe.ErrCredentialsUnavailable) {
				t.Fatalf("[%s] expected ErrCredentialsUnavailable, got %v", tc.name, err)
			}
			if client != nil {
				t.Fatalf("[%s] client must be nil", tc.name)
			}
		})
	}
}

func TestNodeConfigFromPayloadInsecureOptions(t *testing.T) {
	baseNode := domain.Node{
		LogicalID:   "test-insecure-node",
		DisplayName: "Test Insecure Node",
		Protocol:    domain.ProtocolTrojan,
	}

	testCases := []struct {
		name          string
		normTransport map[string]string
		credTransport map[string]string
		expectedInsec bool
	}{
		{
			name:          "norm skip_cert_verify=true",
			normTransport: map[string]string{"skip_cert_verify": "true"},
			expectedInsec: true,
		},
		{
			name:          "norm skip-cert-verify=TRUE uppercase",
			normTransport: map[string]string{"skip-cert-verify": "TRUE"},
			expectedInsec: true,
		},
		{
			name:          "norm insecure=1",
			normTransport: map[string]string{"insecure": "1"},
			expectedInsec: true,
		},
		{
			name:          "norm allow_insecure=yes",
			normTransport: map[string]string{"allow_insecure": "yes"},
			expectedInsec: true,
		},
		{
			name:          "norm allow-insecure=on",
			normTransport: map[string]string{"allow-insecure": "on"},
			expectedInsec: true,
		},
		{
			name:          "cred skip_cert_verify=true",
			credTransport: map[string]string{"skip_cert_verify": "true"},
			expectedInsec: true,
		},
		{
			name:          "cred skip-cert-verify=1",
			credTransport: map[string]string{"skip-cert-verify": "1"},
			expectedInsec: true,
		},
		{
			name:          "cred insecure=true",
			credTransport: map[string]string{"insecure": "true"},
			expectedInsec: true,
		},
		{
			name:          "cred allow_insecure=yes",
			credTransport: map[string]string{"allow_insecure": "yes"},
			expectedInsec: true,
		},
		{
			name:          "explicit false values remain secure",
			normTransport: map[string]string{"skip_cert_verify": "false"},
			credTransport: map[string]string{"insecure": "0"},
			expectedInsec: false,
		},
		{
			name:          "standard secure transport",
			normTransport: map[string]string{"network": "tcp", "tls": "true"},
			credTransport: map[string]string{"network": "tcp"},
			expectedInsec: false,
		},
		{
			name:          "nil transports remain secure",
			normTransport: nil,
			credTransport: nil,
			expectedInsec: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			norm := parser.NormalizedNode{
				Node:      baseNode,
				Server:    "1.1.1.1",
				Port:      443,
				Transport: tc.normTransport,
			}
			payload := &domain.NodeCredentialPayload{
				LogicalID: baseNode.LogicalID,
				Protocol:  baseNode.Protocol,
				Server:    "1.1.1.1",
				Port:      443,
				Credentials: domain.InboundProtocolCredential{
					Password:  "pwd",
					Transport: tc.credTransport,
				},
			}

			cfg := singbox.NodeConfigFromPayload(norm, payload)
			if cfg.SkipCertVerify != tc.expectedInsec {
				t.Fatalf("expected SkipCertVerify=%v, got %v", tc.expectedInsec, cfg.SkipCertVerify)
			}
		})
	}
}

type staticAppResolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (f staticAppResolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

func TestSafeNodeDialerRejectsUnsafeCredentialsAndDNS(t *testing.T) {
	cases := []struct {
		name      string
		protocol  domain.Protocol
		server    string
		transport map[string]string
		ips       []net.IPAddr
	}{
		{name: "mixed public private dns", protocol: domain.ProtocolTrojan, server: "node.example.test", transport: map[string]string{"tls": "true"}, ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.0.0.8")}}},
		{name: "metadata dns", protocol: domain.ProtocolTrojan, server: "node.example.test", transport: map[string]string{"tls": "true"}, ips: []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}},
		{name: "RFC6598 dns", protocol: domain.ProtocolTrojan, server: "node.example.test", transport: map[string]string{"tls": "true"}, ips: []net.IPAddr{{IP: net.ParseIP("100.64.0.1")}}},
		{name: "IPv4 mapped private", protocol: domain.ProtocolTrojan, server: "node.example.test", transport: map[string]string{"tls": "true"}, ips: []net.IPAddr{{IP: net.ParseIP("::ffff:10.0.0.1")}}},
		{name: "Hy2 port hopping", protocol: domain.ProtocolHysteria2, server: "node.example.test", transport: map[string]string{"server_ports": "443,8443"}, ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}},
		{name: "TUIC disable SNI", protocol: domain.ProtocolTUIC, server: "node.example.test", transport: map[string]string{"disable_sni": "true"}, ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var calls atomic.Int32
			resolver := staticAppResolverFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
				if host != tc.server {
					t.Fatalf("unexpected lookup host %q", host)
				}
				calls.Add(1)
				return tc.ips, nil
			})
			dialer := probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{
				Resolver: resolver,
				ClientFactory: func(context.Context, singbox.NodeConfig, singbox.HTTPClientOptions) (*http.Client, func() error, error) {
					t.Fatal("unsafe input reached client factory")
					return nil, nil, nil
				},
			})
			node := domain.Node{
				LogicalID: "negative-node",
				Protocol:  tc.protocol,
				Server:    tc.server,
				Port:      443,
				Credentials: domain.InboundProtocolCredential{
					Password:  "secret",
					UUID:      "00000000-0000-0000-0000-000000000001",
					Transport: tc.transport,
				},
			}
			client, cleanup, err := dialer(ctx, node)
			if cleanup != nil {
				defer cleanup()
			}
			if err == nil || client != nil {
				t.Fatalf("unsafe input accepted: client=%v err=%v", client, err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("secret leaked in error: %v", err)
			}
			if (tc.name == "Hy2 port hopping" || tc.name == "TUIC disable SNI") && calls.Load() != 0 {
				t.Fatalf("resolver calls=%d, want zero lookups before option rejection", calls.Load())
			}
			if tc.name != "Hy2 port hopping" && tc.name != "TUIC disable SNI" && calls.Load() != 1 {
				t.Fatalf("resolver calls=%d, want one validated lookup and no rebind", calls.Load())
			}
		})
	}
}

func TestTargetURLDoesNotConnectViaHost(t *testing.T) {
	resolver := newFixtureResolver()
	resolver.SetIPs("trojan.wire-check.com", "93.184.216.34")
	ctx := context.Background()

	var capturedCfg singbox.NodeConfig
	capturingFactory := func(fCtx context.Context, config singbox.NodeConfig, opts singbox.HTTPClientOptions) (*http.Client, func() error, error) {
		capturedCfg = config
		return singbox.NewHTTPClient(fCtx, config, opts)
	}

	dialer := probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{
		Resolver:      resolver,
		ClientFactory: capturingFactory,
	})

	node := domain.Node{
		LogicalID: "node-wire-check",
		Protocol:  domain.ProtocolTrojan,
		Server:    "trojan.wire-check.com",
		Port:      443,
		Credentials: domain.InboundProtocolCredential{
			Password: "safe-password",
		},
	}

	client, cleanup, err := dialer(ctx, node)
	if err != nil {
		t.Fatalf("dialer failed: %v", err)
	}
	defer func() {
		if cleanup != nil {
			_ = cleanup()
		}
	}()

	if capturedCfg.Server != "93.184.216.34" {
		t.Fatalf("capturedCfg.Server must be pinned IP 93.184.216.34, got %s", capturedCfg.Server)
	}
	if capturedCfg.SNI != "trojan.wire-check.com" {
		t.Fatalf("capturedCfg.SNI must be original domain trojan.wire-check.com, got %s", capturedCfg.SNI)
	}

	boxOpts, _, err := singbox.BuildOptions(capturedCfg)
	if err != nil {
		t.Fatalf("BuildOptions failed: %v", err)
	}
	trojanOpts := boxOpts.Outbounds[0].Options.(*option.TrojanOutboundOptions)
	if trojanOpts.Server != "93.184.216.34" {
		t.Fatalf("singbox outbound destination must be pinned IP 93.184.216.34, got %s", trojanOpts.Server)
	}
	if trojanOpts.TLS.ServerName != "trojan.wire-check.com" {
		t.Fatalf("singbox outbound TLS ServerName must be trojan.wire-check.com, got %s", trojanOpts.TLS.ServerName)
	}
	if trojanOpts.TLS.Insecure {
		t.Fatal("singbox outbound TLS must have strict cert validation")
	}

	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if tr.Proxy != nil {
		t.Fatal("expected tr.Proxy to be nil so host proxy settings are never consulted")
	}
	if tr.DialContext == nil {
		t.Fatal("expected tr.DialContext to be configured to sing-box runtime outbound")
	}

	dummyReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://target.destination.internal/", nil)
	if err := client.CheckRedirect(dummyReq, []*http.Request{dummyReq}); err != http.ErrUseLastResponse {
		t.Fatalf("expected http.ErrUseLastResponse, got %v", err)
	}
}
