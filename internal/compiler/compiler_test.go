package compiler_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/resolver"
)

func findMihomoBinary() string {
	if bin := os.Getenv("MIHOMO_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	if path, err := exec.LookPath("mihomo"); err == nil {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidate := filepath.Join(home, "clashctl", "bin", "mihomo")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func fixtureSnapshot() *resolver.ResolvedPolicySnapshot {
	return &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "snapshot-test-digest",
		CompilerVersion: "1.0.0",
		Nodes: []resolver.ResolvedNode{
			{
				LogicalID:   "0123456789abcdef0123456789abcdef",
				DisplayName: "edge-a",
				Protocol:    domain.ProtocolVMess,
				Server:      "198.51.100.1",
				Port:        443,
				Credentials: domain.InboundProtocolCredential{
					UUID:    "b831381d-6324-4d53-ad4f-8cda48b30811",
					Method:  "auto",
					AlterID: 0,
					Transport: map[string]string{
						"network": "ws",
						"path":    "/vmess",
						"host":    "example.com",
						"tls":     "true",
					},
				},
				Active:   true,
				Position: 0,
			},
			{
				LogicalID:   "abcdef0123456789abcdef0123456789",
				DisplayName: "edge-b",
				Protocol:    domain.ProtocolSS,
				Server:      "198.51.100.2",
				Port:        8388,
				Credentials: domain.InboundProtocolCredential{
					Method:   "aes-256-gcm",
					Password: "test-ss-password",
				},
				Active:   true,
				Position: 1,
			},
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

func assertTargetGoldenFixture(t *testing.T, target domain.CompilerTarget) {
	t.Helper()
	ctx := context.Background()
	snapshot := fixtureSnapshot()

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
		t.Fatalf("output differs from golden fixture for target %s:\nGot:\n%s\nExpected:\n%s", target, string(first.Content), string(golden))
	}
}

func TestCompile_RejectsIllegalLegacyTargetClash(t *testing.T) {
	ctx := context.Background()
	snapshot := fixtureSnapshot()

	_, err := compiler.Compile(ctx, snapshot, domain.CompilerTarget("clash"))
	if err == nil {
		t.Fatal("expected legacy target 'clash' to be hard-rejected")
	}

	var capErr *compiler.CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected CapabilityError, got %T: %v", err, err)
	}
	if capErr.Target != domain.CompilerTarget("clash") || capErr.Reason != "unknown compiler target" {
		t.Fatalf("unexpected capability error for legacy clash: %#v", capErr)
	}
}

func TestCapabilityMatrixIsExplicitAndIndependent(t *testing.T) {
	matrix := compiler.CapabilityMatrix()
	if len(matrix) != 4 {
		t.Fatalf("expected four target capabilities, got %d", len(matrix))
	}
	if _, hasClash := matrix[domain.CompilerTarget("clash")]; hasClash {
		t.Fatal("capability matrix must not contain legacy target 'clash'")
	}
	syntheticProto := domain.Protocol("synthetic-test-protocol")
	matrix[domain.TargetMihomo].Protocols[syntheticProto] = true
	if compiler.CapabilityMatrix()[domain.TargetMihomo].Protocols[syntheticProto] {
		t.Fatal("capability matrix leaked mutable state")
	}
}

func TestCompileRejectsNilSnapshotAndUnknownTarget(t *testing.T) {
	ctx := context.Background()
	_, err := compiler.Compile(ctx, nil, domain.TargetMihomo)
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
	_, err := compiler.Compile(ctx, fixtureSnapshot(), domain.TargetSingBox)
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
	n1 := domain.Node{
		LogicalID:   "0123456789abcdef0123456789abcdef",
		DisplayName: "US-Fast",
		Protocol:    domain.ProtocolTrojan,
		Server:      "198.51.100.20",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			Password: "trojan-pwd-1",
		},
		Active: true,
	}
	n2 := domain.Node{
		LogicalID:   "abcdef0123456789abcdef0123456789",
		DisplayName: "US-Slow",
		Protocol:    domain.ProtocolTrojan,
		Server:      "198.51.100.21",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			Password: "trojan-pwd-2",
		},
		Active: true,
	}

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

			if target == domain.TargetMihomo {
				if !strings.Contains(out, "Auto [Proxy]") {
					t.Errorf("expected target %s output to contain derived group 'Auto [Proxy]', got:\n%s", target, out)
				}
			} else {
				if strings.Contains(out, "Auto [Proxy]") {
					t.Errorf("expected node-only target %s output not to contain derived group 'Auto [Proxy]', got:\n%s", target, out)
				}
			}

			if !strings.Contains(out, "US-Fast") {
				t.Errorf("expected target %s output to contain allowed node 'US-Fast'", target)
			}
		})
	}
}

