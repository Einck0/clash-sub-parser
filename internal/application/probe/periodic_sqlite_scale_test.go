package probe

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

// scaleSnapshotRecorder records the inventory actually consumed by TriggerWindow.
type scaleSnapshotRecorder struct {
	domain.NodeRepository
	db        *sql.DB
	t         *testing.T
	snapshots [][]string
}

func (r *scaleSnapshotRecorder) ListAll(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, error) {
	before, started := r.db.Stats(), time.Now()
	nodes, err := r.NodeRepository.ListAll(ctx, filter)
	after := r.db.Stats()
	r.t.Logf("stage=coordinator_list_all elapsed=%s rows=%d pool_waits_shared=%d pool_wait_duration_shared=%s error=%v", time.Since(started), len(nodes), after.WaitCount-before.WaitCount, after.WaitDuration-before.WaitDuration, err)
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.LogicalID
	}
	r.snapshots = append(r.snapshots, ids)
	return nodes, err
}

// TestPeriodicCoordinator_SQLiteScale uses only synthetic data and a fake probe
// runner. Deadlines bound resource use; elapsed times are diagnostics, not SLAs.
// No coordinator background loop or network access is needed for TriggerWindow.
func TestPeriodicCoordinator_SQLiteScale(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	started := time.Now()
	cfg := sqlite.DefaultConfig(filepath.Join(t.TempDir(), "periodic-scale.db"))
	// Offline preparation schema preserves the historical million-row fixture;
	// migration 30 adds facts evidence tables without sealing modern constraints.
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewMigrationRunner(db, migrations.FS).RunUpTo(ctx, 29); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := sqlite.NewMigrationRunner(db, migrations.FS).RunUpTo(ctx, 30); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})
	var mode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" || db.Stats().MaxOpenConnections != 1 {
		t.Fatalf("expected WAL/default single connection: mode=%q stats=%+v error=%v", mode, db.Stats(), err)
	}
	t.Logf("stage=open_and_migrate elapsed=%s", time.Since(started))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedPeriodicScale(t, ctx, db, now)

	nodes := sqlite.NewNodeRepository(db)
	subs := sqlite.NewSubscriptionRepository(db)
	schedules := sqlite.NewProbeScheduleRepository(db)
	runs := sqlite.NewProbeRunRepository(db)
	sched := domain.DefaultProbeSchedule()
	sched.Enabled, sched.NextDueAt, sched.Generation = true, &now, 1
	if err := schedules.Update(ctx, &sched); err != nil {
		t.Fatal(err)
	}
	recorder := &scaleSnapshotRecorder{NodeRepository: nodes, db: db, t: t}
	var dispatched []string
	runner := &mockRunner{runFn: func(ctx context.Context, run *domain.ProbeRun, ids []string, kinds []domain.ProbeKind) error {
		if len(kinds) != 1 || kinds[0] != domain.ProbeKindBaseline {
			return fmt.Errorf("unexpected probe kinds: %v", kinds)
		}
		dispatched = append(dispatched, ids...)
		// Persist terminal state so the fake runner fulfills the real port's
		// state contract, but never dials any synthetic node.
		return runs.UpdateState(ctx, run.ID, domain.ProbeRunStateSucceeded)
	}}
	coord := NewPeriodicCoordinator(schedules, recorder, runs, runner,
		WithCoordinatorClock(func() time.Time { return now }),
		WithCoordinatorOwner("synthetic-scale"),
		WithCoordinatorMaxTasks(100), WithCoordinatorSweepQuota(1000),
		WithCoordinatorObservations(sqlite.NewProbeObservationRepository(db)),
		WithCoordinatorSubscriptions(subs),
		WithCoordinatorNodeSources(sqlite.NewNodeSourceRepository(db)),
		WithCoordinatorSourceHistory(sqlite.NewNodeSourceHistoryRepository(db)),
	)
	// TriggerWindow has no context parameter. Binding its package-private ctx
	// avoids Start's separate background sweep and bounds the actual call.
	windowCtx, windowCancel := context.WithTimeout(ctx, 90*time.Second)
	defer windowCancel()
	coord.ctx = windowCtx

	// Hold the sole pool connection, start all four real consumers at a common
	// barrier, and release only after all have queued. This deterministically
	// exercises pool contention without manufacturing a slow SQL operation.
	gate, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	before := db.Stats()
	start := make(chan struct{})
	type stageResult struct {
		name string
		err  error
	}
	results := make(chan stageResult, 4)
	var wg sync.WaitGroup
	launch := func(name string, work func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			readCtx, stop := context.WithTimeout(windowCtx, 90*time.Second)
			defer stop()
			stats, began := db.Stats(), time.Now()
			err := work(readCtx)
			after := db.Stats()
			t.Logf("stage=%s elapsed_including_barrier_contention=%s pool_waits_shared=%d pool_wait_duration_shared=%s error=%v", name, time.Since(began), after.WaitCount-stats.WaitCount, after.WaitDuration-stats.WaitDuration, err)
			results <- stageResult{name, err}
		}()
	}
	launch("trigger_window", func(context.Context) error { return coord.TriggerWindow() })
	launch("operational_readiness", func(ctx context.Context) error {
		report, err := sqlite.CheckOperationalReadiness(ctx, db)
		if err == nil && (report == nil || !report.Ready) {
			err = fmt.Errorf("not ready: %+v", report)
		}
		return err
	})
	launch("subscription_list", func(ctx context.Context) error {
		items, total, err := subs.List(ctx, domain.SubscriptionFilter{})
		if err == nil && (total != 2 || len(items) != 2) {
			err = fmt.Errorf("subscription total=%d rows=%d", total, len(items))
		}
		return err
	})
	launch("publication_facts", func(ctx context.Context) error {
		facts, err := sqlite.NewConsistentFactsLoader(db).LoadConsistentFacts(ctx, "periodic-scale-rev", domain.TargetSingBox, now)
		if err != nil {
			return err
		}
		if facts == nil || len(facts.Nodes) != 998 || len(facts.NodeSources) != 998 || len(facts.PrimaryNodeIDs) != 998 || facts.PrimarySubscriptionID != "scale-a" {
			return fmt.Errorf("unexpected facts inventory/primary")
		}
		seen := make(map[string]bool)
		for _, node := range facts.Nodes {
			if node.LogicalID < "periodic-0002" || node.LogicalID > "periodic-0999" || seen[node.LogicalID] {
				return fmt.Errorf("unexpected or duplicate facts ID %s", node.LogicalID)
			}
			seen[node.LogicalID] = true
			wantSources := 1
			if node.LogicalID == "periodic-0003" {
				wantSources = 2
			}
			sources := facts.NodeSources[node.LogicalID]
			if len(sources) != wantSources {
				return fmt.Errorf("unexpected sources for %s", node.LogicalID)
			}
			seenSources := make(map[string]bool)
			for _, source := range sources {
				if (source.SubscriptionID != "scale-a" && source.SubscriptionID != "scale-b") || seenSources[source.SubscriptionID] || source.LastSeenFetchID != source.SubscriptionID+"-current" {
					return fmt.Errorf("unexpected source for %s: %+v", node.LogicalID, source)
				}
				seenSources[source.SubscriptionID] = true
			}
			if !seenSources["scale-a"] {
				return fmt.Errorf("missing primary source for %s", node.LogicalID)
			}
		}
		return nil
	})
	close(start)
	queueErr := waitScalePoolWaits(windowCtx, db, before.WaitCount+4)
	_ = gate.Close() // Always release before waiting for workers, including error paths.
	wg.Wait()
	close(results)
	if queueErr != nil {
		t.Errorf("synchronized pool entry: %v", queueErr)
	}
	for result := range results {
		if result.err != nil {
			t.Errorf("%s: %v", result.name, result.err)
		}
	}
	after := db.Stats()
	t.Logf("stage=concurrent_consumers pool_waits=%d pool_wait_duration=%s", after.WaitCount-before.WaitCount, after.WaitDuration-before.WaitDuration)
	if t.Failed() {
		t.FailNow()
	}

	// One full inventory snapshot is sorted by logical ID before dispatch.
	if len(recorder.snapshots) != 1 {
		t.Fatalf("coordinator snapshots=%d want=1", len(recorder.snapshots))
	}
	inventory := recorder.snapshots[0]
	if len(inventory) != 998 {
		t.Fatalf("inventory=%d want=998", len(inventory))
	}
	if len(dispatched) != 998 {
		t.Fatalf("dispatched=%d want=998", len(dispatched))
	}
	for i := range inventory {
		if want := fmt.Sprintf("periodic-%04d", 999-i); inventory[i] != want {
			t.Fatalf("inventory[%d]=%s want=%s", i, inventory[i], want)
		}
		if want := fmt.Sprintf("periodic-%04d", i+2); dispatched[i] != want {
			t.Fatalf("dispatch[%d]=%s want=%s", i, dispatched[i], want)
		}
	}
	batch, err := schedules.GetBatchByWindow(ctx, 1, now)
	if err != nil || batch == nil {
		t.Fatalf("batch: %v", err)
	}
	if batch.State != domain.ProbeBatchStateSucceeded || batch.Counts.TotalNodes != 998 || batch.Counts.CompletedRuns != 10 || batch.Counts.DispatchedRuns != 10 || batch.Counts.SkippedNodes != 0 || len(batch.RunIDs) != 10 {
		t.Fatalf("unexpected batch: %+v", batch)
	}
	for _, id := range batch.RunIDs {
		run, err := runs.GetByID(ctx, id)
		if err != nil || run.State != domain.ProbeRunStateSucceeded {
			t.Fatalf("persisted run %s: %+v error=%v", id, run, err)
		}
	}
	updated, err := schedules.Get(ctx)
	if err != nil || updated.NextDueAt == nil || !updated.NextDueAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("schedule did not advance one sweep: %+v error=%v", updated, err)
	}
	if err := coord.TriggerWindow(); err != nil || len(dispatched) != 998 || len(recorder.snapshots) != 1 {
		t.Fatalf("not-due window dispatched again: error=%v nodes=%d snapshots=%d", err, len(dispatched), len(recorder.snapshots))
	}

	// Cancellation while queued must release the waiter without poisoning the
	// single-connection pool. This does not claim interruption of running SQL.
	gate, err = db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	cancelCtx, stopRead := context.WithCancel(ctx)
	defer stopRead()
	cancelResult := make(chan error, 1)
	before = db.Stats()
	started = time.Now()
	go func() {
		_, _, err := nodes.List(cancelCtx, domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions})
		cancelResult <- err
	}()
	queueErr = waitScalePoolWaits(windowCtx, db, before.WaitCount+1)
	stopRead()
	cancelErr := <-cancelResult
	_ = gate.Close()
	t.Logf("stage=cancel_queued_list elapsed=%s error=%v", time.Since(started), cancelErr)
	if queueErr != nil || !errors.Is(cancelErr, context.Canceled) {
		t.Fatalf("queued cancellation: queue=%v read=%v", queueErr, cancelErr)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	// Reopen the same offline-prepared WAL file and run coordinator recovery on a synthetic
	// abandoned batch/run; no production DB or crash simulation is involved.
	old := now.Add(-time.Hour)
	abandonedRun := &domain.ProbeRun{ID: "scale-abandoned-run", IdempotencyKey: "scale-abandoned", ActorScope: "synthetic", State: domain.ProbeRunStateRunning, DeadlineAt: old, CreatedAt: old, UpdatedAt: old}
	if err := runs.Create(ctx, abandonedRun); err != nil {
		t.Fatal(err)
	}
	abandoned := &domain.ProbeBatch{ID: "scale-abandoned-batch", WindowAt: old, Generation: 1, Owner: "stopped-synthetic-worker", LeaseUntil: &old, State: domain.ProbeBatchStateRunning, RunIDs: []string{abandonedRun.ID}, CreatedAt: old, UpdatedAt: old}
	if err := schedules.CreateBatch(ctx, abandoned); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	db, err = sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	schedules = sqlite.NewProbeScheduleRepository(db)
	runs = sqlite.NewProbeRunRepository(db)
	recovered := NewPeriodicCoordinator(schedules, sqlite.NewNodeRepository(db), runs, &mockRunner{}, WithCoordinatorClock(func() time.Time { return now }))
	if err := recovered.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("stage=reopen_offline_prepared_recover elapsed=%s", time.Since(started))
	abandoned, err = schedules.GetBatchByID(ctx, abandoned.ID)
	if err != nil || abandoned.State != domain.ProbeBatchStateExpired {
		t.Fatalf("abandoned batch recovery: %+v error=%v", abandoned, err)
	}
	abandonedRun, err = runs.GetByID(ctx, abandonedRun.ID)
	if err != nil || abandonedRun.State != domain.ProbeRunStateExpired {
		t.Fatalf("abandoned run recovery: %+v error=%v", abandonedRun, err)
	}
	report, err := sqlite.CheckOperationalReadiness(ctx, db)
	if err != nil || report == nil || !report.Ready {
		t.Fatalf("reopened database readiness: %+v error=%v", report, err)
	}
	persisted, err := schedules.GetBatchByID(ctx, batch.ID)
	if err != nil || persisted.State != domain.ProbeBatchStateSucceeded || persisted.Counts.CompletedRuns != 10 {
		t.Fatalf("completed batch lost on reopen/recovery: %+v error=%v", persisted, err)
	}
	items, total, err := sqlite.NewNodeRepository(db).List(ctx, domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, ExcludeNotices: true, Pagination: domain.Pagination{Page: 10, PageSize: 100}})
	if err != nil || total != 998 || len(items) != 98 || items[97].LogicalID != "periodic-0002" {
		t.Fatalf("reopened inventory total=%d rows=%d error=%v", total, len(items), err)
	}
}

