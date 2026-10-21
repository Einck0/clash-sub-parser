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

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func testNodeLogicalID(name string) string {
	h := sha256.Sum256([]byte(name))
	return hex.EncodeToString(h[:])
}

func TestDeterministicResolver_IdenticalInputProducesExactSameDigest(t *testing.T) {
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	// FR-C3: Admission rule DSL matching is retired. Legacy admission rules
	// (such as reject or quarantine expressions) are ignored and no longer filter active nodes.
	// All active nodes remain admitted, and a single diagnostic warning
	// "legacy_admission_rules_ignored" is emitted without expression text.
	r := sourceFixtureResolver{}
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

	// Under retired contract, all 3 active nodes remain admitted despite legacy reject/quarantine rules
	if len(snap.Nodes) != 3 {
		t.Fatalf("expected all 3 active nodes admitted under retired admission contract, got %d nodes: %#v", len(snap.Nodes), snap.Nodes)
	}

	if len(snap.ExcludedNodeIDs) != 0 {
		t.Fatalf("expected 0 excluded nodes (active nodes not excluded by legacy admission), got %d: %v", len(snap.ExcludedNodeIDs), snap.ExcludedNodeIDs)
	}

	group := snap.Groups[0]
	if len(group.Members) != 3 {
		t.Fatalf("expected group to contain all 3 admitted members, got %d: %#v", len(group.Members), group.Members)
	}

	// Verify exactly one stable diagnostic warning is emitted for legacy admission rules
	legacyDiagCount := 0
	for _, d := range snap.Diagnostics {
		if d.Code == "legacy_admission_rules_ignored" {
			legacyDiagCount++
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for legacy_admission_rules_ignored, got: %s", d.Severity)
			}
			if strings.Contains(d.Message, "country == 'US'") || strings.Contains(d.Message, "contains 'Quarantined'") {
				t.Errorf("diagnostic message must NOT contain raw expression text, got: %s", d.Message)
			}
		}
	}
	if legacyDiagCount != 1 {
		t.Fatalf("expected exactly 1 legacy_admission_rules_ignored diagnostic, got %d", legacyDiagCount)
	}
}

