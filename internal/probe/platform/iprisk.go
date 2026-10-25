package platform

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"clash-sub-parser/internal/domain"
)

// ParseScoreInt strictly parses a raw integer score string.
// It requires an integer in the range 0..100.
// It rejects non-numeric characters, signs, decimals, empty strings,
// leading zeros (except single '0'), and values outside 0..100.
func ParseScoreInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 3 {
		return 0, false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, false
		}
	}
	if len(raw) > 1 && raw[0] == '0' {
		return 0, false
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 0 || val > 100 {
		return 0, false
	}
	return val, true
}

// ParseRiskScore parses a risk score string (e.g. "15% low", "15%", "0%", "100%")
// or a raw score integer (e.g. "15", "0", "100").
// It strictly requires a complete 0..100 integer.
// It rejects negative numbers, >100, decimals, empty strings, non-numeric strings,
// multiple '%' symbols, and trailing garbage attached to the percent sign.
func ParseRiskScore(scoreStr string) (int, bool) {
	scoreStr = strings.TrimSpace(scoreStr)
	if scoreStr == "" {
		return 0, false
	}
	if strings.Count(scoreStr, "%") > 1 {
		return 0, false
	}
	var numStr string
	if idx := strings.IndexByte(scoreStr, '%'); idx != -1 {
		numStr = scoreStr[:idx]
		rest := scoreStr[idx+1:]
		if len(rest) > 0 && !unicode.IsSpace(rune(rest[0])) {
			return 0, false
		}
	} else {
		numStr = scoreStr
	}
	return ParseScoreInt(numStr)
}

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
	if len(scoreParts) < 2 {
		cap.Verdict = domain.VerdictUnknown
		cap.Reason = "contract_drift"
		cap.Summary = "Score Unavailable"
		return cap
	}

	key := strings.ToLower(strings.Trim(string(bytes.TrimSpace(scoreParts[0])), `"`))
	if key != "score" {
		cap.Verdict = domain.VerdictUnknown
		cap.Reason = "contract_drift"
		cap.Summary = "Score Unavailable"
		return cap
	}

	rawScore := strings.Trim(string(bytes.TrimSpace(scoreParts[1])), `",`)
	rawScore = strings.TrimSpace(rawScore)
	scoreInt, ok := ParseScoreInt(rawScore)
	if !ok {
		cap.Verdict = domain.VerdictUnknown
		cap.Reason = "contract_drift"
		cap.Summary = "Score Unavailable"
		return cap
	}

	riskLine := bytes.TrimSpace(lines[5])
	riskParts := bytes.Split(riskLine, []byte(":"))
	risk := ""
	if len(riskParts) >= 2 {
		rKey := strings.ToLower(strings.Trim(string(bytes.TrimSpace(riskParts[0])), `"`))
		if rKey == "risk" {
			risk = strings.Trim(string(bytes.TrimSpace(riskParts[1])), `",`)
			risk = strings.TrimSpace(risk)
		}
	}

	scoreStr := fmt.Sprintf("%d%%", scoreInt)
	if risk != "" {
		scoreStr = scoreStr + " " + risk
	}

	cap.Verdict = domain.VerdictAvailable
	cap.RiskScore = scoreStr
	cap.Summary = scoreStr
	cap.Reason = "scamalytics_scored"
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
