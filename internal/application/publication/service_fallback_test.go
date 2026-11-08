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

	primaryNodeID1 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	primaryNodeID2 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)
	secondaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.3", 8388, nil)

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

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

	hasFallbackDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for fallback diagnostic, got: %s", d.Severity)
			}
			break
		}
	}
	if !hasFallbackDiag {
		t.Fatalf("expected primary_subscription_fallback diagnostic, got %+v", snap.Diagnostics)
	}
}

func TestBuildCurrentArtifact_DirectReject_NotOverridden(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

	ruleID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-direct",
			ContentDigest: "digest-rev-dir",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-direct",
				BuiltinTarget: "DIRECT",
				Expression:    "MATCH",
				Position:      0,
				Enabled:       true,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected direct rule compile to succeed without fallback, got error: %v", err)
	}

	if strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("content should not contain '主订阅兜底'")
	}
	if !strings.Contains(string(artifact.Content), "DIRECT") {
		t.Errorf("expected DIRECT in output, got: %s", string(artifact.Content))
	}
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			t.Fatal("primary_subscription_fallback diagnostic should not be present on explicit DIRECT rule")
		}
	}
}

func TestBuildCurrentArtifact_NormalHealthyRoutes_NoFallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

func TestBuildCurrentArtifact_EffectiveRoutesWithZeroEnabledPolicyRule_NoFallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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
			ID:            "rev-zero-enabled",
			ContentDigest: "digest-rev-ze",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{n},
		Groups: []domain.NodeGroup{
			{
				ID:        groupID,
				Name:      "Legacy Fixture Group",
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
				RevisionID:    "rev-zero-enabled",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
				Enabled:       false,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{nodeID},
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected normal build to succeed with effective routes even when Enabled is zero, got: %v", err)
	}

	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			t.Fatal("primary_subscription_fallback diagnostic should not be present when effective routes exist")
		}
	}
	if strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("content should NOT contain '主订阅兜底' on normal export")
	}
	if !strings.Contains(string(artifact.Content), "Legacy Fixture Group") {
		t.Errorf("content should contain 'Legacy Fixture Group'")
	}
}

func TestBuildCurrentArtifact_Fallback_PartialConfiguration_Recovers(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

	// 1. When primary is available, partial configuration error must recover through primary fallback
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected partial configuration error to recover via primary fallback, got error: %v", err)
	}

	if !strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("expected fallback content to contain '主订阅兜底', got: %s", string(artifact.Content))
	}
	if !strings.Contains(string(artifact.Content), "Usable Node") {
		t.Errorf("expected fallback content to contain primary node, got: %s", string(artifact.Content))
	}

	hasFallbackDiag := false
	hasEmptyGroupDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for fallback diagnostic, got: %s", d.Severity)
			}
			if strings.Contains(d.Message, "all policy rules or filters resulted in empty nodes") {
				t.Errorf("inaccurate exhaustion wording in fallback diagnostic for partial configuration: %s", d.Message)
			}
			if !strings.Contains(d.Message, "empty policy group validation failed") {
				t.Errorf("expected accurate empty group validation failure message, got: %s", d.Message)
			}
		}
		if d.Code == "empty_routed_group" || d.Code == "required_nonempty" {
			hasEmptyGroupDiag = true
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for preserved original empty group diagnostic, got: %s", d.Severity)
			}
		}
	}
	if !hasFallbackDiag {
		t.Errorf("expected primary_subscription_fallback diagnostic in snapshot, got: %+v", snap.Diagnostics)
	}
	if !hasEmptyGroupDiag {
		t.Errorf("expected safe preserved empty group diagnostic in snapshot, got: %+v", snap.Diagnostics)
	}

	// 2. When primary is unavailable, must fail closed
	factsNoPrimary := *facts
	factsNoPrimary.PrimarySubscriptionID = ""
	factsNoPrimary.PrimaryNodeIDs = nil
	_, snapFail, errFail := svc.BuildCurrentArtifact(ctx, &factsNoPrimary, domain.TargetMihomo)
	if errFail == nil {
		t.Fatal("expected failure when primary is unavailable, got nil")
	}
	errStr := errFail.Error()
	if !strings.Contains(errStr, "empty") && !strings.Contains(errStr, "rejected") && !strings.Contains(errStr, "required_nonempty") && !strings.Contains(errStr, "primary_subscription_fallback_unavailable") {
		t.Errorf("expected error mentioning empty group / fallback unavailable, got: %v", errFail)
	}
	hasUnavailable := false
	for _, d := range snapFail.Diagnostics {
		if d.Code == "primary_subscription_fallback_unavailable" {
			hasUnavailable = true
			break
		}
	}
	if !hasUnavailable {
		t.Errorf("expected primary_subscription_fallback_unavailable diagnostic when primary absent, got %+v", snapFail.Diagnostics)
	}
}

