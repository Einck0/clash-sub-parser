package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func sealTestVersionConsumers(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`UPDATE subscription_storage_readiness SET consumers_closed=1 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionVersionRetentionUnsealedStorageAndMissingHead(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	sealTestVersionConsumers(t, db)
	repo := sqlite.NewNodeRepository(db)
	node := domain.Node{LogicalID: "guard-node", Protocol: domain.ProtocolSS, DisplayName: "Synthetic", Server: "192.0.2.1", Port: 8388, Active: true}
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	// A missing head must never mean that every version is deletable.
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_connection_heads WHERE logical_id=?`, node.LogicalID); err != nil {
			return err
		}
		return sqlite.PruneConnectionVersionsTx(ctx, tx, node.LogicalID)
	}); err == nil {
		t.Fatal("missing head allowed prune")
	}
	if _, err := sqlite.NewNodeConnectionVersionRepository(db).GetHead(ctx, node.LogicalID); err != nil {
		t.Fatalf("failed guard did not rollback head: %v", err)
	}
	// Runtime must not implicitly adopt unsealed legacy storage. This fixture
	// removes only the seal in a disposable database, not in production.
	if _, err := db.ExecContext(ctx, `DELETE FROM subscription_storage_readiness`); err != nil {
		t.Fatal(err)
	}
	node.Server = "192.0.2.2"
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions WHERE node_logical_id=?`, node.LogicalID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("unsealed history was pruned: count=%d err=%v", count, err)
	}
}

func TestConnectionVersionRetentionPropagatesClosureQueryError(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewNodeRepository(db)
	node := domain.Node{LogicalID: "closure-error-node", Protocol: domain.ProtocolSS, DisplayName: "Synthetic", Server: "192.0.2.1", Port: 8388, Active: true}
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE subscription_storage_readiness RENAME COLUMN consumers_closed TO broken_consumer_closure`); err != nil {
		t.Fatal(err)
	}
	node.Server = "192.0.2.2"
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err == nil || !strings.Contains(err.Error(), "consumers_closed") {
		t.Fatalf("closure query error was hidden: %v", err)
	}
	current, err := repo.GetByLogicalID(ctx, node.LogicalID)
	if err != nil || current == nil || current.Server != "192.0.2.1" || current.ConnectionRevision != 1 {
		t.Fatalf("closure query error did not roll back node/head: %+v err=%v", current, err)
	}
}

func TestConnectionVersionRetentionRequiresConsumerClosure(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewNodeRepository(db)
	node := domain.Node{LogicalID: "open-consumer-node", Protocol: domain.ProtocolSS, DisplayName: "Synthetic", Server: "192.0.2.1", Port: 8388, Active: true}
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE subscription_storage_readiness SET consumers_closed=0 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	var closed int
	if err := db.QueryRowContext(ctx, `SELECT consumers_closed FROM subscription_storage_readiness WHERE singleton=1`).Scan(&closed); err != nil || closed != 0 {
		t.Fatalf("fixture must have unconfirmed consumer closure: closed=%d err=%v", closed, err)
	}
	node.Server = "192.0.2.2"
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	versions, _, err := sqlite.SnapshotRecoveryVersions(ctx, db, node.LogicalID, "open-consumer-test", true)
	if err != nil || len(versions) != 2 || versions[0].Revision != 1 || versions[1].Revision != 2 {
		t.Fatalf("unproved closure lost historical recovery version: %+v err=%v", versions, err)
	}
	sealTestVersionConsumers(t, db)
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.PruneConnectionVersionsTx(ctx, tx, node.LogicalID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlite.NewNodeConnectionVersionRepository(db).GetByRevision(ctx, node.LogicalID, 1); err == nil {
		t.Fatal("closed consumer fixture did not prune unleased old version")
	}
}
