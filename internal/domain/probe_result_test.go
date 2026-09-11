package domain_test

import (
	"encoding/json"
	"testing"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestNodeProbeResult_Validation(t *testing.T) {
	tests := []struct {
		name    string
		result  domain.NodeProbeResult
		wantErr bool
	}{
		{
			name: "valid online probe result",
			result: domain.NodeProbeResult{
				NodeKey: "1.1.1.1:443:vless",
				Name:    "HK Node",
				Server:  "1.1.1.1",
				Port:    443,
				Type:    "vless",
				Status:  domain.ProbeStatusOK,
			},
			wantErr: false,
		},
		{
			name: "missing node key",
			result: domain.NodeProbeResult{
				Name:   "HK Node",
				Server: "1.1.1.1",
				Port:   443,
				Status: domain.ProbeStatusOK,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			result: domain.NodeProbeResult{
				NodeKey: "1.1.1.1:443:vless",
				Status:  "unknown-status-xyz",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.result.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("NodeProbeResult.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeProbeResult_UnlockHelpers(t *testing.T) {
	latency := int64(85)
	speed := 128.5
	asn := int64(13335)

	res := domain.NodeProbeResult{
		NodeKey:      "hk.test:443:vless",
		Name:         "HK-01",
		Server:       "hk.test",
		Port:         443,
		Type:         "vless",
		Status:       domain.ProbeStatusOK,
		LatencyMs:    &latency,
		SpeedMbps:    &speed,
		IP:           "104.16.1.1",
		Country:      "HK",
		ASN:          &asn,
		Organization: "Cloudflare",
		Media: domain.MediaUnlockInfo{
			Netflix: "Full",
			YouTube: "Premium",
			Disney:  "Yes",
			ChatGPT: "Yes",
			Gemini:  "Yes",
			Claude:  "No",
		},
		CheckedAt: 1789090000,
	}

	if !res.IsAvailable() {
		t.Errorf("expected res to be available")
	}
	if !res.HasMediaUnlock("netflix") {
		t.Errorf("expected netflix unlock to be true")
	}
	if !res.HasAIUnlock("chatgpt") {
		t.Errorf("expected chatgpt unlock to be true")
	}
	if !res.HasAIUnlock("gemini") {
		t.Errorf("expected gemini unlock to be true")
	}
	if res.HasAIUnlock("claude") {
		t.Errorf("expected claude unlock to be false")
	}
}

func TestNodeProbeResult_Serialization(t *testing.T) {
	latency := int64(120)
	res := domain.NodeProbeResult{
		ID:        1001,
		NodeKey:   "us-01.example.com:443:trojan",
		Name:      "US-01",
		Server:    "us-01.example.com",
		Port:      443,
		Type:      "trojan",
		Status:    domain.ProbeStatusOK,
		LatencyMs: &latency,
		Country:   "US",
		Media: domain.MediaUnlockInfo{
			Netflix: "Yes",
			ChatGPT: "Yes",
		},
		CheckedAt: 1789091234,
	}

	// JSON
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled domain.NodeProbeResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if unmarshaled.NodeKey != res.NodeKey || unmarshaled.Media.Netflix != "Yes" {
		t.Errorf("JSON mismatch: %+v vs %+v", unmarshaled, res)
	}

	// YAML
	yamlBytes, err := yaml.Marshal(res)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	var yamlRes domain.NodeProbeResult
	if err := yaml.Unmarshal(yamlBytes, &yamlRes); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if yamlRes.Country != "US" {
		t.Errorf("YAML mismatch: %+v vs %+v", yamlRes, res)
	}
}