func TestBuildCurrentArtifact_FallbackUnavailable_WhenNoPrimaryOrDisabled(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

func TestBuildCurrentArtifact_Fallback_PreservesConfiguredDNS(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)

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
			ID:            "rev-dns-fb",
			ContentDigest: "digest-rev-dfb",
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
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected fallback with DNS to succeed, got error: %v", err)
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

	hasFallbackDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for fallback diagnostic, got: %s", d.Severity)
			}
			break
		}
	}
	if !hasFallbackDiag {
		t.Fatalf("expected primary_subscription_fallback diagnostic, got %+v", snap.Diagnostics)
	}
}

func TestBuildCurrentArtifact_ChainReferenceMissing_NeverFallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	targetNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)

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
	nTarget := domain.Node{
		LogicalID:   targetNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Target Node",
		Server:      "1.1.1.2",
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
			ID:            "rev-chain-missing",
			ContentDigest: "digest-rev-cm",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary, nTarget},
		// Global filter excludes all nodes, so dialer node is not exportable
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID, targetNodeID},
		ProxyChainBindings: []domain.ProxyChainBinding{
			{
				ID:         "pcb-1",
				TargetType: domain.ProxyChainTargetNode,
				TargetID:   targetNodeID,
				DialerType: domain.ProxyChainDialerNode,
				DialerID:   primaryNodeID,
				Enabled:    true,
				SortOrder:  1,
				Revision:   1,
			},
		},
	}

	ctx := context.Background()
	_, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err == nil {
		t.Fatal("expected chain_reference_missing error, got nil")
	}
	if !strings.Contains(err.Error(), "chain_reference_missing") {
		t.Errorf("expected error to contain 'chain_reference_missing', got: %v", err)
	}
}

func TestBuildCurrentArtifact_NormalRoute_PreservesDNSAndChainBindings(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	dialerNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	targetNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)

	nDialer := domain.Node{
		LogicalID:   dialerNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Dialer Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-dialer",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	nTarget := domain.Node{
		LogicalID:   targetNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Target Node",
		Server:      "1.1.1.2",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-target",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-dns-chain-normal",
			ContentDigest: "digest-rev-dcn",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nDialer, nTarget},
		Groups: []domain.NodeGroup{
			{
				ID:        groupID,
				Name:      "Chained Group",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            "src-target",
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						Position:      0,
						NodeLogicalID: &targetNodeID,
					},
				},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-dns-chain-normal",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
				Enabled:       true,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{dialerNodeID, targetNodeID},
		DNSSettings: &domain.DNSSettings{
			Enabled: true,
			RawYAML: "enable: true\nnameserver:\n  - 8.8.8.8\n  - 1.1.1.1\n",
		},
		ProxyChainBindings: []domain.ProxyChainBinding{
			{
				ID:         "pcb-1",
				TargetType: domain.ProxyChainTargetNode,
				TargetID:   targetNodeID,
				DialerType: domain.ProxyChainDialerNode,
				DialerID:   dialerNodeID,
				Enabled:    true,
				SortOrder:  1,
				Revision:   1,
			},
		},
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected normal route with DNS & chains to succeed, got error: %v", err)
	}

	content := string(artifact.Content)
	if !strings.Contains(content, "8.8.8.8") || !strings.Contains(content, "1.1.1.1") {
		t.Errorf("expected rendered DNS in normal YAML, got: %s", content)
	}
	if !strings.Contains(content, "Chained Group") {
		t.Errorf("expected group 'Chained Group' in normal YAML")
	}
	if strings.Contains(content, "主订阅兜底") {
		t.Errorf("normal export should not contain '主订阅兜底'")
	}
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			t.Fatal("primary_subscription_fallback diagnostic should not be present on normal route")
		}
	}
}

