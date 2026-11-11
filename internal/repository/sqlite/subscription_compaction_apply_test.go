package sqlite_test

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func preparedCompactionScratch(t *testing.T) (*sql.DB, *domain.SubscriptionStoragePlan, []sqlite.SubscriptionCompactionDecision) {
	t.Helper()
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.RunUpTo(ctx, 29); err != nil {
		t.Fatal(err)
	}
	retentionCreateTestSubscription(t, db, "compact-sub", "Synthetic")
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"old", "middle", "current"} {
		retentionInsertTestFetch(t, db, id+"-fetch", "compact-sub", now.Add(time.Duration(i)*time.Hour), domain.FetchOutcomeSuccess)
		retentionInsertTestPayload(t, db, id, "compact-sub", id+"-fetch", now.Add(time.Duration(i)*time.Hour), true)
		retentionInsertTestEntry(t, db, id+"-entry", id, "compact-sub", "key", "Synthetic", domain.EntryKindNotice, nil)
	}
	retentionInsertTestFetch(t, db, "failed-fetch", "compact-sub", now.Add(3*time.Hour), domain.FetchOutcomeFailed)
	if _, err := db.ExecContext(ctx, `INSERT INTO subscription_original_configs(subscription_id,source_revision,content_digest,body_blob,fetch_id)VALUES('compact-sub','rev-1','sha256:digest',x'78','current-fetch');
		INSERT INTO nodes(logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at,connection_revision)VALUES('zero-node','ss','Synthetic','192.0.2.1',8388,'{}',1,'2026-11-10T00:00:00Z','2026-11-10T00:00:00Z',1);`); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	plan, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return db, plan, []sqlite.SubscriptionCompactionDecision{{SubscriptionID: "compact-sub", SourceRevision: "rev-1", PayloadID: "current"}}
}

func TestOfflineCompactionPreparedScratchSingleGeneration(t *testing.T) {
	ctx := context.Background()
	db, baseline, decisions := preparedCompactionScratch(t)
	result, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions)
	if err != nil {
		t.Fatal(err)
	}
	if result.Counts["subscription_payloads"] != 1 || result.Counts["subscription_entries"] != 1 || result.Counts["subscription_payload_evidence"] != 3 || result.Counts["subscription_fetches"] != 2 {
		t.Fatalf("unexpected compact counts: %+v", result.Counts)
	}
	if !reflect.DeepEqual(result.ProtectedDigests, baseline.ProtectedDigests) {
		t.Fatal("protected assets changed")
	}
	verification, err := sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
	if err != nil || !verification.Verified || verification.ConsumerClosureSealed {
		t.Fatalf("verification=%+v err=%v", verification, err)
	}
	var applied, attempt, status string
	if err := db.QueryRowContext(ctx, `SELECT applied_fetch_id,last_attempt_fetch_id,last_attempt_status FROM subscription_current_state`).Scan(&applied, &attempt, &status); err != nil {
		t.Fatal(err)
	}
	if applied != "current-fetch" || attempt != "failed-fetch" || status != "failed" {
		t.Fatalf("applied/attempt conflated: %s %s %s", applied, attempt, status)
	}
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT connection_revision FROM node_connection_heads WHERE logical_id='zero-node'`).Scan(&revision); err != nil || revision != 1 {
		t.Fatalf("current revision changed: %d %v", revision, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO subscription_payloads(id,subscription_id,fetch_id,content_digest,body_blob,created_at)VALUES('second','compact-sub','current-fetch','digest',X'','2026-11-10T00:00:00Z')`); err == nil {
		t.Fatal("single generation constraint missing")
	}
}

func TestOfflineCompactionMembershipRevalidatedAfterReconciliation(t *testing.T) {
	for _, branch := range []string{"valid current membership", "evidence removed during reconciliation"} {
		t.Run(branch, func(t *testing.T) {
			ctx := context.Background()
			db, _, decisions := preparedCompactionScratch(t)
			if _, err := db.Exec(`UPDATE subscription_entries SET node_logical_id='zero-node' WHERE id='current-entry';
				INSERT INTO node_sources(node_logical_id,subscription_id,last_seen_fetch_id) VALUES('zero-node','compact-sub','current-fetch')`); err != nil {
				t.Fatal(err)
			}
			if branch == "evidence removed during reconciliation" {
				// Planning and input validation see valid membership. Only the
				// finalizer's post-reconciliation proof can catch this change.
				if _, err := db.Exec(`CREATE TRIGGER remove_compaction_membership AFTER INSERT ON node_source_history
					BEGIN UPDATE subscription_entries SET node_logical_id=NULL WHERE id='current-entry'; END`); err != nil {
					t.Fatal(err)
				}
			}
			baseline, err := sqlite.InspectSubscriptionCompaction(ctx, db)
			if err != nil || !baseline.ApplySupported {
				t.Fatalf("expected valid pre-reconciliation evidence: plan=%+v err=%v", baseline, err)
			}
			result, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions)
			if branch == "evidence removed during reconciliation" {
				if err == nil || !strings.Contains(err.Error(), "unreconciled source membership") {
					t.Fatalf("post-reconciliation missing evidence was not rejected by membership check: %v", err)
				}
				after, inspectErr := sqlite.InspectSubscriptionCompaction(ctx, db)
				if inspectErr != nil || !reflect.DeepEqual(after, baseline) {
					t.Fatalf("membership refusal did not fully roll back: %v", inspectErr)
				}
				return
			}
			if err != nil || result.Counts["node_sources"] != 1 {
				t.Fatalf("valid current membership lost: result=%+v err=%v", result, err)
			}
			verification, err := sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
			if err != nil || !verification.Verified {
				t.Fatalf("valid membership conversion verification=%+v err=%v", verification, err)
			}
		})
	}
}

