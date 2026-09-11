package pipeline

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultYouTubeURL   = "https://www.youtube.com/premium"
	defaultNetflixBase  = "https://www.netflix.com"
	defaultDisneyURL    = "https://www.disneyplus.com/"
	defaultBilibiliURL  = "https://api.bilibili.com/pgc/player/web/v2/playurl?ep_id=268176"
)

var youtubeCountryRe = regexp.MustCompile(`"countryCode":\s*"([A-Z]{2})"`)

// CheckStreaming runs all streaming platform evaluations concurrently within the given timeout.
func CheckStreaming(ctx context.Context, client *http.Client, timeout time.Duration) (*StreamingResult, error) {
	return CheckStreamingWithConfig(ctx, client, Config{StageTimeout: timeout})
}

// CheckStreamingWithConfig runs all streaming platform evaluations respecting custom endpoint overrides.
func CheckStreamingWithConfig(ctx context.Context, client *http.Client, cfg Config) (*StreamingResult, error) {
	timeout := cfg.StageTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	stageCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := &StreamingResult{}
	var wg sync.WaitGroup

	ytURL := cfg.YouTubeURL
	if ytURL == "" {
		ytURL = defaultYouTubeURL
	}
	nfBase := cfg.NetflixBaseURL
	if nfBase == "" {
		nfBase = defaultNetflixBase
	}
	disneyURL := cfg.DisneyURL
	if disneyURL == "" {
		disneyURL = defaultDisneyURL
	}
	biliURL := cfg.BilibiliURL
	if biliURL == "" {
		biliURL = defaultBilibiliURL
	}

	wg.Add(4)
	go func() {
		defer wg.Done()
		res, _ := CheckYouTubeWithURL(stageCtx, client, ytURL)
		result.YouTube = res
	}()

	go func() {
		defer wg.Done()
		res, _ := CheckNetflixWithBaseURL(stageCtx, client, nfBase)
		result.Netflix = res
	}()

	go func() {
		defer wg.Done()
		res, _ := CheckDisneyWithURL(stageCtx, client, disneyURL)
		result.Disney = res
	}()

	go func() {
		defer wg.Done()
		res, _ := CheckBilibiliWithURL(stageCtx, client, biliURL)
		result.Bilibili = res
	}()

	wg.Wait()
	return result, nil
}

// CheckYouTube evaluates YouTube Premium unlock capability and regional code.
func CheckYouTube(ctx context.Context, client *http.Client) (string, error) {
	return CheckYouTubeWithURL(ctx, client, defaultYouTubeURL)
}

// CheckYouTubeWithURL evaluates YouTube Premium capability against a custom URL.
func CheckYouTubeWithURL(ctx context.Context, client *http.Client, targetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "Error", err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Connection", "close")

	resp, err := client.Do(req)
	if err != nil {
		return "Failed", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "Rate Limited", nil
	}
	if resp.StatusCode == http.StatusForbidden {
		return "Blocked", nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Error", err
	}
	text := string(bodyBytes)

	if strings.Contains(text, "Before you continue") || strings.Contains(text, "unusual traffic") {
		return "Blocked", nil
	}
	if strings.Contains(text, "Premium is not available in your country") || strings.Contains(text, "YouTube Premium 在你所在的国家/地区不可用") {
		return "No", nil
	}

	match := youtubeCountryRe.FindStringSubmatch(text)
	if len(match) > 1 {
		return match[1], nil
	}

	if resp.StatusCode == http.StatusOK {
		return "Yes", nil
	}

	return "No", nil
}

// CheckNetflix evaluates Netflix streaming capability (Full, Originals Only, No, Blocked).
func CheckNetflix(ctx context.Context, client *http.Client) (string, error) {
	return CheckNetflixWithBaseURL(ctx, client, defaultNetflixBase)
}

