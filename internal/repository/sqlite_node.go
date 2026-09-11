package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

type sqliteNodeRepository struct {
	db *SQLiteDB
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return hex.EncodeToString(b)
}

func scanNode(s rowScanner) (*domain.Node, error) {
	var (
		node                                  domain.Node
		protoStr, stateStr, payloadRaw        string
		createdAtStr, updatedAtStr            sql.NullString
	)

	err := s.Scan(
		&node.ID,
		&node.LogicalID,
		&node.Name,
		&protoStr,
		&node.Server,
		&node.Port,
		&payloadRaw,
		&node.PayloadFingerprint,
		&stateStr,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan node: %w", err)
	}

	node.Protocol = domain.ProtocolType(protoStr)
	node.LifecycleState = domain.LifecycleState(stateStr)

	if createdAtStr.Valid {
		if t, err := parseTime(createdAtStr.String); err == nil {
			node.CreatedAt = t
		}
	}
	if updatedAtStr.Valid {
		if t, err := parseTime(updatedAtStr.String); err == nil {
			node.UpdatedAt = t
		}
	}

	if payloadRaw != "" {
		_ = unmarshalJSONSafe(payloadRaw, &node.NormalizedPayload)
	}
	if node.NormalizedPayload == nil {
		node.NormalizedPayload = make(map[string]any)
	}

	return &node, nil
}

const nodeSelectFields = `
	id, logical_id, name, protocol, server, port,
	normalized_payload, payload_fingerprint, lifecycle_state,
	created_at, updated_at
`

func (r *sqliteNodeRepository) GetByID(ctx context.Context, id int64) (*domain.Node, error) {
	query := fmt.Sprintf("SELECT %s FROM nodes WHERE id = ?", nodeSelectFields)
	row := r.db.QueryRowContext(ctx, query, id)
	return scanNode(row)
}

func (r *sqliteNodeRepository) GetByLogicalID(ctx context.Context, logicalID string) (*domain.Node, error) {
	query := fmt.Sprintf("SELECT %s FROM nodes WHERE logical_id = ?", nodeSelectFields)
	row := r.db.QueryRowContext(ctx, query, logicalID)
	return scanNode(row)
}

func (r *sqliteNodeRepository) GetByFingerprint(ctx context.Context, fingerprint string) (*domain.Node, error) {
	query := fmt.Sprintf("SELECT %s FROM nodes WHERE payload_fingerprint = ?", nodeSelectFields)
	row := r.db.QueryRowContext(ctx, query, fingerprint)
	return scanNode(row)
}

func (r *sqliteNodeRepository) List(ctx context.Context, state domain.LifecycleState) ([]*domain.Node, error) {
	query := fmt.Sprintf("SELECT %s FROM nodes", nodeSelectFields)
	var args []any
	if state != "" {
		query += " WHERE lifecycle_state = ?"
		args = append(args, string(state))
	}
	query += " ORDER BY id"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	var result []*domain.Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nodes: %w", err)
	}
	return result, nil
}

func (r *sqliteNodeRepository) prepareNodeDefaults(node *domain.Node) {
	if strings.TrimSpace(node.LogicalID) == "" {
		node.LogicalID = generateUUID()
	}
	if strings.TrimSpace(node.PayloadFingerprint) == "" {
		node.PayloadFingerprint = node.ComputeFingerprint()
	}
	if node.LifecycleState == "" {
		node.LifecycleState = domain.LifecycleActive
	}
	now := time.Now().UTC()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	node.UpdatedAt = now
}