func TestOfflineCompactionRejectsUnprovedAndChangedInputsWithoutMutation(t *testing.T) {
	for _, branch := range []string{"missing", "duplicate", "source_revision", "older_generation", "raw_mismatch", "changed_snapshot", "cancelled", "invalid_current_revision", "contradictory_head", "conflicting_applied", "conflicting_applied_revision", "half_applied", "changed_projection"} {
		t.Run(branch, func(t *testing.T) {
			db, baseline, decisions := preparedCompactionScratch(t)
			ctx := context.Background()
			switch branch {
			case "missing":
				decisions = nil
			case "duplicate":
				decisions = append(decisions, decisions[0])
			case "source_revision":
				decisions[0].SourceRevision = "different"
			case "older_generation":
				decisions[0].PayloadID = "old"
			case "conflicting_applied", "conflicting_applied_revision", "half_applied":
				payload, revision := "middle", "rev-1"
				if branch == "conflicting_applied_revision" {
					payload, revision = "current", "rev-old"
				}
				if _, err := db.Exec(`INSERT INTO subscription_current_state(subscription_id,source_revision,applied_payload_id,applied_fetch_id)VALUES('compact-sub',?,?,?)`, revision, payload, payload+"-fetch"); err != nil {
					t.Fatal(err)
				}
				if branch == "half_applied" {
					if _, err := db.Exec(`UPDATE subscription_current_state SET applied_payload_id=NULL`); err != nil {
						t.Fatal(err)
					}
				}
				var err error
				baseline, err = sqlite.InspectSubscriptionCompaction(ctx, db)
				if err != nil {
					t.Fatal(err)
				}
			case "raw_mismatch":
				if _, err := db.Exec(`UPDATE subscription_original_configs SET content_digest='other'`); err != nil {
					t.Fatal(err)
				}
				var err error
				baseline, err = sqlite.InspectSubscriptionCompaction(ctx, db)
				if err != nil {
					t.Fatal(err)
				}
			case "changed_projection":
				// Snapshot hash alone must not bless attacker-supplied projections.
				baseline.ReconciledSourcesDigest = "forged"
			case "changed_snapshot":
				if _, err := db.Exec(`UPDATE nodes SET display_name='changed'`); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "invalid_current_revision":
				// Simulate storage corruption, not a supported legacy node. Restore
				// constraint enforcement before apply; it must reject without
				// renumbering the node or changing any facts.
				if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON;
					UPDATE nodes SET connection_revision=0 WHERE logical_id='zero-node';
					PRAGMA ignore_check_constraints=OFF`); err != nil {
					t.Fatal(err)
				}
				var err error
				baseline, err = sqlite.InspectSubscriptionCompaction(ctx, db)
				if err != nil {
					t.Fatal(err)
				}
			case "contradictory_head":
				if _, err := db.Exec(`INSERT INTO node_connection_versions(node_logical_id,connection_revision,effective_config_json,config_fingerprint,created_at)VALUES('zero-node',1,'{"server":"other","port":8388,"credentials":{}}','bad','2026-11-10T00:00:00Z')`); err != nil {
					t.Fatal(err)
				}
				var err error
				baseline, err = sqlite.InspectSubscriptionCompaction(ctx, db)
				if err != nil {
					t.Fatal(err)
				}
			}
			before, err := sqlite.InspectSubscriptionCompaction(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := sqlite.FinalizeSubscriptionCompaction(ctx, db, baseline, decisions); err == nil {
				t.Fatal("unsafe decision reported success")
			}
			after, err := sqlite.InspectSubscriptionCompaction(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if before.SnapshotDigest != after.SnapshotDigest {
				t.Fatal("rejected transformation did not fully roll back")
			}
		})
	}
}
