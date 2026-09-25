package resolver_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func testNodeLogicalID(name string) string {
	h := sha256.Sum256([]byte(name))
	return hex.EncodeToString(h[:])
}

func TestDeterministicResolver_IdenticalInputProducesExactSameDigest(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()
	node1ID := testNodeLogicalID("node-hk-01")
	node2ID := testNodeLogicalID("node-us-02")

	input := resolver.ResolveInput{
		RevisionID:         "rev-1",
		InventoryWatermark: "watermark-100",
		CompilerVersion:    "1.0.0",
		Nodes: []domain.Node{
			{LogicalID: node1ID, DisplayName: "HK 01", Protocol: domain.ProtocolVMess, Active: true},
			{LogicalID: node2ID, DisplayName: "US 02", Protocol: domain.ProtocolTrojan, Active: true},
		},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "PROXY", GroupType: domain.GroupTypeSelect},
			{ID: childID, Name: "AUTO", GroupType: domain.GroupTypeURLTest},
		},
		Edges: map[string][]domain.GroupEdge{
			parentID: {
				{ID: "e1", ParentGroupID: parentID, ChildGroupID: &childID, Position: 0},
				{ID: "e2", ParentGroupID: parentID, NodeLogicalID: &node1ID, Position: 1},
			},
			childID: {
				{ID: "e3", ParentGroupID: childID, NodeLogicalID: &node1ID, Position: 0},
				{ID: "e4", ParentGroupID: childID, NodeLogicalID: &node2ID, Position: 1},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentID, Expression: "DOMAIN-SUFFIX,google.com", Position: 0},
			{ID: "r2", TargetGroupID: parentID, Expression: "MATCH", Position: 1},
		},
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		},
	}

	snap1, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}

	snap2, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	if snap1.SnapshotDigest == "" {
		t.Fatal("expected non-empty SnapshotDigest")
	}

	if snap1.SnapshotDigest != snap2.SnapshotDigest {
		t.Fatalf("digest mismatch between runs:\nrun1: %s\nrun2: %s", snap1.SnapshotDigest, snap2.SnapshotDigest)
	}

	if snap1.InputDigest != snap2.InputDigest {
		t.Fatalf("input digest mismatch:\nrun1: %s\nrun2: %s", snap1.InputDigest, snap2.InputDigest)
	}
}

func TestDeterministicResolver_ShuffledInputProducesIdenticalSnapshotAndDigest(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()
	node1ID := testNodeLogicalID("node-hk-01")
	node2ID := testNodeLogicalID("node-us-02")
	node3ID := testNodeLogicalID("node-jp-03")

	nodes := []domain.Node{
		{LogicalID: node1ID, DisplayName: "HK 01", Protocol: domain.ProtocolVMess, Active: true},
		{LogicalID: node2ID, DisplayName: "US 02", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: node3ID, DisplayName: "JP 03", Protocol: domain.ProtocolSS, Active: true},
	}

	groups := []domain.NodeGroup{
		{ID: parentID, Name: "PROXY", GroupType: domain.GroupTypeSelect},
		{ID: childID, Name: "AUTO", GroupType: domain.GroupTypeURLTest},
	}

	edges := map[string][]domain.GroupEdge{
		parentID: {
			{ID: "e1", ParentGroupID: parentID, ChildGroupID: &childID, Position: 0},
			{ID: "e2", ParentGroupID: parentID, NodeLogicalID: &node1ID, Position: 1},
		},
		childID: {
			{ID: "e3", ParentGroupID: childID, NodeLogicalID: &node2ID, Position: 0},
			{ID: "e4", ParentGroupID: childID, NodeLogicalID: &node3ID, Position: 1},
		},
	}

	rules := []domain.PolicyRule{
		{ID: "r1", TargetGroupID: parentID, Expression: "DOMAIN-SUFFIX,google.com", Position: 0},
		{ID: "r2", TargetGroupID: childID, Expression: "GEOIP,CN", Position: 1},
		{ID: "r3", TargetGroupID: parentID, Expression: "MATCH", Position: 2},
	}

	inputOriginal := resolver.ResolveInput{
		RevisionID:         "rev-perm",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              nodes,
		Groups:             groups,
		Edges:              edges,
		PolicyRules:        rules,
		DNS:                resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1", "8.8.8.8"}},
	}

	snapBase, err := r.Resolve(ctx, inputOriginal)
	if err != nil {
		t.Fatalf("base resolve failed: %v", err)
	}

	// Permute order of nodes, groups, rules
	for i := 0; i < 10; i++ {
		shuffledNodes := make([]domain.Node, len(nodes))
		copy(shuffledNodes, nodes)
		rand.Shuffle(len(shuffledNodes), func(a, b int) {
			shuffledNodes[a], shuffledNodes[b] = shuffledNodes[b], shuffledNodes[a]
		})

		shuffledGroups := make([]domain.NodeGroup, len(groups))
		copy(shuffledGroups, groups)
		rand.Shuffle(len(shuffledGroups), func(a, b int) {
			shuffledGroups[a], shuffledGroups[b] = shuffledGroups[b], shuffledGroups[a]
		})

		shuffledRules := make([]domain.PolicyRule, len(rules))
		copy(shuffledRules, rules)
		rand.Shuffle(len(shuffledRules), func(a, b int) {
			shuffledRules[a], shuffledRules[b] = shuffledRules[b], shuffledRules[a]
		})

		inputShuffled := resolver.ResolveInput{
			RevisionID:         "rev-perm",
			InventoryWatermark: "wm-1",
			CompilerVersion:    "1.0.0",
			Nodes:              shuffledNodes,
			Groups:             shuffledGroups,
			Edges:              edges,
			PolicyRules:        shuffledRules,
			DNS:                resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1", "8.8.8.8"}},
		}

		snapShuffled, err := r.Resolve(ctx, inputShuffled)
		if err != nil {
			t.Fatalf("iteration %d: resolve failed: %v", i, err)
		}

		if snapShuffled.SnapshotDigest != snapBase.SnapshotDigest {
			t.Fatalf("iteration %d: digest diverged after shuffling:\nbase:     %s\nshuffled: %s",
				i, snapBase.SnapshotDigest, snapShuffled.SnapshotDigest)
		}

		if len(snapShuffled.Nodes) != len(snapBase.Nodes) {
			t.Fatalf("iteration %d: node count mismatch", i)
		}
		for j := range snapBase.Nodes {
			if snapShuffled.Nodes[j].LogicalID != snapBase.Nodes[j].LogicalID {
				t.Fatalf("iteration %d: node ordering mismatch at index %d", i, j)
			}
		}

		if len(snapShuffled.Groups) != len(snapBase.Groups) {
			t.Fatalf("iteration %d: group count mismatch", i)
		}
		for j := range snapBase.Groups {
			if snapShuffled.Groups[j].ID != snapBase.Groups[j].ID {
				t.Fatalf("iteration %d: group ordering mismatch at index %d", i, j)
			}
		}

		if len(snapShuffled.Rules) != len(snapBase.Rules) {
			t.Fatalf("iteration %d: rule count mismatch", i)
		}
		for j := range snapBase.Rules {
			if snapShuffled.Rules[j].Expression != snapBase.Rules[j].Expression {
				t.Fatalf("iteration %d: rule ordering mismatch at index %d", i, j)
			}
		}
	}
}

