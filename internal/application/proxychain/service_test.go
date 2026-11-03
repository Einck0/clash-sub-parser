package proxychain_test

import (
	"context"
	"errors"
	"testing"

	"clash-sub-parser/internal/application/proxychain"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// mockProxyChainRepo implements domain.ProxyChainRepository in memory.
type mockProxyChainRepo struct {
	config *domain.ProxyChainConfig
}

func newMockProxyChainRepo(bindings []domain.ProxyChainBinding) *mockProxyChainRepo {
	bCopy := append([]domain.ProxyChainBinding(nil), bindings...)
	return &mockProxyChainRepo{
		config: &domain.ProxyChainConfig{
			Bindings: bCopy,
			Version:  domain.ProxyChainConfigVersion(bCopy),
		},
	}
}

func (m *mockProxyChainRepo) Get(ctx context.Context) (*domain.ProxyChainConfig, error) {
	bCopy := append([]domain.ProxyChainBinding(nil), m.config.Bindings...)
	return &domain.ProxyChainConfig{
		Bindings: bCopy,
		Version:  m.config.Version,
	}, nil
}

func (m *mockProxyChainRepo) Replace(ctx context.Context, expectedVersion string, bindings []domain.ProxyChainBinding) (*domain.ProxyChainConfig, error) {
	if strings.TrimSpace(expectedVersion) == "" {
		return nil, domain.NewValidationError("invalid_chain_binding", "expected config_version must not be blank")
	}
	if m.config.Version != expectedVersion {
		return nil, domain.NewConflictError("proxy_chain_conflict", "version mismatch")
	}
	bCopy := append([]domain.ProxyChainBinding(nil), bindings...)
	newVer := domain.ProxyChainConfigVersion(bCopy)
	m.config = &domain.ProxyChainConfig{
		Bindings: bCopy,
		Version:  newVer,
	}
	return &domain.ProxyChainConfig{
		Bindings: bCopy,
		Version:  newVer,
	}, nil
}

// mockAuditRepo captures audit events in memory.
type mockAuditRepo struct {
	events []*domain.AuditEvent
}

func (m *mockAuditRepo) Record(ctx context.Context, event *domain.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockAuditRepo) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEvent, int, error) {
	return nil, 0, nil
}

func (m *mockAuditRepo) DeleteOlderThan(ctx context.Context, age int) error {
	return nil
}

// mockResolver intercepts Resolve calls.
type mockResolver struct {
	resolveFn func(ctx context.Context, input resolver.ResolveInput) (*resolver.ResolvedPolicySnapshot, error)
}

func (m *mockResolver) Resolve(ctx context.Context, input resolver.ResolveInput) (*resolver.ResolvedPolicySnapshot, error) {
	if m.resolveFn != nil {
		return m.resolveFn(ctx, input)
	}
	return &resolver.ResolvedPolicySnapshot{
		Nodes: []resolver.ResolvedNode{
			{LogicalID: "node-1", DisplayName: "Node 1", DialerProxy: "Group A"},
		},
		ChainEffects: []resolver.ChainEffect{
			{BindingID: "b-1", TargetCount: 1, ChainCount: 1},
		},
	}, nil
}

func TestService_GetAndOptions(t *testing.T) {
	ctx := context.Background()
	repo := newMockProxyChainRepo([]domain.ProxyChainBinding{
		{
			ID:         "b-1",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-2",
			Enabled:    true,
			SortOrder:  1,
			Revision:   1,
		},
	})

	svc := proxychain.NewService(repo, nil, nil, nil)

	cfg, err := svc.Get(ctx)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(cfg.Bindings) != 1 || cfg.Bindings[0].ID != "b-1" {
		t.Fatalf("unexpected bindings: %v", cfg.Bindings)
	}

	opts, err := svc.Options(ctx)
	if err != nil {
		t.Fatalf("Options failed: %v", err)
	}
	if opts.Nodes == nil || opts.Groups == nil || opts.Subscriptions == nil {
		t.Fatal("expected non-nil slices in OptionsResult")
	}
}

