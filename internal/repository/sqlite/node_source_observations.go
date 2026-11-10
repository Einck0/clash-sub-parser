package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
)

func savePayloadEvidenceTx(ctx context.Context, tx *sql.Tx, e domain.PayloadEvidence) error {
	if e.PayloadID == "" || e.ContentDigest == "" || e.SourceIdentity == "" {
		return fmt.Errorf("invalid payload evidence")
	}
	var sub any
	if e.SubscriptionID != "" {
		sub = e.SubscriptionID
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO subscription_payload_evidence
		(payload_id,subscription_id,source_identity,fetch_id,content_digest,original_created_at,captured_at,http_status,evidence_kind,quality,pinned)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, e.PayloadID, sub, e.SourceIdentity, e.FetchID, e.ContentDigest, e.OriginalCreatedAt, e.CapturedAt, e.HTTPStatus, e.EvidenceKind, e.Quality, e.Pinned); err != nil {
		return err
	}
	var digest, identity, fetchID string
	if err := tx.QueryRowContext(ctx, `SELECT content_digest,source_identity,fetch_id FROM subscription_payload_evidence WHERE payload_id=?`, e.PayloadID).Scan(&digest, &identity, &fetchID); err != nil {
		return err
	}
	if digest != e.ContentDigest || identity != e.SourceIdentity || fetchID != e.FetchID {
		return fmt.Errorf("immutable payload evidence conflict")
	}
	return nil
}

// loadNodePayloadEvidence binds origins to the very same read snapshot as
// complete node configuration. A missing old origin stays unknown; never use a
// new current entry as a historical version's substitute.
func loadNodePayloadEvidence(ctx context.Context, tx *sql.Tx, nodes []domain.Node) (map[string][]domain.PayloadEvidence, error) {
	result := make(map[string][]domain.PayloadEvidence)
	for _, node := range nodes {
		rows, err := tx.QueryContext(ctx, `SELECT payload_id,subscription_id,source_identity,fetch_id,content_digest,original_created_at,captured_at,http_status,evidence_kind,quality,pinned FROM (
			SELECT e.payload_id,e.subscription_id,e.source_identity,e.fetch_id,e.content_digest,e.original_created_at,e.captured_at,e.http_status,e.evidence_kind,e.quality,e.pinned
			FROM node_connection_provenance p JOIN subscription_payload_evidence e ON e.payload_id=p.origin_payload_id
			WHERE p.node_logical_id=? AND p.connection_revision=?
			UNION
			SELECT p.id,p.subscription_id,'sub:'||p.subscription_id,p.fetch_id,p.content_digest,p.created_at,p.created_at,p.http_status,'payload_origin','observed',p.pinned
			FROM node_connection_versions v JOIN subscription_entries se ON se.id=v.source_entry_id JOIN subscription_payloads p ON p.id=se.payload_id
			WHERE v.node_logical_id=? AND v.connection_revision=?
		) ORDER BY payload_id`, node.LogicalID, node.ConnectionRevision, node.LogicalID, node.ConnectionRevision)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var evidence domain.PayloadEvidence
			var subID sql.NullString
			if err := rows.Scan(&evidence.PayloadID, &subID, &evidence.SourceIdentity, &evidence.FetchID, &evidence.ContentDigest, &evidence.OriginalCreatedAt, &evidence.CapturedAt, &evidence.HTTPStatus, &evidence.EvidenceKind, &evidence.Quality, &evidence.Pinned); err != nil {
				rows.Close()
				return nil, err
			}
			evidence.SubscriptionID = subID.String
			result[node.LogicalID] = append(result[node.LogicalID], evidence)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return result, nil
}

// PreserveSubscriptionGenerationTx detaches only after stable origin evidence
// and lightweight name matching observations have been persisted. It is called
// inside the reconcile transaction before replacing a sealed current generation.
// Legacy multi-generation data must never reach this runtime path.
func PreserveSubscriptionGenerationTx(ctx context.Context, tx *sql.Tx, subID string) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_payloads WHERE subscription_id=?`, subID).Scan(&count); err != nil {
		return err
	}
	var sealed int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_storage_readiness WHERE singleton=1 AND contract_version=1`).Scan(&sealed); err != nil || sealed != 1 {
		return fmt.Errorf("maintenance_required: current-generation replacement requires sealed storage")
	}
	if count > 1 {
		return fmt.Errorf("maintenance_required: legacy generations require offline reconciliation")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Preserve only genuine pin/publication/version consumers, not every old
	// payload. Current origins are never rewritten to a later observation.
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO subscription_payload_evidence
		(payload_id,subscription_id,source_identity,fetch_id,content_digest,original_created_at,captured_at,http_status,evidence_kind,quality,pinned)
		SELECT p.id,p.subscription_id,'sub:'||p.subscription_id,p.fetch_id,p.content_digest,p.created_at,?,p.http_status,'payload_origin','observed',p.pinned
		FROM subscription_payloads p WHERE p.subscription_id=? AND
		(p.pinned=1 OR EXISTS(SELECT 1 FROM publication_payload_refs r WHERE r.payload_id=p.id)
		OR EXISTS(SELECT 1 FROM node_connection_versions v JOIN subscription_entries e ON e.id=v.source_entry_id WHERE e.payload_id=p.id))`, now, subID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO node_connection_provenance
		(node_logical_id,connection_revision,origin_payload_id,created_at)
		SELECT v.node_logical_id,v.connection_revision,e.payload_id,v.created_at
		FROM node_connection_versions v JOIN subscription_entries e ON e.id=v.source_entry_id WHERE e.subscription_id=?`, subID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO node_source_observations
		(node_logical_id,source_identity,subscription_id,raw_name,protocol,source_key,observed_at)
		SELECT e.node_logical_id,'sub:'||e.subscription_id,e.subscription_id,e.raw_name,e.protocol,e.source_key,e.created_at
		FROM subscription_entries e WHERE e.subscription_id=? AND e.node_logical_id IS NOT NULL
		AND e.rowid=(SELECT e2.rowid FROM subscription_entries e2 WHERE e2.node_logical_id=e.node_logical_id AND e2.subscription_id=e.subscription_id ORDER BY e2.created_at DESC,e2.rowid DESC LIMIT 1)
		ON CONFLICT(node_logical_id,source_identity) DO UPDATE SET raw_name=excluded.raw_name,protocol=excluded.protocol,source_key=excluded.source_key,observed_at=excluded.observed_at`, subID)
	if err != nil {
		return err
	}
	// Observed entries do not prove validation/removal. Preserve one unknown
	// relationship per source only when no actual ledger relationship exists.
	_, err = tx.ExecContext(ctx, `INSERT INTO node_source_history
		(id,node_logical_id,subscription_id,source_identity,source_label,relation_state,cause,first_observed_at,last_observed_at,evidence_kind,evidence_key,evidence_json,created_at)
		SELECT 'observation:'||o.node_logical_id||':'||o.source_identity,o.node_logical_id,o.subscription_id,o.source_identity,s.name,
		'unknown','unresolved',o.observed_at,o.observed_at,'retention_entry_observation','observation:'||o.source_identity,'{}',?
		FROM node_source_observations o JOIN subscriptions s ON s.id=o.subscription_id
		WHERE o.subscription_id=?
		AND NOT EXISTS(SELECT 1 FROM node_sources ns WHERE ns.node_logical_id=o.node_logical_id AND ns.subscription_id=o.subscription_id)
		AND NOT EXISTS(SELECT 1 FROM node_source_history h WHERE h.node_logical_id=o.node_logical_id AND h.source_identity=o.source_identity)
		ON CONFLICT(node_logical_id,evidence_key) DO NOTHING`, now, subID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE node_connection_versions SET source_entry_id=NULL WHERE source_entry_id IN (SELECT id FROM subscription_entries WHERE subscription_id=?)`, subID)
	return err
}

