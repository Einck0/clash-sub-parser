package policy_test

import (
	"context"
	"errors"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestURLTestGroupTypeChangeClearsInactiveSettings(t *testing.T) {
	for _, target := range []domain.GroupType{domain.GroupTypeSelect, domain.GroupTypeFallback, domain.GroupTypeLoadBalance} {
		t.Run(string(target), func(t *testing.T) {
			db := setupTestDB(t)
			svc, _ := setupTestService(t, db)
			ctx := context.Background()
			interval, tolerance := 120, 20
			builtin := "DIRECT"
			created, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{
				Name: "Auto", GroupType: domain.GroupTypeURLTest,
				URLTestInterval: &interval, URLTestTolerance: &tolerance,
				Sources: []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindTypedBuiltin, BuiltinName: &builtin}},
			})
			if err != nil {
				t.Fatal(err)
			}
			// Explicit URL-Test input for a different type remains an actionable error.
			_, err = svc.UpdateGroup(ctx, policy.UpdateGroupCommand{ID: created.ID, GroupType: &target, URLTestInterval: &interval})
			var de *domain.DomainError
			if !errors.As(err, &de) || de.Code != "invalid_urltest_settings" {
				t.Fatalf("expected dedicated-setting rejection, got %v", err)
			}
			repo := sqlite.NewPolicyRepository(db)
			before, err := repo.GetGroupByID(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if before.GroupType != domain.GroupTypeURLTest || before.Revision != created.Revision || before.URLTestInterval == nil || *before.URLTestInterval != interval || before.URLTestTolerance == nil || *before.URLTestTolerance != tolerance {
				t.Fatalf("rejected update mutated settings: %+v", before)
			}
			// The real editor sends type, sources and revision, but omits inactive settings.
			updated, err := svc.UpdateGroup(ctx, policy.UpdateGroupCommand{
				ID: created.ID, GroupType: &target, ExpectedRevision: &created.Revision, Sources: &created.Sources,
			})
			if err != nil {
				t.Fatalf("changing configured URL-Test to %s failed: %v", target, err)
			}
			if updated.GroupType != target || updated.URLTestInterval != nil || updated.URLTestTolerance != nil {
				t.Fatalf("response retained inactive settings: %+v", updated)
			}
			after, err := repo.GetGroupByID(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.GroupType != target || after.URLTestInterval != nil || after.URLTestTolerance != nil || after.Revision != created.Revision+1 || len(after.Sources) != 1 || after.Sources[0].ID != created.Sources[0].ID {
				t.Fatalf("type change lost sources or persisted inactive settings: %+v", after)
			}
		})
	}
}
