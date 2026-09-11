package service

import (
	"context"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
)

// SubscriptionService orchestrates fetching, filtering, and updating subscription feeds.
type SubscriptionService interface {
	GetSubscription(ctx context.Context, id int64) (*domain.Subscription, error)
	ListSubscriptions(ctx context.Context, enabledOnly bool) ([]*domain.Subscription, error)
	CreateSubscription(ctx context.Context, sub *domain.Subscription) error
	UpdateSubscription(ctx context.Context, sub *domain.Subscription) error
	DeleteSubscription(ctx context.Context, id int64) error
	RefreshSubscription(ctx context.Context, id int64) error
	RefreshAll(ctx context.Context) error
}

// NodeService orchestrates node inventory management, deduplication, and lifecycle transitions.
type NodeService interface {
	GetNode(ctx context.Context, id int64) (*domain.Node, error)
	ListNodes(ctx context.Context, state domain.LifecycleState) ([]*domain.Node, error)
	CreateNode(ctx context.Context, node *domain.Node) error
	UpdateNode(ctx context.Context, node *domain.Node) error
	DeleteNode(ctx context.Context, id int64) error
	DeduplicateNodes(ctx context.Context) (int64, error)
}

// NodeGroupService orchestrates routing policy groups and node selection matching.
type NodeGroupService interface {
	GetGroup(ctx context.Context, id int64) (*domain.NodeGroup, error)
	ListGroups(ctx context.Context) ([]*domain.NodeGroup, error)
	CreateGroup(ctx context.Context, group *domain.NodeGroup) error
	UpdateGroup(ctx context.Context, group *domain.NodeGroup) error
	DeleteGroup(ctx context.Context, id int64) error
	ResolveGroupNodes(ctx context.Context, group *domain.NodeGroup) ([]string, error)
}

// RuleService orchestrates routing rules, ordering, and classification.
type RuleService interface {
	GetRule(ctx context.Context, id int64) (*domain.Rule, error)
	ListRules(ctx context.Context, enabledOnly bool) ([]*domain.Rule, error)
	CreateRule(ctx context.Context, rule *domain.Rule) error
	UpdateRule(ctx context.Context, rule *domain.Rule) error
	DeleteRule(ctx context.Context, id int64) error
}

// ProbeService orchestrates concurrent connectivity and capability probing of nodes.
type ProbeService interface {
	ProbeNode(ctx context.Context, node *domain.Node) (*domain.NodeProbeResult, error)
	ProbeAll(ctx context.Context, concurrency int) (<-chan *domain.NodeProbeResult, error)
	GetProbeResult(ctx context.Context, nodeKey string) (*domain.NodeProbeResult, error)
}

// GenerateService orchestrates the compilation and rendering of client configuration files.
type GenerateService interface {
	Generate(ctx context.Context, req *domain.GenerateRequest) ([]byte, string, error)
	GenerateResult(ctx context.Context, req *domain.GenerateRequest, subID *int64, filterKeyword string) (*compiler.Result, error)
	GetConfig(ctx context.Context) (*domain.GenerateConfig, error)
	UpdateConfig(ctx context.Context, cfg *domain.GenerateConfig) error
}
