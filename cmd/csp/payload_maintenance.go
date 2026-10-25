package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"clash-sub-parser/internal/repository/sqlite"
)

// NamespaceScopeNotice documents the explicit operational boundary of the CLI concurrency guards.
// Concurrency checks and open-file inspections only observe the current Linux host/container PID and
// mount namespace. In multi-container setups or separated namespace environments, external processes
// accessing the database file might not be visible. Operators must stop all external processes and
// containers accessing the target database before executing --apply.
const NamespaceScopeNotice = "Process and open file checks operate within the current mount and PID namespace. External processes running in separated container namespaces may not be detected. Ensure all external services accessing this database are stopped."

// RetirePayloadBodiesReport is the structured JSON output emitted by retire-payload-bodies.
// It reports before-and-after storage statistics, physical file sizes, vacuum status, and
// SQLite integrity check results. In accordance with security requirements, it contains
// zero credentials, tokens, or raw payload text.
type RetirePayloadBodiesReport struct {
	Status           string                       `json:"status"` // "SUCCESS", "DRY_RUN", or "FAILED"
	DBPath           string                       `json:"db_path"`
	DryRun           bool                         `json:"dry_run"`
	Offline          bool                         `json:"offline"`
	BackupFile       string                       `json:"backup_file,omitempty"`
	VacuumRequested  bool                         `json:"vacuum_requested"`
	VacuumStatus     string                       `json:"vacuum_status,omitempty"` // "SUCCESS", "FAILED", or "SKIPPED"
	VacuumError      string                       `json:"vacuum_error,omitempty"`
	BeforeFileBytes  int64                        `json:"before_file_bytes"`
	AfterFileBytes   int64                        `json:"after_file_bytes"`
	Storage          *sqlite.PayloadStorageReport `json:"storage,omitempty"`
	QuickCheck       string                       `json:"quick_check"`
	ForeignKeyCheck  string                       `json:"foreign_key_check"`
	NamespaceWarning string                       `json:"namespace_warning"`
	Message          string                       `json:"message"`
}

type retirePayloadBodiesDeps struct {
	inspectPayloadStorage  func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error)
	clearPayloadBodies     func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error)
	checkConcurrentService func(dbPath string) (int, error)
	checkOpenFiles         func(absDBPath string) (int, error)
	openReadOnly           func(path string) (*sql.DB, error)
	openReadWrite          func(cfg sqlite.Config) (*sql.DB, error)
	vacuumDB               func(ctx context.Context, db *sql.DB) error
}

func defaultRetirePayloadBodiesDeps() *retirePayloadBodiesDeps {
	return &retirePayloadBodiesDeps{
		inspectPayloadStorage:  sqlite.InspectPayloadStorage,
		clearPayloadBodies:     sqlite.ClearPayloadBodies,
		checkConcurrentService: checkConcurrentService,
		checkOpenFiles:         checkOpenDatabaseFiles,
		openReadOnly:           sqlite.OpenReadOnly,
		openReadWrite:          sqlite.Open,
		vacuumDB: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, "VACUUM;")
			return err
		},
	}
}

// checkOpenDatabaseFiles inspects /proc entries on Linux to detect whether any other process
// has the target database or its WAL/SHM companion files open.
func checkOpenDatabaseFiles(absDBPath string) (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		// Non-Linux platform or /proc not accessible; bounded check gracefully passes
		return 0, nil
	}

	currentPID := os.Getpid()
	targetPaths := map[string]bool{
		absDBPath:          true,
		absDBPath + "-wal": true,
		absDBPath + "-shm": true,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err != nil || pid <= 0 || pid == currentPID {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // Permission denied or process exited
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			cleanedLink := strings.TrimSuffix(link, " (deleted)")
			if targetPaths[cleanedLink] {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("detected process PID %d with target database file open (%s)", pid, link)
				}
			}
		}
	}
	return 0, nil
}

