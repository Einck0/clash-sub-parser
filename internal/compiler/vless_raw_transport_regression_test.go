package compiler_test

import (
	"context"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func TestCompileMihomo_VLESSStoredRawUsesTCPWithoutMutatingIdentityOrCredentials(t *testing.T) {
	node := resolver.ResolvedNode{
		LogicalID:   "synthetic-vless-raw-revision-1",
		DisplayName: "Synthetic VLESS Raw",
		Protocol:    domain.ProtocolVLESS,
		Server:      "vless.synthetic.invalid",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			UUID: "11111111-2222-4333-8444-555555555555",
			Transport: map[string]string{
				"network": "raw",
				"tls":     "true",
				"sni":     "sni.synthetic.invalid",
				"flow":    "xtls-rprx-vision",
				"pbk":     "synthetic-public-key",
				"sid":     "abcd",
			},
		},
		Active: true,
	}
	snapshot := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest: "synthetic-raw-snapshot",
		Nodes:          []resolver.ResolvedNode{node},
	}

	if diagnostics := compiler.ValidateTargetCapabilities(snapshot, domain.TargetMihomo); len(diagnostics) != 0 {
		t.Fatalf("stored VLESS raw must be accepted as TCP, diagnostics: %+v", diagnostics)
	}
	result, err := compiler.CompileMihomo(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("compile stored VLESS raw: %v", err)
	}

	var output struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(result.Content, &output); err != nil {
		t.Fatalf("decode compiled YAML: %v", err)
	}
	if len(output.Proxies) != 1 {
		t.Fatalf("expected one compiled proxy, got %d", len(output.Proxies))
	}
	proxy := output.Proxies[0]
	if proxy["network"] != "tcp" {
		t.Errorf("compiled VLESS network = %v, want tcp", proxy["network"])
	}
	for field, want := range map[string]any{
		"name": "Synthetic VLESS Raw", "server": "vless.synthetic.invalid", "port": 443,
		"uuid": "11111111-2222-4333-8444-555555555555", "tls": true,
		"servername": "sni.synthetic.invalid", "flow": "xtls-rprx-vision",
	} {
		if proxy[field] != want {
			t.Errorf("compiled field %q = %v, want %v", field, proxy[field], want)
		}
	}
	reality, ok := proxy["reality-opts"].(map[string]any)
	if !ok || reality["public-key"] != "synthetic-public-key" || reality["short-id"] != "abcd" {
		t.Errorf("compiled Reality fields changed: %#v", proxy["reality-opts"])
	}
	if node.Credentials.Transport["network"] != "raw" {
		t.Errorf("compiler mutated stored transport network to %q", node.Credentials.Transport["network"])
	}
	if snapshot.Nodes[0].LogicalID != "synthetic-vless-raw-revision-1" {
		t.Errorf("compiler mutated logical identity: %q", snapshot.Nodes[0].LogicalID)
	}
	tcpNode := node
	tcpNode.Credentials.Transport = make(map[string]string, len(node.Credentials.Transport))
	for k, v := range node.Credentials.Transport {
		tcpNode.Credentials.Transport[k] = v
	}
	tcpNode.Credentials.Transport["network"] = "tcp"
	tcpSnapshot := *snapshot
	tcpSnapshot.Nodes = []resolver.ResolvedNode{tcpNode}
	tcpResult, err := compiler.CompileMihomo(context.Background(), &tcpSnapshot)
	if err != nil {
		t.Fatalf("compile stored VLESS tcp: %v", err)
	}
	if string(result.Content) != string(tcpResult.Content) {
		t.Fatal("raw and tcp generated different Mihomo configurations")
	}
}

func TestCompileMihomo_UnknownVLESSTransportRemainsUnsupported(t *testing.T) {
	snapshot := &resolver.ResolvedPolicySnapshot{Nodes: []resolver.ResolvedNode{{
		LogicalID:   "synthetic-vless-unknown-transport",
		Protocol:    domain.ProtocolVLESS,
		DisplayName: "Unknown transport fixture",
		Server:      "vless.synthetic.invalid",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			UUID:      "11111111-2222-4333-8444-555555555555",
			Transport: map[string]string{"network": "unrecognized-transport"},
		},
		Active: true,
	}}}

	diagnostics := compiler.ValidateTargetCapabilities(snapshot, domain.TargetMihomo)
	if len(diagnostics) == 0 {
		t.Fatal("unknown VLESS transport must produce a capability diagnostic")
	}
	found := false
	for _, diagnostic := range diagnostics {
		if strings.Contains(strings.ToLower(diagnostic.Message), "unrecognized-transport") {
			found = true
		}
	}
	if !found {
		t.Fatalf("unknown VLESS transport diagnostic did not identify the value: %+v", diagnostics)
	}
	if _, err := compiler.CompileMihomo(context.Background(), snapshot); err == nil {
		t.Fatal("unknown VLESS transport must not compile")
	}
}
