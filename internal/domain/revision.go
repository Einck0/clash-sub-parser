package domain

import (
	"time"
)

// ConfigurationRevision represents an immutable snapshot of system configuration.
type ConfigurationRevision struct {
	ID            string                     `json:"id"`
	ParentID      *string                    `json:"parent_id,omitempty"`
	ContentDigest string                     `json:"content_digest"`
	State         ConfigurationRevisionState `json:"state"`
	CreatedAt     time.Time                  `json:"created_at"`
}
