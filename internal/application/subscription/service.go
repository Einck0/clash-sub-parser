// Package subscription provides subscription management use cases.
package subscription

import (
	"context"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
)

const (
	defaultRefreshIntervalSeconds = 86400
	defaultTimeoutSeconds         = 30
	defaultMaxResponseBytes       = 10 * 1024 * 1024
)

// CreateSubscriptionCommand defines the mutable configuration accepted when creating a subscription.
type CreateSubscriptionCommand struct {
	Name               string
	SourceURLSecretRef string
	Enabled            bool
	RefreshPolicy      domain.RefreshPolicy
	Config             domain.SubscriptionConfig
	RequestID          string
	ActorKind          domain.ActorKind
}

// UpdateSubscriptionCommand defines a partial update of mutable subscription configuration.
type UpdateSubscriptionCommand struct {
	ID                 string
	Revision           string
	Name               *string
	SourceURLSecretRef *string
	Enabled            *bool
	RefreshPolicy      *domain.RefreshPolicy
	Config             *domain.SubscriptionConfig
	RequestID          string
	ActorKind          domain.ActorKind
}

// DeleteSubscriptionCommand identifies a subscription for deletion.
type DeleteSubscriptionCommand struct {
	ID        string
	RequestID string
	ActorKind domain.ActorKind
}

// ToggleSubscriptionCommand changes a subscription enabled state with optimistic concurrency.
type ToggleSubscriptionCommand struct {
	ID        string
	Revision  string
	Enabled   bool
	RequestID string
	ActorKind domain.ActorKind
}

// GetSubscriptionQuery identifies one subscription.
type GetSubscriptionQuery struct {
	ID string
}

// ListSubscriptionsQuery supplies the supported subscription filters.
type ListSubscriptionsQuery struct {
	Page        int
	PageSize    int
	EnabledOnly bool
	SearchText  string
}

// SubscriptionView is the API-safe representation of a subscription.
type SubscriptionView struct {
	ID                 string                    `json:"id"`
	Name               string                    `json:"name"`
	SourceURLSecretRef string                    `json:"source_url_secret_ref"`
	Enabled            bool                      `json:"enabled"`
	RefreshPolicy      domain.RefreshPolicy      `json:"refresh_policy"`
	Config             domain.SubscriptionConfig `json:"config"`
	Revision           string                    `json:"revision"`
	CreatedAt          string                    `json:"created_at"`
	UpdatedAt          string                    `json:"updated_at"`
}

// ListResult is a page of API-safe subscriptions.
type ListResult struct {
	Items    []SubscriptionView
	Page     int
	PageSize int
	Total    int
}

// Reconciler 定义将远程订阅内容对齐至节点台账的执行契约
type Reconciler interface {
	ReconcileSubscription(ctx context.Context, subID string) (*inventory.ReconcileResult, error)
}

// ReconcileSummary 描述刷新完成后的数据摘要
type ReconcileSummary struct {
	FetchID        string `json:"fetch_id"`
	SubscriptionID string `json:"subscription_id"`
	Outcome        string `json:"outcome"`
	NodesParsed    int    `json:"nodes_parsed"`
	NodesValid     int    `json:"nodes_valid"`
	RedactedError  string `json:"redacted_error,omitempty"`
	RefreshedAt    string `json:"refreshed_at,omitempty"`
}

// Option 支持可选配置订阅服务
type Option func(*Service)

// WithReconciler 注入对齐执行器契约实现
func WithReconciler(r Reconciler) Option {
	return func(s *Service) {
		s.reconciler = r
	}
}

// Service orchestrates subscription commands and append-only audit events.
type Service struct {
	subscriptions domain.SubscriptionRepository
	audit         domain.AuditRepository
	reconciler    Reconciler
	mu            sync.Mutex
}

