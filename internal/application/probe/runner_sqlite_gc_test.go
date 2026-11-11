package probe_test

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

// A probe owns its captured node value, not a lease on its historical DB row.
func TestProbeRunnerCapturedNodeSurvivesConcurrentVersionGC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := sqlite.Config{Path: filepath.Join(t.TempDir(), "probe.db"), ForeignKeys: true, WALMode: true, BusyTimeout: 5 * time.Second}
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatal(err)
	}
	// Fresh-database initialization seals readiness as part of the migrations.
	var contractVersion, consumersClosed int
	if err := db.QueryRowContext(ctx, `SELECT contract_version, consumers_closed FROM subscription_storage_readiness WHERE singleton=1`).Scan(&contractVersion, &consumersClosed); err != nil {
		t.Fatal(err)
	}
	if contractVersion != 1 || consumersClosed != 1 {
		t.Fatalf("fresh database readiness: contract_version=%d consumers_closed=%d", contractVersion, consumersClosed)
	}
	writerDB, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer writerDB.Close()
	nodes := sqlite.NewNodeRepository(db)
	writer := sqlite.NewNodeRepository(writerDB)
	node := domain.Node{LogicalID: "probe-gc-node", DisplayName: "Synthetic", Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 8388, Active: true, Credentials: domain.InboundProtocolCredential{Password: "synthetic-old"}}
	if err := writer.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	original, err := nodes.GetByLogicalID(ctx, node.LogicalID)
	if err != nil || original.ConnectionRevision != 1 {
		t.Fatalf("initial revision: node=%+v err=%v", original, err)
	}

	runs := sqlite.NewProbeRunRepository(db)
	observations := sqlite.NewProbeObservationRepository(db)
	run := &domain.ProbeRun{ID: "probe-gc-run", IdempotencyKey: "probe-gc-key", ActorScope: "test", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Minute)}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	scheduler, err := queue.NewScheduler(queue.Config{Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	captured := make(chan domain.Node, 1)
	runner := probe.NewDefaultRunner(nodes, observations, scheduler, runs,
		probe.WithNodeScope(domain.NodeScopeAllAssets),
		probe.WithNodeDialer(func(dialCtx context.Context, snapshot domain.Node) (*http.Client, func() error, error) {
			close(entered)
			select {
			case <-release:
			case <-dialCtx.Done():
				return nil, nil, dialCtx.Err()
			}
			captured <- snapshot
			return mockHTTPClient(http.StatusNoContent, "", nil), func() error { return nil }, nil
		}),
	)
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, run, []string{node.LogicalID}, []domain.ProbeKind{domain.ProbeKindBaseline})
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("runner ended before reaching dialer: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Release even on a failing assertion, before scheduler and database cleanup.
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	node.Credentials.Password = "synthetic-new"
	if err := writer.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	current, err := nodes.GetByLogicalID(ctx, node.LogicalID)
	if err != nil || current.ConnectionRevision != 2 || current.Credentials.Password != "synthetic-new" {
		t.Fatalf("updated head: node=%+v err=%v", current, err)
	}
	var oldRows int
	if err := writerDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions WHERE node_logical_id=? AND connection_revision=1`, node.LogicalID).Scan(&oldRows); err != nil || oldRows != 0 {
		t.Fatalf("old full config was not collected: count=%d err=%v", oldRows, err)
	}
	unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runner: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	snapshot := <-captured
	if snapshot.ConnectionRevision != 1 || snapshot.Credentials.Password != "synthetic-old" || snapshot.Server != "192.0.2.1" || snapshot.Port != 8388 {
		t.Fatalf("dialer did not retain original snapshot: %+v", snapshot)
	}
	items, err := observations.ListByRun(ctx, run.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("observations: count=%d err=%v", len(items), err)
	}
	obs := items[0]
	if obs.Kind != domain.ProbeKindBaseline || obs.Verdict != domain.VerdictAvailable || obs.ConnectionRevision == nil || *obs.ConnectionRevision != 1 {
		t.Fatalf("observation did not describe original revision: %+v", obs)
	}
	latest, err := observations.ListLatestByNodes(ctx, []string{node.LogicalID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if health := domain.ClassifyNodeHealth(latest[node.LogicalID], current.ConnectionRevision, time.Now().UTC(), 0); health != domain.HealthCategoryUnknown {
		t.Fatalf("stale observation contaminated current health: %s", health)
	}
}
