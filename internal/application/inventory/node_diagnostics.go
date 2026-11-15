package inventory

import (
	"encoding/json"
	"fmt"
	"strings"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/resolver"
)

// TargetCompatibility 只描述节点自身的目标能力，不代表策略拓扑一定可用
type TargetCompatibility struct {
	Status      string                          `json:"status"`
	Diagnostics []compiler.CapabilityDiagnostic `json:"diagnostics,omitempty"`
}

func nodeTargetCompatibility(node domain.Node) map[domain.CompilerTarget]TargetCompatibility {
	if err := parser.ValidateNodeFormat(node); err != nil {
		result := make(map[domain.CompilerTarget]TargetCompatibility)
		for _, target := range compiler.SortedCapabilities() {
			result[target] = TargetCompatibility{Status: "format_error", Diagnostics: []compiler.CapabilityDiagnostic{{
				NodeID: node.LogicalID, Code: "format_error", Message: err.Error(), Target: string(target),
			}}}
		}
		return result
	}
	// 每个节点单独判断，避免列表中的重名或分组改变节点自身的能力状态
	snapshot := &resolver.ResolvedPolicySnapshot{Nodes: []resolver.ResolvedNode{{
		LogicalID:   node.LogicalID,
		DisplayName: node.DisplayName,
		Protocol:    node.Protocol,
		Server:      node.Server,
		Port:        node.Port,
		Credentials: node.Credentials,
		Active:      node.Active,
	}}}
	result := make(map[domain.CompilerTarget]TargetCompatibility)
	for _, target := range compiler.SortedCapabilities() {
		diagnostics := compiler.ValidateTargetCapabilities(snapshot, target)
		status := "compatible"
		if len(diagnostics) > 0 {
			status = "incompatible"
		}
		result[target] = TargetCompatibility{Status: status, Diagnostics: diagnostics}
	}
	return result
}

func parseErrorEntries(fetch domain.SubscriptionFetch) []domain.SubscriptionEntry {
	if len(fetch.RedactedError) > 64*1024 || !strings.HasPrefix(fetch.RedactedError, "[stage:parse") {
		return nil
	}
	prefix, summary, found := strings.Cut(fetch.RedactedError, "] ")
	if !found || (prefix != "[stage:parse" && !strings.HasPrefix(prefix, "[stage:parse reason_code:")) {
		return nil
	}
	// 当前存量有 reason_code 外层前缀和 DiagnosticSummary 内层前缀，也兼容单层记录
	summary = strings.TrimPrefix(summary, "[stage:parse] ")
	var diagnostic struct {
		Rejected int                 `json:"rejected"`
		Entries  []parser.Diagnostic `json:"entries"`
	}
	// 历史抓取记录可能只有文本错误，只投影结构化的脱敏诊断
	if json.Unmarshal([]byte(summary), &diagnostic) != nil || diagnostic.Rejected < 0 || len(diagnostic.Entries) > 32 {
		return nil
	}
	entries := make([]domain.SubscriptionEntry, 0, len(diagnostic.Entries)+1)
	for i, problem := range diagnostic.Entries {
		problem = safeParseDiagnostic(problem)
		encoded, _ := json.Marshal(problem)
		entries = append(entries, domain.SubscriptionEntry{
			ID:                    fmt.Sprintf("parse_%s_%d", fetch.ID, i),
			SubscriptionID:        fetch.SubscriptionID,
			Ordinal:               problem.Entry,
			RawName:               fmt.Sprintf("%s · entry %d", problem.Format, problem.Entry),
			Protocol:              domain.Protocol(problem.Protocol),
			EntryKind:             domain.EntryKindFormatError,
			ClassificationReason:  fmt.Sprintf("%s: %s (fetch %s)", problem.Reason, problem.Field, fetch.ID),
			ClassificationVersion: "parse-diagnostic-v1",
			SourceProvenanceJSON:  string(encoded),
			CreatedAt:             fetch.StartedAt,
		})
	}
	if remaining := diagnostic.Rejected - len(diagnostic.Entries); remaining > 0 {
		entries = append(entries, domain.SubscriptionEntry{
			ID:                    "parse_" + fetch.ID + "_remaining",
			SubscriptionID:        fetch.SubscriptionID,
			RawName:               fmt.Sprintf("%d additional rejected entries", remaining),
			EntryKind:             domain.EntryKindFormatError,
			ClassificationReason:  fmt.Sprintf("diagnostic detail limit reached (fetch %s)", fetch.ID),
			ClassificationVersion: "parse-diagnostic-v1",
			CreatedAt:             fetch.StartedAt,
		})
	}
	return entries
}

// 历史诊断也只展示解析器拥有的词汇，避免损坏记录把原订阅文本带回界面
func safeParseDiagnostic(problem parser.Diagnostic) parser.Diagnostic {
	allowed := func(value string, vocabulary string) bool {
		return strings.Contains("|"+vocabulary+"|", "|"+value+"|") && value != "" && !strings.Contains(value, "|")
	}
	if !allowed(problem.Format, "yaml|url|text|base64_yaml|base64_url|base64_text|subscription") {
		problem.Format = "subscription"
	}
	if !domain.Protocol(problem.Protocol).IsValid() {
		problem.Protocol = ""
	}
	if !allowed(problem.Reason, "invalid_entry|invalid_document|invalid_field|invalid_payload|invalid_endpoint|invalid_url|unsupported_protocol|missing_credential|missing_field|empty_subscription") {
		problem.Reason = "invalid_entry"
	}
	if !allowed(problem.Field, "entry|document|proxies|plugin_opts|local_address|reserved|mtu|authority|endpoint|url|payload|protocol|cipher|password|uuid|private_key|public_key|logical_id|display_name") {
		problem.Field = "entry"
	}
	if problem.Entry < 0 {
		problem.Entry = 0
	}
	return problem
}
