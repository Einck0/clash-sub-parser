package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func setupTestPayloadDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test-payload.db")

	dbCfg := sqlite.DefaultConfig(dbPath)
	db, err := sqlite.Open(dbCfg)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	return dbPath, db
}

func createTestBackupFile(t *testing.T, srcDBPath string) string {
	t.Helper()
	backupPath := filepath.Join(t.TempDir(), "backup.db")
	srcData, err := os.ReadFile(srcDBPath)
	if err != nil {
		t.Fatalf("failed to read src db for backup: %v", err)
	}
	if err := os.WriteFile(backupPath, srcData, 0644); err != nil {
		t.Fatalf("failed to write backup file: %v", err)
	}
	return backupPath
}

func mockInspectReport() *sqlite.PayloadStorageReport {
	return &sqlite.PayloadStorageReport{
		PayloadCount:      5,
		NonemptyBodyCount: 3,
		BodyBytes:         15000,
		ClearedBodyCount:  0,
		ClearedBodyBytes:  0,
	}
}

func mockClearReport() *sqlite.PayloadStorageReport {
	return &sqlite.PayloadStorageReport{
		PayloadCount:      5,
		NonemptyBodyCount: 0,
		BodyBytes:         0,
		ClearedBodyCount:  3,
		ClearedBodyBytes:  15000,
	}
}

func mockIdempotentClearReport() *sqlite.PayloadStorageReport {
	return &sqlite.PayloadStorageReport{
		PayloadCount:      5,
		NonemptyBodyCount: 0,
		BodyBytes:         0,
		ClearedBodyCount:  0,
		ClearedBodyBytes:  0,
	}
}

func TestRetirePayloadBodies_FlagValidation(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()

	t.Run("help flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--help"}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("expected exit code 0 for --help, got %d", code)
		}
	})

	t.Run("neither dry-run nor apply specified", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--db", dbPath}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "either --dry-run or --apply must be explicitly specified") {
			t.Errorf("expected fail-safe error message, got %q", stderr.String())
		}
	})

	t.Run("both dry-run and apply specified", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--db", dbPath, "--dry-run", "--apply"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "cannot specify both --dry-run and --apply") {
			t.Errorf("expected mutual exclusion error message, got %q", stderr.String())
		}
	})

	t.Run("vacuum with dry-run rejected", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--db", dbPath, "--dry-run", "--vacuum"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "--vacuum is only permitted with --apply") {
			t.Errorf("expected vacuum rejection error, got %q", stderr.String())
		}
	})

	t.Run("vacuum without apply rejected", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--db", dbPath, "--vacuum"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
	})

	t.Run("apply without offline rejected", func(t *testing.T) {
		backupPath := createTestBackupFile(t, dbPath)
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--db", dbPath, "--apply", "--backup-file", backupPath}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "apply requires --offline") {
			t.Errorf("expected offline error message, got %q", stderr.String())
		}
	})

	t.Run("apply without backup-file rejected", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--db", dbPath, "--apply", "--offline"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "apply requires verified backup file") {
			t.Errorf("expected backup file error message, got %q", stderr.String())
		}
	})

	t.Run("target db not found", func(t *testing.T) {
		nonExistent := filepath.Join(t.TempDir(), "nonexistent.db")
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--db", nonExistent, "--dry-run"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "not found") {
			t.Errorf("expected not found message, got %q", stderr.String())
		}
	})

	t.Run("target db is directory", func(t *testing.T) {
		dir := t.TempDir()
		var stdout, stderr bytes.Buffer
		code := run([]string{"retire-payload-bodies", "--db", dir, "--dry-run"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "is a directory") {
			t.Errorf("expected directory error message, got %q", stderr.String())
		}
	})
}

