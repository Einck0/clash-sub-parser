package pipeline

import (
	"bufio"
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
	defaultChatGPTURL   = "https://ios.chat.openai.com/public-api/mobile/server_status/v1"
	defaultGeminiURL    = "https://gemini.google.com/"
	defaultAIStudioURL  = "https://generativelanguage.googleapis.com/v1beta/models?key=AIzaSyDummyCheckKey"
	defaultClaudeURL    = "https://claude.ai/cdn-cgi/trace"
)

var (
	geminiAlpha3Re = regexp.MustCompile(`,2,1,200,"([A-Z]{3})"`)
	geminiBlockedAlpha3 = map[string]bool{
		"CHN": true, "RUS": true, "BLR": true, "CUB": true,
		"IRN": true, "PRK": true, "SYR": true, "HKG": true, "MAC": true,
	}
	alpha3ToAlpha2 = map[string]string{
		"USA": "US", "GBR": "GB", "JPN": "JP", "SGP": "SG", "DEU": "DE",
		"FRA": "FR", "CAN": "CA", "AUS": "AU", "TWN": "TW", "KOR": "KR",
		"IND": "IN", "NLD": "NL", "SWE": "SE", "CHE": "CH", "HKG": "HK",
		"CHN": "CN", "MAC": "MO", "RUS": "RU", "BRA": "BR", "ZAF": "ZA",
		"MEX": "MX", "MYS": "MY", "THA": "TH", "VNM": "VN", "IDN": "ID",
		"PHL": "PH", "NZL": "NZ", "IRL": "IE", "ITA": "IT", "ESP": "ES",
	}
)

// CheckAI runs all AI service capability evaluations concurrently.
func CheckAI(ctx context.Context, client *http.Client, timeout time.Duration) (*AIResult, error) {
	return CheckAIWithConfig(ctx, client, Config{StageTimeout: timeout})
}

// CheckAIWithConfig runs all AI service capability evaluations respecting custom endpoint overrides.
func CheckAIWithConfig(ctx context.Context, client *http.Client, cfg Config) (*AIResult, error) {
	timeout := cfg.StageTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	stageCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := &AIResult{}
	var wg sync.WaitGroup

	gptURL := cfg.ChatGPTURL
	if gptURL == "" {
		gptURL = defaultChatGPTURL
	}
	gemURL := cfg.GeminiURL
	if gemURL == "" {
		gemURL = defaultGeminiURL
	}
	claudeURL := cfg.ClaudeURL
	if claudeURL == "" {
		claudeURL = defaultClaudeURL
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		res, _ := CheckChatGPTWithURL(stageCtx, client, gptURL)
		result.ChatGPT = res
	}()

	go func() {
		defer wg.Done()
		res, _ := CheckGeminiWithURL(stageCtx, client, gemURL)
		if res == "No" || res == "Failed" || res == "Blocked" {
			// Fallback to AI Studio geofence check
			aiRes, err := CheckAIStudio(stageCtx, client)
			if err == nil && aiRes == "Yes" {
				res = "Yes (AI Studio)"
			}
		}
		result.Gemini = res
	}()

	go func() {
		defer wg.Done()
		res, _ := CheckClaudeWithURL(stageCtx, client, claudeURL)
		result.Claude = res
	}()

	wg.Wait()
	return result, nil
}

// CheckChatGPT evaluates OpenAI / ChatGPT connectivity and accessibility.
func CheckChatGPT(ctx context.Context, client *http.Client) (string, error) {
	return CheckChatGPTWithURL(ctx, client, defaultChatGPTURL)
}

