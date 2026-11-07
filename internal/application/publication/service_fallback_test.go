package publication_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func TestBuildCurrentArtifact_Fallback_GlobalFilterEmptiesAll(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID1 := "node_primary_00000001"
	primaryNodeID2 := "node_primary_00000002"
	secondaryNodeID := "node_secondary_00001"

	nPrimary1 := domain.Node{
		LogicalID:   primaryNodeID1,
		Protocol:    domain.ProtocolSS,
		DisplayName: "HK Primary 1",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary-1",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	nPrimary2 := domain.Node{
		LogicalID:   primaryNodeID2,
		Protocol:    domain.ProtocolSS,
		DisplayName: "US Primary 2",
		Server:      "1.1.1.2",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary-2",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	nSecondary := domain.Node{
		LogicalID:   secondaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "SG Secondary",
		Server:      "1.1.1.3",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-secondary",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()

	expectedUserInfo := "upload=100; download=200; total=1000; expire=1800000000"

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-fallback-1",
			ContentDigest: "digest-rev-fb",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary1, nPrimary2, nSecondary},
		Groups: []domain.NodeGroup{
			{
				ID:        groupID,
				Name:      "Normal Group",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            "src-1",
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						Position:      0,
						NodeLogicalID: &primaryNodeID1,
					},
				},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-fallback-1",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
				Enabled:       true,
			},
		},
		// Global filter excludes all nodes
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field: domain.FilterFieldDisplayName,
					Op:    domain.FilterOpContains,
					Value: "NONEXISTENT_MATCH",
				},
			},
		},
		NodeSources: map[string][]domain.NodeSource{
			primaryNodeID1:  {{NodeLogicalID: primaryNodeID1, SubscriptionID: "sub-primary"}},
			primaryNodeID2:  {{NodeLogicalID: primaryNodeID2, SubscriptionID: "sub-primary"}},
			secondaryNodeID: {{NodeLogicalID: secondaryNodeID, SubscriptionID: "sub-secondary"}},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID1, primaryNodeID2},
		PrimaryUserInfo:       expectedUserInfo,
	}

	// 1. TargetMihomo fallback
	ctx := context.Background()
	artifactMihomo, snapMihomo, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected Mihomo fallback to succeed, got error: %v", err)
	}

	// Check diagnostic
	hasFallbackDiag := false
	for _, d := range snapMihomo.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for fallback diagnostic, got: %s", d.Severity)
			}
			break
		}
	}
	if !hasFallbackDiag {
		t.Fatalf("expected primary_subscription_fallback diagnostic in snapshot, got: %+v", snapMihomo.Diagnostics)
	}

	// Check Mihomo content structure
	var parsed struct {
		Proxies []struct {
			Name string `yaml:"name"`
		} `yaml:"proxies"`
		ProxyGroups []struct {
			Name    string   `yaml:"name"`
			Type    string   `yaml:"type"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal(artifactMihomo.Content, &parsed); err != nil {
		t.Fatalf("failed to unmarshal fallback mihomo YAML: %v", err)
	}

	// Must contain primary nodes only (nPrimary1 and nPrimary2), not nSecondary
	if len(parsed.Proxies) != 2 {
		t.Fatalf("expected 2 proxies in fallback output, got %d", len(parsed.Proxies))
	}
	if parsed.Proxies[0].Name != nPrimary1.DisplayName || parsed.Proxies[1].Name != nPrimary2.DisplayName {
		t.Errorf("proxies mismatch, got %+v", parsed.Proxies)
	}

	// Must have minimal select group named "主订阅兜底"
	if len(parsed.ProxyGroups) != 1 {
		t.Fatalf("expected 1 fallback proxy group, got %d", len(parsed.ProxyGroups))
	}
	fbGroup := parsed.ProxyGroups[0]
	if fbGroup.Name != "主订阅兜底" || fbGroup.Type != "select" {
		t.Errorf("expected select group '主订阅兜底', got name=%q type=%q", fbGroup.Name, fbGroup.Type)
	}
	if len(fbGroup.Proxies) != 2 || fbGroup.Proxies[0] != nPrimary1.DisplayName || fbGroup.Proxies[1] != nPrimary2.DisplayName {
		t.Errorf("fallback group proxies mismatch: %+v", fbGroup.Proxies)
	}

	// Rule must match to 主订阅兜底
	if len(parsed.Rules) != 1 || parsed.Rules[0] != "MATCH,主订阅兜底" {
		t.Errorf("expected rule ['MATCH,主订阅兜底'], got %+v", parsed.Rules)
	}

	// UserInfo forwarded
	if artifactMihomo.UserInfo != expectedUserInfo {
		t.Errorf("expected UserInfo %q, got %q", expectedUserInfo, artifactMihomo.UserInfo)
	}

	// Deterministic digest check
	artifactMihomo2, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if artifactMihomo.ContentDigest != artifactMihomo2.ContentDigest {
		t.Errorf("expected deterministic content digest, got %q vs %q", artifactMihomo.ContentDigest, artifactMihomo2.ContentDigest)
	}
	if artifactMihomo.SnapshotDigest != artifactMihomo2.SnapshotDigest {
		t.Errorf("expected deterministic snapshot digest, got %q vs %q", artifactMihomo.SnapshotDigest, artifactMihomo2.SnapshotDigest)
	}

	// 2. TargetSingBox fallback
	artifactSingBox, snapSingBox, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetSingBox)
	if err != nil {
		t.Fatalf("expected SingBox fallback to succeed, got error: %v", err)
	}
	if len(snapSingBox.Nodes) != 2 {
		t.Errorf("expected 2 primary nodes for SingBox fallback, got %d", len(snapSingBox.Nodes))
	}
	if !strings.Contains(string(artifactSingBox.Content), nPrimary1.DisplayName) ||
		!strings.Contains(string(artifactSingBox.Content), nPrimary2.DisplayName) ||
		strings.Contains(string(artifactSingBox.Content), nSecondary.DisplayName) {
		t.Errorf("SingBox content should contain primary nodes only, got: %s", string(artifactSingBox.Content))
	}
}

func TestBuildCurrentArtifact_Fallback_AllRoutedGroupsEmpty(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID := "node_primary_00000001"
	nPrimary := domain.Node{
		LogicalID:   primaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "HK Primary 1",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary-1",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-empty-group",
			ContentDigest: "digest-rev-eg",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary},
		// Group has no member sources -> 0 candidates -> empty routed group
		Groups: []domain.NodeGroup{
			{
				ID:        groupID,
				Name:      "Empty Routed Group",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-empty-group",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
				Enabled:       true,
			},
		},
		NodeSources: map[string][]domain.NodeSource{
			primaryNodeID: {{NodeLogicalID: primaryNodeID, SubscriptionID: "sub-primary"}},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
		PrimaryUserInfo:       "info",
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected fallback on all routed groups empty, got error: %v", err)
	}

	hasFallbackDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			break
		}
	}
	if !hasFallbackDiag {
		t.Fatalf("expected primary_subscription_fallback diagnostic, got %+v", snap.Diagnostics)
	}
	if !strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("expected content to contain '主订阅兜底', got: %s", string(artifact.Content))
	}
}

func TestBuildCurrentArtifact_Fallback_NoEffectiveRulesOrGroups(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID := "node_primary_00000001"
	nPrimary := domain.Node{
		LogicalID:   primaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "HK Primary 1",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary-1",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-no-rules",
			ContentDigest: "digest-rev-nr",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes:                 []domain.Node{nPrimary},
		Groups:                nil,
		PolicyRules:           nil,
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
		PrimaryUserInfo:       "info",
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected fallback when no rules/groups in active revision, got error: %v", err)
	}
	if !strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("expected content to contain '主订阅兜底', got: %s", string(artifact.Content))
	}
	if len(snap.Nodes) != 1 {
		t.Errorf("expected 1 node in snapshot, got %d", len(snap.Nodes))
	}
}

func TestBuildCurrentArtifact_NormalHealthyRoutes_NoFallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeID := "node_0000000000000001"
	n := domain.Node{
		LogicalID:   nodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Healthy Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-normal",
			ContentDigest: "digest-rev-norm",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{n},
		Groups: []domain.NodeGroup{
			{
				ID:        groupID,
				Name:      "Configured Group",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            "src-1",
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						Position:      0,
						NodeLogicalID: &nodeID,
					},
				},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-normal",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
				Enabled:       true,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{nodeID},
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected normal build to succeed, got: %v", err)
	}

	// Fallback diagnostic MUST NOT be present
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			t.Fatal("primary_subscription_fallback diagnostic should not be present on healthy export")
		}
	}
	if strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("content should NOT contain '主订阅兜底' on normal export")
	}
	if !strings.Contains(string(artifact.Content), "Configured Group") {
		t.Errorf("content should contain 'Configured Group'")
	}
}

func TestBuildCurrentArtifact_PartialConfigurationError_NoFallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeID := "node_0000000000000001"
	n := domain.Node{
		LogicalID:   nodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Usable Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	groupUsableID := domain.MustNewUUIDv7()
	groupEmptyID := domain.MustNewUUIDv7()

	rule1ID := domain.MustNewUUIDv7()
	rule2ID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-partial",
			ContentDigest: "digest-rev-part",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{n},
		Groups: []domain.NodeGroup{
			{
				ID:        groupUsableID,
				Name:      "Usable Group",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            "src-1",
						GroupID:       groupUsableID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						Position:      0,
						NodeLogicalID: &nodeID,
					},
				},
			},
			{
				ID:        groupEmptyID,
				Name:      "Empty Group",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{}, // 0 nodes
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            rule1ID,
				RevisionID:    "rev-partial",
				TargetGroupID: groupUsableID,
				Expression:    "DOMAIN-SUFFIX,google.com",
				Position:      0,
				Enabled:       true,
			},
			{
				ID:            rule2ID,
				RevisionID:    "rev-partial",
				TargetGroupID: groupEmptyID,
				Expression:    "MATCH",
				Position:      1,
				Enabled:       true,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{nodeID},
	}

	ctx := context.Background()
	_, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err == nil {
		t.Fatal("expected partial configuration error to fail, but it succeeded")
	}

	// Must fail with preflight error or capability error regarding empty group, NOT fallback
	errStr := err.Error()
	if !strings.Contains(errStr, "empty") && !strings.Contains(errStr, "rejected") && !strings.Contains(errStr, "required_nonempty") {
		t.Errorf("expected error mentioning empty group / rejected preflight, got: %v", err)
	}
}

func TestBuildCurrentArtifact_FallbackUnavailable_WhenNoPrimaryOrDisabled(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeID := "node_0000000000000001"
	n := domain.Node{
		LogicalID:   nodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Node 1",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	makeFacts := func(primaryID string, primaryNodeIDs []string) *domain.ConsistentFacts {
		return &domain.ConsistentFacts{
			AsOf: now,
			Revision: &domain.ConfigurationRevision{
				ID:            "rev-no-pri",
				ContentDigest: "digest-rev-np",
				State:         domain.RevisionStateActive,
				CreatedAt:     now,
			},
			Nodes: []domain.Node{n},
			GlobalFilter: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
				},
			},
			PrimarySubscriptionID: primaryID,
			PrimaryNodeIDs:        primaryNodeIDs,
		}
	}

	ctx := context.Background()

	// Scenario 1: Primary ID empty
	facts1 := makeFacts("", nil)
	_, snap1, err1 := svc.BuildCurrentArtifact(ctx, facts1, domain.TargetSingBox)
	if err1 == nil {
		t.Fatal("expected error when primary absent, got nil")
	}
	hasUnavailable1 := false
	for _, d := range snap1.Diagnostics {
		if d.Code == "primary_subscription_fallback_unavailable" {
			hasUnavailable1 = true
			break
		}
	}
	if !hasUnavailable1 {
		t.Errorf("expected primary_subscription_fallback_unavailable diagnostic when primary absent, got %+v", snap1.Diagnostics)
	}

	// Scenario 2: Primary Node IDs empty
	facts2 := makeFacts("sub-primary", []string{})
	_, snap2, err2 := svc.BuildCurrentArtifact(ctx, facts2, domain.TargetSingBox)
	if err2 == nil {
		t.Fatal("expected error when primary nodes empty, got nil")
	}
	hasUnavailable2 := false
	for _, d := range snap2.Diagnostics {
		if d.Code == "primary_subscription_fallback_unavailable" {
			hasUnavailable2 = true
			break
		}
	}
	if !hasUnavailable2 {
		t.Errorf("expected primary_subscription_fallback_unavailable diagnostic when primary nodes empty, got %+v", snap2.Diagnostics)
	}
}

func TestBuildCurrentArtifact_NoFallbackBypass_OnIndependentRiskBlocker(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	nodeID := "node_risky_000000001"
	n := domain.Node{
		LogicalID:   nodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Risky Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Create IP risk service
	ipriskSvc := iprisk.NewService(nil, nil)
	svc := publication.NewService(nil, nil, publication.WithIPRiskService(ipriskSvc))

	activeRiskPolicy := makeTestRiskPolicy("0191e4a0-0000-7000-8000-000000000001", domain.RiskActionBlock)
	activeRiskPolicy.RiskPolicy.UnknownAction = domain.RiskActionBlock

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-risky",
			ContentDigest: "digest-rev-r",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{n},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{nodeID},
		ActiveRiskPolicy:      activeRiskPolicy,
	}

	ctx := context.Background()
	_, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetSingBox)
	if err == nil {
		t.Fatal("expected risk blocked preflight failure, got nil")
	}
	if !strings.Contains(err.Error(), "rejected") && !strings.Contains(err.Error(), "risk") {
		t.Errorf("expected preflight rejection mentioning risk, got: %v", err)
	}
}

func TestBuildCurrentArtifact_PreservesConfiguredDNS_And_ChainBindings(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID := "node_primary_00000001"
	nPrimary := domain.Node{
		LogicalID:   primaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Primary Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-dns-chain",
			ContentDigest: "digest-rev-dc",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
		DNSSettings: &domain.DNSSettings{
			Enabled: true,
			RawYAML: "enable: true\nnameserver:\n  - 8.8.8.8\n  - 1.1.1.1\n",
		},
		ProxyChainBindings: []domain.ProxyChainBinding{
			{
				ID:         "pcb-1",
				TargetType: domain.ProxyChainTargetSubscription,
				TargetID:   "sub-target",
				DialerType: domain.ProxyChainDialerDirect,
				Enabled:    true,
			},
		},
		EnabledSubscriptionIDs: map[string]bool{"sub-target": true},
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected fallback with DNS & chains to succeed, got error: %v", err)
	}

	// DNS must be rendered in YAML
	content := string(artifact.Content)
	if !strings.Contains(content, "8.8.8.8") || !strings.Contains(content, "1.1.1.1") {
		t.Errorf("expected rendered DNS in fallback YAML, got: %s", content)
	}
	if !strings.Contains(content, "主订阅兜底") {
		t.Errorf("expected group '主订阅兜底' in fallback YAML")
	}
	if len(snap.Nodes) != 1 {
		t.Errorf("expected 1 node in snapshot, got %d", len(snap.Nodes))
	}
}

func TestGenerateLiveArtifact_And_CheckSubscriptionLink_Fallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	primaryNodeID := "node_primary_00000001"
	nPrimary := domain.Node{
		LogicalID:   primaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Primary Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-live-fb",
			ContentDigest: "digest-rev-lfb",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
		PrimaryUserInfo:       "upload=50; download=100; total=500",
	}

	linkRepo := newMemorySubscriptionLinkRepo()
	linkID := "link-fb-1"
	rawToken := "sub_token_1234567890abcdef"
	tokenHash := publication.HashToken(rawToken)

	link := &domain.SubscriptionLink{
		ID:        linkID,
		Label:     "Test Live Link",
		Target:    domain.TargetMihomo,
		Token:     rawToken,
		TokenHash: tokenHash,
		State:     domain.SubscriptionLinkStateActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = linkRepo.Create(context.Background(), link)

	factsLoader := &memoryFactsLoader{facts: facts}

	svc := publication.NewService(nil, nil,
		publication.WithSubscriptionLinkRepository(linkRepo),
		publication.WithConsistentFactsLoader(factsLoader),
	)

	ctx := context.Background()

	// 1. GenerateLiveArtifact with correct token -> returns fallback artifact
	artifact, err := svc.GenerateLiveArtifact(ctx, linkID, rawToken)
	if err != nil {
		t.Fatalf("GenerateLiveArtifact failed: %v", err)
	}
	if !strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("expected fallback content in live artifact, got: %s", string(artifact.Content))
	}
	if artifact.UserInfo != facts.PrimaryUserInfo {
		t.Errorf("expected UserInfo %q, got %q", facts.PrimaryUserInfo, artifact.UserInfo)
	}

	// 2. GenerateLiveArtifact with invalid token -> fails 401 BEFORE facts or fallback
	_, errInvalid := svc.GenerateLiveArtifact(ctx, linkID, "wrong_token")
	if errInvalid != publication.ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got: %v", errInvalid)
	}

	// 3. CheckSubscriptionLink -> allowed=true with diagnostic
	checkRes, err := svc.CheckSubscriptionLink(ctx, linkID)
	if err != nil {
		t.Fatalf("CheckSubscriptionLink failed: %v", err)
	}
	if !checkRes.Allowed {
		t.Errorf("expected CheckSubscriptionLink to be Allowed=true on fallback")
	}
	if checkRes.NodeCount != 1 {
		t.Errorf("expected NodeCount=1, got %d", checkRes.NodeCount)
	}
	hasDiag := false
	for _, d := range checkRes.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasDiag = true
			break
		}
	}
	if !hasDiag {
		t.Errorf("expected primary_subscription_fallback in CheckSubscriptionLink diagnostics, got: %+v", checkRes.Diagnostics)
	}
}

func TestPreview_WithConsistentFactsLoader_Fallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	primaryNodeID := "node_primary_00000001"
	nPrimary := domain.Node{
		LogicalID:   primaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Primary Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-prev-fb",
			ContentDigest: "digest-rev-pfb",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
	}

	factsLoader := &memoryFactsLoader{facts: facts}
	svc := publication.NewService(nil, nil,
		publication.WithConsistentFactsLoader(factsLoader),
	)

	ctx := context.Background()
	prevRes, err := svc.Preview(ctx, publication.PreviewQuery{
		Target: domain.TargetMihomo,
	})
	if err != nil {
		t.Fatalf("expected Preview to succeed via fallback, got error: %v", err)
	}

	hasFallbackDiag := false
	for _, d := range prevRes.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			break
		}
	}
	if !hasFallbackDiag {
		t.Errorf("expected primary_subscription_fallback in Preview diagnostics, got: %+v", prevRes.Diagnostics)
	}
	if !strings.Contains(string(prevRes.Content), "主订阅兜底") {
		t.Errorf("expected Preview content to contain '主订阅兜底', got: %s", string(prevRes.Content))
	}
}