func TestRetirePayloadBodies_BackupValidation(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()

	t.Run("backup file does not exist", func(t *testing.T) {
		nonExistentBackup := filepath.Join(t.TempDir(), "missing-backup.db")
		var stdout, stderr bytes.Buffer
		code := run([]string{
			"retire-payload-bodies",
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", nonExistentBackup,
		}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "backup verification failed") {
			t.Errorf("expected backup verification failure, got %q", stderr.String())
		}
	})

	t.Run("backup file is a directory", func(t *testing.T) {
		dirBackup := t.TempDir()
		var stdout, stderr bytes.Buffer
		code := run([]string{
			"retire-payload-bodies",
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", dirBackup,
		}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "is a directory") {
			t.Errorf("expected directory error message, got %q", stderr.String())
		}
	})

	t.Run("backup file has invalid header", func(t *testing.T) {
		badHeaderFile := filepath.Join(t.TempDir(), "invalid.db")
		_ = os.WriteFile(badHeaderFile, []byte("NOT A SQLITE FILE HEADER"), 0644)
		var stdout, stderr bytes.Buffer
		code := run([]string{
			"retire-payload-bodies",
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", badHeaderFile,
		}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "header mismatch") {
			t.Errorf("expected header mismatch error, got %q", stderr.String())
		}
	})

	t.Run("backup file is identical to target db", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{
			"retire-payload-bodies",
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", dbPath,
		}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "cannot be identical") && !strings.Contains(stderr.String(), "same file") {
			t.Errorf("expected same file rejection error, got %q", stderr.String())
		}
	})

	t.Run("backup file is a hard link to target db", func(t *testing.T) {
		hardlinkPath := filepath.Join(t.TempDir(), "hardlink.db")
		if err := os.Link(dbPath, hardlinkPath); err != nil {
			t.Skipf("cannot create hard link: %v", err)
		}
		var stdout, stderr bytes.Buffer
		code := run([]string{
			"retire-payload-bodies",
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", hardlinkPath,
		}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "hard link rejected") && !strings.Contains(stderr.String(), "same file or hard link") {
			t.Errorf("expected hard link rejection error, got %q", stderr.String())
		}
	})

	t.Run("backup file corrupted failing quick_check", func(t *testing.T) {
		corruptedBackup := filepath.Join(t.TempDir(), "corrupt.db")
		// Write SQLite header followed by garbage
		header := make([]byte, 512)
		copy(header, []byte("SQLite format 3\x00"))
		_ = os.WriteFile(corruptedBackup, header, 0644)

		var stdout, stderr bytes.Buffer
		code := run([]string{
			"retire-payload-bodies",
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", corruptedBackup,
		}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "quick_check") && !strings.Contains(stderr.String(), "backup verification failed") {
			t.Errorf("expected quick_check or backup failure, got %q", stderr.String())
		}
	})
}

func TestRetirePayloadBodies_ConcurrencyAndLockRejection(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()
	backupPath := createTestBackupFile(t, dbPath)

	t.Run("lock file already exists", func(t *testing.T) {
		lockFile := dbPath + ".maintenance.lock"
		if err := os.WriteFile(lockFile, []byte("99999\n"), 0600); err != nil {
			t.Fatalf("failed to create lock file: %v", err)
		}
		defer os.Remove(lockFile)

		var stdout, stderr bytes.Buffer
		code := run([]string{
			"retire-payload-bodies",
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", backupPath,
		}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "locked by another maintenance run") {
			t.Errorf("expected lock file error, got %q", stderr.String())
		}
	})

	t.Run("concurrent service detected via hook", func(t *testing.T) {
		deps := &retirePayloadBodiesDeps{
			checkConcurrentService: func(path string) (int, error) {
				return 12345, errors.New("concurrent csp serve process (PID 12345) running")
			},
		}

		var stdout, stderr bytes.Buffer
		code := runRetirePayloadBodiesWithDeps(context.Background(), []string{
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", backupPath,
		}, &stdout, &stderr, deps)

		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "concurrent service detected") {
			t.Errorf("expected concurrent service error, got %q", stderr.String())
		}
	})

	t.Run("open files detected via hook", func(t *testing.T) {
		deps := &retirePayloadBodiesDeps{
			checkConcurrentService: func(path string) (int, error) {
				return 0, nil
			},
			checkOpenFiles: func(absPath string) (int, error) {
				return 54321, errors.New("process PID 54321 has file open")
			},
		}

		var stdout, stderr bytes.Buffer
		code := runRetirePayloadBodiesWithDeps(context.Background(), []string{
			"--db", dbPath,
			"--apply",
			"--offline",
			"--backup-file", backupPath,
		}, &stdout, &stderr, deps)

		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "target database file is opened by process") {
			t.Errorf("expected open file error, got %q", stderr.String())
		}
	})
}

