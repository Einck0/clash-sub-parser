// Package main provides the entry point for the CSP 1.0 single-binary control plane.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/platform"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
	"clash-sub-parser/internal/webassets"
)

const (
	appName = "csp"
)

// runServe executes the long-running HTTP control plane and SPA server.
func runServe(args []string, stdout, stderr io.Writer) int {
	return runServeWithContext(context.Background(), args, stdout, stderr)
}

// serveShutdownResult owns the final DB-close decision: a failed drain must
// leave resources open for process termination rather than race active work.
func serveShutdownResult(drainErr error, closeDB func()) bool {
	if drainErr != nil {
		return false
	}
	closeDB()
	return true
}

type serveDependencies struct {
	shutdownTimeout      time.Duration
	closeDBFn            func(*sql.DB) error
	newProbeRunner       func(db *sql.DB, nodeRepo domain.NodeRepository, obsRepo domain.ProbeObservationRepository, scheduler *queue.Scheduler, runRepo domain.ProbeRunRepository, opts ...probe.DefaultRunnerOption) probe.Runner
	observeDrainBegin    func()
	observeDrainDecision func(drainErr error)
	observeDBClose       func(stage string, err error)
	observeCleanExit     func()
}

// runServeWithContext allows testing graceful startup and cancellation via context.
func runServeWithContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runServeWithLifecycleHook(ctx, args, stdout, stderr, nil)
}

// runServeWithLifecycleHook keeps lifecycle instrumentation private to this package;
// production callers use runServeWithContext and provide nil dependencies.
func runServeWithLifecycleHook(ctx context.Context, args []string, stdout, stderr io.Writer, deps *serveDependencies) int {
	return runServeWithDependencies(ctx, args, stdout, stderr, deps)
}

