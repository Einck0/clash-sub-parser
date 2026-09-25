// Package fetch provides secure HTTP fetching for subscription sources with strict SSRF defense.
package fetch

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// Default resource and timeout bounds.
const (
	DefaultTimeoutSeconds   = 30
	DefaultMaxResponseBytes = 10 * 1024 * 1024 // 10 MB
	DefaultMaxRedirects     = 5
	DefaultUserAgent        = "clash-sub-parser/1.0"
)

// Options holds parameters for a secure fetch operation.
type Options struct {
	URL                  string
	UserAgent            string
	FetchProxyURL        string
	Timeout              time.Duration
	MaxResponseBytes     int64
	MaxDecompressedBytes int64
	MaxRedirects         int
	Policy               *Policy
}

// Response contains the result of a successful fetch operation.
type Response struct {
	StatusCode    int
	Body          []byte
	ContentDigest string
	ContentType   string
	ETag          string
	LastModified  string
}

// Fetcher defines the contract for fetching subscription content safely.
type Fetcher interface {
	Fetch(ctx context.Context, opts Options) (*Response, error)
}

// Client implements the Fetcher interface with SSRF mitigation and resource controls.
type Client struct {
	policy *Policy
}

// NewClient returns a Client with the standard production SSRF policy.
func NewClient() *Client {
	return &Client{
		policy: DefaultPolicy(),
	}
}

// NewClientWithPolicy returns a Client configured with a specific SSRF policy.
func NewClientWithPolicy(policy *Policy) *Client {
	if policy == nil {
		policy = DefaultPolicy()
	}
	return &Client{
		policy: policy,
	}
}

// OptionsFromPolicy translates domain.RefreshPolicy into fetch Options.
func OptionsFromPolicy(rawURL string, refPolicy domain.RefreshPolicy, fetchProxyURL string) Options {
	timeout := time.Duration(refPolicy.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds * time.Second
	}
	maxBytes := refPolicy.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBytes
	}

	return Options{
		URL:                  rawURL,
		UserAgent:            refPolicy.UserAgentPolicy,
		FetchProxyURL:        fetchProxyURL,
		Timeout:              timeout,
		MaxResponseBytes:     maxBytes,
		MaxDecompressedBytes: maxBytes,
		MaxRedirects:         DefaultMaxRedirects,
	}
}

// RedactError extracts a redacted, safe error message without sensitive credentials or tokens.
func RedactError(err error) string {
	if err == nil {
		return ""
	}
	return domain.RedactSensitiveInfo(err.Error())
}

