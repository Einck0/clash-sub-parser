package compiler_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// helper to build a baseline valid node for each protocol
func makeTestNode(logicalID, displayName string, proto domain.Protocol, pos int) resolver.ResolvedNode {
	node := resolver.ResolvedNode{
		LogicalID:   logicalID,
		DisplayName: displayName,
		Protocol:    proto,
		Server:      "198.51.100.1",
		Port:        8443,
		Active:      true,
		Position:    pos,
	}

	switch proto {
	case domain.ProtocolSS:
		node.Port = 8388
		node.Credentials = domain.InboundProtocolCredential{
			Method:   "aes-256-gcm",
			Password: "ss-secret-password-1234",
		}
	case domain.ProtocolVMess:
		node.Credentials = domain.InboundProtocolCredential{
			UUID:    "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
			Method:  "auto",
			AlterID: 0,
			Transport: map[string]string{
				"network": "ws",
				"path":    "/vmess",
				"tls":     "true",
			},
		}
	case domain.ProtocolVLESS:
		node.Credentials = domain.InboundProtocolCredential{
			UUID: "b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e",
			Transport: map[string]string{
				"network": "tcp",
				"tls":     "true",
				"flow":    "xtls-rprx-vision",
				"pbk":     "jNXHt1yRo0vD5_1N6p2W3x4Y5z6A7b8C9d0E1f2G3h4",
				"sid":     "0123456789abcdef",
			},
		}
	case domain.ProtocolTrojan:
		node.Credentials = domain.InboundProtocolCredential{
			Password: "trojan-secret-pass-5678",
			Transport: map[string]string{
				"network": "tcp",
				"tls":     "true",
				"sni":     "trojan.example.com",
			},
		}
	case domain.ProtocolHysteria2:
		node.Credentials = domain.InboundProtocolCredential{
			Password: "hy2-secret-pass-9012",
			Transport: map[string]string{
				"network": "udp",
				"sni":     "hy2.example.com",
			},
		}
	case domain.ProtocolWireGuard:
		node.Port = 51820
		node.Credentials = domain.InboundProtocolCredential{
			PrivateKey:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			PublicKey:    "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
			PreSharedKey: "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=",
			LocalAddress: []string{"10.0.0.2/32"},
			Reserved:     []uint8{1, 2, 3},
			MTU:          1420,
		}
	case domain.ProtocolTUIC:
		node.Credentials = domain.InboundProtocolCredential{
			UUID:     "c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f",
			Password: "tuic-secret-pass-3456",
			Transport: map[string]string{
				"network": "udp",
				"sni":     "tuic.example.com",
			},
		}
	}

	return node
}

func makeAdaptationSnapshot(nodes []resolver.ResolvedNode) *resolver.ResolvedPolicySnapshot {
	nodeIDs := make([]string, len(nodes))
	members := make([]resolver.ResolvedGroupMember, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.LogicalID
		members[i] = resolver.ResolvedGroupMember{
			Kind:        resolver.MemberKindNode,
			TargetID:    n.LogicalID,
			DisplayName: n.DisplayName,
			Position:    i,
		}
	}

	return &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "test-snapshot-digest-1234",
		CompilerVersion: "1.0.0",
		Nodes:           nodes,
		NodeLogicalIDs:  nodeIDs,
		Groups: []resolver.ResolvedGroup{
			{
				ID:                "group-select-id",
				Name:              "ProxySelect",
				GroupType:         domain.GroupTypeSelect,
				Members:           members,
				NodeLogicalIDs:    nodeIDs,
				AllNodeLogicalIDs: nodeIDs,
				Position:          0,
			},
		},
		Rules: []resolver.ResolvedRule{
			{
				ID:              "rule-terminal",
				TargetGroupID:   "group-select-id",
				TargetGroupName: "ProxySelect",
				Expression:      "MATCH",
				Position:        0,
				IsTerminal:      true,
			},
		},
	}
}

