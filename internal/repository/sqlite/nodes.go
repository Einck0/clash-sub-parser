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

type nodeRepository struct {
	db *sql.DB
}

// NewNodeRepository constructs a SQLite implementation of domain.NodeRepository.
func NewNodeRepository(db *sql.DB) domain.NodeRepository {
	return &nodeRepository{db: db}
}

func (r *nodeRepository) GetByLogicalID(ctx context.Context, logicalID string) (*domain.Node, error) {
	const query = `
	SELECT logical_id, protocol, display_name, normalized_config_secret_ref, credential_version, active, created_at, updated_at
	FROM nodes
	WHERE logical_id = ?;`

	var node domain.Node
	var activeInt int
	var createdStr, updatedStr string

	err := r.db.QueryRowContext(ctx, query, logicalID).Scan(
		&node.LogicalID,
		&node.Protocol,
		&node.DisplayName,
		&node.NormalizedConfigSecretRef,
		&node.CredentialVersion,
		&activeInt,
		&createdStr,
		&updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("node_not_found", fmt.Sprintf("node %s not found", logicalID))
		}
		return nil, fmt.Errorf("failed to query node %s: %w", logicalID, err)
	}

	node.Active = activeInt == 1
	node.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	node.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &node, nil
}

func (r *nodeRepository) List(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, int, error) {
	whereClauses := make([]string, 0)
	args := make([]interface{}, 0)

	if filter.ActiveOnly {
		whereClauses = append(whereClauses, "active = 1")
	}
	if len(filter.Protocols) > 0 {
		placeholders := make([]string, len(filter.Protocols))
		for i, p := range filter.Protocols {
			placeholders[i] = "?"
			args = append(args, string(p))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("protocol IN (%s)", strings.Join(placeholders, ",")))
	}
	if filter.SearchText != "" {
		whereClauses = append(whereClauses, "display_name LIKE ?")
		args = append(args, "%"+filter.SearchText+"%")
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM nodes %s;", whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count nodes: %w", err)
	}

	pageSize := filter.Pagination.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	page := filter.Pagination.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	orderBy := "created_at DESC"
	if filter.SortBy == "display_name" {
		order := "ASC"
		if strings.ToUpper(filter.SortOrder) == "DESC" {
			order = "DESC"
		}
		orderBy = fmt.Sprintf("display_name %s", order)
	}

	selectQuery := fmt.Sprintf(`
	SELECT logical_id, protocol, display_name, normalized_config_secret_ref, credential_version, active, created_at, updated_at
	FROM nodes
	%s
	ORDER BY %s
	LIMIT ? OFFSET ?;`, whereSQL, orderBy)

	queryArgs := append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Node, 0)
	for rows.Next() {
		var node domain.Node
		var activeInt int
		var createdStr, updatedStr string

		err := rows.Scan(
			&node.LogicalID,
			&node.Protocol,
			&node.DisplayName,
			&node.NormalizedConfigSecretRef,
			&node.CredentialVersion,
			&activeInt,
			&createdStr,
			&updatedStr,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan node: %w", err)
		}

		node.Active = activeInt == 1
		node.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		node.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

		items = append(items, node)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating nodes: %w", err)
	}

	return items, total, nil
}

func (r *nodeRepository) UpsertBatch(ctx context.Context, nodes []domain.Node) error {
	if len(nodes) == 0 {
		return nil
	}

	const query = `
	INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, credential_version, active, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(logical_id) DO UPDATE SET
		protocol = excluded.protocol,
		display_name = excluded.display_name,
		normalized_config_secret_ref = excluded.normalized_config_secret_ref,
		credential_version = excluded.credential_version,
		active = excluded.active,
		updated_at = excluded.updated_at;`

	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to prepare upsert node statement: %w", err)
		}
		defer stmt.Close()

		nowStr := domain.NowUTC().Format(time.RFC3339)
		for _, node := range nodes {
			activeInt := 0
			if node.Active {
				activeInt = 1
			}

			createdStr := node.CreatedAt.Format(time.RFC3339)
			if node.CreatedAt.IsZero() {
				createdStr = nowStr
			}
			updatedStr := node.UpdatedAt.Format(time.RFC3339)
			if node.UpdatedAt.IsZero() {
				updatedStr = nowStr
			}

			_, err := stmt.ExecContext(ctx,
				node.LogicalID,
				string(node.Protocol),
				node.DisplayName,
				node.NormalizedConfigSecretRef,
				node.CredentialVersion,
				activeInt,
				createdStr,
				updatedStr,
			)
			if err != nil {
				return fmt.Errorf("failed to upsert node %s: %w", node.LogicalID, err)
			}
		}
		return nil
	})
}

