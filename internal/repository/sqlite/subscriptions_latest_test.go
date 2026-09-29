package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func newSubscriptionsTestDB(t *testing.T) (*sql.DB, domain.SubscriptionRepository) {
	t.Helper()
	cfg := sqlite.Config{
		Path:            fmt.Sprintf("file:sub_test_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, sqlite.NewSubscriptionRepository(db)
}

func insertTestFetch(t *testing.T, db *sql.DB, id, subID, startedAt, finishedAt, outcome string) {
	t.Helper()
	const query = `
	INSERT INTO subscription_fetches (
		id, subscription_id, started_at, finished_at, outcome,
		content_digest, redacted_error, nodes_parsed, nodes_valid
	) VALUES (?, ?, ?, ?, ?, '', '', 10, 10);`

	var fin sql.NullString
	if finishedAt != "" {
		fin = sql.NullString{String: finishedAt, Valid: true}
	}
	_, err := db.Exec(query, id, subID, startedAt, fin, outcome)
	if err != nil {
		t.Fatalf("failed to insert test fetch: %v", err)
	}
}

func TestSubscriptionsLatestFetch_NoRecordsAndIncomplete(t *testing.T) {
	ctx := context.Background()
	db, repo := newSubscriptionsTestDB(t)

	// Sub 1: completely clean, no fetch records
	sub1 := &domain.Subscription{
		ID:                 "01910000-0000-7000-8000-000000000001",
		Name:               "No Fetches",
		SourceURLSecretRef: "secret://sub-1",
		Enabled:            true,
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          domain.NowUTC().Add(-1 * time.Hour),
		UpdatedAt:          domain.NowUTC().Add(-1 * time.Hour),
	}
	if err := repo.Create(ctx, sub1); err != nil {
		t.Fatalf("Create sub1 failed: %v", err)
	}

	// Sub 2: has an in-progress / uncompleted fetch (finished_at is NULL / empty)
	sub2 := &domain.Subscription{
		ID:                 "01910000-0000-7000-8000-000000000002",
		Name:               "Incomplete Fetch",
		SourceURLSecretRef: "secret://sub-2",
		Enabled:            true,
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          domain.NowUTC().Add(-30 * time.Minute),
		UpdatedAt:          domain.NowUTC().Add(-30 * time.Minute),
	}
	if err := repo.Create(ctx, sub2); err != nil {
		t.Fatalf("Create sub2 failed: %v", err)
	}
	// Incomplete fetch: NULL finished_at
	insertTestFetch(t, db, "fetch-in-progress", sub2.ID, domain.NowUTC().Format(time.RFC3339), "", "success")

	// Verify GetByID for sub1
	got1, err := repo.GetByID(ctx, sub1.ID)
	if err != nil {
		t.Fatalf("GetByID sub1 failed: %v", err)
	}
	if got1.LastRefreshedAt != nil {
		t.Fatalf("expected nil LastRefreshedAt for sub with no fetches, got: %v", got1.LastRefreshedAt)
	}
	if got1.LastRefreshOutcome != nil {
		t.Fatalf("expected nil LastRefreshOutcome for sub with no fetches, got: %v", got1.LastRefreshOutcome)
	}

	// Verify GetByID for sub2 (incomplete fetch should NOT be treated as completed refresh)
	got2, err := repo.GetByID(ctx, sub2.ID)
	if err != nil {
		t.Fatalf("GetByID sub2 failed: %v", err)
	}
	if got2.LastRefreshedAt != nil {
		t.Fatalf("expected nil LastRefreshedAt for sub with incomplete fetch, got: %v", got2.LastRefreshedAt)
	}
	if got2.LastRefreshOutcome != nil {
		t.Fatalf("expected nil LastRefreshOutcome for sub with incomplete fetch, got: %v", got2.LastRefreshOutcome)
	}

	// Verify List
	subs, total, err := repo.List(ctx, domain.SubscriptionFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != 2 || len(subs) != 2 {
		t.Fatalf("expected 2 subs, got total=%d, len=%d", total, len(subs))
	}
	for _, s := range subs {
		if s.LastRefreshedAt != nil {
			t.Errorf("expected sub %s LastRefreshedAt to be nil, got %v", s.ID, s.LastRefreshedAt)
		}
	}
}

func TestSubscriptionsLatestFetch_SuccessFollowedByFailure(t *testing.T) {
	ctx := context.Background()
	db, repo := newSubscriptionsTestDB(t)

	sub := &domain.Subscription{
		ID:                 "01910000-0000-7000-8000-000000000010",
		Name:               "Flapping Sub",
		SourceURLSecretRef: "secret://flapping",
		Enabled:            true,
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          domain.NowUTC().Add(-2 * time.Hour),
		UpdatedAt:          domain.NowUTC().Add(-2 * time.Hour),
	}
	if err := repo.Create(ctx, sub); err != nil {
		t.Fatalf("Create sub failed: %v", err)
	}

	t1Start := "2026-09-28T10:00:00Z"
	t1End := "2026-09-28T10:00:05Z"
	insertTestFetch(t, db, "fetch-1-success", sub.ID, t1Start, t1End, "success")

	t2Start := "2026-09-29T10:00:00Z"
	t2End := "2026-09-29T10:00:03Z"
	insertTestFetch(t, db, "fetch-2-failed", sub.ID, t2Start, t2End, "failed")

	// GetByID must reflect the latest failed attempt
	got, err := repo.GetByID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.LastRefreshedAt == nil {
		t.Fatalf("expected non-nil LastRefreshedAt")
	}
	if got.LastRefreshedAt.Format(time.RFC3339) != t2End {
		t.Fatalf("expected LastRefreshedAt %s, got %s", t2End, got.LastRefreshedAt.Format(time.RFC3339))
	}
	if got.LastRefreshOutcome == nil || *got.LastRefreshOutcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected LastRefreshOutcome failed, got: %v", got.LastRefreshOutcome)
	}

	// List must also reflect the latest failed attempt
	listSubs, _, err := repo.List(ctx, domain.SubscriptionFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listSubs) != 1 {
		t.Fatalf("expected 1 sub in list, got %d", len(listSubs))
	}
	if listSubs[0].LastRefreshedAt == nil || listSubs[0].LastRefreshedAt.Format(time.RFC3339) != t2End {
		t.Fatalf("List sub LastRefreshedAt mismatch: want %s, got %v", t2End, listSubs[0].LastRefreshedAt)
	}
	if listSubs[0].LastRefreshOutcome == nil || *listSubs[0].LastRefreshOutcome != domain.FetchOutcomeFailed {
		t.Fatalf("List sub LastRefreshOutcome mismatch: want failed, got %v", listSubs[0].LastRefreshOutcome)
	}
}

func TestSubscriptionsLatestFetch_TieOnStartedAt(t *testing.T) {
	ctx := context.Background()
	db, repo := newSubscriptionsTestDB(t)

	sub := &domain.Subscription{
		ID:                 "01910000-0000-7000-8000-000000000020",
		Name:               "Tie Sub",
		SourceURLSecretRef: "secret://tie",
		Enabled:            true,
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          domain.NowUTC().Add(-2 * time.Hour),
		UpdatedAt:          domain.NowUTC().Add(-2 * time.Hour),
	}
	if err := repo.Create(ctx, sub); err != nil {
		t.Fatalf("Create sub failed: %v", err)
	}

	sameStartedAt := "2026-09-29T12:00:00Z"
	// ID 'fetch-tie-1' vs 'fetch-tie-2'
	// 'fetch-tie-2' > 'fetch-tie-1' lexicographically in id DESC
	insertTestFetch(t, db, "fetch-tie-1", sub.ID, sameStartedAt, "2026-09-29T12:00:02Z", "success")
	insertTestFetch(t, db, "fetch-tie-2", sub.ID, sameStartedAt, "2026-09-29T12:00:05Z", "failed")

	got, err := repo.GetByID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.LastRefreshedAt == nil {
		t.Fatalf("expected non-nil LastRefreshedAt")
	}
	// Deterministic selection by id DESC must pick 'fetch-tie-2'
	expectedTime := "2026-09-29T12:00:05Z"
	if got.LastRefreshedAt.Format(time.RFC3339) != expectedTime {
		t.Fatalf("tie breaker did not pick fetch-tie-2: want %s, got %s", expectedTime, got.LastRefreshedAt.Format(time.RFC3339))
	}
	if got.LastRefreshOutcome == nil || *got.LastRefreshOutcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected outcome failed from fetch-tie-2, got: %v", got.LastRefreshOutcome)
	}
}

func TestSubscriptionsLatestFetch_Pagination(t *testing.T) {
	ctx := context.Background()
	db, repo := newSubscriptionsTestDB(t)

	// Create 5 subscriptions with spaced created_at so ordering is predictable
	baseTime := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		subID := fmt.Sprintf("01910000-0000-7000-8000-00000000000%d", i)
		sub := &domain.Subscription{
			ID:                 subID,
			Name:               fmt.Sprintf("Sub %d", i),
			SourceURLSecretRef: fmt.Sprintf("secret://sub-%d", i),
			Enabled:            true,
			Revision:           domain.MustNewUUIDv7(),
			CreatedAt:          baseTime.Add(time.Duration(i) * time.Hour),
			UpdatedAt:          baseTime.Add(time.Duration(i) * time.Hour),
		}
		if err := repo.Create(ctx, sub); err != nil {
			t.Fatalf("Create sub %d failed: %v", i, err)
		}

		// Only subs 2, 4, 5 have completed fetches
		if i == 2 {
			insertTestFetch(t, db, fmt.Sprintf("fetch-%d", i), subID, "2026-09-29T02:00:00Z", "2026-09-29T02:00:10Z", "partial")
		} else if i == 4 {
			insertTestFetch(t, db, fmt.Sprintf("fetch-%d", i), subID, "2026-09-29T04:00:00Z", "2026-09-29T04:00:10Z", "success")
		} else if i == 5 {
			insertTestFetch(t, db, fmt.Sprintf("fetch-%d", i), subID, "2026-09-29T05:00:00Z", "2026-09-29T05:00:10Z", "failed")
		}
	}

	seenIDs := make(map[string]bool)

	// Page 1: pageSize 2
	p1, total, err := repo.List(ctx, domain.SubscriptionFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 2},
	})
	if err != nil {
		t.Fatalf("Page 1 failed: %v", err)
	}
	if total != 5 || len(p1) != 2 {
		t.Fatalf("Page 1 count mismatch: total=%d, len=%d", total, len(p1))
	}
	for _, s := range p1 {
		seenIDs[s.ID] = true
	}

	// Page 2: pageSize 2
	p2, total2, err := repo.List(ctx, domain.SubscriptionFilter{
		Pagination: domain.Pagination{Page: 2, PageSize: 2},
	})
	if err != nil {
		t.Fatalf("Page 2 failed: %v", err)
	}
	if total2 != 5 || len(p2) != 2 {
		t.Fatalf("Page 2 count mismatch: total=%d, len=%d", total2, len(p2))
	}
	for _, s := range p2 {
		if seenIDs[s.ID] {
			t.Fatalf("duplicate sub ID %s appeared across page 1 and page 2", s.ID)
		}
		seenIDs[s.ID] = true
	}

	// Page 3: pageSize 2 (should have 1 item)
	p3, total3, err := repo.List(ctx, domain.SubscriptionFilter{
		Pagination: domain.Pagination{Page: 3, PageSize: 2},
	})
	if err != nil {
		t.Fatalf("Page 3 failed: %v", err)
	}
	if total3 != 5 || len(p3) != 1 {
		t.Fatalf("Page 3 count mismatch: total=%d, len=%d", total3, len(p3))
	}
	for _, s := range p3 {
		if seenIDs[s.ID] {
			t.Fatalf("duplicate sub ID %s appeared in page 3", s.ID)
		}
		seenIDs[s.ID] = true
	}

	if len(seenIDs) != 5 {
		t.Fatalf("expected 5 distinct subs across pagination, got %d", len(seenIDs))
	}

	// Verify sub 5 (in page 1 since ordered by created_at DESC) has failed outcome
	sub5Found := false
	for _, s := range p1 {
		if s.ID == "01910000-0000-7000-8000-000000000005" {
			sub5Found = true
			if s.LastRefreshedAt == nil || s.LastRefreshOutcome == nil || *s.LastRefreshOutcome != domain.FetchOutcomeFailed {
				t.Fatalf("sub 5 latest fetch mismatch: at=%v, outcome=%v", s.LastRefreshedAt, s.LastRefreshOutcome)
			}
		}
	}
	if !sub5Found {
		t.Fatalf("sub 5 not found on page 1")
	}
}

