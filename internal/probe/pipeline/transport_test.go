package pipeline_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"clash-sub-parser/internal/probe/pipeline"
)

func TestMeasureTransport_Success204(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // simulate transport time
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := ts.Client()
	res, err := pipeline.MeasureTransport(context.Background(), client, []string{ts.URL}, 2*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !res.Available {
		t.Errorf("expected Available == true")
	}
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", res.StatusCode)
	}
	if res.Latency.RTTMs <= 0 {
		t.Errorf("expected positive RTTMs, got %d", res.Latency.RTTMs)
	}
	if res.TargetURL != ts.URL {
		t.Errorf("expected target URL %s, got %s", ts.URL, res.TargetURL)
	}
}

func TestMeasureTransport_FallbackToSecondURL(t *testing.T) {
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts2.Close()

	client := ts2.Client()
	// First URL is invalid, second URL is valid
	res, err := pipeline.MeasureTransport(context.Background(), client, []string{"http://127.0.0.1:54321/generate_204", ts2.URL}, 2*time.Second)
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}

	if !res.Available {
		t.Errorf("expected Available == true on fallback")
	}
	if res.TargetURL != ts2.URL {
		t.Errorf("expected target to be ts2.URL, got %s", res.TargetURL)
	}
}

func TestMeasureTransport_AllFail(t *testing.T) {
	client := http.DefaultClient
	res, err := pipeline.MeasureTransport(context.Background(), client, []string{"http://127.0.0.1:54321/generate_204"}, 500*time.Millisecond)
	if err == nil {
		t.Fatalf("expected error when all URLs fail, got nil")
	}
	if res.Available {
		t.Errorf("expected Available == false")
	}
}

func TestMeasureTransport_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := ts.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res, err := pipeline.MeasureTransport(ctx, client, []string{ts.URL}, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if res.Available {
		t.Errorf("expected Available == false")
	}
}
