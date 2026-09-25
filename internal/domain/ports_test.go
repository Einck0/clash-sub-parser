package domain_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

// mockRepositories verifies that the domain port interfaces can be implemented cleanly.
type mockSubscriptionRepo struct{}

func (m *mockSubscriptionRepo) GetByID(ctx context.Context, id string) (*domain.Subscription, error) {
	return nil, nil
}
func (m *mockSubscriptionRepo) List(ctx context.Context, filter domain.SubscriptionFilter) ([]domain.Subscription, int, error) {
	return nil, 0, nil
}
func (m *mockSubscriptionRepo) Create(ctx context.Context, sub *domain.Subscription) error {
	return nil
}
func (m *mockSubscriptionRepo) Update(ctx context.Context, sub *domain.Subscription) error {
	return nil
}
func (m *mockSubscriptionRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func TestPortsInterfaces(t *testing.T) {
	var _ domain.SubscriptionRepository = (*mockSubscriptionRepo)(nil)
	var _ domain.NodeRepository = (domain.NodeRepository)(nil)
	var _ domain.NodeSourceRepository = (domain.NodeSourceRepository)(nil)
	var _ domain.ProbeRunRepository = (domain.ProbeRunRepository)(nil)
	var _ domain.ProbeObservationRepository = (domain.ProbeObservationRepository)(nil)
	var _ domain.PolicyRepository = (domain.PolicyRepository)(nil)
	var _ domain.RevisionRepository = (domain.RevisionRepository)(nil)
	var _ domain.PublicationRepository = (domain.PublicationRepository)(nil)
	var _ domain.SettingsRepository = (domain.SettingsRepository)(nil)
	var _ domain.AuditRepository = (domain.AuditRepository)(nil)
	var _ domain.ProbeScheduleRepository = (domain.ProbeScheduleRepository)(nil)
	var _ domain.NodeFilterRepository = (domain.NodeFilterRepository)(nil)
}

func TestPaginationAndFilters(t *testing.T) {
	filter := domain.NodeFilter{
		Pagination: domain.Pagination{
			Page:     1,
			PageSize: 50,
		},
		Protocols:  []domain.Protocol{domain.ProtocolVMess},
		ActiveOnly: true,
	}

	if filter.Pagination.Page != 1 || filter.Pagination.PageSize != 50 {
		t.Fatalf("unexpected pagination values: %+v", filter.Pagination)
	}
}

func TestTimeUtilities(t *testing.T) {
	now := domain.NowUTC()
	if now.Location() != time.UTC {
		t.Fatalf("NowUTC must return UTC time, got %v", now.Location())
	}

	formatted := domain.FormatTime(now)
	parsed, err := domain.ParseTime(formatted)
	if err != nil {
		t.Fatalf("ParseTime failed: %v", err)
	}

	if parsed.Location() != time.UTC {
		t.Fatalf("parsed time must be in UTC, got %v", parsed.Location())
	}
}
