package compiler_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func fixtureSnapshot() *resolver.ResolvedPolicySnapshot {
	return &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "snapshot-test-digest",
		CompilerVersion: "1.0.0",
		Nodes: []resolver.ResolvedNode{
			{LogicalID: "0123456789abcdef0123456789abcdef", DisplayName: "edge-a", Protocol: domain.ProtocolVMess, Active: true, Position: 0},
			{LogicalID: "abcdef0123456789abcdef0123456789", DisplayName: "edge-b", Protocol: domain.ProtocolSS, Active: true, Position: 1},
		},
		Groups: []resolver.ResolvedGroup{
			{
				ID:        "018f0b6e-4d7a-7abc-8def-0123456789ab",
				Name:      "proxy",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: "0123456789abcdef0123456789abcdef", DisplayName: "edge-a", Position: 0},
					{Kind: resolver.MemberKindNode, TargetID: "abcdef0123456789abcdef0123456789", DisplayName: "edge-b", Position: 1},
				},
				NodeLogicalIDs:    []string{"0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789"},
				AllNodeLogicalIDs: []string{"0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789"},
				Position:          0,
			},
		},
		Rules: []resolver.ResolvedRule{
			{ID: "rule-1", TargetGroupID: "018f0b6e-4d7a-7abc-8def-0123456789ab", TargetGroupName: "proxy", Expression: "DOMAIN-SUFFIX,example.com", Position: 0},
			{ID: "rule-2", TargetGroupID: "018f0b6e-4d7a-7abc-8def-0123456789ab", TargetGroupName: "proxy", Expression: "MATCH", Position: 1, IsTerminal: true},
		},
	}
}

func TestRenderersProduceDeterministicGoldenFixtures(t *testing.T) {
	ctx := context.Background()
	snapshot := fixtureSnapshot()
	for _, target := range compiler.Targets() {
		t.Run(string(target), func(t *testing.T) {
			first, err := compiler.Compile(ctx, snapshot, target)
			if err != nil {
				t.Fatalf("compile failed: %v", err)
			}
			second, err := compiler.Compile(ctx, snapshot, target)
			if err != nil {
				t.Fatalf("second compile failed: %v", err)
			}
			if string(first.Content) != string(second.Content) {
				t.Fatal("same snapshot produced different output")
			}
			if first.ContentDigest == "" || first.SnapshotDigest != snapshot.SnapshotDigest {
				t.Fatalf("missing result digests: %#v", first)
			}
			if strings.Contains(string(first.Content), "secret") || strings.Contains(string(first.Content), "token") {
				t.Fatal("renderer output contains a secret-bearing field")
			}
			goldenPath := filepath.Join("testdata", "golden", string(target)+".golden")
			if os.Getenv("UPDATE_GOLDENS") == "1" {
				if writeErr := os.WriteFile(goldenPath, first.Content, 0o644); writeErr != nil {
					t.Fatalf("write golden: %v", writeErr)
				}
			}
			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if string(first.Content) != string(golden) {
				t.Fatalf("output differs from golden fixture")
			}
		})
	}
}

func TestCompileRejectsUnsupportedProtocolWithTargetDiagnostic(t *testing.T) {
	snapshot := fixtureSnapshot()
	snapshot.Nodes = append(snapshot.Nodes, resolver.ResolvedNode{
		LogicalID:   "11111111111111111111111111111111",
		DisplayName: "wg-node",
		Protocol:    domain.ProtocolWireGuard,
		Active:      true,
		Position:    2,
	})
	_, err := compiler.Compile(context.Background(), snapshot, domain.TargetQuantumultX)
	if err == nil {
		t.Fatal("expected unsupported protocol to hard-fail")
	}
	var capabilityErr *compiler.CapabilityError
	if !errors.As(err, &capabilityErr) {
		t.Fatalf("expected CapabilityError, got %T: %v", err, err)
	}
	if capabilityErr.Target != domain.TargetQuantumultX || capabilityErr.Feature != string(domain.ProtocolWireGuard) {
		t.Fatalf("missing target-specific diagnostic: %#v", capabilityErr)
	}
	if capabilityErr.Location != "nodes[2]" {
		t.Fatalf("unexpected diagnostic location: %s", capabilityErr.Location)
	}
}

func TestCompileRejectsUnsupportedGroupTypeWithPosition(t *testing.T) {
	snapshot := fixtureSnapshot()
	snapshot.Groups[0].GroupType = domain.GroupTypeURLTest
	_, err := compiler.Compile(context.Background(), snapshot, domain.TargetQuantumultX)
	if err == nil {
		t.Fatal("expected unsupported group type to hard-fail")
	}
	var capabilityErr *compiler.CapabilityError
	if !errors.As(err, &capabilityErr) {
		t.Fatalf("expected CapabilityError, got %T: %v", err, err)
	}
	if capabilityErr.Target != domain.TargetQuantumultX || capabilityErr.Location != "groups[0]" {
		t.Fatalf("missing group diagnostic: %#v", capabilityErr)
	}
}

