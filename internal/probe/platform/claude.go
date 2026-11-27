package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"github.com/biter777/countries"
)

// Claude sanctioned / blocked regions (ISO 3166-1 alpha-2)
var claudeBlockedRegions = map[string]bool{
	"AF": true, "BY": true, "CN": true, "CU": true, "HK": true,
	"IR": true, "KP": true, "MO": true, "RU": true, "SY": true,
}

// CheckClaude detects Anthropic Claude availability via CDN trace and sanctioned region blacklist.
// Notice: This is a CDN trace geolocation heuristic. It does not perform full account authentication or chat dialogue acceptance.
func CheckClaude(ctx context.Context, httpClient *http.Client) domain.PlatformCapability {
	now := time.Now().UTC()
	start := time.Now()

	if httpClient == nil {
		lat := int64(-1)
		return domain.PlatformCapability{
			Verdict:    domain.VerdictError,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Reason:     "transport_error",
			Summary:    "Inconclusive",
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://claude.ai/cdn-cgi/trace", nil)
	if err != nil {
		lat := int64(-1)
		return domain.PlatformCapability{
			Verdict:    domain.VerdictError,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Reason:     "request_build_failed",
		}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	latency := time.Since(start).Milliseconds()

	cap := domain.PlatformCapability{
		LatencyMS:  &latency,
		ObservedAt: &now,
	}

	if err != nil {
		verdict, reason := responseReadFailure(err)
		cap.Verdict = verdict
		cap.Summary = "Error"
		cap.Reason = reason
		return cap
	}
	defer resp.Body.Close()

	if !validFinalOrigin(resp, "claude.ai") || resp.Request.URL.Path != "/cdn-cgi/trace" {
		cap.Verdict = domain.VerdictRestricted
		cap.Summary = "Access restricted"
		cap.Reason = "unexpected_redirect"
		return cap
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnavailableForLegalReasons {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Banned"
		cap.Reason = "access_restricted"
		return cap
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Rate Limited"
		cap.Reason = "rate_limited"
		return cap
	}
	if resp.StatusCode != http.StatusOK {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Detection failed"
		cap.Reason = "unexpected_status"
		return cap
	}

	// A trace is small key/value text. Reuse the generic detector bound, not
	// Gemini's larger page bound; an early loc must not hide late invalid evidence.
	body, truncated, err := readBoundedBody(resp.Body, detectorBodyLimit)
	if err != nil {
		verdict, reason := responseReadFailure(err)
		cap.Verdict = verdict
		cap.Summary = "Error"
		cap.Reason = reason
		return cap
	}

	if truncated {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Detection failed"
		cap.Reason = "response_body_too_large"
		return cap
	}

	// Cloudflare explicitly marks Challenge Pages with cf-mitigated. Also
	// recognize HTML challenge responses, without treating echoed UA field
	// values containing a challenge marker as proof of an actual challenge.
	text := string(body)
	challenge := strings.EqualFold(resp.Header.Get("Cf-Mitigated"), "challenge")
	html := false
	for line := range strings.SplitSeq(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "<") {
			continue
		}
		html = true
		line = strings.ToLower(line)
		challenge = challenge || strings.Contains(line, "challenge-platform") ||
			strings.Contains(line, "cf-chl-") || strings.Contains(line, "<title>just a moment")
	}
	if challenge {
		cap.Verdict = domain.VerdictRestricted
		cap.Summary = "Access restricted"
		cap.Reason = "challenge_or_login"
		return cap
	}
	if html {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Detection failed"
		cap.Reason = "trace_loc_missing"
		return cap
	}
	region, reason := claudeTraceRegion(text)
	if reason != "" {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Detection failed"
		cap.Reason = reason
		return cap
	}

	cap.Region = region
	cap.RegionSource = "cdn_trace"
	cap.EvidenceScope = "cdn_trace"

	if claudeBlockedRegions[region] {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = fmt.Sprintf("CDN trace blocked region (%s); account/chat eligibility unverified", region)
		cap.Reason = "sanctioned_region"
		return cap
	}

	cap.Verdict = domain.VerdictAvailable
	cap.SubTier = "unlocked"
	cap.Summary = fmt.Sprintf("CDN trace region (%s); account/chat eligibility unverified", region)
	cap.Reason = "claude_available"
	return cap
}

// Only complete loc line fields are evidence. Inspect every recognized field;
// syntax failure outranks unknown countries, which outrank country conflicts.
func claudeTraceRegion(text string) (string, string) {
	region := ""
	unknown, conflict := false, false
	for line := range strings.SplitSeq(text, "\n") {
		value, found := strings.CutPrefix(strings.TrimSuffix(line, "\r"), "loc=")
		if !found {
			continue
		}
		if len(value) != 2 || value[0] < 'A' || value[0] > 'Z' || value[1] < 'A' || value[1] > 'Z' {
			return "", "trace_loc_malformed"
		}
		country := countries.ByName(value)
		if country == countries.Unknown || country.Alpha2() != value {
			unknown = true
		}
		if region != "" && region != value {
			conflict = true
		}
		region = value
	}
	switch {
	case unknown:
		return "", "unknown_region_code"
	case conflict:
		return "", "conflicting_region_markers"
	case region == "":
		return "", "trace_loc_missing"
	default:
		return region, ""
	}
}
