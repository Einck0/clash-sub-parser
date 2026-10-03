package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

var (
	disneyRegionRe    = regexp.MustCompile(`"countryCode"\s*:\s*"([^"]+)"`)
	disneySupportedRe = regexp.MustCompile(`"inSupportedLocation"\s*:\s*(false|true)`)
	disneyMainPageRe  = regexp.MustCompile(`region"\s*:\s*"([^"]+)`)
)

// CheckDisney detects Disney+ unlock state via BAMGrid assertion tokens and GraphQL.
func CheckDisney(ctx context.Context, httpClient *http.Client) domain.PlatformCapability {
	now := time.Now().UTC()
	start := time.Now()

	const (
		cookie    = "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange&latitude=0&longitude=0&platform=browser&subject_token=DISNEYASSERTION&subject_token_type=urn%3Abamtech%3Aparams%3Aoauth%3Atoken-type%3Adevice"
		assertion = `{"deviceFamily":"browser","applicationRuntime":"chrome","deviceProfile":"windows","attributes":{}}`
		authBear  = "Bearer ZGlzbmV5JmJyb3dzZXImMS4wLjA.Cu56AgSfBTDag5NiRA81oLHkDZfu5L3CKadnefEAY84"
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
	)

	latency := func() int64 {
		return time.Since(start).Milliseconds()
	}

	cap := domain.PlatformCapability{
		ObservedAt: &now,
	}

	// 1. Get assertion token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://disney.api.edge.bamgrid.com/devices", strings.NewReader(assertion))
	if err != nil {
		lat := latency()
		cap.LatencyMS = &lat
		cap.Verdict = domain.VerdictError
		cap.Reason = "request_build_failed"
		return cap
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", authBear)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	lat := latency()
	cap.LatencyMS = &lat
	if err != nil {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Error"
		cap.Reason = "network_error"
		return cap
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Banned"
		cap.Reason = "devices_403"
		return cap
	}

	var assertionResp map[string]interface{}
	if err := readJSONPooled(resp.Body, &assertionResp); err != nil {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Error"
		cap.Reason = "json_decode_failed"
		return cap
	}

	assertionToken, ok := assertionResp["assertion"].(string)
	if !ok || assertionToken == "" {
		// Fallback to main page
		return checkDisneyMainPage(ctx, httpClient, cap)
	}

	// 2. Exchange token
	tokenData := strings.Replace(cookie, "DISNEYASSERTION", assertionToken, 1)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, "https://disney.api.edge.bamgrid.com/token", strings.NewReader(tokenData))
	if err != nil {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", authBear)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err = httpClient.Do(req)
	if err != nil {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}
	defer resp.Body.Close()

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}
	tokenBody := buf.Bytes()

	if bytes.Contains(tokenBody, []byte("forbidden-location")) || bytes.Contains(tokenBody, []byte("403 ERROR")) {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Banned"
		cap.Reason = "forbidden_location"
		return cap
	}

	var tokenResp map[string]interface{}
	if err := json.Unmarshal(tokenBody, &tokenResp); err != nil {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}

	refreshToken, ok := tokenResp["refresh_token"].(string)
	if !ok || refreshToken == "" {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}

	// 3. GraphQL device refresh
	gqlQuery := fmt.Sprintf(`{"query":"mutation refreshToken($input: RefreshTokenInput!) {refreshToken(refreshToken: $input) {activeSession {sessionId}}}","variables":{"input":{"refreshToken":"%s"}}}`, refreshToken)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, "https://disney.api.edge.bamgrid.com/graph/v1/device/graphql", strings.NewReader(gqlQuery))
	if err != nil {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", authBear)

	resp, err = httpClient.Do(req)
	if err != nil {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}
	defer resp.Body.Close()

	gqlBuf := getPooledBuf()
	defer putPooledBuf(gqlBuf)
	if _, err := gqlBuf.ReadFrom(resp.Body); err != nil {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}
	gqlBody := gqlBuf.Bytes()

	regionMatch := disneyRegionRe.FindSubmatch(gqlBody)
	if len(regionMatch) < 2 {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}
	region := strings.ToUpper(string(regionMatch[1]))

	// Special case: Japan does not return inSupportedLocation=true in GraphQL, but is unlocked
	if region == "JP" {
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "unlocked"
		cap.Region = region
		cap.Summary = "Unlocked (JP)"
		cap.Reason = "jp_unlocked"
		return cap
	}

	supportedMatch := disneySupportedRe.FindSubmatch(gqlBody)
	if len(supportedMatch) < 2 {
		return checkDisneyMainPage(ctx, httpClient, cap)
	}

	if string(supportedMatch[1]) == "true" {
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "unlocked"
		cap.Region = region
		cap.Summary = fmt.Sprintf("Unlocked (%s)", region)
		cap.Reason = "supported_location"
	} else {
		// Region coming soon: must NOT be reported as unlocked; verdict is restricted
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "soon"
		cap.Region = region
		cap.Summary = fmt.Sprintf("Soon (%s)", region)
		cap.Reason = "location_coming_soon"
	}

	return cap
}

func checkDisneyMainPage(ctx context.Context, httpClient *http.Client, cap domain.PlatformCapability) domain.PlatformCapability {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.disneyplus.com/", nil)
	if err != nil {
		cap.Verdict = domain.VerdictRestricted
		cap.SubTier = "banned"
		cap.Summary = "Inaccessible"
		cap.Reason = "main_page_req_failed"
		return cap
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Error"
		cap.Reason = "main_page_dial_failed"
		return cap
	}
	defer resp.Body.Close()

	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		cap.Verdict = domain.VerdictError
		cap.Summary = "Error"
		cap.Reason = "main_page_read_failed"
		return cap
	}

	match := disneyMainPageRe.FindSubmatch(buf.Bytes())
	if len(match) > 1 {
		region := strings.ToUpper(string(match[1]))
		cap.Verdict = domain.VerdictAvailable
		cap.SubTier = "unlocked"
		cap.Region = region
		cap.Summary = fmt.Sprintf("Unlocked (%s)", region)
		cap.Reason = "main_page_region_matched"
		return cap
	}

	cap.Verdict = domain.VerdictRestricted
	cap.SubTier = "banned"
	cap.Summary = "Banned"
	cap.Reason = "disney_inaccessible"
	return cap
}
