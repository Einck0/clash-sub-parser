package pipeline_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/dialer"
	"clash-sub-parser/internal/probe/pipeline"
	"github.com/sagernet/sing-box/adapter"
)

type mockDialer struct {
	dialCount atomic.Int64
	closed    atomic.Bool
}

func (m *mockDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	m.dialCount.Add(1)
	clientConn, serverConn := net.Pipe()
	go func() {
		defer serverConn.Close()
		buf := make([]byte, 1024)
		n, _ := serverConn.Read(buf)
		if n > 0 {
			resp := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"
			_, _ = serverConn.Write([]byte(resp))
		}
	}()
	return clientConn, nil
}

func (m *mockDialer) Dial(network, addr string) (net.Conn, error) {
	return m.DialContext(context.Background(), network, addr)
}

func (m *mockDialer) ListenPacket(ctx context.Context, destination string) (net.PacketConn, error) {
	return nil, nil
}

func (m *mockDialer) Outbound() adapter.Outbound {
	return nil
}

func (m *mockDialer) HTTPClient(opts dialer.HTTPClientOptions) *http.Client {
	return pipeline.NewIsolatedHTTPClient(m, opts.Timeout)
}

func (m *mockDialer) Close() error {
	m.closed.Store(true)
	return nil
}

func TestNewIsolatedHTTPClient_StrippedEnvironmentProxies(t *testing.T) {
	// Set dummy proxy env vars that would break connections if used
	_ = os.Setenv("HTTP_PROXY", "http://invalid-dummy-proxy-12345.local:8080")
	_ = os.Setenv("HTTPS_PROXY", "http://invalid-dummy-proxy-12345.local:8080")
	_ = os.Setenv("ALL_PROXY", "socks5://invalid-dummy-proxy-12345.local:1080")
	defer func() {
		_ = os.Unsetenv("HTTP_PROXY")
		_ = os.Unsetenv("HTTPS_PROXY")
		_ = os.Unsetenv("ALL_PROXY")
	}()

	d := &mockDialer{}
	client := pipeline.NewIsolatedHTTPClient(d, 2*time.Second)

	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}

	// Verify Proxy is strictly nil, never inheriting ProxyFromEnvironment
	if tr.Proxy != nil {
		t.Errorf("expected tr.Proxy == nil, got non-nil")
	}
	if !tr.DisableKeepAlives {
		t.Errorf("expected DisableKeepAlives == true for physical isolation")
	}

	// Execute an HTTP request to ensure it dials through the mockDialer, NOT the invalid environment proxy
	req, err := http.NewRequestWithContext(context.Background(), "GET", "http://test.local/probe", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed unexpectedly (possibly leaked to env proxy): %v", err)
	}
	defer resp.Body.Close()

	if d.dialCount.Load() == 0 {
		t.Errorf("expected dialCount > 0 through mockDialer")
	}
}

func TestNewIsolatedHTTPClient_SeparateInstancesAreIsolated(t *testing.T) {
	d1 := &mockDialer{}
	d2 := &mockDialer{}

	c1 := pipeline.NewIsolatedHTTPClient(d1, 2*time.Second)
	c2 := pipeline.NewIsolatedHTTPClient(d2, 2*time.Second)

	req1, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test.local/1", nil)
	req2, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test.local/2", nil)

	_, err1 := c1.Do(req1)
	if err1 != nil {
		t.Fatalf("req1 failed: %v", err1)
	}
	_, err2 := c2.Do(req2)
	if err2 != nil {
		t.Fatalf("req2 failed: %v", err2)
	}

	if d1.dialCount.Load() != 1 {
		t.Errorf("expected d1 dialCount == 1, got %d", d1.dialCount.Load())
	}
	if d2.dialCount.Load() != 1 {
		t.Errorf("expected d2 dialCount == 1, got %d", d2.dialCount.Load())
	}
}
