package policy_test

import (
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"context"
	"testing"
)

func TestGroupEmptyPassPatchTristateAndInvalidAtomic(t *testing.T) {
	db := setupTestDB(t)
	svc, _ := setupTestService(t, db)
	ctx := context.Background()
	group, err := svc.CreateGroup(ctx, policy.CreateGroupCommand{Name: "strict", GroupType: domain.GroupTypeSelect})
	if err != nil {
		t.Fatal(err)
	}
	if group.EmptyFallbackPass {
		t.Fatal("new group default opt-in")
	}
	yes, no := true, false
	group, err = svc.UpdateGroup(ctx, policy.UpdateGroupCommand{ID: group.ID, EmptyFallbackPass: &yes})
	if err != nil || !group.EmptyFallbackPass {
		t.Fatalf("enable: %+v %v", group, err)
	}
	group, err = svc.UpdateGroup(ctx, policy.UpdateGroupCommand{ID: group.ID})
	if err != nil || !group.EmptyFallbackPass {
		t.Fatal("omission reset flag")
	}
	name := "should not persist"
	bad := &domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpRegex, Value: "("}}}
	var before, after int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM configuration_revisions`).Scan(&before)
	if _, err := svc.UpdateGroup(ctx, policy.UpdateGroupCommand{ID: group.ID, Name: &name, EmptyFallbackPass: &no, NodeFilter: bad}); err == nil {
		t.Fatal("invalid regex accepted")
	}
	persisted, err := svc.GetGroup(ctx, group.ID)
	if err != nil || persisted.Name != "strict" || !persisted.EmptyFallbackPass {
		t.Fatalf("invalid update partially saved: %+v %v", persisted, err)
	}
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM configuration_revisions`).Scan(&after)
	if after != before {
		t.Fatal("invalid update created revision")
	}
	group, err = svc.UpdateGroup(ctx, policy.UpdateGroupCommand{ID: group.ID, EmptyFallbackPass: &no})
	if err != nil || group.EmptyFallbackPass {
		t.Fatal("explicit false ignored")
	}
}
