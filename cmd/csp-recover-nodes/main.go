package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"clash-sub-parser/internal/recovery"
	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "Path to the SQLite database file (required)")
	applyFlag := flag.Bool("apply", false, "Apply the recovery (default is dry-run inspect)")
	rollbackFlag := flag.Bool("rollback", false, "Rollback the recovery (re-deactivate restored candidates)")
	planPath := flag.String("plan", "", "Path to existing recovery plan JSON (used for rollback)")
	batchPrefix := flag.String("batch-prefix", recovery.DefaultMisdeactivationBatchPrefix, "Timestamp prefix for mis-deactivated batch")
	outputPlanPath := flag.String("output-plan", "", "Path to save recovery plan JSON")
	outputReportPath := flag.String("output-report", "", "Path to save execution report JSON")
	rehearsalApproval := flag.String("rehearsal-approval", "", "Explicit rehearsal approval token in format 'actor:evidence' for test copy drill")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintf(os.Stderr, "Error: --db is required\n")
		flag.Usage()
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var plan *recovery.RecoveryPlan
	if *planPath != "" {
		data, rErr := os.ReadFile(*planPath)
		if rErr != nil {
			fmt.Fprintf(os.Stderr, "Error reading plan file %s: %v\n", *planPath, rErr)
			os.Exit(1)
		}
		plan = &recovery.RecoveryPlan{}
		if err = json.Unmarshal(data, plan); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing plan JSON %s: %v\n", *planPath, err)
			os.Exit(1)
		}
		plan.TargetDBPath = *dbPath
	} else {
		plan, err = recovery.Inspect(ctx, db, *dbPath, *batchPrefix)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error inspecting database: %v\n", err)
			os.Exit(1)
		}
	}

	if *rehearsalApproval != "" {
		if !recovery.IsVerifiableRehearsalSandbox(*dbPath) {
			fmt.Fprintf(os.Stderr, "Error: --rehearsal-approval cannot be used on non-sandbox or production database (%s). Rehearsal is strictly restricted to verifiable isolated test replicas/sandboxes.\n", *dbPath)
			os.Exit(1)
		}
		parts := strings.SplitN(*rehearsalApproval, ":", 2)
		actor := parts[0]
		evidence := "manual-drill"
		if len(parts) > 1 {
			evidence = parts[1]
		}
		if aErr := recovery.ApproveForRehearsal(plan, actor, evidence); aErr != nil {
			fmt.Fprintf(os.Stderr, "Error approving plan for rehearsal: %v\n", aErr)
			os.Exit(1)
		}
	}

	fmt.Printf("=== Recovery Inspection Summary ===\n")
	fmt.Printf("Database:           %s\n", plan.TargetDBPath)
	fmt.Printf("Batch Prefix:       %s\n", plan.BatchTimestampPrefix)
	fmt.Printf("Total Inactive:     %d\n", plan.TotalInactiveNodes)
	fmt.Printf("Heuristic (Hold):   %d\n", len(plan.HeuristicCandidates))
	fmt.Printf("Attributed (Ready): %d\n", len(plan.AttributedCandidates))
	fmt.Printf("Ambiguous (Hold):   %d\n", len(plan.AmbiguousNodes))
	fmt.Printf("User Disabled:      %d\n", len(plan.ManuallyDisabledIDs))
	if plan.Approval != nil {
		fmt.Printf("Approval Status:    Approved by %s (Simulated: %t)\n", plan.Approval.ApprovedBy, plan.Approval.Simulated)
	} else {
		fmt.Printf("Approval Status:    UNAPPROVED (Fail-Closed: production apply blocked)\n")
	}
	fmt.Println("===================================")

	if *outputPlanPath != "" {
		raw, mErr := json.MarshalIndent(plan, "", "  ")
		if mErr == nil {
			_ = os.WriteFile(*outputPlanPath, raw, 0644)
			fmt.Printf("Saved plan JSON to %s\n", *outputPlanPath)
		}
	}

	var report *recovery.ExecutionReport
	if *rollbackFlag {
		fmt.Println("Executing Rollback...")
		report, err = recovery.Rollback(ctx, db, plan)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error during rollback: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Rollback complete! Rows affected: %d\n", report.RowsAffected)
	} else if *applyFlag {
		fmt.Println("Executing Apply...")
		report, err = recovery.Apply(ctx, db, plan)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error during apply: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Apply complete! Mode: %s, Rows restored: %d\n", report.Mode, report.RowsAffected)
		if *outputPlanPath != "" {
			if raw, mErr := json.MarshalIndent(plan, "", "  "); mErr == nil {
				_ = os.WriteFile(*outputPlanPath, raw, 0644)
			}
		}
	} else {
		fmt.Println("DRY RUN completed. To apply changes on rehearsal copy, pass --apply --rehearsal-approval=<actor>:<evidence>")
		report = &recovery.ExecutionReport{
			Mode:           "dry-run",
			ExecutedAt:     time.Now().UTC(),
			TargetDBPath:   *dbPath,
			HeuristicCount: len(plan.HeuristicCandidates),
			CandidateCount: len(plan.AttributedCandidates),
			AmbiguousCount: len(plan.AmbiguousNodes),
			RowsAffected:   0,
		}
	}

	if *outputReportPath != "" && report != nil {
		raw, mErr := json.MarshalIndent(report, "", "  ")
		if mErr == nil {
			_ = os.WriteFile(*outputReportPath, raw, 0644)
			fmt.Printf("Saved report JSON to %s\n", *outputReportPath)
		}
	}
}
