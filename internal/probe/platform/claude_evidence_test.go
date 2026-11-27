package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

// These are controlled response fixtures, not observations of provider or node
// accuracy. Keep their inputs unchanged across the measurement and repair commits.
// The 256 KiB boundary is the existing generic detector bound, not Gemini's 2 MiB.
const claudeCorpusBoundary = 256 << 10

type claudeCorpusCase struct {
	name                   string
	body                   string
	status                 int
	finalURL               string
	readErr                error
	wantVerdict            domain.ProbeVerdict
	wantReason, wantRegion string
}

func claudeControlledCorpus() []claudeCorpusCase {
	trace := "fl=fixture\nh=claude.ai\nip=192.0.2.1\nloc=US\ntls=TLSv1.3\n"
	// Padding belongs to a different trace field. A full-body parser must inspect
	// the late loc field, and must not mistake a value containing loc= for a key.
	late := "uag=" + strings.Repeat("x", claudeCorpusBoundary-len("uag=\nloc=US\n")) + "\nloc=US\n"
	lateConflict := "loc=CN\nuag=" + strings.Repeat("x", claudeCorpusBoundary-len("loc=CN\nuag=\nloc=US\n")) + "\nloc=US\n"
	oversized := trace + "uag=" + strings.Repeat("x", (1<<20)-len(trace)-len("uag=\n")) + "\n"
	return []claudeCorpusCase{
		{name: "valid_us", body: trace, wantVerdict: domain.VerdictAvailable, wantReason: "claude_available", wantRegion: "US"},
		{name: "blocked_cn", body: "loc=CN\nip=192.0.2.1\n", wantVerdict: domain.VerdictRestricted, wantReason: "sanctioned_region", wantRegion: "CN"},
		{name: "valid_crlf", body: "loc=JP\r\nip=192.0.2.1\r\n", wantVerdict: domain.VerdictAvailable, wantReason: "claude_available", wantRegion: "JP"},
		{name: "valid_no_terminal_newline", body: "ip=192.0.2.1\nloc=US", wantVerdict: domain.VerdictAvailable, wantReason: "claude_available", wantRegion: "US"},
		{name: "duplicate_same_country", body: "loc=US\nloc=US\n", wantVerdict: domain.VerdictAvailable, wantReason: "claude_available", wantRegion: "US"},
		{name: "valid_late_at_boundary", body: late, wantVerdict: domain.VerdictAvailable, wantReason: "claude_available", wantRegion: "US"},
		{name: "embedded_value_before_blocked", body: "uag=Agent/loc=US\nloc=CN\n", wantVerdict: domain.VerdictRestricted, wantReason: "sanctioned_region", wantRegion: "CN"},
		{name: "lookalike_key_only", body: "xloc=US\n", wantVerdict: domain.VerdictError, wantReason: "trace_loc_missing"},
		{name: "html_substring_only", body: "<html><body>loc=US</body></html>", wantVerdict: domain.VerdictError, wantReason: "trace_loc_missing"},
		{name: "missing_loc", body: "ip=192.0.2.1\n", wantVerdict: domain.VerdictError, wantReason: "trace_loc_missing"},
		{name: "malformed_long", body: "loc=USA\n", wantVerdict: domain.VerdictError, wantReason: "trace_loc_malformed"},
		{name: "malformed_lowercase", body: "loc=us\n", wantVerdict: domain.VerdictError, wantReason: "trace_loc_malformed"},
		{name: "malformed_empty", body: "loc=\n", wantVerdict: domain.VerdictError, wantReason: "trace_loc_malformed"},
		{name: "malformed_after_valid", body: "loc=US\nloc=USA\n", wantVerdict: domain.VerdictError, wantReason: "trace_loc_malformed"},
		{name: "malformed_before_valid", body: "loc=USA\nloc=US\n", wantVerdict: domain.VerdictError, wantReason: "trace_loc_malformed"},
		{name: "unknown_country", body: "loc=ZZ\n", wantVerdict: domain.VerdictError, wantReason: "unknown_region_code"},
		{name: "conflict_allowed_first", body: "loc=US\nloc=CN\n", wantVerdict: domain.VerdictError, wantReason: "conflicting_region_markers"},
		{name: "conflict_blocked_first", body: "loc=CN\nloc=US\n", wantVerdict: domain.VerdictError, wantReason: "conflicting_region_markers"},
		{name: "late_conflict_at_boundary", body: lateConflict, wantVerdict: domain.VerdictError, wantReason: "conflicting_region_markers"},
		{name: "challenge_with_loc", body: "loc=US\n<html><title>Just a moment</title><script src=\"/cdn-cgi/challenge-platform/\"></script></html>", wantVerdict: domain.VerdictRestricted, wantReason: "challenge_or_login"},
		{name: "login_redirect_with_loc", body: trace, finalURL: "https://claude.ai/login", wantVerdict: domain.VerdictRestricted, wantReason: "unexpected_redirect"},
		{name: "foreign_redirect_with_loc", body: trace, finalURL: "https://example.invalid/cdn-cgi/trace", wantVerdict: domain.VerdictRestricted, wantReason: "unexpected_redirect"},
		{name: "overflow_one_byte", body: late + "x", wantVerdict: domain.VerdictError, wantReason: "response_body_too_large"},
		{name: "oversized_one_mib", body: oversized, wantVerdict: domain.VerdictError, wantReason: "response_body_too_large"},
		{name: "forbidden", status: http.StatusForbidden, body: trace, wantVerdict: domain.VerdictRestricted, wantReason: "access_restricted"},
		{name: "legal_restriction", status: http.StatusUnavailableForLegalReasons, body: trace, wantVerdict: domain.VerdictRestricted, wantReason: "access_restricted"},
		{name: "rate_limited", status: http.StatusTooManyRequests, body: trace, wantVerdict: domain.VerdictRestricted, wantReason: "rate_limited"},
		{name: "upstream_error", status: http.StatusServiceUnavailable, body: trace, wantVerdict: domain.VerdictError, wantReason: "unexpected_status"},
		{name: "read_failure_after_valid", body: trace, readErr: errors.New("fixture read failure"), wantVerdict: domain.VerdictError, wantReason: "transport_error"},
		{name: "deadline_after_valid", body: trace, readErr: context.DeadlineExceeded, wantVerdict: domain.VerdictError, wantReason: "deadline_exceeded"},
		{name: "cancelled_after_valid", body: trace, readErr: context.Canceled, wantVerdict: domain.VerdictSkipped, wantReason: "cancelled"},
	}
}

