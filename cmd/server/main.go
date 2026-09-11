package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"clash-sub-parser/internal/compiler/template"
	"clash-sub-parser/internal/probe"
	"clash-sub-parser/internal/repository"
	"clash-sub-parser/internal/server"
)

var (
	version   = "1.0.0-go-clean-slate"
	buildDate = "2026-09-11"
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func resolveDefaultDB() string {
	if val := os.Getenv("CSP_DB_PATH"); val != "" {
		return val
	}
	if val := os.Getenv("DB_PATH"); val != "" {
		return val
	}
	if val := os.Getenv("CLASH_DATABASE_URL"); val != "" {
		for _, prefix := range []string{"sqlite+aiosqlite:///", "sqlite:///"} {
			if strings.HasPrefix(val, prefix) {
				return strings.TrimPrefix(val, prefix)
			}
		}
		return val
	}
	return "clash_sub_parser.db"
}

func main() {
	defaultPort := getEnvInt("CSP_PORT", getEnvInt("PORT", 18080))
	defaultBind := getEnv("CSP_BIND", getEnv("CSP_HOST", "0.0.0.0"))
	defaultDB := resolveDefaultDB()

	port := flag.Int("port", defaultPort, "HTTP server listening port")
	bind := flag.String("bind", defaultBind, "HTTP server bind address")
	dbPath := flag.String("db", defaultDB, "Path to SQLite database")
	gracefulTimeout := flag.Duration("graceful-timeout", 15*time.Second, "Graceful shutdown deadline")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("CSP Server version %s (built %s)\n", version, buildDate)
		os.Exit(0)
	}

	log.Printf("[CSP-Server] Starting Clash Subscription Parser Go Server %s (%s)...", version, buildDate)
	log.Printf("[CSP-Server] Database path: %s", *dbPath)

	// 1. Initialize SQLite Database with WAL and foreign key constraints
	dbOpts := repository.Options{
		Path:         strings.TrimSpace(*dbPath),
		MaxOpenConns: 25,
		MaxIdleConns: 5,
		BusyTimeout:  30 * time.Second,
	}
	sqliteDB, err := repository.NewSQLiteDB(dbOpts)
	if err != nil {
		log.Fatalf("[CSP-Server] Failed to initialize SQLite database: %v", err)
	}
	defer sqliteDB.Close()

	// Ensure schemas and baseline seeds exist
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := sqliteDB.InitSchema(initCtx); err != nil {
		initCancel()
		log.Fatalf("[CSP-Server] Failed to initialize database schema: %v", err)
	}
	initCancel()
	log.Println("[CSP-Server] SQLite database schema and constraints verified.")

	repos := sqliteDB.Repositories()

	// 2. Initialize High-Concurrency Probe Engine
	probeEngine := probe.NewEngine(nil)
	log.Println("[CSP-Server] High-concurrency probe engine initialized.")

	// 3. Initialize 5-Target Configuration Compiler
	comp, err := template.NewCompiler()
	if err != nil {
		log.Fatalf("[CSP-Server] Failed to initialize configuration template compiler: %v", err)
	}
	log.Println("[CSP-Server] Template compiler initialized.")

	// 4. Initialize HTTP Server and mount embedded frontend SPA
	srv := server.NewServerWithCompiler(repos, probeEngine, comp)
	log.Println("[CSP-Server] HTTP REST routes and embedded frontend SPA mounted.")

	addr := fmt.Sprintf("%s:%d", *bind, *port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		log.Printf("[CSP-Server] Listening and serving on http://%s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
		close(serverErrCh)
	}()

	// 5. Listen for OS interrupt and termination signals
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-shutdownCh:
		log.Printf("[CSP-Server] Shutdown signal (%v) received, starting graceful termination...", sig)
	case err := <-serverErrCh:
		if err != nil {
			log.Fatalf("[CSP-Server] HTTP server encountered fatal error: %v", err)
		}
		return
	}

	// 6. Graceful Shutdown: Drain in-flight probes and HTTP requests
	log.Println("[CSP-Server] Cancelling any active background probe tasks...")
	srv.ProbeManager().Cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), *gracefulTimeout)
	defer shutdownCancel()

	log.Printf("[CSP-Server] Draining active HTTP connections (timeout: %v)...", *gracefulTimeout)
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[CSP-Server] HTTP server shutdown error: %v", err)
	} else {
		log.Println("[CSP-Server] All HTTP connections safely closed.")
	}

	log.Println("[CSP-Server] Closing SQLite database connection pool...")
	if err := sqliteDB.Close(); err != nil {
		log.Printf("[CSP-Server] SQLite database close error: %v", err)
	} else {
		log.Println("[CSP-Server] SQLite connections safely flushed and closed.")
	}

	log.Println("[CSP-Server] Process gracefully exited.")
}
