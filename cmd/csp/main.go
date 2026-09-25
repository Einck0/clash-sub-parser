// Package main provides the entry point for the CSP 1.0 single-binary control plane.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
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
	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/import/legacy"
	"clash-sub-parser/internal/import/legacy/inspect"
	"clash-sub-parser/internal/platform"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
	"clash-sub-parser/internal/webassets"
)

const (
	appName = "csp"
)

// runLegacyInspect executes the read-only legacy SQLite inspection command.
func runLegacyInspect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("legacy-inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var sourcePath string
	var format string
	var outputPath string

	fs.StringVar(&sourcePath, "source", "", "path to legacy SQLite database file (required)")
	fs.StringVar(&sourcePath, "s", "", "path to legacy SQLite database file (shorthand)")
	fs.StringVar(&format, "format", "text", "output format: text or json (default: text)")
	fs.StringVar(&format, "f", "text", "output format: text or json (shorthand)")
	fs.StringVar(&outputPath, "output", "", "optional output file path (default: stdout)")
	fs.StringVar(&outputPath, "o", "", "optional output file path (shorthand)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	// Positional argument fallback for source if --source not explicitly provided
	if sourcePath == "" && len(fs.Args()) > 0 {
		sourcePath = fs.Args()[0]
	}

	if sourcePath == "" {
		fmt.Fprintln(stderr, "legacy-inspect: --source is required")
		return 2
	}

	report, err := inspect.Inspect(sourcePath)
	if err != nil {
		fmt.Fprintf(stderr, "legacy-inspect: %v\n", err)
		return 1
	}

	var outputContent string
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		encoded, err := inspect.Marshal(report)
		if err != nil {
			fmt.Fprintf(stderr, "legacy-inspect: failed to marshal json report: %v\n", err)
			return 1
		}
		outputContent = string(encoded) + "\n"
	case "text":
		outputContent = inspect.FormatText(report)
	default:
		fmt.Fprintf(stderr, "legacy-inspect: unknown format %q, expected 'text' or 'json'\n", format)
		return 2
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, []byte(outputContent), 0644); err != nil {
			fmt.Fprintf(stderr, "legacy-inspect: failed to write output file: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprint(stdout, outputContent)
	}

	return 0
}

// runLegacyImport executes the offline allowlist-driven migration command.
func runLegacyImport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("legacy-import", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var sourcePath string
	var targetPath string
	var dryRun bool
	var format string
	var reportPath string

	fs.StringVar(&sourcePath, "source", "", "path to legacy SQLite database file (required)")
	fs.StringVar(&sourcePath, "s", "", "path to legacy SQLite database file (shorthand)")
	fs.StringVar(&targetPath, "target", "", "path to target CSP 1.0 SQLite database file (required unless --dry-run)")
	fs.StringVar(&targetPath, "t", "", "path to target CSP 1.0 SQLite database file (shorthand)")
	fs.BoolVar(&dryRun, "dry-run", false, "simulate import without writing to target database")
	fs.StringVar(&format, "format", "text", "output format: text or json (default: text)")
	fs.StringVar(&format, "f", "text", "output format: text or json (shorthand)")
	fs.StringVar(&reportPath, "report", "", "optional report output file path (default: stdout)")
	fs.StringVar(&reportPath, "r", "", "optional report output file path (shorthand)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	remaining := fs.Args()
	if sourcePath == "" && len(remaining) > 0 {
		sourcePath = remaining[0]
	}
	if targetPath == "" && len(remaining) > 1 {
		targetPath = remaining[1]
	}

	if sourcePath == "" {
		fmt.Fprintln(stderr, "legacy-import: --source is required")
		return 2
	}

	if !dryRun && targetPath == "" {
		fmt.Fprintln(stderr, "legacy-import: --target is required for actual import")
		return 2
	}

	normFormat := strings.ToLower(strings.TrimSpace(format))
	if normFormat != "text" && normFormat != "json" {
		fmt.Fprintf(stderr, "legacy-import: unknown format %q, expected 'text' or 'json'\n", format)
		return 2
	}

	importer := legacy.NewImporter()
	report, err := importer.Import(context.Background(), legacy.Options{
		SourcePath: sourcePath,
		TargetPath: targetPath,
		DryRun:     dryRun,
		ReportPath: reportPath,
		Format:     normFormat,
	})
	if err != nil {
		fmt.Fprintf(stderr, "legacy-import: %v\n", err)
		return 1
	}

	var outputContent string
	switch normFormat {
	case "json":
		encoded, err := report.MarshalJSON()
		if err != nil {
			fmt.Fprintf(stderr, "legacy-import: failed to marshal json report: %v\n", err)
			return 1
		}
		outputContent = string(encoded) + "\n"
	case "text":
		outputContent = report.FormatText()
	}

	if reportPath != "" {
		if err := os.WriteFile(reportPath, []byte(outputContent), 0644); err != nil {
			fmt.Fprintf(stderr, "legacy-import: failed to write report file: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprint(stdout, outputContent)
	}

	return 0
}

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

	var addr string
	var dbPath string
	var adminToken string

	fs.StringVar(&addr, "addr", defaultAddr, "HTTP listen address [host:port] (default from CSP_ADDR/CSP_BIND/CSP_PORT or 0.0.0.0:18080)")
	fs.StringVar(&addr, "a", defaultAddr, "HTTP listen address [host:port] (shorthand)")
	fs.StringVar(&dbPath, "db", defaultDBPath, "path to target SQLite database file (default from CSP_DB_PATH or /data/csp-v1.db)")
	fs.StringVar(&dbPath, "d", defaultDBPath, "path to target SQLite database file (shorthand)")
	fs.StringVar(&adminToken, "admin-token", defaultAdminToken, "admin bearer/cookie token (default from CSP_ADMIN_TOKEN)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	// Initialize node credential vault from environment (fail-fast on invalid key)
	vault, err := domain.NewNodeCredentialVaultFromEnv()
	if err != nil {
		fmt.Fprintf(stderr, "serve: node credential vault initialization failed: %v\n", err)
		return 1
	}
	if vault == nil {
		fmt.Fprintf(stderr, "serve: warning: node credential master key not configured; credential-backed probes disabled, legacy features remain compatible\n")
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

	// Wire repositories
	settingsRepo := sqlite.NewSettingsRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeSourceRepo := sqlite.NewNodeSourceRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	probeRunRepo := sqlite.NewProbeRunRepository(db)
	probeObsRepo := sqlite.NewProbeObservationRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revisionRepo := sqlite.NewRevisionRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	riskBindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)

	// Wire domain application services
	subService := subscription.NewService(subRepo, auditRepo)
	var invOpts []inventory.Option
	var credRepo domain.NodeCredentialRepository
	if vault != nil {
		credRepo = sqlite.NewNodeCredentialRepository(db)
		invOpts = append(invOpts, inventory.WithCredentialVault(vault, credRepo))
	}
	invService := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, nil, invOpts...)
	subService.SetReconciler(invService)

	probeScheduler, err := queue.NewScheduler(queue.Config{
		Concurrency: queue.DefaultConcurrency,
		Context:     startupCtx,
	})
	if err != nil {
		fmt.Fprintf(stderr, "serve: probe scheduler initialization failed: %v\n", err)
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
		if err := probeScheduler.Drain(ctx); err != nil {
			closeDBOnReturn = false
			fmt.Fprintf(stderr, "serve: probe scheduler drain incomplete during cleanup; terminating without closing database: %v\n", err)
		}
	}()

	var runnerOpts []probe.DefaultRunnerOption
	if vault != nil && credRepo != nil {
		safeDialer := probe.NewSafeNodeDialer(credRepo, vault)
		runnerOpts = append(runnerOpts, probe.WithNodeDialer(safeDialer))
	}

	var probeRunner probe.Runner
	if deps != nil && deps.newProbeRunner != nil {
		probeRunner = deps.newProbeRunner(db, nodeRepo, probeObsRepo, probeScheduler, probeRunRepo, runnerOpts...)
	} else {
		probeRunner = probe.NewDefaultRunner(nodeRepo, probeObsRepo, probeScheduler, probeRunRepo, runnerOpts...)
	}
	probeService := probe.NewService(probeRunRepo, probe.WithRunner(probeRunner))
	policyService := policy.NewService(policyRepo, revisionRepo, nodeRepo, auditRepo)
	revisionService := revision.NewService(revisionRepo, auditRepo, revision.WithPolicyRepository(policyRepo))
	ipriskService := iprisk.NewService(
		riskObsRepo,
		riskPolicyRepo,
		iprisk.WithBindingRepository(riskBindingRepo),
		iprisk.WithGroupRepository(policyRepo),
		iprisk.WithNodeRepository(nodeRepo),
		iprisk.WithAuditRepository(auditRepo),
	)
	pubService := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revisionRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithIPRiskService(ipriskService),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskBindingRepository(riskBindingRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	// Embedded web assets
	webHandler, _ := webassets.Handler()

	sessionStore := transporthttp.NewMemorySessionStore(24 * time.Hour)

	// Admin Token Initialization:
	// Database is source of truth. Check if DB already has an admin token verifier.
	// If unconfigured and env/flag adminToken is provided, persist it as initial bootstrap verifier.
	// Never override existing DB token on restart.
	dbSettings, err := settingsRepo.Get(startupCtx)
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
		if err := settingsRepo.UpdateAdminToken(startupCtx, hashed); err != nil {
			fmt.Fprintf(stderr, "serve: failed to persist bootstrap admin token: %v\n", err)
			return 1
		}
		activeVerifier = hashed
	}

	tokenHolder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		InitialVerifier: activeVerifier,
		SettingsRepo:    settingsRepo,
		SessionStore:    sessionStore,
		HashCost:        transporthttp.DefaultHashCost,
	})

	routerCfg := transporthttp.RouterConfig{
		AdminToken:         adminToken,
		TokenHolder:        tokenHolder,
		SettingsRepository: settingsRepo,
		SessionStore:       sessionStore,
		SessionValidator: func(sessionID string) (*transporthttp.SessionInfo, bool) {
			return sessionStore.Get(sessionID)
		},
		ReadinessChecker: func(c context.Context) (*sqlite.ReadinessReport, error) {
			return sqlite.CheckReadiness(c, db)
		},
		PublicationTokenValidator: func(c context.Context, publicationID, token string) (bool, error) {
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
			pub, err := pubRepo.GetByID(c, publicationID)
			if err != nil || pub == nil {
				return false, nil
			}
			if pub.RevokedAt != nil {
				return false, nil
			}
			return pub.TokenHash == hash || pub.TokenHash == token, nil
		},
		IsPublicationToken: func(c context.Context, token string) bool {
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
			pub, err := pubRepo.GetByTokenHash(c, hash)
			if err == nil && pub != nil {
				return true
			}
			pub, err = pubRepo.GetByTokenHash(c, token)
			return err == nil && pub != nil
		},
		SubscriptionService:        subService,
		InventoryService:           invService,
		ProbeService:               probeService,
		PolicyService:              policyService,
		RevisionService:            revisionService,
		PublicationService:         pubService,
		IPRiskService:              ipriskService,
		RiskPolicyRepository:       riskPolicyRepo,
		RiskBindingRepository:      riskBindingRepo,
		ProbeRunRepository:         probeRunRepo,
		ProbeObservationRepository: probeObsRepo,
		AuditRepository:            auditRepo,
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
	if deps != nil && deps.observeDrainBegin != nil {
		deps.observeDrainBegin()
	}
	drainDecided = true
	drainErr := probeScheduler.Drain(shutdownCtx)
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
		case "legacy-inspect":
			return runLegacyInspect(remaining[1:], stdout, stderr)
		case "legacy-import":
			return runLegacyImport(remaining[1:], stdout, stderr)
		case "serve", "server":
			return runServe(remaining[1:], stdout, stderr)
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