func runServeWithDependencies(ctx context.Context, args []string, stdout, stderr io.Writer, deps *serveDependencies) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)

	defaultAddr := os.Getenv("CSP_ADDR")
	if defaultAddr == "" {
		bind := os.Getenv("CSP_BIND")
		port := os.Getenv("CSP_PORT")
		if port == "" {
			port = os.Getenv("PORT")
		}
		if port == "" {
			port = "18080"
		}
		if bind == "" {
			bind = "0.0.0.0"
		}
		defaultAddr = net.JoinHostPort(bind, port)
	}

	defaultDBPath := os.Getenv("CSP_DB_PATH")
	if defaultDBPath == "" {
		defaultDBPath = os.Getenv("DB_PATH")
	}
	if defaultDBPath == "" {
		defaultDBPath = "/data/csp-v1.db"
	}

	defaultAdminToken := os.Getenv("CSP_ADMIN_TOKEN")
	if defaultAdminToken == "" {
		defaultAdminToken = os.Getenv("ADMIN_TOKEN")
	}

	defaultFetchProxy := os.Getenv("CSP_FETCH_PROXY")
	if defaultFetchProxy == "" {
		defaultFetchProxy = os.Getenv("FETCH_PROXY")
	}

	var addr string
	var dbPath string
	var adminToken string
	var fetchProxy string

	fs.StringVar(&addr, "addr", defaultAddr, "HTTP listen address [host:port] (default from CSP_ADDR/CSP_BIND/CSP_PORT or 0.0.0.0:18080)")
	fs.StringVar(&addr, "a", defaultAddr, "HTTP listen address [host:port] (shorthand)")
	fs.StringVar(&dbPath, "db", defaultDBPath, "path to target SQLite database file (default from CSP_DB_PATH or /data/csp-v1.db)")
	fs.StringVar(&dbPath, "d", defaultDBPath, "path to target SQLite database file (shorthand)")
	fs.StringVar(&adminToken, "admin-token", defaultAdminToken, "admin bearer/cookie token (default from CSP_ADMIN_TOKEN)")
	fs.StringVar(&fetchProxy, "fetch-proxy", defaultFetchProxy, "outbound HTTP/HTTPS proxy for subscription fetching (default from CSP_FETCH_PROXY)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	// Ensure parent directory exists if not in-memory
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." && !strings.HasPrefix(dbPath, ":memory:") && !strings.HasPrefix(dbPath, "file::memory:") {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(stderr, "serve: failed to create database directory %q: %v\n", dir, err)
			return 1
		}
	}

	startupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Initialize SQLite connection pool and apply embedded migrations
	dbCfg := sqlite.DefaultConfig(dbPath)
	db, err := sqlite.OpenAndMigrate(startupCtx, dbCfg)
	if err != nil {
		fmt.Fprintf(stderr, "serve: database startup failed: %v\n", err)
		return 1
	}
	closeDBOnReturn := true
	dbClosed := false
	closeDatabase := func() error {
		if dbClosed {
			return nil
		}
		dbClosed = true
		if deps != nil && deps.observeDBClose != nil {
			deps.observeDBClose("db.close.attempt", nil)
		}
		var closeErr error
		if deps != nil && deps.closeDBFn != nil {
			closeErr = deps.closeDBFn(db)
		} else {
			closeErr = db.Close()
		}
		if deps != nil && deps.observeDBClose != nil {
			if closeErr != nil {
				deps.observeDBClose("db.close.error", closeErr)
			} else {
				deps.observeDBClose("db.close.ok", nil)
			}
		}
		if closeErr != nil {
			fmt.Fprintf(stderr, "serve: database close failed: %v\n", closeErr)
		}
		return closeErr
	}
	defer func() {
		if !closeDBOnReturn || dbClosed {
			return
		}
		_ = closeDatabase()
	}()

	var runnerOpts []probe.DefaultRunnerOption
	var newRunnerFn func(db *sql.DB, nodeRepo domain.NodeRepository, obsRepo domain.ProbeObservationRepository, scheduler *queue.Scheduler, runRepo domain.ProbeRunRepository, opts ...probe.DefaultRunnerOption) probe.Runner
	if deps != nil {
		newRunnerFn = deps.newProbeRunner
	}

	appOpts := appServiceOptions{
		fetchProxy:       fetchProxy,
		newProbeRunner:   newRunnerFn,
		probeRunnerOpts:  runnerOpts,
		startCoordinator: true,
	}

	appServices, err := makeApplicationServices(startupCtx, db, appOpts)
	if err != nil {
		fmt.Fprintf(stderr, "serve: application services initialization failed: %v\n", err)
		return 1
	}

	shutdownTimeout := 5 * time.Second
	if deps != nil && deps.shutdownTimeout > 0 {
		shutdownTimeout = deps.shutdownTimeout
	}
	drainDecided := false
	defer func() {
		if drainDecided {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := appServices.probeScheduler.Drain(ctx); err != nil {
			closeDBOnReturn = false
			fmt.Fprintf(stderr, "serve: probe scheduler drain incomplete during cleanup; terminating without closing database: %v\n", err)
		}
	}()

	// Embedded web assets
	webHandler, _ := webassets.Handler()

	sessionStore := transporthttp.NewMemorySessionStore(24 * time.Hour)

	// Admin Token Initialization:
	// Database is source of truth. Check if DB already has an admin token verifier.
	// If unconfigured and env/flag adminToken is provided, persist it as initial bootstrap verifier.
	// Never override existing DB token on restart.
	dbSettings, err := appServices.settingsRepo.Get(startupCtx)
	if err != nil {
		fmt.Fprintf(stderr, "serve: failed to read settings: %v\n", err)
		return 1
	}

	var activeVerifier string
	if dbSettings != nil && dbSettings.AdminToken != "" {
		activeVerifier = dbSettings.AdminToken
	} else if adminToken != "" {
		// Bootstrap unconfigured DB
		hashed, hashErr := transporthttp.HashToken(adminToken)
		if hashErr != nil {
			fmt.Fprintf(stderr, "serve: failed to hash bootstrap admin token: %v\n", hashErr)
			return 1
		}
		if err := appServices.settingsRepo.UpdateAdminToken(startupCtx, hashed); err != nil {
			fmt.Fprintf(stderr, "serve: failed to persist bootstrap admin token: %v\n", err)
			return 1
		}
		activeVerifier = hashed
	}

	tokenHolder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		InitialVerifier:   activeVerifier,
		SettingsRepo:      appServices.settingsRepo,
		SessionStore:      sessionStore,
		HashCost:          transporthttp.DefaultHashCost,
		AdminAuthEnabled:  &dbSettings.AdminAuthEnabled,
		ExportAuthEnabled: &dbSettings.ExportAuthEnabled,
	})

	routerCfg := transporthttp.RouterConfig{
		AdminToken:         adminToken,
		TokenHolder:        tokenHolder,
		SettingsRepository: appServices.settingsRepo,
		SessionStore:       sessionStore,
		SessionValidator: func(sessionID string) (*transporthttp.SessionInfo, bool) {
			return sessionStore.Get(sessionID)
		},
		ReadinessChecker: func(c context.Context) (*sqlite.ReadinessReport, error) {
			return sqlite.CheckReadiness(c, db)
		},
		PublicationTokenValidator:  appServices.pubService.ValidateToken,
		IsPublicationToken:         appServices.pubService.IsPublicationToken,
		SubscriptionService:        appServices.subService,
		InventoryService:           appServices.invService,
		ProbeService:               appServices.probeService,
		PolicyService:              appServices.policyService,
		RevisionService:            appServices.revisionService,
		PublicationService:         appServices.pubService,
		IPRiskService:              appServices.ipriskService,
		RiskPolicyRepository:       appServices.riskPolicyRepo,
		RiskBindingRepository:      appServices.riskBindingRepo,
		ProbeRunRepository:         appServices.probeRunRepo,
		ProbeObservationRepository: appServices.probeObsRepo,
		AuditRepository:            appServices.auditRepo,
		WebHandler:                 webHandler,
	}

	handler := transporthttp.NewRouter(routerCfg)

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	serverErr := make(chan error, 1)
	go func() {
		fmt.Fprintf(stdout, "Starting CSP 1.0 control plane server listening on http://%s (db: %s)\n", addr, dbPath)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		fmt.Fprintf(stderr, "serve: server error: %v\n", err)
		return 1
	case sig := <-sigChan:
		fmt.Fprintf(stdout, "Received signal %v, shutting down gracefully...\n", sig)
	case <-ctx.Done():
		fmt.Fprintln(stdout, "Context cancelled, shutting down gracefully...")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(stderr, "serve: shutdown error: %v\n", err)
		return 1
	}
	appServices.periodicCoordinator.Stop()
	if deps != nil && deps.observeDrainBegin != nil {
		deps.observeDrainBegin()
	}
	drainDecided = true
	drainErr := appServices.probeScheduler.Drain(shutdownCtx)
	if deps != nil && deps.observeDrainDecision != nil {
		deps.observeDrainDecision(drainErr)
	}
	var closeErr error
	if !serveShutdownResult(drainErr, func() {
		closeErr = closeDatabase()
	}) {
		// main exits immediately on this status; deliberately keep the DB open
		// until the OS reclaims the process, rather than racing active callbacks.
		closeDBOnReturn = false
		if deps != nil && deps.observeDBClose != nil {
			deps.observeDBClose("db.close.skipped.drain_timeout", drainErr)
		}
		fmt.Fprintf(stderr, "serve: probe scheduler drain incomplete; terminating without closing database: %v\n", drainErr)
		if deps != nil && deps.observeDBClose != nil {
			deps.observeDBClose("shutdown.failure.final", drainErr)
		}
		return 1
	}
	if closeErr != nil {
		return 1
	}
	fmt.Fprintln(stdout, "CSP control plane stopped cleanly")
	if deps != nil && deps.observeCleanExit != nil {
		deps.observeCleanExit()
	}
	return 0
}

