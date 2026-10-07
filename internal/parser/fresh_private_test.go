package parser_test

import (
	"os"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/probe/mihomo"
	officialAdapter "github.com/metacubex/mihomo/adapter"
	_ "github.com/metacubex/mihomo/config"
	"gopkg.in/yaml.v3"
)

func TestFreshPrivateSubscriptions_ParseAndMapping(t *testing.T) {
	if os.Getenv("CSP_ALLOW_PRIVATE_RAW") != "1" && os.Getenv("CSP_TEST_REAL_RAW") != "1" {
		t.Skip("skipping private test: requires explicit CSP_ALLOW_PRIVATE_RAW=1 or CSP_TEST_REAL_RAW=1 opt-in")
	}

	// 1. Test einck-qzz.raw
	einckData, err := os.ReadFile("/tmp/csp-fresh-private/einck-qzz.raw")
	if os.IsNotExist(err) {
		t.Skip("skipping private test: /tmp/csp-fresh-private/einck-qzz.raw not present")
	}
	if err != nil {
		t.Fatalf("failed to read einck-qzz.raw: %v", err)
	}

	res, err := parser.Parse(einckData)
	if err != nil {
		t.Fatalf("parser.Parse einck-qzz failed: %v", err)
	}

	if len(res.Nodes) != 24 {
		t.Fatalf("expected 24 parsed nodes from einck-qzz, got %d (rejected: %d)", len(res.Nodes), res.Rejected)
	}
	if res.Rejected != 0 {
		t.Fatalf("expected 0 rejected nodes, got %d", res.Rejected)
	}

	// Protocol counts
	counts := make(map[domain.Protocol]int)
	for _, n := range res.Nodes {
		counts[n.Node.Protocol]++
	}
	if counts[domain.ProtocolHTTP] != 13 {
		t.Errorf("expected 13 HTTP nodes, got %d", counts[domain.ProtocolHTTP])
	}
	if counts[domain.ProtocolSocks5] != 7 {
		t.Errorf("expected 7 SOCKS5 nodes, got %d", counts[domain.ProtocolSocks5])
	}
	if counts[domain.ProtocolVLESS] != 2 {
		t.Errorf("expected 2 VLESS nodes, got %d", counts[domain.ProtocolVLESS])
	}
	if counts[domain.ProtocolAnyTLS] != 2 {
		t.Errorf("expected 2 AnyTLS nodes, got %d", counts[domain.ProtocolAnyTLS])
	}

	// Verify all 24 nodes in einck-qzz have distinct Logical IDs (no collision on same endpoint with different creds)
	uniqueIDs := make(map[string]int)
	for i, n := range res.Nodes {
		id := n.Node.LogicalID
		if !domain.IsValidLogicalID(id) {
			t.Errorf("node [%d] has invalid logical ID: %s", i, id)
		}
		if prevIdx, exists := uniqueIDs[id]; exists {
			t.Errorf("logical ID collision between node [%d] and [%d]: %s", prevIdx, i, id)
		}
		uniqueIDs[id] = i

		// Verify no auth_digest or plain secrets in transport
		if _, hasAuthDigest := n.Transport["auth_digest"]; hasAuthDigest {
			t.Errorf("node [%d] transport leaked auth_digest parameter", i)
		}
		if _, hasAuthDigest := n.Node.Credentials.Transport["auth_digest"]; hasAuthDigest {
			t.Errorf("node [%d] credentials transport leaked auth_digest parameter", i)
		}
	}
	if len(uniqueIDs) != 24 {
		t.Fatalf("expected 24 distinct logical IDs for einck-qzz, got %d", len(uniqueIDs))
	}

	// Specifically verify nodes [8] and [9] (same host zebpay.site:443 with different credentials) have distinct IDs
	if res.Nodes[8].Node.LogicalID == res.Nodes[9].Node.LogicalID {
		t.Fatalf("node [8] and [9] on same endpoint must have distinct IDs, got identical: %s", res.Nodes[8].Node.LogicalID)
	}

	// Verify duplicate config with different display name produces identical Logical ID (clean de-duplication)
	dupNodeYAML := `proxies:
  - name: "Original Node"
    type: http
    server: "duplicate.test"
    port: 8080
    username: "user"
    password: "pass"
  - name: "Renamed Duplicate Node"
    type: http
    server: "duplicate.test"
    port: 8080
    username: "user"
    password: "pass"
`
	dupRes, dupErr := parser.Parse([]byte(dupNodeYAML))
	if dupErr != nil {
		t.Fatalf("dup parse failed: %v", dupErr)
	}
	if len(dupRes.Nodes) != 2 {
		t.Fatalf("expected 2 dup nodes, got %d", len(dupRes.Nodes))
	}
	if dupRes.Nodes[0].Node.LogicalID != dupRes.Nodes[1].Node.LogicalID {
		t.Errorf("expected duplicate configuration with different names to produce identical Logical ID: %s != %s",
			dupRes.Nodes[0].Node.LogicalID, dupRes.Nodes[1].Node.LogicalID)
	}

	// Verify each parsed node can be converted to Mihomo mapping and parsed by official adapter.ParseProxy
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(einckData, &doc); err != nil {
		t.Fatalf("unmarshal raw einck yaml failed: %v", err)
	}

	for i, n := range res.Nodes {
		// Test official parse on raw YAML map
		rawProxy := doc.Proxies[i]
		officialProxy, offErr := officialAdapter.ParseProxy(rawProxy)
		if offErr != nil {
			t.Errorf("official ParseProxy on raw [%d] failed: %v", i, offErr)
			continue
		}

		// Test CSP NodeToMapping -> adapter.ParseProxy
		mapping, mapErr := mihomo.NodeToMapping(n.Node)
		if mapErr != nil {
			t.Errorf("NodeToMapping [%d] failed: %v", i, mapErr)
			continue
		}
		cspProxy, cspErr := officialAdapter.ParseProxy(mapping)
		if cspErr != nil {
			t.Errorf("official ParseProxy on CSP mapping [%d] failed: %v", i, cspErr)
			continue
		}

		// Verify types match
		if officialProxy.Type() != cspProxy.Type() {
			t.Errorf("adapter type mismatch at [%d]: official=%s csp=%s", i, officialProxy.Type(), cspProxy.Type())
		}
	}

	// 2. Test Dogegg.raw
	dogeggData, err := os.ReadFile("/tmp/csp-fresh-private/Dogegg.raw")
	if err == nil {
		dogRes, err := parser.Parse(dogeggData)
		if err != nil {
			t.Fatalf("parser.Parse Dogegg failed: %v", err)
		}
		if len(dogRes.Nodes) != 18 {
			t.Fatalf("expected 18 parsed nodes from Dogegg, got %d", len(dogRes.Nodes))
		}

		// Notice entries [0], [1], [2] share the same endpoint and config, so their LogicalIDs are identical
		if dogRes.Nodes[0].Node.LogicalID != dogRes.Nodes[1].Node.LogicalID ||
			dogRes.Nodes[0].Node.LogicalID != dogRes.Nodes[2].Node.LogicalID {
			t.Errorf("expected Dogegg notice pseudo-nodes [0], [1], [2] to share identical Logical ID: %s, %s, %s",
				dogRes.Nodes[0].Node.LogicalID, dogRes.Nodes[1].Node.LogicalID, dogRes.Nodes[2].Node.LogicalID)
		}

		// 15 proxy nodes (indices 3..17) must have 15 distinct IDs
		proxyIDs := make(map[string]int)
		for idx := 3; idx < 18; idx++ {
			id := dogRes.Nodes[idx].Node.LogicalID
			if prev, exists := proxyIDs[id]; exists {
				t.Errorf("collision among proxy nodes [%d] and [%d]: %s", prev, idx, id)
			}
			proxyIDs[id] = idx
		}
		if len(proxyIDs) != 15 {
			t.Errorf("expected 15 distinct proxy node IDs from Dogegg, got %d", len(proxyIDs))
		}

		for i, n := range dogRes.Nodes {
			mapping, mapErr := mihomo.NodeToMapping(n.Node)
			if mapErr != nil {
				t.Errorf("Dogegg NodeToMapping [%d] failed: %v", i, mapErr)
				continue
			}
			_, cspErr := officialAdapter.ParseProxy(mapping)
			if cspErr != nil {
				t.Errorf("Dogegg ParseProxy on CSP mapping [%d] failed: %v", i, cspErr)
			}
		}
	}
}
