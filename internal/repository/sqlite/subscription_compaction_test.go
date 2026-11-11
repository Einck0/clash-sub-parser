package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"

	"clash-sub-parser/migrations"
	"testing"

	"clash-sub-parser/internal/repository/sqlite"
)

func TestCompactionInspectionReadOnlyAndDeterministic(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	// Sentinel includes TEXT/BLOB forms which JSON alone cannot distinguish.
	if _, err := db.ExecContext(ctx, `CREATE TABLE compaction_protected_fixture (id INTEGER PRIMARY KEY, value);
		INSERT INTO compaction_protected_fixture VALUES (1,'synthetic')`); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := db.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	first, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.ApplySupported || len(first.Blockers) == 0 {
		t.Fatal("plan must be stable and explicitly incomplete, not apply approval")
	}
	var after int64
	if err := db.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&after); err != nil || after != before {
		t.Fatalf("inspection wrote to the database: before=%d after=%d err=%v", before, after, err)
	}
	// Digest must detect a protected content change with identical row count.
	if _, err := db.ExecContext(ctx, `UPDATE compaction_protected_fixture SET value='changed' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	changed, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Counts["compaction_protected_fixture"] != first.Counts["compaction_protected_fixture"] ||
		changed.ProtectedDigests["compaction_protected_fixture"] == first.ProtectedDigests["compaction_protected_fixture"] ||
		changed.SnapshotDigest == first.SnapshotDigest {
		t.Fatal("content fingerprints must not reduce to row counts")
	}
	// A BLOB and TEXT containing its base64 encoding must not collide.
	if _, err := db.ExecContext(ctx, `UPDATE compaction_protected_fixture SET value=x'78' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	blob, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE compaction_protected_fixture SET value='eA==' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	text, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if blob.ProtectedDigests["compaction_protected_fixture"] == text.ProtectedDigests["compaction_protected_fixture"] {
		t.Fatal("SQLite storage type must be part of content fingerprint")
	}
}

func TestCompactionInspectionDoesNotCreateMigrationTable(t *testing.T) {
	db, _ := setupEmptyTestDB(t)
	ctx := context.Background()
	if _, err := sqlite.InspectSubscriptionCompaction(ctx, db); err == nil {
		t.Fatal("missing legacy facts must fail, not synthesize an empty migration")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE name='schema_migrations'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("plan created schema_migrations: %d %v", count, err)
	}
}

func TestCompactionInspectionCancellationFailsClosed(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sqlite.InspectSubscriptionCompaction(ctx, db); err == nil {
		t.Fatal("cancelled plan cannot be used as an empty successful plan")
	}
}

func TestStorageStartupLegacyRejectedBeforeAnyMigrationWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	cfg := sqlite.DefaultConfig(path)
	cfg.WALMode = false
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewMigrationRunner(db, migrations.FS).RunUpTo(ctx, 27); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := sqlite.OpenAndMigrate(ctx, cfg); err == nil {
		opened.Close()
		t.Fatal("legacy startup must refuse maintenance_required")
	} else if !strings.Contains(err.Error(), "maintenance_required") {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected startup changed database: %v", err)
	}
	readOnly, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	var version int
	if err := readOnly.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 27 {
		t.Fatalf("legacy schema mutated: %d %v", version, err)
	}
	var count int
	if err := readOnly.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE name='subscription_storage_readiness'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("startup performed preparation: %d %v", count, err)
	}
}

