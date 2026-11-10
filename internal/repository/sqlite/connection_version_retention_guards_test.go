package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestConnectionVersionRetentionUnsealedStorageAndMissingHead(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
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
