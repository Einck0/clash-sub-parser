package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

var netflixRe = regexp.MustCompile(`/([a-z]{2})/title/`)

// CheckNetflix detects Netflix unlock status via Fast.com CDN first, falling back to dual title assertion.
func CheckNetflix(ctx context.Context, httpClient *http.Client) domain.PlatformCapability {
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

	region, banned, cdnErr := checkNetflixCDN(ctx, httpClient)
	latency := time.Since(start).Milliseconds()

	cap := domain.PlatformCapability{
		LatencyMS:  &latency,
		ObservedAt: &now,
	}

	if banned {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Banned"
		cap.Reason = "fast_403"
		return cap
	} else if region != "" {
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "full"
		cap.Region = region
		cap.RegionSource = "cdn_target"
		cap.Summary = fmt.Sprintf("Full (%s)", region)
		cap.Reason = "fast_cdn_unlocked"
		return cap
	}

	// Fallback to title probing:
	// Non-original title: 81280792 (region restricted)
	// Original title: 70143836 (Netflix Originals)
	nonOriginalStatus, nonOrigErr := checkNetflixTitle(ctx, httpClient, "81280792")
	originalStatus, origErr := checkNetflixTitle(ctx, httpClient, "70143836")

	switch {
	case nonOriginalStatus == http.StatusForbidden || originalStatus == http.StatusForbidden:
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Banned"
		cap.Reason = "title_403"

	case nonOriginalStatus == http.StatusTooManyRequests || originalStatus == http.StatusTooManyRequests:
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Rate Limited"
		cap.Reason = "rate_limited"

	case nonOriginalStatus == 200 || nonOriginalStatus == 301:
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "full"
		cap.Reason = "title_non_original_unlocked"
		reg := getNetflixRegion(ctx, httpClient)
		if reg != "" {
			cap.Region = reg
			cap.RegionSource = "service"
			cap.EvidenceScope = "web_region"
			cap.Summary = fmt.Sprintf("Full (%s)", reg)
		} else {
			cap.Summary = "Full"
		}

	case nonOriginalStatus == 404 && (originalStatus == 200 || originalStatus == 301):
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "originals"
		cap.Summary = "Originals Only"
		cap.Reason = "title_originals_only"

	default:
		// A completed CDN lookup without a region is inconclusive, not a
		// successful check. Preserve errors from the title fallback even when
		// another request completed; definitive title evidence wins above.
		errToClassify := errors.Join(nonOrigErr, origErr)
		if errToClassify == nil {
			errToClassify = cdnErr
		}
		if errToClassify != nil {
			verdict, reason := responseReadFailure(errToClassify)
			cap.Verdict = verdict
			cap.Summary = "Error"
			cap.Reason = reason
		} else {
			cap.Verdict = domain.VerdictError
			cap.Summary = "Detection failed"
			cap.Reason = "status_mismatch"
		}
	}

	return cap
}

func checkNetflixCDN(ctx context.Context, httpClient *http.Client) (region string, banned bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.fast.com/netflix/speedtest/v2?https=true&token=YXNkZmFzZGxmbnNkYWZoYXNkZmhrYWxm&urlCount=1", nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return "", true, nil
	}
	if resp.StatusCode != http.StatusOK {
		// A completed HTTP response is not a transport failure. Like an empty
		// target list, it supplies no CDN evidence; title checks determine the
		// result, or report status_mismatch when they are also inconclusive.
		// Reserve err for actual request/read/decode failures and interruption.
		return "", false, nil
	}

	var data struct {
		Targets []struct {
			Location struct {
				Country string `json:"country"`
			} `json:"location"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", false, err
	}
	if len(data.Targets) == 0 || data.Targets[0].Location.Country == "" {
		return "", false, nil
	}
	return strings.ToUpper(data.Targets[0].Location.Country), false, nil
}

func checkNetflixTitle(ctx context.Context, httpClient *http.Client, titleID string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.netflix.com/title/"+titleID, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		buf := getPooledBuf()
		defer putPooledBuf(buf)
		if _, err := buf.ReadFrom(io.LimitReader(resp.Body, 4096)); err != nil {
			return 0, err
		}
		bodyLower := bytes.ToLower(buf.Bytes())
		if bytes.Contains(bodyLower, []byte("<title>netflix</title>")) ||
			bytes.Contains(bodyLower, []byte("watch anywhere")) ||
			bytes.Contains(bodyLower, []byte("netflix - watch tv shows online")) {
			return http.StatusNotFound, nil
		}
	}

	return resp.StatusCode, nil
}

func getNetflixRegion(ctx context.Context, httpClient *http.Client) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.netflix.com/title/80018499", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	// Shallow copy client to avoid mutating caller checkRedirect
	c := *httpClient
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := c.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	location := resp.Header.Get("Location")
	if location == "" {
		return ""
	}

	matches := netflixRe.FindStringSubmatch(location)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}
	return ""
}
