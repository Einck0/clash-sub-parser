package platform

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"github.com/biter777/countries"
)

var geminiRe = regexp.MustCompile(`,2,1,200,"([A-Z]{3})"`)

// Snapshot of ISO alpha-3 entries directly mapped from the official supported
// countries table (retrieved 2026-11-06). Mainland China is excluded because
// the page qualifies it as Workspace-only; ambiguous names stay unverified.
var geminiSupportedRegion = map[string]bool{}

func init() {
	for _, code := range strings.Fields(`ABW AGO AIA ALA ALB AND ARE ARG ARM ASM ATA ATF ATG AUS AUT AZE BDI BEL BEN BES BFA BGD BGR BHS BHR BIH BLM BLZ BMU BOL BRA BRB BRN BTN BWA CAF CAN CCK CHE CHL CIV CMR COD COG COK COL COM CPV CRI CUW CXR CYM CYP CZE DEU DJI DMA DNK DOM DZA ECU EGY ERI ESH ESP EST ETH FIN FJI FLK FRA FRO GAB GBR GEO GGY GHA GIB GIN GLP GMB GNB GNQ GRC GRD GRL GTM GUF GUM GUY HKG HMD FSM HND HRV HTI HUN IDN IMN IND IOT IRL IRQ ISL ISR ITA JAM JEY JOR JPN KAZ KEN KGZ KHM KIR KNA KOR KWT LAO LBN LBR LBY LCA LIE LKA LSO LTU LUX LVA MAR MAF MCO MDA MDG MDV MEX MHL MKD MLI MLT MMR MNE MNG MNP MOZ MRT MSR MTQ MUS MWI MYS MYT NAM NCL NER NFK NGA NIC NIU NLD NOR NPL NRU NZL OMN PAK PAN PER PHL PLW PNG POL PCN PRI PRT PRY PSE PYF QAT REU ROU RWA SAU SDN SEN SGP SGS SHN SJM SLB SLE SLV SMR SOM SPM SRB SSD STP SUR SVK SVN SWE SWZ SXM SYC TCA TCD TGO THA TJK TKL TKM TLS TON TTO TUN TUR TUV TZA UGA UKR UMI URY USA UZB VAT VCT VEN VGB VIR VNM VUT WLF WSM XKX YEM ZAF ZMB ZWE MAC TWN`) {
		geminiSupportedRegion[code] = true
	}
}

const geminiMarkerEvidence = `,2,1,200,"`

func alpha3ToAlpha2(alpha3 string) string {
	code := strings.ToUpper(strings.TrimSpace(alpha3))
	country := countries.ByName(code)
	if country == countries.Unknown {
		return ""
	}
	return country.Alpha2()
}

func validFinalOrigin(resp *http.Response, host string) bool {
	if resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	u := resp.Request.URL
	return strings.EqualFold(u.Scheme, "https") && strings.EqualFold(u.Hostname(), host) && u.Port() == ""
}

func CheckGemini(ctx context.Context, httpClient *http.Client) domain.PlatformCapability {
	now := time.Now().UTC()
	start := time.Now()
	result := func(verdict domain.ProbeVerdict, reason, summary string) domain.PlatformCapability {
		latency := time.Since(start).Milliseconds()
		return domain.PlatformCapability{Verdict: verdict, LatencyMS: &latency, ObservedAt: &now, Reason: reason, Summary: summary}
	}
	if httpClient == nil {
		return result(domain.VerdictUnknown, "transport_error", "Inconclusive")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gemini.google.com/", nil)
	if err != nil {
		return result(domain.VerdictUnknown, "request_build_failed", "Inconclusive")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CSP probe)")
	resp, err := httpClient.Do(req)
	if err != nil {
		reason := ResponseReason(err)
		verdict := domain.VerdictError
		if reason == "cancelled" || reason == "deadline_exceeded" || reason == "budget_not_executed" {
			verdict = domain.VerdictUnknown
		}
		return result(verdict, reason, "Inconclusive")
	}
	defer resp.Body.Close()
	if !validFinalOrigin(resp, "gemini.google.com") {
		return result(domain.VerdictUnknown, "unexpected_redirect", "Inconclusive")
	}
	if strings.Contains(strings.ToLower(resp.Request.URL.Path), "login") {
		return result(domain.VerdictUnknown, "unexpected_redirect", "Inconclusive")
	}
	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusUnavailableForLegalReasons:
		return result(domain.VerdictRestricted, "access_restricted", "Access restricted")
	case http.StatusTooManyRequests:
		return result(domain.VerdictUnknown, "rate_limited", "Inconclusive")
	case http.StatusOK:
	default:
		return result(domain.VerdictUnknown, "unexpected_status", "Inconclusive")
	}
	body, truncated, err := readBoundedBody(resp.Body, detectorBodyLimit)
	if err != nil {
		verdict, reason := responseReadFailure(err)
		return result(verdict, reason, "Inconclusive")
	}
	if truncated {
		return result(domain.VerdictUnknown, "response_body_truncated", "Inconclusive")
	}
	if hasGoogleAccessGate(body) {
		return result(domain.VerdictUnknown, "challenge_or_login", "Inconclusive")
	}
	if !strings.Contains(string(body), geminiMarkerEvidence) {
		return result(domain.VerdictUnknown, "gemini_marker_missing", "Inconclusive")
	}
	matches := geminiRe.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return result(domain.VerdictUnknown, "gemini_marker_missing", "Inconclusive")
	}
	codes := make(map[string]bool)
	for _, match := range matches {
		code := string(match[1])
		if alpha3ToAlpha2(code) == "" {
			return result(domain.VerdictUnknown, "unknown_region_code", "Inconclusive")
		}
		codes[code] = true
	}
	if len(codes) != 1 {
		return result(domain.VerdictUnknown, "conflicting_region_markers", "Inconclusive")
	}
	var code string
	for code = range codes {
	}
	region := alpha3ToAlpha2(code)
	if !geminiSupportedRegion[code] {
		cap := result(domain.VerdictUnknown, "region_support_unverified", fmt.Sprintf("Gemini region support unverified (%s)", region))
		cap.Region = region
		return cap
	}
	cap := result(domain.VerdictAvailable, "gemini_web_reachable", fmt.Sprintf("Gemini web reachable (%s); account eligibility unverified", region))
	cap.Region = region
	cap.SubTier = "web_reachable"
	return cap
}