// runRecoverSourceHistory executes the recover-source-history CLI maintenance command.
func runRecoverSourceHistory(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("recover-source-history", flag.ContinueOnError)
	fs.SetOutput(stderr)

	defaultDBPath := os.Getenv("CSP_DB_PATH")
	if defaultDBPath == "" {
		defaultDBPath = os.Getenv("DB_PATH")
	}
	if defaultDBPath == "" {
		defaultDBPath = "/data/csp-v1.db"
	}

	var (
		manifestPath   string
		dbPath         string
		dryRun         bool
		expectedSHA256 string
		archiveDir     string
	)

	fs.StringVar(&manifestPath, "manifest", "", "path to verified recovery manifest JSON file (required)")
	fs.StringVar(&manifestPath, "m", "", "path to verified recovery manifest JSON file (shorthand)")
	fs.StringVar(&dbPath, "db", defaultDBPath, "path to target SQLite database file")
	fs.StringVar(&dbPath, "d", defaultDBPath, "path to target SQLite database file (shorthand)")
	fs.BoolVar(&dryRun, "dry-run", false, "dry run without modifying database (inspects invariants and counts)")
	fs.StringVar(&expectedSHA256, "expected-sha256", "", "expected sha256 checksum of manifest file (required)")
	fs.StringVar(&archiveDir, "archive-dir", "/home/service/backups", "path to evidence archives directory")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if strings.TrimSpace(manifestPath) == "" {
		fmt.Fprintf(stderr, "recover-source-history: --manifest is required\n")
		return 1
	}
	if strings.TrimSpace(expectedSHA256) == "" {
		fmt.Fprintf(stderr, "recover-source-history: --expected-sha256 is required\n")
		return 1
	}

	ctx := context.Background()

	var db *sql.DB
	if dryRun {
		var err error
		db, err = sqlite.OpenReadOnly(dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "recover-source-history: database connection failed: %v\n", err)
			return 1
		}
	} else {
		dbCfg := sqlite.DefaultConfig(dbPath)
		var err error
		db, err = sqlite.Open(dbCfg)
		if err != nil {
			fmt.Fprintf(stderr, "recover-source-history: database connection failed: %v\n", err)
			return 1
		}
	}
	defer db.Close()

	recCfg := inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: expectedSHA256,
		DryRun:                 dryRun,
		TargetDBPath:           dbPath,
		ArchiveDir:             archiveDir,
	}

	res, err := inventory.RecoverSourceHistory(ctx, db, recCfg)
	if err != nil {
		fmt.Fprintf(stderr, "recover-source-history: recovery failed: %v\n", err)
		return 1
	}

	outJSON, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintln(stdout, string(outJSON))
	return 0
}

