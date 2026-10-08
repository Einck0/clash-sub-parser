package compiler

import (
	"fmt"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// PruneUnavailableOptionalGroups safely prunes empty groups that are not directly targeted by any routing rules
// and are either:
// 1) completely unused (no parents and no routing rules); or
// 2) only referenced as optional candidates inside selector (select) groups, where removing them leaves the parent non-empty.
// If any empty group is directly targeted by a rule, or referenced by non-selector groups (e.g. urltest/fallback),
// or removing it would leave a parent group empty, it is NOT pruned so validation can fail closed.
func PruneUnavailableOptionalGroups(snapshot *resolver.ResolvedPolicySnapshot) (*resolver.ResolvedPolicySnapshot, []resolver.Diagnostic, error) {
	if snapshot == nil {
		return nil, nil, nil
	}

	currSnapshot := snapshot
	var allDiagnostics []resolver.Diagnostic

	// Multi-pass iterative pruning until fixed point
	maxRounds := len(snapshot.Groups) + 1
	for round := 0; round < maxRounds; round++ {
		prunedSnap, diags, changed := pruneOnePass(currSnapshot)
		if !changed {
			break
		}
		allDiagnostics = append(allDiagnostics, diags...)
		currSnapshot = prunedSnap
	}

	if len(allDiagnostics) == 0 {
		return snapshot, nil, nil
	}

	return currSnapshot, allDiagnostics, nil
}

func pruneOnePass(snapshot *resolver.ResolvedPolicySnapshot) (*resolver.ResolvedPolicySnapshot, []resolver.Diagnostic, bool) {
	if snapshot == nil {
		return snapshot, nil, false
	}

	// 1. Identify all groups directly targeted by routing rules
	ruleTargetGroupIDs := make(map[string]bool)
	ruleTargetGroupNames := make(map[string]bool)
	for _, rule := range snapshot.Rules {
		if id := strings.TrimSpace(rule.TargetGroupID); id != "" {
			ruleTargetGroupIDs[id] = true
		}
		if name := strings.TrimSpace(rule.TargetGroupName); name != "" {
			ruleTargetGroupNames[name] = true
		}
	}

	// 2. Map parent-child references among groups: child -> list of parent groups referencing it
	parentRefs := make(map[string][]resolver.ResolvedGroup)
	for _, group := range snapshot.Groups {
		for _, member := range group.Members {
			if member.Kind == resolver.MemberKindGroup || member.Kind == "" {
				if id := strings.TrimSpace(member.TargetID); id != "" {
					parentRefs[id] = append(parentRefs[id], group)
				}
				if name := strings.TrimSpace(member.DisplayName); name != "" {
					parentRefs[name] = append(parentRefs[name], group)
				}
			}
		}
	}

	// 3. Find empty groups that are safe to prune
	pruneGroupIDs := make(map[string]bool)
	pruneGroupNames := make(map[string]bool)
	var diagnostics []resolver.Diagnostic

	for _, group := range snapshot.Groups {
		if group.UsesEmptyPass() || len(group.Members) > 0 {
			continue // Non-empty group, keep as-is
		}

		// Check if directly targeted by any rule
		if ruleTargetGroupIDs[group.ID] || ruleTargetGroupNames[group.Name] {
			// Directed by rule: CANNOT prune!
			continue
		}

		// Check all parent groups referencing this empty group
		parentsByID := parentRefs[group.ID]
		parentsByName := parentRefs[group.Name]
		allParents := append([]resolver.ResolvedGroup(nil), parentsByID...)
		for _, p := range parentsByName {
			found := false
			for _, ep := range allParents {
				if ep.ID == p.ID {
					found = true
					break
				}
			}
			if !found {
				allParents = append(allParents, p)
			}
		}

		canPrune := true
		parentNamesSet := make(map[string]bool)
		for _, parent := range allParents {
			// Only selector (select) groups treat child groups as optional candidate choices
			if parent.GroupType != domain.GroupTypeSelect {
				canPrune = false
				break
			}
			// Check if removing this member would leave the parent group empty
			remainingMembers := 0
			for _, m := range parent.Members {
				mID := strings.TrimSpace(m.TargetID)
				mName := strings.TrimSpace(m.DisplayName)
				if (mID != "" && mID == group.ID) || (mName != "" && mName == group.Name) {
					continue
				}
				remainingMembers++
			}
			if remainingMembers == 0 {
				canPrune = false
				break
			}
			parentNamesSet[parent.Name] = true
		}

		if !canPrune {
			continue
		}

		// Group is safe to prune!
		if group.ID != "" {
			pruneGroupIDs[group.ID] = true
		}
		if group.Name != "" {
			pruneGroupNames[group.Name] = true
		}

		var parentNames []string
		for pName := range parentNamesSet {
			parentNames = append(parentNames, pName)
		}
		sort.Strings(parentNames)

		var msg string
		if len(parentNames) > 0 {
			msg = fmt.Sprintf("WARP暂不可用，本次备选移除，用户设置保留 (group %s pruned from %d selector references: %s; reason: disabled-source)", group.Name, len(parentNames), strings.Join(parentNames, ", "))
		} else {
			msg = fmt.Sprintf("group %s is unused and has 0 active members; safely omitted from rendered configuration (reason: unused-empty)", group.Name)
		}

		diagnostics = append(diagnostics, resolver.Diagnostic{
			Severity: resolver.DiagnosticSeverityWarning,
			Code:     "optional_group_pruned",
			Message:  msg,
			Target:   group.Name,
			Reason:   "disabled-source",
		})
	}

	if len(pruneGroupIDs) == 0 && len(pruneGroupNames) == 0 {
		return snapshot, nil, false
	}

	// 4. Construct pruned snapshot
	var keptGroups []resolver.ResolvedGroup
	for _, group := range snapshot.Groups {
		if pruneGroupIDs[group.ID] || pruneGroupNames[group.Name] {
			continue // Omit pruned group
		}
		// Filter out references to pruned groups in members
		var keptMembers []resolver.ResolvedGroupMember
		for _, member := range group.Members {
			mID := strings.TrimSpace(member.TargetID)
			mName := strings.TrimSpace(member.DisplayName)
			if (mID != "" && pruneGroupIDs[mID]) || (mName != "" && pruneGroupNames[mName]) {
				continue // Omit pruned child reference
			}
			keptMembers = append(keptMembers, member)
		}
		groupCopy := group
		groupCopy.Members = keptMembers
		keptGroups = append(keptGroups, groupCopy)
	}

	newSnap := *snapshot
	newSnap.Groups = keptGroups
	newSnap.Diagnostics = append(append([]resolver.Diagnostic(nil), snapshot.Diagnostics...), diagnostics...)

	return &newSnap, diagnostics, true
}
