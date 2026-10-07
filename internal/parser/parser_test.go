package parser_test

import (
	"os"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
)

func TestParseYAMLNormalizesSupportedProtocols(t *testing.T) {
	content := readFixture(t, "testdata/clash.yaml")

	result, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Rejected != 0 {
		t.Fatalf("Parse() rejected %d nodes, want 0", result.Rejected)
	}
	if len(result.Nodes) != 7 {
		t.Fatalf("Parse() returned %d nodes, want 7", len(result.Nodes))
	}

	wantProtocols := map[domain.Protocol]bool{
		domain.ProtocolSS: true, domain.ProtocolVMess: true, domain.ProtocolVLESS: true,
		domain.ProtocolTrojan: true, domain.ProtocolHysteria2: true,
		domain.ProtocolWireGuard: true, domain.ProtocolTUIC: true,
	}
	for _, node := range result.Nodes {
		if !wantProtocols[node.Node.Protocol] {
			t.Errorf("unexpected protocol %q", node.Node.Protocol)
		}
		if node.Server == "" || node.Port < 1 || node.Port > 65535 {
			t.Errorf("node %q lacks normalized endpoint: %+v", node.Node.DisplayName, node)
		}
		if node.Node.LogicalID == "" || !domain.IsValidLogicalID(node.Node.LogicalID) {
			t.Errorf("node %q has invalid logical ID %q", node.Node.DisplayName, node.Node.LogicalID)
		}
	}
}

func TestParseBase64SubscriptionToleratesInvalidLines(t *testing.T) {
	content := readFixture(t, "testdata/subscription.base64")

	result, err := parser.Parse(content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(result.Nodes) != 7 {
		t.Fatalf("Parse() returned %d nodes, want 7", len(result.Nodes))
	}
	if result.Rejected != 1 {
		t.Fatalf("Parse() rejected %d lines, want 1", result.Rejected)
	}
}

func TestParseYAMLAndBase64ShareLogicalIDs(t *testing.T) {
	yamlResult, err := parser.Parse(readFixture(t, "testdata/clash.yaml"))
	if err != nil {
		t.Fatalf("Parse(YAML) error = %v", err)
	}
	base64Result, err := parser.Parse(readFixture(t, "testdata/subscription.base64"))
	if err != nil {
		t.Fatalf("Parse(Base64) error = %v", err)
	}

	yamlIDs := map[domain.Protocol]string{}
	for _, node := range yamlResult.Nodes {
		yamlIDs[node.Node.Protocol] = node.Node.LogicalID
	}
	for _, node := range base64Result.Nodes {
		if got, want := node.Node.LogicalID, yamlIDs[node.Node.Protocol]; got != want {
			t.Errorf("%s logical ID differs across formats: got %q, want %q", node.Node.Protocol, got, want)
		}
	}
}

func TestParseLogicalIDDistinguishesCredentialsAndIgnoresNames(t *testing.T) {
	first, err := parser.Parse([]byte("vless://first-secret@edge.example:443?type=ws&security=tls&host=cdn.example&path=%2Frelay#Hong%20Kong"))
	if err != nil {
		t.Fatalf("Parse(first) error = %v", err)
	}
	renamed, err := parser.Parse([]byte("vless://first-secret@EDGE.EXAMPLE:443?path=%2Frelay&host=cdn.example&security=tls&type=ws#Renamed"))
	if err != nil {
		t.Fatalf("Parse(renamed) error = %v", err)
	}
	second, err := parser.Parse([]byte("vless://second-secret@EDGE.EXAMPLE:443?path=%2Frelay&host=cdn.example&security=tls&type=ws#Renamed"))
	if err != nil {
		t.Fatalf("Parse(second) error = %v", err)
	}

	// Same connection and credential with different names / casing / param order MUST produce identical LogicalID
	if first.Nodes[0].Node.LogicalID != renamed.Nodes[0].Node.LogicalID {
		t.Fatalf("logical IDs differ for same connection: %q != %q", first.Nodes[0].Node.LogicalID, renamed.Nodes[0].Node.LogicalID)
	}

	// Distinct credentials on same endpoint MUST produce distinct LogicalIDs
	if first.Nodes[0].Node.LogicalID == second.Nodes[0].Node.LogicalID {
		t.Fatalf("logical IDs must differ for different credentials: %q == %q", first.Nodes[0].Node.LogicalID, second.Nodes[0].Node.LogicalID)
	}
	if first.Nodes[0].Node.Credentials.UUID == second.Nodes[0].Node.Credentials.UUID {
		t.Fatal("distinct credentials must be preserved on parsed nodes")
	}
}

func TestParsePopulatesPlaintextNodeConfiguration(t *testing.T) {
	const credential = "top-secret-password"
	result, err := parser.Parse([]byte("trojan://" + credential + "@secure.example:443?sni=secure.example#Private"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result.Nodes))
	}
	node := result.Nodes[0].Node
	if node.Server != "secure.example" || node.Port != 443 || node.Credentials.Password != credential {
		t.Fatalf("expected plaintext server/port/credentials on Node, got %+v", node)
	}
}

func TestParseRejectsUnsupportedContent(t *testing.T) {
	_, err := parser.Parse([]byte("not a subscription"))
	if err == nil {
		t.Fatal("Parse() error = nil, want invalid content error")
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", name, err)
	}
	return content
}
