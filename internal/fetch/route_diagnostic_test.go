package fetch_test

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

func TestCredentialedHTTPSProxyAndStrictTargets(t *testing.T) {
	var receivedHost, receivedAuth, requestURI string
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost, receivedAuth, requestURI = r.Host, r.Header.Get("Proxy-Authorization"), r.RequestURI
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://127.0.0.1/private", 302)
			return
		}
		w.Write([]byte("trusted-proxy"))
	}))
	defer proxy.Close()
	roots := x509.NewCertPool()
	roots.AddCert(proxy.Certificate())
	proxyURL := strings.Replace(proxy.URL, "://", "://fixture:password@", 1)
	policy := fetch.DefaultPolicy()
	policy.Resolver = &testMockResolver{mapping: map[string][]net.IPAddr{"feed.example": {{IP: net.ParseIP("93.184.216.34")}}}}
	if err := policy.AddAllowedProxy(proxyURL); err != nil {
		t.Fatal(err)
	}
	client := fetch.NewClientWithPolicy(policy)
	response, err := client.Fetch(context.Background(), fetch.Options{URL: "http://feed.example/feed?token=fixture", FetchProxyURL: proxyURL, RootCAs: roots, Timeout: 2 * time.Second})
	if err != nil || string(response.Body) != "trusted-proxy" {
		t.Fatalf("HTTPS proxy: %+v %v", response, err)
	}
	if receivedHost != "feed.example" || receivedAuth != "Basic Zml4dHVyZTpwYXNzd29yZA==" || requestURI != "http://feed.example/feed?token=fixture" {
		t.Fatalf("wrong wire semantics host=%s URI=%s auth-present=%v", receivedHost, requestURI, receivedAuth != "")
	}
	for _, target := range []string{"http://127.0.0.1/feed", "http://feed.example/redirect"} {
		_, err = client.Fetch(context.Background(), fetch.Options{URL: target, FetchProxyURL: proxyURL, RootCAs: roots, Timeout: 2 * time.Second})
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "ssrf_blocked" {
			t.Fatalf("private target/redirect allowed: %v", err)
		}
	}
	_, err = client.Fetch(context.Background(), fetch.Options{URL: "http://feed.example/feed", FetchProxyURL: proxyURL, Timeout: 2 * time.Second})
	de, ok := domain.AsDomainError(err)
	if !ok || de.Details["stage"] != "tls" || de.Details["reason_code"] != "tls_certificate_failed" {
		t.Fatalf("proxy TLS must verify independently: %v", err)
	}
}

func TestFetchFailureStagesDoNotLeak(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	policy := fetch.DefaultPolicy()
	policy.AllowedHosts = []string{"127.0.0.1"}
	client := fetch.NewClientWithPolicy(policy)
	_, err = client.Fetch(context.Background(), fetch.Options{URL: "http://user:password-secret@" + addr + "/feed?token=query-secret", Timeout: time.Second})
	de, ok := domain.AsDomainError(err)
	if !ok || de.Details["reason_code"] != "connection_refused" || de.Details["stage"] != "connect" {
		t.Fatalf("no refused cause: %v", err)
	}
	if strings.Contains(err.Error(), "password-secret") || strings.Contains(err.Error(), "query-secret") || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("unsafe or swallowed diagnostics: %v", err)
	}
	policy = fetch.DefaultPolicy()
	policy.Resolver = &testMockResolver{mapping: map[string][]net.IPAddr{}}
	_, err = fetch.NewClientWithPolicy(policy).Fetch(context.Background(), fetch.Options{URL: "http://missing.example/feed?token=query-secret"})
	de, ok = domain.AsDomainError(err)
	if !ok || de.Details["stage"] != "dns" {
		t.Fatalf("no DNS stage: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	policy = fetch.DefaultPolicy()
	policy.AllowedHosts = []string{"127.0.0.1"}
	_, err = fetch.NewClientWithPolicy(policy).Fetch(context.Background(), fetch.Options{URL: server.URL})
	de, ok = domain.AsDomainError(err)
	if !ok || de.Details["stage"] != "http" || !strings.Contains(de.Message, "503") {
		t.Fatalf("no HTTP cause: %v", err)
	}
}
