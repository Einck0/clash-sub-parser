package platform

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

const detectorBodyLimit int64 = 256 << 10

func responseReadFailure(err error) (domain.ProbeVerdict, string) {
	reason := ResponseReason(err)
	if reason == "cancelled" || reason == "deadline_exceeded" || reason == "budget_not_executed" {
		return domain.VerdictUnknown, reason
	}
	return domain.VerdictError, reason
}

func hasGoogleAccessGate(body []byte) bool {
	text := strings.ToLower(string(body))
	for _, marker := range []string{
		"/recaptcha/", "g-recaptcha", "captcha-form", "challenge-platform",
		"verify you are human", "unusual traffic", "accounts.google.com/signin",
		"accounts.google.com/v3/signin", "<form id=\"gaia_loginform\"",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func readBoundedBody(r io.Reader, limit int64) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > limit {
		return body[:limit], true, nil
	}
	return body, false, nil
}

// CheckAntigravity only observes the anonymous official product entry page.
// Reachability is not evidence of authentication or account eligibility.
func CheckAntigravity(ctx context.Context, client *http.Client) domain.PlatformCapability {
	now, started := time.Now().UTC(), time.Now()
	finish := func(verdict domain.ProbeVerdict, reason, summary, tier string) domain.PlatformCapability {
		latency := time.Since(started).Milliseconds()
		return domain.PlatformCapability{Verdict: verdict, Reason: reason, Summary: summary, SubTier: tier, LatencyMS: &latency, ObservedAt: &now}
	}
	if client == nil {
		return finish(domain.VerdictUnknown, "transport_error", "Inconclusive", "")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://antigravity.google/", nil)
	if err != nil {
		return finish(domain.VerdictUnknown, "request_build_failed", "Inconclusive", "")
	}
	resp, err := client.Do(req)
	if err != nil {
		verdict, reason := responseReadFailure(err)
		return finish(verdict, reason, "Inconclusive", "")
	}
	defer resp.Body.Close()
	if !validFinalOrigin(resp, "antigravity.google") {
		return finish(domain.VerdictUnknown, "unexpected_redirect", "Inconclusive", "")
	}
	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusUnavailableForLegalReasons:
		return finish(domain.VerdictRestricted, "access_restricted", "Access restricted", "")
	case http.StatusTooManyRequests:
		return finish(domain.VerdictUnknown, "rate_limited", "Inconclusive", "")
	case http.StatusOK:
	default:
		return finish(domain.VerdictUnknown, "unexpected_status", "Inconclusive", "")
	}
	body, truncated, err := readBoundedBody(resp.Body, detectorBodyLimit)
	if err != nil {
		verdict, reason := responseReadFailure(err)
		return finish(verdict, reason, "Inconclusive", "")
	}
	if truncated {
		return finish(domain.VerdictUnknown, "response_body_truncated", "Inconclusive", "")
	}
	if resp.Request == nil || resp.Request.URL == nil || strings.Contains(strings.ToLower(resp.Request.URL.Path), "login") {
		return finish(domain.VerdictUnknown, "unexpected_redirect", "Inconclusive", "")
	}
	if hasGoogleAccessGate(body) {
		return finish(domain.VerdictUnknown, "challenge_or_login", "Inconclusive", "")
	}
	text := strings.ToLower(string(body))
	if !(strings.Contains(text, "antigravity") && (strings.Contains(text, "agentic") || strings.Contains(text, "coding"))) {
		return finish(domain.VerdictUnknown, "product_marker_missing", "Inconclusive", "")
	}
	return finish(domain.VerdictUnknown, "antigravity_reachable_auth_unverified", "Antigravity product entry reachable; product/account eligibility unverified", "reachable")
}
