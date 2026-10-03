package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
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

func marshalNodeCredentials(creds domain.InboundProtocolCredential) (string, error) {
	raw, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("failed to marshal node credentials: %w", err)
	}
	if len(raw) == 0 {
		return "{}", nil
	}
	return string(raw), nil
}

func unmarshalNodeCredentials(configJSON string) (domain.InboundProtocolCredential, error) {
	var creds domain.InboundProtocolCredential
	trimmed := strings.TrimSpace(configJSON)
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return creds, nil
	}
	if err := json.Unmarshal([]byte(trimmed), &creds); err != nil {
		return domain.InboundProtocolCredential{}, fmt.Errorf("failed to unmarshal node credentials: %w", err)
	}
	return creds, nil
}

func (r *nodeRepository) GetByLogicalID(ctx context.Context, logicalID string) (*domain.Node, error) {
	const query = `
	SELECT n.logical_id, n.protocol, n.display_name,
	       COALESCE(json_extract(v.effective_config_json, '$.server'), n.server),
	       COALESCE(json_extract(v.effective_config_json, '$.port'), n.port),
	       COALESCE(json_extract(v.effective_config_json, '$.credentials'), v.effective_config_json, n.config_json),
	       n.active, n.created_at, n.updated_at,
	       COALESCE(h.connection_revision, n.connection_revision)
	FROM nodes n
	LEFT JOIN node_connection_heads h ON n.logical_id = h.logical_id
	LEFT JOIN node_connection_versions v ON h.logical_id = v.node_logical_id AND h.connection_revision = v.connection_revision
	WHERE n.logical_id = ?;`

	var node domain.Node
	var configJSON string
	var activeInt int
	var createdStr, updatedStr string

	err := r.db.QueryRowContext(ctx, query, logicalID).Scan(
		&node.LogicalID,
		&node.Protocol,
		&node.DisplayName,
		&node.Server,
		&node.Port,
		&configJSON,
		&activeInt,
		&createdStr,
		&updatedStr,
		&node.ConnectionRevision,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("node_not_found", fmt.Sprintf("node %s not found", logicalID))
		}
		return nil, fmt.Errorf("failed to query node %s: %w", logicalID, err)
	}

	creds, err := unmarshalNodeCredentials(configJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials for node %s: %w", logicalID, err)
	}
	node.Credentials = creds
	node.Active = activeInt == 1
	node.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	node.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &node, nil
}