// NewService constructs a subscription service from its domain ports.
func NewService(subscriptions domain.SubscriptionRepository, audit domain.AuditRepository, opts ...Option) *Service {
	s := &Service{subscriptions: subscriptions, audit: audit}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetReconciler 动态设置或更新对齐执行器
func (s *Service) SetReconciler(r Reconciler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconciler = r
}

// Create validates and persists a new subscription with UUIDv7 identity and revision.
func (s *Service) Create(ctx context.Context, command CreateSubscriptionCommand) (SubscriptionView, error) {
	if err := validateCreate(command); err != nil {
		s.recordFailure(ctx, command.ActorKind, command.RequestID, "subscription.create")
		return SubscriptionView{}, err
	}

	sub := domain.Subscription{
		ID:                 domain.MustNewUUIDv7(),
		Name:               strings.TrimSpace(command.Name),
		SourceURLSecretRef: strings.TrimSpace(command.SourceURLSecretRef),
		Enabled:            command.Enabled,
		RefreshPolicy:      normalizedPolicy(command.RefreshPolicy),
		Config:             command.Config,
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := s.subscriptions.Create(ctx, &sub); err != nil {
		s.recordFailure(ctx, command.ActorKind, command.RequestID, "subscription.create")
		return SubscriptionView{}, err
	}
	if err := s.record(ctx, command.ActorKind, command.RequestID, "subscription.create", domain.AuditResultSuccess); err != nil {
		return SubscriptionView{}, err
	}
	return redact(sub), nil
}

// Update applies a version-checked subscription update and issues a new revision.
func (s *Service) Update(ctx context.Context, command UpdateSubscriptionCommand) (SubscriptionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, err := s.subscriptions.GetByID(ctx, command.ID)
	if err != nil {
		s.recordFailure(ctx, command.ActorKind, command.RequestID, "subscription.update")
		return SubscriptionView{}, err
	}
	if strings.TrimSpace(command.Revision) == "" || command.Revision != sub.Revision {
		err = domain.NewConflictError("revision_conflict", "subscription revision does not match")
		s.recordFailure(ctx, command.ActorKind, command.RequestID, "subscription.update")
		return SubscriptionView{}, err
	}
	if command.Name != nil {
		if strings.TrimSpace(*command.Name) == "" {
			err = domain.NewValidationError("invalid_subscription_name", "subscription name is required")
			s.recordFailure(ctx, command.ActorKind, command.RequestID, "subscription.update")
			return SubscriptionView{}, err
		}
		sub.Name = strings.TrimSpace(*command.Name)
	}
	if command.SourceURLSecretRef != nil {
		if strings.TrimSpace(*command.SourceURLSecretRef) == "" {
			err = domain.NewValidationError("invalid_source_url_secret_ref", "source URL secret reference is required")
			s.recordFailure(ctx, command.ActorKind, command.RequestID, "subscription.update")
			return SubscriptionView{}, err
		}
		sub.SourceURLSecretRef = strings.TrimSpace(*command.SourceURLSecretRef)
	}
	if command.Enabled != nil {
		sub.Enabled = *command.Enabled
	}
	if command.RefreshPolicy != nil {
		sub.RefreshPolicy = normalizedPolicy(*command.RefreshPolicy)
	}
	if command.Config != nil {
		sub.Config = *command.Config
	}
	sub.Revision = domain.MustNewUUIDv7()
	sub.UpdatedAt = domain.NowUTC()
	if err := s.subscriptions.Update(ctx, sub); err != nil {
		s.recordFailure(ctx, command.ActorKind, command.RequestID, "subscription.update")
		return SubscriptionView{}, err
	}
	if err := s.record(ctx, command.ActorKind, command.RequestID, "subscription.update", domain.AuditResultSuccess); err != nil {
		return SubscriptionView{}, err
	}
	return redact(*sub), nil
}

// Delete removes a subscription and relies on repository foreign-key cascades for its associations.
func (s *Service) Delete(ctx context.Context, command DeleteSubscriptionCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.subscriptions.Delete(ctx, command.ID); err != nil {
		s.recordFailure(ctx, command.ActorKind, command.RequestID, "subscription.delete")
		return err
	}
	return s.record(ctx, command.ActorKind, command.RequestID, "subscription.delete", domain.AuditResultSuccess)
}

// Toggle changes enabled state through the versioned update path.
func (s *Service) Toggle(ctx context.Context, command ToggleSubscriptionCommand) (SubscriptionView, error) {
	return s.Update(ctx, UpdateSubscriptionCommand{
		ID: command.ID, Revision: command.Revision, Enabled: &command.Enabled,
		RequestID: command.RequestID, ActorKind: command.ActorKind,
	})
}

// Get returns one API-safe subscription.
func (s *Service) Get(ctx context.Context, query GetSubscriptionQuery) (SubscriptionView, error) {
	sub, err := s.subscriptions.GetByID(ctx, query.ID)
	if err != nil {
		return SubscriptionView{}, err
	}
	return redact(*sub), nil
}

// List returns a filtered, server-paginated page of API-safe subscriptions.
func (s *Service) List(ctx context.Context, query ListSubscriptionsQuery) (ListResult, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	items, total, err := s.subscriptions.List(ctx, domain.SubscriptionFilter{
		Pagination: domain.Pagination{Page: page, PageSize: pageSize}, EnabledOnly: query.EnabledOnly, SearchText: strings.TrimSpace(query.SearchText),
	})
	if err != nil {
		return ListResult{}, err
	}
	views := make([]SubscriptionView, 0, len(items))
	for _, item := range items {
		views = append(views, redact(item))
	}
	return ListResult{Items: views, Page: page, PageSize: pageSize, Total: total}, nil
}

// Refresh 执行真正的刷新并返回数据摘要
func (s *Service) Refresh(ctx context.Context, id, requestID string, actorKind domain.ActorKind) (*ReconcileSummary, error) {
	sub, err := s.subscriptions.GetByID(ctx, id)
	if err != nil {
		s.recordFailure(ctx, actorKind, requestID, "subscription.refresh")
		return nil, err
	}
	if !sub.Enabled {
		err = domain.NewValidationError("subscription_disabled", "disabled subscriptions cannot be refreshed")
		s.recordFailure(ctx, actorKind, requestID, "subscription.refresh")
		return nil, err
	}

	if s.reconciler == nil {
		if err := s.record(ctx, actorKind, requestID, "subscription.refresh", domain.AuditResultSuccess); err != nil {
			return nil, err
		}
		return &ReconcileSummary{
			SubscriptionID: sub.ID,
			Outcome:        "success",
			RefreshedAt:    domain.NowUTC().Format(time.RFC3339),
		}, nil
	}

	result, err := s.reconciler.ReconcileSubscription(ctx, id)
	if err != nil {
		s.recordFailure(ctx, actorKind, requestID, "subscription.refresh")
		summary := &ReconcileSummary{
			SubscriptionID: sub.ID,
			Outcome:        "failed",
			RefreshedAt:    domain.NowUTC().Format(time.RFC3339),
		}
		if result != nil {
			summary.FetchID = result.FetchID
			summary.Outcome = string(result.Outcome)
			summary.NodesParsed = result.NodesParsed
			summary.NodesValid = result.NodesValid
			summary.RedactedError = result.RedactedError
		}
		return summary, err
	}

	summary := &ReconcileSummary{
		FetchID:        result.FetchID,
		SubscriptionID: result.SubscriptionID,
		Outcome:        string(result.Outcome),
		NodesParsed:    result.NodesParsed,
		NodesValid:     result.NodesValid,
		RedactedError:  result.RedactedError,
		RefreshedAt:    domain.NowUTC().Format(time.RFC3339),
	}

	if err := s.record(ctx, actorKind, requestID, "subscription.refresh", domain.AuditResultSuccess); err != nil {
		return summary, err
	}
	return summary, nil
}

func validateCreate(command CreateSubscriptionCommand) error {
	if strings.TrimSpace(command.Name) == "" {
		return domain.NewValidationError("invalid_subscription_name", "subscription name is required")
	}
	if strings.TrimSpace(command.SourceURLSecretRef) == "" {
		return domain.NewValidationError("invalid_source_url_secret_ref", "source URL secret reference is required")
	}
	return nil
}

func normalizedPolicy(policy domain.RefreshPolicy) domain.RefreshPolicy {
	if policy.IntervalSeconds <= 0 {
		policy.IntervalSeconds = defaultRefreshIntervalSeconds
	}
	if policy.TimeoutSeconds <= 0 {
		policy.TimeoutSeconds = defaultTimeoutSeconds
	}
	if policy.MaxResponseBytes <= 0 {
		policy.MaxResponseBytes = defaultMaxResponseBytes
	}
	return policy
}

func redact(sub domain.Subscription) SubscriptionView {
	return SubscriptionView{
		ID: sub.ID, Name: sub.Name, SourceURLSecretRef: "***", Enabled: sub.Enabled, RefreshPolicy: sub.RefreshPolicy,
		Config: sub.Config, Revision: sub.Revision,
		CreatedAt: sub.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), UpdatedAt: sub.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (s *Service) record(ctx context.Context, actor domain.ActorKind, requestID, action string, result domain.AuditResult) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.Record(ctx, &domain.AuditEvent{ID: domain.MustNewUUIDv7(), ActorKind: actor, RequestID: requestID, Action: action, Result: result, CreatedAt: domain.NowUTC()})
}

func (s *Service) recordFailure(ctx context.Context, actor domain.ActorKind, requestID, action string) {
	_ = s.record(ctx, actor, requestID, action, domain.AuditResultFailure)
}
