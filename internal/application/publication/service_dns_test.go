package publication_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

type mockDNSSettingsRepo struct {
	settings *domain.DNSSettings
	err      error
}

func (m *mockDNSSettingsRepo) Get(ctx context.Context) (*domain.DNSSettings, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.settings, nil
}

func (m *mockDNSSettingsRepo) Update(ctx context.Context, settings *domain.DNSSettings, expectedRevision int64) error {
	return nil
}

func setupMinimalPubService(t *testing.T, extraOpts ...publication.Option) *publication.Service {
	t.Helper()
	ctx := context.Background()
	pubRepo := newMockPublicationRepo()
	auditRepo := &mockAuditRepo{}
	nodeRepo := newMockNodeRepo()
	revRepo := newMockRevisionRepo()
	policyRepo := newMockPolicyRepo()

	revID := "rev-active-dns"
	gID := domain.MustNewUUIDv7()
	_ = revRepo.Create(ctx, &domain.ConfigurationRevision{
		ID:    revID,
		State: domain.RevisionStateActive,
	})
	_ = revRepo.SetActive(ctx, revID)

	nID := "n1"
	_ = nodeRepo.UpsertBatch(ctx, []domain.Node{
		{LogicalID: nID, DisplayName: "N1", Protocol: domain.ProtocolTrojan, Active: true},
	})

	_ = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
		ID:        gID,
		Name:      "PROXY",
		GroupType: domain.GroupTypeSelect,
	})
	_ = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID:            "r1",
		RevisionID:    revID,
		TargetGroupID: gID,
		Expression:    "MATCH",
		Position:      0,
	})

	opts := []publication.Option{
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithResolver(resolver.New()),
	}
	opts = append(opts, extraOpts...)

	return publication.NewService(pubRepo, auditRepo, opts...)
}

func TestPublication_AssembleResolveInput_DNS(t *testing.T) {
	ctx := context.Background()

	// 1. Without injected DNSSettingsRepository -> fallbacks to DefaultDNSSettings
	t.Run("fallback to default when repo is nil", func(t *testing.T) {
		svc := setupMinimalPubService(t) // no dns repo
		inp, err := svc.AssembleResolveInput(ctx, "")
		if err != nil {
			t.Fatalf("assemble failed: %v", err)
		}
		if !inp.DNS.Enabled {
			t.Fatalf("expected default DNS to be enabled")
		}
		if inp.DNS.Config == nil || inp.DNS.Config["enable"] != true {
			t.Fatalf("expected DNS config with enable: true, got %+v", inp.DNS.Config)
		}
	})

	// 2. With configured DNSSettingsRepository -> loads saved settings
	t.Run("loads custom saved settings from repo", func(t *testing.T) {
		repo := &mockDNSSettingsRepo{
			settings: &domain.DNSSettings{
				Enabled:  false,
				RawYAML:  "enable: false\nnameserver:\n  - 10.0.0.1\n",
				Revision: 2,
				Source:   "saved",
			},
		}
		svc := setupMinimalPubService(t, publication.WithDNSSettingsRepository(repo))
		inp, err := svc.AssembleResolveInput(ctx, "")
		if err != nil {
			t.Fatalf("assemble failed: %v", err)
		}
		if inp.DNS.Enabled {
			t.Fatalf("expected DNS to be disabled as configured")
		}
		if inp.DNS.Config == nil || inp.DNS.Config["enable"] != false {
			t.Fatalf("expected DNS config with enable: false, got %+v", inp.DNS.Config)
		}
	})

	// 3. Real repo error must fail, never fallback to default
	t.Run("fails on repo error without falling back to default", func(t *testing.T) {
		repo := &mockDNSSettingsRepo{
			err: errors.New("database disk I/O error"),
		}
		svc := setupMinimalPubService(t, publication.WithDNSSettingsRepository(repo))
		_, err := svc.AssembleResolveInput(ctx, "")
		if err == nil {
			t.Fatalf("expected error on repo failure, got nil")
		}
		var de *domain.DomainError
		if !errors.As(err, &de) || de.Code != "dns_settings_unavailable" {
			t.Fatalf("expected dns_settings_unavailable domain error, got %v", err)
		}
	})
}

func TestPublication_BuildCurrentArtifact_DNS(t *testing.T) {
	ctx := context.Background()
	svc := setupMinimalPubService(t)

	grpID := domain.MustNewUUIDv7()
	nodeID := "0123456789abcdef0123456789abcdef"
	facts := &domain.ConsistentFacts{
		AsOf: time.Now().UTC(),
		Revision: &domain.ConfigurationRevision{
			ID:    domain.MustNewUUIDv7(),
			State: domain.RevisionStateActive,
		},
		Nodes: []domain.Node{
			{
				LogicalID:   nodeID,
				DisplayName: "Node 1",
				Protocol:    domain.ProtocolTrojan,
				Server:      "1.1.1.1",
				Port:        443,
				Credentials: domain.InboundProtocolCredential{Password: "trojan-password"},
				Active:      true,
			},
		},
		Groups: []domain.NodeGroup{
			{
				ID:        grpID,
				Name:      "Group 1",
				GroupType: domain.GroupTypeSelect,
			},
		},
		GroupSources: map[string][]domain.MemberSource{
			grpID: {
				{
					ID:            domain.MustNewUUIDv7(),
					GroupID:       grpID,
					Operation:     domain.OperationAdd,
					Kind:          domain.SourceKindManualNode,
					NodeLogicalID: &nodeID,
					Position:      0,
				},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            "rule-1",
				TargetGroupID: grpID,
				Expression:    "MATCH",
				Position:      0,
			},
		},
		DNSSettings: &domain.DNSSettings{
			Enabled:  true,
			RawYAML:  "enable: true\nnameserver:\n  - 1.1.1.1\n  - 8.8.8.8\nenhanced-mode: fake-ip\n",
			Revision: 5,
			Source:   "saved",
		},
	}

	art, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("BuildCurrentArtifact failed: %v", err)
	}
	if art == nil || snap == nil {
		t.Fatalf("expected non-nil artifact and snapshot")
	}

	if !snap.DNS.Enabled {
		t.Fatalf("expected snap.DNS.Enabled=true")
	}
	if snap.DNS.Config == nil || snap.DNS.Config["enhanced-mode"] != "fake-ip" {
		t.Fatalf("expected snap.DNS.Config to contain enhanced-mode: fake-ip, got %+v", snap.DNS.Config)
	}
}
