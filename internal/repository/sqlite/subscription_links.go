package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

type subscriptionLinkRepository struct {
	db *sql.DB
}

// NewSubscriptionLinkRepository constructs a SQLite implementation of domain.SubscriptionLinkRepository.
func NewSubscriptionLinkRepository(db *sql.DB) domain.SubscriptionLinkRepository {
	return &subscriptionLinkRepository{db: db}
}

const selectSubscriptionLinkColumnsSQL = `
	SELECT
		id,
		label,
		target,
		token,
		token_hash,
		state,
		is_default,
		created_at,
		updated_at,
		revoked_at
	FROM subscription_links`

func scanSubscriptionLink(scanner interface{ Scan(dest ...any) error }, notFoundMsg string) (*domain.SubscriptionLink, error) {
	var link domain.SubscriptionLink
	var targetStr, stateStr, createdStr, updatedStr string
	var revokedStr sql.NullString
	var isDefaultInt int

	err := scanner.Scan(
		&link.ID,
		&link.Label,
		&targetStr,
		&link.Token,
		&link.TokenHash,
		&stateStr,
		&isDefaultInt,
		&createdStr,
		&updatedStr,
		&revokedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("subscription_link_not_found", notFoundMsg)
		}
		return nil, fmt.Errorf("failed to scan subscription link: %w", err)
	}

	link.Target = domain.CompilerTarget(targetStr)
	link.State = domain.SubscriptionLinkState(stateStr)
	link.IsDefault = isDefaultInt == 1
	link.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	if link.CreatedAt.IsZero() {
		link.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	}
	link.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	if link.UpdatedAt.IsZero() {
		link.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	}
	if revokedStr.Valid && revokedStr.String != "" {
		revTime, err := time.Parse(time.RFC3339Nano, revokedStr.String)
		if err != nil {
			revTime, _ = time.Parse(time.RFC3339, revokedStr.String)
		}
		if !revTime.IsZero() {
			link.RevokedAt = &revTime
		}
	}

	return &link, nil
}

