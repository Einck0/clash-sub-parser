package resolver_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func TestResolveChains_SyntheticSubscriptionAndSpringboardGroup_OverlapSkippedAndChained(t *testing.T) {
	nodeAID := testNodeLogicalID("node-a")
	nodeBID := testNodeLogicalID("node-b")
	subID := "sub-main"
	springboardGroupID := domain.MustNewUUIDv7()

	nodes := []resolver.ResolvedNode{
		{LogicalID: nodeAID, DisplayName: "Node A", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: nodeBID, DisplayName: "Node B", Protocol: domain.ProtocolTrojan, Active: true},
	}

	groups := []resolver.ResolvedGroup{
		{
			ID:                springboardGroupID,
			Name:              "Springboard Group",
			GroupType:         domain.GroupTypeSelect,
			AllNodeLogicalIDs: []string{nodeAID},
		},
	}

	nodeSources := map[string][]domain.NodeSource{
		nodeAID: {{NodeLogicalID: nodeAID, SubscriptionID: subID}},
		nodeBID: {{NodeLogicalID: nodeBID, SubscriptionID: subID}},
	}

	bindings := []domain.ProxyChainBinding{
		{
			ID:         "bind-sub",
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   subID,
			DialerType: domain.ProxyChainDialerNodeGroup,
			DialerID:   springboardGroupID,
			Enabled:    true,
			SortOrder:  0,
			Note:       "sub to springboard",
			Revision:   1,
		},
	}

	resNodes, effects, err := resolver.ResolveChains(nodes, groups, nodeSources, bindings)
	if err != nil {
		t.Fatalf("unexpected ResolveChains error: %v", err)
	}

	if len(effects) != 1 {
		t.Fatalf("expected 1 effect, got %d", len(effects))
	}

	eff := effects[0]
	if eff.BindingID != "bind-sub" {
		t.Errorf("expected binding_id 'bind-sub', got %q", eff.BindingID)
	}
	if eff.TargetCount != 2 {
		t.Errorf("expected target_count 2, got %d", eff.TargetCount)
	}
	if eff.SkipCount != 1 {
		t.Errorf("expected skip_count 1, got %d", eff.SkipCount)
	}
	if eff.ChainCount != 1 {
		t.Errorf("expected chain_count 1, got %d", eff.ChainCount)
	}
	if eff.OverriddenCount != 0 {
		t.Errorf("expected overridden_count 0, got %d", eff.OverriddenCount)
	}
	if len(eff.SkipSamples) != 1 || eff.SkipSamples[0] != "Node A" {
		t.Errorf("expected skip_samples ['Node A'], got %+v", eff.SkipSamples)
	}
	if len(eff.ChainSamples) != 1 || eff.ChainSamples[0] != "Node B" {
		t.Errorf("expected chain_samples ['Node B'], got %+v", eff.ChainSamples)
	}

	// Verify node fields: Node A unchanged, Node B chained
	var foundA, foundB resolver.ResolvedNode
	for _, n := range resNodes {
		if n.LogicalID == nodeAID {
			foundA = n
		}
		if n.LogicalID == nodeBID {
			foundB = n
		}
	}

	if foundA.DialerProxy != "" {
		t.Errorf("expected Node A DialerProxy empty, got %q", foundA.DialerProxy)
	}
	if foundA.ChainBindingID != "" {
		t.Errorf("expected Node A ChainBindingID empty, got %q", foundA.ChainBindingID)
	}

	if foundB.DialerProxy != "Springboard Group" {
		t.Errorf("expected Node B DialerProxy 'Springboard Group', got %q", foundB.DialerProxy)
	}
	if foundB.ChainBindingID != "bind-sub" {
		t.Errorf("expected Node B ChainBindingID 'bind-sub', got %q", foundB.ChainBindingID)
	}
	if foundB.ChainSource != "subscription" {
		t.Errorf("expected Node B ChainSource 'subscription', got %q", foundB.ChainSource)
	}
}

