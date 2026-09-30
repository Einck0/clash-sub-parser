package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestConnectionRevisionPersistentAndHistoricalEvidence(t *testing.T) {
	ctx := context.Background()
	cfg := sqlite.Config{Path: filepath.Join(t.TempDir(), "revisions.sqlite"), BusyTimeout: 5 * time.Second, ForeignKeys: true, WALMode: true}
	db, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewNodeRepository(db)
	node := domain.Node{LogicalID: "rev-node", Protocol: domain.ProtocolVLESS, DisplayName: "first", Server: "1.1.1.1", Port: 443, Active: true, Credentials: domain.InboundProtocolCredential{UUID: "uuid", Transport: map[string]string{"flow": "xtls-rprx-vision", "tls": "true"}}}
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	get := func(want int64) domain.Node {
		t.Helper()
		got, err := repo.GetByLogicalID(ctx, node.LogicalID)
		if err != nil {
			t.Fatal(err)
		}
		if got.ConnectionRevision != want {
			t.Fatalf("connection revision %d, want %d", got.ConnectionRevision, want)
		}
		list, total, err := repo.List(ctx, domain.NodeFilter{})
		if err != nil || total != 1 || list[0].ConnectionRevision != want {
			t.Fatalf("list revision mismatch: %+v %d %v", list, total, err)
		}
		models, total, err := repo.ListReadModel(ctx, domain.NodeFilter{})
		if err != nil || total != 1 || models[0].Node.ConnectionRevision != want {
			t.Fatalf("read-model revision mismatch: %+v %d %v", models, total, err)
		}
		return *got
	}
	get(1)
	node.DisplayName = "renamed"
	node.Credentials.Transport = map[string]string{"tls": "true", "flow": "xtls-rprx-vision"}
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	get(1)
	node.Server = "1.0.0.1"
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	get(2)
	node.Server = "1.1.1.1" // A -> B -> A cannot revive A evidence
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	get(3)
	node.Port = 8443
	if err := sqlite.UpdateNode(ctx, db, &node); err != nil {
		t.Fatal(err)
	}
	if node.ConnectionRevision != 4 {
		t.Fatalf("updated node revision %d", node.ConnectionRevision)
	}
	node.DisplayName = "changed by API"
	if err := sqlite.UpdateNode(ctx, db, &node); err != nil {
		t.Fatal(err)
	}
	get(4)
	if err := repo.DeactivateNodesNotIn(ctx, nil); err != nil {
		t.Fatal(err)
	}
	get(4)
	node.Active = true
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	get(5)
	if err := repo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	get(5)
	// A second handle observes persistent revision, not an in-process signature.
	other, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second := sqlite.NewNodeRepository(other)
	secondNode, err := second.GetByLogicalID(ctx, node.LogicalID)
	if err != nil || secondNode.ConnectionRevision != 5 {
		t.Fatalf("second handle: %+v %v", secondNode, err)
	}
	_ = other.Close()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedNode, err := sqlite.NewNodeRepository(reopened).GetByLogicalID(ctx, node.LogicalID)
	if err != nil || reopenedNode.ConnectionRevision != 5 {
		t.Fatalf("restart: %+v %v", reopenedNode, err)
	}
}

func TestConnectionRevisionMigrationDoesNotAttributeLegacyObservations(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)
	all, err := sqlite.NewMigrationRunner(db, migrations.FS).LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Version >= 12 {
			break
		}
		if _, err := db.ExecContext(ctx, m.SQL); err != nil {
			t.Fatalf("migration %d: %v", m.Version, err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO schema_migrations(version,name,applied_at) VALUES(?,?,?)", m.Version, m.Name, time.Now().UTC().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO nodes(logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at) VALUES('old','ss','old','1.1.1.1',443,'{"password":"secret"}',1,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO probe_runs(id,idempotency_key,state,deadline_at,created_at,updated_at) VALUES('run','run','succeeded','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO probe_observations(id,probe_run_id,node_logical_id,kind,verdict,observed_at) VALUES('old-obs','run','old','baseline','available','2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatal(err)
	}
	node, err := sqlite.NewNodeRepository(db).GetByLogicalID(ctx, "old")
	if err != nil || node.ConnectionRevision != 1 {
		t.Fatalf("migrated node %+v %v", node, err)
	}
	repo := sqlite.NewProbeObservationRepository(db)
	obs, err := repo.GetByID(ctx, "old-obs")
	if err != nil || obs.ConnectionRevision != nil {
		t.Fatalf("legacy observation %+v %v", obs, err)
	}
	run := &domain.ProbeRun{ID: "new-run", IdempotencyKey: "new-run", ActorScope: "test", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Hour)}
	if err := sqlite.NewProbeRunRepository(db).Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	revision := node.ConnectionRevision
	fresh := &domain.ProbeObservation{ID: "new-obs", ProbeRunID: run.ID, NodeLogicalID: node.LogicalID, Kind: domain.ProbeKindBaseline, Verdict: domain.VerdictAvailable, ObservedAt: time.Now().UTC(), ConnectionRevision: &revision}
	if err := repo.Create(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	for _, read := range []func() (*domain.ProbeObservation, error){
		func() (*domain.ProbeObservation, error) { return repo.GetByID(ctx, fresh.ID) },
		func() (*domain.ProbeObservation, error) {
			rows, e := repo.ListByRun(ctx, run.ID)
			if e != nil {
				return nil, e
			}
			return &rows[0], nil
		},
		func() (*domain.ProbeObservation, error) {
			rows, e := repo.ListByNode(ctx, node.LogicalID, 1)
			if e != nil {
				return nil, e
			}
			return &rows[0], nil
		},
		func() (*domain.ProbeObservation, error) {
			rows, _, e := repo.(domain.ProbeObservationNodePager).ListByNodePaginated(ctx, node.LogicalID, 1, 1)
			if e != nil {
				return nil, e
			}
			return &rows[0], nil
		},
		func() (*domain.ProbeObservation, error) {
			rows, e := repo.ListLatestByNodes(ctx, []string{node.LogicalID}, []domain.ProbeKind{domain.ProbeKindBaseline})
			if e != nil {
				return nil, e
			}
			v := rows[node.LogicalID][domain.ProbeKindBaseline]
			return &v, nil
		},
	} {
		got, err := read()
		if err != nil || got.ConnectionRevision == nil || *got.ConnectionRevision != revision {
			t.Fatalf("observation read %+v %v", got, err)
		}
	}
	var legacy sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT connection_revision FROM probe_observations WHERE id = 'old-obs'").Scan(&legacy); err != nil || legacy.Valid {
		t.Fatalf("legacy revision %+v %v", legacy, err)
	}
}