func TestSubscriptionDeleteDeactivatesOrphanNodes(t *testing.T) {
	ctx := context.Background()
	db, subRepo := newSubscriptionsTestDB(t)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)

	now := domain.NowUTC().Add(-10 * time.Minute)

	subA := &domain.Subscription{
		ID:                 "01910000-0000-7000-8000-000000000101",
		Name:               "Sub A",
		SourceURLSecretRef: "https://example.com/sub-a",
		Enabled:            true,
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	subB := &domain.Subscription{
		ID:                 "01910000-0000-7000-8000-000000000102",
		Name:               "Sub B",
		SourceURLSecretRef: "https://example.com/sub-b",
		Enabled:            true,
		Revision:           domain.MustNewUUIDv7(),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := subRepo.Create(ctx, subA); err != nil {
		t.Fatalf("Create subA failed: %v", err)
	}
	if err := subRepo.Create(ctx, subB); err != nil {
		t.Fatalf("Create subB failed: %v", err)
	}

	nodes := []domain.Node{
		{
			LogicalID:   "node-exclusive-a1",
			Protocol:    domain.ProtocolVMess,
			DisplayName: "Exclusive A1",
			Server:      "198.51.100.1",
			Port:        443,
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			LogicalID:   "node-exclusive-a2",
			Protocol:    domain.ProtocolTrojan,
			DisplayName: "Exclusive A2",
			Server:      "198.51.100.2",
			Port:        443,
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			LogicalID:   "node-shared-ab",
			Protocol:    domain.ProtocolVLESS,
			DisplayName: "Shared AB",
			Server:      "198.51.100.3",
			Port:        443,
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			LogicalID:   "node-exclusive-b1",
			Protocol:    domain.ProtocolTrojan,
			DisplayName: "Exclusive B1",
			Server:      "198.51.100.4",
			Port:        8388,
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatalf("UpsertBatch nodes failed: %v", err)
	}

	sources := []domain.NodeSource{
		{NodeLogicalID: "node-exclusive-a1", SubscriptionID: subA.ID, LastSeenFetchID: "fetch-a"},
		{NodeLogicalID: "node-exclusive-a2", SubscriptionID: subA.ID, LastSeenFetchID: "fetch-a"},
		{NodeLogicalID: "node-shared-ab", SubscriptionID: subA.ID, LastSeenFetchID: "fetch-a"},
		{NodeLogicalID: "node-shared-ab", SubscriptionID: subB.ID, LastSeenFetchID: "fetch-b"},
		{NodeLogicalID: "node-exclusive-b1", SubscriptionID: subB.ID, LastSeenFetchID: "fetch-b"},
	}
	for i := range sources {
		if err := sourceRepo.Upsert(ctx, &sources[i]); err != nil {
			t.Fatalf("Upsert source %+v failed: %v", sources[i], err)
		}
	}

	// Deleting nonexistent subscription must return not_found and not deactivate anything
	err := subRepo.Delete(ctx, "01910000-0000-7000-8000-nonexistent")
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Category != domain.CategoryNotFound {
		t.Fatalf("Delete nonexistent sub error = %v, want not_found", err)
	}

	// Delete Sub A
	if err := subRepo.Delete(ctx, subA.ID); err != nil {
		t.Fatalf("Delete subA failed: %v", err)
	}

	// Sub A must be gone
	if _, err := subRepo.GetByID(ctx, subA.ID); !errors.As(err, &domErr) || domErr.Category != domain.CategoryNotFound {
		t.Fatalf("GetByID(subA) after delete error = %v, want not_found", err)
	}

	// Exclusive nodes of Sub A must be deactivated (active = false)
	for _, id := range []string{"node-exclusive-a1", "node-exclusive-a2"} {
		n, err := nodeRepo.GetByLogicalID(ctx, id)
		if err != nil {
			t.Fatalf("GetByLogicalID(%s) failed: %v", id, err)
		}
		if n.Active {
			t.Fatalf("expected orphan node %s to have Active=false after deleting Sub A, got true", id)
		}
		remSources, err := sourceRepo.ListByNode(ctx, id)
		if err != nil {
			t.Fatalf("ListByNode(%s) failed: %v", id, err)
		}
		if len(remSources) != 0 {
			t.Fatalf("expected 0 remaining sources for %s, got %+v", id, remSources)
		}
	}

	// Shared node and Sub B exclusive node must remain active (active = true)
	for _, id := range []string{"node-shared-ab", "node-exclusive-b1"} {
		n, err := nodeRepo.GetByLogicalID(ctx, id)
		if err != nil {
			t.Fatalf("GetByLogicalID(%s) failed: %v", id, err)
		}
		if !n.Active {
			t.Fatalf("expected node %s still backed by Sub B to remain Active=true, got false", id)
		}
		remSources, err := sourceRepo.ListByNode(ctx, id)
		if err != nil {
			t.Fatalf("ListByNode(%s) failed: %v", id, err)
		}
		if len(remSources) != 1 || remSources[0].SubscriptionID != subB.ID {
			t.Fatalf("expected 1 remaining source (Sub B) for %s, got %+v", id, remSources)
		}
	}

	activeNodes, totalActive, err := nodeRepo.List(ctx, domain.NodeFilter{
		ActiveOnly: true,
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
	})
	if err != nil {
		t.Fatalf("List active nodes failed: %v", err)
	}
	if totalActive != 2 || len(activeNodes) != 2 {
		t.Fatalf("expected 2 active nodes after deleting Sub A, got total=%d len=%d", totalActive, len(activeNodes))
	}

	// Now delete Sub B -> all remaining nodes become orphans and must be deactivated
	if err := subRepo.Delete(ctx, subB.ID); err != nil {
		t.Fatalf("Delete subB failed: %v", err)
	}

	activeNodesAfterB, totalActiveAfterB, err := nodeRepo.List(ctx, domain.NodeFilter{
		ActiveOnly: true,
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
	})
	if err != nil {
		t.Fatalf("List active nodes after Sub B delete failed: %v", err)
	}
	if totalActiveAfterB != 0 || len(activeNodesAfterB) != 0 {
		t.Fatalf("expected 0 active nodes after deleting Sub B, got total=%d len=%d", totalActiveAfterB, len(activeNodesAfterB))
	}
}