func TestDeterministicResolver_WARPGroup_LegacyAdmissionIgnored_RiskAndInactiveStillExcluded(t *testing.T) {
	// FR-C3 & Spec Scenario 2:
	// Active VLESS node has legacy admission rule (reject protocol vless), does not match global filter,
	// but belongs to a WARP group with InheritGlobalFilter = false.
	// Contract:
	// 1. Legacy admission reject rule is ignored -> node is admitted at admission layer.
	// 2. WARP group (InheritGlobalFilter = false) can reference and include the active VLESS node.
	// 3. Normal group (InheritGlobalFilter = true) filters it out via GlobalFilter.
	// 4. Inactive VLESS node and risk-excluded VLESS node are strictly excluded by hard constraints,
	//    and cannot enter either normal or WARP groups.
	// 5. Exactly 1 stable diagnostic warning legacy_admission_rules_ignored is emitted without expression text.
	r := resolver.New()
	ctx := context.Background()

	proxyGroupID := domain.MustNewUUIDv7()
	warpGroupID := domain.MustNewUUIDv7()

	nHK := domain.Node{
		LogicalID:   testNodeLogicalID("node-hk-vmess"),
		DisplayName: "HK Active VMess",
		Protocol:    domain.ProtocolVMess,
		Active:      true,
	}
	nWARPActive := domain.Node{
		LogicalID:   testNodeLogicalID("node-warp-active-vless"),
		DisplayName: "WARP Active VLess",
		Protocol:    domain.ProtocolVLESS,
		Active:      true,
	}
	nWARPInactive := domain.Node{
		LogicalID:   testNodeLogicalID("node-warp-inactive-vless"),
		DisplayName: "WARP Inactive VLess",
		Protocol:    domain.ProtocolVLESS,
		Active:      false,
	}
	nWARPRisk := domain.Node{
		LogicalID:   testNodeLogicalID("node-warp-risk-vless"),
		DisplayName: "WARP Risk VLess",
		Protocol:    domain.ProtocolVLESS,
		Active:      true,
	}

	input := resolver.ResolveInput{
		RevisionID:         "rev-warp-admission-test",
		InventoryWatermark: "wm-warp-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{nHK, nWARPActive, nWARPInactive, nWARPRisk},
		ExcludedNodeIDs:    []string{nWARPRisk.LogicalID}, // Explicit risk exclusion hard constraint
		Groups: []domain.NodeGroup{
			{
				ID:                  proxyGroupID,
				Name:                "PROXY",
				GroupType:           domain.GroupTypeSelect,
				InheritGlobalFilter: true,
				Sources: []domain.MemberSource{
					{ID: "s1", GroupID: proxyGroupID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 0, NodeLogicalID: &nHK.LogicalID},
					{ID: "s2", GroupID: proxyGroupID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 1, NodeLogicalID: &nWARPActive.LogicalID},
				},
			},
			{
				ID:                  warpGroupID,
				Name:                "WARP",
				GroupType:           domain.GroupTypeSelect,
				InheritGlobalFilter: false,
				Sources: []domain.MemberSource{
					{ID: "w1", GroupID: warpGroupID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 0, NodeLogicalID: &nHK.LogicalID},
					{ID: "w2", GroupID: warpGroupID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 1, NodeLogicalID: &nWARPActive.LogicalID},
					{ID: "w3", GroupID: warpGroupID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 2, NodeLogicalID: &nWARPInactive.LogicalID},
					{ID: "w4", GroupID: warpGroupID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 3, NodeLogicalID: &nWARPRisk.LogicalID},
				},
			},
		},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				// Global filter allows only VMess protocols
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpEquals, Value: "vmess"},
			},
		},
		AdmissionRules: []domain.AdmissionRule{
			{ID: "adm1", Name: "legacy-reject-vless", Expression: "protocol == 'vless'", Action: domain.RuleActionReject, Position: 0},
			{ID: "adm2", Name: "legacy-quarantine-warp", Expression: "name contains 'WARP'", Action: domain.RuleActionQuarantine, Position: 1},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: proxyGroupID, Expression: "MATCH", Position: 0},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// 1. Verify Normal Group (InheritGlobalFilter = true):
	// - Contains nHK (matches global filter)
	// - Does NOT contain nWARPActive (filtered by global filter)
	var proxyGroup *resolver.ResolvedGroup
	var warpGroup *resolver.ResolvedGroup
	for i := range snap.Groups {
		if snap.Groups[i].ID == proxyGroupID {
			proxyGroup = &snap.Groups[i]
		}
		if snap.Groups[i].ID == warpGroupID {
			warpGroup = &snap.Groups[i]
		}
	}
	if proxyGroup == nil || warpGroup == nil {
		t.Fatalf("expected both proxy and warp groups in snapshot")
	}

	if len(proxyGroup.Members) != 1 || proxyGroup.Members[0].TargetID != nHK.LogicalID {
		t.Fatalf("expected proxy group to only contain nHK, got: %#v", proxyGroup.Members)
	}

	// 2. Verify WARP Group (InheritGlobalFilter = false):
	// - Contains nHK
	// - Contains nWARPActive (exempt from global filter, and legacy admission reject is ignored!)
	// - Does NOT contain nWARPInactive (hard inactive constraint excluded at admission)
	// - Does NOT contain nWARPRisk (hard risk constraint excluded at admission)
	warpMemberIDs := make(map[string]bool)
	for _, m := range warpGroup.Members {
		warpMemberIDs[m.TargetID] = true
	}
	if !warpMemberIDs[nHK.LogicalID] {
		t.Errorf("expected WARP group to contain nHK")
	}
	if !warpMemberIDs[nWARPActive.LogicalID] {
		t.Errorf("expected WARP group to contain active nWARPActive (legacy reject rule must be ignored and global filter exempted)")
	}
	if warpMemberIDs[nWARPInactive.LogicalID] {
		t.Errorf("WARP group must NOT contain inactive node nWARPInactive")
	}
	if warpMemberIDs[nWARPRisk.LogicalID] {
		t.Errorf("WARP group must NOT contain risk-excluded node nWARPRisk")
	}
	if len(warpGroup.Members) != 2 {
		t.Fatalf("expected exactly 2 members in WARP group, got %d: %#v", len(warpGroup.Members), warpGroup.Members)
	}

	// 3. ExcludedNodeIDs must contain nWARPInactive and nWARPRisk
	excludedMap := make(map[string]bool)
	for _, id := range snap.ExcludedNodeIDs {
		excludedMap[id] = true
	}
	if !excludedMap[nWARPInactive.LogicalID] {
		t.Errorf("expected nWARPInactive in ExcludedNodeIDs")
	}
	if !excludedMap[nWARPRisk.LogicalID] {
		t.Errorf("expected nWARPRisk in ExcludedNodeIDs")
	}
	if excludedMap[nWARPActive.LogicalID] {
		t.Errorf("nWARPActive must NOT be in ExcludedNodeIDs")
	}

	// 4. Verify diagnostics
	var legacyDiagFound bool
	var legacyDiagCount int
	for _, d := range snap.Diagnostics {
		if d.Code == "legacy_admission_rules_ignored" {
			legacyDiagCount++
			legacyDiagFound = true
			if d.Severity != resolver.DiagnosticSeverityWarning {
				t.Errorf("expected warning severity for legacy_admission_rules_ignored, got: %s", d.Severity)
			}
			if strings.Contains(d.Message, "vless") || strings.Contains(d.Message, "WARP") {
				t.Errorf("legacy_admission_rules_ignored diagnostic message must NOT contain raw expression text: %s", d.Message)
			}
		}
	}
	if !legacyDiagFound || legacyDiagCount != 1 {
		t.Errorf("expected exactly 1 legacy_admission_rules_ignored diagnostic, got count: %d", legacyDiagCount)
	}
}

