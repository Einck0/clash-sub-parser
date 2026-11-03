package proxychain

import (
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// OptionItem represents a safe ID-and-name candidate for target/dialer selection.
type OptionItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OptionsResult aggregates eligible nodes, policy groups, and subscriptions for selection.
type OptionsResult struct {
	Nodes         []OptionItem `json:"nodes"`
	Groups        []OptionItem `json:"groups"`
	Subscriptions []OptionItem `json:"subscriptions"`
}

// EffectNode represents an egress node with its resolved chain proxy and binding provenance.
type EffectNode struct {
	LogicalID      string `json:"logical_id"`
	DisplayName    string `json:"display_name"`
	DialerProxy    string `json:"dialer_proxy,omitempty"`
	ChainBindingID string `json:"chain_binding_id,omitempty"`
	ChainSource    string `json:"chain_source,omitempty"`
}

// EffectsResult conveys the resolved chain effects and node dialer assignments.
type EffectsResult struct {
	Version string                 `json:"version"`
	Effects []resolver.ChainEffect `json:"effects"`
	Nodes   []EffectNode           `json:"nodes"`
}

// PreviewCommand encapsulates candidate binding preview parameters.
type PreviewCommand struct {
	Binding       domain.ProxyChainBinding `json:"binding"`
	ConfigVersion string                   `json:"config_version"`
}

// CreateCommand contains parameters to create a new proxy chain binding.
type CreateCommand struct {
	Binding       domain.ProxyChainBinding `json:"binding"`
	ConfigVersion string                   `json:"config_version"`
	ActorKind     domain.ActorKind         `json:"-"`
	RequestID     string                   `json:"-"`
}

// CreateResult returns the newly created binding with assigned ID, revision, and new config version.
type CreateResult struct {
	Binding domain.ProxyChainBinding `json:"binding"`
	Version string                   `json:"version"`
}

// UpdateCommand contains parameters to update an existing proxy chain binding.
type UpdateCommand struct {
	Binding          domain.ProxyChainBinding `json:"binding"`
	ConfigVersion    string                   `json:"config_version"`
	ExpectedRevision int64                    `json:"expected_revision"`
	ActorKind        domain.ActorKind         `json:"-"`
	RequestID        string                   `json:"-"`
}

// UpdateResult returns the updated binding with incremented revision and new config version.
type UpdateResult struct {
	Binding domain.ProxyChainBinding `json:"binding"`
	Version string                   `json:"version"`
}

// DeleteCommand contains parameters to delete a proxy chain binding.
type DeleteCommand struct {
	ConfigVersion    string           `json:"config_version"`
	ExpectedRevision int64            `json:"expected_revision"`
	ActorKind        domain.ActorKind `json:"-"`
	RequestID        string           `json:"-"`
}

// DeleteResult returns the new config version after successful deletion.
type DeleteResult struct {
	Version string `json:"version"`
}
