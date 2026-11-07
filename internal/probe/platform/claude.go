package platform

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

var claudeRe = regexp.MustCompile(`loc=([A-Z]{2})`)

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

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		verdict, reason := responseReadFailure(err)
		cap.Verdict = verdict
		cap.Summary = "Error"
		cap.Reason = reason
		return cap
	}

	matches := claudeRe.FindSubmatch(buf.Bytes())
	if len(matches) <= 1 {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Detection failed"
		cap.Reason = "trace_loc_missing"
		return cap
	}

	region := strings.ToUpper(string(matches[1]))
	cap.Region = region
	cap.RegionSource = "cdn_trace"
	cap.EvidenceScope = "cdn_trace"

	if claudeBlockedRegions[region] {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = fmt.Sprintf("Banned (%s)", region)
		cap.Reason = "sanctioned_region"
		return cap
	}

	cap.Verdict = domain.VerdictAvailable
	cap.SubTier = "unlocked"
	cap.Summary = fmt.Sprintf("Unlocked (%s)", region)
	cap.Reason = "claude_available"
	return cap
}
