package publication_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

type dummyChainRepo struct {
	config *domain.ProxyChainConfig
}

func (d *dummyChainRepo) Get(ctx context.Context) (*domain.ProxyChainConfig, error) {
	if d.config == nil {
		return &domain.ProxyChainConfig{
			Bindings: []domain.ProxyChainBinding{},
			Version:  domain.ProxyChainConfigVersion(nil),
		}, nil
	}
	return d.config, nil
}

func (d *dummyChainRepo) Replace(ctx context.Context, expectedVersion string, bindings []domain.ProxyChainBinding) (*domain.ProxyChainConfig, error) {
	d.config = &domain.ProxyChainConfig{
		Bindings: bindings,
		Version:  domain.ProxyChainConfigVersion(bindings),
	}
	return d.config, nil
}

type dummyNodeSourceRepo struct {
	listByNodesCalled bool
}

func (d *dummyNodeSourceRepo) ListByNode(ctx context.Context, logicalID string) ([]domain.NodeSource, error) {
	return nil, nil
}

func (d *dummyNodeSourceRepo) ListByNodes(ctx context.Context, logicalIDs []string) (map[string][]domain.NodeSource, error) {
	d.listByNodesCalled = true
	res := make(map[string][]domain.NodeSource, len(logicalIDs))
	for _, id := range logicalIDs {
		res[id] = []domain.NodeSource{
			{SubscriptionID: "sub-1", NodeLogicalID: id},
		}
	}
	return res, nil
}

func (d *dummyNodeSourceRepo) ListBySubscription(ctx context.Context, subID string) ([]domain.NodeSource, error) {
	return nil, nil
}

func (d *dummyNodeSourceRepo) Upsert(ctx context.Context, src *domain.NodeSource) error {
	return nil
}

func (d *dummyNodeSourceRepo) DeleteBySubscriptionAndFetch(ctx context.Context, subID string, currentFetchID string) error {
	return nil
}

type dummySubscriptionRepo struct {
	subs map[string]*domain.Subscription
}

type mockResolver struct {
	resolveFn func(ctx context.Context, input resolver.ResolveInput) (*resolver.ResolvedPolicySnapshot, error)
}

func (m *mockResolver) Resolve(ctx context.Context, input resolver.ResolveInput) (*resolver.ResolvedPolicySnapshot, error) {
	if m.resolveFn != nil {
		return m.resolveFn(ctx, input)
	}
	return resolver.New().Resolve(ctx, input)
}

func newDummySubscriptionRepo(subs []*domain.Subscription) *dummySubscriptionRepo {
	m := &dummySubscriptionRepo{
		subs: make(map[string]*domain.Subscription),
	}
	for _, s := range subs {
		sCopy := *s
		m.subs[s.ID] = &sCopy
	}
	return m
}

func (d *dummySubscriptionRepo) GetByID(ctx context.Context, id string) (*domain.Subscription, error) {
	sub, ok := d.subs[id]
	if !ok {
		return nil, domain.NewNotFoundError("subscription_not_found", "subscription not found")
	}
	subCopy := *sub
	return &subCopy, nil
}

func (d *dummySubscriptionRepo) GetPrimary(ctx context.Context) (*domain.Subscription, error) {
	return nil, domain.NewNotFoundError("subscription_not_found", "no primary subscription")
}

func (d *dummySubscriptionRepo) List(ctx context.Context, filter domain.SubscriptionFilter) ([]domain.Subscription, int, error) {
	res := make([]domain.Subscription, 0, len(d.subs))
	for _, s := range d.subs {
		res = append(res, *s)
	}
	return res, len(res), nil
}

func (d *dummySubscriptionRepo) Create(ctx context.Context, sub *domain.Subscription) error {
	sCopy := *sub
	d.subs[sub.ID] = &sCopy
	return nil
}

func (d *dummySubscriptionRepo) Update(ctx context.Context, sub *domain.Subscription) error {
	sCopy := *sub
	d.subs[sub.ID] = &sCopy
	return nil
}

