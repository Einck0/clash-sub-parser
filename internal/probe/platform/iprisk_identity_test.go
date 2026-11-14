package platform_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

func TestCheckIPRiskRejectsUnsuccessfulTraceIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    io.ReadCloser
		verdict domain.ProbeVerdict
		reason  string
	}{
		{"unauthorized", 401, nil, domain.VerdictRestricted, "access_restricted"},
		{"forbidden", 403, nil, domain.VerdictRestricted, "access_restricted"},
		{"rate limited", 429, nil, domain.VerdictRestricted, "rate_limited"},
		{"rate limited unreadable body", 429, failingBody{err: io.ErrUnexpectedEOF}, domain.VerdictRestricted, "rate_limited"},
		{"server error with IP", 500, nil, domain.VerdictError, "unexpected_status"},
		{"server error unreadable body", 503, failingBody{err: io.ErrUnexpectedEOF}, domain.VerdictError, "unexpected_status"},
		{"not found with IP", 404, nil, domain.VerdictError, "unexpected_status"},
		{"partial trace", 206, nil, domain.VerdictError, "unexpected_status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := mockClient(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Host != "cloudflare.com" || calls != 1 {
					t.Fatalf("unexpected scoring request after unsuccessful trace: %s", req.URL)
				}
				resp, err := htmlResponse(tc.status, "ip=1.2.3.4\n")
				if tc.body != nil {
					resp.Body = tc.body
				}
				return resp, err
			})
			cap := platform.CheckIPRisk(context.Background(), client, "")
			if cap.Verdict != tc.verdict || cap.Reason != tc.reason || cap.RiskScore != "" || calls != 1 {
				t.Fatalf("calls=%d capability=%+v, want %s/%s with no score", calls, cap, tc.verdict, tc.reason)
			}
		})
	}
}

func TestCheckIPRiskRequiresValidIdentityBeforeScoring(t *testing.T) {
	for _, tc := range []struct {
		name string
		ip   string
	}{
		{"not an IP", "not-an-ip"},
		{"path changes identity", "1.2.3.4/../5.6.7.8"},
		{"query suffix", "1.2.3.4?other=5.6.7.8"},
		{"URL", "https://example.invalid"},
		{"out of range", "999.2.3.4"},
		{"IPv6 zone", "fe80::1%eth0"},
	} {
		for _, fromTrace := range []bool{false, true} {
			source := "supplied/"
			if fromTrace {
				source = "trace/"
			}
			t.Run(source+tc.name, func(t *testing.T) {
				calls := 0
				client := mockClient(func(req *http.Request) (*http.Response, error) {
					calls++
					if !fromTrace || req.URL.Host != "cloudflare.com" || calls != 1 {
						t.Fatalf("invalid identity must not be scored: %s", req.URL)
					}
					return htmlResponse(http.StatusOK, "ip="+tc.ip+"\n")
				})
				ip := tc.ip
				wantCalls := 0
				if fromTrace {
					ip = ""
					wantCalls = 1
				}
				cap := platform.CheckIPRisk(context.Background(), client, ip)
				if cap.Verdict != domain.VerdictError || cap.Reason != "invalid_exit_identity" || cap.RiskScore != "" || calls != wantCalls {
					t.Fatalf("calls=%d capability=%+v, want invalid identity with no score", calls, cap)
				}
			})
		}
	}
}

func TestCheckIPRiskScoresCanonicalTraceIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, traceIP, canonicalIP string
	}{
		{"IPv4", "1.2.3.4", "1.2.3.4"},
		{"IPv6", "2001:0db8:0000:0000:0000:0000:0000:0001", "2001:db8::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := mockClient(func(req *http.Request) (*http.Response, error) {
				calls++
				switch calls {
				case 1:
					if req.URL.Host != "cloudflare.com" {
						t.Fatalf("expected trace request, got %s", req.URL)
					}
					return htmlResponse(http.StatusOK, "colo=SJC\nip= "+tc.traceIP+" \r\n")
				case 2:
					if req.URL.Host != "scamalytics.com" || req.URL.Path != "/ip/"+tc.canonicalIP || req.URL.RawQuery != "" {
						t.Fatalf("wrong scoring identity: %s", req.URL)
					}
					return htmlResponse(http.StatusOK, "IP Fraud Risk API\n1\n2\n3\n\"score\": \"12\",\n\"risk\": \"low\"\n6")
				default:
					t.Fatalf("unexpected request: %s", req.URL)
					return nil, nil
				}
			})
			cap := platform.CheckIPRisk(context.Background(), client, "")
			if calls != 2 || cap.Verdict != domain.VerdictAvailable || cap.Reason != "scamalytics_scored" || cap.RiskScore != "12% low" {
				t.Fatalf("calls=%d capability=%+v, want score for canonical identity", calls, cap)
			}
		})
	}
}

func TestCheckIPRiskTraceTransportAndBodyFailuresRemainInconclusive(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		bodyErr error
		verdict domain.ProbeVerdict
		reason  string
	}{
		{"transport", io.ErrUnexpectedEOF, nil, domain.VerdictError, "transport_error"},
		{"deadline", context.DeadlineExceeded, nil, domain.VerdictError, "deadline_exceeded"},
		{"body failure", nil, io.ErrUnexpectedEOF, domain.VerdictError, "transport_error"},
		{"body budget", nil, platform.ErrBodyBudget, domain.VerdictSkipped, "budget_not_executed"},
		{"cancelled", context.Canceled, nil, domain.VerdictSkipped, "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := mockClient(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls != 1 || req.URL.Host != "cloudflare.com" {
					t.Fatalf("failed trace must not be scored: %s", req.URL)
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(io.MultiReader(
						strings.NewReader("ip=1.2.3.4\n"), failingBody{err: tc.bodyErr},
					)),
				}, nil
			})
			cap := platform.CheckIPRisk(context.Background(), client, "")
			if calls != 1 || cap.Verdict != tc.verdict || cap.Reason != tc.reason || cap.RiskScore != "" {
				t.Fatalf("calls=%d capability=%+v, want %s/%s with no score", calls, cap, tc.verdict, tc.reason)
			}
		})
	}
}
