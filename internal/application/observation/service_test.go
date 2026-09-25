package observation_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/observation"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/profiles"
)

type memoryObservations struct {
	items []domain.ProbeObservation
}

func (m *memoryObservations) GetByID(context.Context, string) (*domain.ProbeObservation, error) {
	return nil, domain.NewNotFoundError("observation_not_found", "not found")
}
func (m *memoryObservations) ListByRun(context.Context, string) ([]domain.ProbeObservation, error) {
	return m.items, nil
}
func (m *memoryObservations) ListByNode(context.Context, string, int) ([]domain.ProbeObservation, error) {
	return m.items, nil
}
func (m *memoryObservations) ListLatestByNodes(context.Context, []string, []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	return make(map[string]map[domain.ProbeKind]domain.ProbeObservation), nil
}
func (m *memoryObservations) Create(_ context.Context, item *domain.ProbeObservation) error {
	m.items = append(m.items, *item)
	return nil
}

func TestRecordPersistsOnlyRedactedSummaryAndDigest(t *testing.T) {
	repo := &memoryObservations{}
	svc := observation.NewService(repo, observation.WithClock(func() time.Time {
		return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	}))
	secretBody := []byte("token=super-secret cookie=session-secret")
	item, err := svc.Record(context.Background(), observation.Input{
		RunID:         "run-1",
		NodeLogicalID: "node-1",
		Profile:       profiles.Baseline(),
		Result:        profiles.Result{StatusCode: 200, Body: secretBody},
		ObservedAt:    time.Date(2026, 9, 15, 11, 59, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.EvidenceDigest == "" || !strings.HasPrefix(item.EvidenceDigest, "sha256:") {
		t.Fatalf("digest = %q, want sha256 digest", item.EvidenceDigest)
	}
	if strings.Contains(item.RedactedSummary, "super-secret") || strings.Contains(item.RedactedSummary, "session-secret") {
		t.Fatalf("summary leaked secret: %q", item.RedactedSummary)
	}
	if len(repo.items) != 1 {
		t.Fatalf("persisted observations = %d, want 1", len(repo.items))
	}
}

func TestRecordDoesNotTreatHTTP200AsAvailable(t *testing.T) {
	repo := &memoryObservations{}
	svc := observation.NewService(repo)
	item, err := svc.Record(context.Background(), observation.Input{
		RunID:         "run-2",
		NodeLogicalID: "node-2",
		Profile:       profiles.Baseline(),
		Result:        profiles.Result{StatusCode: 200},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Verdict == domain.VerdictAvailable {
		t.Fatal("HTTP 200 without semantic evidence must not be available")
	}
}

func TestRecordClassifiesStaleObservation(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	svc := observation.NewService(&memoryObservations{}, observation.WithClock(func() time.Time { return now }))
	item, err := svc.Record(context.Background(), observation.Input{
		RunID:         "run-3",
		NodeLogicalID: "node-3",
		Profile:       profiles.Baseline(),
		Result:        profiles.Result{StatusCode: 200, ContractMatched: true},
		ObservedAt:    now.Add(-profiles.Baseline().MaxAge - time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Verdict != domain.VerdictStale {
		t.Fatalf("verdict = %s, want stale", item.Verdict)
	}
}

func TestRecordRejectsInvalidProfile(t *testing.T) {
	svc := observation.NewService(&memoryObservations{})
	_, err := svc.Record(context.Background(), observation.Input{
		RunID:         "run-4",
		NodeLogicalID: "node-4",
		Profile:       profiles.Profile{Kind: domain.ProbeKindBaseline},
	})
	if err == nil {
		t.Fatal("invalid profile must be rejected")
	}
}
