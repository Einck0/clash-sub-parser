package domain

import (
	"fmt"
	"time"
)

// Publication represents a published configuration bundle export.
// It is immutable once compiled, but can be revoked by an administrator.
type Publication struct {
	ID              string           `json:"id"`
	Target          CompilerTarget   `json:"target"`
	SnapshotDigest  string           `json:"snapshot_digest"`
	CompilerVersion string           `json:"compiler_version"`
	TokenHash       string           `json:"token_hash"`
	State           PublicationState `json:"state"`
	CreatedAt       time.Time        `json:"created_at"`
	RevokedAt       *time.Time       `json:"revoked_at,omitempty"`
}

// IsActive returns whether the publication is active and serving requests.
func (p *Publication) IsActive() bool {
	return p.State == PublicationStateActive
}

// Revoke revokes the publication so that export endpoints will reject future access.
func (p *Publication) Revoke(now time.Time) error {
	if p.State == PublicationStateRevoked {
		return NewConflictError("already_revoked", fmt.Sprintf("publication %s is already revoked", p.ID))
	}
	utcNow := now.UTC()
	p.State = PublicationStateRevoked
	p.RevokedAt = &utcNow
	return nil
}
