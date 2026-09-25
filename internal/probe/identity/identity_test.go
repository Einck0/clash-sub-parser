package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/identity"
)

const (
	testIP1    = "198.51.100.22"
	testIP2    = "203.0.113.99"
	testNodeID = "node_0123456789abcdef"
)

func TestResolverRequiresPairwiseAgreementBeforePublishingIdentity(t *testing.T) {
	fixedTime := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixedTime }
	resolver := identity.NewResolver(identity.WithClock(clock), identity.WithTTL(10*time.Minute))

	sources := []identity.Source{
		identity.NewStaticSource("ip.sb", identity.Candidate{
			IP:           testIP1,
			CountryCode:  "us",
			ASN:          13335,
			Organization: "Cloudflare Inc.",
		}),
		identity.NewStaticSource("cloudflare", identity.Candidate{
			IP:          testIP1,
			CountryCode: "US",
		}),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Status != identity.StatusVerified {
		t.Fatalf("status = %s, want %s", result.Status, identity.StatusVerified)
	}
	if result.Identity == nil {
		t.Fatal("expected non-nil ExitIdentity")
	}

	var _ *domain.ExitIdentity = result.Identity

	// Validate ExitIdentity domain contract
	if err := result.Identity.Validate(); err != nil {
		t.Fatalf("ExitIdentity.Validate() failed: %v", err)
	}

	if result.Identity.NodeLogicalID != testNodeID {
		t.Fatalf("nodeLogicalID = %s, want %s", result.Identity.NodeLogicalID, testNodeID)
	}
	if result.Identity.CountryCode != "US" {
		t.Fatalf("countryCode = %s, want US", result.Identity.CountryCode)
	}
	if result.Identity.ASN == nil || *result.Identity.ASN != 13335 {
		t.Fatalf("ASN = %v, want 13335", result.Identity.ASN)
	}
	if !strings.HasPrefix(result.Identity.IdentityDigest, "sha256:") || len(result.Identity.IdentityDigest) != 71 {
		t.Fatalf("invalid identity digest format: %s", result.Identity.IdentityDigest)
	}
	if len(result.AgreedSources) != 2 {
		t.Fatalf("agreed sources length = %d, want 2", len(result.AgreedSources))
	}
}

