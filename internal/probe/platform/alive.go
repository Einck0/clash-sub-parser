package platform

import (
	"context"
	"io"
	"net/http"
	"time"

	"clash-sub-parser/internal/domain"
)

const DefaultAliveURL = "http://cp.cloudflare.com/generate_204"

// AliveResult encapsulates the outcome of a stage 1 alive check.
type AliveResult struct {
	Verdict    domain.ProbeVerdict
	LatencyMS  int64
	Reason     string
	StatusCode int
	BodyBytes  int64
}

// CheckAlive performs a lightweight stage 1 alive check against Cloudflare or CDN 204.
func CheckAlive(ctx context.Context, httpClient *http.Client, testURL string) (bool, AliveResult) {
	if testURL == "" {
		testURL = DefaultAliveURL
	}

	if httpClient == nil {
		return false, AliveResult{
			Verdict:   domain.VerdictError,
			LatencyMS: -1,
			Reason:    "transport_error",
		}
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
		verdict, reason := responseReadFailure(err)
		return false, AliveResult{
			Verdict:   verdict,
			LatencyMS: latency,
			Reason:    reason,
		}
	}
	defer resp.Body.Close()
	// Consume a bounded body through the shared limiter if one is installed.
	n, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if readErr != nil {
		verdict, reason := responseReadFailure(readErr)
		return false, AliveResult{Verdict: verdict, Reason: reason, StatusCode: resp.StatusCode, BodyBytes: n, LatencyMS: latency}
	}

	if resp.StatusCode == http.StatusNoContent || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return true, AliveResult{
			Verdict:    domain.VerdictAvailable,
			LatencyMS:  latency,
			Reason:     "alive_any2xx",
			StatusCode: resp.StatusCode,
			BodyBytes:  n,
		}
	}

	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusUnavailableForLegalReasons:
		return false, AliveResult{
			Verdict:    domain.VerdictRestricted,
			LatencyMS:  latency,
			Reason:     "access_restricted",
			StatusCode: resp.StatusCode,
			BodyBytes:  n,
		}
	case http.StatusTooManyRequests:
		return false, AliveResult{
			Verdict:    domain.VerdictRestricted,
			LatencyMS:  latency,
			Reason:     "rate_limited",
			StatusCode: resp.StatusCode,
			BodyBytes:  n,
		}
	default:
		return false, AliveResult{
			Verdict:    domain.VerdictError,
			LatencyMS:  latency,
			Reason:     "unexpected_status",
			StatusCode: resp.StatusCode,
			BodyBytes:  n,
		}
	}
}