func TestRetirePayloadBodies_DryRun(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()

	deps := &retirePayloadBodiesDeps{
		inspectPayloadStorage: func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error) {
			return mockInspectReport(), nil
		},
	}

	var stdout, stderr bytes.Buffer
	code := runRetirePayloadBodiesWithDeps(context.Background(), []string{
		"--db", dbPath,
		"--dry-run",
	}, &stdout, &stderr, deps)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}

	var rep RetirePayloadBodiesReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v, raw output: %s", err, stdout.String())
	}

	if rep.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", rep.Status)
	}
	if !rep.DryRun {
		t.Errorf("expected dry_run = true")
	}
	if rep.Storage == nil {
		t.Fatalf("expected storage report, got nil")
	}
	if rep.Storage.PayloadCount != 5 || rep.Storage.NonemptyBodyCount != 3 {
		t.Errorf("unexpected storage report values: %+v", rep.Storage)
	}
	if rep.QuickCheck != "ok" {
		t.Errorf("expected quick_check ok, got %s", rep.QuickCheck)
	}
	if rep.VacuumRequested {
		t.Errorf("expected vacuum_requested = false")
	}
	if !strings.Contains(rep.NamespaceWarning, "namespace") {
		t.Errorf("expected namespace warning in report, got %q", rep.NamespaceWarning)
	}
}

func TestRetirePayloadBodies_Apply(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()
	backupPath := createTestBackupFile(t, dbPath)

	clearedCalled := false
	deps := &retirePayloadBodiesDeps{
		clearPayloadBodies: func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error) {
			clearedCalled = true
			return mockClearReport(), nil
		},
		checkConcurrentService: func(path string) (int, error) {
			return 0, nil
		},
		checkOpenFiles: func(absPath string) (int, error) {
			return 0, nil
		},
	}

	var stdout, stderr bytes.Buffer
	code := runRetirePayloadBodiesWithDeps(context.Background(), []string{
		"--db", dbPath,
		"--apply",
		"--offline",
		"--backup-file", backupPath,
	}, &stdout, &stderr, deps)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	if !clearedCalled {
		t.Errorf("expected clearPayloadBodies to be called")
	}

	var rep RetirePayloadBodiesReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v", err)
	}

	if rep.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", rep.Status)
	}
	if rep.DryRun {
		t.Errorf("expected dry_run = false")
	}
	if !rep.Offline {
		t.Errorf("expected offline = true")
	}
	if rep.VacuumRequested {
		t.Errorf("expected vacuum_requested = false")
	}
	if rep.VacuumStatus != "SKIPPED" {
		t.Errorf("expected vacuum_status SKIPPED, got %s", rep.VacuumStatus)
	}
	if rep.Storage == nil || rep.Storage.ClearedBodyCount != 3 {
		t.Errorf("unexpected storage report: %+v", rep.Storage)
	}
	if rep.QuickCheck != "ok" {
		t.Errorf("expected quick_check ok, got %s", rep.QuickCheck)
	}
	if !strings.Contains(rep.ForeignKeyCheck, "ok") {
		t.Errorf("expected foreign_key_check ok, got %s", rep.ForeignKeyCheck)
	}
}

func TestRetirePayloadBodies_ApplyWithVacuum(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()
	backupPath := createTestBackupFile(t, dbPath)

	vacuumExecuted := false
	deps := &retirePayloadBodiesDeps{
		clearPayloadBodies: func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error) {
			return mockClearReport(), nil
		},
		checkConcurrentService: func(path string) (int, error) {
			return 0, nil
		},
		checkOpenFiles: func(absPath string) (int, error) {
			return 0, nil
		},
		vacuumDB: func(ctx context.Context, db *sql.DB) error {
			vacuumExecuted = true
			_, err := db.ExecContext(ctx, "VACUUM;")
			return err
		},
	}

	var stdout, stderr bytes.Buffer
	code := runRetirePayloadBodiesWithDeps(context.Background(), []string{
		"--db", dbPath,
		"--apply",
		"--offline",
		"--backup-file", backupPath,
		"--vacuum",
	}, &stdout, &stderr, deps)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	if !vacuumExecuted {
		t.Errorf("expected vacuumDB to be executed")
	}

	var rep RetirePayloadBodiesReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v", err)
	}

	if rep.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", rep.Status)
	}
	if !rep.VacuumRequested {
		t.Errorf("expected vacuum_requested = true")
	}
	if rep.VacuumStatus != "SUCCESS" {
		t.Errorf("expected vacuum_status SUCCESS, got %s", rep.VacuumStatus)
	}
}