func TestGenerateLiveArtifact_And_CheckSubscriptionLink_Fallback(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

func TestBuildCurrentArtifact_Fallback_PartialEmptyHongKong_Recovers(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID1 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	primaryNodeID2 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)

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

	groupUSID := domain.MustNewUUIDv7()
	groupHKID := domain.MustNewUUIDv7()

	ruleUSID := domain.MustNewUUIDv7()
	ruleHKID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-hk-empty",
			ContentDigest: "digest-rev-hk",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary1, nPrimary2},
		Groups: []domain.NodeGroup{
			{
				ID:        groupUSID,
				Name:      "US Group",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            "src-us",
						GroupID:       groupUSID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						Position:      0,
						NodeLogicalID: &primaryNodeID2,
					},
				},
			},
			{
				ID:        groupHKID,
				Name:      "香港",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{}, // 0 nodes in 香港 group
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleUSID,
				RevisionID:    "rev-hk-empty",
				TargetGroupID: groupUSID,
				Expression:    "DOMAIN-SUFFIX,google.com",
				Position:      0,
				Enabled:       true,
			},
			{
				ID:            ruleHKID,
				RevisionID:    "rev-hk-empty",
				TargetGroupID: groupHKID,
				Expression:    "MATCH",
				Position:      1,
				Enabled:       true,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID1, primaryNodeID2},
		PrimaryUserInfo:       "upload=10; download=20; total=100; expire=1900000000",
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected partial empty 香港 to recover via primary fallback, got error: %v", err)
	}

	// Verify fallback artifact content
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
	if err := yaml.Unmarshal(artifact.Content, &parsed); err != nil {
		t.Fatalf("failed to parse fallback YAML: %v", err)
	}

	if len(parsed.Proxies) != 2 {
		t.Fatalf("expected 2 proxies in fallback artifact, got %d", len(parsed.Proxies))
	}
	if len(parsed.ProxyGroups) != 1 || parsed.ProxyGroups[0].Name != "主订阅兜底" {
		t.Fatalf("expected single proxy group '主订阅兜底', got %+v", parsed.ProxyGroups)
	}
	if len(parsed.Rules) != 1 || parsed.Rules[0] != "MATCH,主订阅兜底" {
		t.Fatalf("expected rule 'MATCH,主订阅兜底', got %+v", parsed.Rules)
	}

	// Verify diagnostics: both primary_subscription_fallback and original safe cause
	hasFallbackDiag := false
	hasOriginalCauseDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for fallback, got: %s", d.Severity)
			}
			if strings.Contains(d.Message, "all policy rules or filters resulted in empty nodes") {
				t.Errorf("inaccurate exhaustion wording in fallback diagnostic for partial empty group: %s", d.Message)
			}
			if !strings.Contains(d.Message, "empty policy group validation failed") {
				t.Errorf("expected accurate empty group validation failure message, got: %s", d.Message)
			}
		}
		if d.Code == "empty_routed_group" || d.Code == "required_nonempty" {
			hasOriginalCauseDiag = true
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for safe original cause, got: %s", d.Severity)
			}
		}
	}
	if !hasFallbackDiag {
		t.Errorf("expected primary_subscription_fallback diagnostic, got: %+v", snap.Diagnostics)
	}
	if !hasOriginalCauseDiag {
		t.Errorf("expected safe original cause diagnostic for empty 香港, got: %+v", snap.Diagnostics)
	}

	// Verify UserInfo is preserved
	if artifact.UserInfo != facts.PrimaryUserInfo {
		t.Errorf("expected UserInfo %q, got %q", facts.PrimaryUserInfo, artifact.UserInfo)
	}

	// Verify input facts groups and rules are completely unmodified
	if len(facts.Groups) != 2 || facts.Groups[1].Name != "香港" {
		t.Errorf("facts groups were mutated: %+v", facts.Groups)
	}
	if len(facts.PolicyRules) != 2 {
		t.Errorf("facts policy rules were mutated: %+v", facts.PolicyRules)
	}
}