func TestService_EffectsAndPreview(t *testing.T) {
	ctx := context.Background()
	repo := newMockProxyChainRepo([]domain.ProxyChainBinding{
		{
			ID:         "b-1",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-1",
			DialerType: domain.ProxyChainDialerNodeGroup,
			DialerID:   "group-A",
			Enabled:    true,
			SortOrder:  1,
			Revision:   1,
		},
	})

	res := &mockResolver{}
	svc := proxychain.NewService(repo, nil, nil, nil, proxychain.WithResolver(res))
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{
			RevisionID: "rev-1",
		}, nil
	})

	// Effects
	eff, err := svc.Effects(ctx)
	if err != nil {
		t.Fatalf("Effects failed: %v", err)
	}
	if len(eff.Nodes) != 1 || eff.Nodes[0].DialerProxy != "Group A" {
		t.Fatalf("unexpected Effects nodes: %v", eff.Nodes)
	}

	// Preview with stale config_version fails 409
	_, err = svc.Preview(ctx, proxychain.PreviewCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-3",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-4",
			Enabled:    true,
			SortOrder:  2,
			Revision:   1,
		},
		ConfigVersion: "stale-version",
	})
	if err == nil {
		t.Fatal("expected conflict error on stale config version preview")
	}

	// Preview with valid config succeeds without writing to repo
	prev, err := svc.Preview(ctx, proxychain.PreviewCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "node-3",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-4",
			Enabled:    true,
			SortOrder:  2,
			Revision:   1,
		},
		ConfigVersion: repo.config.Version,
	})
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if len(prev.Nodes) != 1 {
		t.Fatalf("expected 1 node in preview effects, got %d", len(prev.Nodes))
	}
	// Verify repo wasn't written to
	if len(repo.config.Bindings) != 1 {
		t.Fatalf("expected repo bindings count to remain 1, got %d", len(repo.config.Bindings))
	}
}

func testSSNode(id, name, server string, port int) domain.Node {
	return domain.Node{
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

func TestService_Create_ValidationAndAudit(t *testing.T) {
	ctx := context.Background()
	n1ID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	n2ID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)
	repo := newMockProxyChainRepo(nil)
	audit := &mockAuditRepo{}
	subRepo := newMockSubscriptionRepo([]*domain.Subscription{
		{ID: "sub-1", Name: "Sub 1", Enabled: true},
	})

	svc := proxychain.NewService(repo, nil, nil, subRepo, proxychain.WithAuditRepository(audit))
	nodes := []domain.Node{
		testSSNode(n1ID, "Node 1", "1.1.1.1", 8388),
		testSSNode(n2ID, "Node 2", "1.1.1.2", 8388),
	}
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{
			Nodes: nodes,
			NodeSources: map[string][]domain.NodeSource{
				n1ID: {{SubscriptionID: "sub-1", NodeLogicalID: n1ID}},
				n2ID: {{SubscriptionID: "sub-1", NodeLogicalID: n2ID}},
			},
		}, nil
	})

	// 1. Caller provides nonempty ID -> rejected (422)
	_, err := svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			ID:         "client-specified-id",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   n1ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   n2ID,
			Enabled:    true,
		},
		ConfigVersion: repo.config.Version,
	})
	if err == nil {
		t.Fatal("expected validation error for client-specified ID")
	}

	// 2. Caller provides invalid binding -> rejected (422)
	_, err = svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: "unknown-type",
			TargetID:   n1ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   n2ID,
			Enabled:    true,
		},
		ConfigVersion: repo.config.Version,
	})
	if err == nil {
		t.Fatal("expected validation error for invalid target type")
	}

	// 3. Stale config version -> conflict (409)
	_, err = svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   n1ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   n2ID,
			Enabled:    true,
		},
		ConfigVersion: "wrong-version",
	})
	if err == nil {
		t.Fatal("expected conflict for wrong config version")
	}

	// 4. Successful creation
	created, err := svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   n1ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   n2ID,
			Enabled:    true,
			SortOrder:  5,
			Note:       "chain 1",
		},
		ConfigVersion: repo.config.Version,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.Binding.ID == "" {
		t.Fatal("expected assigned server UUIDv7")
	}
	if created.Binding.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", created.Binding.Revision)
	}
	if len(repo.config.Bindings) != 1 {
		t.Fatalf("expected 1 binding in repo, got %d", len(repo.config.Bindings))
	}
	if len(audit.events) != 1 || audit.events[0].Action != "proxy_chain_create" {
		t.Fatalf("expected audit event for proxy_chain_create, got: %v", audit.events)
	}
}

