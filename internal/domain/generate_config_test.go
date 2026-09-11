package domain_test

import (
	"encoding/json"
	"testing"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestGenerateConfig_Defaults(t *testing.T) {
	cfg := domain.DefaultGenerateConfig()
	if !cfg.Enabled || !cfg.Subscriptions || !cfg.NodeGroups || !cfg.Rules || !cfg.DNS || !cfg.ExcludeNodeProxies {
		t.Errorf("DefaultGenerateConfig() flags should all default to true: %+v", cfg)
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultGenerateConfig().Validate() error = %v", err)
	}
}

func TestGenerateRequest_TargetsAndValidation(t *testing.T) {
	validTargets := []domain.ExportTarget{
		domain.TargetClash,
		domain.TargetMihomo,
		domain.TargetStash,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetLoon,
		domain.TargetQuantumultX,
		domain.TargetShadowrocket,
	}

	for _, target := range validTargets {
		req := domain.DefaultGenerateRequest(target)
		if err := req.Validate(); err != nil {
			t.Errorf("request for target %s failed validation: %v", target, err)
		}
		if req.Target != target {
			t.Errorf("request target mismatch: got %s, want %s", req.Target, target)
		}
	}

	invalidReq := domain.GenerateRequest{
		Target: "unsupported-client-xyz",
	}
	if err := invalidReq.Validate(); err == nil {
		t.Errorf("expected error for unsupported target, got nil")
	}
}

func TestGenerateConfig_Serialization(t *testing.T) {
	cfg := domain.GenerateConfig{
		ID:                  1,
		Enabled:             true,
		Subscriptions:       true,
		NodeGroups:          false,
		Rules:               true,
		DNS:                 false,
		ExcludeNodeProxies:  true,
	}

	// JSON
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled domain.GenerateConfig
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if unmarshaled.NodeGroups != false || unmarshaled.Rules != true {
		t.Errorf("JSON mismatch: %+v vs %+v", unmarshaled, cfg)
	}

	// YAML
	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	var yamlCfg domain.GenerateConfig
	if err := yaml.Unmarshal(yamlBytes, &yamlCfg); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if yamlCfg.DNS != false {
		t.Errorf("YAML mismatch: %+v vs %+v", yamlCfg, cfg)
	}
}
