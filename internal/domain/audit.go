package domain

import (
	"time"
)

// AuditEvent represents an append-only audit trail entry for operational actions.
type AuditEvent struct {
	ID              string      `json:"id"`
	ActorKind       ActorKind   `json:"actor_kind"`
	RequestID       string      `json:"request_id"`
	Action          string      `json:"action"`
	Result          AuditResult `json:"result"`
	RedactedSummary string      `json:"redacted_summary"`
	CreatedAt       time.Time   `json:"created_at"`
}
