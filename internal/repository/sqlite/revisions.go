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

// CreateActive rolls back the new revision and previous active state if activation fails.
func (r *revisionRepository) CreateActive(ctx context.Context, rev *domain.ConfigurationRevision) error {
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		var parent sql.NullString
		if rev.ParentID != nil && *rev.ParentID != "" {
			parent = sql.NullString{String: *rev.ParentID, Valid: true}
		}
		created := rev.CreatedAt
		if created.IsZero() {
			created = domain.NowUTC()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at) VALUES (?, ?, ?, 'draft', ?);`,
			rev.ID, parent, rev.ContentDigest, created.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("failed to insert configuration revision: %w", err)
		}
		return setActiveRevision(ctx, tx, rev.ID)
	})
}

func (r *revisionRepository) SetActive(ctx context.Context, id string) error {
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		return setActiveRevision(ctx, tx, id)
	})
}

func setActiveRevision(ctx context.Context, tx *sql.Tx, id string) error {
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
}

// PublicationRepository implementation

type publicationRepository struct {
	db *sql.DB
}

// NewPublicationRepository constructs a SQLite implementation of domain.PublicationRepository.
func NewPublicationRepository(db *sql.DB) domain.PublicationRepository {
	return &publicationRepository{db: db}
}

const selectPublicationColumnsSQL = `
	SELECT
		id,
		revision_id,
		target,
		snapshot_digest,
		content_digest,
		content_type,
		filename,
		content,
		compiler_version,
		token_hash,
		state,
		created_at,
		revoked_at
	FROM publications`

func scanPublicationRow(row *sql.Row, notFoundMsg string) (*domain.Publication, error) {
	var pub domain.Publication
	var targetStr, stateStr, createdStr string
	var revokedStr sql.NullString
	var content []byte

	err := row.Scan(
		&pub.ID,
		&pub.RevisionID,
		&targetStr,
		&pub.SnapshotDigest,
		&pub.ContentDigest,
		&pub.ContentType,
		&pub.Filename,
		&content,
		&pub.CompilerVersion,
		&pub.TokenHash,
		&stateStr,
		&createdStr,
		&revokedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("publication_not_found", notFoundMsg)
		}
		return nil, fmt.Errorf("failed to query publication: %w", err)
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
	if len(content) > 0 {
		pub.Content = append([]byte(nil), content...)
	}

	return &pub, nil
}

func (r *publicationRepository) GetByID(ctx context.Context, id string) (*domain.Publication, error) {
	query := selectPublicationColumnsSQL + "\n\tWHERE id = ?;"
	row := r.db.QueryRowContext(ctx, query, id)
	return scanPublicationRow(row, fmt.Sprintf("publication %s not found", id))
}

func (r *publicationRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Publication, error) {
	query := selectPublicationColumnsSQL + "\n\tWHERE token_hash = ?;"
	row := r.db.QueryRowContext(ctx, query, tokenHash)
	return scanPublicationRow(row, fmt.Sprintf("publication for token hash %s not found", tokenHash))
}

func (r *publicationRepository) Create(ctx context.Context, pub *domain.Publication) error {
	if pub == nil {
		return domain.NewValidationError("nil_publication", "publication cannot be nil")
	}

	const query = `
	INSERT INTO publications (
		id,
		revision_id,
		target,
		snapshot_digest,
		content_digest,
		content_type,
		filename,
		content,
		compiler_version,
		token_hash,
		state,
		created_at,
		revoked_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	createdStr := pub.CreatedAt.Format(time.RFC3339)
	if pub.CreatedAt.IsZero() {
		createdStr = domain.NowUTC().Format(time.RFC3339)
	}

	var revokedStr sql.NullString
	if pub.RevokedAt != nil && !pub.RevokedAt.IsZero() {
		revokedStr = sql.NullString{String: pub.RevokedAt.Format(time.RFC3339), Valid: true}
	}

	content := pub.Content
	if content == nil {
		content = []byte{}
	}

	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query,
			pub.ID,
			pub.RevisionID,
			string(pub.Target),
			pub.SnapshotDigest,
			pub.ContentDigest,
			pub.ContentType,
			pub.Filename,
			content,
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
	})
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

func (r *publicationRepository) Activate(ctx context.Context, id, tokenHash string) error {
	const query = `
	UPDATE publications SET state = 'active', token_hash = ?
	WHERE id = ?;`

	res, err := r.db.ExecContext(ctx, query, tokenHash, id)
	if err != nil {
		return fmt.Errorf("failed to activate publication %s: %w", id, err)
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
