package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Migration represents a single sequential database migration step.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// MigrationRunner coordinates the sequential application of embedded migrations.
type MigrationRunner struct {
	db   *sql.DB
	fsys fs.FS
}

// NewMigrationRunner creates a MigrationRunner configured with the given database and filesystem.
func NewMigrationRunner(db *sql.DB, fsys fs.FS) *MigrationRunner {
	return &MigrationRunner{
		db:   db,
		fsys: fsys,
	}
}

// EnsureMigrationTable creates the schema_migrations version table if it does not already exist.
func EnsureMigrationTable(ctx context.Context, db *sql.DB) error {
	const query = `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	);`
	_, err := db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}
	return nil
}

// AppliedVersions queries schema_migrations and returns a map of applied version numbers to applied_at timestamps.
func (r *MigrationRunner) AppliedVersions(ctx context.Context) (map[int]string, error) {
	if err := EnsureMigrationTable(ctx, r.db); err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, "SELECT version, applied_at FROM schema_migrations ORDER BY version ASC;")
	if err != nil {
		return nil, fmt.Errorf("failed to query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]string)
	for rows.Next() {
		var ver int
		var appliedAt string
		if err := rows.Scan(&ver, &appliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration row: %w", err)
		}
		applied[ver] = appliedAt
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating schema_migrations: %w", err)
	}

	return applied, nil
}

// LatestVersion returns the highest migration version number recorded in schema_migrations, or 0 if none.
func (r *MigrationRunner) LatestVersion(ctx context.Context) (int, error) {
	if err := EnsureMigrationTable(ctx, r.db); err != nil {
		return 0, err
	}

	var ver sql.NullInt64
	err := r.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations;").Scan(&ver)
	if err != nil {
		return 0, fmt.Errorf("failed to query latest migration version: %w", err)
	}

	if !ver.Valid {
		return 0, nil
	}
	return int(ver.Int64), nil
}

// parseMigrationFilename extracts version and name from a migration filename like "000001_initial_schema.sql".
func parseMigrationFilename(filename string) (int, string, error) {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	if ext != ".sql" {
		return 0, "", fmt.Errorf("not a .sql migration file: %s", filename)
	}

	cleanName := strings.TrimSuffix(base, ext)
	cleanName = strings.TrimSuffix(cleanName, ".up") // support .up.sql if present

	parts := strings.SplitN(cleanName, "_", 2)
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("invalid migration filename format %q, expected <version>_<name>.sql", filename)
	}

	ver, err := strconv.Atoi(parts[0])
	if err != nil || ver <= 0 {
		return 0, "", fmt.Errorf("invalid migration version number %q in %s", parts[0], filename)
	}

	return ver, base, nil
}

// LoadMigrations loads and parses all SQL migration files from the filesystem.
func (r *MigrationRunner) LoadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(r.fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrationsList []Migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		ver, filename, err := parseMigrationFilename(name)
		if err != nil {
			return nil, err
		}

		sqlBytes, err := fs.ReadFile(r.fsys, name)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", name, err)
		}

		migrationsList = append(migrationsList, Migration{
			Version: ver,
			Name:    filename,
			SQL:     string(sqlBytes),
		})
	}

	sort.Slice(migrationsList, func(i, j int) bool {
		return migrationsList[i].Version < migrationsList[j].Version
	})

	return migrationsList, nil
}

// Run scans, orders, and applies all unapplied migrations inside isolated transactions.
func (r *MigrationRunner) Run(ctx context.Context) error {
	if err := EnsureMigrationTable(ctx, r.db); err != nil {
		return err
	}

	applied, err := r.AppliedVersions(ctx)
	if err != nil {
		return err
	}

	allMigrations, err := r.LoadMigrations()
	if err != nil {
		return err
	}

	for _, m := range allMigrations {
		if _, ok := applied[m.Version]; ok {
			continue // already applied
		}

		// Apply migration in a short atomic transaction
		err := WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
				return fmt.Errorf("failed executing migration %s (v%d): %w", m.Name, m.Version, err)
			}

			const recordQuery = `
			INSERT INTO schema_migrations (version, name, applied_at)
			VALUES (?, ?, ?);`
			nowStr := time.Now().UTC().Format(time.RFC3339)
			if _, err := tx.ExecContext(ctx, recordQuery, m.Version, m.Name, nowStr); err != nil {
				return fmt.Errorf("failed to record migration %s (v%d): %w", m.Name, m.Version, err)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("migration run aborted at version %d (%s): %w", m.Version, m.Name, err)
		}
	}

	return nil
}
