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

// CapturePolicyRevisionSnapshot binds the immutable graph and digest in one write transaction.
func CapturePolicyRevisionSnapshot(ctx context.Context, tx *sql.Tx, revisionID string) error {
	return withPolicyGroups(ctx, tx, func(groups []domain.NodeGroup) error {
		data, err := json.Marshal(groups)
		if err != nil {
			return err
		}
		digest, err := policyGroupsDigest(groups)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO policy_revision_snapshots(revision_id,groups_json) VALUES(?,?)`, revisionID, string(data)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE configuration_revisions SET content_digest=? WHERE id=?`, digest, revisionID)
		return err
	})
}

func policyGroupsDigest(groups []domain.NodeGroup) (string, error) {
	edges := make(map[string][]domain.GroupEdge, len(groups))
	for _, g := range groups {
		edges[g.ID] = domain.ProjectSourceEdges(g.Sources)
	}
	return domain.ComputeConfigurationDigest(groups, edges)
}

func withPolicyGroups(ctx context.Context, tx *sql.Tx, use func([]domain.NodeGroup) error) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,name,group_type,empty_fallback_pass,revision FROM node_groups ORDER BY id`)
	if err != nil {
		return err
	}
	groups := make([]domain.NodeGroup, 0)
	for rows.Next() {
		var g domain.NodeGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.GroupType, &g.EmptyFallbackPass, &g.Revision); err != nil {
			rows.Close()
			return err
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT id,group_id,operation,kind,position,node_logical_id,target_group_id,pattern,filter_spec_json,builtin_name FROM policy_group_sources ORDER BY group_id,position,id`)
	if err != nil {
		return err
	}
	byID := make(map[string]int, len(groups))
	for i, g := range groups {
		byID[g.ID] = i
	}
	for rows.Next() {
		var s domain.MemberSource
		var filter sql.NullString
		if err := rows.Scan(&s.ID, &s.GroupID, &s.Operation, &s.Kind, &s.Position, &s.NodeLogicalID, &s.TargetGroupID, &s.Pattern, &filter, &s.BuiltinName); err != nil {
			rows.Close()
			return err
		}
		if filter.Valid {
			if err := json.Unmarshal([]byte(filter.String), &s.FilterSpec); err != nil {
				rows.Close()
				return err
			}
		}
		i, ok := byID[s.GroupID]
		if !ok {
			rows.Close()
			return fmt.Errorf("orphan policy source")
		}
		groups[i].Sources = append(groups[i].Sources, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	return use(groups)
}

func (r *revisionRepository) GetPolicySnapshot(ctx context.Context, revisionID string) ([]domain.NodeGroup, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT groups_json FROM policy_revision_snapshots WHERE revision_id=?`, revisionID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.NewConflictError("revision_snapshot_unavailable", "historical revision has no policy sources snapshot")
	}
	if err != nil {
		return nil, err
	}
	var groups []domain.NodeGroup
	if err := json.Unmarshal([]byte(data), &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// RestorePolicySnapshot requires the complete current revision vector; deletions,
// additions and intervening edits all conflict before any graph write.
func (r *revisionRepository) RestorePolicySnapshot(ctx context.Context, revisionID string, expected map[string]int64) error {
	groups, err := r.GetPolicySnapshot(ctx, revisionID)
	if err != nil {
		return err
	}
	if err := domain.ValidatePolicyTopology(groups, nil, nil, false); err != nil {
		return err
	}
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id,revision FROM node_groups`)
		if err != nil {
			return err
		}
		current := map[string]int64{}
		for rows.Next() {
			var id string
			var rev int64
			if err := rows.Scan(&id, &rev); err != nil {
				rows.Close()
				return err
			}
			current[id] = rev
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(current) != len(expected) {
			return domain.NewConflictError("revision_conflict", "policy group set changed; reload before rollback")
		}
		for id, rev := range current {
			if want, ok := expected[id]; !ok || want != rev {
				e := domain.NewConflictError("revision_conflict", "policy group changed; reload before rollback")
				e.CurrentRevision = &rev
				return e
			}
		}
		now := domain.NowUTC().Format(time.RFC3339)
		// Removing sources first permits restoring mutually referenced groups in any order.
		if _, err = tx.ExecContext(ctx, `DELETE FROM policy_group_sources`); err != nil {
			return err
		}
		keep := map[string]bool{}
		for _, g := range groups {
			keep[g.ID] = true
			next := current[g.ID] + 1
			if next <= g.Revision {
				next = g.Revision + 1
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO node_groups(id,name,group_type,empty_fallback_pass,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,group_type=excluded.group_type,empty_fallback_pass=excluded.empty_fallback_pass,revision=excluded.revision,updated_at=excluded.updated_at`, g.ID, g.Name, g.GroupType, g.EmptyFallbackPass, next, now, now); err != nil {
				return err
			}
		}
		// Never cascade-delete live routing rules as a side effect of rollback.
		for id := range current {
			if !keep[id] {
				var n int
				if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM policy_rules WHERE target_group_id=?`, id).Scan(&n); err != nil {
					return err
				}
				if n > 0 {
					return domain.NewConflictError("group_in_use", "rollback would remove a routing rule target")
				}
				if _, err = tx.ExecContext(ctx, `DELETE FROM node_groups WHERE id=?`, id); err != nil {
					return err
				}
			}
		}
		for _, g := range groups {
			for _, s := range g.Sources {
				var filter any
				if s.FilterSpec != nil {
					b, e := json.Marshal(s.FilterSpec)
					if e != nil {
						return e
					}
					filter = string(b)
				}
				if _, err = tx.ExecContext(ctx, `INSERT INTO policy_group_sources(id,group_id,operation,kind,position,node_logical_id,target_group_id,pattern,filter_spec_json,builtin_name,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, s.ID, g.ID, s.Operation, s.Kind, s.Position, s.NodeLogicalID, s.TargetGroupID, s.Pattern, filter, s.BuiltinName, now, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
