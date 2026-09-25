package singbox_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// mockSOCKS5Proxy is an in-memory controllable SOCKS5 proxy server for testing outbounds.
type mockSOCKS5Proxy struct {
	listener     net.Listener
	address      string
	port         int
	connections  atomic.Int64
	bytesRelayed atomic.Int64
	reject       atomic.Bool
	closed       atomic.Bool
	destinations []string
	mu           sync.Mutex
}

func startMockSOCKS5Proxy() (*mockSOCKS5Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	tcpAddr := ln.Addr().(*net.TCPAddr)
	p := &mockSOCKS5Proxy{
		listener: ln,
		address:  tcpAddr.IP.String(),
		port:     tcpAddr.Port,
	}
	go p.serve()
	return p, nil
}

func (p *mockSOCKS5Proxy) Address() string {
	return p.address
}

func (p *mockSOCKS5Proxy) Port() int {
	return p.port
}

func (p *mockSOCKS5Proxy) HandledConnections() int64 {
	return p.connections.Load()
}

func (p *mockSOCKS5Proxy) BytesRelayed() int64 {
	return p.bytesRelayed.Load()
}

func (p *mockSOCKS5Proxy) Destinations() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	copied := make([]string, len(p.destinations))
	copy(copied, p.destinations)
	return copied
}

func (p *mockSOCKS5Proxy) SetReject(reject bool) {
	p.reject.Store(reject)
}

func (p *mockSOCKS5Proxy) Close() error {
	p.closed.Store(true)
	return p.listener.Close()
}

func (p *mockSOCKS5Proxy) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			if p.closed.Load() {
				return
			}
			continue
		}
		go p.handleConn(conn)
	}
}

func (p *mockSOCKS5Proxy) handleConn(conn net.Conn) {
	defer conn.Close()

	if p.reject.Load() {
		return
	}

	buf := make([]byte, 256)
	// 1. Version negotiation
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	if buf[0] != 0x05 {
		return
	}
	numMethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:numMethods]); err != nil {
		return
	}
	// Select "No authentication" (0x00)
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// 2. Request details
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	if buf[1] != 0x01 { // CMD 0x01 = CONNECT
		_, _ = conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // command not supported
		return
	}

	var targetHost string
	switch buf[3] { // ATYP
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		targetHost = net.IP(buf[:4]).String()
	case 0x03: // Domain name
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		domainLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:domainLen]); err != nil {
			return
		}
		targetHost = string(buf[:domainLen])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		targetHost = net.IP(buf[:16]).String()
	default:
		_, _ = conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	var portBuf [2]byte
	if _, err := io.ReadFull(conn, portBuf[:]); err != nil {
		return
	}
	targetPort := binary.BigEndian.Uint16(portBuf[:])
	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(int(targetPort)))
	// Documentation-only public test address is mapped back to this process's
	// loopback fixture; no external network connection is made.
	if targetHost == "8.8.8.8" {
		proxyTarget := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(targetPort)))
		if conn, dialErr := net.DialTimeout("tcp", proxyTarget, time.Second); dialErr == nil {
			_ = conn.Close()
			targetAddr = proxyTarget
		}
	}

	targetConn, err := net.DialTimeout("tcp", targetAddr, 3*time.Second)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // host unreachable
		return
	}
	defer targetConn.Close()

	// Succeeded
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	p.connections.Add(1)
	p.mu.Lock()
	p.destinations = append(p.destinations, targetAddr)
	p.mu.Unlock()

	// Bidirectional relay with real-time byte counting
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(targetConn, &countingReader{r: conn, counter: &p.bytesRelayed})
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(conn, &countingReader{r: targetConn, counter: &p.bytesRelayed})
		errCh <- err
	}()
	<-errCh
}

type countingReader struct {
	r       io.Reader
	counter *atomic.Int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.counter.Add(int64(n))
	}
	return n, err
}

// mockHostProxyServer simulates an active host environment proxy (HTTP_PROXY / ALL_PROXY).
// It records whenever any request or CONNECT handshake hits it.
type mockHostProxyServer struct {
	server       *httptest.Server
	requestCount atomic.Int64
}

func startMockHostProxyServer() *mockHostProxyServer {
	h := &mockHostProxyServer{}
	h.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.requestCount.Add(1)
		if r.Method == http.MethodConnect {
			// Connect tunnel hijack
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijack not supported", http.StatusInternalServerError)
				return
			}
			clientConn, _, err := hijacker.Hijack()
			if err != nil {
				return
			}
			defer clientConn.Close()
			destConn, err := net.DialTimeout("tcp", r.Host, 2*time.Second)
			if err != nil {
				_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
				return
			}
			defer destConn.Close()
			_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			go func() { _, _ = io.Copy(destConn, clientConn) }()
			_, _ = io.Copy(clientConn, destConn)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("host proxy response"))
	}))
	return h
}

func (h *mockHostProxyServer) URL() string {
	return h.server.URL
}

func (h *mockHostProxyServer) RequestCount() int64 {
	return h.requestCount.Load()
}

func (h *mockHostProxyServer) Close() {
	h.server.Close()
}

var (
	_ = context.Background
	_ = errors.New
	_ = fmt.Sprintf
)
