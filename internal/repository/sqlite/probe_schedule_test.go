package sqlite_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestProbeScheduleRepository(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	repo := sqlite.NewProbeScheduleRepository(db)

	// 1. Get default schedule seeded by migration
	sched, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get default probe schedule: %v", err)
	}
	if sched.Enabled {
		t.Fatalf("expected default schedule to be disabled, got enabled=true")
	}
	if sched.IntervalSeconds != 3600 {
		t.Fatalf("expected interval 3600, got %d", sched.IntervalSeconds)
	}
	if len(sched.Kinds) != 1 || sched.Kinds[0] != domain.ProbeKindBaseline {
		t.Fatalf("expected kinds [baseline], got %+v", sched.Kinds)
	}
	if sched.NextDueAt != nil {
		t.Fatalf("expected next_due_at to be nil initially, got %v", sched.NextDueAt)
	}

	// 2. Validate bounds on Update
	badSched := *sched
	badSched.IntervalSeconds = 10 // < 60
	if err := repo.Update(ctx, &badSched); err == nil {
		t.Fatalf("expected validation error for interval < 60, got nil")
	}

	badSched.IntervalSeconds = 604801 // > 604800
	if err := repo.Update(ctx, &badSched); err == nil {
		t.Fatalf("expected validation error for interval > 604800, got nil")
	}

	badSched.IntervalSeconds = 300
	badSched.Kinds = []domain.ProbeKind{}
	if err := repo.Update(ctx, &badSched); err == nil {
		t.Fatalf("expected validation error for empty kinds, got nil")
	}

	// 3. Update valid schedule
	now := time.Now().UTC().Truncate(time.Second)
	nextDue := now.Add(5 * time.Minute)
	sched.Enabled = true
	sched.IntervalSeconds = 600
	sched.Kinds = []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindGeo}
	sched.NextDueAt = &nextDue
	sched.Generation = 1

	if err := repo.Update(ctx, sched); err != nil {
		t.Fatalf("failed to update probe schedule: %v", err)
	}

	updated, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get updated probe schedule: %v", err)
	}
	if !updated.Enabled {
		t.Fatalf("expected enabled=true")
	}
	if updated.IntervalSeconds != 600 {
		t.Fatalf("expected interval 600, got %d", updated.IntervalSeconds)
	}
	if len(updated.Kinds) != 2 || updated.Kinds[1] != domain.ProbeKindGeo {
		t.Fatalf("expected kinds [baseline, geo], got %+v", updated.Kinds)
	}
	if updated.NextDueAt == nil || !updated.NextDueAt.Equal(nextDue) {
		t.Fatalf("expected next_due_at %v, got %v", nextDue, updated.NextDueAt)
	}
	if updated.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", updated.Generation)
	}
}

