package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// PrepareConnectionVersionLeasesTx creates only the lease structure. It must
// be called by explicit offline preparation after a migration number is assigned,
// never by acquisition, startup or an automatic cleanup fallback. Preparation
// does NOT mean that every version consumer has registered or GC is safe.
func PrepareConnectionVersionLeasesTx(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return errors.New("lease preparation requires a transaction")
	}
	_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS node_connection_version_leases (
		consumer_kind TEXT NOT NULL,
		consumer_id TEXT NOT NULL,
		node_logical_id TEXT NOT NULL,
		connection_revision INTEGER NOT NULL,
		owner TEXT NOT NULL,
		fencing_token TEXT NOT NULL,
		heartbeat_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		PRIMARY KEY (consumer_kind, consumer_id, node_logical_id, connection_revision),
		FOREIGN KEY (node_logical_id, connection_revision)
			REFERENCES node_connection_versions(node_logical_id, connection_revision) ON DELETE RESTRICT
	)`)
	return err
}

func validateConnectionVersionLease(lease domain.ConnectionVersionLease) error {
	if strings.TrimSpace(lease.ConsumerKind) == "" || strings.TrimSpace(lease.ConsumerID) == "" ||
		strings.TrimSpace(lease.NodeID) == "" || strings.TrimSpace(lease.Owner) == "" || strings.TrimSpace(lease.FencingToken) == "" || lease.Revision < 0 {
		return errors.New("invalid connection version lease identity")
	}
	return nil
}

// AcquireConnectionVersionLeaseTx writes the lease before reading the complete
// configuration, serializing against any deletion on SQLite's write lock. The
// caller consumes the result only after commit. Acquisition never creates a
// missing version, replaces another owner, steals an expired lease or reads head
// as a substitute for an unavailable revision (including revision zero).
func AcquireConnectionVersionLeaseTx(ctx context.Context, tx *sql.Tx, lease domain.ConnectionVersionLease) (*domain.LeasedConnectionVersion, error) {
	if tx == nil {
		return nil, errors.New("lease acquisition requires a transaction")
	}
	if err := validateConnectionVersionLease(lease); err != nil {
		return nil, err
	}
	if lease.HeartbeatAt.IsZero() || !lease.ExpiresAt.After(lease.HeartbeatAt) {
		return nil, errors.New("lease requires an explicit heartbeat and later diagnostic expiry")
	}
	var foreignKeys int
	if err := tx.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return nil, err
	}
	if foreignKeys != 1 {
		return nil, errors.New("lease acquisition requires foreign key enforcement")
	}
	// Only an already-owned token may renew the same logical lease. SQLite's
	// WHERE clause also fences concurrent/retried acquisitions by another owner.
	result, err := tx.ExecContext(ctx, `INSERT INTO node_connection_version_leases
		(consumer_kind,consumer_id,node_logical_id,connection_revision,owner,fencing_token,heartbeat_at,expires_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(consumer_kind,consumer_id,node_logical_id,connection_revision) DO UPDATE SET
			heartbeat_at=excluded.heartbeat_at, expires_at=excluded.expires_at
		WHERE node_connection_version_leases.owner=excluded.owner
		  AND node_connection_version_leases.fencing_token=excluded.fencing_token`,
		lease.ConsumerKind, lease.ConsumerID, lease.NodeID, lease.Revision, lease.Owner, lease.FencingToken,
		lease.HeartbeatAt.UTC().Format(time.RFC3339Nano), lease.ExpiresAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("acquire connection version lease: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, errors.New("connection version lease owner conflict")
	}
	snapshot := &domain.LeasedConnectionVersion{NodeID: lease.NodeID, Revision: lease.Revision}
	if err := tx.QueryRowContext(ctx, `SELECT effective_config_json FROM node_connection_versions WHERE node_logical_id=? AND connection_revision=?`,
		lease.NodeID, lease.Revision).Scan(&snapshot.EffectiveConfigJSON); err != nil {
		return nil, fmt.Errorf("historical_configuration_unavailable: %w", err)
	}
	return snapshot, nil
}

// SnapshotRecoveryVersions loads exactly the available revisions in one write
// transaction, registering leases before exposing their immutable copies. A
// read-only dry-run uses one read transaction instead and never enables GC.
func SnapshotRecoveryVersions(ctx context.Context, db *sql.DB, nodeID, consumerID string, dryRun bool) ([]domain.LeasedConnectionVersion, []domain.ConnectionVersionLease, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: dryRun})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	// Reserve the write transaction before discovering revisions; a GC writer
	// cannot remove a discovered version between discovery and acquisition.
	if !dryRun {
		if _, err := tx.ExecContext(ctx, `UPDATE subscription_storage_readiness SET consumers_closed=consumers_closed WHERE singleton=1`); err != nil {
			return nil, nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT connection_revision,effective_config_json FROM node_connection_versions WHERE node_logical_id=? ORDER BY connection_revision`, nodeID)
	if err != nil {
		return nil, nil, err
	}
	var snapshots []domain.LeasedConnectionVersion
	for rows.Next() {
		var v domain.LeasedConnectionVersion
		v.NodeID = nodeID
		if err := rows.Scan(&v.Revision, &v.EffectiveConfigJSON); err != nil {
			rows.Close()
			return nil, nil, err
		}
		snapshots = append(snapshots, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()
	leases := make([]domain.ConnectionVersionLease, 0, len(snapshots))
	if !dryRun {
		for i, v := range snapshots {
			now := time.Now().UTC()
			lease := domain.ConnectionVersionLease{ConsumerKind: "recover_history", ConsumerID: consumerID, NodeID: nodeID, Revision: v.Revision, Owner: consumerID, FencingToken: consumerID, HeartbeatAt: now, ExpiresAt: now.Add(time.Hour)}
			captured, err := AcquireConnectionVersionLeaseTx(ctx, tx, lease)
			if err != nil {
				return nil, nil, err
			}
			snapshots[i] = *captured
			leases = append(leases, lease)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return snapshots, leases, nil
}

// ReleaseConnectionVersionLeaseTx is an explicit owner action after consumer
// completion. Neither a wall-clock expiry nor a batch scheduler lease calls it.
// Unknown owners must be investigated and fenced before release, not GC'ed.
func ReleaseConnectionVersionLeaseTx(ctx context.Context, tx *sql.Tx, lease domain.ConnectionVersionLease) error {
	if tx == nil {
		return errors.New("lease release requires a transaction")
	}
	if err := validateConnectionVersionLease(lease); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM node_connection_version_leases
		WHERE consumer_kind=? AND consumer_id=? AND node_logical_id=? AND connection_revision=? AND owner=? AND fencing_token=?`,
		lease.ConsumerKind, lease.ConsumerID, lease.NodeID, lease.Revision, lease.Owner, lease.FencingToken)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("connection version lease release owner conflict or already released")
	}
	return nil
}
