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

type policyRepository struct {
	db *sql.DB
}

// NewPolicyRepository constructs a SQLite implementation of domain.PolicyRepository.
const timeFormatFixed = "2006-01-02T15:04:05.000000000Z07:00"

func NewPolicyRepository(db *sql.DB) domain.PolicyRepository {
	return &policyRepository{db: db}
}

func (r *policyRepository) GetGroupByID(ctx context.Context, id string) (*domain.NodeGroup, error) {
	const query = `
	SELECT id, name, group_type, created_at, updated_at, empty_fallback_pass, COALESCE(revision, 1), position
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
		&group.EmptyFallbackPass,
		&group.Revision,
		&group.Position,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("node_group_not_found", fmt.Sprintf("node group %s not found", id))
		}
		return nil, fmt.Errorf("failed to query node group %s: %w", id, err)
	}

	group.GroupType = domain.GroupType(groupTypeStr)
	group.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	if group.CreatedAt.IsZero() {
		group.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	}
	group.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	if group.UpdatedAt.IsZero() {
		group.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	}

	sources, err := fetchSourcesForGroup(ctx, r.db, group.ID)
	if err != nil {
		return nil, err
	}
	group.Sources = sources

	return &group, nil
}

func (r *policyRepository) ListGroups(ctx context.Context) ([]domain.NodeGroup, error) {
	const query = `
	SELECT id, name, group_type, created_at, updated_at, empty_fallback_pass, COALESCE(revision, 1), position
	FROM node_groups
	ORDER BY position ASC, created_at ASC, id ASC;`

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
			&group.EmptyFallbackPass,
			&group.Revision,
			&group.Position,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan node group: %w", err)
		}

		group.GroupType = domain.GroupType(groupTypeStr)
		group.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		if group.CreatedAt.IsZero() {
			group.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		}
		group.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		if group.UpdatedAt.IsZero() {
			group.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		}

		items = append(items, group)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node groups: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, err
	}
	allSources, err := fetchAllSources(ctx, r.db)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Sources = allSources[items[i].ID]
	}

	return items, nil
}

func (r *policyRepository) CreateGroup(ctx context.Context, group *domain.NodeGroup) error {
	const query = `
	INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, position)
	VALUES (?, ?, ?, ?, ?, ?, ?);`

	nowStr := domain.NowUTC().Format(timeFormatFixed)
	createdStr := group.CreatedAt.Format(timeFormatFixed)
	if group.CreatedAt.IsZero() {
		createdStr = nowStr
	}
	updatedStr := group.UpdatedAt.Format(timeFormatFixed)
	if group.UpdatedAt.IsZero() {
		updatedStr = nowStr
	}

	_, err := r.db.ExecContext(ctx, query,
		group.ID,
		group.Name,
		string(group.GroupType),
		createdStr,
		updatedStr,
		group.EmptyFallbackPass,
		group.Position,
	)
	if err != nil {
		return fmt.Errorf("failed to insert node group: %w", err)
	}
	return nil
}

func (r *policyRepository) UpdateGroup(ctx context.Context, group *domain.NodeGroup) error {
	return r.SaveGroup(ctx, group, false, nil, false, nil)
}

// SaveGroup uses the existing SQLite transaction boundary for all group components.
func (r *policyRepository) SaveGroup(ctx context.Context, group *domain.NodeGroup, create bool, edges *[]domain.GroupEdge, changeFilter bool, filter *domain.NodeFilterSpec) error {
	if err := filter.Validate(); err != nil {
		return err
	}
	sources := group.Sources
	expected := group.Revision
	if create {
		expected = 0
	} else {
		current, err := r.GetGroupByID(ctx, group.ID)
		if err != nil {
			return err
		}
		if expected == 0 {
			expected = current.Revision
		}
		sources = current.Sources
	}
	if edges != nil || changeFilter {
		var err error
		sources, err = domain.EditLegacySources(group.ID, sources, edges, changeFilter, filter)
		if err != nil {
			return err
		}
	}
	saved, err := r.SaveGroupWithSources(ctx, group, expected, sources)
	if err == nil {
		*group = *saved
	}
	return err
}

func (r *policyRepository) DeleteGroup(ctx context.Context, id string) error {
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM policy_group_sources WHERE target_group_id=? AND group_id<>?)+(SELECT count(*) FROM policy_rules WHERE target_group_id=?)`, id, id, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return domain.NewConflictError("group_in_use", "group is referenced by member sources or routing rules")
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM node_groups WHERE id=?`, id)
		if err != nil {
			return err
		}
		num, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if num == 0 {
			return domain.NewNotFoundError("node_group_not_found", "group not found")
		}
		return nil
	})
}

func (r *policyRepository) ListEdgesByGroup(ctx context.Context, parentGroupID string) ([]domain.GroupEdge, error) {
	sources, err := r.GetSourcesByGroup(ctx, parentGroupID)
	if err != nil {
		return nil, err
	}
	return domain.ProjectSourceEdges(sources), nil
}

func (r *policyRepository) SetEdgesForGroup(ctx context.Context, parentGroupID string, edges []domain.GroupEdge) error {
	group, err := r.GetGroupByID(ctx, parentGroupID)
	if err != nil {
		return err
	}
	return r.SaveGroup(ctx, group, false, &edges, false, nil)
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

func (r *policyRepository) DeleteAdmissionRule(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM admission_rules WHERE id = ?;", id)
	if err != nil {
		return fmt.Errorf("failed to delete admission rule: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.NewNotFoundError("rule_not_found", fmt.Sprintf("admission rule %s not found", id))
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

func (r *policyRepository) DeletePolicyRule(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM policy_rules WHERE id = ?;", id)
	if err != nil {
		return fmt.Errorf("failed to delete policy rule: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.NewNotFoundError("rule_not_found", fmt.Sprintf("policy rule %s not found", id))
	}
	return nil
}

func (r *policyRepository) SaveGroupWithSources(ctx context.Context, group *domain.NodeGroup, expectedRevision int64, sources []domain.MemberSource) (*domain.NodeGroup, error) {
	for i := range sources {
		sources[i].Position = i
		if err := domain.ValidateMemberSource(sources[i]); err != nil {
			return nil, err
		}
	}

	var savedGroup domain.NodeGroup
	err := WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		var currentRev int64 = 1
		var existingID string
		err := tx.QueryRowContext(ctx, `SELECT id, COALESCE(revision, 1) FROM node_groups WHERE id = ?`, group.ID).Scan(&existingID, &currentRev)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to check group revision: %w", err)
		}

		nowStr := domain.NowUTC().Format(timeFormatFixed)
		isNew := errors.Is(err, sql.ErrNoRows)

		if !isNew {
			if expectedRevision > 0 && expectedRevision != currentRev {
				conflictErr := domain.NewConflictError("revision_conflict", fmt.Sprintf("Policy group revision mismatch. Current revision is %d.", currentRev))
				conflictErr.CurrentRevision = &currentRev
				return conflictErr
			}
			currentRev++
			_, err = tx.ExecContext(ctx,
				`UPDATE node_groups SET name = ?, group_type = ?, empty_fallback_pass = ?, revision = ?, position = ?, updated_at = ? WHERE id = ?`,
				group.Name, string(group.GroupType), group.EmptyFallbackPass, currentRev, group.Position, nowStr, group.ID)
			if err != nil {
				return fmt.Errorf("failed to update node group: %w", err)
			}
		} else {
			if expectedRevision > 0 {
				conflictErr := domain.NewConflictError("revision_conflict", fmt.Sprintf("Policy group revision mismatch. Current revision is %d.", 0))
				var zero int64 = 0
				conflictErr.CurrentRevision = &zero
				return conflictErr
			}
			currentRev = 1
			_, err = tx.ExecContext(ctx,
				`INSERT INTO node_groups (id, name, group_type, empty_fallback_pass, revision, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				group.ID, group.Name, string(group.GroupType), group.EmptyFallbackPass, currentRev, group.Position, nowStr, nowStr)
			if err != nil {
				return fmt.Errorf("failed to insert node group: %w", err)
			}
		}

		// Clear existing sources
		if _, err := tx.ExecContext(ctx, `DELETE FROM policy_group_sources WHERE group_id = ?`, group.ID); err != nil {
			return fmt.Errorf("failed to clear existing sources: %w", err)
		}

		// Insert new sources
		const insertSourceQ = `
		INSERT INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, target_group_id, pattern, filter_spec_json, builtin_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

		stmt, err := tx.PrepareContext(ctx, insertSourceQ)
		if err != nil {
			return fmt.Errorf("failed to prepare insert source statement: %w", err)
		}
		defer stmt.Close()

		savedSources := make([]domain.MemberSource, len(sources))
		for i, s := range sources {
			s.GroupID = group.ID
			s.Position = i
			if s.ID == "" {
				newID, idErr := domain.NewUUIDv7()
				if idErr != nil {
					return idErr
				}
				s.ID = newID
			}

			var filterJSON sql.NullString
			if s.FilterSpec != nil {
				bytes, mErr := json.Marshal(s.FilterSpec)
				if mErr != nil {
					return mErr
				}
				filterJSON = sql.NullString{String: string(bytes), Valid: true}
			}

			var nodeID, targetID, pattern, builtin sql.NullString
			if s.NodeLogicalID != nil && *s.NodeLogicalID != "" {
				nodeID = sql.NullString{String: *s.NodeLogicalID, Valid: true}
			}
			if s.TargetGroupID != nil && *s.TargetGroupID != "" {
				targetID = sql.NullString{String: *s.TargetGroupID, Valid: true}
			}
			if s.Pattern != nil && *s.Pattern != "" {
				pattern = sql.NullString{String: *s.Pattern, Valid: true}
			}
			if s.BuiltinName != nil && *s.BuiltinName != "" {
				builtin = sql.NullString{String: *s.BuiltinName, Valid: true}
			}

			s.CreatedAt = domain.NowUTC()
			s.UpdatedAt = s.CreatedAt

			_, err = stmt.ExecContext(ctx,
				s.ID, s.GroupID, string(s.Operation.Normalize()), string(s.Kind.Normalize()), s.Position,
				nodeID, targetID, pattern, filterJSON, builtin, nowStr, nowStr,
			)
			if err != nil {
				return fmt.Errorf("failed to insert source %d: %w", i, err)
			}
			savedSources[i] = s
		}

		savedGroup = *group
		savedGroup.Revision = currentRev
		savedGroup.Sources = savedSources
		savedGroup.UpdatedAt = domain.NowUTC()
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &savedGroup, nil
}

func (r *policyRepository) GetSourcesByGroup(ctx context.Context, groupID string) ([]domain.MemberSource, error) {
	return fetchSourcesForGroup(ctx, r.db, groupID)
}

func (r *policyRepository) ListAllSources(ctx context.Context) (map[string][]domain.MemberSource, error) {
	return fetchAllSources(ctx, r.db)
}

func fetchSourcesForGroup(ctx context.Context, db *sql.DB, groupID string) ([]domain.MemberSource, error) {
	const q = `
	SELECT id, group_id, operation, kind, position, node_logical_id, target_group_id, pattern, filter_spec_json, builtin_name, created_at, updated_at
	FROM policy_group_sources
	WHERE group_id = ?
	ORDER BY position ASC;`

	rows, err := db.QueryContext(ctx, q, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy sources: %w", err)
	}
	defer rows.Close()

	var sources []domain.MemberSource
	for rows.Next() {
		var s domain.MemberSource
		var opStr, kindStr, createdStr, updatedStr string
		var nodeID, targetID, pattern, filterJSON, builtin sql.NullString

		err := rows.Scan(
			&s.ID,
			&s.GroupID,
			&opStr,
			&kindStr,
			&s.Position,
			&nodeID,
			&targetID,
			&pattern,
			&filterJSON,
			&builtin,
			&createdStr,
			&updatedStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member source: %w", err)
		}

		s.Operation = domain.SourceOperation(opStr)
		s.Kind = domain.MemberSourceKind(kindStr)
		if nodeID.Valid {
			s.NodeLogicalID = &nodeID.String
		}
		if targetID.Valid {
			s.TargetGroupID = &targetID.String
		}
		if pattern.Valid {
			s.Pattern = &pattern.String
		}
		if filterJSON.Valid && filterJSON.String != "" {
			var spec domain.NodeFilterSpec
			if err := json.Unmarshal([]byte(filterJSON.String), &spec); err != nil {
				return nil, fmt.Errorf("invalid stored source filter: %w", err)
			}
			s.FilterSpec = &spec
		}
		if builtin.Valid {
			s.BuiltinName = &builtin.String
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		sources = append(sources, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sources, nil
}

func fetchAllSources(ctx context.Context, db *sql.DB) (map[string][]domain.MemberSource, error) {
	const q = `
	SELECT id, group_id, operation, kind, position, node_logical_id, target_group_id, pattern, filter_spec_json, builtin_name, created_at, updated_at
	FROM policy_group_sources
	ORDER BY group_id ASC, position ASC;`

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy sources: %w", err)
	}
	defer rows.Close()

	res := make(map[string][]domain.MemberSource)
	for rows.Next() {
		var s domain.MemberSource
		var opStr, kindStr, createdStr, updatedStr string
		var nodeID, targetID, pattern, filterJSON, builtin sql.NullString

		err := rows.Scan(
			&s.ID,
			&s.GroupID,
			&opStr,
			&kindStr,
			&s.Position,
			&nodeID,
			&targetID,
			&pattern,
			&filterJSON,
			&builtin,
			&createdStr,
			&updatedStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member source: %w", err)
		}

		s.Operation = domain.SourceOperation(opStr)
		s.Kind = domain.MemberSourceKind(kindStr)
		if nodeID.Valid {
			s.NodeLogicalID = &nodeID.String
		}
		if targetID.Valid {
			s.TargetGroupID = &targetID.String
		}
		if pattern.Valid {
			s.Pattern = &pattern.String
		}
		if filterJSON.Valid && filterJSON.String != "" {
			var spec domain.NodeFilterSpec
			if err := json.Unmarshal([]byte(filterJSON.String), &spec); err != nil {
				return nil, fmt.Errorf("invalid stored source filter: %w", err)
			}
			s.FilterSpec = &spec
		}
		if builtin.Valid {
			s.BuiltinName = &builtin.String
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		res[s.GroupID] = append(res[s.GroupID], s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
