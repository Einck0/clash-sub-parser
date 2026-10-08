package profiles_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/profiles"
)

func TestAllProfilesAreVersionedAndHaveContracts(t *testing.T) {
	all := profiles.All()
	if len(all) != 5 {
		t.Fatalf("profiles = %d, want 5", len(all))
	}
	for _, profile := range all {
		if err := profile.Validate(); err != nil {
			t.Errorf("%s: invalid profile: %v", profile.Kind, err)
		}
		if profile.Version == "" || profile.Contract == "" {
			t.Errorf("%s must declare version and contract", profile.Kind)
		}
	}
}

func TestHTTP200WithoutSemanticContractIsNotAvailable(t *testing.T) {
	profile := profiles.Baseline()
	result := profile.Evaluate(profiles.Result{StatusCode: 200})
	if result.Verdict == domain.VerdictAvailable {
		t.Fatal("HTTP 200 alone must not be available")
	}
}

func TestChallengeAndLoginAreRestricted(t *testing.T) {
	for _, profile := range profiles.All() {
		if profile.Kind == domain.ProbeKindBaseline {
			continue
		} // v2 intentionally accepts arbitrary 2xx bodies
		for _, body := range []string{
			"<html>Just a moment... cf-chl-turnstile</html>",
			"<html><form action='/login'>Sign in</form></html>",
		} {
			got := profile.Evaluate(profiles.Result{StatusCode: 200, Body: []byte(body)})
			if got.Verdict != domain.VerdictRestricted {
				t.Errorf("%s body %q verdict = %s, want restricted", profile.Kind, body, got.Verdict)
			}
		}
	}
}

func TestContractDriftIsUnknown(t *testing.T) {
	profile := profiles.Streaming()
	got := profile.Evaluate(profiles.Result{
		StatusCode:      200,
		ContractMatched: true,
		ContractVersion: "streaming-v0",
	})
	if got.Verdict != domain.VerdictUnknown {
		t.Fatalf("drift verdict = %s, want unknown", got.Verdict)
	}
}

func TestSpeedRequiresOptInAndBudgets(t *testing.T) {
	profile := profiles.Speed()
	if profile.SpeedBudget.OptInRequired != true {
		t.Fatal("speed profile must require explicit opt-in")
	}
	if got := profile.Evaluate(profiles.Result{StatusCode: 200, ContractMatched: true}); got.Verdict == domain.VerdictAvailable {
		t.Fatal("speed must not run without opt-in")
	}
	if got := profile.Evaluate(profiles.Result{
		StatusCode:      200,
		ContractMatched: true,
		OptIn:           true,
		BytesRead:       profile.SpeedBudget.MaxBytesPerRequest + 1,
	}); got.Verdict != domain.VerdictError {
		t.Fatalf("over-budget verdict = %s, want error", got.Verdict)
	}
}

func TestStaleResultIsClassifiedByObservationServiceInput(t *testing.T) {
	if profiles.Baseline().MaxAge <= 0 {
		t.Fatal("baseline must declare a freshness window")
	}
	_ = time.Now()
}

func TestIPRiskProfileIsVersionedAndHasContract(t *testing.T) {
	profile := profiles.IPRisk()
	if err := profile.Validate(); err != nil {
		t.Fatalf("IPRisk profile validation failed: %v", err)
	}
	if profile.Kind != domain.ProbeKindIPRisk {
		t.Fatalf("IPRisk kind = %s, want %s", profile.Kind, domain.ProbeKindIPRisk)
	}
	if profile.Version == "" || profile.Contract == "" {
		t.Fatal("IPRisk profile must declare version and contract")
	}
	if profile.MaxAge <= 0 {
		t.Fatal("IPRisk profile must declare a freshness window")
	}
}

func TestIPRiskChallengeAndRestrictionProduceUnknown(t *testing.T) {
	profile := profiles.IPRisk()
	for _, body := range []string{
		"<html>Just a moment... cf-chl-turnstile</html>",
		"<html><form action='/login'>Sign in</form></html>",
		"<html>human verification challenge</html>",
	} {
		got := profile.Evaluate(profiles.Result{StatusCode: 200, Body: []byte(body)})
		if got.Verdict != domain.VerdictUnknown {
			t.Errorf("IPRisk challenge verdict = %s, want unknown", got.Verdict)
		}
	}

	for _, code := range []int{401, 403} {
		got := profile.Evaluate(profiles.Result{StatusCode: code})
		if got.Verdict != domain.VerdictUnknown {
			t.Errorf("IPRisk status %d verdict = %s, want unknown", code, got.Verdict)
		}
	}
}

func TestIPRiskContractDriftProducesUnknown(t *testing.T) {
	profile := profiles.IPRisk()
	got := profile.Evaluate(profiles.Result{
		StatusCode:      200,
		ContractMatched: true,
		ContractVersion: "ip-risk-v0",
	})
	if got.Verdict != domain.VerdictUnknown {
		t.Fatalf("IPRisk drift verdict = %s, want unknown", got.Verdict)
	}
}

