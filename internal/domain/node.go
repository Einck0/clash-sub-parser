package domain

import (
	"time"
)

// Node represents the current normalized state of a proxy node, including plaintext connection and credentials.
// All node references use the stable non-secret LogicalID, never internal row IDs.
type Node struct {
	LogicalID   string                    `json:"logical_id"`
	Protocol    Protocol                  `json:"protocol"`
	DisplayName string                    `json:"display_name"`
	Server      string                    `json:"server"`
	Port        int                       `json:"port"`
	Credentials InboundProtocolCredential `json:"credentials"`
	Active      bool                      `json:"active"`
	// ConnectionRevision advances only when connection settings change or a node is reactivated.
	ConnectionRevision int64     `json:"connection_revision,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// NodeSource tracks the provenance association between a node and a subscription.
type NodeSource struct {
	NodeLogicalID   string `json:"node_logical_id"`
	SubscriptionID  string `json:"subscription_id"`
	LastSeenFetchID string `json:"last_seen_fetch_id"`
}

// NodeReadModel combines a normalized Node with its API-safe IPRiskSummary.
type NodeReadModel struct {
	Node          Node           `json:"node"`
	IPRiskSummary *IPRiskSummary `json:"ip_risk_summary,omitempty"`
}
