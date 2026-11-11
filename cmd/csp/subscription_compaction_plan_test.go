package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestCompactionInspectionSafeErrorClassification(t *testing.T) {
	const secret = "secret://sentinel-password SQL /private/snapshot.db"
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, driverErr := db.Exec(`SELECT * FROM "secret://sentinel-password"`)
	if driverErr == nil {
		t.Fatal("expected real SQLite error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, inspectionErr := sqlite.InspectSubscriptionCompaction(ctx, db)
	for _, tc := range []struct {
		name, category, stage string
		err                   error
	}{
		{"deadline", "deadline", "unknown", fmt.Errorf("%s: %w", secret, context.DeadlineExceeded)},
		{"cancelled", "cancelled", "unknown", fmt.Errorf("%s: %w", secret, context.Canceled)},
		{"sqlite", "sqlite", "unknown", fmt.Errorf("%s: %w", secret, driverErr)},
		{"unknown", "unknown", "unknown", errors.New(secret)},
		{"trusted stage", "cancelled", "begin_snapshot", fmt.Errorf("%s: %w", secret, inspectionErr)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			writeCompactionInspectionFailure(&stderr, tc.err)
			text := stderr.String()
			if strings.Contains(text, "sentinel") || strings.Contains(text, "SQL") || strings.Contains(text, "/private") {
				t.Fatalf("untrusted error leaked: %s", text)
			}
			if !strings.Contains(text, "category="+tc.category) || !strings.Contains(text, "stage="+tc.stage) {
				t.Fatalf("incorrect safe classification: %s", text)
			}
			if tc.category == "sqlite" && !strings.Contains(text, "sqlite_code=1)") {
				t.Fatalf("real SQLite numeric code missing: %s", text)
			}
		})
	}
}

func TestCompactionPlanInspectionFailureEmitsNoApprovedReport(t *testing.T) {
	for _, branch := range []string{"secret-bearing scan error", "deadline"} {
		t.Run(branch, func(t *testing.T) {
			f := makeCompactionCLIFixture(t)
			if branch == "secret-bearing scan error" {
				cfg := sqlite.DefaultConfig(f.scratch)
				cfg.WALMode = false
				db, err := sqlite.Open(cfg)
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Exec(`DROP TABLE schema_migrations; CREATE TABLE schema_migrations(version TEXT); INSERT INTO schema_migrations VALUES('secret://sentinel-password')`)
				db.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(f.scratch)
			if err != nil {
				t.Fatal(err)
			}
			report := filepath.Join(filepath.Dir(f.scratch), "failed-plan.json")
			args := []string{"plan", "--db", f.scratch, "--report", report}
			if branch == "deadline" {
				args = append(args, "--timeout", "1ns")
			}
			var out, stderr bytes.Buffer
			if code := runSubscriptionCompaction(args, &out, &stderr); code != 1 || out.Len() != 0 {
				t.Fatalf("failed plan exit=%d stdout=%s stderr=%s", code, out.String(), stderr.String())
			}
			text := stderr.String()
			if strings.Contains(text, "sentinel") || strings.Contains(text, f.scratch) || strings.Contains(text, "schema_migrations") {
				t.Fatalf("failure leaked source details: %s", text)
			}
			if branch == "deadline" && (!strings.Contains(text, "category=deadline") || !strings.Contains(text, "stage=begin_snapshot")) {
				t.Fatalf("timeout not diagnosed: %s", text)
			}
			if branch != "deadline" && (!strings.Contains(text, "category=unknown") || !strings.Contains(text, "stage=validate_schema")) {
				t.Fatalf("scan failure not diagnosed: %s", text)
			}
			raw, err := os.ReadFile(report)
			if err != nil || len(raw) != 0 {
				t.Fatalf("failure emitted a usable plan: bytes=%d err=%v", len(raw), err)
			}
			after, err := os.ReadFile(f.scratch)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("failed plan changed input: %v", err)
			}
		})
	}
}

func TestSubscriptionCompactionPlanBlocksAmbiguousLegacyGenerations(t *testing.T) {
	for _, branch := range []string{"unknown_membership", "missing_payload", "duplicate_payload", "invalid_time"} {
		t.Run(branch, func(t *testing.T) {
			f := makeCompactionCLIFixture(t)
			cfg := sqlite.DefaultConfig(f.scratch)
			cfg.WALMode = false
			db, err := sqlite.Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			q := map[string]string{
				"unknown_membership": `UPDATE node_sources SET last_seen_fetch_id='unknown'`,
				"missing_payload":    `DELETE FROM subscription_payloads WHERE id='current'`,
				"duplicate_payload":  `INSERT INTO subscription_payloads(id,subscription_id,fetch_id,content_digest,body_blob,created_at)VALUES('duplicate','sub','partial-fetch','digest',X'','2026-11-10T01:00:00Z')`,
				"invalid_time":       `UPDATE subscription_fetches SET started_at='invalid' WHERE id='partial-fetch'`,
			}[branch]
			if _, err := db.ExecContext(context.Background(), q); err != nil {
				db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(f.scratch)
			if err != nil {
				t.Fatal(err)
			}
			report := filepath.Join(filepath.Dir(f.scratch), "blocked-plan.json")
			var out, stderr bytes.Buffer
			if code := runSubscriptionCompaction([]string{"plan", "--db", f.scratch, "--report", report}, &out, &stderr); code != 0 {
				t.Fatalf("plan exit %d: %s", code, stderr.String())
			}
			raw, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			var plan domain.SubscriptionStoragePlan
			if err := json.Unmarshal(raw, &plan); err != nil {
				t.Fatal(err)
			}
			if plan.ApplySupported {
				t.Fatal("ambiguous legacy input admitted")
			}
			found := false
			for _, b := range plan.Blockers {
				if b == "legacy_generation_or_membership_unresolved" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing blocker: %+v", plan.Blockers)
			}
			after, err := os.ReadFile(f.scratch)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("blocked plan wrote input: %v", err)
			}
		})
	}
}
