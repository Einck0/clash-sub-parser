package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
)

type policyRepository struct {
	db *sql.DB
}

// NewPolicyRepository constructs a SQLite implementation of domain.PolicyRepository.
func NewPolicyRepository(db *sql.DB) domain.PolicyRepository {
	return &policyRepository{db: db}
}

func (r *policyRepository) GetGroupByID(ctx context.Context, id string) (*domain.NodeGroup, error) {
	const query = `
	SELECT id, name, group_type, created_at, updated_at
	FROM node_groups
	WHERE id = ?;`

	var group domain.NodeGroup
	var groupTypeStr, createdStr, updatedStr string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&group.ID,
		&group.Name,
		&groupTypeStr,
		&createdStr,
		&updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("node_group_not_found", fmt.Sprintf("node group %s not found", id))
		}
		return nil, fmt.Errorf("failed to query node group %s: %w", id, err)
	}

	group.GroupType = domain.GroupType(groupTypeStr)
	group.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	group.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &group, nil
}

func (r *policyRepository) ListGroups(ctx context.Context) ([]domain.NodeGroup, error) {
	const query = `
	SELECT id, name, group_type, created_at, updated_at
	FROM node_groups
	ORDER BY name ASC;`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list node groups: %w", err)
	}
	defer rows.Close()

	items := make([]domain.NodeGroup, 0)
	for rows.Next() {
		var group domain.NodeGroup
		var groupTypeStr, createdStr, updatedStr string

		err := rows.Scan(
			&group.ID,
			&group.Name,
			&groupTypeStr,
			&createdStr,
			&updatedStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node group: %w", err)
		}

		group.GroupType = domain.GroupType(groupTypeStr)
		group.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		group.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

		items = append(items, group)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node groups: %w", err)
	}
	return items, nil
}

func (r *policyRepository) CreateGroup(ctx context.Context, group *domain.NodeGroup) error {
	const query = `
	INSERT INTO node_groups (id, name, group_type, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?);`

	nowStr := domain.NowUTC().Format(time.RFC3339)
	createdStr := group.CreatedAt.Format(time.RFC3339)
	if group.CreatedAt.IsZero() {
		createdStr = nowStr
	}
	updatedStr := group.UpdatedAt.Format(time.RFC3339)
	if group.UpdatedAt.IsZero() {
		updatedStr = nowStr
	}

	_, err := r.db.ExecContext(ctx, query,
		group.ID,
		group.Name,
		string(group.GroupType),
		createdStr,
		updatedStr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert node group: %w", err)
	}
	return nil
}

func (r *policyRepository) UpdateGroup(ctx context.Context, group *domain.NodeGroup) error {
	const query = `
	UPDATE node_groups SET name = ?, group_type = ?, updated_at = ?
	WHERE id = ?;`

	updatedStr := domain.NowUTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx, query,
		group.Name,
		string(group.GroupType),
		updatedStr,
		group.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update node group: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.NewNotFoundError("node_group_not_found", fmt.Sprintf("node group %s not found", group.ID))
	}
	return nil
}

