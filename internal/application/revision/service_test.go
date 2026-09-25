package revision_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func setupRevisionDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(sqlite.Config{
		Path:        fmt.Sprintf("file:test_revision_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		ForeignKeys: true,
		BusyTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertRevision(t *testing.T, db *sql.DB, id string, state domain.ConfigurationRevisionState) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO configuration_revisions (id, content_digest, state, created_at) VALUES (?, ?, ?, ?)`, id, "digest-"+id, state, "2026-09-16T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
}

func TestServiceReviewsOnlyDraftAndActivatesExplicitly(t *testing.T) {
	db := setupRevisionDB(t)
	repo := sqlite.NewRevisionRepository(db)
	audit := sqlite.NewAuditRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	svc := revision.NewService(repo, audit, revision.WithPolicyRepository(policyRepo))

	insertRevision(t, db, "draft-1", domain.RevisionStateDraft)

	// Review draft
	got, err := svc.Review(context.Background(), "draft-1", revision.Action{RequestID: "req-review", ActorKind: domain.ActorKindAdmin})
	if err != nil || got.State != domain.RevisionStateDraft {
		t.Fatalf("review must preserve draft state: got=%+v err=%v", got, err)
	}

	// Verify unactivated draft does NOT appear as active
	active, err := repo.GetActive(context.Background())
	if err == nil || active != nil {
		t.Fatalf("review must not create an active revision: got=%+v err=%v", active, err)
	}

	// Verify review audit event
	revEvents, _, err := audit.List(context.Background(), domain.AuditFilter{Action: "revision.review", Pagination: domain.Pagination{Page: 1, PageSize: 10}})
	if err != nil || len(revEvents) != 1 || revEvents[0].RequestID != "req-review" {
		t.Fatalf("review audit missing: events=%+v err=%v", revEvents, err)
	}

	// Explicit activation
	activated, err := svc.Activate(context.Background(), "draft-1", revision.Action{RequestID: "req-activate", ActorKind: domain.ActorKindAdmin})
	if err != nil || activated.State != domain.RevisionStateActive {
		t.Fatalf("activate must transition to active: got=%+v err=%v", activated, err)
	}

	// Verify active revision is now draft-1
	active, err = repo.GetActive(context.Background())
	if err != nil || active.ID != "draft-1" {
		t.Fatalf("explicit activation must select draft: got=%+v err=%v", active, err)
	}

	// Verify activation audit event
	actEvents, _, err := audit.List(context.Background(), domain.AuditFilter{Action: "revision.activate", Pagination: domain.Pagination{Page: 1, PageSize: 10}})
	if err != nil || len(actEvents) != 1 || actEvents[0].RequestID != "req-activate" {
		t.Fatalf("activation audit missing: events=%+v err=%v", actEvents, err)
	}
}

func TestServiceRejectsReviewAndActivationForNonDraft(t *testing.T) {
	db := setupRevisionDB(t)
	repo := sqlite.NewRevisionRepository(db)
	audit := sqlite.NewAuditRepository(db)
	svc := revision.NewService(repo, audit)

	insertRevision(t, db, "active-1", domain.RevisionStateActive)
	insertRevision(t, db, "archived-1", domain.RevisionStateArchived)

	for _, id := range []string{"active-1", "archived-1"} {
		if _, err := svc.Review(context.Background(), id, revision.Action{RequestID: "req-fail"}); err == nil {
			t.Errorf("review of %s should fail", id)
		}
		if _, err := svc.Activate(context.Background(), id, revision.Action{RequestID: "req-fail"}); err == nil {
			t.Errorf("activation of %s should fail", id)
		}
	}
}

func TestServiceActivationValidatesPolicyGraph(t *testing.T) {
	db := setupRevisionDB(t)
	repo := sqlite.NewRevisionRepository(db)
	audit := sqlite.NewAuditRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	svc := revision.NewService(repo, audit, revision.WithPolicyRepository(policyRepo))

	// Insert invalid group cycle into policy tables
	grpAID := domain.MustNewUUIDv7()
	grpBID := domain.MustNewUUIDv7()
	grpA := domain.NodeGroup{ID: grpAID, Name: "Group A", GroupType: domain.GroupTypeSelect, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	grpB := domain.NodeGroup{ID: grpBID, Name: "Group B", GroupType: domain.GroupTypeSelect, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := policyRepo.CreateGroup(context.Background(), &grpA); err != nil {
		t.Fatal(err)
	}
	if err := policyRepo.CreateGroup(context.Background(), &grpB); err != nil {
		t.Fatal(err)
	}

	// Create cycle: A -> B and B -> A
	if err := policyRepo.SetEdgesForGroup(context.Background(), grpAID, []domain.GroupEdge{
		{ID: domain.MustNewUUIDv7(), ParentGroupID: grpAID, ChildGroupID: &grpBID, Position: 0},
	}); err != nil {
		t.Fatal(err)
	}
	if err := policyRepo.SetEdgesForGroup(context.Background(), grpBID, []domain.GroupEdge{
		{ID: domain.MustNewUUIDv7(), ParentGroupID: grpBID, ChildGroupID: &grpAID, Position: 0},
	}); err != nil {
		t.Fatal(err)
	}

	insertRevision(t, db, "draft-invalid-graph", domain.RevisionStateDraft)

	// Activation should fail due to cycle detection
	_, err := svc.Activate(context.Background(), "draft-invalid-graph", revision.Action{RequestID: "req-invalid-activate", ActorKind: domain.ActorKindAdmin})
	if err == nil {
		t.Fatal("expected activation of cyclic policy graph to fail")
	}

	// Verify revision was NOT activated
	active, err := repo.GetActive(context.Background())
	if err == nil || active != nil {
		t.Fatalf("invalid revision must not be active: got=%+v", active)
	}
}

func TestServiceListPaginationAndFilter(t *testing.T) {
	db := setupRevisionDB(t)
	repo := sqlite.NewRevisionRepository(db)
	svc := revision.NewService(repo, nil)

	insertRevision(t, db, "rev-1", domain.RevisionStateArchived)
	insertRevision(t, db, "rev-2", domain.RevisionStateActive)
	insertRevision(t, db, "rev-3", domain.RevisionStateDraft)
	insertRevision(t, db, "rev-4", domain.RevisionStateDraft)

	// List all
	items, total, err := svc.List(context.Background(), domain.RevisionFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
	})
	if err != nil || total != 4 || len(items) != 4 {
		t.Fatalf("expected 4 total revisions, got total=%d len=%d err=%v", total, len(items), err)
	}

	// Filter by draft
	draftState := domain.RevisionStateDraft
	drafts, draftTotal, err := svc.List(context.Background(), domain.RevisionFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 10},
		State:      &draftState,
	})
	if err != nil || draftTotal != 2 || len(drafts) != 2 {
		t.Fatalf("expected 2 draft revisions, got total=%d len=%d err=%v", draftTotal, len(drafts), err)
	}
}

func TestServiceConcurrentActivationsRaceSafety(t *testing.T) {
	db := setupRevisionDB(t)
	repo := sqlite.NewRevisionRepository(db)
	audit := sqlite.NewAuditRepository(db)
	svc := revision.NewService(repo, audit)

	for i := 1; i <= 10; i++ {
		insertRevision(t, db, fmt.Sprintf("draft-conc-%d", i), domain.RevisionStateDraft)
	}

	var wg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		id := fmt.Sprintf("draft-conc-%d", i)
		go func(revID string) {
			defer wg.Done()
			_, _ = svc.Review(context.Background(), revID, revision.Action{RequestID: "req-conc"})
			_, _ = svc.Activate(context.Background(), revID, revision.Action{RequestID: "req-conc"})
		}(id)
	}
	wg.Wait()

	active, err := repo.GetActive(context.Background())
	if err != nil || active == nil {
		t.Fatalf("expected an active revision after concurrent activations: err=%v", err)
	}
}
