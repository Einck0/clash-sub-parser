package compiler_test

import (
	"context"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func TestProjectionFormatContractIsTargetIndependent(t *testing.T) {
	for _, brokenField := range []string{"display_name", "endpoint", "password"} {
		t.Run(brokenField, func(t *testing.T) {
			node := makeTestNode("bad", "bad", domain.ProtocolSS, 0)
			switch brokenField {
			case "display_name":
				node.DisplayName = " "
			case "endpoint":
				node.Port = 0
			case "password":
				node.Credentials.Password = ""
			}
			for _, target := range compiler.Targets() {
				snap := &resolver.ResolvedPolicySnapshot{Nodes: []resolver.ResolvedNode{node}}
				diagnostics := compiler.ValidateTargetCapabilities(snap, target)
				if len(diagnostics) != 1 || diagnostics[0].Code != "format_error" {
					t.Fatalf("%s: format error became target-specific: %+v", target, diagnostics)
				}
				projected, excluded := compiler.FilterCompatibleSnapshot(snap, target)
				if len(projected.Nodes) != 0 || len(excluded) != 1 || excluded[0].Code != "format_error" {
					t.Fatalf("%s: inconsistent projection: %+v", target, excluded)
				}
			}
		})
	}
}

func TestProjectionChainCollisionCannotRebindToSurvivingNames(t *testing.T) {
	for _, bound := range []bool{false, true} {
		bad := incompatibleMihomoSS("entry")
		good := makeTestNode("good", "entry", domain.ProtocolSS, 1)
		tail := makeTestNode("tail", "tail", domain.ProtocolSS, 2)
		tail.DialerProxy = "entry"
		if bound {
			tail.DialerID, tail.DialerType = bad.LogicalID, domain.ProxyChainDialerNode
		}
		snap := makeAdaptationSnapshot([]resolver.ResolvedNode{bad, good, tail})
		projected, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
		if len(excluded) != 1 || len(projected.Nodes) != 2 {
			t.Fatalf("ambiguous binding was silently excluded: %+v %+v", projected.Nodes, excluded)
		}
		found := false
		for _, diagnostic := range compiler.ValidateTargetCapabilities(projected, domain.TargetMihomo) {
			found = found || diagnostic.Code == "invalid_chain_topology"
		}
		if !found {
			t.Fatal("projection hid original name ambiguity")
		}
		if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err == nil {
			t.Fatal("ambiguous chain silently rebound to surviving node")
		}
	}
}

func TestProjectionChainNodeGroupCollisionAndBindingMismatchFail(t *testing.T) {
	for _, scenario := range []string{"node-group-collision", "binding-mismatch", "missing-reference"} {
		t.Run(scenario, func(t *testing.T) {
			entry := makeTestNode("entry", "entry", domain.ProtocolSS, 0)
			tail := makeTestNode("tail", "tail", domain.ProtocolSS, 1)
			tail.DialerProxy, tail.DialerID, tail.DialerType = "entry", "entry", domain.ProxyChainDialerNode
			snap := makeAdaptationSnapshot([]resolver.ResolvedNode{entry, tail})
			switch scenario {
			case "node-group-collision":
				snap.Groups[0].Name = "entry"
			case "binding-mismatch":
				snap.Nodes[1].DialerID = "another-id"
			case "missing-reference":
				snap.Nodes[1].DialerProxy = "unknown"
			}
			projected, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
			if len(excluded) != 0 {
				t.Fatalf("topology was confused with capability: %+v", excluded)
			}
			if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err == nil || !strings.Contains(err.Error(), "dialer-proxy") {
				t.Fatalf("invalid chain did not fail closed: %v", err)
			}
		})
	}
}

func TestProjectionBindsLegacyMemberIDBeforeRecomputingPass(t *testing.T) {
	bad := incompatibleMihomoSS("bad")
	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{bad})
	snap.Groups[0].EmptyFallbackPass = true
	snap.Groups[0].Members[0].Kind = ""
	// Same display name must not override the member's resolved node ID.
	snap.Groups = append(snap.Groups, resolver.ResolvedGroup{ID: "same-name-group", Name: bad.DisplayName, GroupType: domain.GroupTypeSelect,
		Members: []resolver.ResolvedGroupMember{{Kind: resolver.MemberKindBuiltin, TargetID: "REJECT", DisplayName: "REJECT"}}})
	projected, _ := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if len(projected.Groups[0].Members) != 0 || !projected.Groups[0].UsesEmptyPass() {
		t.Fatalf("stable node reference was mistaken for same-name group: %+v", projected.Groups[0])
	}
	if diagnostics := compiler.ValidateTargetCapabilities(projected, domain.TargetMihomo); len(diagnostics) != 0 {
		t.Fatalf("PASS rejected after binding ID: %+v", diagnostics)
	}
	// Without the ID, the original ambiguity must remain fatal even after Bad is removed.
	snap.Groups[0].Members[0].TargetID = ""
	projected, _ = compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err == nil {
		t.Fatal("ambiguous member silently rebound to group")
	}
}

func TestProjectionPreservesExplicitBuiltinPass(t *testing.T) {
	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{incompatibleMihomoSS("bad")})
	snap.Groups[0].Members = append(snap.Groups[0].Members, resolver.ResolvedGroupMember{Kind: resolver.MemberKindBuiltin, TargetID: "PASS", DisplayName: "PASS"})
	projected, _ := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if diagnostics := compiler.ValidateTargetCapabilities(projected, domain.TargetMihomo); len(diagnostics) != 0 {
		t.Fatalf("explicit PASS rejected: %+v", diagnostics)
	}
	result, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo)
	if err != nil || !strings.Contains(string(result.Content), "- PASS") {
		t.Fatalf("explicit PASS lost: %s %v", result.Content, err)
	}
}
