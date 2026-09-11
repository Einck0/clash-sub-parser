package domain

import (
	"strings"
)

// ExportTarget represents supported client configuration targets.
type ExportTarget string

const (
	TargetClash        ExportTarget = "clash"
	TargetMihomo       ExportTarget = "mihomo"
	TargetStash        ExportTarget = "stash"
	TargetSingBox      ExportTarget = "sing-box"
	TargetSurge        ExportTarget = "surge"
	TargetLoon         ExportTarget = "loon"
	TargetQuantumultX  ExportTarget = "quantumult-x"
	TargetShadowrocket ExportTarget = "shadowrocket"
)

// GenerateConfig encapsulates global configuration toggles for client config generation.
type GenerateConfig struct {
	ID                 int64 `json:"id" yaml:"id"`
	Enabled            bool  `json:"enabled" yaml:"enabled"`
	Subscriptions      bool  `json:"subscriptions" yaml:"subscriptions"`
	NodeGroups         bool  `json:"node_groups" yaml:"node_groups"`
	Rules              bool  `json:"rules" yaml:"rules"`
	DNS                bool  `json:"dns" yaml:"dns"`
	ExcludeNodeProxies bool  `json:"exclude_node_proxies" yaml:"exclude_node_proxies"`
}

// DefaultGenerateConfig produces standard default generation switches.
func DefaultGenerateConfig() *GenerateConfig {
	return &GenerateConfig{
		ID:                 1,
		Enabled:            true,
		Subscriptions:      true,
		NodeGroups:         true,
		Rules:              true,
		DNS:                true,
		ExcludeNodeProxies: true,
	}
}

// Validate ensures GenerateConfig holds consistent state.
func (g *GenerateConfig) Validate() error {
	return nil
}

// GenerateRequest represents a specific client export request and its options.
type GenerateRequest struct {
	Target               ExportTarget    `json:"target" yaml:"target"`
	Format               string          `json:"format,omitempty" yaml:"format,omitempty"`
	IncludeUnchecked     bool            `json:"include_unchecked" yaml:"include_unchecked"`
	FilterGroups         []string        `json:"filter_groups,omitempty" yaml:"filter_groups,omitempty"`
	Switches             map[string]bool `json:"switches,omitempty" yaml:"switches,omitempty"`
	CustomRuleCategories []string        `json:"custom_rule_categories,omitempty" yaml:"custom_rule_categories,omitempty"`
}

// DefaultGenerateRequest returns a request with standard options for the given target.
func DefaultGenerateRequest(target ExportTarget) *GenerateRequest {
	return &GenerateRequest{
		Target:           target,
		IncludeUnchecked: true,
		Switches: map[string]bool{
			"subscriptions":        true,
			"node_groups":          true,
			"rules":                true,
			"dns":                  true,
			"exclude_node_proxies": true,
		},
	}
}

// Validate checks that the requested export target is supported.
func (r *GenerateRequest) Validate() error {
	norm := ExportTarget(strings.ToLower(strings.TrimSpace(string(r.Target))))
	switch norm {
	case TargetClash, TargetMihomo, TargetStash, TargetSingBox,
		TargetSurge, TargetLoon, TargetQuantumultX, TargetShadowrocket:
		return nil
	default:
		return ErrUnsupportedExportTarget
	}
}