// TestTargetAdaptation_PerNodeRenderability_TransportErrors verifies native renderer failure detection
// for target-specific transport incompatibilities and protocol constraints.
func TestTargetAdaptation_PerNodeRenderability_TransportErrors(t *testing.T) {
	tests := []struct {
		name          string
		target        domain.CompilerTarget
		node          resolver.ResolvedNode
		wantAdmitted  bool
		reasonSubstr  string
		reasonSubstrs []string
	}{
		// Surge transport tests
		{
			name:         "Surge rejects VLESS protocol",
			target:       domain.TargetSurge,
			node:         makeTestNode("id-vless", "VLESS-Node", domain.ProtocolVLESS, 0),
			wantAdmitted: false,
			reasonSubstr: "does not support protocol vless",
		},
		{
			name:   "Surge rejects SS with grpc transport",
			target: domain.TargetSurge,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-ss-grpc", "SS-GRPC", domain.ProtocolSS, 0)
				n.Credentials.Transport = map[string]string{"network": "grpc"}
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "unsupported network \"grpc\" in shadowsocks transport",
			reasonSubstrs: []string{
				"unsupported network \"grpc\" in shadowsocks transport",
				"grpc transport is not supported in shadowsocks for surge",
			},
		},
		{
			name:   "Surge rejects VMess with reality transport",
			target: domain.TargetSurge,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-vmess-reality", "VMess-Reality", domain.ProtocolVMess, 0)
				n.Credentials.Transport = map[string]string{
					"network": "tcp",
					"tls":     "true",
					"pbk":     "reality-public-key-here",
				}
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "reality or xtls flow transport is not supported by surge",
		},
		{
			name:   "Surge rejects VMess with grpc transport",
			target: domain.TargetSurge,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-vmess-grpc", "VMess-GRPC", domain.ProtocolVMess, 0)
				n.Credentials.Transport = map[string]string{
					"network":      "grpc",
					"service_name": "test-svc",
				}
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "unsupported network \"grpc\" in vmess transport",
			reasonSubstrs: []string{
				"unsupported network \"grpc\" in vmess transport",
				"grpc service_name is not supported in vmess transport for surge",
			},
		},
		{
			name:   "Surge rejects Hysteria2 with invalid network",
			target: domain.TargetSurge,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-hy2-badnet", "Hy2-BadNet", domain.ProtocolHysteria2, 0)
				n.Credentials.Transport = map[string]string{"network": "invalid-net"}
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "unsupported network \"invalid-net\" in hysteria2 transport",
		},
		{
			name:         "Surge admits valid SS node",
			target:       domain.TargetSurge,
			node:         makeTestNode("id-ss-ok", "SS-OK", domain.ProtocolSS, 0),
			wantAdmitted: true,
		},
		{
			name:         "Surge admits valid VMess ws node",
			target:       domain.TargetSurge,
			node:         makeTestNode("id-vmess-ok", "VMess-OK", domain.ProtocolVMess, 0),
			wantAdmitted: true,
		},

		// Quantumult X transport tests
		{
			name:         "Quantumult X rejects VLESS protocol",
			target:       domain.TargetQuantumultX,
			node:         makeTestNode("id-qx-vless", "QX-VLESS", domain.ProtocolVLESS, 0),
			wantAdmitted: false,
			reasonSubstr: "does not support protocol vless",
		},
		{
			name:         "Quantumult X rejects Hysteria2 protocol",
			target:       domain.TargetQuantumultX,
			node:         makeTestNode("id-qx-hy2", "QX-Hy2", domain.ProtocolHysteria2, 0),
			wantAdmitted: false,
			reasonSubstr: "does not support protocol hysteria2",
		},
		{
			name:   "Quantumult X rejects VMess with reality transport",
			target: domain.TargetQuantumultX,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-qx-vmess-reality", "QX-VMess-Reality", domain.ProtocolVMess, 0)
				n.Credentials.Transport = map[string]string{
					"network": "tcp",
					"tls":     "true",
					"pbk":     "some-public-key-value",
					"sid":     "12345678",
				}
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "reality transport is not supported by Quantumult X",
		},
		{
			name:   "Quantumult X rejects VMess with grpc transport",
			target: domain.TargetQuantumultX,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-qx-vmess-grpc", "QX-VMess-GRPC", domain.ProtocolVMess, 0)
				n.Credentials.Transport = map[string]string{
					"network":      "tcp",
					"service_name": "qx-service",
				}
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "grpc service_name is not supported by Quantumult X",
		},
		{
			name:   "Quantumult X rejects SS with unsupported cipher",
			target: domain.TargetQuantumultX,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-qx-ss-badcipher", "QX-SS-BadCipher", domain.ProtocolSS, 0)
				n.Credentials.Method = "unsupported-cipher-algorithm"
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "unsupported cipher in shadowsocks credentials",
		},
		{
			name:         "Quantumult X admits valid SS node",
			target:       domain.TargetQuantumultX,
			node:         makeTestNode("id-qx-ss-ok", "QX-SS-OK", domain.ProtocolSS, 0),
			wantAdmitted: true,
		},

		// Sing-Box transport tests
		{
			name:   "Sing-Box rejects VLESS with xhttp transport",
			target: domain.TargetSingBox,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-sb-xhttp", "SingBox-xhttp", domain.ProtocolVLESS, 0)
				n.Credentials.Transport = map[string]string{"network": "xhttp"}
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "sing-box does not support xhttp transport protocol",
		},
		{
			name:         "Sing-Box admits valid VLESS reality node",
			target:       domain.TargetSingBox,
			node:         makeTestNode("id-sb-vless-ok", "SingBox-VLESS-OK", domain.ProtocolVLESS, 0),
			wantAdmitted: true,
		},

		// Mihomo transport tests
		{
			name:   "Mihomo rejects VMess with unsupported network",
			target: domain.TargetMihomo,
			node: func() resolver.ResolvedNode {
				n := makeTestNode("id-mihomo-badnet", "Mihomo-BadNet", domain.ProtocolVMess, 0)
				n.Credentials.Transport = map[string]string{"network": "ftp"}
				return n
			}(),
			wantAdmitted: false,
			reasonSubstr: "unsupported network \"ftp\" in vmess transport",
		},
		{
			name:         "Mihomo admits valid VLESS reality node",
			target:       domain.TargetMihomo,
			node:         makeTestNode("id-mihomo-vless-ok", "Mihomo-VLESS-OK", domain.ProtocolVLESS, 0),
			wantAdmitted: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snap := makeAdaptationSnapshot([]resolver.ResolvedNode{tc.node})

			matchReason := func(msg string) bool {
				if tc.reasonSubstr != "" && strings.Contains(msg, tc.reasonSubstr) {
					return true
				}
				for _, sub := range tc.reasonSubstrs {
					if sub != "" && strings.Contains(msg, sub) {
						return true
					}
				}
				return false
			}

			// 1. Check FilterCompatibleSnapshot
			filtered, excluded := compiler.FilterCompatibleSnapshot(snap, tc.target)
			if tc.wantAdmitted {
				if len(filtered.Nodes) != 1 {
					t.Fatalf("expected node to be admitted, got %d admitted nodes", len(filtered.Nodes))
				}
				if len(excluded) != 0 {
					t.Fatalf("expected 0 excluded nodes, got %+v", excluded)
				}
			} else {
				if len(filtered.Nodes) != 0 {
					t.Fatalf("expected node to be excluded, got %d admitted nodes", len(filtered.Nodes))
				}
				if len(excluded) != 1 {
					t.Fatalf("expected 1 excluded node, got %d", len(excluded))
				}
				if excluded[0].Code != "unsupported_target_capability" {
					t.Errorf("expected exclusion code 'unsupported_target_capability', got %q", excluded[0].Code)
				}
				if !matchReason(excluded[0].Reason) {
					t.Errorf("expected exclusion reason to contain %q or %v, got %q", tc.reasonSubstr, tc.reasonSubstrs, excluded[0].Reason)
				}
			}

			// 2. Check ValidateTargetCapabilities (shared canonical decision)
			diags := compiler.ValidateTargetCapabilities(snap, tc.target)
			if tc.wantAdmitted {
				for _, d := range diags {
					if d.NodeID == tc.node.LogicalID {
						t.Errorf("unexpected diagnostic for admitted node: %+v", d)
					}
				}
			} else {
				found := false
				for _, d := range diags {
					if d.NodeID == tc.node.LogicalID && matchReason(d.Message) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected diagnostic containing %q or %v for node %s, got diags: %+v", tc.reasonSubstr, tc.reasonSubstrs, tc.node.LogicalID, diags)
				}
			}
		})
	}
}