func TestDeterministicResolver_SubtreeNodeExpansion(t *testing.T) {
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
			{ID: parentID, Name: "PROXY", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true},
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
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

func TestResolver_DirectlyRoutedEmptyGroup_EmitsErrorRegardlessOfFilter(t *testing.T) {
	r := sourceFixtureResolver{}
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	unroutedID := domain.MustNewUUIDv7()

	input := resolver.ResolveInput{
		RevisionID:         "rev-old-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "EmptyRoutedGroup", GroupType: domain.GroupTypeSelect},
			{ID: unroutedID, Name: "EmptyUnroutedGroup", GroupType: domain.GroupTypeSelect},
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

	foundError := false
	foundWarning := false
	for _, d := range snap.Diagnostics {
		if d.Code == "empty_routed_group" && d.Severity == resolver.DiagnosticSeverityError && d.Target == parentID {
			foundError = true
		}
		if d.Code == "empty_group" && d.Severity == resolver.DiagnosticSeverityWarning && d.Target == unroutedID {
			foundWarning = true
		}
	}
	if !foundError {
		t.Fatalf("expected empty_routed_group error diagnostic for directly routed empty group regardless of filter, got %+v", snap.Diagnostics)
	}
	if !foundWarning {
		t.Fatalf("expected empty_group warning diagnostic for unrouted empty group, got %+v", snap.Diagnostics)
	}
}

func TestResolver_ProbeFailClosed_MissingExpiredMismatchNotEquals(t *testing.T) {
	r := sourceFixtureResolver{}
	ctx := context.Background()

	gid := domain.MustNewUUIDv7()
	nID := testNodeLogicalID("node-probe-test")
	rev := int64(1)
	node := domain.Node{
		LogicalID:          nID,
		DisplayName:        "Probe Test Node",
		Protocol:           domain.ProtocolTrojan,
		Active:             true,
		ConnectionRevision: rev,
	}

	freshness := 300
	kind := domain.ProbeKindBaseline
	now := time.Now().UTC()

	baseInput := func() resolver.ResolveInput {
		return resolver.ResolveInput{
			RevisionID:         "rev-p1",
			InventoryWatermark: "wm-1",
			CompilerVersion:    "1.0.0",
			Nodes:              []domain.Node{node},
			Groups:             []domain.NodeGroup{{ID: gid, Name: "G1", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true}},
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

	// 2. Ancient observation on same revision passes (redesigned age semantics)
	t.Run("ancient observation on same revision passes", func(t *testing.T) {
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
					ID:                 "obs-1",
					NodeLogicalID:      nID,
					Kind:               domain.ProbeKindBaseline,
					Verdict:            domain.VerdictAvailable,
					ConnectionRevision: &rev,
					ObservedAt:         expiredTime,
				},
			},
		}

		snap, err := r.Resolve(ctx, inp)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if len(snap.Nodes) != 1 {
			t.Fatalf("ancient observation on same revision should pass, got %d nodes", len(snap.Nodes))
		}
	})

	// 3. Future observation rejects
	t.Run("future observation rejects", func(t *testing.T) {
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
		futureTime := now.Add(10 * time.Minute)
		inp.LatestObservations = map[string]map[domain.ProbeKind]domain.ProbeObservation{
			nID: {
				domain.ProbeKindBaseline: {
					ID:                 "obs-future",
					NodeLogicalID:      nID,
					Kind:               domain.ProbeKindBaseline,
					Verdict:            domain.VerdictAvailable,
					ConnectionRevision: &rev,
					ObservedAt:         futureTime,
				},
			},
		}

		snap, err := r.Resolve(ctx, inp)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if len(snap.Nodes) != 0 {
			t.Fatalf("future observation should be rejected")
		}
	})

	// 4. Mismatched revision rejects
	t.Run("mismatched revision rejects", func(t *testing.T) {
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
		diffRev := int64(2)
		inp.LatestObservations = map[string]map[domain.ProbeKind]domain.ProbeObservation{
			nID: {
				domain.ProbeKindBaseline: {
					ID:                 "obs-mismatch",
					NodeLogicalID:      nID,
					Kind:               domain.ProbeKindBaseline,
					Verdict:            domain.VerdictAvailable,
					ConnectionRevision: &diffRev,
					ObservedAt:         now.Add(-50 * time.Second),
				},
			},
		}

		snap, err := r.Resolve(ctx, inp)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if len(snap.Nodes) != 0 {
			t.Fatalf("mismatched revision should be rejected")
		}
	})

	// 5. Missing revision on observation rejects
	t.Run("missing revision on observation rejects", func(t *testing.T) {
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
		inp.LatestObservations = map[string]map[domain.ProbeKind]domain.ProbeObservation{
			nID: {
				domain.ProbeKindBaseline: {
					ID:                 "obs-nil-rev",
					NodeLogicalID:      nID,
					Kind:               domain.ProbeKindBaseline,
					Verdict:            domain.VerdictAvailable,
					ConnectionRevision: nil,
					ObservedAt:         now.Add(-50 * time.Second),
				},
			},
		}

		snap, err := r.Resolve(ctx, inp)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if len(snap.Nodes) != 0 {
			t.Fatalf("observation with nil revision should be rejected")
		}
	})

	// 6. Valid observation passes
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
					ID:                 "obs-1",
					NodeLogicalID:      nID,
					Kind:               domain.ProbeKindBaseline,
					Verdict:            domain.VerdictAvailable,
					LatencyMS:          45,
					ConnectionRevision: &rev,
					ObservedAt:         validTime,
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
	r := sourceFixtureResolver{}
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
	r := sourceFixtureResolver{}
	ctx := context.Background()

	gid := domain.MustNewUUIDv7()
	nID := testNodeLogicalID("node-freshness-digest")
	node := domain.Node{
		LogicalID:   nID,
		DisplayName: "Digest Node",
		Protocol:    domain.ProtocolTrojan,
		Active:      true,
	}

	kind := domain.ProbeKindBaseline
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
						ID:            "obs-1",
						NodeLogicalID: nID,
						Kind:          kind,
						Verdict:       domain.VerdictAvailable,
						LatencyMS:     30,
						ObservedAt:    obsTime,
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

func TestResolver_InheritGlobalFilter_False_WARP_IncludesNodeInProxies(t *testing.T) {
	r := sourceFixtureResolver{}
	ctx := context.Background()

	warpGroupID := domain.MustNewUUIDv7()
	proxyGroupID := domain.MustNewUUIDv7()

	nSS := domain.Node{LogicalID: testNodeLogicalID("node-ss"), DisplayName: "Regular SS", Protocol: domain.ProtocolSS, Active: true}
	nWARP := domain.Node{LogicalID: testNodeLogicalID("node-warp"), DisplayName: "WARP-WireGuard", Protocol: domain.ProtocolWireGuard, Active: true}

	input := resolver.ResolveInput{
		RevisionID:         "rev-warp-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{nSS, nWARP},
		Groups: []domain.NodeGroup{
			{ID: proxyGroupID, Name: "PROXY", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true},
			{ID: warpGroupID, Name: "WARP", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: false},
		},
		Edges: map[string][]domain.GroupEdge{
			proxyGroupID: {{ID: "e1", ParentGroupID: proxyGroupID, NodeLogicalID: &nSS.LogicalID, Position: 0}},
			warpGroupID:  {{ID: "e2", ParentGroupID: warpGroupID, NodeLogicalID: &nWARP.LogicalID, Position: 0}},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: proxyGroupID, Expression: "MATCH", Position: 0},
			{ID: "r2", TargetGroupID: warpGroupID, Expression: "DOMAIN-SUFFIX,warp.dev", Position: 1},
		},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				// Only SS allowed by global filter; WireGuard node excluded
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpEquals, Value: "ss"},
			},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// 1. Verify WARP group successfully picked up nWARP
	var warpGroup *resolver.ResolvedGroup
	for i := range snap.Groups {
		if snap.Groups[i].ID == warpGroupID {
			warpGroup = &snap.Groups[i]
			break
		}
	}
	if warpGroup == nil {
		t.Fatalf("WARP group not found in snapshot")
	}
	if len(warpGroup.Members) != 1 || warpGroup.Members[0].TargetID != nWARP.LogicalID {
		t.Fatalf("expected WARP group to contain nWARP (%s), got %+v", nWARP.LogicalID, warpGroup.Members)
	}

	// 2. Verify snapshot.Nodes contains BOTH nSS and nWARP (for proxies in YAML)
	var foundSS, foundWARP bool
	for _, n := range snap.Nodes {
		if n.LogicalID == nSS.LogicalID {
			foundSS = true
		}
		if n.LogicalID == nWARP.LogicalID {
			foundWARP = true
		}
	}
	if !foundSS || !foundWARP {
		t.Fatalf("expected both nSS and nWARP in snapshot.Nodes, foundSS=%v foundWARP=%v (total %d nodes)", foundSS, foundWARP, len(snap.Nodes))
	}
}

