package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ProxyChainTargetType defines the target scope of a chain binding.
type ProxyChainTargetType string

const (
	ProxyChainTargetSubscription ProxyChainTargetType = "subscription"
	ProxyChainTargetNodeGroup    ProxyChainTargetType = "node_group"
	ProxyChainTargetNode         ProxyChainTargetType = "node"
)

// IsValid checks whether the target type is a recognized target scope.
func (t ProxyChainTargetType) IsValid() bool {
	return t == ProxyChainTargetSubscription || t == ProxyChainTargetNodeGroup || t == ProxyChainTargetNode
}

// ProxyChainDialerType defines the upstream dialer entity type.
type ProxyChainDialerType string

const (
	ProxyChainDialerNode      ProxyChainDialerType = "node"
	ProxyChainDialerNodeGroup ProxyChainDialerType = "node_group"
)

// IsValid checks whether the dialer type is a recognized upstream dialer.
func (d ProxyChainDialerType) IsValid() bool {
	return d == ProxyChainDialerNode || d == ProxyChainDialerNodeGroup
}

// ProxyChainBinding binds an egress target entity to an upstream dialer entity.
type ProxyChainBinding struct {
	ID         string               `json:"id"`
	TargetType ProxyChainTargetType `json:"target_type"`
	TargetID   string               `json:"target_id"`
	DialerType ProxyChainDialerType `json:"dialer_type"`
	DialerID   string               `json:"dialer_id"`
	Enabled    bool                 `json:"enabled"`
	SortOrder  int                  `json:"sort_order"`
	Note       string               `json:"note"`
	Revision   int64                `json:"revision"`
}

// Validate checks internal field validity and direct self-reference.
func (b ProxyChainBinding) Validate() error {
	if !b.TargetType.IsValid() {
		return NewValidationError("invalid_chain_binding", fmt.Sprintf("invalid chain target type: %s", b.TargetType))
	}
	if strings.TrimSpace(b.TargetID) == "" {
		return NewValidationError("invalid_chain_binding", "chain target id cannot be empty")
	}
	if !b.DialerType.IsValid() {
		return NewValidationError("invalid_chain_binding", fmt.Sprintf("invalid chain dialer type: %s", b.DialerType))
	}
	if strings.TrimSpace(b.DialerID) == "" {
		return NewValidationError("invalid_chain_binding", "chain dialer id cannot be empty")
	}
	if b.Revision < 0 {
		return NewValidationError("invalid_chain_binding", "chain revision must be non-negative")
	}
	if b.TargetType == ProxyChainTargetNode && b.DialerType == ProxyChainDialerNode && b.TargetID == b.DialerID {
		return NewValidationError("chain_cycle", fmt.Sprintf("node %s cannot chain to itself", b.TargetID))
	}
	if b.TargetType == ProxyChainTargetNodeGroup && b.DialerType == ProxyChainDialerNodeGroup && b.TargetID == b.DialerID {
		return NewValidationError("chain_cycle", fmt.Sprintf("group %s cannot chain to itself", b.TargetID))
	}
	return nil
}

// ProxyChainConfig captures the full persisted binding collection and its stable global version.
type ProxyChainConfig struct {
	Bindings []ProxyChainBinding `json:"bindings"`
	Version  string              `json:"version"`
}

// ProxyChainRepository defines atomic persistence operations for proxy chain bindings.
type ProxyChainRepository interface {
	Get(ctx context.Context) (*ProxyChainConfig, error)
	Replace(ctx context.Context, expectedVersion string, bindings []ProxyChainBinding) (*ProxyChainConfig, error)
}

// ProxyChainConfigVersion computes a stable SHA-256 version of ID-sorted canonical binding data.
// An empty slice yields a deterministic non-empty version.
func ProxyChainConfigVersion(bindings []ProxyChainBinding) string {
	if len(bindings) == 0 {
		h := sha256.Sum256([]byte("[]"))
		return hex.EncodeToString(h[:])
	}

	canonical := make([]ProxyChainBinding, len(bindings))
	copy(canonical, bindings)
	sort.SliceStable(canonical, func(i, j int) bool {
		return canonical[i].ID < canonical[j].ID
	})

	data, err := json.Marshal(canonical)
	if err != nil {
		h := sha256.Sum256([]byte("[]"))
		return hex.EncodeToString(h[:])
	}

	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
