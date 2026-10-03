package platform

import (
	"bytes"
	"context"
	"encoding/json"
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

	case nonOriginalStatus == 200 || nonOriginalStatus == 301:
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "full"
		cap.Reason = "title_non_original_unlocked"
		reg := getNetflixRegion(ctx, httpClient)
		if reg != "" {
			cap.Region = reg
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
		if cdnErr != nil && nonOrigErr != nil && origErr != nil {
			cap.Verdict = domain.VerdictError
			cap.Summary = "Error"
			cap.Reason = "network_error"
		} else {
			cap.Verdict = domain.VerdictUnknown
			cap.Summary = "Inconclusive"
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
		return "", false, fmt.Errorf("unexpected fast.com status: %d", resp.StatusCode)
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
		if _, err := buf.ReadFrom(io.LimitReader(resp.Body, 4096)); err == nil {
			bodyLower := bytes.ToLower(buf.Bytes())
			if bytes.Contains(bodyLower, []byte("<title>netflix</title>")) ||
				bytes.Contains(bodyLower, []byte("watch anywhere")) ||
				bytes.Contains(bodyLower, []byte("netflix - watch tv shows online")) {
				return http.StatusNotFound, nil
			}
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
