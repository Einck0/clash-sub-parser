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
// It reports before-and-after storage statistics, physical file sizes (distinguishing main database
// and WAL files), vacuum status, and SQLite integrity check results. In accordance with security
// requirements, it contains zero credentials, tokens, or raw payload text.
type RetirePayloadBodiesReport struct {
	Status               string                       `json:"status"` // "SUCCESS", "DRY_RUN", or "FAILED"
	DBPath               string                       `json:"db_path"`
	CanonicalDBPath      string                       `json:"canonical_db_path,omitempty"`
	DryRun               bool                         `json:"dry_run"`
	Offline              bool                         `json:"offline"`
	BackupFile           string                       `json:"backup_file,omitempty"`
	VacuumRequested      bool                         `json:"vacuum_requested"`
	VacuumStatus         string                       `json:"vacuum_status,omitempty"` // "SUCCESS", "FAILED", or "SKIPPED"
	VacuumError          string                       `json:"vacuum_error,omitempty"`
	BeforeFileBytes      int64                        `json:"before_file_bytes"`
	AfterFileBytes       int64                        `json:"after_file_bytes"`
	BeforeWALBytes       int64                        `json:"before_wal_bytes"`
	AfterWALBytes        int64                        `json:"after_wal_bytes"`
	AfterVacuumFileBytes int64                        `json:"after_vacuum_file_bytes,omitempty"`
	Storage              *sqlite.PayloadStorageReport `json:"storage,omitempty"`
	QuickCheck           string                       `json:"quick_check"`
	ForeignKeyCheck      string                       `json:"foreign_key_check"`
	NamespaceWarning     string                       `json:"namespace_warning"`
	Message              string                       `json:"message"`
}

type retirePayloadBodiesDeps struct {
	inspectPayloadStorage  func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error)
	clearPayloadBodies     func(ctx context.Context, db *sql.DB) (*sqlite.PayloadStorageReport, error)
	checkConcurrentService func(dbPath string) (int, error)
	checkOpenFiles         func(canonicalDBPath string) (int, error)
	openReadOnly           func(path string) (*sql.DB, error)
	openReadWrite          func(cfg sqlite.Config) (*sql.DB, error)
	vacuumDB               func(ctx context.Context, db *sql.DB) error
	closeDB                func(db *sql.DB) error
	verifyIntegrity        func(ctx context.Context, db *sql.DB) (string, string, error)
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
		closeDB: func(db *sql.DB) error {
			if db != nil {
				return db.Close()
			}
			return nil
		},
		verifyIntegrity: verifySQLiteIntegrity,
	}
}

// getFileSizes returns the size of the database file and its companion WAL file if present.
func getFileSizes(dbPath string) (mainBytes int64, walBytes int64) {
	if fi, err := os.Stat(dbPath); err == nil && fi.Mode().IsRegular() {
		mainBytes = fi.Size()
	}
	if fi, err := os.Stat(dbPath + "-wal"); err == nil && fi.Mode().IsRegular() {
		walBytes = fi.Size()
	}
	return mainBytes, walBytes
}