func TestDeterministicResolver_RejectsGraphCycle(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gA := domain.MustNewUUIDv7()
	gB := domain.MustNewUUIDv7()
	gC := domain.MustNewUUIDv7()

	// 3-node cycle: A -> B -> C -> A
	input := resolver.ResolveInput{
		Groups: []domain.NodeGroup{
			{ID: gA, Name: "Group A", GroupType: domain.GroupTypeSelect},
			{ID: gB, Name: "Group B", GroupType: domain.GroupTypeSelect},
			{ID: gC, Name: "Group C", GroupType: domain.GroupTypeSelect},
		},
		Edges: map[string][]domain.GroupEdge{
			gA: {{ID: "e1", ParentGroupID: gA, ChildGroupID: &gB, Position: 0}},
			gB: {{ID: "e2", ParentGroupID: gB, ChildGroupID: &gC, Position: 0}},
			gC: {{ID: "e3", ParentGroupID: gC, ChildGroupID: &gA, Position: 0}},
		},
	}

	_, err := r.Resolve(ctx, input)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "cycle_detected" {
		t.Fatalf("expected code cycle_detected, got: %v", err)
	}

	if domErr.Details["cycle"] == "" {
		t.Fatal("expected cycle path in details")
	}
}

func TestDeterministicResolver_RejectsSelfLoop(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gA := domain.MustNewUUIDv7()

	input := resolver.ResolveInput{
		Groups: []domain.NodeGroup{
			{ID: gA, Name: "Group A", GroupType: domain.GroupTypeSelect},
		},
		Edges: map[string][]domain.GroupEdge{
			gA: {{ID: "e1", ParentGroupID: gA, ChildGroupID: &gA, Position: 0}},
		},
	}

	_, err := r.Resolve(ctx, input)
	if err == nil {
		t.Fatal("expected self loop error, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "self_loop_forbidden" {
		t.Fatalf("expected code self_loop_forbidden, got: %v", err)
	}
}

func TestDeterministicResolver_RejectsMissingTargetGroup(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gA := domain.MustNewUUIDv7()
	missingID := domain.MustNewUUIDv7()

	input := resolver.ResolveInput{
		Groups: []domain.NodeGroup{
			{ID: gA, Name: "Group A", GroupType: domain.GroupTypeSelect},
		},
		Edges: map[string][]domain.GroupEdge{
			gA: {{ID: "e1", ParentGroupID: gA, ChildGroupID: &missingID, Position: 0}},
		},
	}

	_, err := r.Resolve(ctx, input)
	if err == nil {
		t.Fatal("expected target_group_not_found error, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "target_group_not_found" {
		t.Fatalf("expected code target_group_not_found, got: %v", err)
	}
}

func TestDeterministicResolver_RejectsMultipleMatchRules(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gA := domain.MustNewUUIDv7()

	input := resolver.ResolveInput{
		Groups: []domain.NodeGroup{
			{ID: gA, Name: "Group A", GroupType: domain.GroupTypeSelect},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: gA, Expression: "MATCH", Position: 0},
			{ID: "r2", TargetGroupID: gA, Expression: "FINAL", Position: 1},
		},
	}

	_, err := r.Resolve(ctx, input)
	if err == nil {
		t.Fatal("expected invalid_match_rule_ordering error, got nil")
	}

	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "invalid_match_rule_ordering" {
		t.Fatalf("expected code invalid_match_rule_ordering, got: %v", err)
	}
}

func TestDeterministicResolver_MatchRulePlacedTerminal(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gID := domain.MustNewUUIDv7()
	nodeID := testNodeLogicalID("node-1")

	input := resolver.ResolveInput{
		Nodes:  []domain.Node{{LogicalID: nodeID, DisplayName: "Node 1", Protocol: domain.ProtocolVMess, Active: true}},
		Groups: []domain.NodeGroup{{ID: gID, Name: "Proxy", GroupType: domain.GroupTypeSelect}},
		Edges: map[string][]domain.GroupEdge{
			gID: {{ID: "e1", ParentGroupID: gID, NodeLogicalID: &nodeID, Position: 0}},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r-match", TargetGroupID: gID, Expression: "MATCH", Position: 0},
			{ID: "r-domain", TargetGroupID: gID, Expression: "DOMAIN-SUFFIX,example.com", Position: 1},
			{ID: "r-ip", TargetGroupID: gID, Expression: "IP-CIDR,192.168.1.0/24", Position: 2},
		},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if len(snap.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(snap.Rules))
	}

	// Terminal rule MUST be at the end
	lastRule := snap.Rules[len(snap.Rules)-1]
	if lastRule.Expression != "MATCH" || !lastRule.IsTerminal {
		t.Fatalf("expected last rule to be terminal MATCH, got: %#v", lastRule)
	}

	if snap.Rules[0].Expression == "MATCH" {
		t.Fatal("MATCH rule was not moved to terminal bottom position")
	}
}

func TestDeterministicResolver_AdmissionRulesAndExpressions(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gID := domain.MustNewUUIDv7()
	nHK := testNodeLogicalID("node-hk")
	nUS := testNodeLogicalID("node-us")
	nJP := testNodeLogicalID("node-jp")

	input := resolver.ResolveInput{
		Nodes: []domain.Node{
			{LogicalID: nHK, DisplayName: "HK Premium 01", Protocol: domain.ProtocolVMess, Active: true},
			{LogicalID: nUS, DisplayName: "US Standard 02", Protocol: domain.ProtocolTrojan, Active: true},
			{LogicalID: nJP, DisplayName: "JP Quarantined 03", Protocol: domain.ProtocolSS, Active: true},
		},
		Groups: []domain.NodeGroup{{ID: gID, Name: "Proxy", GroupType: domain.GroupTypeSelect}},
		Edges: map[string][]domain.GroupEdge{
			gID: {
				{ID: "e1", ParentGroupID: gID, NodeLogicalID: &nHK, Position: 0},
				{ID: "e2", ParentGroupID: gID, NodeLogicalID: &nUS, Position: 1},
				{ID: "e3", ParentGroupID: gID, NodeLogicalID: &nJP, Position: 2},
			},
		},
		AdmissionRules: []domain.AdmissionRule{
			{ID: "a1", Name: "reject-us", Expression: "country == 'US'", Action: domain.RuleActionReject, Position: 0},
			{ID: "a2", Name: "quarantine-jp", Expression: "name contains 'Quarantined'", Action: domain.RuleActionQuarantine, Position: 1},
			{ID: "a3", Name: "allow-all", Expression: "*", Action: domain.RuleActionAllow, Position: 2},
		},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if len(snap.Nodes) != 1 || snap.Nodes[0].LogicalID != nHK {
		t.Fatalf("expected only nHK admitted, got %d nodes: %#v", len(snap.Nodes), snap.Nodes)
	}

	if len(snap.ExcludedNodeIDs) != 2 {
		t.Fatalf("expected 2 excluded nodes, got %d", len(snap.ExcludedNodeIDs))
	}

	group := snap.Groups[0]
	if len(group.Members) != 1 || group.Members[0].TargetID != nHK {
		t.Fatalf("expected group to contain only nHK, got %#v", group.Members)
	}
}

func TestDeterministicResolver_SubtreeNodeExpansion(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gRoot := domain.MustNewUUIDv7()
	gChild1 := domain.MustNewUUIDv7()
	gChild2 := domain.MustNewUUIDv7()

	n1 := testNodeLogicalID("node-1")
	n2 := testNodeLogicalID("node-2")
	n3 := testNodeLogicalID("node-3")

	// Tree: Root -> [Child1, Child2]
	// Child1 -> [Node1, Node2]
	// Child2 -> [Node2, Node3]
	input := resolver.ResolveInput{
		Nodes: []domain.Node{
			{LogicalID: n1, DisplayName: "N1", Protocol: domain.ProtocolVMess, Active: true},
			{LogicalID: n2, DisplayName: "N2", Protocol: domain.ProtocolTrojan, Active: true},
			{LogicalID: n3, DisplayName: "N3", Protocol: domain.ProtocolSS, Active: true},
		},
		Groups: []domain.NodeGroup{
			{ID: gRoot, Name: "Root", GroupType: domain.GroupTypeSelect},
			{ID: gChild1, Name: "Child1", GroupType: domain.GroupTypeSelect},
			{ID: gChild2, Name: "Child2", GroupType: domain.GroupTypeSelect},
		},
		Edges: map[string][]domain.GroupEdge{
			gRoot: {
				{ID: "e0", ParentGroupID: gRoot, ChildGroupID: &gChild1, Position: 0},
				{ID: "e1", ParentGroupID: gRoot, ChildGroupID: &gChild2, Position: 1},
			},
			gChild1: {
				{ID: "e2", ParentGroupID: gChild1, NodeLogicalID: &n1, Position: 0},
				{ID: "e3", ParentGroupID: gChild1, NodeLogicalID: &n2, Position: 1},
			},
			gChild2: {
				{ID: "e4", ParentGroupID: gChild2, NodeLogicalID: &n2, Position: 0},
				{ID: "e5", ParentGroupID: gChild2, NodeLogicalID: &n3, Position: 1},
			},
		},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	var rootGroup *resolver.ResolvedGroup
	for i := range snap.Groups {
		if snap.Groups[i].ID == gRoot {
			rootGroup = &snap.Groups[i]
			break
		}
	}

	if rootGroup == nil {
		t.Fatal("root group not found in snapshot")
	}

	// AllNodeLogicalIDs must contain all 3 nodes deduplicated
	if len(rootGroup.AllNodeLogicalIDs) != 3 {
		t.Fatalf("expected 3 nodes in root AllNodeLogicalIDs, got %d: %v",
			len(rootGroup.AllNodeLogicalIDs), rootGroup.AllNodeLogicalIDs)
	}
}

func TestDeterministicResolver_RiskMetadataPreserved(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gID := domain.MustNewUUIDv7()
	n1 := testNodeLogicalID("node-1")
	now := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)

	input := resolver.ResolveInput{
		RevisionID:         "rev-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{{LogicalID: n1, DisplayName: "N1", Protocol: domain.ProtocolVMess, Active: true}},
		Groups:             []domain.NodeGroup{{ID: gID, Name: "Proxy", GroupType: domain.GroupTypeSelect}},
		Edges: map[string][]domain.GroupEdge{
			gID: {{ID: "e1", ParentGroupID: gID, NodeLogicalID: &n1, Position: 0}},
		},
		RiskPolicyRevision: "risk-rev-42",
		RiskDecisionDigest: "decision-digest-abc",
		RiskEvaluatedAt:    &now,
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if snap.RiskPolicyRevision != "risk-rev-42" {
		t.Errorf("expected RiskPolicyRevision risk-rev-42, got %s", snap.RiskPolicyRevision)
	}
	if snap.RiskDecisionDigest != "decision-digest-abc" {
		t.Errorf("expected RiskDecisionDigest decision-digest-abc, got %s", snap.RiskDecisionDigest)
	}
	if snap.RiskEvaluatedAt == nil || !snap.RiskEvaluatedAt.Equal(now) {
		t.Errorf("expected RiskEvaluatedAt %v, got %v", now, snap.RiskEvaluatedAt)
	}
}

func TestDeterministicResolver_ContextCancellation(t *testing.T) {
	r := resolver.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := r.Resolve(ctx, resolver.ResolveInput{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
}

func TestRiskAwareResolver_ExcludesBlockedAndReviewNodesWithStableDiagnostics(t *testing.T) {
	r := resolver.New()
	groupID := domain.MustNewUUIDv7()
	nodeBlock := testNodeLogicalID("risk-block")
	nodeReview := testNodeLogicalID("risk-review")
	nodeAllow := testNodeLogicalID("risk-allow")
	evaluatedAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	policyRevision := domain.MustNewUUIDv7()
	input := resolver.ResolveInput{
		RevisionID:         "revision-risk",
		InventoryWatermark: "inventory-risk",
		CompilerVersion:    "1.0.0",
		Nodes: []domain.Node{
			{LogicalID: nodeAllow, DisplayName: "Allow", Protocol: domain.ProtocolVMess, Active: true},
			{LogicalID: nodeReview, DisplayName: "Review", Protocol: domain.ProtocolVMess, Active: true},
			{LogicalID: nodeBlock, DisplayName: "Block", Protocol: domain.ProtocolVMess, Active: true},
		},
		Groups: []domain.NodeGroup{{ID: groupID, Name: "Proxy", GroupType: domain.GroupTypeSelect}},
		Edges: map[string][]domain.GroupEdge{groupID: {
			{ID: "allow", ParentGroupID: groupID, NodeLogicalID: &nodeAllow, Position: 0},
			{ID: "review", ParentGroupID: groupID, NodeLogicalID: &nodeReview, Position: 1},
			{ID: "block", ParentGroupID: groupID, NodeLogicalID: &nodeBlock, Position: 2},
		}},
		RiskPolicyRevision: policyRevision,
		RiskDecisionDigest: "sha256:" + strings.Repeat("a", 64),
		RiskEvaluatedAt:    &evaluatedAt,
		RiskDecisions: []domain.RiskDecision{
			{NodeLogicalID: nodeBlock, PolicyRevisionID: policyRevision, Decision: domain.RiskActionBlock, ReasonCode: "score_band_high", EvaluatedAt: evaluatedAt},
			{NodeLogicalID: nodeReview, PolicyRevisionID: policyRevision, Decision: domain.RiskActionReview, ReasonCode: "observation_missing", EvaluatedAt: evaluatedAt},
			{NodeLogicalID: nodeAllow, PolicyRevisionID: policyRevision, Decision: domain.RiskActionAllow, ReasonCode: "score_band_low", EvaluatedAt: evaluatedAt},
		},
	}

	first, err := r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	second, err := r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if got := first.NodeLogicalIDs; len(got) != 1 || got[0] != nodeAllow {
		t.Fatalf("expected only allowed node, got %v", got)
	}
	expectedExcluded := []string{nodeBlock, nodeReview}
	sort.Strings(expectedExcluded)
	if !reflect.DeepEqual(first.ExcludedNodeIDs, expectedExcluded) {
		t.Fatalf("expected sorted risk exclusions, got %v", first.ExcludedNodeIDs)
	}
	if first.SnapshotDigest != second.SnapshotDigest {
		t.Fatalf("risk-aware snapshot digest is not stable: %s != %s", first.SnapshotDigest, second.SnapshotDigest)
	}
	if !reflect.DeepEqual(first.Diagnostics, second.Diagnostics) {
		t.Fatalf("risk diagnostics are not stable: %#v != %#v", first.Diagnostics, second.Diagnostics)
	}
	if !hasDiagnostic(first.Diagnostics, "risk_blocked", nodeBlock) || !hasDiagnostic(first.Diagnostics, "risk_review", nodeReview) {
		t.Fatalf("expected block and review diagnostics, got %#v", first.Diagnostics)
	}
	admissionChanged := input
	admissionChanged.RiskDecisions = append([]domain.RiskDecision(nil), input.RiskDecisions...)
	admissionChanged.RiskDecisions[0].Decision = domain.RiskActionAllow
	changed, err := r.Resolve(context.Background(), admissionChanged)
	if err != nil {
		t.Fatalf("changed risk resolve failed: %v", err)
	}
	if first.SnapshotDigest == changed.SnapshotDigest {
		t.Fatal("snapshot digest ignored changed risk admission")
	}
}

func TestRiskAwareResolver_ReviewAllowKeepsReviewedNode(t *testing.T) {
	r := resolver.New()
	groupID := domain.MustNewUUIDv7()
	nodeID := testNodeLogicalID("risk-review-allow")
	evaluatedAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	policyRevision := domain.MustNewUUIDv7()
	input := resolver.ResolveInput{
		Nodes:              []domain.Node{{LogicalID: nodeID, DisplayName: "Reviewed", Protocol: domain.ProtocolVMess, Active: true}},
		Groups:             []domain.NodeGroup{{ID: groupID, Name: "Proxy", GroupType: domain.GroupTypeSelect}},
		Edges:              map[string][]domain.GroupEdge{groupID: {{ID: "edge", ParentGroupID: groupID, NodeLogicalID: &nodeID, Position: 0}}},
		RiskPolicyRevision: policyRevision,
		RiskDecisionDigest: "sha256:" + strings.Repeat("b", 64),
		RiskEvaluatedAt:    &evaluatedAt,
		RiskReviewAction:   domain.RiskActionAllow,
		RiskDecisions: []domain.RiskDecision{{
			NodeLogicalID: nodeID, PolicyRevisionID: policyRevision, Decision: domain.RiskActionReview,
			ReasonCode: "manual_review", EvaluatedAt: evaluatedAt,
		}},
	}
	snapshot, err := r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(snapshot.Nodes) != 1 || snapshot.Nodes[0].LogicalID != nodeID {
		t.Fatalf("review-allowed node was excluded: %#v", snapshot.Nodes)
	}
	if len(snapshot.ExcludedNodeIDs) != 0 {
		t.Fatalf("expected no exclusions, got %v", snapshot.ExcludedNodeIDs)
	}
}

func TestRiskAwareResolver_UnboundInputPreservesOriginalAdmission(t *testing.T) {
	r := resolver.New()
	groupID := domain.MustNewUUIDv7()
	nodeID := testNodeLogicalID("unbound-risk")
	input := resolver.ResolveInput{
		Nodes:  []domain.Node{{LogicalID: nodeID, DisplayName: "Unbound", Protocol: domain.ProtocolVMess, Active: true}},
		Groups: []domain.NodeGroup{{ID: groupID, Name: "Proxy", GroupType: domain.GroupTypeSelect}},
		Edges:  map[string][]domain.GroupEdge{groupID: {{ID: "edge", ParentGroupID: groupID, NodeLogicalID: &nodeID, Position: 0}}},
	}
	snapshot, err := r.Resolve(context.Background(), input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(snapshot.Nodes) != 1 || len(snapshot.ExcludedNodeIDs) != 0 || snapshot.RiskPolicyRevision != "" {
		t.Fatalf("unbound risk input changed original admission: %#v", snapshot)
	}
}

func hasDiagnostic(diagnostics []resolver.Diagnostic, code, target string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && diagnostic.Target == target {
			return true
		}
	}
	return false
}

func TestDeterministicResolver_ConcurrentResolutionsAreIdentical(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()
	node1ID := testNodeLogicalID("node-1")
	node2ID := testNodeLogicalID("node-2")

	input := resolver.ResolveInput{
		RevisionID:         "rev-concurrent",
		InventoryWatermark: "wm-100",
		CompilerVersion:    "1.0.0",
		Nodes: []domain.Node{
			{LogicalID: node1ID, DisplayName: "HK 1", Protocol: domain.ProtocolVMess, Active: true},
			{LogicalID: node2ID, DisplayName: "US 2", Protocol: domain.ProtocolTrojan, Active: true},
		},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "PROXY", GroupType: domain.GroupTypeSelect},
			{ID: childID, Name: "AUTO", GroupType: domain.GroupTypeURLTest},
		},
		Edges: map[string][]domain.GroupEdge{
			parentID: {
				{ID: "e1", ParentGroupID: parentID, ChildGroupID: &childID, Position: 0},
				{ID: "e2", ParentGroupID: parentID, NodeLogicalID: &node1ID, Position: 1},
			},
			childID: {
				{ID: "e3", ParentGroupID: childID, NodeLogicalID: &node1ID, Position: 0},
				{ID: "e4", ParentGroupID: childID, NodeLogicalID: &node2ID, Position: 1},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentID, Expression: "DOMAIN,example.com", Position: 0},
			{ID: "r2", TargetGroupID: parentID, Expression: "MATCH", Position: 1},
		},
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		},
	}

	// 100 concurrent resolutions
	const concurrency = 100
	var wg sync.WaitGroup
	digests := make([]string, concurrency)
	errorsList := make([]error, concurrency)

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			snap, err := r.Resolve(ctx, input)
			if err != nil {
				errorsList[idx] = err
				return
			}
			digests[idx] = snap.SnapshotDigest
		}(i)
	}
	wg.Wait()

	for i := 0; i < concurrency; i++ {
		if errorsList[i] != nil {
			t.Fatalf("goroutine %d failed: %v", i, errorsList[i])
		}
		if digests[i] == "" {
			t.Fatalf("goroutine %d produced empty digest", i)
		}
		if digests[i] != digests[0] {
			t.Fatalf("goroutine %d digest mismatch: got %s, expected %s", i, digests[i], digests[0])
		}
	}
}

