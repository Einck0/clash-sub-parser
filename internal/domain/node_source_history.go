package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RelationState represents the verification quality of a node-to-source provenance link.
type RelationState string

const (
	RelationStateVerified RelationState = "verified"
	RelationStateConflict RelationState = "conflict"
	RelationStateUnknown  RelationState = "unknown"
)

var validRelationStates = map[RelationState]bool{
	RelationStateVerified: true,
	RelationStateConflict: true,
	RelationStateUnknown:  true,
}

func (s RelationState) IsValid() bool {
	return validRelationStates[s]
}

func ParseRelationState(s string) (RelationState, error) {
	st := RelationState(strings.ToLower(strings.TrimSpace(s)))
	if !st.IsValid() {
		return "", NewValidationError("invalid_relation_state", fmt.Sprintf("unsupported relation state: %s", s))
	}
	return st, nil
}

// AttributionCause defines the operational cause that generated a node source history record.
type AttributionCause string

const (
	CauseLegacyImport        AttributionCause = "legacy_import"
	CauseRefreshRemoved      AttributionCause = "refresh_removed"
	CauseSubscriptionDeleted AttributionCause = "subscription_deleted"
	CauseManualConfirmed     AttributionCause = "manual_confirmed"
	CauseUnresolved          AttributionCause = "unresolved"
)

var validAttributionCauses = map[AttributionCause]bool{
	CauseLegacyImport:        true,
	CauseRefreshRemoved:      true,
	CauseSubscriptionDeleted: true,
	CauseManualConfirmed:     true,
	CauseUnresolved:          true,
}

func (c AttributionCause) IsValid() bool {
	return validAttributionCauses[c]
}

func ParseAttributionCause(s string) (AttributionCause, error) {
	ac := AttributionCause(strings.ToLower(strings.TrimSpace(s)))
	if !ac.IsValid() {
		return "", NewValidationError("invalid_attribution_cause", fmt.Sprintf("unsupported attribution cause: %s", s))
	}
	return ac, nil
}

// AttributionStatus is the aggregated provenance status for a node across live sources and history ledger.
type AttributionStatus string

const (
	AttributionStatusCurrent            AttributionStatus = "current"
	AttributionStatusHistoricalVerified AttributionStatus = "historical_verified"
	AttributionStatusManualConfirmed    AttributionStatus = "manual_confirmed"
	AttributionStatusConflict           AttributionStatus = "conflict"
	AttributionStatusUnknown            AttributionStatus = "unknown"
)

var validAttributionStatuses = map[AttributionStatus]bool{
	AttributionStatusCurrent:            true,
	AttributionStatusHistoricalVerified: true,
	AttributionStatusManualConfirmed:    true,
	AttributionStatusConflict:           true,
	AttributionStatusUnknown:            true,
}

func (s AttributionStatus) IsValid() bool {
	return validAttributionStatuses[s]
}

// NodeSourceHistory models an immutable historical source observation record in the node_source_history ledger.
type NodeSourceHistory struct {
	ID                 string           `json:"id"`
	NodeLogicalID      string           `json:"node_logical_id"`
	SubscriptionID     *string          `json:"subscription_id,omitempty"`
	SourceIdentity     string           `json:"source_identity"`
	SourceLabel        string           `json:"source_label"`
	ConnectionRevision *int64           `json:"connection_revision,omitempty"`
	RelationState      RelationState    `json:"relation_state"`
	Cause              AttributionCause `json:"cause"`
	FirstObservedAt    *time.Time       `json:"first_observed_at,omitempty"`
	LastObservedAt     *time.Time       `json:"last_observed_at,omitempty"`
	EvidenceKind       string           `json:"evidence_kind"`
	EvidenceKey        string           `json:"evidence_key"`
	EvidenceJSON       string           `json:"evidence_json"`
	CreatedAt          time.Time        `json:"created_at"`
}

// Validate ensures structural validity and enforces the strict zero-secret invariant on evidence_json.
func (h *NodeSourceHistory) Validate() error {
	if strings.TrimSpace(h.NodeLogicalID) == "" {
		return NewValidationError("invalid_node_logical_id", "node_logical_id cannot be empty")
	}
	if strings.TrimSpace(h.SourceIdentity) == "" {
		return NewValidationError("invalid_source_identity", "source_identity cannot be empty")
	}
	if strings.TrimSpace(h.SourceLabel) == "" {
		return NewValidationError("invalid_source_label", "source_label cannot be empty")
	}
	if !h.RelationState.IsValid() {
		return NewValidationError("invalid_relation_state", fmt.Sprintf("unsupported relation state: %s", h.RelationState))
	}
	if !h.Cause.IsValid() {
		return NewValidationError("invalid_cause", fmt.Sprintf("unsupported attribution cause: %s", h.Cause))
	}
	if strings.TrimSpace(h.EvidenceKind) == "" {
		return NewValidationError("invalid_evidence_kind", "evidence_kind cannot be empty")
	}

	// Secret scrubbing invariant: evidence_json must never store raw secrets or private tokens.
	if raw := strings.TrimSpace(h.EvidenceJSON); raw != "" && raw != "{}" {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return NewValidationError("invalid_evidence_json", "evidence_json must be valid JSON: "+err.Error())
		}
		for key, val := range decoded {
			lowerK := strings.ToLower(key)
			if strings.Contains(lowerK, "password") || strings.Contains(lowerK, "secret") ||
				strings.Contains(lowerK, "token") || strings.Contains(lowerK, "private_key") ||
				strings.Contains(lowerK, "credential") {
				return NewValidationError("forbidden_evidence_secret", fmt.Sprintf("evidence_json key %q is forbidden (secrets prohibited)", key))
			}
			if strVal, ok := val.(string); ok {
				lowerVal := strings.ToLower(strVal)
				if strings.Contains(lowerVal, "password=") || strings.Contains(lowerVal, "token=") {
					return NewValidationError("forbidden_evidence_secret_val", fmt.Sprintf("evidence_json field %q contains sensitive credentials", key))
				}
			}
		}
	}
	return nil
}

// AggregateAttributionStatus determines the aggregated attribution status for a node according to domain invariant rules:
// 1. If currently attached to at least one active source -> current
// 2. Otherwise, if any verified historical source exists -> historical_verified (verified historical evidence cannot be overwritten by conflicts)
// 3. Otherwise, if positive manual audit confirmation exists -> manual_confirmed (manual only for positive creation audit, not lack-of-source)
// 4. Otherwise, if conflicting evidence exists -> conflict
// 5. Otherwise -> unknown
func AggregateAttributionStatus(hasCurrentSources bool, history []NodeSourceHistory) AttributionStatus {
	if hasCurrentSources {
		return AttributionStatusCurrent
	}
	if len(history) == 0 {
		return AttributionStatusUnknown
	}

	var hasVerified bool
	var hasManual bool
	var hasConflict bool

	for _, item := range history {
		if item.RelationState == RelationStateVerified {
			hasVerified = true
		}
		if item.Cause == CauseManualConfirmed {
			hasManual = true
		}
		if item.RelationState == RelationStateConflict {
			hasConflict = true
		}
	}

	if hasVerified {
		return AttributionStatusHistoricalVerified
	}
	if hasManual {
		return AttributionStatusManualConfirmed
	}
	if hasConflict {
		return AttributionStatusConflict
	}
	return AttributionStatusUnknown
}
