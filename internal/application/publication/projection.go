package publication

import (
	"fmt"
	"strings"
	"unicode"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// 预检、预览和公开导出共用目标投影，不把单节点能力错误升级为整包失败
func projectExportSnapshot(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) (*resolver.ResolvedPolicySnapshot, []domain.ManifestExcludedNode, error) {
	if snapshot == nil {
		return snapshot, nil, nil
	}
	if len(snapshot.Nodes) == 0 {
		if hasRoutedEmptyPass(snapshot, target) {
			return snapshot, nil, nil
		}
		return snapshot, nil, domain.NewValidationError("filtered_nodes_empty", "no exportable nodes remain and no routed PASS fallback is configured")
	}
	projected, excluded := compiler.FilterCompatibleSnapshot(snapshot, target)
	// Resolve labels from the same pre-projection snapshot, including nodes
	// excluded through dialer dependencies. Never consult today's inventory.
	excludedByID := make(map[string]int, len(excluded))
	for i := range excluded {
		excludedByID[excluded[i].NodeID] = i
	}
	for _, node := range snapshot.Nodes {
		if i, ok := excludedByID[node.LogicalID]; ok {
			excluded[i].DisplayName = safeExcludedDisplayName(node)
		}
	}
	for _, node := range excluded {
		projected.Diagnostics = append(projected.Diagnostics, resolver.Diagnostic{
			Severity: resolver.DiagnosticSeverityWarning,
			Code:     node.Code,
			Message:  fmt.Sprintf("node %s excluded from %s export: %s", node.NodeID, target, node.Reason),
			Target:   string(target),
			Reason:   node.Reason,
		})
	}
	digest, err := resolver.ComputeSnapshotDigest(projected)
	if err != nil {
		return projected, excluded, fmt.Errorf("compute target projection digest: %w", err)
	}
	projected.SnapshotDigest = digest
	if len(projected.Nodes) == 0 {
		err := domain.NewValidationError("no_compatible_nodes", fmt.Sprintf("target %s has no compatible nodes among %d candidate nodes", target, len(snapshot.Nodes)))
		err.Details = map[string]string{"target": string(target)}
		for _, node := range excluded {
			err.Details[node.NodeID] = node.Reason
		}
		return projected, excluded, err
	}
	return projected, excluded, nil
}

// Omit credential-like labels rather than risk turning a diagnostic into a
// connection URL or credential viewer. The node ID remains a stable fallback.
func safeExcludedDisplayName(node resolver.ResolvedNode) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, node.DisplayName)
	if strings.Contains(name, "://") {
		return ""
	}
	// Share the compiler's collector, including nested plugin options. Checking
	// only outer transport keys would freeze bare inner secrets as node names.
	secrets, complete := compiler.CollectCredentialSecrets(node.Credentials)
	if !complete {
		return ""
	}
	for _, secret := range secrets {
		if strings.Contains(strings.ToLower(name), strings.ToLower(secret)) {
			return ""
		}
	}
	name = domain.RedactSensitiveInfo(name)
	runes := []rune(name)
	if len(runes) > 128 {
		name = string(runes[:127]) + "…"
	}
	return name
}

func projectionWarnings(snapshot *resolver.ResolvedPolicySnapshot) []PreflightDiagnostic {
	var diagnostics []PreflightDiagnostic
	if snapshot == nil {
		return diagnostics
	}
	for _, diagnostic := range snapshot.Diagnostics {
		if (diagnostic.Code == "unsupported_target_capability" || diagnostic.Code == "format_error") && diagnostic.Severity == resolver.DiagnosticSeverityWarning {
			diagnostics = append(diagnostics, PreflightDiagnostic{
				Severity: diagnostic.Severity, Code: diagnostic.Code, Message: diagnostic.Message, Target: diagnostic.Target,
			})
		}
	}
	return diagnostics
}

func publicationExclusions(pub *domain.Publication) []domain.ManifestExcludedNode {
	if pub.Manifest == nil {
		return []domain.ManifestExcludedNode{}
	}
	return append([]domain.ManifestExcludedNode{}, pub.Manifest.Excluded...)
}
