// Package fetch provides secure HTTP fetching for subscription sources with strict SSRF defense.
package fetch

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
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
	DefaultUserAgent        = "clash-verge/v2.5.6"
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
	RootCAs              *x509.CertPool
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

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var activeProxyURL *url.URL
	if opts.FetchProxyURL != "" {
		proxyURL, err := url.Parse(opts.FetchProxyURL)
		if err != nil {
			return nil, domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("malformed proxy URL: %v", err))
		}

		if err := policy.ValidateProxy(reqCtx, proxyURL); err != nil {
			return nil, err
		}

		activeProxyURL = proxyURL
	}

	ua := opts.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}

	maxRedirects := opts.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = policy.MaxRedirects
	}
	if maxRedirects <= 0 {
		maxRedirects = DefaultMaxRedirects
	}

	currentURLStr := opts.URL
	hop := 0
	var httpResp *http.Response

	for {
		if hop > maxRedirects {
			return nil, domain.NewSecurityError("too_many_redirects", fmt.Sprintf("exceeded maximum redirect limit of %d", maxRedirects))
		}

		currURL, err := url.Parse(currentURLStr)
		if err != nil {
			return nil, domain.NewValidationError("invalid_url", fmt.Sprintf("malformed URL: %v", err))
		}

		scheme := strings.ToLower(currURL.Scheme)
		if scheme != "http" && scheme != "https" {
			return nil, domain.NewSecurityError("unsupported_scheme", fmt.Sprintf("unsupported scheme %q, only http and https are allowed", currURL.Scheme))
		}

		targetHost := strings.TrimSpace(currURL.Hostname())
		if targetHost == "" {
			return nil, domain.NewValidationError("invalid_url", "URL is missing a host")
		}

		if strings.HasSuffix(targetHost, ".") {
			return nil, domain.NewValidationError("invalid_url", fmt.Sprintf("target host %q has a trailing dot", targetHost))
		}

		if isObfuscatedIP(targetHost) {
			return nil, domain.NewSecurityError("ssrf_blocked", fmt.Sprintf("target host %s is an obfuscated IP format", targetHost))
		}

		if strings.Contains(strings.ToLower(targetHost), "::ffff:") || strings.Contains(strings.ToLower(targetHost), ":ffff:") {
			return nil, domain.NewSecurityError("ssrf_blocked", fmt.Sprintf("target host %s is an IPv4-mapped IPv6 address", targetHost))
		}

		targetPort := currURL.Port()
		if targetPort == "" {
			if scheme == "https" {
				targetPort = "443"
			} else {
				targetPort = "80"
			}
		}

		isExempted := policy.AllowPrivate || policy.IsHostAllowed(targetHost) || policy.IsHostAllowed(currURL.Host)

		var pinnedIP net.IP
		if isExempted {
			if ip := net.ParseIP(targetHost); ip != nil {
				pinnedIP = ip
			} else {
				resolver := policy.Resolver
				if resolver == nil {
					resolver = net.DefaultResolver
				}
				ips, err := resolver.LookupIPAddr(reqCtx, targetHost)
				if err != nil {
					return nil, domain.NewSecurityError("dns_resolution_failed", fmt.Sprintf("failed to resolve %s: %v", targetHost, err))
				}
				if len(ips) == 0 {
					return nil, domain.NewSecurityError("dns_resolution_failed", fmt.Sprintf("no IP addresses resolved for %s", targetHost))
				}
				pinnedIP = ips[0].IP
			}
		} else {
			if ip := net.ParseIP(targetHost); ip != nil {
				if ip.To4() != nil && strings.Contains(targetHost, ":") {
					return nil, domain.NewSecurityError("ssrf_blocked", fmt.Sprintf("target IP %s is blocked by SSRF policy", targetHost))
				}
				if err := policy.ValidateIP(ip); err != nil {
					return nil, err
				}
				pinnedIP = ip
			} else {
				resolver := policy.Resolver
				if resolver == nil {
					resolver = net.DefaultResolver
				}
				ips, err := resolver.LookupIPAddr(reqCtx, targetHost)
				if err != nil {
					return nil, domain.NewSecurityError("dns_resolution_failed", fmt.Sprintf("failed to resolve %s: %v", targetHost, err))
				}
				if len(ips) == 0 {
					return nil, domain.NewSecurityError("dns_resolution_failed", fmt.Sprintf("no IP addresses resolved for %s", targetHost))
				}
				for _, ipAddr := range ips {
					if err := policy.ValidateIP(ipAddr.IP); err != nil {
						return nil, err
					}
				}
				pinnedIP = ips[0].IP
				for _, ipAddr := range ips {
					if ipAddr.IP.To4() != nil {
						pinnedIP = ipAddr.IP
						break
					}
				}
			}
		}

		pinnedAddr := net.JoinHostPort(pinnedIP.String(), targetPort)
		origHostHeader := currURL.Host

		dialer := &net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}

		var transport *http.Transport
		var hopReq *http.Request

		if activeProxyURL != nil {
			proxyDialContext := func(dCtx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(dCtx, network, addr)
			}

			if scheme == "http" {
				hopReq, err = http.NewRequestWithContext(reqCtx, http.MethodGet, currURL.String(), nil)
				if err != nil {
					return nil, domain.NewValidationError("invalid_request", fmt.Sprintf("failed to create request: %v", err))
				}
				hopReq.URL.Host = pinnedAddr
				escapedPath := currURL.EscapedPath()
				if escapedPath == "" {
					escapedPath = "/"
				}
				hopReq.URL.Opaque = "//" + pinnedAddr + escapedPath
				hopReq.Host = origHostHeader

				transport = &http.Transport{
					Proxy:                 http.ProxyURL(activeProxyURL),
					DialContext:           proxyDialContext,
					DisableCompression:    true,
					MaxIdleConns:          10,
					IdleConnTimeout:       30 * time.Second,
					ResponseHeaderTimeout: timeout,
				}
			} else {
				hopReq, err = http.NewRequestWithContext(reqCtx, http.MethodGet, currURL.String(), nil)
				if err != nil {
					return nil, domain.NewValidationError("invalid_request", fmt.Sprintf("failed to create request: %v", err))
				}
				hopReq.URL.Host = pinnedAddr
				hopReq.URL.Opaque = ""
				hopReq.Host = origHostHeader

				transport = &http.Transport{
					Proxy:       http.ProxyURL(activeProxyURL),
					DialContext: proxyDialContext,
					TLSClientConfig: &tls.Config{
						ServerName:         targetHost,
						RootCAs:            opts.RootCAs,
						MinVersion:         tls.VersionTLS12,
						InsecureSkipVerify: false,
					},
					DisableCompression:    true,
					MaxIdleConns:          10,
					IdleConnTimeout:       30 * time.Second,
					TLSHandshakeTimeout:   10 * time.Second,
					ResponseHeaderTimeout: timeout,
				}
			}
		} else {
			directDialContext := func(dCtx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(dCtx, network, pinnedAddr)
			}

			hopReq, err = http.NewRequestWithContext(reqCtx, http.MethodGet, currURL.String(), nil)
			if err != nil {
				return nil, domain.NewValidationError("invalid_request", fmt.Sprintf("failed to create request: %v", err))
			}
			hopReq.URL.Host = pinnedAddr
			hopReq.URL.Opaque = ""
			hopReq.Host = origHostHeader

			transport = &http.Transport{
				DialContext: directDialContext,
				TLSClientConfig: &tls.Config{
					ServerName:         targetHost,
					RootCAs:            opts.RootCAs,
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: false,
				},
				DisableCompression:    true,
				MaxIdleConns:          10,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: timeout,
			}
		}

		defer transport.CloseIdleConnections()

		hopReq.Header.Set("User-Agent", ua)
		hopReq.Header.Set("Accept", "*/*")
		hopReq.Header.Set("Accept-Encoding", "gzip, deflate")

		hopClient := &http.Client{
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := hopClient.Do(hopReq)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
				return nil, domain.NewDomainError("fetch_timeout", fmt.Sprintf("fetch timed out after %v", timeout), domain.CategoryInternal)
			}
			if de, ok := domain.AsDomainError(err); ok {
				return nil, de
			}
			return nil, domain.NewDomainError("fetch_failed", RedactError(err), domain.CategoryInternal)
		}

		if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusTemporaryRedirect ||
			resp.StatusCode == http.StatusPermanentRedirect {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if loc == "" {
				return nil, domain.NewDomainError("http_error", fmt.Sprintf("redirect %d missing Location header", resp.StatusCode), domain.CategoryInternal)
			}
			nextURL, err := currURL.Parse(loc)
			if err != nil {
				return nil, domain.NewValidationError("invalid_redirect", fmt.Sprintf("malformed redirect Location %q: %v", loc, err))
			}
			currentURLStr = nextURL.String()
			hop++
			continue
		}

		httpResp = resp
		break
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