func (r *sqliteNodeRepository) Create(ctx context.Context, node *domain.Node) error {
	if node == nil {
		return ErrNilEntity
	}

	if err := node.Validate(); err != nil {
		return fmt.Errorf("validate node: %w", err)
	}

	r.prepareNodeDefaults(node)

	query := `
		INSERT INTO nodes (
			logical_id, name, protocol, server, port,
			normalized_payload, payload_fingerprint, lifecycle_state,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := r.db.ExecContext(ctx, query,
		node.LogicalID,
		node.Name,
		string(node.CanonicalProtocol()),
		node.Server,
		node.Port,
		marshalJSONSafe(node.NormalizedPayload, "{}"),
		node.PayloadFingerprint,
		string(node.LifecycleState),
		formatTime(node.CreatedAt),
		formatTime(node.UpdatedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicate
		}
		return fmt.Errorf("create node: %w", err)
	}

	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		node.ID = id
	}
	return nil
}

func (r *sqliteNodeRepository) Update(ctx context.Context, node *domain.Node) error {
	if node == nil {
		return ErrNilEntity
	}

	if err := node.Validate(); err != nil {
		return fmt.Errorf("validate node: %w", err)
	}

	node.UpdatedAt = time.Now().UTC()
	if strings.TrimSpace(node.PayloadFingerprint) == "" {
		node.PayloadFingerprint = node.ComputeFingerprint()
	}

	query := `
		UPDATE nodes SET
			name = ?,
			protocol = ?,
			server = ?,
			port = ?,
			normalized_payload = ?,
			payload_fingerprint = ?,
			lifecycle_state = ?,
			updated_at = ?
		WHERE id = ?
	`

	res, err := r.db.ExecContext(ctx, query,
		node.Name,
		string(node.CanonicalProtocol()),
		node.Server,
		node.Port,
		marshalJSONSafe(node.NormalizedPayload, "{}"),
		node.PayloadFingerprint,
		string(node.LifecycleState),
		formatTime(node.UpdatedAt),
		node.ID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicate
		}
		return fmt.Errorf("update node: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteNodeRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM nodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteNodeRepository) BatchUpsert(ctx context.Context, nodes []*domain.Node) (int64, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx for batch upsert: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	selectStmt, err := tx.PrepareContext(ctx, "SELECT id, logical_id FROM nodes WHERE payload_fingerprint = ? OR logical_id = ? LIMIT 1")
	if err != nil {
		return 0, fmt.Errorf("prepare select statement: %w", err)
	}
	defer selectStmt.Close()

	updateStmt, err := tx.PrepareContext(ctx, `
		UPDATE nodes SET
			name = ?,
			protocol = ?,
			server = ?,
			port = ?,
			normalized_payload = ?,
			payload_fingerprint = ?,
			lifecycle_state = ?,
			updated_at = ?
		WHERE id = ?
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare update statement: %w", err)
	}
	defer updateStmt.Close()

	insertStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO nodes (
			logical_id, name, protocol, server, port,
			normalized_payload, payload_fingerprint, lifecycle_state,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare insert statement: %w", err)
	}
	defer insertStmt.Close()

	var affected int64
	now := time.Now().UTC()

	for _, node := range nodes {
		if node == nil {
			return 0, ErrNilEntity
		}
		if err := node.Validate(); err != nil {
			return 0, fmt.Errorf("validate node: %w", err)
		}
		r.prepareNodeDefaults(node)

		var existingID int64
		var existingLogicalID string
		err := selectStmt.QueryRowContext(ctx, node.PayloadFingerprint, node.LogicalID).Scan(&existingID, &existingLogicalID)
		if err == nil {
			// Update existing
			node.ID = existingID
			node.LogicalID = existingLogicalID
			_, err = updateStmt.ExecContext(ctx,
				node.Name,
				string(node.CanonicalProtocol()),
				node.Server,
				node.Port,
				marshalJSONSafe(node.NormalizedPayload, "{}"),
				node.PayloadFingerprint,
				string(node.LifecycleState),
				formatTime(now),
				existingID,
			)
			if err != nil {
				return 0, fmt.Errorf("update existing node %s: %w", node.LogicalID, err)
			}
			affected++
		} else if errors.Is(err, sql.ErrNoRows) {
			// Insert new
			res, err := insertStmt.ExecContext(ctx,
				node.LogicalID,
				node.Name,
				string(node.CanonicalProtocol()),
				node.Server,
				node.Port,
				marshalJSONSafe(node.NormalizedPayload, "{}"),
				node.PayloadFingerprint,
				string(node.LifecycleState),
				formatTime(now),
				formatTime(now),
			)
			if err != nil {
				return 0, fmt.Errorf("insert new node %s: %w", node.LogicalID, err)
			}
			id, err := res.LastInsertId()
			if err == nil && id > 0 {
				node.ID = id
			}
			affected++
		} else {
			return 0, fmt.Errorf("check existing node: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit batch upsert tx: %w", err)
	}

	return affected, nil
}

func (r *sqliteNodeRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM nodes").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count nodes: %w", err)
	}
	return count, nil
}

func (r *sqliteNodeRepository) ListFiltered(ctx context.Context, filter NodeFilter) ([]*domain.Node, int64, error) {
	whereClauses := []string{"1=1"}
	var args []any

	if filter.State != "" {
		whereClauses = append(whereClauses, "lifecycle_state = ?")
		args = append(args, string(filter.State))
	}
	if filter.Protocol != "" {
		whereClauses = append(whereClauses, "LOWER(protocol) = LOWER(?)")
		args = append(args, strings.TrimSpace(filter.Protocol))
	}
	if filter.Keyword != "" {
		kw := "%" + strings.TrimSpace(filter.Keyword) + "%"
		whereClauses = append(whereClauses, "(name LIKE ? OR server LIKE ?)")
		args = append(args, kw, kw)
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT count(*) FROM nodes WHERE %s", whereSQL)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count filtered nodes: %w", err)
	}

	query := fmt.Sprintf("SELECT %s FROM nodes WHERE %s ORDER BY id", nodeSelectFields, whereSQL)
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list filtered nodes: %w", err)
	}
	defer rows.Close()

	var result []*domain.Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, node)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate filtered nodes: %w", err)
	}

	return result, total, nil
}

func (r *sqliteNodeRepository) BatchDelete(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf("DELETE FROM nodes WHERE id IN (%s)", strings.Join(placeholders, ","))
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("batch delete nodes: %w", err)
	}
	return res.RowsAffected()
}