// validateBackupFile verifies that the provided backup file exists, is a regular file, is not
// the same file or a hard link to the target database, has a valid SQLite header, and passes quick_check.
func validateBackupFile(ctx context.Context, backupFile string, targetDBFi os.FileInfo, absDBPath string, openRO func(path string) (*sql.DB, error)) error {
	cleanBackup := filepath.Clean(backupFile)
	absBackup, err := filepath.Abs(cleanBackup)
	if err != nil {
		absBackup = cleanBackup
	}

	// 1. Path equality check
	if absBackup == absDBPath {
		return fmt.Errorf("backup file %q cannot be identical to target database %q", backupFile, absDBPath)
	}

	// 2. Stat and regular file check
	fiBackup, err := os.Stat(cleanBackup)
	if err != nil {
		return fmt.Errorf("backup file %q not accessible: %w", backupFile, err)
	}
	if fiBackup.IsDir() {
		return fmt.Errorf("backup file %q is a directory, expected regular file", backupFile)
	}
	if !fiBackup.Mode().IsRegular() {
		return fmt.Errorf("backup file %q is not a regular file", backupFile)
	}

	// 3. Same file / inode / hard link rejection
	if os.SameFile(targetDBFi, fiBackup) {
		return fmt.Errorf("backup file %q is identical to target database %q (same file or hard link rejected)", backupFile, absDBPath)
	}
	statDB, okDB := targetDBFi.Sys().(*syscall.Stat_t)
	statBackup, okBackup := fiBackup.Sys().(*syscall.Stat_t)
	if okDB && okBackup && statDB.Dev == statBackup.Dev && statDB.Ino == statBackup.Ino {
		return fmt.Errorf("backup file %q shares device and inode with target database %q (hard link rejected)", backupFile, absDBPath)
	}

	// 4. SQLite format 3 magic header verification
	bf, err := os.Open(cleanBackup)
	if err != nil {
		return fmt.Errorf("cannot open backup file %q: %w", backupFile, err)
	}
	hdr := make([]byte, 16)
	n, readErr := bf.Read(hdr)
	_ = bf.Close()
	if readErr != nil || n < 16 || string(hdr[:15]) != "SQLite format 3" {
		return fmt.Errorf("backup file %q is not a valid SQLite database (header mismatch)", backupFile)
	}

	// 5. SQLite quick_check on backup
	bDB, err := openRO(cleanBackup)
	if err != nil {
		return fmt.Errorf("cannot open backup file %q for integrity verification: %w", backupFile, err)
	}
	defer bDB.Close()

	var qc string
	if err := bDB.QueryRowContext(ctx, "PRAGMA quick_check;").Scan(&qc); err != nil {
		return fmt.Errorf("backup file %q quick_check query failed: %w", backupFile, err)
	}
	if qc != "ok" {
		return fmt.Errorf("backup file %q quick_check failed with: %s", backupFile, qc)
	}

	return nil
}

// verifySQLiteIntegrity runs PRAGMA quick_check and PRAGMA foreign_key_check.
func verifySQLiteIntegrity(ctx context.Context, db *sql.DB) (string, string, error) {
	var quickCheck string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check;").Scan(&quickCheck); err != nil {
		return "", "", fmt.Errorf("quick_check failed: %w", err)
	}
	if quickCheck != "ok" {
		return quickCheck, "", fmt.Errorf("quick_check reported corruption: %s", quickCheck)
	}

	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return quickCheck, "", fmt.Errorf("foreign_key_check query failed: %w", err)
	}
	defer rows.Close()

	fkViolations := 0
	var violationDetails []string
	for rows.Next() {
		var table, parent string
		var rowid int64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err == nil {
			fkViolations++
			if fkViolations <= 5 {
				violationDetails = append(violationDetails, fmt.Sprintf("table %s row %d -> %s", table, rowid, parent))
			}
		}
	}
	if fkViolations > 0 {
		fkCheck := fmt.Sprintf("%d violations (%s)", fkViolations, strings.Join(violationDetails, "; "))
		return quickCheck, fkCheck, fmt.Errorf("foreign_key_check found %d violations: %s", fkViolations, fkCheck)
	}

	return quickCheck, "ok (0 violations)", nil
}

// runRetirePayloadBodies executes the retire-payload-bodies CLI command.
func runRetirePayloadBodies(args []string, stdout, stderr io.Writer) int {
	return runRetirePayloadBodiesWithDeps(context.Background(), args, stdout, stderr, nil)
}

