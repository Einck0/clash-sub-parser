package publication_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

func boolPtr(value bool) *bool { return &value }

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
		// Global filtering removes every node, while the global empty-group PASS
		// setting preserves the routed group without inventing subscription nodes.
		GlobalEmptyFallbackPass: boolPtr(true),
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

	// Mihomo retains the configured route with a PASS-only empty group; it must
	// not resurrect any raw primary-subscription nodes.
	ctx := context.Background()
	artifactMihomo, snapMihomo, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected globally enabled empty-group PASS export to succeed, got error: %v", err)
	}
	if len(snapMihomo.Nodes) != 0 {
		t.Fatalf("expected all nodes to remain excluded by the global filter, got %d", len(snapMihomo.Nodes))
	}
	for _, d := range snapMihomo.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			t.Fatal("primary-subscription fallback diagnostic must not be emitted")
		}
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

	// No fabricated proxies/group: retain the actual configured route and PASS.
	if len(parsed.Proxies) != 0 {
		t.Fatalf("expected no proxies after global filter exclusion, got %+v", parsed.Proxies)
	}
	if len(parsed.ProxyGroups) != 1 || parsed.ProxyGroups[0].Name != "Normal Group" {
		t.Fatalf("expected the configured Normal Group only, got %+v", parsed.ProxyGroups)
	}
	if len(parsed.ProxyGroups[0].Proxies) != 1 || parsed.ProxyGroups[0].Proxies[0] != "PASS" {
		t.Errorf("expected configured empty group to route to PASS, got %+v", parsed.ProxyGroups[0].Proxies)
	}
	if len(parsed.Rules) != 1 || parsed.Rules[0] != "MATCH,Normal Group" {
		t.Errorf("expected original configured route, got %+v", parsed.Rules)
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

	// Non-Mihomo targets cannot represent this configured policy; they must fail
	// closed rather than bypassing filters with raw primary subscription nodes.
	_, _, err = svc.BuildCurrentArtifact(ctx, facts, domain.TargetSingBox)
	if err == nil {
		t.Fatal("expected non-Mihomo export to fail instead of bypassing the global filter")
	}
	if strings.Contains(err.Error(), "primary_subscription_fallback") {
		t.Fatalf("unexpected legacy fallback error: %v", err)
	}
}

