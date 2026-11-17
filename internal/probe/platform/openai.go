package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

const openAIMaxBody = 1 << 20

var openaiRe = regexp.MustCompile(`(?m)^loc=([A-Z]{2})$`)

type openAIResult struct {
	check    domain.PlatformCheck
	body     string
	finalURL *url.URL
}

// CheckOpenAI is an anonymous public-entry preflight, not an authenticated
// ChatGPT, API, account, model, or mobile-app capability check.
func CheckOpenAI(ctx context.Context, client *http.Client) domain.PlatformCapability {
	now, start := time.Now().UTC(), time.Now()
	cap := domain.PlatformCapability{Verdict: domain.VerdictUnknown, ObservedAt: &now, EvidenceScope: "anonymous_preflight", Checks: map[string]domain.PlatformCheck{}}
	if client == nil {
		cap.Verdict, cap.Reason = domain.VerdictError, "transport_error"
		cap.Summary = "OpenAI preflight transport is unavailable"
		return cap
	}
	compliance := openAIRequest(ctx, client, "https://api.openai.com/compliance/cookie_requirements", "compliance", openAICompliance)
	web := openAIRequest(ctx, client, "https://chatgpt.com/", "chatgpt_web", openAIWeb)
	ion := openAIRequest(ctx, client, "https://ios.chat.openai.com/", "legacy_ios_entry", nil)
	cap.Checks["cookie_requirements"], cap.Checks["chatgpt_web"], cap.Checks["legacy_ios_entry"] = compliance.check, web.check, ion.check
	cap.LatencyMS = ptrInt64(time.Since(start).Milliseconds())
	cap.Verdict, cap.Reason = aggregateOpenAIPreflight(compliance.check, web.check)
	cap.Summary = "Anonymous public-entry preflight only; account, login, model, API, and app capability not verified"
	if compliance.check.Verdict == domain.VerdictAvailable && web.check.Verdict == domain.VerdictAvailable {
		cap.Summary = "Public ChatGPT webpage and compliance preflight passed; account, login, model, API, and app capability not verified"
	}
	// The historical iOS gateway is diagnostic only and never gates the preflight.
	if region := getOpenAIRegion(ctx, client); region != "" {
		cap.Region, cap.RegionSource = region, "cdn_trace"
	}
	return cap
}
func ptrInt64(v int64) *int64 { return &v }

type openAIValidator func(status int, header http.Header, body string) (domain.ProbeVerdict, string)

