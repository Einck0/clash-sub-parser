package compiler_test

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/adapter/outboundgroup"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	MC "github.com/metacubex/mihomo/context"
	ML "github.com/metacubex/mihomo/log"
	R "github.com/metacubex/mihomo/rules"
	"github.com/metacubex/mihomo/tunnel"
	"gopkg.in/yaml.v3"
)

func passSnapshot() *resolver.ResolvedPolicySnapshot {
	return &resolver.ResolvedPolicySnapshot{Groups: []resolver.ResolvedGroup{{ID: "empty", Name: "empty", GroupType: domain.GroupTypeSelect, EmptyFallbackPass: true, EffectiveEmptyPass: true}}, Rules: []resolver.ResolvedRule{{Expression: "DOMAIN,fixture.invalid", TargetGroupID: "empty", TargetGroupName: "empty"}, {Expression: "MATCH", TargetGroupName: "REJECT", IsTerminal: true}}}
}

func TestExplicitEmptyPassCompilerAndCapabilities(t *testing.T) {
	ctx := context.Background()
	snap := passSnapshot()
	rendered, err := compiler.Compile(ctx, snap, domain.TargetMihomo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered.Content), "empty-fallback: PASS") || !strings.Contains(string(rendered.Content), "- PASS") || strings.Contains(string(rendered.Content), "COMPATIBLE") || strings.Contains(string(rendered.Content), "DIRECT") {
		t.Fatalf("invalid explicit PASS output: %s", rendered.Content)
	}
	pruned, diags, err := compiler.PruneUnavailableOptionalGroups(snap)
	if err != nil || len(pruned.Groups) != 1 || len(diags) != 0 {
		t.Fatalf("PASS pruned: %+v %v", diags, err)
	}
	for _, target := range []domain.CompilerTarget{domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX} {
		if _, err := compiler.Compile(ctx, snap, target); err == nil {
			t.Fatalf("%s accepted unsupported PASS routing", target)
		}
	}
	snap.Groups[0].EmptyFallbackPass = false
	if _, err := compiler.Compile(ctx, snap, domain.TargetMihomo); err == nil || !strings.Contains(err.Error(), "required_nonempty") {
		t.Fatalf("unchecked group did not stay strict: %v", err)
	}
}

// The actual official tunnel handles a net.Pipe connection. No listener, DNS,
// geodata, HTTP health check or remote socket is required. A subprocess confines
// Mihomo's global tunnel/proxy state and goroutines to this fixture.
func TestMihomoNativePassContinuesRules(t *testing.T) {
	if os.Getenv("CSP_NATIVE_PASS_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMihomoNativePassContinuesRules$")
		cmd.Env = append(os.Environ(), "CSP_NATIVE_PASS_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native Mihomo rule fixture: %v\n%s", err, output)
		}
		return
	}
	rendered, err := compiler.Compile(context.Background(), passSnapshot(), domain.TargetMihomo)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Groups []map[string]any `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(rendered.Content, &raw); err != nil {
		t.Fatal(err)
	}
	proxies := map[string]C.Proxy{"PASS": adapter.NewProxy(outbound.NewPass()), "REJECT": adapter.NewProxy(outbound.NewReject()), "DIRECT": adapter.NewProxy(outbound.NewDirect())}
	providers := map[string]P.ProxyProvider{}
	for _, g := range raw.Groups {
		group, err := outboundgroup.ParseProxyGroup(g, proxies, providers, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		proxies[group.Name()] = adapter.NewProxy(group)
	}
	first, err := R.ParseRule("DOMAIN", "fixture.invalid", "empty", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := R.ParseRule("MATCH", "", "REJECT", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tunnel.UpdateProxies(proxies, providers)
	tunnel.UpdateRules([]C.Rule{first, next}, nil, nil)
	tunnel.SetMode(tunnel.Rule)
	tunnel.OnRunning()
	sub := ML.Subscribe()
	defer ML.UnSubscribe(sub)
	client, server := net.Pipe()
	defer client.Close()
	metadata := &C.Metadata{NetWork: C.TCP, Type: C.INNER, Host: "fixture.invalid", DstIP: netip.MustParseAddr("192.0.2.1"), DstPort: 443, SrcIP: netip.MustParseAddr("127.0.0.1"), SrcPort: 12345}
	tunnel.TCPIn() <- MC.NewConnContext(server, metadata)
	go func() { _, _ = client.Write([]byte("x")) }()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	sawPass := false
	for {
		select {
		case event := <-sub:
			if strings.Contains(event.Payload, "match Pass rule") {
				sawPass = true
			}
			if strings.Contains(event.Payload, "match Match using REJECT") {
				if !sawPass {
					t.Fatal("control matched without traversing PASS")
				}
				return
			}
			if strings.Contains(event.Payload, "using DIRECT") || strings.Contains(event.Payload, "using COMPATIBLE") {
				t.Fatalf("hidden direct fallback: %s", event.Payload)
			}
		case <-timeout.C:
			t.Fatal("native tunnel did not reach non-DIRECT control rule")
		}
	}
}