func TestResolverRejectsPairwiseDisagreementOnIP(t *testing.T) {
	resolver := identity.NewResolver()
	sources := []identity.Source{
		identity.NewStaticSource("ip.sb", identity.Candidate{IP: testIP1, CountryCode: "US"}),
		identity.NewStaticSource("cloudflare", identity.Candidate{IP: testIP2, CountryCode: "US"}),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Status != identity.StatusConflicted {
		t.Fatalf("status = %s, want %s", result.Status, identity.StatusConflicted)
	}
	if result.Identity != nil {
		t.Fatal("conflicted resolution must NOT produce ExitIdentity")
	}
}

func TestResolverRejectsPairwiseDisagreementOnCountry(t *testing.T) {
	resolver := identity.NewResolver()
	sources := []identity.Source{
		identity.NewStaticSource("ip.sb", identity.Candidate{IP: testIP1, CountryCode: "US"}),
		identity.NewStaticSource("cloudflare", identity.Candidate{IP: testIP1, CountryCode: "GB"}),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Status != identity.StatusConflicted {
		t.Fatalf("status = %s, want %s", result.Status, identity.StatusConflicted)
	}
	if result.Identity != nil {
		t.Fatal("conflicted country resolution must NOT produce ExitIdentity")
	}
}

func TestResolverRejectsPairwiseDisagreementOnASN(t *testing.T) {
	resolver := identity.NewResolver()
	sources := []identity.Source{
		identity.NewStaticSource("ip.sb", identity.Candidate{IP: testIP1, CountryCode: "US", ASN: 13335}),
		identity.NewStaticSource("other", identity.Candidate{IP: testIP1, CountryCode: "US", ASN: 15169}),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Status != identity.StatusConflicted {
		t.Fatalf("status = %s, want %s", result.Status, identity.StatusConflicted)
	}
	if result.Identity != nil {
		t.Fatal("conflicted ASN resolution must NOT produce ExitIdentity")
	}
}

func TestResolverOneSourceFailingYieldsUnavailable(t *testing.T) {
	resolver := identity.NewResolver()
	sources := []identity.Source{
		identity.NewStaticSource("ip.sb", identity.Candidate{IP: testIP1, CountryCode: "US"}),
		identity.NewFailingStaticSource("cloudflare", errors.New("connection timeout")),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Single source cannot form verified consensus; must be quarantined as unavailable
	if result.Status != identity.StatusUnavailable {
		t.Fatalf("status = %s, want %s", result.Status, identity.StatusUnavailable)
	}
	if result.Identity != nil {
		t.Fatal("quarantined resolution must NOT produce ExitIdentity")
	}
}

func TestResolverAllSourcesFailingYieldsUnavailable(t *testing.T) {
	resolver := identity.NewResolver()
	sources := []identity.Source{
		identity.NewFailingStaticSource("ip.sb", errors.New("network unreachable")),
		identity.NewFailingStaticSource("cloudflare", errors.New("timeout")),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Status != identity.StatusUnavailable {
		t.Fatalf("status = %s, want %s", result.Status, identity.StatusUnavailable)
	}
	if result.Identity != nil {
		t.Fatal("all failed resolution must NOT produce ExitIdentity")
	}
}

func TestResolverFewerThanTwoSourcesYieldsUnavailable(t *testing.T) {
	resolver := identity.NewResolver()
	sources := []identity.Source{
		identity.NewStaticSource("single_source", identity.Candidate{IP: testIP1, CountryCode: "US"}),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Status != identity.StatusUnavailable {
		t.Fatalf("status = %s, want %s", result.Status, identity.StatusUnavailable)
	}
}

func TestResolverPerNodeTTLCacheAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	ttl := 5 * time.Minute

	resolver := identity.NewResolver(identity.WithClock(clock), identity.WithTTL(ttl))

	callCount1 := 0
	callCount2 := 0
	sources := []identity.Source{
		&mockCountingSource{
			name: "s1",
			fn: func() (identity.Candidate, error) {
				callCount1++
				return identity.Candidate{IP: testIP1, CountryCode: "US"}, nil
			},
		},
		&mockCountingSource{
			name: "s2",
			fn: func() (identity.Candidate, error) {
				callCount2++
				return identity.Candidate{IP: testIP1, CountryCode: "US"}, nil
			},
		},
	}

	// 1. Initial resolution (cache miss)
	res1, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	if res1.Status != identity.StatusVerified {
		t.Fatalf("status = %s, want verified", res1.Status)
	}
	if callCount1 != 1 || callCount2 != 1 {
		t.Fatalf("call counts = (%d, %d), want (1, 1)", callCount1, callCount2)
	}

	// 2. Cache hit within TTL
	now = now.Add(2 * time.Minute)
	res2, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("cached Resolve() error = %v", err)
	}
	if res2.Status != identity.StatusVerified {
		t.Fatalf("status = %s, want verified", res2.Status)
	}
	if callCount1 != 1 || callCount2 != 1 {
		t.Fatalf("cache hit called sources again! counts = (%d, %d)", callCount1, callCount2)
	}
	if res1.Identity.IdentityDigest != res2.Identity.IdentityDigest {
		t.Fatalf("digests differ across cache hit: %s != %s", res1.Identity.IdentityDigest, res2.Identity.IdentityDigest)
	}

	// 3. Cache expired after TTL
	now = now.Add(4 * time.Minute) // 6 minutes total > 5 minutes TTL
	res3, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("post-expiry Resolve() error = %v", err)
	}
	if res3.Status != identity.StatusVerified {
		t.Fatalf("status = %s, want verified", res3.Status)
	}
	if callCount1 != 2 || callCount2 != 2 {
		t.Fatalf("post-expiry call counts = (%d, %d), want (2, 2)", callCount1, callCount2)
	}
}

func TestResolverCacheManagement(t *testing.T) {
	resolver := identity.NewResolver(identity.WithTTL(10 * time.Minute))

	sources := []identity.Source{
		identity.NewStaticSource("s1", identity.Candidate{IP: testIP1, CountryCode: "JP"}),
		identity.NewStaticSource("s2", identity.Candidate{IP: testIP1, CountryCode: "JP"}),
	}

	_, err := resolver.Resolve(context.Background(), http.DefaultClient, "node_0123456789abcdef", sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	_, err = resolver.Resolve(context.Background(), http.DefaultClient, "node_fedcba9876543210", sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resolver.Len() != 2 {
		t.Fatalf("cache length = %d, want 2", resolver.Len())
	}

	// Invalidate one
	resolver.Invalidate("node_0123456789abcdef")
	if resolver.Len() != 1 {
		t.Fatalf("cache length after Invalidate = %d, want 1", resolver.Len())
	}
	if _, ok := resolver.Get("node_0123456789abcdef"); ok {
		t.Fatal("expected invalidated node to be absent from cache")
	}
	if _, ok := resolver.Get("node_fedcba9876543210"); !ok {
		t.Fatal("expected second node to remain in cache")
	}

	// Clear all
	resolver.Clear()
	if resolver.Len() != 0 {
		t.Fatalf("cache length after Clear = %d, want 0", resolver.Len())
	}
}

func TestResolverHandlesIPv6CanonicalEquivalence(t *testing.T) {
	resolver := identity.NewResolver()
	// Two different string notations of the exact same IPv6 address
	ipv6A := "2001:db8::1"
	ipv6B := "2001:0db8:0000:0000:0000:0000:0000:0001"

	sources := []identity.Source{
		identity.NewStaticSource("s1", identity.Candidate{IP: ipv6A, CountryCode: "SG"}),
		identity.NewStaticSource("s2", identity.Candidate{IP: ipv6B, CountryCode: "SG"}),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result.Status != identity.StatusVerified {
		t.Fatalf("IPv6 equivalent representations failed consensus! status = %s", result.Status)
	}
	if result.Identity == nil {
		t.Fatal("expected non-nil ExitIdentity for IPv6")
	}
	if result.Identity.CountryCode != "SG" {
		t.Fatalf("countryCode = %s, want SG", result.Identity.CountryCode)
	}
}

func TestExtractCandidateFromJSON(t *testing.T) {
	// Standard ip.sb json
	payload1 := []byte(`{"ip":"198.51.100.22","country_code":"US","asn":13335,"organization":"Cloudflare"}`)
	c1, err := identity.ExtractCandidate(payload1)
	if err != nil {
		t.Fatalf("ExtractCandidate() json error = %v", err)
	}
	if c1.NormalizedIP() != testIP1 || c1.NormalizedCountryCode() != "US" || c1.ASN != 13335 {
		t.Fatalf("c1 unexpected: %+v", c1)
	}

	// String ASN with "AS" prefix
	payload2 := []byte(`{"query":"203.0.113.99","country":"HK","asn":"AS15169"}`)
	c2, err := identity.ExtractCandidate(payload2)
	if err != nil {
		t.Fatalf("ExtractCandidate() query/country error = %v", err)
	}
	if c2.NormalizedIP() != testIP2 || c2.NormalizedCountryCode() != "HK" || c2.ASN != 15169 {
		t.Fatalf("c2 unexpected: %+v", c2)
	}

	// Lowercase country code normalized to uppercase
	payload3 := []byte(`{"ip":"198.51.100.22","countryCode":"jp"}`)
	c3, err := identity.ExtractCandidate(payload3)
	if err != nil {
		t.Fatalf("ExtractCandidate() countryCode error = %v", err)
	}
	if c3.NormalizedCountryCode() != "JP" {
		t.Fatalf("countryCode not uppercase: %s", c3.NormalizedCountryCode())
	}
}

func TestExtractCandidateFromTrace(t *testing.T) {
	tracePayload := []byte("fl=123f45\nh=cloudflare.com\nip=198.51.100.22\nts=1788546780\nloc=US\nasn=13335\nvisit_scheme=https\n")
	cand, err := identity.ExtractCandidate(tracePayload)
	if err != nil {
		t.Fatalf("ExtractCandidate() trace error = %v", err)
	}
	if cand.NormalizedIP() != testIP1 || cand.NormalizedCountryCode() != "US" || cand.ASN != 13335 {
		t.Fatalf("cand unexpected: %+v", cand)
	}
}

func TestExtractCandidateRejectsInvalidPayloads(t *testing.T) {
	for _, payload := range [][]byte{
		{},
		[]byte(""),
		[]byte("not a valid payload"),
		[]byte(`{"status":"error"}`),
		[]byte(`{"ip":"invalid_ip_format","country_code":"US"}`),
		[]byte(`{"ip":"198.51.100.22","country_code":"INVALID_3_CHARS"}`),
		[]byte("loc=US\n"),           // missing IP
		[]byte("ip=198.51.100.22\n"), // missing country
	} {
		_, err := identity.ExtractCandidate(payload)
		if err == nil {
			t.Errorf("expected error for payload %q, got nil", string(payload))
		}
	}
}

func TestHTTPSourceLiveMock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"198.51.100.22","country_code":"US","asn":13335}`))
	}))
	defer ts.Close()

	src := identity.NewHTTPSource("mock_http", ts.URL)
	if src.Name() != "mock_http" {
		t.Fatalf("name = %s, want mock_http", src.Name())
	}

	cand, err := src.Probe(context.Background(), ts.Client())
	if err != nil {
		t.Fatalf("src.Probe() error = %v", err)
	}
	if cand.NormalizedIP() != testIP1 || cand.NormalizedCountryCode() != "US" {
		t.Fatalf("unexpected cand: %+v", cand)
	}
}

func TestResolutionZeroIPLeakageInSerialization(t *testing.T) {
	resolver := identity.NewResolver()
	sources := []identity.Source{
		identity.NewStaticSource("s1", identity.Candidate{IP: testIP1, CountryCode: "US", ASN: 13335}),
		identity.NewStaticSource("s2", identity.Candidate{IP: testIP1, CountryCode: "US", ASN: 13335}),
	}

	result, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, testIP1) {
		t.Fatalf("SECURITY VIOLATION: Resolution JSON contains raw IP %s: %s", testIP1, jsonStr)
	}

	// Ensure Candidate itself does not serialize raw IP
	cand := identity.Candidate{IP: testIP1, CountryCode: "US", ASN: 13335}
	candData, _ := json.Marshal(cand)
	if strings.Contains(string(candData), testIP1) {
		t.Fatalf("SECURITY VIOLATION: Candidate JSON contains raw IP: %s", string(candData))
	}
}

func TestConcurrentResolverCallsAreRaceFreeAndDeduplicated(t *testing.T) {
	resolver := identity.NewResolver(identity.WithTTL(10 * time.Minute))

	callCount := 0
	var mu sync.Mutex
	sources := []identity.Source{
		&mockCountingSource{
			name: "s1",
			fn: func() (identity.Candidate, error) {
				mu.Lock()
				callCount++
				mu.Unlock()
				time.Sleep(10 * time.Millisecond) // simulate I/O
				return identity.Candidate{IP: testIP1, CountryCode: "DE", ASN: 24940}, nil
			},
		},
		&mockCountingSource{
			name: "s2",
			fn: func() (identity.Candidate, error) {
				time.Sleep(10 * time.Millisecond)
				return identity.Candidate{IP: testIP1, CountryCode: "DE", ASN: 24940}, nil
			},
		},
	}

	const concurrency = 30
	var wg sync.WaitGroup
	results := make([]identity.Resolution, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			res, err := resolver.Resolve(context.Background(), http.DefaultClient, testNodeID, sources)
			if err != nil {
				t.Errorf("goroutine %d Resolve() error = %v", idx, err)
			}
			results[idx] = res
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r.Status != identity.StatusVerified {
			t.Fatalf("result %d status = %s, want verified", i, r.Status)
		}
		if r.Identity == nil || r.Identity.CountryCode != "DE" {
			t.Fatalf("result %d bad identity: %+v", i, r.Identity)
		}
	}

	mu.Lock()
	count := callCount
	mu.Unlock()

	// Thanks to singleflight and caching, 30 concurrent calls should only trigger 1 network call
	if count != 1 {
		t.Fatalf("expected singleflight to trigger 1 network call, got: %d", count)
	}
}

type mockCountingSource struct {
	name string
	fn   func() (identity.Candidate, error)
}

func (m *mockCountingSource) Name() string {
	return m.name
}

func (m *mockCountingSource) Probe(context.Context, *http.Client) (identity.Candidate, error) {
	return m.fn()
}