func TestService_Update_Lifecycle(t *testing.T) {
	ctx := context.Background()
	n1ID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	n2ID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)
	n3ID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.3", 8388, nil)
	repo := newMockProxyChainRepo([]domain.ProxyChainBinding{
		{
			ID:         "b-1",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   n1ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   n2ID,
			Enabled:    true,
			SortOrder:  10,
			Revision:   1,
		},
	})
	audit := &mockAuditRepo{}
	subRepo := newMockSubscriptionRepo([]*domain.Subscription{
		{ID: "sub-1", Name: "Sub 1", Enabled: true},
	})

	svc := proxychain.NewService(repo, nil, nil, subRepo, proxychain.WithAuditRepository(audit))
	nodes := []domain.Node{
		testSSNode(n1ID, "Node 1", "1.1.1.1", 8388),
		testSSNode(n2ID, "Node 2", "1.1.1.2", 8388),
		testSSNode(n3ID, "Node 3", "1.1.1.3", 8388),
	}
	g1ID := "018f0000-0000-7000-8000-000000000001"
	g2ID := "018f0000-0000-7000-8000-000000000002"
	groups := []domain.NodeGroup{
		{
			ID:        g1ID,
			Name:      "Group 1",
			GroupType: domain.GroupTypeSelect,
			Sources: []domain.MemberSource{
				{
					Operation:     domain.OperationAdd,
					Kind:          domain.SourceKindManualNode,
					Position:      0,
					NodeLogicalID: &n2ID,
				},
			},
		},
		{
			ID:        g2ID,
			Name:      "Group 2",
			GroupType: domain.GroupTypeSelect,
			Sources: []domain.MemberSource{
				{
					Operation:     domain.OperationAdd,
					Kind:          domain.SourceKindManualNode,
					Position:      0,
					NodeLogicalID: &n3ID,
				},
			},
		},
	}
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{
			Nodes:  nodes,
			Groups: groups,
			NodeSources: map[string][]domain.NodeSource{
				n1ID: {{SubscriptionID: "sub-1", NodeLogicalID: n1ID}},
				n2ID: {{SubscriptionID: "sub-1", NodeLogicalID: n2ID}},
				n3ID: {{SubscriptionID: "sub-1", NodeLogicalID: n3ID}},
			},
		}, nil
	})

	// 1. Path ID mismatch -> 422
	_, err := svc.Update(ctx, "b-1", proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			ID:         "b-other",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   n1ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   n2ID,
		},
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 1,
	})
	if err == nil {
		t.Fatal("expected validation error on ID mismatch")
	}

	// 2. Not found -> 404
	_, err = svc.Update(ctx, "b-missing", proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   n1ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   n2ID,
		},
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 1,
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Category != domain.CategoryNotFound {
		t.Fatalf("expected not found category, got %v", err)
	}

	// 3. Revision mismatch -> 409
	_, err = svc.Update(ctx, "b-1", proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   n1ID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   n2ID,
			Enabled:    true,
		},
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 99,
	})
	if err == nil {
		t.Fatal("expected conflict on revision mismatch")
	}

	// 4. Successful update increments revision
	res, err := svc.Update(ctx, "b-1", proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNodeGroup,
			TargetID:   g1ID,
			DialerType: domain.ProxyChainDialerNodeGroup,
			DialerID:   g2ID,
			Enabled:    true,
			SortOrder:  20,
			Note:       "updated note",
		},
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if res.Binding.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", res.Binding.Revision)
	}
	if repo.config.Bindings[0].Revision != 2 {
		t.Fatalf("expected repo revision 2, got %d", repo.config.Bindings[0].Revision)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "proxy_chain_update" {
		t.Fatalf("expected audit event for proxy_chain_update, got: %v", audit.events)
	}
}

