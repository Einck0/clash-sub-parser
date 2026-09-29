package fetch_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

func TestSSRFBlocked_Loopback(t *testing.T) {
	ctx := context.Background()
	client := fetch.NewClient()

	loopbackTargets := []string{
		"http://127.0.0.1:8080/sub",
		"http://127.0.0.2:8080/sub",
		"http://127.255.255.254:8080/sub",
		"http://localhost:8080/sub",
		"http://[::1]:8080/sub",
		"http://0.0.0.0:8080/sub",
	}

	for _, target := range loopbackTargets {
		t.Run(target, func(t *testing.T) {
			_, err := client.Fetch(ctx, fetch.Options{URL: target})
			if err == nil {
				t.Fatalf("expected SSRF error for loopback target %s, got nil", target)
			}
			de, ok := domain.AsDomainError(err)
			if !ok {
				t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
			}
			if de.Category != domain.CategorySecurity {
				t.Errorf("expected security category, got %s", de.Category)
			}
			if de.Code != "ssrf_blocked" {
				t.Errorf("expected code ssrf_blocked, got %s", de.Code)
			}
		})
	}
}

func TestSSRFBlocked_RFC1918Private(t *testing.T) {
	ctx := context.Background()
	client := fetch.NewClient()

	privateTargets := []string{
		"http://10.0.0.1:8080/sub",
		"http://10.255.255.254:8080/sub",
		"http://172.16.0.1:8080/sub",
		"http://172.31.255.254:8080/sub",
		"http://192.168.0.1:8080/sub",
		"http://192.168.1.100:8080/sub",
	}

	for _, target := range privateTargets {
		t.Run(target, func(t *testing.T) {
			_, err := client.Fetch(ctx, fetch.Options{URL: target})
			if err == nil {
				t.Fatalf("expected SSRF error for private target %s, got nil", target)
			}
			de, ok := domain.AsDomainError(err)
			if !ok {
				t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
			}
			if de.Category != domain.CategorySecurity || de.Code != "ssrf_blocked" {
				t.Errorf("expected security/ssrf_blocked, got %s/%s", de.Category, de.Code)
			}
		})
	}
}

func TestSSRFBlocked_LinkLocalAndReserved(t *testing.T) {
	ctx := context.Background()
	client := fetch.NewClient()

	targets := []string{
		"http://169.254.169.254/latest/meta-data", // Cloud metadata
		"http://169.254.1.1/sub",                  // Link-local IPv4
		"http://[fe80::1]:8080/sub",               // Link-local IPv6
		"http://[fc00::1]:8080/sub",               // ULA IPv6
		"http://224.0.0.1:8080/sub",               // Multicast
		"http://240.0.0.1:8080/sub",               // Reserved
		"http://255.255.255.255:8080/sub",         // Broadcast
		"http://100.64.0.1:8080/sub",              // CGNAT
		"http://[::ffff:127.0.0.1]:8080/sub",      // IPv4-mapped loopback
		"http://[::ffff:10.0.0.1]:8080/sub",       // IPv4-mapped private
		"http://[::ffff:192.168.1.1]:8080/sub",    // IPv4-mapped private
	}

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			_, err := client.Fetch(ctx, fetch.Options{URL: target})
			if err == nil {
				t.Fatalf("expected SSRF error for reserved target %s, got nil", target)
			}
			de, ok := domain.AsDomainError(err)
			if !ok {
				t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
			}
			if de.Category != domain.CategorySecurity || de.Code != "ssrf_blocked" {
				t.Errorf("expected security/ssrf_blocked, got %s/%s", de.Category, de.Code)
			}
		})
	}
}

func TestUnsupportedSchemes(t *testing.T) {
	ctx := context.Background()
	client := fetch.NewClient()

	schemes := []string{
		"file:///etc/passwd",
		"ftp://example.com/sub",
		"gopher://example.com:70",
		"data:text/plain;base64,SGVsbG8=",
		"javascript:alert(1)",
		"ldap://example.com/dc=example",
	}

	for _, target := range schemes {
		t.Run(target, func(t *testing.T) {
			_, err := client.Fetch(ctx, fetch.Options{URL: target})
			if err == nil {
				t.Fatalf("expected scheme error for target %s, got nil", target)
			}
			de, ok := domain.AsDomainError(err)
			if !ok {
				t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
			}
			if de.Code != "unsupported_scheme" {
				t.Errorf("expected code unsupported_scheme, got %s", de.Code)
			}
		})
	}
}

func TestRedirectToPrivateBlocked(t *testing.T) {
	ctx := context.Background()

	// Redirect server simulates an external URL redirecting to private / internal address
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dest := r.URL.Query().Get("dest")
		if dest == "" {
			dest = "http://127.0.0.1:8080/forbidden"
		}
		http.Redirect(w, r, dest, http.StatusFound)
	}))
	defer redirectServer.Close()

	u, err := url.Parse(redirectServer.URL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	// Create policy allowing the redirect server host, but keeping default private/loopback blocked
	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}

	client := fetch.NewClientWithPolicy(policy)

	testCases := []struct {
		name        string
		redirectURL string
		expectCode  string
	}{
		{"Redirect to 127.0.0.1", "http://127.0.0.1:9999/secret", "ssrf_blocked"},
		{"Redirect to 10.0.0.1", "http://10.0.0.1/admin", "ssrf_blocked"},
		{"Redirect to 192.168.1.1", "http://192.168.1.1/router", "ssrf_blocked"},
		{"Redirect to file scheme", "file:///etc/hosts", "unsupported_scheme"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := fmt.Sprintf("%s/redirect?dest=%s", redirectServer.URL, url.QueryEscape(tc.redirectURL))
			_, err := client.Fetch(ctx, fetch.Options{
				URL:    target,
				Policy: policy,
			})
			if err == nil {
				t.Fatalf("expected redirect to be blocked for %s, got nil", tc.redirectURL)
			}
			de, ok := domain.AsDomainError(err)
			if !ok {
				t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
			}
			if de.Code != tc.expectCode {
				t.Errorf("expected code %s, got %s (err: %v)", tc.expectCode, de.Code, de)
			}
		})
	}
}

func TestTooManyRedirects(t *testing.T) {
	ctx := context.Background()

	var redirectServer *httptest.Server
	redirectServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectServer.URL+"/loop", http.StatusFound)
	}))
	defer redirectServer.Close()

	u, err := url.Parse(redirectServer.URL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}

	client := fetch.NewClientWithPolicy(policy)

	_, err = client.Fetch(ctx, fetch.Options{
		URL:          redirectServer.URL,
		MaxRedirects: 3,
		Policy:       policy,
	})
	if err == nil {
		t.Fatal("expected error on redirect loop, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok {
		t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
	}
	if de.Code != "too_many_redirects" {
		t.Errorf("expected code too_many_redirects, got %s", de.Code)
	}
}