// runResetNodeInventory executes the reset-node-inventory CLI maintenance command.
func runResetNodeInventory(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reset-node-inventory", flag.ContinueOnError)
	fs.SetOutput(stderr)

	defaultDBPath := os.Getenv("CSP_DB_PATH")
	if defaultDBPath == "" {
		defaultDBPath = os.Getenv("DB_PATH")
	}
	if defaultDBPath == "" {
		defaultDBPath = "/data/csp-v1.db"
	}

	var (
		dbPath              string
		dryRun              bool
		apply               bool
		confirmBackup       bool
		backupFile          string
		restoreSourceTokens bool
		archiveDBPath       string
	)

	fs.StringVar(&dbPath, "db", defaultDBPath, "path to target SQLite database file")
	fs.StringVar(&dbPath, "d", defaultDBPath, "path to target SQLite database file (shorthand)")
	fs.BoolVar(&dryRun, "dry-run", false, "dry run without modifying database (inspects invariants and counts)")
	fs.BoolVar(&apply, "apply", false, "apply clean-slate node inventory reset transaction")
	fs.BoolVar(&confirmBackup, "confirm-backup", false, "confirm verified database backup exists before executing apply")
	fs.StringVar(&backupFile, "backup-file", "", "optional path to verified backup file to inspect before apply")
	fs.BoolVar(&restoreSourceTokens, "restore-source-tokens", false, "restore full authenticated URLs for 7li and 魔戒 from cold archive")
	fs.StringVar(&archiveDBPath, "archive-db", "/home/service/backups/csp-legacy-cold-archive-20260919.db", "path to legacy cold archive SQLite database")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if !dryRun && !apply {
		fmt.Fprintf(stderr, "reset-node-inventory: either --dry-run or --apply must be explicitly specified (fail-safe)\n")
		return 1
	}
	if dryRun && apply {
		fmt.Fprintf(stderr, "reset-node-inventory: cannot specify both --dry-run and --apply\n")
		return 1
	}

	if apply && !confirmBackup && backupFile == "" {
		fmt.Fprintf(stderr, "reset-node-inventory: apply requires backup confirmation; pass --confirm-backup or --backup-file <path>\n")
		return 1
	}
	if backupFile != "" {
		bf, err := os.Open(backupFile)
		if err != nil {
			fmt.Fprintf(stderr, "reset-node-inventory: backup file %q cannot be opened: %v\n", backupFile, err)
			return 1
		}
		hdr := make([]byte, 16)
		n, _ := bf.Read(hdr)
		bf.Close()
		if n < 15 || string(hdr[:15]) != "SQLite format 3" {
			fmt.Fprintf(stderr, "reset-node-inventory: backup file %q is not a valid SQLite database\n", backupFile)
			return 1
		}
	}

	if fi, err := os.Stat(dbPath); err != nil {
		fmt.Fprintf(stderr, "reset-node-inventory: target database %q not found: %v\n", dbPath, err)
		return 1
	} else if fi.IsDir() {
		fmt.Fprintf(stderr, "reset-node-inventory: target database %q is a directory\n", dbPath)
		return 1
	}

	ctx := context.Background()

	var db *sql.DB
	if dryRun {
		var err error
		db, err = sqlite.OpenReadOnly(dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "reset-node-inventory: database connection failed: %v\n", err)
			return 1
		}
	} else {
		dbCfg := sqlite.DefaultConfig(dbPath)
		var err error
		db, err = sqlite.Open(dbCfg)
		if err != nil {
			fmt.Fprintf(stderr, "reset-node-inventory: database connection failed: %v\n", err)
			return 1
		}
	}
	defer db.Close()

	opts := inventory.ResetOptions{
		TargetDBPath:        dbPath,
		DryRun:              dryRun,
		RestoreSourceTokens: restoreSourceTokens,
		ArchiveDBPath:       archiveDBPath,
	}

	report, err := inventory.RunResetNodeInventory(ctx, db, opts)
	if err != nil {
		fmt.Fprintf(stderr, "reset-node-inventory: reset failed: %v\n", err)
		return 1
	}

	outJSON, _ := json.MarshalIndent(report, "", "  ")
	fmt.Fprintln(stdout, string(outJSON))
	return 0
}

