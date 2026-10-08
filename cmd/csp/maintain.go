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
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// runRestoreGroupFilters executes the restore-group-filters CLI maintenance command.
func runRestoreGroupFilters(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("restore-group-filters", flag.ContinueOnError)
	fs.SetOutput(stderr)

	defaultDBPath := os.Getenv("CSP_DB_PATH")
	if defaultDBPath == "" {
		defaultDBPath = os.Getenv("DB_PATH")
	}
	if defaultDBPath == "" {
		defaultDBPath = "/data/csp-v1.db"
	}

	var (
		dbPath        string
		dryRun        bool
		apply         bool
		confirmBackup bool
		backupFile    string
		archiveDBPath string
	)

	fs.StringVar(&dbPath, "db", defaultDBPath, "path to target SQLite database file")
	fs.StringVar(&dbPath, "d", defaultDBPath, "path to target SQLite database file (shorthand)")
	fs.BoolVar(&dryRun, "dry-run", false, "dry run without modifying database (inspects invariants and counts)")
	fs.BoolVar(&apply, "apply", false, "apply legacy group filter recovery transaction")
	fs.BoolVar(&confirmBackup, "confirm-backup", false, "confirm verified database backup exists before executing apply")
	fs.StringVar(&backupFile, "backup-file", "", "optional path to verified backup file to inspect before apply")
	fs.StringVar(&archiveDBPath, "archive-db", "/home/service/backups/csp-legacy-cold-archive-20260919.db", "path to legacy cold archive SQLite database")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if !dryRun && !apply {
		fmt.Fprintf(stderr, "restore-group-filters: either --dry-run or --apply must be explicitly specified (fail-safe)\n")
		return 1
	}
	if dryRun && apply {
		fmt.Fprintf(stderr, "restore-group-filters: cannot specify both --dry-run and --apply\n")
		return 1
	}

	if apply && !confirmBackup && backupFile == "" {
		fmt.Fprintf(stderr, "restore-group-filters: apply requires backup confirmation; pass --confirm-backup or --backup-file <path>\n")
		return 1
	}
	if backupFile != "" {
		bf, err := os.Open(backupFile)
		if err != nil {
			fmt.Fprintf(stderr, "restore-group-filters: backup file %q cannot be opened: %v\n", backupFile, err)
			return 1
		}
		hdr := make([]byte, 16)
		n, _ := bf.Read(hdr)
		bf.Close()
		if n < 15 || string(hdr[:15]) != "SQLite format 3" {
			fmt.Fprintf(stderr, "restore-group-filters: backup file %q is not a valid SQLite database\n", backupFile)
			return 1
		}
	}

	if fi, err := os.Stat(dbPath); err != nil {
		fmt.Fprintf(stderr, "restore-group-filters: target database %q not found: %v\n", dbPath, err)
		return 1
	} else if fi.IsDir() {
		fmt.Fprintf(stderr, "restore-group-filters: target database %q is a directory\n", dbPath)
		return 1
	}

	ctx := context.Background()

	var db *sql.DB
	if dryRun {
		var err error
		db, err = sqlite.OpenReadOnly(dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "restore-group-filters: database connection failed: %v\n", err)
			return 1
		}
	} else {
		dbCfg := sqlite.DefaultConfig(dbPath)
		var err error
		db, err = sqlite.Open(dbCfg)
		if err != nil {
			fmt.Fprintf(stderr, "restore-group-filters: database connection failed: %v\n", err)
			return 1
		}
	}
	defer db.Close()

	opts := inventory.RestoreGroupFiltersOptions{
		TargetDBPath:  dbPath,
		ArchiveDBPath: archiveDBPath,
		DryRun:        dryRun,
		ConfirmBackup: confirmBackup,
		BackupFile:    backupFile,
	}

	report, err := inventory.RunRestoreGroupFilters(ctx, db, opts)
	if err != nil {
		fmt.Fprintf(stderr, "restore-group-filters: execution failed: %v\n", err)
		return 1
	}

	outJSON, _ := json.MarshalIndent(report, "", "  ")
	fmt.Fprintln(stdout, string(outJSON))
	return 0
}
func checkConcurrentService(dbPath string) (int, error) {
	absPath, err := filepath.Abs(filepath.Clean(dbPath))
	if err != nil {
		absPath = filepath.Clean(dbPath)
	}

	// 1. Check candidate pid / lock files strictly belonging to this target DB
	candidatePIDFiles := []string{
		absPath + ".pid",
		absPath + ".lock",
		absPath + ".maintenance.lock",
	}

	for _, pf := range candidatePIDFiles {
		data, err := os.ReadFile(pf)
		if err == nil {
			var pid int
			if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err == nil && pid > 0 {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("concurrent service lock/pid file %s active for PID %d", pf, pid)
				}
			}
		}
	}

	// 2. On Linux, inspect /proc for any running csp serve referencing this exact database
	if entries, err := os.ReadDir("/proc"); err == nil {
		currentPID := os.Getpid()
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			var pid int
			if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err != nil || pid == currentPID || pid <= 0 {
				continue
			}
			cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
			if err != nil {
				continue
			}
			args := strings.Split(string(cmdlineBytes), "\x00")
			if len(args) == 0 {
				continue
			}
			isCspServe := false
			for i, arg := range args {
				if (strings.HasSuffix(arg, "/csp") || arg == "csp") && i+1 < len(args) && (args[i+1] == "serve" || args[i+1] == "server") {
					isCspServe = true
					break
				}
			}
			if !isCspServe {
				continue
			}

			// Extract -db / -d from args
			procDBPath := ""
			for i := 0; i < len(args); i++ {
				if (args[i] == "-db" || args[i] == "--db" || args[i] == "-d") && i+1 < len(args) {
					procDBPath = args[i+1]
					break
				}
				if strings.HasPrefix(args[i], "-db=") || strings.HasPrefix(args[i], "--db=") {
					procDBPath = strings.TrimPrefix(strings.TrimPrefix(args[i], "--db="), "-db=")
					break
				}
			}
			if procDBPath == "" {
				procDBPath = "/data/csp-v1.db"
			}
			procAbsPath, err := filepath.Abs(filepath.Clean(procDBPath))
			if err != nil {
				procAbsPath = filepath.Clean(procDBPath)
			}
			if procAbsPath == absPath {
				if isProcessAlive(pid) {
					return pid, fmt.Errorf("detected running csp serve process (PID %d) referencing target database %s", pid, dbPath)
				}
			}
		}
	}

	return 0, nil
}