func TestResponseSizeLimit(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// Send 10KB of data
		payload := bytes.Repeat([]byte("A"), 10*1024)
		w.Write(payload)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}

	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}

	client := fetch.NewClientWithPolicy(policy)

	t.Run("Exceeds max_response_bytes", func(t *testing.T) {
		_, err := client.Fetch(ctx, fetch.Options{
			URL:              server.URL,
			MaxResponseBytes: 1024, // 1KB limit vs 10KB response
			Policy:           policy,
		})
		if err == nil {
			t.Fatal("expected error for oversized response, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok {
			t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
		}
		if de.Code != "response_too_large" {
			t.Errorf("expected code response_too_large, got %s", de.Code)
		}
	})

	t.Run("Within max_response_bytes", func(t *testing.T) {
		resp, err := client.Fetch(ctx, fetch.Options{
			URL:              server.URL,
			MaxResponseBytes: 20 * 1024, // 20KB limit
			Policy:           policy,
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(resp.Body) != 10*1024 {
			t.Errorf("expected 10240 bytes, got %d", len(resp.Body))
		}
		expectedDigest := sha256.Sum256(resp.Body)
		if resp.ContentDigest != hex.EncodeToString(expectedDigest[:]) {
			t.Errorf("content digest mismatch: %s", resp.ContentDigest)
		}
	})
}

func TestDecompressionBombProtection(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")

		// Create a gzip payload that compresses highly (e.g. 50KB of zeros compresses to ~100 bytes)
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		gw.Write(bytes.Repeat([]byte{0}, 50*1024)) // 50KB decompressed
		gw.Close()

		w.Write(buf.Bytes())
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}

	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}

	client := fetch.NewClientWithPolicy(policy)

	t.Run("Decompression exceeds limit", func(t *testing.T) {
		_, err := client.Fetch(ctx, fetch.Options{
			URL:                  server.URL,
			MaxResponseBytes:     10 * 1024, // 10KB wire is enough for ~100 bytes
			MaxDecompressedBytes: 5 * 1024,  // 5KB decompressed limit vs 50KB actual
			Policy:               policy,
		})
		if err == nil {
			t.Fatal("expected decompression bomb error, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok {
			t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
		}
		if de.Code != "decompression_limit_exceeded" {
			t.Errorf("expected code decompression_limit_exceeded, got %s", de.Code)
		}
	})

	t.Run("Decompression within limit", func(t *testing.T) {
		resp, err := client.Fetch(ctx, fetch.Options{
			URL:                  server.URL,
			MaxResponseBytes:     10 * 1024,
			MaxDecompressedBytes: 100 * 1024, // 100KB decompressed limit
			Policy:               policy,
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(resp.Body) != 50*1024 {
			t.Errorf("expected 51200 decompressed bytes, got %d", len(resp.Body))
		}
	})
}

func TestProxyIsolationAndSSRF(t *testing.T) {
	ctx := context.Background()

	// 1. Host environment proxy MUST NOT be inherited
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9999")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9999")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:9999")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("proxyless direct connection"))
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}

	client := fetch.NewClientWithPolicy(policy)

	// Fetch should succeed directly without attempting to connect to 127.0.0.1:9999
	resp, err := client.Fetch(ctx, fetch.Options{
		URL:    server.URL,
		Policy: policy,
	})
	if err != nil {
		t.Fatalf("fetch should ignore env proxy and succeed, got error: %v", err)
	}
	if string(resp.Body) != "proxyless direct connection" {
		t.Errorf("unexpected body: %s", string(resp.Body))
	}

	// 2. Explicit fetch_proxy MUST undergo SSRF validation
	t.Run("Proxy SSRF to private IP", func(t *testing.T) {
		_, err := client.Fetch(ctx, fetch.Options{
			URL:           server.URL,
			FetchProxyURL: "http://10.0.0.1:8080",
			Policy:        policy,
		})
		if err == nil {
			t.Fatal("expected error on private proxy URL, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok {
			t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
		}
		if de.Code != "proxy_ssrf_blocked" {
			t.Errorf("expected code proxy_ssrf_blocked, got %s", de.Code)
		}
	})

	t.Run("Proxy SSRF to loopback", func(t *testing.T) {
		_, err := client.Fetch(ctx, fetch.Options{
			URL:           server.URL,
			FetchProxyURL: "http://127.0.0.1:8080",
			Policy:        policy,
		})
		if err == nil {
			t.Fatal("expected error on loopback proxy URL, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok {
			t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
		}
		if de.Code != "proxy_ssrf_blocked" {
			t.Errorf("expected code proxy_ssrf_blocked, got %s", de.Code)
		}
	})
}

func TestTimeoutHandling(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}

	client := fetch.NewClientWithPolicy(policy)

	_, err := client.Fetch(ctx, fetch.Options{
		URL:     server.URL,
		Timeout: 50 * time.Millisecond,
		Policy:  policy,
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok {
		t.Fatalf("expected domain.DomainError, got %T: %v", err, err)
	}
	if de.Code != "fetch_timeout" {
		t.Errorf("expected code fetch_timeout, got %s", de.Code)
	}
}

func TestSecretRedactionInErrors(t *testing.T) {
	ctx := context.Background()
	client := fetch.NewClient()

	sensitiveURL := "http://user:secretpassword999@127.0.0.1:8080/sub?token=secrettoken123"
	_, err := client.Fetch(ctx, fetch.Options{URL: sensitiveURL})
	if err == nil {
		t.Fatal("expected error on loopback target, got nil")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "secretpassword999") {
		t.Errorf("error leaked password: %s", errMsg)
	}
	if strings.Contains(errMsg, "secrettoken123") {
		t.Errorf("error leaked query token: %s", errMsg)
	}

	redacted := fetch.RedactError(err)
	if strings.Contains(redacted, "secretpassword999") || strings.Contains(redacted, "secrettoken123") {
		t.Errorf("RedactError leaked sensitive data: %s", redacted)
	}
}

func TestOptionsFromPolicy(t *testing.T) {
	refPolicy := domain.RefreshPolicy{
		IntervalSeconds:  3600,
		UserAgentPolicy:  "CustomClashUA/2.0",
		FetchProxyRef:    "proxy_ref_123",
		TimeoutSeconds:   15,
		MaxResponseBytes: 5 * 1024 * 1024,
	}

	opts := fetch.OptionsFromPolicy("https://example.com/feed", refPolicy, "http://proxy.example.com:8080")
	if opts.URL != "https://example.com/feed" {
		t.Errorf("unexpected URL: %s", opts.URL)
	}
	if opts.UserAgent != "CustomClashUA/2.0" {
		t.Errorf("unexpected UserAgent: %s", opts.UserAgent)
	}
	if opts.FetchProxyURL != "http://proxy.example.com:8080" {
		t.Errorf("unexpected FetchProxyURL: %s", opts.FetchProxyURL)
	}
	if opts.Timeout != 15*time.Second {
		t.Errorf("unexpected Timeout: %v", opts.Timeout)
	}
	if opts.MaxResponseBytes != 5*1024*1024 {
		t.Errorf("unexpected MaxResponseBytes: %d", opts.MaxResponseBytes)
	}
	if opts.MaxDecompressedBytes != 5*1024*1024 {
		t.Errorf("unexpected MaxDecompressedBytes: %d", opts.MaxDecompressedBytes)
	}
	if opts.MaxRedirects != 5 {
		t.Errorf("unexpected MaxRedirects: %d", opts.MaxRedirects)
	}
}

func TestConcurrentFetches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("concurrent payload"))
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}

	client := fetch.NewClientWithPolicy(policy)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Fetch(context.Background(), fetch.Options{
				URL:    server.URL,
				Policy: policy,
			})
			if err != nil {
				t.Errorf("concurrent fetch error: %v", err)
				return
			}
			if string(resp.Body) != "concurrent payload" {
				t.Errorf("unexpected body: %s", string(resp.Body))
			}
		}()
	}
	wg.Wait()
}

