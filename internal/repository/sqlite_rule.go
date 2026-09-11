package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"clash-sub-parser/internal/domain"
)

type sqliteRuleRepository struct {
	db *SQLiteDB
}

func scanRule(s rowScanner) (*domain.Rule, error) {
	var (
		rule         domain.Rule
		typeStr      string
		optionsRaw   sql.NullString
		sortOrder    int64
		enabledInt   int64
	)

	err := s.Scan(
		&rule.ID,
		&rule.Name,
		&rule.Category,
		&typeStr,
		&rule.Value,
		&rule.Proxy,
		&optionsRaw,
		&sortOrder,
		&enabledInt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan rule: %w", err)
	}

	rule.Type = domain.RuleType(typeStr)
	rule.SortOrder = int(sortOrder)
	rule.Enabled = intToBool(enabledInt)

	_ = unmarshalJSONSafe(stringFromNull(optionsRaw), &rule.Options)

	return &rule, nil
}

const ruleSelectFields = `id, name, category, type, value, proxy, options, sort_order, enabled`

func (r *sqliteRuleRepository) GetByID(ctx context.Context, id int64) (*domain.Rule, error) {
	query := fmt.Sprintf("SELECT %s FROM rules WHERE id = ?", ruleSelectFields)
	row := r.db.QueryRowContext(ctx, query, id)
	return scanRule(row)
}

func (r *sqliteRuleRepository) List(ctx context.Context, enabledOnly bool) ([]*domain.Rule, error) {
	query := fmt.Sprintf("SELECT %s FROM rules", ruleSelectFields)
	var args []any
	if enabledOnly {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY sort_order, id"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	var result []*domain.Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	return result, nil
}

func (r *sqliteRuleRepository) ListByCategory(ctx context.Context, category string) ([]*domain.Rule, error) {
	query := fmt.Sprintf("SELECT %s FROM rules WHERE category = ? ORDER BY sort_order, id", ruleSelectFields)
	rows, err := r.db.QueryContext(ctx, query, category)
	if err != nil {
		return nil, fmt.Errorf("list rules by category: %w", err)
	}
	defer rows.Close()

	var result []*domain.Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules by category: %w", err)
	}
	return result, nil
}

func (r *sqliteRuleRepository) Create(ctx context.Context, rule *domain.Rule) error {
	if rule == nil {
		return ErrNilEntity
	}

	if err := rule.Validate(); err != nil {
		return fmt.Errorf("validate rule: %w", err)
	}

	query := `
		INSERT INTO rules (name, category, type, value, proxy, options, sort_order, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := r.db.ExecContext(ctx, query,
		rule.Name,
		rule.Category,
		string(rule.Type),
		rule.Value,
		rule.Proxy,
		marshalJSONSafe(rule.Options, "[]"),
		rule.SortOrder,
		boolToInt(rule.Enabled),
	)
	if err != nil {
		return fmt.Errorf("create rule: %w", err)
	}

	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		rule.ID = id
	}
	return nil
}

func (r *sqliteRuleRepository) Update(ctx context.Context, rule *domain.Rule) error {
	if rule == nil {
		return ErrNilEntity
	}

	if err := rule.Validate(); err != nil {
		return fmt.Errorf("validate rule: %w", err)
	}

	query := `
		UPDATE rules SET
			name = ?,
			category = ?,
			type = ?,
			value = ?,
			proxy = ?,
			options = ?,
			sort_order = ?,
			enabled = ?
		WHERE id = ?
	`

	res, err := r.db.ExecContext(ctx, query,
		rule.Name,
		rule.Category,
		string(rule.Type),
		rule.Value,
		rule.Proxy,
		marshalJSONSafe(rule.Options, "[]"),
		rule.SortOrder,
		boolToInt(rule.Enabled),
		rule.ID,
	)
	if err != nil {
		return fmt.Errorf("update rule: %w", err)
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

func (r *sqliteRuleRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM rules WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
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

func (r *sqliteRuleRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM rules").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count rules: %w", err)
	}
	return count, nil
}
