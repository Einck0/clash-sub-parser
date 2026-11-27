package platform_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

// These additional C regressions do not change the frozen B comparison corpus.
func TestClaudeTraceEvidenceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, body, mitigated string
		readErr               error
		want                  domain.ProbeVerdict
		reason, region        string
	}{
		{name: "challenge_header", body: "loc=US\n", mitigated: "challenge", want: domain.VerdictRestricted, reason: "challenge_or_login"},
		{name: "header_does_not_hide_read_error", body: "loc=US\n", mitigated: "challenge", readErr: io.ErrUnexpectedEOF, want: domain.VerdictError, reason: "transport_error"},
		{name: "challenge_does_not_hide_cancel", body: "<html>challenge-platform</html>\nloc=US\n", readErr: context.Canceled, want: domain.VerdictSkipped, reason: "cancelled"},
		{name: "socket_timeout_after_loc", body: "loc=US\n", readErr: os.ErrDeadlineExceeded, want: domain.VerdictError, reason: "timeout"},
		{name: "challenge_text_in_ua_is_not_challenge", body: "uag=<html>cf-chl- /cdn-cgi/challenge-platform/ loc=CN</html>\nloc=US\n", want: domain.VerdictAvailable, reason: "claude_available", region: "US"},
		{name: "html_with_separate_loc_line", body: "<html>\nloc=US\n</html>", want: domain.VerdictError, reason: "trace_loc_missing"},
		{name: "unknown_before_valid", body: "loc=ZZ\nloc=US\n", want: domain.VerdictError, reason: "unknown_region_code"},
		{name: "unknown_after_valid", body: "loc=US\nloc=ZZ\n", want: domain.VerdictError, reason: "unknown_region_code"},
		{name: "unknown_before_malformed", body: "loc=ZZ\nloc=usa\n", want: domain.VerdictError, reason: "trace_loc_malformed"},
		{name: "conflict_before_malformed", body: "loc=US\nloc=CN\nloc=US-extra\n", want: domain.VerdictError, reason: "trace_loc_malformed"},
		{name: "trailing_whitespace_not_repaired", body: "loc=US \n", want: domain.VerdictError, reason: "trace_loc_malformed"},
		{name: "one_letter_country", body: "loc=U\n", want: domain.VerdictError, reason: "trace_loc_malformed"},
		{name: "embedded_cr_not_repaired", body: "loc=US\r\r\n", want: domain.VerdictError, reason: "trace_loc_malformed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := io.Reader(strings.NewReader(tc.body))
			if tc.readErr != nil {
				reader = io.MultiReader(reader, failingBody{err: tc.readErr})
			}
			body := &claudeCountedBody{Reader: reader}
			client := &http.Client{Transport: detectorTransport(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Cf-Mitigated": []string{tc.mitigated}}, Body: body, Request: req}, nil
			})}
			got := platform.CheckClaude(context.Background(), client)
			if got.Verdict != tc.want || got.Reason != tc.reason || got.Region != tc.region || body.closes != 1 {
				t.Fatalf("got %+v closes=%d; want %s/%s region=%q", got, body.closes, tc.want, tc.reason, tc.region)
			}
			if tc.region == "" && (got.RegionSource != "" || got.EvidenceScope != "" || got.SubTier != "") {
				t.Fatalf("unverified result carries positive evidence: %+v", got)
			}
		})
	}
}

func TestClaudeFinalOriginAndControlBoundaries(t *testing.T) {
	for _, target := range []string{
		"http://claude.ai/cdn-cgi/trace",
		"https://claude.ai:8443/cdn-cgi/trace",
		"https://claude.ai/cdn-cgi/trace-extra",
	} {
		t.Run(target, func(t *testing.T) {
			client := detectorClient(200, "loc=US\n", "", nil, nil)
			transport := client.Transport
			client.Transport = detectorTransport(func(req *http.Request) (*http.Response, error) {
				resp, err := transport.RoundTrip(req)
				final, parseErr := http.NewRequest(http.MethodGet, target, nil)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				resp.Request = final
				return resp, err
			})
			got := platform.CheckClaude(context.Background(), client)
			if got.Verdict != domain.VerdictRestricted || got.Reason != "unexpected_redirect" || got.Region != "" {
				t.Fatalf("non-trace response accepted: %+v", got)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		budget int64
		cancel bool
		reason string
	}{
		{"budget_before_request", 0, false, "budget_not_executed"},
		{"budget_during_body", 3, false, "budget_not_executed"},
		{"cancelled_before_read", 100, true, "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			budget := platform.NewBodyBudget(tc.budget)
			recorder := &platform.ResponseRecorder{}
			client := platform.BudgetClient(detectorClient(200, "loc=US\n", "", nil, nil), ctx, budget, recorder, cancel)
			got := platform.CheckClaude(ctx, client)
			_, used := budget.Snapshot()
			if got.Verdict != domain.VerdictSkipped || got.Reason != tc.reason || got.Region != "" || got.EvidenceScope != "" || used > tc.budget {
				t.Fatalf("control outcome lost: %+v used=%d", got, used)
			}
		})
	}
}