func TestService_Delete_UnsafeLowerExposureRejection(t *testing.T) {
	ctx := context.Background()
	repo := newMockProxyChainRepo([]domain.ProxyChainBinding{
		{
			ID:         "b-high",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
			SortOrder:  1,
			Revision:   1,
		},
		{
			ID:         "b-low-cyclic",
			TargetType: domain.ProxyChainTargetNodeGroup,
			TargetID:   "g1",
			DialerType: domain.ProxyChainDialerNodeGroup,
			DialerID:   "g2",
			Enabled:    true,
			SortOrder:  10,
			Revision:   1,
		},
	})
	audit := &mockAuditRepo{}

	// Resolver simulates a cycle error when b-high is removed and b-low-cyclic is evaluated
	res := &mockResolver{
		resolveFn: func(ctx context.Context, input resolver.ResolveInput) (*resolver.ResolvedPolicySnapshot, error) {
			for _, b := range input.ChainBindings {
				if b.ID == "b-high" {
					return &resolver.ResolvedPolicySnapshot{}, nil
				}
			}
			// When b-high is missing, simulate cycle detection failure
			return nil, domain.NewValidationError("chain_cycle", "cycle detected in lower binding")
		},
	}

	svc := proxychain.NewService(repo, nil, nil, nil,
		proxychain.WithResolver(res),
		proxychain.WithAuditRepository(audit),
	)
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{}, nil
	})

	// Deleting b-high exposes b-low-cyclic which fails resolver validation -> 422
	_, err := svc.Delete(ctx, "b-high", proxychain.DeleteCommand{
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 1,
	})
	if err == nil {
		t.Fatal("expected delete to fail when unsafe lower binding is exposed")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_cycle" {
		t.Fatalf("expected chain_cycle validation error, got: %v", err)
	}
	// Verify repo wasn't modified
	if len(repo.config.Bindings) != 2 {
		t.Fatalf("expected repo bindings count to remain 2, got %d", len(repo.config.Bindings))
	}

	// Deleting b-low-cyclic succeeds because b-high remains safe
	res.resolveFn = nil // default mock returns success
	delRes, err := svc.Delete(ctx, "b-low-cyclic", proxychain.DeleteCommand{
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if len(repo.config.Bindings) != 1 || repo.config.Bindings[0].ID != "b-high" {
		t.Fatalf("expected only b-high to remain, got %v", repo.config.Bindings)
	}
	if delRes.Version != repo.config.Version {
		t.Fatalf("expected version %s, got %s", repo.config.Version, delRes.Version)
	}
}

// mockSubscriptionRepo implements domain.SubscriptionRepository in memory.
type mockSubscriptionRepo struct {
	subs map[string]*domain.Subscription
}

func newMockSubscriptionRepo(subs []*domain.Subscription) *mockSubscriptionRepo {
	m := &mockSubscriptionRepo{
		subs: make(map[string]*domain.Subscription),
	}
	for _, s := range subs {
		sCopy := *s
		m.subs[s.ID] = &sCopy
	}
	return m
}

func (m *mockSubscriptionRepo) GetByID(ctx context.Context, id string) (*domain.Subscription, error) {
	sub, ok := m.subs[id]
	if !ok {
		return nil, domain.NewNotFoundError("subscription_not_found", "subscription not found")
	}
	subCopy := *sub
	return &subCopy, nil
}

func (m *mockSubscriptionRepo) GetPrimary(ctx context.Context) (*domain.Subscription, error) {
	return nil, domain.NewNotFoundError("subscription_not_found", "no primary subscription")
}

func (m *mockSubscriptionRepo) List(ctx context.Context, filter domain.SubscriptionFilter) ([]domain.Subscription, int, error) {
	res := make([]domain.Subscription, 0, len(m.subs))
	for _, s := range m.subs {
		res = append(res, *s)
	}
	return res, len(res), nil
}

func (m *mockSubscriptionRepo) Create(ctx context.Context, sub *domain.Subscription) error {
	sCopy := *sub
	m.subs[sub.ID] = &sCopy
	return nil
}

func (m *mockSubscriptionRepo) Update(ctx context.Context, sub *domain.Subscription) error {
	sCopy := *sub
	m.subs[sub.ID] = &sCopy
	return nil
}

func (m *mockSubscriptionRepo) Delete(ctx context.Context, id string) error {
	delete(m.subs, id)
	return nil
}

var _ domain.SubscriptionRepository = (*mockSubscriptionRepo)(nil)

// Ensure unused error imports don't trigger compile errors
var _ = errors.New

func TestService_SubscriptionTarget_MissingAndDisabledRejection(t *testing.T) {
	ctx := context.Background()
	repo := newMockProxyChainRepo([]domain.ProxyChainBinding{
		{
			ID:         "b-existing",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
			SortOrder:  1,
			Revision:   1,
		},
	})
	subRepo := newMockSubscriptionRepo([]*domain.Subscription{
		{ID: "sub-disabled", Name: "Disabled Sub", Enabled: false},
	})
	res := &mockResolver{}

	svc := proxychain.NewService(repo, nil, nil, subRepo, proxychain.WithResolver(res))
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{}, nil
	})

	// 1. Preview with missing subscription -> 422 chain_reference_missing
	_, err := svc.Preview(ctx, proxychain.PreviewCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-missing",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		},
		ConfigVersion: repo.config.Version,
	})
	if err == nil {
		t.Fatal("expected Preview to reject missing target subscription")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing code, got: %v", err)
	}

	// 2. Preview with disabled subscription -> 422 chain_reference_missing
	_, err = svc.Preview(ctx, proxychain.PreviewCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-disabled",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		},
		ConfigVersion: repo.config.Version,
	})
	if err == nil {
		t.Fatal("expected Preview to reject disabled target subscription")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing code, got: %v", err)
	}

	// 3. Create with missing subscription -> 422 chain_reference_missing
	_, err = svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-missing",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		},
		ConfigVersion: repo.config.Version,
	})
	if err == nil {
		t.Fatal("expected Create to reject missing target subscription")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing code, got: %v", err)
	}

	// 4. Create with disabled subscription -> 422 chain_reference_missing
	_, err = svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-disabled",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		},
		ConfigVersion: repo.config.Version,
	})
	if err == nil {
		t.Fatal("expected Create to reject disabled target subscription")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing code, got: %v", err)
	}

	// 5. Update with missing subscription -> 422 chain_reference_missing
	_, err = svc.Update(ctx, "b-existing", proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-missing",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		},
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 1,
	})
	if err == nil {
		t.Fatal("expected Update to reject missing target subscription")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing code, got: %v", err)
	}

	// 6. Update with disabled subscription -> 422 chain_reference_missing
	_, err = svc.Update(ctx, "b-existing", proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-disabled",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		},
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 1,
	})
	if err == nil {
		t.Fatal("expected Update to reject disabled target subscription")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing code, got: %v", err)
	}
}

