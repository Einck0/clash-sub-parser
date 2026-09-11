package repository

import (
	"context"
	"errors"

	"clash-sub-parser/internal/domain"
)

// Standard persistence errors
var (
	ErrNotFound  = errors.New("record not found")
	ErrDuplicate = errors.New("record already exists")
	ErrNilEntity = errors.New("entity cannot be nil")
)

// SubscriptionRepository defines persistence operations for proxy subscriptions.
type SubscriptionRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.Subscription, error)
	GetByName(ctx context.Context, name string) (*domain.Subscription, error)
	List(ctx context.Context, enabledOnly bool) ([]*domain.Subscription, error)
	Create(ctx context.Context, sub *domain.Subscription) error
	Update(ctx context.Context, sub *domain.Subscription) error
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int64, error)
}

// NodeFilter encapsulates parameters for filtering and paginating node inventory.
type NodeFilter struct {
	State          domain.LifecycleState
	Protocol       string
	Keyword        string
	SubscriptionID int64
	Limit          int
	Offset         int
}

// NodeRepository defines persistence operations for proxy nodes.
type NodeRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.Node, error)
	GetByLogicalID(ctx context.Context, logicalID string) (*domain.Node, error)
	GetByFingerprint(ctx context.Context, fingerprint string) (*domain.Node, error)
	List(ctx context.Context, state domain.LifecycleState) ([]*domain.Node, error)
	ListFiltered(ctx context.Context, filter NodeFilter) ([]*domain.Node, int64, error)
	Create(ctx context.Context, node *domain.Node) error
	Update(ctx context.Context, node *domain.Node) error
	Delete(ctx context.Context, id int64) error
	BatchDelete(ctx context.Context, ids []int64) (int64, error)
	BatchUpsert(ctx context.Context, nodes []*domain.Node) (int64, error)
	Count(ctx context.Context) (int64, error)
}

// NodeGroupRepository defines persistence operations for routing policy groups.
type NodeGroupRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.NodeGroup, error)
	GetByName(ctx context.Context, name string) (*domain.NodeGroup, error)
	List(ctx context.Context) ([]*domain.NodeGroup, error)
	Create(ctx context.Context, group *domain.NodeGroup) error
	Update(ctx context.Context, group *domain.NodeGroup) error
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int64, error)
}

// RuleRepository defines persistence operations for routing rules.
type RuleRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.Rule, error)
	List(ctx context.Context, enabledOnly bool) ([]*domain.Rule, error)
	ListByCategory(ctx context.Context, category string) ([]*domain.Rule, error)
	Create(ctx context.Context, rule *domain.Rule) error
	Update(ctx context.Context, rule *domain.Rule) error
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context) (int64, error)
}

// ProbeRepository defines persistence operations for probe observation records.
type ProbeRepository interface {
	GetByNodeKey(ctx context.Context, nodeKey string) (*domain.NodeProbeResult, error)
	List(ctx context.Context, limit int, offset int) ([]*domain.NodeProbeResult, error)
	Upsert(ctx context.Context, result *domain.NodeProbeResult) error
	BatchUpsert(ctx context.Context, results []*domain.NodeProbeResult) (int64, error)
	Count(ctx context.Context) (int64, error)
	DeleteAll(ctx context.Context) error
}

// GenerateConfigRepository defines persistence operations for global export config.
type GenerateConfigRepository interface {
	Get(ctx context.Context) (*domain.GenerateConfig, error)
	Update(ctx context.Context, config *domain.GenerateConfig) error
}

// DNSConfigRepository defines persistence operations for DNS resolution configuration.
type DNSConfigRepository interface {
	Get(ctx context.Context) (*domain.DNSConfig, error)
	Update(ctx context.Context, config *domain.DNSConfig) error
}

// Repositories aggregates all domain persistence repositories.
type Repositories struct {
	Subscriptions  SubscriptionRepository
	Nodes          NodeRepository
	NodeGroups     NodeGroupRepository
	Rules          RuleRepository
	Probes         ProbeRepository
	GenerateConfig GenerateConfigRepository
	DNSConfig      DNSConfigRepository
}