func TestBuildCurrentArtifact_Fallback_UnusedEmptyHongKong_Recovers(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
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

	groupUSID := domain.MustNewUUIDv7()
	groupHKID := domain.MustNewUUIDv7()
	ruleUSID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-unused-hk",
			ContentDigest: "digest-rev-unused-hk",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary},
		Groups: []domain.NodeGroup{
			{
				ID:        groupUSID,
				Name:      "US Group",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            "src-us",
						GroupID:       groupUSID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						Position:      0,
						NodeLogicalID: &primaryNodeID,
					},
				},
			},
			{
				ID:        groupHKID,
				Name:      "香港",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{}, // unrouted empty group
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleUSID,
				RevisionID:    "rev-unused-hk",
				TargetGroupID: groupUSID,
				Expression:    "MATCH",
				Position:      0,
				Enabled:       true,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected unused empty group to recover via primary fallback, got error: %v", err)
	}

	if !strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("expected content to contain '主订阅兜底', got: %s", string(artifact.Content))
	}
	hasFallbackDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			if strings.Contains(d.Message, "all policy rules or filters resulted in empty nodes") {
				t.Errorf("inaccurate exhaustion wording in fallback diagnostic for unused empty group: %s", d.Message)
			}
			if !strings.Contains(d.Message, "empty policy group validation failed") {
				t.Errorf("expected accurate empty group validation failure message, got: %s", d.Message)
			}
			break
		}
	}
	if !hasFallbackDiag {
		t.Errorf("expected primary_subscription_fallback diagnostic, got: %+v", snap.Diagnostics)
	}
}

func TestBuildCurrentArtifact_Fallback_BuiltinDirectRejectWithFailingGroup_Recovers(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	nPrimary := domain.Node{
		LogicalID:   primaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Primary Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	groupHKID := domain.MustNewUUIDv7()

	rule1ID := domain.MustNewUUIDv7()
	rule2ID := domain.MustNewUUIDv7()
	rule3ID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-builtin-mix",
			ContentDigest: "digest-rev-bm",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary},
		Groups: []domain.NodeGroup{
			{
				ID:        groupHKID,
				Name:      "香港",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{}, // empty group causes export failure
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            rule1ID,
				RevisionID:    "rev-builtin-mix",
				BuiltinTarget: "DIRECT",
				Expression:    "GEOIP,CN",
				Position:      0,
				Enabled:       true,
			},
			{
				ID:            rule2ID,
				RevisionID:    "rev-builtin-mix",
				BuiltinTarget: "REJECT",
				Expression:    "DOMAIN-SUFFIX,ad.com",
				Position:      1,
				Enabled:       true,
			},
			{
				ID:            rule3ID,
				RevisionID:    "rev-builtin-mix",
				TargetGroupID: groupHKID,
				Expression:    "MATCH",
				Position:      2,
				Enabled:       true,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected fallback to succeed when builtin rules exist alongside failing group, got: %v", err)
	}

	if !strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("expected fallback content to contain '主订阅兜底', got: %s", string(artifact.Content))
	}
	hasFallbackDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
			if strings.Contains(d.Message, "all policy rules or filters resulted in empty nodes") {
				t.Errorf("inaccurate exhaustion wording in fallback diagnostic for builtin + empty group: %s", d.Message)
			}
			if !strings.Contains(d.Message, "empty policy group validation failed") {
				t.Errorf("expected accurate empty group validation failure message, got: %s", d.Message)
			}
			break
		}
	}
	if !hasFallbackDiag {
		t.Errorf("expected primary_subscription_fallback diagnostic, got: %+v", snap.Diagnostics)
	}
}