// TestTargetAdaptation_DuplicateNames_FirstRenderableWins tests that duplicate node display names
// are handled deterministically: the first renderable node wins, subsequent duplicates are excluded,
// and unrenderable nodes do not claim names.
func TestTargetAdaptation_DuplicateNames_FirstRenderableWins(t *testing.T) {
	ctx := context.Background()

	t.Run("two renderable nodes with same name: first wins, second excluded", func(t *testing.T) {
		node1 := makeTestNode("id-ss-1", "Proxy-Shared", domain.ProtocolSS, 0)
		node2 := makeTestNode("id-ss-2", "Proxy-Shared", domain.ProtocolSS, 1)

		snap := makeAdaptationSnapshot([]resolver.ResolvedNode{node1, node2})

		// Compatible mode: node1 admitted, node2 excluded as duplicate
		filtered, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetSurge)
		if len(filtered.Nodes) != 1 {
			t.Fatalf("expected exactly 1 admitted node, got %d", len(filtered.Nodes))
		}
		if filtered.Nodes[0].LogicalID != "id-ss-1" {
			t.Errorf("expected first node to win, got %s", filtered.Nodes[0].LogicalID)
		}
		if len(excluded) != 1 {
			t.Fatalf("expected exactly 1 excluded node, got %d", len(excluded))
		}
		if excluded[0].NodeID != "id-ss-2" {
			t.Errorf("expected second node to be excluded, got %s", excluded[0].NodeID)
		}
		if !strings.Contains(excluded[0].Reason, "duplicate node display name") {
			t.Errorf("expected duplicate name reason, got %q", excluded[0].Reason)
		}

		// ValidateTargetCapabilities reports duplicate
		diags := compiler.ValidateTargetCapabilities(snap, domain.TargetSurge)
		foundDupDiag := false
		for _, d := range diags {
			if d.NodeID == "id-ss-2" && strings.Contains(d.Message, "duplicate node display name") {
				foundDupDiag = true
				break
			}
		}
		if !foundDupDiag {
			t.Errorf("expected duplicate diagnostic for node2 in ValidateTargetCapabilities, got: %+v", diags)
		}

		// Strict Compile must fail closed
		_, err := compiler.Compile(ctx, snap, domain.TargetSurge)
		if err == nil {
			t.Fatal("expected strict Compile to fail on duplicate node display names")
		}
		if !strings.Contains(err.Error(), "duplicate node display name") {
			t.Errorf("expected duplicate display name in error, got %v", err)
		}
	})

	t.Run("unrenderable node first: does not claim name, subsequent renderable node wins", func(t *testing.T) {
		// Node 1 is VLESS (unrenderable on Surge) with name "HK-Node"
		node1 := makeTestNode("id-vless-hk", "HK-Node", domain.ProtocolVLESS, 0)
		// Node 2 is SS (renderable on Surge) with same name "HK-Node"
		node2 := makeTestNode("id-ss-hk", "HK-Node", domain.ProtocolSS, 1)
		// Node 3 is SS (renderable on Surge) with same name "HK-Node"
		node3 := makeTestNode("id-ss-hk-3", "HK-Node", domain.ProtocolSS, 2)

		snap := makeAdaptationSnapshot([]resolver.ResolvedNode{node1, node2, node3})

		filtered, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetSurge)

		// Node 2 should be the sole admitted node!
		if len(filtered.Nodes) != 1 {
			t.Fatalf("expected 1 admitted node, got %d", len(filtered.Nodes))
		}
		if filtered.Nodes[0].LogicalID != "id-ss-hk" {
			t.Errorf("expected node2 (id-ss-hk) to win, got %s", filtered.Nodes[0].LogicalID)
		}

		if len(excluded) != 2 {
			t.Fatalf("expected 2 excluded nodes, got %d", len(excluded))
		}

		// First excluded: node1 because of protocol (not duplicate)
		if excluded[0].NodeID != "id-vless-hk" {
			t.Errorf("expected first excluded to be id-vless-hk, got %s", excluded[0].NodeID)
		}
		if !strings.Contains(excluded[0].Reason, "does not support protocol") {
			t.Errorf("expected protocol reason for node1, got %q", excluded[0].Reason)
		}

		// Second excluded: node3 because it's a duplicate of node2
		if excluded[1].NodeID != "id-ss-hk-3" {
			t.Errorf("expected second excluded to be id-ss-hk-3, got %s", excluded[1].NodeID)
		}
		if !strings.Contains(excluded[1].Reason, "duplicate node display name") {
			t.Errorf("expected duplicate name reason for node3, got %q", excluded[1].Reason)
		}
	})
}