func TestCompileRejectsUnsupportedRuleWithTargetDiagnostic(t *testing.T) {
	snapshot := fixtureSnapshot()
	snapshot.Rules = append(snapshot.Rules, resolver.ResolvedRule{
		ID:              "rule-geosite",
		TargetGroupID:   snapshot.Groups[0].ID,
		TargetGroupName: snapshot.Groups[0].Name,
		Expression:      "GEOSITE,category-ads-all",
		Position:        2,
	})

	// Surge does not support GEOSITE rules -> must hard-fail
	_, err := compiler.Compile(context.Background(), snapshot, domain.TargetSurge)
	if err == nil {
		t.Fatal("expected unsupported GEOSITE rule on Surge to hard-fail")
	}
	var capabilityErr *compiler.CapabilityError
	if !errors.As(err, &capabilityErr) {
		t.Fatalf("expected CapabilityError, got %T: %v", err, err)
	}
	if capabilityErr.Target != domain.TargetSurge || capabilityErr.Feature != "GEOSITE" {
		t.Fatalf("unexpected diagnostic for unsupported rule: %#v", capabilityErr)
	}
	if capabilityErr.Location != "rules[2]" {
		t.Fatalf("unexpected location for unsupported rule: %s", capabilityErr.Location)
	}

	// Mihomo and SingBox DO support GEOSITE -> should pass
	if _, err := compiler.Compile(context.Background(), snapshot, domain.TargetMihomo); err != nil {
		t.Fatalf("expected Mihomo to support GEOSITE rule: %v", err)
	}
	if _, err := compiler.Compile(context.Background(), snapshot, domain.TargetSingBox); err != nil {
		t.Fatalf("expected SingBox to support GEOSITE rule: %v", err)
	}
}

func TestCompileAcceptsProcessNameRuleOnSupportedTargets(t *testing.T) {
	snapshot := fixtureSnapshot()
	snapshot.Rules = append(snapshot.Rules, resolver.ResolvedRule{
		ID:              "rule-process-name",
		TargetGroupID:   snapshot.Groups[0].ID,
		TargetGroupName: snapshot.Groups[0].Name,
		Expression:      "PROCESS-NAME,curl",
		Position:        2,
	})

	for _, target := range []domain.CompilerTarget{domain.TargetMihomo, domain.TargetClash, domain.TargetSingBox, domain.TargetSurge} {
		t.Run(string(target), func(t *testing.T) {
			if _, err := compiler.Compile(context.Background(), snapshot, target); err != nil {
				t.Fatalf("expected %s to support PROCESS-NAME rule: %v", target, err)
			}
		})
	}

	// QuantumultX does not support PROCESS-NAME -> must fail cleanly with 422 CapabilityError
	t.Run("quantumult-x-rejection", func(t *testing.T) {
		_, err := compiler.Compile(context.Background(), snapshot, domain.TargetQuantumultX)
		if err == nil {
			t.Fatal("expected Quantumult-X to reject PROCESS-NAME")
		}
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) {
			t.Fatalf("expected CapabilityError, got %v", err)
		}
		if capErr.Feature != "PROCESS-NAME" {
			t.Fatalf("expected feature PROCESS-NAME, got %s", capErr.Feature)
		}
	})
}

func TestCapabilityMatrixIsExplicitAndIndependent(t *testing.T) {
	matrix := compiler.CapabilityMatrix()
	if len(matrix) != 5 {
		t.Fatalf("expected five target capabilities, got %d", len(matrix))
	}
	matrix[domain.TargetQuantumultX].Protocols[domain.ProtocolWireGuard] = true
	if compiler.CapabilityMatrix()[domain.TargetQuantumultX].Protocols[domain.ProtocolWireGuard] {
		t.Fatal("capability matrix leaked mutable state")
	}
}

func TestSingBoxOutputIsValidJSON(t *testing.T) {
	result, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetSingBox)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(result.Content, &value); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestClashAndMihomoOutputIsValidYAML(t *testing.T) {
	for _, target := range []domain.CompilerTarget{domain.TargetClash, domain.TargetMihomo} {
		res, err := compiler.Compile(context.Background(), fixtureSnapshot(), target)
		if err != nil {
			t.Fatalf("compile %s failed: %v", target, err)
		}
		var parsed map[string]any
		if err := yaml.Unmarshal(res.Content, &parsed); err != nil {
			t.Fatalf("%s output invalid YAML: %v", target, err)
		}
		proxies, ok := parsed["proxies"].([]any)
		if !ok || len(proxies) != 2 {
			t.Fatalf("%s missing expected proxies array: %#v", target, parsed["proxies"])
		}
		groups, ok := parsed["proxy-groups"].([]any)
		if !ok || len(groups) != 1 {
			t.Fatalf("%s missing expected proxy-groups array: %#v", target, parsed["proxy-groups"])
		}
	}
}