func TestRetirePayloadBodies_ApplyWithVacuumFailure(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()
	backupPath := createTestBackupFile(t, dbPath)

	deps := &retirePayloadBodiesDeps{
		clearPayloadBodies: func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error) {
			return mockClearReport(), nil
		},
		checkConcurrentService: func(path string) (int, error) {
			return 0, nil
		},
		checkOpenFiles: func(absPath string) (int, error) {
			return 0, nil
		},
		vacuumDB: func(ctx context.Context, db *sql.DB) error {
			return errors.New("simulated disk full during vacuum")
		},
	}

	var stdout, stderr bytes.Buffer
	code := runRetirePayloadBodiesWithDeps(context.Background(), []string{
		"--db", dbPath,
		"--apply",
		"--offline",
		"--backup-file", backupPath,
		"--vacuum",
	}, &stdout, &stderr, deps)

	// Must return non-zero exit code on VACUUM failure
	if code != 1 {
		t.Fatalf("expected exit code 1 on vacuum failure, got %d", code)
	}

	// Must explicitly state in stderr that logical cleanup succeeded
	if !strings.Contains(stderr.String(), "logical cleanup was already committed successfully") {
		t.Errorf("expected stderr to explain logical cleanup succeeded, got %q", stderr.String())
	}

	var rep RetirePayloadBodiesReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v, raw output: %s", err, stdout.String())
	}

	if rep.Status != "FAILED" {
		t.Errorf("expected status FAILED, got %s", rep.Status)
	}
	if rep.VacuumStatus != "FAILED" {
		t.Errorf("expected vacuum_status FAILED, got %s", rep.VacuumStatus)
	}
	if !strings.Contains(rep.VacuumError, "simulated disk full during vacuum") {
		t.Errorf("expected vacuum error recorded, got %q", rep.VacuumError)
	}
	// Storage report from logical cleanup must still be included in the output
	if rep.Storage == nil || rep.Storage.ClearedBodyCount != 3 {
		t.Errorf("expected storage report intact in failed vacuum output, got: %+v", rep.Storage)
	}
}

func TestRetirePayloadBodies_IdempotentApply(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()
	backupPath := createTestBackupFile(t, dbPath)

	deps := &retirePayloadBodiesDeps{
		clearPayloadBodies: func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error) {
			return mockIdempotentClearReport(), nil
		},
		checkConcurrentService: func(path string) (int, error) {
			return 0, nil
		},
		checkOpenFiles: func(absPath string) (int, error) {
			return 0, nil
		},
	}

	var stdout, stderr bytes.Buffer
	code := runRetirePayloadBodiesWithDeps(context.Background(), []string{
		"--db", dbPath,
		"--apply",
		"--offline",
		"--backup-file", backupPath,
	}, &stdout, &stderr, deps)

	if code != 0 {
		t.Fatalf("expected exit code 0 for idempotent apply, got %d", code)
	}

	var rep RetirePayloadBodiesReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v", err)
	}

	if rep.Storage == nil || rep.Storage.ClearedBodyCount != 0 {
		t.Errorf("expected 0 cleared bodies on idempotent run, got %+v", rep.Storage)
	}
}

func TestRetirePayloadBodies_NoSecretsInReport(t *testing.T) {
	dbPath, db := setupTestPayloadDB(t)
	_ = db.Close()

	deps := &retirePayloadBodiesDeps{
		inspectPayloadStorage: func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error) {
			return mockInspectReport(), nil
		},
	}

	var stdout, stderr bytes.Buffer
	code := runRetirePayloadBodiesWithDeps(context.Background(), []string{
		"--db", dbPath,
		"--dry-run",
	}, &stdout, &stderr, deps)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	output := stdout.String()
	sensitiveKeywords := []string{"token", "password", "secret", "body_blob", "bearer"}
	for _, kw := range sensitiveKeywords {
		if strings.Contains(strings.ToLower(output), `"`+kw+`"`) {
			t.Errorf("report leaked sensitive keyword %q: %s", kw, output)
		}
	}
}
