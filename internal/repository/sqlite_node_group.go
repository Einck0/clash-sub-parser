package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"clash-sub-parser/internal/domain"
)

type sqliteNodeGroupRepository struct {
	db *SQLiteDB
}

func scanNodeGroup(s rowScanner) (*domain.NodeGroup, error) {
	var (
		group                                              domain.NodeGroup
		kindStr, groupTypeStr                              string
		sortOrder                                          int64
		regexRulesRaw, filterMediaUnlockRaw                sql.NullString
		filterMinSpeed                                     sql.NullFloat64
		includeNodesRaw, includeGroupIDsRaw                sql.NullString
		includeGroupNodesIDsRaw, includeEntriesRaw         sql.NullString
		addFallbackInt                                     int64
		excludeNodesRaw, excludeGroupIDsRaw                sql.NullString
		urlTestConfigRaw, loadBalanceConfigRaw, fallbackConfigRaw sql.NullString
	)

	err := s.Scan(
		&group.ID,
		&group.Name,
		&kindStr,
		&groupTypeStr,
		&sortOrder,
		&regexRulesRaw,
		&filterMinSpeed,
		&filterMediaUnlockRaw,
		&includeNodesRaw,
		&includeGroupIDsRaw,
		&includeGroupNodesIDsRaw,
		&includeEntriesRaw,
		&addFallbackInt,
		&excludeNodesRaw,
		&excludeGroupIDsRaw,
		&urlTestConfigRaw,
		&loadBalanceConfigRaw,
		&fallbackConfigRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan node group: %w", err)
	}

	group.Kind = domain.GroupKind(kindStr)
	group.GroupType = domain.GroupType(groupTypeStr)
	group.SortOrder = int(sortOrder)
	group.FilterMinSpeedMbps = float64PtrFromNull(filterMinSpeed)
	group.AddFallback = intToBool(addFallbackInt)

	// JSON unmarshals
	_ = unmarshalJSONSafe(stringFromNull(regexRulesRaw), &group.RegexRules)
	_ = unmarshalJSONSafe(stringFromNull(filterMediaUnlockRaw), &group.FilterMediaUnlock)
	_ = unmarshalJSONSafe(stringFromNull(includeNodesRaw), &group.IncludeNodes)
	_ = unmarshalJSONSafe(stringFromNull(includeGroupIDsRaw), &group.IncludeGroupIDs)
	_ = unmarshalJSONSafe(stringFromNull(includeGroupNodesIDsRaw), &group.IncludeGroupNodesIDs)
	_ = unmarshalJSONSafe(stringFromNull(includeEntriesRaw), &group.IncludeEntries)
	_ = unmarshalJSONSafe(stringFromNull(excludeNodesRaw), &group.ExcludeNodes)
	_ = unmarshalJSONSafe(stringFromNull(excludeGroupIDsRaw), &group.ExcludeGroupIDs)
	_ = unmarshalJSONSafe(stringFromNull(urlTestConfigRaw), &group.URLTestConfig)
	_ = unmarshalJSONSafe(stringFromNull(loadBalanceConfigRaw), &group.LoadBalanceConfig)
	_ = unmarshalJSONSafe(stringFromNull(fallbackConfigRaw), &group.FallbackConfig)

	return &group, nil
}

const nodeGroupSelectFields = `
	id, name, kind, group_type, sort_order,
	regex_rules, filter_min_speed_mbps, filter_media_unlock,
	include_nodes, include_group_ids, include_group_nodes_ids, include_entries,
	add_fallback, exclude_nodes, exclude_group_ids,
	url_test_config, load_balance_config, fallback_config
`

func (r *sqliteNodeGroupRepository) GetByID(ctx context.Context, id int64) (*domain.NodeGroup, error) {
	query := fmt.Sprintf("SELECT %s FROM node_groups WHERE id = ?", nodeGroupSelectFields)
	row := r.db.QueryRowContext(ctx, query, id)
	return scanNodeGroup(row)
}

func (r *sqliteNodeGroupRepository) GetByName(ctx context.Context, name string) (*domain.NodeGroup, error) {
	query := fmt.Sprintf("SELECT %s FROM node_groups WHERE name = ?", nodeGroupSelectFields)
	row := r.db.QueryRowContext(ctx, query, name)
	return scanNodeGroup(row)
}