func TestResolveChains_PriorityHierarchy_NodeOverridesGroupOverridesSubscription(t *testing.T) {
	nodeAID := testNodeLogicalID("node-a")
	nodeBID := testNodeLogicalID("node-b")
	nodeCID := testNodeLogicalID("node-c")
	dialerNodeID := testNodeLogicalID("dialer-node")

	subID := "sub-all"
	groupID := domain.MustNewUUIDv7()
	dialerGroupID := domain.MustNewUUIDv7()

	nodes := []resolver.ResolvedNode{
		{LogicalID: nodeAID, DisplayName: "Node A", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: nodeBID, DisplayName: "Node B", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: nodeCID, DisplayName: "Node C", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: dialerNodeID, DisplayName: "Dialer Node", Protocol: domain.ProtocolTrojan, Active: true},
	}

	groups := []resolver.ResolvedGroup{
		{
			ID:                groupID,
			Name:              "Target Group",
			GroupType:         domain.GroupTypeSelect,
			AllNodeLogicalIDs: []string{nodeAID, nodeBID},
		},
		{
			ID:                dialerGroupID,
			Name:              "Dialer Group",
			GroupType:         domain.GroupTypeSelect,
			AllNodeLogicalIDs: []string{dialerNodeID},
		},
	}

	nodeSources := map[string][]domain.NodeSource{
		nodeAID: {{NodeLogicalID: nodeAID, SubscriptionID: subID}},
		nodeBID: {{NodeLogicalID: nodeBID, SubscriptionID: subID}},
		nodeCID: {{NodeLogicalID: nodeCID, SubscriptionID: subID}},
	}

	// 3 bindings:
	// b_sub targets all 3 via subscription -> dials dialerGroupID
	// b_grp targets A and B via group -> dials dialerNodeID
	// b_node targets A via node -> dials dialerGroupID
	bindings := []domain.ProxyChainBinding{
		{
			ID:         "b-sub",
			TargetType: domain.ProxyChainTargetSubscription,
			TargetID:   subID,
			DialerType: domain.ProxyChainDialerNodeGroup,
			DialerID:   dialerGroupID,
			Enabled:    true,
			SortOrder:  0,
		},
		{
			ID:         "b-grp",
			TargetType: domain.ProxyChainTargetNodeGroup,
			TargetID:   groupID,
			DialerType: domain.ProxyChainDialerNode,
			DialerID:   dialerNodeID,
			Enabled:    true,
			SortOrder:  0,
		},
		{
			ID:         "b-node",
			TargetType: domain.ProxyChainTargetNode,
			TargetID:   nodeAID,
			DialerType: domain.ProxyChainDialerNodeGroup,
			DialerID:   dialerGroupID,
			Enabled:    true,
			SortOrder:  0,
		},
	}

	resNodes, effects, err := resolver.ResolveChains(nodes, groups, nodeSources, bindings)
	if err != nil {
		t.Fatalf("unexpected ResolveChains error: %v", err)
	}

	effectMap := make(map[string]resolver.ChainEffect)
	for _, eff := range effects {
		effectMap[eff.BindingID] = eff
	}

	// b-node: targets A, wins A
	if eff := effectMap["b-node"]; eff.ChainCount != 1 || eff.OverriddenCount != 0 {
		t.Errorf("b-node unexpected effect: %+v", eff)
	}
	// b-grp: targets A, B; A overridden by b-node, wins B
	if eff := effectMap["b-grp"]; eff.ChainCount != 1 || eff.OverriddenCount != 1 {
		t.Errorf("b-grp unexpected effect: %+v", eff)
	}
	// b-sub: targets A, B, C; A overridden by b-node, B overridden by b-grp, wins C
	if eff := effectMap["b-sub"]; eff.ChainCount != 1 || eff.OverriddenCount != 2 {
		t.Errorf("b-sub unexpected effect: %+v", eff)
	}

	nodeMap := make(map[string]resolver.ResolvedNode)
	for _, n := range resNodes {
		nodeMap[n.LogicalID] = n
	}

	// Node A won by b-node (dialer: "Dialer Group", source: "node")
	if nA := nodeMap[nodeAID]; nA.ChainBindingID != "b-node" || nA.DialerProxy != "Dialer Group" || nA.ChainSource != "node" {
		t.Errorf("Node A unexpected chain: %+v", nA)
	}
	// Node B won by b-grp (dialer: "Dialer Node", source: "node_group")
	if nB := nodeMap[nodeBID]; nB.ChainBindingID != "b-grp" || nB.DialerProxy != "Dialer Node" || nB.ChainSource != "node_group" {
		t.Errorf("Node B unexpected chain: %+v", nB)
	}
	// Node C won by b-sub (dialer: "Dialer Group", source: "subscription")
	if nC := nodeMap[nodeCID]; nC.ChainBindingID != "b-sub" || nC.DialerProxy != "Dialer Group" || nC.ChainSource != "subscription" {
		t.Errorf("Node C unexpected chain: %+v", nC)
	}
}

