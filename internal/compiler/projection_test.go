package compiler_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func incompatibleMihomoSS(id string) resolver.ResolvedNode {
	node := makeTestNode(id, id, domain.ProtocolSS, 0)
	node.Credentials.Transport = map[string]string{"skip_cert_verify": "true"}
	return node
}

func TestTargetProjectionMembershipDigestAndImmutability(t *testing.T) {
	bad := incompatibleMihomoSS("bad")
	good := makeTestNode("good", "good", domain.ProtocolSS, 1)
	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{bad, good})
	snap.Groups[0].ExpandedNodeIDs = []string{"bad", "good"}
	snap.Groups = append(snap.Groups, resolver.ResolvedGroup{
		ID: "parent", Name: "parent", GroupType: domain.GroupTypeSelect,
		Members:       []resolver.ResolvedGroupMember{{Kind: resolver.MemberKindGroup, TargetID: snap.Groups[0].ID, DisplayName: snap.Groups[0].Name}},
		ChildGroupIDs: []string{snap.Groups[0].ID}, AllNodeLogicalIDs: []string{"bad", "good"}, ExpandedNodeIDs: []string{"bad", "good"},
	})
	snap.AdmittedNodeIDs = []string{"bad", "good"}
	snap.FilterCounts = &resolver.FilterLayerCounts{
		RawTotal: 2, AdmittedTotal: 2, GlobalFilteredTotal: 2, GroupFilteredTotal: 2,
		GroupCounts: map[string]resolver.GroupFilterCount{snap.Groups[0].ID: {Candidate: 2, Kept: 2}},
	}
	before, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	projected, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	after, _ := json.Marshal(snap)
	if string(before) != string(after) {
		t.Fatal("projection mutated the source snapshot")
	}
	if len(excluded) != 1 || excluded[0].NodeID != "bad" || !strings.Contains(excluded[0].Reason, "skip_cert_verify") {
		t.Fatalf("missing target exclusion: %+v", excluded)
	}
	if !reflect.DeepEqual(projected.NodeLogicalIDs, []string{"good"}) || projected.Nodes[0].Position != 0 {
		t.Fatalf("snapshot membership/order not recomputed: %+v", projected.Nodes)
	}
	for _, group := range projected.Groups {
		if !reflect.DeepEqual(group.AllNodeLogicalIDs, []string{"good"}) || !reflect.DeepEqual(group.ExpandedNodeIDs, []string{"good"}) {
			t.Fatalf("stale expanded membership: %+v", group)
		}
	}
	if !reflect.DeepEqual(projected.Groups[0].NodeLogicalIDs, []string{"good"}) || len(projected.Groups[0].Members) != 1 || len(projected.Groups[1].NodeLogicalIDs) != 0 {
		t.Fatalf("direct membership not recomputed: %+v", projected.Groups)
	}
	count := projected.FilterCounts.GroupCounts[snap.Groups[0].ID]
	if count.Kept != 1 || count.Excluded != 1 || count.ExclusionSummary["unsupported_target_capability"] != 1 || projected.FilterCounts.GroupFilteredTotal != 1 {
		t.Fatalf("stale group summary: %+v", projected.FilterCounts)
	}
	if snap.FilterCounts.GroupCounts[snap.Groups[0].ID].Kept != 2 {
		t.Fatal("source count was mutated")
	}
	wantDigest, err := resolver.ComputeSnapshotDigest(projected)
	if err != nil || projected.SnapshotDigest != wantDigest || projected.SnapshotDigest == snap.SnapshotDigest {
		t.Fatalf("digest not recomputed: got=%s want=%s err=%v", projected.SnapshotDigest, wantDigest, err)
	}
	again, _ := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if again.SnapshotDigest != projected.SnapshotDigest {
		t.Fatal("projection digest is not deterministic")
	}
	if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err != nil {
		t.Fatalf("compatible remainder must compile: %v", err)
	}
}