// CheckNetflixWithBaseURL evaluates Netflix against a custom base URL.
func CheckNetflixWithBaseURL(ctx context.Context, client *http.Client, baseURL string) (string, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Check non-original licensed title: Breaking Bad (70143836)
	nonOrigURL := baseURL + "/title/70143836"
	reqNon, err := http.NewRequestWithContext(ctx, "GET", nonOrigURL, nil)
	if err != nil {
		return "Error", err
	}
	reqNon.Header.Set("User-Agent", DefaultUserAgent)
	reqNon.Header.Set("Connection", "close")

	respNon, err := client.Do(reqNon)
	if err != nil {
		return "Failed", err
	}
	defer respNon.Body.Close()

	if respNon.StatusCode == http.StatusTooManyRequests {
		return "Rate Limited", nil
	}
	if respNon.StatusCode == http.StatusForbidden {
		return "Blocked", nil
	}

	bodyNonBytes, _ := io.ReadAll(respNon.Body)
	textNon := string(bodyNonBytes)
	if strings.Contains(textNon, "cf-challenge") {
		return "Blocked", nil
	}

	nonOrigAvailable := false
	if respNon.StatusCode == http.StatusOK {
		if strings.Contains(textNon, "title-70143836") || strings.Contains(textNon, "Breaking Bad") || strings.Contains(textNon, "watch-button") {
			nonOrigAvailable = true
		}
	}

	// Check original title: Stranger Things (80018499)
	origURL := baseURL + "/title/80018499"
	reqOrig, err := http.NewRequestWithContext(ctx, "GET", origURL, nil)
	if err != nil {
		return "Error", err
	}
	reqOrig.Header.Set("User-Agent", DefaultUserAgent)
	reqOrig.Header.Set("Connection", "close")

	respOrig, err := client.Do(reqOrig)
	if err != nil {
		return "Failed", err
	}
	defer respOrig.Body.Close()

	if respOrig.StatusCode == http.StatusTooManyRequests {
		return "Rate Limited", nil
	}
	if respOrig.StatusCode == http.StatusForbidden {
		return "Blocked", nil
	}

	bodyOrigBytes, _ := io.ReadAll(respOrig.Body)
	textOrig := string(bodyOrigBytes)
	if strings.Contains(textOrig, "cf-challenge") {
		return "Blocked", nil
	}

	origAvailable := false
	if respOrig.StatusCode == http.StatusOK {
		if strings.Contains(textOrig, "title-80018499") || strings.Contains(textOrig, "Stranger Things") || strings.Contains(textOrig, "watch-button") || strings.Contains(textOrig, "Netflix Original") {
			origAvailable = true
		}
	}

	if nonOrigAvailable && origAvailable {
		return "Full", nil
	}
	if origAvailable {
		return "Originals Only", nil
	}

	return "No", nil
}

// CheckDisney evaluates Disney+ streaming capability through landing redirect check.
func CheckDisney(ctx context.Context, client *http.Client) (string, error) {
	return CheckDisneyWithURL(ctx, client, defaultDisneyURL)
}

// CheckDisneyWithURL evaluates Disney+ against a custom URL.
func CheckDisneyWithURL(ctx context.Context, client *http.Client, targetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "Error", err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Connection", "close")

	resp, err := client.Do(req)
	if err != nil {
		return "Failed", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "Rate Limited", nil
	}
	if resp.StatusCode == http.StatusForbidden {
		return "Blocked", nil
	}

	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "unavailable") || strings.Contains(loc, "preview") {
		return "No", nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	text := string(bodyBytes)

	if resp.Request != nil && strings.Contains(resp.Request.URL.Path, "unavailable") {
		return "No", nil
	}
	if strings.Contains(text, "unavailable") || strings.Contains(text, "preview") {
		return "No", nil
	}

	if strings.Contains(text, "Disney") || strings.Contains(text, "disneyplus") {
		return "Yes", nil
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		return "Yes", nil
	}

	return "No", nil
}

// CheckBilibili evaluates Bilibili regional content restriction (HK/MO/TW vs Mainland).
func CheckBilibili(ctx context.Context, client *http.Client) (string, error) {
	return CheckBilibiliWithURL(ctx, client, defaultBilibiliURL)
}

// CheckBilibiliWithURL evaluates Bilibili against a custom URL.
func CheckBilibiliWithURL(ctx context.Context, client *http.Client, targetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "Error", err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Connection", "close")

	resp, err := client.Do(req)
	if err != nil {
		return "Failed", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "Rate Limited", nil
	}
	if resp.StatusCode == http.StatusForbidden {
		return "Blocked", nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Error", err
	}

	var m struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(bodyBytes, &m); err == nil {
		if m.Code == 0 {
			return "港澳台限定", nil
		}
		if m.Code == -10403 {
			return "仅大陆", nil
		}
	}

	return "No", nil
}
