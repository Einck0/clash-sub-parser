package platform_test

import (
	"context"
	"net/http"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

func TestCheckIPRisk_ScoreKeyAndRestrictionEvidence(t *testing.T) {
	scored := func(key string) string {
		return "IP Fraud Risk API\n1\n2\n3\n" + key + ": \"15\",\n\"risk\": \"low\"\n6"
	}
	for _, tc := range []struct {
		name    string
		body    string
		verdict domain.ProbeVerdict
		reason  string
		score   string
	}{
		{"valid_key", scored(`"score"`), domain.VerdictAvailable, "scamalytics_scored", "15% low"},
		{"unquoted_key", scored(`score`), domain.VerdictError, "contract_drift", ""},
		{"unmatched_key", scored(`"score`), domain.VerdictError, "contract_drift", ""},
		{"duplicated_key_quotes", scored(`""score""`), domain.VerdictError, "contract_drift", ""},
		{"challenge", `<html><div class="cf-chl-widget">Verify you are human</div></html>`, domain.VerdictRestricted, "access_restricted", ""},
		{"login_gate", `<html><form action="/login">Sign in</form></html>`, domain.VerdictRestricted, "access_restricted", ""},
		{"ordinary_login_navigation", `<nav><a href="/login">Sign in</a></nav><main>Provider temporarily changed</main>`, domain.VerdictError, "contract_drift", ""},
		{"unrecognized_page", `<html>Provider temporarily changed</html>`, domain.VerdictError, "contract_drift", ""},
		// A navigation link is not a login gate when real score evidence exists.
		{"scored_page_with_login_link", `<a href="/login">Sign in</a>` + scored(`"score"`), domain.VerdictAvailable, "scamalytics_scored", "15% low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := mockClient(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "scamalytics.com" || req.URL.Path != "/ip/1.2.3.4" {
					t.Fatalf("unexpected request %s", req.URL)
				}
				return htmlResponse(http.StatusOK, tc.body)
			})
			got := platform.CheckIPRisk(context.Background(), client, "1.2.3.4")
			if got.Verdict != tc.verdict || got.Reason != tc.reason || got.RiskScore != tc.score {
				t.Fatalf("risk evidence=%+v, want verdict=%s reason=%s score=%q", got, tc.verdict, tc.reason, tc.score)
			}
		})
	}
}