func TestResolver_InheritedFilterGroupRef_PreservesChildNodesAndDerivedEmptyFallback(t *testing.T) {
	ctx := context.Background()
	r := resolver.New()

	nSS := domain.Node{
		LogicalID:   testNodeLogicalID("node-ss-valid"),
		DisplayName: "SS-01",
		Protocol:    domain.ProtocolSS,
		Server:      "198.51.100.10",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-256-gcm",
			Password: "ss-password",
		},
		Active: true,
	}
	nWARPHK := domain.Node{
		LogicalID:   testNodeLogicalID("node-warp-hk-valid"),
		DisplayName: "WARP-HK",
		Protocol:    domain.ProtocolWireGuard,
		Server:      "162.159.192.1",
		Port:        2408,
		Credentials: domain.InboundProtocolCredential{
			PrivateKey:   "aGVsbG8td29ybGQtdGVzdC1wcml2YXRlLWtleS0xMjM=",
			PublicKey:    "aGVsbG8td29ybGQtdGVzdC1wdWJsaWMta2V5LTEyMzQ=",
			LocalAddress: []string{"10.0.0.2/32"},
			MTU:          1420,
		},
		Active: true,
	}
	nWARPUS := domain.Node{
		LogicalID:   testNodeLogicalID("node-warp-us-valid"),
		DisplayName: "WARP-US",
		Protocol:    domain.ProtocolWireGuard,
		Server:      "162.159.192.2",
		Port:        2408,
		Credentials: domain.InboundProtocolCredential{
			PrivateKey:   "aGVsbG8td29ybGQtdGVzdC1wcml2YXRlLWtleS0xMjM=",
			PublicKey:    "aGVsbG8td29ybGQtdGVzdC1wdWJsaWMta2V5LTEyMzQ=",
			LocalAddress: []string{"10.0.0.3/32"},
			MTU:          1420,
		},
		Active: true,
	}

	parentGroupID := domain.MustNewUUIDv7()
	childGroupID := domain.MustNewUUIDv7()

	childSources := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 0, NodeLogicalID: &nWARPHK.LogicalID},
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 1, NodeLogicalID: &nWARPUS.LogicalID},
	}
	parentSources := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 0, NodeLogicalID: &nSS.LogicalID},
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupRef,
			Position:      1,
			TargetGroupID: &childGroupID,
			FilterSpec: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "HK"},
				},
			},
		},
	}

	input := resolver.ResolveInput{
		RevisionID:         "rev-warp-proj-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{nSS, nWARPHK, nWARPUS},
		Groups: []domain.NodeGroup{
			{
				ID:                  parentGroupID,
				Name:                "选择节点",
				GroupType:           domain.GroupTypeSelect,
				InheritGlobalFilter: true,
				Sources:             parentSources,
			},
			{
				ID:                  childGroupID,
				Name:                "WARP",
				GroupType:           domain.GroupTypeSelect,
				InheritGlobalFilter: false,
				EmptyFallbackPass:   true,
				Sources:             childSources,
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentGroupID, Expression: "MATCH", Position: 0},
		},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldProtocol, Op: domain.FilterOpEquals, Value: "ss"},
			},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// Verify derived group was created
	var derivedGroup *resolver.ResolvedGroup
	for i := range snap.Groups {
		if strings.Contains(snap.Groups[i].Name, "WARP [选择节点]") {
			derivedGroup = &snap.Groups[i]
			break
		}
	}
	if derivedGroup == nil {
		t.Fatalf("expected derived group 'WARP [选择节点]' to be created, groups: %+v", snap.Groups)
	}

	// Verify derived group inherited childGroup's settings and candidate selection
	if derivedGroup.InheritGlobalFilter {
		t.Errorf("expected derived group to have InheritGlobalFilter=false from child group")
	}
	if !derivedGroup.EmptyFallbackPass {
		t.Errorf("expected derived group to have EmptyFallbackPass=true from child group")
	}
	if len(derivedGroup.NodeLogicalIDs) != 1 || derivedGroup.NodeLogicalIDs[0] != nWARPHK.LogicalID {
		t.Fatalf("expected derived group to contain WARP-HK node (%s), got %+v", nWARPHK.LogicalID, derivedGroup.NodeLogicalIDs)
	}

	// Verify compiler succeeds for Mihomo (Clash)
	res, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("compiler.Compile for Mihomo failed: %v", err)
	}
	content := string(res.Content)
	if !strings.Contains(content, "WARP [选择节点]") {
		t.Errorf("expected compiled output to contain 'WARP [选择节点]', got:\n%s", content)
	}
	if !strings.Contains(content, "WARP-HK") {
		t.Errorf("expected compiled output to contain 'WARP-HK', got:\n%s", content)
	}

	// Sub-test: empty fallback degradation when filter matches 0 nodes
	parentSourcesEmpty := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 0, NodeLogicalID: &nSS.LogicalID},
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupRef,
			Position:      1,
			TargetGroupID: &childGroupID,
			FilterSpec: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT"},
				},
			},
		},
	}
	inputEmpty := input
	inputEmpty.Groups = []domain.NodeGroup{
		{
			ID:                  parentGroupID,
			Name:                "选择节点",
			GroupType:           domain.GroupTypeSelect,
			InheritGlobalFilter: true,
			Sources:             parentSourcesEmpty,
		},
		input.Groups[1],
	}

	snapEmpty, err := r.Resolve(ctx, inputEmpty)
	if err != nil {
		t.Fatalf("resolve with empty filter failed: %v", err)
	}

	var derivedEmptyGroup *resolver.ResolvedGroup
	for i := range snapEmpty.Groups {
		if strings.Contains(snapEmpty.Groups[i].Name, "WARP [选择节点]") {
			derivedEmptyGroup = &snapEmpty.Groups[i]
			break
		}
	}
	if derivedEmptyGroup == nil {
		t.Fatalf("expected derived empty group 'WARP [选择节点]' to be created")
	}
	if !derivedEmptyGroup.UsesEmptyPass() {
		t.Errorf("expected derived empty group to use EmptyPass, got UsesEmptyPass=false, EffectiveEmptyPass=%v, EmptyFallbackPass=%v",
			derivedEmptyGroup.EffectiveEmptyPass, derivedEmptyGroup.EmptyFallbackPass)
	}

	resEmpty, err := compiler.Compile(ctx, snapEmpty, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("compiler.Compile with empty derived group failed: %v", err)
	}
	contentEmpty := string(resEmpty.Content)
	if !strings.Contains(contentEmpty, "WARP [选择节点]") {
		t.Errorf("expected compiled output to contain 'WARP [选择节点]', got:\n%s", contentEmpty)
	}
}

