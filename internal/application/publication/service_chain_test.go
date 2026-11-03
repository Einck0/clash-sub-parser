package publication_test

import (
	"context"
	"strings"
	"testing"
	"time"

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

	// 1. Missing subscription: AssembleResolveInput succeeds without prevalidating old persisted subscription,
	// but actual Publish fails closed with 422 chain_reference_missing
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
	inputMissing, err := svcMissing.AssembleResolveInput(ctx, "")
	if err != nil {
		t.Fatalf("expected AssembleResolveInput to succeed without prevalidation, got error: %v", err)
	}
	if len(inputMissing.ChainBindings) != 1 || inputMissing.ChainBindings[0].ID != "b-sub" {
		t.Fatalf("expected chain bindings returned for candidate repair, got: %+v", inputMissing.ChainBindings)
	}
	_, err = svcMissing.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetMihomo,
		ActorKind: domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatal("expected Publish to reject missing target subscription")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing from Publish, got: %v", err)
	}

	// 2. Disabled subscription: AssembleResolveInput succeeds without prevalidation,
	// but actual Publish fails closed with 422 chain_reference_missing
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
	inputDisabled, err := svcDisabled.AssembleResolveInput(ctx, "")
	if err != nil {
		t.Fatalf("expected AssembleResolveInput to succeed without prevalidation, got error: %v", err)
	}
	if len(inputDisabled.ChainBindings) != 1 || inputDisabled.ChainBindings[0].ID != "b-sub" {
		t.Fatalf("expected chain bindings returned for candidate repair, got: %+v", inputDisabled.ChainBindings)
	}
	_, err = svcDisabled.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetMihomo,
		ActorKind: domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatal("expected Publish to reject disabled target subscription")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing from Publish, got: %v", err)
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
	_, err = svcDisabledBinding.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetMihomo,
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("unexpected Publish failure for disabled bindings: %v", err)
	}

	// 4. Missing chainSubRepo when enabled subscription binding exists fails in Publish
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
	inputNoSubRepo, err := svcNoSubRepo.AssembleResolveInput(ctx, "")
	if err != nil {
		t.Fatalf("expected AssembleResolveInput to succeed without subRepo, got: %v", err)
	}
	if len(inputNoSubRepo.ChainBindings) != 1 {
		t.Fatalf("expected 1 chain binding in input, got: %d", len(inputNoSubRepo.ChainBindings))
	}
	_, err = svcNoSubRepo.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetMihomo,
		ActorKind: domain.ActorKindAdmin,
	})
	if err == nil {
		t.Fatal("expected Publish to fail when chainSubRepo is nil but enabled subscription binding exists")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "subscription_repository_unavailable" {
		t.Fatalf("expected subscription_repository_unavailable from Publish, got: %v", err)
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

func TestPublication_BuildCurrentArtifact_DirectGeneration_EmitsDialerAndRejectsNonMihomo(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeAID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	nodeBID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)
	nodeA := *testValidSSNode(nodeAID, "Node A", "1.1.1.1", 8388)
	nodeB := *testValidSSNode(nodeBID, "Node B", "1.1.1.2", 8388)

	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()
	source1ID := domain.MustNewUUIDv7()
	source2ID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-1",
			ContentDigest: "digest-rev-1",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nodeA, nodeB},
		AdmissionRules: []domain.AdmissionRule{
			{
				ID:         "ar-1",
				RevisionID: "rev-1",
				Name:       "Allow All",
				Expression: "true",
				Action:     domain.RuleActionAllow,
				Position:   0,
			},
		},
		Groups: []domain.NodeGroup{
			{
				ID:        groupID,
				Name:      "Proxy Group",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            source1ID,
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						NodeLogicalID: &nodeA.LogicalID,
						Position:      0,
					},
					{
						ID:            source2ID,
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						NodeLogicalID: &nodeB.LogicalID,
						Position:      1,
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-1",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
			},
		},
		ProxyChainBindings: []domain.ProxyChainBinding{
			{
				ID:         "b-b-to-a",
				TargetType: domain.ProxyChainTargetNode,
				TargetID:   nodeB.LogicalID,
				DialerType: domain.ProxyChainDialerNode,
				DialerID:   nodeA.LogicalID,
				Enabled:    true,
				SortOrder:  1,
				Revision:   1,
			},
		},
		EnabledSubscriptionIDs: make(map[string]bool),
	}

	// 1. Mihomo target emits dialer-proxy: Node A
	artMihomo, snapMihomo, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("BuildCurrentArtifact(Mihomo) failed: %v", err)
	}
	if len(snapMihomo.Nodes) < 2 {
		t.Fatalf("expected 2 resolved nodes in snapshot, got %d", len(snapMihomo.Nodes))
	}
	foundDialer := false
	for _, n := range snapMihomo.Nodes {
		if n.LogicalID == nodeB.LogicalID {
			if n.DialerProxy != "Node A" {
				t.Fatalf("expected Node B DialerProxy 'Node A', got %q", n.DialerProxy)
			}
			foundDialer = true
		}
	}
	if !foundDialer {
		t.Fatal("Node B not found in snapshot nodes")
	}
	if !strings.Contains(string(artMihomo.Content), "dialer-proxy: Node A") && !strings.Contains(string(artMihomo.Content), "dialer-proxy: \"Node A\"") {
		t.Fatalf("rendered Mihomo artifact missing dialer-proxy for Node A:\n%s", string(artMihomo.Content))
	}

	// 2. 3 other targets (SingBox, Surge, QuantumultX) must reject with unsupported_target_capability before graph stripping
	unsupportedTargets := []domain.CompilerTarget{
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}
	for _, target := range unsupportedTargets {
		_, _, err := svc.BuildCurrentArtifact(ctx, facts, target)
		if err == nil {
			t.Fatalf("expected target %s to reject active proxy chains, but got nil error", target)
		}
		domErr, ok := domain.AsDomainError(err)
		if !ok || domErr.Code != "unsupported_target_capability" {
			t.Fatalf("expected unsupported_target_capability for %s, got: %v", target, err)
		}
	}
}