func TestIPRiskTimeoutAndTransportErrorProduceError(t *testing.T) {
	profile := profiles.IPRisk()
	for _, tc := range []struct {
		name   string
		result profiles.Result
	}{
		{"deadline_exceeded", profiles.Result{DeadlineExceeded: true}},
		{"dns_error", profiles.Result{DNSError: true}},
		{"network_error", profiles.Result{NetworkError: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := profile.Evaluate(tc.result)
			if got.Verdict != domain.VerdictError {
				t.Fatalf("IPRisk %s verdict = %s, want error", tc.name, got.Verdict)
			}
		})
	}
}

func TestIPRiskMissingExitIdentityProducesUnknown(t *testing.T) {
	profile := profiles.IPRisk()
	got := profile.Evaluate(profiles.Result{
		StatusCode:          200,
		ContractMatched:     true,
		ContractVersion:     profile.Contract,
		ExitIdentityMissing: true,
	})
	if got.Verdict != domain.VerdictUnknown {
		t.Fatalf("IPRisk missing exit identity verdict = %s, want unknown", got.Verdict)
	}
}

func TestIPRiskExitIdentityWithoutRiskProviderProducesUnknown(t *testing.T) {
	profile := profiles.IPRisk()
	for _, tc := range []struct {
		name            string
		contractMatched bool
		body            string
	}{
		{
			name:            "trace_exit_identity_only_contract_unmatched",
			contractMatched: false,
			body:            "ip=203.0.113.10\nloc=SG\n",
		},
		{
			name:            "trace_exit_identity_only_even_if_caller_sets_matched",
			contractMatched: true,
			body:            "ip=203.0.113.10\nloc=SG\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := profile.Evaluate(profiles.Result{
				StatusCode:      200,
				Body:            []byte(tc.body),
				ContractMatched: tc.contractMatched,
				ContractVersion: profile.Contract,
			})
			if got.Verdict != domain.VerdictUnknown || got.Reason != "contract_drift" {
				t.Fatalf("IPRisk verdict = %s (reason=%s), want unknown (reason=contract_drift)", got.Verdict, got.Reason)
			}
		})
	}
}

func TestBaselineNormalizedAny2xxBodyContract(t *testing.T) {
	profile := profiles.Baseline()

	t.Run("204_empty_body_is_available", func(t *testing.T) {
		got := profile.Evaluate(profiles.Result{
			StatusCode:      204,
			Body:            nil,
			BytesRead:       0,
			ContractMatched: true,
			ContractVersion: profile.Contract,
		})
		if got.Verdict != domain.VerdictAvailable || got.Reason != "contract_matched" {
			t.Fatalf("baseline 204 empty verdict = %s (reason=%s), want available (reason=contract_matched)", got.Verdict, got.Reason)
		}
	})

	for _, tc := range []struct {
		name       string
		statusCode int
		body       []byte
		bytesRead  int64
	}{
		{name: "200_empty", statusCode: 200, body: nil, bytesRead: 0},
		{name: "200_ok_text", statusCode: 200, body: []byte("OK"), bytesRead: 2},
		{name: "200_captive_portal_html", statusCode: 200, body: []byte("<html><body>Welcome to Airport Wi-Fi</body></html>"), bytesRead: 50},
		{name: "204_non_empty_body", statusCode: 204, body: []byte("unexpected"), bytesRead: 10},
		{name: "204_non_zero_bytes_read", statusCode: 204, body: nil, bytesRead: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := profile.Evaluate(profiles.Result{
				StatusCode:      tc.statusCode,
				Body:            tc.body,
				BytesRead:       tc.bytesRead,
				ContractMatched: true,
				ContractVersion: profile.Contract,
			})
			if got.Verdict != domain.VerdictAvailable || got.Reason != "contract_matched" {
				t.Fatalf("baseline %s verdict = %s (reason=%s), want available (reason=contract_matched)", tc.name, got.Verdict, got.Reason)
			}
		})
	}
}