// checkOpenDatabaseFiles inspects /proc entries on Linux to detect whether any other process
// has the target database or its WAL/SHM companion files open via file identity (os.SameFile / Inode).
func checkOpenDatabaseFiles(canonicalDBPath string) (int, error) {
	targetFi, err := os.Stat(canonicalDBPath)
	if err != nil {
		return 0, fmt.Errorf("cannot stat target database %q: %w", canonicalDBPath, err)
	}

	var walFi os.FileInfo
	if fi, err := os.Stat(canonicalDBPath + "-wal"); err == nil && fi.Mode().IsRegular() {
		walFi = fi
	}

	var shmFi os.FileInfo
	if fi, err := os.Stat(canonicalDBPath + "-shm"); err == nil && fi.Mode().IsRegular() {
		shmFi = fi
	}

	targetStat, okTargetStat := targetFi.Sys().(*syscall.Stat_t)
	var walStat, shmStat *syscall.Stat_t
	if walFi != nil {
		walStat, _ = walFi.Sys().(*syscall.Stat_t)
	}
	if shmFi != nil {
		shmStat, _ = shmFi.Sys().(*syscall.Stat_t)
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		if os.IsNotExist(err) {
			// Non-Linux or non-procfs environment
			return 0, nil
		}
		return 0, fmt.Errorf("cannot inspect /proc for open files (permission or traversal error): %w", err)
	}

	currentPID := os.Getpid()
	targetPaths := map[string]bool{
		canonicalDBPath:          true,
		canonicalDBPath + "-wal": true,
		canonicalDBPath + "-shm": true,
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
			// Process exited between /proc listing and reading fdDir: ENOENT is expected and skipped
			if os.IsNotExist(err) {
				continue
			}
			// Unreadable or permission error: fail closed
			return pid, fmt.Errorf("cannot inspect file descriptors for process %d (permission or access error): %w", pid, err)
		}

		for _, fd := range fds {
			fdPath := filepath.Join(fdDir, fd.Name())

			// 1. Identity match via os.Stat (follows symlink to underlying open file)
			fdFi, statErr := os.Stat(fdPath)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					// The open fd might point to a deleted file, socket, or pipe where os.Stat returns ENOENT.
					// Fall back to os.Readlink to verify text link
					link, linkErr := os.Readlink(fdPath)
					if linkErr == nil {
						cleanedLink := strings.TrimSuffix(link, " (deleted)")
						if targetPaths[cleanedLink] {
							if isProcessAlive(pid) {
								return pid, fmt.Errorf("detected process PID %d with target database file open (%s)", pid, link)
							}
						}
					}
					continue
				}
				if os.IsPermission(statErr) {
					return pid, fmt.Errorf("permission denied reading target of file descriptor %s for process %d: %w", fdPath, pid, statErr)
				}
				continue
			}

			// os.SameFile comparison against database, WAL, and SHM files
			if os.SameFile(targetFi, fdFi) {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("detected process PID %d with target database file open (identity match via fd %s)", pid, fd.Name())
				}
			}
			if walFi != nil && os.SameFile(walFi, fdFi) {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("detected process PID %d with target database WAL file open (identity match via fd %s)", pid, fd.Name())
				}
			}
			if shmFi != nil && os.SameFile(shmFi, fdFi) {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("detected process PID %d with target database SHM file open (identity match via fd %s)", pid, fd.Name())
				}
			}

			// Direct dev/ino comparison for hard links or aliases
			fdStat, okFdStat := fdFi.Sys().(*syscall.Stat_t)
			if okTargetStat && okFdStat && targetStat.Dev == fdStat.Dev && targetStat.Ino == fdStat.Ino {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("detected process PID %d with target database file open (inode match %d via fd %s)", pid, targetStat.Ino, fd.Name())
				}
			}
			if walStat != nil && okFdStat && walStat.Dev == fdStat.Dev && walStat.Ino == fdStat.Ino {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("detected process PID %d with target database WAL file open (inode match via fd %s)", pid, fd.Name())
				}
			}
			if shmStat != nil && okFdStat && shmStat.Dev == fdStat.Dev && shmStat.Ino == fdStat.Ino {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("detected process PID %d with target database SHM file open (inode match via fd %s)", pid, fd.Name())
				}
			}

			// 2. Text path check via os.Readlink as secondary protection
			link, linkErr := os.Readlink(fdPath)
			if linkErr == nil {
				cleanedLink := strings.TrimSuffix(link, " (deleted)")
				if targetPaths[cleanedLink] {
					if isProcessAlive(pid) {
						return pid, fmt.Errorf("detected process PID %d with target database file open (%s)", pid, link)
					}
				}
			}
		}
	}
	return 0, nil
}