func (r *nodeRepository) DeactivateNodesNotIn(ctx context.Context, activeLogicalIDs []string) error {
	nowStr := domain.NowUTC().Format(time.RFC3339)

	if len(activeLogicalIDs) == 0 {
		// Deactivate all nodes
		_, err := r.db.ExecContext(ctx, "UPDATE nodes SET active = 0, updated_at = ? WHERE active = 1;", nowStr)
		if err != nil {
			return fmt.Errorf("failed to deactivate all nodes: %w", err)
		}
		return nil
	}

	placeholders := make([]string, len(activeLogicalIDs))
	args := make([]interface{}, len(activeLogicalIDs)+1)
	args[0] = nowStr
	for i, id := range activeLogicalIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := fmt.Sprintf("UPDATE nodes SET active = 0, updated_at = ? WHERE active = 1 AND logical_id NOT IN (%s);", strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to deactivate inactive nodes: %w", err)
	}

	return nil
}

// NodeSourceRepository implementation

type nodeSourceRepository struct {
	db *sql.DB
}

// NewNodeSourceRepository constructs a SQLite implementation of domain.NodeSourceRepository.
func NewNodeSourceRepository(db *sql.DB) domain.NodeSourceRepository {
	return &nodeSourceRepository{db: db}
}

func (r *nodeSourceRepository) ListByNode(ctx context.Context, logicalID string) ([]domain.NodeSource, error) {
	const query = `
	SELECT node_logical_id, subscription_id, last_seen_fetch_id
	FROM node_sources
	WHERE node_logical_id = ?;`

	rows, err := r.db.QueryContext(ctx, query, logicalID)
	if err != nil {
		return nil, fmt.Errorf("failed to query node sources for node %s: %w", logicalID, err)
	}
	defer rows.Close()

	items := make([]domain.NodeSource, 0)
	for rows.Next() {
		var s domain.NodeSource
		if err := rows.Scan(&s.NodeLogicalID, &s.SubscriptionID, &s.LastSeenFetchID); err != nil {
			return nil, fmt.Errorf("failed to scan node source: %w", err)
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node sources: %w", err)
	}
	return items, nil
}

func (r *nodeSourceRepository) ListByNodes(ctx context.Context, logicalIDs []string) (map[string][]domain.NodeSource, error) {
	result := make(map[string][]domain.NodeSource)
	if len(logicalIDs) == 0 {
		return result, nil
	}

	for _, id := range logicalIDs {
		result[id] = make([]domain.NodeSource, 0)
	}

	const chunkSize = 100
	for i := 0; i < len(logicalIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(logicalIDs) {
			end = len(logicalIDs)
		}
		chunk := logicalIDs[i:end]

		placeholders := make([]string, len(chunk))
		args := make([]interface{}, len(chunk))
		for j, id := range chunk {
			placeholders[j] = "?"
			args[j] = id
		}

		query := fmt.Sprintf(`
		SELECT node_logical_id, subscription_id, last_seen_fetch_id
		FROM node_sources
		WHERE node_logical_id IN (%s)
		ORDER BY node_logical_id ASC, subscription_id ASC;`, strings.Join(placeholders, ", "))

		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to query node sources for batch: %w", err)
		}

		for rows.Next() {
			var s domain.NodeSource
			if err := rows.Scan(&s.NodeLogicalID, &s.SubscriptionID, &s.LastSeenFetchID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan node source in batch: %w", err)
			}
			result[s.NodeLogicalID] = append(result[s.NodeLogicalID], s)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("error iterating node sources in batch: %w", err)
		}
		rows.Close()
	}

	return result, nil
}

func (r *nodeSourceRepository) ListBySubscription(ctx context.Context, subID string) ([]domain.NodeSource, error) {
	const query = `
	SELECT node_logical_id, subscription_id, last_seen_fetch_id
	FROM node_sources
	WHERE subscription_id = ?;`

	rows, err := r.db.QueryContext(ctx, query, subID)
	if err != nil {
		return nil, fmt.Errorf("failed to query node sources for subscription %s: %w", subID, err)
	}
	defer rows.Close()

	items := make([]domain.NodeSource, 0)
	for rows.Next() {
		var s domain.NodeSource
		if err := rows.Scan(&s.NodeLogicalID, &s.SubscriptionID, &s.LastSeenFetchID); err != nil {
			return nil, fmt.Errorf("failed to scan node source: %w", err)
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node sources: %w", err)
	}
	return items, nil
}

func (r *nodeSourceRepository) Upsert(ctx context.Context, src *domain.NodeSource) error {
	const query = `
	INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
	VALUES (?, ?, ?)
	ON CONFLICT(node_logical_id, subscription_id) DO UPDATE SET
		last_seen_fetch_id = excluded.last_seen_fetch_id;`

	_, err := r.db.ExecContext(ctx, query, src.NodeLogicalID, src.SubscriptionID, src.LastSeenFetchID)
	if err != nil {
		return fmt.Errorf("failed to upsert node source: %w", err)
	}
	return nil
}

func (r *nodeSourceRepository) DeleteBySubscriptionAndFetch(ctx context.Context, subID string, currentFetchID string) error {
	const query = `
	DELETE FROM node_sources
	WHERE subscription_id = ? AND last_seen_fetch_id != ?;`

	_, err := r.db.ExecContext(ctx, query, subID, currentFetchID)
	if err != nil {
		return fmt.Errorf("failed to prune obsolete node sources for sub %s: %w", subID, err)
	}
	return nil
}
