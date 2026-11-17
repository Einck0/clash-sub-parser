package platform_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

const validWeb = `<!doctype html><html><head><title>ChatGPT</title></head><body><script>window.__reactRouterContext={}</script><script src="/cdn/assets/app.js"></script></body></html>`

func preflightClient(complianceStatus int, complianceBody string, webStatus int, webBody string, webHeader http.Header, iosStatus int, iosBody string) *http.Client {
	return mockClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.openai.com":
			return jsonResponse(complianceStatus, complianceBody)
		case "chatgpt.com":
			resp, _ := htmlResponse(webStatus, webBody)
			if webHeader != nil {
				resp.Header = webHeader
			}
			return resp, nil
		case "ios.chat.openai.com":
			return jsonResponse(iosStatus, iosBody)
		case "chat.openai.com":
			return htmlResponse(200, "loc=US\n")
		default:
			return nil, errors.New("unexpected request")
		}
	})
}
func TestCheckOpenAIRequiresBothPublicPreflightContracts(t *testing.T) {
	goodHeader := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	cases := []struct {
		name   string
		cookie string
		ws     int
		wb     string
		wh     http.Header
		iv     domain.ProbeVerdict
		want   domain.ProbeVerdict
		reason string
	}{
		{"both pass", `{"cookie_consent_required":false}`, 200, validWeb, goodHeader, domain.VerdictUnknown, domain.VerdictAvailable, "anonymous_preflight_passed"},
		{"compliance contract drift with valid web", `{}`, 200, validWeb, goodHeader, domain.VerdictUnknown, domain.VerdictError, "partial_preflight_error"},
		{"web contract drift with valid compliance", `{"cookie_consent_required":true}`, 200, "<html><title>ChatGPT</title></html>", goodHeader, domain.VerdictUnknown, domain.VerdictError, "partial_preflight_error"},
		{"challenge with valid compliance", `{"cookie_consent_required":true}`, 403, "<title>Just a moment</title>cf-chl-", goodHeader, domain.VerdictUnknown, domain.VerdictUnknown, "partial_preflight"},
		{"generic 403 with valid compliance", `{"cookie_consent_required":true}`, 403, "forbidden", goodHeader, domain.VerdictUnknown, domain.VerdictUnknown, "partial_preflight"},
		{"region denial conflicts with passing web", `{"error":{"code":"unsupported_country"}}`, 200, validWeb, goodHeader, domain.VerdictUnknown, domain.VerdictAbnormal, "conflicting_entry_evidence"},
		{"rate limit with valid compliance", `{"cookie_consent_required":true}`, 429, "", goodHeader, domain.VerdictUnknown, domain.VerdictUnknown, "partial_preflight"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := platform.CheckOpenAI(context.Background(), preflightClient(200, tc.cookie, tc.ws, tc.wb, tc.wh, 403, "unsupported_country"))
			if got.Verdict != tc.want || got.Reason != tc.reason {
				t.Fatalf("result=%+v", got)
			}
			if got.EvidenceScope != "anonymous_preflight" || got.SubTier != "" {
				t.Fatalf("scope/tier=%+v", got)
			}
			if got.Checks["legacy_ios_entry"].Verdict != tc.iv {
				t.Fatalf("iOS diagnostic gated or misclassified: %+v", got.Checks)
			}
			if got.Checks["cookie_requirements"].StatusCode == nil || *got.Checks["cookie_requirements"].StatusCode != 200 {
				t.Fatalf("status evidence missing: %+v", got.Checks)
			}
		})
	}
}
func TestCheckOpenAIDoesNotPromoteArbitraryComplianceOrWebContent(t *testing.T) {
	for _, body := range []string{`{}`, `{"cookie_consent_required":"false"}`, `null`, `OK`} {
		t.Run(strings.ReplaceAll(body, "/", "_"), func(t *testing.T) {
			got := platform.CheckOpenAI(context.Background(), preflightClient(200, body, 200, validWeb, http.Header{"Content-Type": []string{"text/html"}}, 200, "{}"))
			if got.Verdict == domain.VerdictAvailable {
				t.Fatalf("unverified compliance accepted: %+v", got)
			}
		})
	}
}
func TestCheckOpenAIAggregatesOnlyConcordantCoreEntryEvidence(t *testing.T) {
	header := http.Header{"Content-Type": []string{"text/html"}}
	cases := []struct {
		name         string
		cookieStatus int
		cookieBody   string
		webStatus    int
		webBody      string
		want         domain.ProbeVerdict
		reason       string
	}{
		{"restricted plus challenge", 403, `{"error":{"code":"unsupported_country"}}`, 403, "<title>Just a moment</title>cf-chl-", domain.VerdictUnknown, "incomplete_preflight"},
		{"both restricted", 403, `{"error":{"type":"unsupported_country_region_territory"}}`, 403, `{"error":{"code":"unsupported_country"}}`, domain.VerdictRestricted, "entry_restrictions_confirmed"},
		{"web restricted conflicts with compliance pass", 200, `{"cookie_consent_required":true}`, 403, `{"error":{"code":"unsupported_country"}}`, domain.VerdictAbnormal, "conflicting_entry_evidence"},
		{"technical failure dominates partial restriction", 403, `{"error":{"code":"unsupported_country"}}`, 503, "failure", domain.VerdictError, "partial_preflight_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := platform.CheckOpenAI(context.Background(), preflightClient(tc.cookieStatus, tc.cookieBody, tc.webStatus, tc.webBody, header, 200, "{}"))
			if got.Verdict != tc.want || got.Reason != tc.reason {
				t.Fatalf("got verdict=%s reason=%s", got.Verdict, got.Reason)
			}
		})
	}
}

