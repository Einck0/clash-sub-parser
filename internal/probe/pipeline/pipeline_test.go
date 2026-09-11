package pipeline_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/dialer"
	"clash-sub-parser/internal/probe/pipeline"
	"github.com/sagernet/sing-box/adapter"
)

type pipelineMockDialer struct {
	serverURL string
	dialCount atomic.Int64
	closed    atomic.Bool
	failDial  bool
}

func (p *pipelineMockDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	p.dialCount.Add(1)
	if p.failDial {
		return nil, fmt.Errorf("connection refused to %s", addr)
	}
	d := &net.Dialer{}
	return d.DialContext(ctx, network, addr)
}

func (p *pipelineMockDialer) Dial(network, addr string) (net.Conn, error) {
	return p.DialContext(context.Background(), network, addr)
}

func (p *pipelineMockDialer) ListenPacket(ctx context.Context, destination string) (net.PacketConn, error) {
	return nil, nil
}

func (p *pipelineMockDialer) Outbound() adapter.Outbound {
	return nil
}

func (p *pipelineMockDialer) HTTPClient(opts dialer.HTTPClientOptions) *http.Client {
	return pipeline.NewIsolatedHTTPClient(p, opts.Timeout)
}

func (p *pipelineMockDialer) Close() error {
	p.closed.Store(true)
	return nil
}

func TestPipeline_HappyPath(t *testing.T) {
	// Mock server handling 204, GeoIP, and Streaming
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/generate_204":
			w.WriteHeader(http.StatusNoContent)
		case "/geoip":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ip": "1.2.3.4", "country_code": "JP", "asn": 1234, "organization": "Test ISP"}`))
		case "/premium":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"countryCode": "JP"}`))
		case "/title/70143836":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`Breaking Bad title-70143836 watch-button`))
		case "/title/80018499":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`Stranger Things title-80018499 Netflix Original`))
		case "/disney":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`Disney+ stream`))
		case "/chatgpt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status": "normal"}`))
		case "/gemini":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`Google Gemini,2,1,200,"JPN"`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	d := &pipelineMockDialer{serverURL: ts.URL}
	pipe := pipeline.New(pipeline.Config{
		Timeout:          5 * time.Second,
		StageTimeout:     2 * time.Second,
		Transport204URLs: []string{ts.URL + "/generate_204"},
		IPEndpoints:      []string{ts.URL + "/geoip"},
		CheckStreaming:   true,
		CheckAI:          true,
		YouTubeURL:       ts.URL + "/premium",
		NetflixBaseURL:   ts.URL,
		DisneyURL:        ts.URL + "/disney",
		ChatGPTURL:       ts.URL + "/chatgpt",
		GeminiURL:        ts.URL + "/gemini",
	})

	node := &domain.Node{
		ID:        10,
		LogicalID: "test-node-10",
		Name:      "Test-Node",
		Server:    "1.2.3.4",
		Port:      443,
		Protocol:  domain.ProtocolVLESS,
	}

	res, err := pipe.Execute(context.Background(), node, d)
	if err != nil {
		t.Fatalf("unexpected pipeline execution error: %v", err)
	}

	if res.Status != domain.ProbeStatusOK {
		t.Errorf("expected status online, got %s", res.Status)
	}
	if res.LatencyMs == nil || *res.LatencyMs <= 0 {
		t.Errorf("expected valid latency, got %v", res.LatencyMs)
	}
	if res.IP != "1.2.3.4" {
		t.Errorf("expected IP 1.2.3.4, got %s", res.IP)
	}
	if res.Country != "JP" {
		t.Errorf("expected country JP, got %s", res.Country)
	}
	if res.ASN == nil || *res.ASN != 1234 {
		t.Errorf("expected ASN 1234, got %v", res.ASN)
	}
	if res.Organization != "Test ISP" {
		t.Errorf("expected organization Test ISP, got %s", res.Organization)
	}
}

func TestPipeline_CircuitBreaker_TransportFailure(t *testing.T) {
	// When 204 fails, pipeline MUST immediately circuit-break and NOT make Geo or Media requests!
	var geoCalled atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/generate_204":
			w.WriteHeader(http.StatusInternalServerError)
		case "/geoip":
			geoCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	d := &pipelineMockDialer{serverURL: ts.URL}
	pipe := pipeline.New(pipeline.Config{
		Timeout:          2 * time.Second,
		StageTimeout:     1 * time.Second,
		Transport204URLs: []string{ts.URL + "/generate_204"},
		IPEndpoints:      []string{ts.URL + "/geoip"},
		CheckStreaming:   true,
		CheckAI:          true,
	})

	node := &domain.Node{
		ID:        11,
		LogicalID: "circuit-breaker-node",
		Name:      "CB-Node",
		Server:    "1.2.3.4",
		Port:      443,
		Protocol:  domain.ProtocolVMess,
	}

	res, err := pipe.Execute(context.Background(), node, d)
	if err != nil {
		t.Fatalf("pipeline returned error instead of result struct: %v", err)
	}

	if res.Status != domain.ProbeStatusFailed {
		t.Errorf("expected status offline/fail, got %s", res.Status)
	}
	if geoCalled.Load() {
		t.Errorf("circuit breaker failed: GeoIP was called after 204 transport failure!")
	}
}

func TestPipeline_FalsePositive_HostNetworkDown(t *testing.T) {
	d := &pipelineMockDialer{failDial: true}
	// Baseline verifier reports host network is DOWN
	baseline := &mockBaselineVerifier{available: false}

	pipe := pipeline.New(pipeline.Config{
		Timeout:          2 * time.Second,
		StageTimeout:     1 * time.Second,
		Transport204URLs: []string{"http://127.0.0.1:59999/generate_204"},
		BaselineVerifier: baseline,
	})

	node := &domain.Node{
		ID:        12,
		LogicalID: "false-positive-node",
		Name:      "FP-Node",
		Server:    "1.2.3.4",
		Port:      443,
		Protocol:  domain.ProtocolShadowsocks,
	}

	res, err := pipe.Execute(context.Background(), node, d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != domain.ProbeStatusError {
		t.Errorf("expected status error for false-positive protection, got %s", res.Status)
	}
	if res.Error == "" {
		t.Errorf("expected error message explaining host network unreachable")
	}
}
