package compiler

import (
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func filterChainDependencies(snapshot *resolver.ResolvedPolicySnapshot, nodes []resolver.ResolvedNode, excluded []domain.ManifestExcludedNode) ([]resolver.ResolvedNode, []domain.ManifestExcludedNode) {
	dependencies, _ := resolveChainDependencies(snapshot)
	for {
		keptIDs := make(map[string]bool, len(nodes))
		for _, node := range nodes {
			keptIDs[node.LogicalID] = true
		}
		kept := make([]resolver.ResolvedNode, 0, len(nodes))
		for _, node := range nodes {
			dialer := strings.TrimSpace(node.DialerProxy)
			if dialer == "" {
				kept = append(kept, node)
				continue
			}
			dependency, resolved := dependencies[node.LogicalID]
			if !resolved {
				// 无法确定绑定的节点留给严格拓扑校验，不能当成能力排除而隐藏错误
				kept = append(kept, node)
				continue
			}
			usable := false
			for _, id := range dependency {
				if keptIDs[id] {
					usable = true
					break
				}
			}
			if !usable {
				excluded = append(excluded, domain.ManifestExcludedNode{
					NodeID: node.LogicalID, Code: "unsupported_target_capability",
					Reason: "dialer-proxy dependency is unavailable for target",
				})
				continue
			}
			kept = append(kept, node)
		}
		if len(kept) == len(nodes) {
			// 排除项也按原节点顺序返回，避免依赖展开轮次影响预览
			order := make(map[string]int, len(snapshot.Nodes))
			for i, node := range snapshot.Nodes {
				order[node.LogicalID] = i
			}
			sort.SliceStable(excluded, func(i, j int) bool { return order[excluded[i].NodeID] < order[excluded[j].NodeID] })
			return kept, excluded
		}
		nodes = kept
	}
}

func refreshProjectedGroups(snapshot *resolver.ResolvedPolicySnapshot) {
	nodeNames := make(map[string]string, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		nodeNames[strings.TrimSpace(node.DisplayName)] = node.LogicalID
	}
	groups := make(map[string]int, len(snapshot.Groups))
	groupNames := make(map[string]string, len(snapshot.Groups))
	for i, group := range snapshot.Groups {
		groups[group.ID] = i
		groupNames[strings.TrimSpace(group.Name)] = group.ID
	}
	for i := range snapshot.Groups {
		group := &snapshot.Groups[i]
		group.NodeLogicalIDs = nil
		group.ChildGroupIDs = nil
		for _, member := range group.Members {
			kind := member.Kind
			if kind == "" {
				name := strings.TrimSpace(member.DisplayName)
				if _, ok := groupNames[name]; ok {
					kind = resolver.MemberKindGroup
				} else if _, ok := nodeNames[name]; ok {
					kind = resolver.MemberKindNode
				}
			}
			switch kind {
			case resolver.MemberKindNode:
				id := member.TargetID
				if id == "" {
					id = nodeNames[strings.TrimSpace(member.DisplayName)]
				}
				if id != "" {
					group.NodeLogicalIDs = append(group.NodeLogicalIDs, id)
				}
			case resolver.MemberKindGroup:
				id := member.TargetID
				if id == "" {
					id = groupNames[strings.TrimSpace(member.DisplayName)]
				}
				if id != "" {
					group.ChildGroupIDs = append(group.ChildGroupIDs, id)
				}
			}
		}
	}
	for i := range snapshot.Groups {
		ids := make(map[string]bool)
		visited := make(map[string]bool)
		hasBuiltin := false
		var visit func(string)
		visit = func(id string) {
			if visited[id] {
				return
			}
			visited[id] = true
			index, ok := groups[id]
			if !ok {
				return
			}
			group := snapshot.Groups[index]
			for _, id := range group.NodeLogicalIDs {
				ids[id] = true
			}
			for _, member := range group.Members {
				if member.Kind == resolver.MemberKindBuiltin || (member.Kind == "" && isBuiltInPolicyTarget(member.DisplayName)) {
					hasBuiltin = true
				}
			}
			for _, child := range group.ChildGroupIDs {
				visit(child)
			}
		}
		visit(snapshot.Groups[i].ID)
		group := &snapshot.Groups[i]
		group.AllNodeLogicalIDs = make([]string, 0, len(ids))
		for id := range ids {
			group.AllNodeLogicalIDs = append(group.AllNodeLogicalIDs, id)
		}
		sort.Strings(group.AllNodeLogicalIDs)
		group.ExpandedNodeIDs = append([]string(nil), group.AllNodeLogicalIDs...)
		// 只使用本组已解析的 PASS 开关，不从子组取得授权
		group.EffectiveEmptyPass = group.EmptyFallbackPass && len(ids) == 0 && !hasBuiltin
	}
}

// 仅本组已解析的 PASS 权限可以允许空组，不能从子组继承
func groupHasExportDestination(snapshot *resolver.ResolvedPolicySnapshot, group resolver.ResolvedGroup) bool {
	return group.UsesEmptyPass() || (len(group.Members) > 0 && groupHasConcreteDestination(snapshot, group, make(map[string]bool)))
}

func groupHasConcreteDestination(snapshot *resolver.ResolvedPolicySnapshot, group resolver.ResolvedGroup, visited map[string]bool) bool {
	if visited[group.ID] {
		return false
	}
	visited[group.ID] = true
	if len(group.AllNodeLogicalIDs) > 0 {
		return true
	}
	for _, member := range group.Members {
		if member.Kind == resolver.MemberKindBuiltin || (member.Kind == "" && isBuiltInPolicyTarget(member.DisplayName)) {
			return true
		}
		if member.Kind == resolver.MemberKindNode || member.Kind == "" {
			for _, node := range snapshot.Nodes {
				if (member.TargetID != "" && member.TargetID == node.LogicalID) || (member.TargetID == "" && strings.TrimSpace(member.DisplayName) == strings.TrimSpace(node.DisplayName)) {
					return true
				}
			}
		}
		if member.Kind == resolver.MemberKindGroup || member.Kind == "" {
			for _, child := range snapshot.Groups {
				if (member.TargetID != "" && child.ID == member.TargetID) || (member.TargetID == "" && child.Name == member.DisplayName) {
					if groupHasConcreteDestination(snapshot, child, visited) {
						return true
					}
				}
			}
		}
	}
	return false
}

func refreshProjectionSummaries(original, projected *resolver.ResolvedPolicySnapshot, excluded []domain.ManifestExcludedNode, target domain.CompilerTarget) {
	if len(excluded) == 0 {
		return
	}
	exclusionCodes := make(map[string]string, len(excluded))
	for _, node := range excluded {
		exclusionCodes[node.NodeID] = node.Code
	}
	kept := make(map[string]bool, len(projected.Nodes))
	for _, node := range projected.Nodes {
		kept[node.LogicalID] = true
	}
	projected.AdmittedNodeIDs = make([]string, 0, len(projected.Nodes))
	for _, node := range projected.Nodes {
		projected.AdmittedNodeIDs = append(projected.AdmittedNodeIDs, node.LogicalID)
	}
	sort.Strings(projected.AdmittedNodeIDs)
	projected.ExcludedNodeIDs = append([]string(nil), original.ExcludedNodeIDs...)
	for _, node := range excluded {
		projected.ExcludedNodeIDs = append(projected.ExcludedNodeIDs, node.NodeID)
	}
	sort.Strings(projected.ExcludedNodeIDs)
	if original.FilterCounts != nil {
		counts := *original.FilterCounts
		counts.GroupCounts = make(map[string]resolver.GroupFilterCount, len(original.FilterCounts.GroupCounts))
		groupNodes := make(map[string]bool)
		for _, group := range projected.Groups {
			for _, id := range group.AllNodeLogicalIDs {
				groupNodes[id] = true
			}
		}
		counts.GroupFilteredTotal = len(groupNodes)
		for id, count := range original.FilterCounts.GroupCounts {
			count.ExclusionSummary = copyCountSummary(count.ExclusionSummary)
			for _, group := range original.Groups {
				if group.ID != id {
					continue
				}
				removed := 0
				for _, nodeID := range group.AllNodeLogicalIDs {
					if !kept[nodeID] && removed < count.Kept {
						removed++
						count.ExclusionSummary[exclusionCodes[nodeID]]++
					}
				}
				if removed > count.Kept {
					removed = count.Kept
				}
				count.Kept -= removed
				count.Excluded += removed
				break
			}
			counts.GroupCounts[id] = count
		}
		projected.FilterCounts = &counts
		// 全局候选集合未保存在快照中，不能根据并集倒推全局过滤后的数量
		projected.Diagnostics = append(projected.Diagnostics, resolver.Diagnostic{
			Severity: resolver.DiagnosticSeverityInfo, Code: "target_projection_filter_counts",
			Message: "raw, admission and global filter counts describe resolution before target projection; group counts include target exclusions",
			Target:  string(target),
		})
	}
	if len(original.ChainEffects) > 0 {
		// 绑定的完整候选与跳过集合未保存，不能用有限样本伪造投影后的汇总
		projected.ChainEffects = nil
		projected.Diagnostics = append(projected.Diagnostics, resolver.Diagnostic{
			Severity: resolver.DiagnosticSeverityInfo, Code: "target_projection_chain_effects_unavailable",
			Message: "chain effect summaries require binding inputs after target exclusions; retained nodes preserve their dialer-proxy assignments",
			Target:  string(target),
		})
	}
}

func copyCountSummary(source map[string]int) map[string]int {
	copy := make(map[string]int, len(source))
	for key, count := range source {
		copy[key] = count
	}
	return copy
}
