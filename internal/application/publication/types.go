// Package publication provides domain use cases for immutable configuration publications,
// target-bound export token generation, active revocation, and deterministic client delivery.
package publication

import (
	"errors"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

var (
	// ErrNotFound indicates the requested publication was not found.
	ErrNotFound = errors.New("publication not found")

	// ErrRevoked indicates the publication has been explicitly revoked and must not be served.
	ErrRevoked = errors.New("publication has been revoked")

	// ErrUnauthorized indicates the provided publication export token is missing or invalid.
	ErrUnauthorized = errors.New("invalid or missing publication token")

	// ErrUnsupportedTarget indicates the requested compiler target is unknown or invalid.
	ErrUnsupportedTarget = errors.New("unsupported compiler target")
)

// PreflightDiagnostic describes a safe, actionable publication admission finding.
type PreflightDiagnostic struct {
	Severity resolver.DiagnosticSeverity `json:"severity"`
	Code     string                      `json:"code"`
	Message  string                      `json:"message"`
	Target   string                      `json:"target,omitempty"`
}

// PreflightResult contains the diagnostics evaluated before publication creation.
type PreflightResult struct {
	Allowed        bool                  `json:"allowed"`
	Diagnostics    []PreflightDiagnostic `json:"diagnostics,omitempty"`
	PolicyRevision string                `json:"policy_revision,omitempty"`
	SnapshotDigest string                `json:"snapshot_digest,omitempty"`
}

// PreflightCommand contains parameters for evaluating preflight diagnostics without publishing.
type PreflightCommand struct {
	Target     domain.CompilerTarget            `json:"target"`
	RevisionID string                           `json:"revision_id,omitempty"`
	Snapshot   *resolver.ResolvedPolicySnapshot `json:"-"`
	ActorKind  domain.ActorKind                 `json:"-"`
	RequestID  string                           `json:"-"`
}

// PreflightError indicates that a resolved snapshot cannot be published safely.
type PreflightError struct {
	Result PreflightResult
}

func (e *PreflightError) Error() string {
	return "publication preflight rejected"
}

func (e *PreflightError) Unwrap() error {
	return domain.NewConflictError("publication_preflight_rejected", "publication preflight rejected")
}

// PublishCommand contains the input parameters required to publish a new configuration bundle.
type PublishCommand struct {
	Target     domain.CompilerTarget
	RevisionID string
	Snapshot   *resolver.ResolvedPolicySnapshot
	ActorKind  domain.ActorKind
	RequestID  string
}

// PublishResult encapsulates the published immutable publication entity and its export token.
type PublishResult struct {
	Publication    domain.Publication `json:"publication"`
	RawToken       string             `json:"raw_token"`
	ExportURL      string             `json:"export_url"`
	ContentDigest  string             `json:"content_digest"`
	SnapshotDigest string             `json:"snapshot_digest"`
	ContentType    string             `json:"content_type"`
	Filename       string             `json:"filename"`
	Size           int                `json:"size"`
}

// PreviewQuery defines parameters for previewing compiled output from a resolved snapshot.
type PreviewQuery struct {
	Target     domain.CompilerTarget            `json:"target"`
	RevisionID string                           `json:"revision_id,omitempty"`
	Snapshot   *resolver.ResolvedPolicySnapshot `json:"-"`
}

// PreviewResult contains rendered configuration content, digests, and diagnostics for preview.
type PreviewResult struct {
	Target         domain.CompilerTarget       `json:"target"`
	SnapshotDigest string                      `json:"snapshot_digest"`
	ContentDigest  string                      `json:"content_digest"`
	Content        []byte                      `json:"content"`
	ContentType    string                      `json:"content_type"`
	Filename       string                      `json:"filename"`
	Diagnostics    []resolver.Diagnostic       `json:"diagnostics"`
	FilterCounts   *resolver.FilterLayerCounts `json:"filter_counts,omitempty"`
}

// RevokeCommand specifies a publication to revoke along with audit context.
type RevokeCommand struct {
	ID        string
	ActorKind domain.ActorKind
	RequestID string
}

// Artifact holds the compiled bytes and metadata for an immutable publication.
type Artifact struct {
	PublicationID  string                `json:"publication_id"`
	Target         domain.CompilerTarget `json:"target"`
	Content        []byte                `json:"-"`
	ContentType    string                `json:"content_type"`
	Filename       string                `json:"filename"`
	ContentDigest  string                `json:"content_digest"`
	SnapshotDigest string                `json:"snapshot_digest"`
}

// PublicationDetail represents detailed publication state returned to management endpoints.
type PublicationDetail struct {
	Publication    domain.Publication `json:"publication"`
	ExportURL      string             `json:"export_url"`
	ContentDigest  string             `json:"content_digest,omitempty"`
	SnapshotDigest string             `json:"snapshot_digest,omitempty"`
	ContentType    string             `json:"content_type,omitempty"`
	Filename       string             `json:"filename,omitempty"`
}
