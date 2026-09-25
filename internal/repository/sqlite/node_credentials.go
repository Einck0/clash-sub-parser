package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
)

type nodeCredentialRepository struct {
	db *sql.DB
}

// NewNodeCredentialRepository creates a SQLite repository for encrypted node credentials.
func NewNodeCredentialRepository(db *sql.DB) domain.NodeCredentialRepository {
	return &nodeCredentialRepository{db: db}
}

func (r *nodeCredentialRepository) GetByLogicalID(ctx context.Context, logicalID string, version int) (*domain.NodeCredentialRecord, error) {
	const query = `
	SELECT logical_id, version, key_id, nonce, ciphertext, created_at, updated_at
	FROM node_credentials
	WHERE logical_id = ? AND version = ?;`

	var record domain.NodeCredentialRecord
	var createdStr, updatedStr string
	err := r.db.QueryRowContext(ctx, query, logicalID, version).Scan(
		&record.LogicalID,
		&record.Version,
		&record.KeyID,
		&record.Nonce,
		&record.Ciphertext,
		&createdStr,
		&updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("node_credential_not_found", fmt.Sprintf("credentials for node %s version %d not found", logicalID, version))
		}
		return nil, fmt.Errorf("failed to query node credential: %w", err)
	}

	record.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	record.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &record, nil
}

func (r *nodeCredentialRepository) GetLatestByLogicalID(ctx context.Context, logicalID string) (*domain.NodeCredentialRecord, error) {
	return r.queryLatest(ctx, r.db.QueryRowContext, logicalID)
}

// GetLatestByLogicalIDTx reads from the caller's transaction so inventory can
// make version decisions atomically with node and credential updates.
func (r *nodeCredentialRepository) GetLatestByLogicalIDTx(ctx context.Context, tx *sql.Tx, logicalID string) (*domain.NodeCredentialRecord, error) {
	if tx == nil {
		return nil, domain.NewValidationError("nil_tx", "transaction cannot be nil")
	}
	return r.queryLatest(ctx, tx.QueryRowContext, logicalID)
}

type queryRowFunc func(ctx context.Context, query string, args ...any) *sql.Row

func (r *nodeCredentialRepository) queryLatest(ctx context.Context, queryRow queryRowFunc, logicalID string) (*domain.NodeCredentialRecord, error) {
	const query = `
	SELECT logical_id, version, key_id, nonce, ciphertext, created_at, updated_at
	FROM node_credentials
	WHERE logical_id = ?
	ORDER BY version DESC
	LIMIT 1;`

	var record domain.NodeCredentialRecord
	var createdStr, updatedStr string
	err := queryRow(ctx, query, logicalID).Scan(
		&record.LogicalID,
		&record.Version,
		&record.KeyID,
		&record.Nonce,
		&record.Ciphertext,
		&createdStr,
		&updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("node_credential_not_found", fmt.Sprintf("no credentials for node %s found", logicalID))
		}
		return nil, fmt.Errorf("failed to query latest node credential: %w", err)
	}

	record.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	record.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &record, nil
}

func (r *nodeCredentialRepository) Upsert(ctx context.Context, record *domain.NodeCredentialRecord) error {
	if record == nil {
		return domain.NewValidationError("nil_record", "credential record is nil")
	}

	const query = `
	INSERT INTO node_credentials (logical_id, version, key_id, nonce, ciphertext, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(logical_id, version) DO UPDATE SET
		key_id = excluded.key_id,
		nonce = excluded.nonce,
		ciphertext = excluded.ciphertext,
		updated_at = excluded.updated_at;`

	nowStr := domain.NowUTC().Format(time.RFC3339)
	createdStr := record.CreatedAt.Format(time.RFC3339)
	if record.CreatedAt.IsZero() {
		createdStr = nowStr
	}

	_, err := r.db.ExecContext(ctx, query,
		record.LogicalID,
		record.Version,
		record.KeyID,
		record.Nonce,
		record.Ciphertext,
		createdStr,
		nowStr,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert node credential: %w", err)
	}
	return nil
}

func (r *nodeCredentialRepository) DeleteByLogicalID(ctx context.Context, logicalID string) error {
	const query = `DELETE FROM node_credentials WHERE logical_id = ?;`
	_, err := r.db.ExecContext(ctx, query, logicalID)
	if err != nil {
		return fmt.Errorf("failed to delete node credentials for %s: %w", logicalID, err)
	}
	return nil
}
