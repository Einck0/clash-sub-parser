package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestConnectionVersionRetentionHundredChangesAndOwnedRelease(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewNodeRepository(db)
	versions := sqlite.NewNodeConnectionVersionRepository(db)
	node := domain.Node{LogicalID: "bounded-node", Protocol: domain.ProtocolSS, DisplayName: "Synthetic", Server: "192.0.2.1", Port: 8388, Active: true, Credentials: domain.InboundProtocolCredential{Password: "synthetic-A"}}
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	// An owner unknown to runtime remains authoritative even past diagnostic
	// expiry. No allow-list or scheduler timeout can permit its deletion.
	now := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	lease := domain.ConnectionVersionLease{ConsumerKind: "unknown-consumer", ConsumerID: "unknown-id", NodeID: node.LogicalID, Revision: 1, Owner: "unknown-owner", FencingToken: "unknown-token", HeartbeatAt: now, ExpiresAt: now.Add(time.Minute)}
	var held *domain.LeasedConnectionVersion
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		held, err = sqlite.AcquireConnectionVersionLeaseTx(ctx, tx, lease)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assertBound := func(wantRevision int64, wantVersions int) {
		t.Helper()
		current, err := repo.GetByLogicalID(ctx, node.LogicalID)
		if err != nil || current.ConnectionRevision != wantRevision || current.Credentials.Password != node.Credentials.Password {
			t.Fatalf("current revision/config: %+v err=%v want revision=%d", current, err, wantRevision)
		}
		var nodeCount, versionCount, unexpected int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&nodeCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions`).Scan(&versionCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions v
			WHERE NOT EXISTS(SELECT 1 FROM node_connection_heads h WHERE h.logical_id=v.node_logical_id AND h.connection_revision=v.connection_revision)
			AND NOT EXISTS(SELECT 1 FROM node_connection_version_leases l WHERE l.node_logical_id=v.node_logical_id AND l.connection_revision=v.connection_revision)`).Scan(&unexpected); err != nil {
			t.Fatal(err)
		}
		if nodeCount != 1 || versionCount != wantVersions || unexpected != 0 {
			t.Fatalf("not heads union leases: nodes=%d versions=%d unexpected=%d", nodeCount, versionCount, unexpected)
		}
	}
	for i := 1; i <= 100; i++ {
		node.Credentials.Password = "synthetic-B"
		if i%2 == 0 {
			node.Credentials.Password = "synthetic-A"
		}
		// Both native repository writers must enforce the same transactional
		// boundary. Returning to A cannot reuse revision 1 or its probe evidence.
		if i%2 == 0 {
			if err := sqlite.UpdateNode(ctx, db, &node); err != nil {
				t.Fatal(err)
			}
		} else if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
			t.Fatal(err)
		}
		assertBound(int64(i+1), 2)
	}
	if held == nil || !strings.Contains(held.EffectiveConfigJSON, "synthetic-A") || held.Revision != 1 {
		t.Fatal("held immutable snapshot lost")
	}
	if _, err := versions.GetByRevision(ctx, node.LogicalID, 2); err == nil || !strings.Contains(err.Error(), "version_not_found") {
		t.Fatalf("retired revision must return explicit unavailable, not head: %v", err)
	}
	wrong := lease
	wrong.FencingToken = "wrong-token"
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.ReleaseConnectionVersionLeaseTx(ctx, tx, wrong)
	}); err == nil {
		t.Fatal("wrong token released unknown owner")
	}
	assertBound(101, 2)
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.ReleaseConnectionVersionLeaseTx(ctx, tx, lease)
	}); err != nil {
		t.Fatal(err)
	}
	assertBound(101, 1)
	oldRevision := int64(1)
	oldProbe := &domain.ProbeObservation{Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ConnectionRevision: &oldRevision, ObservedAt: time.Now().UTC()}
	if status, _ := domain.EvaluateBaselineHealth(oldProbe, 101, time.Now().UTC(), time.Hour); status != domain.BaselineUnknown {
		t.Fatalf("A-B-A revived old probe freshness: %s", status)
	}
	if _, err := versions.GetByRevision(ctx, node.LogicalID, 1); err == nil {
		t.Fatal("released old credentials still available")
	}
	// Unchanged replay must not create an extra version or revision.
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	assertBound(101, 1)
}