func (d *dummySubscriptionRepo) Delete(ctx context.Context, id string) error {
	delete(d.subs, id)
	return nil
}

func testValidSSNode(id, name, server string, port int) *domain.Node {
	return &domain.Node{
		LogicalID:   id,
		DisplayName: name,
		Protocol:    domain.ProtocolSS,
		Server:      server,
		Port:        port,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-256-gcm",
			Password: "secret-password",
		},
		Active: true,
	}
}

func TestPublication_WithProxyChainRepository_LoadsBindingsAndSources(t *testing.T) {
	ctx := context.Background()

	polRepo := newMockPolicyRepo()
	revRepo := &mockRevisionRepo{
		revisions: map[string]*domain.ConfigurationRevision{
			"rev-1": {ID: "rev-1", State: domain.RevisionStateActive},
		},
		activeID: "rev-1",
	}
	nodeRepo := &mockNodeRepo{
		nodes: map[string]*domain.Node{
			"node-1": testValidSSNode("node-1", "Node 1", "1.1.1.1", 8388),
		},
	}
	sourceRepo := &dummyNodeSourceRepo{}
	subRepo := newDummySubscriptionRepo([]*domain.Subscription{
		{ID: "sub-1", Name: "Sub 1", Enabled: true},
	})
	chainRepo := &dummyChainRepo{
		config: &domain.ProxyChainConfig{
			Bindings: []domain.ProxyChainBinding{
				{
					ID:         "b-sub",
					TargetType: domain.ProxyChainTargetSubscription,
					TargetID:   "sub-1",
					DialerType: domain.ProxyChainDialerNode,
					DialerID:   "node-2",
					Enabled:    true,
					SortOrder:  1,
					Revision:   1,
				},
			},
			Version: "v1",
		},
	}

	svc := publication.NewService(
		newMockPublicationRepo(),
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithProxyChainRepository(chainRepo),
		publication.WithChainSubscriptionRepository(subRepo),
		publication.WithResolver(&mockResolver{}),
	)

	input, err := svc.AssembleResolveInput(ctx, "")
	if err != nil {
		t.Fatalf("AssembleResolveInput failed: %v", err)
	}

	if len(input.ChainBindings) != 1 || input.ChainBindings[0].ID != "b-sub" {
		t.Fatalf("expected chain bindings loaded in resolve input: %v", input.ChainBindings)
	}
	if !sourceRepo.listByNodesCalled {
		t.Fatal("expected NodeSources to be loaded for enabled subscription binding even without filter dependency")
	}
}

func TestPublication_ResolveTargetSnapshot_RejectsNonMihomoWithActiveChain(t *testing.T) {
	ctx := context.Background()

	polRepo := newMockPolicyRepo()
	revRepo := &mockRevisionRepo{
		revisions: map[string]*domain.ConfigurationRevision{
			"rev-1": {ID: "rev-1", State: domain.RevisionStateActive},
		},
		activeID: "rev-1",
	}
	nodeRepo := &mockNodeRepo{
		nodes: map[string]*domain.Node{
			"node-1": testValidSSNode("node-1", "Node 1", "1.1.1.1", 8388),
			"node-2": testValidSSNode("node-2", "Node 2", "1.1.1.2", 8388),
		},
	}
	chainRepo := &dummyChainRepo{
		config: &domain.ProxyChainConfig{
			Bindings: []domain.ProxyChainBinding{
				{
					ID:         "b-node",
					TargetType: domain.ProxyChainTargetNode,
					TargetID:   "node-1",
					DialerType: domain.ProxyChainDialerNode,
					DialerID:   "node-2",
					Enabled:    true,
					SortOrder:  1,
					Revision:   1,
				},
			},
			Version: "v1",
		},
	}

	svc := publication.NewService(
		newMockPublicationRepo(),
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithProxyChainRepository(chainRepo),
		publication.WithResolver(&mockResolver{}),
	)

	// Non-Mihomo target (e.g. SingBox) must fail with unsupported_target_capability
	_, err := svc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetSingBox,
		ActorKind: domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatal("expected Publish to reject non-Mihomo target with active chain bindings")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "unsupported_target_capability" {
		t.Fatalf("expected unsupported_target_capability error, got: %v", err)
	}

	// Disabled binding must NOT reject non-Mihomo
	chainRepo.config.Bindings[0].Enabled = false
	_, err = svc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetSingBox,
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected Publish failure with disabled chain: %v", err)
	}
}

