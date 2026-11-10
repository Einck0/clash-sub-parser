package domain

import (
	"context"
	"time"
)

// SubscriptionOriginalConfig represents the latest usable raw original configuration
// response body received from an upstream subscription endpoint before transformations.
type SubscriptionOriginalConfig struct {
	SubscriptionID string    `json:"subscription_id"`
	SourceRevision string    `json:"-"`
	ContentDigest  string    `json:"content_digest"`
	Body           []byte    `json:"-"`
	FetchedAt      time.Time `json:"fetched_at"`
	ContentType    string    `json:"-"`
	FetchID        string    `json:"fetch_id"`
}

// SubscriptionOriginalConfigRepository defines operations for raw original subscription configuration persistence.
type SubscriptionOriginalConfigRepository interface {
	GetBySubscriptionID(ctx context.Context, subscriptionID string) (*SubscriptionOriginalConfig, error)
	DeleteBySubscriptionID(ctx context.Context, subscriptionID string) error
}