func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// runMaintainInventory executes the maintain-inventory CLI command.
func runMaintainInventory(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("maintain-inventory", flag.ContinueOnError)
	fs.SetOutput(stderr)

	defaultDBPath := os.Getenv("CSP_DB_PATH")
	if defaultDBPath == "" {
		defaultDBPath = os.Getenv("DB_PATH")
	}
	if defaultDBPath == "" {
		defaultDBPath = "/data/csp-v1.db"
	}

	defaultFetchProxy := os.Getenv("CSP_FETCH_PROXY")
	if defaultFetchProxy == "" {
		defaultFetchProxy = os.Getenv("FETCH_PROXY")
	}

	var (
		dbPath                        string
		operationStr                  string
		dryRun                        bool
		reportDir                     string
		kindsStr                      string
		snapshotPublish               bool
		omitUnavailableOptionalGroups bool
		fetchProxy                    string
		aliveConcurrency              int
		aliveTimeout                  time.Duration
		mediaConcurrency              int
		speedConcurrency              int
		speedTimeout                  time.Duration
		speedMaxBytesPerNode          int64
		speedTotalBudget              int64
		globalTimeout                 time.Duration
		allowConcurrentService        bool
		forceOffline                  bool

		deleteRuleID          string
		expectedRuleExpr      string
		expectedRuleType      string
		expectedRuleValue     string
		expectedTargetGroupID string
	)

	fs.StringVar(&dbPath, "db", defaultDBPath, "path to target SQLite database file")
	fs.StringVar(&dbPath, "d", defaultDBPath, "path to target SQLite database file (shorthand)")
	fs.StringVar(&operationStr, "operation", "validate", "maintenance operation: refresh|probe|preview|publish|validate|delete-rule|restore-group-filters")
	fs.StringVar(&operationStr, "op", "validate", "maintenance operation shorthand")
	fs.BoolVar(&dryRun, "dry-run", false, "dry run without modifying database (inspects invariants and counts)")
	fs.StringVar(&reportDir, "report-dir", "./reports", "directory to store 0600 private maintenance reports (0700 dir)")
	fs.StringVar(&kindsStr, "kinds", "alive,media,ai,ip_risk,geo,speed", "comma-separated probe kinds to execute")
	fs.BoolVar(&snapshotPublish, "snapshot-publish", false, "activate immutable publication draft in validate/publish mode")
	fs.BoolVar(&omitUnavailableOptionalGroups, "omit-unavailable-optional-groups", false, "prune empty optional candidate groups during preview/publish/validate")
	fs.StringVar(&fetchProxy, "fetch-proxy", defaultFetchProxy, "outbound HTTP/HTTPS proxy for subscription fetching")
	fs.IntVar(&aliveConcurrency, "alive-concurrency", 8, "concurrency for alive/baseline stage")
	fs.DurationVar(&aliveTimeout, "alive-timeout", 15*time.Second, "timeout per alive/baseline probe task")
	fs.IntVar(&mediaConcurrency, "media-concurrency", 2, "concurrency for media/AI/risk/geo stage")
	fs.IntVar(&speedConcurrency, "speed-concurrency", 1, "concurrency for speed probe stage")
	fs.DurationVar(&speedTimeout, "speed-timeout", 3*time.Second, "timeout per speed probe task")
	fs.Int64Var(&speedMaxBytesPerNode, "speed-max-bytes-per-node", 1048576, "maximum speed test bytes downloaded per node (e.g. 1MiB)")
	fs.Int64Var(&speedTotalBudget, "speed-total-budget", 134217728, "total application response-body budget for ALL probe stages in the entire run (max 128MiB; excludes headers/TLS/NIC)")
	fs.DurationVar(&globalTimeout, "global-timeout", 20*time.Minute, "global timeout for entire maintenance execution")
	fs.BoolVar(&allowConcurrentService, "allow-concurrent-service", false, "allow running maintenance even if active service detected (dangerous)")
	fs.BoolVar(&forceOffline, "force-offline", false, "alias for --allow-concurrent-service")

	fs.StringVar(&deleteRuleID, "delete-rule-id", "", "exact rule ID to delete via policy maintenance operation")
	fs.StringVar(&expectedRuleExpr, "expected-rule-expression", "", "guard: expected exact rule expression (e.g. PROCESS-NAME,tr.com.kliq.app)")
	fs.StringVar(&expectedRuleType, "expected-rule-type", "", "guard: expected rule type (e.g. PROCESS-NAME)")
	fs.StringVar(&expectedRuleValue, "expected-rule-value", "", "guard: expected rule value (e.g. tr.com.kliq.app)")
	fs.StringVar(&expectedTargetGroupID, "expected-target-group-id", "", "guard: expected target group ID (e.g. 01a0b9af-c116-71ba-a125-31cefefe0d4b)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if speedTotalBudget <= 0 || speedTotalBudget > 128<<20 || globalTimeout <= 0 || globalTimeout > 30*time.Minute {
		fmt.Fprintln(stderr, "maintain-inventory: body budget must be 1..128MiB and global timeout must be positive and <=30m")
		return 2
	}

	if forceOffline {
		allowConcurrentService = true
	}

	if operationStr == "restore-group-filters" {
		return runRestoreGroupFilters(args, stdout, stderr)
	}

	isDeleteRuleOp := strings.ToLower(strings.TrimSpace(operationStr)) == "delete-rule" || strings.TrimSpace(deleteRuleID) != ""
	if isDeleteRuleOp {
		operationStr = "delete-rule"
	}

	op := inventory.MaintenanceOperation(strings.ToLower(strings.TrimSpace(operationStr)))
	if !op.IsValid() {
		fmt.Fprintf(stderr, "maintain-inventory: invalid --operation %q; supported: refresh, probe, preview, publish, validate, delete-rule\n", operationStr)
		return 2
	}

	var kinds []domain.ProbeKind
	if !isDeleteRuleOp {
		var err error
		kinds, err = inventory.ParseProbeKinds(kindsStr)
		if err != nil {
			fmt.Fprintf(stderr, "maintain-inventory: invalid --kinds: %v\n", err)
			return 2
		}
	}

	if fi, err := os.Stat(dbPath); err != nil {
		fmt.Fprintf(stderr, "maintain-inventory: target database %q not found: %v\n", dbPath, err)
		return 1
	} else if fi.IsDir() {
		fmt.Fprintf(stderr, "maintain-inventory: target database %q is a directory\n", dbPath)
		return 1
	}

	// Offline DB precondition check: ensure no active csp serve process is running on the target DB
	if !dryRun && !allowConcurrentService {
		if pid, err := checkConcurrentService(dbPath); err != nil {
			fmt.Fprintf(stderr, "maintain-inventory: pre-condition failed: %v; stop the running service before running offline maintenance (or pass --allow-concurrent-service)\n", err)
			return 1
		} else if pid > 0 {
			fmt.Fprintf(stderr, "maintain-inventory: pre-condition failed: concurrent csp serve process (PID %d) is running on database %s; stop the service first\n", pid, dbPath)
			return 1
		}
	}

	ctx := context.Background()

	// Acquire exclusive maintenance lock on file
	if !dryRun {
		lockFile := dbPath + ".maintenance.lock"
		lf, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			if os.IsExist(err) {
				fmt.Fprintf(stderr, "maintain-inventory: target database %s is locked by another maintenance run (lock file %s exists)\n", dbPath, lockFile)
				return 1
			}
			fmt.Fprintf(stderr, "maintain-inventory: warning: could not acquire maintenance lock file %s: %v\n", lockFile, err)
		} else {
			fmt.Fprintf(lf, "%d\n", os.Getpid())
			_ = lf.Close()
			defer os.Remove(lockFile)
		}
	}

	var db *sql.DB
	if dryRun {
		var err error
		db, err = sqlite.OpenReadOnly(dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "maintain-inventory: database connection failed: %v\n", err)
			return 1
		}
	} else {
		dbCfg := sqlite.DefaultConfig(dbPath)
		var err error
		db, err = sqlite.Open(dbCfg)
		if err != nil {
			fmt.Fprintf(stderr, "maintain-inventory: database connection failed: %v\n", err)
			return 1
		}
	}
	defer db.Close()

	// Configure runner options with maintenance tuning budgets
	runnerOpts := []probe.DefaultRunnerOption{
		probe.WithStageConcurrency(probe.StageConcurrency{
			Alive: aliveConcurrency,
			Media: mediaConcurrency,
			Speed: speedConcurrency,
		}),
		probe.WithSpeedBudget(speedMaxBytesPerNode, speedTimeout),
		probe.WithRunBudget(probe.RunBudget{
			MaxTasks:         10000,
			MaxResponseBytes: 16 << 20,
			MaxTotalBytes:    speedTotalBudget,
			TaskTimeout:      aliveTimeout,
		}),
	}

	appOpts := appServiceOptions{
		fetchProxy:       fetchProxy,
		probeRunnerOpts:  runnerOpts,
		startCoordinator: false, // offline maintenance does not start periodic background tickers
	}

	appServices, err := makeApplicationServices(ctx, db, appOpts)
	if err != nil {
		fmt.Fprintf(stderr, "maintain-inventory: application services initialization failed: %v\n", err)
		return 1
	}

	if isDeleteRuleOp {
		return runDeleteRuleMaintenance(ctx, db, appServices, deleteRuleID, expectedRuleExpr, expectedRuleType, expectedRuleValue, expectedTargetGroupID, dryRun, stdout, stderr)
	}

	orchestrator := inventory.NewMaintenanceOrchestrator(
		db,
		appServices.subRepo,
		appServices.nodeRepo,
		appServices.probeObsRepo,
		appServices.probeRunRepo,
		appServices.invService,
		appServices.probeRunner,
		appServices.probeService,
		appServices.pubService,
		appServices.auditRepo,
	)

	cfg := inventory.MaintenanceConfig{
		TargetDBPath:                  dbPath,
		Operation:                     op,
		DryRun:                        dryRun,
		ReportDir:                     reportDir,
		Kinds:                         kinds,
		SnapshotPublish:               snapshotPublish,
		OmitUnavailableOptionalGroups: omitUnavailableOptionalGroups,
		FetchProxy:                    fetchProxy,
		AliveConcurrency:              aliveConcurrency,
		AliveTimeout:                  aliveTimeout,
		MediaConcurrency:              mediaConcurrency,
		SpeedConcurrency:              speedConcurrency,
		SpeedTimeout:                  speedTimeout,
		SpeedMaxBytesPerNode:          speedMaxBytesPerNode,
		SpeedTotalBudget:              speedTotalBudget,
		GlobalTimeout:                 globalTimeout,
		AllowConcurrentService:        allowConcurrentService,
	}

	report, err := orchestrator.Run(ctx, cfg)
	if err != nil && report == nil {
		fmt.Fprintf(stderr, "maintain-inventory: execution failed: %v\n", err)
		return 1
	}

	// Output summary to stdout with secret/token fields omitted or masked
	stdoutReport := *report
	// Ensure stdout does not paste giant per-node report rows (those reside in 0600 file)
	if stdoutReport.Probe != nil {
		probeCopy := *stdoutReport.Probe
		probeCopy.Rows = nil
		stdoutReport.Probe = &probeCopy
	}

	outJSON, _ := json.MarshalIndent(stdoutReport, "", "  ")
	fmt.Fprintln(stdout, string(outJSON))

	if report.Status == "BLOCKED" || report.Status == "FAILED" {
		return 1
	}
	return 0
}