func TestPublication_AssembleResolveInput_ValidatesTargetSubscription(t *testing.T) {
	ctx := context.Background()

	polRepo := newMockPolicyRepo()
	revRepo := &mockRevisionRepo{
		revisions: map[string]*domain.ConfigurationRevision{
			"rev-1": {ID: "rev-1", State: domain.RevisionStateActive},
		},
		activeID: "rev-1",
	}
	nodeRepo := &mockNodeRepo{
		nodes: map[string]*domain.Node{
			"node-1": testValidSSNode("node-1", "Node 1", "1.1.1.1", 8388),
		},
	}
	sourceRepo := &dummyNodeSourceRepo{}

	// 1. Missing subscription rejected with 422 chain_reference_missing
	chainRepoMissing := &dummyChainRepo{
		config: &domain.ProxyChainConfig{
			Bindings: []domain.ProxyChainBinding{
				{
					ID:         "b-sub",
					TargetType: domain.ProxyChainTargetSubscription,
					TargetID:   "sub-missing",
					DialerType: domain.ProxyChainDialerNode,
					DialerID:   "node-2",
					Enabled:    true,
				},
			},
			Version: "v1",
		},
	}
	subRepo := newDummySubscriptionRepo([]*domain.Subscription{
		{ID: "sub-disabled", Name: "Sub Disabled", Enabled: false},
	})
	svcMissing := publication.NewService(
		newMockPublicationRepo(),
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithProxyChainRepository(chainRepoMissing),
		publication.WithChainSubscriptionRepository(subRepo),
		publication.WithResolver(&mockResolver{}),
	)
	_, err := svcMissing.AssembleResolveInput(ctx, "")
	if err == nil {
		t.Fatal("expected AssembleResolveInput to reject missing target subscription")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing, got: %v", err)
	}

	// 2. Disabled subscription rejected with 422 chain_reference_missing
	chainRepoDisabled := &dummyChainRepo{
		config: &domain.ProxyChainConfig{
			Bindings: []domain.ProxyChainBinding{
				{
					ID:         "b-sub",
					TargetType: domain.ProxyChainTargetSubscription,
					TargetID:   "sub-disabled",
					DialerType: domain.ProxyChainDialerNode,
					DialerID:   "node-2",
					Enabled:    true,
				},
			},
			Version: "v1",
		},
	}
	svcDisabled := publication.NewService(
		newMockPublicationRepo(),
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithProxyChainRepository(chainRepoDisabled),
		publication.WithChainSubscriptionRepository(subRepo),
		publication.WithResolver(&mockResolver{}),
	)
	_, err = svcDisabled.AssembleResolveInput(ctx, "")
	if err == nil {
		t.Fatal("expected AssembleResolveInput to reject disabled target subscription")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing, got: %v", err)
	}

	// 3. Disabled binding targeting missing or disabled subscription allowed
	chainRepoDisabledBinding := &dummyChainRepo{
		config: &domain.ProxyChainConfig{
			Bindings: []domain.ProxyChainBinding{
				{
					ID:         "b-sub-missing-but-disabled",
					TargetType: domain.ProxyChainTargetSubscription,
					TargetID:   "sub-missing",
					DialerType: domain.ProxyChainDialerNode,
					DialerID:   "node-2",
					Enabled:    false,
				},
				{
					ID:         "b-sub-disabled-and-binding-disabled",
					TargetType: domain.ProxyChainTargetSubscription,
					TargetID:   "sub-disabled",
					DialerType: domain.ProxyChainDialerNode,
					DialerID:   "node-2",
					Enabled:    false,
				},
			},
			Version: "v1",
		},
	}
	svcDisabledBinding := publication.NewService(
		newMockPublicationRepo(),
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithProxyChainRepository(chainRepoDisabledBinding),
		publication.WithChainSubscriptionRepository(subRepo),
		publication.WithResolver(&mockResolver{}),
	)
	input, err := svcDisabledBinding.AssembleResolveInput(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error for disabled bindings: %v", err)
	}
	if len(input.ChainBindings) != 2 {
		t.Fatalf("expected 2 chain bindings, got %d", len(input.ChainBindings))
	}

	// 4. Missing chainSubRepo when enabled subscription binding exists fails with internal error
	svcNoSubRepo := publication.NewService(
		newMockPublicationRepo(),
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithProxyChainRepository(chainRepoMissing),
		publication.WithResolver(&mockResolver{}),
	)
	_, err = svcNoSubRepo.AssembleResolveInput(ctx, "")
	if err == nil {
		t.Fatal("expected error when chainSubRepo is nil but enabled subscription binding exists")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "subscription_repository_unavailable" {
		t.Fatalf("expected subscription_repository_unavailable, got: %v", err)
	}
}

func TestPublication_AssembleResolveInput_LoadsNodeSourcesWhenChainRepoConfiguredEvenIfNoBindingsInDB(t *testing.T) {
	ctx := context.Background()

	polRepo := newMockPolicyRepo()
	revRepo := &mockRevisionRepo{
		revisions: map[string]*domain.ConfigurationRevision{
			"rev-1": {ID: "rev-1", State: domain.RevisionStateActive},
		},
		activeID: "rev-1",
	}
	nodeRepo := &mockNodeRepo{
		nodes: map[string]*domain.Node{
			"node-1": testValidSSNode("node-1", "Node 1", "1.1.1.1", 8388),
		},
	}
	sourceRepo := &dummyNodeSourceRepo{}
	// Empty chain repo in database (no bindings yet)
	emptyChainRepo := &dummyChainRepo{
		config: &domain.ProxyChainConfig{
			Bindings: []domain.ProxyChainBinding{},
			Version:  "v-empty",
		},
	}

	svc := publication.NewService(
		newMockPublicationRepo(),
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithProxyChainRepository(emptyChainRepo),
		publication.WithResolver(&mockResolver{}),
	)

	// AssembleResolveInput with includeGraph=true loads NodeSources from sourceRepo
	input, err := svc.AssembleResolveInput(ctx, "")
	if err != nil {
		t.Fatalf("AssembleResolveInput failed: %v", err)
	}
	if !sourceRepo.listByNodesCalled {
		t.Fatal("expected NodeSources to be loaded when chainRepo is configured and includeGraph is true, even with 0 saved bindings")
	}
	if input.NodeSources == nil || len(input.NodeSources["node-1"]) == 0 {
		t.Fatalf("expected populated NodeSources in resolve input: %v", input.NodeSources)
	}

	// Pure-node target export (includeGraph=false) does not require sourceRepo
	sourceRepoNoCall := &dummyNodeSourceRepo{}
	svcNoSource := publication.NewService(
		newMockPublicationRepo(),
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeSourceRepository(sourceRepoNoCall),
		publication.WithProxyChainRepository(emptyChainRepo),
		publication.WithResolver(&mockResolver{}),
	)
	_, err = svcNoSource.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetSingBox,
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected Publish failure for pure-node target: %v", err)
	}
	if sourceRepoNoCall.listByNodesCalled {
		t.Fatal("expected pure-node export (includeGraph=false) not to load NodeSources when no filter requires it")
	}
}
