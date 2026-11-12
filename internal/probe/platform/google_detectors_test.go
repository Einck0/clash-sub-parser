package platform_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

type detectorTransport func(*http.Request) (*http.Response, error)

func (f detectorTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingBody struct{ err error }

func (r failingBody) Read([]byte) (int, error) { return 0, r.err }
func (r failingBody) Close() error             { return nil }

func detectorClient(status int, body string, finalURL string, err error, bodyReader io.ReadCloser) *http.Client {
	return &http.Client{Transport: detectorTransport(func(r *http.Request) (*http.Response, error) {
		if err != nil {
			return nil, err
		}
		responseRequest := r.WithContext(r.Context())
		if finalURL != "" {
			copyURL := *r.URL
			copyURL.Host = finalURL
			responseRequest.URL = &copyURL
		}
		reader := io.NopCloser(strings.NewReader(body))
		if bodyReader != nil {
			reader = bodyReader
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: reader, Request: responseRequest}, nil
	})}
}

func TestCheckGeminiClassifiesOnlyVerifiedSupportEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       domain.ProbeVerdict
		reason     string
	}{
		{"supported country", `prefix,2,1,200,"USA"suffix`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"Japan", `,2,1,200,"JPN"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"Singapore", `,2,1,200,"SGP"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"Hong Kong", `,2,1,200,"HKG"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"Macau", `,2,1,200,"MAC"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"Taiwan", `,2,1,200,"TWN"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"Germany", `,2,1,200,"DEU"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"Australia", `,2,1,200,"AUS"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"Mainland China Workspace-only", `,2,1,200,"CHN"`, domain.VerdictAbnormal, "support_unverified"},
		{"unknown code", `,2,1,200,"ZZZ"`, domain.VerdictError, "unknown_region_code"},
		{"not in supported table", `,2,1,200,"RUS"`, domain.VerdictAbnormal, "support_unverified"},
		{"missing", `<html>other</html>`, domain.VerdictError, "gemini_marker_missing"},
		{"valid and malformed short code", `,2,1,200,"USA" ,2,1,200,"ZZ"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"valid and lowercase code", `,2,1,200,"USA" ,2,1,200,"usa"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"valid and unclosed marker", `,2,1,200,"USA" ,2,1,200,"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"unclosed three-letter marker only", `,2,1,200,"USA`, domain.VerdictError, "gemini_marker_missing"},
		{"valid and unclosed three-letter marker", `,2,1,200,"USA" ,2,1,200,"CAN`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"observed null-prefixed format", `[null,2,1,200,"USA"]`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"duplicate region", `,2,1,200,"USA" ,2,1,200,"USA"`, domain.VerdictAvailable, "gemini_web_reachable"},
		{"ambiguous", `,2,1,200,"USA" ,2,1,200,"CAN"`, domain.VerdictError, "conflicting_region_markers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := platform.CheckGemini(context.Background(), detectorClient(http.StatusOK, tc.body, "", nil, nil))
			if got.Verdict != tc.want || got.Reason != tc.reason {
				t.Fatalf("got verdict=%s reason=%s; want %s/%s", got.Verdict, got.Reason, tc.want, tc.reason)
			}
			if tc.want == domain.VerdictAvailable && strings.Contains(strings.ToLower(got.Summary), "account eligible") {
				t.Fatal("network marker was overstated as account eligibility")
			}
		})
	}
	for _, tc := range []struct {
		name    string
		body    string
		verdict domain.ProbeVerdict
		reason  string
	}{
		{"foreign origin", `,2,1,200,"USA"`, domain.VerdictRestricted, "unexpected_redirect"},
		{"login redirect", `,2,1,200,"USA"`, domain.VerdictRestricted, "unexpected_redirect"},
		{"oversized", strings.Repeat("x", 2<<20+1), domain.VerdictError, "response_body_too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := detectorClient(http.StatusOK, tc.body, "", nil, nil)
			if tc.name == "foreign origin" {
				client = detectorClient(http.StatusOK, tc.body, "accounts.google.com", nil, nil)
			}
			if tc.name == "login redirect" {
				client.Transport = detectorTransport(func(r *http.Request) (*http.Response, error) {
					u := *r.URL
					u.Path = "/login"
					request := r.WithContext(r.Context())
					request.URL = &u
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: request}, nil
				})
			}
			got := platform.CheckGemini(context.Background(), client)
			if got.Verdict != tc.verdict || got.Reason != tc.reason {
				t.Fatalf("unexpected classification: got %s/%s, want %s/%s", got.Verdict, got.Reason, tc.verdict, tc.reason)
			}
		})
	}
}

func TestGoogleDetectorFailureBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
		want   domain.ProbeVerdict
		reason string
	}{
		{"forbidden", 403, "", nil, domain.VerdictRestricted, "access_restricted"},
		{"throttled", 429, "", nil, domain.VerdictRestricted, "rate_limited"},
		{"transport", 0, "", errors.New("private transport detail"), domain.VerdictError, "transport_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := detectorClient(tc.status, tc.body, "", tc.err, nil)
			got := platform.CheckGemini(context.Background(), client)
			if got.Verdict != tc.want || got.Reason != tc.reason || strings.Contains(got.Summary+got.Reason, "private") {
				t.Fatalf("unexpected safe classification: %+v", got)
			}
		})
	}
	for _, tc := range []struct {
		name    string
		status  int
		verdict domain.ProbeVerdict
		reason  string
	}{
		{"denied", 403, domain.VerdictRestricted, "access_restricted"},
		{"legal restriction", 451, domain.VerdictRestricted, "access_restricted"},
		{"throttled", 429, domain.VerdictRestricted, "rate_limited"},
		{"server error", 503, domain.VerdictError, "unexpected_status"},
	} {
		t.Run("antigravity status "+tc.name, func(t *testing.T) {
			got := platform.CheckAntigravity(context.Background(), detectorClient(tc.status, "", "", nil, nil))
			if got.Verdict != tc.verdict || got.Reason != tc.reason {
				t.Fatalf("unexpected classification: %+v", got)
			}
		})
	}
	got := platform.CheckAntigravity(context.Background(), detectorClient(200, "ordinary homepage", "", nil, nil))
	if got.Verdict != domain.VerdictError || got.Reason != "product_marker_missing" {
		t.Fatalf("generic 200 produced positive result: %+v", got)
	}
	got = platform.CheckAntigravity(context.Background(), detectorClient(200, "Antigravity agentic development", "", nil, nil))
	if got.Verdict != domain.VerdictAvailable || got.SubTier != "reachable" || got.Reason != "antigravity_entry_reachable" || got.EvidenceScope != "network_entry" {
		t.Fatalf("unexpected product entry result: %+v", got)
	}
	for _, tc := range []struct {
		name string
		host string
		want string
	}{
		{"foreign origin", "accounts.google.com", "unexpected_redirect"},
		{"login path", "/login", "unexpected_redirect"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := detectorClient(200, "Antigravity agentic development", "", nil, nil)
			if tc.host != "" && tc.host != "/login" {
				client = detectorClient(200, "Antigravity agentic development", tc.host, nil, nil)
			}
			if tc.host == "/login" {
				client.Transport = detectorTransport(func(r *http.Request) (*http.Response, error) {
					u := *r.URL
					u.Path = "/login"
					request := r.WithContext(r.Context())
					request.URL = &u
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("Antigravity agentic")), Request: request}, nil
				})
			}
			got := platform.CheckAntigravity(context.Background(), client)
			if got.Reason != tc.want || got.Verdict != domain.VerdictRestricted {
				t.Fatalf("redirect classification: %+v", got)
			}
		})
	}
}

func TestGoogleDetectorsReadFailuresControlMappings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		verdict domain.ProbeVerdict
		reason  string
	}{
		{"budget", platform.ErrBodyBudget, domain.VerdictSkipped, "budget_not_executed"},
		{"cancelled", context.Canceled, domain.VerdictSkipped, "cancelled"},
		{"deadline", context.DeadlineExceeded, domain.VerdictError, "deadline_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, check := range []struct {
				name string
				call func() domain.PlatformCapability
			}{
				{"Gemini", func() domain.PlatformCapability {
					return platform.CheckGemini(context.Background(), detectorClient(200, "", "", nil, failingBody{tc.err}))
				}},
				{"Antigravity", func() domain.PlatformCapability {
					return platform.CheckAntigravity(context.Background(), detectorClient(200, "", "", nil, failingBody{tc.err}))
				}},
			} {
				t.Run(check.name, func(t *testing.T) {
					got := check.call()
					if got.Verdict != tc.verdict || got.Reason != tc.reason {
						t.Fatalf("read error classified incorrectly: got %s/%s, want %s/%s", got.Verdict, got.Reason, tc.verdict, tc.reason)
					}
				})
			}
		})
	}
}