// TestTargetAdaptation_DialerProxyRejection tests that non-Mihomo dialer proxy nodes are rejected
// while Mihomo preserves dialer proxy nodes without error.
func TestTargetAdaptation_DialerProxyRejection(t *testing.T) {
	ctx := context.Background()

	nodeWithDialer := makeTestNode("id-dialer-node", "Dialer-Node", domain.ProtocolSS, 0)
	nodeWithDialer.DialerProxy = "parent-proxy-chain"

	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{nodeWithDialer})

	nonMihomoTargets := []domain.CompilerTarget{
		domain.TargetSurge,
		domain.TargetSingBox,
		domain.TargetQuantumultX,
	}

	for _, target := range nonMihomoTargets {
		t.Run(fmt.Sprintf("%s rejects non-empty DialerProxy", target), func(t *testing.T) {
			// Compatible mode: excluded
			filtered, excluded := compiler.FilterCompatibleSnapshot(snap, target)
			if len(filtered.Nodes) != 0 {
				t.Errorf("expected 0 admitted nodes on %s, got %d", target, len(filtered.Nodes))
			}
			if len(excluded) != 1 {
				t.Fatalf("expected 1 excluded node on %s, got %d", target, len(excluded))
			}
			if !strings.Contains(excluded[0].Reason, "dialer-proxy") {
				t.Errorf("expected dialer-proxy in exclusion reason on %s, got %q", target, excluded[0].Reason)
			}

			// ValidateTargetCapabilities reports diagnostic
			diags := compiler.ValidateTargetCapabilities(snap, target)
			found := false
			for _, d := range diags {
				if d.NodeID == "id-dialer-node" && strings.Contains(d.Message, "dialer-proxy") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected dialer-proxy diagnostic on %s, got: %+v", target, diags)
			}

			// Strict Compile fails closed
			_, err := compiler.Compile(ctx, snap, target)
			if err == nil {
				t.Errorf("expected Compile to fail on %s with DialerProxy", target)
			}
		})
	}

	t.Run("Mihomo accepts non-empty DialerProxy", func(t *testing.T) {
		filtered, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
		if len(filtered.Nodes) != 1 {
			t.Fatalf("expected 1 admitted node on Mihomo, got %d", len(filtered.Nodes))
		}
		if len(excluded) != 0 {
			t.Fatalf("expected 0 excluded nodes on Mihomo, got %+v", excluded)
		}
		if filtered.Nodes[0].DialerProxy != "parent-proxy-chain" {
			t.Errorf("expected DialerProxy preserved on Mihomo, got %q", filtered.Nodes[0].DialerProxy)
		}

		diags := compiler.ValidateTargetCapabilities(snap, domain.TargetMihomo)
		for _, d := range diags {
			if strings.Contains(d.Message, "dialer-proxy") {
				t.Errorf("unexpected dialer-proxy diagnostic on Mihomo: %+v", d)
			}
		}

		res, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
		if err != nil {
			t.Fatalf("expected Mihomo Compile to succeed with DialerProxy, got %v", err)
		}
		if !strings.Contains(string(res.Content), "dialer-proxy: parent-proxy-chain") {
			t.Errorf("expected dialer-proxy in rendered Mihomo output, got:\n%s", string(res.Content))
		}
	})
}

// TestTargetAdaptation_AllExclusions tests that when all inventory nodes are incompatible with a target,
// FilterCompatibleSnapshot returns an empty admitted slice and complete exclusions list without modifying input.
func TestTargetAdaptation_AllExclusions(t *testing.T) {
	node1 := makeTestNode("id-vless-1", "VLESS-1", domain.ProtocolVLESS, 0)
	node2 := makeTestNode("id-vless-2", "VLESS-2", domain.ProtocolVLESS, 1)
	node3 := makeTestNode("id-vless-3", "VLESS-3", domain.ProtocolVLESS, 2)

	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{node1, node2, node3})

	filtered, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetSurge)

	if len(filtered.Nodes) != 0 {
		t.Fatalf("expected 0 admitted nodes, got %d", len(filtered.Nodes))
	}
	if len(filtered.NodeLogicalIDs) != 0 {
		t.Fatalf("expected 0 NodeLogicalIDs, got %d", len(filtered.NodeLogicalIDs))
	}
	if len(excluded) != 3 {
		t.Fatalf("expected 3 exclusions, got %d", len(excluded))
	}
	for i, exc := range excluded {
		if exc.Code != "unsupported_target_capability" {
			t.Errorf("exclusion %d code: got %s, want unsupported_target_capability", i, exc.Code)
		}
		if !strings.Contains(exc.Reason, "does not support protocol vless") {
			t.Errorf("exclusion %d reason: got %s", i, exc.Reason)
		}
	}

	// Verify group derived memberships in filtered snapshot are cleaned
	if len(filtered.Groups) > 0 {
		g := filtered.Groups[0]
		if len(g.Members) != 0 {
			t.Errorf("expected 0 group members, got %d", len(g.Members))
		}
		if len(g.NodeLogicalIDs) != 0 {
			t.Errorf("expected 0 group NodeLogicalIDs, got %d", len(g.NodeLogicalIDs))
		}
		if len(g.AllNodeLogicalIDs) != 0 {
			t.Errorf("expected 0 group AllNodeLogicalIDs, got %d", len(g.AllNodeLogicalIDs))
		}
	}

	// Verify original snapshot is untouched
	if len(snap.Nodes) != 3 {
		t.Fatalf("original snapshot nodes modified: got %d, want 3", len(snap.Nodes))
	}
	if len(snap.Groups[0].Members) != 3 {
		t.Fatalf("original snapshot group members modified: got %d, want 3", len(snap.Groups[0].Members))
	}
}