func TestValidateCredentialEnvelope_AllSevenProtocols(t *testing.T) {
	ctx := context.Background()

	validCredForProto := func(proto domain.Protocol) domain.InboundProtocolCredential {
		switch proto {
		case domain.ProtocolSS:
			return domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret-ss-password"}
		case domain.ProtocolVMess:
			return domain.InboundProtocolCredential{UUID: "b831381d-6324-4d53-ad4f-8cda48b30811"}
		case domain.ProtocolVLESS:
			return domain.InboundProtocolCredential{
				UUID:      "b831381d-6324-4d53-ad4f-8cda48b30812",
				Transport: map[string]string{"pbk": "secret-reality-pbk", "sid": "01ab"},
			}
		case domain.ProtocolTrojan:
			return domain.InboundProtocolCredential{Password: "secret-trojan-password"}
		case domain.ProtocolHysteria2:
			return domain.InboundProtocolCredential{Password: "secret-hy2-password"}
		case domain.ProtocolWireGuard:
			return domain.InboundProtocolCredential{
				PrivateKey:   "secret-wg-private-key",
				PublicKey:    "secret-wg-public-key",
				PreSharedKey: "secret-wg-psk",
				LocalAddress: []string{"10.0.0.2/32", "fd00::2/128"},
				Reserved:     []uint8{1, 2, 3},
				MTU:          1420,
			}
		case domain.ProtocolTUIC:
			return domain.InboundProtocolCredential{
				UUID:     "b831381d-6324-4d53-ad4f-8cda48b30813",
				Password: "secret-tuic-password",
			}
		default:
			return domain.InboundProtocolCredential{}
		}
	}

	makeSingleNodeSnapshot := func(id, name string, proto domain.Protocol) *resolver.ResolvedPolicySnapshot {
		return &resolver.ResolvedPolicySnapshot{
			SnapshotDigest:  "snap-" + id,
			CompilerVersion: "1.0.0",
			Nodes: []resolver.ResolvedNode{
				{
					LogicalID:   id,
					DisplayName: name,
					Protocol:    proto,
					Server:      "198.51.100.50",
					Port:        443,
					Credentials: validCredForProto(proto),
					Active:      true,
					Position:    0,
				},
			},
			Groups: []resolver.ResolvedGroup{
				{
					ID:        "018f0b6e-4d7a-7abc-8def-0123456789ab",
					Name:      "proxy",
					GroupType: domain.GroupTypeSelect,
					Members: []resolver.ResolvedGroupMember{
						{Kind: resolver.MemberKindNode, TargetID: id, DisplayName: name, Position: 0},
					},
					NodeLogicalIDs: []string{id},
					Position:       0,
				},
			},
			Rules: []resolver.ResolvedRule{
				{ID: "r1", TargetGroupID: "018f0b6e-4d7a-7abc-8def-0123456789ab", TargetGroupName: "proxy", Expression: "MATCH", Position: 0, IsTerminal: true},
			},
		}
	}

	allProtocols := []domain.Protocol{
		domain.ProtocolSS,
		domain.ProtocolVMess,
		domain.ProtocolVLESS,
		domain.ProtocolTrojan,
		domain.ProtocolHysteria2,
		domain.ProtocolWireGuard,
		domain.ProtocolTUIC,
	}

	for _, proto := range allProtocols {
		t.Run("Valid_"+string(proto), func(t *testing.T) {
			id := "node-" + string(proto)
			snap := makeSingleNodeSnapshot(id, "Edge-"+string(proto), proto)
			if _, err := compiler.Compile(ctx, snap, domain.TargetSingBox); err != nil {
				t.Fatalf("expected valid %s credentials to pass envelope check: %v", proto, err)
			}
		})
	}

	type errorCase struct {
		name         string
		proto        domain.Protocol
		mutate       func(*resolver.ResolvedNode)
		secretMarker string
		wantReason   string
	}

	cases := []errorCase{
		{
			name:  "Envelope_EmptyServer",
			proto: domain.ProtocolSS,
			mutate: func(n *resolver.ResolvedNode) {
				n.Server = "   "
			},
			secretMarker: "secret-ss-password",
			wantReason:   "missing server address",
		},
		{
			name:  "Envelope_InvalidPort",
			proto: domain.ProtocolSS,
			mutate: func(n *resolver.ResolvedNode) {
				n.Port = 70000
			},
			secretMarker: "secret-ss-password",
			wantReason:   "invalid port",
		},
		{
			name:  "SS_MissingMethod",
			proto: domain.ProtocolSS,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.Method = ""
			},
			secretMarker: "secret-ss-password",
			wantReason:   "missing required cipher or password",
		},
		{
			name:  "SS_MissingPassword",
			proto: domain.ProtocolSS,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.Password = ""
			},
			secretMarker: "aes-256-gcm",
			wantReason:   "missing required cipher or password",
		},
		{
			name:  "VMess_MissingUUID",
			proto: domain.ProtocolVMess,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.UUID = " "
			},
			wantReason: "missing required uuid in vmess",
		},
		{
			name:  "VMess_NegativeAlterID",
			proto: domain.ProtocolVMess,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.AlterID = -2
			},
			secretMarker: "b831381d-6324-4d53-ad4f-8cda48b30811",
			wantReason:   "invalid alterId in vmess",
		},
		{
			name:  "VLESS_MissingUUID",
			proto: domain.ProtocolVLESS,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.UUID = ""
			},
			secretMarker: "secret-reality-pbk",
			wantReason:   "missing required uuid in vless",
		},
		{
			name:  "VLESS_RealitySidWithoutPbk",
			proto: domain.ProtocolVLESS,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.Transport["pbk"] = ""
			},
			secretMarker: "b831381d-6324-4d53-ad4f-8cda48b30812",
			wantReason:   "missing required reality public key",
		},
		{
			name:  "Trojan_MissingPassword",
			proto: domain.ProtocolTrojan,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.Password = ""
			},
			wantReason: "missing required password in trojan",
		},
		{
			name:  "Hysteria2_MissingPassword",
			proto: domain.ProtocolHysteria2,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.Password = ""
			},
			wantReason: "missing required password in hysteria2",
		},
		{
			name:  "WireGuard_MissingPrivateKey",
			proto: domain.ProtocolWireGuard,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.PrivateKey = ""
			},
			secretMarker: "secret-wg-public-key",
			wantReason:   "missing required private_key in wireguard",
		},
		{
			name:  "WireGuard_MissingPublicKey",
			proto: domain.ProtocolWireGuard,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.PublicKey = ""
			},
			secretMarker: "secret-wg-private-key",
			wantReason:   "missing required public_key in wireguard",
		},
		{
			name:  "WireGuard_MissingLocalAddress",
			proto: domain.ProtocolWireGuard,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.LocalAddress = nil
			},
			secretMarker: "secret-wg-private-key",
			wantReason:   "missing required local_address in wireguard",
		},
		{
			name:  "WireGuard_InvalidLocalAddressCIDR",
			proto: domain.ProtocolWireGuard,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.LocalAddress = []string{"secret-invalid-cidr-token"}
			},
			secretMarker: "secret-invalid-cidr-token",
			wantReason:   "invalid local_address CIDR in wireguard",
		},
		{
			name:  "WireGuard_InvalidReservedLength",
			proto: domain.ProtocolWireGuard,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.Reserved = []uint8{1, 2}
			},
			secretMarker: "secret-wg-private-key",
			wantReason:   "invalid reserved bytes in wireguard",
		},
		{
			name:  "WireGuard_InvalidMTU",
			proto: domain.ProtocolWireGuard,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.MTU = 70000
			},
			secretMarker: "secret-wg-private-key",
			wantReason:   "invalid mtu in wireguard",
		},
		{
			name:  "TUIC_MissingUUID",
			proto: domain.ProtocolTUIC,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.UUID = ""
			},
			secretMarker: "secret-tuic-password",
			wantReason:   "missing required uuid in tuic",
		},
		{
			name:  "TUIC_MissingPassword",
			proto: domain.ProtocolTUIC,
			mutate: func(n *resolver.ResolvedNode) {
				n.Credentials.Password = ""
			},
			secretMarker: "b831381d-6324-4d53-ad4f-8cda48b30813",
			wantReason:   "missing required password in tuic",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "node-" + string(tc.proto)
			snap := makeSingleNodeSnapshot(id, "Edge-"+string(tc.proto), tc.proto)
			tc.mutate(&snap.Nodes[0])

			_, err := compiler.Compile(ctx, snap, domain.TargetSingBox)
			if err == nil {
				t.Fatalf("expected error for case %s, got nil", tc.name)
			}
			var capErr *compiler.CapabilityError
			if !errors.As(err, &capErr) {
				t.Fatalf("expected CapabilityError, got %T: %v", err, err)
			}
			if capErr.Location != "nodes[0]" || capErr.Feature != string(tc.proto) {
				t.Fatalf("unexpected CapabilityError location/feature: %#v", capErr)
			}
			if !strings.Contains(capErr.Reason, tc.wantReason) {
				t.Fatalf("expected reason containing %q, got %q", tc.wantReason, capErr.Reason)
			}
			if tc.secretMarker != "" && strings.Contains(err.Error(), tc.secretMarker) {
				t.Fatalf("error leaked secret marker %q: %v", tc.secretMarker, err)
			}
		})
	}

	t.Run("ExtractWithCredentials_DualFormatCompilesDirectly", func(t *testing.T) {
		yamlSub := `
proxies:
  - name: WG-Dual
    type: wireguard
    server: 198.51.100.88
    port: 51820
    ip: 10.0.0.2/32
    private-key: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
    public-key: "HyCEC7mK3/cd/2d+p4I5dfB3nBvV9uG1D2L8aF+p+A8="
`
		uriSub := `tuic://33333333-3333-4333-8333-333333333333:secret-tuic-uri-pass@198.51.100.89:8443?congestion_control=bbr&udp_relay_mode=native&alpn=h3&sni=tuic.dual.example.com#TUIC-Dual`

		for _, raw := range []string{yamlSub, uriSub} {
			ext, err := parser.ExtractWithCredentials([]byte(raw))
			if err != nil || len(ext.Items) != 1 {
				t.Fatalf("ExtractWithCredentials failed: err=%v items=%d", err, len(ext.Items))
			}
			item := ext.Items[0]
			snap := makeSingleNodeSnapshot(item.Normalized.Node.LogicalID, item.Normalized.Node.DisplayName, item.Normalized.Node.Protocol)
			snap.Nodes[0].Server = item.Normalized.Server
			snap.Nodes[0].Port = item.Normalized.Port
			snap.Nodes[0].Credentials = item.Credentials

			for _, target := range []domain.CompilerTarget{domain.TargetMihomo, domain.TargetSingBox} {
				if _, err := compiler.Compile(ctx, snap, target); err != nil {
					t.Fatalf("expected valid parsed node %s to compile for %s: %v", item.Normalized.Node.DisplayName, target, err)
				}
			}
		}
	})
}