func TestGeoRequiresValidExitIPAndAlpha2Country(t *testing.T) {
	profile := profiles.Geo()

	for _, tc := range []struct {
		name       string
		statusCode int
		body       string
		want       domain.ProbeVerdict
		wantReason string
	}{
		{
			name:       "valid_json_ipv4_and_country",
			statusCode: 200,
			body:       `{"ip":"203.0.113.10","country_code":"SG","asn":13335}`,
			want:       domain.VerdictAvailable,
			wantReason: "contract_matched",
		},
		{
			name:       "valid_trace_ipv6_and_country",
			statusCode: 200,
			body:       "ip=2001:db8::1\nloc=JP\n",
			want:       domain.VerdictAvailable,
			wantReason: "contract_matched",
		},
		{
			name:       "204_empty_rejected",
			statusCode: 204,
			body:       "",
			want:       domain.VerdictUnknown,
			wantReason: "contract_drift",
		},
		{
			name:       "200_empty_rejected",
			statusCode: 200,
			body:       "",
			want:       domain.VerdictUnknown,
			wantReason: "contract_drift",
		},
		{
			name:       "200_ok_text_rejected",
			statusCode: 200,
			body:       "OK",
			want:       domain.VerdictUnknown,
			wantReason: "contract_drift",
		},
		{
			name:       "missing_ip_rejected",
			statusCode: 200,
			body:       `{"country_code":"US"}`,
			want:       domain.VerdictUnknown,
			wantReason: "contract_drift",
		},
		{
			name:       "invalid_ip_rejected",
			statusCode: 200,
			body:       `{"ip":"999.999.999.999","country_code":"US"}`,
			want:       domain.VerdictUnknown,
			wantReason: "contract_drift",
		},
		{
			name:       "invalid_country_code_length_rejected",
			statusCode: 200,
			body:       `{"ip":"203.0.113.10","country_code":"USA"}`,
			want:       domain.VerdictUnknown,
			wantReason: "contract_drift",
		},
		{
			name:       "non_alpha_country_code_rejected",
			statusCode: 200,
			body:       `{"ip":"203.0.113.10","country_code":"12"}`,
			want:       domain.VerdictUnknown,
			wantReason: "contract_drift",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := profile.Evaluate(profiles.Result{
				StatusCode:      tc.statusCode,
				Body:            []byte(tc.body),
				BytesRead:       int64(len(tc.body)),
				ContractMatched: true,
				ContractVersion: profile.Contract,
			})
			if got.Verdict != tc.want || got.Reason != tc.wantReason {
				t.Fatalf("geo %s = (%s, %s), want (%s, %s)", tc.name, got.Verdict, got.Reason, tc.want, tc.wantReason)
			}
		})
	}
}

func TestStreamingAndAINeverProduceAvailableWithoutVerifiedCapabilityContract(t *testing.T) {
	for _, prof := range []profiles.Profile{profiles.Streaming(), profiles.AI()} {
		t.Run(string(prof.Kind), func(t *testing.T) {
			for _, status := range []int{401, 403, 451} {
				got := prof.Evaluate(profiles.Result{StatusCode: status})
				if got.Verdict != domain.VerdictRestricted || got.Reason != "access_restricted" {
					t.Fatalf("%s status %d = (%s, %s), want (restricted, access_restricted)", prof.Kind, status, got.Verdict, got.Reason)
				}
			}

			for _, body := range []string{"", "OK", "<html><head><title>Home</title></head><body>Welcome</body></html>"} {
				for _, status := range []int{200, 204} {
					got := prof.Evaluate(profiles.Result{
						StatusCode:      status,
						Body:            []byte(body),
						BytesRead:       int64(len(body)),
						ContractMatched: true,
						ContractVersion: prof.Contract,
					})
					if got.Verdict != domain.VerdictUnknown || got.Reason != "contract_drift" {
						t.Fatalf("%s status %d body %q = (%s, %s), want (unknown, contract_drift)", prof.Kind, status, body, got.Verdict, got.Reason)
					}
				}
			}
		})
	}
}

func TestSpeedRejectsTinyPayloadsAndAcceptsValidPayload(t *testing.T) {
	profile := profiles.Speed()

	for _, tc := range []struct {
		name       string
		statusCode int
		bytesRead  int64
		want       domain.ProbeVerdict
		wantReason string
	}{
		{name: "204_empty", statusCode: 204, bytesRead: 0, want: domain.VerdictUnknown, wantReason: "contract_drift"},
		{name: "200_empty", statusCode: 200, bytesRead: 0, want: domain.VerdictUnknown, wantReason: "contract_drift"},
		{name: "200_tiny_2_bytes", statusCode: 200, bytesRead: 2, want: domain.VerdictUnknown, wantReason: "contract_drift"},
		{name: "200_below_threshold_1023", statusCode: 200, bytesRead: profiles.MinValidSpeedBytes - 1, want: domain.VerdictUnknown, wantReason: "contract_drift"},
		{name: "200_exact_threshold_1024", statusCode: 200, bytesRead: profiles.MinValidSpeedBytes, want: domain.VerdictAvailable, wantReason: "contract_matched"},
		{name: "200_valid_4096", statusCode: 200, bytesRead: 4096, want: domain.VerdictAvailable, wantReason: "contract_matched"},
		{name: "200_over_request_budget", statusCode: 200, bytesRead: profile.SpeedBudget.MaxBytesPerRequest + 1, want: domain.VerdictError, wantReason: "speed_budget_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := profile.Evaluate(profiles.Result{
				StatusCode:      tc.statusCode,
				OptIn:           true,
				BytesRead:       tc.bytesRead,
				ContractMatched: true,
				ContractVersion: profile.Contract,
			})
			if got.Verdict != tc.want || got.Reason != tc.wantReason {
				t.Fatalf("speed %s = (%s, %s), want (%s, %s)", tc.name, got.Verdict, got.Reason, tc.want, tc.wantReason)
			}
		})
	}
}
