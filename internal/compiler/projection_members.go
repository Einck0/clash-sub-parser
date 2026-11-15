package compiler

import (
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// 已有 ID 时只认 ID，旧的无类型成员必须能唯一定位，不能按名称猜节点或组
func bindProjectionGroupMembers(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) ([]resolver.ResolvedGroup, []resolver.Diagnostic) {
	groups := append([]resolver.ResolvedGroup(nil), snapshot.Groups...)
	if target != domain.TargetMihomo {
		return groups, nil
	}
	var diagnostics []resolver.Diagnostic
	for i := range groups {
		groups[i].Members = append([]resolver.ResolvedGroupMember(nil), groups[i].Members...)
		for j := range groups[i].Members {
			member := &groups[i].Members[j]
			if member.Kind != "" {
				continue
			}
			name := strings.TrimSpace(member.DisplayName)
			matches := 0
			var kind resolver.MemberKind
			var id string
			for _, node := range snapshot.Nodes {
				if (member.TargetID != "" && member.TargetID == node.LogicalID) || (member.TargetID == "" && name == strings.TrimSpace(node.DisplayName)) {
					matches++
					kind, id = resolver.MemberKindNode, node.LogicalID
				}
			}
			for _, group := range snapshot.Groups {
				if (member.TargetID != "" && member.TargetID == group.ID) || (member.TargetID == "" && name == strings.TrimSpace(group.Name)) {
					matches++
					kind, id = resolver.MemberKindGroup, group.ID
				}
			}
			if isBuiltInPolicyTarget(name) && (member.TargetID == "" || member.TargetID == name) {
				matches++
				kind, id = resolver.MemberKindBuiltin, name
			}
			if matches == 1 {
				member.Kind, member.TargetID = kind, id
			} else {
				diagnostics = append(diagnostics, resolver.Diagnostic{
					Severity: resolver.DiagnosticSeverityError, Code: "invalid_group_topology", Target: string(target),
					Message: "group member reference is missing or ambiguous",
				})
			}
		}
	}
	return groups, diagnostics
}
