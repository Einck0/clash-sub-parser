package sqlite_test

import (
	"context"
	"strings"
	"testing"

	"clash-sub-parser/internal/repository/sqlite"
)

func TestOfflineCompactionLatestAttemptChronologicalOrder(t *testing.T) {
	for _, tc := range []struct {
		name, appliedAt, failedAt string
	}{
		{"mixed precision", "2026-11-10T02:00:00Z", "2026-11-10T02:00:00.500Z"},
		{"timezone offset", "2026-11-10T03:00:00+01:00", "2026-11-10T02:00:00.500Z"},
		{"equal instant uses rowid", "2026-11-10T02:00:00Z", "2026-11-10T02:00:00.000Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, _, decisions := preparedCompactionScratch(t)
			// Bind each statement separately: the SQLite driver starts anonymous
			// parameter numbering again for each statement in a multi-statement Exec.
			if _, err := db.ExecContext(ctx, `UPDATE subscription_fetches SET started_at=?,outcome='partial' WHERE id='current-fetch'`, tc.appliedAt); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE subscription_fetches SET started_at=? WHERE id='failed-fetch'`, tc.failedAt); err != nil {
				t.Fatal(err)
			}
			// Prove the counterexample exists BEFORE planning or deleting rows.
			for _, expected := range []struct{ id, timestamp, outcome string }{
				{"current-fetch", tc.appliedAt, "partial"},
				{"failed-fetch", tc.failedAt, "failed"},
			} {
				var timestamp, outcome string
				if err := db.QueryRowContext(ctx, `SELECT started_at,outcome FROM subscription_fetches WHERE id=?`, expected.id).Scan(&timestamp, &outcome); err != nil {
					t.Fatal(err)
				}
				if timestamp != expected.timestamp || outcome != expected.outcome {
					t.Fatalf("invalid input fixture %s: timestamp=%q outcome=%q, want timestamp=%q outcome=%q", expected.id, timestamp, outcome, expected.timestamp, expected.outcome)
				}
			}
			baseline, err := sqlite.InspectSubscriptionCompaction(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			if len(baseline.Subscriptions) != 1 || baseline.Subscriptions[0].LastAttemptID != "failed-fetch" || baseline.Subscriptions[0].LastAttemptStatus != "failed" {
				t.Fatalf("plan lost latest attempt: %+v", baseline.Subscriptions)
			}
			if _, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions, true); err != nil {
				t.Fatal(err)
			}
			var applied, attempt, status, timestamp string
			if err := db.QueryRowContext(ctx, `SELECT c.applied_fetch_id,c.last_attempt_fetch_id,c.last_attempt_status,f.started_at
				FROM subscription_current_state c JOIN subscription_fetches f ON f.id=c.last_attempt_fetch_id`).Scan(&applied, &attempt, &status, &timestamp); err != nil {
				t.Fatal(err)
			}
			if applied != "current-fetch" || attempt != "failed-fetch" || status != "failed" || timestamp != tc.failedAt {
				t.Fatalf("lost exact applied/attempt facts: applied=%q attempt=%q status=%q timestamp=%q, want timestamp=%q", applied, attempt, status, timestamp, tc.failedAt)
			}
			verified, err := sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
			if err != nil || !verified.Verified {
				t.Fatalf("verification=%+v err=%v", verified, err)
			}
		})
	}
}

func TestVerifyCompactionUsesOriginalLastAttemptNotOnlySurvivors(t *testing.T) {
	for _, branch := range []string{"deleted latest attempt", "changed outcome", "missing current state"} {
		t.Run(branch, func(t *testing.T) {
			ctx := context.Background()
			db, baseline, decisions := preparedCompactionScratch(t)
			if _, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions, true); err != nil {
				t.Fatal(err)
			}
			mutation := map[string]string{
				"deleted latest attempt": `UPDATE subscription_current_state SET last_attempt_fetch_id=applied_fetch_id,last_attempt_status='success'; DELETE FROM subscription_fetches WHERE id='failed-fetch'`,
				"changed outcome":        `UPDATE subscription_fetches SET outcome='partial' WHERE id='failed-fetch'; UPDATE subscription_current_state SET last_attempt_status='partial'`,
				"missing current state":  `DELETE FROM subscription_current_state`,
			}[branch]
			if _, err := db.ExecContext(ctx, mutation); err != nil {
				t.Fatal(err)
			}
			// All surviving attempt rows can be internally consistent while
			// contradicting the immutable pre-deletion plan.
			verified, err := sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
			if err != nil {
				t.Fatal(err)
			}
			requireCompactionFailure(t, verified.Verified, verified.Failures, "last_attempt_changed:compact-sub")
		})
	}
}

func TestOfflineCompactionHistoricalVersionsClosureAndRollback(t *testing.T) {
	for _, branch := range []string{"unconfirmed closure", "closed", "unknown expired lease", "delete trigger"} {
		t.Run(branch, func(t *testing.T) {
			ctx := context.Background()
			db, _, decisions := preparedCompactionScratch(t)
			// Revision zero must remain when closure is unconfirmed and can be
			// reclaimed only after explicit confirmation, with no active lease.
			if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_versions(node_logical_id,connection_revision,effective_config_json,config_fingerprint,source_entry_id,created_at)
				VALUES('zero-node',0,'{"server":"192.0.2.99","port":8388,"credentials":{}}','synthetic-old','old-entry','2026-11-10T00:00:00Z')`); err != nil {
				t.Fatal(err)
			}
			if branch == "unknown expired lease" {
				if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_version_leases VALUES('unknown-consumer','unknown-id','zero-node',0,'unknown-owner','unknown-token','2020-01-01T00:00:00Z','2020-01-01T00:01:00Z')`); err != nil {
					t.Fatal(err)
				}
			}
			if branch == "delete trigger" {
				if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_offline_version_delete BEFORE DELETE ON node_connection_versions BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			baseline, err := sqlite.InspectSubscriptionCompaction(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			_, err = sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions, branch != "unconfirmed closure")
			if branch == "closed" {
				if err != nil {
					t.Fatal(err)
				}
				var count int
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions WHERE node_logical_id='zero-node' AND connection_revision=0`).Scan(&count); err != nil || count != 0 {
					t.Fatalf("confirmed closure did not reclaim old version: count=%d err=%v", count, err)
				}
				verified, err := sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
				if err != nil || !verified.Verified || !verified.ConsumerClosureSealed {
					t.Fatalf("confirmed closure verification=%+v err=%v", verified, err)
				}
				return
			}
			if err == nil {
				t.Fatal("unsafe conversion reported success")
			}
			after, inspectErr := sqlite.InspectSubscriptionCompaction(ctx, db)
			if inspectErr != nil || after.SnapshotDigest != baseline.SnapshotDigest {
				t.Fatalf("refused conversion changed scratch/lease: %v", inspectErr)
			}
			var config, entry string
			if err := db.QueryRowContext(ctx, `SELECT effective_config_json,source_entry_id FROM node_connection_versions WHERE node_logical_id='zero-node' AND connection_revision=0`).Scan(&config, &entry); err != nil || config != `{"server":"192.0.2.99","port":8388,"credentials":{}}` || entry != "old-entry" {
				t.Fatalf("refused conversion lost historical version: config=%q entry=%q err=%v", config, entry, err)
			}
		})
	}
}