func TestConnectionVersionRetentionRecoverySnapshotAcrossAdvance(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewNodeRepository(db)
	node := domain.Node{LogicalID: "snapshot-node", Protocol: domain.ProtocolSS, DisplayName: "Synthetic", Server: "192.0.2.1", Port: 8388, Active: true}
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	snapshots, leases, err := sqlite.SnapshotRecoveryVersions(ctx, db, node.LogicalID, "recovery-owner", false)
	if err != nil || len(snapshots) != 1 || len(leases) != 1 {
		t.Fatalf("snapshot before writer: snapshots=%d leases=%d err=%v", len(snapshots), len(leases), err)
	}
	node.Server = "192.0.2.2"
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	v1, err := sqlite.NewNodeConnectionVersionRepository(db).GetByRevision(ctx, node.LogicalID, 1)
	if err != nil || v1.EffectiveConfigJSON != snapshots[0].EffectiveConfigJSON {
		t.Fatalf("writer invalidated recovery snapshot: %v", err)
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.ReleaseConnectionVersionLeaseTx(ctx, tx, leases[0])
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlite.NewNodeConnectionVersionRepository(db).GetByRevision(ctx, node.LogicalID, 1); err == nil {
		t.Fatal("completed recovery retained old full configuration")
	}
	if !strings.Contains(snapshots[0].EffectiveConfigJSON, "192.0.2.1") {
		t.Fatal("captured copy mutated after release")
	}
	// The opposite serialization (writer commits first) exposes only the new
	// available head; acquisition must never reconstruct a removed revision.
	latest, _, err := sqlite.SnapshotRecoveryVersions(ctx, db, node.LogicalID, "dry-run", true)
	if err != nil || len(latest) != 1 || latest[0].Revision != 2 {
		t.Fatalf("post-writer snapshot: %+v err=%v", latest, err)
	}
}

func TestConnectionVersionRetentionSetHeadAndRollback(t *testing.T) {
	for _, failure := range []string{"none", "delete", "evidence"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			db, _ := setupTestDB(t)
			repo := sqlite.NewNodeRepository(db)
			node := domain.Node{LogicalID: "head-node", Protocol: domain.ProtocolSS, DisplayName: "Synthetic", Server: "192.0.2.1", Port: 8388, Active: true}
			if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
				t.Fatal(err)
			}
			retentionCreateTestSubscription(t, db, "head-sub", "Synthetic")
			now := time.Now().UTC()
			retentionInsertTestFetch(t, db, "head-fetch", "head-sub", now, domain.FetchOutcomeSuccess)
			retentionInsertTestPayload(t, db, "head-payload", "head-sub", "head-fetch", now, false)
			retentionInsertTestEntry(t, db, "head-entry", "head-payload", "head-sub", "key", "Raw Name", domain.EntryKindProxy, &node.LogicalID)
			if _, err := db.ExecContext(ctx, `UPDATE node_connection_versions SET source_entry_id='head-entry' WHERE node_logical_id=? AND connection_revision=1`, node.LogicalID); err != nil {
				t.Fatal(err)
			}
			versions := sqlite.NewNodeConnectionVersionRepository(db)
			if failure == "none" {
				// The explicit Save -> SetHead API must prune too, not only UpdateNode.
				if err := versions.Save(ctx, &domain.NodeConnectionVersion{NodeLogicalID: node.LogicalID, ConnectionRevision: 2, EffectiveConfigJSON: `{"server":"192.0.2.2","port":8388,"credentials":{}}`, ConfigFingerprint: "synthetic-new"}); err != nil {
					t.Fatal(err)
				}
				if err := versions.SetHead(ctx, node.LogicalID, 2, now); err != nil {
					t.Fatal(err)
				}
				var origin, digest string
				if err := db.QueryRowContext(ctx, `SELECT p.origin_payload_id,e.content_digest FROM node_connection_provenance p JOIN subscription_payload_evidence e ON e.payload_id=p.origin_payload_id WHERE p.node_logical_id=? AND p.connection_revision=1`, node.LogicalID).Scan(&origin, &digest); err != nil || origin != "head-payload" || digest != "sha256:digest" {
					t.Fatalf("retired version lost origin: %s %s %v", origin, digest, err)
				}
				if _, err := versions.GetByRevision(ctx, node.LogicalID, 1); err == nil {
					t.Fatal("SetHead retained unleased old version")
				}
				return
			}
			ddl := `CREATE TRIGGER fail_retention BEFORE DELETE ON node_connection_versions BEGIN SELECT RAISE(ABORT,'synthetic delete failure'); END`
			if failure == "evidence" {
				ddl = `CREATE TRIGGER fail_retention BEFORE INSERT ON node_connection_provenance BEGIN SELECT RAISE(ABORT,'synthetic evidence failure'); END`
			}
			if _, err := db.ExecContext(ctx, ddl); err != nil {
				t.Fatal(err)
			}
			node.Server = "192.0.2.2"
			if err := sqlite.UpdateNode(ctx, db, &node); err == nil {
				t.Fatal("retention failure silently committed node write")
			}
			current, err := repo.GetByLogicalID(ctx, node.LogicalID)
			if err != nil || current.ConnectionRevision != 1 || current.Server != "192.0.2.1" {
				t.Fatalf("retention error did not rollback node/head: %+v %v", current, err)
			}
			for _, query := range []string{`SELECT COUNT(*) FROM node_connection_versions WHERE connection_revision=2`, `SELECT COUNT(*) FROM node_connection_provenance`, `SELECT COUNT(*) FROM subscription_payload_evidence`} {
				var count int
				if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
					t.Fatalf("partial retention writes after rollback: count=%d err=%v query=%s", count, err, query)
				}
			}
		})
	}
}