func openAIRequest(ctx context.Context, c *http.Client, endpoint, name string, validate openAIValidator) openAIResult {
	result := openAIResult{check: domain.PlatformCheck{Verdict: domain.VerdictUnknown, Reason: "unclassified", EvidenceScope: "network_entry"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		result.check.Verdict, result.check.Reason = domain.VerdictError, "transport_error"
		return result
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/122.0.0.0 Safari/537.36")
	resp, err := c.Do(req)
	if err != nil {
		result.check.Verdict, result.check.Reason = responseReadFailure(err)
		return result
	}
	defer resp.Body.Close()
	status := resp.StatusCode
	result.check.StatusCode = &status
	result.finalURL = req.URL
	if resp.Request != nil && resp.Request.URL != nil {
		result.finalURL = resp.Request.URL
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, openAIMaxBody+1))
	if err != nil {
		result.check.Verdict, result.check.Reason = domain.VerdictError, "response_read_error"
		return result
	}
	if len(body) > openAIMaxBody {
		result.check.Verdict, result.check.Reason = domain.VerdictError, "response_too_large"
		return result
	}
	result.body = string(body)
	expectedHost := map[string]string{"compliance": "api.openai.com", "chatgpt_web": "chatgpt.com", "trace": "chatgpt.com"}[name]
	if expectedHost != "" && (!strings.EqualFold(result.finalURL.Scheme, "https") || !strings.EqualFold(result.finalURL.Host, expectedHost)) {
		result.check.Verdict, result.check.Reason = domain.VerdictError, "unexpected_redirect"
		return result
	}
	if validate != nil {
		result.check.Verdict, result.check.Reason = validate(status, resp.Header, result.body)
		return result
	}
	result.check.Verdict, result.check.Reason = classifyOpenAI(status, resp.Header, result.body)
	return result
}
func classifyOpenAI(status int, h http.Header, b string) (domain.ProbeVerdict, string) {
	low := strings.ToLower(b)
	if strings.EqualFold(h.Get("cf-mitigated"), "challenge") || strings.Contains(low, "<title>just a moment") || strings.Contains(low, "cf-chl-") {
		return domain.VerdictUnknown, "challenge"
	}
	if status == http.StatusTooManyRequests {
		return domain.VerdictUnknown, "rate_limited"
	}
	if status == http.StatusUnavailableForLegalReasons {
		return domain.VerdictRestricted, "legal_restriction"
	}
	if status == http.StatusForbidden {
		if structuredUnsupportedRegion(b) {
			return domain.VerdictRestricted, "unsupported_region"
		}
		return domain.VerdictUnknown, "access_denied_unclassified"
	}
	if status == http.StatusUnauthorized {
		return domain.VerdictUnknown, "authentication_required"
	}
	if status >= 500 {
		return domain.VerdictError, "upstream_error"
	}
	if status >= 300 && status < 400 {
		return domain.VerdictError, "unexpected_redirect"
	}
	if status == 404 {
		return domain.VerdictError, "endpoint_contract_changed"
	}
	return domain.VerdictUnknown, "endpoint_not_capability_evidence"
}
func aggregateOpenAIPreflight(a, b domain.PlatformCheck) (domain.ProbeVerdict, string) {
	av, bv := a.Verdict, b.Verdict
	if av == domain.VerdictAvailable && bv == domain.VerdictAvailable {
		return domain.VerdictAvailable, "anonymous_preflight_passed"
	}
	if (av == domain.VerdictAvailable && bv == domain.VerdictRestricted) || (bv == domain.VerdictAvailable && av == domain.VerdictRestricted) {
		return domain.VerdictAbnormal, "conflicting_entry_evidence"
	}
	if av == domain.VerdictRestricted && bv == domain.VerdictRestricted {
		return domain.VerdictRestricted, "entry_restrictions_confirmed"
	}
	if av == domain.VerdictError || bv == domain.VerdictError {
		return domain.VerdictError, "partial_preflight_error"
	}
	if av == domain.VerdictRestricted || bv == domain.VerdictRestricted {
		return domain.VerdictUnknown, "incomplete_preflight"
	}
	if av == domain.VerdictUnknown && bv == domain.VerdictUnknown {
		for _, reason := range []string{a.Reason, b.Reason} {
			if reason == "challenge" || reason == "rate_limited" || reason == "access_denied_unclassified" {
				return domain.VerdictUnknown, reason
			}
		}
	}
	return domain.VerdictUnknown, "partial_preflight"
}

func structuredUnsupportedRegion(body string) bool {
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil {
		return false
	}
	errObject, ok := payload["error"].(map[string]any)
	if !ok {
		return false
	}
	for _, field := range []string{"code", "type"} {
		if value, ok := errObject[field].(string); ok && (value == "unsupported_country" || value == "unsupported_country_region_territory") {
			return true
		}
	}
	return false
}

func openAICompliance(status int, h http.Header, body string) (domain.ProbeVerdict, string) {
	v, r := classifyOpenAI(status, h, body)
	if v != domain.VerdictUnknown || (r != "endpoint_not_capability_evidence" && r != "unclassified") {
		return v, r
	}
	var payload map[string]any
	if status != 200 || json.Unmarshal([]byte(body), &payload) != nil {
		return domain.VerdictError, "contract_drift"
	}
	if structuredUnsupportedRegion(body) {
		return domain.VerdictRestricted, "unsupported_region"
	}
	if value, ok := payload["cookie_consent_required"].(bool); ok {
		_ = value
		return domain.VerdictAvailable, "compliance_protocol_matched"
	}
	return domain.VerdictError, "contract_drift"
}
func openAIWeb(status int, h http.Header, body string) (domain.ProbeVerdict, string) {
	v, r := classifyOpenAI(status, h, body)
	if v != domain.VerdictUnknown || (r != "endpoint_not_capability_evidence" && r != "unclassified") {
		return v, r
	}
	low := strings.ToLower(body)
	if status != 200 || !strings.Contains(strings.ToLower(h.Get("Content-Type")), "text/html") {
		return domain.VerdictError, "contract_drift"
	}
	if !strings.Contains(low, "<title>chatgpt</title>") || !strings.Contains(low, "window.__reactroutercontext") || (!strings.Contains(low, "/cdn/assets/") && !strings.Contains(low, "oaistatic.com")) {
		return domain.VerdictError, "contract_drift"
	}
	return domain.VerdictAvailable, "public_web_entry_matched"
}
func getOpenAIRegion(ctx context.Context, c *http.Client) string {
	r := openAIRequest(ctx, c, "https://chatgpt.com/cdn-cgi/trace", "trace", nil)
	if r.check.Verdict != domain.VerdictUnknown || r.check.StatusCode == nil || *r.check.StatusCode != 200 {
		return ""
	}
	m := openaiRe.FindStringSubmatch(r.body)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
