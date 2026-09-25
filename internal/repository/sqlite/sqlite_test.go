package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

// setupTestDB creates an in-memory or temp file SQLite database with migrations applied.
func setupTestDB(t *testing.T) (*sql.DB, sqlite.Config) {
	t.Helper()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:test_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false, // in-memory SQLite uses MEMORY journal mode
	}

	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations on test db: %v", err)
	}

	return db, cfg
}

// setupEmptyTestDB creates a database without applying migrations.
func setupEmptyTestDB(t *testing.T) (*sql.DB, sqlite.Config) {
	t.Helper()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:empty_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}

	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open empty test sqlite db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db, cfg
}

// setupTempFileDB creates a temp file-backed SQLite database with WAL mode enabled.
func setupTempFileDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "csp_test.db")

	cfg := sqlite.Config{
		Path:        dbPath,
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     true,
	}

	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to open and migrate temp file db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db, dbPath
}

func TestSequentialMigrationAndIdempotency(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	runner := sqlite.NewMigrationRunner(db, migrations.FS)

	// Step 1: Verify version before migration is 0
	ver, err := runner.LatestVersion(ctx)
	if err != nil {
		t.Fatalf("unexpected error querying initial version: %v", err)
	}
	if ver != 0 {
		t.Fatalf("expected initial version 0, got %d", ver)
	}

	// Step 2: Run migration
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Step 3: Verify version after migration is 1
	ver, err = runner.LatestVersion(ctx)
	if err != nil {
		t.Fatalf("unexpected error querying version after migration: %v", err)
	}
	if ver < 1 {
		t.Fatalf("expected version >= 1, got %d", ver)
	}

	// Step 4: Verify schema_migrations table records
	applied, err := runner.AppliedVersions(ctx)
	if err != nil {
		t.Fatalf("failed to query applied versions: %v", err)
	}
	if len(applied) == 0 {
		t.Fatalf("expected at least 1 applied migration record, got %d", len(applied))
	}
	if _, ok := applied[1]; !ok {
		t.Fatalf("expected migration version 1 to be recorded in schema_migrations")
	}

	// Step 5: Run migration again - must be idempotent and succeed without error
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("second migration run must be idempotent, got error: %v", err)
	}

	ver2, err := runner.LatestVersion(ctx)
	if err != nil {
		t.Fatalf("failed to query version after second run: %v", err)
	}
	if ver2 != ver {
		t.Fatalf("version changed after idempotent run: was %d, now %d", ver, ver2)
	}
}