func TestResolver_TwoLevelFiltering_GlobalAndGroup(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	n1 := domain.Node{LogicalID: testNodeLogicalID("hk-ss-01"), DisplayName: "HK SS 01", Protocol: domain.ProtocolSS, Active: true}
	n2 := domain.Node{LogicalID: testNodeLogicalID("hk-vmess-02"), DisplayName: "HK VMess 02", Protocol: domain.ProtocolVMess, Active: true}
	n3 := domain.Node{LogicalID: testNodeLogicalID("us-ss-03"), DisplayName: "US SS 03", Protocol: domain.ProtocolSS, Active: true}

	input := resolver.ResolveInput{
		RevisionID:         "rev-filter-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{n1, n2, n3},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "PROXY", GroupType: domain.GroupTypeSelect},
		},
		Edges: map[string][]domain.GroupEdge{
			parentID: {
				{ID: "e1", ParentGroupID: parentID, NodeLogicalID: &n1.LogicalID, Position: 0},
				{ID: "e2", ParentGroupID: parentID, NodeLogicalID: &n2.LogicalID, Position: 1},
				{ID: "e3", ParentGroupID: parentID, NodeLogicalID: &n3.LogicalID, Position: 2},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentID, Expression: "MATCH", Position: 0},
		},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpEquals, Value: "ss"},
			},
		},
		GroupFilters: map[string]domain.NodeFilterSpec{
			parentID: {
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "HK"},
				},
			},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// Global filter allows SS only: n1 (HK SS) and n3 (US SS). n2 (VMess) excluded globally.
	if len(snap.Nodes) != 2 {
		t.Fatalf("expected 2 globally admitted nodes, got %d", len(snap.Nodes))
	}

	// Group filter allows "HK" only: from {n1, n3}, only n1 passes.
	if len(snap.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(snap.Groups))
	}
	grp := snap.Groups[0]
	if len(grp.Members) != 1 || grp.Members[0].TargetID != n1.LogicalID {
		t.Fatalf("expected group to contain only n1, got %+v", grp.Members)
	}

	// Verify diagnostics: n2 excluded by global filter, n3 excluded by group filter
	foundGlobalExcl := false
	foundGroupExcl := false
	for _, d := range snap.Diagnostics {
		if d.Code == "global_filter_excluded" && d.Target == n2.LogicalID {
			foundGlobalExcl = true
		}
		if d.Code == "group_member_filtered" && d.Target == n3.LogicalID {
			foundGroupExcl = true
		}
	}
	if !foundGlobalExcl {
		t.Errorf("missing global_filter_excluded diagnostic for n2")
	}
	if !foundGroupExcl {
		t.Errorf("missing group_member_filtered diagnostic for n3")
	}
}