// TestTargetAdaptation_SnapshotImmutability verifies that calling FilterCompatibleSnapshot
// or ValidateTargetCapabilities does not mutate the input snapshot or any of its child slices.
func TestTargetAdaptation_SnapshotImmutability(t *testing.T) {
	node1 := makeTestNode("id-ss", "SS-Node", domain.ProtocolSS, 0)
	node2 := makeTestNode("id-vless", "VLESS-Node", domain.ProtocolVLESS, 1)

	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{node1, node2})

	// Record original addresses and values
	origNodeCount := len(snap.Nodes)
	origNodeIDsCount := len(snap.NodeLogicalIDs)
	origGroupMemberCount := len(snap.Groups[0].Members)
	origGroupNodeIDsCount := len(snap.Groups[0].NodeLogicalIDs)
	origRuleCount := len(snap.Rules)
	origDigest := snap.SnapshotDigest

	// Run FilterCompatibleSnapshot for Surge (which excludes VLESS)
	filtered, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetSurge)

	if len(snap.Nodes) != origNodeCount {
		t.Errorf("snap.Nodes modified: got %d, want %d", len(snap.Nodes), origNodeCount)
	}
	if len(snap.NodeLogicalIDs) != origNodeIDsCount {
		t.Errorf("snap.NodeLogicalIDs modified: got %d, want %d", len(snap.NodeLogicalIDs), origNodeIDsCount)
	}
	if len(snap.Groups[0].Members) != origGroupMemberCount {
		t.Errorf("snap.Groups[0].Members modified: got %d, want %d", len(snap.Groups[0].Members), origGroupMemberCount)
	}
	if len(snap.Groups[0].NodeLogicalIDs) != origGroupNodeIDsCount {
		t.Errorf("snap.Groups[0].NodeLogicalIDs modified: got %d, want %d", len(snap.Groups[0].NodeLogicalIDs), origGroupNodeIDsCount)
	}
	if len(snap.Rules) != origRuleCount {
		t.Errorf("snap.Rules modified: got %d, want %d", len(snap.Rules), origRuleCount)
	}
	if snap.SnapshotDigest != origDigest {
		t.Errorf("snap.SnapshotDigest modified: got %s, want %s", snap.SnapshotDigest, origDigest)
	}

	// Verify filtered snapshot is distinct and correctly isolated
	if len(filtered.Nodes) != 1 {
		t.Fatalf("filtered.Nodes length: got %d, want 1", len(filtered.Nodes))
	}
	if len(excluded) != 1 {
		t.Fatalf("excluded length: got %d, want 1", len(excluded))
	}

	// ValidateTargetCapabilities does not mutate
	_ = compiler.ValidateTargetCapabilities(snap, domain.TargetSurge)
	if len(snap.Nodes) != origNodeCount || len(snap.Groups[0].Members) != origGroupMemberCount {
		t.Fatal("ValidateTargetCapabilities mutated input snapshot")
	}
}

// TestTargetAdaptation_DerivedGroupMembershipConsistency verifies that group members,
// child group references, builtin references, and derived ID slices remain consistent.
func TestTargetAdaptation_DerivedGroupMembershipConsistency(t *testing.T) {
	nodeSS := makeTestNode("id-ss", "SS-Node", domain.ProtocolSS, 0)
	nodeVLESS := makeTestNode("id-vless", "VLESS-Node", domain.ProtocolVLESS, 1)

	snap := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "digest-derived-consistency",
		CompilerVersion: "1.0.0",
		Nodes:           []resolver.ResolvedNode{nodeSS, nodeVLESS},
		NodeLogicalIDs:  []string{"id-ss", "id-vless"},
		Groups: []resolver.ResolvedGroup{
			{
				ID:        "grp-main",
				Name:      "MainGroup",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "id-ss", DisplayName: "SS-Node", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "id-vless", DisplayName: "VLESS-Node", Position: 1},
					{Kind: resolver.MemberKindGroup, TargetID: "grp-child", DisplayName: "ChildGroup", Position: 2},
					{Kind: resolver.MemberKindBuiltin, DisplayName: "DIRECT", Position: 3},
				},
				ChildGroupIDs:     []string{"grp-child"},
				NodeLogicalIDs:    []string{"id-ss", "id-vless"},
				AllNodeLogicalIDs: []string{"id-ss", "id-vless"},
				Position:          0,
			},
		},
	}

	// Filter for Surge: VLESS is excluded, SS is kept
	filtered, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetSurge)
	if len(excluded) != 1 || excluded[0].NodeID != "id-vless" {
		t.Fatalf("expected VLESS to be excluded, got %+v", excluded)
	}

	if len(filtered.Nodes) != 1 || filtered.Nodes[0].LogicalID != "id-ss" {
		t.Fatalf("expected only SS node admitted, got %+v", filtered.Nodes)
	}
	if filtered.Nodes[0].Position != 0 {
		t.Errorf("expected admitted node position to be re-indexed to 0, got %d", filtered.Nodes[0].Position)
	}

	g := filtered.Groups[0]
	// MemberKindNode for VLESS removed; SS kept; Group and Builtin members preserved
	if len(g.Members) != 3 {
		t.Fatalf("expected 3 group members, got %d: %+v", len(g.Members), g.Members)
	}
	if g.Members[0].TargetID != "id-ss" {
		t.Errorf("member[0] expected id-ss, got %s", g.Members[0].TargetID)
	}
	if g.Members[1].Kind != resolver.MemberKindGroup || g.Members[1].DisplayName != "ChildGroup" {
		t.Errorf("member[1] expected ChildGroup, got %+v", g.Members[1])
	}
	if g.Members[2].Kind != resolver.MemberKindBuiltin || g.Members[2].DisplayName != "DIRECT" {
		t.Errorf("member[2] expected DIRECT, got %+v", g.Members[2])
	}

	// Derived ID slices must contain only admitted node IDs
	if len(g.NodeLogicalIDs) != 1 || g.NodeLogicalIDs[0] != "id-ss" {
		t.Errorf("expected g.NodeLogicalIDs == ['id-ss'], got %+v", g.NodeLogicalIDs)
	}
	if len(g.AllNodeLogicalIDs) != 1 || g.AllNodeLogicalIDs[0] != "id-ss" {
		t.Errorf("expected g.AllNodeLogicalIDs == ['id-ss'], got %+v", g.AllNodeLogicalIDs)
	}
	if len(filtered.NodeLogicalIDs) != 1 || filtered.NodeLogicalIDs[0] != "id-ss" {
		t.Errorf("expected filtered.NodeLogicalIDs == ['id-ss'], got %+v", filtered.NodeLogicalIDs)
	}
}

