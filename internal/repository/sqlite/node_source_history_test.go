package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestNodeSourceHistoryRepository_BatchInsertAndQuery(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	nodeRepo := sqlite.NewNodeRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	histRepo := sqlite.NewNodeSourceHistoryRepository(db)

	// Create test node
	node := domain.Node{
		LogicalID:          "node-hist-test-1",
		Protocol:           domain.ProtocolSS,
		DisplayName:        "Hist Test Node",
		Server:             "1.1.1.1",
		Port:               8388,
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatalf("failed to insert test node: %v", err)
	}

	// Create test subscription
	sub := domain.Subscription{
		ID:                 "sub-hist-test-1",
		Name:               "Test Sub A",
		SourceURLSecretRef: "secret-url-ref",
		Enabled:            true,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	if err := subRepo.Create(ctx, &sub); err != nil {
		t.Fatalf("failed to create test sub: %v", err)
	}

	t1 := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	rev1 := int64(1)

	subID := sub.ID
	records := []domain.NodeSourceHistory{
		{
			ID:                 "h-1",
			NodeLogicalID:      node.LogicalID,
			SubscriptionID:     &subID,
			SourceIdentity:     "legacy:src:1",
			SourceLabel:        "Test Sub A",
			ConnectionRevision: &rev1,
			RelationState:      domain.RelationStateVerified,
			Cause:              domain.CauseLegacyImport,
			FirstObservedAt:    &t1,
			LastObservedAt:     &t1,
			EvidenceKind:       "legacy_cold_archive",
			EvidenceKey:        "legacy:pk:1:link:1",
			EvidenceJSON:       `{"archive_sha256":"test-hash"}`,
			CreatedAt:          t1,
		},
		{
			ID:                 "h-2",
			NodeLogicalID:      node.LogicalID,
			SubscriptionID:     &subID,
			SourceIdentity:     "sub:sub-hist-test-1",
			SourceLabel:        "Test Sub A",
			ConnectionRevision: &rev1,
			RelationState:      domain.RelationStateVerified,
			Cause:              domain.CauseRefreshRemoved,
			FirstObservedAt:    &t2,
			LastObservedAt:     &t2,
			EvidenceKind:       "subscription_refresh_prune",
			EvidenceKey:        "refresh:fetch-1",
			EvidenceJSON:       `{"fetch_id":"fetch-1"}`,
			CreatedAt:          t2,
		},
	}

	if err := histRepo.InsertBatch(ctx, records); err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	// Test Query by Node Logical ID (should be ordered by observation time DESC: h-2 then h-1)
	byNode, err := histRepo.ListByNodeLogicalID(ctx, node.LogicalID)
	if err != nil {
		t.Fatalf("ListByNodeLogicalID failed: %v", err)
	}
	if len(byNode) != 2 {
		t.Fatalf("expected 2 history records, got %d", len(byNode))
	}
	if byNode[0].ID != "h-2" || byNode[1].ID != "h-1" {
		t.Errorf("expected records ordered DESC by time, got [%s, %s]", byNode[0].ID, byNode[1].ID)
	}

	// Test Query by Subscription ID
	bySub, err := histRepo.ListBySubscriptionID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ListBySubscriptionID failed: %v", err)
	}
	if len(bySub) != 2 {
		t.Fatalf("expected 2 records for sub, got %d", len(bySub))
	}

	// Test Idempotent Insert (Insert same records again)
	if err := histRepo.InsertBatch(ctx, records); err != nil {
		t.Fatalf("idempotent re-insert failed: %v", err)
	}
	afterReinsert, err := histRepo.ListByNodeLogicalID(ctx, node.LogicalID)
	if err != nil || len(afterReinsert) != 2 {
		t.Fatalf("expected still 2 records after reinsert, got %d (err: %v)", len(afterReinsert), err)
	}

	// Test ON DELETE SET NULL cascade: deleting subscription must NOT delete history, but set subscription_id to NULL
	if err := subRepo.Delete(ctx, sub.ID); err != nil {
		t.Fatalf("subRepo.Delete failed: %v", err)
	}

	afterSubDelete, err := histRepo.ListByNodeLogicalID(ctx, node.LogicalID)
	if err != nil {
		t.Fatalf("ListByNodeLogicalID after sub delete failed: %v", err)
	}
	if len(afterSubDelete) < 2 {
		t.Fatalf("expected at least 2 records retained after sub delete, got %d", len(afterSubDelete))
	}
	for _, rec := range afterSubDelete {
		if rec.SubscriptionID != nil {
			t.Errorf("expected subscription_id to be NULL after sub delete, got %v", *rec.SubscriptionID)
		}
		if rec.SourceLabel != "Test Sub A" {
			t.Errorf("expected SourceLabel to remain 'Test Sub A', got %q", rec.SourceLabel)
		}
	}
}

func TestNodeSourceHistoryMigration_IdempotencyAndFKCheck(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	// Verify table exists
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='node_source_history';").Scan(&count); err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected node_source_history table to exist, got count %d", count)
	}

	// Verify idempotency: re-running migrations must succeed without error
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("re-running migrations must be idempotent, got: %v", err)
	}

	// Verify PRAGMA foreign_key_check is completely clean
	fkRows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_check failed: %v", err)
	}
	defer fkRows.Close()

	if fkRows.Next() {
		var table, rowid, parent, fkid sql.NullString
		_ = fkRows.Scan(&table, &rowid, &parent, &fkid)
		t.Fatalf("foreign key check failed on table %s (rowid=%s, parent=%s, fkid=%s)", table.String, rowid.String, parent.String, fkid.String)
	}
}