func TestResolver_DynamicGroup_MatchesFromGlobalPool(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gid := domain.MustNewUUIDv7()
	n1 := domain.Node{LogicalID: testNodeLogicalID("n-jp-01"), DisplayName: "JP Fast 01", Protocol: domain.ProtocolTrojan, Active: true}
	n2 := domain.Node{LogicalID: testNodeLogicalID("n-us-02"), DisplayName: "US Fast 02", Protocol: domain.ProtocolTrojan, Active: true}
	n3 := domain.Node{LogicalID: testNodeLogicalID("n-jp-03"), DisplayName: "JP Slow 03", Protocol: domain.ProtocolTrojan, Active: true}

	input := resolver.ResolveInput{
		RevisionID:         "rev-dyn-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{n1, n2, n3},
		Groups: []domain.NodeGroup{
			{ID: gid, Name: "JP-Group", GroupType: domain.GroupTypeURLTest},
		},
		// No edges for gid!
		Edges: map[string][]domain.GroupEdge{},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: gid, Expression: "MATCH", Position: 0},
		},
		GroupFilters: map[string]domain.NodeFilterSpec{
			gid: {
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "JP"},
				},
			},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if len(snap.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(snap.Groups))
	}
	grp := snap.Groups[0]
	// Dynamically matched n1 and n3, sorted by DisplayName ASC
	if len(grp.Members) != 2 {
		t.Fatalf("expected 2 dynamic members, got %d", len(grp.Members))
	}
	if grp.Members[0].TargetID != n1.LogicalID || grp.Members[1].TargetID != n3.LogicalID {
		t.Fatalf("unexpected dynamic members: %+v", grp.Members)
	}
}

