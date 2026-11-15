package parser_test

import (
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
)

func TestNodeFormatRequiresNormalizedNameAndConnectionFields(t *testing.T) {
	result, err := parser.ExtractWithCredentials([]byte("http://192.0.2.1:8080"))
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("parse fixture: %+v %v", result, err)
	}
	node := result.Items[0].Normalized.Node
	if node.DisplayName != "192.0.2.1:8080" || parser.ValidateNodeFormat(node) != nil {
		t.Fatalf("parser must supply a valid fallback name: %+v", node)
	}
	unnamedYAML, err := parser.ExtractWithCredentials([]byte("proxies:\n - {name: '   ', type: http, server: 192.0.2.1, port: 8080}\n"))
	if err != nil || len(unnamedYAML.Items) != 1 || unnamedYAML.Items[0].Normalized.Node.DisplayName != node.DisplayName {
		t.Fatalf("blank input name must receive the same normalized endpoint name: %+v %v", unnamedYAML, err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*domain.Node)
	}{
		{"display_name", func(n *domain.Node) { n.DisplayName = " \t" }},
		{"logical_id", func(n *domain.Node) { n.LogicalID = "" }},
		{"endpoint", func(n *domain.Node) { n.Server = "" }},
		{"protocol", func(n *domain.Node) { n.Protocol = "invalid" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := node
			tc.mutate(&broken)
			if err := parser.ValidateNodeFormat(broken); err == nil || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("missing global format diagnosis: %v", err)
			}
		})
	}
}
