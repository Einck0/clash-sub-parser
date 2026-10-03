package mihomo

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"clash-sub-parser/internal/domain"
	"github.com/metacubex/mihomo/constant"
)

// ProxyOutbound abstracts the minimal outbound connection methods needed by ProxyClient.
type ProxyOutbound interface {
	DialContext(ctx context.Context, metadata *constant.Metadata) (constant.Conn, error)
	Close() error
}

// ProxyClient provides an HTTP client routed through a single Mihomo in-process outbound.
type ProxyClient struct {
	*http.Client
	Proxy        ProxyOutbound
	BytesRead    *uint64
	BytesWritten *uint64

	ctx           context.Context
	cancel        context.CancelFunc
	baseTransport *http.Transport
}

// NewProxyClient creates a ProxyClient for the given domain.Node with explicit timeout.
func NewProxyClient(node domain.Node, timeout time.Duration) (*ProxyClient, error) {
	proxy, err := ParseProxy(node)
	if err != nil {
		return nil, err
	}

	return NewProxyClientFromOutbound(proxy, timeout, node.DisplayName)
}

// NewProxyClientFromOutbound creates a ProxyClient wrapping an existing ProxyOutbound.
func NewProxyClientFromOutbound(proxy ProxyOutbound, timeout time.Duration, name string) (*ProxyClient, error) {
	if proxy == nil {
		return nil, fmt.Errorf("nil proxy outbound")
	}

	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	var bytesRead uint64
	var bytesWritten uint64

	baseTransport := &http.Transport{
		DialContext: func(reqCtx context.Context, network, addr string) (net.Conn, error) {
			mergedCtx, mergedCancel := context.WithCancel(reqCtx)
			stop := context.AfterFunc(ctx, func() {
				mergedCancel()
			})
			defer mergedCancel()
			defer stop()

			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			var u16Port uint16
			if p, err := strconv.ParseUint(portStr, 10, 16); err == nil {
				u16Port = uint16(p)
			}

			// Destination domain name is sent directly to remote proxy without local DNS lookup.
			metadata := &constant.Metadata{
				Host:    host,
				DstPort: u16Port,
			}

			conn, err := proxy.DialContext(mergedCtx, metadata)
			if err != nil {
				return nil, err
			}

			return &statsConn{
				Conn:         conn,
				bytesRead:    &bytesRead,
				bytesWritten: &bytesWritten,
			}, nil
		},
		// Strict isolation: never inherit host environment proxy settings
		Proxy:               nil,
		ForceAttemptHTTP2:   true,
		DisableKeepAlives:   false,
		IdleConnTimeout:     10 * time.Second,
		MaxIdleConnsPerHost: 5,
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: baseTransport,
	}

	pc := &ProxyClient{
		Client:        client,
		Proxy:         proxy,
		BytesRead:     &bytesRead,
		BytesWritten:  &bytesWritten,
		ctx:           ctx,
		cancel:        cancel,
		baseTransport: baseTransport,
	}

	return pc, nil
}

// Close gracefully cancels pending requests and cleans up underlying network resources.
func (pc *ProxyClient) Close() error {
	if pc == nil {
		return nil
	}
	if pc.cancel != nil {
		pc.cancel()
	}
	if pc.Client != nil {
		pc.Client.CloseIdleConnections()
	}
	if pc.Proxy != nil {
		p := pc.Proxy
		done := make(chan struct{})
		go func() {
			_ = p.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			slog.Debug("closing proxy outbound timed out, leaving for gc")
		}
	}
	return nil
}

type statsConn struct {
	net.Conn
	bytesRead    *uint64
	bytesWritten *uint64
}

func (c *statsConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if n > 0 && c.bytesRead != nil {
		atomic.AddUint64(c.bytesRead, uint64(n))
	}
	return n, err
}

func (c *statsConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if n > 0 && c.bytesWritten != nil {
		atomic.AddUint64(c.bytesWritten, uint64(n))
	}
	return n, err
}

// NewNodeDialer returns a dialer closure conforming to probe.NodeDialer signature.
func NewNodeDialer(timeout time.Duration) func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
	return func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		pc, err := NewProxyClient(node, timeout)
		if err != nil {
			return nil, nil, err
		}
		cleanup := func() error {
			return pc.Close()
		}
		return pc.Client, cleanup, nil
	}
}
