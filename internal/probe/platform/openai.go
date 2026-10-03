package platform

import (
	"bytes"
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

var openaiRe = regexp.MustCompile(`loc=([A-Z]{2})`)

// CheckOpenAI inspects ChatGPT/OpenAI unlock state via dual-track detection (web cookies + iOS app).
// Returns a domain.PlatformCapability.
func CheckOpenAI(ctx context.Context, httpClient *http.Client) domain.PlatformCapability {
	now := time.Now().UTC()
	start := time.Now()

	cookiesOK, cookiesRestricted, cookiesErr := checkCookies(ctx, httpClient)
	clientOK, clientRestricted, clientErr := checkClient(ctx, httpClient)
	latency := time.Since(start).Milliseconds()

	cap := domain.PlatformCapability{
		LatencyMS:  &latency,
		ObservedAt: &now,
	}

	if cookiesOK && clientOK {
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "full"
		cap.Summary = "Full (GPT⁺)"
		cap.Reason = "cookies_and_client_passed"
	} else if cookiesOK && !clientOK {
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "web"
		cap.Summary = "Web (GPT)"
		cap.Reason = "cookies_passed_client_failed"
	} else if !cookiesOK && clientOK {
		// App-only path: must be reported as "app", not forced to "web"
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "app"
		cap.Summary = "App (ChatGPT)"
		cap.Reason = "client_passed_cookies_failed"
	} else {
		// Both failed
		if cookiesErr != nil && clientErr != nil {
			cap.Verdict = domain.VerdictError
			cap.Reason = "network_error"
			cap.Summary = "Error"
			return cap
		}
		if cookiesRestricted || clientRestricted {
			cap.Verdict = domain.VerdictRestricted
			cap.SubTier = "banned"
			cap.Summary = "Banned"
			cap.Reason = "access_restricted"
		} else {
			cap.Verdict = domain.VerdictUnknown
			cap.Summary = "Inconclusive"
			cap.Reason = "contract_drift"
		}
		return cap
	}

	// Try extracting trace country code
	region := getOpenAIRegion(ctx, httpClient)
	if region != "" {
		cap.Region = region
		cap.Summary = cap.Summary + " (" + region + ")"
	}

	return cap
}

func getOpenAIRegion(ctx context.Context, httpClient *http.Client) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chat.openai.com/cdn-cgi/trace", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return ""
	}

	matches := openaiRe.FindSubmatch(buf.Bytes())
	if len(matches) > 1 {
		return strings.ToUpper(string(matches[1]))
	}
	return ""
}

func checkCookies(ctx context.Context, httpClient *http.Client) (bool, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/compliance/cookie_requirements", nil)
	if err != nil {
		return false, false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnavailableForLegalReasons {
		return false, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, false, nil
	}

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return false, false, err
	}
	body := buf.Bytes()
	bodyLower := bytes.ToLower(body)

	if bytes.Contains(bodyLower, []byte("unsupported_country")) {
		return false, true, nil
	}
	if len(body) == 0 || bytes.Equal(bodyLower, []byte("ok")) || bytes.Contains(bodyLower, []byte("<html>")) {
		return false, false, nil
	}

	return true, false, nil
}

func checkClient(ctx context.Context, httpClient *http.Client) (bool, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ios.chat.openai.com", nil)
	if err != nil {
		return false, false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6_0 like Mac OS X) AppleWebKit/537.36 (KHTML, like Gecko) Mobile/16G29 ChatGPT/3.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "com.openai.chatgpt")
	req.Header.Set("Referer", "https://chat.openai.com/")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://chat.openai.com")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("sec-ch-ua-mobile", "?1")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnavailableForLegalReasons {
		return false, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, false, nil
	}

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return false, false, err
	}
	bodyLower := bytes.ToLower(buf.Bytes())

	if bytes.Contains(bodyLower, []byte("unsupported_country")) ||
		bytes.Contains(bodyLower, []byte("vpn")) ||
		bytes.Contains(bodyLower, []byte("disallowed isp")) ||
		bytes.Contains(bodyLower, []byte("sorry, you have been blocked")) {
		return false, true, nil
	}
	if len(bodyLower) == 0 || bytes.Equal(bodyLower, []byte("ok")) || bytes.Contains(bodyLower, []byte("<html>")) {
		return false, false, nil
	}

	return true, false, nil
}
