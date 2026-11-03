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
			{SubscriptionID: "sub-1", LogicalID: id},
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

func TestPublication_WithProxyChainRepository_LoadsBindingsAndSources(t *testing.T) {
	ctx := context.Background()

	polRepo := &mockPolicyRepo{}
	revRepo := &mockRevisionRepo{
		active: &domain.ConfigurationRevision{ID: "rev-1", State: domain.RevisionStateActive},
	}
	nodeRepo := &mockNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-1", DisplayName: "Node 1", Active: true},
		},
	}
	sourceRepo := &dummyNodeSourceRepo{}
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
		&mockPubRepo{},
		&mockAuditRepo{},
		publication.WithPolicyRepository(polRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithProxyChainRepository(chainRepo),
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

	polRepo := &mockPolicyRepo{}
	revRepo := &mockRevisionRepo{
		active: &domain.ConfigurationRevision{ID: "rev-1", State: domain.RevisionStateActive},
	}
	nodeRepo := &mockNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-1", DisplayName: "Node 1", Active: true},
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
		&mockPubRepo{},
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