// TestTargetAdaptation_CrossTargetMatrix verifies compatible filtering and diagnostics
// across all 4 targets for a comprehensive mixed inventory.
func TestTargetAdaptation_CrossTargetMatrix(t *testing.T) {
	nodes := []resolver.ResolvedNode{
		makeTestNode("id-ss", "Node-SS", domain.ProtocolSS, 0),
		makeTestNode("id-vmess", "Node-VMess", domain.ProtocolVMess, 1),
		makeTestNode("id-vless", "Node-VLESS", domain.ProtocolVLESS, 2),
		makeTestNode("id-trojan", "Node-Trojan", domain.ProtocolTrojan, 3),
		makeTestNode("id-hy2", "Node-Hy2", domain.ProtocolHysteria2, 4),
		makeTestNode("id-wg", "Node-WG", domain.ProtocolWireGuard, 5),
		makeTestNode("id-tuic", "Node-TUIC", domain.ProtocolTUIC, 6),
	}

	snap := makeAdaptationSnapshot(nodes)

	matrix := []struct {
		target          domain.CompilerTarget
		wantAdmitted    int
		wantExcluded    int
		excludedNodeIDs []string
	}{
		{
			target:          domain.TargetMihomo,
			wantAdmitted:    7,
			wantExcluded:    0,
			excludedNodeIDs: nil,
		},
		{
			target:          domain.TargetSingBox,
			wantAdmitted:    7,
			wantExcluded:    0,
			excludedNodeIDs: nil,
		},
		{
			target:          domain.TargetSurge,
			wantAdmitted:    6,
			wantExcluded:    1,
			excludedNodeIDs: []string{"id-vless"},
		},
		{
			target:          domain.TargetQuantumultX,
			wantAdmitted:    3, // SS, VMess, Trojan
			wantExcluded:    4, // VLESS, Hy2, WG, TUIC
			excludedNodeIDs: []string{"id-vless", "id-hy2", "id-wg", "id-tuic"},
		},
	}

	for _, tc := range matrix {
		t.Run(string(tc.target), func(t *testing.T) {
			filtered, excluded := compiler.FilterCompatibleSnapshot(snap, tc.target)

			if len(filtered.Nodes) != tc.wantAdmitted {
				t.Errorf("admitted count: got %d, want %d", len(filtered.Nodes), tc.wantAdmitted)
			}
			if len(excluded) != tc.wantExcluded {
				t.Fatalf("excluded count: got %d, want %d", len(excluded), tc.wantExcluded)
			}

			excludedMap := make(map[string]bool)
			for _, exc := range excluded {
				excludedMap[exc.NodeID] = true
				if exc.Code != "unsupported_target_capability" {
					t.Errorf("exclusion code: got %q, want unsupported_target_capability", exc.Code)
				}
				if strings.TrimSpace(exc.Reason) == "" {
					t.Errorf("empty exclusion reason for node %s", exc.NodeID)
				}
			}

			for _, wantID := range tc.excludedNodeIDs {
				if !excludedMap[wantID] {
					t.Errorf("expected node %s to be excluded on %s", wantID, tc.target)
				}
			}
		})
	}
}

// TestTargetAdaptation_SafeDiagnostics_NoCredentialsLeaked verifies that error diagnostics
// and exclusion reasons never reveal plaintext passwords, UUIDs, private keys, or credentials,
// including short secrets and transport secrets.
func TestTargetAdaptation_SafeDiagnostics_NoCredentialsLeaked(t *testing.T) {
	secretPassword := "my-super-secret-password-xyz123"
	secretUUID := "88888888-4444-4444-4444-121212121212"
	secretPrivKey := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	secretPSK := "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC="
	secretShortPassword := "pwd"
	secretTransportSecret := "transport-secret-value-777"

	// Node 1: SS with secret password but invalid cipher
	node1 := makeTestNode("id-secret-ss", "Secret-SS", domain.ProtocolSS, 0)
	node1.Credentials.Password = secretPassword
	node1.Credentials.Method = "unsupported-cipher-method"

	// Node 2: VMess with secret UUID but invalid transport option for QX
	node2 := makeTestNode("id-secret-vmess", "Secret-VMess", domain.ProtocolVMess, 1)
	node2.Credentials.UUID = secretUUID
	node2.Credentials.Transport = map[string]string{
		"network": "tcp",
		"tls":     "true",
		"pbk":     "some-reality-pbk",
	}

	// Node 3: WireGuard with secret private key and PSK
	node3 := makeTestNode("id-secret-wg", "Secret-WG", domain.ProtocolWireGuard, 2)
	node3.Credentials.PrivateKey = secretPrivKey
	node3.Credentials.PreSharedKey = secretPSK
	node3.Credentials.LocalAddress = []string{"invalid-cidr"}

	// Node 4: SS with short secret password and transport secret
	node4 := makeTestNode("id-secret-short", "Secret-Short", domain.ProtocolSS, 3)
	node4.Credentials.Password = secretShortPassword
	node4.Credentials.Method = "unsupported-cipher-short"
	node4.Credentials.Transport = map[string]string{
		"obfs-password": secretTransportSecret,
	}

	nodes := []resolver.ResolvedNode{node1, node2, node3, node4}
	snap := makeAdaptationSnapshot(nodes)

	targets := compiler.Targets()
	for _, target := range targets {
		t.Run(string(target), func(t *testing.T) {
			_, excluded := compiler.FilterCompatibleSnapshot(snap, target)
			diags := compiler.ValidateTargetCapabilities(snap, target)

			checkString := func(contextInfo, s string) {
				if strings.Contains(s, secretPassword) {
					t.Errorf("[%s on %s] leaked secretPassword in %q", contextInfo, target, s)
				}
				if strings.Contains(s, secretUUID) {
					t.Errorf("[%s on %s] leaked secretUUID in %q", contextInfo, target, s)
				}
				if strings.Contains(s, secretPrivKey) {
					t.Errorf("[%s on %s] leaked secretPrivKey in %q", contextInfo, target, s)
				}
				if strings.Contains(s, secretPSK) {
					t.Errorf("[%s on %s] leaked secretPSK in %q", contextInfo, target, s)
				}
				if strings.Contains(s, secretShortPassword) {
					t.Errorf("[%s on %s] leaked secretShortPassword in %q", contextInfo, target, s)
				}
				if strings.Contains(s, secretTransportSecret) {
					t.Errorf("[%s on %s] leaked secretTransportSecret in %q", contextInfo, target, s)
				}
			}

			for _, exc := range excluded {
				checkString("exclusion.Reason", exc.Reason)
			}
			for _, d := range diags {
				checkString("diagnostic.Message", d.Message)
			}
		})
	}
}

