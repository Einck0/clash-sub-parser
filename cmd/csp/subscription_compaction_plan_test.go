package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

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
