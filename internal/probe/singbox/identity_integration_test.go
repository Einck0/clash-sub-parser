package singbox_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/identity"
	"clash-sub-parser/internal/probe/singbox"
)

func TestIPRiskChannelExitIdentityResolutionWithConsensusAndProxyIsolation(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	hostProxy := startMockHostProxyServer()
	defer hostProxy.Close()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, hostProxy.URL())
	}

	target1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"203.0.113.50","country_code":"JP","asn":2516}`))
	}))
	defer target1.Close()

	target2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ip=203.0.113.50\nloc=JP\nasn=2516\n"))
	}))
	defer target2.Close()

	config := testNode(domain.Protocol("socks5"))
	config.Server = proxy.Address()
	config.Port = proxy.Port()
	config.LogicalID = "node_0123456789abcdef"

	opts := singbox.IPRiskChannelOptions{
		Timeout:          3 * time.Second,
		MaxResponseBytes: 64 * 1024,
	}
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, opts)
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	defer closeRuntime()

	resolver := identity.NewResolver(identity.WithTTL(10 * time.Minute))
	sources := []identity.Source{
		identity.NewHTTPSource("mock_ipsb", target1.URL),
		identity.NewHTTPSource("mock_cf", target2.URL),
	}

	// 1. Initial resolution should succeed and reach verified consensus
	res, err := resolver.Resolve(context.Background(), channel.HTTPClient(), channel.LogicalID(), sources)
	if err != nil {
		t.Fatalf("resolver.Resolve() error = %v", err)
	}

	if res.Status != identity.StatusVerified {
		t.Fatalf("status = %s, want verified", res.Status)
	}
	if res.Identity == nil {
		t.Fatal("expected non-nil ExitIdentity")
	}
	if res.Identity.CountryCode != "JP" {
		t.Fatalf("countryCode = %s, want JP", res.Identity.CountryCode)
	}
	if res.Identity.ASN == nil || *res.Identity.ASN != 2516 {
		t.Fatalf("ASN = %v, want 2516", res.Identity.ASN)
	}

	// Verify zero host proxy leakage
	if hostProxy.RequestCount() != 0 {
		t.Fatalf("host proxy received %d requests; expected 0 (Proxy:nil violated)", hostProxy.RequestCount())
	}
	// Verify outbound traffic traversed the node proxy
	initialConns := proxy.HandledConnections()
	if initialConns < 2 {
		t.Fatalf("proxy handled %d connections; expected at least 2", initialConns)
	}

	// 2. Second resolution should hit cache and not make new proxy connections
	cachedRes, err := resolver.Resolve(context.Background(), channel.HTTPClient(), channel.LogicalID(), sources)
	if err != nil {
		t.Fatalf("cached resolver.Resolve() error = %v", err)
	}
	if cachedRes.Status != identity.StatusVerified {
		t.Fatalf("cached status = %s, want verified", cachedRes.Status)
	}
	if proxy.HandledConnections() != initialConns {
		t.Fatalf("proxy connections increased on cache hit: %d != %d", proxy.HandledConnections(), initialConns)
	}
}

func TestIPRiskChannelExitIdentityDisagreementYieldsConflicted(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	// Target 1 reports JP, Target 2 reports US
	target1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"203.0.113.50","country_code":"JP","asn":2516}`))
	}))
	defer target1.Close()

	target2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ip=203.0.113.50\nloc=US\nasn=2516\n"))
	}))
	defer target2.Close()

	config := testNode(domain.Protocol("socks5"))
	config.Server = proxy.Address()
	config.Port = proxy.Port()
	config.LogicalID = "node_0123456789abcdef"

	opts := singbox.IPRiskChannelOptions{Timeout: 3 * time.Second}
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, opts)
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	defer closeRuntime()

	resolver := identity.NewResolver()
	sources := []identity.Source{
		identity.NewHTTPSource("mock_ipsb", target1.URL),
		identity.NewHTTPSource("mock_cf", target2.URL),
	}

	res, err := resolver.Resolve(context.Background(), channel.HTTPClient(), channel.LogicalID(), sources)
	if err != nil {
		t.Fatalf("resolver.Resolve() error = %v", err)
	}

	if res.Status != identity.StatusConflicted {
		t.Fatalf("status = %s, want conflicted", res.Status)
	}
	if res.Identity != nil {
		t.Fatal("conflicted resolution must NOT emit ExitIdentity")
	}
}