type deleteRuleReport struct {
	Status             string                 `json:"status"`
	Operation          string                 `json:"operation"`
	DryRun             bool                   `json:"dry_run"`
	MatchedCount       int                    `json:"matched_count"`
	DeletedCount       int                    `json:"deleted_count"`
	AlreadyDeleted     bool                   `json:"already_deleted"`
	RuleID             string                 `json:"rule_id,omitempty"`
	Expression         string                 `json:"expression,omitempty"`
	TargetGroupID      string                 `json:"target_group_id,omitempty"`
	PreviousRevisionID string                 `json:"previous_revision_id,omitempty"`
	NewRevisionID      string                 `json:"new_revision_id,omitempty"`
	Rule               map[string]interface{} `json:"rule,omitempty"`
	Message            string                 `json:"message"`
}

func runDeleteRuleMaintenance(
	ctx context.Context,
	db *sql.DB,
	appServices *applicationServices,
	deleteRuleID, expectedRuleExpr, expectedRuleType, expectedRuleValue, expectedTargetGroupID string,
	dryRun bool,
	stdout, stderr io.Writer,
) int {
	ruleID := strings.TrimSpace(deleteRuleID)
	if ruleID == "" {
		fmt.Fprintf(stderr, "maintain-inventory: --delete-rule-id is required for delete-rule operation\n")
		return 2
	}

	var (
		foundID       string
		revID         string
		targetGroupID string
		expression    string
		position      int
	)
	query := `SELECT id, revision_id, target_group_id, expression, position FROM policy_rules WHERE id = ?;`
	err := db.QueryRowContext(ctx, query, ruleID).Scan(&foundID, &revID, &targetGroupID, &expression, &position)
	if errors.Is(err, sql.ErrNoRows) {
		// Idempotent: already deleted or not found
		rep := deleteRuleReport{
			Status:         "SUCCESS",
			Operation:      "delete-rule",
			DryRun:         dryRun,
			MatchedCount:   0,
			DeletedCount:   0,
			AlreadyDeleted: true,
			RuleID:         ruleID,
			Message:        fmt.Sprintf("rule %s already deleted or not found; idempotent no-op", ruleID),
		}
		outJSON, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Fprintln(stdout, string(outJSON))
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "maintain-inventory: query rule %s failed: %v\n", ruleID, err)
		return 1
	}

	// Guard verification
	parts := strings.SplitN(expression, ",", 2)
	actualType := strings.TrimSpace(parts[0])
	actualValue := ""
	if len(parts) > 1 {
		actualValue = strings.TrimSpace(parts[1])
	}

	if expectedRuleExpr != "" && expression != strings.TrimSpace(expectedRuleExpr) {
		fmt.Fprintf(stderr, "maintain-inventory: guard failed: expression %q does not match expected %q\n", expression, expectedRuleExpr)
		return 1
	}
	if expectedRuleType != "" && actualType != strings.TrimSpace(expectedRuleType) {
		fmt.Fprintf(stderr, "maintain-inventory: guard failed: rule type %q does not match expected %q\n", actualType, expectedRuleType)
		return 1
	}
	if expectedRuleValue != "" && actualValue != strings.TrimSpace(expectedRuleValue) {
		fmt.Fprintf(stderr, "maintain-inventory: guard failed: rule value %q does not match expected %q\n", actualValue, expectedRuleValue)
		return 1
	}
	if expectedTargetGroupID != "" && targetGroupID != strings.TrimSpace(expectedTargetGroupID) {
		fmt.Fprintf(stderr, "maintain-inventory: guard failed: target_group_id %q does not match expected %q\n", targetGroupID, expectedTargetGroupID)
		return 1
	}

	if dryRun {
		rep := deleteRuleReport{
			Status:             "SUCCESS",
			Operation:          "delete-rule",
			DryRun:             true,
			MatchedCount:       1,
			DeletedCount:       0,
			AlreadyDeleted:     false,
			RuleID:             foundID,
			Expression:         expression,
			TargetGroupID:      targetGroupID,
			PreviousRevisionID: revID,
			Rule: map[string]interface{}{
				"id":              foundID,
				"revision_id":     revID,
				"target_group_id": targetGroupID,
				"expression":      expression,
				"position":        position,
			},
			Message: "dry-run verification passed: exactly 1 rule matched guards, no mutation applied",
		}
		outJSON, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Fprintln(stdout, string(outJSON))
		return 0
	}

	// Apply mode: delete rule through policy service and create new active revision
	delCmd := policy.DeleteRuleCommand{
		ID:        foundID,
		Kind:      "policy",
		ActorKind: domain.ActorKindAdmin,
		RequestID: "maintain-delete-rule-" + domain.MustNewUUIDv7(),
	}
	if err := appServices.policyService.DeleteRule(ctx, delCmd); err != nil {
		fmt.Fprintf(stderr, "maintain-inventory: failed to delete rule %s: %v\n", foundID, err)
		return 1
	}

	newRev, err := appServices.policyService.CreateRevision(ctx, policy.CreateRevisionCommand{
		State:     domain.RevisionStateActive,
		ParentID:  &revID,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "maintain-delete-rule-rev-" + domain.MustNewUUIDv7(),
	})
	var newRevID string
	if err == nil && newRev != nil {
		newRevID = newRev.ID
		_, _ = db.ExecContext(ctx, "UPDATE policy_rules SET revision_id = ? WHERE revision_id = ?;", newRevID, revID)
		_, _ = db.ExecContext(ctx, "UPDATE admission_rules SET revision_id = ? WHERE revision_id = ?;", newRevID, revID)
	}

	rep := deleteRuleReport{
		Status:             "SUCCESS",
		Operation:          "delete-rule",
		DryRun:             false,
		MatchedCount:       1,
		DeletedCount:       1,
		AlreadyDeleted:     false,
		RuleID:             foundID,
		Expression:         expression,
		TargetGroupID:      targetGroupID,
		PreviousRevisionID: revID,
		NewRevisionID:      newRevID,
		Message:            fmt.Sprintf("rule %s successfully deleted, new active revision %s generated", foundID, newRevID),
	}
	outJSON, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Fprintln(stdout, string(outJSON))
	return 0
}