func TestResolver_NonInheritingGroup_RemoveFilter_UsesActualCandidateScope(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	warpGroupID := domain.MustNewUUIDv7()

	nWG1 := domain.Node{LogicalID: testNodeLogicalID("node-wg-hk"), DisplayName: "WARP HK 01", Protocol: domain.ProtocolWireGuard, Active: true}
	nWG2 := domain.Node{LogicalID: testNodeLogicalID("node-wg-us"), DisplayName: "WARP US 02", Protocol: domain.ProtocolWireGuard, Active: true}
	nSS := domain.Node{LogicalID: testNodeLogicalID("node-ss-hk"), DisplayName: "SS HK 01", Protocol: domain.ProtocolSS, Active: true}

	// Global filter permits only SS protocol. WireGuard nodes fail global filter.
	globalFilter := &domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: domain.FilterFieldProtocol, Op: domain.FilterOpEquals, Value: string(domain.ProtocolSS)},
		},
	}

	// Group has InheritGlobalFilter = false, so it can select WireGuard nodes.
	// Step 1: Add all WARP nodes via regex "WARP.*" (adds nWG1 and nWG2).
	// Step 2: Remove filter with DisplayName contains "US" (must remove nWG2!).
	patWARP := "WARP.*"
	warpSources := []domain.MemberSource{
		{
			Operation: domain.OperationAdd,
			Kind:      domain.SourceKindRegex,
			Position:  0,
			Pattern:   &patWARP,
		},
		{
			Operation: domain.OperationRemove,
			Kind:      domain.SourceKindFilter,
			Position:  1,
			FilterSpec: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "US"},
				},
			},
		},
	}

	input := resolver.ResolveInput{
		RevisionID:         "rev-warp-remove",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{nSS, nWG1, nWG2},
		Groups: []domain.NodeGroup{
			{
				ID:                  warpGroupID,
				Name:                "WARP",
				GroupType:           domain.GroupTypeSelect,
				InheritGlobalFilter: false,
				Sources:             warpSources,
			},
		},
		GlobalFilter: globalFilter,
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1"},
		},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if len(snap.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(snap.Groups))
	}

	rg := snap.Groups[0]
	// nWG2 ("WARP US 02") must have been removed! Only nWG1 ("WARP HK 01") should remain!
	if len(rg.Members) != 1 {
		t.Fatalf("expected exactly 1 member in WARP group after remove filter, got %d: %+v", len(rg.Members), rg.Members)
	}
	if rg.Members[0].TargetID != nWG1.LogicalID {
		t.Errorf("expected member %s (WARP HK 01), got %s (%s)", nWG1.LogicalID, rg.Members[0].TargetID, rg.Members[0].DisplayName)
	}

	// Verify FilterCounts for the group
	if snap.FilterCounts == nil || snap.FilterCounts.GroupCounts == nil {
		t.Fatalf("expected group counts in snap.FilterCounts")
	}
	gc := snap.FilterCounts.GroupCounts[warpGroupID]
	if gc.Candidate != 3 {
		t.Errorf("expected 3 candidates evaluated, got %d", gc.Candidate)
	}
	if gc.Kept != 1 {
		t.Errorf("expected 1 kept, got %d", gc.Kept)
	}
	if gc.Excluded != 2 {
		t.Errorf("expected 2 excluded, got %d", gc.Excluded)
	}
	if len(gc.ExclusionSummary) == 0 {
		t.Errorf("expected non-empty ExclusionSummary in GroupFilterCount")
	}
}