func TestCheckOpenAIRequiresStructuredRegionErrorAndRejectsCrossHostRedirects(t *testing.T) {
	for _, body := range []string{"unsupported_country", `<html>unsupported_country</html>`, `{"message":"unsupported_country"}`, `<title>Just a moment</title>unsupported_country`} {
		got := platform.CheckOpenAI(context.Background(), preflightClient(403, body, 200, validWeb, http.Header{"Content-Type": []string{"text/html"}}, 200, "{}"))
		if got.Checks["cookie_requirements"].Verdict != domain.VerdictUnknown {
			t.Fatalf("unstructured body became region restriction: %s => %+v", body, got.Checks["cookie_requirements"])
		}
	}
	client := mockClient(func(req *http.Request) (*http.Response, error) {
		var resp *http.Response
		switch req.URL.Host {
		case "api.openai.com":
			resp, _ = jsonResponse(200, `{"cookie_consent_required":false}`)
			u, _ := url.Parse("https://attacker.example/fake")
			resp.Request = &http.Request{URL: u}
			return resp, nil
		case "chatgpt.com":
			resp, _ = htmlResponse(200, validWeb)
			resp.Header.Set("Content-Type", "text/html")
			return resp, nil
		case "ios.chat.openai.com":
			return jsonResponse(200, `{}`)
		case "chat.openai.com":
			return htmlResponse(200, "loc=US\n")
		default:
			return nil, errors.New("unexpected request")
		}
	})
	got := platform.CheckOpenAI(context.Background(), client)
	if got.Verdict == domain.VerdictAvailable || got.Checks["cookie_requirements"].Reason != "unexpected_redirect" {
		t.Fatalf("foreign redirect trusted: %+v", got)
	}

	traceClient := mockClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "api.openai.com":
			return jsonResponse(200, `{"cookie_consent_required":false}`)
		case "chatgpt.com":
			resp, _ := htmlResponse(200, validWeb)
			resp.Header.Set("Content-Type", "text/html")
			return resp, nil
		case "ios.chat.openai.com":
			return jsonResponse(200, `{}`)
		case "chat.openai.com":
			resp, _ := htmlResponse(200, "loc=US\n")
			u, _ := url.Parse("https://attacker.example/trace")
			resp.Request = &http.Request{URL: u}
			return resp, nil
		default:
			return nil, errors.New("unexpected request")
		}
	})
	got = platform.CheckOpenAI(context.Background(), traceClient)
	if got.Region != "" || got.RegionSource != "" {
		t.Fatalf("foreign trace response was trusted: %+v", got)
	}
}

func TestCheckOpenAINilTransport(t *testing.T) {
	got := platform.CheckOpenAI(context.Background(), nil)
	if got.Verdict != domain.VerdictError || got.Reason != "transport_error" {
		t.Fatalf("nil transport = %+v", got)
	}
}
