package domain_test

import (
	"encoding/json"
	"testing"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestNodeGroup_Validation(t *testing.T) {
	tests := []struct {
		name    string
		group   domain.NodeGroup
		wantErr bool
	}{
		{
			name: "valid select group",
			group: domain.NodeGroup{
				Name:      "Proxy",
				Kind:      domain.GroupKindManual,
				GroupType: domain.GroupTypeSelect,
			},
			wantErr: false,
		},
		{
			name: "valid url-test group with default config",
			group: domain.NodeGroup{
				Name:      "Auto-Select",
				Kind:      domain.GroupKindAuto,
				GroupType: domain.GroupTypeURLTest,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			group: domain.NodeGroup{
				Name:      "",
				GroupType: domain.GroupTypeSelect,
			},
			wantErr: true,
		},
		{
			name: "invalid group type",
			group: domain.NodeGroup{
				Name:      "Bad Group",
				GroupType: "non-existent-type",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.group.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("NodeGroup.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroup_EffectiveConfigs(t *testing.T) {
	grp := domain.NodeGroup{
		Name:      "Fastest",
		GroupType: domain.GroupTypeURLTest,
	}

	cfg := grp.EffectiveURLTestConfig()
	if cfg.URL == "" || cfg.Interval == 0 {
		t.Errorf("EffectiveURLTestConfig() didn't provide sensible defaults: %+v", cfg)
	}

	fbGrp := domain.NodeGroup{
		Name:      "Fallback-Group",
		GroupType: domain.GroupTypeFallback,
	}
	fbCfg := fbGrp.EffectiveFallbackConfig()
	if fbCfg.URL == "" || fbCfg.Interval == 0 {
		t.Errorf("EffectiveFallbackConfig() didn't provide sensible defaults: %+v", fbCfg)
	}
}

func TestNodeGroup_Matching(t *testing.T) {
	grp := domain.NodeGroup{
		Name:         "Hong Kong Nodes",
		GroupType:    domain.GroupTypeSelect,
		RegexRules:   []string{"(?i)HK", "(?i)Hong\\s*Kong"},
		ExcludeNodes: []string{"Traffic-Limit"},
	}

	nodeHK := &domain.Node{Name: "HK - 01 (Premium)"}
	nodeJP := &domain.Node{Name: "JP - 01 (Tokyo)"}
	nodeExcluded := &domain.Node{Name: "HK Traffic-Limit"}

	if !grp.MatchesNode(nodeHK) {
		t.Errorf("expected nodeHK to match grp")
	}
	if grp.MatchesNode(nodeJP) {
		t.Errorf("expected nodeJP not to match grp")
	}
	if grp.MatchesNode(nodeExcluded) {
		t.Errorf("expected nodeExcluded not to match grp")
	}
}

func TestNodeGroup_Serialization(t *testing.T) {
	minSpeed := 50.0
	grp := domain.NodeGroup{
		ID:                 5,
		Name:               "Streaming",
		Kind:               domain.GroupKindAuto,
		GroupType:          domain.GroupTypeURLTest,
		SortOrder:          1,
		RegexRules:         []string{"(?i)US", "(?i)HK"},
		FilterMinSpeedMbps: &minSpeed,
		FilterMediaUnlock:  []string{"netflix", "disney"},
		IncludeNodes:       []string{"DIRECT"},
		IncludeGroupIDs:    []int64{1, 2},
		AddFallback:        true,
		URLTestConfig: domain.URLTestConfig{
			URL:       "https://cp.cloudflare.com/generate_204",
			Interval:  300,
			Tolerance: 50,
		},
	}

	// JSON
	data, err := json.Marshal(grp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var unmarshaled domain.NodeGroup
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if unmarshaled.Name != grp.Name || unmarshaled.URLTestConfig.Tolerance != 50 {
		t.Errorf("JSON mismatch: %+v vs %+v", unmarshaled, grp)
	}

	// YAML
	yamlBytes, err := yaml.Marshal(grp)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	var yamlGrp domain.NodeGroup
	if err := yaml.Unmarshal(yamlBytes, &yamlGrp); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if yamlGrp.GroupType != domain.GroupTypeURLTest {
		t.Errorf("YAML mismatch: %+v vs %+v", yamlGrp, grp)
	}
}