func TestBuildCurrentArtifact_Fallback_PreviewConsistentWithLive_StrictMode(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	nPrimary := domain.Node{
		LogicalID:   primaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Primary Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	groupHKID := domain.MustNewUUIDv7()
	ruleHKID := domain.MustNewUUIDv7()

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-preview-strict",
			ContentDigest: "digest-rev-ps",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary},
		Groups: []domain.NodeGroup{
			{
				ID:        groupHKID,
				Name:      "香港",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{}, // empty group
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleHKID,
				RevisionID:    "rev-preview-strict",
				TargetGroupID: groupHKID,
				Expression:    "MATCH",
				Position:      0,
				Enabled:       true,
			},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID},
	}

	pubRepo := newMockPublicationRepo()
	factsLoader := &memoryFactsLoader{facts: facts}
	svc := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithConsistentFactsLoader(factsLoader),
	)

	ctx := context.Background()

	// 1. Live artifact succeeds via fallback
	liveArt, liveSnap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected live BuildCurrentArtifact to succeed via fallback, got: %v", err)
	}

	// 2. Default/compatible preview succeeds via fallback and matches live
	prevDefault, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetMihomo,
		RevisionID: "rev-preview-strict",
	})
	if err != nil {
		t.Fatalf("expected default preview to succeed via fallback, got: %v", err)
	}
	if string(prevDefault.Content) != string(liveArt.Content) {
		t.Fatalf("preview content does not match live content")
	}

	// 3. Strict mode preview also succeeds via fallback and matches live
	prevStrict, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetMihomo,
		RevisionID: "rev-preview-strict",
		CompatMode: "strict",
	})
	if err != nil {
		t.Fatalf("expected strict preview to succeed via fallback, got: %v", err)
	}
	if string(prevStrict.Content) != string(liveArt.Content) {
		t.Fatalf("strict preview content does not match live content")
	}
	if prevStrict.SnapshotDigest != liveSnap.SnapshotDigest {
		t.Errorf("expected snapshot digest %q, got %q", liveSnap.SnapshotDigest, prevStrict.SnapshotDigest)
	}
	if !strings.HasPrefix(prevStrict.SnapshotID, "snapshot_") {
		t.Errorf("expected persisted preview draft snapshot ID, got: %s", prevStrict.SnapshotID)
	}
}