func TestResolver_NestedGroupProjection_RestrictsChildNodes(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()

	n1 := domain.Node{LogicalID: testNodeLogicalID("us-fast-01"), DisplayName: "US Fast 01", Protocol: domain.ProtocolTrojan, Active: true}
	n2 := domain.Node{LogicalID: testNodeLogicalID("us-slow-02"), DisplayName: "US Slow 02", Protocol: domain.ProtocolTrojan, Active: true}

	input := resolver.ResolveInput{
		RevisionID:         "rev-nested-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{n1, n2},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "Proxy", GroupType: domain.GroupTypeSelect},
			{ID: childID, Name: "US-Auto", GroupType: domain.GroupTypeURLTest},
		},
		Edges: map[string][]domain.GroupEdge{
			parentID: {
				{ID: "e1", ParentGroupID: parentID, ChildGroupID: &childID, Position: 0},
			},
			childID: {
				{ID: "e2", ParentGroupID: childID, NodeLogicalID: &n1.LogicalID, Position: 0},
				{ID: "e3", ParentGroupID: childID, NodeLogicalID: &n2.LogicalID, Position: 1},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentID, Expression: "MATCH", Position: 0},
		},
		GroupFilters: map[string]domain.NodeFilterSpec{
			parentID: {
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "Fast"},
				},
			},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// Parent has filter "Fast". Child has n1 (Fast) and n2 (Slow).
	// A derived projection for Child under Parent must be created, containing only n1!
	// In snap.Groups, we should have original Parent, original Child, and derived Child.
	if len(snap.Groups) < 3 {
		t.Fatalf("expected at least 3 groups including derived projection, got %d", len(snap.Groups))
	}

	var parentGroup *resolver.ResolvedGroup
	for i := range snap.Groups {
		if snap.Groups[i].ID == parentID {
			parentGroup = &snap.Groups[i]
			break
		}
	}
	if parentGroup == nil {
		t.Fatalf("parent group not found")
	}

	// Parent's member must reference the derived group
	if len(parentGroup.Members) != 1 {
		t.Fatalf("expected 1 member in parent, got %d", len(parentGroup.Members))
	}
	derivedMember := parentGroup.Members[0]
	if derivedMember.TargetID == childID {
		t.Fatalf("parent member should reference derived group ID, not original child ID")
	}
	if !strings.Contains(derivedMember.DisplayName, "US-Auto [Proxy]") {
		t.Fatalf("expected derived name 'US-Auto [Proxy]', got %s", derivedMember.DisplayName)
	}

	// Parent's AllNodeLogicalIDs must only contain n1 (not n2!)
	if len(parentGroup.AllNodeLogicalIDs) != 1 || parentGroup.AllNodeLogicalIDs[0] != n1.LogicalID {
		t.Fatalf("parent AllNodeLogicalIDs must only contain n1, got %+v", parentGroup.AllNodeLogicalIDs)
	}
}

