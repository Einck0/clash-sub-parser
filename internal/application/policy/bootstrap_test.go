package policy_test

import (
	"context"
	"errors"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestPolicyRevisionBootstrapWithSeed_RepositoryReadFailureDoesNotSeed(t *testing.T) {
	for _, stage := range []string{"active", "list"} {
		t.Run(stage, func(t *testing.T) {
			db := setupTestDB(t)
			wantErr := errors.New("revision " + stage + " read failed")
			repo := &bootstrapReadFailureRepository{
				RevisionRepository: sqlite.NewRevisionRepository(db),
				stage:              stage,
				err:                wantErr,
			}
			svc := policy.NewService(sqlite.NewPolicyRepository(db), repo, sqlite.NewNodeRepository(db), nil)

			rev, err := svc.EnsureActiveRevision(context.Background(), policy.WithDefaultRulesSeed())
			if rev != nil || !errors.Is(err, wantErr) {
				t.Fatalf("read failure must propagate: rev=%+v err=%v", rev, err)
			}
			if repo.seedCalled {
				t.Fatal("repository read failure was treated as a fresh database")
			}
			for _, table := range []string{"configuration_revisions", "policy_revision_snapshots", "policy_rules"} {
				var count int
				if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("read failure changed %s: count=%d err=%v", table, count, err)
				}
			}
		})
	}
}

type bootstrapReadFailureRepository struct {
	domain.RevisionRepository
	stage      string
	err        error
	seedCalled bool
}

func (r *bootstrapReadFailureRepository) GetActive(ctx context.Context) (*domain.ConfigurationRevision, error) {
	if r.stage == "active" {
		return nil, r.err
	}
	return r.RevisionRepository.GetActive(ctx)
}

func (r *bootstrapReadFailureRepository) List(ctx context.Context, filter domain.RevisionFilter) ([]domain.ConfigurationRevision, int, error) {
	if r.stage == "list" {
		return nil, 0, r.err
	}
	return r.RevisionRepository.List(ctx, filter)
}

func (r *bootstrapReadFailureRepository) CreateActiveWithSeed(context.Context, *domain.ConfigurationRevision) error {
	r.seedCalled = true
	return errors.New("unexpected initial seed after read failure")
}