// run executes the root application logic with injectable standard I/O for testing.

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	fs.SetOutput(stderr)

	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&showVersion, "v", false, "print version and exit (shorthand)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if showVersion {
		fmt.Fprintf(stdout, "%s %s (commit: %s, built: %s)\n", appName, platform.Version, platform.Commit, platform.BuildDate)
		return 0
	}

	remaining := fs.Args()
	if len(remaining) > 0 {
		switch remaining[0] {
		case "version":
			fmt.Fprintf(stdout, "%s %s\n", appName, platform.Version)
			return 0
		case "serve", "server":
			return runServe(remaining[1:], stdout, stderr)
		case "recover-source-history":
			return runRecoverSourceHistory(remaining[1:], stdout, stderr)
		case "reset-node-inventory":
			return runResetNodeInventory(remaining[1:], stdout, stderr)
		case "restore-group-filters":
			return runRestoreGroupFilters(remaining[1:], stdout, stderr)
		case "maintain-inventory":
			return runMaintainInventory(remaining[1:], stdout, stderr)
		default:
			fmt.Fprintf(stderr, "unknown command: %s\n", remaining[0])
			return 1
		}
	}

	fmt.Fprintf(stdout, "%s %s - Clash Sub Parser Control Plane\n", appName, platform.Version)
	return 0
}

func main() {
	code := run(os.Args[1:], os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}