func TestResolver_EmptyRoutedGroup_FailClosedDiagnostic(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	n1 := domain.Node{LogicalID: testNodeLogicalID("hk-01"), DisplayName: "HK 01", Protocol: domain.ProtocolTrojan, Active: true}

	input := resolver.ResolveInput{
		RevisionID:         "rev-empty-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{n1},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "Proxy", GroupType: domain.GroupTypeSelect},
		},
		Edges: map[string][]domain.GroupEdge{
			parentID: {
				{ID: "e1", ParentGroupID: parentID, NodeLogicalID: &n1.LogicalID, Position: 0},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentID, Expression: "MATCH", Position: 0},
		},
		// Filter excludes n1, making Proxy completely empty
		GroupFilters: map[string]domain.NodeFilterSpec{
			parentID: {
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
				},
			},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	foundEmptyRouted := false
	for _, d := range snap.Diagnostics {
		if d.Code == "empty_routed_group" && d.Severity == resolver.DiagnosticSeverityError {
			foundEmptyRouted = true
			if d.Target != parentID {
				t.Fatalf("expected target to be %s, got %s", parentID, d.Target)
			}
		}
	}
	if !foundEmptyRouted {
		t.Fatalf("expected empty_routed_group error diagnostic, got %+v", snap.Diagnostics)
	}
}

