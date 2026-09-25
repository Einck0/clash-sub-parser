package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// IPRiskProviderSettings contains deployment references and bounded provider budgets.
type IPRiskProviderSettings struct {
	Provider           string        `json:"provider"`
	SchemaVersion      string        `json:"schema_version"`
	Enabled            bool          `json:"enabled"`
	SecretReference    string        `json:"-"`
	MaxConcurrency     int           `json:"max_concurrency"`
	RequestsPerMinute  int           `json:"requests_per_minute"`
	DailyRequestBudget int           `json:"daily_request_budget"`
	PerRequestTimeout  time.Duration `json:"per_request_timeout"`
	MaxResponseBytes   int64         `json:"max_response_bytes"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

// Validate checks that provider settings contain only a reference to a secret.
func (s IPRiskProviderSettings) Validate() error {
	if err := validateIPRiskIdentifier("provider", s.Provider); err != nil {
		return err
	}
	if err := validateIPRiskIdentifier("schema_version", s.SchemaVersion); err != nil {
		return err
	}
	if strings.TrimSpace(s.SecretReference) == "" {
		return NewValidationError("missing_ip_risk_secret_reference", "provider secret reference is required")
	}
	lowerReference := strings.ToLower(s.SecretReference)
	parsedReference, parseErr := url.Parse(s.SecretReference)
	if parseErr != nil || (parsedReference.Scheme != "secret" && parsedReference.Scheme != "vault") ||
		parsedReference.Host == "" || parsedReference.User != nil || parsedReference.RawQuery != "" || parsedReference.Fragment != "" {
		return NewSecurityError("invalid_ip_risk_secret_reference", "provider settings must contain a credential-free secret reference")
	}
	for _, marker := range []string{"api_key=", "apikey=", "token=", "cookie:", "begin private key", "bearer ", "sk_live_"} {
		if strings.Contains(lowerReference, marker) {
			return NewSecurityError("plaintext_ip_risk_secret", "provider settings must contain a secret reference, not a secret value")
		}
	}
	if s.MaxConcurrency <= 0 || s.RequestsPerMinute <= 0 || s.DailyRequestBudget <= 0 ||
		s.PerRequestTimeout <= 0 || s.MaxResponseBytes <= 0 {
		return NewValidationError("invalid_ip_risk_provider_budget", "provider budgets and timeout must be positive")
	}
	return nil
}

// IPRiskObservationFilter describes bounded server-side observation queries.
type IPRiskObservationFilter struct {
	NodeLogicalID string
	Provider      string
	SchemaVersion string
	Status        IPRiskStatus
	Page          int
	PageSize      int
}

// Normalize applies safe defaults and an upper bound to observation paging.
func (f IPRiskObservationFilter) Normalize() IPRiskObservationFilter {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 50
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	return f
}

// RiskPolicyRevision is a persisted version of a local risk policy.
type RiskPolicyRevision struct {
	RiskPolicy
	Active        bool       `json:"active"`
	CreatedAt     time.Time  `json:"created_at"`
	ActivatedAt   *time.Time `json:"activated_at,omitempty"`
	DeactivatedAt *time.Time `json:"deactivated_at,omitempty"`
}

// Validate checks the embedded policy and persistence timestamps.
func (r RiskPolicyRevision) Validate() error {
	if err := r.RiskPolicy.Validate(); err != nil {
		return err
	}
	if r.CreatedAt.IsZero() {
		return NewValidationError("missing_risk_policy_created_at", "risk policy revision created_at is required")
	}
	return nil
}

// RiskPolicyProvider binds a versioned provider to a policy revision.
type RiskPolicyProvider struct {
	RevisionID    string `json:"revision_id"`
	Provider      string `json:"provider"`
	SchemaVersion string `json:"schema_version"`
	Position      int    `json:"position"`
}

// Validate checks the stable provider binding references.
func (p RiskPolicyProvider) Validate() error {
	if !IsValidUUIDv7(p.RevisionID) || p.Position < 0 {
		return NewValidationError("invalid_risk_policy_provider", fmt.Sprintf("invalid provider binding for revision %s", p.RevisionID))
	}
	if err := validateIPRiskIdentifier("provider", p.Provider); err != nil {
		return NewValidationError("invalid_risk_policy_provider", fmt.Sprintf("invalid provider binding for revision %s", p.RevisionID))
	}
	if err := validateIPRiskIdentifier("schema_version", p.SchemaVersion); err != nil {
		return NewValidationError("invalid_risk_policy_provider", fmt.Sprintf("invalid provider binding for revision %s", p.RevisionID))
	}
	return nil
}

// RiskPolicyScoreBand is the persisted form of a policy score band.
type RiskPolicyScoreBand struct {
	RevisionID string `json:"revision_id"`
	ScoreBand
	Position int `json:"position"`
}

// RiskPolicyTraitRule is the persisted form of a policy trait rule.
type RiskPolicyTraitRule struct {
	RevisionID string `json:"revision_id"`
	TraitRule
}

// RiskPolicyFilter defines query filters and pagination for risk policy revisions.
type RiskPolicyFilter struct {
	Active   *bool
	Page     int
	PageSize int
}

// Normalize applies safe defaults and bounds to risk policy pagination.
func (f RiskPolicyFilter) Normalize() RiskPolicyFilter {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.Page = 50
	}
	if f.PageSize > 100 {
		f.Page = 100
	}
	return f
}

// RiskPolicyGroupBinding associates a node group with a risk policy revision.
type RiskPolicyGroupBinding struct {
	GroupID          string    `json:"group_id"`
	PolicyRevisionID string    `json:"policy_revision_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Validate checks that group and policy revision references are valid UUIDv7.
func (b RiskPolicyGroupBinding) Validate() error {
	if !IsValidUUIDv7(b.GroupID) {
		return NewValidationError("invalid_group_id", "group ID must be a valid UUIDv7")
	}
	if !IsValidUUIDv7(b.PolicyRevisionID) {
		return NewValidationError("invalid_policy_revision_id", "policy revision ID must be a valid UUIDv7")
	}
	if b.CreatedAt.IsZero() || b.UpdatedAt.IsZero() {
		return NewValidationError("missing_binding_timestamp", "binding timestamps are required")
	}
	return nil
}
