package platform

import (
	"bytes"
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

var openaiRe = regexp.MustCompile(`loc=([A-Z]{2})`)

type openAIEndpointResult struct {
	responded bool
	verdict   domain.ProbeVerdict
	reason    string
}

// CheckOpenAI checks whether the anonymous web-compliance and iOS gateway
// endpoints respond through the supplied node transport. Neither endpoint has
// a verified positive contract for ChatGPT, API, account, model, or Codex
// capability, so an ordinary response is deliberately inconclusive.
func CheckOpenAI(ctx context.Context, httpClient *http.Client) domain.PlatformCapability {
	now := time.Now().UTC()
	start := time.Now()
	cap := domain.PlatformCapability{
		Verdict:       domain.VerdictUnknown,
		ObservedAt:    &now,
		EvidenceScope: "network_entry",
	}
	if httpClient == nil {
		latency := int64(-1)
		cap.Verdict, cap.LatencyMS = domain.VerdictError, &latency
		cap.Reason, cap.Summary = "transport_error", "OpenAI endpoints were not checked"
		return cap
	}

	web := checkOpenAIEndpoint(ctx, httpClient, "https://api.openai.com/compliance/cookie_requirements", false)
	app := checkOpenAIEndpoint(ctx, httpClient, "https://ios.chat.openai.com", true)
	latency := time.Since(start).Milliseconds()
	cap.LatencyMS = &latency

	// Explicit endpoint access restrictions are evidence about that endpoint
	// only. Rate limiting must never be described as an account ban.
	if web.reason == "rate_limited" || app.reason == "rate_limited" {
		cap.Verdict, cap.Reason = domain.VerdictRestricted, "rate_limited"
		cap.Summary = appendOpenAITrackDetails("OpenAI endpoint rate limited; capability not verified", web, app)
		return cap
	}
	if web.reason == "access_restricted" || app.reason == "access_restricted" {
		cap.Verdict, cap.Reason = domain.VerdictRestricted, "access_restricted"
		cap.Summary = appendOpenAITrackDetails("OpenAI endpoint access restricted; account and API capability not verified", web, app)
		return cap
	}

	responded := web.responded || app.responded
	if !responded {
		// Keep the most informative real failure. A skipped request must not
		// conceal a transport failure from its peer, or vice versa.
		for _, result := range []openAIEndpointResult{web, app} {
			if result.reason != "" && result.verdict != domain.VerdictSkipped {
				cap.Verdict, cap.Reason, cap.Summary = result.verdict, result.reason, "OpenAI endpoint check failed"
				break
			}
		}
		if cap.Reason == "" {
			cap.Verdict, cap.Reason, cap.Summary = domain.VerdictSkipped, firstOpenAIReason(web, app), "OpenAI endpoint checks were not executed"
		}
		return cap
	}

	cap.Verdict = domain.VerdictUnknown
	cap.Reason = "endpoint_reachable_capability_unverified"
	cap.Summary = appendOpenAITrackDetails("OpenAI network endpoint responded; ChatGPT, API, account, model, and Codex capability not verified", web, app)

	// CDN trace is auxiliary geographic evidence. It does not alter the
	// capability verdict or replace the scope of the endpoint checks.
	if region := getOpenAIRegion(ctx, httpClient); region != "" {
		cap.Region = region
		cap.RegionSource = "cdn_trace"
	}
	return cap
}

func appendOpenAITrackDetails(summary string, web, app openAIEndpointResult) string {
	var details []string
	if web.reason != "" && web.reason != "endpoint_reachable_capability_unverified" {
		details = append(details, "web_entry="+web.reason)
	}
	if app.reason != "" && app.reason != "endpoint_reachable_capability_unverified" {
		details = append(details, "ios_gateway="+app.reason)
	}
	if len(details) == 0 {
		return summary
	}
	return summary + " (" + strings.Join(details, "; ") + ")"
}

func firstOpenAIReason(results ...openAIEndpointResult) string {
	for _, result := range results {
		if result.reason != "" {
			return result.reason
		}
	}
	return "budget_not_executed"
}

func checkOpenAIEndpoint(ctx context.Context, httpClient *http.Client, endpoint string, mobile bool) openAIEndpointResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		verdict, reason := responseReadFailure(err)
		return openAIEndpointResult{verdict: verdict, reason: reason}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	if mobile {
		req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/16G29 ChatGPT/3.0")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "com.openai.chatgpt")
		req.Header.Set("Referer", "https://chat.openai.com/")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Origin", "https://chat.openai.com")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("sec-ch-ua-mobile", "?1")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		verdict, reason := responseReadFailure(err)
		return openAIEndpointResult{verdict: verdict, reason: reason}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return openAIEndpointResult{responded: true, verdict: domain.VerdictRestricted, reason: "rate_limited"}
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnavailableForLegalReasons {
		return openAIEndpointResult{responded: true, verdict: domain.VerdictRestricted, reason: "access_restricted"}
	}
	if resp.StatusCode >= 500 {
		return openAIEndpointResult{verdict: domain.VerdictError, reason: "upstream_error"}
	}

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		verdict, reason := responseReadFailure(err)
		return openAIEndpointResult{verdict: verdict, reason: reason}
	}
	bodyLower := bytes.ToLower(buf.Bytes())
	if bytes.Contains(bodyLower, []byte("unsupported_country")) {
		return openAIEndpointResult{responded: true, verdict: domain.VerdictRestricted, reason: "access_restricted"}
	}
	return openAIEndpointResult{responded: true, verdict: domain.VerdictUnknown, reason: "endpoint_reachable_capability_unverified"}
}

func getOpenAIRegion(ctx context.Context, httpClient *http.Client) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chat.openai.com/cdn-cgi/trace", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return ""
	}

	matches := openaiRe.FindSubmatch(buf.Bytes())
	if len(matches) > 1 {
		return strings.ToUpper(string(matches[1]))
	}
	return ""
}