func TestBuildCurrentArtifact_EmptyRoutedGroup_UsesGlobalPASS(t *testing.T) {
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
		Nodes:                  []domain.Node{nPrimary},
		GlobalEmptyFallbackPass: boolPtr(true),
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
		t.Fatalf("expected globally enabled empty-group PASS to compile, got error: %v", err)
	}
	if len(snap.Nodes) != 0 {
		t.Fatalf("empty group should not introduce primary nodes, got %d", len(snap.Nodes))
	}
	var parsed struct {
		ProxyGroups []struct { Name string `yaml:"name"`; Proxies []string `yaml:"proxies"` } `yaml:"proxy-groups"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal(artifact.Content, &parsed); err != nil { t.Fatal(err) }
	if len(parsed.ProxyGroups) != 1 || parsed.ProxyGroups[0].Name != "Empty Routed Group" || len(parsed.ProxyGroups[0].Proxies) != 1 || parsed.ProxyGroups[0].Proxies[0] != "PASS" {
		t.Fatalf("expected configured empty group to contain only PASS, got %+v", parsed.ProxyGroups)
	}
	if len(parsed.Rules) != 1 || parsed.Rules[0] != "MATCH,Empty Routed Group" {
		t.Fatalf("expected configured rule to remain unchanged, got %+v", parsed.Rules)
	}
}

func TestBuildCurrentArtifact_NoRouting_DoesNotInventGroups(t *testing.T) {
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
	artifact, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err == nil {
		t.Fatal("expected missing routing to fail instead of inventing a group")
	}
	if artifact != nil && strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Fatalf("missing routing must not produce a fabricated fallback group")
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

func TestBuildCurrentArtifact_PartialConfiguration_DoesNotBypassEmptyGroup(t *testing.T) {
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

	// A usable group does not authorize bypassing the separately routed empty group.
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err == nil {
		t.Fatal("expected partial empty-group policy to fail without globally enabled PASS")
	}
	if artifact != nil && strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Fatal("partial configuration must not produce a fabricated primary group")
	}

	hasFallbackDiag := false
	hasEmptyGroupDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
		}
		if d.Code == "empty_routed_group" || d.Code == "required_nonempty" {
			hasEmptyGroupDiag = true
		}
	}
	if hasFallbackDiag {
		t.Fatal("primary-subscription fallback diagnostic must not be emitted")
	}
	if !hasEmptyGroupDiag {
		t.Errorf("expected original empty-group diagnostic, got: %+v", snap.Diagnostics)
	}

	// Removing primary metadata must not alter the fail-closed behavior.
	factsNoPrimary := *facts
	factsNoPrimary.PrimarySubscriptionID = ""
	factsNoPrimary.PrimaryNodeIDs = nil
	_, _, errFail := svc.BuildCurrentArtifact(ctx, &factsNoPrimary, domain.TargetMihomo)
	if errFail == nil {
		t.Fatal("expected failure when configured empty group has no PASS, regardless of primary metadata")
	}
}

func TestBuildCurrentArtifact_GlobalFilterDisabledPASS_FailsClosed(t *testing.T) {
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
			Groups: []domain.NodeGroup{{
				ID:        "group-disabled-pass",
				Name:      "Filtered Group",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{},
			}},
			PolicyRules: []domain.PolicyRule{{
				ID:            "rule-disabled-pass",
				RevisionID:    "rev-no-pri",
				TargetGroupID: "group-disabled-pass",
				Expression:    "MATCH",
				Enabled:       true,
			}},
			GlobalFilter: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
				},
			},
			PrimarySubscriptionID:   primaryID,
			PrimaryNodeIDs:          primaryNodeIDs,
			GlobalEmptyFallbackPass: boolPtr(false),
		}
	}

	ctx := context.Background()

	// Primary metadata cannot bypass a globally filtered empty selection.
	for _, facts := range []*domain.ConsistentFacts{
		makeFacts("", nil),
		makeFacts("sub-primary", []string{nodeID}),
	} {
		artifact, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
		if err == nil {
			t.Fatal("expected non-Mihomo export to fail for an empty filtered selection")
		}
		if artifact != nil && strings.Contains(string(artifact.Content), "主订阅兜底") {
			t.Fatal("disabled global PASS must not synthesize fallback content")
		}
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

func TestBuildCurrentArtifact_GlobalEmptyPASS_PreservesConfiguredDNS(t *testing.T) {
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
		GlobalEmptyFallbackPass: boolPtr(true),
		PrimarySubscriptionID:   "sub-primary",
		PrimaryNodeIDs:          []string{primaryNodeID},
		DNSSettings: &domain.DNSSettings{
			Enabled: true,
			RawYAML: "enable: true\nnameserver:\n  - 8.8.8.8\n  - 1.1.1.1\n",
		},
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected empty-group PASS with configured DNS to succeed, got error: %v", err)
	}
	content := string(artifact.Content)
	if !strings.Contains(content, "8.8.8.8") || !strings.Contains(content, "1.1.1.1") {
		t.Errorf("expected configured DNS to remain in YAML, got: %s", content)
	}
	if strings.Contains(content, "主订阅兜底") {
		t.Errorf("empty-group PASS must not fabricate a primary group")
	}
	if len(snap.Nodes) != 0 {
		t.Errorf("expected global filter to exclude every node, got %d", len(snap.Nodes))
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

func TestGenerateLiveArtifact_And_CheckSubscriptionLink_FilteredEmptyFailsClosed(t *testing.T) {
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

	// A valid token does not permit live publication to bypass the configured filter.
	_, err := svc.GenerateLiveArtifact(ctx, linkID, rawToken)
	if err == nil {
		t.Fatal("expected filtered-empty live artifact to fail without configured PASS route")
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
	if checkRes.Allowed {
		t.Errorf("expected CheckSubscriptionLink to reject filtered-empty export")
	}
	if checkRes.NodeCount != 0 {
		t.Errorf("expected NodeCount=0 after filter, got %d", checkRes.NodeCount)
	}
}

func TestPreview_WithConsistentFactsLoader_FilteredEmptyFailsClosed(t *testing.T) {
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
	_, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo})
	if err == nil {
		t.Fatal("expected Preview to fail for a globally filtered empty selection with no configured route")
	}
}

func TestBuildCurrentArtifact_PartialEmptyHongKong_UsesGlobalPASS(t *testing.T) {
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
		Nodes:                   []domain.Node{nPrimary1, nPrimary2},
		GlobalEmptyFallbackPass: boolPtr(true),
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
		GlobalEmptyFallbackPass: boolPtr(true),
		PrimarySubscriptionID:   "sub-primary",
		PrimaryNodeIDs:          []string{primaryNodeID1, primaryNodeID2},
		PrimaryUserInfo:         "upload=10; download=20; total=100; expire=1900000000",
	}

	ctx := context.Background()
	artifact, snap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected globally enabled empty-group PASS to compile, got error: %v", err)
	}

	// Verify configured nodes and routes remain; no primary fallback is synthesized.
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
		t.Fatalf("expected only the two configured nodes, got %d", len(parsed.Proxies))
	}
	if len(parsed.ProxyGroups) != 2 || parsed.ProxyGroups[0].Name == "主订阅兜底" || parsed.ProxyGroups[1].Name == "主订阅兜底" {
		t.Fatalf("expected configured groups and no fabricated fallback group, got %+v", parsed.ProxyGroups)
	}
	if len(parsed.Rules) != 2 || parsed.Rules[1] != "MATCH,香港" {
		t.Fatalf("expected original configured rule to remain, got %+v", parsed.Rules)
	}

	// Verify diagnostics: both primary_subscription_fallback and original safe cause
	hasFallbackDiag := false
	hasOriginalCauseDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
		}
		if d.Code == "empty_routed_group" || d.Code == "required_nonempty" {
			hasOriginalCauseDiag = true
		}
	}
	if hasFallbackDiag {
		t.Errorf("primary-subscription fallback diagnostic must not be emitted")
	}
	if hasOriginalCauseDiag {
		t.Errorf("globally enabled PASS should resolve the empty group without an error diagnostic: %+v", snap.Diagnostics)
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

func TestBuildCurrentArtifact_UnusedEmptyHongKong_DoesNotInventFallback(t *testing.T) {
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
		Nodes:                   []domain.Node{nPrimary},
		GlobalEmptyFallbackPass: boolPtr(true),
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
	if err == nil {
		t.Fatal("expected unused invalid empty group to fail validation")
	}
	if artifact != nil && strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("must not fabricate fallback group")
	}
	hasFallbackDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
		}
	}
	if hasFallbackDiag {
		t.Errorf("primary-subscription fallback diagnostic must not be emitted")
	}
}

func TestBuildCurrentArtifact_BuiltinRoutesWithEmptyGroup_UsesGlobalPASS(t *testing.T) {
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
		Nodes:                   []domain.Node{nPrimary},
		GlobalEmptyFallbackPass: boolPtr(true),
		Groups: []domain.NodeGroup{
			{
				ID:        groupHKID,
				Name:      "香港",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{},
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
		t.Fatalf("expected builtin routes and globally enabled PASS group to compile, got: %v", err)
	}

	if strings.Contains(string(artifact.Content), "主订阅兜底") {
		t.Errorf("configured builtin routes must not synthesize a primary group")
	}
	hasFallbackDiag := false
	for _, d := range snap.Diagnostics {
		if d.Code == "primary_subscription_fallback" {
			hasFallbackDiag = true
		}
	}
	if hasFallbackDiag {
		t.Errorf("primary-subscription fallback diagnostic must not be emitted")
	}
}

func TestBuildCurrentArtifact_EmptyPASS_PreviewConsistentWithLive_StrictMode(t *testing.T) {
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
		Nodes:                   []domain.Node{nPrimary},
		GlobalEmptyFallbackPass: boolPtr(true),
		Groups: []domain.NodeGroup{
			{
				ID:        groupHKID,
				Name:      "香港",
				GroupType: domain.GroupTypeSelect,
				Sources:   []domain.MemberSource{},
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

	// Live and both preview modes should compile the configured PASS route identically.
	liveArt, liveSnap, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected live BuildCurrentArtifact to succeed with configured PASS, got: %v", err)
	}

	// Default/compatible preview matches live
	prevDefault, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetMihomo,
		RevisionID: "rev-preview-strict",
	})
	if err != nil {
		t.Fatalf("expected default preview to succeed, got: %v", err)
	}
	if string(prevDefault.Content) != string(liveArt.Content) {
		t.Fatalf("preview content does not match live content")
	}

	// Strict mode preview also matches live
	prevStrict, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetMihomo,
		RevisionID: "rev-preview-strict",
		CompatMode: "strict",
	})
	if err != nil {
		t.Fatalf("expected strict preview to succeed, got: %v", err)
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