func (r *nodeRepository) List(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, int, error) {
	whereClauses := make([]string, 0)
	args := make([]interface{}, 0)

	if filter.ActiveOnly {
		whereClauses = append(whereClauses, "n.active = 1")
	}
	if len(filter.LogicalIDs) > 0 {
		placeholders := make([]string, len(filter.LogicalIDs))
		for i, id := range filter.LogicalIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("n.logical_id IN (%s)", strings.Join(placeholders, ",")))
	}
	if len(filter.Protocols) > 0 {
		placeholders := make([]string, len(filter.Protocols))
		for i, p := range filter.Protocols {
			placeholders[i] = "?"
			args = append(args, string(p))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("n.protocol IN (%s)", strings.Join(placeholders, ",")))
	}
	if filter.SearchText != "" {
		whereClauses = append(whereClauses, "n.display_name LIKE ?")
		args = append(args, "%"+filter.SearchText+"%")
	}
	if filter.ExcludeNotices {
		var tableExists int
		_ = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='subscription_entries';").Scan(&tableExists)
		if tableExists > 0 {
			whereClauses = append(whereClauses, `n.logical_id NOT IN (
				SELECT se1.node_logical_id FROM subscription_entries se1
				WHERE se1.node_logical_id IS NOT NULL
				  AND COALESCE(se1.user_kind_override, se1.entry_kind) = 'notice'
				  AND NOT EXISTS (
				      SELECT 1 FROM subscription_entries se2
				      WHERE se2.node_logical_id = se1.node_logical_id
				        AND se2.user_kind_override = 'proxy'
				  )
			)`)
		}
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countWhereSQL := strings.ReplaceAll(whereSQL, "n.", "")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM nodes %s;", countWhereSQL)
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

	orderBy := "n.created_at DESC"
	if filter.SortBy == "display_name" {
		order := "ASC"
		if strings.ToUpper(filter.SortOrder) == "DESC" {
			order = "DESC"
		}
		orderBy = fmt.Sprintf("n.display_name %s", order)
	}

	selectQuery := fmt.Sprintf(`
	SELECT n.logical_id, n.protocol, n.display_name,
	       COALESCE(json_extract(v.effective_config_json, '$.server'), n.server),
	       COALESCE(json_extract(v.effective_config_json, '$.port'), n.port),
	       COALESCE(json_extract(v.effective_config_json, '$.credentials'), v.effective_config_json, n.config_json),
	       n.active, n.created_at, n.updated_at,
	       COALESCE(h.connection_revision, n.connection_revision)
	FROM nodes n
	LEFT JOIN node_connection_heads h ON n.logical_id = h.logical_id
	LEFT JOIN node_connection_versions v ON h.logical_id = v.node_logical_id AND h.connection_revision = v.connection_revision
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
		var configJSON string
		var activeInt int
		var createdStr, updatedStr string

		err := rows.Scan(
			&node.LogicalID,
			&node.Protocol,
			&node.DisplayName,
			&node.Server,
			&node.Port,
			&configJSON,
			&activeInt,
			&createdStr,
			&updatedStr,
			&node.ConnectionRevision,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan node: %w", err)
		}

		creds, err := unmarshalNodeCredentials(configJSON)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal credentials for node %s: %w", node.LogicalID, err)
		}
		node.Credentials = creds
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
	INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(logical_id) DO UPDATE SET
		protocol = excluded.protocol,
		display_name = excluded.display_name,
		server = excluded.server,
		port = excluded.port,
		config_json = excluded.config_json,
		active = excluded.active,
		updated_at = excluded.updated_at,
		connection_revision = nodes.connection_revision + CASE WHEN
			nodes.protocol IS NOT excluded.protocol OR nodes.server IS NOT excluded.server OR
			nodes.port IS NOT excluded.port OR nodes.config_json IS NOT excluded.config_json OR
			(nodes.active = 0 AND excluded.active = 1)
			THEN 1 ELSE 0 END;`

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

			configJSON, err := marshalNodeCredentials(node.Credentials)
			if err != nil {
				return fmt.Errorf("failed to marshal credentials for node %s: %w", node.LogicalID, err)
			}

			createdStr := node.CreatedAt.Format(time.RFC3339)
			if node.CreatedAt.IsZero() {
				createdStr = nowStr
			}
			updatedStr := node.UpdatedAt.Format(time.RFC3339)
			if node.UpdatedAt.IsZero() {
				updatedStr = nowStr
			}

			_, err = stmt.ExecContext(ctx,
				node.LogicalID,
				string(node.Protocol),
				node.DisplayName,
				node.Server,
				node.Port,
				configJSON,
				activeInt,
				createdStr,
				updatedStr,
			)
			if err != nil {
				return fmt.Errorf("failed to upsert node %s: %w", node.LogicalID, err)
			}

			// Maintain node_connection_versions and node_connection_heads
			var curRev int64
			var curServer string
			var curPort int
			var curConfig string
			if qErr := tx.QueryRowContext(ctx, "SELECT connection_revision, server, port, config_json FROM nodes WHERE logical_id = ?;", node.LogicalID).Scan(&curRev, &curServer, &curPort, &curConfig); qErr == nil {
				effJSON := fmt.Sprintf(`{"server":%q,"port":%d,"credentials":%s}`, curServer, curPort, curConfig)
				fp := domain.ComputeConnectionFingerprint(curServer, curPort, node.Credentials)
				_, _ = tx.ExecContext(ctx, `
					INSERT OR IGNORE INTO node_connection_versions (node_logical_id, connection_revision, effective_config_json, config_fingerprint, schema_version, created_at)
					VALUES (?, ?, ?, ?, 1, ?);
				`, node.LogicalID, curRev, effJSON, fp, updatedStr)
				_, _ = tx.ExecContext(ctx, `
					INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)
					VALUES (?, ?, ?)
					ON CONFLICT(logical_id) DO UPDATE SET
						connection_revision = excluded.connection_revision,
						updated_at = excluded.updated_at;
				`, node.LogicalID, curRev, updatedStr)
			}
		}
		return nil
	})
}

// Update persists plaintext display_name, server, port, and config_json updates for an existing node.
func (r *nodeRepository) Update(ctx context.Context, node *domain.Node) error {
	return UpdateNode(ctx, r.db, node)
}

// UpdateNode updates an existing node's plaintext connection and credentials in SQLite.
func UpdateNode(ctx context.Context, db *sql.DB, node *domain.Node) error {
	if node == nil {
		return domain.NewValidationError("nil_node", "node cannot be nil")
	}
	configJSON, err := marshalNodeCredentials(node.Credentials)
	if err != nil {
		return err
	}
	updatedAt := node.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = domain.NowUTC()
	}
	updatedStr := updatedAt.UTC().Format(time.RFC3339)

	const query = `
	UPDATE nodes
	SET display_name = ?, server = ?, port = ?, config_json = ?, updated_at = ?,
		connection_revision = connection_revision + CASE WHEN
			server IS NOT ? OR port IS NOT ? OR config_json IS NOT ? THEN 1 ELSE 0 END
	WHERE logical_id = ? RETURNING connection_revision;`

	err = db.QueryRowContext(ctx, query,
		node.DisplayName,
		node.Server,
		node.Port,
		configJSON,
		updatedStr,
		node.Server, node.Port, configJSON,
		node.LogicalID,
	).Scan(&node.ConnectionRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewNotFoundError("node_not_found", fmt.Sprintf("node %s not found", node.LogicalID))
	}
	if err != nil {
		return fmt.Errorf("failed to update node %s: %w", node.LogicalID, err)
	}
	node.UpdatedAt = updatedAt.UTC()

	// Maintain node_connection_versions and node_connection_heads
	effJSON := fmt.Sprintf(`{"server":%q,"port":%d,"credentials":%s}`, node.Server, node.Port, configJSON)
	fp := domain.ComputeConnectionFingerprint(node.Server, node.Port, node.Credentials)
	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO node_connection_versions (node_logical_id, connection_revision, effective_config_json, config_fingerprint, schema_version, created_at)
		VALUES (?, ?, ?, ?, 1, ?);
	`, node.LogicalID, node.ConnectionRevision, effJSON, fp, updatedStr)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(logical_id) DO UPDATE SET
			connection_revision = excluded.connection_revision,
			updated_at = excluded.updated_at;
	`, node.LogicalID, node.ConnectionRevision, updatedStr)

	return nil
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
