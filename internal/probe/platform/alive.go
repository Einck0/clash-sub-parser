package platform

import (
	"context"
	"net/http"
	"time"

	"clash-sub-parser/internal/domain"
)

const DefaultAliveURL = "http://cp.cloudflare.com/generate_204"

// AliveResult encapsulates the outcome of a stage 1 alive check.
type AliveResult struct {
	Verdict   domain.ProbeVerdict
	LatencyMS int64
	Reason    string
}

// CheckAlive performs a lightweight stage 1 alive check against Cloudflare or CDN 204.
func CheckAlive(ctx context.Context, httpClient *http.Client, testURL string) (bool, AliveResult) {
	if testURL == "" {
		testURL = DefaultAliveURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return false, AliveResult{
			Verdict:   domain.VerdictError,
			LatencyMS: -1,
			Reason:    "request_build_failed",
		}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CSP-Probe/1.0)")

	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return false, AliveResult{
			Verdict:   domain.VerdictError,
			LatencyMS: latency,
			Reason:    "dial_failed",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return true, AliveResult{
			Verdict:   domain.VerdictAvailable,
			LatencyMS: latency,
			Reason:    "alive_204",
		}
	}

	return false, AliveResult{
		Verdict:   domain.VerdictRestricted,
		LatencyMS: latency,
		Reason:    "unexpected_status",
	}
}
