package resolver

import (
	"fmt"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
)

type bindingCandidate struct {
	binding           domain.ProxyChainBinding
	tier              int // 1: node, 2: node_group, 3: subscription
	dialerDisplayName string
}

// ResolveChains executes single-track deterministic chain expansion, priority resolution,
// overlap skipping, and complete transitive cycle detection across nodes and groups.
func ResolveChains(
	nodes []ResolvedNode,
	groups []ResolvedGroup,
	nodeSources map[string][]domain.NodeSource,
	bindings []domain.ProxyChainBinding,
) ([]ResolvedNode, []ChainEffect, error) {
	var enabledBindings []domain.ProxyChainBinding
	for _, b := range bindings {
		if err := b.Validate(); err != nil {
			return nil, nil, err
		}
		if b.Enabled {
			enabledBindings = append(enabledBindings, b)
		}
	}
	if len(enabledBindings) == 0 {
		return nodes, nil, nil
	}

	nodeMap := make(map[string]ResolvedNode, len(nodes))
	for _, n := range nodes {
		nodeMap[n.LogicalID] = n
	}

	groupMap := make(map[string]ResolvedGroup, len(groups))
	for _, g := range groups {
		groupMap[g.ID] = g
	}

	type validatedBindingMeta struct {
		binding           domain.ProxyChainBinding
		tier              int
		dialerDisplayName string
		dialerLeaves      map[string]struct{}
		targetCount       int
		skipCount         int
		skipSamples       []string
		eligibleNodes     []ResolvedNode
	}

	metaList := make([]validatedBindingMeta, 0, len(enabledBindings))

	for _, b := range enabledBindings {
		// 1. Validate Dialer
		var dialerDisplayName string
		dialerLeaves := make(map[string]struct{})

		switch b.DialerType {
		case domain.ProxyChainDialerNode:
			dialerNode, exists := nodeMap[b.DialerID]
			if !exists {
				return nil, nil, domain.NewValidationError(
					"chain_reference_missing",
					fmt.Sprintf("dialer node %q not found or not exportable", b.DialerID),
				)
			}
			dialerDisplayName = dialerNode.DisplayName
			dialerLeaves[b.DialerID] = struct{}{}

		case domain.ProxyChainDialerNodeGroup:
			dialerGroup, exists := groupMap[b.DialerID]
			if !exists {
				return nil, nil, domain.NewValidationError(
					"chain_reference_missing",
					fmt.Sprintf("dialer group %q not found", b.DialerID),
				)
			}
			if len(dialerGroup.AllNodeLogicalIDs) == 0 {
				return nil, nil, domain.NewValidationError(
					"chain_reference_missing",
					fmt.Sprintf("dialer group %q has no active nodes", b.DialerID),
				)
			}
			dialerDisplayName = dialerGroup.Name
			for _, nid := range dialerGroup.AllNodeLogicalIDs {
				dialerLeaves[nid] = struct{}{}
			}

		default:
			return nil, nil, domain.NewValidationError(
				"invalid_chain_binding",
				fmt.Sprintf("invalid chain dialer type: %s", b.DialerType),
			)
		}

		// 2. Direct Self-Reference
		if b.TargetType == domain.ProxyChainTargetNode && b.DialerType == domain.ProxyChainDialerNode && b.TargetID == b.DialerID {
			return nil, nil, domain.NewValidationError(
				"chain_cycle",
				fmt.Sprintf("node %q cannot chain to itself", b.TargetID),
			)
		}
		if b.TargetType == domain.ProxyChainTargetNodeGroup && b.DialerType == domain.ProxyChainDialerNodeGroup && b.TargetID == b.DialerID {
			return nil, nil, domain.NewValidationError(
				"chain_cycle",
				fmt.Sprintf("group %q cannot chain to itself", b.TargetID),
			)
		}

		// 3. Validate Target & Resolve Candidate Nodes
		var tier int
		var candidates []ResolvedNode

		switch b.TargetType {
		case domain.ProxyChainTargetNode:
			tier = 1
			targetNode, exists := nodeMap[b.TargetID]
			if !exists {
				return nil, nil, domain.NewValidationError(
					"chain_reference_missing",
					fmt.Sprintf("target node %q not found or not exportable", b.TargetID),
				)
			}
			candidates = []ResolvedNode{targetNode}

		case domain.ProxyChainTargetNodeGroup:
			tier = 2
			targetGroup, exists := groupMap[b.TargetID]
			if !exists {
				return nil, nil, domain.NewValidationError(
					"chain_reference_missing",
					fmt.Sprintf("target group %q not found", b.TargetID),
				)
			}
			candidates = make([]ResolvedNode, 0, len(targetGroup.AllNodeLogicalIDs))
			for _, nid := range targetGroup.AllNodeLogicalIDs {
				if n, ok := nodeMap[nid]; ok {
					candidates = append(candidates, n)
				}
			}

		case domain.ProxyChainTargetSubscription:
			tier = 3
			candidates = make([]ResolvedNode, 0)
			for _, n := range nodes {
				srcs := nodeSources[n.LogicalID]
				for _, s := range srcs {
					if s.SubscriptionID == b.TargetID {
						candidates = append(candidates, n)
						break
					}
				}
			}

		default:
			return nil, nil, domain.NewValidationError(
				"invalid_chain_binding",
				fmt.Sprintf("invalid chain target type: %s", b.TargetType),
			)
		}

		// 4. Overlap Check
		targetCount := len(candidates)
		var skipCount int
		var skipSamples []string
		var eligibleNodes []ResolvedNode

		for _, cand := range candidates {
			if _, overlaps := dialerLeaves[cand.LogicalID]; overlaps {
				skipCount++
				if len(skipSamples) < 10 {
					skipSamples = append(skipSamples, cand.DisplayName)
				}
			} else {
				eligibleNodes = append(eligibleNodes, cand)
			}
		}

		if targetCount > 0 && skipCount == targetCount {
			return nil, nil, domain.NewValidationError(
				"chain_all_targets_overlap",
				fmt.Sprintf("all %d target nodes overlap with dialer upstream for binding %s", targetCount, b.ID),
			)
		}

		metaList = append(metaList, validatedBindingMeta{
			binding:           b,
			tier:              tier,
			dialerDisplayName: dialerDisplayName,
			dialerLeaves:      dialerLeaves,
			targetCount:       targetCount,
			skipCount:         skipCount,
			skipSamples:       skipSamples,
			eligibleNodes:     eligibleNodes,
		})
	}

	// 5. Priority Resolution per Node
	// Map node logical ID -> list of candidate bindings
	eligibleByNode := make(map[string][]bindingCandidate, len(nodes))
	for _, meta := range metaList {
		for _, n := range meta.eligibleNodes {
			eligibleByNode[n.LogicalID] = append(eligibleByNode[n.LogicalID], bindingCandidate{
				binding:           meta.binding,
				tier:              meta.tier,
				dialerDisplayName: meta.dialerDisplayName,
			})
		}
	}

	winningAssignments := make(map[string]bindingCandidate, len(nodes))
	chainCounts := make(map[string]int, len(metaList))
	chainSamples := make(map[string][]string, len(metaList))
	overriddenCounts := make(map[string]int, len(metaList))

	for _, n := range nodes {
		candidates := eligibleByNode[n.LogicalID]
		if len(candidates) == 0 {
			continue
		}

		// Sort candidates by priority:
		// 1. Tier (node > group > subscription)
		// 2. SortOrder ascending (smaller value has higher priority)
		// 3. ID ascending (lexicographical)
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].tier != candidates[j].tier {
				return candidates[i].tier < candidates[j].tier
			}
			if candidates[i].binding.SortOrder != candidates[j].binding.SortOrder {
				return candidates[i].binding.SortOrder < candidates[j].binding.SortOrder
			}
			return candidates[i].binding.ID < candidates[j].binding.ID
		})

		winner := candidates[0]
		winningAssignments[n.LogicalID] = winner
		chainCounts[winner.binding.ID]++
		if len(chainSamples[winner.binding.ID]) < 10 {
			chainSamples[winner.binding.ID] = append(chainSamples[winner.binding.ID], n.DisplayName)
		}

		for _, loser := range candidates[1:] {
			overriddenCounts[loser.binding.ID]++
		}
	}

	// 6. Complete Transitive Cycle Detection (DFS)
	adj := make(map[string][]string)
	vertexSet := make(map[string]struct{})

	addVertex := func(v string) {
		vertexSet[v] = struct{}{}
		if _, exists := adj[v]; !exists {
			adj[v] = nil
		}
	}
	addEdge := func(u, v string) {
		addVertex(u)
		addVertex(v)
		adj[u] = append(adj[u], v)
	}

	dialerGroupsUsed := make(map[string]struct{})

	for nodeID, winner := range winningAssignments {
		u := "node:" + nodeID
		if winner.binding.DialerType == domain.ProxyChainDialerNode {
			v := "node:" + winner.binding.DialerID
			addEdge(u, v)
		} else if winner.binding.DialerType == domain.ProxyChainDialerNodeGroup {
			v := "group:" + winner.binding.DialerID
			addEdge(u, v)
			dialerGroupsUsed[winner.binding.DialerID] = struct{}{}
		}
	}

	for groupID := range dialerGroupsUsed {
		g := groupMap[groupID]
		u := "group:" + groupID
		for _, leafID := range g.AllNodeLogicalIDs {
			v := "node:" + leafID
			addEdge(u, v)
		}
	}

	allVertices := make([]string, 0, len(vertexSet))
	for v := range vertexSet {
		allVertices = append(allVertices, v)
	}
	sort.Strings(allVertices)

	for _, v := range allVertices {
		sort.Strings(adj[v])
	}

	color := make(map[string]int) // 0: unvisited, 1: visiting, 2: visited
	var stack []string
	var cycleErr error

	var dfs func(u string) bool
	dfs = func(u string) bool {
		color[u] = 1
		stack = append(stack, u)

		for _, v := range adj[u] {
			if color[v] == 1 {
				idx := -1
				for i, s := range stack {
					if s == v {
						idx = i
						break
					}
				}
				var cyclePath []string
				if idx >= 0 {
					cyclePath = append(cyclePath, stack[idx:]...)
					cyclePath = append(cyclePath, v)
				} else {
					cyclePath = []string{u, v, u}
				}
				pathStr := strings.Join(cyclePath, " -> ")
				domErr := domain.NewValidationError("chain_cycle", fmt.Sprintf("chain cycle detected: %s", pathStr))
				domErr.CyclePath = cyclePath
				domErr.Details = map[string]string{
					"cycle": pathStr,
				}
				cycleErr = domErr
				return true
			}
			if color[v] == 0 {
				if dfs(v) {
					return true
				}
			}
		}

		stack = stack[:len(stack)-1]
		color[u] = 2
		return false
	}

	for _, v := range allVertices {
		if color[v] == 0 {
			if dfs(v) {
				return nil, nil, cycleErr
			}
		}
	}

	// 7. Assemble Effects
	effects := make([]ChainEffect, 0, len(metaList))
	for _, meta := range metaList {
		b := meta.binding
		eff := ChainEffect{
			BindingID:       b.ID,
			TargetCount:     meta.targetCount,
			ChainCount:      chainCounts[b.ID],
			SkipCount:       meta.skipCount,
			OverriddenCount: overriddenCounts[b.ID],
			ChainSamples:    chainSamples[b.ID],
			SkipSamples:     meta.skipSamples,
		}

		if eff.SkipCount > 0 {
			eff.Diagnostics = append(eff.Diagnostics, Diagnostic{
				Severity: DiagnosticSeverityInfo,
				Code:     "chain_target_skipped_overlap",
				Message:  fmt.Sprintf("%d target node(s) skipped due to overlap with dialer upstream", eff.SkipCount),
				Target:   b.ID,
				Reason:   "target node overlaps with dialer upstream",
			})
		}
		if eff.OverriddenCount > 0 {
			eff.Diagnostics = append(eff.Diagnostics, Diagnostic{
				Severity: DiagnosticSeverityInfo,
				Code:     "chain_target_overridden",
				Message:  fmt.Sprintf("%d target node(s) overridden by higher priority chain bindings", eff.OverriddenCount),
				Target:   b.ID,
				Reason:   "overridden by higher priority binding",
			})
		}
		if eff.TargetCount == 0 {
			eff.Diagnostics = append(eff.Diagnostics, Diagnostic{
				Severity: DiagnosticSeverityInfo,
				Code:     "chain_target_empty",
				Message:  "chain binding matched 0 target nodes",
				Target:   b.ID,
			})
		}

		effects = append(effects, eff)
	}

	sort.SliceStable(effects, func(i, j int) bool {
		return effects[i].BindingID < effects[j].BindingID
	})

	// 8. Produce Final Resolved Nodes
	updatedNodes := make([]ResolvedNode, len(nodes))
	copy(updatedNodes, nodes)
	for i, n := range updatedNodes {
		if winner, ok := winningAssignments[n.LogicalID]; ok {
			updatedNodes[i].DialerProxy = winner.dialerDisplayName
			updatedNodes[i].ChainBindingID = winner.binding.ID
			updatedNodes[i].ChainSource = string(winner.binding.TargetType)
		}
	}

	return updatedNodes, effects, nil
}
