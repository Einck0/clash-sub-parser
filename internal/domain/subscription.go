package domain

import (
	"regexp"
	"strings"
	"time"
)

// Subscription represents a proxy subscription source and its transformation rules.
type Subscription struct {
	ID                    int64             `json:"id" yaml:"id"`
	Name                  string            `json:"name" yaml:"name"`
	URL                   string            `json:"url" yaml:"url"`
	UpdateInterval        int               `json:"update_interval,omitempty" yaml:"update_interval,omitempty"`
	IsPrimary             bool              `json:"is_primary" yaml:"is_primary"`
	Enabled               bool              `json:"enabled" yaml:"enabled"`
	NodePrefix            string            `json:"node_prefix,omitempty" yaml:"node_prefix,omitempty"`
	FilterRegex           []string          `json:"filter_regex,omitempty" yaml:"filter_regex,omitempty"`
	FilterMinSpeedMbps    *float64          `json:"filter_min_speed_mbps,omitempty" yaml:"filter_min_speed_mbps,omitempty"`
	FilterMediaUnlock     []string          `json:"filter_media_unlock,omitempty" yaml:"filter_media_unlock,omitempty"`
	IncludeNodeNames      []string          `json:"include_node_names,omitempty" yaml:"include_node_names,omitempty"`
	ExcludeNodeNames      []string          `json:"exclude_node_names,omitempty" yaml:"exclude_node_names,omitempty"`
	NodeRenames           map[string]string `json:"node_renames,omitempty" yaml:"node_renames,omitempty"`
	SourceNodes           []*Node           `json:"source_nodes,omitempty" yaml:"source_nodes,omitempty"`
	ManualNodes           []*Node           `json:"manual_nodes,omitempty" yaml:"manual_nodes,omitempty"`
	RawNodes              []*Node           `json:"raw_nodes,omitempty" yaml:"raw_nodes,omitempty"`
	LastFetchedAt         *time.Time        `json:"last_fetched_at,omitempty" yaml:"last_fetched_at,omitempty"`
	LastFetchError        string            `json:"last_fetch_error,omitempty" yaml:"last_fetch_error,omitempty"`
	FetchFailedCount      int               `json:"fetch_failed_count" yaml:"fetch_failed_count"`
	FetchComments         []string          `json:"fetch_comments,omitempty" yaml:"fetch_comments,omitempty"`
	SubscriptionUserinfo  string            `json:"subscription_userinfo,omitempty" yaml:"subscription_userinfo,omitempty"`
	ProfileUpdateInterval string            `json:"profile_update_interval,omitempty" yaml:"profile_update_interval,omitempty"`
	ProfileWebPageURL     string            `json:"profile_web_page_url,omitempty" yaml:"profile_web_page_url,omitempty"`
	ProxyChain            string            `json:"proxy_chain,omitempty" yaml:"proxy_chain,omitempty"`
	NodeProxyChains       map[string]string `json:"node_proxy_chains,omitempty" yaml:"node_proxy_chains,omitempty"`
}

// Validate ensures all subscription attributes conform to business domain rules.
func (s *Subscription) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return ErrInvalidName
	}
	if strings.TrimSpace(s.URL) == "" {
		return ErrInvalidURL
	}
	if s.UpdateInterval < 0 {
		return ErrInvalidUpdateInterval
	}
	return nil
}

// MatchesFilter checks if a node name matches any of the filter regex patterns.
// If FilterRegex is empty, all nodes pass by default.
func (s *Subscription) MatchesFilter(nodeName string) bool {
	if len(s.FilterRegex) == 0 {
		return true
	}
	for _, pattern := range s.FilterRegex {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err == nil && re.MatchString(nodeName) {
			return true
		}
	}
	return false
}

// IsExcluded checks if a node name is explicitly excluded or matches exclude patterns.
func (s *Subscription) IsExcluded(nodeName string) bool {
	for _, excluded := range s.ExcludeNodeNames {
		trimmed := strings.TrimSpace(excluded)
		if trimmed == "" {
			continue
		}
		if strings.Contains(nodeName, trimmed) {
			return true
		}
		re, err := regexp.Compile(trimmed)
		if err == nil && re.MatchString(nodeName) {
			return true
		}
	}
	return false
}

// ApplyRename transforms the given node name according to configured rename mappings.
func (s *Subscription) ApplyRename(nodeName string) string {
	if s.NodeRenames == nil {
		return nodeName
	}
	if target, exists := s.NodeRenames[nodeName]; exists {
		return target
	}
	for pattern, replacement := range s.NodeRenames {
		re, err := regexp.Compile(pattern)
		if err == nil && re.MatchString(nodeName) {
			return re.ReplaceAllString(nodeName, replacement)
		}
	}
	return nodeName
}

// FormatNodeName applies prefix and renaming logic to produce the final node name.
func (s *Subscription) FormatNodeName(nodeName string) string {
	renamed := s.ApplyRename(nodeName)
	if s.NodePrefix != "" {
		return s.NodePrefix + renamed
	}
	return renamed
}
