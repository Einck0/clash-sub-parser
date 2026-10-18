package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RecomputeNodeDisplayNames renders source labels without changing raw entries or connection identity.
// Callers pass the affected IDs and prior labels while still inside their write transaction.
func RecomputeNodeDisplayNames(ctx context.Context, tx *sql.Tx, ids []string, priorPrefixes map[string]string, updatedAt string) error {
	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		var current string
		if err := tx.QueryRowContext(ctx, "SELECT display_name FROM nodes WHERE logical_id = ?", id).Scan(&current); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}
		var overrideJSON string
		err := tx.QueryRowContext(ctx, "SELECT override_value_json FROM node_overrides WHERE node_logical_id = ? AND field_path = 'display_name'", id).Scan(&overrideJSON)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		name := current
		if err == nil {
			if err := json.Unmarshal([]byte(overrideJSON), &name); err != nil {
				return fmt.Errorf("invalid display name override: %w", err)
			}
		} else {
			rows, err := tx.QueryContext(ctx, `SELECT s.id, s.prefix,
				(SELECT se.raw_name FROM subscription_entries se
				 WHERE se.subscription_id = s.id AND se.node_logical_id = ns.node_logical_id
				 ORDER BY se.created_at DESC, se.rowid DESC LIMIT 1)
				FROM node_sources ns JOIN subscriptions s ON s.id = ns.subscription_id
				WHERE ns.node_logical_id = ? ORDER BY s.id`, id)
			if err != nil {
				return err
			}
			var prefixes []string
			previous := make(map[string]string, len(priorPrefixes))
			for sourceID, prefix := range priorPrefixes {
				previous[sourceID] = prefix
			}
			var base sql.NullString
			var firstPrefix string
			owners := 0
			labels := make(map[string]bool)
			for rows.Next() {
				var sourceID, prefix string
				var raw sql.NullString
				if err := rows.Scan(&sourceID, &prefix, &raw); err != nil {
					rows.Close()
					return err
				}
				if owners == 0 {
					base = raw
					firstPrefix = prefix
				}
				owners++
				if _, changed := previous[sourceID]; !changed {
					previous[sourceID] = prefix
				}
				if prefix != "" && !labels[prefix] {
					labels[prefix] = true
					prefixes = append(prefixes, prefix)
				}
			}
			rowErr := rows.Err()
			rows.Close()
			if rowErr != nil {
				return rowErr
			}
			if owners == 0 {
				continue // Retained orphan assets keep their last known display label.
			}
			name = current
			var strip []string
			if base.Valid && base.String != "" {
				name = base.String
				strip = []string{firstPrefix}
			} else {
				// Legacy nodes without raw entries: remove only the exact previous labels,
				// in their original stable source order (including a just-deleted owner).
				sourceIDs := make([]string, 0, len(previous))
				for sourceID := range previous {
					sourceIDs = append(sourceIDs, sourceID)
				}
				sort.Strings(sourceIDs)
				stripped := make(map[string]bool)
				for _, sourceID := range sourceIDs {
					prefix := previous[sourceID]
					if prefix != "" && !stripped[prefix] {
						strip = append(strip, prefix)
						stripped[prefix] = true
					}
				}
			}
			for _, prefix := range strip {
				if prefix != "" {
					name = strings.TrimPrefix(name, prefix)
				}
			}
			name = strings.Join(prefixes, "") + name
		}
		if name != current {
			if _, err := tx.ExecContext(ctx, "UPDATE nodes SET display_name = ?, updated_at = ? WHERE logical_id = ?", name, updatedAt, id); err != nil {
				return fmt.Errorf("update source display name: %w", err)
			}
		}
	}
	return nil
}

func subscriptionNodeIDs(ctx context.Context, tx *sql.Tx, subID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT node_logical_id FROM node_sources WHERE subscription_id = ?", subID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
