package iprisk

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// Provider represents a versioned, external IP risk intelligence adapter.
type Provider interface {
	Name() string
	SchemaVersion() string
	Probe(ctx context.Context, client *http.Client, identity domain.ExitIdentity) (*NormalizedRiskCandidate, error)
}

// NormalizedRiskCandidate represents the normalized, in-memory risk findings from a provider.
type NormalizedRiskCandidate struct {
	Provider              string                   `json:"provider"`
	ProviderSchemaVersion string                   `json:"provider_schema_version"`
	ObservedAt            time.Time                `json:"observed_at"`
	ExpiresAt             time.Time                `json:"expires_at"`
	Status                domain.IPRiskStatus      `json:"status"`
	Score                 *int                     `json:"score,omitempty"`
	Confidence            *int                     `json:"confidence,omitempty"`
	NetworkClass          domain.NetworkClass      `json:"network_class"`
	AnonymizerTraits      []domain.AnonymizerTrait `json:"anonymizer_traits"`
	RawSummary            string                   `json:"raw_summary,omitempty"`
}

// Validate ensures normalized candidate contains valid enums and no leaking secrets or full IPs.
func (c *NormalizedRiskCandidate) Validate() error {
	if strings.TrimSpace(c.Provider) == "" {
		return domain.NewValidationError("missing_provider", "provider name is required")
	}
	if strings.TrimSpace(c.ProviderSchemaVersion) == "" {
		return domain.NewValidationError("missing_schema_version", "provider schema version is required")
	}
	if !c.Status.IsValid() {
		return domain.NewValidationError("invalid_status", "candidate status is invalid")
	}
	if c.Score != nil {
		if *c.Score < 0 || *c.Score > 100 {
			return domain.NewValidationError("invalid_score", "score must be between 0 and 100")
		}
	}
	if c.Confidence != nil {
		if *c.Confidence < 0 || *c.Confidence > 100 {
			return domain.NewValidationError("invalid_confidence", "confidence must be between 0 and 100")
		}
	}
	if !c.NetworkClass.IsValid() {
		return domain.NewValidationError("invalid_network_class", "network class is invalid")
	}
	for _, trait := range c.AnonymizerTraits {
		if !trait.IsValid() {
			return domain.NewValidationError("invalid_anonymizer_trait", "anonymizer trait is invalid")
		}
	}
	if c.RawSummary != "" {
		redacted := domain.RedactSensitiveInfo(c.RawSummary)
		if redacted != c.RawSummary {
			return domain.NewValidationError("sensitive_data_leak", "raw summary contained sensitive data or complete IP")
		}
		if len(c.RawSummary) > domain.MaxIPRiskRedactedSummaryLength {
			return domain.NewValidationError("summary_too_long", "raw summary exceeds maximum length")
		}
	}
	return nil
}

// ToObservation converts the validated candidate into an immutable domain.IPRiskObservation.
func (c *NormalizedRiskCandidate) ToObservation(nodeLogicalID, exitIdentityDigest, evidenceDigest string) (*domain.IPRiskObservation, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !domain.IsValidLogicalID(nodeLogicalID) {
		return nil, domain.NewValidationError("invalid_node_logical_id", "node logical ID is invalid")
	}
	obsID := domain.MustNewUUIDv7()

	summary := domain.RedactSensitiveInfo(c.RawSummary)
	if len(summary) > domain.MaxIPRiskRedactedSummaryLength {
		summary = summary[:domain.MaxIPRiskRedactedSummaryLength]
	}

	obs := &domain.IPRiskObservation{
		ID:                    obsID,
		NodeLogicalID:         nodeLogicalID,
		ExitIdentityDigest:    exitIdentityDigest,
		Provider:              c.Provider,
		ProviderSchemaVersion: c.ProviderSchemaVersion,
		ObservedAt:            c.ObservedAt,
		ExpiresAt:             c.ExpiresAt,
		Status:                c.Status,
		Score:                 c.Score,
		Confidence:            c.Confidence,
		NetworkClass:          c.NetworkClass,
		AnonymizerTraits:      c.AnonymizerTraits,
		EvidenceDigest:        evidenceDigest,
		RedactedSummary:       summary,
	}

	if err := obs.Validate(); err != nil {
		return nil, fmt.Errorf("validate observation: %w", err)
	}

	return obs, nil
}
