package fetch_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