func runRetirePayloadBodiesWithDeps(ctx context.Context, args []string, stdout, stderr io.Writer, deps *retirePayloadBodiesDeps) int {
	if deps == nil {
		deps = defaultRetirePayloadBodiesDeps()
	} else {
		// Populate any nil hooks with defaults
		defaults := defaultRetirePayloadBodiesDeps()
		if deps.inspectPayloadStorage == nil {
			deps.inspectPayloadStorage = defaults.inspectPayloadStorage
		}
		if deps.clearPayloadBodies == nil {
			deps.clearPayloadBodies = defaults.clearPayloadBodies
		}
		if deps.checkConcurrentService == nil {
			deps.checkConcurrentService = defaults.checkConcurrentService
		}
		if deps.checkOpenFiles == nil {
			deps.checkOpenFiles = defaults.checkOpenFiles
		}
		if deps.openReadOnly == nil {
			deps.openReadOnly = defaults.openReadOnly
		}
		if deps.openReadWrite == nil {
			deps.openReadWrite = defaults.openReadWrite
		}
		if deps.vacuumDB == nil {
			deps.vacuumDB = defaults.vacuumDB
		}
	}

	fs := flag.NewFlagSet("retire-payload-bodies", flag.ContinueOnError)
	fs.SetOutput(stderr)

	defaultDBPath := os.Getenv("CSP_DB_PATH")
	if defaultDBPath == "" {
		defaultDBPath = os.Getenv("DB_PATH")
	}
	if defaultDBPath == "" {
		defaultDBPath = "/data/csp-v1.db"
	}

	var (
		dbPath     string
		dryRun     bool
		apply      bool
		offline    bool
		backupFile string
		vacuum     bool
	)

	fs.StringVar(&dbPath, "db", defaultDBPath, "path to target SQLite database file")
	fs.StringVar(&dbPath, "d", defaultDBPath, "path to target SQLite database file (shorthand)")
	fs.BoolVar(&dryRun, "dry-run", false, "dry run without modifying database (inspects non-empty body counts and bytes)")
	fs.BoolVar(&apply, "apply", false, "apply payload body retirement (zeroes body_blob in an atomic transaction)")
	fs.BoolVar(&offline, "offline", false, "explicit assertion that the database is offline and no active processes are accessing it")
	fs.StringVar(&backupFile, "backup-file", "", "path to verified SQLite backup file before executing apply")
	fs.BoolVar(&vacuum, "vacuum", false, "execute physical VACUUM after clearing payload bodies (only valid with --apply and --offline)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	// 1. Mutual exclusion and required flag validation
	if !dryRun && !apply {
		fmt.Fprintf(stderr, "retire-payload-bodies: either --dry-run or --apply must be explicitly specified (fail-safe)\n")
		return 1
	}
	if dryRun && apply {
		fmt.Fprintf(stderr, "retire-payload-bodies: cannot specify both --dry-run and --apply\n")
		return 1
	}
	if vacuum && !apply {
		fmt.Fprintf(stderr, "retire-payload-bodies: --vacuum is only permitted with --apply and --offline\n")
		return 1
	}
	if apply && !offline {
		fmt.Fprintf(stderr, "retire-payload-bodies: apply requires --offline to ensure no concurrent writers\n")
		return 1
	}
	if apply && strings.TrimSpace(backupFile) == "" {
		fmt.Fprintf(stderr, "retire-payload-bodies: apply requires verified backup file; pass --backup-file <path>\n")
		return 1
	}

	// 2. Target database file validation
	cleanDBPath := filepath.Clean(dbPath)
	absDBPath, err := filepath.Abs(cleanDBPath)
	if err != nil {
		absDBPath = cleanDBPath
	}

	fi, err := os.Stat(cleanDBPath)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: target database %q not found: %v\n", dbPath, err)
		return 1
	}
	if fi.IsDir() {
		fmt.Fprintf(stderr, "retire-payload-bodies: target database %q is a directory\n", dbPath)
		return 1
	}
	if !fi.Mode().IsRegular() {
		fmt.Fprintf(stderr, "retire-payload-bodies: target database %q is not a regular file\n", dbPath)
		return 1
	}

	// 3. Apply mode pre-flight validations (backup, concurrency, and exclusive lock)
	if apply {
		if err := validateBackupFile(ctx, backupFile, fi, absDBPath, deps.openReadOnly); err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: backup verification failed: %v\n", err)
			return 1
		}

		// Concurrency check via cmdline/pid files
		if pid, err := deps.checkConcurrentService(cleanDBPath); err != nil || pid > 0 {
			fmt.Fprintf(stderr, "retire-payload-bodies: pre-condition failed: concurrent service detected (PID %d): %v\n", pid, err)
			return 1
		}

		// Concurrency check via open file descriptors in current namespace
		if openPID, err := deps.checkOpenFiles(absDBPath); err != nil || openPID > 0 {
			fmt.Fprintf(stderr, "retire-payload-bodies: pre-condition failed: target database file is opened by process (PID %d): %v\n", openPID, err)
			return 1
		}

		// Exclusive maintenance lock acquisition
		lockFile := cleanDBPath + ".maintenance.lock"
		lf, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			if os.IsExist(err) {
				fmt.Fprintf(stderr, "retire-payload-bodies: target database %s is locked by another maintenance run (lock file %s exists)\n", cleanDBPath, lockFile)
				return 1
			}
			fmt.Fprintf(stderr, "retire-payload-bodies: failed to acquire maintenance lock file %s: %v\n", lockFile, err)
			return 1
		}
		fmt.Fprintf(lf, "%d\n", os.Getpid())
		_ = lf.Close()
		defer os.Remove(lockFile)
	}

	// 4. Execution
	if dryRun {
		db, err := deps.openReadOnly(cleanDBPath)
		if err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: database connection failed: %v\n", err)
			return 1
		}
		defer db.Close()

		qc, fk, err := verifySQLiteIntegrity(ctx, db)
		if err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: pre-flight integrity check failed: %v\n", err)
			return 1
		}

		storageReport, err := deps.inspectPayloadStorage(ctx, db)
		if err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: inspect payload storage failed: %v\n", err)
			return 1
		}

		beforeBytes := fi.Size()
		cliReport := RetirePayloadBodiesReport{
			Status:           "SUCCESS",
			DBPath:           cleanDBPath,
			DryRun:           true,
			Offline:          false,
			BeforeFileBytes:  beforeBytes,
			AfterFileBytes:   beforeBytes,
			Storage:          storageReport,
			QuickCheck:       qc,
			ForeignKeyCheck:  fk,
			NamespaceWarning: NamespaceScopeNotice,
			Message:          "dry-run verification completed: no changes made to database",
		}

		outJSON, _ := json.MarshalIndent(cliReport, "", "  ")
		fmt.Fprintln(stdout, string(outJSON))
		return 0
	}

	// Apply mode: verify target DB pre-flight integrity read-only first
	preDB, err := deps.openReadOnly(cleanDBPath)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: failed to open target database for pre-flight check: %v\n", err)
		return 1
	}
	preQC, preFK, err := verifySQLiteIntegrity(ctx, preDB)
	_ = preDB.Close()
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: target database pre-flight integrity check failed: %v\n", err)
		return 1
	}

	beforeBytes := fi.Size()

	// Open target DB in read-write mode (does NOT invoke migrations)
	dbCfg := sqlite.DefaultConfig(cleanDBPath)
	db, err := deps.openReadWrite(dbCfg)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: failed to open target database for writing: %v\n", err)
		return 1
	}
	defer db.Close()

	// Execute logical clear transaction
	storageReport, err := deps.clearPayloadBodies(ctx, db)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: clear payload bodies failed: %v\n", err)
		return 1
	}

	// Verify integrity immediately after logical commit
	postQC, postFK, err := verifySQLiteIntegrity(ctx, db)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: post-clear integrity check failed: %v\n", err)
		return 1
	}

	vacuumStatus := "SKIPPED"
	var vacuumErr string
	if vacuum {
		vacErr := deps.vacuumDB(ctx, db)
		if vacErr != nil {
			vacuumStatus = "FAILED"
			vacuumErr = vacErr.Error()
			fmt.Fprintf(stderr, "retire-payload-bodies: VACUUM failed: %v (note: logical cleanup was already committed successfully; database is consistent but physical disk space was not reclaimed)\n", vacErr)

			// Still verify integrity even after failed vacuum
			qcAfter, fkAfter, _ := verifySQLiteIntegrity(ctx, db)
			afterFi, _ := os.Stat(cleanDBPath)
			afterBytes := beforeBytes
			if afterFi != nil {
				afterBytes = afterFi.Size()
			}

			cliReport := RetirePayloadBodiesReport{
				Status:           "FAILED",
				DBPath:           cleanDBPath,
				DryRun:           false,
				Offline:          offline,
				BackupFile:       backupFile,
				VacuumRequested:  true,
				VacuumStatus:     vacuumStatus,
				VacuumError:      vacuumErr,
				BeforeFileBytes:  beforeBytes,
				AfterFileBytes:   afterBytes,
				Storage:          storageReport,
				QuickCheck:       qcAfter,
				ForeignKeyCheck:  fkAfter,
				NamespaceWarning: NamespaceScopeNotice,
				Message:          fmt.Sprintf("logical cleanup committed, but physical VACUUM failed: %v", vacErr),
			}
			outJSON, _ := json.MarshalIndent(cliReport, "", "  ")
			fmt.Fprintln(stdout, string(outJSON))
			return 1 // non-zero exit code as required by spec
		}

		vacuumStatus = "SUCCESS"
		postQC, postFK, err = verifySQLiteIntegrity(ctx, db)
		if err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: post-vacuum integrity check failed: %v\n", err)
			return 1
		}
	}

	afterFi, _ := os.Stat(cleanDBPath)
	afterBytes := beforeBytes
	if afterFi != nil {
		afterBytes = afterFi.Size()
	}

	cliReport := RetirePayloadBodiesReport{
		Status:           "SUCCESS",
		DBPath:           cleanDBPath,
		DryRun:           false,
		Offline:          offline,
		BackupFile:       backupFile,
		VacuumRequested:  vacuum,
		VacuumStatus:     vacuumStatus,
		BeforeFileBytes:  beforeBytes,
		AfterFileBytes:   afterBytes,
		Storage:          storageReport,
		QuickCheck:       postQC,
		ForeignKeyCheck:  postFK,
		NamespaceWarning: NamespaceScopeNotice,
		Message:          "payload bodies retired successfully",
	}

	outJSON, _ := json.MarshalIndent(cliReport, "", "  ")
	fmt.Fprintln(stdout, string(outJSON))
	_ = preQC
	_ = preFK
	return 0
}
