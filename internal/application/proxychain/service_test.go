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
	if expectedVersion != "" && m.config.Version != expectedVersion {
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

func TestService_Create_ValidationAndAudit(t *testing.T) {
	ctx := context.Background()
	repo := newMockProxyChainRepo(nil)
	audit := &mockAuditRepo{}

	svc := proxychain.NewService(repo, nil, nil, nil, proxychain.WithAuditRepository(audit))
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{}, nil
	})

	// 1. Caller provides nonempty ID -> rejected (422)
	_, err := svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			ID:         "client-specified-id",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		},
	})
	if err == nil {
		t.Fatal("expected validation error for client-specified ID")
	}

	// 2. Caller provides invalid binding -> rejected (422)
	_, err = svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: "unknown-type",
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
		},
	})
	if err == nil {
		t.Fatal("expected validation error for invalid target type")
	}

	// 3. Stale config version -> conflict (409)
	_, err = svc.Create(ctx, proxychain.CreateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
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
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
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
	repo := newMockProxyChainRepo([]domain.ProxyChainBinding{
		{
			ID:         "b-1",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
			Enabled:    true,
			SortOrder:  10,
			Revision:   1,
		},
	})
	audit := &mockAuditRepo{}

	svc := proxychain.NewService(repo, nil, nil, nil, proxychain.WithAuditRepository(audit))
	svc.SetInputProvider(func(ctx context.Context) (*resolver.ResolveInput, error) {
		return &resolver.ResolveInput{}, nil
	})

	// 1. Path ID mismatch -> 422
	_, err := svc.Update(ctx, "b-1", proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			ID:         "b-other",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
		},
	})
	if err == nil {
		t.Fatal("expected validation error on ID mismatch")
	}

	// 2. Not found -> 404
	_, err = svc.Update(ctx, "b-missing", proxychain.UpdateCommand{
		Binding: domain.ProxyChainBinding{
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
		},
		ConfigVersion: repo.config.Version,
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
			TargetID:   "n1",
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   "n2",
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
			TargetID:   "g1",
			DialerType: domain.ProxyChainDialerNodeGroup,
			DialerID:   "g2",
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

// Ensure unused error imports don't trigger compile errors
var _ = errors.New