func TestPublication_BuildCurrentArtifact_NoOrDisabledChains_PureExportsUnchanged(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeAID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	nodeBID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)
	nodeA := *testValidSSNode(nodeAID, "Node A", "1.1.1.1", 8388)
	nodeB := *testValidSSNode(nodeBID, "Node B", "1.1.1.2", 8388)

	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()
	source1ID := domain.MustNewUUIDv7()
	source2ID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-1",
			ContentDigest: "digest-rev-1",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nodeA, nodeB},
		AdmissionRules: []domain.AdmissionRule{
			{
				ID:         "ar-1",
				RevisionID: "rev-1",
				Name:       "Allow All",
				Expression: "true",
				Action:     domain.RuleActionAllow,
				Position:   0,
			},
		},
		Groups: []domain.NodeGroup{
			{
				ID:        groupID,
				Name:      "Proxy Group",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            source1ID,
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						NodeLogicalID: &nodeA.LogicalID,
						Position:      0,
					},
					{
						ID:            source2ID,
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						NodeLogicalID: &nodeB.LogicalID,
						Position:      1,
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-1",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
			},
		},
		ProxyChainBindings: []domain.ProxyChainBinding{
			{
				ID:         "b-disabled",
				TargetType: domain.ProxyChainTargetNode,
				TargetID:   nodeB.LogicalID,
				DialerType: domain.ProxyChainDialerNode,
				DialerID:   nodeA.LogicalID,
				Enabled:    false, // disabled
				SortOrder:  1,
				Revision:   1,
			},
		},
		EnabledSubscriptionIDs: make(map[string]bool),
	}

	targets := []domain.CompilerTarget{
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}

	for _, target := range targets {
		art, _, err := svc.BuildCurrentArtifact(ctx, facts, target)
		if err != nil {
			t.Fatalf("expected successful export for target %s with disabled chain, got: %v", target, err)
		}
		if target == domain.TargetMihomo {
			if strings.Contains(string(art.Content), "dialer-proxy") {
				t.Fatalf("expected disabled chain not to emit dialer-proxy in Mihomo output:\n%s", string(art.Content))
			}
		}
	}
}

