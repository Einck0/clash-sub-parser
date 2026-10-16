package domain

import (
	"fmt"
	"time"
)

// SubscriptionLinkState represents the lifecycle state of a live subscription link.
type SubscriptionLinkState string

const (
	SubscriptionLinkStateActive  SubscriptionLinkState = "active"
	SubscriptionLinkStateRevoked SubscriptionLinkState = "revoked"
)

// IsValid validates whether the state is recognized.
func (s SubscriptionLinkState) IsValid() bool {
	return s == SubscriptionLinkStateActive || s == SubscriptionLinkStateRevoked
}

// SubscriptionLink represents a stable, live subscription endpoint binding.
// It maps a fixed compiler target and high-entropy secret token to real-time configuration generation.
type SubscriptionLink struct {
	ID        string                `json:"id"`
	Label     string                `json:"label"`
	Target    CompilerTarget        `json:"target"`
	Token     string                `json:"token,omitempty"`
	TokenHash string                `json:"-"`
	State     SubscriptionLinkState `json:"state"`
	IsDefault bool                  `json:"is_default"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	RevokedAt *time.Time            `json:"revoked_at,omitempty"`
}

// IsActive returns whether the link is active and serving requests.
func (l *SubscriptionLink) IsActive() bool {
	return l.State == SubscriptionLinkStateActive
}

// Revoke transitions the link into revoked state.
func (l *SubscriptionLink) Revoke(now time.Time) error {
	if l.State == SubscriptionLinkStateRevoked {
		return NewConflictError("already_revoked", fmt.Sprintf("subscription link %s is already revoked", l.ID))
	}
	utcNow := now.UTC()
	l.State = SubscriptionLinkStateRevoked
	l.RevokedAt = &utcNow
	l.UpdatedAt = utcNow
	return nil
}

// RotateToken replaces the link's credentials with new token values.
func (l *SubscriptionLink) RotateToken(newToken, newTokenHash string, now time.Time) error {
	if !l.IsActive() {
		return NewConflictError("not_active", fmt.Sprintf("cannot rotate token on inactive subscription link %s", l.ID))
	}
	l.Token = newToken
	l.TokenHash = newTokenHash
	l.UpdatedAt = now.UTC()
	return nil
}

// MaskedToken returns a desensitized representation of the token for listings.
func (l *SubscriptionLink) MaskedToken() string {
	if len(l.Token) <= 8 {
		return "******"
	}
	return l.Token[:4] + "..." + l.Token[len(l.Token)-4:]
}
