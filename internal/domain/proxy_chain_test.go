package domain_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestProxyChainTargetType_IsValid(t *testing.T) {
	tests := []struct {
		target domain.ProxyChainTargetType
		valid  bool
	}{
		{domain.ProxyChainTargetSubscription, true},
		{domain.ProxyChainTargetNodeGroup, true},
		{domain.ProxyChainTargetNode, true},
		{domain.ProxyChainTargetType("unknown"), false},
		{domain.ProxyChainTargetType(""), false},
	}

	for _, tt := range tests {
		if got := tt.target.IsValid(); got != tt.valid {
			t.Errorf("ProxyChainTargetType(%q).IsValid() = %v, want %v", tt.target, got, tt.valid)
		}
	}
}

func TestProxyChainDialerType_IsValid(t *testing.T) {
	tests := []struct {
		dialer domain.ProxyChainDialerType
		valid  bool
	}{
		{domain.ProxyChainDialerNode, true},
		{domain.ProxyChainDialerNodeGroup, true},
		{domain.ProxyChainDialerType("subscription"), false},
		{domain.ProxyChainDialerType(""), false},
	}

	for _, tt := range tests {
		if got := tt.dialer.IsValid(); got != tt.valid {
			t.Errorf("ProxyChainDialerType(%q).IsValid() = %v, want %v", tt.dialer, got, tt.valid)
		}
	}
}

func TestProxyChainBinding_Validate(t *testing.T) {
	validBinding := domain.ProxyChainBinding{
		ID:         "b1",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   "node-1",
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   "node-2",
		Enabled:    true,
		SortOrder:  1,
		Note:       "test chain",
		Revision:   0,
	}

	if err := validBinding.Validate(); err != nil {
		t.Fatalf("expected valid binding, got: %v", err)
	}

	// Invalid target type
	invalidTarget := validBinding
	invalidTarget.TargetType = "bad_type"
	if err := invalidTarget.Validate(); err == nil {
		t.Fatal("expected error for invalid target type")
	}

	// Empty target ID
	emptyTargetID := validBinding
	emptyTargetID.TargetID = "  "
	if err := emptyTargetID.Validate(); err == nil {
		t.Fatal("expected error for empty target ID")
	}

	// Invalid dialer type
	invalidDialer := validBinding
	invalidDialer.DialerType = "bad_dialer"
	if err := invalidDialer.Validate(); err == nil {
		t.Fatal("expected error for invalid dialer type")
	}

	// Empty dialer ID
	emptyDialerID := validBinding
	emptyDialerID.DialerID = ""
	if err := emptyDialerID.Validate(); err == nil {
		t.Fatal("expected error for empty dialer ID")
	}

	// Negative revision
	negRev := validBinding
	negRev.Revision = -1
	if err := negRev.Validate(); err == nil {
		t.Fatal("expected error for negative revision")
	}

	// Direct node self reference
	selfNode := validBinding
	selfNode.TargetType = domain.ProxyChainTargetNode
	selfNode.TargetID = "node-same"
	selfNode.DialerType = domain.ProxyChainDialerNode
	selfNode.DialerID = "node-same"
	err := selfNode.Validate()
	if err == nil {
		t.Fatal("expected error for direct node self-reference")
	}
	if domErr, ok := domain.AsDomainError(err); !ok || domErr.Code != "chain_cycle" {
		t.Fatalf("expected chain_cycle domain error, got: %v", err)
	}

	// Direct group self reference
	selfGroup := validBinding
	selfGroup.TargetType = domain.ProxyChainTargetNodeGroup
	selfGroup.TargetID = "group-same"
	selfGroup.DialerType = domain.ProxyChainDialerNodeGroup
	selfGroup.DialerID = "group-same"
	err = selfGroup.Validate()
	if err == nil {
		t.Fatal("expected error for direct group self-reference")
	}
	if domErr, ok := domain.AsDomainError(err); !ok || domErr.Code != "chain_cycle" {
		t.Fatalf("expected chain_cycle domain error, got: %v", err)
	}
}

func TestProxyChainConfigVersion_DeterminismAndStability(t *testing.T) {
	// Empty slice vs nil slice must produce the exact same non-empty version
	emptyVer := domain.ProxyChainConfigVersion([]domain.ProxyChainBinding{})
	nilVer := domain.ProxyChainConfigVersion(nil)
	if emptyVer == "" {
		t.Fatal("empty version must be non-empty")
	}
	if len(emptyVer) != 64 {
		t.Fatalf("expected 64-char sha256 hex string, got len %d: %q", len(emptyVer), emptyVer)
	}
	if emptyVer != nilVer {
		t.Fatalf("expected emptyVer == nilVer, got %q vs %q", emptyVer, nilVer)
	}

	b1 := domain.ProxyChainBinding{
		ID:         "b-01",
		TargetType: domain.ProxyChainTargetSubscription,
		TargetID:   "sub-1",
		DialerType: domain.ProxyChainDialerNodeGroup,
		DialerID:   "grp-1",
		Enabled:    true,
		SortOrder:  0,
		Note:       "sub chain",
		Revision:   1,
	}

	b2 := domain.ProxyChainBinding{
		ID:         "b-02",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   "node-hk",
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   "node-jp",
		Enabled:    false,
		SortOrder:  10,
		Note:       "direct override",
		Revision:   2,
	}

	ver1 := domain.ProxyChainConfigVersion([]domain.ProxyChainBinding{b1, b2})
	ver2 := domain.ProxyChainConfigVersion([]domain.ProxyChainBinding{b2, b1})

	if ver1 == "" || ver1 == emptyVer {
		t.Fatalf("unexpected version for non-empty bindings: %q", ver1)
	}
	if ver1 != ver2 {
		t.Fatalf("expected permutation to have identical version, got %q vs %q", ver1, ver2)
	}

	// Changing enabled must change version
	b2Mod := b2
	b2Mod.Enabled = true
	verMod := domain.ProxyChainConfigVersion([]domain.ProxyChainBinding{b1, b2Mod})
	if verMod == ver1 {
		t.Fatal("expected changing enabled to produce different version")
	}

	// Changing sort order must change version
	b1Mod := b1
	b1Mod.SortOrder = 5
	verSortMod := domain.ProxyChainConfigVersion([]domain.ProxyChainBinding{b1Mod, b2})
	if verSortMod == ver1 {
		t.Fatal("expected changing sort order to produce different version")
	}
}

type mockProxyChainRepo struct {
	config *domain.ProxyChainConfig
}

func (m *mockProxyChainRepo) Get(ctx context.Context) (*domain.ProxyChainConfig, error) {
	return m.config, nil
}

func (m *mockProxyChainRepo) Replace(ctx context.Context, expectedVersion string, bindings []domain.ProxyChainBinding) (*domain.ProxyChainConfig, error) {
	currentVer := domain.ProxyChainConfigVersion(m.config.Bindings)
	if expectedVersion != currentVer {
		return nil, domain.NewConflictError("config_version_mismatch", "conflict")
	}
	m.config = &domain.ProxyChainConfig{
		Bindings: bindings,
		Version:  domain.ProxyChainConfigVersion(bindings),
	}
	return m.config, nil
}

var _ domain.ProxyChainRepository = (*mockProxyChainRepo)(nil)