func TestResolver_DynamicSources_CompoundFilter_And_Ordering(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	now := time.Now().UTC()
	grpID := domain.MustNewUUIDv7()

	n1 := domain.Node{LogicalID: testNodeLogicalID("n1"), DisplayName: "HK Hysteria Fast", Protocol: domain.ProtocolHysteria2, Active: true}
	n2 := domain.Node{LogicalID: testNodeLogicalID("n2"), DisplayName: "HK Hysteria Slow", Protocol: domain.ProtocolHysteria2, Active: true}
	n3 := domain.Node{LogicalID: testNodeLogicalID("n3"), DisplayName: "US VMess Fast", Protocol: domain.ProtocolVMess, Active: true}

	sub1 := domain.MustNewUUIDv7()
	sub2 := domain.MustNewUUIDv7()

	nodeSources := map[string][]domain.NodeSource{
		n1.LogicalID: {{SubscriptionID: sub1, NodeLogicalID: n1.LogicalID}},
		n2.LogicalID: {{SubscriptionID: sub2, NodeLogicalID: n2.LogicalID}},
		n3.LogicalID: {{SubscriptionID: sub1, NodeLogicalID: n3.LogicalID}},
	}
	latestObs := map[string]map[domain.ProbeKind]domain.ProbeObservation{
		n1.LogicalID: {
			domain.ProbeKindBaseline: {Verdict: domain.VerdictAvailable, LatencyMS: 50, ObservedAt: now},
		},
		n2.LogicalID: {
			domain.ProbeKindBaseline: {Verdict: domain.VerdictAvailable, LatencyMS: 300, ObservedAt: now},
		},
		n3.LogicalID: {
			domain.ProbeKindBaseline: {Verdict: domain.VerdictAvailable, LatencyMS: 60, ObservedAt: now},
		},
	}

	baselineProbe := domain.ProbeKindBaseline
	patHK := "HK.*"
	builtinDirect := "DIRECT"
	sources := []domain.MemberSource{
		// 1. Add HK nodes via regex
		{Operation: domain.OperationAdd, Kind: domain.SourceKindRegex, Position: 0, Pattern: &patHK},
		// 2. Add via compound filter: Protocol == hysteria2 AND probe verdict == available AND latency <= 100
		{
			Operation: domain.OperationAdd,
			Kind:      domain.SourceKindFilter,
			Position:  1,
			FilterSpec: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldProtocol, Op: domain.FilterOpEquals, Value: string(domain.ProtocolHysteria2)},
					{Field: domain.FilterFieldProbeVerdict, Op: domain.FilterOpEquals, Value: string(domain.VerdictAvailable), ProbeKind: &baselineProbe},
					{Field: domain.FilterFieldProbeLatencyMS, Op: domain.FilterOpLTE, Value: "100", ProbeKind: &baselineProbe},
				},
			},
		},
		// 3. Add DIRECT builtin
		{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, Position: 2, BuiltinName: &builtinDirect},
		// 4. Remove nodes from sub-2 (which removes n2!)
		{
			Operation: domain.OperationRemove,
			Kind:      domain.SourceKindFilter,
			Position:  3,
			FilterSpec: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldSourceSubscriptions, Op: domain.FilterOpContains, Value: sub2},
				},
			},
		},
	}

	input := resolver.ResolveInput{
		RevisionID:         "rev-dyn-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{n1, n2, n3},
		Groups: []domain.NodeGroup{
			{ID: grpID, Name: "DYNAMIC_PROXY", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true, Sources: sources},
		},
		NodeSources:        nodeSources,
		LatestObservations: latestObs,
		AsOf:               now,
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1"},
		},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	rg := snap.Groups[0]
	// Expected members: n1 ("HK Hysteria Fast") and DIRECT builtin! n2 is removed by sub-2 filter, n3 was never added.
	if len(rg.Members) != 2 {
		t.Fatalf("expected 2 members (n1 and DIRECT), got %d: %+v", len(rg.Members), rg.Members)
	}
	if rg.Members[0].Kind != resolver.MemberKindNode || rg.Members[0].TargetID != n1.LogicalID {
		t.Errorf("expected member 0 to be n1, got %+v", rg.Members[0])
	}
	if rg.Members[1].Kind != resolver.MemberKindBuiltin || rg.Members[1].TargetID != "DIRECT" {
		t.Errorf("expected member 1 to be DIRECT builtin, got %+v", rg.Members[1])
	}
}

