package domain_test

import (
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestProtocolEnums(t *testing.T) {
	validProtocols := []domain.Protocol{
		domain.ProtocolSS,
		domain.ProtocolVMess,
		domain.ProtocolVLESS,
		domain.ProtocolTrojan,
		domain.ProtocolHysteria2,
		domain.ProtocolWireGuard,
		domain.ProtocolTUIC,
	}

	for _, p := range validProtocols {
		if !p.IsValid() {
			t.Errorf("expected protocol %s to be valid", p)
		}
		parsed, err := domain.ParseProtocol(string(p))
		if err != nil {
			t.Errorf("ParseProtocol(%s) failed: %v", p, err)
		}
		if parsed != p {
			t.Errorf("ParseProtocol mismatch: expected %s, got %s", p, parsed)
		}
	}

	invalidProtocols := []string{"", "http", "socks5", "shadowsocksr", "openvpn"}
	for _, ip := range invalidProtocols {
		if domain.Protocol(ip).IsValid() {
			t.Errorf("expected invalid protocol %s to return false from IsValid", ip)
		}
		if _, err := domain.ParseProtocol(ip); err == nil {
			t.Errorf("expected ParseProtocol(%s) to fail, got nil err", ip)
		}
	}
}

func TestProbeVerdictEnums(t *testing.T) {
	validVerdicts := []domain.ProbeVerdict{
		domain.VerdictAvailable,
		domain.VerdictRestricted,
		domain.VerdictUnknown,
		domain.VerdictError,
		domain.VerdictStale,
	}

	for _, v := range validVerdicts {
		if !v.IsValid() {
			t.Errorf("expected verdict %s to be valid", v)
		}
		parsed, err := domain.ParseProbeVerdict(string(v))
		if err != nil {
			t.Errorf("ParseProbeVerdict(%s) failed: %v", v, err)
		}
		if parsed != v {
			t.Errorf("ParseProbeVerdict mismatch: expected %s, got %s", v, parsed)
		}
	}

	invalidVerdicts := []string{"", "passed", "ok", "success", "failed"}
	for _, iv := range invalidVerdicts {
		if domain.ProbeVerdict(iv).IsValid() {
			t.Errorf("expected invalid verdict %s to return false", iv)
		}
		if _, err := domain.ParseProbeVerdict(iv); err == nil {
			t.Errorf("expected ParseProbeVerdict(%s) to return error", iv)
		}
	}
}

func TestProbeRunStateEnums(t *testing.T) {
	terminalStates := map[domain.ProbeRunState]bool{
		domain.ProbeRunStateSucceeded: true,
		domain.ProbeRunStateFailed:    true,
		domain.ProbeRunStateCancelled: true,
		domain.ProbeRunStateExpired:   true,
	}

	nonTerminalStates := map[domain.ProbeRunState]bool{
		domain.ProbeRunStateQueued:  false,
		domain.ProbeRunStateRunning: false,
	}

	for s, expected := range terminalStates {
		if !s.IsValid() {
			t.Errorf("state %s should be valid", s)
		}
		if s.IsTerminal() != expected {
			t.Errorf("state %s IsTerminal() expected %v, got %v", s, expected, s.IsTerminal())
		}
	}

	for s, expected := range nonTerminalStates {
		if !s.IsValid() {
			t.Errorf("state %s should be valid", s)
		}
		if s.IsTerminal() != expected {
			t.Errorf("state %s IsTerminal() expected %v, got %v", s, expected, s.IsTerminal())
		}
	}
}

func TestCompilerTargetEnums(t *testing.T) {
	validTargets := []domain.CompilerTarget{
		domain.TargetClash,
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}

	for _, target := range validTargets {
		if !target.IsValid() {
			t.Errorf("target %s should be valid", target)
		}
		parsed, err := domain.ParseCompilerTarget(string(target))
		if err != nil {
			t.Errorf("ParseCompilerTarget(%s) failed: %v", target, err)
		}
		if parsed != target {
			t.Errorf("parsed target mismatch: expected %s, got %s", target, parsed)
		}
	}
}

func TestProbeKindEnums(t *testing.T) {
	validKinds := []domain.ProbeKind{
		domain.ProbeKindBaseline,
		domain.ProbeKindGeo,
		domain.ProbeKindStreaming,
		domain.ProbeKindAI,
		domain.ProbeKindSpeed,
		domain.ProbeKindIPRisk,
	}

	for _, k := range validKinds {
		if !k.IsValid() {
			t.Errorf("probe kind %s should be valid", k)
		}
		parsed, err := domain.ParseProbeKind(string(k))
		if err != nil {
			t.Errorf("ParseProbeKind(%s) failed: %v", k, err)
		}
		if parsed != k {
			t.Errorf("parsed probe kind mismatch: expected %s, got %s", k, parsed)
		}
	}

	invalidKinds := []string{"", "unknown_kind", "latency", "iprisk"}
	for _, ik := range invalidKinds {
		if domain.ProbeKind(ik).IsValid() {
			t.Errorf("invalid kind %s should return false from IsValid()", ik)
		}
		if _, err := domain.ParseProbeKind(ik); err == nil {
			t.Errorf("ParseProbeKind(%s) should return error", ik)
		}
	}
}

