package domain

import (
	"regexp"
	"strings"
)

// GroupKind categorizes how nodes are assembled into this group.
type GroupKind string

const (
	GroupKindManual  GroupKind = "manual"
	GroupKindAuto    GroupKind = "auto"
	GroupKindFilter  GroupKind = "filter"
	GroupKindSpecial GroupKind = "special"
)

// GroupType defines the behavioral routing strategy of this proxy group.
type GroupType string

const (
	GroupTypeSelect      GroupType = "select"
	GroupTypeURLTest     GroupType = "url-test"
	GroupTypeFallback    GroupType = "fallback"
	GroupTypeLoadBalance GroupType = "load-balance"
)

// URLTestConfig holds parameters for url-test (auto fastest) groups.
type URLTestConfig struct {
	URL       string `json:"url,omitempty" yaml:"url,omitempty"`
	Interval  int    `json:"interval,omitempty" yaml:"interval,omitempty"`
	Tolerance int    `json:"tolerance,omitempty" yaml:"tolerance,omitempty"`
}

// FallbackConfig holds parameters for fallback groups.
type FallbackConfig struct {
	URL      string `json:"url,omitempty" yaml:"url,omitempty"`
	Interval int    `json:"interval,omitempty" yaml:"interval,omitempty"`
}

// LoadBalanceConfig holds parameters for load-balance groups.
type LoadBalanceConfig struct {
	Strategy string `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	URL      string `json:"url,omitempty" yaml:"url,omitempty"`
	Interval int    `json:"interval,omitempty" yaml:"interval,omitempty"`
}

// GroupIncludeEntry defines composite inclusion items (e.g. nested groups or regex).
type GroupIncludeEntry struct {
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value" yaml:"value"`
}

// NodeGroup represents a routing policy group (e.g. Select, Auto-Fastest, Fallback).
type NodeGroup struct {
	ID                  int64               `json:"id" yaml:"id"`
	Name                string              `json:"name" yaml:"name"`
	Kind                GroupKind           `json:"kind" yaml:"kind"`
	GroupType           GroupType           `json:"group_type" yaml:"group_type"`
	SortOrder           int                 `json:"sort_order" yaml:"sort_order"`
	RegexRules          []string            `json:"regex_rules,omitempty" yaml:"regex_rules,omitempty"`
	FilterMinSpeedMbps  *float64            `json:"filter_min_speed_mbps,omitempty" yaml:"filter_min_speed_mbps,omitempty"`
	FilterMediaUnlock   []string            `json:"filter_media_unlock,omitempty" yaml:"filter_media_unlock,omitempty"`
	IncludeNodes        []string            `json:"include_nodes,omitempty" yaml:"include_nodes,omitempty"`
	IncludeGroupIDs     []int64             `json:"include_group_ids,omitempty" yaml:"include_group_ids,omitempty"`
	IncludeGroupNodesIDs []int64            `json:"include_group_nodes_ids,omitempty" yaml:"include_group_nodes_ids,omitempty"`
	IncludeEntries      []GroupIncludeEntry `json:"include_entries,omitempty" yaml:"include_entries,omitempty"`
	AddFallback         bool                `json:"add_fallback" yaml:"add_fallback"`
	ExcludeNodes        []string            `json:"exclude_nodes,omitempty" yaml:"exclude_nodes,omitempty"`
	ExcludeGroupIDs     []int64             `json:"exclude_group_ids,omitempty" yaml:"exclude_group_ids,omitempty"`
	URLTestConfig       URLTestConfig       `json:"url_test_config,omitempty" yaml:"url_test_config,omitempty"`
	LoadBalanceConfig   LoadBalanceConfig   `json:"load_balance_config,omitempty" yaml:"load_balance_config,omitempty"`
	FallbackConfig      FallbackConfig      `json:"fallback_config,omitempty" yaml:"fallback_config,omitempty"`
	ResolvedNodeNames   []string            `json:"resolved_node_names,omitempty" yaml:"resolved_node_names,omitempty"`
}

// Validate checks that the group configuration adheres to domain rules.
func (g *NodeGroup) Validate() error {
	if strings.TrimSpace(g.Name) == "" {
		return ErrInvalidName
	}
	switch g.GroupType {
	case GroupTypeSelect, GroupTypeURLTest, GroupTypeFallback, GroupTypeLoadBalance:
		// Valid
	default:
		return ErrInvalidGroupType
	}
	return nil
}

// IsURLTest reports whether the group is an auto-latency URL test group.
func (g *NodeGroup) IsURLTest() bool {
	return g.GroupType == GroupTypeURLTest
}

// IsFallback reports whether the group is a fallback group.
func (g *NodeGroup) IsFallback() bool {
	return g.GroupType == GroupTypeFallback
}

// IsSelect reports whether the group is a manual selector group.
func (g *NodeGroup) IsSelect() bool {
	return g.GroupType == GroupTypeSelect
}

// TypeString returns the string representation of GroupType.
func (g *NodeGroup) TypeString() string {
	return string(g.GroupType)
}

// EffectiveURLTestConfig returns the configured URLTestConfig or sensible defaults.
func (g *NodeGroup) EffectiveURLTestConfig() URLTestConfig {
	cfg := g.URLTestConfig
	if cfg.URL == "" {
		cfg.URL = "https://cp.cloudflare.com/generate_204"
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 300
	}
	if cfg.Tolerance <= 0 {
		cfg.Tolerance = 50
	}
	return cfg
}

// EffectiveFallbackConfig returns the configured FallbackConfig or sensible defaults.
func (g *NodeGroup) EffectiveFallbackConfig() FallbackConfig {
	cfg := g.FallbackConfig
	if cfg.URL == "" {
		cfg.URL = "https://cp.cloudflare.com/generate_204"
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 300
	}
	return cfg
}

// MatchesNode determines if a candidate Node should be included in this group.
func (g *NodeGroup) MatchesNode(node *Node) bool {
	if node == nil {
		return false
	}

	// 1. Check exclusions
	for _, excl := range g.ExcludeNodes {
		trimmed := strings.TrimSpace(excl)
		if trimmed == "" {
			continue
		}
		if strings.Contains(node.Name, trimmed) {
			return false
		}
		re, err := regexp.Compile(trimmed)
		if err == nil && re.MatchString(node.Name) {
			return false
		}
	}

	// 2. Check explicit inclusion list
	for _, inc := range g.IncludeNodes {
		if strings.TrimSpace(inc) == node.Name {
			return true
		}
	}

	// 3. Check regex rules
	if len(g.RegexRules) > 0 {
		for _, pattern := range g.RegexRules {
			trimmed := strings.TrimSpace(pattern)
			if trimmed == "" {
				continue
			}
			re, err := regexp.Compile(trimmed)
			if err == nil && re.MatchString(node.Name) {
				return true
			}
		}
		return false
	}

	return len(g.IncludeNodes) == 0
}
