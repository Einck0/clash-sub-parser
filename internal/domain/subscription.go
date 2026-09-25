package domain

import (
	"time"
)

// RefreshPolicy defines the configuration for subscription fetching.
type RefreshPolicy struct {
	IntervalSeconds  int    `json:"interval_seconds"`
	UserAgentPolicy  string `json:"user_agent_policy"`
	FetchProxyRef    string `json:"fetch_proxy_ref,omitempty"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	MaxResponseBytes int64  `json:"max_response_bytes"`
}

// RenameRule defines a pattern-based node renaming rule.
type RenameRule struct {
	Pattern string `json:"pattern"`
	Replace string `json:"replace"`
}

// FilterRule defines an inclusion or exclusion filter rule.
type FilterRule struct {
	Type    string `json:"type"` // "include" | "exclude"
	Pattern string `json:"pattern"`
}

// SubscriptionConfig holds advanced subscription parameters.
type SubscriptionConfig struct {
	CronSchedule string       `json:"cron_schedule,omitempty"`
	AutoTest     bool         `json:"auto_test"`
	RenameRules  []RenameRule `json:"rename_rules,omitempty"`
	FilterRules  []FilterRule `json:"filter_rules,omitempty"`
	TargetGroups []string     `json:"target_groups,omitempty"`
}

// Subscription represents a mutable subscription source configuration.
type Subscription struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	SourceURLSecretRef string             `json:"source_url_secret_ref"`
	Enabled            bool               `json:"enabled"`
	RefreshPolicy      RefreshPolicy      `json:"refresh_policy"`
	Config             SubscriptionConfig `json:"config"`
	Revision           string             `json:"revision"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// SubscriptionFetch represents an immutable audit record of a fetch operation.
type SubscriptionFetch struct {
	ID             string       `json:"id"`
	SubscriptionID string       `json:"subscription_id"`
	StartedAt      time.Time    `json:"started_at"`
	FinishedAt     *time.Time   `json:"finished_at,omitempty"`
	Outcome        FetchOutcome `json:"outcome"`
	ContentDigest  string       `json:"content_digest"`
	RedactedError  string       `json:"redacted_error,omitempty"`
	NodesParsed    int          `json:"nodes_parsed"`
	NodesValid     int          `json:"nodes_valid"`
}