func TestValidateSnapshot_GroupsRulesAndEmptyBoundary(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptySnapshotAndZeroNodeSnapshotAllowed", func(t *testing.T) {
		emptySnap := &resolver.ResolvedPolicySnapshot{
			SnapshotDigest:  "empty-digest",
			CompilerVersion: "1.0.0",
		}
		for _, target := range compiler.SortedCapabilities() {
			res, err := compiler.Compile(ctx, emptySnap, target)
			if err != nil {
				t.Fatalf("expected empty snapshot to succeed for target %s: %v", target, err)
			}
			if len(res.Content) == 0 {
				t.Fatalf("expected non-empty rendered skeleton for target %s", target)
			}
		}
	})

	t.Run("InvalidNodeFieldsRejected", func(t *testing.T) {
		snap := fixtureSnapshot()
		snap.Nodes[0].DisplayName = "   "
		_, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "nodes[0]" {
			t.Fatalf("expected nodes[0] CapabilityError for blank display name, got %v", err)
		}
	})

	t.Run("InvalidGroupAndMemberReferencesRejected", func(t *testing.T) {
		snap := fixtureSnapshot()
		snap.Groups[0].Name = "  "
		_, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "groups[0]" {
			t.Fatalf("expected groups[0] CapabilityError for blank group name, got %v", err)
		}

		snap2 := fixtureSnapshot()
		snap2.Groups[0].Members = append(snap2.Groups[0].Members, resolver.ResolvedGroupMember{
			Kind:        resolver.MemberKindNode,
			TargetID:    "missing-node-id",
			DisplayName: "ghost-node",
			Position:    2,
		})
		_, err = compiler.Compile(ctx, snap2, domain.TargetMihomo)
		if !errors.As(err, &capErr) || capErr.Location != "groups[0]" || !strings.Contains(capErr.Reason, "unknown node") {
			t.Fatalf("expected unknown node member error at groups[0], got %v", err)
		}

		snap3 := fixtureSnapshot()
		snap3.Groups[0].Members = append(snap3.Groups[0].Members, resolver.ResolvedGroupMember{
			Kind:        resolver.MemberKindGroup,
			TargetID:    "missing-group-id",
			DisplayName: "ghost-group",
			Position:    2,
		})
		_, err = compiler.Compile(ctx, snap3, domain.TargetMihomo)
		if !errors.As(err, &capErr) || capErr.Location != "groups[0]" || !strings.Contains(capErr.Reason, "unknown policy group") {
			t.Fatalf("expected unknown policy group member error at groups[0], got %v", err)
		}
	})

	t.Run("InvalidRuleExpressionsAndTargetReferencesRejected", func(t *testing.T) {
		snap := fixtureSnapshot()
		snap.Rules[0].Expression = "DOMAIN-SUFFIX,"
		_, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
		var capErr *compiler.CapabilityError
		if !errors.As(err, &capErr) || capErr.Location != "rules[0]" || !strings.Contains(capErr.Reason, "rule value is required") {
			t.Fatalf("expected rules[0] value error, got %v", err)
		}

		snap2 := fixtureSnapshot()
		snap2.Rules[0].TargetGroupName = "NonExistentGroup"
		_, err = compiler.Compile(ctx, snap2, domain.TargetMihomo)
		if !errors.As(err, &capErr) || capErr.Location != "rules[0]" || !strings.Contains(capErr.Reason, "unknown target group") {
			t.Fatalf("expected rules[0] unknown target group error, got %v", err)
		}

		snapBuiltIn := fixtureSnapshot()
		snapBuiltIn.Rules[0].TargetGroupName = "DIRECT"
		snapBuiltIn.Rules[0].TargetGroupID = ""
		if _, err := compiler.Compile(ctx, snapBuiltIn, domain.TargetMihomo); err != nil {
			t.Fatalf("expected built-in DIRECT target in rule to succeed: %v", err)
		}
	})
}
