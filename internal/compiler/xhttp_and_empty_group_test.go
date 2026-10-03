package compiler_test

import (
	"context"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func TestMihomoXHTTPExport(t *testing.T) {
	ctx := context.Background()

	xhttpNode := resolver.ResolvedNode{
		LogicalID:   "node_vless_xhttp_01",
		Protocol:    domain.ProtocolVLESS,
		DisplayName: "VLESS-xhttp-Node",
		Server:      "edge.example.com",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			UUID: "22222222-2222-2222-2222-222222222222",
			Transport: map[string]string{
				"network":   "xhttp",
				"path":      "/xhttp-path",
				"host":      "edge.example.com",
				"mode":      "auto",
				"headers":   `{"X-Custom-Header":"custom-val"}`,
				"extra_foo": "should_be_isolated",
			},
		},
		Active: true,
	}

	snap := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "sha256:snap_xhttp",
		CompilerVersion: "1.0.0",
		Nodes:           []resolver.ResolvedNode{xhttpNode},
		Groups: []resolver.ResolvedGroup{
			{
				Name:      "Proxy",
				GroupType: domain.GroupTypeSelect,
				Members:   []resolver.ResolvedGroupMember{{DisplayName: "VLESS-xhttp-Node"}},
			},
		},
	}

	// 1. Mihomo export test
	mihomoRes, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("expected Mihomo compile to succeed with xhttp: %v", err)
	}
	content := string(mihomoRes.Content)
	if !strings.Contains(content, "network: xhttp") {
		t.Fatalf("expected 'network: xhttp' in Mihomo output, got:\n%s", content)
	}
	if !strings.Contains(content, "xhttp-opts:") {
		t.Fatalf("expected 'xhttp-opts:' in Mihomo output, got:\n%s", content)
	}
	if !strings.Contains(content, "path: /xhttp-path") {
		t.Fatalf("expected 'path: /xhttp-path' in Mihomo output, got:\n%s", content)
	}
	if !strings.Contains(content, "host: edge.example.com") {
		t.Fatalf("expected 'host: edge.example.com' in Mihomo output, got:\n%s", content)
	}
	if !strings.Contains(content, "mode: auto") {
		t.Fatalf("expected 'mode: auto' in Mihomo output, got:\n%s", content)
	}
	if !strings.Contains(content, "X-Custom-Header: custom-val") {
		t.Fatalf("expected headers in xhttp-opts, got:\n%s", content)
	}
	// Extra unknown parameters must NOT be blindly emitted in xhttp-opts
	if strings.Contains(content, "extra_foo") || strings.Contains(content, "should_be_isolated") {
		t.Fatalf("extra unknown parameters leaked into Mihomo output:\n%s", content)
	}

	// 2. sing-box target must fail closed with unsupported_target_capability
	_, singErr := compiler.Compile(ctx, snap, domain.TargetSingBox)
	if singErr == nil {
		t.Fatalf("expected sing-box compile to fail on xhttp node, but it succeeded")
	}
	if !strings.Contains(singErr.Error(), "sing-box does not support xhttp transport protocol") {
		t.Fatalf("expected error indicating sing-box xhttp unsupported, got: %v", singErr)
	}
}

func TestEmptyGroupRejection(t *testing.T) {
	ctx := context.Background()

	node := resolver.ResolvedNode{
		LogicalID:   "node_vmess_01",
		Protocol:    domain.ProtocolVMess,
		DisplayName: "VMess-Node",
		Server:      "vmess.example.com",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			UUID: "11111111-1111-1111-1111-111111111111",
		},
		Active: true,
	}

	// Group with 0 members (e.g. filtered out)
	snapEmptyGroup := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "sha256:snap_empty_group",
		CompilerVersion: "1.0.0",
		Nodes:           []resolver.ResolvedNode{node},
		Groups: []resolver.ResolvedGroup{
			{
				Name:      "FilteredEmptyGroup",
				GroupType: domain.GroupTypeSelect,
				Members:   []resolver.ResolvedGroupMember{}, // 0 members!
			},
		},
	}

	// 1. Surge must reject empty group with empty_group_not_allowed and NEVER auto-insert DIRECT
	_, errSurge := compiler.RenderSurgeGroupLine(0, snapEmptyGroup.Groups[0])
	if errSurge == nil {
		t.Fatalf("expected Surge to reject empty group, got success")
	}
	if !strings.Contains(errSurge.Error(), "empty_group_not_allowed") {
		t.Fatalf("expected empty_group_not_allowed error for Surge, got: %v", errSurge)
	}

	// 2. Mihomo must NEVER auto-insert DIRECT
	resMihomo, errMihomo := compiler.Compile(ctx, snapEmptyGroup, domain.TargetMihomo)
	if errMihomo == nil {
		content := string(resMihomo.Content)
		if strings.Contains(content, "- DIRECT") {
			t.Fatalf("Mihomo output must never auto-insert DIRECT into empty group, got:\n%s", content)
		}
	}
}