func TestSurgeAndQuantumultXOutputFormat(t *testing.T) {
	surgeRes, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetSurge)
	if err != nil {
		t.Fatalf("compile Surge failed: %v", err)
	}
	surgeStr := string(surgeRes.Content)
	if !strings.Contains(surgeStr, "[General]") || !strings.Contains(surgeStr, "[Proxy]") ||
		!strings.Contains(surgeStr, "[Proxy Group]") || !strings.Contains(surgeStr, "[Rule]") {
		t.Fatalf("Surge output missing expected sections: %s", surgeStr)
	}

	qxRes, err := compiler.Compile(context.Background(), fixtureSnapshot(), domain.TargetQuantumultX)
	if err != nil {
		t.Fatalf("compile QuantumultX failed: %v", err)
	}
	qxStr := string(qxRes.Content)
	if !strings.Contains(qxStr, "[general]") || !strings.Contains(qxStr, "[server_local]") ||
		!strings.Contains(qxStr, "[policy]") || !strings.Contains(qxStr, "[filter_local]") {
		t.Fatalf("QuantumultX output missing expected sections: %s", qxStr)
	}
}

func TestCompileRejectsNilSnapshotAndUnknownTarget(t *testing.T) {
	ctx := context.Background()
	_, err := compiler.Compile(ctx, nil, domain.TargetClash)
	if err == nil {
		t.Fatal("expected nil snapshot to fail")
	}

	snapshot := fixtureSnapshot()
	_, err = compiler.Compile(ctx, snapshot, domain.CompilerTarget("unknown_target"))
	if err == nil {
		t.Fatal("expected unknown target to fail")
	}
	var capErr *compiler.CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected CapabilityError for unknown target, got %T", err)
	}
}

func TestCompileContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := compiler.Compile(ctx, fixtureSnapshot(), domain.TargetClash)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCompileConcurrentHighLoad(t *testing.T) {
	snapshot := fixtureSnapshot()
	ctx := context.Background()
	targets := compiler.Targets()

	var baseline = make(map[domain.CompilerTarget]compiler.Result)
	for _, target := range targets {
		res, err := compiler.Compile(ctx, snapshot, target)
		if err != nil {
			t.Fatalf("baseline compile %s failed: %v", target, err)
		}
		baseline[target] = res
	}

	const goroutines = 100
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*len(targets))

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, target := range targets {
				res, err := compiler.Compile(ctx, snapshot, target)
				if err != nil {
					errCh <- err
					return
				}
				base := baseline[target]
				if res.ContentDigest != base.ContentDigest {
					errCh <- errors.New("concurrent content digest mismatch")
					return
				}
				if string(res.Content) != string(base.Content) {
					errCh <- errors.New("concurrent content bytes mismatch")
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent compile failure: %v", err)
		}
	}
}

func TestCompiler_DerivedProjectedGroupsAcrossAllTargets(t *testing.T) {
	ctx := context.Background()
	r := resolver.New()

	parentID := domain.MustNewUUIDv7()
	childID := domain.MustNewUUIDv7()
	n1 := domain.Node{LogicalID: "0123456789abcdef0123456789abcdef", DisplayName: "US-Fast", Protocol: domain.ProtocolTrojan, Active: true}
	n2 := domain.Node{LogicalID: "abcdef0123456789abcdef0123456789", DisplayName: "US-Slow", Protocol: domain.ProtocolTrojan, Active: true}

	input := resolver.ResolveInput{
		RevisionID:         "rev-comp-1",
		InventoryWatermark: "wm-1",
		CompilerVersion:    "1.0.0",
		Nodes:              []domain.Node{n1, n2},
		Groups: []domain.NodeGroup{
			{ID: parentID, Name: "Proxy", GroupType: domain.GroupTypeSelect},
			{ID: childID, Name: "Auto", GroupType: domain.GroupTypeSelect},
		},
		Edges: map[string][]domain.GroupEdge{
			parentID: {{ID: "e1", ParentGroupID: parentID, ChildGroupID: &childID, Position: 0}},
			childID: {
				{ID: "e2", ParentGroupID: childID, NodeLogicalID: &n1.LogicalID, Position: 0},
				{ID: "e3", ParentGroupID: childID, NodeLogicalID: &n2.LogicalID, Position: 1},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{ID: "r1", TargetGroupID: parentID, Expression: "MATCH", Position: 0},
		},
		GroupFilters: map[string]domain.NodeFilterSpec{
			parentID: {
				Conditions: []domain.FilterCondition{
					{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "Fast"},
				},
			},
		},
		DNS: resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1"}},
	}

	snap, err := r.Resolve(ctx, input)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	for _, target := range compiler.Targets() {
		t.Run(string(target), func(t *testing.T) {
			res, err := compiler.Compile(ctx, snap, target)
			if err != nil {
				t.Fatalf("compile for %s failed: %v", target, err)
			}
			out := string(res.Content)

			// Must contain derived group name "Auto [Proxy]"
			if !strings.Contains(out, "Auto [Proxy]") {
				t.Errorf("expected target %s output to contain derived group 'Auto [Proxy]', got:\n%s", target, out)
			}

			// Must contain allowed node US-Fast
			if !strings.Contains(out, "US-Fast") {
				t.Errorf("expected target %s output to contain allowed node 'US-Fast'", target)
			}

			// Must not contain secret or token
			if strings.Contains(out, "secret") || strings.Contains(out, "token") {
				t.Errorf("target %s output leaks secret or token", target)
			}
		})
	}
}
