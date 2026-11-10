package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestConnectionVersionLeaseAtomicSnapshotAndExplicitRelease(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `INSERT INTO nodes (logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at,connection_revision)
		VALUES ('lease-node','ss','Synthetic','192.0.2.1',8388,'{}',1,?,?,0)`, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	const oldConfig = `{"server":"192.0.2.1","port":8388,"credentials":{"password":"synthetic-old"}}`
	if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_versions (node_logical_id,connection_revision,effective_config_json,config_fingerprint,created_at)
		VALUES ('lease-node',0,?,'synthetic-old',?)`, oldConfig, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.PrepareConnectionVersionLeasesTx(ctx, tx)
	}); err != nil {
		t.Fatal(err)
	}
	lease := domain.ConnectionVersionLease{
		ConsumerKind: "recover_history", ConsumerID: "synthetic-recovery", NodeID: "lease-node", Revision: 0,
		Owner: "worker-1", FencingToken: "token-1", HeartbeatAt: now, ExpiresAt: now.Add(time.Minute),
	}
	var snapshot *domain.LeasedConnectionVersion
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		snapshot, err = sqlite.AcquireConnectionVersionLeaseTx(ctx, tx, lease)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || snapshot.Revision != 0 || snapshot.EffectiveConfigJSON != oldConfig {
		t.Fatal("lease must capture requested revision zero without head substitution")
	}
	// Advance head; the old consumer's configuration remains protected even
	// though its diagnostic expiry is in the past relative to current time.
	if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_versions (node_logical_id,connection_revision,effective_config_json,config_fingerprint,created_at)
		VALUES ('lease-node',1,'{}','synthetic-new',?);
		INSERT INTO node_connection_heads (logical_id,connection_revision,updated_at) VALUES ('lease-node',1,?)`, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM node_connection_versions WHERE node_logical_id='lease-node' AND connection_revision=0`); err == nil {
		t.Fatal("foreign key must protect expired but unreleased consumer")
	}
	wrong := lease
	wrong.Owner, wrong.FencingToken = "worker-2", "token-2"
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := sqlite.AcquireConnectionVersionLeaseTx(ctx, tx, wrong)
		return err
	}); err == nil {
		t.Fatal("expired lease must not be stolen")
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.ReleaseConnectionVersionLeaseTx(ctx, tx, wrong)
	}); err == nil {
		t.Fatal("wrong owner release must fail")
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.ReleaseConnectionVersionLeaseTx(ctx, tx, lease)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM node_connection_versions WHERE node_logical_id='lease-node' AND connection_revision=0`); err != nil {
		t.Fatalf("released non-head version should be deletable: %v", err)
	}
	if snapshot.EffectiveConfigJSON != oldConfig {
		t.Fatal("already captured in-memory configuration changed across head advance")
	}
}

func TestConnectionVersionLeaseUnavailableNeverFallsBackToHead(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.PrepareConnectionVersionLeasesTx(ctx, tx)
	}); err != nil {
		t.Fatal(err)
	}
	lease := domain.ConnectionVersionLease{
		ConsumerKind: "recover_history", ConsumerID: "missing", NodeID: "missing", Revision: 7,
		Owner: "worker", FencingToken: "token", HeartbeatAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := sqlite.AcquireConnectionVersionLeaseTx(ctx, tx, lease)
		return err
	}); err == nil {
		t.Fatal("missing version must fail")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_version_leases`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed acquisition left a lease: count=%d err=%v", count, err)
	}
}
