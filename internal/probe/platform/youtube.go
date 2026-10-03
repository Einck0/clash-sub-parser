package platform

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

var youtubeReList = []*regexp.Regexp{
	regexp.MustCompile(`"INNERTUBE_CONTEXT_GL"\s*:\s*"([^"]+)"`),
	regexp.MustCompile(`id=["']country-code["'][^>]*>\s*([A-Za-z]{2,3})\s*<`),
	regexp.MustCompile(`"GL"\s*:\s*"([A-Za-z]{2})"`),
	regexp.MustCompile(`"countryCode"\s*:\s*"([A-Za-z]{2})"`),
	regexp.MustCompile(`"country_code"\s*:\s*"([A-Za-z]{2})"`),
}

// CheckYoutube detects YouTube Premium availability and regional classification.
func CheckYoutube(ctx context.Context, httpClient *http.Client) domain.PlatformCapability {
	now := time.Now().UTC()
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.youtube.com/premium?hl=en", nil)
	if err != nil {
		lat := int64(-1)
		return domain.PlatformCapability{
			Verdict:    domain.VerdictError,
			LatencyMS:  &lat,
			ObservedAt: &now,
			Reason:     "request_build_failed",
		}
	}

	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("sec-ch-ua", `"Chromium";v="131", "Not_A Brand";v="24", "Google Chrome";v="131"`)
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "none")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

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

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Error"
		cap.Reason = "read_failed"
		return cap
	}
	body := buf.Bytes()

	// Direct redirect to www.google.cn indicates IP sent to China
	if bytes.Contains(body, []byte("www.google.cn")) {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Region = "CN"
		cap.Summary = "Redirected to CN"
		cap.Reason = "redirected_to_cn"
		return cap
	}

	bodyLower := bytes.ToLower(body)

	if bytes.Contains(bodyLower, []byte("premium is not available in your country")) ||
		bytes.Contains(bodyLower, []byte("premium is not available in your region")) {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Unavailable"
		cap.Reason = "premium_unavailable"
		return cap
	}

	unlocked := resp.StatusCode >= 200 && resp.StatusCode < 300 &&
		(bytes.Contains(bodyLower, []byte("youtube premium")) ||
			bytes.Contains(bodyLower, []byte("ad-free")) ||
			bytes.Contains(bodyLower, []byte(`"browseid":"spunlimited"`)))

	if !unlocked {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Not Unlocked"
		cap.Reason = "positive_markers_missing"
		return cap
	}

	for _, re := range youtubeReList {
		match := re.FindSubmatch(body)
		if len(match) > 1 {
			if region := strings.ToUpper(string(match[1])); region != "" {
				cap.Verdict = domain.VerdictAvailable
				cap.SubTier = "unlocked"
				cap.Region = region
				cap.Summary = fmt.Sprintf("Unlocked (%s)", region)
				cap.Reason = "youtube_premium_unlocked"
				return cap
			}
		}
	}

	cap.Verdict = domain.VerdictAvailable
	cap.SubTier = "unlocked"
	cap.Summary = "Unlocked"
	cap.Reason = "youtube_premium_unlocked"
	return cap
}