func TestService_SubscriptionTarget_DisabledBindingAllowed(t *testing.T) {
	ctx := context.Background()
	repo := newMockProxyChainRepo(nil)
	subRepo := newMockSubscriptionRepo([]*domain.Subscription{
		{ID: "sub-disabled", Name: "Disabled Sub", Enabled: false},
	})
	res := &mockResolver{}

	svc := proxychain.NewService(repo, nil, nil, subRepo, proxychain.WithResolver(res))
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{}, nil
	})

	// 1. Preview with disabled binding targeting missing subscription succeeds
	_, err := svc.Preview(ctx, proxychain.PreviewCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-missing",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    false,
		},
		ConfigVersion: repo.config.Version,
	})
	if err != nil {
		t.Fatalf("expected Preview to allow disabled binding with missing subscription: %v", err)
	}

	// 2. Preview with disabled binding targeting disabled subscription succeeds
	_, err = svc.Preview(ctx, proxychain.PreviewCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-disabled",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    false,
		},
		ConfigVersion: repo.config.Version,
	})
	if err != nil {
		t.Fatalf("expected Preview to allow disabled binding with disabled subscription: %v", err)
	}

	// 3. Create with disabled binding targeting missing subscription succeeds
	created, err := svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-missing",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    false,
		},
		ConfigVersion: repo.config.Version,
	})
	if err != nil {
		t.Fatalf("expected Create to allow disabled binding with missing subscription: %v", err)
	}
	if created.Binding.ID == "" || created.Binding.Revision != 1 {
		t.Fatalf("unexpected created binding: %+v", created.Binding)
	}

	// 4. Update disabled binding targeting disabled subscription succeeds
	updated, err := svc.Update(ctx, created.Binding.ID, proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-disabled",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    false,
		},
		ConfigVersion:    repo.config.Version,
		ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("expected Update to allow disabled binding with disabled subscription: %v", err)
	}
	if updated.Binding.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", updated.Binding.Revision)
	}
}

