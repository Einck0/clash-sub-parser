package singbox

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"clash-sub-parser/internal/domain"
)

// Default IP risk timeout and payload limits
const (
	DefaultIPRiskTimeout               = 5 * time.Second
	DefaultIPRiskTLSHandshakeTimeout   = 3 * time.Second
	DefaultIPRiskResponseHeaderTimeout = 3 * time.Second
	DefaultIPRiskIdleConnTimeout       = 15 * time.Second
	DefaultIPRiskMaxResponseBytes      = 512 * 1024 // 512 KB
)

var (
	// ErrIPRiskResponseTooLarge is returned when the response body exceeds MaxResponseBytes.
	ErrIPRiskResponseTooLarge = domain.NewValidationError("response_too_large", "ip risk response exceeded maximum byte limit")

	// ErrIPRiskNilRequest is returned when a nil HTTP request is passed.
	ErrIPRiskNilRequest = domain.NewValidationError("nil_request", "http request is nil")
)

// IPRiskChannelOptions configures an isolated IP risk probe channel.
type IPRiskChannelOptions struct {
	Timeout               time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxResponseBytes      int64
	ForceAttemptHTTP2     bool
}

// IPRiskProbeResponse contains the structured result of an IP risk probe request.
type IPRiskProbeResponse struct {
	StatusCode       int
	Body             []byte
	Headers          http.Header
	LatencyMS        int64
	DeadlineExceeded bool
	DNSError         bool
	NetworkError     bool
}

// IPRiskChannel provides an isolated, outbound-bound probe channel for IP risk queries.
// It guarantees zero environment proxy leakage (Proxy: nil), isolated timeouts per probe,
// and bounded memory consumption.
type IPRiskChannel struct {
	runtime   *Runtime
	options   IPRiskChannelOptions
	client    *http.Client
	transport *http.Transport
}

// newIPRiskChannel constructs an IPRiskChannel for a given sing-box Runtime.
func newIPRiskChannel(rt *Runtime, opts IPRiskChannelOptions) *IPRiskChannel {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultIPRiskTimeout
	}
	tlsTimeout := opts.TLSHandshakeTimeout
	if tlsTimeout <= 0 {
		tlsTimeout = DefaultIPRiskTLSHandshakeTimeout
	}
	headerTimeout := opts.ResponseHeaderTimeout
	if headerTimeout <= 0 {
		headerTimeout = DefaultIPRiskResponseHeaderTimeout
	}
	idleTimeout := opts.IdleConnTimeout
	if idleTimeout <= 0 {
		idleTimeout = DefaultIPRiskIdleConnTimeout
	}
	maxBytes := opts.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = DefaultIPRiskMaxResponseBytes
	}

	effectiveOpts := IPRiskChannelOptions{
		Timeout:               timeout,
		TLSHandshakeTimeout:   tlsTimeout,
		ResponseHeaderTimeout: headerTimeout,
		IdleConnTimeout:       idleTimeout,
		MaxResponseBytes:      maxBytes,
		ForceAttemptHTTP2:     opts.ForceAttemptHTTP2,
	}

	transport := &http.Transport{
		Proxy:                 nil, // Never consult host HTTP_PROXY / HTTPS_PROXY / ALL_PROXY
		DialContext:           rt.DialContext,
		ForceAttemptHTTP2:     opts.ForceAttemptHTTP2,
		MaxIdleConns:          10,
		IdleConnTimeout:       idleTimeout,
		TLSHandshakeTimeout:   tlsTimeout,
		ResponseHeaderTimeout: headerTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return &IPRiskChannel{
		runtime:   rt,
		options:   effectiveOpts,
		client:    client,
		transport: transport,
	}
}

// Do executes an HTTP request strictly through the runtime's outbound with timeout isolation and memory bounds.
func (c *IPRiskChannel) Do(ctx context.Context, req *http.Request) (*IPRiskProbeResponse, error) {
	if req == nil {
		return nil, ErrIPRiskNilRequest
	}
	if c.runtime == nil || c.runtime.IsClosed() {
		return nil, ErrRuntimeClosed
	}

	reqCtx := ctx
	if reqCtx == nil {
		reqCtx = context.Background()
	}

	// Apply channel timeout isolation if ctx doesn't already have a tighter deadline
	var cancel context.CancelFunc
	if c.options.Timeout > 0 {
		reqCtx, cancel = context.WithTimeout(reqCtx, c.options.Timeout)
		defer cancel()
	}

	req = req.WithContext(reqCtx)

	start := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		probeResp := &IPRiskProbeResponse{
			LatencyMS: latency,
		}

		// Categorize error
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			probeResp.DeadlineExceeded = true
		} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			probeResp.DeadlineExceeded = true
		}

		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			probeResp.DNSError = true
		}

		if !probeResp.DeadlineExceeded && !probeResp.DNSError && !errors.Is(err, context.Canceled) && !errors.Is(reqCtx.Err(), context.Canceled) {
			probeResp.NetworkError = true
		}

		return probeResp, err
	}
	defer resp.Body.Close()

	// Read bounded body
	limitReader := io.LimitReader(resp.Body, c.options.MaxResponseBytes+1)
	body, readErr := io.ReadAll(limitReader)
	if readErr != nil {
		return &IPRiskProbeResponse{
			StatusCode:   resp.StatusCode,
			Headers:      resp.Header,
			LatencyMS:    latency,
			NetworkError: true,
		}, readErr
	}

	if int64(len(body)) > c.options.MaxResponseBytes {
		return &IPRiskProbeResponse{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			LatencyMS:  latency,
		}, ErrIPRiskResponseTooLarge
	}

	return &IPRiskProbeResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
		LatencyMS:  latency,
	}, nil
}

// HTTPClient returns the underlying *http.Client configured for outbound-bound IP risk queries.
func (c *IPRiskChannel) HTTPClient() *http.Client {
	return c.client
}

// Options returns a copy of the channel options.
func (c *IPRiskChannel) Options() IPRiskChannelOptions {
	return c.options
}

// LogicalID returns the logical ID of the tested node.
func (c *IPRiskChannel) LogicalID() string {
	if c.runtime == nil {
		return ""
	}
	return c.runtime.LogicalID()
}

// Close releases any idle connections associated with this channel.
func (c *IPRiskChannel) Close() error {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	return nil
}

// IPRiskChannel returns an isolated IP risk probe channel on this runtime.
func (r *Runtime) IPRiskChannel(options IPRiskChannelOptions) *IPRiskChannel {
	return newIPRiskChannel(r, options)
}

// NewIPRiskChannel creates and starts an isolated in-memory sing-box runtime and returns an IPRiskChannel.
func NewIPRiskChannel(ctx context.Context, config NodeConfig, options IPRiskChannelOptions) (*IPRiskChannel, func() error, error) {
	rt, err := New(ctx, config)
	if err != nil {
		return nil, nil, err
	}
	channel := rt.IPRiskChannel(options)
	cleanup := func() error {
		_ = channel.Close()
		return rt.Close()
	}
	return channel, cleanup, nil
}