// Fetch executes a safe HTTP GET request adhering to SSRF policies, timeouts, and body size limits.
func (c *Client) Fetch(ctx context.Context, opts Options) (*Response, error) {
	policy := opts.Policy
	if policy == nil {
		policy = c.policy
	}
	if policy == nil {
		policy = DefaultPolicy()
	}

	// 1. Validate Target URL
	targetURL, err := url.Parse(opts.URL)
	if err != nil {
		return nil, domain.NewValidationError("invalid_url", fmt.Sprintf("malformed URL: %v", err))
	}

	if err := policy.ValidateTarget(ctx, targetURL); err != nil {
		return nil, err
	}

	// 2. Validate Explicit Proxy if specified
	var proxyFunc func(*http.Request) (*url.URL, error)
	if opts.FetchProxyURL != "" {
		proxyURL, err := url.Parse(opts.FetchProxyURL)
		if err != nil {
			return nil, domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("malformed proxy URL: %v", err))
		}

		proxyScheme := strings.ToLower(proxyURL.Scheme)
		if proxyScheme != "http" && proxyScheme != "https" && proxyScheme != "socks5" {
			return nil, domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("unsupported proxy scheme %q", proxyURL.Scheme))
		}

		if err := policy.ValidateTarget(ctx, proxyURL); err != nil {
			return nil, domain.NewSecurityError("proxy_ssrf_blocked", fmt.Sprintf("proxy host %s blocked by SSRF policy: %v", proxyURL.Hostname(), err))
		}

		proxyFunc = http.ProxyURL(proxyURL)
	}

	// 3. Configure Timeouts and Deadlines
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 4. Configure Dialer with DNS Hop Validation and Address Pinning
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	dialContext := func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, domain.NewValidationError("invalid_address", fmt.Sprintf("invalid address %s: %v", addr, err))
		}

		if policy.IsHostAllowed(host) || policy.IsHostAllowed(addr) {
			return dialer.DialContext(dialCtx, network, addr)
		}

		if ip := net.ParseIP(host); ip != nil {
			if err := policy.ValidateIP(ip); err != nil {
				return nil, err
			}
			return dialer.DialContext(dialCtx, network, addr)
		}

		resolver := policy.Resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}

		ips, err := resolver.LookupIPAddr(dialCtx, host)
		if err != nil {
			return nil, domain.NewSecurityError("dns_resolution_failed", fmt.Sprintf("failed to resolve %s: %v", host, err))
		}
		if len(ips) == 0 {
			return nil, domain.NewSecurityError("dns_resolution_failed", fmt.Sprintf("no IP addresses resolved for %s", host))
		}

		for _, ipAddr := range ips {
			if err := policy.ValidateIP(ipAddr.IP); err != nil {
				return nil, err
			}
		}

		// Connect to verified IPs directly to eliminate DNS rebinding
		var lastDialErr error
		for _, ipAddr := range ips {
			pinnedAddr := net.JoinHostPort(ipAddr.IP.String(), port)
			conn, err := dialer.DialContext(dialCtx, network, pinnedAddr)
			if err == nil {
				return conn, nil
			}
			lastDialErr = err
		}
		return nil, domain.NewDomainError("connection_failed", fmt.Sprintf("failed to connect to %s: %v", host, lastDialErr), domain.CategoryInternal)
	}

	// 5. Configure Transport (NEVER inherit environment proxy)
	transport := &http.Transport{
		Proxy:               proxyFunc,
		DialContext:         dialContext,
		DisableCompression:  true, // Manual decompression to control memory limits against decompression bombs
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	maxRedirects := opts.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = policy.MaxRedirects
	}
	if maxRedirects <= 0 {
		maxRedirects = DefaultMaxRedirects
	}

	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return domain.NewSecurityError("too_many_redirects", fmt.Sprintf("exceeded maximum redirect limit of %d", maxRedirects))
			}
			if err := policy.ValidateTarget(req.Context(), req.URL); err != nil {
				return err
			}
			return nil
		},
	}

	// 6. Build HTTP Request
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return nil, domain.NewValidationError("invalid_request", fmt.Sprintf("failed to create request: %v", err))
	}

	ua := opts.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	httpReq.Header.Set("User-Agent", ua)
	httpReq.Header.Set("Accept-Encoding", "gzip, deflate")

	// 7. Execute Request
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return nil, domain.NewDomainError("fetch_timeout", fmt.Sprintf("fetch timed out after %v", timeout), domain.CategoryInternal)
		}
		if de, ok := domain.AsDomainError(err); ok {
			return nil, de
		}
		return nil, domain.NewDomainError("fetch_failed", err.Error(), domain.CategoryInternal)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, domain.NewDomainError(
			"http_error",
			fmt.Sprintf("request failed with status %d: %s", httpResp.StatusCode, http.StatusText(httpResp.StatusCode)),
			domain.CategoryInternal,
		)
	}

	// 8. Enforce Response Wire Size Limit
	maxResponseBytes := opts.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = DefaultMaxResponseBytes
	}

	if httpResp.ContentLength > maxResponseBytes {
		return nil, domain.NewSecurityError(
			"response_too_large",
			fmt.Sprintf("content length %d exceeds limit of %d bytes", httpResp.ContentLength, maxResponseBytes),
		)
	}

	limitedReader := io.LimitReader(httpResp.Body, maxResponseBytes+1)
	rawBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, domain.NewDomainError("read_error", fmt.Sprintf("failed to read response body: %v", err), domain.CategoryInternal)
	}

	if int64(len(rawBytes)) > maxResponseBytes {
		return nil, domain.NewSecurityError(
			"response_too_large",
			fmt.Sprintf("response body size exceeded limit of %d bytes", maxResponseBytes),
		)
	}

	// 9. Decompression Handling with Independent Expansion Boundary
	maxDecompressedBytes := opts.MaxDecompressedBytes
	if maxDecompressedBytes <= 0 {
		maxDecompressedBytes = maxResponseBytes
	}

	contentEncoding := strings.ToLower(strings.TrimSpace(httpResp.Header.Get("Content-Encoding")))
	isGzip := contentEncoding == "gzip" || (len(rawBytes) >= 2 && rawBytes[0] == 0x1f && rawBytes[1] == 0x8b)
	isDeflate := contentEncoding == "deflate"

	var finalBytes []byte
	if isGzip {
		gzReader, err := gzip.NewReader(bytes.NewReader(rawBytes))
		if err != nil {
			return nil, domain.NewValidationError("invalid_gzip", fmt.Sprintf("failed to parse gzip stream: %v", err))
		}
		defer gzReader.Close()

		limitedDecomp := io.LimitReader(gzReader, maxDecompressedBytes+1)
		decompBytes, err := io.ReadAll(limitedDecomp)
		if err != nil {
			return nil, domain.NewDomainError("decompression_failed", fmt.Sprintf("failed to decompress gzip content: %v", err), domain.CategoryInternal)
		}
		if int64(len(decompBytes)) > maxDecompressedBytes {
			return nil, domain.NewSecurityError(
				"decompression_limit_exceeded",
				fmt.Sprintf("decompressed body exceeded limit of %d bytes", maxDecompressedBytes),
			)
		}
		finalBytes = decompBytes
	} else if isDeflate {
		var r io.Reader
		zlibReader, err := zlib.NewReader(bytes.NewReader(rawBytes))
		if err == nil {
			defer zlibReader.Close()
			r = zlibReader
		} else {
			flateReader := flate.NewReader(bytes.NewReader(rawBytes))
			defer flateReader.Close()
			r = flateReader
		}

		limitedDecomp := io.LimitReader(r, maxDecompressedBytes+1)
		decompBytes, err := io.ReadAll(limitedDecomp)
		if err != nil {
			return nil, domain.NewDomainError("decompression_failed", fmt.Sprintf("failed to decompress deflate content: %v", err), domain.CategoryInternal)
		}
		if int64(len(decompBytes)) > maxDecompressedBytes {
			return nil, domain.NewSecurityError(
				"decompression_limit_exceeded",
				fmt.Sprintf("decompressed body exceeded limit of %d bytes", maxDecompressedBytes),
			)
		}
		finalBytes = decompBytes
	} else {
		finalBytes = rawBytes
	}

	// 10. Calculate Content Digest (SHA-256)
	digest := sha256.Sum256(finalBytes)
	contentDigest := hex.EncodeToString(digest[:])

	return &Response{
		StatusCode:    httpResp.StatusCode,
		Body:          finalBytes,
		ContentDigest: contentDigest,
		ContentType:   httpResp.Header.Get("Content-Type"),
		ETag:          httpResp.Header.Get("ETag"),
		LastModified:  httpResp.Header.Get("Last-Modified"),
	}, nil
}

// Fetch executes a fetch using the package default Client.
func Fetch(ctx context.Context, opts Options) (*Response, error) {
	return NewClient().Fetch(ctx, opts)
}
