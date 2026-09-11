package pipeline_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/pipeline"
)

func TestResolveGeoIP_IPsbJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"ip": "203.0.113.42",
			"country_code": "JP",
			"country": "Japan",
			"city": "Tokyo",
			"asn": 2497,
			"organization": "Internet Initiative Japan"
		}`))
	}))
	defer ts.Close()

	client := ts.Client()
	res, err := pipeline.ResolveGeoIP(context.Background(), client, []string{ts.URL}, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.IP != "203.0.113.42" {
		t.Errorf("expected IP 203.0.113.42, got %s", res.IP)
	}
	if res.Country != "JP" {
		t.Errorf("expected country JP, got %s", res.Country)
	}
	if res.City != "Tokyo" {
		t.Errorf("expected city Tokyo, got %s", res.City)
	}
	if res.ASN == nil || *res.ASN != 2497 {
		t.Errorf("expected ASN 2497, got %v", res.ASN)
	}
	if res.Organization != "Internet Initiative Japan" {
		t.Errorf("expected organization Internet Initiative Japan, got %s", res.Organization)
	}
}

func TestResolveGeoIP_CloudflareTrace(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fl=123\r\nh=cloudflare.com\r\nip=198.51.100.25\r\nts=1600000000\r\nvisit_scheme=https\r\nuag=test\r\ncolo=NRT\r\nloc=JP\r\n"))
	}))
	defer ts.Close()

	client := ts.Client()
	res, err := pipeline.ResolveGeoIP(context.Background(), client, []string{ts.URL}, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.IP != "198.51.100.25" {
		t.Errorf("expected IP 198.51.100.25, got %s", res.IP)
	}
	if res.Country != "JP" {
		t.Errorf("expected country JP, got %s", res.Country)
	}
}

func TestResolveGeoIP_ConsensusVerification(t *testing.T) {
	// Two providers agreeing on IP and Country
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip": "203.0.113.88", "country_code": "SG", "asn": 13335, "organization": "Cloudflare"}`))
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ip=203.0.113.88\r\nloc=SG\r\n"))
	}))
	defer ts2.Close()

	client := ts1.Client()
	res, err := pipeline.ResolveGeoIP(context.Background(), client, []string{ts1.URL, ts2.URL}, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.IP != "203.0.113.88" {
		t.Errorf("expected IP 203.0.113.88, got %s", res.IP)
	}
	if res.Country != "SG" {
		t.Errorf("expected country SG, got %s", res.Country)
	}
	if res.Confidence != "verified" {
		t.Errorf("expected verified confidence on agreement, got %s", res.Confidence)
	}
}
