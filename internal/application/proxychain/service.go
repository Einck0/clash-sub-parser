package proxychain

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// ResolveInputProvider supplies the canonical ResolveInput assembled by publication service.
type ResolveInputProvider func(ctx context.Context) (*resolver.ResolveInput, error)

// Service provides application-level use cases for proxy chain bindings.
type Service struct {
	repo          domain.ProxyChainRepository
	nodeRepo      domain.NodeRepository
	policyRepo    domain.PolicyRepository
	subRepo       domain.SubscriptionRepository
	auditRepo     domain.AuditRepository
	resolver      resolver.Resolver
	inputProvider ResolveInputProvider
}

// Option configures optional Service dependencies.
type Option func(*Service)

// WithAuditRepository sets the audit event repository.
func WithAuditRepository(repo domain.AuditRepository) Option {
	return func(s *Service) {
		s.auditRepo = repo
	}
}

// WithInputProvider sets the resolve input provider function.
func WithInputProvider(provider ResolveInputProvider) Option {
	return func(s *Service) {
		s.inputProvider = provider
	}
}

// WithResolver sets the deterministic policy resolver implementation.
func WithResolver(res resolver.Resolver) Option {
	return func(s *Service) {
		s.resolver = res
	}
}

// NewService constructs a new proxy chain application service.
func NewService(
	repo domain.ProxyChainRepository,
	nodeRepo domain.NodeRepository,
	policyRepo domain.PolicyRepository,
	subRepo domain.SubscriptionRepository,
	opts ...Option,
) *Service {
	s := &Service{
		repo:       repo,
		nodeRepo:   nodeRepo,
		policyRepo: policyRepo,
		subRepo:    subRepo,
		resolver:   resolver.New(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetInputProvider allows dynamic wiring of the resolve input provider.
func (s *Service) SetInputProvider(provider ResolveInputProvider) {
	s.inputProvider = provider
}

func (s *Service) recordAudit(ctx context.Context, actorKind domain.ActorKind, requestID, action string, result domain.AuditResult, summary string) {
	if s.auditRepo == nil {
		return
	}
	if actorKind == "" {
		actorKind = domain.ActorKindAdmin
	}
	id, err := domain.NewUUIDv7()
	if err != nil {
		return
	}
	event := domain.AuditEvent{
		ID:              id,
		ActorKind:       actorKind,
		RequestID:       requestID,
		Action:          action,
		Result:          result,
		RedactedSummary: domain.RedactSensitiveInfo(summary),
		CreatedAt:       domain.NowUTC(),
	}
	_ = s.auditRepo.Record(ctx, &event)
}

func mapSnapshotToEffects(version string, snap *resolver.ResolvedPolicySnapshot) *EffectsResult {
	res := &EffectsResult{
		Version: version,
		Effects: make([]resolver.ChainEffect, 0),
		Nodes:   make([]EffectNode, 0),
	}
	if snap == nil {
		return res
	}
	if len(snap.ChainEffects) > 0 {
		res.Effects = append(res.Effects, snap.ChainEffects...)
	}
	for _, n := range snap.Nodes {
		res.Nodes = append(res.Nodes, EffectNode{
			LogicalID:      n.LogicalID,
			DisplayName:    n.DisplayName,
			DialerProxy:    n.DialerProxy,
			ChainBindingID: n.ChainBindingID,
			ChainSource:    n.ChainSource,
		})
	}
	return res
}

func (s *Service) validateEnabledSubscriptionBindings(ctx context.Context, bindings []domain.ProxyChainBinding) error {
	for _, b := range bindings {
		if !b.Enabled || b.TargetType != domain.ProxyChainTargetSubscription {
			continue
		}
		if s.subRepo == nil {
			return domain.NewInternalError("subscription_repository_unavailable", "subscription repository required for enabled subscription target validation")
		}
		sub, err := s.subRepo.GetByID(ctx, b.TargetID)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if domErr, ok := domain.AsDomainError(err); ok && domErr.Category != domain.CategoryNotFound {
				return err
			}
			return domain.NewValidationError("chain_reference_missing", fmt.Sprintf("target subscription %q not found", b.TargetID))
		}
		if sub == nil {
			return domain.NewValidationError("chain_reference_missing", fmt.Sprintf("target subscription %q not found", b.TargetID))
		}
		if !sub.Enabled {
			return domain.NewValidationError("chain_reference_missing", fmt.Sprintf("target subscription %q is disabled", b.TargetID))
		}
	}
	return nil
}

// Get returns the current persisted proxy chain configuration.
func (s *Service) Get(ctx context.Context) (*domain.ProxyChainConfig, error) {
	return s.repo.Get(ctx)
}

// Options returns eligible nodes, policy groups, and subscriptions for selection.
func (s *Service) Options(ctx context.Context) (*OptionsResult, error) {
	res := &OptionsResult{
		Nodes:         make([]OptionItem, 0),
		Groups:        make([]OptionItem, 0),
		Subscriptions: make([]OptionItem, 0),
	}

	if s.nodeRepo != nil {
		nodes, err := s.nodeRepo.ListAll(ctx, domain.NodeFilter{
			Scope:          domain.NodeScopeEnabledSubscriptions,
			ActiveOnly:     true,
			ExcludeNotices: true,
		})
		if err != nil {
			return nil, domain.NewInternalError("node_inventory_failed", fmt.Sprintf("failed to read nodes for options: %v", err))
		}
		for _, n := range nodes {
			name := n.DisplayName
			if strings.TrimSpace(name) == "" {
				name = n.LogicalID
			}
			res.Nodes = append(res.Nodes, OptionItem{
				ID:   n.LogicalID,
				Name: name,
			})
		}
		sort.SliceStable(res.Nodes, func(i, j int) bool {
			if res.Nodes[i].Name != res.Nodes[j].Name {
				return res.Nodes[i].Name < res.Nodes[j].Name
			}
			return res.Nodes[i].ID < res.Nodes[j].ID
		})
	}

	if s.policyRepo != nil {
		groups, err := s.policyRepo.ListGroups(ctx)
		if err != nil {
			return nil, domain.NewInternalError("group_inventory_failed", fmt.Sprintf("failed to read groups for options: %v", err))
		}
		for _, g := range groups {
			res.Groups = append(res.Groups, OptionItem{
				ID:   g.ID,
				Name: g.Name,
			})
		}
		sort.SliceStable(res.Groups, func(i, j int) bool {
			if res.Groups[i].Name != res.Groups[j].Name {
				return res.Groups[i].Name < res.Groups[j].Name
			}
			return res.Groups[i].ID < res.Groups[j].ID
		})
	}

	if s.subRepo != nil {
		page := 1
		for {
			subs, total, err := s.subRepo.List(ctx, domain.SubscriptionFilter{
				Pagination: domain.Pagination{
					Page:     page,
					PageSize: 100,
				},
			})
			if err != nil {
				return nil, domain.NewInternalError("subscription_inventory_failed", fmt.Sprintf("failed to read subscriptions for options: %v", err))
			}
			for _, sub := range subs {
				res.Subscriptions = append(res.Subscriptions, OptionItem{
					ID:   sub.ID,
					Name: sub.Name,
				})
			}
			if len(subs) == 0 || len(res.Subscriptions) >= total {
				break
			}
			page++
		}
		sort.SliceStable(res.Subscriptions, func(i, j int) bool {
			if res.Subscriptions[i].Name != res.Subscriptions[j].Name {
				return res.Subscriptions[i].Name < res.Subscriptions[j].Name
			}
			return res.Subscriptions[i].ID < res.Subscriptions[j].ID
		})
	}

	return res, nil
}

// Effects evaluates the current persisted bindings against the current resolve input.
func (s *Service) Effects(ctx context.Context) (*EffectsResult, error) {
	if s.inputProvider == nil {
		return nil, domain.NewInternalError("input_provider_unavailable", "resolve input provider not configured")
	}
	currentConfig, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.validateEnabledSubscriptionBindings(ctx, currentConfig.Bindings); err != nil {
		return nil, err
	}

	baseInput, err := s.inputProvider(ctx)
	if err != nil {
		return nil, err
	}
	input := *baseInput
	input.ChainBindings = currentConfig.Bindings

	snap, err := s.resolver.Resolve(ctx, input)
	if err != nil {
		return nil, err
	}

	return mapSnapshotToEffects(currentConfig.Version, snap), nil
}

// Preview tests candidate binding effects without persisting changes.
func (s *Service) Preview(ctx context.Context, cmd PreviewCommand) (*EffectsResult, error) {
	currentConfig, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if cmd.ConfigVersion != "" && cmd.ConfigVersion != currentConfig.Version {
		return nil, domain.NewConflictError("proxy_chain_conflict", fmt.Sprintf("proxy chain config version mismatch: expected %s, got %s", cmd.ConfigVersion, currentConfig.Version))
	}

	candidate := cmd.Binding
	if strings.TrimSpace(candidate.ID) == "" {
		candidate.ID = "preview-temp-binding"
	}
	if err := candidate.Validate(); err != nil {
		return nil, err
	}

	if s.inputProvider == nil {
		return nil, domain.NewInternalError("input_provider_unavailable", "resolve input provider not configured")
	}
	baseInput, err := s.inputProvider(ctx)
	if err != nil {
		return nil, err
	}

	candidateBindings := make([]domain.ProxyChainBinding, 0, len(currentConfig.Bindings)+1)
	replaced := false
	for _, b := range currentConfig.Bindings {
		if b.ID == candidate.ID {
			candidateBindings = append(candidateBindings, candidate)
			replaced = true
		} else {
			candidateBindings = append(candidateBindings, b)
		}
	}
	if !replaced {
		candidateBindings = append(candidateBindings, candidate)
	}

	if err := s.validateEnabledSubscriptionBindings(ctx, candidateBindings); err != nil {
		return nil, err
	}

	input := *baseInput
	input.ChainBindings = candidateBindings

	snap, err := s.resolver.Resolve(ctx, input)
	if err != nil {
		return nil, err
	}

	return mapSnapshotToEffects(currentConfig.Version, snap), nil
}

// Create inserts a new binding with optimistic version check and full candidate resolution.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*CreateResult, error) {
	if strings.TrimSpace(cmd.Binding.ID) != "" {
		return nil, domain.NewValidationError("invalid_chain_binding", "new binding ID must be empty")
	}
	newID, err := domain.NewUUIDv7()
	if err != nil {
		return nil, domain.NewInternalError("uuid_generation_failed", "failed to generate binding ID")
	}
	cmd.Binding.ID = newID
	cmd.Binding.Revision = 1

	if err := cmd.Binding.Validate(); err != nil {
		return nil, err
	}

	currentConfig, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if cmd.ConfigVersion != "" && cmd.ConfigVersion != currentConfig.Version {
		return nil, domain.NewConflictError("proxy_chain_conflict", fmt.Sprintf("proxy chain config version mismatch: expected %s, got %s", cmd.ConfigVersion, currentConfig.Version))
	}

	candidateBindings := append([]domain.ProxyChainBinding(nil), currentConfig.Bindings...)
	candidateBindings = append(candidateBindings, cmd.Binding)

	if err := s.validateEnabledSubscriptionBindings(ctx, candidateBindings); err != nil {
		return nil, err
	}

	// Validate candidate resolution before write
	if s.inputProvider != nil {
		baseInput, err := s.inputProvider(ctx)
		if err != nil {
			return nil, err
		}
		input := *baseInput
		input.ChainBindings = candidateBindings
		if _, err := s.resolver.Resolve(ctx, input); err != nil {
			return nil, err
		}
	}

	newConfig, err := s.repo.Replace(ctx, currentConfig.Version, candidateBindings)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "proxy_chain_create", "success", fmt.Sprintf("created proxy chain binding %s (target=%s:%s, dialer=%s:%s)", cmd.Binding.ID, cmd.Binding.TargetType, cmd.Binding.TargetID, cmd.Binding.DialerType, cmd.Binding.DialerID))

	return &CreateResult{
		Binding: cmd.Binding,
		Version: newConfig.Version,
	}, nil
}