func TestService_SubscriptionTarget_FirstSubscriptionPreviewAccuracy(t *testing.T) {
	ctx := context.Background()
	// Repo has 0 bindings in DB initially
	repo := newMockProxyChainRepo(nil)
	subRepo := newMockSubscriptionRepo([]*domain.Subscription{
		{ID: "sub-1", Name: "Sub 1", Enabled: true},
	})

	var receivedInput resolver.ResolveInput
	res := &mockResolver{
		resolveFn: func(ctx context.Context, input resolver.ResolveInput) (*resolver.ResolvedPolicySnapshot, error) {
			receivedInput = input
			// Verify that candidate binding is present and node sources are available
			if len(input.ChainBindings) != 1 {
				t.Fatalf("expected 1 candidate binding in resolver input, got %d", len(input.ChainBindings))
			}
			if input.NodeSources == nil || len(input.NodeSources["node-1"]) == 0 {
				t.Fatalf("expected NodeSources to be provided for first subscription preview, got: %v", input.NodeSources)
			}
			return &resolver.ResolvedPolicySnapshot{
				Nodes: []resolver.ResolvedNode{
					{
						LogicalID:      "node-1",
						DisplayName:    "Node 1",
						DialerProxy:    "Jump Node",
						ChainBindingID: input.ChainBindings[0].ID,
						ChainSource:    string(domain.ProxyChainTargetSubscription),
					},
				},
				ChainEffects: []resolver.ChainEffect{
					{
						BindingID:   input.ChainBindings[0].ID,
						TargetCount: 1,
						ChainCount:  1,
					},
				},
			}, nil
		},
	}

	svc := proxychain.NewService(repo, nil, nil, subRepo, proxychain.WithResolver(res))

	// Input provider simulates publication's AssembleResolveInput which now loads NodeSources
	// when chainRepo is configured and includeGraph is true, even if 0 bindings are saved in DB.
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{
			RevisionID: "rev-1",
			NodeSources: map[string][]domain.NodeSource{
				"node-1": {{SubscriptionID: "sub-1", NodeLogicalID: "node-1"}},
			},
		}, nil
	})

	// Preview first subscription binding
	prev, err := svc.Preview(ctx, proxychain.PreviewCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   "sub-1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "node-jump",
			Enabled:    true,
			SortOrder:  1,
		},
		ConfigVersion: repo.config.Version,
	})
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if len(prev.Effects) != 1 || prev.Effects[0].ChainCount != 1 {
		t.Fatalf("expected 1 chain effect with ChainCount 1, got: %v", prev.Effects)
	}
	if len(prev.Nodes) != 1 || prev.Nodes[0].DialerProxy != "Jump Node" {
		t.Fatalf("expected affected node with DialerProxy, got: %v", prev.Nodes)
	}
	if prev.Nodes[0].ChainSource != string(domain.ProxyChainTargetSubscription) {
		t.Fatalf("expected ChainSource to be subscription, got: %s", prev.Nodes[0].ChainSource)
	}

	// Verify repo was not modified (remains 0 bindings)
	if len(repo.config.Bindings) != 0 {
		t.Fatalf("expected repo bindings count to remain 0 after preview, got %d", len(repo.config.Bindings))
	}
	if receivedInput.ChainBindings[0].TargetID != "sub-1" {
		t.Fatalf("expected resolver to receive candidate targetID sub-1, got: %s", receivedInput.ChainBindings[0].TargetID)
	}
}

