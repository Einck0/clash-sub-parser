package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

type nodeSourceHistoryRepository struct {
	db *sql.DB
}

// NewNodeSourceHistoryRepository creates a SQLite implementation of domain.NodeSourceHistoryRepository.
func NewNodeSourceHistoryRepository(db *sql.DB) domain.NodeSourceHistoryRepository {
	return &nodeSourceHistoryRepository{db: db}
}

func parseFlexibleTimestamp(s string) (*time.Time, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, nil
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, trimmed); err == nil {
			tUTC := t.UTC()
			return &tUTC, nil
		}
	}
	return nil, fmt.Errorf("unable to parse timestamp: %s", s)
}

func formatTimestampOrNull(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func (r *nodeSourceHistoryRepository) InsertBatch(ctx context.Context, records []domain.NodeSourceHistory) error {
	if len(records) == 0 {
		return nil
	}

	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		const query = `
		INSERT INTO node_source_history (
			id,
			node_logical_id,
			subscription_id,
			source_identity,
			source_label,
			connection_revision,
			relation_state,
			cause,
			first_observed_at,
			last_observed_at,
			evidence_kind,
			evidence_key,
			evidence_json,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_logical_id, evidence_key) DO UPDATE SET
			last_observed_at = COALESCE(excluded.last_observed_at, node_source_history.last_observed_at);`

		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to prepare insert node_source_history statement: %w", err)
		}
		defer stmt.Close()

		for _, rec := range records {
			if err := rec.Validate(); err != nil {
				return fmt.Errorf("validation error for node_source_history (node=%s, key=%s): %w", rec.NodeLogicalID, rec.EvidenceKey, err)
			}

			id := rec.ID
			if strings.TrimSpace(id) == "" {
				id = domain.MustNewUUIDv7()
			}

			var subID any = nil
			if rec.SubscriptionID != nil && strings.TrimSpace(*rec.SubscriptionID) != "" {
				subID = strings.TrimSpace(*rec.SubscriptionID)
			}

			var connRev any = nil
			if rec.ConnectionRevision != nil {
				connRev = *rec.ConnectionRevision
			}

			firstObs := formatTimestampOrNull(rec.FirstObservedAt)
			lastObs := formatTimestampOrNull(rec.LastObservedAt)

			createdAtStr := rec.CreatedAt.UTC().Format(time.RFC3339)
			if rec.CreatedAt.IsZero() {
				createdAtStr = domain.NowUTC().Format(time.RFC3339)
			}

			evidenceJSON := rec.EvidenceJSON
			if strings.TrimSpace(evidenceJSON) == "" {
				evidenceJSON = "{}"
			}

			if _, err := stmt.ExecContext(ctx,
				id,
				rec.NodeLogicalID,
				subID,
				rec.SourceIdentity,
				rec.SourceLabel,
				connRev,
				string(rec.RelationState),
				string(rec.Cause),
				firstObs,
				lastObs,
				rec.EvidenceKind,
				rec.EvidenceKey,
				evidenceJSON,
				createdAtStr,
			); err != nil {
				return fmt.Errorf("failed to execute insert node_source_history (node=%s, key=%s): %w", rec.NodeLogicalID, rec.EvidenceKey, err)
			}
		}

		return nil
	})
}

func (r *nodeSourceHistoryRepository) ListByNodeLogicalID(ctx context.Context, nodeLogicalID string) ([]domain.NodeSourceHistory, error) {
	const query = `
	SELECT
		id,
		node_logical_id,
		subscription_id,
		source_identity,
		source_label,
		connection_revision,
		relation_state,
		cause,
		first_observed_at,
		last_observed_at,
		evidence_kind,
		evidence_key,
		evidence_json,
		created_at
	FROM node_source_history
	WHERE node_logical_id = ?
	ORDER BY COALESCE(last_observed_at, first_observed_at, created_at) DESC, id DESC;`

	rows, err := r.db.QueryContext(ctx, query, strings.TrimSpace(nodeLogicalID))
	if err != nil {
		return nil, fmt.Errorf("failed to query node_source_history by node: %w", err)
	}
	defer rows.Close()

	return scanHistoryRows(rows)
}

func (r *nodeSourceHistoryRepository) ListBySubscriptionID(ctx context.Context, subscriptionID string) ([]domain.NodeSourceHistory, error) {
	const query = `
	SELECT
		id,
		node_logical_id,
		subscription_id,
		source_identity,
		source_label,
		connection_revision,
		relation_state,
		cause,
		first_observed_at,
		last_observed_at,
		evidence_kind,
		evidence_key,
		evidence_json,
		created_at
	FROM node_source_history
	WHERE subscription_id = ?
	ORDER BY COALESCE(last_observed_at, first_observed_at, created_at) DESC, id DESC;`

	rows, err := r.db.QueryContext(ctx, query, strings.TrimSpace(subscriptionID))
	if err != nil {
		return nil, fmt.Errorf("failed to query node_source_history by subscription: %w", err)
	}
	defer rows.Close()

	return scanHistoryRows(rows)
}

func scanHistoryRows(rows *sql.Rows) ([]domain.NodeSourceHistory, error) {
	var list []domain.NodeSourceHistory

	for rows.Next() {
		var (
			h            domain.NodeSourceHistory
			subID        sql.NullString
			connRev      sql.NullInt64
			relState     string
			cause        string
			firstObsStr  sql.NullString
			lastObsStr   sql.NullString
			createdAtStr string
		)

		if err := rows.Scan(
			&h.ID,
			&h.NodeLogicalID,
			&subID,
			&h.SourceIdentity,
			&h.SourceLabel,
			&connRev,
			&relState,
			&cause,
			&firstObsStr,
			&lastObsStr,
			&h.EvidenceKind,
			&h.EvidenceKey,
			&h.EvidenceJSON,
			&createdAtStr,
		); err != nil {
			return nil, fmt.Errorf("failed to scan node_source_history row: %w", err)
		}

		if subID.Valid {
			s := subID.String
			h.SubscriptionID = &s
		}
		if connRev.Valid {
			r := connRev.Int64
			h.ConnectionRevision = &r
		}

		h.RelationState = domain.RelationState(relState)
		h.Cause = domain.AttributionCause(cause)

		if firstObsStr.Valid && strings.TrimSpace(firstObsStr.String) != "" {
			t, _ := parseFlexibleTimestamp(firstObsStr.String)
			h.FirstObservedAt = t
		}
		if lastObsStr.Valid && strings.TrimSpace(lastObsStr.String) != "" {
			t, _ := parseFlexibleTimestamp(lastObsStr.String)
			h.LastObservedAt = t
		}

		if t, err := parseFlexibleTimestamp(createdAtStr); err == nil && t != nil {
			h.CreatedAt = *t
		}

		list = append(list, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading node_source_history rows: %w", err)
	}

	return list, nil
}
