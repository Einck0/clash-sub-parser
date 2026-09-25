package domain

import (
	"fmt"
	"time"
)

// Settings encapsulates global system parameters and resource bounds.
// Note: Settings NEVER contains plaintext secrets.
type Settings struct {
	ProbeConcurrencyWindow int       `json:"probe_concurrency_window"` // default 16, valid 10-20, hard limit 32
	MaxConcurrentProbes    int       `json:"max_concurrent_probes"`
	ProbePerNodeTTLSeconds int       `json:"probe_per_node_ttl_seconds"`
	FetchTimeoutSeconds    int       `json:"fetch_timeout_seconds"`
	FetchMaxResponseBytes  int64     `json:"fetch_max_response_bytes"`
	MaxPageSize            int       `json:"max_page_size"`     // default 50, maximum allowed 100
	DefaultPageSize        int       `json:"default_page_size"` // default 50
	// AdminToken stores the cryptographic verifier/hash for admin authentication, NEVER usable plaintext secret.
	AdminToken string    `json:"-"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DefaultSettings returns the standard production baseline settings.
func DefaultSettings() Settings {
	return Settings{
		ProbeConcurrencyWindow: 16,
		MaxConcurrentProbes:    16,
		ProbePerNodeTTLSeconds: 300,
		FetchTimeoutSeconds:    30,
		FetchMaxResponseBytes:  10 * 1024 * 1024, // 10 MB
		MaxPageSize:            100,
		DefaultPageSize:        50,
		AdminToken:             "",
		UpdatedAt:              NowUTC(),
	}
}

// Validate checks that settings values adhere to system boundary constraints.
func (s Settings) Validate() error {
	if s.ProbeConcurrencyWindow < 10 || s.ProbeConcurrencyWindow > 32 {
		return NewValidationError("invalid_probe_concurrency_window",
			fmt.Sprintf("probe_concurrency_window must be between 10 and 32, got %d", s.ProbeConcurrencyWindow))
	}
	if s.MaxPageSize < 1 || s.MaxPageSize > 100 {
		return NewValidationError("invalid_max_page_size",
			fmt.Sprintf("max_page_size must be between 1 and 100, got %d", s.MaxPageSize))
	}
	if s.DefaultPageSize < 1 || s.DefaultPageSize > s.MaxPageSize {
		return NewValidationError("invalid_default_page_size",
			fmt.Sprintf("default_page_size must be between 1 and %d, got %d", s.MaxPageSize, s.DefaultPageSize))
	}
	if s.FetchTimeoutSeconds < 1 || s.FetchTimeoutSeconds > 300 {
		return NewValidationError("invalid_fetch_timeout",
			fmt.Sprintf("fetch_timeout_seconds must be between 1 and 300, got %d", s.FetchTimeoutSeconds))
	}
	if s.FetchMaxResponseBytes <= 0 {
		return NewValidationError("invalid_fetch_max_bytes", "fetch_max_response_bytes must be positive")
	}
	return nil
}
