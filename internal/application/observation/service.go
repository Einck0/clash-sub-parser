// Package observation records safe, evidence-based probe observations.
package observation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/profiles"
)

const maxSummaryLength = 512

// Input contains the runtime result needed to persist one observation.
type Input struct {
	RunID         string
	NodeLogicalID string
	Profile       profiles.Profile
	Result        profiles.Result
	ObservedAt    time.Time
	LatencyMS     int64
}

// Service evaluates and persists probe observations.
type Service struct {
	repository domain.ProbeObservationRepository
	clock      func() time.Time
}

// Option configures the observation service.
type Option func(*Service)

// WithClock sets the clock used for freshness checks and timestamps.
func WithClock(clock func() time.Time) Option {
	return func(s *Service) { s.clock = clock }
}

// NewService constructs an observation recorder.
func NewService(repository domain.ProbeObservationRepository, options ...Option) *Service {
	s := &Service{repository: repository, clock: time.Now}
	for _, option := range options {
		option(s)
	}
	return s
}

// Record evaluates a result and persists only a bounded redacted summary and digest.
func (s *Service) Record(ctx context.Context, input Input) (*domain.ProbeObservation, error) {
	if s.repository == nil {
		return nil, domain.NewInternalError("observation_repository_missing", "observation repository is required")
	}
	if err := input.Profile.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.NodeLogicalID) == "" {
		return nil, domain.NewValidationError("observation_identity_required", "run and node identifiers are required")
	}

	evaluation := input.Profile.Evaluate(input.Result)
	observedAt := input.ObservedAt
	if observedAt.IsZero() {
		observedAt = s.clock().UTC()
	}
	if observedAt.Before(s.clock().UTC().Add(-input.Profile.MaxAge)) {
		evaluation = profiles.Evaluation{Verdict: domain.VerdictStale, Reason: "observation_expired"}
	}

	obs := &domain.ProbeObservation{
		ID:              domain.MustNewUUIDv7(),
		ProbeRunID:      input.RunID,
		NodeLogicalID:   input.NodeLogicalID,
		Kind:            input.Profile.Kind,
		Verdict:         evaluation.Verdict,
		EvidenceDigest:  digest(input, evaluation),
		ObservedAt:      observedAt.UTC(),
		LatencyMS:       input.LatencyMS,
		RedactedSummary: summary(input, evaluation),
	}
	if err := s.repository.Create(ctx, obs); err != nil {
		return nil, fmt.Errorf("persist probe observation: %w", err)
	}
	return obs, nil
}

func digest(input Input, evaluation profiles.Evaluation) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%d\x00%s", input.RunID, input.NodeLogicalID,
		input.Profile.Version, evaluation.Verdict, input.Result.StatusCode, evaluation.Reason)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func summary(input Input, evaluation profiles.Evaluation) string {
	text := fmt.Sprintf("profile=%s version=%s verdict=%s reason=%s status=%d latency_ms=%d",
		input.Profile.Kind, input.Profile.Version, evaluation.Verdict, evaluation.Reason,
		input.Result.StatusCode, input.LatencyMS)
	text = domain.RedactSensitiveInfo(text)
	if len(text) > maxSummaryLength {
		text = text[:maxSummaryLength]
	}
	return strings.TrimSpace(text + " bytes=" + strconv.FormatInt(input.Result.BytesRead, 10))
}
