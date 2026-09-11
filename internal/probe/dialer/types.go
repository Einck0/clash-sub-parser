package dialer

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/sagernet/sing-box/adapter"
)

var (
	// ErrNilNode is returned when a nil *domain.Node is passed.
	ErrNilNode = errors.New("node is nil")

	// ErrUnsupportedProtocol is returned when a node's protocol is not supported.
	ErrUnsupportedProtocol = errors.New("unsupported protocol for sing-box dialer")

	// ErrMissingOutbound is returned when an outbound could not be resolved from sing-box runtime.
	ErrMissingOutbound = errors.New("outbound not found in sing-box runtime")

	// ErrDialerClosed is returned when dialing on a closed dialer.
	ErrDialerClosed = errors.New("dialer is closed")
)

// Dialer defines the interface for pure in-memory proxy dialing backed by sing-box outbounds.
type Dialer interface {
	// DialContext connects to the destination address through the proxy outbound with context.
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)

	// Dial connects to the destination address through the proxy outbound.
	Dial(network, addr string) (net.Conn, error)

	// ListenPacket creates an in-memory packet connection for UDP communications through the outbound.
	ListenPacket(ctx context.Context, destination string) (net.PacketConn, error)

	// Outbound returns the underlying sing-box adapter.Outbound.
	Outbound() adapter.Outbound

	// HTTPClient constructs an isolated http.Client using this dialer with environment proxies stripped.
	HTTPClient(opts HTTPClientOptions) *http.Client

	// Close shuts down the sing-box runtime and releases in-memory resources and active connections.
	Close() error
}

// HTTPClientOptions configures HTTP client creation from a dialer.
type HTTPClientOptions struct {
	Timeout               time.Duration
	TLSHandshakeTimeout   time.Duration
	IdleConnTimeout       time.Duration
	ForceAttemptHTTP2     bool
}
