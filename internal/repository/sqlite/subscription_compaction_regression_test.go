package sqlite_test

import (
	"context"
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
			if _, err := db.ExecContext(ctx, `UPDATE subscription_fetches SET started_at=?,outcome='partial' WHERE id='current-fetch';
				UPDATE subscription_fetches SET started_at=? WHERE id='failed-fetch'`, tc.appliedAt, tc.failedAt); err != nil {
				t.Fatal(err)
			}
			baseline, err := sqlite.InspectSubscriptionCompaction(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			if len(baseline.Subscriptions) != 1 || baseline.Subscriptions[0].LastAttemptID != "failed-fetch" || baseline.Subscriptions[0].LastAttemptStatus != "failed" {
				t.Fatalf("plan lost latest attempt: %+v", baseline.Subscriptions)
			}
			if _, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions); err != nil {
				t.Fatal(err)
			}
			var applied, attempt, status, timestamp string
			if err := db.QueryRowContext(ctx, `SELECT c.applied_fetch_id,c.last_attempt_fetch_id,c.last_attempt_status,f.started_at
				FROM subscription_current_state c JOIN subscription_fetches f ON f.id=c.last_attempt_fetch_id`).Scan(&applied, &attempt, &status, &timestamp); err != nil {
				t.Fatal(err)
			}
			if applied != "current-fetch" || attempt != "failed-fetch" || status != "failed" || timestamp != tc.failedAt {
				t.Fatalf("lost exact applied/attempt facts: %s %s %s %s", applied, attempt, status, timestamp)
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
			if _, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions); err != nil {
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

func TestOfflineCompactionReclaimsUntouchedHistoryAndPreservesOrigin(t *testing.T) {
	for _, branch := range []string{"reclaim", "unknown expired lease", "delete failure"} {
		t.Run(branch, func(t *testing.T) {
			ctx := context.Background()
			db, _, decisions := preparedCompactionScratch(t)
			// Revision zero is valid historical data; current node revision is
			// still 1. This node will never receive a runtime update in this test.
			if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_versions(node_logical_id,connection_revision,effective_config_json,config_fingerprint,source_entry_id,created_at)
				VALUES('zero-node',0,'{"server":"192.0.2.99","port":8388,"credentials":{}}','synthetic-old','old-entry','2026-11-10T00:00:00Z')`); err != nil {
				t.Fatal(err)
			}
			if branch == "unknown expired lease" {
				if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_version_leases VALUES('unknown-consumer','unknown-id','zero-node',0,'unknown-owner','unknown-token','2020-01-01T00:00:00Z','2020-01-01T00:01:00Z')`); err != nil {
					t.Fatal(err)
				}
			}
			if branch == "delete failure" {
				if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_offline_version_delete BEFORE DELETE ON node_connection_versions BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			baseline, err := sqlite.InspectSubscriptionCompaction(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			result, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions)
			if branch != "reclaim" {
				if err == nil {
					t.Fatal("unresolved lease or failed deletion must refuse conversion")
				}
				after, inspectErr := sqlite.InspectSubscriptionCompaction(ctx, db)
				if inspectErr != nil || after.SnapshotDigest != baseline.SnapshotDigest {
					t.Fatalf("refused conversion changed scratch/lease: %v", inspectErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Counts["node_connection_versions"] != 1 || result.Counts["node_connection_heads"] != 1 || result.Counts["nodes"] != 1 {
				t.Fatalf("untouched full history remains: %+v", result.Counts)
			}
			var origin, digest string
			if err := db.QueryRowContext(ctx, `SELECT p.origin_payload_id,e.content_digest FROM node_connection_provenance p JOIN subscription_payload_evidence e ON e.payload_id=p.origin_payload_id WHERE p.node_logical_id='zero-node' AND p.connection_revision=0`).Scan(&origin, &digest); err != nil || origin != "old" || digest != "sha256:digest" {
				t.Fatalf("retired version origin lost: %s %s %v", origin, digest, err)
			}
			verified, err := sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
			if err != nil || !verified.Verified {
				t.Fatalf("verification=%+v err=%v", verified, err)
			}
		})
	}
}

func TestVerifyCompactionRejectsUnleasedHistoryButAllowsLease(t *testing.T) {
	ctx := context.Background()
	db, baseline, decisions := preparedCompactionScratch(t)
	if _, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions); err != nil {
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
