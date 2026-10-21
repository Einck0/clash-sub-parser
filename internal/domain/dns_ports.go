package domain

import "context"

// DNSSettingsRepository defines atomic, optimistic-concurrency persistence for singleton DNS configuration.
type DNSSettingsRepository interface {
	Get(ctx context.Context) (*DNSSettings, error)
	Update(ctx context.Context, settings *DNSSettings, expectedRevision int64) error
}
