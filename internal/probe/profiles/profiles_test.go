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

func TestIPRiskMatchedContractProducesAvailable(t *testing.T) {
	profile := profiles.IPRisk()
	got := profile.Evaluate(profiles.Result{
		StatusCode:      200,
		ContractMatched: true,
		ContractVersion: profile.Contract,
	})
	if got.Verdict != domain.VerdictAvailable {
		t.Fatalf("IPRisk matched contract verdict = %s, want available", got.Verdict)
	}
}

