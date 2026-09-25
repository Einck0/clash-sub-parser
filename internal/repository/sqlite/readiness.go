package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ForeignKeyViolation describes a foreign key constraint violation detected by PRAGMA foreign_key_check.
type ForeignKeyViolation struct {
	Table       string `json:"table"`
	RowID       int64  `json:"rowid"`
	TargetTable string `json:"target_table"`
	FkID        int    `json:"fkid"`
}

// ReadinessReport details the operational and schema readiness of the SQLite database.
type ReadinessReport struct {
	Ready                bool                  `json:"ready"`
	SchemaVersion        int                   `json:"schema_version"`
	RequiredTables       []string              `json:"required_tables"`
	MissingTables        []string              `json:"missing_tables"`
	ForeignKeyViolations []ForeignKeyViolation `json:"foreign_key_violations"`
	Error                string                `json:"error,omitempty"`
}

// RequiredTables returns the complete frozen list of 14 CSP 1.0 core database tables.
func RequiredTables() []string {
	return []string{
		"subscriptions",
		"subscription_fetches",
		"nodes",
		"node_sources",
		"probe_runs",
		"probe_observations",
		"node_groups",
		"group_edges",
		"admission_rules",
		"policy_rules",
		"configuration_revisions",
		"publications",
		"settings",
		"audit_events",
	}
}

// CheckReadiness validates that all required tables exist, migrations have executed, and no foreign key violations exist.
func CheckReadiness(ctx context.Context, db *sql.DB) (*ReadinessReport, error) {
	report := &ReadinessReport{
		Ready:                false,
		RequiredTables:       RequiredTables(),
		MissingTables:        make([]string, 0),
		ForeignKeyViolations: make([]ForeignKeyViolation, 0),
	}

	// 1. Connection check
	if err := db.PingContext(ctx); err != nil {
		report.Error = fmt.Sprintf("database ping failed: %v", err)
		return report, nil
	}

	// 2. Discover existing tables in sqlite_master
	const tableQuery = "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%';"
	rows, err := db.QueryContext(ctx, tableQuery)
	if err != nil {
		report.Error = fmt.Sprintf("failed to query sqlite_master tables: %v", err)
		return report, nil
	}
	defer rows.Close()

	existingTables := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			report.Error = fmt.Sprintf("failed to scan table name: %v", err)
			return report, nil
		}
		existingTables[name] = true
	}
	if err := rows.Err(); err != nil {
		report.Error = fmt.Sprintf("error reading sqlite_master: %v", err)
		return report, nil
	}

	// 3. Check for required tables
	for _, req := range report.RequiredTables {
		if !existingTables[req] {
			report.MissingTables = append(report.MissingTables, req)
		}
	}

	// 4. Check schema_migrations version
	if !existingTables["schema_migrations"] {
		report.MissingTables = append(report.MissingTables, "schema_migrations")
		report.Error = "schema_migrations table does not exist; database is not migrated"
		return report, nil
	}

	var latestVer sql.NullInt64
	err = db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations;").Scan(&latestVer)
	if err != nil {
		report.Error = fmt.Sprintf("failed to query schema_migrations version: %v", err)
		return report, nil
	}
	if latestVer.Valid {
		report.SchemaVersion = int(latestVer.Int64)
	}

	if report.SchemaVersion < 1 {
		report.Error = "database has not completed baseline schema migration (version < 1)"
		return report, nil
	}

	if len(report.MissingTables) > 0 {
		report.Error = fmt.Sprintf("missing %d required tables: %s", len(report.MissingTables), strings.Join(report.MissingTables, ", "))
		return report, nil
	}

	// 5. Check foreign key consistency
	fkRows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		report.Error = fmt.Sprintf("failed executing PRAGMA foreign_key_check: %v", err)
		return report, nil
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var v ForeignKeyViolation
		if err := fkRows.Scan(&v.Table, &v.RowID, &v.TargetTable, &v.FkID); err != nil {
			report.Error = fmt.Sprintf("failed scanning foreign key violation: %v", err)
			return report, nil
		}
		report.ForeignKeyViolations = append(report.ForeignKeyViolations, v)
	}
	if err := fkRows.Err(); err != nil {
		report.Error = fmt.Sprintf("error checking foreign key violations: %v", err)
		return report, nil
	}

	if len(report.ForeignKeyViolations) > 0 {
		report.Error = fmt.Sprintf("detected %d foreign key constraint violations", len(report.ForeignKeyViolations))
		return report, nil
	}

	// Database is fully ready
	report.Ready = true
	return report, nil
}
