package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

// TestCheckOperationalReadiness_MigratedDatabase verifies that a freshly migrated database
// passes operational readiness with schema version >= 1 and all 14 required core tables present.
func TestCheckOperationalReadiness_MigratedDatabase(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	report, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil {
		t.Fatalf("CheckOperationalReadiness returned unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil readiness report")
	}
	if !report.Ready {
		t.Fatalf("expected operational readiness to be true, got false (error: %s, missing: %v)", report.Error, report.MissingTables)
	}
	if report.SchemaVersion < 1 {
		t.Fatalf("expected schema version >= 1, got %d", report.SchemaVersion)
	}
	if len(report.MissingTables) != 0 {
		t.Fatalf("expected 0 missing tables, got %v", report.MissingTables)
	}
	if len(report.ForeignKeyViolations) != 0 {
		t.Fatalf("expected 0 foreign key violations, got %v", report.ForeignKeyViolations)
	}
	if report.Error != "" {
		t.Fatalf("expected empty error string, got: %s", report.Error)
	}

	required := sqlite.RequiredTables()
	if len(required) != 14 {
		t.Fatalf("expected 14 required tables, got %d", len(required))
	}
	for _, req := range required {
		found := false
		for _, tbl := range report.RequiredTables {
			if tbl == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required table %q not present in report.RequiredTables", req)
		}
	}
}

// TestCheckOperationalReadiness_UnmigratedEmptyDatabase verifies that an empty database without
// schema migrations applied fails operational readiness.
func TestCheckOperationalReadiness_UnmigratedEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	emptyDB, _ := setupEmptyTestDB(t)

	report, err := sqlite.CheckOperationalReadiness(ctx, emptyDB)
	if err != nil {
		t.Fatalf("CheckOperationalReadiness returned unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil readiness report")
	}
	if report.Ready {
		t.Fatal("empty unmigrated database must NOT be reported as ready")
	}
	if report.SchemaVersion != 0 {
		t.Fatalf("expected schema version 0 for unmigrated database, got %d", report.SchemaVersion)
	}
	if len(report.MissingTables) == 0 {
		t.Fatal("expected missing tables for empty database, got 0")
	}
	if report.Error == "" {
		t.Fatal("expected non-empty error message for unmigrated database")
	}
}

// TestCheckOperationalReadiness_MissingRequiredTable verifies that dropping any required core table
// causes operational readiness to fail and report the specific missing table.
func TestCheckOperationalReadiness_MissingRequiredTable(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	if _, err := db.ExecContext(ctx, "DROP TABLE publications;"); err != nil {
		t.Fatalf("failed dropping publications table: %v", err)
	}

	report, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil {
		t.Fatalf("CheckOperationalReadiness returned error: %v", err)
	}
	if report.Ready {
		t.Fatal("database missing required table 'publications' must NOT be ready")
	}

	var foundMissing bool
	for _, tbl := range report.MissingTables {
		if tbl == "publications" {
			foundMissing = true
			break
		}
	}
	if !foundMissing {
		t.Fatalf("expected 'publications' in MissingTables, got: %v", report.MissingTables)
	}
	if !strings.Contains(report.Error, "publications") {
		t.Fatalf("expected error to mention 'publications', got: %s", report.Error)
	}
}

// TestCheckOperationalReadiness_MissingSchemaMigrations verifies that a database lacking the
// schema_migrations table fails operational readiness.
func TestCheckOperationalReadiness_MissingSchemaMigrations(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	if _, err := db.ExecContext(ctx, "DROP TABLE schema_migrations;"); err != nil {
		t.Fatalf("failed dropping schema_migrations table: %v", err)
	}

	report, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil {
		t.Fatalf("CheckOperationalReadiness returned error: %v", err)
	}
	if report.Ready {
		t.Fatal("database without schema_migrations table must NOT be ready")
	}
	if !strings.Contains(report.Error, "schema_migrations table does not exist") {
		t.Fatalf("expected error about missing schema_migrations, got: %s", report.Error)
	}
}

// TestCheckOperationalReadiness_ZeroMigrationVersion verifies that an empty schema_migrations
// table (version 0 / NULL) reports database as not migrated.
func TestCheckOperationalReadiness_ZeroMigrationVersion(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	if err := sqlite.EnsureMigrationTable(ctx, db); err != nil {
		t.Fatalf("EnsureMigrationTable failed: %v", err)
	}

	report, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil {
		t.Fatalf("CheckOperationalReadiness returned error: %v", err)
	}
	if report.Ready {
		t.Fatal("database with 0 migrations applied must NOT be ready")
	}
	if report.SchemaVersion != 0 {
		t.Fatalf("expected schema version 0, got %d", report.SchemaVersion)
	}
	if !strings.Contains(report.Error, "version < 1") {
		t.Fatalf("expected error about version < 1, got: %s", report.Error)
	}
}