func TestResolveChains_SameTier_SortOrderAndIDPriority(t *testing.T) {
	nodeAID := testNodeLogicalID("node-a")
	dialer1ID := testNodeLogicalID("dialer-1")
	dialer2ID := testNodeLogicalID("dialer-2")

	nodes := []resolver.ResolvedNode{
		{LogicalID: nodeAID, DisplayName: "Node A", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: dialer1ID, DisplayName: "Dialer 1", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: dialer2ID, DisplayName: "Dialer 2", Protocol: domain.ProtocolTrojan, Active: true},
	}

	// Two node bindings for node A: b1 has sort_order 10, b2 has sort_order 5
	b1 := domain.ProxyChainBinding{
		ID:         "b1",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   nodeAID,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   dialer1ID,
		Enabled:    true,
		SortOrder:  10,
	}
	b2 := domain.ProxyChainBinding{
		ID:         "b2",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   nodeAID,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   dialer2ID,
		Enabled:    true,
		SortOrder:  5,
	}

	resNodes, _, err := resolver.ResolveChains(nodes, nil, nil, []domain.ProxyChainBinding{b1, b2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var nodeA resolver.ResolvedNode
	for _, n := range resNodes {
		if n.LogicalID == nodeAID {
			nodeA = n
		}
	}
	if nodeA.ChainBindingID != "b2" || nodeA.DialerProxy != "Dialer 2" {
		t.Errorf("expected b2 (sort_order 5) to win, got: %+v", nodeA)
	}

	// When sort_order is identical, lexicographical ID wins ("b-alpha" vs "b-beta")
	bAlpha := domain.ProxyChainBinding{
		ID:         "b-alpha",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   nodeAID,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   dialer1ID,
		Enabled:    true,
		SortOrder:  5,
	}
	bBeta := domain.ProxyChainBinding{
		ID:         "b-beta",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   nodeAID,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   dialer2ID,
		Enabled:    true,
		SortOrder:  5,
	}

	resNodes2, _, err := resolver.ResolveChains(nodes, nil, nil, []domain.ProxyChainBinding{bBeta, bAlpha})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, n := range resNodes2 {
		if n.LogicalID == nodeAID {
			if n.ChainBindingID != "b-alpha" || n.DialerProxy != "Dialer 1" {
				t.Errorf("expected b-alpha to win by ID order, got: %+v", n)
			}
		}
	}
}

func TestResolveChains_DisablingBindingRestoresLowerPriority(t *testing.T) {
	nodeAID := testNodeLogicalID("node-a")
	dialer1ID := testNodeLogicalID("dialer-1")
	dialer2ID := testNodeLogicalID("dialer-2")

	nodes := []resolver.ResolvedNode{
		{LogicalID: nodeAID, DisplayName: "Node A", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: dialer1ID, DisplayName: "Dialer 1", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: dialer2ID, DisplayName: "Dialer 2", Protocol: domain.ProtocolTrojan, Active: true},
	}

	// b-high is disabled, b-low is enabled
	bHigh := domain.ProxyChainBinding{
		ID:         "b-high",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   nodeAID,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   dialer1ID,
		Enabled:    false, // DISABLED!
		SortOrder:  1,
	}
	bLow := domain.ProxyChainBinding{
		ID:         "b-low",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   nodeAID,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   dialer2ID,
		Enabled:    true,
		SortOrder:  10,
	}

	resNodes, effects, err := resolver.ResolveChains(nodes, nil, nil, []domain.ProxyChainBinding{bHigh, bLow})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// bHigh is disabled, does not participate in effects
	if len(effects) != 1 || effects[0].BindingID != "b-low" {
		t.Fatalf("expected 1 effect for b-low, got %+v", effects)
	}

	for _, n := range resNodes {
		if n.LogicalID == nodeAID {
			if n.ChainBindingID != "b-low" || n.DialerProxy != "Dialer 2" {
				t.Errorf("expected b-low to win when b-high is disabled, got: %+v", n)
			}
		}
	}
}

func TestResolveChains_DirectSelfReferences_FailWithCycle(t *testing.T) {
	nodeAID := testNodeLogicalID("node-a")
	nodes := []resolver.ResolvedNode{
		{LogicalID: nodeAID, DisplayName: "Node A", Protocol: domain.ProtocolTrojan, Active: true},
	}

	// Node self-reference
	bSelfNode := domain.ProxyChainBinding{
		ID:         "b-self-node",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   nodeAID,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   nodeAID,
		Enabled:    true,
	}

	_, _, err := resolver.ResolveChains(nodes, nil, nil, []domain.ProxyChainBinding{bSelfNode})
	if err == nil {
		t.Fatal("expected chain_cycle error on direct node self-reference")
	}
	if domErr, ok := domain.AsDomainError(err); !ok || domErr.Code != "chain_cycle" {
		t.Fatalf("expected code chain_cycle, got: %v", err)
	}

	// Group self-reference
	grpID := domain.MustNewUUIDv7()
	groups := []resolver.ResolvedGroup{
		{ID: grpID, Name: "Group 1", GroupType: domain.GroupTypeSelect, AllNodeLogicalIDs: []string{nodeAID}},
	}
	bSelfGroup := domain.ProxyChainBinding{
		ID:         "b-self-grp",
		TargetType: domain.ProxyChainTargetNodeGroup,
		TargetID:   grpID,
		DialerType: domain.ProxyChainDialerNodeGroup,
		DialerID:   grpID,
		Enabled:    true,
	}

	_, _, err = resolver.ResolveChains(nodes, groups, nil, []domain.ProxyChainBinding{bSelfGroup})
	if err == nil {
		t.Fatal("expected chain_cycle error on direct group self-reference")
	}
	if domErr, ok := domain.AsDomainError(err); !ok || domErr.Code != "chain_cycle" {
		t.Fatalf("expected code chain_cycle, got: %v", err)
	}
}

func TestResolveChains_TransitiveCycles_FailWithCycle(t *testing.T) {
	node1 := testNodeLogicalID("node-1")
	node2 := testNodeLogicalID("node-2")
	node3 := testNodeLogicalID("node-3")

	nodes := []resolver.ResolvedNode{
		{LogicalID: node1, DisplayName: "N1", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: node2, DisplayName: "N2", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: node3, DisplayName: "N3", Protocol: domain.ProtocolTrojan, Active: true},
	}

	// 1. Pure node cycle: 1 -> 2 -> 3 -> 1
	bindings := []domain.ProxyChainBinding{
		{ID: "b1", TargetType: domain.ProxyChainTargetNode, TargetID: node1, DialerType: domain.ProxyChainDialerNode, DialerID: node2, Enabled: true},
		{ID: "b2", TargetType: domain.ProxyChainTargetNode, TargetID: node2, DialerType: domain.ProxyChainDialerNode, DialerID: node3, Enabled: true},
		{ID: "b3", TargetType: domain.ProxyChainTargetNode, TargetID: node3, DialerType: domain.ProxyChainDialerNode, DialerID: node1, Enabled: true},
	}

	_, _, err := resolver.ResolveChains(nodes, nil, nil, bindings)
	if err == nil {
		t.Fatal("expected transitive node cycle error")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_cycle" {
		t.Fatalf("expected chain_cycle error, got: %v", err)
	}
	if len(domErr.CyclePath) == 0 {
		t.Errorf("expected non-empty CyclePath, got %+v", domErr.CyclePath)
	}

	// 2. Node-Group mixed cycle:
	// Group G has leaf Node 2.
	// Node 1 dials Group G (so 1 -> G -> 2).
	// Node 2 dials Node 1 (so 2 -> 1).
	grpID := domain.MustNewUUIDv7()
	groups := []resolver.ResolvedGroup{
		{ID: grpID, Name: "G", GroupType: domain.GroupTypeSelect, AllNodeLogicalIDs: []string{node2}},
	}
	mixedBindings := []domain.ProxyChainBinding{
		{ID: "b-m1", TargetType: domain.ProxyChainTargetNode, TargetID: node1, DialerType: domain.ProxyChainDialerNodeGroup, DialerID: grpID, Enabled: true},
		{ID: "b-m2", TargetType: domain.ProxyChainTargetNode, TargetID: node2, DialerType: domain.ProxyChainDialerNode, DialerID: node1, Enabled: true},
	}

	_, _, err = resolver.ResolveChains(nodes, groups, nil, mixedBindings)
	if err == nil {
		t.Fatal("expected mixed node-group cycle error")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_cycle" {
		t.Fatalf("expected chain_cycle error, got: %v", err)
	}
	if len(domErr.CyclePath) == 0 {
		t.Errorf("expected non-empty CyclePath, got %+v", domErr.CyclePath)
	}
}

func TestResolveChains_AllTargetsOverlap_Fails(t *testing.T) {
	node1 := testNodeLogicalID("node-1")
	node2 := testNodeLogicalID("node-2")

	nodes := []resolver.ResolvedNode{
		{LogicalID: node1, DisplayName: "N1", Protocol: domain.ProtocolTrojan, Active: true},
		{LogicalID: node2, DisplayName: "N2", Protocol: domain.ProtocolTrojan, Active: true},
	}

	grpTargetID := domain.MustNewUUIDv7()
	grpDialerID := domain.MustNewUUIDv7()

	groups := []resolver.ResolvedGroup{
		// Target group has N1
		{ID: grpTargetID, Name: "Target Group", GroupType: domain.GroupTypeSelect, AllNodeLogicalIDs: []string{node1}},
		// Dialer group has N1 and N2 (so all target nodes overlap!)
		{ID: grpDialerID, Name: "Dialer Group", GroupType: domain.GroupTypeSelect, AllNodeLogicalIDs: []string{node1, node2}},
	}

	binding := domain.ProxyChainBinding{
		ID:         "b-overlap",
		TargetType: domain.ProxyChainTargetNodeGroup,
		TargetID:   grpTargetID,
		DialerType: domain.ProxyChainDialerNodeGroup,
		DialerID:   grpDialerID,
		Enabled:    true,
	}

	_, _, err := resolver.ResolveChains(nodes, groups, nil, []domain.ProxyChainBinding{binding})
	if err == nil {
		t.Fatal("expected chain_all_targets_overlap error")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "chain_all_targets_overlap" {
		t.Fatalf("expected chain_all_targets_overlap code, got: %v", err)
	}
}

func TestResolveChains_MissingReferences_FailEnabled_AllowedDisabled(t *testing.T) {
	node1 := testNodeLogicalID("node-1")
	nodes := []resolver.ResolvedNode{
		{LogicalID: node1, DisplayName: "N1", Protocol: domain.ProtocolTrojan, Active: true},
	}

	// 1. Missing target node when enabled
	bMissingTarget := domain.ProxyChainBinding{
		ID:         "b-missing-tgt",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   "missing-node-id",
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   node1,
		Enabled:    true,
	}
	_, _, err := resolver.ResolveChains(nodes, nil, nil, []domain.ProxyChainBinding{bMissingTarget})
	if err == nil {
		t.Fatal("expected chain_reference_missing for missing target node")
	}
	if domErr, ok := domain.AsDomainError(err); !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing code, got: %v", err)
	}

	// When disabled, missing target is ALLOWED
	bMissingTargetDisabled := bMissingTarget
	bMissingTargetDisabled.Enabled = false
	_, effects, err := resolver.ResolveChains(nodes, nil, nil, []domain.ProxyChainBinding{bMissingTargetDisabled})
	if err != nil {
		t.Fatalf("expected no error for disabled missing target, got: %v", err)
	}
	if len(effects) != 0 {
		t.Errorf("expected 0 effects for disabled binding, got %d", len(effects))
	}

	// 2. Missing dialer node when enabled
	bMissingDialer := domain.ProxyChainBinding{
		ID:         "b-missing-dlr",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   node1,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   "missing-dialer-id",
		Enabled:    true,
	}
	_, _, err = resolver.ResolveChains(nodes, nil, nil, []domain.ProxyChainBinding{bMissingDialer})
	if err == nil {
		t.Fatal("expected chain_reference_missing for missing dialer node")
	}

	// 3. Empty dialer group when enabled
	emptyGrpID := domain.MustNewUUIDv7()
	groups := []resolver.ResolvedGroup{
		{ID: emptyGrpID, Name: "Empty Group", GroupType: domain.GroupTypeSelect, AllNodeLogicalIDs: nil},
	}
	bEmptyDialerGroup := domain.ProxyChainBinding{
		ID:         "b-empty-grp",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   node1,
		DialerType: domain.ProxyChainDialerNodeGroup,
		DialerID:   emptyGrpID,
		Enabled:    true,
	}
	_, _, err = resolver.ResolveChains(nodes, groups, nil, []domain.ProxyChainBinding{bEmptyDialerGroup})
	if err == nil {
		t.Fatal("expected chain_reference_missing for empty dialer group")
	}
	if domErr, ok := domain.AsDomainError(err); !ok || domErr.Code != "chain_reference_missing" {
		t.Fatalf("expected chain_reference_missing code, got: %v", err)
	}
}

func TestResolveChains_ZeroTargetAllowed(t *testing.T) {
	node1 := testNodeLogicalID("node-1")
	nodes := []resolver.ResolvedNode{
		{LogicalID: node1, DisplayName: "N1", Protocol: domain.ProtocolTrojan, Active: true},
	}

	// Subscription with 0 matching nodes in snapshot
	bZeroSub := domain.ProxyChainBinding{
		ID:         "b-zero-sub",
		TargetType: domain.ProxyChainTargetSubscription,
		TargetID:   "sub-empty",
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   node1,
		Enabled:    true,
	}

	_, effects, err := resolver.ResolveChains(nodes, nil, nil, []domain.ProxyChainBinding{bZeroSub})
	if err != nil {
		t.Fatalf("expected 0 target to be allowed, got error: %v", err)
	}
	if len(effects) != 1 {
		t.Fatalf("expected 1 effect, got %d", len(effects))
	}
	if effects[0].TargetCount != 0 || effects[0].ChainCount != 0 {
		t.Errorf("expected 0 target_count and chain_count, got: %+v", effects[0])
	}
	if len(effects[0].Diagnostics) == 0 || effects[0].Diagnostics[0].Code != "chain_target_empty" {
		t.Errorf("expected chain_target_empty diagnostic, got: %+v", effects[0].Diagnostics)
	}
}

func TestResolver_ChainsAffectInputDigestAndSnapshotDigest(t *testing.T) {
	r := resolver.New()
	ctx := context.Background()

	parentID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()
	node1ID := testNodeLogicalID("node-hk-01")
	node2ID := testNodeLogicalID("node-us-02")

	baseInput := resolver.ResolveInput{
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
		GroupSources: map[string][]domain.MemberSource{
			parentID: {
				{ID: "s1", GroupID: parentID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &node1ID, Position: 0},
				{ID: "s2", GroupID: parentID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &node2ID, Position: 1},
			},
			childID: {
				{ID: "s3", GroupID: childID, Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &node2ID, Position: 0},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentID, Expression: "MATCH", Position: 0},
		},
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1"},
		},
	}

	// 1. Resolve with NO chain bindings
	snapBase, err := r.Resolve(ctx, baseInput)
	if err != nil {
		t.Fatalf("base resolve failed: %v", err)
	}

	// 2. Resolve with chain binding
	bindingInput := baseInput
	b1 := domain.ProxyChainBinding{
		ID:         "bind-1",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   node1ID,
		DialerType: domain.ProxyChainDialerNode,
		DialerID:   node2ID,
		Enabled:    true,
		SortOrder:  0,
	}
	bindingInput.ChainBindings = []domain.ProxyChainBinding{b1}

	snapChained, err := r.Resolve(ctx, bindingInput)
	if err != nil {
		t.Fatalf("chained resolve failed: %v", err)
	}

	if snapChained.InputDigest == snapBase.InputDigest {
		t.Fatal("expected InputDigest to change when chain bindings added")
	}
	if snapChained.SnapshotDigest == snapBase.SnapshotDigest {
		t.Fatal("expected SnapshotDigest to change when chain bindings added")
	}

	if len(snapChained.ChainEffects) != 1 {
		t.Fatalf("expected 1 ChainEffect, got %d", len(snapChained.ChainEffects))
	}

	// 3. Shuffled chain bindings must produce exact same InputDigest and SnapshotDigest
	b2 := domain.ProxyChainBinding{
		ID:         "bind-2",
		TargetType: domain.ProxyChainTargetNode,
		TargetID:   node2ID,
		DialerType: domain.ProxyChainDialerNodeGroup,
		DialerID:   parentID,
		Enabled:    false, // disabled binding also part of input
		SortOrder:  5,
	}
	inputOrder1 := baseInput
	inputOrder1.ChainBindings = []domain.ProxyChainBinding{b1, b2}

	inputOrder2 := baseInput
	inputOrder2.ChainBindings = []domain.ProxyChainBinding{b2, b1}

	snapOrder1, err := r.Resolve(ctx, inputOrder1)
	if err != nil {
		t.Fatalf("order1 resolve failed: %v", err)
	}
	snapOrder2, err := r.Resolve(ctx, inputOrder2)
	if err != nil {
		t.Fatalf("order2 resolve failed: %v", err)
	}

	if snapOrder1.InputDigest != snapOrder2.InputDigest {
		t.Fatalf("InputDigest mismatch on permutation: %s vs %s", snapOrder1.InputDigest, snapOrder2.InputDigest)
	}
	if snapOrder1.SnapshotDigest != snapOrder2.SnapshotDigest {
		t.Fatalf("SnapshotDigest mismatch on permutation: %s vs %s", snapOrder1.SnapshotDigest, snapOrder2.SnapshotDigest)
	}
}
