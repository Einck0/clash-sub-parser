package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"clash-sub-parser/internal/migration"
)

func main() {
	defaultDB := os.Getenv("CSP_DB_PATH")
	if defaultDB == "" {
		defaultDB = filepath.Join("backups", "clash_sub_parser_pre_go_rewrite.db")
	}

	defaultOutput := os.Getenv("CSP_REPORT_PATH")
	if defaultOutput == "" {
		defaultOutput = filepath.Join("backups", "migration_dry_run_report.json")
	}

	defaultMD := os.Getenv("CSP_MD_REPORT_PATH")
	if defaultMD == "" {
		defaultMD = filepath.Join("backups", "migration_dry_run_report.md")
	}

	dbPath := flag.String("db", defaultDB, "Path to the SQLite database to verify")
	outputPath := flag.String("output", defaultOutput, "Path to output JSON report")
	mdPath := flag.String("md", defaultMD, "Path to output Markdown report")
	strict := flag.Bool("strict", true, "Enforce strict zero-loss validation and exact row count checks")
	flag.Parse()

	// Verify database file exists before starting
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: database file not found at %s: %v\n", *dbPath, err)
		os.Exit(1)
	}

	fmt.Printf("================================================================================\n")
	fmt.Printf("CSP Clean-Slate Go 1.0: Read-Only Migration Verification Engine\n")
	fmt.Printf("================================================================================\n")
	fmt.Printf("Target Database : %s\n", *dbPath)
	fmt.Printf("JSON Report     : %s\n", *outputPath)
	fmt.Printf("Markdown Report : %s\n", *mdPath)
	fmt.Printf("StrictMode      : %t\n", *strict)
	fmt.Printf("Engine Driver   : modernc.org/sqlite (Pure Go, mode=ro)\n")
	fmt.Printf("--------------------------------------------------------------------------------\n\n")

	opts := migration.Options{
		DBPath:       *dbPath,
		OutputPath:   *outputPath,
		MarkdownPath: *mdPath,
		Strict:       *strict,
	}

	verifier := migration.NewVerifier(opts)
	report, err := verifier.Verify(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Verification failed with error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Audit Execution Summary:\n")
	fmt.Printf("  Database SHA-256 : %s\n", report.DatabaseSHA256)
	fmt.Printf("  Database Size    : %d bytes\n", report.DatabaseSizeBytes)
	fmt.Printf("  Read-Only Mode   : %t (Write attempt strictly blocked)\n", report.IsReadOnlyEnforced)
	fmt.Printf("  Tables Discovered: %d\n", report.TotalTables)
	fmt.Printf("  Duration         : %d ms\n\n", report.DurationMs)

	fmt.Printf("Core Entity Counts & Integrity Checksums:\n")
	fmt.Printf("  - Nodes          : %d / 7,236 expected (SHA-256: %s)\n", report.Entities.Nodes.Count, report.Entities.Nodes.Checksum)
	fmt.Printf("  - Subscriptions  : %d / 8 expected     (SHA-256: %s)\n", report.Entities.Subscriptions.Count, report.Entities.Subscriptions.Checksum)
	fmt.Printf("  - Node Groups    : %d / 29 expected    (SHA-256: %s)\n", report.Entities.NodeGroups.Count, report.Entities.NodeGroups.Checksum)
	fmt.Printf("  - Rules          : %d / 470 expected   (SHA-256: %s)\n", report.Entities.Rules.Count, report.Entities.Rules.Checksum)
	fmt.Printf("  - Probe Results  : %d / 7,297 expected (SHA-256: %s)\n\n", report.Entities.ProbeResults.Count, report.Entities.ProbeResults.Checksum)

	fmt.Printf("Protocols Distribution (%d total nodes):\n", report.Entities.Nodes.Count)
	for proto, cnt := range report.ProtocolStats {
		fmt.Printf("  * %-12s : %5d\n", proto, cnt)
	}
	fmt.Println()

	fmt.Printf("Probe Status Distribution (%d total records):\n", report.Entities.ProbeResults.Count)
	for status, cnt := range report.ProbeStatusStats {
		fmt.Printf("  * %-12s : %5d\n", status, cnt)
	}
	fmt.Println()

	fmt.Printf("Field-Level Invariant Checks (%d checks performed):\n", len(report.FieldChecks))
	for _, fc := range report.FieldChecks {
		statusStr := "PASS"
		if fc.Failed > 0 {
			statusStr = "FAIL"
		}
		fmt.Printf("  * [%-4s] %-18s :: %-26s (Checked: %5d, Passed: %5d, Failed: %d)\n",
			statusStr, fc.Entity, fc.Field, fc.Checked, fc.Passed, fc.Failed)
	}
	fmt.Println()

	if *strict {
		mismatch := false
		if report.Entities.Nodes.Count != 7236 {
			fmt.Fprintf(os.Stderr, "Strict verification error: nodes count mismatch (got %d, expected 7236)\n", report.Entities.Nodes.Count)
			mismatch = true
		}
		if report.Entities.Subscriptions.Count != 8 {
			fmt.Fprintf(os.Stderr, "Strict verification error: subscriptions count mismatch (got %d, expected 8)\n", report.Entities.Subscriptions.Count)
			mismatch = true
		}
		if report.Entities.NodeGroups.Count != 29 {
			fmt.Fprintf(os.Stderr, "Strict verification error: node groups count mismatch (got %d, expected 29)\n", report.Entities.NodeGroups.Count)
			mismatch = true
		}
		if report.Entities.Rules.Count != 470 {
			fmt.Fprintf(os.Stderr, "Strict verification error: rules count mismatch (got %d, expected 470)\n", report.Entities.Rules.Count)
			mismatch = true
		}
		if report.Entities.ProbeResults.Count != 7297 {
			fmt.Fprintf(os.Stderr, "Strict verification error: probe results count mismatch (got %d, expected 7297)\n", report.Entities.ProbeResults.Count)
			mismatch = true
		}
		if report.ValidationErrors > 0 {
			fmt.Fprintf(os.Stderr, "Strict verification error: %d validation errors encountered\n", report.ValidationErrors)
			mismatch = true
		}

		if mismatch {
			fmt.Printf("VERDICT: FAILED (Strict checks failed)\n")
			os.Exit(1)
		}
	}

	fmt.Printf("--------------------------------------------------------------------------------\n")
	fmt.Printf("VERDICT: %s\n", report.Verdict)
	fmt.Printf("Reports successfully generated:\n")
	fmt.Printf("  - JSON: %s\n", *outputPath)
	fmt.Printf("  - MD  : %s\n", *mdPath)
	fmt.Printf("================================================================================\n")
}
