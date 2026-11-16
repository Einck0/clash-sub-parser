package platform_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

func TestCheckOpenAIRoutesAllChecksThroughSuppliedTransport(t *testing.T) {
	seen := make(map[string]int)
	client := mockClient(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Errorf("request method = %q, want GET", req.Method)
		}

		switch {
		case req.URL.Scheme == "https" && req.URL.Host == "api.openai.com" && req.URL.Path == "/compliance/cookie_requirements":
			seen["web"]++
			if !strings.Contains(req.Header.Get("User-Agent"), "Chrome/122.0.0.0") {
				t.Errorf("web User-Agent = %q", req.Header.Get("User-Agent"))
			}
			return jsonResponse(http.StatusOK, `{}`)
		case req.URL.Scheme == "https" && req.URL.Host == "ios.chat.openai.com" && req.URL.Path == "":
			seen["ios"]++
			if !strings.Contains(req.Header.Get("User-Agent"), "ChatGPT/3.0") || req.Header.Get("X-Requested-With") != "com.openai.chatgpt" {
				t.Errorf("iOS gateway headers = %#v", req.Header)
			}
			return jsonResponse(http.StatusOK, `{}`)
		case req.URL.Scheme == "https" && req.URL.Host == "chat.openai.com" && req.URL.Path == "/cdn-cgi/trace":
			seen["trace"]++
			return htmlResponse(http.StatusOK, "loc=US\n")
		default:
			t.Errorf("unexpected outbound request: %s %s", req.Method, req.URL)
			return nil, errors.New("unexpected request")
		}
	})

	got := platform.CheckOpenAI(context.Background(), client)
	if got.Verdict != domain.VerdictUnknown || got.EvidenceScope != "network_entry" || got.SubTier != "" {
		t.Fatalf("endpoint responses overstated capability: %+v", got)
	}
	if got.Region != "US" || got.RegionSource != "cdn_trace" {
		t.Fatalf("trace evidence missing from result: %+v", got)
	}
	if !reflect.DeepEqual(seen, map[string]int{"web": 1, "ios": 1, "trace": 1}) {
		t.Fatalf("requests handled by supplied node transport = %v", seen)
	}
}

func TestCheckOpenAIOnlyReportsEndpointEvidence(t *testing.T) {
	bodies := []struct {
		name string
		body string
	}{
		{name: "empty"},
		{name: "ok", body: "OK"},
		{name: "empty json", body: `{}`},
		{name: "unrelated json", body: `{"status":"ok"}`},
		{name: "malformed json", body: `{"`},
		{name: "html login", body: `<!doctype html><html>Sign in</html>`},
	}
	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			client := mockClient(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "chat.openai.com" {
					return htmlResponse(http.StatusOK, "loc=US\n")
				}
				return jsonResponse(http.StatusOK, tc.body)
			})
			got := platform.CheckOpenAI(context.Background(), client)
			if got.Verdict != domain.VerdictUnknown || got.SubTier != "" || got.EvidenceScope != "network_entry" {
				t.Fatalf("endpoint response was overstated: %+v", got)
			}
			if got.Region != "US" || got.RegionSource != "cdn_trace" {
				t.Fatalf("trace region was not retained independently: %+v", got)
			}
		})
	}
}

func TestCheckOpenAINonContractStatusesRemainNonAvailable(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusFound, http.StatusUnauthorized, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := mockClient(func(*http.Request) (*http.Response, error) {
				return jsonResponse(status, `{}`)
			})
			got := platform.CheckOpenAI(context.Background(), client)
			if got.Verdict != domain.VerdictUnknown || got.SubTier != "" {
				t.Fatalf("HTTP %d was treated as capability evidence: %+v", status, got)
			}
		})
	}

	client := mockClient(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadGateway, "upstream failure")
	})
	got := platform.CheckOpenAI(context.Background(), client)
	if got.Verdict != domain.VerdictError || got.Reason != "upstream_error" {
		t.Fatalf("5xx should remain a failure: %+v", got)
	}
}