type claudeCountedBody struct {
	io.Reader
	bytes, reads, closes int
	eof                  bool
}

func (b *claudeCountedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.bytes += n
	b.reads++
	b.eof = b.eof || errors.Is(err, io.EOF)
	return n, err
}

func (b *claudeCountedBody) Close() error { b.closes++; return nil }

// Revision B intentionally logs classifications without asserting the old
// algorithm's answers. The oracle describes synthetic input semantics; a green
// measurement run does NOT mean the detector satisfies it. Revision C adds the
// semantic assertions against the same inputs after preserving B's cloud output.
func TestClaudeControlledResponseCorpus(t *testing.T) {
	for _, tc := range claudeControlledCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			reader := io.Reader(strings.NewReader(tc.body))
			if tc.readErr != nil {
				reader = io.MultiReader(reader, failingBody{err: tc.readErr})
			}
			body := &claudeCountedBody{Reader: reader}
			requests := 0
			base := &http.Client{Transport: detectorTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.Method != http.MethodGet || req.URL.String() != "https://claude.ai/cdn-cgi/trace" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				responseRequest := req.Clone(req.Context())
				if tc.finalURL != "" {
					u, err := url.Parse(tc.finalURL)
					if err != nil {
						t.Fatal(err)
					}
					responseRequest.URL = u
				}
				status := tc.status
				if status == 0 {
					status = http.StatusOK
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: body, Request: responseRequest}, nil
			})}
			budget := platform.NewBodyBudget(8 << 20)
			recorder := &platform.ResponseRecorder{}
			client := platform.BudgetClient(base, context.Background(), budget, recorder, nil)
			got := platform.CheckClaude(context.Background(), client)
			rows, _ := recorder.Snapshot()
			_, used := budget.Snapshot()
			if requests != 1 || body.closes != 1 || len(rows) != 1 || used != int64(body.bytes) || rows[0].Bytes != used {
				t.Fatalf("measurement/cleanup mismatch: requests=%d closes=%d rows=%+v used=%d read=%d", requests, body.closes, rows, used, body.bytes)
			}
			observation := map[string]any{
				"case": tc.name, "payload_bytes": len(tc.body), "requests": requests,
				"body_bytes": body.bytes, "read_calls": body.reads, "body_eof": body.eof, "body_closes": body.closes,
				"verdict": got.Verdict, "reason": got.Reason, "region": got.Region, "evidence_scope": got.EvidenceScope,
				"want_verdict": tc.wantVerdict, "want_reason": tc.wantReason, "want_region": tc.wantRegion,
				"matches_oracle": got.Verdict == tc.wantVerdict && got.Reason == tc.wantReason && got.Region == tc.wantRegion,
			}
			encoded, err := json.Marshal(observation)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("CLAUDE_CORPUS %s", encoded)
		})
	}
}

// Sequential ordering isolates the effect of one malformed response on the
// shared production BodyBudget. It is not a model of runner scheduling/throughput.
func TestClaudeSharedBudgetControlledCost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	budget := platform.NewBodyBudget(1 << 20)
	recorder := &platform.ResponseRecorder{}
	large := "loc=US\nuag=" + strings.Repeat("x", 4<<20)
	gemini := `,2,1,200,"USA"`
	claudeBody := &claudeCountedBody{Reader: strings.NewReader(large)}
	geminiBody := &claudeCountedBody{Reader: strings.NewReader(gemini)}
	requests := 0
	base := &http.Client{Transport: detectorTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		var body io.ReadCloser
		switch req.URL.Hostname() {
		case "claude.ai":
			body = claudeBody
		case "gemini.google.com":
			body = geminiBody
		default:
			t.Fatalf("unexpected fixture host: %s", req.URL.Hostname())
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
	})}
	client := platform.BudgetClient(base, ctx, budget, recorder, cancel)
	first := platform.CheckClaude(ctx, client)
	second := platform.CheckGemini(ctx, client)
	rows, _ := recorder.Snapshot()
	limit, used := budget.Snapshot()
	if used != int64(claudeBody.bytes+geminiBody.bytes) || used > limit || claudeBody.closes != 1 || (geminiBody.reads > 0 && geminiBody.closes != 1) {
		t.Fatalf("measurement/cleanup mismatch: used=%d claude=%+v gemini=%+v", used, claudeBody, geminiBody)
	}
	encoded, err := json.Marshal(map[string]any{
		"budget_limit": limit, "body_bytes": used, "requests": requests, "recorded_attempts": len(rows),
		"claude_payload_bytes": len(large), "claude_body_bytes": claudeBody.bytes, "claude_read_calls": claudeBody.reads,
		"claude_verdict": first.Verdict, "claude_reason": first.Reason,
		"gemini_body_bytes": geminiBody.bytes, "gemini_verdict": second.Verdict, "gemini_reason": second.Reason,
		"budget_cancelled": ctx.Err() != nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("CLAUDE_BUDGET %s", encoded)
}
