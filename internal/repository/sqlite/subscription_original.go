package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
)

type subscriptionOriginalRepository struct {
	db *sql.DB
}

// NewSubscriptionOriginalRepository constructs a SQLite implementation of domain.SubscriptionOriginalConfigRepository.
func NewSubscriptionOriginalRepository(db *sql.DB) domain.SubscriptionOriginalConfigRepository {
	return &subscriptionOriginalRepository{db: db}
}

// GetOriginalConfigBySubscriptionID performs a metadata-only direct query checking subscription
// existence and retrieving the raw original configuration without loading unnecessary entities.
func GetOriginalConfigBySubscriptionID(ctx context.Context, db *sql.DB, subscriptionID string) (*domain.SubscriptionOriginalConfig, error) {
	if db == nil {
		return nil, domain.NewInternalError("database_unavailable", "database connection not available")
	}

	const query = `
	SELECT
		s.id,
		c.subscription_id,
		c.source_revision,
		c.content_digest,
		c.body_blob,
		c.fetched_at,
		c.content_type,
		c.fetch_id
	FROM subscriptions s
	LEFT JOIN subscription_original_configs c ON s.id = c.subscription_id
	WHERE s.id = ?;`

	var subID string
	var origSubID, sourceRev, contentDigest, contentType, fetchID sql.NullString
	var bodyBlob []byte
	var fetchedAtStr sql.NullString

	err := db.QueryRowContext(ctx, query, subscriptionID).Scan(
		&subID,
		&origSubID,
		&sourceRev,
		&contentDigest,
		&bodyBlob,
		&fetchedAtStr,
		&contentType,
		&fetchID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("subscription_not_found", "subscription not found")
		}
		return nil, fmt.Errorf("failed to query subscription original config: %w", err)
	}

	if !origSubID.Valid || len(bodyBlob) == 0 {
		return nil, domain.NewNotFoundError("original_config_not_found", "subscription original configuration not found")
	}

	var fetchedAt time.Time
	if fetchedAtStr.Valid && fetchedAtStr.String != "" {
		var pErr error
		fetchedAt, pErr = time.Parse(time.RFC3339Nano, fetchedAtStr.String)
		if pErr != nil {
			fetchedAt, _ = time.Parse(time.RFC3339, fetchedAtStr.String)
		}
	}

	return &domain.SubscriptionOriginalConfig{
		SubscriptionID: origSubID.String,
		ContentDigest:  contentDigest.String,
		Body:           bodyBlob,
		FetchedAt:      fetchedAt,
		ContentType:    contentType.String,
		FetchID:        fetchID.String,
	}, nil
}

func (r *subscriptionOriginalRepository) GetBySubscriptionID(ctx context.Context, subscriptionID string) (*domain.SubscriptionOriginalConfig, error) {
	return GetOriginalConfigBySubscriptionID(ctx, r.db, subscriptionID)
}

func (r *subscriptionOriginalRepository) DeleteBySubscriptionID(ctx context.Context, subscriptionID string) error {
	if r.db == nil {
		return domain.NewInternalError("database_unavailable", "database connection not available")
	}

	_, err := r.db.ExecContext(ctx, "DELETE FROM subscription_original_configs WHERE subscription_id = ?;", subscriptionID)
	if err != nil {
		return fmt.Errorf("failed to delete subscription original config: %w", err)
	}
	return nil
}

// UpsertOriginalConfigTx inserts or replaces the raw original config within an active transaction.
func UpsertOriginalConfigTx(ctx context.Context, tx *sql.Tx, cfg *domain.SubscriptionOriginalConfig, sourceRevision string) error {
	if tx == nil {
		return errors.New("transaction cannot be nil")
	}
	if cfg == nil {
		return errors.New("subscription original config cannot be nil")
	}

	const upsertSQL = `
	INSERT INTO subscription_original_configs (
		subscription_id, source_revision, content_digest, body_blob, fetched_at, content_type, fetch_id
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(subscription_id) DO UPDATE SET
		source_revision = excluded.source_revision,
		content_digest = excluded.content_digest,
		body_blob = excluded.body_blob,
		fetched_at = excluded.fetched_at,
		content_type = excluded.content_type,
		fetch_id = excluded.fetch_id;`

	fetchedAtStr := cfg.FetchedAt.UTC().Format(time.RFC3339Nano)
	if cfg.FetchedAt.IsZero() {
		fetchedAtStr = time.Now().UTC().Format(time.RFC3339Nano)
	}

	_, err := tx.ExecContext(ctx, upsertSQL,
		cfg.SubscriptionID,
		sourceRevision,
		cfg.ContentDigest,
		cfg.Body,
		fetchedAtStr,
		cfg.ContentType,
		cfg.FetchID,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert subscription original config: %w", err)
	}
	return nil
}
