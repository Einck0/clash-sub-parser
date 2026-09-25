package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
)

type nodeFilterRepository struct {
	db *sql.DB
}

// NewNodeFilterRepository constructs a SQLite implementation of domain.NodeFilterRepository.
func NewNodeFilterRepository(db *sql.DB) domain.NodeFilterRepository {
	return &nodeFilterRepository{db: db}
}

func (r *nodeFilterRepository) GetGlobalFilter(ctx context.Context) (*domain.GlobalNodeFilter, error) {
	const query = `
	SELECT filter_spec, updated_at
	FROM global_node_filters
	WHERE id = 1;`

	var specJSON, updatedAtStr string
	err := r.db.QueryRowContext(ctx, query).Scan(&specJSON, &updatedAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return default empty filter if row not present
			return &domain.GlobalNodeFilter{
				Spec:      domain.NodeFilterSpec{Conditions: []domain.FilterCondition{}},
				UpdatedAt: domain.NowUTC(),
			}, nil
		}
		return nil, fmt.Errorf("failed to query global node filter: %w", err)
	}

	var spec domain.NodeFilterSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal global node filter spec: %w", err)
	}

	updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		// Fallback to SQLite datetime format or now
		if parsed, pErr := time.Parse("2006-01-02 15:04:05", updatedAtStr); pErr == nil {
			updatedAt = parsed
		} else {
			updatedAt = domain.NowUTC()
		}
	}

	return &domain.GlobalNodeFilter{
		Spec:      spec,
		UpdatedAt: updatedAt,
	}, nil
}

func (r *nodeFilterRepository) SetGlobalFilter(ctx context.Context, filter *domain.GlobalNodeFilter) error {
	if filter == nil {
		return domain.NewValidationError("invalid_filter", "global node filter cannot be nil")
	}

	if err := filter.Spec.Validate(); err != nil {
		return err
	}

	specJSON, err := json.Marshal(filter.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal global node filter spec: %w", err)
	}

	now := domain.NowUTC()
	if !filter.UpdatedAt.IsZero() {
		now = filter.UpdatedAt
	}

	const query = `
	INSERT INTO global_node_filters (id, filter_spec, updated_at)
	VALUES (1, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		filter_spec = excluded.filter_spec,
		updated_at = excluded.updated_at;`

	_, err = r.db.ExecContext(ctx, query, string(specJSON), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to upsert global node filter: %w", err)
	}
	return nil
}

func (r *nodeFilterRepository) GetGroupFilter(ctx context.Context, groupID string) (*domain.GroupNodeFilter, error) {
	if groupID == "" {
		return nil, domain.NewValidationError("invalid_group_id", "group_id cannot be empty")
	}

	const query = `
	SELECT group_id, filter_spec, updated_at
	FROM group_node_filters
	WHERE group_id = ?;`

	var gid, specJSON, updatedAtStr string
	err := r.db.QueryRowContext(ctx, query, groupID).Scan(&gid, &specJSON, &updatedAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("group_filter_not_found", fmt.Sprintf("no node filter found for group %s", groupID))
		}
		return nil, fmt.Errorf("failed to query group node filter for %s: %w", groupID, err)
	}

	var spec domain.NodeFilterSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal group node filter spec: %w", err)
	}

	updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		if parsed, pErr := time.Parse("2006-01-02 15:04:05", updatedAtStr); pErr == nil {
			updatedAt = parsed
		} else {
			updatedAt = domain.NowUTC()
		}
	}

	return &domain.GroupNodeFilter{
		GroupID:   gid,
		Spec:      spec,
		UpdatedAt: updatedAt,
	}, nil
}

func (r *nodeFilterRepository) SetGroupFilter(ctx context.Context, filter *domain.GroupNodeFilter) error {
	if filter == nil {
		return domain.NewValidationError("invalid_filter", "group node filter cannot be nil")
	}
	if filter.GroupID == "" {
		return domain.NewValidationError("invalid_group_id", "group_id cannot be empty")
	}

	if err := filter.Spec.Validate(); err != nil {
		return err
	}

	specJSON, err := json.Marshal(filter.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal group node filter spec: %w", err)
	}

	now := domain.NowUTC()
	if !filter.UpdatedAt.IsZero() {
		now = filter.UpdatedAt
	}

	const query = `
	INSERT INTO group_node_filters (group_id, filter_spec, updated_at)
	VALUES (?, ?, ?)
	ON CONFLICT(group_id) DO UPDATE SET
		filter_spec = excluded.filter_spec,
		updated_at = excluded.updated_at;`

	_, err = r.db.ExecContext(ctx, query, filter.GroupID, string(specJSON), now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to upsert group node filter for %s: %w", filter.GroupID, err)
	}
	return nil
}

func (r *nodeFilterRepository) DeleteGroupFilter(ctx context.Context, groupID string) error {
	if groupID == "" {
		return domain.NewValidationError("invalid_group_id", "group_id cannot be empty")
	}

	const query = `DELETE FROM group_node_filters WHERE group_id = ?;`
	_, err := r.db.ExecContext(ctx, query, groupID)
	if err != nil {
		return fmt.Errorf("failed to delete group node filter for %s: %w", groupID, err)
	}
	return nil
}

func (r *nodeFilterRepository) ListGroupFilters(ctx context.Context) (map[string]domain.NodeFilterSpec, error) {
	const query = `
	SELECT group_id, filter_spec
	FROM group_node_filters;`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list group node filters: %w", err)
	}
	defer rows.Close()

	result := make(map[string]domain.NodeFilterSpec)
	for rows.Next() {
		var gid, specJSON string
		if err := rows.Scan(&gid, &specJSON); err != nil {
			return nil, fmt.Errorf("failed to scan group node filter: %w", err)
		}
		var spec domain.NodeFilterSpec
		if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal group node filter spec for %s: %w", gid, err)
		}
		result[gid] = spec
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating group node filters: %w", err)
	}

	return result, nil
}