func TestResolver_OldConfigEmptyGroup_PreservesWarning(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()

	input := resolver.ResolveInput{
		RevisionID:         "rev-old-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "EmptyOldGroup", GroupType: domain.GroupTypeSelect},
		},
		Edges: map[string][]domain.GroupEdge{},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentID, Expression: "MATCH", Position: 0},
		},
		// No filters defined anywhere!
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	foundWarning := false
	for _, d := range snap.Diagnostics {
		if d.Code == "empty_routed_group" {
			t.Fatalf("old config with no filters should not emit empty_routed_group error")
		}
		if d.Code == "empty_group" && d.Severity == resolver.DiagnosticSeverityWarning {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected empty_group warning diagnostic for old config, got %+v", snap.Diagnostics)
	}
}

func TestResolver_ProbeFailClosed_MissingExpiredMismatchNotEquals(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gid := domain.MustNewUUIDv7()
	nID := testNodeLogicalID("node-probe-test")
	node := domain.Node{
		LogicalID:         nID,
		DisplayName:       "Probe Test Node",
		Protocol:          domain.ProtocolTrojan,
		Active:            true,
		CredentialVersion: 2,
	}

	freshness := 300
	kind := domain.ProbeKindBaseline
	ver := 2
	now := time.Now().UTC()

	baseInput := func() resolver.ResolveInput {
		return resolver.ResolveInput{
			RevisionID:         "rev-p1",
			InventoryWatermark: "wm-1",
			CompilerVersion:    "1.0.0",
			Nodes:              []domain.Node{node},
			Groups:             []domain.NodeGroup{{ID: gid, Name: "G1", GroupType: domain.GroupTypeSelect}},
			Edges: map[string][]domain.GroupEdge{
				gid: {{ID: "e1", ParentGroupID: gid, NodeLogicalID: &nID, Position: 0}},
			},
			PolicyRules: []domain.PolicyRule{{ID: "r1", TargetGroupID: gid, Expression: "MATCH", Position: 0}},
			DNS:         resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
			AsOf:        now,
		}
	}

	// 1. Missing observation: Even NOT_EQUALS 'error' must fail-closed (return false)
	t.Run("missing observation rejects not_equals", func(t *testing.T) {
		inp := baseInput()
		inp.GlobalFilter = &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field:            domain.FilterFieldProbeVerdict,
					Op:               domain.FilterOpNotEquals,
					Value:            "error",
					ProbeKind:        &kind,
					FreshnessSeconds: &freshness,
				},
			},
		}
		inp.LatestObservations = nil // missing

		snap, err := r.Resolve(ctx, inp)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if len(snap.Nodes) != 0 {
			t.Fatalf("missing observation should fail-closed and reject node even for not_equals")
		}
	})

	// 2. Mismatched credential version (obs version 1 vs node version 2)
	t.Run("mismatched credential version rejects", func(t *testing.T) {
		inp := baseInput()
		inp.GlobalFilter = &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field:            domain.FilterFieldProbeVerdict,
					Op:               domain.FilterOpEquals,
					Value:            "available",
					ProbeKind:        &kind,
					FreshnessSeconds: &freshness,
				},
			},
		}
		mismatchedVer := 1
		inp.LatestObservations = map[string]map[domain.ProbeKind]domain.ProbeObservation{
			nID: {
				domain.ProbeKindBaseline: {
					ID:                "obs-1",
					NodeLogicalID:     nID,
					Kind:              domain.ProbeKindBaseline,
					Verdict:           domain.VerdictAvailable,
					ObservedAt:        now,
					CredentialVersion: &mismatchedVer,
				},
			},
		}

		snap, err := r.Resolve(ctx, inp)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if len(snap.Nodes) != 0 {
			t.Fatalf("mismatched credential version should fail-closed and reject node")
		}
	})

	// 3. Expired observation
	t.Run("expired observation rejects", func(t *testing.T) {
		inp := baseInput()
		inp.GlobalFilter = &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field:            domain.FilterFieldProbeVerdict,
					Op:               domain.FilterOpEquals,
					Value:            "available",
					ProbeKind:        &kind,
					FreshnessSeconds: &freshness, // 300s
				},
			},
		}
		expiredTime := now.Add(-400 * time.Second) // 400s ago
		inp.LatestObservations = map[string]map[domain.ProbeKind]domain.ProbeObservation{
			nID: {
				domain.ProbeKindBaseline: {
					ID:                "obs-1",
					NodeLogicalID:     nID,
					Kind:              domain.ProbeKindBaseline,
					Verdict:           domain.VerdictAvailable,
					ObservedAt:        expiredTime,
					CredentialVersion: &ver,
				},
			},
		}

		snap, err := r.Resolve(ctx, inp)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if len(snap.Nodes) != 0 {
			t.Fatalf("expired observation should fail-closed and reject node")
		}
	})

	// 4. Valid observation passes
	t.Run("valid observation passes", func(t *testing.T) {
		inp := baseInput()
		inp.GlobalFilter = &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field:            domain.FilterFieldProbeVerdict,
					Op:               domain.FilterOpEquals,
					Value:            "available",
					ProbeKind:        &kind,
					FreshnessSeconds: &freshness,
				},
			},
		}
		validTime := now.Add(-50 * time.Second)
		inp.LatestObservations = map[string]map[domain.ProbeKind]domain.ProbeObservation{
			nID: {
				domain.ProbeKindBaseline: {
					ID:                "obs-1",
					NodeLogicalID:     nID,
					Kind:              domain.ProbeKindBaseline,
					Verdict:           domain.VerdictAvailable,
					LatencyMS:         45,
					ObservedAt:        validTime,
					CredentialVersion: &ver,
				},
			},
		}

		snap, err := r.Resolve(ctx, inp)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if len(snap.Nodes) != 1 {
			t.Fatalf("valid observation should admit node")
		}
	})
}