func TestReadinessValidation(t *testing.T) {
	ctx := context.Background()

	// Case 1: Empty database must report NOT ready and list missing tables
	emptyDB, _ := setupEmptyTestDB(t)
	report, err := sqlite.CheckReadiness(ctx, emptyDB)
	if err != nil {
		t.Fatalf("CheckReadiness returned unexpected error: %v", err)
	}
	if report.Ready {
		t.Fatalf("empty unmigrated database must NOT be reported as ready")
	}
	if len(report.MissingTables) == 0 {
		t.Fatalf("empty database must report missing tables, got 0")
	}

	// Case 2: Migrated database must report Ready = true with zero missing tables
	migratedDB, _ := setupTestDB(t)
	report2, err := sqlite.CheckReadiness(ctx, migratedDB)
	if err != nil {
		t.Fatalf("CheckReadiness returned unexpected error: %v", err)
	}
	if !report2.Ready {
		t.Fatalf("migrated database must be reported as ready, error: %s, missing: %v", report2.Error, report2.MissingTables)
	}
	if len(report2.MissingTables) != 0 {
		t.Fatalf("expected 0 missing tables, got %v", report2.MissingTables)
	}
	if len(report2.ForeignKeyViolations) != 0 {
		t.Fatalf("expected 0 foreign key violations, got %v", report2.ForeignKeyViolations)
	}

	// Verify all 14 required tables are reported
	required := sqlite.RequiredTables()
	if len(required) != 14 {
		t.Fatalf("expected exactly 14 required tables, got %d", len(required))
	}
	for _, tbl := range required {
		found := false
		for _, rt := range report2.RequiredTables {
			if rt == tbl {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required table %s not in report.RequiredTables", tbl)
		}
	}
}

func TestForeignKeyConstraintEnforcement(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	// Ensure foreign_keys pragma is enabled
	var fkEnabled int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fkEnabled); err != nil {
		t.Fatalf("failed to query PRAGMA foreign_keys: %v", err)
	}
	if fkEnabled != 1 {
		t.Fatalf("expected foreign_keys=1, got %d", fkEnabled)
	}

	// 1. Insert subscription_fetches with nonexistent subscription_id -> MUST fail
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome)
		VALUES ('0191e4a0-0000-7000-8000-000000000001', 'nonexistent-sub-id', '2026-09-15T00:00:00Z', 'success');
	`)
	if err == nil {
		t.Fatalf("expected foreign key violation on subscription_fetches with nonexistent subscription_id, got nil")
	}

	// 2. Insert valid subscription, then subscription_fetch -> MUST succeed
	subID := "0191e4a0-0000-7000-8000-000000000010"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at)
		VALUES (?, 'Test Sub', 'sec://sub1', 'rev1', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
	`, subID)
	if err != nil {
		t.Fatalf("failed to insert valid subscription: %v", err)
	}

	fetchID := "0191e4a0-0000-7000-8000-000000000011"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome)
		VALUES (?, ?, '2026-09-15T00:00:00Z', 'success');
	`, fetchID, subID)
	if err != nil {
		t.Fatalf("failed to insert valid subscription_fetch: %v", err)
	}

	// 3. Insert node_sources with nonexistent node_logical_id -> MUST fail
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES ('0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', ?, ?);
	`, subID, fetchID)
	if err == nil {
		t.Fatalf("expected foreign key violation on node_sources with nonexistent node, got nil")
	}

	// 4. Insert valid node, then node_sources -> MUST succeed
	nodeLogicalID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, created_at, updated_at)
		VALUES (?, 'ss', 'Node 1', 'sec://node1', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
	`, nodeLogicalID)
	if err != nil {
		t.Fatalf("failed to insert valid node: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, nodeLogicalID, subID, fetchID)
	if err != nil {
		t.Fatalf("failed to insert valid node_source: %v", err)
	}

	// 5. Test Cascade Delete: Deleting subscription must cascade delete subscription_fetches and node_sources
	_, err = db.ExecContext(ctx, "DELETE FROM subscriptions WHERE id = ?", subID)
	if err != nil {
		t.Fatalf("failed to delete subscription: %v", err)
	}

	var fetchCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_fetches WHERE id = ?", fetchID).Scan(&fetchCount)
	if fetchCount != 0 {
		t.Fatalf("expected subscription_fetch to be cascade-deleted, count: %d", fetchCount)
	}

	var sourceCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_sources WHERE subscription_id = ?", subID).Scan(&sourceCount)
	if sourceCount != 0 {
		t.Fatalf("expected node_source to be cascade-deleted, count: %d", sourceCount)
	}

	// 6. Verify PRAGMA foreign_key_check returns zero violations
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		t.Fatalf("failed to run foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatalf("foreign_key_check found unexpected violations")
	}
}

func TestTableConstraintsAndIntegrity(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	// 1. Group edge self-loop prevention: parent_group_id == child_group_id MUST fail CHECK constraint
	groupID := "0191e4a0-0000-7000-8000-000000000020"
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at)
		VALUES (?, 'Group 1', 'select', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
	`, groupID)
	if err != nil {
		t.Fatalf("failed to insert group: %v", err)
	}

	edgeID := "0191e4a0-0000-7000-8000-000000000021"
	_, err = db.ExecContext(ctx, `
		INSERT INTO group_edges (id, parent_group_id, child_group_id, position)
		VALUES (?, ?, ?, 0);
	`, edgeID, groupID, groupID)
	if err == nil {
		t.Fatalf("expected CHECK constraint failure for group_edge self loop, got nil")
	}

	// 2. Group edge exclusivity: specifying both child_group_id AND node_logical_id MUST fail CHECK constraint
	nodeLogicalID := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, created_at, updated_at)
		VALUES (?, 'vless', 'Node 2', 'sec://node2', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
	`, nodeLogicalID)
	if err != nil {
		t.Fatalf("failed to insert node 2: %v", err)
	}

	childGroupID := "0191e4a0-0000-7000-8000-000000000022"
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at)
		VALUES (?, 'Child Group', 'url-test', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
	`, childGroupID)
	if err != nil {
		t.Fatalf("failed to insert child group: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO group_edges (id, parent_group_id, child_group_id, node_logical_id, position)
		VALUES (?, ?, ?, ?, 0);
	`, edgeID, groupID, childGroupID, nodeLogicalID)
	if err == nil {
		t.Fatalf("expected CHECK constraint failure for specifying both child_group_id and node_logical_id, got nil")
	}

	// 3. Probe run idempotency uniqueness: duplicate (actor_scope, idempotency_key) MUST fail
	runID1 := "0191e4a0-0000-7000-8000-000000000030"
	runID2 := "0191e4a0-0000-7000-8000-000000000031"
	_, err = db.ExecContext(ctx, `
		INSERT INTO probe_runs (id, idempotency_key, actor_scope, state, deadline_at, created_at, updated_at)
		VALUES (?, 'idemp-key-1', 'scope-a', 'queued', '2026-09-15T01:00:00Z', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
	`, runID1)
	if err != nil {
		t.Fatalf("failed to insert probe run 1: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO probe_runs (id, idempotency_key, actor_scope, state, deadline_at, created_at, updated_at)
		VALUES (?, 'idemp-key-1', 'scope-a', 'queued', '2026-09-15T01:00:00Z', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
	`, runID2)
	if err == nil {
		t.Fatalf("expected UNIQUE constraint violation on duplicate (actor_scope, idempotency_key), got nil")
	}

	// 4. Publication token_hash uniqueness MUST fail on duplicate
	pubID1 := "0191e4a0-0000-7000-8000-000000000040"
	pubID2 := "0191e4a0-0000-7000-8000-000000000041"
	_, err = db.ExecContext(ctx, `
		INSERT INTO publications (id, target, snapshot_digest, compiler_version, token_hash, state, created_at)
		VALUES (?, 'clash', 'sha256:abc', '1.0.0', 'hash_token_123', 'active', '2026-09-15T00:00:00Z');
	`, pubID1)
	if err != nil {
		t.Fatalf("failed to insert publication 1: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO publications (id, target, snapshot_digest, compiler_version, token_hash, state, created_at)
		VALUES (?, 'mihomo', 'sha256:def', '1.0.0', 'hash_token_123', 'active', '2026-09-15T00:00:00Z');
	`, pubID2)
	if err == nil {
		t.Fatalf("expected UNIQUE constraint violation on duplicate token_hash, got nil")
	}

	// 5. Settings table singleton constraint (id = 1)
	_, err = db.ExecContext(ctx, `
		INSERT INTO settings (id, probe_concurrency_window, updated_at)
		VALUES (2, 20, '2026-09-15T00:00:00Z');
	`)
	if err == nil {
		t.Fatalf("expected CHECK constraint failure for settings id != 1, got nil")
	}
}

func TestWithTxRollbackAndCommit(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	// Case 1: Transaction commits when fn returns nil
	subID := "0191e4a0-0000-7000-8000-000000000050"
	err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at)
			VALUES (?, 'Committed Sub', 'sec://sub50', 'rev1', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
		`, subID)
		return err
	})
	if err != nil {
		t.Fatalf("WithTx returned unexpected error: %v", err)
	}

	var count int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscriptions WHERE id = ?", subID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 committed subscription, got %d", count)
	}

	// Case 2: Transaction automatically rolls back when fn returns an error
	subIDFail := "0191e4a0-0000-7000-8000-000000000051"
	customErr := errors.New("simulated error")
	err = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at)
			VALUES (?, 'Rolled Back Sub', 'sec://sub51', 'rev1', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
		`, subIDFail)
		if err != nil {
			return err
		}
		return customErr
	})
	if !errors.Is(err, customErr) {
		t.Fatalf("expected customErr, got: %v", err)
	}

	var countFail int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscriptions WHERE id = ?", subIDFail).Scan(&countFail)
	if countFail != 0 {
		t.Fatalf("expected rolled back subscription count 0, got %d", countFail)
	}

	// Case 3: Transaction automatically rolls back on context cancellation
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel immediately

	subIDCancel := "0191e4a0-0000-7000-8000-000000000052"
	err = sqlite.WithTx(cancelledCtx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at)
			VALUES (?, 'Cancelled Sub', 'sec://sub52', 'rev1', '2026-09-15T00:00:00Z', '2026-09-15T00:00:00Z');
		`, subIDCancel)
		return err
	})
	if err == nil {
		t.Fatalf("expected error on cancelled context, got nil")
	}

	var countCancel int
	_ = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM subscriptions WHERE id = ?", subIDCancel).Scan(&countCancel)
	if countCancel != 0 {
		t.Fatalf("expected cancelled subscription count 0, got %d", countCancel)
	}
}