func TestService_CASAndRevisionValidation(t *testing.T) {
	ctx := context.Background()
	initialBinding := domain.ProxyChainBinding{
		ID:         "b-test-1",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   "n1",
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   "n2",
		Enabled:    true,
		SortOrder:  1,
		Revision:   3,
	}
	repo := newMockProxyChainRepo([]domain.ProxyChainBinding{initialBinding})
	audit := &mockAuditRepo{}
	svc := proxychain.NewService(repo, nil, nil, nil,
		proxychain.WithResolver(&mockResolver{}),
		proxychain.WithAuditRepository(audit),
	)
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{}, nil
	})

	validVersion := repo.config.Version

	// 1. Preview checks
	t.Run("Preview config_version validation", func(t *testing.T) {
		sampleBinding := domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		}

		// Empty config_version -> 422
		_, err := svc.Preview(ctx, proxychain.PreviewCommand{
			Binding:       sampleBinding,
			ConfigVersion: "",
		})
		if err == nil {
			t.Fatal("expected validation error for empty ConfigVersion")
		}
		domErr, ok := domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Whitespace config_version -> 422
		_, err = svc.Preview(ctx, proxychain.PreviewCommand{
			Binding:       sampleBinding,
			ConfigVersion: "   ",
		})
		if err == nil {
			t.Fatal("expected validation error for whitespace ConfigVersion")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Stale config_version -> 409
		_, err = svc.Preview(ctx, proxychain.PreviewCommand{
			Binding:       sampleBinding,
			ConfigVersion: "stale-version",
		})
		if err == nil {
			t.Fatal("expected conflict error for stale ConfigVersion")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryConflict {
			t.Fatalf("expected CategoryConflict, got %v", err)
		}
	})

	// 2. Create checks
	t.Run("Create config_version validation", func(t *testing.T) {
		sampleBinding := domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n3",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n4",
			Enabled:    true,
		}

		// Empty config_version -> 422
		_, err := svc.Create(ctx, proxychain.CreateCommand{
			Binding:       sampleBinding,
			ConfigVersion: "",
		})
		if err == nil {
			t.Fatal("expected validation error for empty ConfigVersion")
		}
		domErr, ok := domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Whitespace config_version -> 422
		_, err = svc.Create(ctx, proxychain.CreateCommand{
			Binding:       sampleBinding,
			ConfigVersion: "   ",
		})
		if err == nil {
			t.Fatal("expected validation error for whitespace ConfigVersion")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Stale config_version -> 409
		_, err = svc.Create(ctx, proxychain.CreateCommand{
			Binding:       sampleBinding,
			ConfigVersion: "stale-version",
		})
		if err == nil {
			t.Fatal("expected conflict error for stale ConfigVersion")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryConflict {
			t.Fatalf("expected CategoryConflict, got %v", err)
		}
	})

	// 3. Update checks
	t.Run("Update config_version and revision validation", func(t *testing.T) {
		sampleBinding := domain.ProxyChainBinding{
			ID:         "b-test-1",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		}

		// Empty config_version -> 422
		_, err := svc.Update(ctx, "b-test-1", proxychain.UpdateCommand{
			Binding:          sampleBinding,
			ConfigVersion:    "",
			ExpectedRevision: 3,
		})
		if err == nil {
			t.Fatal("expected validation error for empty ConfigVersion")
		}
		domErr, ok := domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Whitespace config_version -> 422
		_, err = svc.Update(ctx, "b-test-1", proxychain.UpdateCommand{
			Binding:          sampleBinding,
			ConfigVersion:    "   ",
			ExpectedRevision: 3,
		})
		if err == nil {
			t.Fatal("expected validation error for whitespace ConfigVersion")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Revision == 0 -> 422
		_, err = svc.Update(ctx, "b-test-1", proxychain.UpdateCommand{
			Binding:          sampleBinding,
			ConfigVersion:    validVersion,
			ExpectedRevision: 0,
		})
		if err == nil {
			t.Fatal("expected validation error for zero ExpectedRevision")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Revision < 0 -> 422
		_, err = svc.Update(ctx, "b-test-1", proxychain.UpdateCommand{
			Binding:          sampleBinding,
			ConfigVersion:    validVersion,
			ExpectedRevision: -5,
		})
		if err == nil {
			t.Fatal("expected validation error for negative ExpectedRevision")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Stale config_version -> 409
		_, err = svc.Update(ctx, "b-test-1", proxychain.UpdateCommand{
			Binding:          sampleBinding,
			ConfigVersion:    "stale-version",
			ExpectedRevision: 3,
		})
		if err == nil {
			t.Fatal("expected conflict error for stale ConfigVersion")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryConflict {
			t.Fatalf("expected CategoryConflict, got %v", err)
		}

		// Stale ExpectedRevision -> 409
		_, err = svc.Update(ctx, "b-test-1", proxychain.UpdateCommand{
			Binding:          sampleBinding,
			ConfigVersion:    validVersion,
			ExpectedRevision: 2,
		})
		if err == nil {
			t.Fatal("expected conflict error for stale ExpectedRevision")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryConflict {
			t.Fatalf("expected CategoryConflict, got %v", err)
		}
		if domErr.CurrentRevision == nil || *domErr.CurrentRevision != 3 {
			t.Fatalf("expected CurrentRevision=3, got: %v", domErr.CurrentRevision)
		}

		// Matching version + matching revision succeeds and increments revision
		upRes, err := svc.Update(ctx, "b-test-1", proxychain.UpdateCommand{
			Binding:          sampleBinding,
			ConfigVersion:    validVersion,
			ExpectedRevision: 3,
		})
		if err != nil {
			t.Fatalf("expected update to succeed, got %v", err)
		}
		if upRes.Binding.Revision != 4 {
			t.Fatalf("expected row revision incremented to 4, got %d", upRes.Binding.Revision)
		}
		validVersion = upRes.Version
	})

	// 4. Delete checks
	t.Run("Delete config_version and revision validation", func(t *testing.T) {
		// Empty config_version -> 422
		_, err := svc.Delete(ctx, "b-test-1", proxychain.DeleteCommand{
			ConfigVersion:    "",
			ExpectedRevision: 4,
		})
		if err == nil {
			t.Fatal("expected validation error for empty ConfigVersion")
		}
		domErr, ok := domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Whitespace config_version -> 422
		_, err = svc.Delete(ctx, "b-test-1", proxychain.DeleteCommand{
			ConfigVersion:    "   ",
			ExpectedRevision: 4,
		})
		if err == nil {
			t.Fatal("expected validation error for whitespace ConfigVersion")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Revision == 0 -> 422
		_, err = svc.Delete(ctx, "b-test-1", proxychain.DeleteCommand{
			ConfigVersion:    validVersion,
			ExpectedRevision: 0,
		})
		if err == nil {
			t.Fatal("expected validation error for zero ExpectedRevision")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Revision < 0 -> 422
		_, err = svc.Delete(ctx, "b-test-1", proxychain.DeleteCommand{
			ConfigVersion:    validVersion,
			ExpectedRevision: -1,
		})
		if err == nil {
			t.Fatal("expected validation error for negative ExpectedRevision")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryValidation {
			t.Fatalf("expected CategoryValidation, got %v", err)
		}

		// Stale config_version -> 409
		_, err = svc.Delete(ctx, "b-test-1", proxychain.DeleteCommand{
			ConfigVersion:    "stale-version",
			ExpectedRevision: 4,
		})
		if err == nil {
			t.Fatal("expected conflict error for stale ConfigVersion")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryConflict {
			t.Fatalf("expected CategoryConflict, got %v", err)
		}

		// Stale ExpectedRevision -> 409
		_, err = svc.Delete(ctx, "b-test-1", proxychain.DeleteCommand{
			ConfigVersion:    validVersion,
			ExpectedRevision: 99,
		})
		if err == nil {
			t.Fatal("expected conflict error for stale ExpectedRevision")
		}
		domErr, ok = domain.AsDomainError(err)
		if !ok || domErr.Category != domain.CategoryConflict {
			t.Fatalf("expected CategoryConflict, got %v", err)
		}
		if domErr.CurrentRevision == nil || *domErr.CurrentRevision != 4 {
			t.Fatalf("expected CurrentRevision=4, got: %v", domErr.CurrentRevision)
		}

		// Matching version + matching revision succeeds
		delRes, err := svc.Delete(ctx, "b-test-1", proxychain.DeleteCommand{
			ConfigVersion:    validVersion,
			ExpectedRevision: 4,
		})
		if err != nil {
			t.Fatalf("expected delete to succeed, got %v", err)
		}
		if len(repo.config.Bindings) != 0 {
			t.Fatalf("expected 0 bindings remaining, got %d", len(repo.config.Bindings))
		}
		if delRes.Version == "" {
			t.Fatal("expected non-empty version after delete")
		}
	})
}
