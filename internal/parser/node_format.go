package parser

import (
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
)

// ValidateNodeFormat 对存量节点复用解析时的必要字段检查，不检查任何目标能力
func ValidateNodeFormat(node domain.Node) error {
	if strings.TrimSpace(node.LogicalID) == "" {
		return parseIssue("missing_field", "logical_id")
	}
	if strings.TrimSpace(node.DisplayName) == "" {
		return parseIssue("missing_field", "display_name")
	}
	if !node.Protocol.IsValid() {
		return parseIssue("invalid_field", "protocol")
	}
	if _, _, err := endpoint(node.Server, strconv.Itoa(node.Port)); err != nil {
		return err
	}
	return validateCredentials(node.Protocol, node.Credentials)
}