func TestGoogleDetectorsRejectChallengeAndLoginGates(t *testing.T) {
	for _, body := range []string{
		`<html>Gemini,2,1,200,"USA" <div class="g-recaptcha">`,
		`<html>Antigravity agentic development<script src="/recaptcha/api.js"></script>`,
		`<form id="gaia_loginform">Antigravity Gemini agentic ,2,1,200,"USA"`,
	} {
		for _, check := range []struct {
			name string
			call func() domain.PlatformCapability
		}{
			{"Gemini", func() domain.PlatformCapability {
				return platform.CheckGemini(context.Background(), detectorClient(200, body, "", nil, nil))
			}},
			{"Antigravity", func() domain.PlatformCapability {
				return platform.CheckAntigravity(context.Background(), detectorClient(200, body, "", nil, nil))
			}},
		} {
			got := check.call()
			if got.Verdict != domain.VerdictRestricted || got.Reason != "challenge_or_login" {
				t.Fatalf("%s challenge page was not rejected: %+v", check.name, got)
			}
		}
	}
}

func TestGoogleDetectorsBoundResponseBodiesAndHonorCancellation(t *testing.T) {
	large := strings.Repeat("x", 256<<10+1)
	got := platform.CheckAntigravity(context.Background(), detectorClient(200, large, "", nil, nil))
	if got.Reason != "response_body_truncated" || got.Verdict != domain.VerdictError {
		t.Fatalf("oversized body produced unexpected result: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = platform.CheckAntigravity(ctx, detectorClient(200, "", "", context.Canceled, nil))
	if got.Reason != "cancelled" || got.Verdict != domain.VerdictSkipped {
		t.Fatalf("cancellation was not represented safely: %+v", got)
	}
}

func TestGeminiLateMarkerAndOverflow(t *testing.T) {
	// 1. >256KiB late valid marker (simulate realistic ~870KB body with marker around offset 770000)
	pad770k := strings.Repeat(" ", 770280)
	pad100k := strings.Repeat(" ", 100000)
	lateValidBody := pad770k + `,2,1,200,"USA"` + pad100k
	gotValid := platform.CheckGemini(context.Background(), detectorClient(http.StatusOK, lateValidBody, "", nil, nil))
	if gotValid.Verdict != domain.VerdictAvailable || gotValid.Region != "US" || gotValid.Reason != "gemini_web_reachable" || gotValid.EvidenceScope != "web_region" || gotValid.SubTier != "web_reachable" {
		t.Fatalf("late valid marker failed: %+v", gotValid)
	}

	// 2. Conflicting late marker (>256KiB): early USA, late CAN
	conflictingBody := `,2,1,200,"USA"` + pad770k + `,2,1,200,"CAN"`
	gotConflict := platform.CheckGemini(context.Background(), detectorClient(http.StatusOK, conflictingBody, "", nil, nil))
	if gotConflict.Verdict != domain.VerdictError || gotConflict.Reason != "conflicting_region_markers" {
		t.Fatalf("conflicting late marker failed: %+v", gotConflict)
	}

	// 3. 2MiB overflow: body exceeds 2MiB limit
	overflowBody := strings.Repeat("x", (2<<20)+1)
	gotOverflow := platform.CheckGemini(context.Background(), detectorClient(http.StatusOK, overflowBody, "", nil, nil))
	if gotOverflow.Verdict != domain.VerdictError || gotOverflow.Reason != "response_body_too_large" {
		t.Fatalf("2MiB overflow failed: %+v", gotOverflow)
	}
}

func TestAntigravityNetworkEntryScope(t *testing.T) {
	body := "<html><head><title>Google Antigravity</title></head><body>Antigravity agentic coding platform</body></html>"
	got := platform.CheckAntigravity(context.Background(), detectorClient(http.StatusOK, body, "", nil, nil))
	if got.Verdict != domain.VerdictAvailable {
		t.Fatalf("expected VerdictAvailable, got %v", got.Verdict)
	}
	if got.SubTier != "reachable" {
		t.Fatalf("expected sub_tier reachable, got %s", got.SubTier)
	}
	if got.EvidenceScope != "network_entry" {
		t.Fatalf("expected evidence_scope network_entry, got %s", got.EvidenceScope)
	}
	if got.Reason != "antigravity_entry_reachable" {
		t.Fatalf("expected reason antigravity_entry_reachable, got %s", got.Reason)
	}
	if strings.Contains(strings.ToLower(got.Summary), "account eligible") {
		t.Fatal("summary must not claim account eligibility")
	}
}