func TestTempFileDBWithWALMode(t *testing.T) {
	ctx := context.Background()
	db, dbPath := setupTempFileDB(t)

	// Verify file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("db file was not created: %v", err)
	}

	// Verify journal_mode is WAL
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("expected journal_mode=wal, got %s", journalMode)
	}

	// Verify busy_timeout is set
	var busyTimeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatalf("failed to query busy_timeout: %v", err)
	}
	if busyTimeout < 1000 {
		t.Fatalf("expected busy_timeout >= 1000, got %d", busyTimeout)
	}

	// Verify readiness on file db
	report, err := sqlite.CheckReadiness(ctx, db)
	if err != nil {
		t.Fatalf("readiness check failed: %v", err)
	}
	if !report.Ready {
		t.Fatalf("expected ready=true, got false: %s", report.Error)
	}
}

func TestRepositoriesCRUD(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	// 1. SettingsRepository
	settingsRepo := sqlite.NewSettingsRepository(db)
	st, err := settingsRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get default settings: %v", err)
	}
	if st.ProbeConcurrencyWindow != 16 {
		t.Fatalf("expected default probe concurrency window 16, got %d", st.ProbeConcurrencyWindow)
	}
	st.ProbeConcurrencyWindow = 20
	if err := settingsRepo.Update(ctx, st); err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}
	st2, err := settingsRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get updated settings: %v", err)
	}
	if st2.ProbeConcurrencyWindow != 20 {
		t.Fatalf("expected updated probe concurrency window 20, got %d", st2.ProbeConcurrencyWindow)
	}

	// Test AdminToken persistence and UpdateAdminToken
	if st2.AdminToken != "" {
		t.Fatalf("expected empty default admin_token, got %q", st2.AdminToken)
	}
	testVerifier := "$2a$10$abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqr"
	if err := settingsRepo.UpdateAdminToken(ctx, testVerifier); err != nil {
		t.Fatalf("failed to update admin token: %v", err)
	}
	st3, err := settingsRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get settings after token update: %v", err)
	}
	if st3.AdminToken != testVerifier {
		t.Fatalf("expected admin_token %q, got %q", testVerifier, st3.AdminToken)
	}
	// Clear admin token
	if err := settingsRepo.UpdateAdminToken(ctx, ""); err != nil {
		t.Fatalf("failed to clear admin token: %v", err)
	}
	st4, err := settingsRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get settings after token clear: %v", err)
	}
	if st4.AdminToken != "" {
		t.Fatalf("expected cleared admin_token to be empty, got %q", st4.AdminToken)
	}

	// 2. SubscriptionRepository
	subRepo := sqlite.NewSubscriptionRepository(db)
	subID := "0191e4a0-0000-7000-8000-000000000100"
	sub := &domain.Subscription{
		ID:                 subID,
		Name:               "Alpha Sub",
		SourceURLSecretRef: "sec://alpha",
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds:  3600,
			UserAgentPolicy:  "clash",
			FetchProxyRef:    "sec://proxy1",
			TimeoutSeconds:   45,
			MaxResponseBytes: 5242880,
		},
		Revision:  "rev-1",
		CreatedAt: domain.NowUTC(),
		UpdatedAt: domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	fetchedSub, err := subRepo.GetByID(ctx, subID)
	if err != nil {
		t.Fatalf("failed to get subscription by ID: %v", err)
	}
	if fetchedSub.Name != "Alpha Sub" || fetchedSub.RefreshPolicy.IntervalSeconds != 3600 {
		t.Fatalf("fetched subscription mismatch: %+v", fetchedSub)
	}

	subs, total, err := subRepo.List(ctx, domain.SubscriptionFilter{
		Pagination:  domain.Pagination{Page: 1, PageSize: 10},
		EnabledOnly: true,
	})
	if err != nil {
		t.Fatalf("failed to list subscriptions: %v", err)
	}
	if total != 1 || len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got total=%d, len=%d", total, len(subs))
	}

	// 3. NodeRepository & NodeSourceRepository
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeSourceRepo := sqlite.NewNodeSourceRepository(db)
	nodeLogicalID := "1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff"
	node := domain.Node{
		LogicalID:                 nodeLogicalID,
		Protocol:                  domain.ProtocolVMess,
		DisplayName:               "Node Alpha",
		NormalizedConfigSecretRef: "sec://node-alpha",
		Active:                    true,
		CreatedAt:                 domain.NowUTC(),
		UpdatedAt:                 domain.NowUTC(),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatalf("failed to upsert node: %v", err)
	}

	fetchedNode, err := nodeRepo.GetByLogicalID(ctx, nodeLogicalID)
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if fetchedNode.DisplayName != "Node Alpha" || fetchedNode.Protocol != domain.ProtocolVMess {
		t.Fatalf("fetched node mismatch: %+v", fetchedNode)
	}

	nodeSource := &domain.NodeSource{
		NodeLogicalID:   nodeLogicalID,
		SubscriptionID:  subID,
		LastSeenFetchID: "fetch-1",
	}
	if err := nodeSourceRepo.Upsert(ctx, nodeSource); err != nil {
		t.Fatalf("failed to upsert node source: %v", err)
	}

	sources, err := nodeSourceRepo.ListByNode(ctx, nodeLogicalID)
	if err != nil {
		t.Fatalf("failed to list node sources: %v", err)
	}
	if len(sources) != 1 || sources[0].SubscriptionID != subID {
		t.Fatalf("node sources mismatch: %+v", sources)
	}

	// 4. ProbeRunRepository & ProbeObservationRepository
	probeRunRepo := sqlite.NewProbeRunRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	runID := "0191e4a0-0000-7000-8000-000000000200"
	run := &domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: "run-key-100",
		ActorScope:     "test",
		ConfigRevision: "rev-probe-1",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     domain.NowUTC().Add(time.Hour),
		CreatedAt:      domain.NowUTC(),
		UpdatedAt:      domain.NowUTC(),
	}
	if err := probeRunRepo.Create(ctx, run); err != nil {
		t.Fatalf("failed to create probe run: %v", err)
	}

	byKey, err := probeRunRepo.GetByIdempotencyKey(ctx, "test", "run-key-100")
	if err != nil {
		t.Fatalf("failed to get probe run by idempotency key: %v", err)
	}
	if byKey.ID != runID {
		t.Fatalf("expected run ID %s, got %s", runID, byKey.ID)
	}

	if err := probeRunRepo.UpdateState(ctx, runID, domain.ProbeRunStateRunning); err != nil {
		t.Fatalf("failed to update probe run state: %v", err)
	}

	obsID := "0191e4a0-0000-7000-8000-000000000201"
	obs := &domain.ProbeObservation{
		ID:              obsID,
		ProbeRunID:      runID,
		NodeLogicalID:   nodeLogicalID,
		Kind:            domain.ProbeKindBaseline,
		Verdict:         domain.VerdictAvailable,
		EvidenceDigest:  "digest-xyz",
		ObservedAt:      domain.NowUTC(),
		LatencyMS:       42,
		RedactedSummary: "ok",
	}
	if err := obsRepo.Create(ctx, obs); err != nil {
		t.Fatalf("failed to record probe observation: %v", err)
	}

	obsList, err := obsRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("failed to list observations by run: %v", err)
	}
	if len(obsList) != 1 || obsList[0].Verdict != domain.VerdictAvailable {
		t.Fatalf("observations mismatch: %+v", obsList)
	}

	// 5. AuditRepository
	auditRepo := sqlite.NewAuditRepository(db)
	auditID := "0191e4a0-0000-7000-8000-000000000300"
	auditEvent := &domain.AuditEvent{
		ID:              auditID,
		ActorKind:       domain.ActorKindAdmin,
		RequestID:       "req-999",
		Action:          "subscription.create",
		Result:          domain.AuditResultSuccess,
		RedactedSummary: "created sub alpha",
		CreatedAt:       domain.NowUTC(),
	}
	if err := auditRepo.Record(ctx, auditEvent); err != nil {
		t.Fatalf("failed to record audit event: %v", err)
	}

	events, totalAudits, err := auditRepo.List(ctx, domain.AuditFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
		Action:     "subscription.create",
	})
	if err != nil {
		t.Fatalf("failed to list audit events: %v", err)
	}
	if totalAudits != 1 || len(events) != 1 || events[0].ID != auditID {
		t.Fatalf("audit events mismatch: total=%d, events=%+v", totalAudits, events)
	}
}