// TestCheckOperationalReadiness_ConnectionFailure verifies connection failure handling.
func TestCheckOperationalReadiness_ConnectionFailure(t *testing.T) {
	ctx := context.Background()

	// Nil database check
	reportNil, err := sqlite.CheckOperationalReadiness(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error for nil db: %v", err)
	}
	if reportNil.Ready {
		t.Fatal("nil database must NOT be ready")
	}
	if !strings.Contains(reportNil.Error, "database connection is nil") {
		t.Fatalf("expected nil connection error, got: %s", reportNil.Error)
	}

	// Closed database check
	db, _ := setupTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("failed closing db: %v", err)
	}

	reportClosed, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil {
		t.Fatalf("unexpected error for closed db: %v", err)
	}
	if reportClosed.Ready {
		t.Fatal("closed database must NOT be ready")
	}
	if !strings.Contains(reportClosed.Error, "database ping failed") {
		t.Fatalf("expected ping failure error, got: %s", reportClosed.Error)
	}
}

// TestReadiness_DeepCorruptionVsOperationalDifferentiated proves the crucial requirement (FR011):
// Operational readiness (/readyz) checks live connectivity, schema version, and required tables
// without performing expensive full foreign key table scans.
// In contrast, deep CheckReadiness scans foreign keys and detects relational integrity violations.
func TestReadiness_DeepCorruptionVsOperationalDifferentiated(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	// Inject an orphaned relation with foreign keys temporarily bypassed
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF;"); err != nil {
		t.Fatalf("disable fk: %v", err)
	}
	_, err := db.ExecContext(ctx, "INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('phantom-orphan-node', 'sub-corrupt', 'fetch-corrupt');")
	if err != nil {
		t.Fatalf("insert orphaned relation: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("re-enable fk: %v", err)
	}

	// 1. Fast operational check must report Ready = true because connectivity, tables, and schema version are intact.
	opReport, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil {
		t.Fatalf("CheckOperationalReadiness error: %v", err)
	}
	if !opReport.Ready {
		t.Fatalf("CheckOperationalReadiness must report ready=true even with FK violations (operational latency protection), got false: %s", opReport.Error)
	}
	if len(opReport.ForeignKeyViolations) != 0 {
		t.Fatalf("CheckOperationalReadiness must not scan foreign keys; expected 0 violations reported, got %d", len(opReport.ForeignKeyViolations))
	}

	// 2. Deep diagnostic check must report Ready = false and identify the exact foreign key violation.
	deepReport, err := sqlite.CheckReadiness(ctx, db)
	if err != nil {
		t.Fatalf("CheckReadiness error: %v", err)
	}
	if deepReport.Ready {
		t.Fatal("deep CheckReadiness must report ready=false on foreign key violation")
	}
	if len(deepReport.ForeignKeyViolations) == 0 {
		t.Fatal("deep CheckReadiness must detect and report foreign key violations")
	}
	if deepReport.ForeignKeyViolations[0].Table != "node_sources" {
		t.Fatalf("expected violation on 'node_sources', got: %+v", deepReport.ForeignKeyViolations[0])
	}
	if !strings.Contains(deepReport.Error, "foreign key constraint violations") {
		t.Fatalf("expected error mentioning foreign key constraint violations, got: %s", deepReport.Error)
	}
}

// TestReadiness_DynamicNotCached verifies that neither operational nor deep readiness
// uses permanent cached state, correctly reflecting dynamic changes to the database.
func TestReadiness_DynamicNotCached(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	// Step 1: Empty database is not ready
	rep1, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil || rep1.Ready {
		t.Fatalf("expected unmigrated db to not be ready, got ready=%v, err=%v", rep1.Ready, err)
	}

	// Step 2: Apply migrations -> now ready
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed applying migrations: %v", err)
	}
	rep2, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil || !rep2.Ready {
		t.Fatalf("expected migrated db to be ready dynamically, got ready=%v, err=%v", rep2.Ready, err)
	}

	// Step 3: Drop required table -> dynamically becomes not ready
	if _, err := db.ExecContext(ctx, "DROP TABLE audit_events;"); err != nil {
		t.Fatalf("failed dropping table: %v", err)
	}
	rep3, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil || rep3.Ready {
		t.Fatalf("expected db missing audit_events to not be ready, got ready=%v, err=%v", rep3.Ready, err)
	}
}

// TestReadiness_ForeignKeyEnforcementNotWeakened verifies that operational readiness checks
// never disable or weaken foreign key enforcement on the SQLite database connection.
func TestReadiness_ForeignKeyEnforcementNotWeakened(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	// Run operational check
	rep, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil || !rep.Ready {
		t.Fatalf("readiness check failed: %v", err)
	}

	// Verify PRAGMA foreign_keys is still ON
	var fkPragma int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fkPragma); err != nil {
		t.Fatalf("failed querying PRAGMA foreign_keys: %v", err)
	}
	if fkPragma != 1 {
		t.Fatalf("expected PRAGMA foreign_keys == 1 (enabled), got %d", fkPragma)
	}

	// Verify foreign key enforcement is active by asserting that an invalid insert fails
	_, err = db.ExecContext(ctx, "INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('phantom-fk-test', 'sub-x', 'fetch-x');")
	if err == nil {
		t.Fatal("expected insert with foreign key violation to fail when foreign_keys is enabled, but it succeeded")
	}
}