// validateBackupFile verifies that the provided backup file exists, is a regular file, is not
// the same file or a hard link to the target database, has a valid SQLite header, and passes quick_check.
func validateBackupFile(ctx context.Context, backupFile string, targetDBFi os.FileInfo, canonicalDBPath string, openRO func(path string) (*sql.DB, error)) error {
	cleanBackup := filepath.Clean(backupFile)
	absBackup, err := filepath.Abs(cleanBackup)
	if err != nil {
		absBackup = cleanBackup
	}
	canonicalBackup, err := filepath.EvalSymlinks(cleanBackup)
	if err == nil {
		if absCanonical, err := filepath.Abs(canonicalBackup); err == nil {
			canonicalBackup = absCanonical
		}
	} else {
		canonicalBackup = absBackup
	}

	// 1. Path equality check (both absolute and canonical paths)
	if absBackup == canonicalDBPath || canonicalBackup == canonicalDBPath {
		return fmt.Errorf("backup file %q cannot be identical to target database %q", backupFile, canonicalDBPath)
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
		return fmt.Errorf("backup file %q is identical to target database %q (same file or hard link rejected)", backupFile, canonicalDBPath)
	}
	statDB, okDB := targetDBFi.Sys().(*syscall.Stat_t)
	statBackup, okBackup := fiBackup.Sys().(*syscall.Stat_t)
	if okDB && okBackup && statDB.Dev == statBackup.Dev && statDB.Ino == statBackup.Ino {
		return fmt.Errorf("backup file %q shares device and inode with target database %q (hard link rejected)", backupFile, canonicalDBPath)
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
// Any returned row from foreign_key_check indicates a violation.
// Scanning uses nullable types to support WITHOUT ROWID tables (where rowid is NULL).
// Any row scan error or rows.Err() fails closed.
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
		fkViolations++
		var table, parent sql.NullString
		var rowid sql.NullInt64
		var fkid sql.NullInt64
		if scanErr := rows.Scan(&table, &rowid, &parent, &fkid); scanErr != nil {
			// Scan error fails closed immediately
			errMsg := fmt.Sprintf("foreign_key_check scan failed on violation row %d: %v", fkViolations, scanErr)
			return quickCheck, fmt.Sprintf("%d violations (%s)", fkViolations, errMsg), errors.New(errMsg)
		}
		if len(violationDetails) < 5 {
			rowStr := "NULL"
			if rowid.Valid {
				rowStr = fmt.Sprintf("%d", rowid.Int64)
			}
			tblStr := "unknown"
			if table.Valid {
				tblStr = table.String
			}
			parentStr := "unknown"
			if parent.Valid {
				parentStr = parent.String
			}
			violationDetails = append(violationDetails, fmt.Sprintf("table %s row %s -> %s", tblStr, rowStr, parentStr))
		}
	}
	if err := rows.Err(); err != nil {
		return quickCheck, "", fmt.Errorf("foreign_key_check row iteration failed: %w", err)
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
		if deps.closeDB == nil {
			deps.closeDB = defaults.closeDB
		}
		if deps.verifyIntegrity == nil {
			deps.verifyIntegrity = defaults.verifyIntegrity
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

	// 2. Canonical target database path and file validation
	cleanDBPath := filepath.Clean(dbPath)
	canonicalDBPath, err := filepath.EvalSymlinks(cleanDBPath)
	if err != nil {
		canonicalDBPath = cleanDBPath
	}
	if absCanonical, err := filepath.Abs(canonicalDBPath); err == nil {
		canonicalDBPath = absCanonical
	}

	fi, err := os.Stat(canonicalDBPath)
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

	// 3. Apply mode pre-flight validations (backup, concurrency, and exclusive lock on canonical path)
	if apply {
		if err := validateBackupFile(ctx, backupFile, fi, canonicalDBPath, deps.openReadOnly); err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: backup verification failed: %v\n", err)
			return 1
		}

		// Concurrency check via cmdline/pid files on canonical path
		if pid, err := deps.checkConcurrentService(canonicalDBPath); err != nil || pid > 0 {
			fmt.Fprintf(stderr, "retire-payload-bodies: pre-condition failed: concurrent service detected (PID %d): %v\n", pid, err)
			return 1
		}

		// Concurrency check via open file descriptors in current namespace
		if openPID, err := deps.checkOpenFiles(canonicalDBPath); err != nil || openPID > 0 {
			fmt.Fprintf(stderr, "retire-payload-bodies: pre-condition failed: target database file is opened by process (PID %d): %v\n", openPID, err)
			return 1
		}

		// Exclusive maintenance lock acquisition on canonical DB path to avoid symlink alias lock bypass
		lockFile := canonicalDBPath + ".maintenance.lock"
		lf, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			if os.IsExist(err) {
				fmt.Fprintf(stderr, "retire-payload-bodies: target database %s is locked by another maintenance run (lock file %s exists)\n", canonicalDBPath, lockFile)
				return 1
			}
			fmt.Fprintf(stderr, "retire-payload-bodies: failed to acquire maintenance lock file %s: %v\n", lockFile, err)
			return 1
		}
		fmt.Fprintf(lf, "%d\n", os.Getpid())
		_ = lf.Close()
		defer os.Remove(lockFile)
	}

	beforeMainBytes, beforeWALBytes := getFileSizes(canonicalDBPath)

	// 4. Execution
	if dryRun {
		db, err := deps.openReadOnly(canonicalDBPath)
		if err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: database connection failed: %v\n", err)
			return 1
		}
		defer db.Close()

		qc, fk, err := deps.verifyIntegrity(ctx, db)
		if err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: pre-flight integrity check failed: %v\n", err)
			return 1
		}

		storageReport, err := deps.inspectPayloadStorage(ctx, db)
		if err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: inspect payload storage failed: %v\n", err)
			return 1
		}

		cliReport := RetirePayloadBodiesReport{
			Status:           "SUCCESS",
			DBPath:           cleanDBPath,
			CanonicalDBPath:  canonicalDBPath,
			DryRun:           true,
			Offline:          false,
			BeforeFileBytes:  beforeMainBytes,
			AfterFileBytes:   beforeMainBytes,
			BeforeWALBytes:   beforeWALBytes,
			AfterWALBytes:    beforeWALBytes,
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
	preDB, err := deps.openReadOnly(canonicalDBPath)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: failed to open target database for pre-flight check: %v\n", err)
		return 1
	}
	preQC, preFK, err := deps.verifyIntegrity(ctx, preDB)
	_ = preDB.Close()
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: target database pre-flight integrity check failed: %v\n", err)
		return 1
	}

	// Open target DB in read-write mode (does NOT invoke migrations)
	dbCfg := sqlite.DefaultConfig(canonicalDBPath)
	db, err := deps.openReadWrite(dbCfg)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: failed to open target database for writing: %v\n", err)
		return 1
	}

	var closedDB bool
	closeDBOnce := func() {
		if !closedDB {
			closedDB = true
			_ = deps.closeDB(db)
		}
	}
	defer closeDBOnce()

	// Execute logical clear transaction
	storageReport, err := deps.clearPayloadBodies(ctx, db)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: clear payload bodies failed: %v\n", err)
		return 1
	}

	// Verify integrity immediately after logical commit
	postQC, postFK, err := deps.verifyIntegrity(ctx, db)
	if err != nil {
		fmt.Fprintf(stderr, "retire-payload-bodies: post-clear integrity check failed: %v (note: logical cleanup was already committed successfully; database is consistent)\n", err)
		_, _ = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE);")
		closeDBOnce()
		afterMainBytes, afterWALBytes := getFileSizes(canonicalDBPath)

		cliReport := RetirePayloadBodiesReport{
			Status:           "FAILED",
			DBPath:           cleanDBPath,
			CanonicalDBPath:  canonicalDBPath,
			DryRun:           false,
			Offline:          offline,
			BackupFile:       backupFile,
			VacuumRequested:  vacuum,
			VacuumStatus:     "SKIPPED",
			BeforeFileBytes:  beforeMainBytes,
			AfterFileBytes:   afterMainBytes,
			BeforeWALBytes:   beforeWALBytes,
			AfterWALBytes:    afterWALBytes,
			Storage:          storageReport,
			QuickCheck:       postQC,
			ForeignKeyCheck:  postFK,
			NamespaceWarning: NamespaceScopeNotice,
			Message:          fmt.Sprintf("logical cleanup committed successfully, but post-clear integrity check failed: %v", err),
		}
		outJSON, _ := json.MarshalIndent(cliReport, "", "  ")
		fmt.Fprintln(stdout, string(outJSON))
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
			qcAfter, fkAfter, _ := deps.verifyIntegrity(ctx, db)
			_, _ = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE);")
			closeDBOnce()
			afterMainBytes, afterWALBytes := getFileSizes(canonicalDBPath)

			cliReport := RetirePayloadBodiesReport{
				Status:           "FAILED",
				DBPath:           cleanDBPath,
				CanonicalDBPath:  canonicalDBPath,
				DryRun:           false,
				Offline:          offline,
				BackupFile:       backupFile,
				VacuumRequested:  true,
				VacuumStatus:     vacuumStatus,
				VacuumError:      vacuumErr,
				BeforeFileBytes:  beforeMainBytes,
				AfterFileBytes:   afterMainBytes,
				BeforeWALBytes:   beforeWALBytes,
				AfterWALBytes:    afterWALBytes,
				Storage:          storageReport,
				QuickCheck:       qcAfter,
				ForeignKeyCheck:  fkAfter,
				NamespaceWarning: NamespaceScopeNotice,
				Message:          fmt.Sprintf("logical cleanup committed successfully, but physical VACUUM failed: %v", vacErr),
			}
			outJSON, _ := json.MarshalIndent(cliReport, "", "  ")
			fmt.Fprintln(stdout, string(outJSON))
			return 1 // non-zero exit code as required by spec
		}

		vacuumStatus = "SUCCESS"
		postQC, postFK, err = deps.verifyIntegrity(ctx, db)
		if err != nil {
			fmt.Fprintf(stderr, "retire-payload-bodies: post-vacuum integrity check failed: %v (note: logical cleanup was already committed successfully)\n", err)
			_, _ = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE);")
			closeDBOnce()
			afterMainBytes, afterWALBytes := getFileSizes(canonicalDBPath)

			cliReport := RetirePayloadBodiesReport{
				Status:               "FAILED",
				DBPath:               cleanDBPath,
				CanonicalDBPath:      canonicalDBPath,
				DryRun:               false,
				Offline:              offline,
				BackupFile:           backupFile,
				VacuumRequested:      true,
				VacuumStatus:         vacuumStatus,
				BeforeFileBytes:      beforeMainBytes,
				AfterFileBytes:       afterMainBytes,
				BeforeWALBytes:       beforeWALBytes,
				AfterWALBytes:        afterWALBytes,
				AfterVacuumFileBytes: afterMainBytes,
				Storage:              storageReport,
				QuickCheck:           postQC,
				ForeignKeyCheck:      postFK,
				NamespaceWarning:     NamespaceScopeNotice,
				Message:              fmt.Sprintf("logical cleanup committed successfully, but post-vacuum integrity check failed: %v", err),
			}
			outJSON, _ := json.MarshalIndent(cliReport, "", "  ")
			fmt.Fprintln(stdout, string(outJSON))
			return 1
		}
	}

	// Checkpoint WAL and close connection to capture final confirmed disk sizes
	_, _ = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE);")
	closeDBOnce()

	afterMainBytes, afterWALBytes := getFileSizes(canonicalDBPath)
	var afterVacuumBytes int64
	if vacuum {
		afterVacuumBytes = afterMainBytes
	}

	cliReport := RetirePayloadBodiesReport{
		Status:               "SUCCESS",
		DBPath:               cleanDBPath,
		CanonicalDBPath:      canonicalDBPath,
		DryRun:               false,
		Offline:              offline,
		BackupFile:           backupFile,
		VacuumRequested:      vacuum,
		VacuumStatus:         vacuumStatus,
		BeforeFileBytes:      beforeMainBytes,
		AfterFileBytes:       afterMainBytes,
		BeforeWALBytes:       beforeWALBytes,
		AfterWALBytes:        afterWALBytes,
		AfterVacuumFileBytes: afterVacuumBytes,
		Storage:              storageReport,
		QuickCheck:           postQC,
		ForeignKeyCheck:      postFK,
		NamespaceWarning:     NamespaceScopeNotice,
		Message:              "payload bodies retired successfully",
	}

	outJSON, _ := json.MarshalIndent(cliReport, "", "  ")
	fmt.Fprintln(stdout, string(outJSON))
	_ = preQC
	_ = preFK
	return 0
}
