package singbox

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
)

type boxRuntime struct {
	instance   *box.Box
	closedFlag atomic.Bool
	once       sync.Once
}

func (r *boxRuntime) closed() bool {
	return r.closedFlag.Load()
}

func (r *boxRuntime) close() error {
	var err error
	r.once.Do(func() {
		r.closedFlag.Store(true)
		if r.instance != nil {
			err = r.instance.Close()
		}
	})
	return err
}

// Runtime implements Adapter by embedding an in-memory sing-box instance.
type Runtime struct {
	box       *boxRuntime
	outbound  adapter.Outbound
	tag       string
	logicalID string
	protocol  domain.Protocol
	stopFunc  func() bool
}

// BoxBuildConfig allows test fixtures to customize the in-memory sing-box Options and Context
// without altering production default registries, TLS verification, or IP policies.
type BoxBuildConfig struct {
	Context func(ctx context.Context) context.Context
	Mutate  func(options *option.Options, tag string) error
}

// New creates and starts an isolated in-memory sing-box runtime for the given node configuration.
func New(ctx context.Context, config NodeConfig) (*Runtime, error) {
	return NewWithBoxConfig(ctx, config, nil)
}

// NewWithBoxConfig creates and starts an isolated in-memory sing-box runtime with optional BoxBuildConfig.
func NewWithBoxConfig(ctx context.Context, config NodeConfig, buildCfg *BoxBuildConfig) (*Runtime, error) {
	options, tag, err := BuildOptions(config)
	if err != nil {
		return nil, err
	}
	if buildCfg != nil && buildCfg.Mutate != nil {
		if err := buildCfg.Mutate(&options, tag); err != nil {
			return nil, err
		}
	}

	var bCtx context.Context
	if buildCfg != nil && buildCfg.Context != nil {
		bCtx = buildCfg.Context(ctx)
	} else {
		bCtx = runtimeContext(ctx)
	}

	instance, err := box.New(box.Options{
		Context: bCtx,
		Options: options,
	})
	if err != nil {
		return nil, fmt.Errorf("create sing-box runtime: %w", err)
	}

	if err := instance.Start(); err != nil {
		_ = instance.Close()
		return nil, fmt.Errorf("start sing-box runtime: %w", err)
	}

	var ob adapter.Outbound
	if item, ok := instance.Outbound().Outbound(tag); ok {
		ob = item
	} else if def := instance.Outbound().Default(); def != nil {
		ob = def
	} else if endpoint, ok := instance.Endpoint().Get(tag); ok {
		ob = endpoint
	}

	if ob == nil {
		_ = instance.Close()
		return nil, ErrMissingOutbound
	}

	rt := &Runtime{
		box:       &boxRuntime{instance: instance},
		outbound:  ob,
		tag:       tag,
		logicalID: config.LogicalID,
		protocol:  config.Protocol,
	}

	if ctx != nil && ctx.Done() != nil {
		rt.stopFunc = context.AfterFunc(ctx, func() {
			_ = rt.Close()
		})
	}

	return rt, nil
}

// NewHTTPClient creates an HTTP client bound to an ephemeral runtime, returning the client and a cleanup func.
func NewHTTPClient(ctx context.Context, config NodeConfig, options HTTPClientOptions) (*http.Client, func() error, error) {
	runtime, err := New(ctx, config)
	if err != nil {
		return nil, nil, err
	}
	return runtime.HTTPClient(options), runtime.Close, nil
}

// LogicalID returns the logical ID of the tested node.
func (r *Runtime) LogicalID() string {
	return r.logicalID
}

// Protocol returns the protocol of the tested node.
func (r *Runtime) Protocol() domain.Protocol {
	return r.protocol
}

// Tag returns the outbound tag inside sing-box.
func (r *Runtime) Tag() string {
	return r.tag
}

// IsClosed reports whether the runtime has been closed.
func (r *Runtime) IsClosed() bool {
	if r == nil || r.box == nil {
		return true
	}
	return r.box.closed()
}