// Update updates an existing binding with path ID consistency and optimistic revision check.
func (s *Service) Update(ctx context.Context, id string, cmd UpdateCommand) (*UpdateResult, error) {
	if strings.TrimSpace(id) == "" {
		return nil, domain.NewValidationError("invalid_chain_binding", "binding ID must not be empty")
	}
	if cmd.Binding.ID != "" && cmd.Binding.ID != id {
		return nil, domain.NewValidationError("invalid_chain_binding", "path ID and binding ID do not match")
	}
	cmd.Binding.ID = id
	if err := cmd.Binding.Validate(); err != nil {
		return nil, err
	}

	currentConfig, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if cmd.ConfigVersion != "" && cmd.ConfigVersion != currentConfig.Version {
		return nil, domain.NewConflictError("proxy_chain_conflict", fmt.Sprintf("proxy chain config version mismatch: expected %s, got %s", cmd.ConfigVersion, currentConfig.Version))
	}

	var existing *domain.ProxyChainBinding
	existingIndex := -1
	for i, b := range currentConfig.Bindings {
		if b.ID == id {
			existing = &currentConfig.Bindings[i]
			existingIndex = i
			break
		}
	}
	if existing == nil {
		return nil, domain.NewNotFoundError("chain_binding_not_found", fmt.Sprintf("chain binding %s not found", id))
	}

	if cmd.ExpectedRevision > 0 && existing.Revision != cmd.ExpectedRevision {
		conflict := domain.NewConflictError("proxy_chain_conflict", fmt.Sprintf("chain binding revision mismatch: expected %d, got %d", cmd.ExpectedRevision, existing.Revision))
		conflict.CurrentRevision = &existing.Revision
		return nil, conflict
	}

	updatedBinding := cmd.Binding
	updatedBinding.Revision = existing.Revision + 1

	candidateBindings := make([]domain.ProxyChainBinding, len(currentConfig.Bindings))
	copy(candidateBindings, currentConfig.Bindings)
	candidateBindings[existingIndex] = updatedBinding

	if err := s.validateEnabledSubscriptionBindings(ctx, candidateBindings); err != nil {
		return nil, err
	}

	// Validate candidate resolution before write
	if s.inputProvider != nil {
		baseInput, err := s.inputProvider(ctx)
		if err != nil {
			return nil, err
		}
		input := *baseInput
		input.ChainBindings = candidateBindings
		if _, err := s.resolver.Resolve(ctx, input); err != nil {
			return nil, err
		}
	}

	newConfig, err := s.repo.Replace(ctx, currentConfig.Version, candidateBindings)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "proxy_chain_update", "success", fmt.Sprintf("updated proxy chain binding %s (rev %d -> %d)", id, existing.Revision, updatedBinding.Revision))

	return &UpdateResult{
		Binding: updatedBinding,
		Version: newConfig.Version,
	}, nil
}

