package pipeline

import (
	"context"
	"net"
	"net/http"
	"time"

	"clash-sub-parser/internal/probe/dialer"
)

// ContextDialer abstracts the DialContext capability required to isolate HTTP transport egress.
type ContextDialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// NewIsolatedHTTPClient constructs an http.Client strictly bound to the given dialer.
// It explicitly sets Proxy to nil to prevent environment proxy leakage (HTTP_PROXY, HTTPS_PROXY, ALL_PROXY)
// and sets DisableKeepAlives to true so that no socket or connection state is reused across different probes.
func NewIsolatedHTTPClient(d dialer.Dialer, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	var dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)
	if d != nil {
		dialFunc = d.DialContext
	}

	transport := &http.Transport{
		DialContext:           dialFunc,
		Proxy:                 nil,  // Invariant: NEVER inherit host environment proxies (HTTP_PROXY / ALL_PROXY)
		DisableKeepAlives:     true, // Invariant: Absolute physical connection isolation per probe
		MaxIdleConns:          -1,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
