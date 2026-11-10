package domain

import (
	"context"
	"time"
)

// PayloadEvidence contains only immutable whitelist provenance metadata.
// No raw URL, headers, body or parsed connection configuration may be stored.
type PayloadEvidence struct {
	PayloadID         string
	SubscriptionID    string
	SourceIdentity    string
	FetchID           string
	ContentDigest     string
	OriginalCreatedAt string
	CapturedAt        string
	HTTPStatus        int
	EvidenceKind      string
	Quality           string
	Pinned            bool
}

// PublicationEvidenceWriter persists draft bytes and their frozen provenance in
// one transaction, rather than reporting success after a best-effort ref write.
type PublicationEvidenceWriter interface {
	CreateWithEvidence(context.Context, *Publication, []PayloadEvidence) error
}

// SubscriptionStoragePlan is a read-only inventory, not permission to migrate.
// Blockers must be resolved before a separate, sealed offline apply can exist.
// It deliberately contains no upstream URLs, raw names or configuration bodies.
type SubscriptionStoragePlan struct {
	Version          int                         `json:"version"`
	SchemaVersion    int                         `json:"schema_version"`
	SnapshotDigest   string                      `json:"snapshot_digest"`
	Counts           map[string]int64            `json:"counts"`
	ProtectedDigests map[string]string           `json:"protected_digests"`
	Subscriptions    []SubscriptionStorageStatus `json:"subscriptions"`
	Blockers         []string                    `json:"blockers"`
	ApplySupported   bool                        `json:"apply_supported"`
	VerifySupported  bool                        `json:"verify_supported"`
}

// SubscriptionStorageVerification records a read-only check of a candidate
// database against a sealed plan. It never contains secrets or raw content.
type SubscriptionStorageVerification struct {
	Version               int              `json:"version"`
	Verified              bool             `json:"verified"`
	BaselineDigest        string           `json:"baseline_digest"`
	CandidateDigest       string           `json:"candidate_digest"`
	SchemaVersion         int              `json:"schema_version"`
	Counts                map[string]int64 `json:"counts"`
	Failures              []string         `json:"failures"`
	ConsumerClosureSealed bool             `json:"consumer_closure_sealed"`
}

// SubscriptionStorageStatus keeps attempts, saved raw and applied candidates
// distinct. CandidateCount is not a survivor selection: legacy ambiguity needs
// reconciliation with source membership and the source revision before apply.
type SubscriptionStorageStatus struct {
	SubscriptionID string `json:"subscription_id"`
	PayloadCount   int64  `json:"payload_count"`
	EntryCount     int64  `json:"entry_count"`
	CandidateCount int64  `json:"candidate_count"`
	LastAttemptID  string `json:"last_attempt_fetch_id"`
	RawFetchID     string `json:"raw_fetch_id"`
}

// ConnectionVersionLease identifies an actual consumer of a complete stored
// configuration. Expiry is diagnostic only; it is never authority to reclaim a
// live consumer's configuration. Owner + fencing token guard release/reentry.
type ConnectionVersionLease struct {
	ConsumerKind string
	ConsumerID   string
	NodeID       string
	Revision     int64
	Owner        string
	FencingToken string
	HeartbeatAt  time.Time
	ExpiresAt    time.Time
}

// LeasedConnectionVersion is an immutable in-memory copy acquired atomically
// with its lease. Configuration must never be serialized into maintenance logs.
type LeasedConnectionVersion struct {
	NodeID              string `json:"-"`
	Revision            int64  `json:"-"`
	EffectiveConfigJSON string `json:"-"`
}