func (r *subscriptionLinkRepository) Create(ctx context.Context, link *domain.SubscriptionLink) error {
	if link == nil {
		return domain.NewValidationError("missing_link", "subscription link is required")
	}
	if !link.Target.IsValid() {
		return domain.NewValidationError("invalid_target", fmt.Sprintf("invalid compiler target: %s", link.Target))
	}
	if strings.TrimSpace(link.Token) == "" || strings.TrimSpace(link.TokenHash) == "" {
		return domain.NewValidationError("missing_token", "subscription link token and token_hash are required")
	}

	now := domain.NowUTC()
	if link.ID == "" {
		link.ID = domain.MustNewUUIDv7()
	}
	if link.CreatedAt.IsZero() {
		link.CreatedAt = now
	}
	if link.UpdatedAt.IsZero() {
		link.UpdatedAt = now
	}
	if link.State == "" {
		link.State = domain.SubscriptionLinkStateActive
	}

	// For default links, maintain atomic idempotency
	if link.IsDefault && link.State == domain.SubscriptionLinkStateActive {
		existing, err := r.GetDefaultByTarget(ctx, link.Target)
		if err == nil && existing != nil {
			*link = *existing
			return nil
		}
	}

	const insertSQL = `
	INSERT INTO subscription_links (
		id, label, target, token, token_hash, state, is_default, created_at, updated_at, revoked_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	isDefaultInt := 0
	if link.IsDefault {
		isDefaultInt = 1
	}
	var revokedVal any = nil
	if link.RevokedAt != nil && !link.RevokedAt.IsZero() {
		revokedVal = link.RevokedAt.Format(time.RFC3339Nano)
	}

	_, err := r.db.ExecContext(ctx, insertSQL,
		link.ID,
		link.Label,
		string(link.Target),
		link.Token,
		link.TokenHash,
		string(link.State),
		isDefaultInt,
		link.CreatedAt.Format(time.RFC3339Nano),
		link.UpdatedAt.Format(time.RFC3339Nano),
		revokedVal,
	)
	if err != nil {
		// If unique constraint on (target, is_default) was violated under concurrency, retrieve the existing default link
		if link.IsDefault && (strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint")) {
			existing, getErr := r.GetDefaultByTarget(ctx, link.Target)
			if getErr == nil && existing != nil {
				*link = *existing
				return nil
			}
		}
		return fmt.Errorf("failed to insert subscription link: %w", err)
	}

	return nil
}

func (r *subscriptionLinkRepository) GetByID(ctx context.Context, id string) (*domain.SubscriptionLink, error) {
	query := selectSubscriptionLinkColumnsSQL + " WHERE id = ?;"
	row := r.db.QueryRowContext(ctx, query, strings.TrimSpace(id))
	return scanSubscriptionLink(row, fmt.Sprintf("subscription link %s not found", id))
}

func (r *subscriptionLinkRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.SubscriptionLink, error) {
	query := selectSubscriptionLinkColumnsSQL + " WHERE token_hash = ? AND state = 'active' LIMIT 1;"
	row := r.db.QueryRowContext(ctx, query, strings.TrimSpace(tokenHash))
	return scanSubscriptionLink(row, "subscription link not found for token hash")
}

func (r *subscriptionLinkRepository) GetDefaultByTarget(ctx context.Context, target domain.CompilerTarget) (*domain.SubscriptionLink, error) {
	query := selectSubscriptionLinkColumnsSQL + " WHERE target = ? AND is_default = 1 AND state = 'active' LIMIT 1;"
	row := r.db.QueryRowContext(ctx, query, string(target))
	return scanSubscriptionLink(row, fmt.Sprintf("default subscription link not found for target %s", target))
}

func (r *subscriptionLinkRepository) List(ctx context.Context) ([]domain.SubscriptionLink, error) {
	query := selectSubscriptionLinkColumnsSQL + " ORDER BY created_at DESC, id ASC;"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscription links: %w", err)
	}
	defer rows.Close()

	var links []domain.SubscriptionLink
	for rows.Next() {
		link, err := scanSubscriptionLink(rows, "")
		if err != nil {
			return nil, err
		}
		links = append(links, *link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating subscription links: %w", err)
	}
	return links, nil
}

func (r *subscriptionLinkRepository) UpdateLabel(ctx context.Context, id string, label string, updatedAt time.Time) error {
	const query = `UPDATE subscription_links SET label = ?, updated_at = ? WHERE id = ?;`
	if updatedAt.IsZero() {
		updatedAt = domain.NowUTC()
	}
	res, err := r.db.ExecContext(ctx, query, strings.TrimSpace(label), updatedAt.Format(time.RFC3339Nano), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("failed to update subscription link label: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return domain.NewNotFoundError("subscription_link_not_found", fmt.Sprintf("subscription link %s not found", id))
	}
	return nil
}

func (r *subscriptionLinkRepository) RotateToken(ctx context.Context, id string, newToken, newTokenHash string, updatedAt time.Time) error {
	if strings.TrimSpace(newToken) == "" || strings.TrimSpace(newTokenHash) == "" {
		return domain.NewValidationError("missing_token", "new token and token_hash are required")
	}
	if updatedAt.IsZero() {
		updatedAt = domain.NowUTC()
	}
	const query = `UPDATE subscription_links SET token = ?, token_hash = ?, updated_at = ? WHERE id = ? AND state = 'active';`
	res, err := r.db.ExecContext(ctx, query, newToken, newTokenHash, updatedAt.Format(time.RFC3339Nano), strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("failed to rotate subscription link token: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		// Check whether it was not found or already revoked
		link, err := r.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if link.State == domain.SubscriptionLinkStateRevoked {
			return domain.NewConflictError("link_revoked", fmt.Sprintf("cannot rotate token for revoked subscription link %s", id))
		}
		return domain.NewNotFoundError("subscription_link_not_found", fmt.Sprintf("subscription link %s not found", id))
	}
	return nil
}

func (r *subscriptionLinkRepository) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	if revokedAt.IsZero() {
		revokedAt = domain.NowUTC()
	}
	revokedStr := revokedAt.Format(time.RFC3339Nano)
	const query = `UPDATE subscription_links SET state = 'revoked', revoked_at = ?, updated_at = ? WHERE id = ?;`
	res, err := r.db.ExecContext(ctx, query, revokedStr, revokedStr, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("failed to revoke subscription link: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return domain.NewNotFoundError("subscription_link_not_found", fmt.Sprintf("subscription link %s not found", id))
	}
	return nil
}
