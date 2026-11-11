package inventory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestRecoverySnapshotLeaseLifecycle(t *testing.T) {
	for _, scenario := range []string{"success", "duplicate", "failure", "cancel", "missing-primary-link", "missing-secondary-link", "archive-query-error", "concurrent-update", "missing-head-version", "missing-head"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			dir := t.TempDir()
			cfg := sqlite.DefaultConfig(filepath.Join(dir, "target.db"))
			db, err := sqlite.Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
				t.Fatal(err)
			}
			node := domain.Node{LogicalID: "recovery-lease-node", Protocol: domain.ProtocolSS, DisplayName: "test", Server: "192.0.2.1", Port: 8388, Active: true, Credentials: domain.InboundProtocolCredential{Password: "test-password", Method: "aes-128-gcm"}}
			if err := sqlite.NewNodeRepository(db).UpsertBatch(ctx, []domain.Node{node}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE subscription_storage_readiness SET consumers_closed=1 WHERE singleton=1`); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			otherLease := domain.ConnectionVersionLease{ConsumerKind: "other", ConsumerID: "other-consumer", NodeID: node.LogicalID, Revision: 1, Owner: "other-owner", FencingToken: "other-token", HeartbeatAt: now, ExpiresAt: now.Add(time.Hour)}
			if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
				_, err := sqlite.AcquireConnectionVersionLeaseTx(ctx, tx, otherLease)
				return err
			}); err != nil {
				t.Fatal(err)
			}

			if scenario == "missing-head" {
				if _, err := db.ExecContext(ctx, `DELETE FROM node_connection_heads WHERE logical_id=?`, node.LogicalID); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "missing-head-version" {
				// Simulate a damaged head/version relationship in a disposable database.
				if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, `UPDATE node_connection_heads SET connection_revision=99 WHERE logical_id=?`, node.LogicalID); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
					t.Fatal(err)
				}
			}

			archivePath := filepath.Join(dir, "archive.db")
			archive, err := sqlite.Open(sqlite.Config{Path: archivePath})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := archive.Exec(`
				CREATE TABLE nodes (id INTEGER PRIMARY KEY, protocol TEXT, server TEXT, port INTEGER, normalized_payload TEXT);
				CREATE TABLE sources (id INTEGER PRIMARY KEY, name TEXT, url TEXT);
				CREATE TABLE source_revisions (id INTEGER PRIMARY KEY, source_id INTEGER, fetched_at TEXT);
				CREATE TABLE node_source_links (id INTEGER PRIMARY KEY, source_revision_id INTEGER, node_id INTEGER, created_at TEXT);
				INSERT INTO nodes VALUES (1, 'ss', '192.0.2.1', 8388, '{"type":"ss","name":"test","server":"192.0.2.1","port":8388,"cipher":"aes-128-gcm","password":"test-password"}');
				INSERT INTO sources VALUES (1, 'test', 'https://example.invalid/sub');
				INSERT INTO source_revisions VALUES (1, 1, '2026-01-01');
				INSERT INTO node_source_links VALUES (1, 1, 1, '2026-01-01')`); err != nil {
				archive.Close()
				t.Fatal(err)
			}
			if scenario == "missing-primary-link" {
				if _, err := archive.Exec(`DELETE FROM node_source_links`); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "archive-query-error" {
				if _, err := archive.Exec(`DROP TABLE sources`); err != nil {
					t.Fatal(err)
				}
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			entry := ManifestNode{
				NodeLogicalID: node.LogicalID, Protocol: string(node.Protocol), Server: node.Server, Port: node.Port,
				ProvenanceOrigin:        "legacy_cold_archive",
				PrimaryHistoricalSource: &ManifestSourceEdge{LegacyNodePK: 1, LegacyLinkPK: 1, LegacySourceID: 1, RelationState: "verified"},
			}
			if scenario == "failure" {
				entry.Server = "192.0.2.99"
			}
			if scenario == "missing-secondary-link" {
				entry.SecondarySameCanonicalSources = []ManifestSourceEdge{{LegacyNodePK: 1, LegacyLinkPK: 2, LegacySourceID: 1, RelationState: "verified"}}
			}
			manifest := RecoveryManifest{Metadata: ManifestMetadata{ColdArchive: ManifestArchive{Path: archivePath}}, Nodes: []ManifestNode{entry}}
			if scenario == "duplicate" {
				manifest.Nodes = append(manifest.Nodes, entry)
			}
			raw, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "manifest.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(raw)
			observed := 0
			recoveryCtx := context.WithValue(ctx, recoverySnapshotObserverKey{}, func() {
				observed++
				var count int
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_version_leases WHERE consumer_kind='recover_history' AND node_logical_id=? AND connection_revision=1`, node.LogicalID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("capture did not commit exactly one lease: count=%d err=%v", count, err)
				}
				if scenario == "cancel" {
					cancel()
				}
				if scenario == "concurrent-update" {
					updated := node
					updated.Credentials.Password = "new-password"
					if err := sqlite.NewNodeRepository(db).UpsertBatch(ctx, []domain.Node{updated}); err != nil {
						t.Fatal(err)
					}
					if _, err := sqlite.NewNodeConnectionVersionRepository(db).GetByRevision(ctx, node.LogicalID, 1); err != nil {
						t.Fatalf("GC removed a leased captured head: %v", err)
					}
					// Leave the old version protected only by recovery during the rest of the run.
					if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
						return sqlite.ReleaseConnectionVersionLeaseTx(ctx, tx, otherLease)
					}); err != nil {
						t.Fatal(err)
					}
					if _, err := sqlite.NewNodeConnectionVersionRepository(db).GetByRevision(ctx, node.LogicalID, 1); err != nil {
						t.Fatalf("GC removed recovery's still-leased version: %v", err)
					}
				}
			})
			result, err := RecoverSourceHistory(recoveryCtx, db, RecoveryConfig{ManifestPath: path, ExpectedManifestSHA256: hex.EncodeToString(sum[:])})
			switch scenario {
			case "success", "duplicate":
				if err != nil || result == nil || result.Status != "success" || result.InsertedRecords != 1 {
					t.Fatalf("recovery should insert one history record: result=%+v err=%v", result, err)
				}
			case "failure":
				if err == nil || !strings.Contains(err.Error(), "endpoint mismatch") {
					t.Fatalf("expected endpoint mismatch, got %v", err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation, got %v", err)
				}
			case "missing-primary-link":
				if err == nil || !strings.Contains(err.Error(), "archive source link chain not found") {
					t.Fatalf("missing primary link must fail closed, got %v", err)
				}
			case "missing-secondary-link":
				if err == nil || !strings.Contains(err.Error(), "secondary archive link chain not found") {
					t.Fatalf("missing secondary link must fail closed, got %v", err)
				}
			case "archive-query-error":
				if err == nil || !strings.Contains(err.Error(), "query archive source link chain") || !strings.Contains(err.Error(), "no such table") {
					t.Fatalf("archive query failure must retain its cause, got %v", err)
				}
			case "concurrent-update":
				if err == nil || !strings.Contains(err.Error(), "INVARIANT BREACH") {
					t.Fatalf("concurrent asset update must abort apply, got %v", err)
				}
			case "missing-head-version":
				if err == nil || !strings.Contains(err.Error(), "historical_configuration_unavailable: current head revision 99") {
					t.Fatalf("missing head version must fail closed, got %v", err)
				}
			case "missing-head":
				if err == nil || !strings.Contains(err.Error(), "historical_configuration_unavailable: current head is missing") {
					t.Fatalf("missing sealed head must fail closed, got %v", err)
				}
			}
			wantObserved := 1
			if scenario == "missing-head" || scenario == "missing-head-version" {
				wantObserved = 0
			} else if scenario == "duplicate" {
				wantObserved = 2
			}
			if observed != wantObserved {
				t.Fatalf("unexpected snapshot count: %d", observed)
			}
			var own, other int
			if err := db.QueryRow(`SELECT COUNT(*) FROM node_connection_version_leases WHERE consumer_kind='recover_history'`).Scan(&own); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM node_connection_version_leases WHERE consumer_kind='other' AND consumer_id='other-consumer' AND owner='other-owner' AND fencing_token='other-token'`).Scan(&other); err != nil {
				t.Fatal(err)
			}
			wantOther := 1
			if scenario == "concurrent-update" {
				wantOther = 0
				if _, err := sqlite.NewNodeConnectionVersionRepository(db).GetByRevision(context.Background(), node.LogicalID, 1); err == nil {
					t.Fatal("released, non-head captured version was not collected")
				}
			}
			if own != 0 || other != wantOther {
				t.Fatalf("incorrect lease cleanup: recovery=%d other=%d wantOther=%d", own, other, wantOther)
			}
		})
	}
}