func TestOfflineCompactionPropagatesLeaseQueryError(t *testing.T) {
	ctx := context.Background()
	db, baseline, decisions := preparedCompactionScratch(t)
	if _, err := db.ExecContext(ctx, `DROP TABLE node_connection_version_leases`); err != nil {
		t.Fatal(err)
	}
	_, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions, true)
	if err == nil || !strings.Contains(err.Error(), "node_connection_version_leases") {
		t.Fatalf("lease SQL error was hidden: %v", err)
	}
	var payloads int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_payloads`).Scan(&payloads); err != nil || payloads != 3 {
		t.Fatalf("failed SQL check changed scratch: payloads=%d err=%v", payloads, err)
	}
}

func TestVerifyCompactionRejectsUnleasedHistoryButAllowsLease(t *testing.T) {
	ctx := context.Background()
	db, baseline, decisions := preparedCompactionScratch(t)
	if _, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_versions(node_logical_id,connection_revision,effective_config_json,config_fingerprint,created_at) VALUES('zero-node',0,'{}','synthetic-old','2026-11-10T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	verified, err := sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
	if err != nil {
		t.Fatal(err)
	}
	requireCompactionFailure(t, verified.Verified, verified.Failures, "unleased_nonhead_version")
	if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_version_leases VALUES('unknown-consumer','unknown-id','zero-node',0,'unknown-owner','unknown-token','2020-01-01T00:00:00Z','2020-01-01T00:01:00Z')`); err != nil {
		t.Fatal(err)
	}
	verified, err = sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
	if err != nil || !verified.Verified {
		t.Fatalf("unknown expired lease must protect historical version: %+v %v", verified, err)
	}
}

func requireCompactionFailure(t *testing.T, verified bool, failures []string, expected string) {
	t.Helper()
	if verified {
		t.Fatalf("unsafe candidate verified despite %s", expected)
	}
	for _, failure := range failures {
		if failure == expected {
			return
		}
	}
	t.Fatalf("missing %s in %+v", expected, failures)
}
