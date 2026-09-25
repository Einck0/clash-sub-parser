// Package profiles defines versioned, evidence-based probe contracts.
package profiles

import (
	"bytes"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

const (
	BaselineVersion  = "baseline-v1"
	GeoVersion       = "geo-v1"
	StreamingVersion = "streaming-v1"
	AIVersion        = "ai-v1"
	SpeedVersion     = "speed-v1"
	IPRiskVersion    = "ip-risk-v1"
)

// SpeedBudget contains the hard limits for an opt-in throughput probe.
type SpeedBudget struct {
	OptInRequired      bool
	MaxBytesPerRequest int64
	MaxBytesPerRun     int64
	Deadline           time.Duration
}

// Result is the redacted-independent result of one probe request.
type Result struct {
	StatusCode          int
	Body                []byte
	ContractMatched     bool
	ContractVersion     string
	OptIn               bool
	BytesRead           int64
	DeadlineExceeded    bool
	DNSError            bool
	NetworkError        bool
	ExitIdentityMissing bool
}

// Evaluation is the non-persistent verdict and reason for a probe result.
type Evaluation struct {
	Verdict domain.ProbeVerdict
	Reason  string
}

// Profile is a versioned probe contract.
type Profile struct {
	Kind        domain.ProbeKind
	Version     string
	Contract    string
	MaxAge      time.Duration
	SpeedBudget SpeedBudget
}

// Validate ensures a profile cannot be used without a versioned contract.
func (p Profile) Validate() error {
	if !p.Kind.IsValid() {
		return domain.NewValidationError("invalid_probe_profile_kind", "probe profile kind is invalid")
	}
	if strings.TrimSpace(p.Version) == "" || strings.TrimSpace(p.Contract) == "" {
		return domain.NewValidationError("invalid_probe_profile_contract", "probe profile version and contract are required")
	}
	if p.MaxAge <= 0 {
		return domain.NewValidationError("invalid_probe_profile_ttl", "probe profile freshness window must be positive")
	}
	if p.Kind == domain.ProbeKindSpeed {
		if !p.SpeedBudget.OptInRequired || p.SpeedBudget.MaxBytesPerRequest <= 0 ||
			p.SpeedBudget.MaxBytesPerRun < p.SpeedBudget.MaxBytesPerRequest || p.SpeedBudget.Deadline <= 0 {
			return domain.NewValidationError("invalid_speed_budget", "speed profile requires bounded opt-in budgets")
		}
	}
	return nil
}

// Evaluate applies the profile contract. HTTP status alone never produces available.
func (p Profile) Evaluate(result Result) Evaluation {
	if p.Kind == domain.ProbeKindIPRisk && result.ExitIdentityMissing {
		return Evaluation{Verdict: domain.VerdictUnknown, Reason: "missing_exit_identity"}
	}
	if hasRestrictionMarker(result.Body) || result.StatusCode == 401 || result.StatusCode == 403 {
		if p.Kind == domain.ProbeKindIPRisk {
			return Evaluation{Verdict: domain.VerdictUnknown, Reason: "access_restricted"}
		}
		return Evaluation{Verdict: domain.VerdictRestricted, Reason: "access_restricted"}
	}
	if p.Kind == domain.ProbeKindSpeed {
		if !result.OptIn {
			return Evaluation{Verdict: domain.VerdictUnknown, Reason: "speed_opt_in_required"}
		}
		bytesRead := result.BytesRead
		if bytesRead == 0 {
			bytesRead = int64(len(result.Body))
		}
		if bytesRead > p.SpeedBudget.MaxBytesPerRequest || bytesRead > p.SpeedBudget.MaxBytesPerRun {
			return Evaluation{Verdict: domain.VerdictError, Reason: "speed_budget_exceeded"}
		}
	}
	if result.DNSError || result.NetworkError || result.DeadlineExceeded {
		return Evaluation{Verdict: domain.VerdictError, Reason: "transport_error"}
	}
	if result.StatusCode < 200 || result.StatusCode >= 400 {
		return Evaluation{Verdict: domain.VerdictUnknown, Reason: "unexpected_status"}
	}
	if !result.ContractMatched || result.ContractVersion != p.Contract {
		return Evaluation{Verdict: domain.VerdictUnknown, Reason: "contract_drift"}
	}
	return Evaluation{Verdict: domain.VerdictAvailable, Reason: "contract_matched"}
}

func hasRestrictionMarker(body []byte) bool {
	lower := bytes.ToLower(body)
	markers := [][]byte{
		[]byte("cf-chl"), []byte("turnstile"), []byte("challenge"),
		[]byte("captcha"), []byte("human verification"), []byte("verify you are human"),
		[]byte("/login"), []byte("sign in"), []byte("log in"),
	}
	for _, marker := range markers {
		if bytes.Contains(lower, marker) {
			return true
		}
	}
	return false
}

const defaultMaxAge = 15 * time.Minute

func profile(kind domain.ProbeKind, version string) Profile {
	return Profile{Kind: kind, Version: version, Contract: version, MaxAge: defaultMaxAge}
}

// Baseline returns the handshake, connectivity, and latency contract.
func Baseline() Profile { return profile(domain.ProbeKindBaseline, BaselineVersion) }

// Geo returns the exit IP, country, and ASN contract.
func Geo() Profile { return profile(domain.ProbeKindGeo, GeoVersion) }

// Streaming returns the provider streaming capability contract.
func Streaming() Profile { return profile(domain.ProbeKindStreaming, StreamingVersion) }

// AI returns the provider AI capability contract.
func AI() Profile { return profile(domain.ProbeKindAI, AIVersion) }

// Speed returns the explicitly opt-in bounded throughput contract.
func Speed() Profile {
	p := profile(domain.ProbeKindSpeed, SpeedVersion)
	p.SpeedBudget = SpeedBudget{
		OptInRequired:      true,
		MaxBytesPerRequest: 1 << 20,
		MaxBytesPerRun:     8 << 20,
		Deadline:           10 * time.Second,
	}
	return p
}

// IPRisk returns the isolated IP risk provider and exit identity probe contract.
func IPRisk() Profile {
	return profile(domain.ProbeKindIPRisk, IPRiskVersion)
}

// All returns every supported profile in stable order.
func All() []Profile {
	return []Profile{Baseline(), Geo(), Streaming(), AI(), Speed()}
}
