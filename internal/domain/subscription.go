package domain

import (
	"fmt"
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

// ProbePolicyMode defines the periodic probe scheduling mode for a subscription.
type ProbePolicyMode string

const (
	ProbePolicyModeInherit  ProbePolicyMode = "inherit"
	ProbePolicyModeDisabled ProbePolicyMode = "disabled"
	ProbePolicyModeCustom   ProbePolicyMode = "custom"
)

func (m ProbePolicyMode) IsValid() bool {
	return m == ProbePolicyModeInherit || m == ProbePolicyModeDisabled || m == ProbePolicyModeCustom
}

// SubscriptionProbePolicy defines per-subscription periodic probe settings.
type SubscriptionProbePolicy struct {
	Mode            ProbePolicyMode `json:"mode"`
	IntervalSeconds *int            `json:"interval_seconds,omitempty"`
}

// Validate validates that probe policy mode and interval adhere to system bounds.
// Custom requires interval_seconds between 60 and 604800.
func (p SubscriptionProbePolicy) Validate() error {
	switch p.Mode {
	case ProbePolicyModeInherit, ProbePolicyModeDisabled:
		return nil
	case ProbePolicyModeCustom:
		if p.IntervalSeconds == nil {
			return NewValidationError("invalid_probe_policy_interval", "interval_seconds is required when probe policy mode is custom")
		}
		if *p.IntervalSeconds < 60 || *p.IntervalSeconds > 604800 {
			return NewValidationError("invalid_probe_policy_interval",
				fmt.Sprintf("interval_seconds must be between 60 and 604800, got %d", *p.IntervalSeconds))
		}
		return nil
	default:
		return NewValidationError("invalid_probe_policy_mode",
			fmt.Sprintf("invalid probe policy mode %q: must be inherit, disabled, or custom", p.Mode))
	}
}

// SubscriptionProbePolicyPatch defines partial updates to a subscription's probe policy.
type SubscriptionProbePolicyPatch struct {
	Mode            *ProbePolicyMode `json:"mode,omitempty"`
	IntervalSeconds *int             `json:"interval_seconds,omitempty"`
}

// SubscriptionConfig holds advanced subscription parameters.
type SubscriptionConfig struct {
	CronSchedule string                   `json:"cron_schedule,omitempty"`
	AutoTest     bool                     `json:"auto_test"`
	RenameRules  []RenameRule             `json:"rename_rules,omitempty"`
	FilterRules  []FilterRule             `json:"filter_rules,omitempty"`
	TargetGroups []string                 `json:"target_groups,omitempty"`
	ProbePolicy  *SubscriptionProbePolicy `json:"probe_policy,omitempty"`
}

// EffectiveProbePolicy returns the effective probe policy, defaulting to inherit if unconfigured.
func (c SubscriptionConfig) EffectiveProbePolicy() SubscriptionProbePolicy {
	if c.ProbePolicy == nil || c.ProbePolicy.Mode == "" {
		return SubscriptionProbePolicy{
			Mode: ProbePolicyModeInherit,
		}
	}
	res := *c.ProbePolicy
	if res.Mode != ProbePolicyModeCustom {
		res.IntervalSeconds = nil
	}
	return res
}

// SubscriptionConfigPatch defines partial updates to subscription configuration.
// Pointer fields allow distinguishing between omitted (nil) and explicit empty values.
type SubscriptionConfigPatch struct {
	CronSchedule *string                       `json:"cron_schedule,omitempty"`
	AutoTest     *bool                         `json:"auto_test,omitempty"`
	RenameRules  *[]RenameRule                 `json:"rename_rules,omitempty"`
	FilterRules  *[]FilterRule                 `json:"filter_rules,omitempty"`
	TargetGroups *[]string                     `json:"target_groups,omitempty"`
	ProbePolicy  *SubscriptionProbePolicyPatch `json:"probe_policy,omitempty"`
}

// ApplyPatch merges partial patch values into the subscription configuration.
func (c *SubscriptionConfig) ApplyPatch(patch *SubscriptionConfigPatch) error {
	if patch == nil {
		return nil
	}
	if patch.CronSchedule != nil {
		c.CronSchedule = *patch.CronSchedule
	}
	if patch.AutoTest != nil {
		c.AutoTest = *patch.AutoTest
	}
	if patch.RenameRules != nil {
		c.RenameRules = *patch.RenameRules
	}
	if patch.FilterRules != nil {
		c.FilterRules = *patch.FilterRules
	}
	if patch.TargetGroups != nil {
		c.TargetGroups = *patch.TargetGroups
	}
	if patch.ProbePolicy != nil {
		current := c.EffectiveProbePolicy()
		targetMode := current.Mode
		if patch.ProbePolicy.Mode != nil {
			targetMode = *patch.ProbePolicy.Mode
		}
		var targetInterval *int
		if patch.ProbePolicy.IntervalSeconds != nil {
			targetInterval = patch.ProbePolicy.IntervalSeconds
		} else if patch.ProbePolicy.Mode == nil {
			targetInterval = current.IntervalSeconds
		}
		if targetMode != ProbePolicyModeCustom {
			targetInterval = nil
		}
		newPolicy := SubscriptionProbePolicy{
			Mode:            targetMode,
			IntervalSeconds: targetInterval,
		}
		if err := newPolicy.Validate(); err != nil {
			return err
		}
		c.ProbePolicy = &newPolicy
	}
	return nil
}

// Subscription represents a mutable subscription source configuration.
type Subscription struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	SourceURLSecretRef string             `json:"source_url_secret_ref"`
	Enabled            bool               `json:"enabled"`
	IsPrimary          bool               `json:"is_primary"`
	UserInfo           string             `json:"userinfo,omitempty"`
	Prefix             string             `json:"prefix,omitempty"`
	RefreshPolicy      RefreshPolicy      `json:"refresh_policy"`
	Config             SubscriptionConfig `json:"config"`
	Revision           string             `json:"revision"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	LastRefreshedAt    *time.Time         `json:"last_refreshed_at,omitempty"`
	LastRefreshOutcome *FetchOutcome      `json:"last_refresh_outcome,omitempty"`
	NodeCount          int                `json:"node_count"`
	SourceNodeCount    int                `json:"source_node_count"`
	CountsScope        string             `json:"counts_scope,omitempty"`
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
