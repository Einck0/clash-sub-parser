package iprisk

import (
	"context"
	"net/http"
	"time"

	"clash-sub-parser/internal/domain"
)

// FakeProviderBehavior configures the deterministic output of DeterministicFakeProvider.
type FakeProviderBehavior struct {
	ScoreMissing bool
	Score        int
	Confidence   int
	NetworkClass domain.NetworkClass
	Traits       []domain.AnonymizerTrait
	Status       domain.IPRiskStatus
	Error        error
	TTL          time.Duration
}

// DeterministicFakeProvider is an in-memory, deterministic IP risk provider adapter for testing.
type DeterministicFakeProvider struct {
	name          string
	schemaVersion string
	behavior      FakeProviderBehavior
}

// NewDeterministicFakeProvider constructs a DeterministicFakeProvider.
func NewDeterministicFakeProvider(name, schemaVersion string, behavior FakeProviderBehavior) *DeterministicFakeProvider {
	if behavior.TTL <= 0 {
		behavior.TTL = 15 * time.Minute
	}
	if !behavior.NetworkClass.IsValid() {
		behavior.NetworkClass = domain.NetworkClassUnknown
	}
	return &DeterministicFakeProvider{
		name:          name,
		schemaVersion: schemaVersion,
		behavior:      behavior,
	}
}

func (f *DeterministicFakeProvider) Name() string {
	return f.name
}

func (f *DeterministicFakeProvider) SchemaVersion() string {
	return f.schemaVersion
}

func (f *DeterministicFakeProvider) Probe(ctx context.Context, client *http.Client, identity domain.ExitIdentity) (*NormalizedRiskCandidate, error) {
	if f.behavior.Error != nil {
		return nil, f.behavior.Error
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	now := time.Now().UTC()
	candidate := &NormalizedRiskCandidate{
		Provider:              f.name,
		ProviderSchemaVersion: f.schemaVersion,
		ObservedAt:            now,
		ExpiresAt:             now.Add(f.behavior.TTL),
		Status:                domain.IPRiskStatusAvailable,
		NetworkClass:          f.behavior.NetworkClass,
		AnonymizerTraits:      f.behavior.Traits,
		RawSummary:            "fake provider normalized summary",
	}

	if f.behavior.Status.IsValid() {
		candidate.Status = f.behavior.Status
	}

	if !f.behavior.ScoreMissing {
		score := f.behavior.Score
		candidate.Score = &score
	} else {
		// Missing score cannot be available
		candidate.Status = domain.IPRiskStatusUnknown
	}

	conf := f.behavior.Confidence
	candidate.Confidence = &conf
	if conf < 50 && candidate.Status == domain.IPRiskStatusAvailable {
		// Low confidence defaults to unknown
		candidate.Status = domain.IPRiskStatusUnknown
	}

	return candidate, nil
}
