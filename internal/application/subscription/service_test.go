package subscription_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
)

type memorySubscriptions struct {
	mu    sync.Mutex
	items map[string]domain.Subscription
}

func (r *memorySubscriptions) GetByID(_ context.Context, id string) (*domain.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.items[id]
	if !ok {
		return nil, domain.NewNotFoundError("subscription_not_found", "subscription not found")
	}
	return &sub, nil
}

func (r *memorySubscriptions) List(_ context.Context, filter domain.SubscriptionFilter) ([]domain.Subscription, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]domain.Subscription, 0, len(r.items))
	for _, sub := range r.items {
		if filter.EnabledOnly && !sub.Enabled {
			continue
		}
		items = append(items, sub)
	}
	return items, len(items), nil
}

func (r *memorySubscriptions) Create(_ context.Context, sub *domain.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[sub.ID] = *sub
	return nil
}

func (r *memorySubscriptions) Update(_ context.Context, sub *domain.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[sub.ID]; !ok {
		return domain.NewNotFoundError("subscription_not_found", "subscription not found")
	}
	r.items[sub.ID] = *sub
	return nil
}

func (r *memorySubscriptions) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return domain.NewNotFoundError("subscription_not_found", "subscription not found")
	}
	delete(r.items, id)
	return nil
}

type memoryAudit struct {
	mu     sync.Mutex
	events []domain.AuditEvent
}

func (r *memoryAudit) Record(_ context.Context, event *domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, *event)
	return nil
}

func (r *memoryAudit) List(context.Context, domain.AuditFilter) ([]domain.AuditEvent, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events, len(r.events), nil
}