func TestResolver_ChildGroup_Penetration_NonInheriting(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()

	nSS := domain.Node{LogicalID: testNodeLogicalID("n-ss"), DisplayName: "Regular SS", Protocol: domain.ProtocolSS, Active: true}
	nWARP1 := domain.Node{LogicalID: testNodeLogicalID("n-warp1"), DisplayName: "WARP HK Fast", Protocol: domain.ProtocolWireGuard, Active: true}
	nWARP2 := domain.Node{LogicalID: testNodeLogicalID("n-warp2"), DisplayName: "WARP US Slow", Protocol: domain.ProtocolWireGuard, Active: true}

	// Global filter permits only SS protocol
	globalFilter := &domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: domain.FilterFieldProtocol, Op: domain.FilterOpEquals, Value: string(domain.ProtocolSS)},
		},
	}

	patWARP := "WARP.*"
	childSources := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindRegex, Position: 0, Pattern: &patWARP},
	}

	// Parent has InheritGlobalFilter = true, references child group (which has InheritGlobalFilter = false)
	// with an item filter: DisplayName contains "HK"
	parentSources := []domain.MemberSource{
		{
			Operation:     domain.OperationAdd,
			Kind:          domain.SourceKindGroupRef,
			Position:      0,
			TargetGroupID: &childID,
			FilterSpec: &domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "HK"},
				},
			},
		},
	}

	input := resolver.ResolveInput{
		RevisionID:         "rev-pen-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{nSS, nWARP1, nWARP2},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "PARENT", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true, Sources: parentSources},
			{ID: childID, Name: "WARP_CHILD", GroupType: domain.GroupTypeURLTest, InheritGlobalFilter: false, Sources: childSources},
		},
		GlobalFilter: globalFilter,
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1"},
		},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// Should create a derived group for child under parent
	var derivedGroup *resolver.ResolvedGroup
	for i := range snap.Groups {
		if strings.Contains(snap.Groups[i].Name, "WARP_CHILD [PARENT]") {
			derivedGroup = &snap.Groups[i]
			break
		}
	}
	if derivedGroup == nil {
		t.Fatalf("expected derived group 'WARP_CHILD [PARENT]' to be created")
	}

	// The derived group should contain nWARP1 ("WARP HK Fast") and NOT nWARP2 ("WARP US Slow")
	if len(derivedGroup.NodeLogicalIDs) != 1 || derivedGroup.NodeLogicalIDs[0] != nWARP1.LogicalID {
		t.Errorf("expected derived group to contain exactly nWARP1 (%s), got %+v", nWARP1.LogicalID, derivedGroup.NodeLogicalIDs)
	}

	// The parent group should reference the derived group
	var parentRG *resolver.ResolvedGroup
	for i := range snap.Groups {
		if snap.Groups[i].ID == parentID {
			parentRG = &snap.Groups[i]
			break
		}
	}
	if parentRG == nil {
		t.Fatalf("parent group not found in snapshot")
	}
	if len(parentRG.Members) != 1 || parentRG.Members[0].TargetID != derivedGroup.ID {
		t.Errorf("expected parent to reference derived group %s, got %+v", derivedGroup.ID, parentRG.Members)
	}
	// And parent's AllNodeLogicalIDs must include nWARP1!
	if len(parentRG.AllNodeLogicalIDs) != 1 || parentRG.AllNodeLogicalIDs[0] != nWARP1.LogicalID {
		t.Errorf("expected parent AllNodeLogicalIDs to contain nWARP1, got %+v", parentRG.AllNodeLogicalIDs)
	}
}

func TestResolver_SoftFilter_NotTopologyError(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	gID := domain.MustNewUUIDv7()
	n1 := domain.Node{LogicalID: testNodeLogicalID("n1"), DisplayName: "HK 01", Protocol: domain.ProtocolSS, Active: true}
	n2 := domain.Node{LogicalID: testNodeLogicalID("n2"), DisplayName: "US 02", Protocol: domain.ProtocolSS, Active: true}

	patHK := "HK.*"
	sources := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindRegex, Position: 0, Pattern: &patHK},
	}

	groups := []domain.NodeGroup{
		{ID: gID, Name: "PROXY", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true, Sources: sources},
	}

	// Topology validation should succeed cleanly
	if err := domain.ValidatePolicyTopology(groups, nil, nil, false); err != nil {
		t.Fatalf("expected topology to be valid, got: %v", err)
	}

	input := resolver.ResolveInput{
		RevisionID:         "rev-soft-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{n1, n2},
		Groups:             groups,
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1"},
		},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// No error diagnostics should be emitted for soft filtering
	for _, d := range snap.Diagnostics {
		if d.Severity == resolver.DiagnosticSeverityError {
			t.Errorf("unexpected error diagnostic: %+v", d)
		}
	}

	// FilterCounts should record candidate: 2, kept: 1, excluded: 1
	gc := snap.FilterCounts.GroupCounts[gID]
	if gc.Candidate != 2 || gc.Kept != 1 || gc.Excluded != 1 {
		t.Errorf("expected counts {Candidate: 2, Kept: 1, Excluded: 1}, got %+v", gc)
	}
}