// CheckChatGPTWithURL evaluates ChatGPT status against a custom endpoint.
func CheckChatGPTWithURL(ctx context.Context, client *http.Client, targetURL string) (string, error) {
	// Mode 1: Web
	reqWeb, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "Error", err
	}
	reqWeb.Header.Set("User-Agent", DefaultUserAgent)
	reqWeb.Header.Set("Connection", "close")

	respWeb, err := client.Do(reqWeb)
	if err != nil {
		return "Failed", err
	}
	defer respWeb.Body.Close()

	if respWeb.StatusCode == http.StatusTooManyRequests {
		return "Rate Limited", nil
	}
	if respWeb.StatusCode == http.StatusForbidden || respWeb.StatusCode == 1020 {
		return "Blocked", nil
	}

	bodyWeb, err := io.ReadAll(respWeb.Body)
	if err != nil {
		return "Error", err
	}
	textWeb := string(bodyWeb)
	if strings.Contains(textWeb, "Cloudflare") || strings.Contains(textWeb, "cf-challenge") || strings.Contains(textWeb, "Access Denied") {
		return "Blocked", nil
	}

	var mWeb struct {
		Status string `json:"status"`
	}
	webOK := false
	if err := json.Unmarshal(bodyWeb, &mWeb); err == nil {
		if strings.EqualFold(mWeb.Status, "normal") || strings.EqualFold(mWeb.Status, "ok") {
			webOK = true
		}
	}

	if !webOK {
		return "No", nil
	}

	// Mode 2: App
	reqApp, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "Yes (Web)", nil
	}
	reqApp.Header.Set("User-Agent", DefaultUserAgent)
	reqApp.Header.Set("X-Requested-With", "com.openai.chatgpt")
	reqApp.Header.Set("Connection", "close")

	respApp, err := client.Do(reqApp)
	if err != nil {
		return "Yes (Web)", nil
	}
	defer respApp.Body.Close()

	if respApp.StatusCode == http.StatusOK {
		var mApp struct {
			Status string `json:"status"`
		}
		bodyA, _ := io.ReadAll(respApp.Body)
		if err := json.Unmarshal(bodyA, &mApp); err == nil {
			if strings.EqualFold(mApp.Status, "normal") || strings.EqualFold(mApp.Status, "ok") {
				return "Yes (App)", nil
			}
		}
	}

	return "Yes (Web)", nil
}

// CheckGemini evaluates Google Gemini service accessibility with country code recognition.
func CheckGemini(ctx context.Context, client *http.Client) (string, error) {
	return CheckGeminiWithURL(ctx, client, defaultGeminiURL)
}

// CheckGeminiWithURL evaluates Google Gemini against a custom URL.
func CheckGeminiWithURL(ctx context.Context, client *http.Client, targetURL string) (string, error) {
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

	match := geminiAlpha3Re.FindStringSubmatch(text)
	if len(match) > 1 {
		alpha3 := strings.ToUpper(match[1])
		if geminiBlockedAlpha3[alpha3] {
			return "No", nil
		}
		alpha2 := alpha3ToAlpha2[alpha3]
		if alpha2 == "" {
			alpha2 = alpha3
		}
		return "Yes (" + alpha2 + ")", nil
	}

	if strings.Contains(text, "unavailable") || strings.Contains(text, "Gemini isn't currently supported in your country") {
		return "No", nil
	}

	if resp.StatusCode == http.StatusOK && (strings.Contains(text, "Google Gemini") || strings.Contains(text, "Gemini")) {
		return "Yes", nil
	}

	return "No", nil
}

// CheckAIStudio evaluates Google AI Studio geofencing using zero-credential request.
func CheckAIStudio(ctx context.Context, client *http.Client) (string, error) {
	return CheckAIStudioWithURL(ctx, client, defaultAIStudioURL)
}

// CheckAIStudioWithURL evaluates AI Studio geofence against a custom URL.
func CheckAIStudioWithURL(ctx context.Context, client *http.Client, targetURL string) (string, error) {
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

	bodyBytes, _ := io.ReadAll(resp.Body)
	text := string(bodyBytes)

	if resp.StatusCode == http.StatusBadRequest {
		if strings.Contains(text, "API_KEY_INVALID") || strings.Contains(text, "API key not valid") {
			// Geofence check runs before API key validation; reaching API_KEY_INVALID means location is allowed!
			return "Yes", nil
		}
		if strings.Contains(text, "FAILED_PRECONDITION") || strings.Contains(text, "User location is not supported") {
			return "No", nil
		}
	}

	if resp.StatusCode == http.StatusOK {
		return "Yes", nil
	}

	return "No", nil
}

// CheckClaude evaluates Claude regional access signal from cloudflare edge trace.
func CheckClaude(ctx context.Context, client *http.Client) (string, error) {
	return CheckClaudeWithURL(ctx, client, defaultClaudeURL)
}

// CheckClaudeWithURL evaluates Claude trace against a custom URL.
func CheckClaudeWithURL(ctx context.Context, client *http.Client, targetURL string) (string, error) {
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

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "loc=") {
			loc := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(line, "loc=")))
			if len(loc) == 2 {
				return "Yes (" + loc + ")", nil
			}
		}
	}

	return "No", nil
}