func TestStorageStartupFreshReadyAndForgedIndexRejected(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fresh.db")
	cfg := sqlite.DefaultConfig(path)
	cfg.WALMode = false
	db, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CheckSubscriptionStorageReady(ctx, db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	for _, ddl := range []string{
		`DROP INDEX idx_subscription_single_payload; CREATE INDEX idx_subscription_single_payload ON subscription_payloads(subscription_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_single_payload ON subscription_payloads(subscription_id); DROP TRIGGER subscription_payload_empty_body_insert; CREATE TRIGGER subscription_payload_empty_body_insert BEFORE INSERT ON subscription_payloads WHEN 0 BEGIN SELECT 1; END`,
	} {
		reopened, err := sqlite.Open(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reopened.ExecContext(ctx, ddl); err != nil {
			reopened.Close()
			t.Fatal(err)
		}
		reopened.Close()
		if opened, err := sqlite.OpenAndMigrate(ctx, cfg); err == nil {
			opened.Close()
			t.Fatal("ready flag with weakened constraint or trigger must not pass")
		}
	}
	alias := filepath.Join(t.TempDir(), "alias.db")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if opened, err := sqlite.OpenAndMigrate(ctx, sqlite.DefaultConfig(alias)); err == nil {
		opened.Close()
		t.Fatal("database symlink must not pass startup preflight")
	}
	missing := filepath.Join(t.TempDir(), "sidecar-only.db")
	if err := os.WriteFile(missing+"-wal", []byte("wal"), 0600); err != nil {
		t.Fatal(err)
	}
	if opened, err := sqlite.OpenAndMigrate(ctx, sqlite.DefaultConfig(missing)); err == nil {
		opened.Close()
		t.Fatal("orphan sidecar must not initialize as a new database")
	}
}

func TestStorageStartupUnclosedCandidatePreservesHistory(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.DefaultConfig(filepath.Join(t.TempDir(), "candidate.db"))
	cfg.WALMode = false
	db, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var closed int
	if err := db.QueryRowContext(ctx, `SELECT consumers_closed FROM subscription_storage_readiness WHERE singleton=1`).Scan(&closed); err != nil || closed != 1 {
		t.Fatalf("fresh storage closure=%d err=%v", closed, err)
	}
	baseline, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE subscription_storage_readiness SET consumers_closed=0 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	report, err := sqlite.VerifySubscriptionCompaction(ctx, db, baseline)
	if err != nil || report.Verified || report.ConsumerClosureSealed {
		t.Fatalf("unclosed candidate verified: %+v err=%v", report, err)
	}
	requireCompactionFailure(t, report.Verified, report.Failures, "version_consumers_not_closed")
	repo := sqlite.NewNodeRepository(db)
	node := domain.Node{LogicalID: "reopened-node", Protocol: domain.ProtocolSS, DisplayName: "Synthetic", Server: "192.0.2.1", Port: 8388, Active: true}
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	node.Server = "192.0.2.2"
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("sealed unclosed database must start: %v", err)
	}
	defer db.Close()
	if err := db.QueryRowContext(ctx, `SELECT consumers_closed FROM subscription_storage_readiness WHERE singleton=1`).Scan(&closed); err != nil || closed != 0 {
		t.Fatalf("startup changed closure: %d %v", closed, err)
	}
	repo = sqlite.NewNodeRepository(db)
	node.Server = "192.0.2.3"
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatalf("refresh with unclosed consumers: %v", err)
	}
	versions := sqlite.NewNodeConnectionVersionRepository(db)
	for revision := int64(1); revision <= 3; revision++ {
		if _, err := versions.GetByRevision(ctx, node.LogicalID, revision); err != nil {
			t.Fatalf("unclosed GC removed revision %d: %v", revision, err)
		}
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.PruneConnectionVersionsTx(ctx, tx, node.LogicalID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := versions.GetByRevision(ctx, node.LogicalID, 1); err != nil {
		t.Fatalf("explicit GC removed unclosed history: %v", err)
	}
	// Fixture-only closure: actual operation requires an offline drain and an
	// independently validated consumer contract, not this SQL assignment.
	sealTestVersionConsumers(t, db)
	now := time.Now().UTC()
	lease := domain.ConnectionVersionLease{ConsumerKind: "fixture", ConsumerID: "fixture", NodeID: node.LogicalID, Revision: 1, Owner: "fixture", FencingToken: "fixture", HeartbeatAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := sqlite.AcquireConnectionVersionLeaseTx(ctx, tx, lease)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.PruneConnectionVersionsTx(ctx, tx, node.LogicalID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := versions.GetByRevision(ctx, node.LogicalID, 2); err == nil {
		t.Fatal("closed GC retained unleased revision")
	}
	for _, revision := range []int64{1, 3} {
		if _, err := versions.GetByRevision(ctx, node.LogicalID, revision); err != nil {
			t.Fatalf("closed GC lost leased revision or head %d: %v", revision, err)
		}
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.ReleaseConnectionVersionLeaseTx(ctx, tx, lease)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := versions.GetByRevision(ctx, node.LogicalID, 1); err == nil {
		t.Fatal("released old version not reclaimed")
	}
}

func TestStorageStartupRejectsInvalidClosure(t *testing.T) {
	for _, branch := range []string{"invalid value", "missing column", "unsealed"} {
		t.Run(branch, func(t *testing.T) {
			ctx := context.Background()
			cfg := sqlite.DefaultConfig(filepath.Join(t.TempDir(), "candidate.db"))
			cfg.WALMode = false
			db, err := sqlite.OpenAndMigrate(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			mutation := map[string]string{
				"invalid value":  `PRAGMA ignore_check_constraints=ON; UPDATE subscription_storage_readiness SET consumers_closed=2`,
				"missing column": `ALTER TABLE subscription_storage_readiness RENAME COLUMN consumers_closed TO broken_closure`,
				"unsealed":       `DELETE FROM subscription_storage_readiness`,
			}[branch]
			if _, err := db.ExecContext(ctx, mutation); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if opened, err := sqlite.OpenAndMigrate(ctx, cfg); err == nil {
				opened.Close()
				t.Fatal("invalid closure or unsealed storage started")
			} else if !strings.Contains(err.Error(), "maintenance_required") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStoragePreparationDoesNotAutomaticallySealLegacy(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.RunUpTo(ctx, 29); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.CheckSubscriptionStorageReady(ctx, db); err == nil {
		t.Fatal("preparation DDL must not imply conversion ready")
	}
}

func TestSingleGenerationConstraintsEvenForPinnedPayloads(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	retentionCreateTestSubscription(t, db, "single-sub", "Single")
	now := time.Now().UTC()
	retentionInsertTestFetch(t, db, "single-fetch", "single-sub", now, domain.FetchOutcomeSuccess)
	retentionInsertTestPayload(t, db, "first", "single-sub", "single-fetch", now, true)
	if _, err := db.ExecContext(ctx, `INSERT INTO subscription_payloads(id,subscription_id,fetch_id,content_digest,body_blob,created_at,pinned)VALUES('second','single-sub','single-fetch','digest',X'',?,1)`, now.Format(time.RFC3339)); err == nil {
		t.Fatal("pin must not bypass one payload constraint")
	}
	if _, err := db.ExecContext(ctx, `UPDATE subscription_payloads SET body_blob=x'78' WHERE id='first'`); err == nil {
		t.Fatal("complete response body must not hide in payload metadata")
	}
}

func TestVerifySubscriptionCompactionDetectsProtectedTamperingAndMissingHead(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	now := "2026-11-10T00:00:00Z"
	if _, err := db.ExecContext(ctx, `INSERT INTO nodes(logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at,connection_revision)VALUES('verify-node','ss','Node','192.0.2.1',8388,'{}',1,?,?,1);
	INSERT INTO node_connection_versions(node_logical_id,connection_revision,effective_config_json,config_fingerprint,created_at)VALUES('verify-node',1,'{}','fp',?);
	INSERT INTO node_connection_heads(logical_id,connection_revision,updated_at)VALUES('verify-node',1,?)`, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	plan, err := sqlite.InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	okReport, err := sqlite.VerifySubscriptionCompaction(ctx, db, plan)
	if err != nil || !okReport.Verified || !okReport.ConsumerClosureSealed {
		t.Fatalf("expected verified candidate with sealed consumer closure: %+v err=%v", okReport, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE nodes SET display_name='Changed' WHERE logical_id='verify-node'`); err != nil {
		t.Fatal(err)
	}
	tampered, err := sqlite.VerifySubscriptionCompaction(ctx, db, plan)
	if err != nil || tampered.Verified {
		t.Fatalf("protected asset content mutation passed verification: %+v err=%v", tampered, err)
	}
}

func TestVerifyCompactionRejectsInconsistentCurrentState(t *testing.T) {
	cases := []struct {
		name, mutation, failure string
	}{
		{"revision", `UPDATE subscription_current_state SET source_revision='stale' WHERE subscription_id='a'`, "invalid_applied_state"},
		{"cross subscription payload", `UPDATE subscription_current_state SET applied_payload_id='pb',applied_fetch_id='fb' WHERE subscription_id='a'`, "invalid_applied_state"},
		{"cross subscription attempt", `UPDATE subscription_current_state SET last_attempt_fetch_id='fb' WHERE subscription_id='a'`, "invalid_last_attempt"},
		{"attempt outcome", `UPDATE subscription_current_state SET last_attempt_status='failed' WHERE subscription_id='a'`, "invalid_last_attempt"},
		{"applied outcome", `UPDATE subscription_fetches SET outcome='failed' WHERE id='fa'`, "invalid_applied_state"},
		{"source generation", `UPDATE node_sources SET last_seen_fetch_id='fb' WHERE subscription_id='a'`, "invalid_node_source_generation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, _ := setupTestDB(t)
			for _, id := range []string{"a", "b"} {
				retentionCreateTestSubscription(t, db, id, id)
				retentionInsertTestFetch(t, db, "f"+id, id, time.Now().UTC(), domain.FetchOutcomeSuccess)
				retentionInsertTestPayload(t, db, "p"+id, id, "f"+id, time.Now().UTC(), false)
				if _, err := db.ExecContext(ctx, `INSERT INTO subscription_current_state(subscription_id,source_revision,applied_payload_id,applied_fetch_id,last_attempt_fetch_id,last_attempt_status) VALUES(?, 'rev-1', ?, ?, ?, 'success')`, id, "p"+id, "f"+id, "f"+id); err != nil {
					t.Fatal(err)
				}
			}
			plan, err := sqlite.InspectSubscriptionCompaction(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			valid, err := sqlite.VerifySubscriptionCompaction(ctx, db, plan)
			if err != nil || !valid.Verified || !valid.ConsumerClosureSealed {
				t.Fatalf("valid candidate: %+v %v", valid, err)
			}
			if tc.name == "source generation" {
				// Create a protected source with no current-generation entry, then take a new baseline.
				_, err = db.ExecContext(ctx, `INSERT INTO nodes(logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at,connection_revision) VALUES('n','ss','N','192.0.2.1',8388,'{}',1,'now','now',1);
				INSERT INTO node_connection_versions(node_logical_id,connection_revision,effective_config_json,config_fingerprint,created_at) VALUES('n',1,'{}','fp','now');
				INSERT INTO node_connection_heads(logical_id,connection_revision,updated_at) VALUES('n',1,'now');
				INSERT INTO node_sources(node_logical_id,subscription_id,last_seen_fetch_id) VALUES('n','a','fa');
				INSERT INTO subscription_entries(id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at) VALUES('e','pa','a',0,'k','N','ss','proxy','n','now')`)
				if err != nil {
					t.Fatal(err)
				}
				plan, err = sqlite.InspectSubscriptionCompaction(ctx, db)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(ctx, tc.mutation); err != nil {
				t.Fatal(err)
			}
			report, err := sqlite.VerifySubscriptionCompaction(ctx, db, plan)
			if err != nil || report.Verified {
				t.Fatalf("inconsistency passed: %+v %v", report, err)
			}
			found := false
			for _, failure := range report.Failures {
				if failure == tc.failure {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s: %+v", tc.failure, report.Failures)
			}
		})
	}
}

// Keep a compile-time check on the explicit transaction boundary used by leases.
var _ func(context.Context, *sql.Tx) error = sqlite.PrepareConnectionVersionLeasesTx