// BeginSubscriptionAttempt fences requests by admission order, not completion
// timestamp. It neither changes applied membership nor overwrites saved raw.
func BeginSubscriptionAttempt(ctx context.Context, db *sql.DB, subID, revision string) (int64, error) {
	var sequence int64
	err := WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		var actualRevision string
		if err := tx.QueryRowContext(ctx, `SELECT revision FROM subscriptions WHERE id=?`, subID).Scan(&actualRevision); err != nil {
			return err
		}
		if actualRevision != revision {
			return domain.NewConflictError("subscription_revision_conflict", "subscription changed before attempt admission")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_current_state (subscription_id,source_revision,attempt_sequence) VALUES (?,?,1)
		ON CONFLICT(subscription_id) DO UPDATE SET attempt_sequence=subscription_current_state.attempt_sequence+1`, subID, revision); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT attempt_sequence FROM subscription_current_state WHERE subscription_id=?`, subID).Scan(&sequence)
	})
	return sequence, err
}

// RecordFailedSubscriptionAttempt persists the failed attempt only if it still
// owns the admission fence and source revision. Failed admission/DB writes are
// returned, never silently reported as a saved diagnostic.
func RecordFailedSubscriptionAttempt(ctx context.Context, db *sql.DB, fetch *domain.SubscriptionFetch, revision string, sequence int64) error {
	return WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		var currentRevision string
		var currentSequence int64
		if err := tx.QueryRowContext(ctx, `SELECT s.revision,c.attempt_sequence FROM subscriptions s JOIN subscription_current_state c ON c.subscription_id=s.id WHERE s.id=?`, fetch.SubscriptionID).Scan(&currentRevision, &currentSequence); err != nil {
			return err
		}
		if currentRevision != revision || currentSequence != sequence {
			return domain.NewConflictError("subscription_refresh_conflict", "subscription changed or newer attempt admitted")
		}
		var finished any
		if fetch.FinishedAt != nil {
			finished = fetch.FinishedAt.UTC().Format(time.RFC3339Nano)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_fetches(id,subscription_id,started_at,finished_at,outcome,content_digest,redacted_error,nodes_parsed,nodes_valid)VALUES(?,?,?,?,?,?,?,?,?)`, fetch.ID, fetch.SubscriptionID, fetch.StartedAt.UTC().Format(time.RFC3339Nano), finished, string(fetch.Outcome), fetch.ContentDigest, fetch.RedactedError, fetch.NodesParsed, fetch.NodesValid); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE subscription_current_state SET last_attempt_fetch_id=?,last_attempt_status=? WHERE subscription_id=?`, fetch.ID, string(fetch.Outcome), fetch.SubscriptionID); err != nil {
			return err
		}
		_, err := PruneFetchSummariesTx(ctx, tx, fetch.SubscriptionID)
		return err
	})
}

// ReplaceCurrentPayloadTx only deletes a sealed single generation. The caller
// has already loaded its identity/matching inputs in the same transaction.
func ReplaceCurrentPayloadTx(ctx context.Context, tx *sql.Tx, subID string) error {
	if err := PreserveSubscriptionGenerationTx(ctx, tx, subID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM subscription_payloads WHERE subscription_id=?`, subID)
	return err
}