// TestTargetAdaptation_SSPlugin_NonMihomoRejection tests that Shadowsocks nodes configured with
// plugins or plugin options are rejected for non-Mihomo targets (Surge, SingBox, QuantumultX)
// to prevent silent semantic loss, while plain Shadowsocks nodes are admitted.
func TestTargetAdaptation_SSPlugin_NonMihomoRejection(t *testing.T) {
	ctx := context.Background()

	nonMihomoTargets := []domain.CompilerTarget{
		domain.TargetSurge,
		domain.TargetSingBox,
		domain.TargetQuantumultX,
	}

	plainNode := makeTestNode("id-ss-plain", "SS-Plain", domain.ProtocolSS, 0)
	pluginNode := makeTestNode("id-ss-plugin", "SS-Plugin", domain.ProtocolSS, 1)
	pluginNode.Credentials.Transport = map[string]string{
		"plugin":      "obfs",
		"plugin-opts": "mode=tls;host=bing.com",
	}

	mixedSnap := makeAdaptationSnapshot([]resolver.ResolvedNode{plainNode, pluginNode})

	for _, target := range nonMihomoTargets {
		t.Run(fmt.Sprintf("%s mixed plainSS+pluginSS -> 1 include 1 exclude", target), func(t *testing.T) {
			filtered, excluded := compiler.FilterCompatibleSnapshot(mixedSnap, target)

			// 1 include: plain SS admitted
			if len(filtered.Nodes) != 1 {
				t.Fatalf("[%s] expected 1 admitted node, got %d", target, len(filtered.Nodes))
			}
			if filtered.Nodes[0].LogicalID != plainNode.LogicalID {
				t.Errorf("[%s] expected admitted node %s, got %s", target, plainNode.LogicalID, filtered.Nodes[0].LogicalID)
			}

			// 1 exclude: plugin SS rejected
			if len(excluded) != 1 {
				t.Fatalf("[%s] expected 1 excluded node, got %d", target, len(excluded))
			}
			if excluded[0].NodeID != pluginNode.LogicalID {
				t.Errorf("[%s] expected excluded node %s, got %s", target, pluginNode.LogicalID, excluded[0].NodeID)
			}
			if excluded[0].Code != "unsupported_target_capability" {
				t.Errorf("[%s] expected exclusion code 'unsupported_target_capability', got %q", target, excluded[0].Code)
			}
			if !strings.Contains(excluded[0].Reason, "plugin") {
				t.Errorf("[%s] expected exclusion reason to contain 'plugin', got %q", target, excluded[0].Reason)
			}

			// Strict detects: ValidateTargetCapabilities reports diagnostic for plugin SS, not plain SS
			diags := compiler.ValidateTargetCapabilities(mixedSnap, target)
			foundPluginDiag := false
			for _, d := range diags {
				if d.NodeID == pluginNode.LogicalID {
					foundPluginDiag = true
					if d.Code != "unsupported_target_capability" {
						t.Errorf("[%s] expected diagnostic code 'unsupported_target_capability', got %q", target, d.Code)
					}
					if !strings.Contains(d.Message, "plugin") {
						t.Errorf("[%s] expected diagnostic message to contain 'plugin', got %q", target, d.Message)
					}
				}
				if d.NodeID == plainNode.LogicalID {
					t.Errorf("[%s] unexpected diagnostic for plain SS node: %+v", target, d)
				}
			}
			if !foundPluginDiag {
				t.Errorf("[%s] ValidateTargetCapabilities did not report plugin diagnostic: %+v", target, diags)
			}

			// Strict detects: Compile fails closed on unfiltered mixed snapshot
			_, err := compiler.Compile(ctx, mixedSnap, target)
			if err == nil {
				t.Fatalf("[%s] expected strict Compile to fail on snapshot containing plugin SS", target)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Errorf("[%s] expected *CapabilityError from Compile, got %T: %v", target, err, err)
			} else if !strings.Contains(capErr.Reason, "plugin") {
				t.Errorf("[%s] expected Compile error reason to contain 'plugin', got %q", target, capErr.Reason)
			}
		})
	}

	// All plugin nodes: none admitted
	allPluginNode1 := makeTestNode("id-plugin-1", "Plugin-1", domain.ProtocolSS, 0)
	allPluginNode1.Credentials.Transport = map[string]string{
		"plugin":      "obfs",
		"plugin-opts": "mode=tls;host=bing.com",
	}
	allPluginNode2 := makeTestNode("id-plugin-2", "Plugin-2", domain.ProtocolSS, 1)
	allPluginNode2.Credentials.Transport = map[string]string{
		"plugin":      "v2ray-plugin",
		"plugin_opts": "mode=websocket;host=example.com",
	}
	allPluginSnap := makeAdaptationSnapshot([]resolver.ResolvedNode{allPluginNode1, allPluginNode2})

	for _, target := range nonMihomoTargets {
		t.Run(fmt.Sprintf("%s all plugin none admitted", target), func(t *testing.T) {
			filtered, excluded := compiler.FilterCompatibleSnapshot(allPluginSnap, target)
			if len(filtered.Nodes) != 0 {
				t.Errorf("[%s] expected 0 admitted nodes, got %d", target, len(filtered.Nodes))
			}
			if len(excluded) != 2 {
				t.Fatalf("[%s] expected 2 excluded nodes, got %d", target, len(excluded))
			}
			for i, exc := range excluded {
				if exc.Code != "unsupported_target_capability" {
					t.Errorf("[%s] exclusion %d code: got %s, want unsupported_target_capability", target, i, exc.Code)
				}
				if !strings.Contains(exc.Reason, "plugin") {
					t.Errorf("[%s] exclusion %d reason: got %q, want 'plugin'", target, i, exc.Reason)
				}
			}

			// Strict Compile fails closed
			_, err := compiler.Compile(ctx, allPluginSnap, target)
			if err == nil {
				t.Errorf("[%s] expected Compile to fail on all-plugin snapshot", target)
			}
		})
	}
}