func TestProbeBatchesAndLeases(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	repo := sqlite.NewProbeScheduleRepository(db)
	runRepo := sqlite.NewProbeRunRepository(db)

	now := time.Now().UTC().Truncate(time.Second)
	window := now.Add(time.Hour)

	// Create test runs to associate
	run1 := &domain.ProbeRun{
		ID:             domain.MustNewUUIDv7(),
		IdempotencyKey: "key-1",
		ActorScope:     "system:periodic-probe",
		State:          domain.ProbeRunStateQueued,
		DeadlineAt:     now.Add(10 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := runRepo.Create(ctx, run1); err != nil {
		t.Fatalf("failed to create run1: %v", err)
	}

	batch := &domain.ProbeBatch{
		ID:         domain.MustNewUUIDv7(),
		WindowAt:   window,
		Generation: 1,
		Owner:      "node-a",
		State:      domain.ProbeBatchStatePending,
		RunIDs:     []string{run1.ID},
		Counts: domain.ProbeBatchCounts{
			TotalNodes:     10,
			DispatchedRuns: 1,
			CompletedRuns:  0,
			SkippedNodes:   1,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 1. Create batch
	if err := repo.CreateBatch(ctx, batch); err != nil {
		t.Fatalf("failed to create batch: %v", err)
	}

	// Verify run association exists
	var assocCount int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM probe_batch_runs WHERE batch_id = ? AND probe_run_id = ?;", batch.ID, run1.ID).Scan(&assocCount)
	if err != nil || assocCount != 1 {
		t.Fatalf("expected 1 row in probe_batch_runs, got %d (err: %v)", assocCount, err)
	}

	// 2. Duplicate batch creation with same (generation, window_at) must return conflict
	dupBatch := &domain.ProbeBatch{
		ID:         domain.MustNewUUIDv7(),
		WindowAt:   window,
		Generation: 1,
		State:      domain.ProbeBatchStatePending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repo.CreateBatch(ctx, dupBatch); err == nil {
		t.Fatalf("expected conflict on duplicate (generation, window_at), got nil")
	}

	// 3. GetBatchByID
	fetched, err := repo.GetBatchByID(ctx, batch.ID)
	if err != nil {
		t.Fatalf("failed to get batch by id: %v", err)
	}
	if fetched.ID != batch.ID || fetched.Generation != 1 || fetched.Counts.TotalNodes != 10 {
		t.Fatalf("unexpected fetched batch: %+v", fetched)
	}
	if len(fetched.RunIDs) != 1 || fetched.RunIDs[0] != run1.ID {
		t.Fatalf("expected runIDs [%s], got %+v", run1.ID, fetched.RunIDs)
	}

	// 4. GetBatchByWindow
	byWindow, err := repo.GetBatchByWindow(ctx, 1, window)
	if err != nil {
		t.Fatalf("failed to get batch by window: %v", err)
	}
	if byWindow.ID != batch.ID {
		t.Fatalf("expected batch id %s, got %s", batch.ID, byWindow.ID)
	}

	// 5. ListBatches
	batches, total, err := repo.ListBatches(ctx, 1, 10)
	if err != nil {
		t.Fatalf("failed to list batches: %v", err)
	}
	if total != 1 || len(batches) != 1 {
		t.Fatalf("expected 1 batch, got total=%d len=%d", total, len(batches))
	}

	// 6. CAS Lease: acquire lease by instance-1
	acquired, err := repo.AcquireLease(ctx, batch.ID, "instance-1", 2*time.Second)
	if err != nil {
		t.Fatalf("failed to acquire lease: %v", err)
	}
	if !acquired {
		t.Fatalf("expected lease acquisition to succeed for instance-1")
	}

	// 7. CAS Lease: instance-2 tries to acquire active lease -> must fail (false)
	acquired2, err := repo.AcquireLease(ctx, batch.ID, "instance-2", 2*time.Second)
	if err != nil {
		t.Fatalf("failed to attempt acquire lease: %v", err)
	}
	if acquired2 {
		t.Fatalf("expected instance-2 to fail acquiring active lease held by instance-1")
	}

	// 8. Re-acquire by same instance-1 -> should succeed
	acquired1Again, err := repo.AcquireLease(ctx, batch.ID, "instance-1", 5*time.Second)
	if err != nil || !acquired1Again {
		t.Fatalf("expected instance-1 to re-acquire lease successfully, got %v (err: %v)", acquired1Again, err)
	}

	// 9. Heartbeat lease by owner instance-1
	if err := repo.HeartbeatLease(ctx, batch.ID, "instance-1", 5*time.Second); err != nil {
		t.Fatalf("expected heartbeat to succeed: %v", err)
	}

	// 10. Heartbeat by wrong owner instance-2 -> conflict error (lost_lease)
	if err := repo.HeartbeatLease(ctx, batch.ID, "instance-2", 5*time.Second); err == nil {
		t.Fatalf("expected heartbeat from wrong owner to fail with conflict, got nil")
	}

	// 11. Release lease by instance-1
	if err := repo.ReleaseLease(ctx, batch.ID, "instance-1"); err != nil {
		t.Fatalf("failed to release lease: %v", err)
	}

	// 12. Instance-2 can now acquire the released lease
	acquired2AfterRelease, err := repo.AcquireLease(ctx, batch.ID, "instance-2", 2*time.Second)
	if err != nil || !acquired2AfterRelease {
		t.Fatalf("expected instance-2 to acquire released lease, got %v (err: %v)", acquired2AfterRelease, err)
	}

	// 13. Update batch state to terminal (succeeded)
	batch.State = domain.ProbeBatchStateSucceeded
	batch.Counts.CompletedRuns = 1
	if err := repo.UpdateBatch(ctx, batch); err != nil {
		t.Fatalf("failed to update batch: %v", err)
	}

	// 14. Terminal batch cannot acquire lease
	acquiredAfterTerminal, err := repo.AcquireLease(ctx, batch.ID, "instance-3", 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error acquiring lease on terminal batch: %v", err)
	}
	if acquiredAfterTerminal {
		t.Fatalf("expected lease acquisition on terminal batch to fail (false)")
	}
}

func TestConcurrentLeaseContention(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	repo := sqlite.NewProbeScheduleRepository(db)

	now := time.Now().UTC().Truncate(time.Second)
	batch := &domain.ProbeBatch{
		ID:         domain.MustNewUUIDv7(),
		WindowAt:   now,
		Generation: 1,
		State:      domain.ProbeBatchStatePending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repo.CreateBatch(ctx, batch); err != nil {
		t.Fatalf("failed to create batch: %v", err)
	}

	const workerCount = 10
	var wg sync.WaitGroup
	winners := make([]string, 0, workerCount)
	var winMu sync.Mutex

	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		workerID := fmt.Sprintf("worker-%d", i)
		go func(id string) {
			defer wg.Done()
			won, err := repo.AcquireLease(ctx, batch.ID, id, 10*time.Second)
			if err != nil {
				t.Errorf("worker %s error acquiring lease: %v", id, err)
				return
			}
			if won {
				winMu.Lock()
				winners = append(winners, id)
				winMu.Unlock()
			}
		}(workerID)
	}
	wg.Wait()

	if len(winners) != 1 {
		t.Fatalf("expected exactly 1 winner in concurrent lease acquisition, got %d: %+v", len(winners), winners)
	}
}