func TestServiceCreatesRedactedSubscriptionAndAudits(t *testing.T) {
	repo := &memorySubscriptions{items: make(map[string]domain.Subscription)}
	audit := &memoryAudit{}
	service := subscription.NewService(repo, audit)

	created, err := service.Create(context.Background(), subscription.CreateSubscriptionCommand{
		Name:               "Primary source",
		SourceURLSecretRef: "secret://subscriptions/primary?token=do-not-leak",
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds: 3600,
			TimeoutSeconds:  30,
		},
		RequestID: "req-create",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !domain.IsValidUUIDv7(created.ID) || !domain.IsValidUUIDv7(created.Revision) {
		t.Fatalf("Create() must produce UUIDv7 id/revision: %#v", created)
	}
	if created.SourceURLSecretRef != "***" {
		t.Fatalf("Create() leaked secret reference: %#v", created)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "subscription.create" || audit.events[0].Result != domain.AuditResultSuccess {
		t.Fatalf("Create() audit events = %#v", audit.events)
	}
}

func TestServiceRejectsStaleRevisionAndDisabledRefresh(t *testing.T) {
	repo := &memorySubscriptions{items: map[string]domain.Subscription{
		"0191e4a0-0000-7000-8000-000000000001": {
			ID: "0191e4a0-0000-7000-8000-000000000001", Name: "Disabled source", SourceURLSecretRef: "secret://subscriptions/disabled", Revision: "0191e4a0-0000-7000-8000-000000000002", Enabled: false,
		},
	}}
	service := subscription.NewService(repo, &memoryAudit{})

	name := "Updated"
	_, err := service.Update(context.Background(), subscription.UpdateSubscriptionCommand{
		ID:       "0191e4a0-0000-7000-8000-000000000001",
		Revision: "0191e4a0-0000-7000-8000-000000000009",
		Name:     &name,
	})
	var conflict *domain.DomainError
	if !errors.As(err, &conflict) || conflict.Category != domain.CategoryConflict {
		t.Fatalf("Update() error = %v, want conflict", err)
	}

	_, err = service.Refresh(context.Background(), "0191e4a0-0000-7000-8000-000000000001", "req-refresh", domain.ActorKindAdmin)
	var validation *domain.DomainError
	if !errors.As(err, &validation) || validation.Code != "subscription_disabled" {
		t.Fatalf("Refresh() error = %v, want subscription_disabled", err)
	}
}

func TestServiceRejectsOneOfTwoConcurrentSameRevisionUpdates(t *testing.T) {
	initialRevision := "0191e4a0-0000-7000-8000-000000000002"
	subID := "0191e4a0-0000-7000-8000-000000000001"
	repo := &memorySubscriptions{items: map[string]domain.Subscription{
		subID: {
			ID:                 subID,
			Name:               "Concurrent source",
			SourceURLSecretRef: "secret://subscriptions/concurrent",
			Revision:           initialRevision,
			Enabled:            true,
		},
	}}
	audit := &memoryAudit{}
	service := subscription.NewService(repo, audit)

	const n = 2
	start := make(chan struct{})
	type res struct {
		view subscription.SubscriptionView
		err  error
	}
	results := make(chan res, n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			<-start
			name := "Updated Name"
			view, err := service.Update(context.Background(), subscription.UpdateSubscriptionCommand{
				ID:        subID,
				Revision:  initialRevision,
				Name:      &name,
				RequestID: "req-concurrent",
				ActorKind: domain.ActorKindAdmin,
			})
			results <- res{view: view, err: err}
		}(i)
	}

	close(start)

	var successes, conflicts int
	for i := 0; i < n; i++ {
		r := <-results
		if r.err == nil {
			successes++
		} else {
			var conflict *domain.DomainError
			if errors.As(r.err, &conflict) && conflict.Category == domain.CategoryConflict {
				conflicts++
			}
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected 1 success and 1 conflict, got %d successes and %d conflicts", successes, conflicts)
	}
}

type mockReconciler struct {
	mu           sync.Mutex
	lastSubID    string
	returnResult *inventory.ReconcileResult
	returnErr    error
}

func (m *mockReconciler) ReconcileSubscription(_ context.Context, subID string) (*inventory.ReconcileResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSubID = subID
	return m.returnResult, m.returnErr
}

func TestSubscriptionRefreshInvokesReconcilerSuccessfully(t *testing.T) {
	subID := "0191e4a0-0000-7000-8000-000000000001"
	repo := &memorySubscriptions{items: map[string]domain.Subscription{
		subID: {
			ID:                 subID,
			Name:               "Active source",
			SourceURLSecretRef: "secret://subscriptions/active",
			Revision:           "0191e4a0-0000-7000-8000-000000000002",
			Enabled:            true,
		},
	}}
	audit := &memoryAudit{}
	mock := &mockReconciler{
		returnResult: &inventory.ReconcileResult{
			FetchID:        "0191e4a0-fetch-7000-8000-000000000003",
			SubscriptionID: subID,
			Outcome:        domain.FetchOutcomeSuccess,
			NodesParsed:    42,
			NodesValid:     40,
		},
	}
	service := subscription.NewService(repo, audit, subscription.WithReconciler(mock))

	summary, err := service.Refresh(context.Background(), subID, "req-refresh-ok", domain.ActorKindAdmin)
	if err != nil {
		t.Fatalf("Refresh() unexpected error = %v", err)
	}
	if summary == nil {
		t.Fatalf("Refresh() returned nil summary")
	}
	if mock.lastSubID != subID {
		t.Fatalf("mock lastSubID = %s, want %s", mock.lastSubID, subID)
	}
	if summary.FetchID != "0191e4a0-fetch-7000-8000-000000000003" {
		t.Fatalf("summary.FetchID = %s, want 0191e4a0-fetch-7000-8000-000000000003", summary.FetchID)
	}
	if summary.Outcome != "success" {
		t.Fatalf("summary.Outcome = %s, want success", summary.Outcome)
	}
	if summary.NodesParsed != 42 || summary.NodesValid != 40 {
		t.Fatalf("summary nodes = (%d, %d), want (42, 40)", summary.NodesParsed, summary.NodesValid)
	}
	if summary.RefreshedAt == "" {
		t.Fatalf("summary.RefreshedAt is empty")
	}

	foundAudit := false
	for _, ev := range audit.events {
		if ev.Action == "subscription.refresh" && ev.Result == domain.AuditResultSuccess {
			foundAudit = true
			break
		}
	}
	if !foundAudit {
		t.Fatalf("audit events missing successful subscription.refresh event: %#v", audit.events)
	}
}

func TestSubscriptionRefreshReconcilerFailure(t *testing.T) {
	subID := "0191e4a0-0000-7000-8000-000000000001"
	repo := &memorySubscriptions{items: map[string]domain.Subscription{
		subID: {
			ID:                 subID,
			Name:               "Active source",
			SourceURLSecretRef: "secret://subscriptions/active",
			Revision:           "0191e4a0-0000-7000-8000-000000000002",
			Enabled:            true,
		},
	}}
	audit := &memoryAudit{}
	mock := &mockReconciler{
		returnResult: &inventory.ReconcileResult{
			FetchID:        "0191e4a0-fetch-7000-8000-000000000003",
			SubscriptionID: subID,
			Outcome:        domain.FetchOutcomeFailed,
			RedactedError:  "network timeout",
		},
		returnErr: fmt.Errorf("network timeout"),
	}
	service := subscription.NewService(repo, audit, subscription.WithReconciler(mock))

	summary, err := service.Refresh(context.Background(), subID, "req-refresh-fail", domain.ActorKindAdmin)
	if err == nil {
		t.Fatalf("Refresh() expected error, got nil")
	}
	if summary == nil {
		t.Fatalf("Refresh() expected non-nil summary on failure")
	}
	if summary.Outcome != "failed" {
		t.Fatalf("summary.Outcome = %s, want failed", summary.Outcome)
	}
	if summary.RedactedError != "network timeout" {
		t.Fatalf("summary.RedactedError = %s, want network timeout", summary.RedactedError)
	}

	foundFailureAudit := false
	for _, ev := range audit.events {
		if ev.Action == "subscription.refresh" && ev.Result == domain.AuditResultFailure {
			foundFailureAudit = true
			break
		}
	}
	if !foundFailureAudit {
		t.Fatalf("audit events missing failed subscription.refresh event: %#v", audit.events)
	}
}

func TestSubscriptionRefreshWithoutReconcilerGraceful(t *testing.T) {
	subID := "0191e4a0-0000-7000-8000-000000000001"
	repo := &memorySubscriptions{items: map[string]domain.Subscription{
		subID: {
			ID:                 subID,
			Name:               "Active source",
			SourceURLSecretRef: "secret://subscriptions/active",
			Revision:           "0191e4a0-0000-7000-8000-000000000002",
			Enabled:            true,
		},
	}}
	audit := &memoryAudit{}
	service := subscription.NewService(repo, audit)

	summary, err := service.Refresh(context.Background(), subID, "req-refresh-noreconciler", domain.ActorKindAdmin)
	if err != nil {
		t.Fatalf("Refresh() unexpected error = %v", err)
	}
	if summary == nil || summary.Outcome != "success" {
		t.Fatalf("summary = %#v, want success outcome", summary)
	}
}

func TestSubscriptionRefreshWithSetReconciler(t *testing.T) {
	var _ subscription.Reconciler = (*inventory.Service)(nil)

	subID := "0191e4a0-0000-7000-8000-000000000001"
	repo := &memorySubscriptions{items: map[string]domain.Subscription{
		subID: {
			ID:                 subID,
			Name:               "Active source",
			SourceURLSecretRef: "secret://subscriptions/active",
			Revision:           "0191e4a0-0000-7000-8000-000000000002",
			Enabled:            true,
		},
	}}
	audit := &memoryAudit{}
	mock := &mockReconciler{
		returnResult: &inventory.ReconcileResult{
			FetchID:        "0191e4a0-fetch-7000-8000-000000000003",
			SubscriptionID: subID,
			Outcome:        domain.FetchOutcomeSuccess,
			NodesParsed:    15,
			NodesValid:     12,
		},
	}
	// Initialise service without reconciler, then wire via SetReconciler matching production assembly
	service := subscription.NewService(repo, audit)
	service.SetReconciler(mock)

	summary, err := service.Refresh(context.Background(), subID, "req-refresh-set-reconciler", domain.ActorKindAdmin)
	if err != nil {
		t.Fatalf("Refresh() unexpected error = %v", err)
	}
	if summary == nil {
		t.Fatalf("Refresh() returned nil summary")
	}
	if mock.lastSubID != subID {
		t.Fatalf("mock lastSubID = %s, want %s", mock.lastSubID, subID)
	}
	if summary.FetchID != "0191e4a0-fetch-7000-8000-000000000003" {
		t.Fatalf("summary.FetchID = %s, want 0191e4a0-fetch-7000-8000-000000000003", summary.FetchID)
	}
	if summary.Outcome != "success" {
		t.Fatalf("summary.Outcome = %s, want success", summary.Outcome)
	}
	if summary.NodesParsed != 15 || summary.NodesValid != 12 {
		t.Fatalf("summary nodes = (%d, %d), want (15, 12)", summary.NodesParsed, summary.NodesValid)
	}
}