func waitScalePoolWaits(ctx context.Context, db *sql.DB, minimum int64) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for db.Stats().WaitCount < minimum {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return nil
}

func seedPeriodicScale(t *testing.T, ctx context.Context, db *sql.DB, now time.Time) {
	t.Helper()
	started := time.Now()
	stamp := now.Format(time.RFC3339)
	nodes := make([]domain.Node, 1000)
	for i := range nodes {
		id := fmt.Sprintf("periodic-%04d", i)
		nodes[i] = domain.Node{LogicalID: id, DisplayName: id, Protocol: domain.ProtocolSS, Server: "192.0.2.1", Port: 10000 + i, Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "synthetic"}, Active: true, ConnectionRevision: 1, CreatedAt: now.Add(time.Duration(i) * time.Second), UpdatedAt: now}
	}
	if err := sqlite.NewNodeRepository(db).UpsertBatch(ctx, nodes); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO configuration_revisions (id,content_digest,state,created_at) VALUES ('periodic-scale-rev','synthetic','active',?)`, stamp)
	for _, sub := range []string{"scale-a", "scale-b"} {
		primary := 0
		if sub == "scale-a" {
			primary = 1
		}
		exec(`INSERT INTO subscriptions (id,name,source_url_secret_ref,enabled,is_primary,revision,created_at,updated_at) VALUES (?,?,'synthetic',1,?,'1',?,?)`, sub, sub, primary, stamp, stamp)
		for payload := 0; payload < 4; payload++ {
			id := fmt.Sprintf("%s-history-%d", sub, payload)
			exec(`INSERT INTO subscription_fetches (id,subscription_id,started_at,outcome,nodes_valid) VALUES (?,?,?,'success',1000)`, id, sub, now.Add(-time.Hour).Format(time.RFC3339))
			exec(`INSERT INTO subscription_payloads (id,subscription_id,fetch_id,content_digest,body_blob,created_at) VALUES (?,?,?,?,X'00',?)`, id, sub, id, id, now.Add(-time.Hour).Format(time.RFC3339))
			exec(`WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<124999)
				INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at)
				SELECT ? || '-' || n,?,?,n,printf('old-%d',n),'old','ss',CASE WHEN n%1000=0 THEN 'notice' ELSE 'proxy' END,printf('periodic-%04d',n%1000),? FROM seq`, id, id, sub, stamp)
		}
		id := sub + "-current"
		exec(`INSERT INTO subscription_fetches (id,subscription_id,started_at,outcome,nodes_valid) VALUES (?,?,?,'success',1000)`, id, sub, stamp)
		exec(`INSERT INTO subscription_payloads (id,subscription_id,fetch_id,content_digest,body_blob,pinned,created_at) VALUES (?,?,?,?,X'00',1,?)`, id, sub, id, id, stamp)
	}
	for i, node := range nodes {
		kind := "proxy"
		if i == 0 || i == 2 {
			kind = "notice"
		}
		exec(`INSERT INTO node_sources (node_logical_id,subscription_id,last_seen_fetch_id) VALUES (?,'scale-a','scale-a-current')`, node.LogicalID)
		exec(`INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at) VALUES (?,'scale-a-current','scale-a',?,?,?,'ss',?,?,?)`, fmt.Sprintf("current-%d", i), i, fmt.Sprintf("key-%d", i), node.DisplayName, kind, node.LogicalID, stamp)
	}
	exec(`INSERT INTO node_sources (node_logical_id,subscription_id,last_seen_fetch_id) VALUES ('periodic-0003','scale-b','scale-b-current')`)
	exec(`INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at) VALUES ('shared','scale-b-current','scale-b',0,'shared','shared','ss','notice','periodic-0003',?)`, stamp)
	for _, override := range []struct{ anchor, kind string }{{"key-1", "notice"}, {"key-2", "proxy"}, {"key-3", "proxy"}} {
		exec(`INSERT INTO subscription_entry_overrides (subscription_id,override_anchor,user_kind_override,override_at) VALUES ('scale-a',?,?,?)`, override.anchor, override.kind, stamp)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Logf("stage=fixture_setup elapsed=%s nodes=1000 historical_entries=1000000 subscriptions=2 historical_payloads=8", time.Since(started))
}