// Outbound returns the underlying sing-box adapter.Outbound.
func (r *Runtime) Outbound() adapter.Outbound {
	return r.outbound
}

// Close stops the sing-box instance and releases all sockets, goroutines, and memory structures.
func (r *Runtime) Close() error {
	if r == nil || r.box == nil {
		return nil
	}
	if r.stopFunc != nil {
		r.stopFunc()
	}
	return r.box.close()
}

type upstreamProvider interface {
	Upstream() any
}

type safeRuntimeConn struct {
	net.Conn
	upstream    io.Closer
	readCloseMu sync.Mutex
}

func wrapSafeRuntimeConn(conn net.Conn) net.Conn {
	if conn == nil {
		return nil
	}
	var upstream io.Closer
	if u, ok := conn.(upstreamProvider); ok {
		if c, ok := u.Upstream().(io.Closer); ok {
			upstream = c
		}
	}
	return &safeRuntimeConn{
		Conn:     conn,
		upstream: upstream,
	}
}

func (c *safeRuntimeConn) Read(p []byte) (int, error) {
	c.readCloseMu.Lock()
	defer c.readCloseMu.Unlock()
	return c.Conn.Read(p)
}

func (c *safeRuntimeConn) Close() error {
	if c.upstream != nil {
		_ = c.upstream.Close()
	}
	c.readCloseMu.Lock()
	defer c.readCloseMu.Unlock()
	return c.Conn.Close()
}

// DialContext dials the target address strictly through this runtime's sing-box outbound.
func (r *Runtime) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if r == nil || r.box == nil || r.box.closed() {
		return nil, ErrRuntimeClosed
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	conn, err := r.outbound.DialContext(ctx, network, M.ParseSocksaddr(address))
	if err != nil {
		return nil, err
	}
	return wrapSafeRuntimeConn(conn), nil
}

// ListenPacket creates an isolated packet connection for UDP communications through the outbound.
func (r *Runtime) ListenPacket(ctx context.Context, destination string) (net.PacketConn, error) {
	if r == nil || r.box == nil || r.box.closed() {
		return nil, ErrRuntimeClosed
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return r.outbound.ListenPacket(ctx, M.ParseSocksaddr(destination))
}

// HTTPClient constructs an http.Client strictly bound to this runtime's outbound.
// It explicitly sets Proxy to nil to ensure no host HTTP_PROXY / HTTPS_PROXY / ALL_PROXY is used.
func (r *Runtime) HTTPClient(options HTTPClientOptions) *http.Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	handshakeTimeout := options.TLSHandshakeTimeout
	if handshakeTimeout <= 0 {
		handshakeTimeout = 5 * time.Second
	}
	idleTimeout := options.IdleConnTimeout
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Second
	}

	resolver := options.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	policy := fetch.DefaultPolicy()
	policy.Resolver = resolver
	dialTarget := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid probe destination: %w", err)
		}
		var approved net.IP
		if literal := net.ParseIP(strings.Trim(host, "[]")); literal != nil {
			if err := policy.ValidateIP(literal); err != nil {
				return nil, fmt.Errorf("probe destination blocked: %w", err)
			}
			approved = literal
		} else {
			ips, err := resolver.LookupIPAddr(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, fmt.Errorf("probe destination DNS resolution failed")
			}
			for _, candidate := range ips {
				if err := policy.ValidateIP(candidate.IP); err != nil {
					return nil, fmt.Errorf("probe destination blocked: %w", err)
				}
				if approved == nil {
					approved = candidate.IP
				}
			}
		}
		return r.DialContext(ctx, network, net.JoinHostPort(approved.String(), port))
	}
	transport := &http.Transport{
		// A nil Proxy prevents net/http from consulting HTTP_PROXY, HTTPS_PROXY, or ALL_PROXY
		Proxy:                 nil,
		DialContext:           dialTarget,
		ForceAttemptHTTP2:     options.ForceAttemptHTTP2,
		MaxIdleConns:          10,
		IdleConnTimeout:       idleTimeout,
		TLSHandshakeTimeout:   handshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