func TestBuildCurrentArtifact_Fallback_IndependentBlockers_FailClosed(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	primaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	nPrimary := domain.Node{
		LogicalID:   primaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Primary Node",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	ctx := context.Background()

	// 1. Resolver Cycle -> Fail Closed
	t.Run("ResolverCycle", func(t *testing.T) {
		svc := publication.NewService(nil, nil)
		g1 := domain.MustNewUUIDv7()
		g2 := domain.MustNewUUIDv7()

		factsCycle := &domain.ConsistentFacts{
			AsOf: now,
			Revision: &domain.ConfigurationRevision{
				ID:            "rev-cycle",
				ContentDigest: "digest-rev-cycle",
				State:         domain.RevisionStateActive,
				CreatedAt:     now,
			},
			Nodes: []domain.Node{nPrimary},
			Groups: []domain.NodeGroup{
				{
					ID:        g1,
					Name:      "Group 1",
					GroupType: domain.GroupTypeSelect,
					Sources: []domain.MemberSource{
						{
							ID:            "src-cycle-1",
							GroupID:       g1,
							Operation:     domain.OperationAdd,
							Kind:          domain.SourceKindGroupRef,
							TargetGroupID: &g2,
						},
					},
				},
				{
					ID:        g2,
					Name:      "Group 2",
					GroupType: domain.GroupTypeSelect,
					Sources: []domain.MemberSource{
						{
							ID:            "src-cycle-2",
							GroupID:       g2,
							Operation:     domain.OperationAdd,
							Kind:          domain.SourceKindGroupRef,
							TargetGroupID: &g1,
						},
					},
				},
			},
			PolicyRules: []domain.PolicyRule{
				{
					ID:            domain.MustNewUUIDv7(),
					RevisionID:    "rev-cycle",
					TargetGroupID: g1,
					Expression:    "MATCH",
					Enabled:       true,
				},
			},
			PrimarySubscriptionID: "sub-primary",
			PrimaryNodeIDs:        []string{primaryNodeID},
		}

		_, _, err := svc.BuildCurrentArtifact(ctx, factsCycle, domain.TargetMihomo)
		if err == nil {
			t.Fatal("expected cycle error to fail closed without fallback, got nil")
		}
	})

	// 2. Risk Blocked -> Fail Closed
	t.Run("RiskBlocked", func(t *testing.T) {
		ipriskSvc := iprisk.NewService(nil, nil)
		svc := publication.NewService(nil, nil, publication.WithIPRiskService(ipriskSvc))

		activeRiskPolicy := makeTestRiskPolicy("0191e4a0-0000-7000-8000-000000000002", domain.RiskActionBlock)
		activeRiskPolicy.RiskPolicy.UnknownAction = domain.RiskActionBlock

		groupEmptyID := domain.MustNewUUIDv7()
		factsRisk := &domain.ConsistentFacts{
			AsOf: now,
			Revision: &domain.ConfigurationRevision{
				ID:            "rev-risk-empty",
				ContentDigest: "digest-rev-re",
				State:         domain.RevisionStateActive,
				CreatedAt:     now,
			},
			Nodes: []domain.Node{nPrimary},
			Groups: []domain.NodeGroup{
				{
					ID:        groupEmptyID,
					Name:      "香港",
					GroupType: domain.GroupTypeSelect,
					Sources:   []domain.MemberSource{}, // empty group
				},
			},
			PolicyRules: []domain.PolicyRule{
				{
					ID:            domain.MustNewUUIDv7(),
					RevisionID:    "rev-risk-empty",
					TargetGroupID: groupEmptyID,
					Expression:    "MATCH",
					Enabled:       true,
				},
			},
			PrimarySubscriptionID: "sub-primary",
			PrimaryNodeIDs:        []string{primaryNodeID},
			ActiveRiskPolicy:      activeRiskPolicy,
		}

		_, _, err := svc.BuildCurrentArtifact(ctx, factsRisk, domain.TargetMihomo)
		if err == nil {
			t.Fatal("expected risk blocker to fail closed without fallback, got nil")
		}
		if !strings.Contains(err.Error(), "risk") && !strings.Contains(err.Error(), "rejected") {
			t.Errorf("expected risk rejection error, got: %v", err)
		}
	})

	// 3. Per-node capability error (unsupported plugin on non-Mihomo) -> Fail Closed
	t.Run("PerNodeCapabilityError", func(t *testing.T) {
		svc := publication.NewService(nil, nil)

		pluginTransport := map[string]string{
			"plugin":      "v2ray-plugin",
			"plugin-opts": "mode=websocket",
		}
		nodeWithPluginID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.3", 8388, pluginTransport)
		nWithPlugin := domain.Node{
			LogicalID:   nodeWithPluginID,
			Protocol:    domain.ProtocolSS,
			DisplayName: "SS with v2ray plugin",
			Server:      "1.1.1.3",
			Port:        8388,
			Active:      true,
			Credentials: domain.InboundProtocolCredential{
				Method:    "aes-128-gcm",
				Password:  "pass",
				Transport: pluginTransport,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		factsPlugin := &domain.ConsistentFacts{
			AsOf: now,
			Revision: &domain.ConfigurationRevision{
				ID:            "rev-plugin",
				ContentDigest: "digest-rev-plugin",
				State:         domain.RevisionStateActive,
				CreatedAt:     now,
			},
			Nodes: []domain.Node{nWithPlugin},
			// Filter empties all nodes so selection exhaustion happens
			GlobalFilter: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
				},
			},
			PrimarySubscriptionID: "sub-primary",
			PrimaryNodeIDs:        []string{nodeWithPluginID},
		}

		// Surge does not support plugins; when primary node itself has unrenderable plugin, strict mode fails closed
		_, _, err := svc.BuildCurrentArtifact(ctx, factsPlugin, domain.TargetSurge)
		if err == nil {
			t.Fatal("expected unsupported plugin capability error on Surge, got nil")
		}
	})
}