// TestTargetAdaptation_SSPlugin_OptionAliasesAndFailClosed tests that both plugin-opts and
// plugin_opts aliases are rejected, and non-empty options even without plugin fail closed.
func TestTargetAdaptation_SSPlugin_OptionAliasesAndFailClosed(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name      string
		transport map[string]string
	}{
		{
			name: "plugin with plugin-opts hyphenated",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin-opts": "mode=tls;host=bing.com",
			},
		},
		{
			name: "plugin with plugin_opts underscored",
			transport: map[string]string{
				"plugin":      "obfs",
				"plugin_opts": "mode=http;host=example.com",
			},
		},
		{
			name: "plugin-opts only without plugin (fail closed)",
			transport: map[string]string{
				"plugin-opts": "mode=tls;host=bing.com",
			},
		},
		{
			name: "plugin_opts only without plugin (fail closed)",
			transport: map[string]string{
				"plugin_opts": "mode=http;host=example.com",
			},
		},
		{
			name: "plugin only without options (fail closed)",
			transport: map[string]string{
				"plugin": "v2ray-plugin",
			},
		},
		{
			name: "plugin-opts with secret password and host (safe reason check)",
			transport: map[string]string{
				"plugin":      "shadow-tls",
				"plugin-opts": "password=super-secret-shadow-password-999;host=internal-hidden-host.corp",
			},
		},
	}

	targets := []domain.CompilerTarget{
		domain.TargetSurge,
		domain.TargetSingBox,
		domain.TargetQuantumultX,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := makeTestNode("id-test-ss", "SS-Test", domain.ProtocolSS, 0)
			node.Credentials.Transport = tc.transport
			snap := makeAdaptationSnapshot([]resolver.ResolvedNode{node})

			for _, target := range targets {
				filtered, excluded := compiler.FilterCompatibleSnapshot(snap, target)
				if len(filtered.Nodes) != 0 {
					t.Errorf("[%s on %s] expected 0 admitted nodes, got %d", tc.name, target, len(filtered.Nodes))
				}
				if len(excluded) != 1 {
					t.Fatalf("[%s on %s] expected 1 excluded node, got %d", tc.name, target, len(excluded))
				}
				if excluded[0].Code != "unsupported_target_capability" {
					t.Errorf("[%s on %s] expected code 'unsupported_target_capability', got %q", tc.name, target, excluded[0].Code)
				}
				if !strings.Contains(excluded[0].Reason, "plugin") {
					t.Errorf("[%s on %s] expected reason to contain 'plugin', got %q", tc.name, target, excluded[0].Reason)
				}

				// Check no secrets or values leaked in exclusion reason
				if strings.Contains(excluded[0].Reason, "super-secret-shadow-password-999") {
					t.Errorf("[%s on %s] leaked password in reason: %q", tc.name, target, excluded[0].Reason)
				}
				if strings.Contains(excluded[0].Reason, "internal-hidden-host.corp") {
					t.Errorf("[%s on %s] leaked host in reason: %q", tc.name, target, excluded[0].Reason)
				}

				diags := compiler.ValidateTargetCapabilities(snap, target)
				if len(diags) == 0 {
					t.Errorf("[%s on %s] expected capability diagnostic, got 0", tc.name, target)
				}

				_, err := compiler.Compile(ctx, snap, target)
				if err == nil {
					t.Errorf("[%s on %s] expected Compile to fail closed", tc.name, target)
				}
			}
		})
	}
}

// TestTargetAdaptation_SSPlugin_MihomoNativePreservation verifies that Mihomo retains legitimate
// native plugin serialization and admits both plain and plugin Shadowsocks nodes.
func TestTargetAdaptation_SSPlugin_MihomoNativePreservation(t *testing.T) {
	ctx := context.Background()

	plainNode := makeTestNode("id-mihomo-plain-ss", "Mihomo-Plain-SS", domain.ProtocolSS, 0)
	pluginNode := makeTestNode("id-mihomo-plugin-ss", "Mihomo-Plugin-SS", domain.ProtocolSS, 1)
	pluginNode.Credentials.Transport = map[string]string{
		"plugin":      "obfs",
		"plugin-opts": "mode=tls;host=bing.com",
	}

	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{plainNode, pluginNode})

	// 1. FilterCompatibleSnapshot admits both nodes on Mihomo
	filtered, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if len(filtered.Nodes) != 2 {
		t.Fatalf("expected 2 admitted nodes on Mihomo, got %d", len(filtered.Nodes))
	}
	if len(excluded) != 0 {
		t.Fatalf("expected 0 excluded nodes on Mihomo, got %+v", excluded)
	}

	// 2. ValidateTargetCapabilities reports no plugin errors for legitimate plugin SS
	diags := compiler.ValidateTargetCapabilities(snap, domain.TargetMihomo)
	for _, d := range diags {
		if strings.Contains(d.Message, "plugin") {
			t.Errorf("unexpected plugin diagnostic on Mihomo: %+v", d)
		}
	}

	// 3. Compile succeeds and outputs legitimate native plugin config
	res, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected Mihomo compile to succeed with native plugin, got: %v", err)
	}
	content := string(res.Content)
	if !strings.Contains(content, "plugin: obfs") {
		t.Errorf("expected Mihomo YAML to contain 'plugin: obfs', got:\n%s", content)
	}
	if !strings.Contains(content, "plugin-opts:") {
		t.Errorf("expected Mihomo YAML to contain 'plugin-opts:', got:\n%s", content)
	}
}