func TestTargetProjectionMalformedStoredNodeIsFormatErrorForEveryTarget(t *testing.T) {
	bad := makeTestNode("corrupt", "corrupt", domain.ProtocolSS, 0)
	bad.Credentials.Password = ""
	good := makeTestNode("good", "good", domain.ProtocolSS, 1)
	snapshot := makeAdaptationSnapshot([]resolver.ResolvedNode{bad, good})
	for _, target := range compiler.Targets() {
		projected, excluded := compiler.FilterCompatibleSnapshot(snapshot, target)
		if len(projected.Nodes) != 1 || len(excluded) != 1 || excluded[0].Code != "format_error" || excluded[0].NodeID != "corrupt" {
			t.Fatalf("%s: corrupt node not classified globally: nodes=%+v excluded=%+v", target, projected.Nodes, excluded)
		}
	}
}

func TestTargetProjectionDoesNotReuseMihomoExclusionsAcrossTargets(t *testing.T) {
	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{incompatibleMihomoSS("legacy-ss")})
	for _, target := range []domain.CompilerTarget{domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX} {
		t.Run(string(target), func(t *testing.T) {
			projected, excluded := compiler.FilterCompatibleSnapshot(snap, target)
			if len(projected.Nodes) != 1 || len(excluded) != 0 {
				t.Fatalf("Mihomo transport exclusion leaked into %s: %+v", target, excluded)
			}
			if _, err := compiler.Compile(context.Background(), projected, target); err != nil {
				t.Fatalf("target must use its own renderer: %v", err)
			}
		})
	}
}

func TestTargetProjectionEmptyPassIsLocalPermission(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "required-proxy", true: "resolved-local-or-global-pass"}[allow], func(t *testing.T) {
			snap := makeAdaptationSnapshot([]resolver.ResolvedNode{incompatibleMihomoSS("bad")})
			snap.Groups[0].EmptyFallbackPass = allow
			projected, _ := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
			if projected.Groups[0].UsesEmptyPass() != allow {
				t.Fatalf("PASS permission was changed: %+v", projected.Groups[0])
			}
			diagnostics := compiler.ValidateTargetCapabilities(projected, domain.TargetMihomo)
			if allow && len(diagnostics) != 0 {
				t.Fatalf("PASS projection failed capability validation: %+v", diagnostics)
			}
			if !allow && (len(diagnostics) != 1 || diagnostics[0].Code != "required_nonempty") {
				t.Fatalf("empty proxy group lost its strict diagnostic: %+v", diagnostics)
			}
			_, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo)
			if allow && err != nil {
				t.Fatalf("resolved PASS fallback must compile: %v", err)
			}
			if !allow && (err == nil || !strings.Contains(err.Error(), "required_nonempty")) {
				t.Fatalf("ordinary empty group must fail: %v", err)
			}
		})
	}
	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{incompatibleMihomoSS("bad")})
	snap.Groups[0].EmptyFallbackPass = true
	snap.Groups = append(snap.Groups, resolver.ResolvedGroup{
		ID: "parent", Name: "parent", GroupType: domain.GroupTypeSelect,
		Members:       []resolver.ResolvedGroupMember{{Kind: resolver.MemberKindGroup, TargetID: snap.Groups[0].ID, DisplayName: snap.Groups[0].Name}},
		ChildGroupIDs: []string{snap.Groups[0].ID}, AllNodeLogicalIDs: []string{"bad"},
	})
	projected, _ := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if projected.Groups[1].UsesEmptyPass() {
		t.Fatal("parent inherited child PASS permission")
	}
	diagnostics := compiler.ValidateTargetCapabilities(projected, domain.TargetMihomo)
	if len(diagnostics) != 1 || diagnostics[0].Code != "required_nonempty" {
		t.Fatalf("child PASS hid parent topology failure: %+v", diagnostics)
	}
	if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err == nil || !strings.Contains(err.Error(), "required_nonempty") {
		t.Fatalf("parent without permission must fail closed: %v", err)
	}
}

