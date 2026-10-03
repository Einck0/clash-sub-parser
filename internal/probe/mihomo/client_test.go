package mihomo_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/mihomo"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/constant"
)

type mockProxy struct {
	lastMetadata *constant.Metadata
	dialCount    int
	closed       bool
	dialFn       func(ctx context.Context, metadata *constant.Metadata) (net.Conn, error)
}

func (m *mockProxy) Close() error {
	m.closed = true
	return nil
}

func (m *mockProxy) DialContext(ctx context.Context, metadata *constant.Metadata) (constant.Conn, error) {
	m.dialCount++
	m.lastMetadata = metadata
	if m.dialFn != nil {
		c, err := m.dialFn(ctx, metadata)
		if err != nil {
			return nil, err
		}
		return outbound.NewConn(c, &outbound.Base{}), nil
	}
	serverConn, clientConn := net.Pipe()
	go func() {
		buf := make([]byte, 1024)
		_, _ = serverConn.Read(buf)
		resp := "HTTP/1.1 200 OK\r\nContent-Length: 13\r\nContent-Type: text/plain\r\n\r\nHello, client"
		_, _ = serverConn.Write([]byte(resp))
		_ = serverConn.Close()
	}()
	return outbound.NewConn(clientConn, &outbound.Base{}), nil
}

func TestProxyClient_ProxyNilAndEnvIsolation(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://198.51.100.1:8080")
	t.Setenv("HTTPS_PROXY", "http://198.51.100.1:8080")
	t.Setenv("ALL_PROXY", "socks5://198.51.100.1:1080")

	mock := &mockProxy{}
	pc, err := mihomo.NewProxyClientFromOutbound(mock, 5*time.Second, "test-client")
	if err != nil {
		t.Fatalf("NewProxyClientFromOutbound failed: %v", err)
	}
	defer pc.Close()

	transport, ok := pc.Client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", pc.Client.Transport)
	}

	if transport.Proxy != nil {
		t.Fatal("expected transport.Proxy to be explicitly nil to avoid environment proxy leakage")
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatal("expected ForceAttemptHTTP2 to be true")
	}
	if transport.DisableKeepAlives {
		t.Fatal("expected DisableKeepAlives to be false")
	}
}

func TestProxyClient_RemoteDomainDirectTransfer(t *testing.T) {
	mock := &mockProxy{}
	pc, err := mihomo.NewProxyClientFromOutbound(mock, 5*time.Second, "test-client-domain")
	if err != nil {
		t.Fatalf("NewProxyClientFromOutbound failed: %v", err)
	}
	defer pc.Close()

	targetURL := "http://unresolvable-remote-tunnel-target.example.org:9090/check"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, targetURL, nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	resp, err := pc.Client.Do(req)
	if err != nil {
		t.Fatalf("Client.Do failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "Hello, client" {
		t.Fatalf("unexpected body: %q", string(body))
	}

	if mock.dialCount != 1 {
		t.Fatalf("expected dialCount=1, got %d", mock.dialCount)
	}
	if mock.lastMetadata == nil {
		t.Fatal("expected metadata to be passed to DialContext")
	}
	if mock.lastMetadata.Host != "unresolvable-remote-tunnel-target.example.org" {
		t.Fatalf("expected metadata.Host to be 'unresolvable-remote-tunnel-target.example.org', got %q", mock.lastMetadata.Host)
	}
	if mock.lastMetadata.DstPort != 9090 {
		t.Fatalf("expected metadata.DstPort=9090, got %d", mock.lastMetadata.DstPort)
	}

	// Verify byte counters
	if pc.BytesRead == nil || *pc.BytesRead == 0 {
		t.Fatal("expected BytesRead > 0")
	}
	if pc.BytesWritten == nil || *pc.BytesWritten == 0 {
		t.Fatal("expected BytesWritten > 0")
	}
}

func TestProxyClient_Close(t *testing.T) {
	mock := &mockProxy{}
	pc, err := mihomo.NewProxyClientFromOutbound(mock, 100*time.Millisecond, "test-client-close")
	if err != nil {
		t.Fatalf("NewProxyClientFromOutbound failed: %v", err)
	}

	if err := pc.Close(); err != nil {
		t.Fatalf("pc.Close() failed: %v", err)
	}

	if !mock.closed {
		t.Fatal("expected mockProxy to be closed")
	}

	// Repeated Close is idempotent and does not panic
	if err := pc.Close(); err != nil {
		t.Fatalf("second pc.Close() failed: %v", err)
	}
}

func TestProxyClient_RealHTTPSServerRouting(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok-routed"))
	}))
	defer ts.Close()

	mock := &mockProxy{
		dialFn: func(ctx context.Context, metadata *constant.Metadata) (net.Conn, error) {
			return net.Dial("tcp", ts.Listener.Addr().String())
		},
	}

	pc, err := mihomo.NewProxyClientFromOutbound(mock, 5*time.Second, "test-routing")
	if err != nil {
		t.Fatalf("NewProxyClientFromOutbound failed: %v", err)
	}
	defer pc.Close()

	resp, err := pc.Client.Get("http://probe-custom-domain.org/")
	if err != nil {
		t.Fatalf("Get request failed: %v", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if string(data) != "ok-routed" {
		t.Fatalf("expected 'ok-routed', got %q", string(data))
	}
}
