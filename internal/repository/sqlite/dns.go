package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
)

type dnsSettingsRepository struct {
	db *sql.DB
}

// NewDNSSettingsRepository creates a SQLite repository for singleton DNS configuration.
func NewDNSSettingsRepository(db *sql.DB) domain.DNSSettingsRepository {
	return &dnsSettingsRepository{db: db}
}

func (r *dnsSettingsRepository) Get(ctx context.Context) (*domain.DNSSettings, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT enabled, raw_yaml, revision, updated_at
		FROM dns_config
		WHERE id = 1
	`)

	var enabledInt int
	var raw string
	var rev int64
	var updated time.Time

	err := row.Scan(&enabledInt, &raw, &rev, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		def := domain.DefaultDNSSettings()
		return &def, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query dns_config: %w", err)
	}

	return &domain.DNSSettings{
		Enabled:   enabledInt == 1,
		RawYAML:   raw,
		Revision:  rev,
		UpdatedAt: updated,
		Source:    "saved",
	}, nil
}

func (r *dnsSettingsRepository) Update(ctx context.Context, settings *domain.DNSSettings, expectedRevision int64) error {
	if settings == nil {
		return domain.NewValidationError("invalid_dns_settings", "DNS settings must not be nil")
	}

	normalized, err := domain.NormalizeDNSSettings(settings.Enabled, settings.RawYAML)
	if err != nil {
		return err
	}

	var enabledInt int
	if settings.Enabled {
		enabledInt = 1
	}
	now := domain.NowUTC()

	if expectedRevision == 0 {
		res, insertErr := r.db.ExecContext(ctx, `
			INSERT INTO dns_config (id, enabled, raw_yaml, revision, updated_at)
			VALUES (1, ?, ?, 1, ?)
			ON CONFLICT(id) DO NOTHING
		`, enabledInt, normalized, now)
		if insertErr != nil {
			return fmt.Errorf("failed to insert initial dns_config: %w", insertErr)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to check inserted rows: %w", err)
		}
		if affected == 1 {
			settings.RawYAML = normalized
			settings.Revision = 1
			settings.UpdatedAt = now
			settings.Source = "saved"
			return nil
		}
	}

	newRevision := expectedRevision + 1
	res, updateErr := r.db.ExecContext(ctx, `
		UPDATE dns_config
		SET enabled = ?, raw_yaml = ?, revision = ?, updated_at = ?
		WHERE id = 1 AND revision = ?
	`, enabledInt, normalized, newRevision, now, expectedRevision)
	if updateErr != nil {
		return fmt.Errorf("failed to update dns_config: %w", updateErr)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check updated rows: %w", err)
	}
	if affected == 0 {
		var currentRevision int64
		queryErr := r.db.QueryRowContext(ctx, `SELECT revision FROM dns_config WHERE id = 1`).Scan(&currentRevision)
		if errors.Is(queryErr, sql.ErrNoRows) {
			currentRevision = 0
		} else if queryErr != nil {
			return fmt.Errorf("failed to read current dns_config revision: %w", queryErr)
		}
		conflict := domain.NewConflictError("dns_revision_conflict", "DNS settings revision mismatch")
		conflict.CurrentRevision = &currentRevision
		return conflict
	}

	settings.RawYAML = normalized
	settings.Revision = newRevision
	settings.UpdatedAt = now
	settings.Source = "saved"
	return nil
}