type mockResolver struct {
	ips []net.IPAddr
	err error
}

func (m *mockResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ips, nil
}

func TestSSRFBlocked_DNSResolutionToPrivateIP(t *testing.T) {
	ctx := context.Background()
	policy := fetch.DefaultPolicy()
	policy.Resolver = &mockResolver{
		ips: []net.IPAddr{
			{IP: net.ParseIP("10.0.0.5")},
		},
	}
	client := fetch.NewClientWithPolicy(policy)

	_, err := client.Fetch(ctx, fetch.Options{
		URL:    "http://benign-looking-domain.com/feed",
		Policy: policy,
	})
	if err == nil {
		t.Fatal("expected SSRF error when DNS resolves to private IP, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Code != "ssrf_blocked" {
		t.Fatalf("expected ssrf_blocked, got %v", err)
	}
}

func TestSSRFBlocked_DualStackWithOnePrivateIP(t *testing.T) {
	ctx := context.Background()
	policy := fetch.DefaultPolicy()
	policy.Resolver = &mockResolver{
		ips: []net.IPAddr{
			{IP: net.ParseIP("93.184.216.34")},
			{IP: net.ParseIP("127.0.0.1")},
		},
	}
	client := fetch.NewClientWithPolicy(policy)

	_, err := client.Fetch(ctx, fetch.Options{
		URL:    "http://dualstack-with-poisoned-record.com/feed",
		Policy: policy,
	})
	if err == nil {
		t.Fatal("expected SSRF error when one dual-stack IP is loopback, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Code != "ssrf_blocked" {
		t.Fatalf("expected ssrf_blocked, got %v", err)
	}
}

func TestFetch_PureNoSideEffects(t *testing.T) {
	ctx := context.Background()
	client := fetch.NewClient()

	// 1. SSRF target
	_, err := client.Fetch(ctx, fetch.Options{URL: "http://127.0.0.1:9999/sub"})
	if err == nil {
		t.Fatal("expected SSRF error")
	}
	if !domain.IsDomainError(err) {
		t.Errorf("expected domain error, got %v", err)
	}

	// 2. Unsupported scheme
	_, err = client.Fetch(ctx, fetch.Options{URL: "file:///etc/shadow"})
	if err == nil {
		t.Fatal("expected unsupported scheme error")
	}

	// 3. Verify repeated attempts do not pollute client or leak state
	resp1, err1 := client.Fetch(ctx, fetch.Options{URL: "http://127.0.0.1:1/a"})
	resp2, err2 := client.Fetch(ctx, fetch.Options{URL: "http://127.0.0.1:1/b"})
	if resp1 != nil || resp2 != nil {
		t.Errorf("expected nil responses on error")
	}
	if err1 == nil || err2 == nil {
		t.Errorf("expected errors on loopback requests")
	}
}

func TestVergeHeadersDefaultAndOverride(t *testing.T) {
	ctx := context.Background()

	var receivedUA, receivedAccept, receivedEncoding string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		receivedAccept = r.Header.Get("Accept")
		receivedEncoding = r.Header.Get("Accept-Encoding")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok payload"))
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}
	client := fetch.NewClientWithPolicy(policy)

	// Case 1: Default Verge UA and standard headers
	resp, err := client.Fetch(ctx, fetch.Options{
		URL:    server.URL,
		Policy: policy,
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if string(resp.Body) != "ok payload" {
		t.Errorf("unexpected body: %s", string(resp.Body))
	}
	if receivedUA != "clash-verge/v2.5.6" {
		t.Errorf("expected default UA clash-verge/v2.5.6, got: %s", receivedUA)
	}
	if receivedAccept != "*/*" {
		t.Errorf("expected Accept */*, got: %s", receivedAccept)
	}
	if !strings.Contains(receivedEncoding, "gzip") {
		t.Errorf("expected Accept-Encoding to contain gzip, got: %s", receivedEncoding)
	}

	// Case 2: Custom UA overrides default
	resp, err = client.Fetch(ctx, fetch.Options{
		URL:       server.URL,
		UserAgent: "custom-clash-meta/v1.0",
		Policy:    policy,
	})
	if err != nil {
		t.Fatalf("fetch with custom UA failed: %v", err)
	}
	if receivedUA != "custom-clash-meta/v1.0" {
		t.Errorf("expected custom UA custom-clash-meta/v1.0, got: %s", receivedUA)
	}
	if receivedAccept != "*/*" {
		t.Errorf("expected Accept */*, got: %s", receivedAccept)
	}
}

func TestHTTPProxy_MockProxyAndSSRFBoundaries(t *testing.T) {
	ctx := context.Background()

	// Target backend server
	var targetHitCount int
	var targetReceivedUA string
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHitCount++
		targetReceivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("proxied payload content"))
	}))
	defer targetServer.Close()

	targetURL, _ := url.Parse(targetServer.URL)

	// Mock forward HTTP proxy server
	var proxyHitCount int
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHitCount++
		// Forward request to target
		outReq, err := http.NewRequestWithContext(r.Context(), r.Method, r.RequestURI, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for k, v := range r.Header {
			outReq.Header[k] = v
		}
		resp, err := http.DefaultTransport.RoundTrip(outReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	defer proxyServer.Close()

	// 1. Success via AllowedProxyHosts
	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{targetURL.Host}
	_ = policy.AddAllowedProxy(proxyServer.URL)

	client := fetch.NewClientWithPolicy(policy)

	resp, err := client.Fetch(ctx, fetch.Options{
		URL:           targetServer.URL,
		FetchProxyURL: proxyServer.URL,
		Policy:        policy,
	})
	if err != nil {
		t.Fatalf("fetch via mock proxy failed: %v", err)
	}
	if string(resp.Body) != "proxied payload content" {
		t.Errorf("unexpected body: %s", string(resp.Body))
	}
	if proxyHitCount != 1 {
		t.Errorf("expected 1 hit on proxy, got %d", proxyHitCount)
	}
	if targetHitCount != 1 {
		t.Errorf("expected 1 hit on target, got %d", targetHitCount)
	}
	if targetReceivedUA != "clash-verge/v2.5.6" {
		t.Errorf("expected Verge UA on target, got: %s", targetReceivedUA)
	}

	// 2. Untrusted proxy to private/loopback/cloud metadata is rejected
	t.Run("untrusted private proxy blocked", func(t *testing.T) {
		strictPolicy := fetch.DefaultPolicy()
		strictPolicy.AllowedHosts = []string{targetURL.Host}
		// proxyURL.Host is NOT in AllowedProxyHosts
		_, err := client.Fetch(ctx, fetch.Options{
			URL:           targetServer.URL,
			FetchProxyURL: proxyServer.URL, // loopback proxy not in AllowedProxyHosts
			Policy:        strictPolicy,
		})
		if err == nil {
			t.Fatal("expected proxy_ssrf_blocked on unlisted loopback proxy, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "proxy_ssrf_blocked" {
			t.Fatalf("expected code proxy_ssrf_blocked, got %v", err)
		}
	})

	t.Run("untrusted cloud metadata proxy blocked", func(t *testing.T) {
		_, err := client.Fetch(ctx, fetch.Options{
			URL:           targetServer.URL,
			FetchProxyURL: "http://169.254.169.254:80",
			Policy:        policy,
		})
		if err == nil {
			t.Fatal("expected proxy_ssrf_blocked for cloud metadata, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "proxy_ssrf_blocked" {
			t.Fatalf("expected code proxy_ssrf_blocked, got %v", err)
		}
	})

	// 3. Target SSRF protection remains strict even when proxy is configured
	t.Run("target SSRF blocked even with proxy", func(t *testing.T) {
		_, err := client.Fetch(ctx, fetch.Options{
			URL:           "http://127.0.0.1:8080/feed",
			FetchProxyURL: proxyServer.URL,
			Policy:        policy, // policy has AllowedProxyHosts, but NOT target 127.0.0.1
		})
		if err == nil {
			t.Fatal("expected target SSRF error, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "ssrf_blocked" {
			t.Fatalf("expected ssrf_blocked, got %v", err)
		}
	})

	// 4. Redirect SSRF protection remains strict with proxy configured
	t.Run("redirect to private IP blocked with proxy", func(t *testing.T) {
		redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://10.0.0.1:8080/internal", http.StatusFound)
		}))
		defer redirectServer.Close()

		uRedir, _ := url.Parse(redirectServer.URL)
		redirPolicy := fetch.DefaultPolicy()
		redirPolicy.AllowedHosts = []string{uRedir.Host}
		_ = redirPolicy.AddAllowedProxy(proxyServer.URL)

		_, err := client.Fetch(ctx, fetch.Options{
			URL:           redirectServer.URL,
			FetchProxyURL: proxyServer.URL,
			Policy:        redirPolicy,
		})
		if err == nil {
			t.Fatal("expected redirect SSRF blocked error, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "ssrf_blocked" {
			t.Fatalf("expected ssrf_blocked on redirect, got %v", err)
		}
	})
}

func TestHTTPProxy_UnreachableFailureClassification(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{u.Host}
	// Pick an unreachable local port in AllowedProxyHosts
	unreachableProxy := "http://127.0.0.1:54321"
	_ = policy.AddAllowedProxy(unreachableProxy)

	client := fetch.NewClientWithPolicy(policy)

	_, err := client.Fetch(ctx, fetch.Options{
		URL:           server.URL,
		FetchProxyURL: unreachableProxy,
		Timeout:       2 * time.Second,
		Policy:        policy,
	})
	if err == nil {
		t.Fatal("expected fetch error with unreachable proxy, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok {
		t.Fatalf("expected domain error, got %T: %v", err, err)
	}
	if de.Code != "fetch_failed" && de.Code != "fetch_timeout" && de.Code != "connection_failed" {
		t.Errorf("unexpected error code: %s", de.Code)
	}
}

type testMockResolver struct {
	mapping map[string][]net.IPAddr
}

func (m *testMockResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ips, ok := m.mapping[host]; ok {
		return ips, nil
	}
	return nil, fmt.Errorf("mock DNS: host not found: %s", host)
}

func TestProxy_ExactEndpointAndPortEnforcement(t *testing.T) {
	ctx := context.Background()

	// Mock resolver resolving host.docker.internal to gateway private IP (172.17.0.1)
	resolver := &testMockResolver{
		mapping: map[string][]net.IPAddr{
			"host.docker.internal": {{IP: net.ParseIP("172.17.0.1")}},
			"public-target.com":    {{IP: net.ParseIP("93.184.216.34")}},
		},
	}

	policy := fetch.DefaultPolicy()
	policy.Resolver = resolver
	// Exactly whitelist host.docker.internal:7890 ONLY
	err := policy.AddAllowedProxy("http://host.docker.internal:7890")
	if err != nil {
		t.Fatalf("failed to add allowed proxy: %v", err)
	}

	// 1. Exact host:port match succeeds
	t.Run("allowed exact proxy endpoint 7890 succeeds", func(t *testing.T) {
		u, _ := url.Parse("http://host.docker.internal:7890")
		if err := policy.ValidateProxy(ctx, u); err != nil {
			t.Errorf("expected http://host.docker.internal:7890 to be allowed, got: %v", err)
		}
	})

	// 2. Dangerous ports on same host are blocked
	dangerousPorts := []string{"2375", "6379", "22"}
	for _, port := range dangerousPorts {
		t.Run(fmt.Sprintf("dangerous port %s rejected", port), func(t *testing.T) {
			u, _ := url.Parse(fmt.Sprintf("http://host.docker.internal:%s", port))
			err := policy.ValidateProxy(ctx, u)
			if err == nil {
				t.Fatalf("expected proxy port %s on host.docker.internal to be blocked, got nil", port)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "proxy_ssrf_blocked" {
				t.Fatalf("expected proxy_ssrf_blocked for port %s, got: %v", port, err)
			}
		})
	}

	// 3. DNS-resolved gateway private IP equivalent bypass is blocked
	t.Run("resolved gateway IP equivalent bypass rejected", func(t *testing.T) {
		u, _ := url.Parse("http://172.17.0.1:7890")
		err := policy.ValidateProxy(ctx, u)
		if err == nil {
			t.Fatal("expected equivalent private IP http://172.17.0.1:7890 to be blocked, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "proxy_ssrf_blocked" {
			t.Fatalf("expected proxy_ssrf_blocked for gateway IP, got: %v", err)
		}
	})

	// 4. Custom loopback proxy rejected
	t.Run("custom loopback proxy rejected", func(t *testing.T) {
		u, _ := url.Parse("http://127.0.0.1:7890")
		err := policy.ValidateProxy(ctx, u)
		if err == nil {
			t.Fatal("expected http://127.0.0.1:7890 to be blocked, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "proxy_ssrf_blocked" {
			t.Fatalf("expected proxy_ssrf_blocked for loopback, got: %v", err)
		}
	})

	// 5. Cloud metadata proxy rejected
	t.Run("cloud metadata proxy rejected", func(t *testing.T) {
		u, _ := url.Parse("http://169.254.169.254:80")
		err := policy.ValidateProxy(ctx, u)
		if err == nil {
			t.Fatal("expected http://169.254.169.254:80 to be blocked, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "proxy_ssrf_blocked" {
			t.Fatalf("expected proxy_ssrf_blocked for metadata, got: %v", err)
		}
	})

	// 6. Port omission rejected
	t.Run("port omission in proxy URL rejected", func(t *testing.T) {
		u, _ := url.Parse("http://host.docker.internal")
		err := policy.ValidateProxy(ctx, u)
		if err == nil {
			t.Fatal("expected error on port omission, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "invalid_proxy_url" {
			t.Fatalf("expected invalid_proxy_url for port omission, got: %v", err)
		}
	})

	// 7. Malicious userinfo rejected
	t.Run("malicious userinfo in proxy URL rejected", func(t *testing.T) {
		u, _ := url.Parse("http://admin:secret@host.docker.internal:7890")
		err := policy.ValidateProxy(ctx, u)
		if err == nil {
			t.Fatal("expected error on userinfo, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "invalid_proxy_url" {
			t.Fatalf("expected invalid_proxy_url for userinfo, got: %v", err)
		}
	})

	// 8. Obfuscated IPs rejected
	obfuscatedCases := []string{
		"http://0177.0.0.1:7890",
		"http://0x7f.0.0.1:7890",
		"http://2130706433:7890",
		"http://127.1:7890",
		"http://127.000.000.001:7890",
	}
	for _, raw := range obfuscatedCases {
		t.Run(fmt.Sprintf("obfuscated IP %s rejected", raw), func(t *testing.T) {
			u, _ := url.Parse(raw)
			err := policy.ValidateProxy(ctx, u)
			if err == nil {
				t.Fatalf("expected obfuscated proxy %s to be blocked, got nil", raw)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "proxy_ssrf_blocked" {
				t.Fatalf("expected proxy_ssrf_blocked for %s, got: %v", raw, err)
			}
		})
	}
}

func TestProxy_DialContext_BareHostAndPortSeparation(t *testing.T) {
	ctx := context.Background()

	resolver := &testMockResolver{
		mapping: map[string][]net.IPAddr{
			"host.docker.internal": {{IP: net.ParseIP("172.17.0.1")}},
			"public-site.example":  {{IP: net.ParseIP("93.184.216.34")}},
		},
	}

	policy := fetch.DefaultPolicy()
	policy.Resolver = resolver
	_ = policy.AddAllowedProxy("http://host.docker.internal:7890")

	client := fetch.NewClientWithPolicy(policy)

	// Fetch using unauthorized port on host.docker.internal must fail at proxy validation
	_, err := client.Fetch(ctx, fetch.Options{
		URL:           "http://public-site.example/feed",
		FetchProxyURL: "http://host.docker.internal:2375",
		Policy:        policy,
	})
	if err == nil {
		t.Fatal("expected fetch to fail for unauthorized proxy port 2375, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Code != "proxy_ssrf_blocked" {
		t.Fatalf("expected proxy_ssrf_blocked, got: %v", err)
	}
}

func TestPolicy_AddAllowedProxy_Validation(t *testing.T) {
	policy := fetch.DefaultPolicy()

	// Valid proxy endpoints
	if err := policy.AddAllowedProxy("http://host.docker.internal:7890"); err != nil {
		t.Errorf("expected success, got: %v", err)
	}
	// socks5 is unsupported scheme and must be rejected
	if err := policy.AddAllowedProxy("socks5://10.0.0.2:1080"); err == nil {
		t.Errorf("expected error for socks5 scheme, got nil")
	}

	// Invalid: port omitted
	if err := policy.AddAllowedProxy("http://host.docker.internal"); err == nil {
		t.Error("expected error for port omission, got nil")
	}

	// Invalid: userinfo
	if err := policy.AddAllowedProxy("http://user:pass@host.docker.internal:7890"); err == nil {
		t.Error("expected error for userinfo, got nil")
	}

	// Invalid: unsupported scheme
	if err := policy.AddAllowedProxy("ftp://host.docker.internal:7890"); err == nil {
		t.Error("expected error for unsupported scheme, got nil")
	}

	// Invalid: obfuscated IP
	if err := policy.AddAllowedProxy("http://0177.0.0.1:7890"); err == nil {
		t.Error("expected error for obfuscated IP, got nil")
	}

	// Invalid: empty
	if err := policy.AddAllowedProxy(""); err == nil {
		t.Error("expected error for empty proxy endpoint, got nil")
	}
}

func TestProxy_ObfuscatedIP_TargetBlocking(t *testing.T) {
	ctx := context.Background()
	policy := fetch.DefaultPolicy()

	obfuscatedTargets := []string{
		"http://0177.0.0.1/feed",
		"http://0x7f.0.0.1/feed",
		"http://2130706433/feed",
		"http://127.1/feed",
	}

	for _, raw := range obfuscatedTargets {
		u, _ := url.Parse(raw)
		err := policy.ValidateTarget(ctx, u)
		if err == nil {
			t.Errorf("expected target %s to be blocked as ssrf_blocked, got nil", raw)
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "ssrf_blocked" {
			t.Errorf("expected ssrf_blocked for %s, got: %v", raw, err)
		}
	}
}

func generateTestCertificate(t *testing.T, dnsName string) (tls.Certificate, *x509.CertPool) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"CSP Test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{dnsName},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(cert)

	return tlsCert, certPool
}

func TestProxy_TargetIPPinning_WireFormat_HTTP(t *testing.T) {
	ctx := context.Background()

	// 1. Mock proxy listener recording wire format
	var capturedRequestLine string
	var capturedHeaders = make(http.Header)
	var captureMu sync.Mutex

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on proxy port: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		br := bufio.NewReader(conn)
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		captureMu.Lock()
		capturedRequestLine = strings.TrimSpace(line)
		for {
			hdrLine, err := br.ReadString('\n')
			if err != nil || strings.TrimSpace(hdrLine) == "" {
				break
			}
			parts := strings.SplitN(strings.TrimSpace(hdrLine), ":", 2)
			if len(parts) == 2 {
				capturedHeaders.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}
		captureMu.Unlock()

		body := "wire-format-verified-payload"
		resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\nContent-Type: text/plain\r\n\r\n%s", len(body), body)
		conn.Write([]byte(resp))
	}()

	mockResolver := &testMockResolver{
		mapping: map[string][]net.IPAddr{
			"sub.example.com": {{IP: net.ParseIP("93.184.216.34")}},
		},
	}

	policy := fetch.DefaultPolicy()
	policy.Resolver = mockResolver
	proxyURLStr := "http://" + proxyListener.Addr().String()
	if err := policy.AddAllowedProxy(proxyURLStr); err != nil {
		t.Fatalf("failed to add allowed proxy: %v", err)
	}

	client := fetch.NewClientWithPolicy(policy)
	resp, err := client.Fetch(ctx, fetch.Options{
		URL:           "http://sub.example.com/sub/feed?token=xyz",
		FetchProxyURL: proxyURLStr,
		Policy:        policy,
	})
	if err != nil {
		t.Fatalf("fetch via proxy failed: %v", err)
	}

	if string(resp.Body) != "wire-format-verified-payload" {
		t.Fatalf("unexpected body: %s", string(resp.Body))
	}

	captureMu.Lock()
	defer captureMu.Unlock()

	expectedReqLine := "GET http://93.184.216.34:80/sub/feed?token=xyz HTTP/1.1"
	if capturedRequestLine != expectedReqLine {
		t.Errorf("expected request line %q, got %q", expectedReqLine, capturedRequestLine)
	}
	if strings.Contains(capturedRequestLine, "sub.example.com") {
		t.Errorf("request line leaked unpinned domain name: %s", capturedRequestLine)
	}

	if h := capturedHeaders.Get("Host"); h != "sub.example.com" {
		t.Errorf("expected Host header 'sub.example.com', got %q", h)
	}
	if ua := capturedHeaders.Get("User-Agent"); ua != "clash-verge/v2.5.6" {
		t.Errorf("expected User-Agent 'clash-verge/v2.5.6', got %q", ua)
	}
	if acc := capturedHeaders.Get("Accept"); acc != "*/*" {
		t.Errorf("expected Accept '*/*', got %q", acc)
	}
	if enc := capturedHeaders.Get("Accept-Encoding"); enc != "gzip, deflate" {
		t.Errorf("expected Accept-Encoding 'gzip, deflate', got %q", enc)
	}
}

func TestProxy_TargetIPPinning_WireFormat_HTTPS(t *testing.T) {
	ctx := context.Background()

	tlsCert, rootCAs := generateTestCertificate(t, "sub.example.com")

	var targetReceivedSNI string
	var targetReceivedHost string
	originListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer originListener.Close()

	tlsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			targetReceivedSNI = r.TLS.ServerName
		}
		targetReceivedHost = r.Host
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("https-pinned-success"))
	}))
	tlsServer.Listener = originListener
	tlsServer.TLS = &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}
	tlsServer.StartTLS()
	defer tlsServer.Close()

	var capturedConnectLine string
	var capturedConnectHost string
	var connectMu sync.Mutex

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on proxy port: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		clientConn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		defer clientConn.Close()

		br := bufio.NewReader(clientConn)
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		connectMu.Lock()
		capturedConnectLine = strings.TrimSpace(line)
		for {
			hdrLine, err := br.ReadString('\n')
			if err != nil || strings.TrimSpace(hdrLine) == "" {
				break
			}
			parts := strings.SplitN(strings.TrimSpace(hdrLine), ":", 2)
			if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "Host") {
				capturedConnectHost = strings.TrimSpace(parts[1])
			}
		}
		connectMu.Unlock()

		originConn, err := net.Dial("tcp", originListener.Addr().String())
		if err != nil {
			clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer originConn.Close()

		clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			io.Copy(originConn, clientConn)
		}()
		go func() {
			defer wg.Done()
			io.Copy(clientConn, originConn)
		}()
		wg.Wait()
	}()

	mockResolver := &testMockResolver{
		mapping: map[string][]net.IPAddr{
			"sub.example.com": {{IP: net.ParseIP("93.184.216.34")}},
		},
	}

	policy := fetch.DefaultPolicy()
	policy.Resolver = mockResolver
	proxyURLStr := "http://" + proxyListener.Addr().String()
	if err := policy.AddAllowedProxy(proxyURLStr); err != nil {
		t.Fatalf("failed to add allowed proxy: %v", err)
	}

	client := fetch.NewClientWithPolicy(policy)

	resp, err := client.Fetch(ctx, fetch.Options{
		URL:           "https://sub.example.com/feed?token=xyz",
		FetchProxyURL: proxyURLStr,
		Policy:        policy,
		RootCAs:       rootCAs,
	})
	if err != nil {
		t.Fatalf("https fetch via proxy failed: %v", err)
	}

	if string(resp.Body) != "https-pinned-success" {
		t.Fatalf("unexpected body: %s", string(resp.Body))
	}

	connectMu.Lock()
	defer connectMu.Unlock()

	expectedConnect := "CONNECT 93.184.216.34:443 HTTP/1.1"
	if capturedConnectLine != expectedConnect {
		t.Errorf("expected CONNECT line %q, got %q", expectedConnect, capturedConnectLine)
	}
	if strings.Contains(capturedConnectLine, "sub.example.com") {
		t.Errorf("CONNECT line leaked unpinned domain name: %s", capturedConnectLine)
	}
	if capturedConnectHost != "93.184.216.34:443" {
		t.Errorf("expected CONNECT Host header '93.184.216.34:443', got %q", capturedConnectHost)
	}

	if targetReceivedSNI != "sub.example.com" {
		t.Errorf("expected origin TLS SNI 'sub.example.com', got %q", targetReceivedSNI)
	}
	if targetReceivedHost != "sub.example.com" {
		t.Errorf("expected origin Host header 'sub.example.com', got %q", targetReceivedHost)
	}

	t.Run("strict TLS verification fails on domain mismatch", func(t *testing.T) {
		wrongCert, wrongPool := generateTestCertificate(t, "evil.attacker.com")
		wrongListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer wrongListener.Close()

		wrongTLSServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		wrongTLSServer.Listener = wrongListener
		wrongTLSServer.TLS = &tls.Config{Certificates: []tls.Certificate{wrongCert}}
		wrongTLSServer.StartTLS()
		defer wrongTLSServer.Close()

		wrongProxyListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer wrongProxyListener.Close()

		go func() {
			c, err := wrongProxyListener.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			br := bufio.NewReader(c)
			for {
				l, _ := br.ReadString('\n')
				if strings.TrimSpace(l) == "" {
					break
				}
			}
			orig, err := net.Dial("tcp", wrongListener.Addr().String())
			if err != nil {
				return
			}
			defer orig.Close()
			c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			go io.Copy(orig, c)
			io.Copy(c, orig)
		}()

		wrongPolicy := fetch.DefaultPolicy()
		wrongPolicy.Resolver = mockResolver
		_ = wrongPolicy.AddAllowedProxy("http://" + wrongProxyListener.Addr().String())

		_, err = client.Fetch(ctx, fetch.Options{
			URL:           "https://sub.example.com/feed",
			FetchProxyURL: "http://" + wrongProxyListener.Addr().String(),
			Policy:        wrongPolicy,
			RootCAs:       wrongPool,
		})
		if err == nil {
			t.Fatal("expected TLS certificate verification failure on mismatch, got nil")
		}
	})
}

func TestProxy_ProxySideDNSPoisoningDefense(t *testing.T) {
	ctx := context.Background()

	var receivedAuthority string
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		c, err := proxyListener.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		line, _ := br.ReadString('\n')
		receivedAuthority = strings.TrimSpace(line)
		for {
			h, _ := br.ReadString('\n')
			if strings.TrimSpace(h) == "" {
				break
			}
		}
		c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	}()

	mockResolver := &testMockResolver{
		mapping: map[string][]net.IPAddr{
			"poisoned.example.com": {{IP: net.ParseIP("93.184.216.34")}},
		},
	}

	policy := fetch.DefaultPolicy()
	policy.Resolver = mockResolver
	proxyURLStr := "http://" + proxyListener.Addr().String()
	_ = policy.AddAllowedProxy(proxyURLStr)

	client := fetch.NewClientWithPolicy(policy)
	_, err = client.Fetch(ctx, fetch.Options{
		URL:           "http://poisoned.example.com/feed",
		FetchProxyURL: proxyURLStr,
		Policy:        policy,
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	if !strings.Contains(receivedAuthority, "93.184.216.34") {
		t.Errorf("proxy request line did not contain pinned public IP: %s", receivedAuthority)
	}
	if strings.Contains(receivedAuthority, "poisoned.example.com") {
		t.Errorf("proxy request line leaked poisoned domain name: %s", receivedAuthority)
	}

	t.Run("local DNS resolving to loopback blocked before proxy", func(t *testing.T) {
		loopbackResolver := &testMockResolver{
			mapping: map[string][]net.IPAddr{
				"bad-dns.example.com": {{IP: net.ParseIP("127.0.0.1")}},
			},
		}
		p := fetch.DefaultPolicy()
		p.Resolver = loopbackResolver
		_ = p.AddAllowedProxy(proxyURLStr)

		_, err := client.Fetch(ctx, fetch.Options{
			URL:           "http://bad-dns.example.com/feed",
			FetchProxyURL: proxyURLStr,
			Policy:        p,
		})
		if err == nil {
			t.Fatal("expected ssrf_blocked, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "ssrf_blocked" {
			t.Fatalf("expected code ssrf_blocked, got: %v", err)
		}
	})

	t.Run("302 redirect to loopback via proxy rejected", func(t *testing.T) {
		redirectProxyListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer redirectProxyListener.Close()

		go func() {
			c, err := redirectProxyListener.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			br := bufio.NewReader(c)
			for {
				h, _ := br.ReadString('\n')
				if strings.TrimSpace(h) == "" {
					break
				}
			}
			c.Write([]byte("HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1:18080/internal\r\nContent-Length: 0\r\n\r\n"))
		}()

		redirPolicy := fetch.DefaultPolicy()
		redirPolicy.Resolver = mockResolver
		_ = redirPolicy.AddAllowedProxy("http://" + redirectProxyListener.Addr().String())

		_, err = client.Fetch(ctx, fetch.Options{
			URL:           "http://poisoned.example.com/feed",
			FetchProxyURL: "http://" + redirectProxyListener.Addr().String(),
			Policy:        redirPolicy,
		})
		if err == nil {
			t.Fatal("expected ssrf_blocked on redirect to loopback, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "ssrf_blocked" {
			t.Fatalf("expected code ssrf_blocked, got: %v", err)
		}
	})
}

func TestProxy_MultiHopRedirectIPRebinding(t *testing.T) {
	ctx := context.Background()

	var hopRequests []string
	var hopHosts []string
	var mu sync.Mutex

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		for i := 0; i < 3; i++ {
			c, err := proxyListener.Accept()
			if err != nil {
				return
			}
			br := bufio.NewReader(c)
			line, _ := br.ReadString('\n')
			var hostHeader string
			for {
				hdr, _ := br.ReadString('\n')
				if strings.TrimSpace(hdr) == "" {
					break
				}
				if strings.HasPrefix(strings.ToLower(hdr), "host:") {
					hostHeader = strings.TrimSpace(strings.TrimPrefix(hdr, "Host:"))
				}
			}
			mu.Lock()
			hopRequests = append(hopRequests, strings.TrimSpace(line))
			hopHosts = append(hopHosts, hostHeader)
			step := len(hopRequests)
			mu.Unlock()

			if step == 1 {
				c.Write([]byte("HTTP/1.1 302 Found\r\nLocation: http://hop2.example.com/step2\r\nContent-Length: 0\r\n\r\n"))
			} else if step == 2 {
				c.Write([]byte("HTTP/1.1 302 Found\r\nLocation: http://hop3.example.com/final\r\nContent-Length: 0\r\n\r\n"))
			} else {
				c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 16\r\n\r\nmulti-hop-done!!"))
			}
			c.Close()
		}
	}()

	mockResolver := &testMockResolver{
		mapping: map[string][]net.IPAddr{
			"hop1.example.com": {{IP: net.ParseIP("93.184.216.10")}},
			"hop2.example.com": {{IP: net.ParseIP("93.184.216.20")}},
			"hop3.example.com": {{IP: net.ParseIP("93.184.216.30")}},
		},
	}

	policy := fetch.DefaultPolicy()
	policy.Resolver = mockResolver
	proxyURLStr := "http://" + proxyListener.Addr().String()
	_ = policy.AddAllowedProxy(proxyURLStr)

	client := fetch.NewClientWithPolicy(policy)
	resp, err := client.Fetch(ctx, fetch.Options{
		URL:           "http://hop1.example.com/start",
		FetchProxyURL: proxyURLStr,
		Policy:        policy,
	})
	if err != nil {
		t.Fatalf("multi-hop fetch failed: %v", err)
	}

	if string(resp.Body) != "multi-hop-done!!" {
		t.Fatalf("unexpected body: %s", string(resp.Body))
	}

	mu.Lock()
	defer mu.Unlock()

	if len(hopRequests) != 3 {
		t.Fatalf("expected 3 hops recorded, got %d", len(hopRequests))
	}

	if !strings.Contains(hopRequests[0], "93.184.216.10:80") || hopHosts[0] != "hop1.example.com" {
		t.Errorf("hop 1 mismatch: req=%q, host=%q", hopRequests[0], hopHosts[0])
	}
	if !strings.Contains(hopRequests[1], "93.184.216.20:80") || hopHosts[1] != "hop2.example.com" {
		t.Errorf("hop 2 mismatch: req=%q, host=%q", hopRequests[1], hopHosts[1])
	}
	if !strings.Contains(hopRequests[2], "93.184.216.30:80") || hopHosts[2] != "hop3.example.com" {
		t.Errorf("hop 3 mismatch: req=%q, host=%q", hopRequests[2], hopHosts[2])
	}
}

func TestProxy_ProtocolAndPortConfusion_EdgeCases(t *testing.T) {
	ctx := context.Background()
	policy := fetch.DefaultPolicy()

	if err := policy.AddAllowedProxy("http://host.docker.internal:7890"); err != nil {
		t.Fatalf("failed to add allowed proxy: %v", err)
	}

	unsupportedSchemes := []string{
		"socks5://host.docker.internal:7890",
		"https://host.docker.internal:7890",
		"ftp://host.docker.internal:7890",
		"host.docker.internal:7890",
	}
	for _, raw := range unsupportedSchemes {
		t.Run("unsupported scheme "+raw, func(t *testing.T) {
			if policy.IsProxyHostAllowed(raw) {
				t.Fatalf("expected %s to NOT be allowed as proxy", raw)
			}
			u, _ := url.Parse(raw)
			err := policy.ValidateProxy(ctx, u)
			if err == nil {
				t.Fatalf("expected ValidateProxy to fail for %s, got nil", raw)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "invalid_proxy_url" {
				t.Fatalf("expected invalid_proxy_url for %s, got: %v", raw, err)
			}
		})
	}

	leadingZeroPorts := []string{
		"http://host.docker.internal:07890",
		"http://host.docker.internal:0080",
	}
	for _, raw := range leadingZeroPorts {
		t.Run("leading zero port "+raw, func(t *testing.T) {
			if policy.IsProxyHostAllowed(raw) {
				t.Fatalf("expected %s to NOT be allowed as proxy", raw)
			}
			u, _ := url.Parse(raw)
			err := policy.ValidateProxy(ctx, u)
			if err == nil {
				t.Fatalf("expected ValidateProxy to fail for leading zero port %s, got nil", raw)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "invalid_proxy_url" {
				t.Fatalf("expected invalid_proxy_url for %s, got: %v", raw, err)
			}
		})
	}

	trailingDots := []string{
		"http://host.docker.internal.:7890",
		"http://127.0.0.1.:7890",
	}
	for _, raw := range trailingDots {
		t.Run("trailing dot "+raw, func(t *testing.T) {
			if policy.IsProxyHostAllowed(raw) {
				t.Fatalf("expected %s to NOT be allowed as proxy", raw)
			}
			u, _ := url.Parse(raw)
			err := policy.ValidateProxy(ctx, u)
			if err == nil {
				t.Fatalf("expected ValidateProxy to fail for trailing dot %s, got nil", raw)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "invalid_proxy_url" {
				t.Fatalf("expected invalid_proxy_url for %s, got: %v", raw, err)
			}
		})
	}

	specialIPv6 := []struct {
		url          string
		expectedCode string
	}{
		{"http://[fe80::1%25eth0]:7890", "invalid_proxy_url"},
		{"http://[::ffff:127.0.0.1]:7890", "proxy_ssrf_blocked"},
	}
	for _, tc := range specialIPv6 {
		t.Run("special IPv6 "+tc.url, func(t *testing.T) {
			if policy.IsProxyHostAllowed(tc.url) {
				t.Fatalf("expected %s to NOT be allowed as proxy", tc.url)
			}
			u, _ := url.Parse(tc.url)
			err := policy.ValidateProxy(ctx, u)
			if err == nil {
				t.Fatalf("expected ValidateProxy to fail for %s, got nil", tc.url)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != tc.expectedCode {
				t.Fatalf("expected %s for %s, got: %v", tc.expectedCode, tc.url, err)
			}
		})
	}

	invalidExtraParts := []string{
		"http://host.docker.internal:7890/some/path",
		"http://host.docker.internal:7890?query=1",
		"http://host.docker.internal:7890#fragment",
	}
	for _, raw := range invalidExtraParts {
		t.Run("extra parts "+raw, func(t *testing.T) {
			u, _ := url.Parse(raw)
			err := policy.ValidateProxy(ctx, u)
			if err == nil {
				t.Fatalf("expected ValidateProxy to fail for %s, got nil", raw)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "invalid_proxy_url" {
				t.Fatalf("expected invalid_proxy_url for %s, got: %v", raw, err)
			}
		})
	}

	t.Run("canonical IPv6 normalization", func(t *testing.T) {
		p := fetch.DefaultPolicy()
		err := p.AddAllowedProxy("http://[2001:0db8:0000:0000:0000:0000:0000:0001]:7890")
		if err != nil {
			t.Fatalf("failed to add IPv6 proxy: %v", err)
		}
		if len(p.AllowedProxyHosts) != 1 || p.AllowedProxyHosts[0] != "http://[2001:db8::1]:7890" {
			t.Errorf("expected normalized http://[2001:db8::1]:7890, got: %v", p.AllowedProxyHosts)
		}
		if !p.IsProxyHostAllowed("http://[2001:db8::1]:7890") {
			t.Error("expected IsProxyHostAllowed to match canonical IPv6")
		}
	})

	t.Run("custom proxy to RFC1918 private IP blocked", func(t *testing.T) {
		u, _ := url.Parse("http://192.168.1.50:8080")
		err := policy.ValidateProxy(ctx, u)
		if err == nil {
			t.Fatal("expected ValidateProxy to block private IP, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "proxy_ssrf_blocked" {
			t.Fatalf("expected proxy_ssrf_blocked for private IP, got: %v", err)
		}
	})
}

func TestProxy_DirectTargetCannotAbuseAllowedProxyHost(t *testing.T) {
	ctx := context.Background()

	mockResolver := &testMockResolver{
		mapping: map[string][]net.IPAddr{
			"host.docker.internal": {{IP: net.ParseIP("172.17.0.1")}},
		},
	}

	policy := fetch.DefaultPolicy()
	policy.Resolver = mockResolver
	_ = policy.AddAllowedProxy("http://host.docker.internal:7890")

	client := fetch.NewClientWithPolicy(policy)

	_, err := client.Fetch(ctx, fetch.Options{
		URL:    "http://host.docker.internal:7890/metrics",
		Policy: policy,
	})
	if err == nil {
		t.Fatal("expected direct fetch to host.docker.internal:7890 to be blocked by SSRF, got nil")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Code != "ssrf_blocked" {
		t.Fatalf("expected code ssrf_blocked, got: %v", err)
	}
}
