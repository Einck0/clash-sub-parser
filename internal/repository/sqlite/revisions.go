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

type revisionRepository struct {
	db *sql.DB
}

// NewRevisionRepository constructs a SQLite implementation of domain.RevisionRepository.
func NewRevisionRepository(db *sql.DB) domain.RevisionRepository {
	return &revisionRepository{db: db}
}

func (r *revisionRepository) GetByID(ctx context.Context, id string) (*domain.ConfigurationRevision, error) {
	const query = `
	SELECT id, parent_id, content_digest, state, created_at
	FROM configuration_revisions
	WHERE id = ?;`

	var rev domain.ConfigurationRevision
	var parentID sql.NullString
	var stateStr, createdStr string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&rev.ID,
		&parentID,
		&rev.ContentDigest,
		&stateStr,
		&createdStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("revision_not_found", fmt.Sprintf("configuration revision %s not found", id))
		}
		return nil, fmt.Errorf("failed to query configuration revision %s: %w", id, err)
	}

	if parentID.Valid && parentID.String != "" {
		rev.ParentID = &parentID.String
	}
	rev.State = domain.ConfigurationRevisionState(stateStr)
	rev.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)

	return &rev, nil
}

func (r *revisionRepository) GetActive(ctx context.Context) (*domain.ConfigurationRevision, error) {
	const query = `
	SELECT id, parent_id, content_digest, state, created_at
	FROM configuration_revisions
	WHERE state = 'active'
	ORDER BY created_at DESC
	LIMIT 1;`

	var rev domain.ConfigurationRevision
	var parentID sql.NullString
	var stateStr, createdStr string

	err := r.db.QueryRowContext(ctx, query).Scan(
		&rev.ID,
		&parentID,
		&rev.ContentDigest,
		&stateStr,
		&createdStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("active_revision_not_found", "no active configuration revision exists")
		}
		return nil, fmt.Errorf("failed to query active configuration revision: %w", err)
	}

	if parentID.Valid && parentID.String != "" {
		rev.ParentID = &parentID.String
	}
	rev.State = domain.ConfigurationRevisionState(stateStr)
	rev.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)

	return &rev, nil
}