// Delete removes an existing binding with optimistic version and revision checks.
func (s *Service) Delete(ctx context.Context, id string, cmd DeleteCommand) (*DeleteResult, error) {
	if strings.TrimSpace(id) == "" {
		return nil, domain.NewValidationError("invalid_chain_binding", "binding ID must not be empty")
	}

	currentConfig, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if cmd.ConfigVersion != "" && cmd.ConfigVersion != currentConfig.Version {
		return nil, domain.NewConflictError("proxy_chain_conflict", fmt.Sprintf("proxy chain config version mismatch: expected %s, got %s", cmd.ConfigVersion, currentConfig.Version))
	}

	var existing *domain.ProxyChainBinding
	candidateBindings := make([]domain.ProxyChainBinding, 0, len(currentConfig.Bindings))
	for _, b := range currentConfig.Bindings {
		if b.ID == id {
			bCopy := b
			existing = &bCopy
		} else {
			candidateBindings = append(candidateBindings, b)
		}
	}
	if existing == nil {
		return nil, domain.NewNotFoundError("chain_binding_not_found", fmt.Sprintf("chain binding %s not found", id))
	}

	if cmd.ExpectedRevision > 0 && existing.Revision != cmd.ExpectedRevision {
		conflict := domain.NewConflictError("proxy_chain_conflict", fmt.Sprintf("chain binding revision mismatch: expected %d, got %d", cmd.ExpectedRevision, existing.Revision))
		conflict.CurrentRevision = &existing.Revision
		return nil, conflict
	}

	if err := s.validateEnabledSubscriptionBindings(ctx, candidateBindings); err != nil {
		return nil, err
	}

	// Deleting a higher-priority binding which exposes an unsafe lower binding must fail 422
	if s.inputProvider != nil {
		baseInput, err := s.inputProvider(ctx)
		if err != nil {
			return nil, err
		}
		input := *baseInput
		input.ChainBindings = candidateBindings
		if _, err := s.resolver.Resolve(ctx, input); err != nil {
			return nil, err
		}
	}

	newConfig, err := s.repo.Replace(ctx, currentConfig.Version, candidateBindings)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "proxy_chain_delete", "success", fmt.Sprintf("deleted proxy chain binding %s", id))

	return &DeleteResult{
		Version: newConfig.Version,
	}, nil
}
