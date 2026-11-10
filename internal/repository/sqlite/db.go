package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"

	_ "modernc.org/sqlite"

	"clash-sub-parser/migrations"
)

// Open establishes a database connection pool to the SQLite database with specified configuration.
func Open(cfg Config) (*sql.DB, error) {
	dsn := cfg.BuildDSN()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 1
	}
	db.SetMaxOpenConns(maxOpen)

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 1
	}
	db.SetMaxIdleConns(maxIdle)

	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	// Verify connection liveness and set core runtime pragmas
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	if cfg.ForeignKeys {
		if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to enforce foreign keys: %w", err)
		}
	}

	timeoutMS := int64(5000)
	if cfg.BusyTimeout > 0 {
		timeoutMS = cfg.BusyTimeout.Milliseconds()
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d;", timeoutMS)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	if cfg.WALMode && !isMemoryPath(cfg.Path) {
		if _, err := db.Exec("PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to configure WAL mode: %w", err)
		}
	}

	return db, nil
}

// OpenReadOnly opens an existing SQLite database strictly in read-only mode with query_only enabled.
// It performs zero DDL or DML writes and requires the target database file to exist.
func OpenReadOnly(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("read-only path cannot be empty")
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=query_only(1)&_pragma=foreign_keys(0)", url.PathEscape(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open read-only sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping read-only sqlite database: %w", err)
	}
	if _, err := db.Exec("PRAGMA query_only = ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set query_only pragma: %w", err)
	}
	return db, nil
}

// OpenAndMigrate opens the database and runs all pending embedded migrations.
func OpenAndMigrate(ctx context.Context, cfg Config) (*sql.DB, error) {
	// Do not Open (whose DSN enables WAL) before inspecting legacy files.
	// Even EnsureMigrationTable/28 index backfill is a prohibited startup write.
	_, err := preflightSubscriptionStorage(ctx, cfg.Path)
	if err != nil {
		return nil, err
	}
	db, err := Open(cfg)
	if err != nil {
		return nil, err
	}

	runner := NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply migrations during startup: %w", err)
	}

	return db, nil
}

func preflightSubscriptionStorage(ctx context.Context, path string) (bool, error) {
	if isMemoryPath(path) {
		return true, nil
	}
	// Non-memory URI paths cannot bypass the filesystem identity inspection.
	if strings.HasPrefix(path, "file:") {
		return false, fmt.Errorf("maintenance_required: startup requires a plain database path")
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := ensureNoSidecarsForMissingDatabase(path); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("maintenance_required: database must be a regular file")
	}
	if info.Size() == 0 {
		if err := ensureNoSidecarsForMissingDatabase(path); err != nil {
			return false, err
		}
		return true, nil
	}
	db, err := OpenReadOnly(path)
	if err != nil {
		return false, fmt.Errorf("maintenance_required: cannot inspect existing database read-only")
	}
	defer db.Close()
	if err := CheckSubscriptionStorageReady(ctx, db); err != nil {
		return false, err
	}
	return false, nil
}

func ensureNoSidecarsForMissingDatabase(path string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			return fmt.Errorf("maintenance_required: orphan database sidecar exists")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// WithTx executes the supplied function within a database transaction.
// The transaction is automatically rolled back if fn returns an error, context is cancelled, or a panic occurs.
func WithTx(ctx context.Context, db *sql.DB, fn func(ctx context.Context, tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = fn(ctx, tx); err != nil {
		return err
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		err = ctxErr
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