func TestTargetProjectionPreservesResolverGlobalPass(t *testing.T) {
	bad := incompatibleMihomoSS(domain.ComputeNodeLogicalID(domain.ProtocolSS, "192.0.2.1", 8388, nil))
	groupID := domain.MustNewUUIDv7()
	on := true
	snap, err := resolver.New().Resolve(context.Background(), resolver.ResolveInput{
		GlobalEmptyFallbackPass: &on,
		Nodes:                   []domain.Node{{LogicalID: bad.LogicalID, DisplayName: bad.DisplayName, Protocol: bad.Protocol, Server: bad.Server, Port: bad.Port, Credentials: bad.Credentials, Active: true}},
		Groups:                  []domain.NodeGroup{{ID: groupID, Name: "global-pass", GroupType: domain.GroupTypeSelect}},
		GroupSources: map[string][]domain.MemberSource{groupID: {{ID: "source-global-pass", GroupID: groupID,
			Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &bad.LogicalID}}},
		PolicyRules: []domain.PolicyRule{{ID: domain.MustNewUUIDv7(), Expression: "MATCH", TargetGroupID: groupID, Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Groups) != 1 || !snap.Groups[0].EmptyFallbackPass || snap.Groups[0].UsesEmptyPass() {
		t.Fatalf("resolver fixture did not resolve global permission onto a populated group: %+v", snap.Groups)
	}
	projected, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if len(excluded) != 1 || !projected.Groups[0].UsesEmptyPass() {
		t.Fatalf("global PASS was lost after target exclusion: groups=%+v excluded=%+v", projected.Groups, excluded)
	}
	if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err != nil {
		t.Fatalf("resolved global PASS must compile: %v", err)
	}
}

func TestTargetProjectionNestedBuiltinIsNotEmptyPass(t *testing.T) {
	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{incompatibleMihomoSS("bad")})
	snap.Groups[0].Members = append(snap.Groups[0].Members, resolver.ResolvedGroupMember{Kind: resolver.MemberKindBuiltin, DisplayName: "REJECT", TargetID: "REJECT"})
	snap.Groups[0].EmptyFallbackPass = true
	snap.Groups = append(snap.Groups, resolver.ResolvedGroup{
		ID: "parent", Name: "parent", GroupType: domain.GroupTypeSelect, EmptyFallbackPass: true,
		Members:       []resolver.ResolvedGroupMember{{Kind: resolver.MemberKindGroup, TargetID: snap.Groups[0].ID, DisplayName: snap.Groups[0].Name}},
		ChildGroupIDs: []string{snap.Groups[0].ID}, AllNodeLogicalIDs: []string{"bad"},
	})
	projected, _ := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	for _, group := range projected.Groups {
		if group.UsesEmptyPass() {
			t.Fatalf("builtin destination was overwritten by PASS: %+v", group)
		}
	}
	if diags := compiler.ValidateTargetCapabilities(projected, domain.TargetMihomo); len(diags) != 0 {
		t.Fatalf("nested builtin must remain usable: %+v", diags)
	}
	if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err != nil {
		t.Fatal(err)
	}
}

func TestTargetProjectionDialerDependenciesFailClosed(t *testing.T) {
	for _, groupDialer := range []bool{false, true} {
		t.Run(map[bool]string{false: "node-dialer", true: "group-dialer"}[groupDialer], func(t *testing.T) {
			bad := incompatibleMihomoSS("entry")
			middle := makeTestNode("middle", "middle", domain.ProtocolSS, 1)
			middle.DialerProxy, middle.ChainBindingID = "entry", "binding"
			middle.DialerID, middle.DialerType = "entry", domain.ProxyChainDialerNode
			tail := makeTestNode("tail", "tail", domain.ProtocolSS, 2)
			tail.DialerProxy = "middle"
			tail.DialerID, tail.DialerType = "middle", domain.ProxyChainDialerNode
			good := makeTestNode("good", "good", domain.ProtocolSS, 3)
			snap := makeAdaptationSnapshot([]resolver.ResolvedNode{tail, middle, bad, good})
			if groupDialer {
				snap.Nodes[1].DialerProxy = "entry-group"
				snap.Nodes[1].DialerID, snap.Nodes[1].DialerType = "entry-group", domain.ProxyChainDialerNodeGroup
				snap.Groups = append(snap.Groups, resolver.ResolvedGroup{
					ID: "entry-group", Name: "entry-group", GroupType: domain.GroupTypeSelect, EmptyFallbackPass: true,
					Members:        []resolver.ResolvedGroupMember{{Kind: resolver.MemberKindNode, TargetID: "entry", DisplayName: "entry"}},
					NodeLogicalIDs: []string{"entry"}, AllNodeLogicalIDs: []string{"entry"},
				})
			}
			snap.ChainEffects = []resolver.ChainEffect{{BindingID: "binding", TargetCount: 1, ChainCount: 1, ChainSamples: []string{"middle"}}}
			projected, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
			if !reflect.DeepEqual(projected.NodeLogicalIDs, []string{"good"}) || len(excluded) != 3 {
				t.Fatalf("dangling chain remained: nodes=%+v excluded=%+v", projected.Nodes, excluded)
			}
			for i, id := range []string{"tail", "middle", "entry"} {
				if excluded[i].NodeID != id {
					t.Fatalf("exclusions lost source ordering: %+v", excluded)
				}
			}
			if len(projected.ChainEffects) != 0 || len(snap.ChainEffects) != 1 {
				t.Fatalf("chain summary must not masquerade as projected counts: %+v", projected.ChainEffects)
			}
			found := false
			for _, diagnostic := range projected.Diagnostics {
				found = found || diagnostic.Code == "target_projection_chain_effects_unavailable"
			}
			if !found {
				t.Fatal("missing explanation of unavailable chain summary")
			}
			if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err != nil {
				t.Fatalf("independent good node must remain exportable: %v", err)
			}
		})
	}
}

func TestTargetProjectionKeepsUsableDialerGroupAndUnknownTopology(t *testing.T) {
	entry := makeTestNode("entry", "entry", domain.ProtocolSS, 0)
	exit := makeTestNode("exit", "exit", domain.ProtocolSS, 1)
	exit.DialerProxy = "entry-group"
	exit.DialerID, exit.DialerType = "entry-group", domain.ProxyChainDialerNodeGroup
	snap := makeAdaptationSnapshot([]resolver.ResolvedNode{entry, exit, incompatibleMihomoSS("bad")})
	snap.Groups = append(snap.Groups, resolver.ResolvedGroup{
		ID: "entry-group", Name: "entry-group", GroupType: domain.GroupTypeSelect,
		Members:        []resolver.ResolvedGroupMember{{Kind: resolver.MemberKindNode, TargetID: "entry", DisplayName: "entry"}},
		NodeLogicalIDs: []string{"entry"}, AllNodeLogicalIDs: []string{"entry"},
	})
	projected, excluded := compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if len(excluded) != 1 || len(projected.Nodes) != 2 || projected.Nodes[1].DialerProxy != "entry-group" {
		t.Fatalf("usable dialer was changed: nodes=%+v excluded=%+v", projected.Nodes, excluded)
	}
	if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err != nil {
		t.Fatal(err)
	}
	snap.Groups[0].Members = append(snap.Groups[0].Members, resolver.ResolvedGroupMember{Kind: resolver.MemberKindNode, TargetID: "unknown", DisplayName: "unknown"})
	projected, _ = compiler.FilterCompatibleSnapshot(snap, domain.TargetMihomo)
	if _, err := compiler.Compile(context.Background(), projected, domain.TargetMihomo); err == nil || !strings.Contains(err.Error(), "unknown node") {
		t.Fatalf("projection hid corrupt topology: %v", err)
	}
}
