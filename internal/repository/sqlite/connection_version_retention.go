package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PruneConnectionVersionsTx bounds full configurations to the head and explicitly
// leased revisions only after all version consumers are known to be closed. Call
// it in the SAME transaction as head advancement or owner release. Expiry is
// diagnostic only: even an unknown, expired owner protects its version.
// Lightweight origin evidence outlives full historical credentials.
//
// Legacy/unsealed databases and databases without a consumer-closure seal are
// deliberately left unchanged. A storage seal alone does not prove that old
// consumers have registered leases.
func PruneConnectionVersionsTx(ctx context.Context, tx *sql.Tx, nodeID string) error {
	if tx == nil || nodeID == "" {
		return fmt.Errorf("connection version retention requires transaction and node")
	}
	var present int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name='subscription_storage_readiness'`).Scan(&present); err != nil {
		return err
	}
	if present == 0 {
		return nil
	}
	var contractVersion, closed int
	if err := tx.QueryRowContext(ctx, `SELECT contract_version,consumers_closed FROM subscription_storage_readiness WHERE singleton=1`).Scan(&contractVersion, &closed); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("inspect version consumer closure: %w", err)
	}
	if closed != 0 && closed != 1 {
		return fmt.Errorf("invalid version consumer closure value")
	}
	if contractVersion != 1 || closed == 0 {
		return nil
	}
	var foreignKeys int
	if err := tx.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return err
	}
	if foreignKeys != 1 {
		return fmt.Errorf("connection version retention requires foreign key enforcement")
	}
	// Fail closed rather than interpreting a missing/corrupt head as permission
	// to remove every available configuration. The caller already holds a write
	// transaction, which also serializes lease acquisition with deletion.
	var head int64
	if err := tx.QueryRowContext(ctx, `SELECT h.connection_revision FROM node_connection_heads h
		JOIN node_connection_versions v ON v.node_logical_id=h.logical_id AND v.connection_revision=h.connection_revision
		WHERE h.logical_id=?`, nodeID).Scan(&head); err != nil {
		return fmt.Errorf("connection version retention requires available head: %w", err)
	}
	const candidates = `v.node_logical_id=? AND v.connection_revision!=?
		AND NOT EXISTS(SELECT 1 FROM node_connection_version_leases l
			WHERE l.node_logical_id=v.node_logical_id AND l.connection_revision=v.connection_revision)`
	// Save origin before deleting the row carrying source_entry_id. Do not bind
	// an old revision to a newer current payload, or fabricate an unknown origin.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO subscription_payload_evidence
		(payload_id,subscription_id,source_identity,fetch_id,content_digest,original_created_at,captured_at,http_status,evidence_kind,quality,pinned)
		SELECT p.id,p.subscription_id,'sub:'||p.subscription_id,p.fetch_id,p.content_digest,p.created_at,?,p.http_status,'payload_origin','observed',p.pinned
		FROM node_connection_versions v JOIN subscription_entries e ON e.id=v.source_entry_id JOIN subscription_payloads p ON p.id=e.payload_id
		WHERE `+candidates, now, nodeID, head); err != nil {
		return fmt.Errorf("preserve retired version evidence: %w", err)
	}
	var conflicts int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions v
		JOIN subscription_entries e ON e.id=v.source_entry_id JOIN subscription_payloads p ON p.id=e.payload_id
		JOIN subscription_payload_evidence pe ON pe.payload_id=p.id WHERE `+candidates+`
		AND (pe.content_digest IS NOT p.content_digest OR pe.source_identity IS NOT 'sub:'||p.subscription_id OR pe.fetch_id IS NOT p.fetch_id)`, nodeID, head).Scan(&conflicts); err != nil {
		return err
	}
	if conflicts != 0 {
		return fmt.Errorf("immutable retired version evidence conflict")
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO node_connection_provenance
		(node_logical_id,connection_revision,origin_payload_id,created_at)
		SELECT v.node_logical_id,v.connection_revision,e.payload_id,v.created_at
		FROM node_connection_versions v JOIN subscription_entries e ON e.id=v.source_entry_id
		WHERE `+candidates, nodeID, head); err != nil {
		return fmt.Errorf("preserve retired version origin: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM node_connection_versions AS v WHERE `+candidates, nodeID, head); err != nil {
		return fmt.Errorf("prune unleased connection versions: %w", err)
	}
	return nil
}