func (r *revisionRepository) List(ctx context.Context, filter domain.RevisionFilter) ([]domain.ConfigurationRevision, int, error) {
	page := filter.Pagination.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.Pagination.PageSize
	if pageSize < 1 {
		pageSize = 50
	} else if pageSize > 100 {
		pageSize = 100
	}

	whereClauses := make([]string, 0, 1)
	args := make([]any, 0, 1)
	if filter.State != nil && *filter.State != "" {
		whereClauses = append(whereClauses, "state = ?")
		args = append(args, string(*filter.State))
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM configuration_revisions %s;", whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count configuration revisions: %w", err)
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, parent_id, content_digest, state, created_at
		FROM configuration_revisions
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?;`, whereSQL)

	queryArgs := append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list configuration revisions: %w", err)
	}
	defer rows.Close()

	items := make([]domain.ConfigurationRevision, 0)
	for rows.Next() {
		var rev domain.ConfigurationRevision
		var parentID sql.NullString
		var stateStr, createdStr string
		if err := rows.Scan(&rev.ID, &parentID, &rev.ContentDigest, &stateStr, &createdStr); err != nil {
			return nil, 0, fmt.Errorf("failed to scan configuration revision: %w", err)
		}
		if parentID.Valid && parentID.String != "" {
			rev.ParentID = &parentID.String
		}
		rev.State = domain.ConfigurationRevisionState(stateStr)
		rev.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		items = append(items, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate configuration revisions: %w", err)
	}
	return items, total, nil
}

func (r *revisionRepository) Create(ctx context.Context, rev *domain.ConfigurationRevision) error {
	const query = `
	INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
	VALUES (?, ?, ?, ?, ?);`

	var parentStr sql.NullString
	if rev.ParentID != nil && *rev.ParentID != "" {
		parentStr = sql.NullString{String: *rev.ParentID, Valid: true}
	}

	createdStr := rev.CreatedAt.Format(time.RFC3339)
	if rev.CreatedAt.IsZero() {
		createdStr = domain.NowUTC().Format(time.RFC3339)
	}

	_, err := r.db.ExecContext(ctx, query,
		rev.ID,
		parentStr,
		rev.ContentDigest,
		string(rev.State),
		createdStr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert configuration revision: %w", err)
	}
	return nil
}

func (r *revisionRepository) SetActive(ctx context.Context, id string) error {
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		// Archive previously active revisions
		_, err := tx.ExecContext(ctx, "UPDATE configuration_revisions SET state = 'archived' WHERE state = 'active';")
		if err != nil {
			return fmt.Errorf("failed to archive previously active revisions: %w", err)
		}

		res, err := tx.ExecContext(ctx, "UPDATE configuration_revisions SET state = 'active' WHERE id = ?;", id)
		if err != nil {
			return fmt.Errorf("failed to activate revision %s: %w", id, err)
		}

		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return domain.NewNotFoundError("revision_not_found", fmt.Sprintf("configuration revision %s not found", id))
		}

		return nil
	})
}

// PublicationRepository implementation

type publicationRepository struct {
	db *sql.DB
}

// NewPublicationRepository constructs a SQLite implementation of domain.PublicationRepository.
func NewPublicationRepository(db *sql.DB) domain.PublicationRepository {
	return &publicationRepository{db: db}
}

func (r *publicationRepository) GetByID(ctx context.Context, id string) (*domain.Publication, error) {
	const query = `
	SELECT id, target, snapshot_digest, compiler_version, token_hash, state, created_at, revoked_at
	FROM publications
	WHERE id = ?;`

	var pub domain.Publication
	var targetStr, stateStr, createdStr string
	var revokedStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&pub.ID,
		&targetStr,
		&pub.SnapshotDigest,
		&pub.CompilerVersion,
		&pub.TokenHash,
		&stateStr,
		&createdStr,
		&revokedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("publication_not_found", fmt.Sprintf("publication %s not found", id))
		}
		return nil, fmt.Errorf("failed to query publication %s: %w", id, err)
	}

	pub.Target = domain.CompilerTarget(targetStr)
	pub.State = domain.PublicationState(stateStr)
	pub.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	if revokedStr.Valid && revokedStr.String != "" {
		revTime, err := time.Parse(time.RFC3339, revokedStr.String)
		if err == nil {
			pub.RevokedAt = &revTime
		}
	}

	return &pub, nil
}

func (r *publicationRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Publication, error) {
	const query = `
	SELECT id, target, snapshot_digest, compiler_version, token_hash, state, created_at, revoked_at
	FROM publications
	WHERE token_hash = ?;`

	var pub domain.Publication
	var targetStr, stateStr, createdStr string
	var revokedStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&pub.ID,
		&targetStr,
		&pub.SnapshotDigest,
		&pub.CompilerVersion,
		&pub.TokenHash,
		&stateStr,
		&createdStr,
		&revokedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("publication_not_found", fmt.Sprintf("publication for token hash %s not found", tokenHash))
		}
		return nil, fmt.Errorf("failed to query publication by token hash: %w", err)
	}

	pub.Target = domain.CompilerTarget(targetStr)
	pub.State = domain.PublicationState(stateStr)
	pub.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	if revokedStr.Valid && revokedStr.String != "" {
		revTime, err := time.Parse(time.RFC3339, revokedStr.String)
		if err == nil {
			pub.RevokedAt = &revTime
		}
	}

	return &pub, nil
}

func (r *publicationRepository) Create(ctx context.Context, pub *domain.Publication) error {
	const query = `
	INSERT INTO publications (id, target, snapshot_digest, compiler_version, token_hash, state, created_at, revoked_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?);`

	createdStr := pub.CreatedAt.Format(time.RFC3339)
	if pub.CreatedAt.IsZero() {
		createdStr = domain.NowUTC().Format(time.RFC3339)
	}

	var revokedStr sql.NullString
	if pub.RevokedAt != nil && !pub.RevokedAt.IsZero() {
		revokedStr = sql.NullString{String: pub.RevokedAt.Format(time.RFC3339), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, query,
		pub.ID,
		string(pub.Target),
		pub.SnapshotDigest,
		pub.CompilerVersion,
		pub.TokenHash,
		string(pub.State),
		createdStr,
		revokedStr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert publication: %w", err)
	}
	return nil
}

func (r *publicationRepository) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	const query = `
	UPDATE publications SET state = 'revoked', revoked_at = ?
	WHERE id = ?;`

	revStr := revokedAt.UTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx, query, revStr, id)
	if err != nil {
		return fmt.Errorf("failed to revoke publication %s: %w", id, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.NewNotFoundError("publication_not_found", fmt.Sprintf("publication %s not found", id))
	}
	return nil
}