func TestResolver_Pipeline_EdgeCases_AddRemoveReentrantDuplicatesSubgroupBuiltinMissing(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	now := time.Now().UTC()
	g1ID := domain.MustNewUUIDv7()
	c1ID := domain.MustNewUUIDv7()
	g2ID := domain.MustNewUUIDv7()

	n1 := domain.Node{LogicalID: testNodeLogicalID("n1-hk"), DisplayName: "HK Alpha", Protocol: domain.ProtocolHysteria2, Active: true}
	n2 := domain.Node{LogicalID: testNodeLogicalID("n2-us"), DisplayName: "US Beta", Protocol: domain.ProtocolVMess, Active: true}
	n3 := domain.Node{LogicalID: testNodeLogicalID("n3-jp"), DisplayName: "JP Gamma", Protocol: domain.ProtocolSS, Active: true}
	missingNodeID := testNodeLogicalID("missing-node-id")

	// Global filter: exclude ProtocolSS (n3)
	globalFilter := &domain.NodeFilterSpec{
		Conditions: []domain.FilterCondition{
			{Field: domain.FilterFieldProtocol, Op: domain.FilterOpNotEquals, Value: string(domain.ProtocolSS)},
		},
	}

	builtinDirect := "DIRECT"
	c1Sources := []domain.MemberSource{
		{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, Position: 0, BuiltinName: &builtinDirect},
	}

	g1Sources := []domain.MemberSource{
		// 1. Add n1
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 0, NodeLogicalID: &n1.LogicalID},
		// 2. Remove n1
		{Operation: domain.OperationRemove, Kind: domain.SourceKindManualNode, Position: 1, NodeLogicalID: &n1.LogicalID},
		// 3. Re-add n1 (re-entrancy)
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 2, NodeLogicalID: &n1.LogicalID},
		// 4. Duplicate add n1
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 3, NodeLogicalID: &n1.LogicalID},
		// 5. Add n3 (excluded by global filter)
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 4, NodeLogicalID: &n3.LogicalID},
		// 6. Add missing node
		{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, Position: 5, NodeLogicalID: &missingNodeID},
		// 7. Add subgroup reference C1 (which has DIRECT builtin)
		{Operation: domain.OperationAdd, Kind: domain.SourceKindGroupRef, Position: 6, TargetGroupID: &c1ID},
	}

	patHK := ".*HK.*"
	g2Sources := []domain.MemberSource{
		// Add group_nodes from C2 (which has n1 and n2), filtered by regex HK
		{Operation: domain.OperationAdd, Kind: domain.SourceKindGroupNodes, Position: 0, TargetGroupID: &g1ID, Pattern: &patHK},
	}

	input := resolver.ResolveInput{
		RevisionID:         "rev-edge-cases",
		InventoryWatermark: "wm-edge",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{n1, n2, n3},
		Groups: []domain.NodeGroup{
			{ID: c1ID, Name: "CHILD_BUILTIN", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true, Sources: c1Sources},
			{ID: g1ID, Name: "PARENT_PIPELINE", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true, EmptyFallbackPass: true, Sources: g1Sources},
			{ID: g2ID, Name: "GROUP_NODES_TEST", GroupType: domain.GroupTypeSelect, InheritGlobalFilter: true, Sources: g2Sources},
		},
		GlobalFilter: globalFilter,
		AsOf:         now,
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1"},
		},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// 1. Verify G1 counts
	if snap.FilterCounts == nil || snap.FilterCounts.GroupCounts == nil {
		t.Fatalf("expected FilterCounts in snapshot")
	}
	gc1, ok := snap.FilterCounts.GroupCounts[g1ID]
	if !ok {
		t.Fatalf("group G1 missing from FilterCounts")
	}

	// Candidates evaluated: n1 (valid), n3 (excluded by global), missingNode (missing) = 3 unique node IDs
	if gc1.Candidate != 3 {
		t.Errorf("expected G1 Candidate=3, got %d", gc1.Candidate)
	}
	// Kept: n1 = 1 node
	if gc1.Kept != 1 {
		t.Errorf("expected G1 Kept=1, got %d", gc1.Kept)
	}
	// Excluded: Candidate - Kept = 2
	if gc1.Excluded != 2 {
		t.Errorf("expected G1 Excluded=2, got %d", gc1.Excluded)
	}

	// Check exclusion reasons in G1
	if gc1.ExclusionSummary["manually removed"] != 1 {
		t.Errorf("expected 1 'manually removed', got %d", gc1.ExclusionSummary["manually removed"])
	}
	if gc1.ExclusionSummary["excluded by global filter"] != 1 {
		t.Errorf("expected 1 'excluded by global filter', got %d", gc1.ExclusionSummary["excluded by global filter"])
	}
	if gc1.ExclusionSummary["node inactive or missing"] != 1 {
		t.Errorf("expected 1 'node inactive or missing', got %d", gc1.ExclusionSummary["node inactive or missing"])
	}

	// G1 resolved members: n1 and C1 -> 2 members
	var g1RG *resolver.ResolvedGroup
	for i := range snap.Groups {
		if snap.Groups[i].ID == g1ID {
			g1RG = &snap.Groups[i]
			break
		}
	}
	if g1RG == nil {
		t.Fatalf("G1 not found in resolved snapshot")
	}
	if len(g1RG.Members) != 2 {
		t.Fatalf("expected G1 to have 2 members (n1 + C1), got %d: %+v", len(g1RG.Members), g1RG.Members)
	}
	// Destination check: since C1 has DIRECT builtin, G1 has destinations, so EffectiveEmptyPass must be false
	if g1RG.EffectiveEmptyPass {
		t.Errorf("expected EffectiveEmptyPass to be false for G1 because it has destinations (n1 and DIRECT)")
	}

	// 2. Verify G2 counts (unpacks G1 which has n1, filtered by regex patHK)
	gc2, ok := snap.FilterCounts.GroupCounts[g2ID]
	if !ok {
		t.Fatalf("group G2 missing from FilterCounts")
	}
	if gc2.Candidate != 1 || gc2.Kept != 1 || gc2.Excluded != 0 {
		t.Errorf("expected G2 {Candidate: 1, Kept: 1, Excluded: 0}, got %+v", gc2)
	}
}
