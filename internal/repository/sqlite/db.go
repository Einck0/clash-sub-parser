package sqlite

import (
	"context"
	"database/sql"
	"fmt"

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

// OpenAndMigrate opens the database and runs all pending embedded migrations.
func OpenAndMigrate(ctx context.Context, cfg Config) (*sql.DB, error) {
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
