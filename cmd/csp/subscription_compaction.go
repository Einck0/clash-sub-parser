package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"

	sqlitedriver "modernc.org/sqlite"
)

// Maintenance never initializes application services or falls through to the
// write-capable startup migration path. Apply has its own offline safeguards.
func runSubscriptionCompaction(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "compact-subscription-storage: explicit plan/apply/verify subcommand required (maintenance_required)")
		return 2
	}
	switch args[0] {
	case "plan":
	case "apply":
		return runSubscriptionCompactionApply(args[1:], stdout, stderr)
	case "verify":
		return runSubscriptionCompactionVerify(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "compact-subscription-storage: unknown operation; use explicit plan/apply/verify (maintenance_required)")
		return 2
	}
	fs := flag.NewFlagSet("compact-subscription-storage plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var dbPath, reportPath string
	var timeout time.Duration
	fs.StringVar(&dbPath, "db", "", "explicit path to an authorized offline read-only snapshot (never the active database)")
	fs.StringVar(&reportPath, "report", "", "new private report path in an existing 0700 directory")
	fs.DurationVar(&timeout, "timeout", 5*time.Minute, "bounded offline scan duration (maximum 30m)")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if dbPath == "" || reportPath == "" || fs.NArg() != 0 || timeout <= 0 || timeout > 30*time.Minute {
		fmt.Fprintln(stderr, "compaction plan: explicit --db, --report and a timeout in (0,30m] are required; positional arguments are forbidden")
		return 2
	}
	// Reject nonregular targets rather than opening devices, directories or
	// symlink aliases; no helper creates parent directories or changes permissions.
	info, err := os.Lstat(dbPath)
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintln(stderr, "compaction plan: snapshot must be an existing regular file, not a symlink")
		return 1
	}
	dir, err := os.Lstat(filepath.Dir(reportPath))
	if err != nil || !dir.IsDir() || dir.Mode().Perm() != 0700 {
		fmt.Fprintln(stderr, "compaction plan: report parent must be an existing private 0700 directory")
		return 1
	}
	// Reserve before the expensive read. O_EXCL also rejects aliases of the
	// input and existing reports without truncating either.
	report, err := os.OpenFile(reportPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintln(stderr, "compaction plan: cannot exclusively create private report")
		return 1
	}
	defer report.Close()
	db, err := sqlite.OpenReadOnly(dbPath)
	if err != nil {
		fmt.Fprintln(stderr, "compaction plan: cannot open snapshot read-only")
		return 1
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	plan, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		writeCompactionInspectionFailure(stderr, err)
		return 1
	}
	if err := json.NewEncoder(report).Encode(plan); err != nil {
		fmt.Fprintln(stderr, "compaction plan: report write failed")
		return 1
	}
	if err := report.Sync(); err != nil {
		fmt.Fprintln(stderr, "compaction plan: report sync failed")
		return 1
	}
	if err := report.Close(); err != nil {
		fmt.Fprintln(stderr, "compaction plan: report close failed")
		return 1
	}
	fmt.Fprintln(stdout, "Read-only inventory recorded. Apply eligibility and unresolved blockers are in the plan; separate offline operator authorization is required. No database was transformed.")
	return 0
}

// Only fixed categories, an inspector-owned phase and a numeric SQLite code
// cross this boundary. Never format err (even a wrapped driver's message).
func writeCompactionInspectionFailure(stderr io.Writer, err error) {
	category, code := "unknown", 0
	var driverErr *sqlitedriver.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		category = "deadline"
	case errors.Is(err, context.Canceled):
		category = "cancelled"
	case errors.As(err, &driverErr):
		category, code = "sqlite", driverErr.Code()
	}
	fmt.Fprintf(stderr, "compaction plan: inspection failed; no data transformed and no valid plan emitted (stage=%s category=%s sqlite_code=%d)\n",
		sqlite.SubscriptionCompactionInspectionStage(err), category, code)
}

func runSubscriptionCompactionVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compact-subscription-storage verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var dbPath, baselinePath string
	var timeout time.Duration
	fs.StringVar(&dbPath, "db", "", "explicit path to an authorized offline candidate database")
	fs.StringVar(&baselinePath, "baseline", "", "sealed baseline plan produced by compact-subscription-storage plan")
	fs.DurationVar(&timeout, "timeout", 5*time.Minute, "bounded offline verification duration (maximum 30m)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if dbPath == "" || baselinePath == "" || fs.NArg() != 0 || timeout <= 0 || timeout > 30*time.Minute {
		fmt.Fprintln(stderr, "compaction verify: explicit --db, --baseline and a timeout in (0,30m] are required; positional arguments are forbidden")
		return 2
	}
	for _, path := range []string{dbPath, baselinePath} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			fmt.Fprintln(stderr, "compaction verify: candidate and baseline must be existing regular files, not symlinks")
			return 1
		}
	}
	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		fmt.Fprintln(stderr, "compaction verify: cannot read baseline plan")
		return 1
	}
	var baseline domain.SubscriptionStoragePlan
	if err := json.Unmarshal(raw, &baseline); err != nil {
		fmt.Fprintln(stderr, "compaction verify: invalid baseline plan")
		return 1
	}
	db, err := sqlite.OpenReadOnly(dbPath)
	if err != nil {
		fmt.Fprintln(stderr, "compaction verify: cannot open candidate read-only")
		return 1
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	report, err := sqlite.VerifySubscriptionCompaction(ctx, db, &baseline)
	if err != nil {
		fmt.Fprintln(stderr, "compaction verify: verification failed; candidate is not approved")
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		fmt.Fprintln(stderr, "compaction verify: output failed")
		return 1
	}
	if !report.Verified {
		fmt.Fprintln(stderr, "compaction verify: invariant or protected-content mismatch (maintenance_required)")
		return 1
	}
	return 0
}
