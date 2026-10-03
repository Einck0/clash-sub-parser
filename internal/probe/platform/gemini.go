package platform

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"github.com/biter777/countries"
)

var geminiRe = regexp.MustCompile(`,2,1,200,"([A-Z]{3})"`)

var geminiBlockedCodes = map[string]bool{
	"CHN": true, "RUS": true, "BLR": true, "CUB": true,
	"IRN": true, "PRK": true, "SYR": true, "HKG": true, "MAC": true,
}

func alpha3ToAlpha2(alpha3 string) string {
	code := strings.ToUpper(strings.TrimSpace(alpha3))
	country := countries.ByName(code)
	if country == countries.Unknown {
		return ""
	}
	return country.Alpha2()
}

// CheckGemini detects Google Gemini web availability and region code.
func CheckGemini(ctx context.Context, httpClient *http.Client) domain.PlatformCapability {
	now := time.Now().UTC()
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gemini.google.com/", nil)
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
		cap.Verdict = domain.VerdictError
		cap.Summary = "Error"
		cap.Reason = "network_error"
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
	if resp.StatusCode != http.StatusOK {
		cap.Verdict = domain.VerdictUnknown
		cap.Summary = "Inconclusive"
		cap.Reason = "unexpected_status"
		return cap
	}

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Error"
		cap.Reason = "read_failed"
		return cap
	}

	matches := geminiRe.FindSubmatch(buf.Bytes())
	if len(matches) <= 1 {
		cap.Verdict = domain.VerdictUnknown
		cap.Summary = "Inaccessible"
		cap.Reason = "gemini_marker_missing"
		return cap
	}

	alpha3Code := strings.ToUpper(string(matches[1]))
	alpha2Code := alpha3ToAlpha2(alpha3Code)
	if alpha2Code == "" {
		alpha2Code = alpha3Code
	}

	if geminiBlockedCodes[alpha3Code] {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Region = alpha2Code
		cap.Summary = fmt.Sprintf("Banned (%s)", alpha2Code)
		cap.Reason = "gemini_blocked_region"
		return cap
	}

	cap.Verdict = domain.VerdictAvailable
	cap.SubTier = "unlocked"
	cap.Region = alpha2Code
	cap.Summary = fmt.Sprintf("Unlocked (%s)", alpha2Code)
	cap.Reason = "gemini_available"
	return cap
}
