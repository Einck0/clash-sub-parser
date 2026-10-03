package platform

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// CheckIPRisk queries Scamalytics for the node exit IP and extracts the IP Fraud Risk score.
func CheckIPRisk(ctx context.Context, httpClient *http.Client, ip string) domain.PlatformCapability {
	now := time.Now().UTC()
	start := time.Now()

	cap := domain.PlatformCapability{
		ObservedAt: &now,
	}

	if ip == "" {
		// Resolve exit IP via Cloudflare trace if not pre-populated
		exitIP, statusCode, exitErr := resolveExitIP(ctx, httpClient)
		lat := time.Since(start).Milliseconds()
		cap.LatencyMS = &lat
		if exitErr != nil {
			cap.Verdict = domain.VerdictError
			cap.Summary = "Error"
			cap.Reason = "network_error"
			return cap
		}
		if statusCode == http.StatusForbidden || statusCode == http.StatusUnauthorized {
			cap.Verdict = domain.VerdictUnknown
			cap.Reason = "access_restricted"
			cap.Summary = "Access Restricted"
			return cap
		}
		if exitIP == "" {
			cap.Verdict = domain.VerdictUnknown
			cap.Reason = "missing_exit_identity"
			cap.Summary = "Unknown IP"
			return cap
		}
		ip = exitIP
	}

	targetURL := fmt.Sprintf("https://scamalytics.com/ip/%s", ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		lat := time.Since(start).Milliseconds()
		cap.LatencyMS = &lat
		cap.Verdict = domain.VerdictError
		cap.Reason = "request_build_failed"
		return cap
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	lat := time.Since(start).Milliseconds()
	cap.LatencyMS = &lat

	if err != nil {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Error"
		cap.Reason = "network_error"
		return cap
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		cap.Verdict = domain.VerdictUnknown
		cap.Summary = fmt.Sprintf("HTTP %d", resp.StatusCode)
		cap.Reason = "access_restricted"
		return cap
	}
	if resp.StatusCode != http.StatusOK {
		cap.Verdict = domain.VerdictRestricted
		cap.Summary = fmt.Sprintf("HTTP %d", resp.StatusCode)
		cap.Reason = "non_200_status"
		return cap
	}

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		cap.Verdict = domain.VerdictError
		cap.Reason = "read_failed"
		return cap
	}

	body := buf.Bytes()
	marker := []byte("IP Fraud Risk API")
	apiIndex := bytes.Index(body, marker)
	if apiIndex == -1 {
		cap.Verdict = domain.VerdictUnknown
		cap.Reason = "contract_drift"
		cap.Summary = "Score Unavailable"
		return cap
	}

	contentAfterAPI := body[apiIndex+len(marker):]
	lines := bytes.Split(contentAfterAPI, []byte("\n"))
	if len(lines) < 7 {
		cap.Verdict = domain.VerdictUnknown
		cap.Reason = "contract_drift"
		cap.Summary = "Score Unavailable"
		return cap
	}

	scoreLine := bytes.TrimSpace(lines[4])
	scoreParts := bytes.Split(scoreLine, []byte(":"))
	riskLine := bytes.TrimSpace(lines[5])
	riskParts := bytes.Split(riskLine, []byte(":"))

	if len(scoreParts) >= 2 {
		score := bytes.TrimSpace(bytes.ReplaceAll(bytes.ReplaceAll(scoreParts[1], []byte(`"`), nil), []byte(`,`), nil))
		risk := ""
		if len(riskParts) >= 2 {
			risk = string(bytes.TrimSpace(bytes.ReplaceAll(bytes.ReplaceAll(riskParts[1], []byte(`"`), nil), []byte(`,`), nil)))
		}

		scoreStr := string(score) + "%"
		if risk != "" {
			scoreStr = scoreStr + " " + risk
		}

		cap.Verdict = domain.VerdictAvailable
		cap.RiskScore = scoreStr
		cap.Summary = scoreStr
		cap.Reason = "scamalytics_scored"
		return cap
	}

	cap.Verdict = domain.VerdictUnknown
	cap.Reason = "contract_drift"
	return cap
}

func resolveExitIP(ctx context.Context, httpClient *http.Client) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return "", resp.StatusCode, nil
	}

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return "", resp.StatusCode, err
	}

	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "ip=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "ip=")), resp.StatusCode, nil
		}
	}
	return "", resp.StatusCode, nil
}