func (r *sqliteNodeGroupRepository) List(ctx context.Context) ([]*domain.NodeGroup, error) {
	query := fmt.Sprintf("SELECT %s FROM node_groups ORDER BY sort_order, id", nodeGroupSelectFields)
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list node groups: %w", err)
	}
	defer rows.Close()

	var result []*domain.NodeGroup
	for rows.Next() {
		g, err := scanNodeGroup(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node groups: %w", err)
	}
	return result, nil
}

func (r *sqliteNodeGroupRepository) Create(ctx context.Context, group *domain.NodeGroup) error {
	if group == nil {
		return ErrNilEntity
	}

	if err := group.Validate(); err != nil {
		return fmt.Errorf("validate node group: %w", err)
	}

	query := `
		INSERT INTO node_groups (
			name, kind, group_type, sort_order,
			regex_rules, filter_min_speed_mbps, filter_media_unlock,
			include_nodes, include_group_ids, include_group_nodes_ids, include_entries,
			add_fallback, exclude_nodes, exclude_group_ids,
			url_test_config, load_balance_config, fallback_config
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := r.db.ExecContext(ctx, query,
		group.Name,
		string(group.Kind),
		string(group.GroupType),
		group.SortOrder,
		marshalJSONSafe(group.RegexRules, "[]"),
		nullFloat64FromPtr(group.FilterMinSpeedMbps),
		marshalJSONSafe(group.FilterMediaUnlock, "[]"),
		marshalJSONSafe(group.IncludeNodes, "[]"),
		marshalJSONSafe(group.IncludeGroupIDs, "[]"),
		marshalJSONSafe(group.IncludeGroupNodesIDs, "[]"),
		marshalJSONSafe(group.IncludeEntries, "[]"),
		boolToInt(group.AddFallback),
		marshalJSONSafe(group.ExcludeNodes, "[]"),
		marshalJSONSafe(group.ExcludeGroupIDs, "[]"),
		marshalJSONSafe(group.URLTestConfig, "{}"),
		marshalJSONSafe(group.LoadBalanceConfig, "{}"),
		marshalJSONSafe(group.FallbackConfig, "{}"),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicate
		}
		return fmt.Errorf("create node group: %w", err)
	}

	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		group.ID = id
	}
	return nil
}

func (r *sqliteNodeGroupRepository) Update(ctx context.Context, group *domain.NodeGroup) error {
	if group == nil {
		return ErrNilEntity
	}

	if err := group.Validate(); err != nil {
		return fmt.Errorf("validate node group: %w", err)
	}

	query := `
		UPDATE node_groups SET
			name = ?,
			kind = ?,
			group_type = ?,
			sort_order = ?,
			regex_rules = ?,
			filter_min_speed_mbps = ?,
			filter_media_unlock = ?,
			include_nodes = ?,
			include_group_ids = ?,
			include_group_nodes_ids = ?,
			include_entries = ?,
			add_fallback = ?,
			exclude_nodes = ?,
			exclude_group_ids = ?,
			url_test_config = ?,
			load_balance_config = ?,
			fallback_config = ?
		WHERE id = ?
	`

	res, err := r.db.ExecContext(ctx, query,
		group.Name,
		string(group.Kind),
		string(group.GroupType),
		group.SortOrder,
		marshalJSONSafe(group.RegexRules, "[]"),
		nullFloat64FromPtr(group.FilterMinSpeedMbps),
		marshalJSONSafe(group.FilterMediaUnlock, "[]"),
		marshalJSONSafe(group.IncludeNodes, "[]"),
		marshalJSONSafe(group.IncludeGroupIDs, "[]"),
		marshalJSONSafe(group.IncludeGroupNodesIDs, "[]"),
		marshalJSONSafe(group.IncludeEntries, "[]"),
		boolToInt(group.AddFallback),
		marshalJSONSafe(group.ExcludeNodes, "[]"),
		marshalJSONSafe(group.ExcludeGroupIDs, "[]"),
		marshalJSONSafe(group.URLTestConfig, "{}"),
		marshalJSONSafe(group.LoadBalanceConfig, "{}"),
		marshalJSONSafe(group.FallbackConfig, "{}"),
		group.ID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicate
		}
		return fmt.Errorf("update node group: %w", err)
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

func (r *sqliteNodeGroupRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM node_groups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete node group: %w", err)
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

func (r *sqliteNodeGroupRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM node_groups").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count node groups: %w", err)
	}
	return count, nil
}
