package singbox_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/singbox"
)

func TestIPRiskChannelNeverUsesEnvironmentProxy(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"203.0.113.50","status":"ok"}`))
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

	opts := singbox.IPRiskChannelOptions{
		Timeout:             3 * time.Second,
		TLSHandshakeTimeout: 1 * time.Second,
		MaxResponseBytes:    64 * 1024,
	}
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, opts)
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	defer closeRuntime()

	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := channel.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("channel.Do() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("statusCode = %d, want 200", resp.StatusCode)
	}
	if string(resp.Body) != `{"ip":"203.0.113.50","status":"ok"}` {
		t.Fatalf("unexpected response body: %q", string(resp.Body))
	}
	if hostProxy.RequestCount() != 0 {
		t.Fatalf("host proxy intercepted %d requests; expected 0 (Proxy:nil violated)", hostProxy.RequestCount())
	}
	if proxy.HandledConnections() == 0 {
		t.Fatal("SOCKS5 proxy handled 0 connections; traffic did not go through node outbound")
	}
}

func TestIPRiskChannelTimeoutIsolation(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	// Slow target that takes 1 second to respond
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		_, _ = w.Write([]byte(`{"slow":true}`))
	}))
	defer target.Close()

	config := testNode(domain.Protocol("socks5"))
	config.Server = proxy.Address()
	config.Port = proxy.Port()

	// Strict isolated timeout: 100ms
	opts := singbox.IPRiskChannelOptions{
		Timeout:          100 * time.Millisecond,
		MaxResponseBytes: 1024,
	}
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, opts)
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	defer closeRuntime()

	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	start := time.Now()
	resp, err := channel.Do(context.Background(), req)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if duration > 800*time.Millisecond {
		t.Fatalf("probe took %v; timeout isolation failed to abort within reasonable bounds", duration)
	}
	if resp == nil || !resp.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded to be true in response, got: %+v", resp)
	}
}

func TestIPRiskChannelContextCancellationIsolation(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer target.Close()

	config := testNode(domain.Protocol("socks5"))
	config.Server = proxy.Address()
	config.Port = proxy.Port()

	opts := singbox.IPRiskChannelOptions{
		Timeout: 5 * time.Second,
	}
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, opts)
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	defer closeRuntime()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	start := time.Now()
	_, err = channel.Do(ctx, req)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if duration > 500*time.Millisecond {
		t.Fatalf("cancellation took %v; expected abort within 500ms", duration)
	}
}

func TestIPRiskChannelMaxResponseBytesEnforced(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		largePayload := make([]byte, 10*1024) // 10 KB
		for i := range largePayload {
			largePayload[i] = 'A'
		}
		_, _ = w.Write(largePayload)
	}))
	defer target.Close()

	config := testNode(domain.Protocol("socks5"))
	config.Server = proxy.Address()
	config.Port = proxy.Port()

	// Restrict to 1 KB
	opts := singbox.IPRiskChannelOptions{
		Timeout:          2 * time.Second,
		MaxResponseBytes: 1024,
	}
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, opts)
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	defer closeRuntime()

	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	_, err = channel.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error due to exceeding MaxResponseBytes, got nil")
	}
	if !errors.Is(err, singbox.ErrIPRiskResponseTooLarge) {
		t.Fatalf("expected ErrIPRiskResponseTooLarge, got: %v", err)
	}
}

func TestIPRiskChannelReportsConfiguredOutboundFailureWithoutHostFallback(t *testing.T) {
	hostProxy := startMockHostProxyServer()
	defer hostProxy.Close()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, hostProxy.URL())
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	closedPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	config := testNode(domain.Protocol("socks5"))
	config.Server = "127.0.0.1"
	config.Port = closedPort

	opts := singbox.IPRiskChannelOptions{
		Timeout: 1 * time.Second,
	}
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, opts)
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	defer closeRuntime()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("should-never-reach"))
	}))
	defer target.Close()

	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := channel.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected outbound failure error, got nil")
	}
	if hostProxy.RequestCount() != 0 {
		t.Fatalf("host proxy intercepted %d requests; node failure fell back to host proxy", hostProxy.RequestCount())
	}
	if resp != nil && !resp.NetworkError && !resp.DeadlineExceeded {
		t.Fatalf("expected NetworkError or DeadlineExceeded in response, got: %+v", resp)
	}
}

func TestIPRiskChannelClosedRuntimeRejection(t *testing.T) {
	config := testNode(domain.ProtocolSS)
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, singbox.IPRiskChannelOptions{})
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	_ = closeRuntime()

	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:18080", nil)
	_, err = channel.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on closed runtime, got nil")
	}
	if !errors.Is(err, singbox.ErrRuntimeClosed) {
		t.Fatalf("expected ErrRuntimeClosed, got: %v", err)
	}
}

func TestIPRiskChannelConcurrentProbesRaceClean(t *testing.T) {
	proxy, err := startMockSOCKS5Proxy()
	if err != nil {
		t.Fatalf("startMockSOCKS5Proxy() error = %v", err)
	}
	defer proxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer target.Close()

	config := testNode(domain.Protocol("socks5"))
	config.Server = proxy.Address()
	config.Port = proxy.Port()

	opts := singbox.IPRiskChannelOptions{
		Timeout:          2 * time.Second,
		MaxResponseBytes: 4096,
	}
	channel, closeRuntime, err := singbox.NewIPRiskChannel(context.Background(), config, opts)
	if err != nil {
		t.Fatalf("NewIPRiskChannel() error = %v", err)
	}
	defer closeRuntime()

	var wg sync.WaitGroup
	concurrent := 16
	errCh := make(chan error, concurrent)

	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, reqErr := http.NewRequest(http.MethodGet, target.URL, nil)
			if reqErr != nil {
				errCh <- reqErr
				return
			}
			resp, doErr := channel.Do(context.Background(), req)
			if doErr != nil {
				errCh <- doErr
				return
			}
			if resp.StatusCode != http.StatusOK {
				errCh <- errors.New("status not 200")
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent probe failed: %v", err)
		}
	}
}