func TestResolver_MultiParentNestedGroupProjection(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parent1ID := domain.MustNewUUIDv7()
	parent2ID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()

	nUS := domain.Node{LogicalID: testNodeLogicalID("n-us-multi"), DisplayName: "US Node", Protocol: domain.ProtocolTrojan, Active: true}
	nHK := domain.Node{LogicalID: testNodeLogicalID("n-hk-multi"), DisplayName: "HK Node", Protocol: domain.ProtocolTrojan, Active: true}

	input := resolver.ResolveInput{
		RevisionID:         "rev-multi-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{nUS, nHK},
		Groups: []domain.NodeGroup{
			{ID: parent1ID, Name: "Parent-US", GroupType: domain.GroupTypeSelect},
			{ID: parent2ID, Name: "Parent-HK", GroupType: domain.GroupTypeSelect},
			{ID: childID, Name: "All-Auto", GroupType: domain.GroupTypeURLTest},
		},
		Edges: map[string][]domain.GroupEdge{
			parent1ID: {{ID: "e1", ParentGroupID: parent1ID, ChildGroupID: &childID, Position: 0}},
			parent2ID: {{ID: "e2", ParentGroupID: parent2ID, ChildGroupID: &childID, Position: 0}},
			childID: {
				{ID: "e3", ParentGroupID: childID, NodeLogicalID: &nUS.LogicalID, Position: 0},
				{ID: "e4", ParentGroupID: childID, NodeLogicalID: &nHK.LogicalID, Position: 1},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parent1ID, Expression: "DOMAIN-SUFFIX,google.com", Position: 0},
			{ID: "r2", TargetGroupID: parent2ID, Expression: "MATCH", Position: 1},
		},
		GroupFilters: map[string]domain.NodeFilterSpec{
			parent1ID: {
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "US"},
				},
			},
			parent2ID: {
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "HK"},
				},
			},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	var p1, p2 *resolver.ResolvedGroup
	for i := range snap.Groups {
		if snap.Groups[i].ID == parent1ID {
			p1 = &snap.Groups[i]
		}
		if snap.Groups[i].ID == parent2ID {
			p2 = &snap.Groups[i]
		}
	}
	if p1 == nil || p2 == nil {
		t.Fatalf("parents not found")
	}

	// p1's AllNodeLogicalIDs must only contain nUS
	if len(p1.AllNodeLogicalIDs) != 1 || p1.AllNodeLogicalIDs[0] != nUS.LogicalID {
		t.Fatalf("p1 should only contain nUS, got %+v", p1.AllNodeLogicalIDs)
	}

	// p2's AllNodeLogicalIDs must only contain nHK
	if len(p2.AllNodeLogicalIDs) != 1 || p2.AllNodeLogicalIDs[0] != nHK.LogicalID {
		t.Fatalf("p2 should only contain nHK, got %+v", p2.AllNodeLogicalIDs)
	}

	// Ensure p1's member derived ID != p2's member derived ID
	if p1.Members[0].TargetID == p2.Members[0].TargetID {
		t.Fatalf("derived child groups for different parents must have unique IDs")
	}
}

func TestResolver_FreshnessBoundaryDigestChange(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gid := domain.MustNewUUIDv7()
	nID := testNodeLogicalID("node-freshness-digest")
	node := domain.Node{
		LogicalID:         nID,
		DisplayName:       "Digest Node",
		Protocol:          domain.ProtocolTrojan,
		Active:            true,
		CredentialVersion: 1,
	}

	kind := domain.ProbeKindBaseline
	ver := 1
	obsTime := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	createInput := func(asOf time.Time) resolver.ResolveInput {
		return resolver.ResolveInput{
			RevisionID:         "rev-f1",
			InventoryWatermark: "wm-1",
			CompilerVersion:    "1.0.0",
			Nodes:              []domain.Node{node},
			Groups:             []domain.NodeGroup{{ID: gid, Name: "G1", GroupType: domain.GroupTypeSelect}},
			Edges: map[string][]domain.GroupEdge{
				gid: {{ID: "e1", ParentGroupID: gid, NodeLogicalID: &nID, Position: 0}},
			},
			PolicyRules: []domain.PolicyRule{{ID: "r1", TargetGroupID: gid, Expression: "MATCH", Position: 0}},
			DNS:         resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
			LatestObservations: map[string]map[domain.ProbeKind]domain.ProbeObservation{
				nID: {
					domain.ProbeKindBaseline: {
						ID:                "obs-1",
						NodeLogicalID:     nID,
						Kind:              kind,
						Verdict:           domain.VerdictAvailable,
						LatencyMS:         30,
						ObservedAt:        obsTime,
						CredentialVersion: &ver,
					},
				},
			},
			AsOf: asOf,
		}
	}

	// Two asOf timestamps within fresh window (e.g. 1 hour and 2 hours after obsTime, where 24h freshness is default)
	asOf1 := obsTime.Add(1 * time.Hour)
	asOf2 := obsTime.Add(2 * time.Hour)
	// One asOf timestamp after expiration (> 24 hours after obsTime)
	asOfExpired := obsTime.Add(25 * time.Hour)

	snap1, err := r.Resolve(ctx, createInput(asOf1))
	if err != nil {
		t.Fatalf("snap1 failed: %v", err)
	}
	snap2, err := r.Resolve(ctx, createInput(asOf2))
	if err != nil {
		t.Fatalf("snap2 failed: %v", err)
	}
	snapExp, err := r.Resolve(ctx, createInput(asOfExpired))
	if err != nil {
		t.Fatalf("snapExp failed: %v", err)
	}

	// Within freshness window, wall-clock advance should not alter input digest
	if snap1.InputDigest != snap2.InputDigest {
		t.Fatalf("input digest should be identical across wall-clock ticks before freshness boundary: %s != %s", snap1.InputDigest, snap2.InputDigest)
	}

	// Crossing the 24h freshness boundary MUST alter input digest
	if snap1.InputDigest == snapExp.InputDigest {
		t.Fatalf("input digest MUST change when crossing freshness expiration boundary")
	}
}
