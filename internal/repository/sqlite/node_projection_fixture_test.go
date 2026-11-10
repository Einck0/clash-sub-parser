package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// setupNodeProjectionDB uses the production DSN and bounded pool rather than
// the legacy reset fixture's raw sql.Open, which leaves foreign keys disabled.
func setupNodeProjectionDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenAndMigrate(context.Background(), DefaultConfig(filepath.Join(t.TempDir(), "projection.db")))
	if err != nil {
		t.Fatalf("open projection fixture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var foreignKeys int
	if err := db.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("projection fixture foreign_keys=%d err=%v", foreignKeys, err)
	}
	return db
}

func TestNodeProjectionForeignKeysOnEveryPoolConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "pool.db"))
	cfg.MaxOpenConns, cfg.MaxIdleConns = 2, 2
	db, err := OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Hold the first connection so acquiring the second must open a different
	// physical connection. A one-time db.Exec(PRAGMA...) is not sufficient.
	first, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for i, conn := range []*sql.Conn{first, second} {
		var foreignKeys int
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
			t.Fatalf("pool connection %d foreign_keys=%d err=%v", i, foreignKeys, err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO node_sources(node_logical_id,subscription_id,last_seen_fetch_id) VALUES('missing-node','missing-sub','missing-fetch')`); err == nil {
			t.Fatalf("pool connection %d accepted foreign key violation", i)
		}
	}
}