func TestPublication_BuildCurrentArtifact_ValidatesEnabledSubscriptionIDs(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeAID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	nodeBID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)
	nodeA := *testValidSSNode(nodeAID, "Node A", "1.1.1.1", 8388)
	nodeB := *testValidSSNode(nodeBID, "Node B", "1.1.1.2", 8388)

	sub1ID := domain.MustNewUUIDv7()
	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()
	source1ID := domain.MustNewUUIDv7()
	source2ID := domain.MustNewUUIDv7()

	baseFacts := func() *domain.ConsistentFacts {
		return &domain.ConsistentFacts{
			AsOf: now,
			Revision: &domain.ConfigurationRevision{
				ID:            "rev-1",
				ContentDigest: "digest-rev-1",
				State:         domain.RevisionStateActive,
				CreatedAt:     now,
			},
			Nodes: []domain.Node{nodeA, nodeB},
			AdmissionRules: []domain.AdmissionRule{
				{
					ID:         "ar-1",
					RevisionID: "rev-1",
					Name:       "Allow All",
					Expression: "true",
					Action:     domain.RuleActionAllow,
					Position:   0,
				},
			},
			Groups: []domain.NodeGroup{
				{
					ID:        groupID,
					Name:      "Proxy Group",
					GroupType: domain.GroupTypeSelect,
					Sources: []domain.MemberSource{
						{
							ID:            source1ID,
							GroupID:       groupID,
							Operation:     domain.OperationAdd,
							Kind:          domain.SourceKindManualNode,
							NodeLogicalID: &nodeA.LogicalID,
							Position:      0,
						},
						{
							ID:            source2ID,
							GroupID:       groupID,
							Operation:     domain.OperationAdd,
							Kind:          domain.SourceKindManualNode,
							NodeLogicalID: &nodeB.LogicalID,
							Position:      1,
						},
					},
					CreatedAt: now,
					UpdatedAt: now,
				},
			},
			PolicyRules: []domain.PolicyRule{
				{
					ID:            ruleID,
					RevisionID:    "rev-1",
					TargetGroupID: groupID,
					Expression:    "MATCH",
					Position:      0,
				},
			},
			NodeSources: map[string][]domain.NodeSource{
				nodeB.LogicalID: {{SubscriptionID: sub1ID, NodeLogicalID: nodeB.LogicalID}},
			},
			ProxyChainBindings: []domain.ProxyChainBinding{
				{
					ID:         "b-sub",
					TargetType: domain.ProxyChainTargetSubscription,
					TargetID:   sub1ID,
					DialerType: domain.ProxyChainDialerNode,
					DialerID:   nodeA.LogicalID,
					Enabled:    true,
					SortOrder:  1,
					Revision:   1,
				},
			},
			EnabledSubscriptionIDs: map[string]bool{
				sub1ID: true,
			},
		}
	}

	// 1. When sub1ID is in EnabledSubscriptionIDs -> succeeds
	factsValid := baseFacts()
	art, snap, err := svc.BuildCurrentArtifact(ctx, factsValid, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected successful BuildCurrentArtifact with enabled subscription in facts: %v", err)
	}
	if len(snap.ChainEffects) != 1 || snap.ChainEffects[0].TargetCount != 1 {
		t.Fatalf("expected 1 chain effect with 1 target, got: %+v", snap.ChainEffects)
	}
	if !strings.Contains(string(art.Content), "dialer-proxy: Node A") && !strings.Contains(string(art.Content), "dialer-proxy: \"Node A\"") {
		t.Fatalf("expected dialer-proxy in artifact:\n%s", string(art.Content))
	}

	// 2. When sub1ID is missing from EnabledSubscriptionIDs -> fails with chain_reference_missing
	factsMissingSub := baseFacts()
	factsMissingSub.EnabledSubscriptionIDs = map[string]bool{}
	_, _, err = svc.BuildCurrentArtifact(ctx, factsMissingSub, domain.TargetMihomo)
	if err == nil {
		t.Fatal("expected BuildCurrentArtifact to fail when target subscription is not enabled in facts")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing, got: %v", err)
	}

	// 3. When the binding is disabled -> succeeds even if subscription is not enabled
	factsDisabledBinding := baseFacts()
	factsDisabledBinding.ProxyChainBindings[0].Enabled = false
	factsDisabledBinding.EnabledSubscriptionIDs = map[string]bool{}
	_, _, err = svc.BuildCurrentArtifact(ctx, factsDisabledBinding, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected disabled binding to succeed even with missing subscription ID: %v", err)
	}
}