func TestCheckOpenAIRestrictionsAndPartialFailure(t *testing.T) {
	t.Run("rate limited is not a ban", func(t *testing.T) {
		client := mockClient(func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusTooManyRequests, "")
		})
		got := platform.CheckOpenAI(context.Background(), client)
		if got.Verdict != domain.VerdictRestricted || got.Reason != "rate_limited" || got.SubTier != "" {
			t.Fatalf("rate limit classification = %+v", got)
		}
	})

	t.Run("rate limit takes precedence over region marker", func(t *testing.T) {
		client := mockClient(func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusTooManyRequests, `{"error":"unsupported_country"}`)
		})
		got := platform.CheckOpenAI(context.Background(), client)
		if got.Verdict != domain.VerdictRestricted || got.Reason != "rate_limited" || got.SubTier != "" {
			t.Fatalf("rate limit classification = %+v", got)
		}
	})

	t.Run("explicit region marker only restricts endpoint access", func(t *testing.T) {
		client := mockClient(func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"error":"unsupported_country"}`)
		})
		got := platform.CheckOpenAI(context.Background(), client)
		if got.Verdict != domain.VerdictRestricted || got.Reason != "access_restricted" || got.SubTier != "" {
			t.Fatalf("explicit endpoint restriction classification = %+v", got)
		}
	})

	t.Run("restricted endpoint does not claim account ban", func(t *testing.T) {
		client := mockClient(func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusForbidden, "challenge")
		})
		got := platform.CheckOpenAI(context.Background(), client)
		if got.Verdict != domain.VerdictRestricted || got.SubTier != "" || got.Reason != "access_restricted" {
			t.Fatalf("endpoint restriction classification = %+v", got)
		}
	})

	t.Run("one endpoint response plus one transport failure remains unknown", func(t *testing.T) {
		client := mockClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "api.openai.com" {
				return jsonResponse(http.StatusOK, `{}`)
			}
			return nil, errors.New("dial failure")
		})
		got := platform.CheckOpenAI(context.Background(), client)
		if got.Verdict != domain.VerdictUnknown || got.Reason != "endpoint_reachable_capability_unverified" {
			t.Fatalf("partial endpoint evidence classification = %+v", got)
		}
	})

	t.Run("all endpoints fail with transport error", func(t *testing.T) {
		client := mockClient(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failure")
		})
		got := platform.CheckOpenAI(context.Background(), client)
		if got.Verdict != domain.VerdictError || got.Reason != "transport_error" {
			t.Fatalf("transport failure classification = %+v", got)
		}
	})
}

type failingOpenAIReader struct{}

func (failingOpenAIReader) Read([]byte) (int, error) { return 0, errors.New("body read failed") }
func (failingOpenAIReader) Close() error             { return nil }

func TestCheckOpenAIPreservesCancellationAndBodyReadFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelledClient := mockClient(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})
	cancelled := platform.CheckOpenAI(ctx, cancelledClient)
	if cancelled.Verdict != domain.VerdictSkipped || cancelled.Reason != "cancelled" {
		t.Fatalf("cancelled requests should be skipped, got %+v", cancelled)
	}

	rateLimitedClient := mockClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: failingOpenAIReader{}}, nil
	})
	rateLimited := platform.CheckOpenAI(context.Background(), rateLimitedClient)
	if rateLimited.Verdict != domain.VerdictRestricted || rateLimited.Reason != "rate_limited" {
		t.Fatalf("429 status should take precedence over body read failure, got %+v", rateLimited)
	}

	readFailureClient := mockClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingOpenAIReader{}}, nil
	})
	readFailure := platform.CheckOpenAI(context.Background(), readFailureClient)
	if readFailure.Verdict != domain.VerdictError || readFailure.Reason != "transport_error" {
		t.Fatalf("body read failure was not preserved, got %+v", readFailure)
	}
}

func TestCheckOpenAINilTransportIsNotCapabilityEvidence(t *testing.T) {
	got := platform.CheckOpenAI(context.Background(), nil)
	if got.Verdict != domain.VerdictError || got.Reason != "transport_error" || got.SubTier != "" {
		t.Fatalf("nil transport result = %+v", got)
	}
}
