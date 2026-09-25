package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	"clash-sub-parser/internal/repository/sqlite"
)

func TestIPRiskMigrationRollsBackFailedDDL(t *testing.T) {
	cfg := sqlite.Config{Path: "file:iprisk_rollback?mode=memory&cache=shared", ForeignKeys: true}
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migrationFS := fstest.MapFS{
		"000001_failed_ip_risk.sql": &fstest.MapFile{Data: []byte("CREATE TABLE transient_ip_risk (id TEXT); THIS IS NOT VALID SQL;")},
	}
	runner := sqlite.NewMigrationRunner(db, migrationFS)
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("invalid migration must fail")
	}
	var count int
	if err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='transient_ip_risk';").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration must roll back DDL")
	}
	var applied int
	if err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM schema_migrations WHERE version=1;").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatal("failed migration must not be recorded")
	}
}

func TestIPRiskSchemaForeignKeyCheckIsClean(t *testing.T) {
	db, _ := setupTestDB(t)
	rows, err := db.QueryContext(context.Background(), "PRAGMA foreign_key_check;")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID, fkID int64
		var target string
		if err := rows.Scan(&table, &rowID, &target, &fkID); err != nil && err != sql.ErrNoRows {
			t.Fatal(err)
		}
		t.Fatalf("foreign_key_check found violation in %s", table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