func (r *policyRepository) DeleteGroup(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM node_groups WHERE id = ?;", id)
	if err != nil {
		return fmt.Errorf("failed to delete node group: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.NewNotFoundError("node_group_not_found", fmt.Sprintf("node group %s not found", id))
	}
	return nil
}

func (r *policyRepository) ListEdgesByGroup(ctx context.Context, parentGroupID string) ([]domain.GroupEdge, error) {
	const query = `
	SELECT id, parent_group_id, child_group_id, node_logical_id, position
	FROM group_edges
	WHERE parent_group_id = ?
	ORDER BY position ASC;`

	rows, err := r.db.QueryContext(ctx, query, parentGroupID)
	if err != nil {
		return nil, fmt.Errorf("failed to query edges for parent group %s: %w", parentGroupID, err)
	}
	defer rows.Close()

	items := make([]domain.GroupEdge, 0)
	for rows.Next() {
		var edge domain.GroupEdge
		var childID, nodeID sql.NullString

		err := rows.Scan(
			&edge.ID,
			&edge.ParentGroupID,
			&childID,
			&nodeID,
			&edge.Position,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan group edge: %w", err)
		}

		if childID.Valid && childID.String != "" {
			edge.ChildGroupID = &childID.String
		}
		if nodeID.Valid && nodeID.String != "" {
			edge.NodeLogicalID = &nodeID.String
		}

		items = append(items, edge)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating group edges: %w", err)
	}
	return items, nil
}

func (r *policyRepository) SetEdgesForGroup(ctx context.Context, parentGroupID string, edges []domain.GroupEdge) error {
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		// Delete existing edges for this parent group
		if _, err := tx.ExecContext(ctx, "DELETE FROM group_edges WHERE parent_group_id = ?;", parentGroupID); err != nil {
			return fmt.Errorf("failed to clear existing edges: %w", err)
		}

		const insertQuery = `
		INSERT INTO group_edges (id, parent_group_id, child_group_id, node_logical_id, position)
		VALUES (?, ?, ?, ?, ?);`

		stmt, err := tx.PrepareContext(ctx, insertQuery)
		if err != nil {
			return fmt.Errorf("failed to prepare insert group edge: %w", err)
		}
		defer stmt.Close()

		for i, edge := range edges {
			edge.ParentGroupID = parentGroupID
			edge.Position = i
			if err := domain.ValidateGroupEdge(edge); err != nil {
				return err
			}

			var childStr, nodeStr sql.NullString
			if edge.ChildGroupID != nil && *edge.ChildGroupID != "" {
				childStr = sql.NullString{String: *edge.ChildGroupID, Valid: true}
			}
			if edge.NodeLogicalID != nil && *edge.NodeLogicalID != "" {
				nodeStr = sql.NullString{String: *edge.NodeLogicalID, Valid: true}
			}

			id := edge.ID
			if id == "" {
				newID, idErr := domain.NewUUIDv7()
				if idErr != nil {
					return idErr
				}
				id = newID
			}

			_, err := stmt.ExecContext(ctx, id, parentGroupID, childStr, nodeStr, edge.Position)
			if err != nil {
				return fmt.Errorf("failed to insert group edge: %w", err)
			}
		}

		return nil
	})
}

func (r *policyRepository) ListAdmissionRules(ctx context.Context, revisionID string) ([]domain.AdmissionRule, error) {
	const query = `
	SELECT id, revision_id, name, expression, action, position
	FROM admission_rules
	WHERE revision_id = ?
	ORDER BY position ASC;`

	rows, err := r.db.QueryContext(ctx, query, revisionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list admission rules: %w", err)
	}
	defer rows.Close()

	items := make([]domain.AdmissionRule, 0)
	for rows.Next() {
		var rule domain.AdmissionRule
		var actionStr string

		err := rows.Scan(
			&rule.ID,
			&rule.RevisionID,
			&rule.Name,
			&rule.Expression,
			&actionStr,
			&rule.Position,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan admission rule: %w", err)
		}

		rule.Action = domain.RuleAction(actionStr)
		items = append(items, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating admission rules: %w", err)
	}
	return items, nil
}

func (r *policyRepository) CreateAdmissionRule(ctx context.Context, rule *domain.AdmissionRule) error {
	const query = `
	INSERT INTO admission_rules (id, revision_id, name, expression, action, position)
	VALUES (?, ?, ?, ?, ?, ?);`

	_, err := r.db.ExecContext(ctx, query,
		rule.ID,
		rule.RevisionID,
		rule.Name,
		rule.Expression,
		string(rule.Action),
		rule.Position,
	)
	if err != nil {
		return fmt.Errorf("failed to insert admission rule: %w", err)
	}
	return nil
}

func (r *policyRepository) ListPolicyRules(ctx context.Context, revisionID string) ([]domain.PolicyRule, error) {
	const query = `
	SELECT id, revision_id, target_group_id, expression, position
	FROM policy_rules
	WHERE revision_id = ?
	ORDER BY position ASC;`

	rows, err := r.db.QueryContext(ctx, query, revisionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list policy rules: %w", err)
	}
	defer rows.Close()

	items := make([]domain.PolicyRule, 0)
	for rows.Next() {
		var rule domain.PolicyRule

		err := rows.Scan(
			&rule.ID,
			&rule.RevisionID,
			&rule.TargetGroupID,
			&rule.Expression,
			&rule.Position,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan policy rule: %w", err)
		}

		items = append(items, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating policy rules: %w", err)
	}
	return items, nil
}

func (r *policyRepository) CreatePolicyRule(ctx context.Context, rule *domain.PolicyRule) error {
	const query = `
	INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
	VALUES (?, ?, ?, ?, ?);`

	_, err := r.db.ExecContext(ctx, query,
		rule.ID,
		rule.RevisionID,
		rule.TargetGroupID,
		rule.Expression,
		rule.Position,
	)
	if err != nil {
		return fmt.Errorf("failed to insert policy rule: %w", err)
	}
	return nil
}
