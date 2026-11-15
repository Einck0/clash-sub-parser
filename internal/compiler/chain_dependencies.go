package compiler

import (
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func chainTopologyDiagnostics(snapshot *resolver.ResolvedPolicySnapshot) []CapabilityDiagnostic {
	_, diagnostics := resolveChainDependencies(snapshot)
	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.Code == "invalid_chain_topology" && diagnostic.Severity == resolver.DiagnosticSeverityError {
			diagnostics = append(diagnostics, CapabilityDiagnostic{
				Code: diagnostic.Code, Message: diagnostic.Message, Target: string(domain.TargetMihomo),
			})
		}
	}
	return diagnostics
}

// 绑定用稳定 ID 判断存活，名称只用于检查实际渲染引用是否唯一
func resolveChainDependencies(snapshot *resolver.ResolvedPolicySnapshot) (map[string][]string, []CapabilityDiagnostic) {
	dependencies := make(map[string][]string)
	var diagnostics []CapabilityDiagnostic
	for _, node := range snapshot.Nodes {
		name := strings.TrimSpace(node.DialerProxy)
		if name == "" && node.DialerID == "" && node.DialerType == "" {
			continue
		}
		matches := 0
		var id string
		var kind domain.ProxyChainDialerType
		var leaves []string
		for _, candidate := range snapshot.Nodes {
			if strings.TrimSpace(candidate.DisplayName) == name {
				matches++
				id, kind = candidate.LogicalID, domain.ProxyChainDialerNode
				leaves = []string{candidate.LogicalID}
			}
		}
		for _, group := range snapshot.Groups {
			if strings.TrimSpace(group.Name) == name {
				matches++
				id, kind, leaves = group.ID, domain.ProxyChainDialerNodeGroup, group.AllNodeLogicalIDs
			}
		}
		// 旧快照缺少绑定字段时，只接受唯一可解析引用，不按同名节点或组猜测
		bound := node.DialerID != "" || node.DialerType != ""
		if name == "" || matches != 1 || isBuiltInPolicyTarget(name) || id == "" ||
			(bound && (node.DialerID != id || node.DialerType != kind)) {
			diagnostics = append(diagnostics, CapabilityDiagnostic{
				NodeID: node.LogicalID, Code: "invalid_chain_topology", Target: string(domain.TargetMihomo),
				Message: "dialer-proxy reference is missing, ambiguous or inconsistent with its resolved binding",
			})
			continue
		}
		dependencies[node.LogicalID] = leaves
	}
	return dependencies, diagnostics
}
