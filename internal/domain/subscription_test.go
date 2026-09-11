package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestSubscription_Validation(t *testing.T) {
	tests := []struct {
		name    string
		sub     domain.Subscription
		wantErr bool
	}{
		{
			name: "valid subscription",
			sub: domain.Subscription{
				Name:           "Test Provider",
				URL:            "https://example.com/sub",
				UpdateInterval: 24,
				Enabled:        true,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			sub: domain.Subscription{
				Name: "",
				URL:  "https://example.com/sub",
			},
			wantErr: true,
		},
		{
			name: "missing URL",
			sub: domain.Subscription{
				Name: "Test Provider",
				URL:  "",
			},
			wantErr: true,
		},
		{
			name: "negative update interval",
			sub: domain.Subscription{
				Name:           "Test Provider",
				URL:            "https://example.com/sub",
				UpdateInterval: -5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sub.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Subscription.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubscription_FilteringAndRenaming(t *testing.T) {
	sub := domain.Subscription{
		Name:             "Provider A",
		URL:              "https://example.com/sub",
		NodePrefix:       "⚡ ",
		FilterRegex:      []string{"(?i)HK", "(?i)US", "(?i)JP"},
		ExcludeNodeNames: []string{"Expired", "Traffic"},
		NodeRenames: map[string]string{
			"Hong Kong 01": "HK Premium 01",
		},
	}

	// Filter test
	if !sub.MatchesFilter("HK - 01") {
		t.Errorf("expected 'HK - 01' to match filter")
	}
	if !sub.MatchesFilter("JP Tokyo 02") {
		t.Errorf("expected 'JP Tokyo 02' to match filter")
	}
	if sub.MatchesFilter("SG Singapore 01") {
		t.Errorf("expected 'SG Singapore 01' to not match filter")
	}
	if sub.IsExcluded("Expired Node") {
		// Should be excluded
	} else {
		t.Errorf("expected 'Expired Node' to be excluded")
	}

	// Rename test
	renamed := sub.ApplyRename("Hong Kong 01")
	if renamed != "HK Premium 01" {
		t.Errorf("ApplyRename('Hong Kong 01') = %q, want 'HK Premium 01'", renamed)
	}

	// Prefix test
	prefixed := sub.FormatNodeName("HK Premium 01")
	if prefixed != "⚡ HK Premium 01" {
		t.Errorf("FormatNodeName('HK Premium 01') = %q, want '⚡ HK Premium 01'", prefixed)
	}
}

func TestSubscription_Serialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	sub := domain.Subscription{
		ID:                    1,
		Name:                  "Primary Sub",
		URL:                   "https://example.com/sub",
		UpdateInterval:        12,
		IsPrimary:             true,
		Enabled:               true,
		NodePrefix:            "[PRO] ",
		FilterRegex:           []string{"HK", "US"},
		FilterMediaUnlock:     []string{"netflix", "chatgpt"},
		IncludeNodeNames:      []string{"Special Node"},
		ExcludeNodeNames:      []string{"Backlog"},
		NodeRenames:           map[string]string{"Old": "New"},
		LastFetchedAt:         &now,
		FetchFailedCount:      0,
		SubscriptionUserinfo:  "upload=100; download=200; total=1000; expire=1789000000",
		ProfileUpdateInterval: "12h",
		ProfileWebPageURL:     "https://example.com",
	}

	// JSON Roundtrip
	data, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var jsonSub domain.Subscription
	if err := json.Unmarshal(data, &jsonSub); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if jsonSub.Name != sub.Name || jsonSub.URL != sub.URL || !jsonSub.IsPrimary {
		t.Errorf("JSON unmarshaled subscription mismatch: got %+v, want %+v", jsonSub, sub)
	}

	// YAML Roundtrip
	yamlData, err := yaml.Marshal(sub)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}

	var yamlSub domain.Subscription
	if err := yaml.Unmarshal(yamlData, &yamlSub); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	if yamlSub.Name != sub.Name || yamlSub.NodePrefix != sub.NodePrefix {
		t.Errorf("YAML unmarshaled subscription mismatch: got %+v, want %+v", yamlSub, sub)
	}
}
