package mihomo_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
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

func TestProxyClient_LocalHTTPProxyFixture(t *testing.T) {
	// Backend target server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-http-response"))
	}))
	defer backend.Close()

	// Local HTTP CONNECT proxy
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy failed: %v", err)
	}
	defer proxyLn.Close()

	go func() {
		for {
			conn, err := proxyLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					return
				}
				reqStr := string(buf[:n])
				if len(reqStr) >= 7 && reqStr[:7] == "CONNECT" {
					// Extract host:port
					parts := strings.Split(reqStr, " ")
					if len(parts) < 2 {
						return
					}
					targetAddr := parts[1]
					targetConn, err := net.DialTimeout("tcp", targetAddr, 2*time.Second)
					if err != nil {
						_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
						return
					}
					defer targetConn.Close()
					_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

					errCh := make(chan struct{}, 2)
					go func() { _, _ = io.Copy(targetConn, c); errCh <- struct{}{} }()
					go func() { _, _ = io.Copy(c, targetConn); errCh <- struct{}{} }()
					<-errCh
				}
			}(conn)
		}
	}()

	proxyHost, proxyPortStr, _ := net.SplitHostPort(proxyLn.Addr().String())
	proxyPort, _ := strconv.Atoi(proxyPortStr)

	node := domain.Node{
		LogicalID:   "node-fixture-http",
		DisplayName: "Local HTTP Fixture",
		Protocol:    domain.ProtocolHTTP,
		Server:      proxyHost,
		Port:        proxyPort,
	}

	proxy, err := mihomo.ParseProxy(node)
	if err != nil {
		t.Fatalf("mihomo.ParseProxy failed: %v", err)
	}

	pc, err := mihomo.NewProxyClientFromOutbound(proxy, 5*time.Second, "local-http-test")
	if err != nil {
		t.Fatalf("NewProxyClientFromOutbound failed: %v", err)
	}
	defer pc.Close()

	// Verify transport proxy is explicitly nil (host env isolation)
	tr := pc.Client.Transport.(*http.Transport)
	if tr.Proxy != nil {
		t.Fatal("expected tr.Proxy to be nil")
	}

	resp, err := pc.Client.Get(backend.URL)
	if err != nil {
		t.Fatalf("pc.Client.Get failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "backend-http-response" {
		t.Fatalf("expected 'backend-http-response', got %q", string(body))
	}

	if err := pc.Close(); err != nil {
		t.Fatalf("pc.Close() failed: %v", err)
	}
}

func TestProxyClient_LocalSocks5ProxyFixture(t *testing.T) {
	// Backend target server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend-socks5-response"))
	}))
	defer backend.Close()

	// Local minimal SOCKS5 proxy
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen socks5 proxy failed: %v", err)
	}
	defer proxyLn.Close()

	go func() {
		for {
			conn, err := proxyLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// SOCKS5 greeting
				buf := make([]byte, 256)
				n, err := c.Read(buf)
				if err != nil || n < 2 || buf[0] != 0x05 {
					return
				}
				// Reply: version 5, no authentication required
				_, _ = c.Write([]byte{0x05, 0x00})

				// SOCKS5 request: 0x05, 0x01 (CONNECT), 0x00, ATYP
				n, err = c.Read(buf)
				if err != nil || n < 7 || buf[0] != 0x05 || buf[1] != 0x01 {
					return
				}

				var destAddr string
				switch buf[3] {
				case 0x01: // IPv4
					ip := net.IP(buf[4:8])
					port := (int(buf[8]) << 8) | int(buf[9])
					destAddr = fmt.Sprintf("%s:%d", ip.String(), port)
				case 0x03: // Domain name
					domainLen := int(buf[4])
					domain := string(buf[5 : 5+domainLen])
					port := (int(buf[5+domainLen]) << 8) | int(buf[6+domainLen])
					destAddr = fmt.Sprintf("%s:%d", domain, port)
				default:
					return
				}

				targetConn, err := net.DialTimeout("tcp", destAddr, 2*time.Second)
				if err != nil {
					_, _ = c.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
					return
				}
				defer targetConn.Close()

				// Success reply
				_, _ = c.Write([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x04, 0x38})

				errCh := make(chan struct{}, 2)
				go func() { _, _ = io.Copy(targetConn, c); errCh <- struct{}{} }()
				go func() { _, _ = io.Copy(c, targetConn); errCh <- struct{}{} }()
				<-errCh
			}(conn)
		}
	}()

	proxyHost, proxyPortStr, _ := net.SplitHostPort(proxyLn.Addr().String())
	proxyPort, _ := strconv.Atoi(proxyPortStr)

	node := domain.Node{
		LogicalID:   "node-fixture-socks5",
		DisplayName: "Local SOCKS5 Fixture",
		Protocol:    domain.ProtocolSocks5,
		Server:      proxyHost,
		Port:        proxyPort,
	}

	proxy, err := mihomo.ParseProxy(node)
	if err != nil {
		t.Fatalf("mihomo.ParseProxy failed: %v", err)
	}

	pc, err := mihomo.NewProxyClientFromOutbound(proxy, 5*time.Second, "local-socks5-test")
	if err != nil {
		t.Fatalf("NewProxyClientFromOutbound failed: %v", err)
	}
	defer pc.Close()

	// Verify transport proxy is explicitly nil (host env isolation)
	tr := pc.Client.Transport.(*http.Transport)
	if tr.Proxy != nil {
		t.Fatal("expected tr.Proxy to be nil")
	}

	resp, err := pc.Client.Get(backend.URL)
	if err != nil {
		t.Fatalf("pc.Client.Get via SOCKS5 failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "backend-socks5-response" {
		t.Fatalf("expected 'backend-socks5-response', got %q", string(body))
	}

	if err := pc.Close(); err != nil {
		t.Fatalf("pc.Close() failed: %v", err)
	}
}
